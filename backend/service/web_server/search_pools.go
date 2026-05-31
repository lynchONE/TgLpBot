package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"TgLpBot/base/models"
	"TgLpBot/service/pool"

	"github.com/ethereum/go-ethereum/common"
)

type searchPoolsEnvelope struct {
	Data             []HotPoolResponse `json:"data"`
	Query            string            `json:"query"`
	Chain            string            `json:"chain"`
	TimeframeMinutes int               `json:"timeframe_minutes"`
	Limit            int               `json:"limit"`
	Source           string            `json:"source"`
}

type dexScreenerToken struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	Symbol  string `json:"symbol"`
}

type dexScreenerTxnsBucket struct {
	Buys  int `json:"buys"`
	Sells int `json:"sells"`
}

type dexScreenerTxns struct {
	H24 dexScreenerTxnsBucket `json:"h24"`
}

type dexScreenerVolume struct {
	H24 float64 `json:"h24"`
}

type dexScreenerLiquidity struct {
	USD float64 `json:"usd"`
}

type dexScreenerPair struct {
	ChainID     string               `json:"chainId"`
	DexID       string               `json:"dexId"`
	PairAddress string               `json:"pairAddress"`
	Labels      []string             `json:"labels"`
	BaseToken   dexScreenerToken     `json:"baseToken"`
	QuoteToken  dexScreenerToken     `json:"quoteToken"`
	PriceUSD    string               `json:"priceUsd"`
	Txns        dexScreenerTxns      `json:"txns"`
	Volume      dexScreenerVolume    `json:"volume"`
	Liquidity   dexScreenerLiquidity `json:"liquidity"`
}

type dexScreenerSearchResponse struct {
	Pairs []dexScreenerPair `json:"pairs"`
}

func normalizeDexScreenerChain(chain string) string {
	v := strings.ToLower(strings.TrimSpace(chain))
	switch v {
	case "", "bsc", "bnb":
		return "bsc"
	case "eth", "ethereum":
		return "ethereum"
	}
	return v
}

func dexScreenerPairAddress(raw string) string {
	addr := strings.TrimSpace(raw)
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, "0x") || strings.HasPrefix(addr, "0X") {
		addr = addr[2:]
	}
	addr = strings.TrimSpace(addr)
	addr = strings.ToLower(addr)
	if addr == "" {
		return ""
	}
	return "0x" + addr
}

