package smart_money_golden_dog

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/notify"
	"TgLpBot/base/security"
	userSvc "TgLpBot/service/user"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultMinWallets         = 3
	DefaultWindowMinutes      = 10
	DefaultCooldownMinutes    = 30
	defaultScanInterval       = 20 * time.Second
	defaultPoolSnapshotMaxAge = 15 * time.Minute
	defaultBarkSound          = "alarm"
)

const (
	BarkIntensityRing           = "ring"
	BarkIntensityPersistentRing = "persistent_ring"
	BarkIntensityCriticalRing   = "critical_ring"
)

const (
	WalletIntensityModeFixed       = "fixed"
	WalletIntensityModeAmountTiers = "amount_tiers"
)

type BarkIntensityOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type AmountIntensityTier struct {
	MinAmountUSD float64 `json:"min_amount_usd"`
	Intensity    string  `json:"intensity"`
}

type BarkStatus struct {
	Enabled    bool
	Configured bool
	Ready      bool
	Config     notify.BarkConfig
}

type Service struct {
	repo     *Repository
	cancel   context.CancelFunc
	interval time.Duration
}

type pairBucket struct {
	Key    string
	Label  string
	Events []pairBucketEvent
}

type pairBucketEvent struct {
	Wallet    string
	SeenAt    time.Time
	AmountUSD float64
}

type pairSignal struct {
	Key            string
	Label          string
	WalletCount    int
	LatestAt       time.Time
	TotalAmountUSD float64
}

type poolSignal struct {
	Key                  string
	Label                string
	Address              string
	TransactionCount     int
	TotalFees            float64
	TotalVolume          float64
	CurrentPoolValue     float64
	FeeRate              int
	ActiveLiquidityRatio float64
	UpdatedAt            time.Time
}

func BarkIntensityOptions() []BarkIntensityOption {
	return []BarkIntensityOption{
		{
			Value:       BarkIntensityRing,
			Label:       "响铃",
			Description: "普通响铃提醒，适合常规跟单监控。",
		},
		{
			Value:       BarkIntensityPersistentRing,
			Label:       "持续响铃",
			Description: "开启 Bark call 模式，提醒会持续响铃直到处理。",
		},
		{
			Value:       BarkIntensityCriticalRing,
			Label:       "静音强提醒",
			Description: "使用 Bark critical 级别，在静音模式下也会强提醒。",
		},
	}
}

func NewService() *Service {
	return &Service{
		repo:     NewRepository(),
		interval: defaultScanInterval,
	}
}

func (s *Service) Start() {
	if config.AppConfig == nil || !config.AppConfig.SmartMoneyEnabled {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go s.loop(ctx)
}

func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Service) loop(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	if err := s.runOnce(ctx); err != nil {
		log.Printf("[GoldenDog] initial scan failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.runOnce(ctx); err != nil {
				log.Printf("[GoldenDog] scan failed: %v", err)
			}
		}
	}
}

