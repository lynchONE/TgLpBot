package web_server

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"TgLpBot/service/smart_lp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ethereum/go-ethereum/common"
)

type smartMoneyOverviewPool struct {
	PoolVersion    string  `json:"pool_version"`
	PoolID         string  `json:"pool_id"`
	WalletCount    int     `json:"wallet_count"`
	AddedLiquidity float64 `json:"added_liquidity"`

	Exchange     string  `json:"exchange,omitempty"`
	Pair         string  `json:"pair,omitempty"`
	FeePct       float64 `json:"fee_pct,omitempty"`
	Token0       string  `json:"token0,omitempty"`
	Token1       string  `json:"token1,omitempty"`
	Token0Symbol string  `json:"token0_symbol,omitempty"`
	Token1Symbol string  `json:"token1_symbol,omitempty"`
}

type smartMoneyOverviewWallet struct {
	WalletAddress string  `json:"wallet_address"`
	PnLUSDT24h    float64 `json:"pnl_usdt_24h"`
	InUSDT24h     float64 `json:"in_usdt_24h"`
	OutUSDT24h    float64 `json:"out_usdt_24h"`
	PnLMarginPct  float64 `json:"pnl_margin_pct,omitempty"`
	EventCount24h int     `json:"event_count_24h"`
	EventCount1h  int     `json:"event_count_1h,omitempty"`
}

type smartMoneyOverviewSummary struct {
	PoolCount            int     `json:"pool_count"`
	WalletCount          int     `json:"wallet_count"`
	TotalInUSDT24h       float64 `json:"total_in_usdt_24h"`
	TotalOutUSDT24h      float64 `json:"total_out_usdt_24h"`
	TotalPnLUSDT24h      float64 `json:"total_pnl_usdt_24h"`
	PositiveWallets24h   int     `json:"positive_wallets_24h"`
	NegativeWallets24h   int     `json:"negative_wallets_24h"`
	ZeroWallets24h       int     `json:"zero_wallets_24h"`
	TotalEvents24h       int     `json:"total_events_24h"`
	TotalEvents1h        int     `json:"total_events_1h"`
	CoverageRatio24h     float64 `json:"coverage_ratio_24h"`
	MissingPriceTokenCnt int     `json:"missing_price_token_count"`
}

type smartMoneyOverviewHistogramBucket struct {
	Label      string  `json:"label"`
	RangeMin   float64 `json:"range_min"`
	RangeMax   float64 `json:"range_max"`
	Wallets    int     `json:"wallets"`
	Share      float64 `json:"share"`
	TotalPnL24 float64 `json:"total_pnl_usdt_24h"`
}

type smartMoneyOverviewEventTrendPoint struct {
	HoursAgo       int `json:"hours_ago"`
	AddEvents      int `json:"add_events"`
	RemoveEvents   int `json:"remove_events"`
	TotalEvents    int `json:"total_events"`
	DistinctWallet int `json:"distinct_wallets"`
}

type smartMoneyOverviewResponse struct {
	Chain          string                              `json:"chain"`
	PoolsWindowSec int                                 `json:"pools_window_sec"`
	PnLWindowSec   int                                 `json:"pnl_window_sec"`
	UpdatedAt      time.Time                           `json:"updated_at"`
	Summary        smartMoneyOverviewSummary           `json:"summary"`
	Pools          []smartMoneyOverviewPool            `json:"pools"`
	Wallets24h     []smartMoneyOverviewWallet          `json:"wallets_24h"`
	PnLHistogram24 []smartMoneyOverviewHistogramBucket `json:"pnl_histogram_24h,omitempty"`
	EventTrend24h  []smartMoneyOverviewEventTrendPoint `json:"event_trend_24h,omitempty"`
	Warnings       []string                            `json:"warnings,omitempty"`
}

type smartMoneyCashflowRow struct {
	WalletAddress string
	PoolVersion   string
	PoolID        string
	Action        string
	Sum0          string
	Sum1          string
	EventCount    uint64
}

type smartMoneyEventTrendRow struct {
	HoursAgo       int32
	AddEvents      uint64
	RemoveEvents   uint64
	DistinctWallet uint64
}

