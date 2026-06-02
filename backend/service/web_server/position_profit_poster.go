package web_server

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/realtime"
	"TgLpBot/service/token_metadata"

	"gorm.io/gorm"
)

type positionProfitPosterRequest struct {
	InitData string `json:"initData"`
	TaskID   uint   `json:"taskId"`
}

type positionProfitPosterPoint struct {
	T     int64   `json:"t"`
	Value float64 `json:"value"`
}

type positionProfitPosterToken struct {
	Address string `json:"address,omitempty"`
	Symbol  string `json:"symbol,omitempty"`
	Name    string `json:"name,omitempty"`
	LogoURL string `json:"logo_url,omitempty"`
}

type positionProfitPosterResponse struct {
	OK bool `json:"ok"`

	TaskID      uint   `json:"task_id"`
	Chain       string `json:"chain"`
	Pair        string `json:"pair"`
	Exchange    string `json:"exchange,omitempty"`
	PoolVersion string `json:"pool_version,omitempty"`
	StatusLabel string `json:"status_label,omitempty"`

	OpenedAt    *time.Time `json:"opened_at,omitempty"`
	GeneratedAt time.Time  `json:"generated_at"`
	WindowLabel string     `json:"window_label"`
	PriceSource string     `json:"price_source"`

	InvestUSD       float64 `json:"invest_usd"`
	CurrentValueUSD float64 `json:"current_value_usd"`
	ProfitUSD       float64 `json:"profit_usd"`
	ProfitPct       float64 `json:"profit_pct"`

	ThemeToken positionProfitPosterToken   `json:"theme_token"`
	Series     []positionProfitPosterPoint `json:"series"`

	Warnings []string `json:"warnings,omitempty"`
}

type positionProfitPosterTradeRow struct {
	OpenedAt      time.Time `gorm:"column:opened_at"`
	OpenUSDTSpent string    `gorm:"column:open_usdt_spent"`
}

type positionProfitPosterTokenCandidate struct {
	address string
	symbol  string
}

var posterBaseLikeSymbols = map[string]struct{}{
	"USDT": {}, "USDC": {}, "BUSD": {}, "DAI": {}, "FDUSD": {}, "USDD": {}, "FRAX": {},
	"WBNB": {}, "BNB": {}, "WETH": {}, "ETH": {}, "WBTC": {}, "BTC": {}, "WSOL": {}, "SOL": {},
}

