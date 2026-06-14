package web_server

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/pricing"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type poolCatalogOptions struct {
	Chain            string
	Sort             string
	TimeframeMinutes int
	Limit            int
	TokenAddress     string
	IncludePools     []string
	Dexes            []string
	MaxFeeRate       *float64
	MinFDVUSD        *float64
}

func parsePoolCatalogOptions(r *http.Request) (poolCatalogOptions, error) {
	query := r.URL.Query()

	chain := strings.ToLower(strings.TrimSpace(query.Get("chain")))
	if chain == "" {
		chain = "bsc"
	}

	sortKey := strings.ToLower(strings.TrimSpace(query.Get("sort")))
	switch sortKey {
	case "fee_rate", "volume":
	default:
		sortKey = "fees"
	}

	timeframe := 5
	if raw := strings.TrimSpace(query.Get("timeframe_minutes")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			timeframe = n
		}
	}
	if timeframe != 5 {
		timeframe = 5
	}

	limit := 50
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}

	var maxFeeRate *float64
	if raw := strings.TrimSpace(query.Get("max_fee_rate")); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return poolCatalogOptions{}, fmt.Errorf("invalid max_fee_rate")
		}
		maxFeeRate = &value
	}
	var minFDVUSD *float64
	minFDVParam := "min_fdv_usd"
	rawMinFDV := strings.TrimSpace(query.Get(minFDVParam))
	if rawMinFDV == "" {
		minFDVParam = "min_market_cap_usd"
		rawMinFDV = strings.TrimSpace(query.Get(minFDVParam))
	}
	if rawMinFDV != "" {
		value, err := strconv.ParseFloat(rawMinFDV, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return poolCatalogOptions{}, fmt.Errorf("invalid %s", minFDVParam)
		}
		minFDVUSD = &value
	}

	return poolCatalogOptions{
		Chain:            chain,
		Sort:             sortKey,
		TimeframeMinutes: timeframe,
		Limit:            limit,
		TokenAddress:     normalizeCatalogHex(query.Get("token_address")),
		IncludePools:     normalizeCatalogHexList(query.Get("include_pools")),
		Dexes:            splitCatalogCSV(query.Get("dex")),
		MaxFeeRate:       maxFeeRate,
		MinFDVUSD:        minFDVUSD,
	}, nil
}

func buildPoolCatalogCacheKey(opts poolCatalogOptions) string {
	if len(opts.IncludePools) > 0 || opts.TokenAddress != "" || len(opts.Dexes) > 0 {
		return ""
	}
	if opts.MaxFeeRate != nil || opts.MinFDVUSD != nil {
		return ""
	}
	return fmt.Sprintf(
		"pools:catalog:v6:chain=%s:sort=%s:tf=%d:limit=%d",
		opts.Chain,
		opts.Sort,
		opts.TimeframeMinutes,
		opts.Limit,
	)
}

func loadPoolCatalogRows(ctx context.Context, opts poolCatalogOptions) ([]models.Pool, error) {
	if database.DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	topLimit := opts.Limit * 5
	if topLimit < 100 {
		topLimit = 100
	}
	if opts.MinFDVUSD != nil && topLimit < 1000 {
		topLimit = 1000
	}
	maxTopLimit := 500
	if opts.MinFDVUSD != nil {
		maxTopLimit = 2000
	}
	if topLimit > maxTopLimit {
		topLimit = maxTopLimit
	}

	topRows := make([]models.Pool, 0, topLimit)
	topQuery := buildPoolCatalogBaseQuery(ctx, opts)
	if err := topQuery.
		Order(poolCatalogOrderClause(opts)).
		Limit(topLimit).
		Find(&topRows).Error; err != nil {
		return nil, err
	}

	merged := make([]models.Pool, 0, len(topRows)+len(opts.IncludePools))
	seen := make(map[string]struct{}, len(topRows)+len(opts.IncludePools))

	appendRows := func(rows []models.Pool) {
		for _, row := range rows {
			addr := normalizeCatalogHex(row.Address)
			if addr == "" {
				continue
			}
			if _, ok := seen[addr]; ok {
				continue
			}
			seen[addr] = struct{}{}
			merged = append(merged, row)
		}
	}

	appendRows(topRows)

	if len(opts.IncludePools) > 0 {
		includedRows := make([]models.Pool, 0, len(opts.IncludePools))
		if err := buildPoolCatalogBaseQuery(ctx, opts).
			Where("address IN ?", opts.IncludePools).
			Find(&includedRows).Error; err != nil {
			return nil, err
		}
		appendRows(includedRows)
	}

	return merged, nil
}

