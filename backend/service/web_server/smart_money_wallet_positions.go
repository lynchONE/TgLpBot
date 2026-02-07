package web_server

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
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
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/sync/errgroup"
)

type smartMoneyWalletPositionsResponse struct {
	Chain         string                       `json:"chain"`
	WalletAddress string                       `json:"wallet_address"`
	WindowSec     int                          `json:"window_sec"`
	UpdatedAt     time.Time                    `json:"updated_at"`
	Positions     []smartMoneyWalletLPPosition `json:"positions"`
	Warnings      []string                     `json:"warnings,omitempty"`
}

type smartMoneyWalletLPPosition struct {
	PoolVersion string `json:"pool_version"`
	PoolID      string `json:"pool_id"`
	PositionID  string `json:"position_id"`

	Exchange string  `json:"exchange,omitempty"`
	Pair     string  `json:"pair,omitempty"`
	FeePct   float64 `json:"fee_pct,omitempty"`

	CurrentTick int  `json:"current_tick,omitempty"`
	TickLower   int  `json:"tick_lower"`
	TickUpper   int  `json:"tick_upper"`
	InRange     bool `json:"in_range,omitempty"`

	Liquidity string `json:"liquidity,omitempty"`

	Token0       string `json:"token0"`
	Token1       string `json:"token1"`
	Token0Symbol string `json:"token0_symbol,omitempty"`
	Token1Symbol string `json:"token1_symbol,omitempty"`
	Token0Dec    int    `json:"token0_decimals,omitempty"`
	Token1Dec    int    `json:"token1_decimals,omitempty"`

	Amount0     float64 `json:"amount0"`
	Amount1     float64 `json:"amount1"`
	Amount0USD  float64 `json:"amount0_usd"`
	Amount1USD  float64 `json:"amount1_usd"`
	PositionUSD float64 `json:"position_usd"`
}

type smartMoneyWalletV3TokenRef struct {
	NPMAddress  string
	TokenID     string
	PoolID      string
	PoolVersion string
	LastEvent   uint64
}

type smartMoneyWalletV4PoolRef struct {
	PoolID string
}

