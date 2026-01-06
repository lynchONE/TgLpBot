package auto_lp

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/clickhouse"
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"TgLpBot/service/smart_lp"
	"TgLpBot/service/strategy"
	"TgLpBot/service/user"
	"TgLpBot/service/wallet"
	"context"
	"fmt"
	"log"
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type AutoLPService struct {
	poolm         *PoolMClient
	ch            *clickhouse.ClickHouseService
	liquidity     *liquidity.LiquidityService
	poolService   *pool.PoolService
	accessService *user.AccessService
	configService *user.GlobalConfigService
	smartLP       *smart_lp.SmartLPService

	stopChan chan struct{}
	ticker   *time.Ticker

	notifier func(userID uint, message string)

	lastRunAt    time.Time
	lastRunError string
}

func NewAutoLPService(ch *clickhouse.ClickHouseService) *AutoLPService {
	interval := 60 * time.Second
	if config.AppConfig != nil && config.AppConfig.AutoLPScanIntervalSeconds > 0 {
		interval = time.Duration(config.AppConfig.AutoLPScanIntervalSeconds) * time.Second
	}

	baseURL := ""
	if config.AppConfig != nil {
		baseURL = strings.TrimSpace(config.AppConfig.AutoLPPoolMBaseURL)
	}

	return &AutoLPService{
		poolm:         NewPoolMClient(baseURL),
		ch:            ch,
		liquidity:     liquidity.NewLiquidityService(),
		poolService:   pool.NewPoolService(),
		accessService: user.NewAccessService(),
		configService: user.NewGlobalConfigService(),
		smartLP:       smart_lp.NewSmartLPService(ch),
		stopChan:      make(chan struct{}),
		ticker:        time.NewTicker(interval),
	}
}

func (s *AutoLPService) SetNotifier(fn func(userID uint, message string)) {
	s.notifier = fn
}

func (s *AutoLPService) Start() {
	if config.AppConfig == nil || !config.AppConfig.AutoLPEnabled {
		log.Println("[AutoLP] disabled (AUTO_LP_ENABLED=0)")
		return
	}
	go s.runLoop()
}

func (s *AutoLPService) Stop() {
	if s == nil {
		return
	}
	select {
	case <-s.stopChan:
	default:
		close(s.stopChan)
	}
	if s.ticker != nil {
		s.ticker.Stop()
	}
}

func (s *AutoLPService) runLoop() {
	log.Println("[AutoLP] service started")
	for {
		select {
		case <-s.ticker.C:
			s.runOnce()
		case <-s.stopChan:
			log.Println("[AutoLP] service stopped")
			return
		}
	}
}