func dexScreenerLooksLikePoolAddress(raw string) bool {
	v := strings.TrimSpace(raw)
	if v == "" {
		return false
	}
	if strings.Contains(v, ":") {
		return false
	}
	if strings.HasPrefix(v, "0x") || strings.HasPrefix(v, "0X") {
		v = v[2:]
	}
	if len(v) != 40 && len(v) != 64 {
		return false
	}
	for _, c := range v {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func dexScreenerProtocolVersion(pair dexScreenerPair) string {
	for _, l := range pair.Labels {
		switch strings.ToLower(strings.TrimSpace(l)) {
		case "v4":
			return "v4"
		case "v3":
			return "v3"
		case "v2":
			return "v2"
		}
	}

	addr := dexScreenerPairAddress(pair.PairAddress)
	if isV4PoolId(addr) {
		return "v4"
	}
	if common.IsHexAddress(addr) {
		return "v3"
	}
	return ""
}

func fetchDexScreenerPairs(ctx context.Context, endpoint string) ([]dexScreenerPair, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("dexscreener http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed dexScreenerSearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return parsed.Pairs, nil
}

func buildSearchPoolResponse(pair dexScreenerPair, poolInfo *pool.PoolInfo, poolVersion string) HotPoolResponse {
	poolAddress := dexScreenerPairAddress(pair.PairAddress)
	tradingPair := strings.TrimSpace(pair.BaseToken.Symbol) + "/" + strings.TrimSpace(pair.QuoteToken.Symbol)
	token0Addr := strings.TrimSpace(pair.BaseToken.Address)
	token1Addr := strings.TrimSpace(pair.QuoteToken.Address)

	factoryName := strings.TrimSpace(pair.DexID)
	feePct := 0.0
	if poolInfo != nil {
		tradingPair = strings.TrimSpace(poolInfo.Token0Symbol) + "/" + strings.TrimSpace(poolInfo.Token1Symbol)
		token0Addr = strings.TrimSpace(poolInfo.Token0)
		token1Addr = strings.TrimSpace(poolInfo.Token1)
		if strings.TrimSpace(poolInfo.Exchange) != "" {
			factoryName = strings.TrimSpace(poolInfo.Exchange)
		}
		if poolInfo.Fee > 0 {
			feePct = float64(poolInfo.Fee) / 10000.0
		}
	}

	volume24h := pair.Volume.H24
	tvl := pair.Liquidity.USD

	totalFees := 0.0
	if volume24h > 0 && feePct > 0 {
		totalFees = volume24h * (feePct / 100.0)
	}

	feeRate := 0.0
	if tvl > 0 && totalFees > 0 {
		feeRate = totalFees / tvl * 100.0
	}

	txCount := pair.Txns.H24.Buys + pair.Txns.H24.Sells
	if txCount < 0 {
		txCount = 0
	}

	priceDisplay := strings.TrimSpace(pair.PriceUSD)
	if priceDisplay != "" && !strings.HasPrefix(priceDisplay, "$") {
		priceDisplay = "$" + priceDisplay
	}

	return HotPoolResponse{
		Chain:            normalizeDexScreenerChain(pair.ChainID),
		ProtocolVersion:  poolVersion,
		PoolAddress:      poolAddress,
		Dex:              strings.TrimSpace(pair.DexID),
		FactoryName:      factoryName,
		TradingPair:      tradingPair,
		FeePercentage:    feePct,
		FeeTier:          0,
		TransactionCount: uint32(txCount),
		TotalFees:        totalFees,
		TotalVolume:      volume24h,
		CurrentPoolValue: tvl,
		FeeRate:          feeRate,
		PriceDisplay:     priceDisplay,
		UpdatedAt:        time.Now(),
		Token0Address:    token0Addr,
		Token1Address:    token1Addr,
		Token0Symbol:     strings.TrimSpace(pair.BaseToken.Symbol),
		Token1Symbol:     strings.TrimSpace(pair.QuoteToken.Symbol),
		Token0Name:       strings.TrimSpace(pair.BaseToken.Name),
		Token1Name:       strings.TrimSpace(pair.QuoteToken.Name),
	}
}

func (s *Server) handleSearchPools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	if status, msg := requireModulePermission(check, models.AccessModuleHotPools); status != 0 {
		http.Error(w, msg, status)
		return
	}

	query := r.URL.Query()
	q := strings.TrimSpace(query.Get("q"))
	if q == "" {
		q = strings.TrimSpace(query.Get("keyword"))
	}
	if q == "" {
		http.Error(w, "missing q", http.StatusBadRequest)
		return
	}
	if len(q) > 96 {
		q = q[:96]
	}

	chain := normalizeDexScreenerChain(query.Get("chain"))
	if chain == "" {
		chain = "bsc"
	}

	limit := 10
	if v := strings.TrimSpace(query.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 10 {
		limit = 10
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	poolService := pool.NewPoolService()
	isPoolIdOrAddress := dexScreenerLooksLikePoolAddress(q)

	var (
		pairs  []dexScreenerPair
		source = "dexscreener"
	)
	if isPoolIdOrAddress {
		addr := dexScreenerPairAddress(q)
		endpoint := fmt.Sprintf("https://api.dexscreener.com/latest/dex/pairs/%s/%s", url.PathEscape(chain), url.PathEscape(addr))
		found, err := fetchDexScreenerPairs(ctx, endpoint)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		pairs = found
	} else {
		if len([]rune(q)) < 2 {
			http.Error(w, "q too short", http.StatusBadRequest)
			return
		}
		endpoint := fmt.Sprintf("https://api.dexscreener.com/latest/dex/search?q=%s", url.QueryEscape(q))
		found, err := fetchDexScreenerPairs(ctx, endpoint)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		out := make([]dexScreenerPair, 0, len(found))
		for _, p := range found {
			if strings.EqualFold(strings.TrimSpace(p.ChainID), chain) {
				out = append(out, p)
			}
		}
		pairs = out
	}

	if !isPoolIdOrAddress && len(pairs) > 1 {
		sort.SliceStable(pairs, func(i, j int) bool {
			return pairs[i].Liquidity.USD > pairs[j].Liquidity.USD
		})
	}

	rows := make([]HotPoolResponse, 0, limit)
	for _, p := range pairs {
		if len(rows) >= limit {
			break
		}
		if strings.TrimSpace(p.ChainID) != "" && !strings.EqualFold(strings.TrimSpace(p.ChainID), chain) {
			continue
		}
		if !dexScreenerLooksLikePoolAddress(p.PairAddress) {
			continue
		}

		poolAddr := dexScreenerPairAddress(p.PairAddress)
		pv := dexScreenerProtocolVersion(p)
		if pv == "v2" || pv == "" {
			continue
		}

		var (
			poolInfo *pool.PoolInfo
			infoErr  error
		)
		switch pv {
		case "v4":
			if strings.EqualFold(chain, "bsc") {
				// Generic keyword search already has token/dex metadata from DexScreener.
				// Avoid spending RPC on every V4 row unless the user is explicitly querying a pool id/address.
				if isPoolIdOrAddress {
					poolInfo, infoErr = poolService.GetPoolInfoForVersionCached(chain, "v4", poolAddr)
				}
			} else {
				continue
			}
		default:
			if !common.IsHexAddress(poolAddr) {
				continue
			}
			poolInfo, infoErr = poolService.GetPoolInfoForVersionCached(chain, "v3", poolAddr)
		}
		if infoErr != nil {
			continue
		}
		if pv != "v4" && poolInfo == nil {
			continue
		}

		rows = append(rows, buildSearchPoolResponse(p, poolInfo, pv))
	}

	// DexScreener 未返回该池子时（例如新池子），按池子ID搜索可回退到链上读取基础信息
	if len(rows) == 0 && isPoolIdOrAddress {
		poolAddr := normalizeHexPrefixed(q)
		var (
			poolInfo *pool.PoolInfo
			infoErr  error
			version  string
		)
		if isV4PoolId(poolAddr) {
			version = "v4"
			if strings.EqualFold(chain, "bsc") {
				poolInfo, infoErr = poolService.GetPoolInfoForVersionCached(chain, "v4", poolAddr)
			} else {
				infoErr = fmt.Errorf("v4 not supported on chain=%s", chain)
			}
		} else {
			version = "v3"
			if !common.IsHexAddress(poolAddr) {
				http.Error(w, "invalid pool address", http.StatusBadRequest)
				return
			}
			poolInfo, infoErr = poolService.GetPoolInfoForVersionCached(chain, "v3", poolAddr)
		}
		if infoErr == nil && poolInfo != nil {
			minimal := dexScreenerPair{
				ChainID:     chain,
				PairAddress: poolAddr,
				BaseToken: dexScreenerToken{
					Address: poolInfo.Token0,
					Symbol:  poolInfo.Token0Symbol,
					Name:    poolInfo.Token0Symbol,
				},
				QuoteToken: dexScreenerToken{
					Address: poolInfo.Token1,
					Symbol:  poolInfo.Token1Symbol,
					Name:    poolInfo.Token1Symbol,
				},
			}
			rows = append(rows, buildSearchPoolResponse(minimal, poolInfo, version))
			source = "chain"
		}
	}

	s.enrichHotPoolDisplayTokens(ctx, chain, rows)
	s.enrichHotPoolTokenRisks(ctx, chain, rows)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(searchPoolsEnvelope{
		Data:             rows,
		Query:            q,
		Chain:            chain,
		TimeframeMinutes: 1440,
		Limit:            limit,
		Source:           source,
	})
}
