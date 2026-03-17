package web_server

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/models"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type smartMoneyPoolAddsPool struct {
	PoolVersion string `json:"pool_version"`
	PoolID      string `json:"pool_id"`

	Exchange     string  `json:"exchange,omitempty"`
	Pair         string  `json:"pair,omitempty"`
	FeePct       float64 `json:"fee_pct,omitempty"`
	Token0       string  `json:"token0,omitempty"`
	Token1       string  `json:"token1,omitempty"`
	Token0Symbol string  `json:"token0_symbol,omitempty"`
	Token1Symbol string  `json:"token1_symbol,omitempty"`
}

type smartMoneyPoolAddsWalletRow struct {
	WalletAddress string `json:"wallet_address"`

	// Current position reference when SmartLP can resolve the LP NFT.
	TokenID    string `json:"token_id,omitempty"`
	NPMAddress string `json:"npm_address,omitempty"`

	TickLower int `json:"tick_lower"`
	TickUpper int `json:"tick_upper"`

	PriceLower float64 `json:"price_lower,omitempty"`
	PriceUpper float64 `json:"price_upper,omitempty"`
	PriceBase  string  `json:"price_base,omitempty"`
	PriceQuote string  `json:"price_quote,omitempty"`

	EventCount int       `json:"event_count"`
	LastAt     time.Time `json:"last_at"`

	Amount0    float64 `json:"amount0"`
	Amount1    float64 `json:"amount1"`
	Amount0USD float64 `json:"amount0_usd"`
	Amount1USD float64 `json:"amount1_usd"`
	TotalUSD   float64 `json:"total_usd"`

	ClaimableFee0    float64 `json:"claimable_fee0,omitempty"`
	ClaimableFee1    float64 `json:"claimable_fee1,omitempty"`
	ClaimableFeesUSD float64 `json:"claimable_fees_usd,omitempty"`
	FeeStatus        string  `json:"fee_status,omitempty"` // ok|skipped|error
	FeeError         string  `json:"fee_error,omitempty"`
}

type smartMoneyPoolAddsResponse struct {
	Chain     string                        `json:"chain"`
	WindowSec int                           `json:"window_sec"`
	UpdatedAt time.Time                     `json:"updated_at"`
	Pool      smartMoneyPoolAddsPool        `json:"pool"`
	Wallets   []smartMoneyPoolAddsWalletRow `json:"wallets"`
	Warnings  []string                      `json:"warnings,omitempty"`
}

type smartMoneyPoolAddRow struct {
	WalletAddress string
	ContractAddr  string
	TokenID       string
	TickLower     int32
	TickUpper     int32
	Sum0          string
	Sum1          string
	EventCount    uint64
	LastAt        time.Time
}

func normalizeHexID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "0x") {
		return raw
	}
	// Accept "deadbeef..." style inputs.
	if isHex(raw) {
		return "0x" + raw
	}
	return raw
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func smartMoneyPositionKeyExprSQL(chainExpr string, poolVersionExpr string, poolIDExpr string, walletExpr string, contractExpr string, tokenExpr string, tickLowerExpr string, tickUpperExpr string) string {
	return fmt.Sprintf(
		"concat(lowerUTF8(%s), '|', lowerUTF8(%s), '|', lowerUTF8(%s), '|', lowerUTF8(%s), '|', lowerUTF8(%s), '|', %s, '|', toString(%s), '|', toString(%s))",
		chainExpr,
		poolVersionExpr,
		poolIDExpr,
		walletExpr,
		contractExpr,
		tokenExpr,
		tickLowerExpr,
		tickUpperExpr,
	)
}

