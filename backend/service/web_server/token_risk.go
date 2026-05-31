package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/exchange"
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	tokenRiskSourceOKX       = "okx_market_token_advanced_info"
	tokenRiskCacheTTL        = 5 * time.Minute
	tokenRiskErrorCacheTTL   = 45 * time.Second
	tokenRiskLookupTimeout   = 5 * time.Second
	tokenRiskListConcurrency = 4
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

var tokenRiskCache sync.Map

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
		warnings = append(warnings, "OKX risk lookup failed: "+strings.TrimSpace(queryErr.Error()))
		return warnings
	}
	if hasHoneypot {
		warnings = append(warnings, "OKX marked honeypot")
	}
	if hasLowLiquidity {
		warnings = append(warnings, "OKX marked low liquidity")
	}
	if level >= 3 {
		warnings = append(warnings, "OKX risk level: "+tokenRiskLevelLabel(level))
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

	if queryErr != nil {
		risk.Error = strings.TrimSpace(queryErr.Error())
		risk.Warnings = tokenRiskWarningList(0, false, false, queryErr)
		return risk
	}
	if advanced == nil {
		err := fmt.Errorf("OKX advanced-info returned empty data")
		risk.Error = err.Error()
		risk.Warnings = tokenRiskWarningList(0, false, false, err)
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
	return risk
}

func (s *Server) lookupTokenRisk(ctx context.Context, target tokenRiskTarget) TokenRiskInfo {
	if cached, ok := readTokenRiskCache(target.Chain, target.Address); ok {
		if strings.TrimSpace(cached.TokenSymbol) == "" {
			cached.TokenSymbol = strings.TrimSpace(target.Symbol)
		}
		if strings.TrimSpace(cached.TokenName) == "" {
			cached.TokenName = strings.TrimSpace(target.Name)
		}
		return cached
	}

	chainIndex := config.ChainToOKXChainIndex(target.Chain)
	if chainIndex == "" {
		risk := buildTokenRiskInfo(target, "", nil, fmt.Errorf("unsupported chain for OKX token risk: %s", target.Chain))
		writeTokenRiskCache(risk)
		return risk
	}

	reqCtx, cancel := context.WithTimeout(ctx, tokenRiskLookupTimeout)
	defer cancel()

	resp, err := exchange.NewOKXDexService().GetMarketTokenAdvancedInfo(reqCtx, exchange.MarketTokenAdvancedInfoRequest{
		ChainIndex:           chainIndex,
		TokenContractAddress: target.Address,
	})
	if err != nil {
		risk := buildTokenRiskInfo(target, chainIndex, nil, err)
		writeTokenRiskCache(risk)
		return risk
	}
	if resp == nil || len(resp.Data) == 0 {
		risk := buildTokenRiskInfo(target, chainIndex, nil, nil)
		writeTokenRiskCache(risk)
		return risk
	}

	risk := buildTokenRiskInfo(target, chainIndex, &resp.Data[0], nil)
	writeTokenRiskCache(risk)
	return risk
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

	results := make(map[string]TokenRiskInfo, len(targets))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, tokenRiskListConcurrency)

	for key, target := range targets {
		key, target := key, target
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				risk := buildTokenRiskInfo(target, config.ChainToOKXChainIndex(target.Chain), nil, ctx.Err())
				mu.Lock()
				results[key] = risk
				mu.Unlock()
				return
			}

			risk := s.lookupTokenRisk(ctx, target)
			mu.Lock()
			results[key] = risk
			mu.Unlock()
		}()
	}
	wg.Wait()

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
