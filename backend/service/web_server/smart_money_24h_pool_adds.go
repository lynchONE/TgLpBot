package web_server

import (
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

type smartMoney24hPool struct {
	PoolVersion    string  `json:"pool_version"`
	PoolID         string  `json:"pool_id"`
	Pair           string  `json:"pair,omitempty"`
	FeePct         float64 `json:"fee_pct,omitempty"`
	Token0Symbol   string  `json:"token0_symbol,omitempty"`
	Token1Symbol   string  `json:"token1_symbol,omitempty"`
	Exchange       string  `json:"exchange,omitempty"`
	EventCount     int     `json:"event_count"`
	WalletCount    int     `json:"wallet_count"`
	TotalAmountUSD float64 `json:"total_amount_usd,omitempty"`
	FirstAddAt     string  `json:"first_add_at"`
	LastAddAt      string  `json:"last_add_at"`
}

type smartMoney24hHourlyTrend struct {
	Hour          string `json:"hour"`
	AddCount      int    `json:"add_count"`
	WalletCount   int    `json:"wallet_count"`
	DistinctPools int    `json:"distinct_pools"`
}

type smartMoney24hDistBucket struct {
	Range string `json:"range"`
	Count int    `json:"count"`
}

type smartMoney24hTopWallet struct {
	WalletAddress string `json:"wallet_address"`
	PoolCount     int    `json:"pool_count"`
	AddCount      int    `json:"add_count"`
}

type smartMoney24hResponse struct {
	Chain                  string                     `json:"chain"`
	WindowHours            int                        `json:"window_hours"`
	UpdatedAt              time.Time                  `json:"updated_at"`
	TotalPools             int                        `json:"total_pools"`
	TotalWallets           int                        `json:"total_wallets"`
	TotalEvents            int                        `json:"total_events"`
	TotalAmountUSD         float64                    `json:"total_amount_usd,omitempty"`
	Pools                  []smartMoney24hPool        `json:"pools"`
	HourlyTrend            []smartMoney24hHourlyTrend `json:"hourly_trend"`
	TickRangeDistribution  []smartMoney24hDistBucket  `json:"tick_range_distribution"`
	PoolAmountDistribution []smartMoney24hDistBucket  `json:"pool_amount_distribution,omitempty"`
	WalletPoolDistribution []smartMoney24hDistBucket  `json:"wallet_pool_distribution,omitempty"`
	TopWallets             []smartMoney24hTopWallet   `json:"top_wallets,omitempty"`
	Warnings               []string                   `json:"warnings,omitempty"`
}

type smartMoney24hPoolAmountRow struct {
	PoolVersion string
	PoolID      string
	Sum0        string
	Sum1        string
}

type smartMoney24hTopWalletRow struct {
	WalletAddress string
	PoolCount     uint64
	AddCount      uint64
}

func clampInt24h(value int, min int, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func usdBucketLabel(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		return "<= $1k"
	}
	switch {
	case v <= 1_000:
		return "<= $1k"
	case v <= 5_000:
		return "$1k-$5k"
	case v <= 20_000:
		return "$5k-$20k"
	case v <= 100_000:
		return "$20k-$100k"
	case v <= 500_000:
		return "$100k-$500k"
	default:
		return ">= $500k"
	}
}

func orderedBuckets(counts map[string]int, order []string) []smartMoney24hDistBucket {
	out := make([]smartMoney24hDistBucket, 0, len(order))
	for _, label := range order {
		out = append(out, smartMoney24hDistBucket{
			Range: label,
			Count: counts[label],
		})
	}
	return out
}

type smartMoney24hPoolInfoCacheEntry struct {
	Info      *pool.PoolInfo
	ExpiresAt time.Time
}

var smartMoney24hPoolInfoCache sync.Map

func clonePoolInfo(info *pool.PoolInfo) *pool.PoolInfo {
	if info == nil {
		return nil
	}
	c := *info
	return &c
}

func loadSmartMoney24hPoolInfoCache(key string) *pool.PoolInfo {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return nil
	}
	raw, ok := smartMoney24hPoolInfoCache.Load(key)
	if !ok {
		return nil
	}
	entry, ok := raw.(smartMoney24hPoolInfoCacheEntry)
	if !ok {
		smartMoney24hPoolInfoCache.Delete(key)
		return nil
	}
	if entry.Info == nil || time.Now().After(entry.ExpiresAt) {
		smartMoney24hPoolInfoCache.Delete(key)
		return nil
	}
	return clonePoolInfo(entry.Info)
}