func (s *Service) runOnce(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return nil
	}

	configs, err := s.repo.ListEnabledConfigs(ctx)
	if err != nil {
		return fmt.Errorf("list enabled configs: %w", err)
	}
	if len(configs) == 0 {
		return nil
	}

	type chainWork struct {
		maxWalletWindow int
		needWalletScan  bool
		needPoolScan    bool
		configs         []models.SmartMoneyGoldenDogConfig
		buckets         map[string]*pairBucket
		pools           []models.Pool
	}
	workByChain := make(map[string]*chainWork)

	for _, cfg := range configs {
		chain := normalizeChain(cfg.Chain)
		work := workByChain[chain]
		if work == nil {
			work = &chainWork{}
			workByChain[chain] = work
		}
		if cfg.Enabled {
			work.needWalletScan = true
		}
		if cfg.PoolEnabled {
			work.needPoolScan = true
		}
		if cfg.WindowMinutes > work.maxWalletWindow {
			work.maxWalletWindow = cfg.WindowMinutes
		}
		work.configs = append(work.configs, cfg)
	}

	now := time.Now()
	for chain, work := range workByChain {
		if work.needWalletScan {
			chainID := chainIDFor(chain)
			if chainID > 0 {
				window := clampWindowMinutes(work.maxWalletWindow)
				events, err := s.repo.ListRecentAddEvents(ctx, chainID, now.Add(-time.Duration(window)*time.Minute))
				if err != nil {
					return fmt.Errorf("list recent add events chain=%s: %w", chain, err)
				}
				work.buckets = buildPairBuckets(events)
			}
		}

		if work.needPoolScan {
			pools, err := s.repo.ListFreshPools(ctx, chain, now.Add(-defaultPoolSnapshotMaxAge))
			if err != nil {
				return fmt.Errorf("list fresh pools chain=%s: %w", chain, err)
			}
			work.pools = pools
		}
	}

	barkCache := make(map[uint]BarkStatus)
	for chain, work := range workByChain {
		for _, cfg := range work.configs {
			if cfg.Enabled {
				signals := pairSignalsForConfig(work.buckets, now, cfg)
				if len(signals) > 0 {
					barkStatus, err := resolveReadyBarkStatus(ctx, barkCache, cfg.UserID)
					if err != nil {
						log.Printf("[GoldenDog] resolve bark status failed user=%d: %v", cfg.UserID, err)
						continue
					}
					if barkStatus.Ready {
						for _, signal := range signals {
							stateKey := walletSignalStateKey(signal.Key)
							state, err := s.repo.GetAlertState(ctx, cfg.UserID, chain, stateKey)
							if err != nil {
								return fmt.Errorf("load wallet alert state user=%d chain=%s pair=%s: %w", cfg.UserID, chain, signal.Key, err)
							}
							if cooldownActive(state, now, clampCooldownMinutes(cfg.CooldownMinutes)) {
								continue
							}

							intensity := ResolveWalletBarkIntensity(cfg, signal.TotalAmountUSD)
							title, body := buildWalletBarkMessage(signal, cfg)
							if err := notify.SendBarkWithConfig(title, body, barkConfigForIntensity(barkStatus.Config, intensity)); err != nil {
								log.Printf("[GoldenDog] bark notify failed user=%d pair=%s: %v", cfg.UserID, signal.Key, err)
								continue
							}

							err = s.repo.UpsertAlertState(ctx, &models.SmartMoneyGoldenDogAlertState{
								UserID:         cfg.UserID,
								Chain:          chain,
								PairKey:        stateKey,
								PairLabel:      signal.Label,
								LastWallets:    signal.WalletCount,
								LastNotifiedAt: now,
							})
							if err != nil {
								return fmt.Errorf("save wallet alert state user=%d chain=%s pair=%s: %w", cfg.UserID, chain, signal.Key, err)
							}
						}
					}
				}
			}

			if cfg.PoolEnabled {
				signals := poolSignalsForConfig(work.pools, cfg)
				if len(signals) == 0 {
					continue
				}

				barkStatus, err := resolveReadyBarkStatus(ctx, barkCache, cfg.UserID)
				if err != nil {
					log.Printf("[GoldenDog] resolve bark status failed user=%d: %v", cfg.UserID, err)
					continue
				}
				if !barkStatus.Ready {
					continue
				}

				for _, signal := range signals {
					state, err := s.repo.GetAlertState(ctx, cfg.UserID, chain, signal.Key)
					if err != nil {
						return fmt.Errorf("load pool alert state user=%d chain=%s pool=%s: %w", cfg.UserID, chain, signal.Address, err)
					}
					if cooldownActive(state, now, clampCooldownMinutes(cfg.PoolCooldownMinutes)) {
						continue
					}

					title, body := buildPoolBarkMessage(signal)
					if err := notify.SendBarkWithConfig(title, body, barkConfigForIntensity(barkStatus.Config, cfg.PoolIntensity)); err != nil {
						log.Printf("[GoldenDog] bark notify failed user=%d pool=%s: %v", cfg.UserID, signal.Address, err)
						continue
					}

					err = s.repo.UpsertAlertState(ctx, &models.SmartMoneyGoldenDogAlertState{
						UserID:         cfg.UserID,
						Chain:          chain,
						PairKey:        signal.Key,
						PairLabel:      signal.Label,
						LastWallets:    signal.TransactionCount,
						LastNotifiedAt: now,
					})
					if err != nil {
						return fmt.Errorf("save pool alert state user=%d chain=%s pool=%s: %w", cfg.UserID, chain, signal.Address, err)
					}
				}
			}
		}
	}

	return nil
}

