package pool_sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMarketPoolsClient_NormalizesCamelCaseAndV4PoolID(t *testing.T) {
	const poolID = "0x8e838bd6f6162f0abfca09df68e5ed6526a7774d826c61aafbda8489811f76f6"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/market/pools" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("protocol"); got != "v3,v4" {
			t.Fatalf("expected protocol=v3,v4, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"timeframe": "5m",
			"requestedLimit": 10,
			"requestedProtocol": ["v4"],
			"requestedChain": "bsc",
			"requestedDex": ["UniswapV4"],
			"totalPools": 1,
			"metricTrendsIndex": [],
			"liquidityTicksIndex": [],
			"data": [{
				"chain": "bsc",
				"protocolVersion": "v4",
				"poolAddress": null,
				"poolId": "` + poolID + `",
				"factoryName": "UniswapV4",
				"poolManager": "0x28e2ea090877bf75740558f6bfb36a5ffee9e9df",
				"tradingPair": "USDT/AAA",
				"token0Symbol": "USDT",
				"token1Symbol": "AAA",
				"token0Address": "0x55d398326f99059ff775485246999027b3197955",
				"token1Address": "0x1111111111111111111111111111111111111111",
				"token0Decimals": 18,
				"token1Decimals": 18,
				"stableCoinSymbol": "USDT",
				"feeRate": 3000,
				"feePercentage": 0.3,
				"transactionCount": 12,
				"totalFees": 42.5,
				"totalVolume": 1234.5,
				"currentPoolValue": null,
				"currentTokenPrice": 0.01,
				"pricedTokenAddress": "0x1111111111111111111111111111111111111111",
				"lastSwapAt": "2026-04-27T10:58:42Z",
				"tickSpacing": 60,
				"currentTick": 100,
				"currentSqrtPriceX96": "1",
				"currentLiquidity": "2",
				"stableCoinPosition": "token0",
				"metricTrends": [],
				"liquidityTicks": [],
				"liquidityCurrentTick": 100,
				"liquidityTickSpacing": 60,
				"badges": []
			}]
		}`))
	}))
	defer server.Close()

	source := PoolDataSourceConfig{
		SourceType:       PoolDataSourceTypeMarketPools,
		BaseURL:          server.URL,
		TimeframeMinutes: 5,
		Limit:            10,
		Protocols:        []string{"v3", "v4"},
		Dexes:            []string{"UniswapV4"},
	}
	resp, err := NewMarketPoolsClient(server.URL).Pools(context.Background(), source, "bsc", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected one pool, got %d", len(resp.Data))
	}
	item := resp.Data[0]
	if item.PoolID != poolID {
		t.Fatalf("expected pool id %s, got %s", poolID, item.PoolID)
	}
	if item.FeeRate != 3000 || item.TransactionCount != 12 || item.TotalFees != 42.5 {
		t.Fatalf("unexpected normalized metrics: %+v", item)
	}

	annotateSnapshotSource(resp, source)
	row, err := (&Service{}).buildRow(resp, item, time.Now())
	if err != nil {
		t.Fatalf("expected build row to use v4 pool id fallback, got %v", err)
	}
	if row.Address != poolID {
		t.Fatalf("expected row address %s, got %s", poolID, row.Address)
	}
	if row.ProtocolVersion != "v4" {
		t.Fatalf("expected protocol v4, got %s", row.ProtocolVersion)
	}
}

func TestMarketPoolsClient_NormalizesSnakeCaseResponse(t *testing.T) {
	const poolAddress = "0xc86071ddd1b9367462af9f22d827747e1bc6f7e0"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/market/pools" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"timeframe": "5m",
			"requested_limit": 2,
			"requested_protocol": ["v3", "v4"],
			"requested_chain": "bsc",
			"requested_dex": ["PancakeswapV3", "UniswapV3", "UniswapV4"],
			"total_pools": 2,
			"metric_trends_index": [],
			"liquidity_ticks_index": [],
			"data": [{
				"chain": "bsc",
				"protocol_version": "v3",
				"pool_address": "` + poolAddress + `",
				"pool_id": null,
				"factory_name": "PancakeswapV3",
				"factory_address": "0x0bfbcf9fa4f9c56b0f40a671ad40e0805a091865",
				"pool_manager": null,
				"trading_pair": "USDT/DAM",
				"token0_symbol": "USDT",
				"token1_symbol": "DAM",
				"token0_address": "0x55d398326f99059ff775485246999027b3197955",
				"token1_address": "0xf9ca3fe094212ffa705742d3626a8ab96aababf8",
				"token0_name": "Tether USD",
				"token1_name": "Reservoir",
				"token0_decimals": 18,
				"token1_decimals": 18,
				"stable_coin_symbol": "USDT",
				"fee_rate": 2500,
				"fee_percentage": 0.25,
				"hook_address": null,
				"transaction_count": 320,
				"total_fees": 205.30699409588,
				"total_volume": 82122.797638352,
				"current_pool_value": 331672.836351552,
				"current_token0_balance": 13178.9879020292,
				"current_token1_balance": 11135351.5920186,
				"current_token_price": 0.0286020468970022,
				"priced_token_address": "0xf9ca3fe094212ffa705742d3626a8ab96aababf8",
				"current_token_total_supply": null,
				"current_token_fdv_usd": null,
				"token_supply_updated_at": null,
				"price_display": "1 DAM = 0.0286020468970022396654799272225621 USD",
				"last_swap_at": "2026-04-28T16:38:52Z",
				"tick_spacing": 50,
				"current_tick": 35516,
				"current_sqrt_price_x96": "467817539287070161347471025315",
				"current_liquidity": "3167892170311023383731035",
				"stable_coin_position": "token0",
				"metric_trends": [],
				"unique_wallets": null,
				"top_wallet_vol_pct": null,
				"active_tick_count": null,
				"active_liquidity_usd": 1339.01922192219,
				"active_liquidity_ratio": 0.00403716878551644,
				"liquidity_ticks": [],
				"liquidity_current_tick": 35516,
				"liquidity_tick_spacing": 50,
				"badges": []
			}]
		}`))
	}))
	defer server.Close()

	source := PoolDataSourceConfig{
		SourceType:       PoolDataSourceTypeMarketPools,
		BaseURL:          server.URL,
		TimeframeMinutes: 5,
		Limit:            2,
	}
	resp, err := NewMarketPoolsClient(server.URL).Pools(context.Background(), source, "bsc", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.RequestedLimit != 2 || resp.RequestedChain != "bsc" || resp.TotalPools != 2 {
		t.Fatalf("unexpected response metadata: %+v", resp)
	}
	if len(resp.RequestedProtocol) != 2 || resp.RequestedProtocol[0] != "v3" {
		t.Fatalf("unexpected requested protocol: %+v", resp.RequestedProtocol)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected one pool, got %d", len(resp.Data))
	}
	item := resp.Data[0]
	if item.PoolAddress != poolAddress {
		t.Fatalf("expected pool address %s, got %s", poolAddress, item.PoolAddress)
	}
	if item.ProtocolVersion != "v3" || item.FactoryName != "PancakeswapV3" {
		t.Fatalf("unexpected pool identity: %+v", item)
	}
	if item.FeeRate != 2500 || item.TransactionCount != 320 || item.TotalFees != 205.30699409588 {
		t.Fatalf("unexpected normalized metrics: %+v", item)
	}
	if item.ActiveLiquidityUSD != 1339.01922192219 || item.CurrentTick != 35516 {
		t.Fatalf("unexpected liquidity fields: %+v", item)
	}
}