func storeSmartMoney24hPoolInfoCache(key string, info *pool.PoolInfo, ttl time.Duration) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" || info == nil {
		return
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	smartMoney24hPoolInfoCache.Store(key, smartMoney24hPoolInfoCacheEntry{
		Info:      clonePoolInfo(info),
		ExpiresAt: time.Now().Add(ttl),
	})
}

func applyPoolInfoTo24hPool(p *smartMoney24hPool, info *pool.PoolInfo) {
	if p == nil || info == nil {
		return
	}
	p.Exchange = strings.TrimSpace(info.Exchange)
	p.Token0Symbol = strings.TrimSpace(info.Token0Symbol)
	p.Token1Symbol = strings.TrimSpace(info.Token1Symbol)
	if p.Token0Symbol != "" || p.Token1Symbol != "" {
		p.Pair = strings.TrimSpace(p.Token0Symbol + "/" + p.Token1Symbol)
		if p.Pair == "/" {
			p.Pair = ""
		}
	}
	if info.Fee > 0 {
		p.FeePct = float64(info.Fee) / 10000.0
	}
}

func fetchPoolInfoBestEffort(
	ctx context.Context,
	poolSvc *pool.PoolService,
	chain string,
	poolVersion string,
	poolID string,
	timeout time.Duration,
) (*pool.PoolInfo, error) {
	if poolSvc == nil {
		return nil, fmt.Errorf("pool service is nil")
	}
	type result struct {
		info *pool.PoolInfo
		err  error
	}
	outCh := make(chan result, 1)
	go func() {
		pv := strings.ToLower(strings.TrimSpace(poolVersion))
		if pv == "v4" {
			info, err := poolSvc.GetV4PoolInfo(poolID)
			outCh <- result{info: info, err: err}
			return
		}
		info, err := poolSvc.GetPoolInfoForChain(chain, poolID)
		outCh <- result{info: info, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, context.DeadlineExceeded
	case r := <-outCh:
		return r.info, r.err
	}
}

func getUSDPricesBestEffort(
	ctx context.Context,
	priceSvc *pricing.TokenPriceService,
	chain string,
	tokens []string,
	timeout time.Duration,
) (map[string]float64, error) {
	if priceSvc == nil {
		return map[string]float64{}, fmt.Errorf("price service is nil")
	}
	if len(tokens) == 0 {
		return map[string]float64{}, nil
	}

	type result struct {
		prices map[string]float64
		err    error
	}
	outCh := make(chan result, 1)
	go func() {
		prices, err := priceSvc.GetUSDPrices(chain, tokens)
		outCh <- result{prices: prices, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return map[string]float64{}, ctx.Err()
	case <-timer.C:
		return map[string]float64{}, context.DeadlineExceeded
	case r := <-outCh:
		if r.prices == nil {
			return map[string]float64{}, r.err
		}
		return r.prices, r.err
	}
}

func (s *Server) handleSmartMoney24hPoolAdds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ClickHouse == nil || s.ClickHouse.Conn == nil {
		http.Error(w, "ClickHouse not configured", http.StatusServiceUnavailable)
		return
	}

	initData := initDataFromQuery(r)
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
	if status, msg := requireSmartMoneyPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	query := r.URL.Query()
	chain := strings.ToLower(strings.TrimSpace(query.Get("chain")))
	if chain == "" {
		chain = "bsc"
	}
	windowHours := 24
	if v := strings.TrimSpace(query.Get("window_hours")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 168 {
			windowHours = n
		}
	}
	poolLimit := 30
	if v := strings.TrimSpace(query.Get("pool_limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 100 {
			poolLimit = n
		}
	}
	topWalletLimit := 20
	if v := strings.TrimSpace(query.Get("top_wallet_limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			topWalletLimit = clampInt24h(n, 1, 100)
		}
	}
	watchedOnly := true
	if v := strings.ToLower(strings.TrimSpace(query.Get("watched_only"))); v != "" {
		if v == "0" || v == "false" || v == "no" {
			watchedOnly = false
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	windowSec := windowHours * 3600
	warnings := make([]string, 0, 4)
	watchedWalletFilterSQL := ""
	watchedFilterArgCount := 0
	if watchedOnly {
		var watchTableCnt uint64
		tableErr := s.ClickHouse.Conn.QueryRow(ctx, `
			SELECT count()
			FROM system.tables
			WHERE database = currentDatabase()
				AND name = 'smart_lp_watched_wallets'
		`).Scan(&watchTableCnt)
		if tableErr == nil && watchTableCnt > 0 {
			watchedWalletFilterSQL = `
				AND wallet_address IN (
					SELECT wallet_address
					FROM (
						SELECT wallet_address
						FROM (
							SELECT wallet_address, argMax(source, updated_at) AS latest_source
							FROM smart_lp_watched_wallets
							WHERE lowerUTF8(chain) = ?
							GROUP BY wallet_address
						)
						WHERE latest_source != 'user_removed'
						UNION DISTINCT
						SELECT wallet_address
						FROM smart_lp_events
						WHERE ts >= now() - INTERVAL 15 DAY
							AND action = 'add'
							AND lowerUTF8(chain) = ?
							AND wallet_address != ''
						GROUP BY wallet_address
					)
				)
			`
			watchedFilterArgCount = 2
		} else {
			watchedWalletFilterSQL = `
				AND wallet_address IN (
					SELECT wallet_address
					FROM smart_lp_events
					WHERE ts >= now() - INTERVAL 15 DAY
						AND action = 'add'
						AND lowerUTF8(chain) = ?
						AND wallet_address != ''
					GROUP BY wallet_address
				)
			`
			watchedFilterArgCount = 1
			if tableErr != nil {
				warnings = append(warnings, fmt.Sprintf("监控钱包表检查失败，已降级为事件发现钱包: %v", tableErr))
			} else {
				warnings = append(warnings, "未找到 smart_lp_watched_wallets 表，已降级为事件发现钱包")
			}
		}
	}
	buildWatchedArgs := func() []any {
		args := make([]any, 0, 1+watchedFilterArgCount)
		args = append(args, chain)
		for i := 0; i < watchedFilterArgCount; i++ {
			args = append(args, chain)
		}
		return args
	}
	activeNetLiquidityExpr := "sum(if(pool_version = 'v4', toInt256OrZero(liquidity_delta), if(action = 'add', toInt256OrZero(liquidity_delta), -toInt256OrZero(liquidity_delta))))"
	activePositionsSubquery := fmt.Sprintf(`
		SELECT
			pool_version,
			pool_id,
			wallet_address,
			tick_lower,
			tick_upper,
			minIf(ts, action = 'add') AS first_add_at,
			maxIf(ts, action = 'add') AS last_add_at,
			countIf(action = 'add') AS add_event_count,
			toString(sumIf(toInt256OrZero(amount0), action = 'add')) AS sum0,
			toString(sumIf(toInt256OrZero(amount1), action = 'add')) AS sum1
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND action IN ('add', 'remove')
			AND lowerUTF8(chain) = ?
			%s
			AND pool_version != '' AND pool_id != ''
			AND wallet_address != ''
		GROUP BY pool_version, pool_id, wallet_address, tick_lower, tick_upper
		HAVING %s > 0
			AND add_event_count > 0
	`, windowSec, watchedWalletFilterSQL, activeNetLiquidityExpr)

	// 1. Query top pools by wallet count in window for positions that still exist.
	poolQuery := fmt.Sprintf(`
		SELECT
			pool_version,
			pool_id,
			sum(add_event_count) AS event_count,
			uniqExact(wallet_address) AS wallet_count,
			min(first_add_at) AS first_add_at,
			max(last_add_at) AS last_add_at
		FROM (%s)
		GROUP BY pool_version, pool_id
		ORDER BY wallet_count DESC, event_count DESC
		LIMIT %d
	`, activePositionsSubquery, poolLimit)

	poolArgs := buildWatchedArgs()
	rows, err := s.ClickHouse.Conn.Query(ctx, poolQuery, poolArgs...)
	if err != nil {
		http.Error(w, fmt.Sprintf("池子查询失败: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	outPools := make([]smartMoney24hPool, 0, poolLimit)
	totalEvents := 0
	for rows.Next() {
		var pv, pid string
		var eventCnt, walletCnt uint64
		var firstAdd, lastAdd time.Time
		if err := rows.Scan(&pv, &pid, &eventCnt, &walletCnt, &firstAdd, &lastAdd); err != nil {
			warnings = append(warnings, fmt.Sprintf("池子结果解析失败: %v", err))
			continue
		}
		totalEvents += int(eventCnt)
		outPools = append(outPools, smartMoney24hPool{
			PoolVersion: pv,
			PoolID:      pid,
			EventCount:  int(eventCnt),
			WalletCount: int(walletCnt),
			FirstAddAt:  firstAdd.Format(time.RFC3339),
			LastAddAt:   lastAdd.Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		warnings = append(warnings, fmt.Sprintf("池子结果遍历失败: %v", err))
	}

	// Resolve pool info with bounded concurrency and soft per-pool timeout.
	poolSvc := pool.NewPoolService()
	poolIndex := make(map[string]*smartMoney24hPool, len(outPools))
	poolInfoByKey := make(map[string]*pool.PoolInfo, len(outPools))
	poolInfoCacheTTL := 20 * time.Minute
	poolInfoSoftTimeout := 1200 * time.Millisecond
	poolInfoResolveWindow := 6 * time.Second
	resolveCtx, resolveCancel := context.WithTimeout(ctx, poolInfoResolveWindow)
	defer resolveCancel()
	g, gctx := errgroup.WithContext(resolveCtx)
	g.SetLimit(6)

	var poolInfoMu sync.Mutex
	poolInfoTimeoutCnt := 0
	poolInfoErrorCnt := 0
	for i := range outPools {
		p := &outPools[i]
		pv := strings.ToLower(strings.TrimSpace(p.PoolVersion))
		pid := strings.ToLower(strings.TrimSpace(p.PoolID))
		poolIndex[pv+"|"+pid] = p
	}
	for i := range outPools {
		i := i
		g.Go(func() error {
			if gctx.Err() != nil {
				return nil
			}
			p := &outPools[i]
			pv := strings.ToLower(strings.TrimSpace(p.PoolVersion))
			pid := strings.ToLower(strings.TrimSpace(p.PoolID))
			key := pv + "|" + pid
			if key == "|" {
				return nil
			}

			info := loadSmartMoney24hPoolInfoCache(key)
			if info == nil {
				fetched, infoErr := fetchPoolInfoBestEffort(gctx, poolSvc, chain, pv, p.PoolID, poolInfoSoftTimeout)
				if infoErr != nil {
					if errors.Is(infoErr, context.DeadlineExceeded) {
						poolInfoMu.Lock()
						poolInfoTimeoutCnt++
						poolInfoMu.Unlock()
					} else if !errors.Is(infoErr, context.Canceled) {
						poolInfoMu.Lock()
						poolInfoErrorCnt++
						poolInfoMu.Unlock()
					}
					return nil
				}
				if fetched == nil {
					return nil
				}
				info = fetched
				storeSmartMoney24hPoolInfoCache(key, info, poolInfoCacheTTL)
			}

			applyPoolInfoTo24hPool(p, info)
			poolInfoMu.Lock()
			poolInfoByKey[key] = info
			poolInfoMu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	if poolInfoTimeoutCnt > 0 {
		warnings = append(warnings, fmt.Sprintf("有 %d 个池子元数据查询超时，已返回部分结果", poolInfoTimeoutCnt))
	}
	if poolInfoErrorCnt > 0 {
		warnings = append(warnings, fmt.Sprintf("有 %d 个池子元数据查询失败，已返回部分结果", poolInfoErrorCnt))
	}
	if resolveCtx.Err() != nil && !errors.Is(resolveCtx.Err(), context.Canceled) {
		warnings = append(warnings, "池子元数据查询达到时间上限，已返回部分结果")
	}

	// 1.1 Query total added token amounts for selected pools and convert to USD.
	selectedPlaceholders := make([]string, 0, len(outPools))
	selectedArgs := buildWatchedArgs()
	for i := range outPools {
		selectedPlaceholders = append(selectedPlaceholders, "(?, ?)")
		selectedArgs = append(selectedArgs, outPools[i].PoolVersion, outPools[i].PoolID)
	}

	poolAmountRows := make([]smartMoney24hPoolAmountRow, 0, len(outPools))
	if len(selectedPlaceholders) > 0 {
		poolAmountQuery := fmt.Sprintf(`
			SELECT
				pool_version,
				pool_id,
				toString(sum(toInt256OrZero(sum0))) AS sum0,
				toString(sum(toInt256OrZero(sum1))) AS sum1
			FROM (%s)
			WHERE (pool_version, pool_id) IN (%s)
			GROUP BY pool_version, pool_id
		`, activePositionsSubquery, strings.Join(selectedPlaceholders, ","))

		amountRows, amountErr := s.ClickHouse.Conn.Query(ctx, poolAmountQuery, selectedArgs...)
		if amountErr != nil {
			warnings = append(warnings, fmt.Sprintf("池子金额查询失败: %v", amountErr))
		} else {
			defer amountRows.Close()
			for amountRows.Next() {
				var row smartMoney24hPoolAmountRow
				if err := amountRows.Scan(&row.PoolVersion, &row.PoolID, &row.Sum0, &row.Sum1); err != nil {
					warnings = append(warnings, fmt.Sprintf("池子金额结果解析失败: %v", err))
					continue
				}
				row.PoolVersion = strings.ToLower(strings.TrimSpace(row.PoolVersion))
				row.PoolID = strings.ToLower(strings.TrimSpace(row.PoolID))
				poolAmountRows = append(poolAmountRows, row)
			}
			if err := amountRows.Err(); err != nil {
				warnings = append(warnings, fmt.Sprintf("池子金额结果遍历失败: %v", err))
			}
		}
	}

	priceSvc := s.TokenPrice
	if priceSvc == nil {
		priceSvc = pricing.NewTokenPriceService()
	}
	tokenSet := make(map[string]struct{})
	tokens := make([]string, 0, 2*len(outPools))
	for _, info := range poolInfoByKey {
		if info == nil {
			continue
		}
		t0 := strings.ToLower(strings.TrimSpace(info.Token0))
		t1 := strings.ToLower(strings.TrimSpace(info.Token1))
		if t0 != "" {
			if _, ok := tokenSet[t0]; !ok {
				tokenSet[t0] = struct{}{}
				tokens = append(tokens, t0)
			}
		}
		if t1 != "" {
			if _, ok := tokenSet[t1]; !ok {
				tokenSet[t1] = struct{}{}
				tokens = append(tokens, t1)
			}
		}
	}

	prices, priceErr := getUSDPricesBestEffort(ctx, priceSvc, chain, tokens, 1800*time.Millisecond)
	if priceErr != nil {
		if errors.Is(priceErr, context.DeadlineExceeded) {
			warnings = append(warnings, "价格查询超时，已使用缓存或兜底价格")
		} else {
			warnings = append(warnings, "价格服务触发限流，已使用缓存或兜底价格")
		}
	}

	decimalsCache := make(map[string]int)
	totalAmountUSD := 0.0
	amountBucketCount := map[string]int{
		"<= $1k":      0,
		"$1k-$5k":     0,
		"$5k-$20k":    0,
		"$20k-$100k":  0,
		"$100k-$500k": 0,
		">= $500k":    0,
	}
	for _, row := range poolAmountRows {
		key := row.PoolVersion + "|" + row.PoolID
		poolRow := poolIndex[key]
		if poolRow == nil {
			continue
		}
		info := poolInfoByKey[key]
		if info == nil {
			continue
		}

		t0 := strings.ToLower(strings.TrimSpace(info.Token0))
		t1 := strings.ToLower(strings.TrimSpace(info.Token1))
		dec0 := getDecimalsCached(t0, decimalsCache)
		dec1 := getDecimalsCached(t1, decimalsCache)

		amt0 := amountToFloat(row.Sum0, dec0)
		amt1 := amountToFloat(row.Sum1, dec1)
		usd := sanitizeFloat(amt0*prices[t0] + amt1*prices[t1])
		poolRow.TotalAmountUSD = usd
		totalAmountUSD += usd
		amountBucketCount[usdBucketLabel(usd)]++
	}

	sort.Slice(outPools, func(i, j int) bool {
		if outPools[i].TotalAmountUSD != outPools[j].TotalAmountUSD {
			return outPools[i].TotalAmountUSD > outPools[j].TotalAmountUSD
		}
		if outPools[i].WalletCount != outPools[j].WalletCount {
			return outPools[i].WalletCount > outPools[j].WalletCount
		}
		return outPools[i].PoolID < outPools[j].PoolID
	})

	// 2. Total unique wallets
	var totalWallets uint64
	totalWalletsQ := fmt.Sprintf(`
		SELECT uniqExact(wallet_address)
		FROM (%s)
	`, activePositionsSubquery)
	totalWalletArgs := buildWatchedArgs()
	if err := s.ClickHouse.Conn.QueryRow(ctx, totalWalletsQ, totalWalletArgs...).Scan(&totalWallets); err != nil {
		warnings = append(warnings, fmt.Sprintf("总钱包数查询失败: %v", err))
	}

	// 3. Hourly trend
	hourlyQ := fmt.Sprintf(`
		SELECT
			toStartOfHour(last_add_at) AS hour,
			sum(add_event_count) AS add_count,
			uniqExact(wallet_address) AS wallet_count,
			uniqExact(concat(pool_version, '|', pool_id)) AS distinct_pools
		FROM (%s)
		GROUP BY hour
		ORDER BY hour ASC
	`, activePositionsSubquery)

	hourlyArgs := buildWatchedArgs()
	hRows, err := s.ClickHouse.Conn.Query(ctx, hourlyQ, hourlyArgs...)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("每小时趋势查询失败: %v", err))
	}
	hourlyTrend := make([]smartMoney24hHourlyTrend, 0, 24)
	if hRows != nil {
		defer hRows.Close()
		for hRows.Next() {
			var hour time.Time
			var addCnt, walletCnt, poolCnt uint64
			if err := hRows.Scan(&hour, &addCnt, &walletCnt, &poolCnt); err != nil {
				continue
			}
			hourlyTrend = append(hourlyTrend, smartMoney24hHourlyTrend{
				Hour:          hour.Format(time.RFC3339),
				AddCount:      int(addCnt),
				WalletCount:   int(walletCnt),
				DistinctPools: int(poolCnt),
			})
		}
	}

	// 4. Tick range width distribution
	tickRangeQ := fmt.Sprintf(`
		SELECT
			multiIf(
				abs(tick_upper - tick_lower) <= 200, '极窄 (≤200 ticks)',
				abs(tick_upper - tick_lower) <= 1000, '窄 (200-1k ticks)',
				abs(tick_upper - tick_lower) <= 5000, '中等 (1k-5k ticks)',
				abs(tick_upper - tick_lower) <= 20000, '宽 (5k-20k ticks)',
				'超宽 (20k+ ticks)'
			) AS range_label,
			count() AS cnt
		FROM (%s)
		WHERE tick_lower != 0 AND tick_upper != 0
		GROUP BY range_label
		ORDER BY cnt DESC
	`, activePositionsSubquery)

	tickArgs := buildWatchedArgs()
	tRows, err := s.ClickHouse.Conn.Query(ctx, tickRangeQ, tickArgs...)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("区间分布查询失败: %v", err))
	}
	tickRangeDist := make([]smartMoney24hDistBucket, 0, 5)
	if tRows != nil {
		defer tRows.Close()
		for tRows.Next() {
			var label string
			var cnt uint64
			if err := tRows.Scan(&label, &cnt); err != nil {
				continue
			}
			tickRangeDist = append(tickRangeDist, smartMoney24hDistBucket{
				Range: label,
				Count: int(cnt),
			})
		}
	}

	// 5. Top wallets by number of pools added in the window.
	topWalletQuery := fmt.Sprintf(`
		SELECT
			wallet_address,
			uniqExact(concat(pool_version, '|', pool_id)) AS pool_count,
			sum(add_event_count) AS add_count
		FROM (%s)
		GROUP BY wallet_address
		ORDER BY pool_count DESC, add_count DESC
		LIMIT %d
	`, activePositionsSubquery, topWalletLimit)

	topWalletArgs := buildWatchedArgs()
	topWalletRows, topWalletErr := s.ClickHouse.Conn.Query(ctx, topWalletQuery, topWalletArgs...)
	topWallets := make([]smartMoney24hTopWallet, 0, topWalletLimit)
	if topWalletErr != nil {
		warnings = append(warnings, fmt.Sprintf("头部钱包查询失败: %v", topWalletErr))
	} else {
		defer topWalletRows.Close()
		for topWalletRows.Next() {
			var row smartMoney24hTopWalletRow
			if err := topWalletRows.Scan(&row.WalletAddress, &row.PoolCount, &row.AddCount); err != nil {
				continue
			}
			topWallets = append(topWallets, smartMoney24hTopWallet{
				WalletAddress: strings.ToLower(strings.TrimSpace(row.WalletAddress)),
				PoolCount:     int(row.PoolCount),
				AddCount:      int(row.AddCount),
			})
		}
	}

	// 6. Distribution of pool counts per wallet in the window.
	walletDistQuery := fmt.Sprintf(`
		SELECT
			multiIf(
				pool_count <= 1, '1 pool',
				pool_count = 2, '2 pools',
				pool_count <= 4, '3-4 pools',
				pool_count <= 9, '5-9 pools',
				'10+ pools'
			) AS range_label,
			count() AS cnt
		FROM (
			SELECT
				wallet_address,
				uniqExact(concat(pool_version, '|', pool_id)) AS pool_count
			FROM (%s)
			GROUP BY wallet_address
		)
		GROUP BY range_label
	`, activePositionsSubquery)

	walletDistArgs := buildWatchedArgs()
	walletDistRows, walletDistErr := s.ClickHouse.Conn.Query(ctx, walletDistQuery, walletDistArgs...)
	walletDistCount := map[string]int{
		"1 pool":    0,
		"2 pools":   0,
		"3-4 pools": 0,
		"5-9 pools": 0,
		"10+ pools": 0,
	}
	if walletDistErr != nil {
		warnings = append(warnings, fmt.Sprintf("钱包分布查询失败: %v", walletDistErr))
	} else {
		defer walletDistRows.Close()
		for walletDistRows.Next() {
			var label string
			var cnt uint64
			if err := walletDistRows.Scan(&label, &cnt); err != nil {
				continue
			}
			label = strings.TrimSpace(label)
			walletDistCount[label] = int(cnt)
		}
	}

	resp := smartMoney24hResponse{
		Chain:                  chain,
		WindowHours:            windowHours,
		UpdatedAt:              time.Now(),
		TotalPools:             len(outPools),
		TotalWallets:           int(totalWallets),
		TotalEvents:            totalEvents,
		TotalAmountUSD:         sanitizeFloat(totalAmountUSD),
		Pools:                  outPools,
		HourlyTrend:            hourlyTrend,
		TickRangeDistribution:  tickRangeDist,
		PoolAmountDistribution: orderedBuckets(amountBucketCount, []string{"<= $1k", "$1k-$5k", "$5k-$20k", "$20k-$100k", "$100k-$500k", ">= $500k"}),
		WalletPoolDistribution: orderedBuckets(walletDistCount, []string{"1 pool", "2 pools", "3-4 pools", "5-9 pools", "10+ pools"}),
		TopWallets:             topWallets,
		Warnings:               warnings,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(resp)
}