func ResolveUserBarkStatus(ctx context.Context, userID uint) (BarkStatus, error) {
	var status BarkStatus
	if config.AppConfig == nil {
		return status, fmt.Errorf("config not loaded")
	}

	cfg, err := userSvc.NewGlobalConfigService().GetOrCreate(userID)
	if err != nil {
		return status, err
	}

	status.Enabled = cfg.BarkEnabled
	status.Configured = strings.TrimSpace(cfg.BarkKeyEncrypted) != ""
	if !status.Enabled || !status.Configured {
		return status, nil
	}

	keyBytes, err := security.DecodeHexKey32(config.AppConfig.EncryptionKey)
	if err != nil {
		return status, err
	}
	plain, err := security.DecryptAESGCMHex(keyBytes, cfg.BarkKeyEncrypted)
	if err != nil {
		return status, err
	}

	status.Ready = true
	status.Config = notify.BarkConfig{
		Server: cfg.BarkServer,
		Key:    strings.TrimSpace(string(plain)),
		Group:  cfg.BarkGroup,
	}
	return status, nil
}

func resolveReadyBarkStatus(ctx context.Context, cache map[uint]BarkStatus, userID uint) (BarkStatus, error) {
	if cache == nil {
		return ResolveUserBarkStatus(ctx, userID)
	}
	if status, ok := cache[userID]; ok {
		return status, nil
	}
	status, err := ResolveUserBarkStatus(ctx, userID)
	if err != nil {
		return BarkStatus{}, err
	}
	cache[userID] = status
	return status, nil
}

func normalizeBarkIntensity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case BarkIntensityPersistentRing:
		return BarkIntensityPersistentRing
	case BarkIntensityCriticalRing:
		return BarkIntensityCriticalRing
	default:
		return BarkIntensityRing
	}
}

func NormalizeBarkIntensity(value string) string {
	return normalizeBarkIntensity(value)
}

func normalizeWalletIntensityMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case WalletIntensityModeAmountTiers, "tiered", "amount", "amount_tier":
		return WalletIntensityModeAmountTiers
	default:
		return WalletIntensityModeFixed
	}
}

func NormalizeWalletIntensityMode(value string) string {
	return normalizeWalletIntensityMode(value)
}

