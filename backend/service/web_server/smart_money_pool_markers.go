package web_server

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"

	"github.com/ethereum/go-ethereum/common"
)

var poolAddressRegex = regexp.MustCompile(`^(0x)?[a-fA-F0-9]{40}$|^(0x)?[a-fA-F0-9]{64}$`)

type smartMoneyPoolMarkerEvent struct {
	EventID        string  `json:"event_id"`
	T              int64   `json:"t"`
	BucketT        int64   `json:"bucket_t"`
	WalletAddress  string  `json:"wallet_address"`
	WalletLabel    string  `json:"wallet_label,omitempty"`
	WalletSource   string  `json:"wallet_source,omitempty"`
	Action         string  `json:"action"`
	TxHash         string  `json:"tx_hash,omitempty"`
	TxURL          string  `json:"tx_url,omitempty"`
	TickLower      int     `json:"tick_lower"`
	TickUpper      int     `json:"tick_upper"`
	PriceLower     float64 `json:"price_lower,omitempty"`
	PriceUpper     float64 `json:"price_upper,omitempty"`
	PriceBase      string  `json:"price_base,omitempty"`
	PriceQuote     string  `json:"price_quote,omitempty"`
	AnchorPrice    float64 `json:"anchor_price,omitempty"`
	Amount0        float64 `json:"amount0"`
	Amount1        float64 `json:"amount1"`
	Amount0USD     float64 `json:"amount0_usd"`
	Amount1USD     float64 `json:"amount1_usd"`
	EstimatedUSD   float64 `json:"estimated_usd"`
	HasPnLEstimate bool    `json:"has_pnl_estimate,omitempty"`
	CostBasisUSD   float64 `json:"cost_basis_usd,omitempty"`
	PnLEstimateUSD float64 `json:"pnl_estimate_usd,omitempty"`
}

type smartMoneyPoolMarkersResponse struct {
	Chain       string                      `json:"chain"`
	BucketSec   int                         `json:"bucket_sec"`
	WindowSec   int                         `json:"window_sec"`
	UpdatedAt   time.Time                   `json:"updated_at"`
	Pool        smartMoneyPoolAddsPool      `json:"pool"`
	Events      []smartMoneyPoolMarkerEvent `json:"events"`
	TotalEvents int                         `json:"total_events"`
	AddCount    int                         `json:"add_count"`
	RemoveCount int                         `json:"remove_count"`
	WalletCount int                         `json:"wallet_count"`
	Truncated   bool                        `json:"truncated,omitempty"`
	Warnings    []string                    `json:"warnings,omitempty"`
}

type smartMoneyPoolMarkerRow struct {
	Ts             time.Time
	EventSeq       uint64
	WalletAddress  string
	Action         string
	TokenID        string
	ContractAddr   string
	Amount0        string
	Amount1        string
	LiquidityDelta string
	TickLower      int32
	TickUpper      int32
	TxHash         string
	BlockNumber    uint64
	LogIndex       uint32
}

