package web_server

import (
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
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

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	windowSec := windowHours * 3600
	warnings := make([]string, 0, 2)

	// 1. Query top pools by wallet count in window
	poolQuery := fmt.Sprintf(`
		SELECT
			pool_version,
			pool_id,
			count() AS event_count,
			uniqExact(wallet_address) AS wallet_count,
			min(ts) AS first_add_at,
			max(ts) AS last_add_at
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND action = 'add'
			AND lowerUTF8(chain) = ?
			AND pool_version != '' AND pool_id != ''
		GROUP BY pool_version, pool_id
		ORDER BY wallet_count DESC, event_count DESC
		LIMIT %d
	`, windowSec, poolLimit)

	rows, err := s.ClickHouse.Conn.Query(ctx, poolQuery, chain)
	if err != nil {
		http.Error(w, fmt.Sprintf("pool query failed: %v", err), http.StatusInternalServerError)
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
			warnings = append(warnings, fmt.Sprintf("pool scan error: %v", err))
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
		warnings = append(warnings, fmt.Sprintf("pool rows error: %v", err))
	}

	// Resolve pool info
	poolSvc := pool.NewPoolService()
	poolIndex := make(map[string]*smartMoney24hPool, len(outPools))
	poolInfoByKey := make(map[string]*pool.PoolInfo, len(outPools))
	for i := range outPools {
		p := &outPools[i]
		pv := strings.ToLower(strings.TrimSpace(p.PoolVersion))
		pid := strings.ToLower(strings.TrimSpace(p.PoolID))
		poolIndex[pv+"|"+pid] = p
		var info *pool.PoolInfo
		var infoErr error
		if pv == "v4" {
			info, infoErr = poolSvc.GetV4PoolInfo(p.PoolID)
		} else {
			info, infoErr = poolSvc.GetPoolInfo(p.PoolID)
		}
		if infoErr != nil || info == nil {
			continue
		}
		poolInfoByKey[pv+"|"+pid] = info
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

	// 1.1 Query total added token amounts for selected pools and convert to USD.
	selectedPlaceholders := make([]string, 0, len(outPools))
	selectedArgs := make([]any, 0, 1+2*len(outPools))
	selectedArgs = append(selectedArgs, chain)
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
				toString(sum(toInt256OrZero(amount0))) AS sum0,
				toString(sum(toInt256OrZero(amount1))) AS sum1
			FROM smart_lp_events
			WHERE ts >= now() - INTERVAL %d SECOND
				AND action = 'add'
				AND lowerUTF8(chain) = ?
				AND (pool_version, pool_id) IN (%s)
			GROUP BY pool_version, pool_id
		`, windowSec, strings.Join(selectedPlaceholders, ","))

		amountRows, amountErr := s.ClickHouse.Conn.Query(ctx, poolAmountQuery, selectedArgs...)
		if amountErr != nil {
			warnings = append(warnings, fmt.Sprintf("pool amount query failed: %v", amountErr))
		} else {
			defer amountRows.Close()
			for amountRows.Next() {
				var row smartMoney24hPoolAmountRow
				if err := amountRows.Scan(&row.PoolVersion, &row.PoolID, &row.Sum0, &row.Sum1); err != nil {
					warnings = append(warnings, fmt.Sprintf("pool amount scan failed: %v", err))
					continue
				}
				row.PoolVersion = strings.ToLower(strings.TrimSpace(row.PoolVersion))
				row.PoolID = strings.ToLower(strings.TrimSpace(row.PoolID))
				poolAmountRows = append(poolAmountRows, row)
			}
			if err := amountRows.Err(); err != nil {
				warnings = append(warnings, fmt.Sprintf("pool amount rows error: %v", err))
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

	prices, priceErr := priceSvc.GetUSDPrices(chain, tokens)
	if priceErr != nil {
		warnings = append(warnings, "price provider limited/rate-limited; using cached/fallback prices where available")
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
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND action = 'add'
			AND lowerUTF8(chain) = ?
	`, windowSec)
	if err := s.ClickHouse.Conn.QueryRow(ctx, totalWalletsQ, chain).Scan(&totalWallets); err != nil {
		warnings = append(warnings, fmt.Sprintf("total wallets query failed: %v", err))
	}

	// 3. Hourly trend
	hourlyQ := fmt.Sprintf(`
		SELECT
			toStartOfHour(ts) AS hour,
			count() AS add_count,
			uniqExact(wallet_address) AS wallet_count,
			uniqExact(concat(pool_version, '|', pool_id)) AS distinct_pools
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND action = 'add'
			AND lowerUTF8(chain) = ?
		GROUP BY hour
		ORDER BY hour ASC
	`, windowSec)

	hRows, err := s.ClickHouse.Conn.Query(ctx, hourlyQ, chain)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("hourly trend query failed: %v", err))
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
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND action = 'add'
			AND lowerUTF8(chain) = ?
			AND tick_lower != 0 AND tick_upper != 0
		GROUP BY range_label
		ORDER BY cnt DESC
	`, windowSec)

	tRows, err := s.ClickHouse.Conn.Query(ctx, tickRangeQ, chain)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("tick range query failed: %v", err))
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
			count() AS add_count
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND action = 'add'
			AND lowerUTF8(chain) = ?
			AND wallet_address != ''
		GROUP BY wallet_address
		ORDER BY pool_count DESC, add_count DESC
		LIMIT %d
	`, windowSec, topWalletLimit)

	topWalletRows, topWalletErr := s.ClickHouse.Conn.Query(ctx, topWalletQuery, chain)
	topWallets := make([]smartMoney24hTopWallet, 0, topWalletLimit)
	if topWalletErr != nil {
		warnings = append(warnings, fmt.Sprintf("top wallets query failed: %v", topWalletErr))
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
			SELECT uniqExact(concat(pool_version, '|', pool_id)) AS pool_count
			FROM smart_lp_events
			WHERE ts >= now() - INTERVAL %d SECOND
				AND action = 'add'
				AND lowerUTF8(chain) = ?
				AND wallet_address != ''
			GROUP BY wallet_address
		)
		GROUP BY range_label
	`, windowSec)

	walletDistRows, walletDistErr := s.ClickHouse.Conn.Query(ctx, walletDistQuery, chain)
	walletDistCount := map[string]int{
		"1 pool":    0,
		"2 pools":   0,
		"3-4 pools": 0,
		"5-9 pools": 0,
		"10+ pools": 0,
	}
	if walletDistErr != nil {
		warnings = append(warnings, fmt.Sprintf("wallet distribution query failed: %v", walletDistErr))
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