func NormalizeAmountIntensityTiers(tiers []AmountIntensityTier) []AmountIntensityTier {
	if len(tiers) == 0 {
		return nil
	}
	out := make([]AmountIntensityTier, 0, len(tiers))
	for _, tier := range tiers {
		minAmount := clampMoneyThreshold(tier.MinAmountUSD)
		if minAmount <= 0 {
			continue
		}
		out = append(out, AmountIntensityTier{
			MinAmountUSD: minAmount,
			Intensity:    normalizeBarkIntensity(tier.Intensity),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MinAmountUSD == out[j].MinAmountUSD {
			return out[i].Intensity < out[j].Intensity
		}
		return out[i].MinAmountUSD < out[j].MinAmountUSD
	})
	if len(out) > 10 {
		out = out[len(out)-10:]
	}
	return out
}

func EncodeAmountIntensityTiers(tiers []AmountIntensityTier) string {
	normalized := NormalizeAmountIntensityTiers(tiers)
	if len(normalized) == 0 {
		return ""
	}
	buf, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(buf)
}

func DecodeAmountIntensityTiers(raw string) []AmountIntensityTier {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var tiers []AmountIntensityTier
	if err := json.Unmarshal([]byte(raw), &tiers); err != nil {
		return nil
	}
	return NormalizeAmountIntensityTiers(tiers)
}

func ResolveWalletBarkIntensity(cfg models.SmartMoneyGoldenDogConfig, totalAmountUSD float64) string {
	fallback := normalizeBarkIntensity(cfg.WalletIntensity)
	if normalizeWalletIntensityMode(cfg.WalletIntensityMode) != WalletIntensityModeAmountTiers {
		return fallback
	}
	selected := fallback
	for _, tier := range DecodeAmountIntensityTiers(cfg.WalletAmountIntensityTiers) {
		if totalAmountUSD+0.0000001 >= tier.MinAmountUSD {
			selected = normalizeBarkIntensity(tier.Intensity)
		}
	}
	return selected
}

func barkConfigForIntensity(base notify.BarkConfig, intensity string) notify.BarkConfig {
	cfg := base
	cfg.Call = ""
	cfg.Level = ""
	if strings.TrimSpace(cfg.Sound) == "" {
		cfg.Sound = defaultBarkSound
	}

	switch normalizeBarkIntensity(intensity) {
	case BarkIntensityPersistentRing:
		cfg.Call = "1"
	case BarkIntensityCriticalRing:
		cfg.Level = "critical"
	}
	return cfg
}

func BarkConfigForIntensity(base notify.BarkConfig, intensity string) notify.BarkConfig {
	return barkConfigForIntensity(base, intensity)
}

func normalizeChain(chain string) string {
	normalized := config.NormalizeChain(chain)
	if normalized == "" {
		return "bsc"
	}
	return normalized
}

func chainIDFor(chain string) int {
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok && cc.ChainID > 0 {
			return int(cc.ChainID)
		}
	}
	switch normalizeChain(chain) {
	case "base":
		return 8453
	default:
		return 56
	}
}

func clampMinWallets(value int) int {
	if value < 1 {
		return 1
	}
	if value > 100 {
		return 100
	}
	return value
}

func clampWindowMinutes(value int) int {
	if value < 1 {
		return DefaultWindowMinutes
	}
	if value > 1440 {
		return 1440
	}
	return value
}

func clampCooldownMinutes(value int) int {
	if value < 0 {
		return DefaultCooldownMinutes
	}
	if value > 10080 {
		return 10080
	}
	return value
}

func clampPoolMetricThreshold(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1_000_000_000_000 {
		return 1_000_000_000_000
	}
	return value
}

func clampMoneyThreshold(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	if value > 1_000_000_000_000 {
		return 1_000_000_000_000
	}
	return value
}

func clampPoolMetricCount(value int) int {
	if value < 0 {
		return 0
	}
	if value > 1_000_000_000 {
		return 1_000_000_000
	}
	return value
}

func cooldownActive(state *models.SmartMoneyGoldenDogAlertState, now time.Time, cooldownMinutes int) bool {
	if state == nil || cooldownMinutes <= 0 {
		return false
	}
	return now.Sub(state.LastNotifiedAt) < time.Duration(cooldownMinutes)*time.Minute
}

func buildPairBuckets(events []models.SmartMoneyLPEvent) map[string]*pairBucket {
	out := make(map[string]*pairBucket)
	for _, event := range events {
		pairKey, pairLabel := canonicalPair(event.Token0Address, event.Token1Address, event.Token0Symbol, event.Token1Symbol)
		if pairKey == "" {
			continue
		}
		wallet := strings.ToLower(strings.TrimSpace(event.WalletAddress))
		if wallet == "" {
			continue
		}

		bucket := out[pairKey]
		if bucket == nil {
			bucket = &pairBucket{
				Key:   pairKey,
				Label: pairLabel,
			}
			out[pairKey] = bucket
		}
		if bucket.Label == "" && pairLabel != "" {
			bucket.Label = pairLabel
		}
		bucket.Events = append(bucket.Events, pairBucketEvent{
			Wallet:    wallet,
			SeenAt:    event.TxTimestamp,
			AmountUSD: eventAmountUSD(event),
		})
	}
	return out
}

