package web_server

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"fmt"
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
}

func parsePoolCatalogOptions(r *http.Request) poolCatalogOptions {
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

	limit := 50
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}

	return poolCatalogOptions{
		Chain:            chain,
		Sort:             sortKey,
		TimeframeMinutes: timeframe,
		Limit:            limit,
		TokenAddress:     normalizeCatalogHex(query.Get("token_address")),
		IncludePools:     normalizeCatalogHexList(query.Get("include_pools")),
		Dexes:            splitCatalogCSV(query.Get("dex")),
	}
}

func buildPoolCatalogCacheKey(opts poolCatalogOptions) string {
	if len(opts.IncludePools) > 0 || opts.TokenAddress != "" || len(opts.Dexes) > 0 {
		return ""
	}
	return fmt.Sprintf(
		"pools:catalog:v3:chain=%s:sort=%s:tf=%d:limit=%d",
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
	if opts.Chain != "" && opts.Chain != "bsc" {
		return nil, nil
	}

	topLimit := opts.Limit * 5
	if topLimit < 100 {
		topLimit = 100
	}
	if topLimit > 500 {
		topLimit = 500
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
			Where("LOWER(address) IN ?", opts.IncludePools).
			Find(&includedRows).Error; err != nil {
			return nil, err
		}
		appendRows(includedRows)
	}

	return merged, nil
}

func buildPoolCatalogBaseQuery(ctx context.Context, opts poolCatalogOptions) *gorm.DB {
	query := database.DB.WithContext(ctx).Model(&models.Pool{})
	if opts.TokenAddress != "" {
		query = query.Where(
			"LOWER(base_token_id) = ? OR LOWER(quote_token_id) = ?",
			opts.TokenAddress,
			opts.TokenAddress,
		)
	}
	if len(opts.Dexes) > 0 {
		query = query.Where("LOWER(dex_id) IN ?", opts.Dexes)
	}
	return query
}

func buildPoolCatalogResponse(rows []models.Pool, opts poolCatalogOptions) ([]HotPoolResponse, time.Time) {
	items := make([]HotPoolResponse, 0, len(rows))
	var updatedAt time.Time
	for _, row := range rows {
		item := buildPoolCatalogItem(row, opts)
		if item.PoolAddress == "" {
			continue
		}
		if item.UpdatedAt.After(updatedAt) {
			updatedAt = item.UpdatedAt
		}
		items = append(items, item)
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
	return items, updatedAt
}

func buildPoolCatalogItem(row models.Pool, opts poolCatalogOptions) HotPoolResponse {
	totalFees, totalVolume := poolCatalogWindowMetrics(row, opts.TimeframeMinutes)
	currentPoolValue := sanitizeFloat(row.ReserveInUSD)
	feeRate := 0.0
	if currentPoolValue > 0 && totalFees > 0 {
		feeRate = totalFees / currentPoolValue * 100.0
	}

	txCount := int(row.TransactionsH24Buys) + int(row.TransactionsH24Sells)
	if txCount < 0 {
		txCount = 0
	}

	protocolVersion := inferPoolCatalogProtocolVersion(row)

	return HotPoolResponse{
		ProtocolVersion:  protocolVersion,
		PoolAddress:      normalizeCatalogHex(row.Address),
		Dex:              strings.TrimSpace(row.DexID),
		FactoryName:      inferPoolCatalogFactoryName(row, protocolVersion),
		TradingPair:      sanitizePoolCatalogName(row.Name),
		FeePercentage:    sanitizeFloat(row.PoolFeePercentage),
		TransactionCount: uint32(txCount),
		TotalFees:        sanitizeFloat(totalFees),
		TotalVolume:      sanitizeFloat(totalVolume),
		CurrentPoolValue: currentPoolValue,
		FeeRate:          sanitizeFloat(feeRate),
		PriceDisplay:     formatPoolCatalogPrice(row.BaseTokenPriceUSD),
		UpdatedAt:        row.UpdatedAt,
		LastSwapAt:       time.Time{},
		Token0Address:    normalizeCatalogHex(row.BaseTokenID),
		Token1Address:    normalizeCatalogHex(row.QuoteTokenID),
	}
}

func poolCatalogWindowMetrics(row models.Pool, timeframeMinutes int) (float64, float64) {
	switch {
	case timeframeMinutes <= 5:
		return row.FeeUSDM5, row.VolumeM5
	case timeframeMinutes <= 60:
		return row.FeeUSDH1, row.VolumeH1
	case timeframeMinutes <= 360:
		return row.FeeUSDH6, row.VolumeH6
	default:
		return row.FeeUSDH24, row.VolumeH24
	}
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
		switch {
		case opts.TimeframeMinutes <= 5:
			return "volume_m5 DESC"
		case opts.TimeframeMinutes <= 60:
			return "volume_h1 DESC"
		case opts.TimeframeMinutes <= 360:
			return "volume_h6 DESC"
		default:
			return "volume_h24 DESC"
		}
	case "fee_rate":
		switch {
		case opts.TimeframeMinutes <= 5:
			return "fee_apr_m5 DESC"
		case opts.TimeframeMinutes <= 60:
			return "fee_apr_h1 DESC"
		case opts.TimeframeMinutes <= 360:
			return "fee_apr_h6 DESC"
		default:
			return "fee_apr_h24 DESC"
		}
	default:
		switch {
		case opts.TimeframeMinutes <= 5:
			return "fee_usd_m5 DESC"
		case opts.TimeframeMinutes <= 60:
			return "fee_usd_h1 DESC"
		case opts.TimeframeMinutes <= 360:
			return "fee_usd_h6 DESC"
		default:
			return "fee_usd_h24 DESC"
		}
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
	text := strings.ToLower(strings.TrimSpace(row.DexID))
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
