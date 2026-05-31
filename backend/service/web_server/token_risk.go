package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/exchange"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	tokenRiskSourceOKX             = "okx_market_token_advanced_info"
	tokenRiskCacheTTL              = 5 * time.Minute
	tokenRiskErrorCacheTTL         = 5 * time.Minute
	tokenRiskLookupTimeout         = 5 * time.Second
	tokenRiskBackgroundTimeout     = 15 * time.Second
	tokenRiskFreshTTL              = 2 * time.Hour
	tokenRiskRiskyFreshTTL         = 30 * time.Minute
	tokenRiskErrorFreshTTL         = 15 * time.Minute
	tokenRiskRateLimitedFreshTTL   = 30 * time.Minute
	tokenRiskExternalMinInterval   = 1200 * time.Millisecond
	tokenRiskRefreshQueueSize      = 512
	tokenRiskPendingRefreshMessage = "OKX 风控信息待后台刷新"
)

type TokenRiskInfo struct {
	Chain                    string    `json:"chain"`
	ChainIndex               string    `json:"chain_index"`
	TokenAddress             string    `json:"token_address"`
	TokenSymbol              string    `json:"token_symbol,omitempty"`
	TokenName                string    `json:"token_name,omitempty"`
	RiskControlLevel         int       `json:"risk_control_level"`
	RiskControlLabel         string    `json:"risk_control_label"`
	RiskTone                 string    `json:"risk_tone"`
	TokenTags                []string  `json:"token_tags,omitempty"`
	HasHoneypot              bool      `json:"has_honeypot"`
	HasLowLiquidity          bool      `json:"has_low_liquidity"`
	Warnings                 []string  `json:"warnings,omitempty"`
	Top10HoldPercent         string    `json:"top10_hold_percent,omitempty"`
	DevHoldingPercent        string    `json:"dev_holding_percent,omitempty"`
	BundleHoldingPercent     string    `json:"bundle_holding_percent,omitempty"`
	SuspiciousHoldingPercent string    `json:"suspicious_holding_percent,omitempty"`
	SniperHoldingPercent     string    `json:"sniper_holding_percent,omitempty"`
	DevRugPullTokenCount     string    `json:"dev_rug_pull_token_count,omitempty"`
	DevCreateTokenCount      string    `json:"dev_create_token_count,omitempty"`
	DevLaunchedTokenCount    string    `json:"dev_launched_token_count,omitempty"`
	Error                    string    `json:"error,omitempty"`
	Source                   string    `json:"source"`
	UpdatedAt                time.Time `json:"updated_at"`
	NextRefreshAt            time.Time `json:"-"`
}

type tokenRiskTarget struct {
	Chain   string
	Address string
	Symbol  string
	Name    string
}

type tokenRiskCacheEntry struct {
	Risk      TokenRiskInfo
	ExpiresAt time.Time
}

var (
	tokenRiskCache         sync.Map
	tokenRiskRefreshOnce   sync.Once
	tokenRiskRefreshQueue  = make(chan tokenRiskTarget, tokenRiskRefreshQueueSize)
	tokenRiskRefreshQueued sync.Map
	tokenRiskOKXMu         sync.Mutex
	tokenRiskNextOKXAt     time.Time
)

func normalizeTokenRiskChain(chain string) string {
	return strings.ToLower(strings.TrimSpace(chain))
}

func tokenRiskCacheKey(chain string, address string) string {
	chain = normalizeTokenRiskChain(chain)
	address = normalizeCatalogHex(address)
	if chain == "" || address == "" {
		return ""
	}
	return chain + ":" + address
}

func readTokenRiskCache(chain string, address string) (TokenRiskInfo, bool) {
	key := tokenRiskCacheKey(chain, address)
	if key == "" {
		return TokenRiskInfo{}, false
	}
	raw, ok := tokenRiskCache.Load(key)
	if !ok {
		return TokenRiskInfo{}, false
	}
	entry, ok := raw.(tokenRiskCacheEntry)
	if !ok || time.Now().After(entry.ExpiresAt) {
		tokenRiskCache.Delete(key)
		return TokenRiskInfo{}, false
	}
	return entry.Risk, true
}

func writeTokenRiskCache(risk TokenRiskInfo) {
	key := tokenRiskCacheKey(risk.Chain, risk.TokenAddress)
	if key == "" {
		return
	}
	ttl := tokenRiskCacheTTL
	if strings.TrimSpace(risk.Error) != "" {
		ttl = tokenRiskErrorCacheTTL
	}
	tokenRiskCache.Store(key, tokenRiskCacheEntry{
		Risk:      risk,
		ExpiresAt: time.Now().Add(ttl),
	})
}