func buildPoolCatalogBaseQuery(ctx context.Context, opts poolCatalogOptions) *gorm.DB {
	query := database.DB.WithContext(ctx).Model(&models.Pool{})
	if opts.Chain != "" {
		query = query.Where("chain = ?", opts.Chain)
	}
	if opts.TokenAddress != "" {
		query = query.Where(
			"base_token_id = ? OR quote_token_id = ?",
			opts.TokenAddress,
			opts.TokenAddress,
		)
	}
	if len(opts.Dexes) > 0 {
		query = query.Where("(LOWER(factory_name) IN ? OR LOWER(dex_id) IN ?)", opts.Dexes, opts.Dexes)
	}
	return query
}

type poolCatalogEnvelope struct {
	Success             bool              `json:"success"`
	Chain               string            `json:"chain,omitempty"`
	Sort                string            `json:"sort,omitempty"`
	Timeframe           string            `json:"timeframe"`
	TimeframeMinutes    int               `json:"timeframe_minutes"`
	RequestedLimit      int               `json:"requested_limit"`
	RequestedProtocol   json.RawMessage   `json:"requested_protocol"`
	RequestedChain      string            `json:"requested_chain"`
	RequestedDex        json.RawMessage   `json:"requested_dex"`
	TotalPools          int               `json:"total_pools"`
	MetricTrendsIndex   json.RawMessage   `json:"metricTrendsIndex"`
	LiquidityTicksIndex json.RawMessage   `json:"liquidityTicksIndex"`
	UpdatedAt           time.Time         `json:"updated_at"`
	Data                []HotPoolResponse `json:"data"`
}

func (s *Server) buildPoolCatalogResponse(ctx context.Context, rows []models.Pool, opts poolCatalogOptions) poolCatalogEnvelope {
	items := make([]HotPoolResponse, 0, len(rows))
	var updatedAt time.Time
	meta := poolCatalogEnvelope{
		Success:             true,
		Chain:               opts.Chain,
		Sort:                opts.Sort,
		Timeframe:           "5 minutes",
		TimeframeMinutes:    5,
		RequestedLimit:      opts.Limit,
		RequestedProtocol:   rawJSONFromString("", "[]"),
		RequestedChain:      opts.Chain,
		RequestedDex:        rawJSONFromString("", "[]"),
		TotalPools:          0,
		MetricTrendsIndex:   rawJSONFromString("", "[]"),
		LiquidityTicksIndex: rawJSONFromString("", "[]"),
		Data:                []HotPoolResponse{},
	}
	for _, row := range rows {
		if poolCatalogLiquidityUSD(row) < 100 {
			continue
		}
		if row.UpdatedAt.After(updatedAt) {
			updatedAt = row.UpdatedAt
			meta.Timeframe = strings.TrimSpace(row.SourceTimeframe)
			if meta.Timeframe == "" {
				meta.Timeframe = "5 minutes"
			}
			if row.SourceRequestedLimit > 0 {
				meta.RequestedLimit = row.SourceRequestedLimit
			}
			meta.RequestedProtocol = rawJSONFromString(row.SourceRequestedProtocolJSON, "[]")
			meta.RequestedChain = strings.TrimSpace(firstNonEmpty(row.SourceRequestedChain, row.Chain, opts.Chain))
			meta.RequestedDex = rawJSONFromString(row.SourceRequestedDexJSON, "[]")
			meta.TotalPools = row.SourceTotalPools
			meta.MetricTrendsIndex = rawJSONFromString(row.MetricTrendsIndexJSON, "[]")
			meta.LiquidityTicksIndex = rawJSONFromString(row.LiquidityTicksIndexJSON, "[]")
		}
		item := buildPoolCatalogItem(row, opts)
		if item.PoolAddress == "" {
			continue
		}
		if opts.MaxFeeRate != nil && (!isFinitePositiveOrZero(item.FeeRate) || item.FeeRate > *opts.MaxFeeRate) {
			continue
		}
		items = append(items, item)
	}

	s.enrichHotPoolMarketData(ctx, opts.Chain, items)
	if opts.MinFDVUSD != nil {
		filtered := items[:0]
		for _, item := range items {
			if poolCatalogFDVUSD(item) < *opts.MinFDVUSD {
				continue
			}
			filtered = append(filtered, item)
		}
		items = filtered
	}

	sort.SliceStable(items, func(i, j int) bool {
		left := poolCatalogSortMetric(items[i], opts.Sort)
		right := poolCatalogSortMetric(items[j], opts.Sort)
		if left != right {
			return left > right
		}
		if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		return items[i].PoolAddress < items[j].PoolAddress
	})

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if len(opts.IncludePools) > 0 {
		limit += len(opts.IncludePools)
	}
	if limit > 200 {
		limit = 200
	}
	if len(items) > limit {
		items = items[:limit]
	}

	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	meta.UpdatedAt = updatedAt
	meta.Data = items
	return meta
}

