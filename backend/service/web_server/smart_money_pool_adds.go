package web_server

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/models"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"context"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/errgroup"
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

	// V3-only identifiers (used for fee simulation).
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
	args := make([]any, 0, 3)
	args = append(args, poolVersion, poolID)
	if chain != "" {
		chainFilter = "AND lowerUTF8(chain) = ?"
		args = append(args, chain)
	}

	// Aggregate per-wallet/position add+remove events for the time window.
	// We keep only positions that have a positive net liquidity delta (i.e., added more than removed),
	// so "add then fully撤销" will not show up in the results.
	netLiqExpr := "sum(if(action='add', toInt256OrZero(liquidity_delta), -toInt256OrZero(liquidity_delta)))"
	if poolVersion == "v4" {
		// V4 `liquidity_delta` is already signed (ModifyLiquidity int256).
		netLiqExpr = "sum(toInt256OrZero(liquidity_delta))"
	}
	q := fmt.Sprintf(`
		SELECT
			wallet_address,
			contract_address,
			token_id,
			tick_lower,
			tick_upper,
			toString(sumIf(toInt256OrZero(amount0), action = 'add')) AS sum0,
			toString(sumIf(toInt256OrZero(amount1), action = 'add')) AS sum1,
			countIf(action='add') AS event_count,
			maxIf(ts, action='add') AS last_at
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND action IN ('add', 'remove')
			AND pool_version = ? AND pool_id = ?
			AND wallet_address != ''
			%s
		GROUP BY wallet_address, contract_address, token_id, tick_lower, tick_upper
		HAVING %s > 0
		ORDER BY last_at DESC, event_count DESC, wallet_address ASC
		LIMIT %d
	`, seconds, chainFilter, netLiqExpr, limit)

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

const v3CollectMinABI = `[
  {
    "inputs": [
      {
        "components": [
          { "internalType": "uint256", "name": "tokenId", "type": "uint256" },
          { "internalType": "address", "name": "recipient", "type": "address" },
          { "internalType": "uint128", "name": "amount0Max", "type": "uint128" },
          { "internalType": "uint128", "name": "amount1Max", "type": "uint128" }
        ],
        "internalType": "struct INonfungiblePositionManager.CollectParams",
        "name": "params",
        "type": "tuple"
      }
    ],
    "name": "collect",
    "outputs": [
      { "internalType": "uint256", "name": "amount0", "type": "uint256" },
      { "internalType": "uint256", "name": "amount1", "type": "uint256" }
    ],
    "stateMutability": "payable",
    "type": "function"
  }
]`

type v3CollectParams struct {
	TokenId    *big.Int       `abi:"tokenId"`
	Recipient  common.Address `abi:"recipient"`
	Amount0Max *big.Int       `abi:"amount0Max"`
	Amount1Max *big.Int       `abi:"amount1Max"`
}

var maxUint128 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))

