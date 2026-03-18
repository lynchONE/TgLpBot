package pool_sync

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/pool"
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm/clause"
)

const (
	defaultSyncInterval = 60 * time.Second
	defaultFetchDelay   = 250 * time.Millisecond
	defaultRetention    = 24 * time.Hour
)

type Service struct {
	poolm       *PoolMClient
	dexScreener *DexScreenerClient
	poolService *pool.PoolService

	stopCh   chan struct{}
	stopOnce sync.Once
	ticker   *time.Ticker
}

type poolKey struct {
	proto string
	addr  string
}

type poolCandidate struct {
	Key       poolKey
	ByWindow  map[int]PoolMFeePool
	Best      PoolMFeePool
	BestScore float64
}

func NewService() *Service {
	baseURL := ""
	if config.AppConfig != nil {
		baseURL = config.AppConfig.PoolsSyncPoolMBaseURL
	}
	return &Service{
		poolm:       NewPoolMClient(baseURL),
		dexScreener: NewDexScreenerClient(""),
		poolService: pool.NewPoolService(),
		stopCh:      make(chan struct{}),
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
	candidates, err := s.fetchCandidates(ctx)
	if err != nil {
		log.Printf("[PoolSync] fetch candidates failed: %v", err)
		return
	}
	if len(candidates) == 0 {
		log.Printf("[PoolSync] no pools fetched")
		return
	}

	rows, err := s.buildRows(ctx, candidates)
	if err != nil {
		log.Printf("[PoolSync] build rows failed: %v", err)
		return
	}
	if len(rows) == 0 {
		log.Printf("[PoolSync] no rows to upsert")
		return
	}

	if err := s.upsertRows(ctx, rows); err != nil {
		log.Printf("[PoolSync] upsert failed: %v", err)
		return
	}
	if err := s.cleanupExpired(ctx); err != nil {
		log.Printf("[PoolSync] cleanup failed: %v", err)
	}

	log.Printf("[PoolSync] synced %d pools in %s", len(rows), time.Since(start).String())
}

func (s *Service) fetchCandidates(ctx context.Context) ([]poolCandidate, error) {
	chain := "bsc"
	dexes := []string{"pcsv3", "univ3", "univ4"}
	delay := defaultFetchDelay
	if config.AppConfig != nil {
		if v := strings.ToLower(strings.TrimSpace(config.AppConfig.PoolsSyncChain)); v != "" {
			chain = v
		}
		if v := poolSyncDexList(config.AppConfig.PoolsSyncDexes); len(v) > 0 {
			dexes = v
		}
		if config.AppConfig.PoolsSyncFetchDelayMillis > 0 {
			delay = time.Duration(config.AppConfig.PoolsSyncFetchDelayMillis) * time.Millisecond
		}
	}

	timeframes := []int{5, 60}
	dexParam := strings.Join(dexes, ",")
	candidates := make(map[poolKey]*poolCandidate)

	for _, tf := range timeframes {
		resp, err := s.poolm.TopFees(ctx, tf, chain, dexParam)
		if err != nil {
			return nil, err
		}

		for _, p := range resp.Data {
			addr := normalizePairAddress(p.PoolAddress)
			if addr == "" {
				continue
			}
			proto := normalizePoolMProtocolVersion(p, addr)
			if proto == "" {
				continue
			}

			key := poolKey{proto: proto, addr: addr}
			candidate, ok := candidates[key]
			if !ok {
				candidate = &poolCandidate{
					Key:      key,
					ByWindow: make(map[int]PoolMFeePool),
				}
				candidates[key] = candidate
			}
			candidate.ByWindow[tf] = p
			score := candidateScore(p)
			if score >= candidate.BestScore {
				candidate.Best = p
				candidate.BestScore = score
			}
		}

		if delay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	out := make([]poolCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		out = append(out, *candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BestScore > out[j].BestScore
	})
	return out, nil
}

func (s *Service) buildRows(ctx context.Context, candidates []poolCandidate) ([]models.Pool, error) {
	chain := "bsc"
	if config.AppConfig != nil && strings.TrimSpace(config.AppConfig.PoolsSyncChain) != "" {
		chain = strings.ToLower(strings.TrimSpace(config.AppConfig.PoolsSyncChain))
	}

	workers := 8
	if len(candidates) < workers {
		workers = len(candidates)
	}
	if workers < 1 {
		return nil, nil
	}

	type result struct {
		row *models.Pool
		err error
	}

	jobs := make(chan poolCandidate)
	results := make(chan result, len(candidates))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for candidate := range jobs {
				row, err := s.buildRow(ctx, chain, candidate)
				results <- result{row: row, err: err}
			}
		}()
	}

	for _, candidate := range candidates {
		jobs <- candidate
	}
	close(jobs)
	wg.Wait()
	close(results)

	rows := make([]models.Pool, 0, len(candidates))
	var firstErr error
	for result := range results {
		if result.err != nil {
			log.Printf("[PoolSync] build row warning: %v", result.err)
			if firstErr == nil {
				firstErr = result.err
			}
			continue
		}
		if result.row != nil {
			rows = append(rows, *result.row)
		}
	}
	if len(rows) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return rows, nil
}