func tokenRiskRefreshTTL(risk TokenRiskInfo) time.Duration {
	errText := strings.ToLower(strings.TrimSpace(risk.Error))
	switch {
	case errText != "" && (strings.Contains(errText, "429") || strings.Contains(errText, "too many") || strings.Contains(errText, "rate limit")):
		return tokenRiskRateLimitedFreshTTL
	case errText != "":
		return tokenRiskErrorFreshTTL
	case risk.HasHoneypot || risk.HasLowLiquidity || risk.RiskControlLevel >= 3:
		return tokenRiskRiskyFreshTTL
	default:
		return tokenRiskFreshTTL
	}
}

func tokenRiskIsFresh(risk TokenRiskInfo) bool {
	if risk.NextRefreshAt.IsZero() {
		return false
	}
	return time.Now().Before(risk.NextRefreshAt)
}

func withTokenRiskTargetMetadata(risk TokenRiskInfo, target tokenRiskTarget) TokenRiskInfo {
	if strings.TrimSpace(risk.TokenSymbol) == "" {
		risk.TokenSymbol = strings.TrimSpace(target.Symbol)
	}
	if strings.TrimSpace(risk.TokenName) == "" {
		risk.TokenName = strings.TrimSpace(target.Name)
	}
	return risk
}

func tokenRiskJSONString(values []string) string {
	if values == nil {
		values = []string{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		log.Printf("[TokenRisk] marshal json list failed: %v", err)
		return "[]"
	}
	return string(raw)
}

func tokenRiskStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		log.Printf("[TokenRisk] parse json list failed: %v", err)
		return nil
	}
	result := make([]string, 0, len(out))
	seen := make(map[string]struct{}, len(out))
	for _, item := range out {
		clean := strings.TrimSpace(item)
		if clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, clean)
	}
	return result
}

func tokenRiskExcludedSymbol(symbol string) bool {
	switch strings.ToLower(strings.TrimSpace(symbol)) {
	case "bnb", "wbnb", "usdt", "usdc", "busd", "dai", "fdusd", "usdd", "frax", "tusd", "usdp", "weth", "eth":
		return true
	default:
		return false
	}
}

func tokenRiskExcludedAddress(chain string, address string) bool {
	addr := normalizeCatalogHex(address)
	if addr == "" {
		return false
	}
	if config.AppConfig == nil {
		return false
	}
	cc, ok := config.AppConfig.GetChainConfig(normalizeTokenRiskChain(chain))
	if !ok {
		return false
	}
	candidates := []string{
		cc.StableAddress,
		cc.USDTAddress,
		cc.USDCAddress,
		cc.BUSDAddress,
		cc.WrappedNativeAddress,
	}
	for _, candidate := range candidates {
		if normalizeCatalogHex(candidate) == addr {
			return true
		}
	}
	return false
}

func tokenRiskNeedsLookup(target tokenRiskTarget) bool {
	if normalizeCatalogHex(target.Address) == "" {
		return false
	}
	if tokenRiskExcludedSymbol(target.Symbol) {
		return false
	}
	if tokenRiskExcludedAddress(target.Chain, target.Address) {
		return false
	}
	return true
}

func tokenRiskCandidates(chain string, token0Addr string, token1Addr string, token0Symbol string, token1Symbol string, token0Name string, token1Name string) []tokenRiskTarget {
	raw := []tokenRiskTarget{
		{
			Chain:   chain,
			Address: token0Addr,
			Symbol:  token0Symbol,
			Name:    token0Name,
		},
		{
			Chain:   chain,
			Address: token1Addr,
			Symbol:  token1Symbol,
			Name:    token1Name,
		},
	}
	out := make([]tokenRiskTarget, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, target := range raw {
		target.Chain = normalizeTokenRiskChain(target.Chain)
		target.Address = normalizeCatalogHex(target.Address)
		target.Symbol = strings.TrimSpace(target.Symbol)
		target.Name = strings.TrimSpace(target.Name)
		if !tokenRiskNeedsLookup(target) {
			continue
		}
		key := tokenRiskCacheKey(target.Chain, target.Address)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, target)
	}
	return out
}

