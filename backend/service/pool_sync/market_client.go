package pool_sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type MarketPoolsClient struct {
	baseURL    string
	httpClient *http.Client
}

type marketPoolsResponse struct {
	Success             bool              `json:"success"`
	Timeframe           string            `json:"timeframe"`
	RequestedLimit      int               `json:"requestedLimit"`
	RequestedProtocol   PoolMStringList   `json:"requestedProtocol"`
	RequestedDex        PoolMStringList   `json:"requestedDex"`
	RequestedChain      string            `json:"requestedChain"`
	TotalPools          int               `json:"totalPools"`
	MetricTrendsIndex   json.RawMessage   `json:"metricTrendsIndex"`
	LiquidityTicksIndex json.RawMessage   `json:"liquidityTicksIndex"`
	Data                []marketPoolsItem `json:"data"`
	Error               string            `json:"error"`
}

func (r *marketPoolsResponse) UnmarshalJSON(data []byte) error {
	type responseAlias marketPoolsResponse
	var aux struct {
		responseAlias
		RequestedLimitSnake      *int            `json:"requested_limit"`
		RequestedProtocolSnake   PoolMStringList `json:"requested_protocol"`
		RequestedDexSnake        PoolMStringList `json:"requested_dex"`
		RequestedChainSnake      *string         `json:"requested_chain"`
		TotalPoolsSnake          *int            `json:"total_pools"`
		MetricTrendsIndexSnake   json.RawMessage `json:"metric_trends_index"`
		LiquidityTicksIndexSnake json.RawMessage `json:"liquidity_ticks_index"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	out := marketPoolsResponse(aux.responseAlias)
	if aux.RequestedLimitSnake != nil {
		out.RequestedLimit = *aux.RequestedLimitSnake
	}
	if len(aux.RequestedProtocolSnake) > 0 {
		out.RequestedProtocol = aux.RequestedProtocolSnake
	}
	if len(aux.RequestedDexSnake) > 0 {
		out.RequestedDex = aux.RequestedDexSnake
	}
	if aux.RequestedChainSnake != nil {
		out.RequestedChain = *aux.RequestedChainSnake
	}
	if aux.TotalPoolsSnake != nil {
		out.TotalPools = *aux.TotalPoolsSnake
	}
	if len(strings.TrimSpace(string(aux.MetricTrendsIndexSnake))) > 0 {
		out.MetricTrendsIndex = aux.MetricTrendsIndexSnake
	}
	if len(strings.TrimSpace(string(aux.LiquidityTicksIndexSnake))) > 0 {
		out.LiquidityTicksIndex = aux.LiquidityTicksIndexSnake
	}
	*r = out
	return nil
}

type marketPoolsItem struct {
	Chain           string `json:"chain"`
	ProtocolVersion string `json:"protocolVersion"`
	Dex             string `json:"dex"`
	PoolAddress     string `json:"poolAddress"`
	PoolID          string `json:"poolId"`
	FactoryName     string `json:"factoryName"`
	FactoryAddress  string `json:"factoryAddress"`
	PoolManager     string `json:"poolManager"`
	TradingPair     string `json:"tradingPair"`
	Token0Symbol    string `json:"token0Symbol"`
	Token1Symbol    string `json:"token1Symbol"`
	Token0Name      string `json:"token0Name"`
	Token1Name      string `json:"token1Name"`
	Token0Address   string `json:"token0Address"`
	Token1Address   string `json:"token1Address"`
	Token0Decimals  int    `json:"token0Decimals"`
	Token1Decimals  int    `json:"token1Decimals"`

	StableCoinSymbol string  `json:"stableCoinSymbol"`
	FeeRate          int     `json:"feeRate"`
	FeePercentage    float64 `json:"feePercentage"`
	HookAddress      string  `json:"hookAddress"`

	TransactionCount        int             `json:"transactionCount"`
	TotalFees               float64         `json:"totalFees"`
	TotalVolume             float64         `json:"totalVolume"`
	CurrentPoolValue        float64         `json:"currentPoolValue"`
	CurrentToken0Balance    float64         `json:"currentToken0Balance"`
	CurrentToken1Balance    float64         `json:"currentToken1Balance"`
	CurrentTokenPrice       float64         `json:"currentTokenPrice"`
	PricedTokenAddress      string          `json:"pricedTokenAddress"`
	CurrentTokenTotalSupply float64         `json:"currentTokenTotalSupply"`
	CurrentTokenFDVUSD      float64         `json:"currentTokenFdvUsd"`
	TokenSupplyUpdatedAt    string          `json:"tokenSupplyUpdatedAt"`
	PriceDisplay            string          `json:"priceDisplay"`
	LastSwapAt              string          `json:"lastSwapAt"`
	TickSpacing             *int            `json:"tickSpacing"`
	CurrentTick             int             `json:"currentTick"`
	CurrentSqrtPriceX96     string          `json:"currentSqrtPriceX96"`
	CurrentLiquidity        string          `json:"currentLiquidity"`
	StableCoinPosition      string          `json:"stableCoinPosition"`
	MetricTrends            json.RawMessage `json:"metricTrends"`
	UniqueWallets           int             `json:"uniqueWallets"`
	TopWalletVolPct         float64         `json:"topWalletVolPct"`
	ActiveTickCount         int             `json:"activeTickCount"`
	ActiveLiquidityUSD      float64         `json:"activeLiquidityUSD"`
	ActiveLiquidityRatio    float64         `json:"activeLiquidityRatio"`
	LiquidityTicks          json.RawMessage `json:"liquidityTicks"`
	LiquidityCurrentTick    int             `json:"liquidityCurrentTick"`
	LiquidityTickSpacing    int             `json:"liquidityTickSpacing"`
	Badges                  json.RawMessage `json:"badges"`
}

func (item *marketPoolsItem) UnmarshalJSON(data []byte) error {
	type itemAlias marketPoolsItem
	var aux struct {
		itemAlias
		ProtocolVersionSnake         *string         `json:"protocol_version"`
		PoolAddressSnake             *string         `json:"pool_address"`
		PoolIDSnake                  *string         `json:"pool_id"`
		FactoryNameSnake             *string         `json:"factory_name"`
		FactoryAddressSnake          *string         `json:"factory_address"`
		PoolManagerSnake             *string         `json:"pool_manager"`
		TradingPairSnake             *string         `json:"trading_pair"`
		Token0SymbolSnake            *string         `json:"token0_symbol"`
		Token1SymbolSnake            *string         `json:"token1_symbol"`
		Token0NameSnake              *string         `json:"token0_name"`
		Token1NameSnake              *string         `json:"token1_name"`
		Token0AddressSnake           *string         `json:"token0_address"`
		Token1AddressSnake           *string         `json:"token1_address"`
		Token0DecimalsSnake          *int            `json:"token0_decimals"`
		Token1DecimalsSnake          *int            `json:"token1_decimals"`
		StableCoinSymbolSnake        *string         `json:"stable_coin_symbol"`
		FeeRateSnake                 *int            `json:"fee_rate"`
		FeePercentageSnake           *float64        `json:"fee_percentage"`
		HookAddressSnake             *string         `json:"hook_address"`
		TransactionCountSnake        *int            `json:"transaction_count"`
		TotalFeesSnake               *float64        `json:"total_fees"`
		TotalVolumeSnake             *float64        `json:"total_volume"`
		CurrentPoolValueSnake        *float64        `json:"current_pool_value"`
		CurrentToken0BalanceSnake    *float64        `json:"current_token0_balance"`
		CurrentToken1BalanceSnake    *float64        `json:"current_token1_balance"`
		CurrentTokenPriceSnake       *float64        `json:"current_token_price"`
		PricedTokenAddressSnake      *string         `json:"priced_token_address"`
		CurrentTokenTotalSupplySnake *float64        `json:"current_token_total_supply"`
		CurrentTokenFDVUSDSnake      *float64        `json:"current_token_fdv_usd"`
		TokenSupplyUpdatedAtSnake    *string         `json:"token_supply_updated_at"`
		PriceDisplaySnake            *string         `json:"price_display"`
		LastSwapAtSnake              *string         `json:"last_swap_at"`
		TickSpacingSnake             *int            `json:"tick_spacing"`
		CurrentTickSnake             *int            `json:"current_tick"`
		CurrentSqrtPriceX96Snake     *string         `json:"current_sqrt_price_x96"`
		CurrentLiquiditySnake        *string         `json:"current_liquidity"`
		StableCoinPositionSnake      *string         `json:"stable_coin_position"`
		MetricTrendsSnake            json.RawMessage `json:"metric_trends"`
		UniqueWalletsSnake           *int            `json:"unique_wallets"`
		TopWalletVolPctSnake         *float64        `json:"top_wallet_vol_pct"`
		ActiveTickCountSnake         *int            `json:"active_tick_count"`
		ActiveLiquidityUSDSnake      *float64        `json:"active_liquidity_usd"`
		ActiveLiquidityRatioSnake    *float64        `json:"active_liquidity_ratio"`
		LiquidityTicksSnake          json.RawMessage `json:"liquidity_ticks"`
		LiquidityCurrentTickSnake    *int            `json:"liquidity_current_tick"`
		LiquidityTickSpacingSnake    *int            `json:"liquidity_tick_spacing"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	out := marketPoolsItem(aux.itemAlias)
	setStringFromPtr(&out.ProtocolVersion, aux.ProtocolVersionSnake)
	setStringFromPtr(&out.PoolAddress, aux.PoolAddressSnake)
	setStringFromPtr(&out.PoolID, aux.PoolIDSnake)
	setStringFromPtr(&out.FactoryName, aux.FactoryNameSnake)
	setStringFromPtr(&out.FactoryAddress, aux.FactoryAddressSnake)
	setStringFromPtr(&out.PoolManager, aux.PoolManagerSnake)
	setStringFromPtr(&out.TradingPair, aux.TradingPairSnake)
	setStringFromPtr(&out.Token0Symbol, aux.Token0SymbolSnake)
	setStringFromPtr(&out.Token1Symbol, aux.Token1SymbolSnake)
	setStringFromPtr(&out.Token0Name, aux.Token0NameSnake)
	setStringFromPtr(&out.Token1Name, aux.Token1NameSnake)
	setStringFromPtr(&out.Token0Address, aux.Token0AddressSnake)
	setStringFromPtr(&out.Token1Address, aux.Token1AddressSnake)
	setIntFromPtr(&out.Token0Decimals, aux.Token0DecimalsSnake)
	setIntFromPtr(&out.Token1Decimals, aux.Token1DecimalsSnake)
	setStringFromPtr(&out.StableCoinSymbol, aux.StableCoinSymbolSnake)
	setIntFromPtr(&out.FeeRate, aux.FeeRateSnake)
	setFloatFromPtr(&out.FeePercentage, aux.FeePercentageSnake)
	setStringFromPtr(&out.HookAddress, aux.HookAddressSnake)
	setIntFromPtr(&out.TransactionCount, aux.TransactionCountSnake)
	setFloatFromPtr(&out.TotalFees, aux.TotalFeesSnake)
	setFloatFromPtr(&out.TotalVolume, aux.TotalVolumeSnake)
	setFloatFromPtr(&out.CurrentPoolValue, aux.CurrentPoolValueSnake)
	setFloatFromPtr(&out.CurrentToken0Balance, aux.CurrentToken0BalanceSnake)
	setFloatFromPtr(&out.CurrentToken1Balance, aux.CurrentToken1BalanceSnake)
	setFloatFromPtr(&out.CurrentTokenPrice, aux.CurrentTokenPriceSnake)
	setStringFromPtr(&out.PricedTokenAddress, aux.PricedTokenAddressSnake)
	setFloatFromPtr(&out.CurrentTokenTotalSupply, aux.CurrentTokenTotalSupplySnake)
	setFloatFromPtr(&out.CurrentTokenFDVUSD, aux.CurrentTokenFDVUSDSnake)
	setStringFromPtr(&out.TokenSupplyUpdatedAt, aux.TokenSupplyUpdatedAtSnake)
	setStringFromPtr(&out.PriceDisplay, aux.PriceDisplaySnake)
	setStringFromPtr(&out.LastSwapAt, aux.LastSwapAtSnake)
	setIntPtrFromPtr(&out.TickSpacing, aux.TickSpacingSnake)
	setIntFromPtr(&out.CurrentTick, aux.CurrentTickSnake)
	setStringFromPtr(&out.CurrentSqrtPriceX96, aux.CurrentSqrtPriceX96Snake)
	setStringFromPtr(&out.CurrentLiquidity, aux.CurrentLiquiditySnake)
	setStringFromPtr(&out.StableCoinPosition, aux.StableCoinPositionSnake)
	setRawMessage(&out.MetricTrends, aux.MetricTrendsSnake)
	setIntFromPtr(&out.UniqueWallets, aux.UniqueWalletsSnake)
	setFloatFromPtr(&out.TopWalletVolPct, aux.TopWalletVolPctSnake)
	setIntFromPtr(&out.ActiveTickCount, aux.ActiveTickCountSnake)
	setFloatFromPtr(&out.ActiveLiquidityUSD, aux.ActiveLiquidityUSDSnake)
	setFloatFromPtr(&out.ActiveLiquidityRatio, aux.ActiveLiquidityRatioSnake)
	setRawMessage(&out.LiquidityTicks, aux.LiquidityTicksSnake)
	setIntFromPtr(&out.LiquidityCurrentTick, aux.LiquidityCurrentTickSnake)
	setIntFromPtr(&out.LiquidityTickSpacing, aux.LiquidityTickSpacingSnake)
	*item = out
	return nil
}

func setStringFromPtr(dst *string, value *string) {
	if dst == nil || value == nil {
		return
	}
	*dst = *value
}

func setIntFromPtr(dst *int, value *int) {
	if dst == nil || value == nil {
		return
	}
	*dst = *value
}

func setFloatFromPtr(dst *float64, value *float64) {
	if dst == nil || value == nil {
		return
	}
	*dst = *value
}

func setIntPtrFromPtr(dst **int, value *int) {
	if dst == nil || value == nil {
		return
	}
	v := *value
	*dst = &v
}

func setRawMessage(dst *json.RawMessage, value json.RawMessage) {
	if dst == nil || len(strings.TrimSpace(string(value))) == 0 {
		return
	}
	*dst = append((*dst)[:0], value...)
}

func NewMarketPoolsClient(baseURL string) *MarketPoolsClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return &MarketPoolsClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *MarketPoolsClient) Pools(ctx context.Context, source PoolDataSourceConfig, chain string, defaultDexes []string) (*PoolMTopFeesResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("market pools client is nil")
	}
	if strings.TrimSpace(c.baseURL) == "" {
		return nil, fmt.Errorf("market pools base url is empty")
	}
	u, err := buildMarketPoolsURL(c.baseURL, source, chain, defaultDexes)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("market pools http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed marketPoolsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode market pools response: %w", err)
	}
	if !parsed.Success {
		if strings.TrimSpace(parsed.Error) != "" {
			return nil, fmt.Errorf("market pools error: %s", strings.TrimSpace(parsed.Error))
		}
		return nil, fmt.Errorf("market pools error: success=false")
	}
	return convertMarketPoolsResponse(parsed), nil
}