func pairSignalsForConfig(buckets map[string]*pairBucket, now time.Time, cfg models.SmartMoneyGoldenDogConfig) []*pairSignal {
	if len(buckets) == 0 {
		return nil
	}

	cutoff := now.Add(-time.Duration(clampWindowMinutes(cfg.WindowMinutes)) * time.Minute)
	minWallets := clampMinWallets(cfg.MinWallets)
	minTotalAmountUSD := clampMoneyThreshold(cfg.WalletMinTotalAmountUSD)

	signals := make([]*pairSignal, 0, len(buckets))
	for _, bucket := range buckets {
		walletSeen := make(map[string]time.Time)
		latestAt := time.Time{}
		totalAmountUSD := 0.0
		for _, event := range bucket.Events {
			if event.SeenAt.Before(cutoff) {
				continue
			}
			totalAmountUSD += event.AmountUSD
			if event.SeenAt.After(latestAt) {
				latestAt = event.SeenAt
			}
			if seenAt, ok := walletSeen[event.Wallet]; !ok || event.SeenAt.After(seenAt) {
				walletSeen[event.Wallet] = event.SeenAt
			}
		}
		count := len(walletSeen)
		if count < minWallets {
			continue
		}
		if minTotalAmountUSD > 0 && totalAmountUSD < minTotalAmountUSD {
			continue
		}
		signals = append(signals, &pairSignal{
			Key:            bucket.Key,
			Label:          bucket.Label,
			WalletCount:    count,
			LatestAt:       latestAt,
			TotalAmountUSD: totalAmountUSD,
		})
	}
	sort.Slice(signals, func(i, j int) bool {
		if signals[i].WalletCount != signals[j].WalletCount {
			return signals[i].WalletCount > signals[j].WalletCount
		}
		if signals[i].TotalAmountUSD != signals[j].TotalAmountUSD {
			return signals[i].TotalAmountUSD > signals[j].TotalAmountUSD
		}
		return signals[i].LatestAt.After(signals[j].LatestAt)
	})
	return signals
}

func eventAmountUSD(event models.SmartMoneyLPEvent) float64 {
	if event.TotalUSD != nil {
		if value := parseDecimalString(*event.TotalUSD); value > 0 {
			return value
		}
	}
	return parseDecimalStringPtr(event.Token0AmountUSD) + parseDecimalStringPtr(event.Token1AmountUSD)
}

func parseDecimalStringPtr(value *string) float64 {
	if value == nil {
		return 0
	}
	return parseDecimalString(*value)
}

func parseDecimalString(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	out, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(out) || math.IsInf(out, 0) || out < 0 {
		return 0
	}
	return out
}