func tokenRiskTargetsForPoolItem(item HotPoolResponse, chain string) []tokenRiskTarget {
	itemChain := strings.TrimSpace(firstNonEmpty(item.Chain, chain))
	targets := tokenRiskCandidates(
		itemChain,
		item.Token0Address,
		item.Token1Address,
		item.Token0Symbol,
		item.Token1Symbol,
		item.Token0Name,
		item.Token1Name,
	)
	if len(targets) <= 1 {
		return targets
	}

	displayAddr := normalizeCatalogHex(item.DisplayTokenAddress)
	if displayAddr == "" {
		displayAddr = normalizeCatalogHex(item.PricedTokenAddress)
	}
	if displayAddr != "" {
		for i, target := range targets {
			if normalizeCatalogHex(target.Address) == displayAddr {
				return append([]tokenRiskTarget{target}, append(targets[:i], targets[i+1:]...)...)
			}
		}
	}
	return targets
}

func tokenRiskTargetsForTask(task *models.StrategyTask) []tokenRiskTarget {
	if task == nil {
		return nil
	}
	return tokenRiskCandidates(
		task.Chain,
		task.Token0Address,
		task.Token1Address,
		task.Token0Symbol,
		task.Token1Symbol,
		"",
		"",
	)
}

func tokenRiskLevelLabel(level int) string {
	switch level {
	case 0:
		return "Undefined"
	case 1:
		return "Low"
	case 2:
		return "Medium"
	case 3:
		return "Medium-high"
	case 4:
		return "High"
	case 5:
		return "High(manual)"
	default:
		return fmt.Sprintf("Unknown(%d)", level)
	}
}

func tokenRiskTone(level int, hasHoneypot bool, hasLowLiquidity bool, hasError bool) string {
	switch {
	case hasError:
		return "unknown"
	case hasHoneypot || level >= 5:
		return "critical"
	case level >= 4 || hasLowLiquidity:
		return "high"
	case level >= 2:
		return "medium"
	case level == 1:
		return "low"
	default:
		return "neutral"
	}
}

func normalizeTokenRiskTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		clean := strings.TrimSpace(tag)
		if clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func tokenRiskHasTag(tags []string, tag string) bool {
	expected := strings.ToLower(strings.TrimSpace(tag))
	for _, item := range tags {
		if strings.EqualFold(strings.TrimSpace(item), expected) {
			return true
		}
	}
	return false
}

func tokenRiskWarningList(level int, hasHoneypot bool, hasLowLiquidity bool, queryErr error) []string {
	warnings := make([]string, 0, 3)
	if queryErr != nil {
		errText := strings.TrimSpace(queryErr.Error())
		lowerErr := strings.ToLower(errText)
		if strings.Contains(lowerErr, "429") || strings.Contains(lowerErr, "too many") {
			warnings = append(warnings, "OKX 风控接口限流，已延后后台刷新")
			return warnings
		}
		warnings = append(warnings, "OKX 风控查询失败: "+errText)
		return warnings
	}
	if hasHoneypot {
		warnings = append(warnings, "OKX 标记为貔貅盘")
	}
	if hasLowLiquidity {
		warnings = append(warnings, "OKX 标记为低流动性")
	}
	if level >= 3 {
		warnings = append(warnings, "OKX 风险等级: "+tokenRiskLevelLabel(level))
	}
	return warnings
}