func buildMarketPoolsURL(baseURL string, source PoolDataSourceConfig, chain string, defaultDexes []string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("parse market pools base url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid market pools base url")
	}

	path := strings.TrimSpace(source.PathTemplate)
	if path == "" && !strings.Contains(strings.ToLower(u.Path), "/api/market/pools") {
		path = "/api/market/pools"
	}
	if path != "" {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		u.Path = path
	}

	q := u.Query()
	for key, value := range source.QueryTemplate {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		q.Set(key, strings.TrimSpace(value))
	}

	timeframeMinutes := positiveOrDefault(source.TimeframeMinutes, 5)
	if q.Get("timeframe") == "" {
		q.Set("timeframe", fmt.Sprintf("%dm", timeframeMinutes))
	}
	if q.Get("timeframe_minutes") == "" {
		q.Set("timeframe_minutes", strconv.Itoa(timeframeMinutes))
	}
	if q.Get("limit") == "" {
		q.Set("limit", strconv.Itoa(positiveOrDefault(source.Limit, 100)))
	}
	if chain = strings.TrimSpace(chain); chain != "" && q.Get("chain") == "" {
		q.Set("chain", chain)
	}
	if q.Get("protocol") == "" {
		protocols := source.Protocols
		if len(protocols) == 0 {
			protocols = []string{"v3", "v4"}
		}
		q.Set("protocol", strings.Join(protocols, ","))
	}
	if q.Get("dex") == "" {
		dexes := source.Dexes
		if len(dexes) == 0 {
			dexes = marketDexNamesFromPoolMDexes(defaultDexes)
		}
		q.Set("dex", strings.Join(dexes, ","))
	}
	u.RawQuery = q.Encode()
	return u, nil
}