func (s *Service) buildRow(ctx context.Context, chain string, candidate poolCandidate) (*models.Pool, error) {
	best := candidate.Best
	if strings.TrimSpace(best.PoolAddress) == "" {
		return nil, fmt.Errorf("empty pool address")
	}

	var (
		pair     *DexScreenerPair
		poolInfo *pool.PoolInfo
	)

	pairCtx, pairCancel := context.WithTimeout(ctx, 12*time.Second)
	defer pairCancel()
	pair, _ = s.dexScreener.GetPair(pairCtx, chain, candidate.Key.addr)

	_, infoCancel := context.WithTimeout(ctx, 12*time.Second)
	defer infoCancel()
	switch candidate.Key.proto {
	case "v4":
		if strings.EqualFold(chain, "bsc") {
			poolInfo, _ = s.poolService.GetV4PoolInfo(candidate.Key.addr)
		}
	default:
		poolInfo, _ = s.poolService.GetPoolInfoForChain(chain, candidate.Key.addr)
	}

	row := models.Pool{
		ID:        candidate.Key.addr,
		Type:      "pool",
		Address:   candidate.Key.addr,
		Name:      strings.TrimSpace(best.TradingPair),
		UpdatedAt: time.Now(),
	}

	if row.Name == "" && poolInfo != nil {
		row.Name = strings.TrimSpace(poolInfo.Token0Symbol + "/" + poolInfo.Token1Symbol)
	}
	if row.Name == "" && pair != nil {
		row.Name = strings.TrimSpace(pair.BaseToken.Symbol + "/" + pair.QuoteToken.Symbol)
	}
	if row.Name == "" {
		row.Name = candidate.Key.addr
	}

	row.DexID = strings.TrimSpace(best.Dex)
	if pair != nil && strings.TrimSpace(pair.DexID) != "" {
		row.DexID = strings.TrimSpace(pair.DexID)
	} else if poolInfo != nil && strings.TrimSpace(poolInfo.Exchange) != "" {
		row.DexID = strings.TrimSpace(poolInfo.Exchange)
	}

	row.BaseTokenID = normalizePairAddress(best.Token0Address)
	row.QuoteTokenID = normalizePairAddress(best.Token1Address)
	if pair != nil {
		if v := normalizePairAddress(pair.BaseToken.Address); v != "" {
			row.BaseTokenID = v
		}
		if v := normalizePairAddress(pair.QuoteToken.Address); v != "" {
			row.QuoteTokenID = v
		}
	}
	if poolInfo != nil {
		if v := normalizePairAddress(poolInfo.Token0); v != "" {
			row.BaseTokenID = v
		}
		if v := normalizePairAddress(poolInfo.Token1); v != "" {
			row.QuoteTokenID = v
		}
	}

	row.BaseTokenPriceUSD = parseFloatString(pairValue(pair, func(p *DexScreenerPair) string { return p.PriceUSD }))
	if row.BaseTokenPriceUSD <= 0 {
		row.BaseTokenPriceUSD = best.CurrentTokenPrice
	}
	row.BaseTokenPriceNativeCurrency = parseFloatString(pairValue(pair, func(p *DexScreenerPair) string { return p.PriceNative }))
	row.QuoteTokenPriceUSD = 0
	row.QuoteTokenPriceNativeCurrency = 0
	row.BaseTokenPriceQuoteToken = 0
	row.QuoteTokenPriceBaseToken = 0

	if pair != nil {
		if pair.PairCreatedAt > 0 {
			t := time.UnixMilli(pair.PairCreatedAt).UTC()
			row.PoolCreatedAt = &t
		}
		row.FDVUSD = pair.FDV
		row.MarketCapUSD = pair.MarketCap
		row.ReserveInUSD = pair.Liquidity.USD
		row.PriceChangeM5 = pair.PriceChange.M5
		row.PriceChangeH1 = pair.PriceChange.H1
		row.PriceChangeH6 = pair.PriceChange.H6
		row.PriceChangeH24 = pair.PriceChange.H24
		row.VolumeM5 = pair.Volume.M5
		row.VolumeH1 = pair.Volume.H1
		row.VolumeH6 = pair.Volume.H6
		row.VolumeH24 = pair.Volume.H24
		row.TransactionsH24Buys = uint32(maxInt(pair.Txns.H24.Buys, 0))
		row.TransactionsH24Sells = uint32(maxInt(pair.Txns.H24.Sells, 0))
		row.TransactionsH24Buyers = uint32(maxInt(pair.Txns.H24.Buyers, 0))
		row.TransactionsH24Sellers = uint32(maxInt(pair.Txns.H24.Sellers, 0))
	}

	if row.ReserveInUSD <= 0 {
		row.ReserveInUSD = best.CurrentPoolValue
	}
	if row.VolumeM5 <= 0 {
		if p, ok := candidate.ByWindow[5]; ok {
			row.VolumeM5 = p.TotalVolume
		}
	}
	if row.VolumeH1 <= 0 {
		if p, ok := candidate.ByWindow[60]; ok {
			row.VolumeH1 = p.TotalVolume
		}
	}

	row.PoolFeePercentage = best.FeePercentage
	if row.PoolFeePercentage <= 0 && poolInfo != nil && poolInfo.Fee > 0 {
		row.PoolFeePercentage = float64(poolInfo.Fee) / 10000.0
	}
	if row.PoolFeePercentage < 0 {
		row.PoolFeePercentage = 0
	}

	if p, ok := candidate.ByWindow[5]; ok {
		row.FeeUSDM5 = p.TotalFees
	}
	if p, ok := candidate.ByWindow[60]; ok {
		row.FeeUSDH1 = p.TotalFees
	}
	if row.FeeUSDM5 <= 0 {
		row.FeeUSDM5 = calcFeeUSD(row.VolumeM5, row.PoolFeePercentage)
	}
	if row.FeeUSDH1 <= 0 {
		row.FeeUSDH1 = calcFeeUSD(row.VolumeH1, row.PoolFeePercentage)
	}
	row.FeeUSDH6 = calcFeeUSD(row.VolumeH6, row.PoolFeePercentage)
	row.FeeUSDH24 = calcFeeUSD(row.VolumeH24, row.PoolFeePercentage)
	row.FeeAPRM5 = calcFeeAPR(row.FeeUSDM5, row.ReserveInUSD, 5*time.Minute)
	row.FeeAPRH1 = calcFeeAPR(row.FeeUSDH1, row.ReserveInUSD, time.Hour)
	row.FeeAPRH6 = calcFeeAPR(row.FeeUSDH6, row.ReserveInUSD, 6*time.Hour)
	row.FeeAPRH24 = calcFeeAPR(row.FeeUSDH24, row.ReserveInUSD, 24*time.Hour)

	return &row, nil
}

func (s *Service) upsertRows(ctx context.Context, rows []models.Pool) error {
	return database.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			UpdateAll: true,
		}).
		CreateInBatches(rows, 100).Error
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

func candidateScore(p PoolMFeePool) float64 {
	return p.TotalFees + p.TotalVolume + p.CurrentPoolValue + float64(maxInt(p.TransactionCount, 0))
}

func calcFeeUSD(volume float64, feePct float64) float64 {
	if volume <= 0 || feePct <= 0 {
		return 0
	}
	return volume * feePct / 100.0
}

func calcFeeAPR(feeUSD float64, reserveUSD float64, window time.Duration) float64 {
	if feeUSD <= 0 || reserveUSD <= 0 || window <= 0 {
		return 0
	}
	annualized := feeUSD * (365 * 24 * float64(time.Hour)) / float64(window)
	return annualized / reserveUSD * 100.0
}

func parseFloatString(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return v
}

func pairValue(pair *DexScreenerPair, pick func(*DexScreenerPair) string) string {
	if pair == nil || pick == nil {
		return ""
	}
	return pick(pair)
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