func buildTokenRiskInfo(target tokenRiskTarget, chainIndex string, advanced *exchange.MarketTokenAdvancedInfo, queryErr error) TokenRiskInfo {
	now := time.Now()
	risk := TokenRiskInfo{
		Chain:            normalizeTokenRiskChain(target.Chain),
		ChainIndex:       strings.TrimSpace(chainIndex),
		TokenAddress:     normalizeCatalogHex(target.Address),
		TokenSymbol:      strings.TrimSpace(target.Symbol),
		TokenName:        strings.TrimSpace(target.Name),
		RiskControlLevel: 0,
		RiskControlLabel: tokenRiskLevelLabel(0),
		RiskTone:         "unknown",
		Source:           tokenRiskSourceOKX,
		UpdatedAt:        now,
	}
	risk.NextRefreshAt = now.Add(tokenRiskRefreshTTL(risk))

	if queryErr != nil {
		risk.Error = strings.TrimSpace(queryErr.Error())
		risk.Warnings = tokenRiskWarningList(0, false, false, queryErr)
		risk.NextRefreshAt = now.Add(tokenRiskRefreshTTL(risk))
		return risk
	}
	if advanced == nil {
		err := fmt.Errorf("OKX advanced-info returned empty data")
		risk.Error = err.Error()
		risk.Warnings = tokenRiskWarningList(0, false, false, err)
		risk.NextRefreshAt = now.Add(tokenRiskRefreshTTL(risk))
		return risk
	}

	if addr := normalizeCatalogHex(advanced.TokenContractAddress); addr != "" {
		risk.TokenAddress = addr
	}
	if chain := strings.TrimSpace(advanced.ChainIndex); chain != "" {
		risk.ChainIndex = chain
	}

	level := 0
	if raw := strings.TrimSpace(advanced.RiskControlLevel); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			level = parsed
		}
	}
	tags := normalizeTokenRiskTags(advanced.TokenTags)
	hasHoneypot := tokenRiskHasTag(tags, "honeypot")
	hasLowLiquidity := tokenRiskHasTag(tags, "lowLiquidity")

	risk.RiskControlLevel = level
	risk.RiskControlLabel = tokenRiskLevelLabel(level)
	risk.TokenTags = tags
	risk.HasHoneypot = hasHoneypot
	risk.HasLowLiquidity = hasLowLiquidity
	risk.RiskTone = tokenRiskTone(level, hasHoneypot, hasLowLiquidity, false)
	risk.Warnings = tokenRiskWarningList(level, hasHoneypot, hasLowLiquidity, nil)
	risk.Top10HoldPercent = strings.TrimSpace(advanced.Top10HoldPercent)
	risk.DevHoldingPercent = strings.TrimSpace(advanced.DevHoldingPercent)
	risk.BundleHoldingPercent = strings.TrimSpace(advanced.BundleHoldingPercent)
	risk.SuspiciousHoldingPercent = strings.TrimSpace(advanced.SuspiciousHoldingPercent)
	risk.SniperHoldingPercent = strings.TrimSpace(advanced.SniperHoldingPercent)
	risk.DevRugPullTokenCount = strings.TrimSpace(advanced.DevRugPullTokenCount)
	risk.DevCreateTokenCount = strings.TrimSpace(advanced.DevCreateTokenCount)
	risk.DevLaunchedTokenCount = strings.TrimSpace(advanced.DevLaunchedTokenCount)
	risk.NextRefreshAt = now.Add(tokenRiskRefreshTTL(risk))
	return risk
}

func tokenRiskInfoToSnapshot(risk TokenRiskInfo) models.TokenRiskSnapshot {
	fetchedAt := risk.UpdatedAt
	if fetchedAt.IsZero() {
		fetchedAt = time.Now()
	}
	nextRefreshAt := risk.NextRefreshAt
	if nextRefreshAt.IsZero() {
		nextRefreshAt = fetchedAt.Add(tokenRiskRefreshTTL(risk))
	}
	return models.TokenRiskSnapshot{
		Chain:                    normalizeTokenRiskChain(risk.Chain),
		ChainIndex:               strings.TrimSpace(risk.ChainIndex),
		TokenAddress:             normalizeCatalogHex(risk.TokenAddress),
		TokenSymbol:              strings.TrimSpace(risk.TokenSymbol),
		TokenName:                strings.TrimSpace(risk.TokenName),
		RiskControlLevel:         risk.RiskControlLevel,
		RiskControlLabel:         strings.TrimSpace(risk.RiskControlLabel),
		RiskTone:                 strings.TrimSpace(risk.RiskTone),
		TokenTagsJSON:            tokenRiskJSONString(risk.TokenTags),
		WarningsJSON:             tokenRiskJSONString(risk.Warnings),
		HasHoneypot:              risk.HasHoneypot,
		HasLowLiquidity:          risk.HasLowLiquidity,
		Top10HoldPercent:         strings.TrimSpace(risk.Top10HoldPercent),
		DevHoldingPercent:        strings.TrimSpace(risk.DevHoldingPercent),
		BundleHoldingPercent:     strings.TrimSpace(risk.BundleHoldingPercent),
		SuspiciousHoldingPercent: strings.TrimSpace(risk.SuspiciousHoldingPercent),
		SniperHoldingPercent:     strings.TrimSpace(risk.SniperHoldingPercent),
		DevRugPullTokenCount:     strings.TrimSpace(risk.DevRugPullTokenCount),
		DevCreateTokenCount:      strings.TrimSpace(risk.DevCreateTokenCount),
		DevLaunchedTokenCount:    strings.TrimSpace(risk.DevLaunchedTokenCount),
		ErrorMessage:             strings.TrimSpace(risk.Error),
		Source:                   strings.TrimSpace(risk.Source),
		FetchedAt:                fetchedAt,
		NextRefreshAt:            nextRefreshAt,
	}
}