func convertMarketPoolsResponse(in marketPoolsResponse) *PoolMTopFeesResponse {
	out := &PoolMTopFeesResponse{
		Success:             in.Success,
		Timeframe:           in.Timeframe,
		RequestedLimit:      in.RequestedLimit,
		RequestedProtocol:   in.RequestedProtocol,
		RequestedDex:        in.RequestedDex,
		RequestedChain:      in.RequestedChain,
		TotalPools:          in.TotalPools,
		MetricTrendsIndex:   jsonFallback(in.MetricTrendsIndex, "[]"),
		LiquidityTicksIndex: jsonFallback(in.LiquidityTicksIndex, "[]"),
		Error:               in.Error,
		Data:                make([]PoolMFeePool, 0, len(in.Data)),
	}
	for _, item := range in.Data {
		out.Data = append(out.Data, PoolMFeePool{
			Chain:                   item.Chain,
			ProtocolVersion:         item.ProtocolVersion,
			Dex:                     item.Dex,
			PoolAddress:             item.PoolAddress,
			PoolID:                  item.PoolID,
			FactoryName:             item.FactoryName,
			FactoryAddress:          item.FactoryAddress,
			PoolManager:             item.PoolManager,
			TradingPair:             item.TradingPair,
			Token0Symbol:            item.Token0Symbol,
			Token1Symbol:            item.Token1Symbol,
			Token0Name:              item.Token0Name,
			Token1Name:              item.Token1Name,
			Token0Address:           item.Token0Address,
			Token1Address:           item.Token1Address,
			Token0Decimals:          item.Token0Decimals,
			Token1Decimals:          item.Token1Decimals,
			StableCoinSymbol:        item.StableCoinSymbol,
			FeeRate:                 item.FeeRate,
			FeePercentage:           item.FeePercentage,
			HookAddress:             item.HookAddress,
			TransactionCount:        item.TransactionCount,
			TotalFees:               item.TotalFees,
			TotalVolume:             item.TotalVolume,
			CurrentPoolValue:        item.CurrentPoolValue,
			CurrentToken0Balance:    item.CurrentToken0Balance,
			CurrentToken1Balance:    item.CurrentToken1Balance,
			CurrentTokenPrice:       item.CurrentTokenPrice,
			PricedTokenAddress:      item.PricedTokenAddress,
			CurrentTokenTotalSupply: item.CurrentTokenTotalSupply,
			CurrentTokenFDVUSD:      item.CurrentTokenFDVUSD,
			TokenSupplyUpdatedAt:    item.TokenSupplyUpdatedAt,
			PriceDisplay:            item.PriceDisplay,
			LastSwapAt:              item.LastSwapAt,
			TickSpacing:             item.TickSpacing,
			CurrentTick:             item.CurrentTick,
			CurrentSqrtPriceX96:     item.CurrentSqrtPriceX96,
			CurrentLiquidity:        item.CurrentLiquidity,
			StableCoinPosition:      item.StableCoinPosition,
			MetricTrends:            jsonFallback(item.MetricTrends, "[]"),
			UniqueWallets:           item.UniqueWallets,
			TopWalletVolPct:         item.TopWalletVolPct,
			ActiveTickCount:         item.ActiveTickCount,
			ActiveLiquidityUSD:      item.ActiveLiquidityUSD,
			ActiveLiquidityRatio:    item.ActiveLiquidityRatio,
			LiquidityTicks:          jsonFallback(item.LiquidityTicks, "[]"),
			LiquidityCurrentTick:    item.LiquidityCurrentTick,
			LiquidityTickSpacing:    item.LiquidityTickSpacing,
			Badges:                  jsonFallback(item.Badges, "[]"),
		})
	}
	return out
}

func jsonFallback(raw json.RawMessage, fallback string) json.RawMessage {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return json.RawMessage(fallback)
	}
	return raw
}

func marketDexNamesFromPoolMDexes(dexes []string) []string {
	if len(dexes) == 0 {
		return []string{"PancakeswapV3", "UniswapV3", "UniswapV4"}
	}
	out := make([]string, 0, len(dexes))
	seen := make(map[string]struct{}, len(dexes))
	for _, dex := range dexes {
		value := strings.ToLower(strings.TrimSpace(dex))
		if value == "" {
			continue
		}
		switch value {
		case "pcsv3", "pancakeswapv3", "pancakeswap_v3", "pancakev3":
			value = "PancakeswapV3"
		case "univ3", "uniswapv3", "uniswap_v3":
			value = "UniswapV3"
		case "univ4", "uniswapv4", "uniswap_v4":
			value = "UniswapV4"
		default:
			value = dex
		}
		key := strings.ToLower(strings.TrimSpace(value))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return []string{"PancakeswapV3", "UniswapV3", "UniswapV4"}
	}
	return out
}