func (s *AutoLPService) runOnce() {
	start := time.Now()
	s.lastRunAt = start
	s.lastRunError = ""
	defer func() {
		if strings.TrimSpace(s.lastRunError) != "" {
			log.Printf("[AutoLP] 扫描结束：失败 err=%s 用时=%s", s.lastRunError, time.Since(start).String())
			return
		}
		log.Printf("[AutoLP] 扫描结束：成功 用时=%s", time.Since(start).String())
	}()

	if config.AppConfig == nil {
		s.lastRunError = "config not loaded"
		return
	}
	if database.DB == nil {
		s.lastRunError = "database not initialized"
		return
	}
	if s.ch == nil || s.ch.Conn == nil {
		s.lastRunError = "clickhouse not initialized"
		return
	}

	dexes := autoLPDexList(config.AppConfig.AutoLPProtocols)
	dexParam := strings.Join(dexes, ",")

	log.Printf("[AutoLP] 开始扫描：链=%s DEX=%s 扫描间隔=%ds 请求间隔=%dms 自动开仓=%v Top推送=%v 调试=%v；硬筛：TVL(current_pool_value,USD)>%.0f 费率(fee_percentage)>%.2f%% 5m费用率(total_fees/current_pool_value)>%.4f%% 5m手续费(total_fees)>%.2f 5m成交量(total_volume)>%.2f；开仓宽度(总宽度)：震荡=%.2f%% 温和上涨=%.2f%% 急涨=%.2f%%",
		strings.ToLower(strings.TrimSpace(config.AppConfig.AutoLPChain)),
		dexParam,
		config.AppConfig.AutoLPScanIntervalSeconds,
		config.AppConfig.AutoLPFetchDelayMillis,
		config.AppConfig.AutoLPExecuteEnabled,
		config.AppConfig.AutoLPNotifyTopCandidate,
		config.AppConfig.AutoLPDebug,
		config.AppConfig.AutoLPMinPoolValueUSD,
		config.AppConfig.AutoLPMinFeePercentage,
		config.AppConfig.AutoLPMinFeeRate5m,
		config.AppConfig.AutoLPMinTotalFees5m,
		config.AppConfig.AutoLPMinTotalVolume5m,
		config.AppConfig.AutoLPWidthSidewaysPercent,
		config.AppConfig.AutoLPWidthMildUptrendPercent,
		config.AppConfig.AutoLPWidthRapidPumpPercent,
	)
	log.Printf("[AutoLP] 共振门槛：5m费用率>=%.4f%% 5m成交量>=%.2f |Z60|>=%.2f",
		config.AppConfig.AutoLPResonanceMinFeeRate5m,
		config.AppConfig.AutoLPResonanceMinTotalVolume5m,
		config.AppConfig.AutoLPResonanceMinAbsZ60,
	)

	timeout := 55 * time.Second
	if config.AppConfig.AutoLPScanIntervalSeconds > 0 {
		t := time.Duration(config.AppConfig.AutoLPScanIntervalSeconds) * time.Second * 3
		if t > timeout {
			timeout = t
		}
	}
	if timeout > 5*time.Minute {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	enabledCfgs := make([]models.AutoLPUserConfig, 0)
	if cfgs, err := NewAutoLPUserConfigService().ListEnabled(); err != nil {
		log.Printf("[AutoLP] list enabled user configs failed: %v", err)
	} else {
		enabledCfgs = cfgs
	}

	legacyUserID := uint(0)
	if len(enabledCfgs) == 0 {
		// Backward compatibility: fallback to single target user if configured.
		if uid, err := s.resolveTargetUserID(); err == nil {
			var cfgCount int64
			if err := database.DB.Model(&models.AutoLPUserConfig{}).Where("user_id = ?", uid).Count(&cfgCount).Error; err != nil {
				if config.AppConfig.AutoLPDebug {
					log.Printf("[AutoLP] legacy target user config check failed: %v", err)
				}
			}
			if cfgCount == 0 {
				legacyUserID = uid
				if config.AppConfig.AutoLPDebug {
					log.Printf("[AutoLP] 目标用户(legacy)：user_id=%d", legacyUserID)
				}
			} else if config.AppConfig.AutoLPDebug {
				log.Printf("[AutoLP] legacy target user has config; skip fallback")
			}
		} else if config.AppConfig.AutoLPDebug {
			log.Printf("[AutoLP] legacy target user not resolved: %v", err)
		}
	}

	extraNotifyCache := make(map[uint]bool)
	extraNotificationsEnabled := func(userID uint) bool {
		if v, ok := extraNotifyCache[userID]; ok {
			return v
		}
		enabled := true
		if s.configService != nil {
			if cfg, err := s.configService.GetOrCreate(userID); err == nil {
				enabled = cfg.ExtraNotificationsEnabled
			} else if config.AppConfig.AutoLPDebug {
				log.Printf("[AutoLP] get global config failed: user_id=%d err=%v", userID, err)
			}
		}
		extraNotifyCache[userID] = enabled
		return enabled
	}

	notifyCore := func(msg string) {
		if len(enabledCfgs) > 0 {
			for i := range enabledCfgs {
				s.notify(enabledCfgs[i].UserID, msg)
			}
			return
		}
		if legacyUserID > 0 {
			s.notify(legacyUserID, msg)
		}
	}

	notifyExtra := func(msg string) {
		if len(enabledCfgs) > 0 {
			for i := range enabledCfgs {
				userID := enabledCfgs[i].UserID
				if !extraNotificationsEnabled(userID) {
					continue
				}
				s.notify(userID, msg)
			}
			return
		}
		if legacyUserID > 0 && extraNotificationsEnabled(legacyUserID) {
			s.notify(legacyUserID, msg)
		}
	}

	snap, rawRows, err := s.fetchPoolMSnapshot(ctx)
	if err != nil {
		s.lastRunError = err.Error()
		notifyCore(fmt.Sprintf("❌ AutoLP 扫描失败：%v", err))
		return
	}

	if err := s.insertPoolMRaw(ctx, rawRows); err != nil {
		s.lastRunError = err.Error()
		notifyCore(fmt.Sprintf("❌ AutoLP 写入 ClickHouse 失败：%v", err))
		return
	}

	if err := s.replacePoolMRealtime(ctx, rawRows); err != nil {
		log.Printf("[AutoLP] PoolM 实时表刷新失败: %v", err)
	}

	log.Printf("[AutoLP] PoolM 数据入库：原始记录=%d（协议×周期×池子）去重池子=%d（协议+池子地址）", len(rawRows), len(snap.data))

	analyses, err := s.analyzeSnapshot(ctx, snap)
	if err != nil {
		s.lastRunError = err.Error()
		notifyCore(fmt.Sprintf("❌ AutoLP 分析失败：%v", err))
		return
	}

	_ = s.insertAnalysis(ctx, analyses)

	candidateCount := 0
	var topCand AutoLPAnalysis
	foundTop := false
	for _, a := range analyses {
		if a.Action != "CANDIDATE" {
			continue
		}
		candidateCount++
		if !foundTop {
			topCand = a
			foundTop = true
		}
	}
	log.Printf("[AutoLP] 分析完成：总记录=%d 候选=%d", len(analyses), candidateCount)

	if config.AppConfig != nil && config.AppConfig.SmartLPEnabled && config.AppConfig.SmartLPMinWallets > 0 {
		for _, a := range analyses {
			// 使用最近时间窗口内添加LP的钱包数进行判断
			if a.RecentAddWalletCount < config.AppConfig.SmartLPMinWallets {
				continue
			}
			if a.CandidateEligible {
				continue
			}
			pair := strings.TrimSpace(a.TradingPair)
			addr := strings.TrimSpace(a.PoolAddress)
			if pair == "" {
				pair = addr
			} else if addr != "" {
				pair = fmt.Sprintf("%s (%s)", pair, addr)
			}
			windowMinutes := config.AppConfig.SmartLPRecentWindowMinutes
			if windowMinutes <= 0 {
				windowMinutes = 10
			}
			notifyExtra(fmt.Sprintf("⚠️ 最近%d分钟内监测到 %d 个钱包在池子 %s 加LP，但未满足 AutoLP 规则，未自动开仓。", windowMinutes, a.RecentAddWalletCount, pair))
		}
	}

	if config.AppConfig.AutoLPNotifyTopCandidate && foundTop {
		msg := fmt.Sprintf(
			"📡 AutoLP 候选池：%d\nTop1：%s %s\n地址：%s\n5m 手续费=%.2f | 手续费率=%.2f%% | 5m 成交量=%.2f | TVL=%.2f | 5m 费用率=%.4f%%\nZ5=%.2f 状态=%s\nZ60=%.2f 趋势=%s\n共振=%s\n宽度：%.2f%%（下 %.2f%% / 上 %.2f%%）\n评分：%.2f\nZ5/Z60：价格 Z-score，Z=(P-MA)/σ；MA/σ 分别统计最近 5/60 分钟 current_token_price（minN=4/12，60m 不足则用 5m 数据窗口 60 分钟回退）",
			candidateCount,
			strings.ToUpper(topCand.ProtocolVersion), topCand.TradingPair,
			topCand.PoolAddress,
			topCand.TotalFees5m, topCand.FeePercentage, topCand.TotalVolume5m, topCand.TVLUSD, topCand.FeeRate5mPct,
			topCand.Z5, autoLPStateZh(topCand.State5),
			topCand.Z60, autoLPTrendZh(topCand.Trend60),
			autoLPResonanceZh(topCand.Resonance),
			topCand.BaseWidthPct, topCand.LowerWidthPct, topCand.UpperWidthPct,
			topCand.Score,
		)
		notifyExtra(msg)
	}

	s.guardActiveAutoTasks(ctx, snap, analyses)

	if !config.AppConfig.AutoLPExecuteEnabled {
		return
	}

	if len(enabledCfgs) > 0 {
		for i := range enabledCfgs {
			cfg := enabledCfgs[i]
			if ok, _ := s.applyUserStopConditions(ctx, cfg); ok {
				continue
			}
			if err := s.executeBestCandidateForUser(ctx, cfg, snap, analyses); err != nil {
				s.notify(cfg.UserID, fmt.Sprintf("❌ AutoLP 自动开仓失败：%v", err))
			}
		}
		return
	}

	// Legacy single-user execution.
	if legacyUserID > 0 {
		if err := s.executeBestCandidate(ctx, legacyUserID, snap, analyses); err != nil {
			s.notify(legacyUserID, fmt.Sprintf("❌ AutoLP 自动开仓失败：%v", err))
		}
	}
}

func (s *AutoLPService) notify(userID uint, message string) {
	if s.notifier == nil {
		return
	}
	s.notifier(userID, message)
}

type poolKey struct {
	proto string
	addr  string
}

type poolMSnapshot struct {
	at    time.Time
	chain string
	// data[key][timeframeMinutes]
	data map[poolKey]map[int]PoolMFeePool
}

type poolMRawRow struct {
	ts                 time.Time
	chain              string
	requestedChain     string
	protocolVersion    string
	timeframe          int
	timeframeLabel     string
	requestedProtocols []string
	requestedDexes     []string
	totalPools         int
	responseSuccess    bool
	responseError      string
	p                  PoolMFeePool
	lastSwapAt         time.Time
}

func (s *AutoLPService) fetchPoolMSnapshot(ctx context.Context) (*poolMSnapshot, []poolMRawRow, error) {
	chain := strings.ToLower(strings.TrimSpace(config.AppConfig.AutoLPChain))
	if chain == "" {
		chain = "bsc"
	}
	dexes := autoLPDexList(config.AppConfig.AutoLPProtocols)
	dexParam := strings.Join(dexes, ",")
	timeframes := []int{5, 60}

	delay := time.Duration(config.AppConfig.AutoLPFetchDelayMillis) * time.Millisecond
	if delay < 0 {
		delay = 0
	}

	log.Printf("[AutoLP] PoolM 拉取：链=%s DEX=%v 周期=%v 请求间隔=%s", chain, dexes, timeframes, delay.String())

	now := time.Now()
	out := &poolMSnapshot{
		at:    now,
		chain: chain,
		data:  make(map[poolKey]map[int]PoolMFeePool),
	}
	var rows []poolMRawRow

	for _, tf := range timeframes {
		resp, err := s.poolm.TopFees(ctx, tf, chain, dexParam)
		if err != nil {
			return nil, nil, err
		}
		log.Printf("[AutoLP] PoolM 返回：DEX=%s 周期=%dmin 本次返回=%d Top总数=%d", dexParam, tf, len(resp.Data), resp.TotalPools)

		requestedChain := strings.TrimSpace(resp.RequestedChain)
		if requestedChain == "" {
			requestedChain = chain
		}
		timeframeLabel := strings.TrimSpace(resp.Timeframe)
		requestedProtocols := []string(resp.RequestedProtocol)
		if requestedProtocols == nil {
			requestedProtocols = []string{}
		}
		requestedDexes := []string(resp.RequestedDex)
		if requestedDexes == nil {
			requestedDexes = []string{}
		}
		totalPools := resp.TotalPools
		responseSuccess := resp.Success
		responseError := strings.TrimSpace(resp.Error)

		for _, p := range resp.Data {
			addr := strings.ToLower(strings.TrimSpace(p.PoolAddress))
			if addr == "" {
				continue
			}
			proto := normalizePoolMProtocolVersion(p, addr)
			if proto == "" {
				continue
			}

			key := poolKey{
				proto: proto,
				addr:  addr,
			}

			if _, ok := out.data[key]; !ok {
				out.data[key] = make(map[int]PoolMFeePool)
			}
			out.data[key][tf] = p

			lastSwap := time.Time{}
			if strings.TrimSpace(p.LastSwapAt) != "" {
				if t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(p.LastSwapAt)); err == nil {
					lastSwap = t
				}
			}
			rows = append(rows, poolMRawRow{
				ts:                 now,
				chain:              chain,
				requestedChain:     requestedChain,
				protocolVersion:    key.proto,
				timeframe:          tf,
				timeframeLabel:     timeframeLabel,
				requestedProtocols: requestedProtocols,
				requestedDexes:     requestedDexes,
				totalPools:         totalPools,
				responseSuccess:    responseSuccess,
				responseError:      responseError,
				p:                  p,
				lastSwapAt:         lastSwap,
			})
		}

		if delay > 0 {
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return out, rows, nil
}

func (s *AutoLPService) insertPoolMRaw(ctx context.Context, rows []poolMRawRow) error {
	if s.ch == nil || s.ch.Conn == nil {
		return fmt.Errorf("clickhouse not initialized")
	}
	if len(rows) == 0 {
		return nil
	}

	batch, err := s.ch.PrepareBatch(ctx, `INSERT INTO poolm_top_fees_raw (
		ts, chain, requested_chain, protocol_version, dex, timeframe_minutes, timeframe_label,
		requested_protocols, requested_dexes, total_pools, response_success, response_error,
		pool_address, factory_name, factory_address, trading_pair,
		token0_symbol, token1_symbol, token0_name, token1_name, token0_address, token1_address,
		token0_decimals, token1_decimals, stable_coin_symbol,
		fee_rate, fee_percentage, transaction_count,
		total_fees, total_volume, current_pool_value,
		current_token0_balance, current_token1_balance,
		current_token_price, price_display, last_swap_at
	)`)
	if err != nil {
		return err
	}

	for _, r := range rows {
		p := r.p
		responseSuccess := uint8(0)
		if r.responseSuccess {
			responseSuccess = 1
		}
		if err := batch.Append(
			r.ts,
			r.chain,
			strings.TrimSpace(r.requestedChain),
			r.protocolVersion,
			strings.TrimSpace(p.Dex),
			uint16(r.timeframe),
			strings.TrimSpace(r.timeframeLabel),
			r.requestedProtocols,
			r.requestedDexes,
			uint32(maxInt(r.totalPools, 0)),
			responseSuccess,
			strings.TrimSpace(r.responseError),
			strings.ToLower(strings.TrimSpace(p.PoolAddress)),
			strings.TrimSpace(p.FactoryName),
			strings.ToLower(strings.TrimSpace(p.FactoryAddress)),
			strings.TrimSpace(p.TradingPair),
			strings.TrimSpace(p.Token0Symbol),
			strings.TrimSpace(p.Token1Symbol),
			strings.TrimSpace(p.Token0Name),
			strings.TrimSpace(p.Token1Name),
			strings.ToLower(strings.TrimSpace(p.Token0Address)),
			strings.ToLower(strings.TrimSpace(p.Token1Address)),
			uint8(maxInt(p.Token0Decimals, 0)),
			uint8(maxInt(p.Token1Decimals, 0)),
			strings.TrimSpace(p.StableCoinSymbol),
			uint32(maxInt(p.FeeRate, 0)),
			p.FeePercentage,
			uint32(maxInt(p.TransactionCount, 0)),
			p.TotalFees,
			p.TotalVolume,
			p.CurrentPoolValue,
			p.CurrentToken0Balance,
			p.CurrentToken1Balance,
			p.CurrentTokenPrice,
			strings.TrimSpace(p.PriceDisplay),
			r.lastSwapAt,
		); err != nil {
			return err
		}
	}
	return batch.Send()
}

func (s *AutoLPService) replacePoolMRealtime(ctx context.Context, rows []poolMRawRow) error {
	if s.ch == nil || s.ch.Conn == nil {
		return fmt.Errorf("clickhouse not initialized")
	}

	if err := s.ch.Conn.Exec(ctx, `TRUNCATE TABLE poolm_top_fees_realtime`); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	batch, err := s.ch.PrepareBatch(ctx, `INSERT INTO poolm_top_fees_realtime (
		ts, chain, protocol_version, timeframe_minutes, dex, pool_address, factory_name, trading_pair,
		fee_percentage, transaction_count, total_fees, total_volume, current_pool_value, price_display, last_swap_at,
		token0_address, token1_address
	)`)
	if err != nil {
		return err
	}

	for _, r := range rows {
		p := r.p
		if err := batch.Append(
			r.ts,
			r.chain,
			r.protocolVersion,
			uint16(r.timeframe),
			strings.TrimSpace(p.Dex),
			strings.ToLower(strings.TrimSpace(p.PoolAddress)),
			strings.TrimSpace(p.FactoryName),
			strings.TrimSpace(p.TradingPair),
			p.FeePercentage,
			uint32(maxInt(p.TransactionCount, 0)),
			p.TotalFees,
			p.TotalVolume,
			p.CurrentPoolValue,
			strings.TrimSpace(p.PriceDisplay),
			r.lastSwapAt,
			strings.ToLower(strings.TrimSpace(p.Token0Address)),
			strings.ToLower(strings.TrimSpace(p.Token1Address)),
		); err != nil {
			return err
		}
	}
	return batch.Send()
}

type AutoLPAnalysis struct {
	Chain           string
	ProtocolVersion string
	PoolAddress     string
	TradingPair     string
	Token0Address   string
	Token1Address   string

	// 5m pool metrics (from PoolM top-fees 5m snapshot)
	FeePercentage float64
	FeeRate5mPct  float64 // total_fees/current_pool_value * 100
	TotalFees5m   float64
	TotalVolume5m float64
	TxCount5m     int
	TVLUSD        float64 // current_pool_value (USD)

	CurrentPrice float64

	MA5    float64
	Sigma5 float64
	Z5     float64

	MA60    float64
	Sigma60 float64
	Z60     float64

	State5    string
	Trend60   string
	Resonance string

	BaseWidthPct  float64
	LowerWidthPct float64
	UpperWidthPct float64

	Action            string
	CandidateEligible bool
	Score             float64

	SmartWalletCount     int // 当前持有LP仓位的钱包数（用于评分加成）
	RecentAddWalletCount int // 最近时间窗口内添加LP的钱包数（用于自动开单判断）
}

func (s *AutoLPService) analyzeSnapshot(ctx context.Context, snap *poolMSnapshot) ([]AutoLPAnalysis, error) {
	if snap == nil {
		return nil, fmt.Errorf("snapshot is nil")
	}

	debug := config.AppConfig != nil && config.AppConfig.AutoLPDebug
	totalPools := len(snap.data)

	minTVL := config.AppConfig.AutoLPMinPoolValueUSD
	minFeePct := config.AppConfig.AutoLPMinFeePercentage
	minFeeRatePct := config.AppConfig.AutoLPMinFeeRate5m
	minFees := config.AppConfig.AutoLPMinTotalFees5m
	minVol := config.AppConfig.AutoLPMinTotalVolume5m
	resMinFeeRate := config.AppConfig.AutoLPResonanceMinFeeRate5m
	resMinVol := config.AppConfig.AutoLPResonanceMinTotalVolume5m
	resMinAbsZ60 := config.AppConfig.AutoLPResonanceMinAbsZ60

	filteredNo5 := 0
	filteredTVL := 0
	filteredFeePct := 0
	filteredFeeRate := 0
	filteredFees := 0
	filteredVol := 0
	passedHard := 0

	var out []AutoLPAnalysis

	for key, tfs := range snap.data {
		p5, ok := tfs[5]
		if !ok {
			filteredNo5++
			continue
		}
		if minTVL > 0 && p5.CurrentPoolValue <= minTVL {
			filteredTVL++
			continue
		}
		if minFeePct > 0 && p5.FeePercentage <= minFeePct {
			filteredFeePct++
			continue
		}
		feeRatePct := 0.0
		if p5.CurrentPoolValue > 0 {
			feeRatePct = (p5.TotalFees / p5.CurrentPoolValue) * 100
		}
		if minFeeRatePct > 0 && feeRatePct <= minFeeRatePct {
			filteredFeeRate++
			continue
		}
		if minFees > 0 && p5.TotalFees <= minFees {
			filteredFees++
			continue
		}
		if minVol > 0 && p5.TotalVolume <= minVol {
			filteredVol++
			continue
		}

		passedHard++

		current := p5.CurrentTokenPrice
		ma5, sigma5, n5, _ := s.priceStats(ctx, snap.chain, key.proto, key.addr, 5, 5)
		ma60, sigma60, n60, _ := s.priceStats(ctx, snap.chain, key.proto, key.addr, 60, 60)

		z5, okZ5 := zScore(current, ma5, sigma5, n5, 4)
		z60, okZ60 := zScore(current, ma60, sigma60, n60, 12)
		if !okZ60 {
			// Some pools may not appear in the 60m top-fees endpoint; fallback to 5m samples but use 60m window.
			ma60, sigma60, n60, _ = s.priceStats(ctx, snap.chain, key.proto, key.addr, 5, 60)
			z60, okZ60 = zScore(current, ma60, sigma60, n60, 12)
		}

		state5 := classifyState(z5, okZ5, sigma5, sigma60)
		trend60 := classifyTrend(z60, okZ60)
		res := classifyResonance(state5, trend60)
		resEligible := true
		if resMinFeeRate > 0 && feeRatePct <= resMinFeeRate {
			resEligible = false
		}
		if resMinVol > 0 && p5.TotalVolume <= resMinVol {
			resEligible = false
		}
		if resMinAbsZ60 > 0 && math.Abs(z60) <= resMinAbsZ60 {
			resEligible = false
		}
		if !resEligible {
			res = "NONE"
		}

		totalWidth := config.AppConfig.AutoLPBaseWidthPercentage
		switch state5 {
		case "SIDEWAYS":
			if config.AppConfig.AutoLPWidthSidewaysPercent > 0 {
				totalWidth = config.AppConfig.AutoLPWidthSidewaysPercent
			}
		case "MILD_UPTREND":
			if config.AppConfig.AutoLPWidthMildUptrendPercent > 0 {
				totalWidth = config.AppConfig.AutoLPWidthMildUptrendPercent
			}
		case "RAPID_PUMP":
			if config.AppConfig.AutoLPWidthRapidPumpPercent > 0 {
				totalWidth = config.AppConfig.AutoLPWidthRapidPumpPercent
			}
		case "CONSOLIDATION":
			// Treat consolidation as a "sideways-like" regime.
			if config.AppConfig.AutoLPWidthSidewaysPercent > 0 {
				totalWidth = config.AppConfig.AutoLPWidthSidewaysPercent
			}
		}
		lowPct, upPct := decideWidth(totalWidth, state5, res)

		score := scoreCandidate(p5.TotalFees, p5.CurrentPoolValue, res, state5)
		eligible := state5 == "RAPID_PUMP" || state5 == "SIDEWAYS" || state5 == "MILD_UPTREND"
		action := "SKIP"
		// Only open in these regimes (V1 tuning): RAPID_PUMP / SIDEWAYS / MILD_UPTREND
		if eligible {
			action = "CANDIDATE"
		}

		out = append(out, AutoLPAnalysis{
			Chain:             snap.chain,
			ProtocolVersion:   key.proto,
			PoolAddress:       key.addr,
			TradingPair:       strings.TrimSpace(p5.TradingPair),
			Token0Address:     strings.ToLower(strings.TrimSpace(p5.Token0Address)),
			Token1Address:     strings.ToLower(strings.TrimSpace(p5.Token1Address)),
			FeePercentage:     p5.FeePercentage,
			FeeRate5mPct:      feeRatePct,
			TotalFees5m:       p5.TotalFees,
			TotalVolume5m:     p5.TotalVolume,
			TxCount5m:         maxInt(p5.TransactionCount, 0),
			TVLUSD:            p5.CurrentPoolValue,
			CurrentPrice:      current,
			MA5:               ma5,
			Sigma5:            sigma5,
			Z5:                z5,
			MA60:              ma60,
			Sigma60:           sigma60,
			Z60:               z60,
			State5:            state5,
			Trend60:           trend60,
			Resonance:         res,
			BaseWidthPct:      totalWidth,
			LowerWidthPct:     lowPct,
			UpperWidthPct:     upPct,
			Action:            action,
			CandidateEligible: eligible,
			Score:             score,
		})
	}

	if s.smartLP != nil && config.AppConfig != nil && config.AppConfig.SmartLPEnabled && len(out) > 0 {
		pools := make([]smart_lp.SmartLPPoolKey, 0, len(out))
		for _, a := range out {
			pools = append(pools, smart_lp.SmartLPPoolKey{
				PoolVersion: a.ProtocolVersion,
				PoolID:      a.PoolAddress,
			})
		}

		// 获取当前持仓钱包数（用于评分加成）
		if counts, err := s.smartLP.GetActiveWalletCounts(ctx, pools); err != nil {
			log.Printf("[AutoLP] smart LP counts failed: %v", err)
		} else {
			bonus := config.AppConfig.SmartLPScorePerWallet
			for i := range out {
				key := strings.ToLower(strings.TrimSpace(out[i].ProtocolVersion)) + "|" + strings.ToLower(strings.TrimSpace(out[i].PoolAddress))
				if c, ok := counts[key]; ok {
					out[i].SmartWalletCount = c
					if bonus > 0 {
						out[i].Score += float64(c) * bonus
					}
				}
			}
		}

		// 获取最近时间窗口内添加LP的钱包数（用于自动开单判断）
		windowMinutes := config.AppConfig.SmartLPRecentWindowMinutes
		if windowMinutes <= 0 {
			windowMinutes = 10 // 默认10分钟
		}
		lookback := time.Duration(windowMinutes) * time.Minute
		if recentCounts, err := s.smartLP.GetRecentAddWalletCounts(ctx, pools, lookback); err != nil {
			log.Printf("[AutoLP] smart LP recent add counts failed: %v", err)
		} else {
			for i := range out {
				key := strings.ToLower(strings.TrimSpace(out[i].ProtocolVersion)) + "|" + strings.ToLower(strings.TrimSpace(out[i].PoolAddress))
				if c, ok := recentCounts[key]; ok {
					out[i].RecentAddWalletCount = c
				}
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})

	if max := config.AppConfig.AutoLPMaxCandidates; max > 0 {
		candidatesSeen := 0
		for i := range out {
			if out[i].Action != "CANDIDATE" {
				continue
			}
			candidatesSeen++
			if candidatesSeen > max {
				out[i].Action = "SKIP"
			}
		}
	}

	candidates := make([]AutoLPAnalysis, 0, len(out))
	for _, a := range out {
		if a.Action != "CANDIDATE" {
			continue
		}
		candidates = append(candidates, a)
	}
	candidateCount := len(candidates)
	log.Printf("[AutoLP] 筛选结果：总池=%d 通过硬筛=%d 进入分析=%d 候选=%d；过滤：缺少5m(没进5m榜单)=%d TVL不达标(current_pool_value)=%d 费率不达标(fee_percentage)=%d 费用率不达标(fee_rate_5m)=%d 5m手续费不达标(total_fees)=%d 5m成交量不达标(total_volume)=%d",
		totalPools, passedHard, len(out), candidateCount, filteredNo5, filteredTVL, filteredFeePct, filteredFeeRate, filteredFees, filteredVol,
	)
	if len(candidates) > 0 {
		top := candidates[0]
		log.Printf("[AutoLP] Top1：评分=%.2f 协议=%s 交易对=%s 地址=%s｜Z5=%.2f 状态=%s(%s)｜Z60=%.2f 趋势=%s(%s)｜共振=%s(%s)｜宽度=%.2f%%（下 %.2f%% / 上 %.2f%%）｜动作=%s(%s)",
			top.Score,
			strings.ToUpper(top.ProtocolVersion), top.TradingPair, top.PoolAddress,
			top.Z5, top.State5, autoLPStateZh(top.State5),
			top.Z60, top.Trend60, autoLPTrendZh(top.Trend60),
			top.Resonance, autoLPResonanceZh(top.Resonance),
			top.BaseWidthPct, top.LowerWidthPct, top.UpperWidthPct,
			top.Action, autoLPActionZh(top.Action),
		)
	}
	if debug {
		limit := 10
		printed := 0
		for i := 0; i < len(candidates) && printed < limit; i++ {
			a := candidates[i]
			log.Printf("[AutoLP] 候选#%d：评分=%.2f 协议=%s 交易对=%s 地址=%s｜Z5=%.2f 状态=%s(%s)｜Z60=%.2f 趋势=%s(%s)｜共振=%s(%s)｜宽度=%.2f%%（下 %.2f%% / 上 %.2f%%）",
				printed+1,
				a.Score,
				strings.ToUpper(a.ProtocolVersion), a.TradingPair, a.PoolAddress,
				a.Z5, a.State5, autoLPStateZh(a.State5),
				a.Z60, a.Trend60, autoLPTrendZh(a.Trend60),
				a.Resonance, autoLPResonanceZh(a.Resonance),
				a.BaseWidthPct, a.LowerWidthPct, a.UpperWidthPct,
			)
			printed++
		}
	}
	return out, nil
}

func (s *AutoLPService) insertAnalysis(ctx context.Context, rows []AutoLPAnalysis) error {
	if s.ch == nil || s.ch.Conn == nil {
		return nil
	}
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.ch.PrepareBatch(ctx, `INSERT INTO auto_lp_analysis (
		ts, chain, protocol_version, pool_address, trading_pair, current_price,
		ma_5, sigma_5, z_5,
		ma_60, sigma_60, z_60,
		state_5, trend_60, resonance,
		base_width_pct, lower_width_pct, upper_width_pct,
		action, score
	)`)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, r := range rows {
		if err := batch.Append(
			now,
			r.Chain,
			r.ProtocolVersion,
			r.PoolAddress,
			r.TradingPair,
			r.CurrentPrice,
			r.MA5,
			r.Sigma5,
			r.Z5,
			r.MA60,
			r.Sigma60,
			r.Z60,
			r.State5,
			r.Trend60,
			r.Resonance,
			r.BaseWidthPct,
			r.LowerWidthPct,
			r.UpperWidthPct,
			r.Action,
			r.Score,
		); err != nil {
			return err
		}
	}
	return batch.Send()
}

func (s *AutoLPService) priceStats(ctx context.Context, chain string, proto string, poolAddr string, timeframeMinutes int, windowMinutes int) (ma float64, sigma float64, n uint64, err error) {
	if s.ch == nil || s.ch.Conn == nil {
		return 0, 0, 0, fmt.Errorf("clickhouse not initialized")
	}
	if timeframeMinutes <= 0 {
		return 0, 0, 0, fmt.Errorf("invalid timeframeMinutes=%d", timeframeMinutes)
	}
	if windowMinutes <= 0 {
		return 0, 0, 0, fmt.Errorf("invalid windowMinutes=%d", windowMinutes)
	}
	chain = strings.ToLower(strings.TrimSpace(chain))
	proto = strings.ToLower(strings.TrimSpace(proto))
	poolAddr = strings.ToLower(strings.TrimSpace(poolAddr))
	if chain == "" || proto == "" || poolAddr == "" {
		return 0, 0, 0, fmt.Errorf("missing key")
	}

	q := fmt.Sprintf(`
		SELECT
			avg(current_token_price) AS ma,
			stddevPop(current_token_price) AS sigma,
			count() AS n
		FROM poolm_top_fees_raw
		WHERE chain = ? AND protocol_version = ? AND timeframe_minutes = ? AND pool_address = ?
		  AND ts >= now() - INTERVAL %d MINUTE
	`, windowMinutes)

	row := s.ch.Conn.QueryRow(ctx, q, chain, proto, uint16(timeframeMinutes), poolAddr)
	if err := row.Scan(&ma, &sigma, &n); err != nil {
		return 0, 0, 0, err
	}
	if math.IsNaN(ma) || math.IsNaN(sigma) || math.IsInf(ma, 0) || math.IsInf(sigma, 0) {
		return 0, 0, 0, fmt.Errorf("invalid stats")
	}
	return ma, sigma, n, nil
}

func zScore(current float64, ma float64, sigma float64, n uint64, minPoints uint64) (float64, bool) {
	if n < minPoints || sigma <= 0 {
		return 0, false
	}
	z := (current - ma) / sigma
	if math.IsNaN(z) || math.IsInf(z, 0) {
		return 0, false
	}
	return z, true
}

func classifyState(z float64, okZ bool, sigma5 float64, sigma60 float64) string {
	if !okZ {
		return "FALLBACK"
	}
	if z < -3 {
		return "CRASH"
	}
	if z > 3 {
		return "RAPID_PUMP"
	}
	if math.Abs(z) < 0.5 {
		if sigma5 > 0 && sigma60 > 0 && sigma5 < sigma60*0.5 {
			return "CONSOLIDATION"
		}
		return "SIDEWAYS"
	}
	if z > 0.5 && z < 1.5 {
		return "MILD_UPTREND"
	}
	if z < -0.5 && z > -1.5 {
		return "MILD_DOWNTREND"
	}
	return "FALLBACK"
}

func classifyTrend(z float64, okZ bool) string {
	if !okZ {
		return "UNKNOWN"
	}
	if z > 0.5 {
		return "UPTREND"
	}
	if z < -0.5 {
		return "DOWNTREND"
	}
	return "SIDEWAYS"
}

func classifyResonance(state5 string, trend60 string) string {
	up5 := state5 == "RAPID_PUMP" || state5 == "MILD_UPTREND"
	down5 := state5 == "CRASH" || state5 == "MILD_DOWNTREND"

	switch trend60 {
	case "UPTREND":
		if up5 {
			return "STRONG"
		}
		if down5 {
			return "DIVERGENCE"
		}
	case "DOWNTREND":
		if down5 {
			return "STRONG"
		}
		if up5 {
			return "DIVERGENCE"
		}
	}
	return "NONE"
}

func decideWidth(baseWidth float64, state5 string, resonance string) (lowerPct float64, upperPct float64) {
	if baseWidth <= 0 || baseWidth >= 100 {
		baseWidth = 5
	}

	total := baseWidth
	lShare := 0.5
	uShare := 0.5

	switch state5 {
	case "RAPID_PUMP":
		// Use a wider total width (configured via AUTO_LP_WIDTH_RAPID_PUMP_PERCENT).
		total = baseWidth
		lShare = 0.4
		uShare = 0.6
	case "MILD_UPTREND":
		total = baseWidth
		lShare = 0.3
		uShare = 0.7
	case "MILD_DOWNTREND":
		total = baseWidth
		lShare = 0.8
		uShare = 0.2
	case "CONSOLIDATION":
		total = baseWidth
		lShare = 0.5
		uShare = 0.5
	case "SIDEWAYS":
		total = baseWidth
		lShare = 0.5
		uShare = 0.5
	default:
		total = baseWidth
		lShare = 0.5
		uShare = 0.5
	}

	if resonance == "DIVERGENCE" {
		total = total * 2.0
		lShare = 0.5
		uShare = 0.5
	}

	if total < 0.5 {
		total = 0.5
	}
	if total > 50 {
		total = 50
	}

	lowerPct = total * lShare
	upperPct = total * uShare
	if lowerPct <= 0 {
		lowerPct = 0.5
	}
	if upperPct <= 0 {
		upperPct = 0.5
	}
	return lowerPct, upperPct
}

func scoreCandidate(fees5m float64, tvl float64, resonance string, state5 string) float64 {
	if state5 == "CRASH" {
		return -1
	}
	score := fees5m
	if tvl > 0 {
		score = score * math.Log1p(tvl/10000.0)
	}
	if resonance == "STRONG" {
		score = score * 1.5
	}
	if resonance == "DIVERGENCE" {
		score = score * 0.7
	}
	return score
}

func splitCSVLower(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func autoLPDexList(raw string) []string {
	parts := splitCSVLower(raw)
	if len(parts) == 0 {
		return []string{"pcsv3", "univ3", "univ4"}
	}

	expanded := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "v3":
			expanded = append(expanded, "pcsv3", "univ3")
		case "v4":
			expanded = append(expanded, "univ4")
		default:
			expanded = append(expanded, part)
		}
	}

	seen := make(map[string]struct{}, len(expanded))
	out := make([]string, 0, len(expanded))
	for _, dex := range expanded {
		if dex == "" {
			continue
		}
		if _, ok := seen[dex]; ok {
			continue
		}
		seen[dex] = struct{}{}
		out = append(out, dex)
	}
	if len(out) == 0 {
		return []string{"pcsv3", "univ3", "univ4"}
	}
	return out
}

func normalizePoolMProtocolVersion(p PoolMFeePool, poolAddr string) string {
	candidates := []string{p.ProtocolVersion, p.Dex, p.FactoryName}
	for _, raw := range candidates {
		v := strings.ToLower(strings.TrimSpace(raw))
		if v == "" {
			continue
		}
		if strings.Contains(v, "v4") {
			return "v4"
		}
		if strings.Contains(v, "v3") {
			return "v3"
		}
	}

	addr := strings.ToLower(strings.TrimSpace(poolAddr))
	if len(addr) == 66 {
		return "v4"
	}
	if len(addr) == 42 {
		return "v3"
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *AutoLPService) resolveTargetUserID() (uint, error) {
	if config.AppConfig == nil {
		return 0, fmt.Errorf("config not loaded")
	}
	if config.AppConfig.AutoLPUserID > 0 {
		return uint(config.AppConfig.AutoLPUserID), nil
	}

	adminAddr := strings.ToLower(strings.TrimSpace(config.AppConfig.AdminWalletAddress))
	if adminAddr == "" {
		return 0, fmt.Errorf("AUTO_LP_USER_ID not set and ADMIN_WALLET_ADDRESS not set")
	}

	var w models.Wallet
	if err := database.DB.Where("LOWER(address) = ?", adminAddr).First(&w).Error; err != nil {
		return 0, fmt.Errorf("find admin wallet failed: %w", err)
	}
	return w.UserID, nil
}

func (s *AutoLPService) applyUserStopConditions(ctx context.Context, cfg models.AutoLPUserConfig) (bool, error) {
	userID := cfg.UserID
	if userID == 0 {
		return true, nil
	}
	if cfg.TakeProfitUSDT <= 0 && cfg.StopLossUSDT <= 0 {
		return false, nil
	}

	// 只计算本轮 AutoLP 开启后的收益，避免历史亏损导致立即触发止损
	profitWei, err := s.sumAutoRealizedProfitWei(ctx, userID, cfg.LastEnabledAt)
	if err != nil {
		return false, err
	}

	toUSDT := func(v *big.Int) float64 {
		if v == nil {
			return 0
		}
		f := new(big.Float).SetInt(v)
		f.Quo(f, big.NewFloat(1e18))
		out, _ := f.Float64()
		return out
	}

	// Take profit: profit >= TP
	if cfg.TakeProfitUSDT > 0 {
		tpWei, err := convert.FloatUSDTToWei(cfg.TakeProfitUSDT)
		if err == nil && profitWei.Cmp(tpWei) >= 0 {
			now := time.Now()
			_, _ = NewAutoLPUserConfigService().Update(userID, map[string]interface{}{
				"enabled":          false,
				"last_disabled_at": now,
			})
			s.notify(userID, fmt.Sprintf("✅ AutoLP 已达到盈利关闭条件：%.2f USDT（累计 %.2f USDT），已自动关闭并开始撤出当前自动仓位。", cfg.TakeProfitUSDT, toUSDT(profitWei)))
			reason := fmt.Sprintf("🎯 盈利关闭触发（%.2f USDT）", cfg.TakeProfitUSDT)
			if err := s.RequestExitForAutoTasks(userID, reason, 1.0); err != nil {
				s.notify(userID, fmt.Sprintf("⚠️ AutoLP 发起撤仓失败：%v", err))
			}
			return true, nil
		}
	}

	// Stop loss: profit <= -SL
	if cfg.StopLossUSDT > 0 {
		slWei, err := convert.FloatUSDTToWei(cfg.StopLossUSDT)
		if err == nil {
			negSL := new(big.Int).Neg(slWei)
			if profitWei.Cmp(negSL) <= 0 {
				now := time.Now()
				_, _ = NewAutoLPUserConfigService().Update(userID, map[string]interface{}{
					"enabled":          false,
					"last_disabled_at": now,
				})
				s.notify(userID, fmt.Sprintf("⚠️ AutoLP 已触发亏损关闭条件：%.2f USDT（累计 %.2f USDT），已自动关闭并开始撤出当前自动仓位。", cfg.StopLossUSDT, toUSDT(profitWei)))
				reason := fmt.Sprintf("⚠️ 亏损关闭触发（%.2f USDT）", cfg.StopLossUSDT)
				if err := s.RequestExitForAutoTasks(userID, reason, 1.0); err != nil {
					s.notify(userID, fmt.Sprintf("⚠️ AutoLP 发起撤仓失败：%v", err))
				}
				return true, nil
			}
		}
	}

	return false, nil
}

func (s *AutoLPService) RequestExitForAutoTasks(userID uint, reason string, gasMultiplier float64) error {
	return s.requestExitForAutoTasks(userID, reason, gasMultiplier)
}

func (s *AutoLPService) requestExitForAutoTasks(userID uint, reason string, gasMultiplier float64) error {
	if userID == 0 || database.DB == nil {
		return nil
	}

	var tasks []models.StrategyTask
	if err := database.DB.Where("user_id = ? AND is_auto = ? AND paused = ? AND status IN ?", userID, true, false, []models.StrategyStatus{
		models.StrategyStatusRunning,
		models.StrategyStatusWaiting,
	}).Find(&tasks).Error; err != nil {
		return err
	}

	var firstErr error
	for i := range tasks {
		if hasTaskPositionForExit(&tasks[i]) {
			if err := s.requestStopLossExit(&tasks[i], reason, gasMultiplier); err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		updates := map[string]interface{}{
			"status":              models.StrategyStatusStopped,
			"out_of_range_since":  nil,
			"error_message":       "",
			"exit_pending_action": "",
			"exit_pending_reason": "",
			"exit_retry_count":    0,
			"exit_next_retry_at":  nil,
			"exit_last_error":     "",
			"exit_give_up_at":     nil,
		}
		if err := database.DB.Model(&tasks[i]).Updates(updates).Error; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func hasTaskPositionForExit(task *models.StrategyTask) bool {
	if task == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(task.PoolVersion)) {
	case "v4":
		v4TokenId := strings.TrimSpace(task.V4TokenID)
		return v4TokenId != "" && v4TokenId != "0"
	default:
		v3TokenId := strings.TrimSpace(task.V3TokenID)
		return v3TokenId != "" && v3TokenId != "0"
	}
}

func (s *AutoLPService) sumAutoRealizedProfitWei(ctx context.Context, userID uint, lastEnabledAt *time.Time) (*big.Int, error) {
	if database.DB == nil {
		return big.NewInt(0), nil
	}

	type row struct {
		Profit string `gorm:"column:profit"`
	}
	out := row{}

	// 只计算 lastEnabledAt 之后关闭的交易记录的收益
	// 这样每次重新开启 AutoLP 时，收益计算会从 0 开始
	var q string
	var err error
	if lastEnabledAt != nil {
		q = `
			SELECT COALESCE(SUM(CAST(tr.profit_usdt AS DECIMAL(65,0))), 0) AS profit
			FROM trade_records tr
			JOIN strategy_tasks st ON st.id = tr.task_id
			WHERE tr.user_id = ? AND tr.status = ? AND st.is_auto = 1 AND tr.closed_at >= ?
		`
		err = database.DB.Raw(q, userID, models.TradeStatusClosed, *lastEnabledAt).Scan(&out).Error
	} else {
		q = `
			SELECT COALESCE(SUM(CAST(tr.profit_usdt AS DECIMAL(65,0))), 0) AS profit
			FROM trade_records tr
			JOIN strategy_tasks st ON st.id = tr.task_id
			WHERE tr.user_id = ? AND tr.status = ? AND st.is_auto = 1
		`
		err = database.DB.Raw(q, userID, models.TradeStatusClosed).Scan(&out).Error
	}
	if err != nil {
		return nil, err
	}

	raw := strings.TrimSpace(out.Profit)
	if raw == "" {
		raw = "0"
	}
	v, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		return nil, fmt.Errorf("invalid profit sum: %q", raw)
	}
	return v, nil
}

func (s *AutoLPService) tryOpenCandidate(ctx context.Context, userID uint, a AutoLPAnalysis, amount float64) (bool, error) {
	if a.Action != "CANDIDATE" {
		return false, nil
	}
	if ok, err := s.hasActiveTask(userID, a.ProtocolVersion, a.PoolAddress); err != nil {
		return false, err
	} else if ok {
		return false, nil
	}

	task, gasMult, err := s.buildTaskForCandidate(ctx, userID, a, amount)
	if err != nil {
		return false, nil
	}

	if err := database.DB.Create(task).Error; err != nil {
		return false, fmt.Errorf("create task failed: %w", err)
	}

	displayLowerPct := task.RangeLowerPercentage
	displayUpperPct := task.RangeUpperPercentage
	if task.RangeLowerPercentage > 0 && task.RangeUpperPercentage > 0 {
		stableLowerPct, stableUpperPct := pricing.StablePercentagesFromTickPercentages(task, task.RangeLowerPercentage, task.RangeUpperPercentage)
		if stableLowerPct > 0 && stableUpperPct > 0 {
			displayLowerPct = stableLowerPct
			displayUpperPct = stableUpperPct
		}
	}

	s.notify(userID, fmt.Sprintf("🤖 AutoLP 开仓中...\n池子: %s (%s)\n投入: %.2f USDT\n宽度：下 %.2f%% / 上 %.2f%%",
		task.Token0Symbol+"/"+task.Token1Symbol, task.PoolId, amount, displayLowerPct, displayUpperPct))

	enterRes, err := s.liquidity.EnterTaskFromUSDTWithOptions(userID, task, liquidity.TxOptions{GasMultiplier: gasMult})
	if err != nil {
		_ = database.DB.Model(task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": fmt.Sprintf("enter failed: %v", err),
		}).Error
		return false, err
	}

	if err := applyEnterResultToTask(task, enterRes); err != nil {
		return false, fmt.Errorf("update task after enter failed: %w", err)
	}

	s.notify(userID, fmt.Sprintf("✅ AutoLP 开仓成功！\n任务ID: %d\n交易哈希: `%s`", task.ID, enterRes.TxHash))
	_ = strategy.NewAutoLPEventService().Record(task, models.AutoLPEventOpen, "")
	return true, nil
}

func autoLPPoolKey(version string, poolID string) string {
	v := strings.ToLower(strings.TrimSpace(version))
	p := strings.ToLower(strings.TrimSpace(poolID))
	if v == "" || p == "" {
		return ""
	}
	return v + "|" + p
}

func autoLPPairKey(token0 string, token1 string) string {
	t0 := strings.ToLower(strings.TrimSpace(token0))
	t1 := strings.ToLower(strings.TrimSpace(token1))
	if t0 == "" || t1 == "" {
		return ""
	}
	if common.IsHexAddress(t0) && common.IsHexAddress(t1) {
		a0 := common.HexToAddress(t0)
		a1 := common.HexToAddress(t1)
		if bytesCompare(a0, a1) > 0 {
			a0, a1 = a1, a0
		}
		return strings.ToLower(a0.Hex()) + "|" + strings.ToLower(a1.Hex())
	}
	if t0 > t1 {
		t0, t1 = t1, t0
	}
	return t0 + "|" + t1
}

func autoLPShouldSwitch(current float64, target float64, minImprovementPct float64) bool {
	if target <= current {
		return false
	}
	if minImprovementPct <= 0 || current <= 0 {
		return true
	}
	return target >= current*(1.0+minImprovementPct/100.0)
}

func autoLPCandidateContainsUSDT(a AutoLPAnalysis) bool {
	if config.AppConfig == nil || !common.IsHexAddress(config.AppConfig.USDTAddress) {
		return true
	}
	usdt := strings.ToLower(strings.TrimSpace(config.AppConfig.USDTAddress))
	t0 := strings.ToLower(strings.TrimSpace(a.Token0Address))
	t1 := strings.ToLower(strings.TrimSpace(a.Token1Address))
	if t0 == "" || t1 == "" {
		return true
	}
	return t0 == usdt || t1 == usdt
}

func autoLPTaskRangePct(task *models.StrategyTask) (float64, float64) {
	if task == nil {
		return 0, 0
	}
	lower := task.RangeLowerPercentage
	upper := task.RangeUpperPercentage
	if lower > 0 && upper > 0 {
		return lower, upper
	}
	if task.RangePercentage > 0 {
		return task.RangePercentage, task.RangePercentage
	}
	return 0, 0
}

func (s *AutoLPService) requestSwitchExit(task *models.StrategyTask, target AutoLPAnalysis, targetLowerPct float64, targetUpperPct float64, reason string, gasMultiplier float64) (bool, error) {
	if task == nil {
		return false, nil
	}
	if task.ExitGiveUpAt != nil {
		return false, nil
	}
	if strings.TrimSpace(task.ExitPendingAction) != "" {
		return false, nil
	}
	if task.RebalancePending {
		return false, nil
	}

	targetPoolVersion := strings.ToLower(strings.TrimSpace(target.ProtocolVersion))
	targetPoolID := strings.TrimSpace(target.PoolAddress)
	if targetPoolVersion == "" || targetPoolID == "" {
		return false, nil
	}

	updates := map[string]interface{}{
		"exit_pending_action":          strategy.ExitActionSwitch,
		"exit_pending_reason":          strings.TrimSpace(reason),
		"exit_gas_multiplier":          gasMultiplier,
		"exit_retry_count":             0,
		"exit_next_retry_at":           nil,
		"exit_last_error":              "",
		"exit_give_up_at":              nil,
		"rebalance_pending":            false,
		"rebalance_retry_count":        0,
		"rebalance_next_retry_at":      nil,
		"rebalance_last_error":         "",
		"error_message":                "",
		"switch_target_pool_version":   targetPoolVersion,
		"switch_target_pool_id":        targetPoolID,
		"switch_target_tick_lower_pct": targetLowerPct,
		"switch_target_tick_upper_pct": targetUpperPct,
	}
	if err := database.DB.Model(task).Updates(updates).Error; err != nil {
		return false, err
	}

	task.ExitPendingAction = strategy.ExitActionSwitch
	task.ExitPendingReason = strings.TrimSpace(reason)
	task.ExitGasMultiplier = gasMultiplier
	task.ExitRetryCount = 0
	task.ExitNextRetryAt = nil
	task.ExitLastError = ""
	task.ExitGiveUpAt = nil
	task.RebalancePending = false
	task.RebalanceRetryCount = 0
	task.RebalanceNextRetryAt = nil
	task.RebalanceLastError = ""
	task.ErrorMessage = ""
	task.SwitchTargetPoolVersion = targetPoolVersion
	task.SwitchTargetPoolId = targetPoolID
	task.SwitchTargetTickLowerPct = targetLowerPct
	task.SwitchTargetTickUpperPct = targetUpperPct

	return true, nil
}

func (s *AutoLPService) trySwitchWorstAutoTask(ctx context.Context, cfg models.AutoLPUserConfig, analyses []AutoLPAnalysis) (bool, error) {
	userID := cfg.UserID
	if userID == 0 || database.DB == nil {
		return false, nil
	}

	analysisByPool := make(map[string]AutoLPAnalysis, len(analyses))
	for _, a := range analyses {
		if k := autoLPPoolKey(a.ProtocolVersion, a.PoolAddress); k != "" {
			analysisByPool[k] = a
		}
	}

	var tasks []models.StrategyTask
	if err := database.DB.Where("user_id = ? AND is_auto = ? AND paused = ? AND status IN ?", userID, true, false, []models.StrategyStatus{
		models.StrategyStatusRunning,
		models.StrategyStatusWaiting,
	}).Find(&tasks).Error; err != nil {
		return false, err
	}

	var worst *models.StrategyTask
	worstYield := math.MaxFloat64
	for i := range tasks {
		task := &tasks[i]
		if strings.TrimSpace(task.ExitPendingAction) != "" || task.RebalancePending || !hasTaskPositionForExit(task) {
			continue
		}
		y := 0.0
		if a, ok := analysisByPool[autoLPPoolKey(task.PoolVersion, task.PoolId)]; ok {
			y = a.FeeRate5mPct
		}
		if y < worstYield {
			worstYield = y
			worst = task
		}
	}
	if worst == nil {
		return false, nil
	}

	var bestCand AutoLPAnalysis
	foundCand := false
	bestYield := 0.0
	for _, a := range analyses {
		if a.Action != "CANDIDATE" || a.FeeRate5mPct <= 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(a.ProtocolVersion), strings.TrimSpace(worst.PoolVersion)) &&
			strings.EqualFold(strings.TrimSpace(a.PoolAddress), strings.TrimSpace(worst.PoolId)) {
			continue
		}
		if ok, err := s.hasActiveTask(userID, a.ProtocolVersion, a.PoolAddress); err != nil {
			return false, err
		} else if ok {
			continue
		}
		if !worst.AllowEntrySwap && !autoLPCandidateContainsUSDT(a) {
			continue
		}
		if !autoLPShouldSwitch(worstYield, a.FeeRate5mPct, cfg.SwitchMinImprovementPct) {
			continue
		}
		if a.FeeRate5mPct > bestYield {
			bestYield = a.FeeRate5mPct
			bestCand = a
			foundCand = true
		}
	}
	if !foundCand {
		return false, nil
	}

	candTask, _, err := s.buildTaskForCandidate(ctx, userID, bestCand, worst.AmountUSDT)
	if err != nil || candTask == nil {
		return false, nil
	}

	reason := fmt.Sprintf("🔁 AutoLP 切换到更高收益池：%s (%.4f%%)", strings.TrimSpace(bestCand.TradingPair), bestYield)
	detail := fmt.Sprintf("🔁 AutoLP 已满仓，触发换池：\n当前最低：%s/%s %.4f%%\n目标池：%s %.4f%%\n阈值：+%.2f%%",
		strings.TrimSpace(worst.Token0Symbol),
		strings.TrimSpace(worst.Token1Symbol),
		worstYield,
		strings.TrimSpace(bestCand.TradingPair),
		bestYield,
		cfg.SwitchMinImprovementPct,
	)
	if scheduled, err := s.requestSwitchExit(worst, bestCand, candTask.RangeLowerPercentage, candTask.RangeUpperPercentage, reason, 1.0); err != nil {
		return false, err
	} else if !scheduled {
		return false, nil
	}
	s.notify(userID, detail)
	return true, nil
}

func (s *AutoLPService) trySwitchManualTask(ctx context.Context, cfg models.AutoLPUserConfig, analyses []AutoLPAnalysis) (bool, error) {
	userID := cfg.UserID
	if userID == 0 || database.DB == nil {
		return false, nil
	}

	analysisByPool := make(map[string]AutoLPAnalysis, len(analyses))
	bestByPair := make(map[string]AutoLPAnalysis)
	for _, a := range analyses {
		if k := autoLPPoolKey(a.ProtocolVersion, a.PoolAddress); k != "" {
			analysisByPool[k] = a
		}
		if a.Action != "CANDIDATE" || a.FeeRate5mPct <= 0 {
			continue
		}
		pair := autoLPPairKey(a.Token0Address, a.Token1Address)
		if pair == "" {
			continue
		}
		if cur, ok := bestByPair[pair]; !ok || a.FeeRate5mPct > cur.FeeRate5mPct {
			bestByPair[pair] = a
		}
	}

	var tasks []models.StrategyTask
	if err := database.DB.Where("user_id = ? AND is_auto = ? AND paused = ? AND status IN ?", userID, false, false, []models.StrategyStatus{
		models.StrategyStatusRunning,
	}).Find(&tasks).Error; err != nil {
		return false, err
	}

	for i := range tasks {
		task := &tasks[i]
		if strings.TrimSpace(task.ExitPendingAction) != "" || task.RebalancePending || !hasTaskPositionForExit(task) {
			continue
		}
		pair := autoLPPairKey(task.Token0Address, task.Token1Address)
		if pair == "" {
			continue
		}
		best, ok := bestByPair[pair]
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(best.ProtocolVersion), strings.TrimSpace(task.PoolVersion)) &&
			strings.EqualFold(strings.TrimSpace(best.PoolAddress), strings.TrimSpace(task.PoolId)) {
			continue
		}
		if ok, err := s.hasActiveTask(userID, best.ProtocolVersion, best.PoolAddress); err != nil {
			return false, err
		} else if ok {
			continue
		}

		currentYield := 0.0
		if a, ok := analysisByPool[autoLPPoolKey(task.PoolVersion, task.PoolId)]; ok {
			currentYield = a.FeeRate5mPct
		}
		if !autoLPShouldSwitch(currentYield, best.FeeRate5mPct, cfg.SwitchMinImprovementPct) {
			continue
		}
		if !task.AllowEntrySwap && !autoLPCandidateContainsUSDT(best) {
			continue
		}

		lowerPct, upperPct := autoLPTaskRangePct(task)
		if lowerPct <= 0 || upperPct <= 0 || lowerPct >= 100 || upperPct >= 100 {
			continue
		}

		reason := fmt.Sprintf("🔁 手动仓位切换更高收益池：%s (%.4f%%)", strings.TrimSpace(best.TradingPair), best.FeeRate5mPct)
		detail := fmt.Sprintf("🔁 手动仓位触发换池：\n当前池：%s/%s %.4f%%\n目标池：%s %.4f%%\n阈值：+%.2f%%",
			strings.TrimSpace(task.Token0Symbol),
			strings.TrimSpace(task.Token1Symbol),
			currentYield,
			strings.TrimSpace(best.TradingPair),
			best.FeeRate5mPct,
			cfg.SwitchMinImprovementPct,
		)
		if scheduled, err := s.requestSwitchExit(task, best, lowerPct, upperPct, reason, 1.0); err != nil {
			return false, err
		} else if !scheduled {
			return false, nil
		}
		s.notify(userID, detail)
		return true, nil
	}

	return false, nil
}

func (s *AutoLPService) executeBestCandidateForUser(ctx context.Context, cfg models.AutoLPUserConfig, snap *poolMSnapshot, analyses []AutoLPAnalysis) error {
	userID := cfg.UserID
	if userID == 0 {
		return nil
	}
	if len(analyses) == 0 {
		return nil
	}
	if cfg.TotalAmountUSDT <= 0 {
		return fmt.Errorf("AutoLP 总投入未设置")
	}
	if cfg.MaxActiveTasks <= 0 {
		return fmt.Errorf("AutoLP 最大任务数未设置")
	}

	amountPerTask := cfg.TotalAmountUSDT / float64(cfg.MaxActiveTasks)
	if amountPerTask <= 0 {
		return fmt.Errorf("AutoLP 单仓投入无效")
	}

	check, err := s.accessService.CheckUserAccess(userID, time.Now())
	if err != nil {
		return err
	}
	if !check.Allowed {
		now := time.Now()
		_, _ = NewAutoLPUserConfigService().Update(userID, map[string]interface{}{
			"enabled":          false,
			"last_disabled_at": now,
		})
		s.notify(userID, fmt.Sprintf("⚠️ AutoLP 已自动关闭：%s", strings.TrimSpace(check.Reason)))
		return nil
	}

	// Respect user task quota (manual + auto).
	if !check.IsAdmin && check.Access != nil && check.Access.MaxActiveTasks > 0 {
		totalActive, _ := s.accessService.CountUserActiveTasks(userID)
		if totalActive >= int64(check.Access.MaxActiveTasks) {
			// Still allow switching existing tasks (does not increase task count).
			if activeCount, _ := s.countActiveAutoTasks(userID); activeCount >= int64(cfg.MaxActiveTasks) {
				if switched, err := s.trySwitchWorstAutoTask(ctx, cfg, analyses); err != nil {
					return err
				} else if switched {
					return nil
				}
			}
			if switched, err := s.trySwitchManualTask(ctx, cfg, analyses); err != nil {
				return err
			} else if switched {
				return nil
			}
			return nil
		}
	}

	activeCount, _ := s.countActiveAutoTasks(userID)
	if activeCount >= int64(cfg.MaxActiveTasks) {
		if switched, err := s.trySwitchWorstAutoTask(ctx, cfg, analyses); err != nil {
			return err
		} else if switched {
			return nil
		}
		if switched, err := s.trySwitchManualTask(ctx, cfg, analyses); err != nil {
			return err
		} else if switched {
			return nil
		}
		return nil
	}

	if switched, err := s.trySwitchManualTask(ctx, cfg, analyses); err != nil {
		return err
	} else if switched {
		return nil
	}

	if ok, err := s.hasEnoughUSDT(userID, amountPerTask); err != nil {
		return err
	} else if !ok {
		return nil
	}

	// 优先使用最近时间窗口内添加LP的钱包数进行自动开单判断
	if config.AppConfig != nil && config.AppConfig.SmartLPMinWallets > 0 {
		for _, a := range analyses {
			if a.Action != "CANDIDATE" || a.RecentAddWalletCount < config.AppConfig.SmartLPMinWallets {
				continue
			}
			if opened, err := s.tryOpenCandidate(ctx, userID, a, amountPerTask); err != nil {
				return err
			} else if opened {
				return nil
			}
		}
	}

	for _, a := range analyses {
		if opened, err := s.tryOpenCandidate(ctx, userID, a, amountPerTask); err != nil {
			return err
		} else if opened {
			return nil
		}
	}

	return nil
}

func (s *AutoLPService) executeBestCandidate(ctx context.Context, userID uint, snap *poolMSnapshot, analyses []AutoLPAnalysis) error {
	if len(analyses) == 0 {
		return nil
	}
	if config.AppConfig.AutoLPAmountUSDT <= 0 {
		return fmt.Errorf("AUTO_LP_AMOUNT_USDT not set")
	}

	check, err := s.accessService.CheckUserAccess(userID, time.Now())
	if err != nil {
		return err
	}
	if !check.Allowed {
		return fmt.Errorf("user not authorized: %s", strings.TrimSpace(check.Reason))
	}

	activeCount, _ := s.countActiveAutoTasks(userID)
	if max := int64(config.AppConfig.AutoLPMaxActiveTasks); max > 0 && activeCount >= max {
		return nil
	}

	openWithAmount := func(a AutoLPAnalysis) (bool, error) {
		amount := config.AppConfig.AutoLPAmountUSDT
		if a.Resonance == "STRONG" {
			if ok, _ := s.hasEnoughUSDT(userID, amount*2); ok {
				amount = amount * 2
			}
		}
		if ok, _ := s.hasEnoughUSDT(userID, amount); !ok {
			return false, nil
		}
		return s.tryOpenCandidate(ctx, userID, a, amount)
	}

	// 优先使用最近时间窗口内添加LP的钱包数进行自动开单判断
	if config.AppConfig != nil && config.AppConfig.SmartLPMinWallets > 0 {
		for _, a := range analyses {
			if a.Action != "CANDIDATE" || a.RecentAddWalletCount < config.AppConfig.SmartLPMinWallets {
				continue
			}
			if opened, err := openWithAmount(a); err != nil {
				return err
			} else if opened {
				return nil
			}
		}
	}

	for _, a := range analyses {
		if a.Action != "CANDIDATE" {
			continue
		}
		if opened, err := openWithAmount(a); err != nil {
			return err
		} else if opened {
			return nil
		}
	}

	return nil
}

func (s *AutoLPService) hasEnoughUSDT(userID uint, amountUSDT float64) (bool, error) {
	if config.AppConfig == nil || !common.IsHexAddress(config.AppConfig.USDTAddress) {
		return false, fmt.Errorf("USDT address not set")
	}
	if blockchain.Client == nil {
		return false, fmt.Errorf("blockchain client not initialized")
	}

	ws := wallet.NewWalletService()
	wallet, err := ws.GetDefaultWallet(userID)
	if err != nil {
		return false, err
	}
	walletAddr := ws.GetWalletAddress(wallet)
	usdt := common.HexToAddress(config.AppConfig.USDTAddress)
	bal, err := blockchain.GetTokenBalance(usdt, walletAddr)
	if err != nil || bal == nil {
		return false, err
	}

	need, err := convert.FloatUSDTToWei(amountUSDT)
	if err != nil {
		return false, err
	}
	return bal.Cmp(need) >= 0, nil
}

func (s *AutoLPService) buildTaskForCandidate(ctx context.Context, userID uint, a AutoLPAnalysis, amountUSDT float64) (*models.StrategyTask, float64, error) {
	version := strings.ToLower(strings.TrimSpace(a.ProtocolVersion))
	poolID := strings.TrimSpace(a.PoolAddress)
	if version != "v3" && version != "v4" {
		return nil, 1, fmt.Errorf("unsupported protocol_version=%q", a.ProtocolVersion)
	}

	var info *pool.PoolInfo
	var err error
	switch version {
	case "v4":
		info, err = s.poolService.GetV4PoolInfo(poolID)
	default:
		info, err = s.poolService.GetPoolInfo(poolID)
	}
	if err != nil {
		return nil, 1, err
	}
	if info == nil || info.TickSpacing <= 0 {
		return nil, 1, fmt.Errorf("pool info invalid")
	}

	currentTick, err := getCurrentTickByVersion(version, poolID)
	if err != nil {
		return nil, 1, err
	}

	tc := pool.NewTickCalculator()
	// AutoLP widths are decided in stable price terms (e.g., "USDT price down/up").
	// Uniswap ticks are in token1/token0 terms, so when the stable coin is token0 we must convert.
	tmpTask := &models.StrategyTask{
		PoolId:        poolID,
		PoolVersion:   version,
		Token0Symbol:  info.Token0Symbol,
		Token1Symbol:  info.Token1Symbol,
		Token0Address: strings.TrimSpace(info.Token0),
		Token1Address: strings.TrimSpace(info.Token1),
	}
	tickLowerPctReq, tickUpperPctReq := pricing.TickPercentagesFromStablePercentages(tmpTask, a.LowerWidthPct, a.UpperWidthPct)
	if tickLowerPctReq <= 0 || tickUpperPctReq <= 0 {
		// Fallback: treat inputs as tick-price percentages if we cannot detect stable side.
		tickLowerPctReq = a.LowerWidthPct
		tickUpperPctReq = a.UpperWidthPct
	}

	tickLower, tickUpper := tc.CalculateTickFromPercentagesBestFit(currentTick, tickLowerPctReq, tickUpperPctReq, info.TickSpacing)
	if err := tc.ValidateTickRange(tickLower, tickUpper, info.TickSpacing); err != nil {
		return nil, 1, err
	}

	effLowerPct, effUpperPct := tc.CalculatePercentagesFromTicks(currentTick, tickLower, tickUpper)
	if effLowerPct <= 0 || effUpperPct <= 0 {
		effLowerPct = tickLowerPctReq
		effUpperPct = tickUpperPctReq
	}

	cfg, err := s.configService.GetOrCreate(userID)
	if err != nil {
		return nil, 1, err
	}

	gasMult := 1.0
	if a.State5 == "RAPID_PUMP" && config.AppConfig.AutoLPEmergencyGasMultiplier > 1 {
		gasMult = config.AppConfig.AutoLPEmergencyGasMultiplier
	}

	now := time.Now()

	task := &models.StrategyTask{
		UserID: userID,
		IsAuto: true,

		PoolId:      poolID,
		PoolVersion: version,
		Exchange:    info.Exchange,

		Token0Symbol:  info.Token0Symbol,
		Token1Symbol:  info.Token1Symbol,
		Token0Address: strings.TrimSpace(info.Token0),
		Token1Address: strings.TrimSpace(info.Token1),
		HooksAddress:  strings.TrimSpace(info.HooksAddress),

		Fee:         info.Fee,
		TickSpacing: info.TickSpacing,

		TickLower:  tickLower,
		TickUpper:  tickUpper,
		AmountUSDT: amountUSDT,

		RangePercentage:      a.BaseWidthPct,
		RangeLowerPercentage: effLowerPct,
		RangeUpperPercentage: effUpperPct,

		GuardOpenVolume5m:           a.TotalVolume5m,
		GuardOpenPrice:              a.CurrentPrice,
		GuardOpenTxCount5m:          int64(maxInt(a.TxCount5m, 0)),
		GuardOpenFeePercentage:      a.FeePercentage,
		GuardOpenFeeRate5mPct:       a.FeeRate5mPct,
		GuardOpenTotalFees5m:        a.TotalFees5m,
		GuardOpenTVLUSD:             a.TVLUSD,
		GuardOpenMetricsAt:          &now,
		GuardVolumeDropArmed:        false,
		GuardVolumeDropLastVolume5m: 0,
		GuardPriceTxDropArmed:       false,

		CurrentLiquidity:     "0",
		ReopenDelaySeconds:   cfg.RebalanceTimeout,
		SlippageTolerance:    cfg.SlippageTolerance,
		AutoReinvest:         cfg.AutoReinvest,
		ResidualTolerance:    cfg.ResidualTolerance,
		AllowEntrySwap:       config.AppConfig.AutoLPAllowEntrySwap,
		StopLossEnabled:      cfg.StopLossEnabled,
		StopLossDelaySeconds: cfg.StopLossDelaySeconds,

		Status:        models.StrategyStatusRunning,
		LastCheckTime: now,
	}

	if !common.IsHexAddress(task.HooksAddress) {
		task.HooksAddress = "0x0000000000000000000000000000000000000000"
	}

	// For V4 exit flow we assume Token0/Token1 addresses are ordered (c0 < c1).
	if version == "v4" && common.IsHexAddress(task.Token0Address) && common.IsHexAddress(task.Token1Address) {
		a0 := common.HexToAddress(task.Token0Address)
		a1 := common.HexToAddress(task.Token1Address)
		if bytesCompare(a0, a1) > 0 {
			task.Token0Address, task.Token1Address = task.Token1Address, task.Token0Address
			task.Token0Symbol, task.Token1Symbol = task.Token1Symbol, task.Token0Symbol
		}
	}

	return task, gasMult, nil
}

func (s *AutoLPService) hasActiveTask(userID uint, poolVersion string, poolID string) (bool, error) {
	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	poolID = strings.TrimSpace(poolID)
	if poolVersion == "" || poolID == "" {
		return false, nil
	}
	var count int64
	if err := database.DB.Model(&models.StrategyTask{}).
		Where("user_id = ? AND pool_version = ? AND pool_id = ? AND status IN ?", userID, poolVersion, poolID, []models.StrategyStatus{
			models.StrategyStatusRunning,
			models.StrategyStatusWaiting,
			models.StrategyStatusStopping,
		}).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *AutoLPService) countActiveAutoTasks(userID uint) (int64, error) {
	var count int64
	if err := database.DB.Model(&models.StrategyTask{}).
		Where("user_id = ? AND is_auto = ? AND status IN ?", userID, true, []models.StrategyStatus{
			models.StrategyStatusRunning,
			models.StrategyStatusWaiting,
			models.StrategyStatusStopping,
		}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func getCurrentTickByVersion(poolVersion string, poolID string) (int, error) {
	if config.AppConfig == nil {
		return 0, fmt.Errorf("config not loaded")
	}
	switch strings.ToLower(strings.TrimSpace(poolVersion)) {
	case "v4":
		if !common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
			return 0, fmt.Errorf("UNISWAP_V4_POOL_MANAGER_ADDRESS not set")
		}
		if !common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) {
			return 0, fmt.Errorf("UNISWAP_V4_STATE_VIEW_ADDRESS not set")
		}
		poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
		stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
		return blockchain.GetUniswapV4PoolCurrentTickViaStateView(stateView, poolManager, poolID)
	default:
		if !common.IsHexAddress(poolID) {
			return 0, fmt.Errorf("invalid V3 pool address: %s", poolID)
		}
		return blockchain.GetV3PoolCurrentTick(common.HexToAddress(poolID))
	}
}

func bytesCompare(a common.Address, b common.Address) int {
	ab := a.Bytes()
	bb := b.Bytes()
	for i := 0; i < len(ab) && i < len(bb); i++ {
		if ab[i] == bb[i] {
			continue
		}
		if ab[i] < bb[i] {
			return -1
		}
		return 1
	}
	if len(ab) == len(bb) {
		return 0
	}
	if len(ab) < len(bb) {
		return -1
	}
	return 1
}

func (s *AutoLPService) guardActiveAutoTasks(ctx context.Context, snap *poolMSnapshot, analyses []AutoLPAnalysis) {
	if database.DB == nil || config.AppConfig == nil {
		return
	}

	var tasks []models.StrategyTask
	if err := database.DB.Where("is_auto = ? AND paused = ? AND status IN ?", true, false, []models.StrategyStatus{
		models.StrategyStatusRunning,
		models.StrategyStatusWaiting,
	}).Find(&tasks).Error; err != nil {
		return
	}

	if len(tasks) == 0 {
		return
	}

	volumeDropPct := config.AppConfig.AutoLPGuardVolumeDropPercent
	if volumeDropPct > 1 && volumeDropPct <= 100 {
		volumeDropPct = volumeDropPct / 100
	}
	if volumeDropPct <= 0 || volumeDropPct >= 1 {
		volumeDropPct = 0.30
	}

	volumeDropPctLow := config.AppConfig.AutoLPGuardVolumeDropPercentLow
	if volumeDropPctLow > 1 && volumeDropPctLow <= 100 {
		volumeDropPctLow = volumeDropPctLow / 100
	}
	if volumeDropPctLow <= 0 || volumeDropPctLow >= 1 {
		volumeDropPctLow = 0
	}

	noExitMinFeeRate5m := config.AppConfig.AutoLPGuardNoExitMinFeeRate5m
	if noExitMinFeeRate5m < 0 {
		noExitMinFeeRate5m = 0
	}
	lowFeeRate5m := config.AppConfig.AutoLPGuardLowFeeRate5m
	if lowFeeRate5m < 0 {
		lowFeeRate5m = 0
	}

	priceTxDropPct := config.AppConfig.AutoLPGuardPriceTxDropPercent
	if priceTxDropPct > 1 && priceTxDropPct <= 100 {
		priceTxDropPct = priceTxDropPct / 100
	}
	if priceTxDropPct <= 0 || priceTxDropPct >= 1 {
		priceTxDropPct = 0.10
	}

	for i := range tasks {
		task := &tasks[i]
		if task.ExitGiveUpAt != nil {
			continue
		}
		pending := strings.TrimSpace(task.ExitPendingAction)
		if pending != "" && pending != strategy.ExitActionRebalance {
			continue
		}

		m, okMetrics, err := s.currentPoolM5mMetrics(ctx, snap, task)
		if err != nil || !okMetrics {
			continue
		}

		feeRatePct := 0.0
		if m.CurrentPoolValue > 0 {
			feeRatePct = (m.TotalFees / m.CurrentPoolValue) * 100
		}

		effectiveVolDropPct := volumeDropPct
		skipVolumeExit := false

		if noExitMinFeeRate5m > 0 || lowFeeRate5m > 0 {
			if noExitMinFeeRate5m > 0 && feeRatePct > noExitMinFeeRate5m {
				skipVolumeExit = true
			} else if lowFeeRate5m > 0 && feeRatePct < lowFeeRate5m && volumeDropPctLow > 0 {
				effectiveVolDropPct = volumeDropPctLow
			}
		}

		initUpdates := map[string]interface{}{}
		if task.GuardOpenVolume5m <= 0 && m.TotalVolume > 0 {
			metricsAt := time.Now()
			initUpdates["guard_open_volume_5m"] = m.TotalVolume
			initUpdates["guard_volume_drop_armed"] = false
			initUpdates["guard_volume_drop_last_volume_5m"] = 0
			initUpdates["guard_open_fee_percentage"] = m.FeePercentage
			initUpdates["guard_open_fee_rate_5m_pct"] = feeRatePct
			initUpdates["guard_open_total_fees_5m"] = m.TotalFees
			initUpdates["guard_open_tvl_usd"] = m.CurrentPoolValue
			initUpdates["guard_open_metrics_at"] = metricsAt
			task.GuardOpenVolume5m = m.TotalVolume
			task.GuardVolumeDropArmed = false
			task.GuardVolumeDropLastVolume5m = 0
			task.GuardOpenFeePercentage = m.FeePercentage
			task.GuardOpenFeeRate5mPct = feeRatePct
			task.GuardOpenTotalFees5m = m.TotalFees
			task.GuardOpenTVLUSD = m.CurrentPoolValue
			task.GuardOpenMetricsAt = &metricsAt
		}
		if task.GuardOpenPrice <= 0 && m.CurrentTokenPrice > 0 {
			initUpdates["guard_open_price"] = m.CurrentTokenPrice
			initUpdates["guard_price_tx_drop_armed"] = false
			task.GuardOpenPrice = m.CurrentTokenPrice
			task.GuardPriceTxDropArmed = false
		}
		if task.GuardOpenTxCount5m <= 0 && m.TransactionCount > 0 {
			initUpdates["guard_open_tx_count_5m"] = int64(m.TransactionCount)
			initUpdates["guard_price_tx_drop_armed"] = false
			task.GuardOpenTxCount5m = int64(m.TransactionCount)
			task.GuardPriceTxDropArmed = false
		}
		if len(initUpdates) > 0 {
			_ = database.DB.Model(task).Updates(initUpdates).Error
		}

		if !skipVolumeExit &&
			effectiveVolDropPct > 0 &&
			task.GuardOpenVolume5m > 0 &&
			m.TotalVolume > 0 {
			threshold := task.GuardOpenVolume5m * (1.0 - effectiveVolDropPct)
			hit := m.TotalVolume <= threshold

			if !hit {
				if task.GuardVolumeDropArmed {
					_ = database.DB.Model(task).Updates(map[string]interface{}{
						"guard_volume_drop_armed":          false,
						"guard_volume_drop_last_volume_5m": 0,
					}).Error
					task.GuardVolumeDropArmed = false
					task.GuardVolumeDropLastVolume5m = 0
				}
			} else {
				if !task.GuardVolumeDropArmed {
					_ = database.DB.Model(task).Updates(map[string]interface{}{
						"guard_volume_drop_armed":          true,
						"guard_volume_drop_last_volume_5m": m.TotalVolume,
					}).Error
					task.GuardVolumeDropArmed = true
					task.GuardVolumeDropLastVolume5m = m.TotalVolume
				} else if last := task.GuardVolumeDropLastVolume5m; last > 0 && m.TotalVolume < last {
					reason := fmt.Sprintf(
						"5m 成交量较开仓时下跌 >=%.0f%% 且继续下降（开仓=%.2f USDT 当前=%.2f USDT）",
						effectiveVolDropPct*100,
						task.GuardOpenVolume5m,
						m.TotalVolume,
					)
					if err := s.requestStopLossExit(task, reason, 1.0); err == nil {
						_ = strategy.NewAutoLPEventService().Record(task, models.AutoLPEventGuardExit, reason)
					}
					continue
				} else {
					_ = database.DB.Model(task).Updates(map[string]interface{}{
						"guard_volume_drop_last_volume_5m": m.TotalVolume,
					}).Error
					task.GuardVolumeDropLastVolume5m = m.TotalVolume
				}
			}
		}

		if priceTxDropPct > 0 &&
			task.GuardOpenPrice > 0 &&
			task.GuardOpenTxCount5m > 0 &&
			m.CurrentTokenPrice > 0 &&
			m.TransactionCount > 0 {
			priceHit := m.CurrentTokenPrice <= task.GuardOpenPrice*(1.0-priceTxDropPct)
			txHit := float64(m.TransactionCount) <= float64(task.GuardOpenTxCount5m)*(1.0-priceTxDropPct)
			hit := priceHit && txHit

			if !hit {
				if task.GuardPriceTxDropArmed {
					_ = database.DB.Model(task).Updates(map[string]interface{}{
						"guard_price_tx_drop_armed": false,
					}).Error
					task.GuardPriceTxDropArmed = false
				}
			} else if !task.GuardPriceTxDropArmed {
				_ = database.DB.Model(task).Updates(map[string]interface{}{
					"guard_price_tx_drop_armed": true,
				}).Error
				task.GuardPriceTxDropArmed = true
			} else {
				reason := fmt.Sprintf(
					"价格与交易笔数较开仓时下跌 >=%.0f%%（开仓价=%.6f 当前价=%.6f 开仓Tx=%d 当前Tx=%d）",
					priceTxDropPct*100,
					task.GuardOpenPrice,
					m.CurrentTokenPrice,
					task.GuardOpenTxCount5m,
					m.TransactionCount,
				)
				if err := s.requestStopLossExit(task, reason, 1.0); err == nil {
					_ = strategy.NewAutoLPEventService().Record(task, models.AutoLPEventGuardExit, reason)
				}
				continue
			}
		}
	}
}

type poolM5mMetrics struct {
	FeePercentage     float64
	TotalFees         float64
	TotalVolume       float64
	CurrentPoolValue  float64
	CurrentTokenPrice float64
	TransactionCount  uint64
}

func poolM5mMetricsFromSnapshot(snap *poolMSnapshot, task *models.StrategyTask) (poolM5mMetrics, bool) {
	if snap == nil || task == nil {
		return poolM5mMetrics{}, false
	}
	proto := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	pool := strings.ToLower(strings.TrimSpace(task.PoolId))
	if proto == "" || pool == "" {
		return poolM5mMetrics{}, false
	}
	tfs, ok := snap.data[poolKey{proto: proto, addr: pool}]
	if !ok {
		return poolM5mMetrics{}, false
	}
	p5, ok := tfs[5]
	if !ok {
		return poolM5mMetrics{}, false
	}
	return poolM5mMetrics{
		FeePercentage:     p5.FeePercentage,
		TotalFees:         p5.TotalFees,
		TotalVolume:       p5.TotalVolume,
		CurrentPoolValue:  p5.CurrentPoolValue,
		CurrentTokenPrice: p5.CurrentTokenPrice,
		TransactionCount:  uint64(maxInt(p5.TransactionCount, 0)),
	}, true
}

func (s *AutoLPService) latestPoolM5mMetrics(ctx context.Context, task *models.StrategyTask) (poolM5mMetrics, bool, error) {
	if s == nil || s.ch == nil || s.ch.Conn == nil || task == nil || config.AppConfig == nil {
		return poolM5mMetrics{}, false, nil
	}

	chain := strings.ToLower(strings.TrimSpace(config.AppConfig.AutoLPChain))
	proto := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	pool := strings.ToLower(strings.TrimSpace(task.PoolId))
	if chain == "" || proto == "" || pool == "" {
		return poolM5mMetrics{}, false, nil
	}

	q := `
		SELECT
			argMax(fee_percentage, ts) AS current_fee_pct,
			argMax(total_fees, ts) AS current_fees,
			argMax(total_volume, ts) AS current_vol,
			argMax(current_pool_value, ts) AS current_tvl,
			argMax(current_token_price, ts) AS current_price,
			argMax(transaction_count, ts) AS current_tx,
			count() AS n
		FROM poolm_top_fees_raw
		WHERE chain = ? AND protocol_version = ? AND timeframe_minutes = 5 AND pool_address = ?
	`

	var feePct float64
	var fees float64
	var vol float64
	var tvl float64
	var price float64
	var tx uint64
	var n uint64
	if err := s.ch.Conn.QueryRow(ctx, q, chain, proto, pool).Scan(&feePct, &fees, &vol, &tvl, &price, &tx, &n); err != nil {
		return poolM5mMetrics{}, false, err
	}
	if n < 1 {
		return poolM5mMetrics{}, false, nil
	}
	return poolM5mMetrics{
		FeePercentage:     feePct,
		TotalFees:         fees,
		TotalVolume:       vol,
		CurrentPoolValue:  tvl,
		CurrentTokenPrice: price,
		TransactionCount:  tx,
	}, true, nil
}

func (s *AutoLPService) currentPoolM5mMetrics(ctx context.Context, snap *poolMSnapshot, task *models.StrategyTask) (poolM5mMetrics, bool, error) {
	if m, ok := poolM5mMetricsFromSnapshot(snap, task); ok {
		return m, true, nil
	}
	return s.latestPoolM5mMetrics(ctx, task)
}

func poolM5mFeeRatePctFromSnapshot(snap *poolMSnapshot, task *models.StrategyTask) (float64, bool) {
	if snap == nil || task == nil {
		return 0, false
	}
	proto := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	pool := strings.ToLower(strings.TrimSpace(task.PoolId))
	if proto == "" || pool == "" {
		return 0, false
	}
	tfs, ok := snap.data[poolKey{proto: proto, addr: pool}]
	if !ok {
		return 0, false
	}
	p5, ok := tfs[5]
	if !ok || p5.CurrentPoolValue <= 0 {
		return 0, false
	}
	return (p5.TotalFees / p5.CurrentPoolValue) * 100, true
}

func (s *AutoLPService) latestFeeRatePctWithin(ctx context.Context, task *models.StrategyTask, window time.Duration) (float64, bool, error) {
	if s.ch == nil || s.ch.Conn == nil || task == nil || config.AppConfig == nil {
		return 0, false, nil
	}
	if window <= 0 {
		return 0, false, nil
	}

	chain := strings.ToLower(strings.TrimSpace(config.AppConfig.AutoLPChain))
	proto := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	pool := strings.ToLower(strings.TrimSpace(task.PoolId))
	if chain == "" || proto == "" || pool == "" {
		return 0, false, nil
	}

	windowSeconds := int(window.Seconds())
	if windowSeconds <= 0 {
		return 0, false, nil
	}

	q := fmt.Sprintf(`
		SELECT
			argMax(total_fees, ts) AS current_fees,
			argMax(current_pool_value, ts) AS current_tvl,
			count() AS n
		FROM poolm_top_fees_raw
		WHERE chain = ? AND protocol_version = ? AND timeframe_minutes = 5 AND pool_address = ?
		  AND ts >= now() - INTERVAL %d SECOND
	`, windowSeconds)

	var currentFees float64
	var currentTVL float64
	var n uint64
	if err := s.ch.Conn.QueryRow(ctx, q, chain, proto, pool).Scan(&currentFees, &currentTVL, &n); err != nil {
		return 0, false, err
	}
	if n < 1 || currentTVL <= 0 {
		return 0, false, nil
	}
	return (currentFees / currentTVL) * 100, true, nil
}

func (s *AutoLPService) requestStopLossExit(task *models.StrategyTask, reason string, gasMultiplier float64) error {
	if task == nil {
		return nil
	}

	if task.ExitGiveUpAt != nil {
		return nil
	}
	pending := strings.TrimSpace(task.ExitPendingAction)
	if pending != "" && pending != strategy.ExitActionRebalance {
		return nil
	}

	updates := map[string]interface{}{
		"exit_pending_action": strategy.ExitActionStopLoss,
		"exit_pending_reason": strings.TrimSpace(reason),
		"exit_gas_multiplier": gasMultiplier,
		"exit_retry_count":    0,
		"exit_next_retry_at":  nil,
		"exit_last_error":     "",
		"exit_give_up_at":     nil,
		"error_message":       "",
		"out_of_range_since":  nil,
		"last_check_time":     time.Now(),
	}
	if err := database.DB.Model(task).Updates(updates).Error; err != nil {
		return err
	}

	task.ExitPendingAction = strategy.ExitActionStopLoss
	task.ExitPendingReason = strings.TrimSpace(reason)
	task.ExitGasMultiplier = gasMultiplier
	task.ExitRetryCount = 0
	task.ExitNextRetryAt = nil
	task.ExitLastError = ""
	task.ExitGiveUpAt = nil

	return nil
}

func (s *AutoLPService) checkVolumeDropWithin(ctx context.Context, task *models.StrategyTask, window time.Duration, dropPct float64) (bool, float64, float64, error) {
	if s.ch == nil || s.ch.Conn == nil || task == nil || config.AppConfig == nil {
		return false, 0, 0, nil
	}
	if window <= 0 || dropPct <= 0 || dropPct >= 1 {
		return false, 0, 0, nil
	}

	chain := strings.ToLower(strings.TrimSpace(config.AppConfig.AutoLPChain))
	proto := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	pool := strings.ToLower(strings.TrimSpace(task.PoolId))
	if chain == "" || proto == "" || pool == "" {
		return false, 0, 0, nil
	}

	windowSeconds := int(window.Seconds())
	if windowSeconds <= 0 {
		return false, 0, 0, nil
	}
	q := fmt.Sprintf(`
		SELECT
			argMax(total_volume, ts) AS current_vol,
			max(total_volume) AS max_vol,
			count() AS n
		FROM poolm_top_fees_raw
		WHERE chain = ? AND protocol_version = ? AND timeframe_minutes = 5 AND pool_address = ?
		  AND ts >= now() - INTERVAL %d SECOND
	`, windowSeconds)

	var currentVol float64
	var maxVol float64
	var n uint64
	if err := s.ch.Conn.QueryRow(ctx, q, chain, proto, pool).Scan(&currentVol, &maxVol, &n); err != nil {
		return false, 0, 0, err
	}
	if n < 2 || currentVol <= 0 || maxVol <= 0 {
		return false, currentVol, maxVol, nil
	}

	return currentVol <= maxVol*(1.0-dropPct), currentVol, maxVol, nil
}

func (s *AutoLPService) checkPriceAndTxDropWithin(ctx context.Context, task *models.StrategyTask, window time.Duration, dropPct float64) (bool, error) {
	if s.ch == nil || s.ch.Conn == nil || task == nil || config.AppConfig == nil {
		return false, nil
	}
	if window <= 0 || dropPct <= 0 || dropPct >= 1 {
		return false, nil
	}

	chain := strings.ToLower(strings.TrimSpace(config.AppConfig.AutoLPChain))
	proto := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	pool := strings.ToLower(strings.TrimSpace(task.PoolId))
	if chain == "" || proto == "" || pool == "" {
		return false, nil
	}

	windowSeconds := int(window.Seconds())
	if windowSeconds <= 0 {
		return false, nil
	}
	q := fmt.Sprintf(`
		SELECT
			argMax(current_token_price, ts) AS current_price,
			max(current_token_price) AS max_price,
			argMax(transaction_count, ts) AS current_tx,
			max(transaction_count) AS max_tx,
			count() AS n
		FROM poolm_top_fees_raw
		WHERE chain = ? AND protocol_version = ? AND timeframe_minutes = 5 AND pool_address = ?
		  AND ts >= now() - INTERVAL %d SECOND
	`, windowSeconds)

	var currentPrice float64
	var maxPrice float64
	var currentTx uint64
	var maxTx uint64
	var n uint64
	if err := s.ch.Conn.QueryRow(ctx, q, chain, proto, pool).Scan(&currentPrice, &maxPrice, &currentTx, &maxTx, &n); err != nil {
		return false, err
	}
	if n < 2 || currentPrice <= 0 || maxPrice <= 0 || currentTx == 0 || maxTx == 0 {
		return false, nil
	}

	priceDrop := currentPrice <= maxPrice*(1.0-dropPct)
	txDrop := float64(currentTx) <= float64(maxTx)*(1.0-dropPct)
	return priceDrop && txDrop, nil
}

func autoLPStateZh(state string) string {
	switch strings.TrimSpace(state) {
	case "CRASH":
		return "暴跌(止损撤出)"
	case "RAPID_PUMP":
		return "急涨"
	case "SIDEWAYS":
		return "震荡"
	case "MILD_UPTREND":
		return "温和上涨"
	case "MILD_DOWNTREND":
		return "温和下跌"
	case "CONSOLIDATION":
		return "收敛(缩窄)"
	case "FALLBACK":
		return "回退(默认网格)"
	default:
		return "未知"
	}
}

func autoLPTrendZh(trend string) string {
	switch strings.TrimSpace(trend) {
	case "UPTREND":
		return "上涨趋势"
	case "DOWNTREND":
		return "下跌趋势"
	case "SIDEWAYS":
		return "横盘"
	case "UNKNOWN":
		return "未知"
	default:
		return "未知"
	}
}

func autoLPResonanceZh(res string) string {
	switch strings.TrimSpace(res) {
	case "STRONG":
		return "强共振(尝试2x)"
	case "DIVERGENCE":
		return "背离(加宽)"
	case "NONE":
		return "无"
	default:
		return "无"
	}
}

func autoLPActionZh(action string) string {
	switch strings.TrimSpace(action) {
	case "CANDIDATE":
		return "候选"
	case "SKIP":
		return "跳过"
	default:
		return "未知"
	}
}