func tokenRiskSnapshotToInfo(snapshot models.TokenRiskSnapshot) TokenRiskInfo {
	risk := TokenRiskInfo{
		Chain:                    normalizeTokenRiskChain(snapshot.Chain),
		ChainIndex:               strings.TrimSpace(snapshot.ChainIndex),
		TokenAddress:             normalizeCatalogHex(snapshot.TokenAddress),
		TokenSymbol:              strings.TrimSpace(snapshot.TokenSymbol),
		TokenName:                strings.TrimSpace(snapshot.TokenName),
		RiskControlLevel:         snapshot.RiskControlLevel,
		RiskControlLabel:         strings.TrimSpace(snapshot.RiskControlLabel),
		RiskTone:                 strings.TrimSpace(snapshot.RiskTone),
		TokenTags:                tokenRiskStringSlice(snapshot.TokenTagsJSON),
		HasHoneypot:              snapshot.HasHoneypot,
		HasLowLiquidity:          snapshot.HasLowLiquidity,
		Warnings:                 tokenRiskStringSlice(snapshot.WarningsJSON),
		Top10HoldPercent:         strings.TrimSpace(snapshot.Top10HoldPercent),
		DevHoldingPercent:        strings.TrimSpace(snapshot.DevHoldingPercent),
		BundleHoldingPercent:     strings.TrimSpace(snapshot.BundleHoldingPercent),
		SuspiciousHoldingPercent: strings.TrimSpace(snapshot.SuspiciousHoldingPercent),
		SniperHoldingPercent:     strings.TrimSpace(snapshot.SniperHoldingPercent),
		DevRugPullTokenCount:     strings.TrimSpace(snapshot.DevRugPullTokenCount),
		DevCreateTokenCount:      strings.TrimSpace(snapshot.DevCreateTokenCount),
		DevLaunchedTokenCount:    strings.TrimSpace(snapshot.DevLaunchedTokenCount),
		Error:                    strings.TrimSpace(snapshot.ErrorMessage),
		Source:                   strings.TrimSpace(snapshot.Source),
		UpdatedAt:                snapshot.FetchedAt,
		NextRefreshAt:            snapshot.NextRefreshAt,
	}
	if risk.RiskControlLabel == "" {
		risk.RiskControlLabel = tokenRiskLevelLabel(risk.RiskControlLevel)
	}
	if risk.RiskTone == "" {
		risk.RiskTone = tokenRiskTone(risk.RiskControlLevel, risk.HasHoneypot, risk.HasLowLiquidity, strings.TrimSpace(risk.Error) != "")
	}
	if risk.Source == "" {
		risk.Source = tokenRiskSourceOKX
	}
	return risk
}

func loadTokenRiskSnapshot(ctx context.Context, target tokenRiskTarget) (TokenRiskInfo, bool) {
	key := tokenRiskCacheKey(target.Chain, target.Address)
	if key == "" || database.DB == nil {
		return TokenRiskInfo{}, false
	}
	var snapshot models.TokenRiskSnapshot
	err := database.DB.WithContext(ctx).
		Where("chain = ? AND token_address = ?", normalizeTokenRiskChain(target.Chain), normalizeCatalogHex(target.Address)).
		First(&snapshot).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TokenRiskInfo{}, false
	}
	if err != nil {
		log.Printf("[TokenRisk] load snapshot failed key=%s err=%v", key, err)
		return TokenRiskInfo{}, false
	}
	return withTokenRiskTargetMetadata(tokenRiskSnapshotToInfo(snapshot), target), true
}

func loadTokenRiskSnapshots(ctx context.Context, targets map[string]tokenRiskTarget) map[string]TokenRiskInfo {
	out := make(map[string]TokenRiskInfo)
	if len(targets) == 0 || database.DB == nil {
		return out
	}

	byChain := make(map[string][]string)
	for _, target := range targets {
		chain := normalizeTokenRiskChain(target.Chain)
		address := normalizeCatalogHex(target.Address)
		if chain == "" || address == "" {
			continue
		}
		byChain[chain] = append(byChain[chain], address)
	}
	for chain, addresses := range byChain {
		var rows []models.TokenRiskSnapshot
		if err := database.DB.WithContext(ctx).
			Where("chain = ? AND token_address IN ?", chain, addresses).
			Find(&rows).Error; err != nil {
			log.Printf("[TokenRisk] load snapshots failed chain=%s err=%v", chain, err)
			continue
		}
		for _, row := range rows {
			key := tokenRiskCacheKey(row.Chain, row.TokenAddress)
			target := targets[key]
			out[key] = withTokenRiskTargetMetadata(tokenRiskSnapshotToInfo(row), target)
		}
	}
	return out
}