func smartMoneyPoolAddActivePositionsSQL(chainFilter string) string {
	return fmt.Sprintf(`
		SELECT
			position_key,
			latest_wallet_address AS wallet_address,
			latest_contract_address AS contract_address,
			latest_token_id AS token_id,
			latest_tick_lower AS tick_lower,
			latest_tick_upper AS tick_upper,
			latest_last_add_at AS last_add_at,
			latest_is_active AS is_active
		FROM (
			SELECT
				position_key,
				argMax(wallet_address, tuple(last_event_seq, updated_at)) AS latest_wallet_address,
				argMax(contract_address, tuple(last_event_seq, updated_at)) AS latest_contract_address,
				argMax(token_id, tuple(last_event_seq, updated_at)) AS latest_token_id,
				argMax(tick_lower, tuple(last_event_seq, updated_at)) AS latest_tick_lower,
				argMax(tick_upper, tuple(last_event_seq, updated_at)) AS latest_tick_upper,
				argMax(last_add_at, tuple(last_event_seq, updated_at)) AS latest_last_add_at,
				argMax(is_active, tuple(last_event_seq, updated_at)) AS latest_is_active
			FROM smart_lp_active_positions
			WHERE pool_version = ? AND pool_id = ?
				%s
			GROUP BY position_key
		)
	`, chainFilter)
}

