package pool_sync

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultSyncInterval = 60 * time.Second
	defaultRetention    = 24 * time.Hour
)

type Service struct {
	sources *PoolDataSourceManager

	stopCh   chan struct{}
	stopOnce sync.Once
	ticker   *time.Ticker
}

func NewService() *Service {
	return &Service{
		sources: DefaultPoolDataSourceManager(),
		stopCh:  make(chan struct{}),
	}
}

func (s *Service) Start() {
	if s == nil {
		return
	}
	if config.AppConfig != nil && !config.AppConfig.PoolsSyncEnabled {
		log.Println("[PoolSync] disabled")
		return
	}

	interval := defaultSyncInterval
	if config.AppConfig != nil && config.AppConfig.PoolsSyncIntervalSeconds > 0 {
		interval = time.Duration(config.AppConfig.PoolsSyncIntervalSeconds) * time.Second
	}
	s.ticker = time.NewTicker(interval)

	go func() {
		s.runOnce()
		for {
			select {
			case <-s.stopCh:
				return
			case <-s.ticker.C:
				s.runOnce()
			}
		}
	}()
}

func (s *Service) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		if s.ticker != nil {
			s.ticker.Stop()
		}
	})
}

func (s *Service) runOnce() {
	if database.DB == nil {
		log.Println("[PoolSync] skipped: mysql not initialized")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	start := time.Now()
	snapshot, err := s.fetchSnapshot(ctx)
	if err != nil {
		log.Printf("[PoolSync] fetch snapshot failed: %v", err)
		return
	}
	if snapshot == nil || len(snapshot.Data) == 0 {
		log.Printf("[PoolSync] no pools fetched")
		return
	}

	rows, err := s.buildRows(snapshot, time.Now())
	if err != nil {
		log.Printf("[PoolSync] build rows failed: %v", err)
		return
	}
	if len(rows) == 0 {
		log.Printf("[PoolSync] no rows to upsert")
		return
	}

	if err := s.replaceRows(ctx, rows); err != nil {
		log.Printf("[PoolSync] replace rows failed: %v", err)
		return
	}
	if err := s.cleanupExpired(ctx); err != nil {
		log.Printf("[PoolSync] cleanup failed: %v", err)
	}

	log.Printf("[PoolSync] synced %d pools in %s", len(rows), time.Since(start).String())
}

func (s *Service) fetchSnapshot(ctx context.Context) (*PoolMTopFeesResponse, error) {
	chain := "bsc"
	if config.AppConfig != nil {
		if v := strings.ToLower(strings.TrimSpace(config.AppConfig.PoolsSyncChain)); v != "" {
			chain = v
		}
	}
	dexes := poolSyncConfiguredDexes()

	sourceManager := s.sources
	if sourceManager == nil {
		sourceManager = DefaultPoolDataSourceManager()
	}
	candidates := sourceManager.CandidateSources(ctx, chain, 5)
	var lastErr error
	for _, source := range candidates {
		start := time.Now()
		snapshot, err := fetchSnapshotFromPoolDataSource(ctx, source, chain, dexes)
		latency := time.Since(start)
		if err != nil {
			lastErr = err
			sourceManager.RecordFailure(ctx, source, latency, err)
			log.Printf("[PoolSync] data source failed name=%s type=%s env=%v err=%v", source.Name, source.SourceType, source.IsEnvFallback, err)
			continue
		}
		if snapshot == nil || len(snapshot.Data) == 0 {
			err = fmt.Errorf("pool data source returned no pools")
			lastErr = err
			sourceManager.RecordFailure(ctx, source, latency, err)
			continue
		}
		annotateSnapshotSource(snapshot, source)
		sourceManager.RecordSuccess(ctx, source, latency, poolDataSourceFieldCoverage(snapshot))
		return snapshot, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no pool data source available")
}

func poolSyncConfiguredDexes() []string {
	if config.AppConfig != nil {
		if v := poolSyncDexList(config.AppConfig.PoolsSyncDexes); len(v) > 0 {
			return v
		}
	}
	return []string{"pcsv3", "univ3", "univ4"}
}

func fetchSnapshotFromPoolDataSource(ctx context.Context, source PoolDataSourceConfig, chain string, dexes []string) (*PoolMTopFeesResponse, error) {
	switch NormalizePoolDataSourceType(source.SourceType) {
	case PoolDataSourceTypePoolMTopFees:
		return NewPoolMClient(source.BaseURL).TopFees(ctx, positiveOrDefault(source.TimeframeMinutes, 5), chain, strings.Join(poolMSourceDexes(source, dexes), ","))
	case PoolDataSourceTypeMarketPools:
		return NewMarketPoolsClient(source.BaseURL).Pools(ctx, source, chain, dexes)
	default:
		return nil, fmt.Errorf("unsupported pool data source type=%s", source.SourceType)
	}
}

func poolMSourceDexes(source PoolDataSourceConfig, fallback []string) []string {
	if len(source.Dexes) > 0 {
		return source.Dexes
	}
	return fallback
}

func (s *Service) buildRows(snapshot *PoolMTopFeesResponse, updatedAt time.Time) ([]models.Pool, error) {
	if snapshot == nil {
		return nil, nil
	}
	rows := make([]models.Pool, 0, len(snapshot.Data))
	var firstErr error
	for _, item := range snapshot.Data {
		row, err := s.buildRow(snapshot, item, updatedAt)
		if err != nil {
			log.Printf("[PoolSync] build row warning: %v", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if row != nil {
			rows = append(rows, *row)
		}
	}
	if len(rows) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return rows, nil
}

func (s *Service) buildRow(snapshot *PoolMTopFeesResponse, item PoolMFeePool, updatedAt time.Time) (*models.Pool, error) {
	addr := normalizePairAddress(item.PoolAddress)
	if addr == "" && strings.EqualFold(normalizePoolMProtocolVersion(item, item.PoolID), "v4") {
		addr = normalizePairAddress(item.PoolID)
	}
	if addr == "" {
		return nil, fmt.Errorf("empty pool address")
	}

	protocolVersion := normalizePoolMProtocolVersion(item, addr)
	if protocolVersion == "" {
		return nil, fmt.Errorf("unknown protocol version for pool %s", addr)
	}

	sourceRequestedLimit := snapshot.RequestedLimit
	if sourceRequestedLimit <= 0 {
		sourceRequestedLimit = len(snapshot.Data)
	}

	priceChange := metricTrendPriceChange(item.MetricTrends)
	name := strings.TrimSpace(item.TradingPair)
	if name == "" {
		name = addr
	}
	dexName := strings.TrimSpace(firstNonEmpty(item.FactoryName, item.Dex))

	row := models.Pool{
		ID:        addr,
		Type:      "pool",
		Address:   addr,
		UpdatedAt: updatedAt,

		Name:         name,
		BaseTokenID:  normalizePairAddress(item.Token0Address),
		QuoteTokenID: normalizePairAddress(item.Token1Address),
		DexID:        dexName,

		BaseTokenPriceUSD: item.CurrentTokenPrice,
		FDVUSD:            item.CurrentTokenFDVUSD,
		MarketCapUSD:      item.CurrentTokenFDVUSD,
		ReserveInUSD:      item.CurrentPoolValue,
		PriceChangeM5:     priceChange,
		PoolFeePercentage: item.FeePercentage,
		VolumeM5:          item.TotalVolume,
		FeeUSDM5:          item.TotalFees,
		FeeAPRM5:          calcFeeAPR(item.TotalFees, item.CurrentPoolValue, 5*time.Minute),

		Chain:                       normalizeLower(firstNonEmpty(item.Chain, snapshot.RequestedChain)),
		ProtocolVersion:             protocolVersion,
		FactoryName:                 strings.TrimSpace(item.FactoryName),
		FactoryAddress:              normalizePairAddress(item.FactoryAddress),
		Token0Symbol:                strings.TrimSpace(item.Token0Symbol),
		Token1Symbol:                strings.TrimSpace(item.Token1Symbol),
		Token0Name:                  strings.TrimSpace(item.Token0Name),
		Token1Name:                  strings.TrimSpace(item.Token1Name),
		Token0Decimals:              item.Token0Decimals,
		Token1Decimals:              item.Token1Decimals,
		StableCoinSymbol:            strings.TrimSpace(item.StableCoinSymbol),
		PoolMFeeRate:                item.FeeRate,
		HookAddress:                 normalizePairAddress(item.HookAddress),
		TransactionCount:            uint32(nonNegativeInt(item.TransactionCount)),
		TotalFees:                   item.TotalFees,
		TotalVolume:                 item.TotalVolume,
		CurrentPoolValue:            item.CurrentPoolValue,
		CurrentToken0Balance:        item.CurrentToken0Balance,
		CurrentToken1Balance:        item.CurrentToken1Balance,
		CurrentTokenPrice:           item.CurrentTokenPrice,
		PricedTokenAddress:          normalizePairAddress(item.PricedTokenAddress),
		CurrentTokenTotalSupply:     item.CurrentTokenTotalSupply,
		CurrentTokenFDVUSD:          item.CurrentTokenFDVUSD,
		TokenSupplyUpdatedAt:        parsePoolMTime(item.TokenSupplyUpdatedAt),
		PriceDisplay:                strings.TrimSpace(item.PriceDisplay),
		LastSwapAt:                  parsePoolMTime(item.LastSwapAt),
		TickSpacing:                 copyIntPtr(item.TickSpacing),
		CurrentTick:                 item.CurrentTick,
		CurrentSqrtPriceX96:         strings.TrimSpace(item.CurrentSqrtPriceX96),
		CurrentLiquidity:            strings.TrimSpace(item.CurrentLiquidity),
		StableCoinPosition:          strings.TrimSpace(item.StableCoinPosition),
		UniqueWallets:               uint32(nonNegativeInt(item.UniqueWallets)),
		TopWalletVolPct:             item.TopWalletVolPct,
		ActiveTickCount:             item.ActiveTickCount,
		ActiveLiquidityUSD:          item.ActiveLiquidityUSD,
		ActiveLiquidityRatio:        item.ActiveLiquidityRatio,
		LiquidityCurrentTick:        item.LiquidityCurrentTick,
		LiquidityTickSpacing:        item.LiquidityTickSpacing,
		SourceTimeframe:             strings.TrimSpace(snapshot.Timeframe),
		SourceRequestedLimit:        sourceRequestedLimit,
		SourceRequestedChain:        normalizeLower(snapshot.RequestedChain),
		SourceTotalPools:            snapshot.TotalPools,
		SourceRequestedProtocolJSON: marshalJSONString(snapshot.RequestedProtocol, "[]"),
		SourceRequestedDexJSON:      marshalJSONString(snapshot.RequestedDex, "[]"),
		MetricTrendsIndexJSON:       jsonText(snapshot.MetricTrendsIndex, "[]"),
		LiquidityTicksIndexJSON:     jsonText(snapshot.LiquidityTicksIndex, "[]"),
		MetricTrendsJSON:            jsonText(item.MetricTrends, "[]"),
		LiquidityTicksJSON:          jsonText(item.LiquidityTicks, "[]"),
		BadgesJSON:                  jsonText(item.Badges, "[]"),
		SourcePayloadJSON:           marshalJSONString(item, "{}"),
		PoolDataSourceID:            snapshot.PoolDataSourceID,
		PoolDataSourceName:          strings.TrimSpace(snapshot.PoolDataSourceName),
		PoolDataSourceType:          strings.TrimSpace(snapshot.PoolDataSourceType),
		PoolDataSourceURL:           strings.TrimSpace(snapshot.PoolDataSourceURL),
	}

	return &row, nil
}

func parsePoolMTime(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, raw); err == nil {
			out := ts.UTC()
			return &out
		}
	}
	return nil
}

func jsonText(raw json.RawMessage, fallback string) string {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return fallback
	}
	return text
}

func marshalJSONString(value any, fallback string) string {
	b, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	text := strings.TrimSpace(string(b))
	if text == "" {
		return fallback
	}
	return text
}

func copyIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}