func (s *Server) handlePositionProfitPoster(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var req positionProfitPosterRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.TaskID == 0 {
		http.Error(w, "missing taskId", http.StatusBadRequest)
		return
	}
	if database.DB == nil {
		http.Error(w, "database not initialized", http.StatusInternalServerError)
		return
	}
	if config.AppConfig == nil {
		http.Error(w, "config not loaded", http.StatusInternalServerError)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if status, msg := requireModulePermission(check, models.AccessModulePositions); status != 0 {
		http.Error(w, msg, status)
		return
	}

	var task models.StrategyTask
	if err := database.DB.Where("id = ? AND user_id = ?", req.TaskID, user.ID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load task", http.StatusInternalServerError)
		return
	}

	if s.Realtime == nil {
		http.Error(w, "realtime service unavailable", http.StatusServiceUnavailable)
		return
	}
	realtimeResp, err := s.Realtime.GetForUser(user.ID)
	if err != nil {
		http.Error(w, "failed to load realtime positions", http.StatusBadGateway)
		return
	}
	position := findRealtimePositionByTaskID(realtimeResp, req.TaskID)
	if position == nil {
		http.Error(w, "task position not found", http.StatusNotFound)
		return
	}

	chain := config.NormalizeChain(position.Chain)
	if chain == "" {
		chain = config.NormalizeChain(task.Chain)
	}
	if chain == "" {
		chain = config.PickEnabledChain("bsc")
	}

	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok || cc.ChainID <= 0 {
		http.Error(w, "invalid chain", http.StatusBadRequest)
		return
	}

	openInfo, err := loadPosterOpenTrade(req.TaskID, user.ID, &task)
	if err != nil {
		http.Error(w, "failed to load trade record", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	openTime := openInfo.OpenedAt
	if openTime.IsZero() {
		openTime = task.CreatedAt
	}
	if openTime.IsZero() || openTime.After(now) {
		openTime = now
	}

	investUSD := resolvePosterInvestUSD(position, openInfo)
	profitUSD := safePosterNumber(position.AbsolutePnLUSD)
	currentValueUSD := safePosterNumber(position.CurrentValueUSD)
	profitPct := 0.0
	if investUSD > 0 {
		profitPct = (profitUSD / investUSD) * 100
	}

	themeCandidate := pickPosterThemeToken(chain, &task, position)
	themeToken := positionProfitPosterToken{
		Address: themeCandidate.address,
		Symbol:  themeCandidate.symbol,
		Name:    themeCandidate.symbol,
	}

	warnings := make([]string, 0, 4)
	if themeCandidate.address != "" {
		if s.TokenMeta == nil {
			s.TokenMeta = token_metadata.NewService()
		}
		metaByAddr, metaErr := s.TokenMeta.GetBatch(r.Context(), chain, []string{themeCandidate.address})
		if metaErr != nil {
			warnings = append(warnings, "未能获取代币展示信息，已使用占位样式")
		} else if meta, ok := metaByAddr[strings.ToLower(strings.TrimSpace(themeCandidate.address))]; ok {
			if strings.TrimSpace(meta.Symbol) != "" {
				themeToken.Symbol = strings.TrimSpace(meta.Symbol)
			}
			if strings.TrimSpace(meta.Name) != "" {
				themeToken.Name = strings.TrimSpace(meta.Name)
			}
			if strings.TrimSpace(meta.LogoURL) != "" {
				themeToken.LogoURL = strings.TrimSpace(meta.LogoURL)
			}
		}
	}

	series, seriesWarnings := buildPosterSeries(chain, themeCandidate.address, openTime, now)
	warnings = append(warnings, seriesWarnings...)

	resp := positionProfitPosterResponse{
		OK:              true,
		TaskID:          req.TaskID,
		Chain:           chain,
		Pair:            firstNonEmpty(strings.TrimSpace(position.Title), buildPairFromTask(&task)),
		Exchange:        firstNonEmpty(strings.TrimSpace(position.Exchange), strings.TrimSpace(task.Exchange)),
		PoolVersion:     firstNonEmpty(strings.TrimSpace(position.Version), strings.TrimSpace(task.PoolVersion)),
		StatusLabel:     strings.TrimSpace(position.StatusLabel),
		OpenedAt:        &openTime,
		GeneratedAt:     now,
		WindowLabel:     "开单以来价格收益",
		PriceSource:     "geckoterminal",
		InvestUSD:       investUSD,
		CurrentValueUSD: currentValueUSD,
		ProfitUSD:       profitUSD,
		ProfitPct:       profitPct,
		ThemeToken:      themeToken,
		Series:          series,
		Warnings:        dedupeWarnings(warnings),
	}
	writeJSON(w, http.StatusOK, resp)
}

func findRealtimePositionByTaskID(resp *realtime.RealtimePositionsResponse, taskID uint) *realtime.RealtimePosition {
	if resp == nil || taskID == 0 {
		return nil
	}
	for i := range resp.Positions {
		if resp.Positions[i].TaskID == taskID {
			return &resp.Positions[i]
		}
	}
	return nil
}

func loadPosterOpenTrade(taskID, userID uint, task *models.StrategyTask) (*positionProfitPosterTradeRow, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	var row positionProfitPosterTradeRow
	err := database.DB.
		Table("trade_records").
		Select("opened_at, open_usdt_spent").
		Where("task_id = ? AND user_id = ? AND status = ?", taskID, userID, models.TradeStatusOpen).
		Order("opened_at DESC").
		Take(&row).Error
	if err == nil {
		return &row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if task == nil {
		return &positionProfitPosterTradeRow{}, nil
	}
	return &positionProfitPosterTradeRow{OpenedAt: task.CreatedAt}, nil
}

func resolvePosterInvestUSD(position *realtime.RealtimePosition, openInfo *positionProfitPosterTradeRow) float64 {
	if position != nil {
		if n := safePosterNumber(position.NetInvestedUSD); n > 0 {
			return n
		}
		if n := safePosterNumber(position.InitialCostUSD); n > 0 {
			return n
		}
		if n := safePosterNumber(position.TaskAmountUSDT); n > 0 {
			return n
		}
	}
	if openInfo != nil {
		if n, ok := weiStrToFloat18Poster(openInfo.OpenUSDTSpent); ok && n > 0 {
			return n
		}
	}
	return 0
}

func safePosterNumber(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func buildPairFromTask(task *models.StrategyTask) string {
	if task == nil {
		return ""
	}
	left := strings.TrimSpace(task.Token0Symbol)
	right := strings.TrimSpace(task.Token1Symbol)
	if left == "" && right == "" {
		return ""
	}
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return left + "/" + right
}

func pickPosterThemeToken(chain string, task *models.StrategyTask, position *realtime.RealtimePosition) positionProfitPosterTokenCandidate {
	candidates := make([]positionProfitPosterTokenCandidate, 0, 2)
	if position != nil {
		for _, row := range position.TokenRows {
			addr := strings.ToLower(strings.TrimSpace(row.Address))
			symbol := strings.TrimSpace(row.Symbol)
			if addr == "" && symbol == "" {
				continue
			}
			candidates = append(candidates, positionProfitPosterTokenCandidate{address: addr, symbol: symbol})
		}
	}
	if len(candidates) == 0 && task != nil {
		if task.Token0Address != "" || task.Token0Symbol != "" {
			candidates = append(candidates, positionProfitPosterTokenCandidate{
				address: strings.ToLower(strings.TrimSpace(task.Token0Address)),
				symbol:  strings.TrimSpace(task.Token0Symbol),
			})
		}
		if task.Token1Address != "" || task.Token1Symbol != "" {
			candidates = append(candidates, positionProfitPosterTokenCandidate{
				address: strings.ToLower(strings.TrimSpace(task.Token1Address)),
				symbol:  strings.TrimSpace(task.Token1Symbol),
			})
		}
	}
	if len(candidates) == 0 {
		return positionProfitPosterTokenCandidate{}
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	firstBaseLike := isPosterBaseLikeToken(chain, candidates[0].symbol, candidates[0].address)
	secondBaseLike := isPosterBaseLikeToken(chain, candidates[1].symbol, candidates[1].address)
	if firstBaseLike && !secondBaseLike {
		return candidates[1]
	}
	if secondBaseLike && !firstBaseLike {
		return candidates[0]
	}
	if candidates[0].address != "" || candidates[0].symbol != "" {
		return candidates[0]
	}
	return candidates[1]
}

func isPosterBaseLikeToken(chain, symbol, address string) bool {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol != "" {
		if _, ok := posterBaseLikeSymbols[symbol]; ok {
			return true
		}
	}
	if config.AppConfig == nil {
		return false
	}
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok {
		return false
	}
	addr := strings.ToLower(strings.TrimSpace(address))
	return addr != "" && (addr == strings.ToLower(strings.TrimSpace(cc.USDTAddress)) ||
		addr == strings.ToLower(strings.TrimSpace(cc.USDCAddress)) ||
		addr == strings.ToLower(strings.TrimSpace(cc.BUSDAddress)) ||
		addr == strings.ToLower(strings.TrimSpace(cc.WrappedNativeAddress)))
}

func buildPosterSeries(chain string, tokenAddress string, openedAt, now time.Time) ([]positionProfitPosterPoint, []string) {
	tokenAddress = strings.ToLower(strings.TrimSpace(tokenAddress))
	if tokenAddress == "" {
		return nil, []string{"当前仓位缺少可用代币地址，无法生成走势曲线"}
	}
	if openedAt.IsZero() || !openedAt.Before(now) {
		return nil, []string{"开单时间异常，无法生成走势曲线"}
	}

	bar, barStep, limit := choosePosterBar(openedAt, now)
	_, barParams, ok := normalizeGeckoBar(bar)
	if !ok {
		return nil, []string{"走势时间粒度不受支持，已降级为纯摘要海报"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	poolAddress, err := fetchGeckoBestPoolAddress(ctx, nil, chain, tokenAddress)
	if err != nil {
		return nil, []string{"未能获取 GeckoTerminal 走势池子，已降级为纯摘要海报"}
	}
	candles, err := fetchGeckoPoolCandles(ctx, nil, chain, poolAddress, "", barParams, limit, "")
	if err != nil {
		return nil, []string{"未能获取 GeckoTerminal 走势数据，已降级为纯摘要海报"}
	}
	if len(candles) == 0 {
		return nil, []string{"暂无 GeckoTerminal 走势数据，已降级为纯摘要海报"}
	}

	rows := make([]tokenCandle, 0, len(candles))
	startSec := openedAt.Add(-barStep).Unix()
	for _, row := range candles {
		if row.T <= 0 || row.C <= 0 || math.IsNaN(row.C) || math.IsInf(row.C, 0) {
			continue
		}
		if row.T < startSec {
			continue
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		rows = append(rows, candles...)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].T < rows[j].T
	})

	baseline := 0.0
	for _, row := range rows {
		if row.C > 0 {
			baseline = row.C
			break
		}
	}
	if baseline <= 0 {
		return nil, []string{"走势基准价格无效，已降级为纯摘要海报"}
	}

	points := make([]positionProfitPosterPoint, 0, len(rows)+1)
	points = append(points, positionProfitPosterPoint{T: openedAt.Unix(), Value: 0})
	lastT := openedAt.Unix()
	for _, row := range rows {
		ts := row.T
		if ts <= 0 || ts < lastT {
			continue
		}
		points = append(points, positionProfitPosterPoint{
			T:     ts,
			Value: ((row.C / baseline) - 1) * 100,
		})
		lastT = ts
	}
	if len(points) <= 1 {
		return nil, []string{"走势数据点不足，已降级为纯摘要海报"}
	}
	return points, nil
}

func choosePosterBar(openedAt, now time.Time) (string, time.Duration, int) {
	duration := now.Sub(openedAt)
	switch {
	case duration <= 4*time.Hour:
		return "1m", time.Minute, clampPosterLimit(int(math.Ceil(duration.Minutes())) + 8)
	case duration <= 24*time.Hour:
		return "5m", 5 * time.Minute, clampPosterLimit(int(math.Ceil(duration.Minutes()/5)) + 8)
	case duration <= 14*24*time.Hour:
		return "1h", time.Hour, clampPosterLimit(int(math.Ceil(duration.Hours())) + 8)
	case duration <= 240*24*time.Hour:
		return "1d", 24 * time.Hour, clampPosterLimit(int(math.Ceil(duration.Hours()/24)) + 4)
	default:
		return "1w", 7 * 24 * time.Hour, clampPosterLimit(int(math.Ceil(duration.Hours()/(24*7))) + 4)
	}
}

func clampPosterLimit(limit int) int {
	if limit < 16 {
		return 16
	}
	if limit > 299 {
		return 299
	}
	return limit
}

func weiStrToFloat18Poster(weiStr string) (float64, bool) {
	weiStr = strings.TrimSpace(weiStr)
	if weiStr == "" {
		weiStr = "0"
	}
	v, ok := new(big.Int).SetString(weiStr, 10)
	if !ok {
		return 0, false
	}
	r := new(big.Rat).SetInt(v)
	r.Quo(r, new(big.Rat).SetInt(big.NewInt(1e18)))
	f, _ := r.Float64()
	return f, true
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

func dedupeWarnings(rows []string) []string {
	if len(rows) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(rows))
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		row = strings.TrimSpace(row)
		if row == "" {
			continue
		}
		if _, ok := seen[row]; ok {
			continue
		}
		seen[row] = struct{}{}
		out = append(out, row)
	}
	return out
}