func saveTokenRiskSnapshot(ctx context.Context, risk TokenRiskInfo) {
	if database.DB == nil {
		return
	}
	row := tokenRiskInfoToSnapshot(risk)
	if row.Chain == "" || row.TokenAddress == "" {
		return
	}
	updates := []string{
		"chain_index",
		"token_symbol",
		"token_name",
		"risk_control_level",
		"risk_control_label",
		"risk_tone",
		"token_tags_json",
		"warnings_json",
		"has_honeypot",
		"has_low_liquidity",
		"top10_hold_percent",
		"dev_holding_percent",
		"bundle_holding_percent",
		"suspicious_holding_percent",
		"sniper_holding_percent",
		"dev_rug_pull_token_count",
		"dev_create_token_count",
		"dev_launched_token_count",
		"error_message",
		"last_error_message",
		"last_failed_at",
		"source",
		"fetched_at",
		"next_refresh_at",
	}
	if err := database.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chain"}, {Name: "token_address"}},
		DoUpdates: clause.AssignmentColumns(updates),
	}).Create(&row).Error; err != nil {
		log.Printf("[TokenRisk] save snapshot failed token=%s chain=%s err=%v", row.TokenAddress, row.Chain, err)
	}
}

func markTokenRiskRefreshFailure(ctx context.Context, existing *TokenRiskInfo, failed TokenRiskInfo) {
	if database.DB == nil || existing == nil {
		return
	}
	key := tokenRiskCacheKey(existing.Chain, existing.TokenAddress)
	if key == "" {
		return
	}
	now := time.Now()
	nextRefreshAt := now.Add(tokenRiskRefreshTTL(failed))
	if err := database.DB.WithContext(ctx).
		Model(&models.TokenRiskSnapshot{}).
		Where("chain = ? AND token_address = ?", normalizeTokenRiskChain(existing.Chain), normalizeCatalogHex(existing.TokenAddress)).
		Updates(map[string]interface{}{
			"last_error_message": strings.TrimSpace(failed.Error),
			"last_failed_at":     now,
			"next_refresh_at":    nextRefreshAt,
		}).Error; err != nil {
		log.Printf("[TokenRisk] mark refresh failure failed key=%s err=%v", key, err)
		return
	}
	existing.NextRefreshAt = nextRefreshAt
	writeTokenRiskCache(*existing)
}