func metricTrendPriceChange(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	var rows [][]float64
	if err := json.Unmarshal(raw, &rows); err != nil {
		return 0
	}
	var first float64
	var last float64
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

func nonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
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

func normalizeLower(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s *Service) replaceRows(ctx context.Context, rows []models.Pool) error {
	if len(rows) == 0 {
		return nil
	}
	if database.DB == nil {
		return fmt.Errorf("database not initialized")
	}
	scopes, err := replacementScopesFromRows(rows)
	if err != nil {
		return err
	}
	if len(scopes) == 0 {
		return fmt.Errorf("pool replacement scope is empty")
	}
	if err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				UpdateAll: true,
			}).
			CreateInBatches(rows, 100).Error; err != nil {
			return err
		}
		for _, scope := range scopes {
			if err := tx.
				Where("chain = ? AND id NOT IN ?", scope.chain, scope.ids).
				Delete(&models.Pool{}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	invalidatePoolCatalogCache(ctx, replacementScopeChains(scopes))
	return nil
}

func (s *Service) cleanupExpired(ctx context.Context) error {
	retention := defaultRetention
	if config.AppConfig != nil && config.AppConfig.PoolsRetentionHours > 0 {
		retention = time.Duration(config.AppConfig.PoolsRetentionHours) * time.Hour
	}
	cutoff := time.Now().Add(-retention)
	return database.DB.WithContext(ctx).
		Where("updated_at < ?", cutoff).
		Delete(&models.Pool{}).Error
}

type poolReplacementScope struct {
	chain string
	ids   []string
}

func replacementScopesFromRows(rows []models.Pool) ([]poolReplacementScope, error) {
	idsByChain := make(map[string]map[string]struct{})
	for _, row := range rows {
		chain := strings.TrimSpace(row.Chain)
		if chain == "" {
			return nil, fmt.Errorf("pool replacement row missing chain id=%s", row.ID)
		}
		id := strings.TrimSpace(row.ID)
		if id == "" {
			return nil, fmt.Errorf("pool replacement row missing id chain=%s", chain)
		}
		if idsByChain[chain] == nil {
			idsByChain[chain] = make(map[string]struct{})
		}
		idsByChain[chain][id] = struct{}{}
	}
	chains := make([]string, 0, len(idsByChain))
	for chain := range idsByChain {
		chains = append(chains, chain)
	}
	sort.Strings(chains)

	scopes := make([]poolReplacementScope, 0, len(chains))
	for _, chain := range chains {
		ids := make([]string, 0, len(idsByChain[chain]))
		for id := range idsByChain[chain] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		scopes = append(scopes, poolReplacementScope{chain: chain, ids: ids})
	}
	return scopes, nil
}

func replacementScopeChains(scopes []poolReplacementScope) []string {
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if strings.TrimSpace(scope.chain) == "" {
			continue
		}
		out = append(out, scope.chain)
	}
	return out
}