func (s *Server) handleSmartMoneyWalletPositions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ClickHouse == nil || s.ClickHouse.Conn == nil {
		http.Error(w, "ClickHouse not configured", http.StatusServiceUnavailable)
		return
	}
	if config.AppConfig == nil {
		http.Error(w, "config not loaded", http.StatusInternalServerError)
		return
	}

	query := r.URL.Query()
	chain := strings.ToLower(strings.TrimSpace(query.Get("chain")))
	if chain == "" {
		chain = "bsc"
	}
	walletAddrRaw := strings.ToLower(strings.TrimSpace(query.Get("wallet_address")))
	if walletAddrRaw == "" {
		http.Error(w, "wallet_address required", http.StatusBadRequest)
		return
	}
	if !common.IsHexAddress(walletAddrRaw) {
		http.Error(w, "invalid wallet_address", http.StatusBadRequest)
		return
	}
	walletAddr := strings.ToLower(common.HexToAddress(walletAddrRaw).Hex())

	windowHours := parseIntQuery(query, "window_hours", 24, 1, 168)
	limit := parseIntQuery(query, "limit", 30, 1, 120)

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

	refs, err := querySmartMoneyWalletRecentV3Positions(ctx, s.ClickHouse.Conn, chain, walletAddr, window, limit*4)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	v4Pools, _ := querySmartMoneyWalletRecentV4Pools(ctx, s.ClickHouse.Conn, chain, walletAddr, window, 200)

	warnings := make([]string, 0, 4)
	out := make([]smartMoneyWalletLPPosition, 0, len(refs))

	metaCache := newSmartMoneyTokenMetaCache()

	priceTokensSet := make(map[string]struct{})
	priceTokens := make([]string, 0, 2*len(refs))

	// Fetch and compute V3 positions.
	type v3Computed struct {
		pos smartMoneyWalletLPPosition
	}
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(6)

	for _, ref := range refs {
		ref := ref
		g.Go(func() error {
			if gctx.Err() != nil {
				return gctx.Err()
			}
			npmStr := strings.ToLower(strings.TrimSpace(ref.NPMAddress))
			if !common.IsHexAddress(npmStr) {
				return nil
			}
			tokenID, ok := new(big.Int).SetString(strings.TrimSpace(ref.TokenID), 10)
			if !ok || tokenID == nil || tokenID.Sign() <= 0 {
				return nil
			}

			npmAddr := common.HexToAddress(npmStr)
			pm, pmErr := blockchain.NewV3PositionManager(npmAddr, blockchain.Client)
			if pmErr != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("init V3 position manager failed: %s", npmAddr.Hex()))
				mu.Unlock()
				return nil
			}

			info, pErr := pm.Positions(&bind.CallOpts{Context: gctx}, tokenID)
			if pErr != nil || info == nil {
				return nil
			}
			if info.Liquidity == nil || info.Liquidity.Sign() == 0 {
				return nil
			}

			poolID := strings.ToLower(strings.TrimSpace(ref.PoolID))
			if !common.IsHexAddress(poolID) {
				poolID = ""
			}

			currentTick := 0
			sqrtP := big.NewInt(0)
			if poolID != "" {
				sp, t, slotErr := getV3Slot0WithTimeout(gctx, common.HexToAddress(poolID))
				if slotErr == nil && sp != nil {
					sqrtP = sp
					currentTick = t
				}
			}

			t0 := strings.ToLower(strings.TrimSpace(info.Token0.Hex()))
			t1 := strings.ToLower(strings.TrimSpace(info.Token1.Hex()))
			dec0 := metaCache.Decimals(t0)
			dec1 := metaCache.Decimals(t1)
			sym0 := metaCache.Symbol(t0)
			sym1 := metaCache.Symbol(t1)

			sqrtA, _ := pool.SqrtRatioAtTick(int32(info.TickLower))
			sqrtB, _ := pool.SqrtRatioAtTick(int32(info.TickUpper))
			amt0Raw, amt1Raw := pool.AmountsForLiquidity(sqrtP, sqrtA, sqrtB, info.Liquidity)

			amt0 := amountToFloat(amountToString(amt0Raw), dec0)
			amt1 := amountToFloat(amountToString(amt1Raw), dec1)

			inRange := currentTick >= info.TickLower && currentTick <= info.TickUpper
			exchange := v3ExchangeLabel(npmAddr)
			feePct := 0.0
			if info.Fee > 0 {
				feePct = float64(info.Fee) / 10000.0
			}

			pair := strings.TrimSpace(sym0 + "/" + sym1)
			if pair == "/" {
				pair = ""
			}

			computed := smartMoneyWalletLPPosition{
				PoolVersion:  "v3",
				PoolID:       poolID,
				PositionID:   tokenID.String(),
				Exchange:     exchange,
				Pair:         pair,
				FeePct:       feePct,
				CurrentTick:  currentTick,
				TickLower:    info.TickLower,
				TickUpper:    info.TickUpper,
				InRange:      inRange,
				Liquidity:    info.Liquidity.String(),
				Token0:       t0,
				Token1:       t1,
				Token0Symbol: sym0,
				Token1Symbol: sym1,
				Token0Dec:    dec0,
				Token1Dec:    dec1,
				Amount0:      amt0,
				Amount1:      amt1,
			}

			mu.Lock()
			out = append(out, computed)
			if common.IsHexAddress(t0) {
				if _, ok := priceTokensSet[t0]; !ok {
					priceTokensSet[t0] = struct{}{}
					priceTokens = append(priceTokens, t0)
				}
			}
			if common.IsHexAddress(t1) {
				if _, ok := priceTokensSet[t1]; !ok {
					priceTokensSet[t1] = struct{}{}
					priceTokens = append(priceTokens, t1)
				}
			}
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	// Best-effort V4 support: only when configured and the wallet has recent v4 pool activity in the same window.
	if len(v4Pools) > 0 {
		v4Warns, v4Pos, v4Tokens := s.loadV4WalletPositions(ctx, chain, walletAddr, v4Pools, limit*2, metaCache)
		if len(v4Warns) > 0 {
			warnings = append(warnings, v4Warns...)
		}
		out = append(out, v4Pos...)
		for _, t := range v4Tokens {
			if _, ok := priceTokensSet[t]; !ok {
				priceTokensSet[t] = struct{}{}
				priceTokens = append(priceTokens, t)
			}
		}
	}

	// Fetch prices and compute USD valuation.
	priceSvc := s.TokenPrice
	if priceSvc == nil {
		priceSvc = pricing.NewTokenPriceService()
	}
	prices, perr := priceSvc.GetUSDPrices(chain, priceTokens)
	if perr != nil {
		warnings = append(warnings, "price provider limited/rate-limited; using cached/fallback prices where available")
	}
	for i := range out {
		t0 := strings.ToLower(strings.TrimSpace(out[i].Token0))
		t1 := strings.ToLower(strings.TrimSpace(out[i].Token1))
		p0 := prices[t0]
		p1 := prices[t1]
		out[i].Amount0USD = sanitizeFloat(out[i].Amount0 * p0)
		out[i].Amount1USD = sanitizeFloat(out[i].Amount1 * p1)
		out[i].PositionUSD = sanitizeFloat(out[i].Amount0USD + out[i].Amount1USD)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].PositionUSD != out[j].PositionUSD {
			return out[i].PositionUSD > out[j].PositionUSD
		}
		if out[i].PoolVersion != out[j].PoolVersion {
			return out[i].PoolVersion < out[j].PoolVersion
		}
		if out[i].Pair != out[j].Pair {
			return out[i].Pair < out[j].Pair
		}
		return out[i].PositionID < out[j].PositionID
	})

	// Keep response bounded.
	if len(out) > limit {
		out = out[:limit]
	}

	resp := smartMoneyWalletPositionsResponse{
		Chain:         chain,
		WalletAddress: walletAddr,
		WindowSec:     int(window.Seconds()),
		UpdatedAt:     time.Now(),
		Positions:     out,
		Warnings:      warnings,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func sanitizeFloat(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func amountToString(v *big.Int) string {
	if v == nil {
		return "0"
	}
	return v.String()
}

type smartMoneyTokenMetaCache struct {
	mu       sync.Mutex
	decimals map[string]int
	symbols  map[string]string
}

func newSmartMoneyTokenMetaCache() *smartMoneyTokenMetaCache {
	return &smartMoneyTokenMetaCache{
		decimals: make(map[string]int),
		symbols:  make(map[string]string),
	}
}

func (c *smartMoneyTokenMetaCache) Decimals(addr string) int {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" || !common.IsHexAddress(addr) {
		return 18
	}
	c.mu.Lock()
	if v, ok := c.decimals[addr]; ok {
		c.mu.Unlock()
		return v
	}
	c.mu.Unlock()

	dec, err := blockchain.GetTokenDecimals(common.HexToAddress(addr))
	out := 18
	if err == nil && dec != 0 {
		out = int(dec)
	}

	c.mu.Lock()
	c.decimals[addr] = out
	c.mu.Unlock()
	return out
}

func (c *smartMoneyTokenMetaCache) Symbol(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" || !common.IsHexAddress(addr) {
		return ""
	}
	c.mu.Lock()
	if v, ok := c.symbols[addr]; ok {
		c.mu.Unlock()
		return v
	}
	c.mu.Unlock()

	sym, err := blockchain.GetTokenSymbol(common.HexToAddress(addr))
	if err != nil {
		sym = ""
	}
	sym = strings.TrimSpace(sym)

	c.mu.Lock()
	c.symbols[addr] = sym
	c.mu.Unlock()
	return sym
}