func waitTokenRiskOKXSlot(ctx context.Context) error {
	tokenRiskOKXMu.Lock()
	now := time.Now()
	if tokenRiskNextOKXAt.Before(now) {
		tokenRiskNextOKXAt = now
	}
	scheduledAt := tokenRiskNextOKXAt
	tokenRiskNextOKXAt = tokenRiskNextOKXAt.Add(tokenRiskExternalMinInterval)
	tokenRiskOKXMu.Unlock()

	delay := time.Until(scheduledAt)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func fetchTokenRiskFromOKX(ctx context.Context, target tokenRiskTarget) TokenRiskInfo {
	chainIndex := config.ChainToOKXChainIndex(target.Chain)
	if chainIndex == "" {
		return buildTokenRiskInfo(target, "", nil, fmt.Errorf("unsupported chain for OKX token risk: %s", target.Chain))
	}
	if err := waitTokenRiskOKXSlot(ctx); err != nil {
		return buildTokenRiskInfo(target, chainIndex, nil, err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, tokenRiskLookupTimeout)
	defer cancel()

	resp, err := exchange.NewOKXDexService().GetMarketTokenAdvancedInfo(reqCtx, exchange.MarketTokenAdvancedInfoRequest{
		ChainIndex:           chainIndex,
		TokenContractAddress: target.Address,
	})
	if err != nil {
		return buildTokenRiskInfo(target, chainIndex, nil, err)
	}
	if resp == nil || len(resp.Data) == 0 {
		return buildTokenRiskInfo(target, chainIndex, nil, nil)
	}
	return buildTokenRiskInfo(target, chainIndex, &resp.Data[0], nil)
}

func refreshTokenRiskSnapshot(ctx context.Context, target tokenRiskTarget, existing *TokenRiskInfo) TokenRiskInfo {
	risk := fetchTokenRiskFromOKX(ctx, target)
	if strings.TrimSpace(risk.Error) != "" {
		if existing != nil {
			markTokenRiskRefreshFailure(ctx, existing, risk)
			stale := withTokenRiskTargetMetadata(*existing, target)
			stale.Error = strings.TrimSpace(risk.Error)
			stale.Warnings = append(stale.Warnings, "OKX 风控刷新失败，当前展示上次快照")
			return stale
		}
		saveTokenRiskSnapshot(ctx, risk)
		writeTokenRiskCache(risk)
		return risk
	}
	saveTokenRiskSnapshot(ctx, risk)
	writeTokenRiskCache(risk)
	return risk
}

func ensureTokenRiskRefreshWorker() {
	tokenRiskRefreshOnce.Do(func() {
		go func() {
			for target := range tokenRiskRefreshQueue {
				key := tokenRiskCacheKey(target.Chain, target.Address)
				func() {
					defer tokenRiskRefreshQueued.Delete(key)
					ctx, cancel := context.WithTimeout(context.Background(), tokenRiskBackgroundTimeout)
					defer cancel()

					existing, ok := loadTokenRiskSnapshot(ctx, target)
					if ok && tokenRiskIsFresh(existing) {
						writeTokenRiskCache(existing)
						return
					}
					if ok {
						refreshTokenRiskSnapshot(ctx, target, &existing)
						return
					}
					refreshTokenRiskSnapshot(ctx, target, nil)
				}()
			}
		}()
	})
}

func enqueueTokenRiskRefresh(target tokenRiskTarget) {
	if !tokenRiskNeedsLookup(target) {
		return
	}
	key := tokenRiskCacheKey(target.Chain, target.Address)
	if key == "" {
		return
	}
	ensureTokenRiskRefreshWorker()
	if _, loaded := tokenRiskRefreshQueued.LoadOrStore(key, time.Now()); loaded {
		return
	}
	select {
	case tokenRiskRefreshQueue <- target:
	default:
		tokenRiskRefreshQueued.Delete(key)
		log.Printf("[TokenRisk] refresh queue full, skip key=%s", key)
	}
}

func buildPendingTokenRiskInfo(target tokenRiskTarget) TokenRiskInfo {
	err := errors.New(tokenRiskPendingRefreshMessage)
	risk := buildTokenRiskInfo(target, config.ChainToOKXChainIndex(target.Chain), nil, err)
	risk.Error = tokenRiskPendingRefreshMessage
	risk.Warnings = []string{tokenRiskPendingRefreshMessage}
	return risk
}

func (s *Server) lookupTokenRisk(ctx context.Context, target tokenRiskTarget) TokenRiskInfo {
	if cached, ok := readTokenRiskCache(target.Chain, target.Address); ok {
		cached = withTokenRiskTargetMetadata(cached, target)
		if tokenRiskIsFresh(cached) {
			return cached
		}
		return refreshTokenRiskSnapshot(ctx, target, &cached)
	}

	existing, ok := loadTokenRiskSnapshot(ctx, target)
	if ok && tokenRiskIsFresh(existing) {
		writeTokenRiskCache(existing)
		return existing
	}
	if ok {
		return refreshTokenRiskSnapshot(ctx, target, &existing)
	}
	return refreshTokenRiskSnapshot(ctx, target, nil)
}

func tokenRiskSeverity(risk TokenRiskInfo) int {
	switch risk.RiskTone {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "unknown":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func pickWorstTokenRisk(risks []TokenRiskInfo) *TokenRiskInfo {
	if len(risks) == 0 {
		return nil
	}
	best := risks[0]
	for _, risk := range risks[1:] {
		left := tokenRiskSeverity(risk)
		right := tokenRiskSeverity(best)
		if left > right || (left == right && risk.RiskControlLevel > best.RiskControlLevel) {
			best = risk
		}
	}
	return &best
}

func (s *Server) resolveTokenRiskForTargets(ctx context.Context, targets []tokenRiskTarget) *TokenRiskInfo {
	if len(targets) == 0 {
		return nil
	}
	risks := make([]TokenRiskInfo, 0, len(targets))
	for _, target := range targets {
		if !tokenRiskNeedsLookup(target) {
			continue
		}
		risks = append(risks, s.lookupTokenRisk(ctx, target))
	}
	return pickWorstTokenRisk(risks)
}

func (s *Server) enrichHotPoolTokenRisks(ctx context.Context, chain string, items []HotPoolResponse) {
	if len(items) == 0 {
		return
	}

	byKey := make(map[string][]int)
	targets := make(map[string]tokenRiskTarget)
	for i := range items {
		for _, target := range tokenRiskTargetsForPoolItem(items[i], chain) {
			key := tokenRiskCacheKey(target.Chain, target.Address)
			if key == "" {
				continue
			}
			if _, ok := targets[key]; !ok {
				targets[key] = target
			}
			byKey[key] = append(byKey[key], i)
		}
	}
	if len(targets) == 0 {
		return
	}

	results := loadTokenRiskSnapshots(ctx, targets)
	for key, target := range targets {
		if risk, ok := results[key]; ok {
			writeTokenRiskCache(risk)
			if !tokenRiskIsFresh(risk) {
				enqueueTokenRiskRefresh(target)
			}
			continue
		}
		if cached, ok := readTokenRiskCache(target.Chain, target.Address); ok {
			risk := withTokenRiskTargetMetadata(cached, target)
			results[key] = risk
			if !tokenRiskIsFresh(risk) {
				enqueueTokenRiskRefresh(target)
			}
			continue
		}
		results[key] = buildPendingTokenRiskInfo(target)
		enqueueTokenRiskRefresh(target)
	}

	perItem := make(map[int][]TokenRiskInfo, len(items))
	for key, risk := range results {
		for _, idx := range byKey[key] {
			perItem[idx] = append(perItem[idx], risk)
		}
	}
	for idx, risks := range perItem {
		items[idx].TokenRisk = pickWorstTokenRisk(risks)
	}
}

func resolveOpenPositionTokenRisk(ctx context.Context, s *Server, task *models.StrategyTask) *TokenRiskInfo {
	if s == nil || task == nil {
		return nil
	}
	return s.resolveTokenRiskForTargets(ctx, tokenRiskTargetsForTask(task))
}

func tokenRiskBlocksOpen(risk *TokenRiskInfo) bool {
	if risk == nil {
		return false
	}
	return risk.HasHoneypot
}

func tokenRiskCheckItem(risk *TokenRiskInfo) *openPositionCheckItem {
	if risk == nil {
		return nil
	}
	status := "pass"
	switch {
	case tokenRiskBlocksOpen(risk):
		status = "fail"
	case strings.TrimSpace(risk.Error) != "" || risk.HasLowLiquidity || risk.RiskControlLevel >= 3:
		status = "warn"
	}

	detail := "OKX risk level: " + risk.RiskControlLabel
	if len(risk.Warnings) > 0 {
		detail = strings.Join(risk.Warnings, "; ")
	}
	return &openPositionCheckItem{
		Key:    "token_risk",
		Status: status,
		Label:  "Token risk",
		Detail: detail,
		Extra: map[string]interface{}{
			"token_address":      risk.TokenAddress,
			"token_symbol":       risk.TokenSymbol,
			"risk_control_level": risk.RiskControlLevel,
			"risk_tone":          risk.RiskTone,
			"has_honeypot":       risk.HasHoneypot,
			"has_low_liquidity":  risk.HasLowLiquidity,
		},
	}
}

func appendTokenRiskCheck(checks []openPositionCheckItem, risk *TokenRiskInfo) []openPositionCheckItem {
	check := tokenRiskCheckItem(risk)
	if check == nil {
		return checks
	}
	return append(checks, *check)
}

func tokenRiskBlockMessage(risk *TokenRiskInfo) string {
	if risk == nil {
		return "Token risk check failed"
	}
	if len(risk.Warnings) > 0 {
		return strings.Join(risk.Warnings, "; ")
	}
	symbol := strings.TrimSpace(risk.TokenSymbol)
	if symbol == "" {
		symbol = strings.TrimSpace(risk.TokenAddress)
	}
	return fmt.Sprintf("%s token risk check failed", symbol)
}

func logTokenRiskWarning(scope string, risk *TokenRiskInfo) {
	if risk == nil || len(risk.Warnings) == 0 {
		return
	}
	log.Printf("[TokenRisk] %s token=%s symbol=%s tone=%s warnings=%s",
		scope,
		risk.TokenAddress,
		risk.TokenSymbol,
		risk.RiskTone,
		strings.Join(risk.Warnings, "|"),
	)
}