func invalidatePoolCatalogCache(ctx context.Context, chains []string) {
	if database.RedisClient == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	seen := make(map[string]struct{}, len(chains))
	for _, chain := range chains {
		chain = strings.TrimSpace(chain)
		if chain == "" {
			continue
		}
		if _, ok := seen[chain]; ok {
			continue
		}
		seen[chain] = struct{}{}
		pattern := fmt.Sprintf("pools:catalog:v6:chain=%s:*", chain)
		iter := database.RedisClient.Scan(ctx, 0, pattern, 100).Iterator()
		keys := make([]string, 0, 16)
		for iter.Next(ctx) {
			keys = append(keys, iter.Val())
			if len(keys) >= 100 {
				if err := database.RedisClient.Del(ctx, keys...).Err(); err != nil {
					log.Printf("[PoolSync] invalidate pool cache failed chain=%s err=%v", chain, err)
					return
				}
				keys = keys[:0]
			}
		}
		if err := iter.Err(); err != nil {
			log.Printf("[PoolSync] scan pool cache failed chain=%s err=%v", chain, err)
			continue
		}
		if len(keys) > 0 {
			if err := database.RedisClient.Del(ctx, keys...).Err(); err != nil {
				log.Printf("[PoolSync] invalidate pool cache failed chain=%s err=%v", chain, err)
			}
		}
	}
}

func poolSyncDexList(raw string) []string {
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

func splitCSVLower(raw string) []string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(raw)), ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
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

	addr := normalizePairAddress(poolAddr)
	switch len(strings.TrimPrefix(addr, "0x")) {
	case 64:
		return "v4"
	case 40:
		return "v3"
	default:
		return ""
	}
}

func calcFeeAPR(feeUSD float64, reserveUSD float64, window time.Duration) float64 {
	if feeUSD <= 0 || reserveUSD <= 0 || window <= 0 {
		return 0
	}
	annualized := feeUSD * (365 * 24 * float64(time.Hour)) / float64(window)
	return annualized / reserveUSD * 100.0
}