func simulateV3Collect(ctx context.Context, npm common.Address, from common.Address, tokenID *big.Int) (*big.Int, *big.Int, error) {
	if blockchain.Client == nil {
		return nil, nil, fmt.Errorf("blockchain client not initialized")
	}
	if (npm == common.Address{}) || (from == common.Address{}) || tokenID == nil || tokenID.Sign() <= 0 {
		return nil, nil, fmt.Errorf("invalid collect args")
	}
	parsedABI, err := abi.JSON(strings.NewReader(v3CollectMinABI))
	if err != nil {
		return nil, nil, err
	}
	data, err := parsedABI.Pack("collect", v3CollectParams{
		TokenId:    tokenID,
		Recipient:  from,
		Amount0Max: maxUint128,
		Amount1Max: maxUint128,
	})
	if err != nil {
		return nil, nil, err
	}
	msg := ethereum.CallMsg{
		From:  from,
		To:    &npm,
		Data:  data,
		Value: big.NewInt(0),
		Gas:   1_000_000,
	}
	callCtx := ctx
	if callCtx == nil {
		callCtx = context.Background()
	}
	raw, err := blockchain.Client.CallContract(callCtx, msg, nil)
	if err != nil {
		return nil, nil, err
	}
	out, err := parsedABI.Unpack("collect", raw)
	if err != nil {
		return nil, nil, err
	}
	if len(out) < 2 {
		return nil, nil, fmt.Errorf("unexpected collect return length: %d", len(out))
	}
	amt0, ok0 := out[0].(*big.Int)
	amt1, ok1 := out[1].(*big.Int)
	if !ok0 || amt0 == nil || !ok1 || amt1 == nil {
		return nil, nil, fmt.Errorf("unexpected collect return types: %T %T", out[0], out[1])
	}
	return amt0, amt1, nil
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

	rows, qerr := querySmartMoneyPoolAdds(ctx, s.ClickHouse.Conn, chain, poolVersion, poolID, window, limit)
	if qerr != nil {
		http.Error(w, qerr.Error(), http.StatusInternalServerError)
		return
	}

	poolSvc := pool.NewPoolService()
	var info *pool.PoolInfo
	var perr error
	if poolVersion == "v4" {
		info, perr = poolSvc.GetV4PoolInfo(poolID)
	} else {
		info, perr = poolSvc.GetPoolInfo(poolID)
	}

	warnings := make([]string, 0, 4)
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

	// Attempt V3 claimable fee simulation for top rows.
	feeCandidates := make([]int, 0, len(wallets))
	for i := range wallets {
		if poolVersion != "v3" {
			break
		}
		if strings.TrimSpace(wallets[i].TokenID) == "" {
			continue
		}
		if !common.IsHexAddress(wallets[i].NPMAddress) {
			continue
		}
		feeCandidates = append(feeCandidates, i)
	}

	if poolVersion == "v3" && feesLimit > 0 && len(feeCandidates) > 0 {
		if blockchain.Client == nil {
			warnings = append(warnings, "blockchain client not initialized; cannot compute claimable fees")
		} else {
			if feesLimit < len(feeCandidates) {
				warnings = append(warnings, fmt.Sprintf("claimable fee simulation limited: computed %d/%d positions", feesLimit, len(feeCandidates)))
				feeCandidates = feeCandidates[:feesLimit]
			}

			pmCache := make(map[common.Address]*blockchain.V3PositionManager)
			var pmMu sync.Mutex

			g, gctx := errgroup.WithContext(ctx)
			g.SetLimit(6)

			for _, idx := range feeCandidates {
				idx := idx
				g.Go(func() error {
					if gctx.Err() != nil {
						return gctx.Err()
					}
					npmAddr := common.HexToAddress(wallets[idx].NPMAddress)
					tokenBI, ok := new(big.Int).SetString(strings.TrimSpace(wallets[idx].TokenID), 10)
					if !ok || tokenBI == nil || tokenBI.Sign() <= 0 {
						return nil
					}

					pmMu.Lock()
					pm := pmCache[npmAddr]
					pmMu.Unlock()
					if pm == nil {
						newPM, err := blockchain.NewV3PositionManager(npmAddr, blockchain.Client)
						if err != nil {
							pmMu.Lock()
							pmCache[npmAddr] = nil
							pmMu.Unlock()
							return nil
						}
						pmMu.Lock()
						pmCache[npmAddr] = newPM
						pmMu.Unlock()
						pm = newPM
					}
					if pm == nil {
						return nil
					}

					callCtx, cancel := context.WithTimeout(gctx, 7*time.Second)
					owner, ownerErr := pm.OwnerOf(&bind.CallOpts{Context: callCtx}, tokenBI)
					cancel()
					if ownerErr != nil || owner == (common.Address{}) {
						wallets[idx].FeeStatus = "error"
						wallets[idx].FeeError = "ownerOf failed"
						return nil
					}

					collectCtx, cancel2 := context.WithTimeout(gctx, 8*time.Second)
					amt0, amt1, cerr := simulateV3Collect(collectCtx, npmAddr, owner, tokenBI)
					cancel2()
					if cerr != nil {
						wallets[idx].FeeStatus = "error"
						wallets[idx].FeeError = truncateErr(cerr, 80)
						return nil
					}

					fee0 := amountToFloat(amt0.String(), dec0)
					fee1 := amountToFloat(amt1.String(), dec1)
					feeUsd := sanitizeFloat(fee0*p0 + fee1*p1)

					wallets[idx].ClaimableFee0 = sanitizeFloat(fee0)
					wallets[idx].ClaimableFee1 = sanitizeFloat(fee1)
					wallets[idx].ClaimableFeesUSD = feeUsd
					wallets[idx].FeeStatus = "ok"
					return nil
				})
			}
			_ = g.Wait()
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