func v3ExchangeLabel(npm common.Address) string {
	if config.AppConfig != nil && common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) && npm == common.HexToAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
		return "PancakeSwap V3"
	}
	if config.AppConfig != nil && common.IsHexAddress(config.AppConfig.UniswapV3PositionManagerAddress) && npm == common.HexToAddress(config.AppConfig.UniswapV3PositionManagerAddress) {
		return "Uniswap V3"
	}
	return "V3"
}

const smartMoneyV3PoolSlot0MinABI = `[
  {
    "inputs": [],
    "name": "slot0",
    "outputs": [
      { "internalType": "uint160", "name": "sqrtPriceX96", "type": "uint160" },
      { "internalType": "int24", "name": "tick", "type": "int24" }
    ],
    "stateMutability": "view",
    "type": "function"
  }
]`

func getV3Slot0WithTimeout(ctx context.Context, poolAddr common.Address) (*big.Int, int, error) {
	if blockchain.Client == nil {
		return nil, 0, fmt.Errorf("blockchain client not initialized")
	}
	if (poolAddr == common.Address{}) {
		return nil, 0, fmt.Errorf("empty pool address")
	}

	parsedABI, err := abi.JSON(strings.NewReader(smartMoneyV3PoolSlot0MinABI))
	if err != nil {
		return nil, 0, err
	}
	data, err := parsedABI.Pack("slot0")
	if err != nil {
		return nil, 0, err
	}
	msg := ethereum.CallMsg{To: &poolAddr, Data: data}

	callCtx := ctx
	if callCtx == nil {
		callCtx = context.Background()
	}
	raw, err := blockchain.Client.CallContract(callCtx, msg, nil)
	if err != nil {
		return nil, 0, err
	}
	out, err := parsedABI.Unpack("slot0", raw)
	if err != nil {
		return nil, 0, err
	}
	if len(out) < 2 {
		return nil, 0, fmt.Errorf("unexpected slot0 return length: %d", len(out))
	}
	sqrtPriceX96, ok0 := out[0].(*big.Int)
	tickBig, ok1 := out[1].(*big.Int)
	if !ok0 || sqrtPriceX96 == nil || !ok1 || tickBig == nil {
		return nil, 0, fmt.Errorf("unexpected slot0 return types: sqrt=%T tick=%T", out[0], out[1])
	}
	return sqrtPriceX96, int(tickBig.Int64()), nil
}