func parseIntQuery(q map[string][]string, key string, def int, min int, max int) int {
	raw := strings.TrimSpace("")
	if q != nil {
		if v := q[key]; len(v) > 0 {
			raw = strings.TrimSpace(v[0])
		}
	}
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func amountToFloat(amountStr string, decimals int) float64 {
	amountStr = strings.TrimSpace(amountStr)
	if amountStr == "" {
		return 0
	}
	i, ok := new(big.Int).SetString(amountStr, 10)
	if !ok || i.Sign() == 0 {
		return 0
	}
	if decimals < 0 {
		decimals = 0
	}

	f := new(big.Float).SetInt(i)
	div := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	f.Quo(f, div)
	v, _ := f.Float64()
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func getDecimalsCached(addr string, cache map[string]int) int {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" || !common.IsHexAddress(addr) {
		return 18
	}
	if cache != nil {
		if v, ok := cache[addr]; ok {
			return v
		}
	}
	dec, err := blockchain.GetTokenDecimals(common.HexToAddress(addr))
	if err != nil || dec == 0 {
		if cache != nil {
			cache[addr] = 18
		}
		return 18
	}
	if cache != nil {
		cache[addr] = int(dec)
	}
	return int(dec)
}

func (s *Server) handleSmartMoneyOverview(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := requireMiniAppPermission(check); status != 0 {
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
	poolLimit := parseIntQuery(query, "pool_limit", 10, 1, 50)
	walletLimit := parseIntQuery(query, "wallet_limit", 50, 1, 200)
	// Keep legacy defaults (1h pools + 24h pnl), while allowing clients to
	// align windows explicitly via query params.
	poolsWindowHours := parseIntQuery(query, "pools_window_hours", 1, 1, 168)
	pnlWindowHours := parseIntQuery(query, "pnl_window_hours", 24, 1, 168)

	poolsWindow := time.Duration(poolsWindowHours) * time.Hour
	pnlWindow := time.Duration(pnlWindowHours) * time.Hour

	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()

	smSvc := smart_lp.NewSmartLPService(s.ClickHouse)
	ranks, err := smSvc.GetTopAddedLiquidityPools(ctx, chain, poolsWindow, poolLimit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	poolSvc := pool.NewPoolService()

	warnings := make([]string, 0, 2)
	if len(ranks) == 0 && chain != "" {
		fallbackRanks, ferr := smSvc.GetTopAddedLiquidityPools(ctx, "", poolsWindow, poolLimit)
		if ferr != nil {
			warnings = append(warnings, fmt.Sprintf("pool rank fallback failed: %v", ferr))
		} else if len(fallbackRanks) > 0 {
			ranks = fallbackRanks
			warnings = append(warnings, fmt.Sprintf("pool rank fallback used (chain=%s returned empty)", chain))
		}
	}
	pools := make([]smart_lp.SmartLPPoolKey, 0, len(ranks))
	outPools := make([]smartMoneyOverviewPool, 0, len(ranks))
	poolInfoByKey := make(map[string]*pool.PoolInfo, len(ranks))

	for _, rank := range ranks {
		pv := strings.ToLower(strings.TrimSpace(rank.PoolVersion))
		pid := strings.ToLower(strings.TrimSpace(rank.PoolID))
		if pv == "" || pid == "" {
			continue
		}
		pools = append(pools, smart_lp.SmartLPPoolKey{PoolVersion: pv, PoolID: pid})

		var info *pool.PoolInfo
		if pv == "v4" {
			info, err = poolSvc.GetV4PoolInfo(pid)
		} else {
			info, err = poolSvc.GetPoolInfo(pid)
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("pool info failed %s %s: %v", pv, pid, err))
			info = nil
		}
		if info != nil {
			poolInfoByKey[pv+"|"+pid] = info
		}

		p := smartMoneyOverviewPool{
			PoolVersion:    pv,
			PoolID:         pid,
			AddedLiquidity: rank.AddedLiquidity,
			WalletCount:    rank.WalletCount,
		}
		if info != nil {
			p.Exchange = strings.TrimSpace(info.Exchange)
			p.Token0 = strings.TrimSpace(info.Token0)
			p.Token1 = strings.TrimSpace(info.Token1)
			p.Token0Symbol = strings.TrimSpace(info.Token0Symbol)
			p.Token1Symbol = strings.TrimSpace(info.Token1Symbol)
			if p.Token0Symbol != "" || p.Token1Symbol != "" {
				p.Pair = strings.TrimSpace(p.Token0Symbol + "/" + p.Token1Symbol)
				if p.Pair == "/" {
					p.Pair = ""
				}
			}
			// Keep same meaning as bot: fee=500 -> 0.0500%.
			if info.Fee > 0 {
				p.FeePct = float64(info.Fee) / 10000.0
			}
		}
		outPools = append(outPools, p)
	}

	walletRanks, werr := smSvc.GetTopAddWalletsInPools(ctx, chain, poolsWindow, pools, walletLimit)
	if werr != nil {
		warnings = append(warnings, fmt.Sprintf("wallet rank query failed: %v", werr))
		walletRanks = nil
	}

	wallets := make([]string, 0, len(walletRanks))
	walletRankMap := make(map[string]smart_lp.SmartLPWalletRank, len(walletRanks))
	for _, wr := range walletRanks {
		addr := strings.ToLower(strings.TrimSpace(wr.WalletAddress))
		if addr == "" {
			continue
		}
		wallets = append(wallets, addr)
		walletRankMap[addr] = wr
	}

	outWallets := make([]smartMoneyOverviewWallet, 0, len(wallets))
	if len(wallets) > 0 && len(pools) > 0 {
		flowRows, ferr := querySmartMoneyCashflows(ctx, s.ClickHouse.Conn, chain, pools, wallets, pnlWindow)
		if ferr != nil {
			warnings = append(warnings, fmt.Sprintf("cashflow query failed: %v", ferr))
		} else {
			decimalsCache := make(map[string]int)

			// Collect unique tokens for pricing.
			tokenSet := make(map[string]struct{}, 2*len(pools))
			tokens := make([]string, 0, 2*len(pools))
			for _, pk := range pools {
				info := poolInfoByKey[pk.PoolVersion+"|"+pk.PoolID]
				if info == nil {
					continue
				}
				t0 := strings.ToLower(strings.TrimSpace(info.Token0))
				t1 := strings.ToLower(strings.TrimSpace(info.Token1))
				if common.IsHexAddress(t0) {
					if _, ok := tokenSet[t0]; !ok {
						tokenSet[t0] = struct{}{}
						tokens = append(tokens, t0)
					}
				}
				if common.IsHexAddress(t1) {
					if _, ok := tokenSet[t1]; !ok {
						tokenSet[t1] = struct{}{}
						tokens = append(tokens, t1)
					}
				}
			}

			priceSvc := s.TokenPrice
			if priceSvc == nil {
				priceSvc = pricing.NewTokenPriceService()
			}
			prices, perr := priceSvc.GetUSDPrices(chain, tokens)
			if perr != nil {
				warnings = append(warnings, fmt.Sprintf("price provider limited/rate-limited; using cached/fallback prices where available"))
			}

			type walletAgg struct {
				inUSDT  float64
				outUSDT float64
				cnt     int
			}
			agg := make(map[string]*walletAgg)

			missingPriceTokens := make(map[string]struct{})
			for _, row := range flowRows {
				addr := strings.ToLower(strings.TrimSpace(row.WalletAddress))
				if addr == "" {
					continue
				}
				info := poolInfoByKey[strings.ToLower(strings.TrimSpace(row.PoolVersion))+"|"+strings.ToLower(strings.TrimSpace(row.PoolID))]
				if info == nil {
					continue
				}
				t0 := strings.ToLower(strings.TrimSpace(info.Token0))
				t1 := strings.ToLower(strings.TrimSpace(info.Token1))
				if !common.IsHexAddress(t0) || !common.IsHexAddress(t1) {
					continue
				}

				p0 := prices[t0]
				p1 := prices[t1]

				dec0 := getDecimalsCached(t0, decimalsCache)
				dec1 := getDecimalsCached(t1, decimalsCache)
				amt0 := amountToFloat(row.Sum0, dec0)
				amt1 := amountToFloat(row.Sum1, dec1)

				usd := amt0*p0 + amt1*p1
				if (amt0 > 0 && p0 == 0) || (amt1 > 0 && p1 == 0) {
					if amt0 > 0 && p0 == 0 {
						missingPriceTokens[t0] = struct{}{}
					}
					if amt1 > 0 && p1 == 0 {
						missingPriceTokens[t1] = struct{}{}
					}
				}

				a := agg[addr]
				if a == nil {
					a = &walletAgg{}
					agg[addr] = a
				}
				a.cnt += int(row.EventCount)

				switch strings.ToLower(strings.TrimSpace(row.Action)) {
				case "add":
					a.outUSDT += usd
				case "remove":
					a.inUSDT += usd
				}
			}

			missingPriceTokenCount := len(missingPriceTokens)
			if missingPriceTokenCount > 0 {
				warnings = append(warnings, fmt.Sprintf("%d token prices are still missing; pnl may be underestimated", missingPriceTokenCount))
			}

			for _, addr := range wallets {
				a := agg[addr]
				inUSDT := 0.0
				outUSDT := 0.0
				cnt := 0
				if a != nil {
					inUSDT = a.inUSDT
					outUSDT = a.outUSDT
					cnt = a.cnt
				}
				wr := walletRankMap[addr]
				marginPct := 0.0
				if outUSDT > 0 {
					marginPct = (inUSDT - outUSDT) / outUSDT * 100.0
				}
				outWallets = append(outWallets, smartMoneyOverviewWallet{
					WalletAddress: addr,
					PnLUSDT24h:    inUSDT - outUSDT,
					InUSDT24h:     inUSDT,
					OutUSDT24h:    outUSDT,
					PnLMarginPct:  marginPct,
					EventCount24h: cnt,
					EventCount1h:  wr.EventCount,
				})
			}

			sort.Slice(outWallets, func(i, j int) bool {
				if outWallets[i].PnLUSDT24h != outWallets[j].PnLUSDT24h {
					return outWallets[i].PnLUSDT24h > outWallets[j].PnLUSDT24h
				}
				if outWallets[i].EventCount1h != outWallets[j].EventCount1h {
					return outWallets[i].EventCount1h > outWallets[j].EventCount1h
				}
				return outWallets[i].WalletAddress < outWallets[j].WalletAddress
			})

			trendRows, terr := querySmartMoneyEventTrend(ctx, s.ClickHouse.Conn, chain, pools, pnlWindow)
			if terr != nil {
				warnings = append(warnings, fmt.Sprintf("event trend query failed: %v", terr))
			}

			summary := buildSmartMoneySummary(outPools, outWallets, missingPriceTokenCount)
			hist := buildSmartMoneyHistogram(outWallets)
			trend := buildSmartMoneyEventTrend(trendRows)

			resp := smartMoneyOverviewResponse{
				Chain:          chain,
				PoolsWindowSec: int(poolsWindow.Seconds()),
				PnLWindowSec:   int(pnlWindow.Seconds()),
				UpdatedAt:      time.Now(),
				Summary:        summary,
				Pools:          outPools,
				Wallets24h:     outWallets,
				PnLHistogram24: hist,
				EventTrend24h:  trend,
				Warnings:       warnings,
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
	}

	resp := smartMoneyOverviewResponse{
		Chain:          chain,
		PoolsWindowSec: int(poolsWindow.Seconds()),
		PnLWindowSec:   int(pnlWindow.Seconds()),
		UpdatedAt:      time.Now(),
		Summary:        buildSmartMoneySummary(outPools, outWallets, 0),
		Pools:          outPools,
		Wallets24h:     outWallets,
		PnLHistogram24: buildSmartMoneyHistogram(outWallets),
		EventTrend24h:  []smartMoneyOverviewEventTrendPoint{},
		Warnings:       warnings,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func buildSmartMoneySummary(pools []smartMoneyOverviewPool, wallets []smartMoneyOverviewWallet, missingPriceTokenCount int) smartMoneyOverviewSummary {
	summary := smartMoneyOverviewSummary{
		PoolCount:            len(pools),
		WalletCount:          len(wallets),
		MissingPriceTokenCnt: missingPriceTokenCount,
	}
	if len(wallets) == 0 {
		return summary
	}

	for _, w := range wallets {
		summary.TotalInUSDT24h += w.InUSDT24h
		summary.TotalOutUSDT24h += w.OutUSDT24h
		summary.TotalPnLUSDT24h += w.PnLUSDT24h
		summary.TotalEvents24h += w.EventCount24h
		summary.TotalEvents1h += w.EventCount1h
		switch {
		case w.PnLUSDT24h > 0:
			summary.PositiveWallets24h++
		case w.PnLUSDT24h < 0:
			summary.NegativeWallets24h++
		default:
			summary.ZeroWallets24h++
		}
	}

	covered := summary.PositiveWallets24h + summary.NegativeWallets24h
	if summary.WalletCount > 0 {
		summary.CoverageRatio24h = float64(covered) / float64(summary.WalletCount)
	}
	return summary
}

func buildSmartMoneyHistogram(wallets []smartMoneyOverviewWallet) []smartMoneyOverviewHistogramBucket {
	if len(wallets) == 0 {
		return []smartMoneyOverviewHistogramBucket{}
	}
	type bucketDef struct {
		label string
		min   float64
		max   float64
	}
	buckets := []bucketDef{
		{label: "<= -1000", min: math.Inf(-1), max: -1000},
		{label: "-1000 ~ -300", min: -1000, max: -300},
		{label: "-300 ~ -100", min: -300, max: -100},
		{label: "-100 ~ -10", min: -100, max: -10},
		{label: "-10 ~ 10", min: -10, max: 10},
		{label: "10 ~ 100", min: 10, max: 100},
		{label: "100 ~ 300", min: 100, max: 300},
		{label: "300 ~ 1000", min: 300, max: 1000},
		{label: ">= 1000", min: 1000, max: math.Inf(1)},
	}
	out := make([]smartMoneyOverviewHistogramBucket, len(buckets))
	total := len(wallets)
	for i, def := range buckets {
		out[i] = smartMoneyOverviewHistogramBucket{
			Label:    def.label,
			RangeMin: def.min,
			RangeMax: def.max,
		}
	}

	for _, wallet := range wallets {
		v := wallet.PnLUSDT24h
		for i := range buckets {
			if i == 0 {
				if v <= buckets[i].max {
					out[i].Wallets++
					out[i].TotalPnL24 += v
					break
				}
				continue
			}
			if i == len(buckets)-1 {
				if v >= buckets[i].min {
					out[i].Wallets++
					out[i].TotalPnL24 += v
					break
				}
				continue
			}
			if v > buckets[i].min && v <= buckets[i].max {
				out[i].Wallets++
				out[i].TotalPnL24 += v
				break
			}
		}
	}

	if total > 0 {
		for i := range out {
			out[i].Share = float64(out[i].Wallets) / float64(total)
		}
	}
	return out
}

func buildSmartMoneyEventTrend(rows []smartMoneyEventTrendRow) []smartMoneyOverviewEventTrendPoint {
	out := make([]smartMoneyOverviewEventTrendPoint, 0, 24)
	byHour := make(map[int]smartMoneyEventTrendRow, len(rows))
	for _, row := range rows {
		h := int(row.HoursAgo)
		if h < 0 || h >= 24 {
			continue
		}
		byHour[h] = row
	}
	for h := 23; h >= 0; h-- {
		row := byHour[h]
		add := int(row.AddEvents)
		remove := int(row.RemoveEvents)
		out = append(out, smartMoneyOverviewEventTrendPoint{
			HoursAgo:       h,
			AddEvents:      add,
			RemoveEvents:   remove,
			TotalEvents:    add + remove,
			DistinctWallet: int(row.DistinctWallet),
		})
	}
	return out
}

type smartMoneyClickHouseQueryer interface {
	Query(ctx context.Context, query string, args ...any) (driver.Rows, error)
}

func querySmartMoneyCashflows(ctx context.Context, conn smartMoneyClickHouseQueryer, chain string, pools []smart_lp.SmartLPPoolKey, wallets []string, window time.Duration) ([]smartMoneyCashflowRow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if conn == nil {
		return nil, fmt.Errorf("clickhouse not initialized")
	}
	if len(pools) == 0 || len(wallets) == 0 {
		return []smartMoneyCashflowRow{}, nil
	}

	if window <= 0 {
		window = 24 * time.Hour
	}
	seconds := int(window.Seconds())
	if seconds <= 0 {
		seconds = 86400
	}

	placeholders := make([]string, 0, len(pools))
	args := make([]any, 0, 2+2*len(pools))
	args = append(args, wallets)
	for _, p := range pools {
		pv := strings.ToLower(strings.TrimSpace(p.PoolVersion))
		pid := strings.ToLower(strings.TrimSpace(p.PoolID))
		if pv == "" || pid == "" {
			continue
		}
		placeholders = append(placeholders, "(?, ?)")
		args = append(args, pv, pid)
	}
	if len(placeholders) == 0 {
		return []smartMoneyCashflowRow{}, nil
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	chainFilter := ""
	if chain != "" {
		chainFilter = "AND lowerUTF8(chain) = ?"
		args = append(args, chain)
	}

	// Prefer `net_amount*` when it is non-zero; otherwise fall back to event amounts.
	q := fmt.Sprintf(`
		SELECT
			wallet_address,
			pool_version,
			pool_id,
			action,
			toString(sum(toInt256OrZero(if(net_amount0 != '' AND net_amount0 != '0', net_amount0, amount0)))) AS sum0,
			toString(sum(toInt256OrZero(if(net_amount1 != '' AND net_amount1 != '0', net_amount1, amount1)))) AS sum1,
			count() AS event_count
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND action IN ('add', 'remove')
			AND wallet_address IN (?)
			AND (pool_version, pool_id) IN (%s)
			%s
		GROUP BY wallet_address, pool_version, pool_id, action
	`, seconds, strings.Join(placeholders, ","), chainFilter)

	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]smartMoneyCashflowRow, 0)
	for rows.Next() {
		var r smartMoneyCashflowRow
		if err := rows.Scan(&r.WalletAddress, &r.PoolVersion, &r.PoolID, &r.Action, &r.Sum0, &r.Sum1, &r.EventCount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func querySmartMoneyEventTrend(ctx context.Context, conn smartMoneyClickHouseQueryer, chain string, pools []smart_lp.SmartLPPoolKey, window time.Duration) ([]smartMoneyEventTrendRow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if conn == nil {
		return nil, fmt.Errorf("clickhouse not initialized")
	}
	if len(pools) == 0 {
		return []smartMoneyEventTrendRow{}, nil
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	seconds := int(window.Seconds())
	if seconds <= 0 {
		seconds = 86400
	}

	placeholders := make([]string, 0, len(pools))
	args := make([]any, 0, 2+2*len(pools))
	for _, p := range pools {
		pv := strings.ToLower(strings.TrimSpace(p.PoolVersion))
		pid := strings.ToLower(strings.TrimSpace(p.PoolID))
		if pv == "" || pid == "" {
			continue
		}
		placeholders = append(placeholders, "(?, ?)")
		args = append(args, pv, pid)
	}
	if len(placeholders) == 0 {
		return []smartMoneyEventTrendRow{}, nil
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	chainFilter := ""
	if chain != "" {
		chainFilter = "AND lowerUTF8(chain) = ?"
		args = append(args, chain)
	}

	q := fmt.Sprintf(`
		SELECT
			toInt32(intDiv(dateDiff('second', ts, now()), 3600)) AS hours_ago,
			sum(if(action='add', 1, 0)) AS add_events,
			sum(if(action='remove', 1, 0)) AS remove_events,
			uniqExact(wallet_address) AS distinct_wallets
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND action IN ('add', 'remove')
			AND (pool_version, pool_id) IN (%s)
			%s
		GROUP BY hours_ago
	`, seconds, strings.Join(placeholders, ","), chainFilter)

	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]smartMoneyEventTrendRow, 0, 24)
	for rows.Next() {
		var r smartMoneyEventTrendRow
		if err := rows.Scan(&r.HoursAgo, &r.AddEvents, &r.RemoveEvents, &r.DistinctWallet); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