type smartMoneyPoolMarkerSummary struct {
	TotalEvents uint64
	AddCount    uint64
	RemoveCount uint64
	WalletCount uint64
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func buildMarkerBucketBounds(start time.Time, end time.Time, bucketSec int) (int64, int64) {
	if bucketSec <= 0 {
		bucketSec = 300
	}
	if end.Before(start) {
		start, end = end, start
	}
	bucketSize := int64(bucketSec)
	startBucket := (start.Unix() / bucketSize) * bucketSize
	endBucket := (end.Unix() / bucketSize) * bucketSize
	return startBucket, endBucket
}

func buildMarkerEventID(txHash string, eventSeq uint64, logIndex uint32) string {
	txHash = strings.ToLower(strings.TrimSpace(txHash))
	if txHash != "" {
		return fmt.Sprintf("%s:%d", txHash, logIndex)
	}
	return fmt.Sprintf("event:%d:%d", eventSeq, logIndex)
}

func loadUserManagedWalletLabels(userID uint, chain string) map[string]string {
	return loadSmartMoneyWalletLabels(userID, chain)
}

func querySmartMoneyPoolMarkerEvents(ctx context.Context, conn smartMoneyClickHouseQueryer, chain string, poolVersion string, poolID string, bucketSec int, start time.Time, end time.Time, limit int) ([]smartMoneyPoolMarkerRow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if conn == nil {
		return nil, fmt.Errorf("clickhouse not initialized")
	}

	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	poolID = strings.ToLower(strings.TrimSpace(poolID))
	if poolVersion == "" || poolID == "" {
		return []smartMoneyPoolMarkerRow{}, nil
	}

	if start.IsZero() || end.IsZero() {
		end = time.Now().UTC()
		start = end.Add(-12 * time.Hour)
	}
	startBucket, endBucket := buildMarkerBucketBounds(start, end, bucketSec)

	if limit <= 0 {
		limit = 300
	}
	if limit > 2000 {
		limit = 2000
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	chainFilter := ""
	args := make([]any, 0, 9)
	args = append(args, poolVersion, poolID, bucketSec, bucketSec, startBucket, bucketSec, bucketSec, endBucket)
	if chain != "" {
		chainFilter = "AND lowerUTF8(chain) = ?"
		args = append(args, chain)
	}

	q := fmt.Sprintf(`
		SELECT
			ts,
			event_seq,
			wallet_address,
			action,
			token_id,
			contract_address,
			toString(if(net_amount0 != '' AND net_amount0 != '0', net_amount0, amount0)) AS amount0_value,
			toString(if(net_amount1 != '' AND net_amount1 != '0', net_amount1, amount1)) AS amount1_value,
			liquidity_delta,
			tick_lower,
			tick_upper,
			tx_hash,
			block_number,
			log_index
		FROM smart_lp_events
		WHERE action IN ('add', 'remove')
			AND pool_version = ? AND pool_id = ?
			AND wallet_address != ''
			AND intDiv(toInt64(toUnixTimestamp(ts)), ?) * ? >= ?
			AND intDiv(toInt64(toUnixTimestamp(ts)), ?) * ? <= ?
			%s
		ORDER BY block_number DESC, log_index DESC
		LIMIT %d
	`, chainFilter, limit)

	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]smartMoneyPoolMarkerRow, 0, limit)
	for rows.Next() {
		var row smartMoneyPoolMarkerRow
		if err := rows.Scan(
			&row.Ts,
			&row.EventSeq,
			&row.WalletAddress,
			&row.Action,
			&row.TokenID,
			&row.ContractAddr,
			&row.Amount0,
			&row.Amount1,
			&row.LiquidityDelta,
			&row.TickLower,
			&row.TickUpper,
			&row.TxHash,
			&row.BlockNumber,
			&row.LogIndex,
		); err != nil {
			return nil, err
		}
		row.WalletAddress = strings.ToLower(strings.TrimSpace(row.WalletAddress))
		row.Action = strings.ToLower(strings.TrimSpace(row.Action))
		row.TxHash = strings.ToLower(strings.TrimSpace(row.TxHash))
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func querySmartMoneyPoolMarkerSummary(ctx context.Context, conn smartMoneyClickHouseQueryer, chain string, poolVersion string, poolID string, bucketSec int, start time.Time, end time.Time) (smartMoneyPoolMarkerSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if conn == nil {
		return smartMoneyPoolMarkerSummary{}, fmt.Errorf("clickhouse not initialized")
	}

	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	poolID = strings.ToLower(strings.TrimSpace(poolID))
	if poolVersion == "" || poolID == "" {
		return smartMoneyPoolMarkerSummary{}, nil
	}

	if start.IsZero() || end.IsZero() {
		end = time.Now().UTC()
		start = end.Add(-12 * time.Hour)
	}
	startBucket, endBucket := buildMarkerBucketBounds(start, end, bucketSec)

	chain = strings.ToLower(strings.TrimSpace(chain))
	chainFilter := ""
	args := make([]any, 0, 9)
	args = append(args, poolVersion, poolID, bucketSec, bucketSec, startBucket, bucketSec, bucketSec, endBucket)
	if chain != "" {
		chainFilter = "AND lowerUTF8(chain) = ?"
		args = append(args, chain)
	}

	q := fmt.Sprintf(`
		SELECT
			count() AS total_events,
			countIf(action = 'add') AS add_count,
			countIf(action = 'remove') AS remove_count,
			uniqExact(wallet_address) AS wallet_count
		FROM smart_lp_events
		WHERE action IN ('add', 'remove')
			AND pool_version = ? AND pool_id = ?
			AND wallet_address != ''
			AND intDiv(toInt64(toUnixTimestamp(ts)), ?) * ? >= ?
			AND intDiv(toInt64(toUnixTimestamp(ts)), ?) * ? <= ?
			%s
	`, chainFilter)

	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return smartMoneyPoolMarkerSummary{}, err
	}
	defer rows.Close()

	var out smartMoneyPoolMarkerSummary
	if rows.Next() {
		if err := rows.Scan(&out.TotalEvents, &out.AddCount, &out.RemoveCount, &out.WalletCount); err != nil {
			return smartMoneyPoolMarkerSummary{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return smartMoneyPoolMarkerSummary{}, err
	}
	return out, nil
}

func (s *Server) handleSmartMoneyPoolMarkers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s == nil || s.ClickHouse == nil || s.ClickHouse.Conn == nil {
		http.Error(w, "ClickHouse not configured", http.StatusServiceUnavailable)
		return
	}

	user, status, msg := authenticateSmartMoneyRequest(r)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	query := r.URL.Query()
	chain := config.NormalizeChain(query.Get("chain"))
	if chain == "" {
		chain = "bsc"
	}

	poolVersion := strings.ToLower(strings.TrimSpace(query.Get("pool_version")))
	switch poolVersion {
	case "v3", "v4":
	default:
		http.Error(w, "invalid pool_version", http.StatusBadRequest)
		return
	}

	poolID := normalizeHexID(query.Get("pool_id"))
	switch poolVersion {
	case "v3":
		if !poolAddressRegex.MatchString(poolID) || len(poolID) != 42 {
			http.Error(w, "invalid pool_id", http.StatusBadRequest)
			return
		}
	case "v4":
		if !strings.HasPrefix(poolID, "0x") || len(poolID) != 66 || !isHex(poolID[2:]) {
			http.Error(w, "invalid pool_id", http.StatusBadRequest)
			return
		}
	}
	poolID = strings.ToLower(poolID)

	bucketSec := parseIntQuery(query, "bucket_sec", 300, 60, 86400)
	windowHours := parseIntQuery(query, "window_hours", 12, 1, 48)
	limit := parseIntQuery(query, "limit", 300, 1, 2000)
	startTS := parseUnixSecondsQuery(query, "start_ts")
	endTS := parseUnixSecondsQuery(query, "end_ts")
	rangeStart, rangeEnd := resolveUnixTimeRange(startTS, endTS, time.Duration(windowHours)*time.Hour)

	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()

	rows, err := querySmartMoneyPoolMarkerEvents(ctx, s.ClickHouse.Conn, chain, poolVersion, poolID, bucketSec, rangeStart, rangeEnd, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	summary, summaryErr := querySmartMoneyPoolMarkerSummary(ctx, s.ClickHouse.Conn, chain, poolVersion, poolID, bucketSec, rangeStart, rangeEnd)

	poolSvc := pool.NewPoolService()
	var info *pool.PoolInfo
	var perr error
	if poolVersion == "v4" {
		info, perr = poolSvc.GetV4PoolInfo(poolID)
	} else {
		info, perr = poolSvc.GetPoolInfo(poolID)
	}

	warnings := make([]string, 0, 4)
	if summaryErr != nil {
		warnings = append(warnings, fmt.Sprintf("marker summary failed: %v", summaryErr))
	}
	if perr != nil {
		warnings = append(warnings, fmt.Sprintf("pool info failed: %v", perr))
	}

	outPool := smartMoneyPoolAddsPool{
		PoolVersion: poolVersion,
		PoolID:      poolID,
	}
	if info != nil {
		outPool.Exchange = strings.TrimSpace(info.Exchange)
		outPool.Token0 = strings.TrimSpace(info.Token0)
		outPool.Token1 = strings.TrimSpace(info.Token1)
		outPool.Token0Symbol = strings.TrimSpace(info.Token0Symbol)
		outPool.Token1Symbol = strings.TrimSpace(info.Token1Symbol)
		if outPool.Token0Symbol != "" || outPool.Token1Symbol != "" {
			outPool.Pair = strings.TrimSpace(outPool.Token0Symbol + "/" + outPool.Token1Symbol)
			if outPool.Pair == "/" {
				outPool.Pair = ""
			}
		}
		if info.Fee > 0 {
			outPool.FeePct = float64(info.Fee) / 10000.0
		}
	}

	userManaged := loadUserManagedWalletLabels(user.ID, chain)
	chRows, chErr := s.loadCHWatchedWallets(chain)
	if chErr != nil {
		warnings = append(warnings, fmt.Sprintf("watchlist metadata failed: %v", chErr))
	}
	walletSource := make(map[string]string, len(chRows))
	for _, row := range chRows {
		addr := strings.ToLower(strings.TrimSpace(row.WalletAddress))
		if !common.IsHexAddress(addr) {
			continue
		}
		if strings.TrimSpace(row.Source) == "" {
			walletSource[addr] = "scan_add"
			continue
		}
		walletSource[addr] = row.Source
	}

	priceSvc := s.TokenPrice
	if priceSvc == nil {
		priceSvc = pricing.NewTokenPriceService()
	}
	decimalsCache := make(map[string]int)
	t0 := strings.ToLower(strings.TrimSpace(outPool.Token0))
	t1 := strings.ToLower(strings.TrimSpace(outPool.Token1))
	tokens := make([]string, 0, 2)
	if common.IsHexAddress(t0) {
		tokens = append(tokens, t0)
	}
	if common.IsHexAddress(t1) {
		tokens = append(tokens, t1)
	}
	prices, priceErr := priceSvc.GetUSDPrices(chain, tokens)
	if priceErr != nil {
		warnings = append(warnings, "price provider limited/rate-limited; using cached/fallback prices where available")
	}

	dec0 := 18
	dec1 := 18
	if common.IsHexAddress(t0) {
		dec0 = getDecimalsCached(t0, decimalsCache)
	}
	if common.IsHexAddress(t1) {
		dec1 = getDecimalsCached(t1, decimalsCache)
	}
	p0 := prices[t0]
	p1 := prices[t1]

	pnlEstimates := make(map[string]smartMoneyPoolMarkerPnLEstimate)
	historyRows, historyErr := querySmartMoneyPoolMarkerHistory(ctx, s.ClickHouse.Conn, chain, poolVersion, poolID, rows, rangeEnd)
	if historyErr != nil {
		warnings = append(warnings, fmt.Sprintf("marker pnl replay failed: %v", historyErr))
	} else if len(historyRows) > 0 {
		pnlEstimates = applyMarkerPnLEstimates(rows, historyRows, dec0, dec1, p0, p1)
	}

	task := &models.StrategyTask{
		PoolId:        poolID,
		PoolVersion:   poolVersion,
		Token0Symbol:  strings.TrimSpace(outPool.Token0Symbol),
		Token1Symbol:  strings.TrimSpace(outPool.Token1Symbol),
		Token0Address: strings.TrimSpace(outPool.Token0),
		Token1Address: strings.TrimSpace(outPool.Token1),
	}

	events := make([]smartMoneyPoolMarkerEvent, 0, len(rows))
	for _, row := range rows {
		ts := row.Ts.Unix()
		if ts <= 0 {
			continue
		}
		amt0 := absFloat(amountToFloat(row.Amount0, dec0))
		amt1 := absFloat(amountToFloat(row.Amount1, dec1))
		usd0 := sanitizeFloat(amt0 * p0)
		usd1 := sanitizeFloat(amt1 * p1)
		total := sanitizeFloat(usd0 + usd1)

		ev := smartMoneyPoolMarkerEvent{
			EventID:       buildMarkerEventID(row.TxHash, row.EventSeq, row.LogIndex),
			T:             ts,
			BucketT:       (ts / int64(bucketSec)) * int64(bucketSec),
			WalletAddress: row.WalletAddress,
			Action:        row.Action,
			TxHash:        row.TxHash,
			TxURL:         config.ExplorerTxURL(chain, row.TxHash),
			TickLower:     int(row.TickLower),
			TickUpper:     int(row.TickUpper),
			Amount0:       sanitizeFloat(amt0),
			Amount1:       sanitizeFloat(amt1),
			Amount0USD:    usd0,
			Amount1USD:    usd1,
			EstimatedUSD:  total,
		}
		if label := strings.TrimSpace(userManaged[row.WalletAddress]); label != "" {
			ev.WalletLabel = label
			if src := strings.TrimSpace(walletSource[row.WalletAddress]); src != "" {
				ev.WalletSource = src
			} else {
				ev.WalletSource = "user_managed"
			}
		} else if src := strings.TrimSpace(walletSource[row.WalletAddress]); src != "" {
			ev.WalletSource = src
		} else {
			ev.WalletSource = "smart_lp_event"
		}

		priceLower, priceUpper, base, quote, okRange := pricing.BuildRangeDisplay(task, ev.TickLower, ev.TickUpper)
		if okRange {
			ev.PriceLower = sanitizeFloat(priceLower)
			ev.PriceUpper = sanitizeFloat(priceUpper)
			ev.PriceBase = strings.TrimSpace(base)
			ev.PriceQuote = strings.TrimSpace(quote)
			if ev.PriceLower > 0 && ev.PriceUpper > 0 {
				ev.AnchorPrice = sanitizeFloat((ev.PriceLower + ev.PriceUpper) / 2)
			}
		}
		if est, ok := pnlEstimates[ev.EventID]; ok && est.HasPnLEstimate {
			ev.HasPnLEstimate = true
			ev.CostBasisUSD = sanitizeFloat(est.CostBasisUSD)
			ev.PnLEstimateUSD = sanitizeFloat(est.PnLEstimateUSD)
		}

		events = append(events, ev)
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].T == events[j].T {
			return events[i].EventID < events[j].EventID
		}
		return events[i].T < events[j].T
	})

	loadedAddCount := 0
	loadedRemoveCount := 0
	loadedWallets := make(map[string]struct{}, len(events))
	for _, ev := range events {
		if strings.ToLower(strings.TrimSpace(ev.Action)) == "remove" {
			loadedRemoveCount++
		} else {
			loadedAddCount++
		}
		addr := strings.ToLower(strings.TrimSpace(ev.WalletAddress))
		if addr != "" {
			loadedWallets[addr] = struct{}{}
		}
	}
	totalEvents := int(summary.TotalEvents)
	addCount := int(summary.AddCount)
	removeCount := int(summary.RemoveCount)
	walletCount := int(summary.WalletCount)
	if summaryErr != nil {
		totalEvents = len(events)
		addCount = loadedAddCount
		removeCount = loadedRemoveCount
		walletCount = len(loadedWallets)
	}
	truncated := summaryErr == nil && totalEvents > len(events)

	writeJSON(w, http.StatusOK, smartMoneyPoolMarkersResponse{
		Chain:       chain,
		BucketSec:   bucketSec,
		WindowSec:   durationSeconds(rangeStart, rangeEnd),
		UpdatedAt:   timeutil.Now(),
		Pool:        outPool,
		Events:      events,
		TotalEvents: totalEvents,
		AddCount:    addCount,
		RemoveCount: removeCount,
		WalletCount: walletCount,
		Truncated:   truncated,
		Warnings:    warnings,
	})
}