func canonicalPair(token0Address string, token1Address string, token0Symbol string, token1Symbol string) (string, string) {
	leftAddr := strings.ToLower(strings.TrimSpace(token0Address))
	rightAddr := strings.ToLower(strings.TrimSpace(token1Address))
	leftSymbol := strings.ToUpper(strings.TrimSpace(token0Symbol))
	rightSymbol := strings.ToUpper(strings.TrimSpace(token1Symbol))
	if leftAddr == "" || rightAddr == "" {
		return "", ""
	}
	if leftAddr > rightAddr {
		leftAddr, rightAddr = rightAddr, leftAddr
		leftSymbol, rightSymbol = rightSymbol, leftSymbol
	}
	labelLeft := firstNonEmpty(leftSymbol, shortenAddress(leftAddr))
	labelRight := firstNonEmpty(rightSymbol, shortenAddress(rightAddr))
	return leftAddr + "|" + rightAddr, labelLeft + "/" + labelRight
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func shortenAddress(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 10 {
		return value
	}
	return value[:6] + "..." + value[len(value)-4:]
}

func buildBarkMessage(signal *pairSignal, cfg models.SmartMoneyGoldenDogConfig) (string, string) {
	label := firstNonEmpty(signal.Label, "未知交易对")
	title := "金狗通知"
	body := fmt.Sprintf("%s 在 %d 分钟内出现 %d 个聪明钱钱包加 LP，合计 %s，建议立即关注", label, clampWindowMinutes(cfg.WindowMinutes), signal.WalletCount, formatMetricCompact(signal.TotalAmountUSD, "$"))
	return title, body
}

func walletSignalStateKey(pairKey string) string {
	return "wallet_pair:" + strings.TrimSpace(pairKey)
}

func poolSignalStateKey(poolAddress string) string {
	return "pool_metric:" + strings.ToLower(strings.TrimSpace(poolAddress))
}

func HasPoolThresholds(cfg models.SmartMoneyGoldenDogConfig) bool {
	return hasAnyPoolThreshold(cfg)
}

func hasAnyPoolThreshold(cfg models.SmartMoneyGoldenDogConfig) bool {
	return clampPoolMetricThreshold(cfg.PoolMinTotalFees) > 0 ||
		clampPoolMetricCount(cfg.PoolMinTransactionCount) > 0 ||
		clampPoolMetricThreshold(cfg.PoolMinTVL) > 0 ||
		clampPoolMetricThreshold(cfg.PoolMinVolume) > 0 ||
		clampPoolMetricThreshold(cfg.PoolMinFeeRate) > 0 ||
		clampPoolMetricThreshold(cfg.PoolMinActiveLiquidityRatio) > 0
}

func poolSignalsForConfig(pools []models.Pool, cfg models.SmartMoneyGoldenDogConfig) []*poolSignal {
	if len(pools) == 0 || !hasAnyPoolThreshold(cfg) {
		return nil
	}

	minTotalFees := clampPoolMetricThreshold(cfg.PoolMinTotalFees)
	minTransactions := clampPoolMetricCount(cfg.PoolMinTransactionCount)
	minTVL := clampPoolMetricThreshold(cfg.PoolMinTVL)
	minVolume := clampPoolMetricThreshold(cfg.PoolMinVolume)
	minFeeRate := clampPoolMetricThreshold(cfg.PoolMinFeeRate)                  // 百分比，如 0.005 表示 0.005%
	minActiveRatio := clampPoolMetricThreshold(cfg.PoolMinActiveLiquidityRatio) // 百分比，如 0.168 表示 0.168%

	signals := make([]*poolSignal, 0, len(pools))
	for _, pool := range pools {
		if strings.TrimSpace(pool.Address) == "" {
			continue
		}
		if minTotalFees > 0 && pool.TotalFees < minTotalFees {
			continue
		}
		if minTransactions > 0 && int(pool.TransactionCount) < minTransactions {
			continue
		}
		if minTVL > 0 && pool.CurrentPoolValue < minTVL {
			continue
		}
		if minVolume > 0 && pool.TotalVolume < minVolume {
			continue
		}
		// 费率 = fees / tvl * 100（百分比），与热门池子计算方式一致
		if minFeeRate > 0 {
			poolFeeRate := 0.0
			if pool.CurrentPoolValue > 0 && pool.TotalFees > 0 {
				poolFeeRate = pool.TotalFees / pool.CurrentPoolValue * 100.0
			}
			if poolFeeRate < minFeeRate {
				continue
			}
		}
		// 活跃费率 = fees / activeLiquidityUSD * 100（百分比），与热门池子计算方式一致
		if minActiveRatio > 0 {
			poolActiveRate := 0.0
			if pool.ActiveLiquidityUSD > 0 && pool.TotalFees > 0 {
				poolActiveRate = pool.TotalFees / pool.ActiveLiquidityUSD * 100.0
			}
			if poolActiveRate < minActiveRatio {
				continue
			}
		}

		signals = append(signals, &poolSignal{
			Key:                  poolSignalStateKey(pool.Address),
			Label:                poolLabel(pool),
			Address:              strings.ToLower(strings.TrimSpace(pool.Address)),
			TransactionCount:     int(pool.TransactionCount),
			TotalFees:            pool.TotalFees,
			TotalVolume:          pool.TotalVolume,
			CurrentPoolValue:     pool.CurrentPoolValue,
			FeeRate:              pool.PoolMFeeRate,
			ActiveLiquidityRatio: pool.ActiveLiquidityRatio,
			UpdatedAt:            pool.UpdatedAt,
		})
	}

	sort.Slice(signals, func(i, j int) bool {
		if signals[i].TotalFees != signals[j].TotalFees {
			return signals[i].TotalFees > signals[j].TotalFees
		}
		if signals[i].TotalVolume != signals[j].TotalVolume {
			return signals[i].TotalVolume > signals[j].TotalVolume
		}
		if signals[i].CurrentPoolValue != signals[j].CurrentPoolValue {
			return signals[i].CurrentPoolValue > signals[j].CurrentPoolValue
		}
		if signals[i].TransactionCount != signals[j].TransactionCount {
			return signals[i].TransactionCount > signals[j].TransactionCount
		}
		return signals[i].UpdatedAt.After(signals[j].UpdatedAt)
	})
	return signals
}

func poolLabel(pool models.Pool) string {
	if label := strings.TrimSpace(pool.Name); label != "" {
		return label
	}
	left := strings.TrimSpace(pool.Token0Symbol)
	right := strings.TrimSpace(pool.Token1Symbol)
	switch {
	case left != "" && right != "":
		return left + "/" + right
	case left != "":
		return left
	case right != "":
		return right
	default:
		return shortenAddress(pool.Address)
	}
}

func buildWalletBarkMessage(signal *pairSignal, cfg models.SmartMoneyGoldenDogConfig) (string, string) {
	label := firstNonEmpty(signal.Label, "未知交易对")
	title := "金狗通知"
	body := fmt.Sprintf("%s 在 %d 分钟内出现 %d 个聪明钱钱包加 LP，合计 %s，建议立即关注", label, clampWindowMinutes(cfg.WindowMinutes), signal.WalletCount, formatMetricCompact(signal.TotalAmountUSD, "$"))
	return title, body
}

func buildPoolBarkMessage(signal *poolSignal) (string, string) {
	label := firstNonEmpty(signal.Label, shortenAddress(signal.Address), "未知池子")
	title := "金狗池子通知"
	body := fmt.Sprintf(
		"%s 命中池子参数监控 | Fees %s | Tx %d | TVL %s | Vol %s | Fee %s | 活跃 %s",
		label,
		formatMetricCompact(signal.TotalFees, "$"),
		signal.TransactionCount,
		formatMetricCompact(signal.CurrentPoolValue, "$"),
		formatMetricCompact(signal.TotalVolume, "$"),
		formatFeeRate(signal.FeeRate),
		formatRatioPercent(signal.ActiveLiquidityRatio),
	)
	return title, body
}

func formatMetricCompact(value float64, prefix string) string {
	if !isFinitePositive(value) {
		return prefix + "0"
	}
	abs := math.Abs(value)
	switch {
	case abs >= 1_000_000_000:
		return fmt.Sprintf("%s%.1fB", prefix, value/1_000_000_000)
	case abs >= 1_000_000:
		return fmt.Sprintf("%s%.1fM", prefix, value/1_000_000)
	case abs >= 1_000:
		return fmt.Sprintf("%s%.1fK", prefix, value/1_000)
	case abs >= 100:
		return fmt.Sprintf("%s%.0f", prefix, value)
	case abs >= 10:
		return fmt.Sprintf("%s%.1f", prefix, value)
	default:
		return fmt.Sprintf("%s%.2f", prefix, value)
	}
}

func formatFeeRate(value int) string {
	if value <= 0 {
		return "--"
	}
	switch value {
	case 100:
		return "0.01%"
	case 500:
		return "0.05%"
	case 2500:
		return "0.25%"
	case 3000:
		return "0.30%"
	case 10000:
		return "1%"
	default:
		return fmt.Sprintf("%.2f%%", float64(value)/10000)
	}
}

func formatRatioPercent(value float64) string {
	if !isFinitePositive(value) {
		return "--"
	}
	return fmt.Sprintf("%.1f%%", value*100)
}

func isFinitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}

func SortedWallets(bucket *pairBucket) []string {
	if bucket == nil || len(bucket.Events) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(bucket.Events))
	for _, event := range bucket.Events {
		if event.Wallet == "" {
			continue
		}
		seen[event.Wallet] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for wallet := range seen {
		out = append(out, wallet)
	}
	sort.Strings(out)
	return out
}
