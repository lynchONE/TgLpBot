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