func querySmartMoneyPoolAdds(ctx context.Context, conn smartMoneyClickHouseQueryer, chain string, poolVersion string, poolID string, window time.Duration, limit int) ([]smartMoneyPoolAddRow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if conn == nil {
		return nil, fmt.Errorf("clickhouse not initialized")
	}
	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	poolID = strings.ToLower(strings.TrimSpace(poolID))
	if poolVersion == "" || poolID == "" {
		return []smartMoneyPoolAddRow{}, nil
	}
	if window <= 0 {
		window = 2 * time.Hour
	}
	seconds := int(window.Seconds())
	if seconds <= 0 {
		seconds = 7200
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 5000 {
		limit = 5000
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	chainFilter := ""
	args := make([]any, 0, 6)
	args = append(args, poolVersion, poolID)
	if chain != "" {
		chainFilter = "AND lowerUTF8(chain) = ?"
		args = append(args, chain)
	}

	positionKeyExpr := smartMoneyPositionKeyExprSQL("chain", "pool_version", "pool_id", "wallet_address", "contract_address", "token_id", "tick_lower", "tick_upper")
	activePositions := smartMoneyPoolAddActivePositionsSQL(chainFilter)
	args = append(args, poolVersion, poolID)
	if chain != "" {
		args = append(args, chain)
	}

	q := fmt.Sprintf(`
		SELECT
			active.wallet_address,
			active.contract_address,
			active.token_id,
			active.tick_lower,
			active.tick_upper,
			ifNull(recent.sum0, '0') AS sum0,
			ifNull(recent.sum1, '0') AS sum1,
			ifNull(recent.event_count, toUInt64(0)) AS event_count,
			ifNull(recent.last_at, active.last_add_at) AS last_at
		FROM (%s) AS active
		LEFT JOIN (
			SELECT
				position_key,
				toString(sumIf(toInt256OrZero(amount0), action = 'add')) AS sum0,
				toString(sumIf(toInt256OrZero(amount1), action = 'add')) AS sum1,
				countIf(action = 'add') AS event_count,
				maxIf(ts, action = 'add') AS last_at
			FROM (
				SELECT
					chain,
					pool_version,
					pool_id,
					wallet_address,
					contract_address,
					token_id,
					tick_lower,
					tick_upper,
					action,
					amount0,
					amount1,
					ts,
					%s AS position_key
				FROM smart_lp_events
				WHERE ts >= now() - INTERVAL %d SECOND
					AND action IN ('add', 'remove')
					AND pool_version = ? AND pool_id = ?
					AND wallet_address != ''
					%s
			)
			GROUP BY position_key
			HAVING countIf(action = 'add') > 0
		) AS recent
			ON active.position_key = recent.position_key
		WHERE active.is_active = 1
			AND active.last_add_at >= now() - INTERVAL %d SECOND
		ORDER BY ifNull(recent.last_at, active.last_add_at) DESC, ifNull(recent.event_count, toUInt64(0)) DESC, active.wallet_address ASC
		LIMIT %d
	`, activePositions, positionKeyExpr, seconds, chainFilter, seconds, limit)

	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]smartMoneyPoolAddRow, 0, limit)
	for rows.Next() {
		var r smartMoneyPoolAddRow
		var tickL int32
		var tickU int32
		if err := rows.Scan(&r.WalletAddress, &r.ContractAddr, &r.TokenID, &tickL, &tickU, &r.Sum0, &r.Sum1, &r.EventCount, &r.LastAt); err != nil {
			return nil, err
		}
		r.WalletAddress = strings.ToLower(strings.TrimSpace(r.WalletAddress))
		r.ContractAddr = strings.ToLower(strings.TrimSpace(r.ContractAddr))
		r.TokenID = strings.TrimSpace(r.TokenID)
		r.TickLower = tickL
		r.TickUpper = tickU
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func querySmartMoneyPoolAddsStable(ctx context.Context, conn smartMoneyClickHouseQueryer, chain string, poolVersion string, poolID string, window time.Duration, limit int) ([]smartMoneyPoolAddRow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if conn == nil {
		return nil, fmt.Errorf("clickhouse not initialized")
	}
	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	poolID = strings.ToLower(strings.TrimSpace(poolID))
	if poolVersion == "" || poolID == "" {
		return []smartMoneyPoolAddRow{}, nil
	}
	if window <= 0 {
		window = 2 * time.Hour
	}
	seconds := int(window.Seconds())
	if seconds <= 0 {
		seconds = 7200
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 5000 {
		limit = 5000
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	chainFilter := ""
	args := make([]any, 0, 6)
	args = append(args, poolVersion, poolID)
	if chain != "" {
		chainFilter = "AND lowerUTF8(chain) = ?"
		args = append(args, chain)
	}

	positionKeyExpr := smartMoneyPositionKeyExprSQL("chain", "pool_version", "pool_id", "wallet_address", "contract_address", "token_id", "tick_lower", "tick_upper")
	activePositions := smartMoneyPoolAddActivePositionsSQL(chainFilter)
	dedupRecentEvents := fmt.Sprintf(`
		SELECT
			argMax(chain, event_seq) AS dedup_chain,
			argMax(pool_version, event_seq) AS dedup_pool_version,
			argMax(pool_id, event_seq) AS dedup_pool_id,
			argMax(wallet_address, event_seq) AS dedup_wallet_address,
			argMax(contract_address, event_seq) AS dedup_contract_address,
			argMax(token_id, event_seq) AS dedup_token_id,
			argMax(tick_lower, event_seq) AS dedup_tick_lower,
			argMax(tick_upper, event_seq) AS dedup_tick_upper,
			argMax(amount0, event_seq) AS dedup_amount0,
			argMax(amount1, event_seq) AS dedup_amount1,
			argMax(action, event_seq) AS dedup_action,
			max(ts) AS event_ts,
			max(event_seq) AS dedup_event_seq
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND action IN ('add', 'remove')
			AND pool_version = ? AND pool_id = ?
			AND wallet_address != ''
			%s
		GROUP BY tx_hash, log_index
	`, seconds, chainFilter)

	args = append(args, poolVersion, poolID)
	if chain != "" {
		args = append(args, chain)
	}

	q := fmt.Sprintf(`
		SELECT
			active.wallet_address,
			active.contract_address,
			active.token_id,
			active.tick_lower,
			active.tick_upper,
			ifNull(recent.sum0, '0') AS sum0,
			ifNull(recent.sum1, '0') AS sum1,
			ifNull(recent.event_count, toUInt64(0)) AS event_count,
			ifNull(recent.last_at, active.last_add_at) AS last_at
		FROM (%s) AS active
		LEFT JOIN (
			SELECT
				position_key,
				toString(sumIf(toInt256OrZero(amount0), dedup_action = 'add')) AS sum0,
				toString(sumIf(toInt256OrZero(amount1), dedup_action = 'add')) AS sum1,
				countIf(dedup_action = 'add') AS event_count,
				maxIf(event_ts, dedup_action = 'add') AS last_at
			FROM (
				SELECT
					dedup_chain AS chain,
					dedup_pool_version AS pool_version,
					dedup_pool_id AS pool_id,
					dedup_wallet_address AS wallet_address,
					dedup_contract_address AS contract_address,
					dedup_token_id AS token_id,
					dedup_tick_lower AS tick_lower,
					dedup_tick_upper AS tick_upper,
					dedup_amount0 AS amount0,
					dedup_amount1 AS amount1,
					dedup_action,
					event_ts,
					%s AS position_key
				FROM (%s)
			)
			GROUP BY position_key
			HAVING countIf(dedup_action = 'add') > 0
		) AS recent
			ON active.position_key = recent.position_key
		WHERE active.is_active = 1
			AND active.last_add_at >= now() - INTERVAL %d SECOND
		ORDER BY ifNull(recent.last_at, active.last_add_at) DESC, ifNull(recent.event_count, toUInt64(0)) DESC, active.wallet_address ASC
		LIMIT %d
	`, activePositions, positionKeyExpr, dedupRecentEvents, seconds, limit)

	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]smartMoneyPoolAddRow, 0, limit)
	for rows.Next() {
		var r smartMoneyPoolAddRow
		var tickL int32
		var tickU int32
		if err := rows.Scan(&r.WalletAddress, &r.ContractAddr, &r.TokenID, &tickL, &tickU, &r.Sum0, &r.Sum1, &r.EventCount, &r.LastAt); err != nil {
			return nil, err
		}
		r.WalletAddress = strings.ToLower(strings.TrimSpace(r.WalletAddress))
		r.ContractAddr = strings.ToLower(strings.TrimSpace(r.ContractAddr))
		r.TokenID = strings.TrimSpace(r.TokenID)
		r.TickLower = tickL
		r.TickUpper = tickU
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func isClickHouseMemoryLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "memory_limit_exceeded") ||
		strings.Contains(msg, "memory limit exceeded") ||
		strings.Contains(msg, "overcommittracker")
}

func querySmartMoneyPoolAddsBestEffort(ctx context.Context, conn smartMoneyClickHouseQueryer, chain string, poolVersion string, poolID string, window time.Duration, limit int) ([]smartMoneyPoolAddRow, bool, []string, error) {
	rows, err := querySmartMoneyPoolAddsStable(ctx, conn, chain, poolVersion, poolID, window, limit)
	if err == nil {
		return rows, false, nil, nil
	}
	if !isClickHouseMemoryLimitError(err) {
		return nil, false, nil, err
	}

	fallbackRows, fallbackErr := querySmartMoneyPoolAdds(ctx, conn, chain, poolVersion, poolID, window, limit)
	if fallbackErr != nil {
		return nil, false, nil, fmt.Errorf("stable query failed: %w; fallback query failed: %v", err, fallbackErr)
	}

	warnings := []string{
		"pool adds query exceeded ClickHouse memory and fell back to lightweight aggregation",
	}
	return fallbackRows, true, warnings, nil
}

type smartMoneyPoolAddLiveResolver interface {
	Resolve(ctx context.Context, ref smartMoneyPositionRef) (*smartMoneyResolvedPosition, error)
}

type smartMoneyPoolAddV4FallbackLoader func(ctx context.Context, walletAddr string, pools []smartMoneyWalletV4PoolRef, limit int) ([]smartMoneyPositionRef, error)

func (r *smartMoneyPositionResolver) canResolvePoolVersion(poolVersion string) bool {
	if r == nil || blockchain.Client == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(poolVersion)) {
	case "v3":
		return len(r.v3Managers) > 0
	case "v4":
		return r.canResolveV4()
	default:
		return false
	}
}

func smartMoneyPositionRefKey(ref smartMoneyPositionRef) string {
	return strings.ToLower(strings.TrimSpace(ref.PoolVersion)) + "|" +
		strings.ToLower(strings.TrimSpace(ref.PoolID)) + "|" +
		strings.ToLower(strings.TrimSpace(ref.WalletAddress)) + "|" +
		strings.TrimSpace(ref.TokenID) + "|" +
		fmt.Sprintf("%d", ref.TickLower) + "|" +
		fmt.Sprintf("%d", ref.TickUpper)
}

func smartMoneyPoolAddsWalletKey(poolVersion string, poolID string, row smartMoneyPoolAddsWalletRow) string {
	return smartMoneyPositionRefKey(smartMoneyPositionRef{
		PoolVersion:     poolVersion,
		PoolID:          poolID,
		WalletAddress:   row.WalletAddress,
		TokenID:         row.TokenID,
		ContractAddress: row.NPMAddress,
		TickLower:       row.TickLower,
		TickUpper:       row.TickUpper,
	})
}

func resolveActiveSmartMoneyPoolAddRows(
	ctx context.Context,
	poolVersion string,
	poolID string,
	wallets []smartMoneyPoolAddsWalletRow,
	resolver smartMoneyPoolAddLiveResolver,
	loadV4Fallback smartMoneyPoolAddV4FallbackLoader,
) ([]smartMoneyPoolAddsWalletRow, []*smartMoneyResolvedPosition, []string) {
	if resolver == nil || len(wallets) == 0 {
		return wallets, nil, nil
	}

	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	poolID = strings.ToLower(strings.TrimSpace(poolID))

	active := make([]smartMoneyPoolAddsWalletRow, 0, len(wallets))
	resolved := make([]*smartMoneyResolvedPosition, 0, len(wallets))
	filtered := 0
	fallbackCache := make(map[string][]smartMoneyPositionRef)

	for _, row := range wallets {
		ref := smartMoneyPositionRef{
			WalletAddress:   strings.ToLower(strings.TrimSpace(row.WalletAddress)),
			PoolVersion:     poolVersion,
			PoolID:          poolID,
			ContractAddress: strings.ToLower(strings.TrimSpace(row.NPMAddress)),
			TokenID:         strings.TrimSpace(row.TokenID),
			TickLower:       row.TickLower,
			TickUpper:       row.TickUpper,
		}

		if ref.TokenID == "" && poolVersion == "v4" && loadV4Fallback != nil {
			cacheKey := ref.WalletAddress + "|" + poolID
			legacyRefs, ok := fallbackCache[cacheKey]
			if !ok {
				scanned, err := loadV4Fallback(ctx, ref.WalletAddress, []smartMoneyWalletV4PoolRef{{PoolID: poolID}}, 20)
				if err != nil {
					scanned = []smartMoneyPositionRef{}
				}
				fallbackCache[cacheKey] = scanned
				legacyRefs = scanned
			}
			match, found := findSmartMoneyV4FallbackRef(legacyRefs, poolID, ref.TickLower, ref.TickUpper)
			if !found {
				filtered++
				continue
			}
			ref = match
			row.TokenID = match.TokenID
		}

		position, _ := resolver.Resolve(ctx, ref)
		if position == nil {
			filtered++
			continue
		}

		if strings.TrimSpace(position.PositionID) != "" {
			row.TokenID = strings.TrimSpace(position.PositionID)
		}
		if poolVersion == "v3" && strings.TrimSpace(position.ContractAddress) != "" {
			row.NPMAddress = strings.ToLower(strings.TrimSpace(position.ContractAddress))
		}

		active = append(active, row)
		resolved = append(resolved, position)
	}

	warnings := make([]string, 0, 1)
	if filtered > 0 {
		warnings = append(warnings, fmt.Sprintf("filtered %d stale rows without active live positions", filtered))
	}

	return active, resolved, warnings
}

func (s *Server) handleSmartMoneyPoolAdds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	poolVersion := strings.ToLower(strings.TrimSpace(query.Get("pool_version")))
	poolID := strings.ToLower(strings.TrimSpace(query.Get("pool_id")))
	if poolVersion == "" || poolID == "" {
		http.Error(w, "pool_version and pool_id required", http.StatusBadRequest)
		return
	}
	switch poolVersion {
	case "v3", "v4":
	default:
		http.Error(w, "invalid pool_version", http.StatusBadRequest)
		return
	}
	poolID = normalizeHexID(poolID)
	if poolVersion == "v3" {
		if !common.IsHexAddress(poolID) {
			http.Error(w, "invalid pool_id", http.StatusBadRequest)
			return
		}
	} else {
		// V4 poolId is bytes32 (0x + 64 hex chars).
		if !strings.HasPrefix(poolID, "0x") || len(poolID) != 66 || !isHex(poolID[2:]) {
			http.Error(w, "invalid pool_id", http.StatusBadRequest)
			return
		}
	}

	if s.ClickHouse == nil || s.ClickHouse.Conn == nil {
		http.Error(w, "ClickHouse not configured", http.StatusServiceUnavailable)
		return
	}

	chain := strings.ToLower(strings.TrimSpace(query.Get("chain")))
	if chain == "" {
		chain = "bsc"
	}

	windowHours := parseIntQuery(query, "window_hours", 2, 1, 168)
	limit := parseIntQuery(query, "limit", 60, 1, 200)
	feesLimit := parseIntQuery(query, "fees_limit", 30, 0, 100)

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

	ctx, cancel := context.WithTimeout(r.Context(), 22*time.Second)
	defer cancel()

	window := time.Duration(windowHours) * time.Hour

	rows, _, queryWarnings, qerr := querySmartMoneyPoolAddsBestEffort(ctx, s.ClickHouse.Conn, chain, poolVersion, poolID, window, limit)
	if qerr != nil {
		http.Error(w, qerr.Error(), http.StatusInternalServerError)
		return
	}

	poolSvc := pool.NewPoolService()
	info, perr := poolSvc.GetPoolInfoForVersionCached(chain, poolVersion, poolID)

	warnings := make([]string, 0, 4+len(queryWarnings))
	warnings = append(warnings, queryWarnings...)
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

	// Prepare pricing + decimals caches.
	decimalsCache := make(map[string]int)
	priceSvc := s.TokenPrice
	if priceSvc == nil {
		priceSvc = pricing.NewTokenPriceService()
	}
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

	task := &models.StrategyTask{
		PoolId:        poolID,
		PoolVersion:   poolVersion,
		Token0Symbol:  strings.TrimSpace(outPool.Token0Symbol),
		Token1Symbol:  strings.TrimSpace(outPool.Token1Symbol),
		Token0Address: strings.TrimSpace(outPool.Token0),
		Token1Address: strings.TrimSpace(outPool.Token1),
	}

	wallets := make([]smartMoneyPoolAddsWalletRow, 0, len(rows))
	for _, row := range rows {
		amt0 := amountToFloat(row.Sum0, dec0)
		amt1 := amountToFloat(row.Sum1, dec1)

		usd0 := sanitizeFloat(amt0 * p0)
		usd1 := sanitizeFloat(amt1 * p1)
		total := sanitizeFloat(usd0 + usd1)

		priceLower, priceUpper, base, quote, okRange := pricing.BuildRangeDisplay(task, int(row.TickLower), int(row.TickUpper))
		item := smartMoneyPoolAddsWalletRow{
			WalletAddress: row.WalletAddress,
			TokenID:       row.TokenID,
			NPMAddress:    row.ContractAddr,
			TickLower:     int(row.TickLower),
			TickUpper:     int(row.TickUpper),
			EventCount:    int(row.EventCount),
			LastAt:        row.LastAt,
			Amount0:       sanitizeFloat(amt0),
			Amount1:       sanitizeFloat(amt1),
			Amount0USD:    usd0,
			Amount1USD:    usd1,
			TotalUSD:      total,
			FeeStatus:     "skipped",
		}
		if okRange {
			item.PriceLower = sanitizeFloat(priceLower)
			item.PriceUpper = sanitizeFloat(priceUpper)
			item.PriceBase = strings.TrimSpace(base)
			item.PriceQuote = strings.TrimSpace(quote)
		}
		wallets = append(wallets, item)
	}

	var (
		resolver       *smartMoneyPositionResolver
		resolvedByKey  map[string]*smartMoneyResolvedPosition
		resolverInited bool
	)
	canResolveLive := false
	if feesLimit > 0 && len(wallets) > 0 {
		resolver, resolverWarnings := newSmartMoneyPositionResolver(newSmartMoneyTokenMetaCache())
		warnings = append(warnings, resolverWarnings...)
		resolverInited = true
		resolvedByKey = make(map[string]*smartMoneyResolvedPosition, len(wallets))
		canResolveLive = resolver.canResolvePoolVersion(poolVersion)
		if !canResolveLive {
			warnings = append(warnings, "claimable fee estimation unavailable for this pool version")
		}
	}

	if feesLimit > 0 && canResolveLive {
		if !resolverInited {
			resolver, _ = newSmartMoneyPositionResolver(newSmartMoneyTokenMetaCache())
			resolvedByKey = make(map[string]*smartMoneyResolvedPosition, len(wallets))
			resolverInited = true
		}

		candidates := make([]int, 0, len(wallets))
		for i := range wallets {
			if strings.TrimSpace(wallets[i].TokenID) != "" || poolVersion == "v4" {
				candidates = append(candidates, i)
			}
		}

		if feesLimit < len(candidates) {
			warnings = append(warnings, fmt.Sprintf("claimable fee estimation limited: computed %d/%d positions", feesLimit, len(candidates)))
			candidates = candidates[:feesLimit]
		}

		fallbackCache := make(map[string][]smartMoneyPositionRef)
		legacyMatched := 0

		for _, idx := range candidates {
			ref := smartMoneyPositionRef{
				WalletAddress:   wallets[idx].WalletAddress,
				PoolVersion:     poolVersion,
				PoolID:          poolID,
				ContractAddress: wallets[idx].NPMAddress,
				TokenID:         wallets[idx].TokenID,
				TickLower:       wallets[idx].TickLower,
				TickUpper:       wallets[idx].TickUpper,
			}

			if ref.TokenID == "" && poolVersion == "v4" {
				cacheKey := strings.ToLower(strings.TrimSpace(wallets[idx].WalletAddress)) + "|" + poolID
				legacyRefs, ok := fallbackCache[cacheKey]
				if !ok {
					scanned, scanErr := scanSmartMoneyV4FallbackRefs(ctx, wallets[idx].WalletAddress, []smartMoneyWalletV4PoolRef{{PoolID: poolID}}, 20)
					if scanErr != nil {
						wallets[idx].FeeStatus = "error"
						wallets[idx].FeeError = truncateErr(scanErr, 120)
						fallbackCache[cacheKey] = []smartMoneyPositionRef{}
						continue
					}
					fallbackCache[cacheKey] = scanned
					legacyRefs = scanned
				}
				match, found := findSmartMoneyV4FallbackRef(legacyRefs, poolID, wallets[idx].TickLower, wallets[idx].TickUpper)
				if !found {
					wallets[idx].FeeStatus = "skipped"
					wallets[idx].FeeError = "token_id missing"
					continue
				}
				ref = match
				wallets[idx].TokenID = match.TokenID
				legacyMatched++
			}

			key := smartMoneyPositionRefKey(ref)
			resolved := resolvedByKey[key]
			var resolveErr error
			if resolved == nil {
				resolved, resolveErr = resolver.Resolve(ctx, ref)
				if resolved != nil {
					resolvedByKey[key] = resolved
				}
			}
			if resolveErr != nil || resolved == nil {
				if strings.TrimSpace(ref.TokenID) != "" {
					wallets[idx].FeeStatus = "error"
					wallets[idx].FeeError = truncateErr(resolveErr, 120)
				}
				continue
			}

			wallets[idx].TokenID = resolved.PositionID
			if poolVersion == "v3" && resolved.ContractAddress != "" {
				wallets[idx].NPMAddress = resolved.ContractAddress
			}
			wallets[idx].ClaimableFee0 = resolved.ClaimableFee0
			wallets[idx].ClaimableFee1 = resolved.ClaimableFee1
			wallets[idx].ClaimableFeesUSD = sanitizeFloat(resolved.ClaimableFee0*p0 + resolved.ClaimableFee1*p1)
			wallets[idx].FeeStatus = resolved.FeeStatus
			wallets[idx].FeeError = resolved.FeeError
		}

		if legacyMatched > 0 {
			warnings = append(warnings, fmt.Sprintf("used legacy V4 NFT fallback for %d pool rows", legacyMatched))
		}
	}

	// Sort wallets by last_at desc, then total_usd desc.
	sort.Slice(wallets, func(i, j int) bool {
		if !wallets[i].LastAt.Equal(wallets[j].LastAt) {
			return wallets[i].LastAt.After(wallets[j].LastAt)
		}
		if wallets[i].TotalUSD != wallets[j].TotalUSD {
			return wallets[i].TotalUSD > wallets[j].TotalUSD
		}
		return wallets[i].WalletAddress < wallets[j].WalletAddress
	})

	resp := smartMoneyPoolAddsResponse{
		Chain:     chain,
		WindowSec: int(window.Seconds()),
		UpdatedAt: time.Now(),
		Pool:      outPool,
		Wallets:   wallets,
		Warnings:  warnings,
	}
	writeJSON(w, http.StatusOK, resp)
}

func truncateErr(err error, max int) string {
	if err == nil {
		return ""
	}
	s := strings.TrimSpace(err.Error())
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