func querySmartMoneyWalletRecentV3Positions(ctx context.Context, conn smartMoneyClickHouseQueryer, chain string, wallet string, window time.Duration, limit int) ([]smartMoneyWalletV3TokenRef, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if conn == nil {
		return nil, fmt.Errorf("clickhouse not initialized")
	}
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	if wallet == "" {
		return []smartMoneyWalletV3TokenRef{}, nil
	}
	if limit <= 0 {
		return []smartMoneyWalletV3TokenRef{}, nil
	}
	if limit > 500 {
		limit = 500
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	seconds := int(window.Seconds())
	if seconds <= 0 {
		seconds = 86400
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	chainFilter := ""
	args := make([]any, 0, 2)
	args = append(args, wallet)
	if chain != "" {
		chainFilter = "AND lowerUTF8(chain) = ?"
		args = append(args, chain)
	}

	q := fmt.Sprintf(`
		SELECT
			argMax(pool_version, event_seq) AS pool_version,
			argMax(pool_id, event_seq) AS pool_id,
			contract_address,
			token_id,
			max(event_seq) AS last_event_seq
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND lowerUTF8(wallet_address) = ?
			AND source = 'v3_npm'
			AND token_id != ''
			AND action IN ('add', 'remove')
			%s
		GROUP BY contract_address, token_id
		ORDER BY last_event_seq DESC
		LIMIT %d
	`, seconds, chainFilter, limit)

	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]smartMoneyWalletV3TokenRef, 0, limit)
	for rows.Next() {
		var pv string
		var pid string
		var contractAddr string
		var tokenID string
		var last uint64
		if err := rows.Scan(&pv, &pid, &contractAddr, &tokenID, &last); err != nil {
			return nil, err
		}
		contractAddr = strings.ToLower(strings.TrimSpace(contractAddr))
		tokenID = strings.TrimSpace(tokenID)
		out = append(out, smartMoneyWalletV3TokenRef{
			NPMAddress:  contractAddr,
			TokenID:     tokenID,
			PoolID:      strings.ToLower(strings.TrimSpace(pid)),
			PoolVersion: strings.ToLower(strings.TrimSpace(pv)),
			LastEvent:   last,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func querySmartMoneyWalletRecentV4Pools(ctx context.Context, conn smartMoneyClickHouseQueryer, chain string, wallet string, window time.Duration, limit int) ([]smartMoneyWalletV4PoolRef, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if conn == nil {
		return nil, fmt.Errorf("clickhouse not initialized")
	}
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	if wallet == "" {
		return []smartMoneyWalletV4PoolRef{}, nil
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	seconds := int(window.Seconds())
	if seconds <= 0 {
		seconds = 86400
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	chainFilter := ""
	args := make([]any, 0, 2)
	args = append(args, wallet)
	if chain != "" {
		chainFilter = "AND lowerUTF8(chain) = ?"
		args = append(args, chain)
	}

	q := fmt.Sprintf(`
		SELECT DISTINCT pool_id
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND lowerUTF8(wallet_address) = ?
			AND pool_version = 'v4'
			%s
		LIMIT %d
	`, seconds, chainFilter, limit)

	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]smartMoneyWalletV4PoolRef, 0, limit)
	for rows.Next() {
		var pid string
		if err := rows.Scan(&pid); err != nil {
			return nil, err
		}
		pid = strings.ToLower(strings.TrimSpace(pid))
		out = append(out, smartMoneyWalletV4PoolRef{PoolID: pid})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func poolID25Key(poolID string) (string, bool) {
	poolID = strings.TrimSpace(poolID)
	if !strings.HasPrefix(poolID, "0x") && !strings.HasPrefix(poolID, "0X") {
		poolID = "0x" + poolID
	}
	if len(poolID) != 66 {
		return "", false
	}
	b, err := hex.DecodeString(poolID[2:])
	if err != nil || len(b) != 32 {
		return "", false
	}
	return hex.EncodeToString(b[:25]), true
}

type v4PackedPosInfo struct {
	PoolId25  string
	TickLower int
	TickUpper int
}

func decodeV4PackedPositionInfo(raw *big.Int) (*v4PackedPosInfo, error) {
	if raw == nil {
		return nil, fmt.Errorf("positionInfo missing")
	}
	mask24 := big.NewInt(0xFFFFFF)

	tickLowerRaw := new(big.Int).And(new(big.Int).Rsh(raw, 8), mask24).Int64()
	tickUpperRaw := new(big.Int).And(new(big.Int).Rsh(raw, 32), mask24).Int64()
	poolId := new(big.Int).Rsh(raw, 56)
	poolIdBytes := poolId.FillBytes(make([]byte, 25))

	return &v4PackedPosInfo{
		PoolId25:  hex.EncodeToString(poolIdBytes),
		TickLower: decodeSignedInt24(tickLowerRaw),
		TickUpper: decodeSignedInt24(tickUpperRaw),
	}, nil
}

func decodeSignedInt24(v int64) int {
	if v&0x800000 != 0 {
		v = v - (1 << 24)
	}
	return int(v)
}

func (s *Server) loadV4WalletPositions(ctx context.Context, chain string, walletAddr string, pools []smartMoneyWalletV4PoolRef, limit int, metaCache *smartMoneyTokenMetaCache) ([]string, []smartMoneyWalletLPPosition, []string) {
	warnings := make([]string, 0, 2)
	if config.AppConfig == nil {
		return warnings, nil, nil
	}
	if blockchain.Client == nil {
		return append(warnings, "blockchain client not initialized"), nil, nil
	}
	if !common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) || !common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
		return warnings, nil, nil
	}
	if config.AppConfig.V4NFTScanFromBlock == 0 {
		return warnings, nil, nil
	}

	wallet := common.HexToAddress(walletAddr)
	v4pmAddr := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
	poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)

	// Build mapping from poolId25 -> full poolId for pools seen in SmartLP events.
	fullBy25 := make(map[string]string)
	for _, p := range pools {
		k, ok := poolID25Key(p.PoolID)
		if !ok {
			continue
		}
		if _, exists := fullBy25[k]; !exists {
			fullBy25[k] = strings.ToLower(strings.TrimSpace(p.PoolID))
		}
	}
	if len(fullBy25) == 0 {
		return warnings, nil, nil
	}

	owned, scanErr := scanERC721OwnedTokenIDsCtx(ctx, v4pmAddr, wallet, config.AppConfig.V4NFTScanFromBlock)
	if scanErr != nil {
		return append(warnings, fmt.Sprintf("scan v4 position NFTs failed: %v", scanErr)), nil, nil
	}
	if len(owned) == 0 {
		return warnings, nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	pm, err := blockchain.NewV4PositionManager(v4pmAddr, blockchain.Client)
	if err != nil {
		return append(warnings, "init V4 position manager failed"), nil, nil
	}

	type v4Candidate struct {
		tokenId string
		poolID  string
	}
	cands := make([]v4Candidate, 0, 12)
	for _, tid := range owned {
		tokenID, ok := new(big.Int).SetString(strings.TrimSpace(tid), 10)
		if !ok || tokenID == nil || tokenID.Sign() <= 0 {
			continue
		}
		raw, perr := pm.PositionInfoPacked(&bind.CallOpts{Context: ctx}, tokenID)
		if perr != nil || raw == nil {
			continue
		}
		decoded, derr := decodeV4PackedPositionInfo(raw)
		if derr != nil || decoded == nil {
			continue
		}
		full := fullBy25[strings.ToLower(strings.TrimSpace(decoded.PoolId25))]
		if full == "" {
			continue
		}
		cands = append(cands, v4Candidate{tokenId: tokenID.String(), poolID: full})
	}
	if len(cands) == 0 {
		return warnings, nil, nil
	}

	// Bound candidates.
	if len(cands) > limit {
		cands = cands[:limit]
	}

	out := make([]smartMoneyWalletLPPosition, 0, len(cands))
	priceTokens := make([]string, 0, 2*len(cands))
	priceSet := make(map[string]struct{})

	if metaCache == nil {
		metaCache = newSmartMoneyTokenMetaCache()
	}

	var mu sync.Mutex
	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(6)
	for _, c := range cands {
		c := c
		g.Go(func() error {
			tokenID, _ := new(big.Int).SetString(strings.TrimSpace(c.tokenId), 10)
			if tokenID == nil || tokenID.Sign() <= 0 {
				return nil
			}
			pos, posErr := blockchain.GetV4PositionInfo(v4pmAddr, poolManager, c.poolID, tokenID)
			if posErr != nil || pos == nil {
				return nil
			}
			if pos.Liquidity == nil || pos.Liquidity.Sign() == 0 {
				return nil
			}

			sqrtP, currentTick, slotErr := blockchain.GetUniswapV4PoolSlot0(poolManager, c.poolID)
			if slotErr != nil || sqrtP == nil {
				return nil
			}

			t0 := strings.ToLower(strings.TrimSpace(pos.Token0.Hex()))
			t1 := strings.ToLower(strings.TrimSpace(pos.Token1.Hex()))
			dec0 := metaCache.Decimals(t0)
			dec1 := metaCache.Decimals(t1)
			sym0 := metaCache.Symbol(t0)
			sym1 := metaCache.Symbol(t1)

			sqrtA, _ := pool.SqrtRatioAtTick(int32(pos.TickLower))
			sqrtB, _ := pool.SqrtRatioAtTick(int32(pos.TickUpper))
			amt0Raw, amt1Raw := pool.AmountsForLiquidity(sqrtP, sqrtA, sqrtB, pos.Liquidity)
			amt0 := amountToFloat(amountToString(amt0Raw), dec0)
			amt1 := amountToFloat(amountToString(amt1Raw), dec1)

			inRange := currentTick >= pos.TickLower && currentTick <= pos.TickUpper
			feePct := 0.0
			if pos.Fee > 0 {
				feePct = float64(pos.Fee) / 10000.0
			}
			pair := strings.TrimSpace(sym0 + "/" + sym1)
			if pair == "/" {
				pair = ""
			}

			item := smartMoneyWalletLPPosition{
				PoolVersion:  "v4",
				PoolID:       strings.ToLower(strings.TrimSpace(c.poolID)),
				PositionID:   c.tokenId,
				Exchange:     "Uniswap V4",
				Pair:         pair,
				FeePct:       feePct,
				CurrentTick:  currentTick,
				TickLower:    pos.TickLower,
				TickUpper:    pos.TickUpper,
				InRange:      inRange,
				Liquidity:    pos.Liquidity.String(),
				Token0:       t0,
				Token1:       t1,
				Token0Symbol: sym0,
				Token1Symbol: sym1,
				Token0Dec:    dec0,
				Token1Dec:    dec1,
				Amount0:      amt0,
				Amount1:      amt1,
			}

			mu.Lock()
			out = append(out, item)
			if common.IsHexAddress(t0) {
				if _, ok := priceSet[t0]; !ok {
					priceSet[t0] = struct{}{}
					priceTokens = append(priceTokens, t0)
				}
			}
			if common.IsHexAddress(t1) {
				if _, ok := priceSet[t1]; !ok {
					priceSet[t1] = struct{}{}
					priceTokens = append(priceTokens, t1)
				}
			}
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	return warnings, out, priceTokens
}

type cachedTokenIDs struct {
	ids     []string
	expires time.Time
}

var (
	smartMoneyV4ScanMu    sync.RWMutex
	smartMoneyV4ScanCache = make(map[string]cachedTokenIDs)
)

func scanERC721OwnedTokenIDsCtx(ctx context.Context, contract common.Address, wallet common.Address, fromBlock uint64) ([]string, error) {
	if blockchain.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if (contract == common.Address{}) || (wallet == common.Address{}) {
		return []string{}, nil
	}
	key := strings.ToLower(wallet.Hex()) + "|" + strings.ToLower(contract.Hex()) + "|" + fmt.Sprintf("%d", fromBlock)
	now := time.Now()

	smartMoneyV4ScanMu.RLock()
	if c, ok := smartMoneyV4ScanCache[key]; ok && c.expires.After(now) {
		ids := make([]string, len(c.ids))
		copy(ids, c.ids)
		smartMoneyV4ScanMu.RUnlock()
		return ids, nil
	}
	smartMoneyV4ScanMu.RUnlock()

	ids, err := scanERC721OwnedTokenIDs(contract, wallet, fromBlock)
	if err != nil {
		return nil, err
	}

	smartMoneyV4ScanMu.Lock()
	smartMoneyV4ScanCache[key] = cachedTokenIDs{ids: ids, expires: now.Add(2 * time.Minute)}
	smartMoneyV4ScanMu.Unlock()
	return ids, nil
}

func scanERC721OwnedTokenIDs(contract common.Address, wallet common.Address, fromBlock uint64) ([]string, error) {
	// Reuse the same scanning logic as realtime positions (by Transfer logs),
	// but keep it local to the SmartMoney endpoint.
	// NOTE: This can be heavy if fromBlock is far in the past; keep fromBlock reasonably recent.
	if blockchain.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	latest, err := blockchain.Client.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	if fromBlock == 0 || fromBlock > latest {
		return []string{}, nil
	}

	transferSig := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	walletTopic := common.BytesToHash(wallet.Bytes())

	const chunkSize = uint64(100_000)
	var allLogs []types.Log
	for start := fromBlock; start <= latest; start += chunkSize {
		end := start + chunkSize - 1
		if end > latest {
			end = latest
		}
		fromBI := new(big.Int).SetUint64(start)
		toBI := new(big.Int).SetUint64(end)

		inQuery := ethereum.FilterQuery{
			FromBlock: fromBI,
			ToBlock:   toBI,
			Addresses: []common.Address{contract},
			Topics:    [][]common.Hash{{transferSig}, nil, {walletTopic}},
		}
		inLogs, err := blockchain.Client.FilterLogs(ctx, inQuery)
		if err != nil {
			return nil, err
		}
		allLogs = append(allLogs, inLogs...)

		outQuery := ethereum.FilterQuery{
			FromBlock: fromBI,
			ToBlock:   toBI,
			Addresses: []common.Address{contract},
			Topics:    [][]common.Hash{{transferSig}, {walletTopic}},
		}
		outLogs, err := blockchain.Client.FilterLogs(ctx, outQuery)
		if err != nil {
			return nil, err
		}
		allLogs = append(allLogs, outLogs...)
	}

	sort.Slice(allLogs, func(i, j int) bool {
		if allLogs[i].BlockNumber != allLogs[j].BlockNumber {
			return allLogs[i].BlockNumber < allLogs[j].BlockNumber
		}
		return allLogs[i].Index < allLogs[j].Index
	})

	owned := make(map[string]struct{})
	for _, lg := range allLogs {
		if len(lg.Topics) < 4 {
			continue
		}
		from := common.BytesToAddress(lg.Topics[1].Bytes()[12:])
		to := common.BytesToAddress(lg.Topics[2].Bytes()[12:])
		tokenId := new(big.Int).SetBytes(lg.Topics[3].Bytes()).String()

		if to == wallet {
			owned[tokenId] = struct{}{}
		}
		if from == wallet {
			delete(owned, tokenId)
		}
	}

	ids := make([]string, 0, len(owned))
	for id := range owned {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}