func poolCatalogLiquidityUSD(row models.Pool) float64 {
	if liq := sanitizeFloat(row.ActiveLiquidityUSD); liq > 0 {
		return liq
	}
	if liq := sanitizeFloat(row.CurrentPoolValue); liq > 0 {
		return liq
	}
	return sanitizeFloat(row.ReserveInUSD)
}

func isFinitePositiveOrZero(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func poolCatalogFDVUSD(item HotPoolResponse) float64 {
	if value := sanitizeFloat(item.FDVUSD); value > 0 {
		return value
	}
	return sanitizeFloat(item.CurrentTokenFDVUSD)
}

func buildPoolCatalogItem(row models.Pool, opts poolCatalogOptions) HotPoolResponse {
	totalFees, totalVolume := poolCatalogWindowMetrics(row, opts.TimeframeMinutes)
	currentPoolValue := sanitizeFloat(row.CurrentPoolValue)
	if currentPoolValue <= 0 {
		currentPoolValue = sanitizeFloat(row.ReserveInUSD)
	}
	feeRate := 0.0
	if currentPoolValue > 0 && totalFees > 0 {
		feeRate = totalFees / currentPoolValue * 100.0
	}

	txCount := int(row.TransactionCount)
	if txCount <= 0 {
		txCount = int(row.TransactionsH24Buys) + int(row.TransactionsH24Sells)
	}
	if txCount < 0 {
		txCount = 0
	}

	protocolVersion := inferPoolCatalogProtocolVersion(row)
	priceDisplay := strings.TrimSpace(row.PriceDisplay)
	if priceDisplay == "" {
		priceDisplay = formatPoolCatalogPrice(firstPositiveFloat(row.CurrentTokenPrice, row.BaseTokenPriceUSD))
	}
	fees24h := sanitizeFloat(row.FeeUSDH24)
	volume24h := sanitizeFloat(row.VolumeH24)
	txCount24h := uint32(int(row.TransactionsH24Buys) + int(row.TransactionsH24Sells))

	return HotPoolResponse{
		Chain:                   normalizeCatalogLower(firstNonEmpty(row.Chain, opts.Chain)),
		ProtocolVersion:         protocolVersion,
		PoolAddress:             normalizeCatalogHex(row.Address),
		Dex:                     strings.TrimSpace(firstNonEmpty(row.DexID, row.FactoryName)),
		FactoryName:             inferPoolCatalogFactoryName(row, protocolVersion),
		FactoryAddress:          normalizeCatalogHex(row.FactoryAddress),
		TradingPair:             sanitizePoolCatalogName(row.Name),
		FeePercentage:           sanitizeFloat(row.PoolFeePercentage),
		FeeRate:                 sanitizeFloat(feeRate),
		FeeTier:                 row.PoolMFeeRate,
		TransactionCount:        uint32(txCount),
		TotalFees:               sanitizeFloat(totalFees),
		TotalVolume:             sanitizeFloat(totalVolume),
		CurrentPoolValue:        currentPoolValue,
		PriceDisplay:            priceDisplay,
		UpdatedAt:               row.UpdatedAt,
		LastSwapAt:              row.LastSwapAt,
		Token0Address:           normalizeCatalogHex(row.BaseTokenID),
		Token1Address:           normalizeCatalogHex(row.QuoteTokenID),
		Token0Symbol:            strings.TrimSpace(row.Token0Symbol),
		Token1Symbol:            strings.TrimSpace(row.Token1Symbol),
		Token0Name:              strings.TrimSpace(row.Token0Name),
		Token1Name:              strings.TrimSpace(row.Token1Name),
		Token0Decimals:          row.Token0Decimals,
		Token1Decimals:          row.Token1Decimals,
		StableCoinSymbol:        strings.TrimSpace(row.StableCoinSymbol),
		HookAddress:             normalizeCatalogHex(row.HookAddress),
		CurrentToken0Balance:    sanitizeFloat(row.CurrentToken0Balance),
		CurrentToken1Balance:    sanitizeFloat(row.CurrentToken1Balance),
		CurrentTokenPrice:       sanitizeFloat(firstPositiveFloat(row.CurrentTokenPrice, row.BaseTokenPriceUSD)),
		PricedTokenAddress:      normalizeCatalogHex(row.PricedTokenAddress),
		CurrentTokenTotalSupply: sanitizeFloat(row.CurrentTokenTotalSupply),
		CurrentTokenFDVUSD:      0,
		MarketCapUSD:            0,
		FDVUSD:                  0,
		TokenSupplyUpdatedAt:    row.TokenSupplyUpdatedAt,
		TickSpacing:             cloneCatalogInt(row.TickSpacing),
		CurrentTick:             row.CurrentTick,
		CurrentSqrtPriceX96:     strings.TrimSpace(row.CurrentSqrtPriceX96),
		CurrentLiquidity:        strings.TrimSpace(row.CurrentLiquidity),
		StableCoinPosition:      strings.TrimSpace(row.StableCoinPosition),
		MetricTrends:            rawJSONFromString(row.MetricTrendsJSON, "[]"),
		UniqueWallets:           row.UniqueWallets,
		TopWalletVolPct:         sanitizeFloat(row.TopWalletVolPct),
		ActiveTickCount:         row.ActiveTickCount,
		ActiveLiquidityUSD:      sanitizeFloat(row.ActiveLiquidityUSD),
		ActiveLiquidityRatio:    sanitizeFloat(row.ActiveLiquidityRatio),
		LiquidityTicks:          rawJSONFromString(row.LiquidityTicksJSON, "[]"),
		LiquidityCurrentTick:    row.LiquidityCurrentTick,
		LiquidityTickSpacing:    row.LiquidityTickSpacing,
		Badges:                  rawJSONFromString(row.BadgesJSON, "[]"),
		TotalFees24h:            fees24h,
		TotalVolume24h:          volume24h,
		TransactionCount24h:     txCount24h,
	}
}

func (s *Server) enrichHotPoolMarketData(ctx context.Context, chain string, items []HotPoolResponse) {
	if s == nil || s.TokenPrice == nil || len(items) == 0 {
		return
	}

	addresses := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for i := range items {
		addr, symbol := poolCatalogPickMarketCapToken(chain, items[i])
		items[i].MarketCapTokenAddress = addr
		items[i].MarketCapTokenSymbol = symbol
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		addresses = append(addresses, addr)
	}
	if len(addresses) == 0 {
		return
	}

	data, err := s.TokenPrice.GetTokenMarketDataWithContext(ctx, chain, addresses)
	if err != nil {
		log.Printf("[Pools API] load realtime token market data failed chain=%s err=%v", chain, err)
	}
	if len(data) == 0 {
		return
	}
	for i := range items {
		addr := strings.ToLower(strings.TrimSpace(items[i].MarketCapTokenAddress))
		if addr == "" {
			continue
		}
		info, ok := data[addr]
		if !ok {
			continue
		}
		items[i].MarketCapUSD = sanitizeFloat(info.MarketCapUSD)
		items[i].FDVUSD = sanitizeFloat(info.FDVUSD)
		items[i].CurrentTokenFDVUSD = sanitizeFloat(info.FDVUSD)
		items[i].MarketCapProvider = strings.TrimSpace(info.Provider)
	}
}

var poolCatalogStableSymbols = map[string]struct{}{
	"usdc":  {},
	"usdt":  {},
	"busd":  {},
	"dai":   {},
	"frax":  {},
	"usdd":  {},
	"fdusd": {},
	"wbnb":  {},
	"weth":  {},
	"wsol":  {},
	"bnb":   {},
	"eth":   {},
	"sol":   {},
}

func poolCatalogStableLikeSymbol(symbol string) bool {
	_, ok := poolCatalogStableSymbols[strings.ToLower(strings.TrimSpace(symbol))]
	return ok
}

func poolCatalogQuoteLikeToken(chain string, address string, symbol string) bool {
	if pricing.IsStableAddress(chain, address) || pricing.IsWrappedNativeAddress(chain, address) {
		return true
	}
	return pricing.IsQuoteLikeSymbol(symbol)
}

func poolCatalogPickMarketCapToken(chain string, item HotPoolResponse) (string, string) {
	leftAddress := normalizeCatalogHex(item.Token0Address)
	rightAddress := normalizeCatalogHex(item.Token1Address)
	leftSymbol := strings.TrimSpace(firstNonEmpty(item.Token0Symbol, poolCatalogPairSymbolsLeft(item.TradingPair)))
	rightSymbol := strings.TrimSpace(firstNonEmpty(item.Token1Symbol, poolCatalogPairSymbolsRight(item.TradingPair)))

	leftQuote := poolCatalogQuoteLikeToken(chain, leftAddress, leftSymbol)
	rightQuote := poolCatalogQuoteLikeToken(chain, rightAddress, rightSymbol)
	switch {
	case leftQuote && !rightQuote:
		return rightAddress, rightSymbol
	case rightQuote && !leftQuote:
		return leftAddress, leftSymbol
	case !leftQuote && !rightQuote:
		displayAddress := normalizeCatalogHex(item.DisplayTokenAddress)
		if displayAddress != "" && displayAddress == rightAddress {
			return rightAddress, rightSymbol
		}
		return leftAddress, leftSymbol
	default:
		return "", ""
	}
}

func poolCatalogPairSymbols(pair string) (string, string) {
	parts := strings.Split(strings.TrimSpace(pair), "/")
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func poolCatalogPickDisplayToken(item *HotPoolResponse) {
	if item == nil {
		return
	}

	leftSymbol := strings.TrimSpace(firstNonEmpty(item.Token0Symbol, poolCatalogPairSymbolsLeft(item.TradingPair)))
	rightSymbol := strings.TrimSpace(firstNonEmpty(item.Token1Symbol, poolCatalogPairSymbolsRight(item.TradingPair)))
	leftAddress := normalizeCatalogHex(item.Token0Address)
	rightAddress := normalizeCatalogHex(item.Token1Address)
	leftName := strings.TrimSpace(item.Token0Name)
	rightName := strings.TrimSpace(item.Token1Name)

	switch strings.ToLower(strings.TrimSpace(item.StableCoinPosition)) {
	case "token0":
		item.DisplayTokenAddress = rightAddress
		item.DisplayTokenSymbol = rightSymbol
		item.DisplayTokenName = rightName
	case "token1":
		item.DisplayTokenAddress = leftAddress
		item.DisplayTokenSymbol = leftSymbol
		item.DisplayTokenName = leftName
	}

	if item.DisplayTokenAddress != "" || item.DisplayTokenSymbol != "" {
		return
	}

	leftStable := poolCatalogStableLikeSymbol(leftSymbol)
	rightStable := poolCatalogStableLikeSymbol(rightSymbol)

	switch {
	case leftStable && !rightStable:
		item.DisplayTokenAddress = rightAddress
		item.DisplayTokenSymbol = rightSymbol
		item.DisplayTokenName = rightName
	case rightStable && !leftStable:
		item.DisplayTokenAddress = leftAddress
		item.DisplayTokenSymbol = leftSymbol
		item.DisplayTokenName = leftName
	default:
		item.DisplayTokenAddress = leftAddress
		item.DisplayTokenSymbol = leftSymbol
		item.DisplayTokenName = leftName
	}

	if strings.TrimSpace(item.DisplayTokenAddress) == "" {
		item.DisplayTokenAddress = rightAddress
	}
	if strings.TrimSpace(item.DisplayTokenSymbol) == "" {
		item.DisplayTokenSymbol = rightSymbol
	}
	if strings.TrimSpace(item.DisplayTokenName) == "" {
		item.DisplayTokenName = rightName
	}
}

func (s *Server) enrichHotPoolDisplayTokens(ctx context.Context, chain string, items []HotPoolResponse) {
	if s == nil || s.TokenMeta == nil || len(items) == 0 {
		return
	}

	addresses := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for i := range items {
		poolCatalogPickDisplayToken(&items[i])
		addr := strings.TrimSpace(items[i].DisplayTokenAddress)
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		addresses = append(addresses, addr)
	}

	if len(addresses) == 0 {
		return
	}

	meta, err := s.TokenMeta.GetBatch(ctx, chain, addresses)
	if err != nil {
		log.Printf("[Pools API] load token metadata failed chain=%s err=%v", chain, err)
	}
	if len(meta) == 0 {
		return
	}

	for i := range items {
		addr := strings.TrimSpace(items[i].DisplayTokenAddress)
		if addr == "" {
			continue
		}
		info, ok := meta[addr]
		if !ok {
			continue
		}
		if strings.TrimSpace(items[i].DisplayTokenSymbol) == "" {
			items[i].DisplayTokenSymbol = strings.TrimSpace(info.Symbol)
		}
		if strings.TrimSpace(info.Symbol) != "" {
			items[i].DisplayTokenSymbol = strings.TrimSpace(info.Symbol)
		}
		if strings.TrimSpace(items[i].DisplayTokenName) == "" {
			items[i].DisplayTokenName = strings.TrimSpace(info.Name)
		}
		if logoURL := strings.TrimSpace(info.LogoURL); logoURL != "" {
			items[i].DisplayTokenLogoURL = logoURL
		}
	}
}

func poolCatalogWindowMetrics(row models.Pool, timeframeMinutes int) (float64, float64) {
	_ = timeframeMinutes
	if row.TotalFees > 0 || row.TotalVolume > 0 {
		return row.TotalFees, row.TotalVolume
	}
	return row.FeeUSDM5, row.VolumeM5
}

func poolCatalogSortMetric(item HotPoolResponse, sortKey string) float64 {
	switch sortKey {
	case "volume":
		return item.TotalVolume
	case "fee_rate":
		return item.FeeRate
	default:
		return item.TotalFees
	}
}

func poolCatalogOrderClause(opts poolCatalogOptions) string {
	switch opts.Sort {
	case "volume":
		return "total_volume DESC, updated_at DESC"
	case "fee_rate":
		return "CASE WHEN current_pool_value > 0 THEN total_fees / current_pool_value * 100 ELSE 0 END DESC, updated_at DESC"
	default:
		return "total_fees DESC, updated_at DESC"
	}
}

func normalizeCatalogHex(value string) string {
	raw := strings.ToLower(strings.TrimSpace(value))
	if raw == "" {
		return ""
	}
	raw = strings.TrimPrefix(raw, "0x")
	switch len(raw) {
	case 40, 64:
		return "0x" + raw
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeCatalogHexList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		value := normalizeCatalogHex(part)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func splitCatalogCSV(raw string) []string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(raw)), ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func sanitizePoolCatalogName(name string) string {
	cleaned := strings.TrimSpace(poolFeeFromNameRegex.ReplaceAllString(name, ""))
	if cleaned != "" {
		return cleaned
	}
	return strings.TrimSpace(name)
}

func inferPoolCatalogProtocolVersion(row models.Pool) string {
	text := strings.ToLower(strings.TrimSpace(firstNonEmpty(row.ProtocolVersion, row.DexID, row.FactoryName)))
	switch {
	case strings.Contains(text, "v4"):
		return "v4"
	case strings.Contains(text, "v3"):
		return "v3"
	}

	addr := strings.TrimPrefix(normalizeCatalogHex(row.Address), "0x")
	switch len(addr) {
	case 64:
		return "v4"
	case 40:
		return "v3"
	default:
		return ""
	}
}

func inferPoolCatalogFactoryName(row models.Pool, protocolVersion string) string {
	if raw := strings.TrimSpace(row.FactoryName); raw != "" {
		return raw
	}

	raw := strings.TrimSpace(row.DexID)
	lower := strings.ToLower(raw)

	switch {
	case strings.Contains(lower, "pancake"):
		if protocolVersion == "v4" {
			return "PancakeSwap V4"
		}
		return "PancakeSwap V3"
	case strings.Contains(lower, "uniswap"), strings.Contains(lower, "univ"), strings.Contains(lower, "uni"):
		if protocolVersion == "v4" {
			return "Uniswap V4"
		}
		return "Uniswap V3"
	}

	if raw == "" {
		if protocolVersion == "" {
			return "DEX"
		}
		return strings.ToUpper(protocolVersion)
	}
	if protocolVersion != "" && !strings.Contains(lower, protocolVersion) {
		return strings.TrimSpace(raw + " " + strings.ToUpper(protocolVersion))
	}
	return raw
}

func formatPoolCatalogPrice(price float64) string {
	price = sanitizeFloat(price)
	if price <= 0 {
		return ""
	}

	precision := 4
	switch {
	case price < 0.0001:
		precision = 10
	case price < 0.01:
		precision = 8
	case price < 1:
		precision = 6
	}

	text := strconv.FormatFloat(price, 'f', precision, 64)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "" {
		return ""
	}
	return "$" + text
}

func rawJSONFromString(raw string, fallback string) json.RawMessage {
	text := strings.TrimSpace(raw)
	if text == "" {
		return json.RawMessage(fallback)
	}
	if !json.Valid([]byte(text)) {
		return json.RawMessage(fallback)
	}
	return json.RawMessage(text)
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func cloneCatalogInt(value *int) *int {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}

func poolCatalogPairSymbolsLeft(pair string) string {
	left, _ := poolCatalogPairSymbols(pair)
	return left
}

func poolCatalogPairSymbolsRight(pair string) string {
	_, right := poolCatalogPairSymbols(pair)
	return right
}

func normalizeCatalogLower(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func metricTrendPriceChange(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}

	var rows [][]float64
	if err := json.Unmarshal(raw, &rows); err != nil {
		return 0
	}

	first := 0.0
	last := 0.0
	for _, row := range rows {
		if len(row) < 5 {
			continue
		}
		price := row[4]
		if price <= 0 {
			continue
		}
		if first <= 0 {
			first = price
		}
		last = price
	}
	if first <= 0 || last <= 0 {
		return 0
	}
	return (last/first - 1) * 100
}
