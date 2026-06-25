package web_server

import (
	"TgLpBot/base/models"
	"context"
	"testing"
	"time"
)

func TestBuildPoolCatalogResponseFiltersLowLiquidityPools(t *testing.T) {
	t.Parallel()

	now := time.Now()
	rows := []models.Pool{
		{
			Chain:              "bsc",
			Address:            "0x1111111111111111111111111111111111111111",
			FactoryName:        "pcs",
			Name:               "AAA/USDT",
			Token0Symbol:       "AAA",
			Token1Symbol:       "USDT",
			ActiveLiquidityUSD: 99,
			CurrentPoolValue:   400,
			UpdatedAt:          now,
		},
		{
			Chain:              "bsc",
			Address:            "0x2222222222222222222222222222222222222222",
			FactoryName:        "pcs",
			Name:               "BBB/USDT",
			Token0Symbol:       "BBB",
			Token1Symbol:       "USDT",
			ActiveLiquidityUSD: 180,
			CurrentPoolValue:   600,
			UpdatedAt:          now.Add(time.Second),
		},
	}

	resp := (&Server{}).buildPoolCatalogResponse(context.Background(), rows, poolCatalogOptions{
		Chain:            "bsc",
		Sort:             "fees",
		TimeframeMinutes: 5,
		Limit:            10,
	})

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 visible pool after filtering, got %d", len(resp.Data))
	}
	if resp.Data[0].PoolAddress != "0x2222222222222222222222222222222222222222" {
		t.Fatalf("unexpected pool left after filtering: %s", resp.Data[0].PoolAddress)
	}
}

func TestPoolCatalogPickMarketCapTokenExcludesQuoteToken(t *testing.T) {
	t.Parallel()

	addr, symbol := poolCatalogPickMarketCapToken("bsc", HotPoolResponse{
		Token0Address: "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c",
		Token0Symbol:  "WBNB",
		Token1Address: "0x1111111111111111111111111111111111111111",
		Token1Symbol:  "AAA",
	})

	if addr != "0x1111111111111111111111111111111111111111" || symbol != "AAA" {
		t.Fatalf("market cap token = %s/%s, want AAA token", addr, symbol)
	}
}

func TestPoolCatalogFDVUSDUsesFDVBeforeCurrentTokenFDV(t *testing.T) {
	t.Parallel()

	got := poolCatalogFDVUSD(HotPoolResponse{
		MarketCapUSD:       1_000,
		FDVUSD:             4_000,
		CurrentTokenFDVUSD: 3_000,
	})

	if got != 4_000 {
		t.Fatalf("fdv metric = %.0f, want 4000", got)
	}
}

func TestBuildPoolCatalogItemMarksV4DynamicFee(t *testing.T) {
	t.Parallel()

	item := buildPoolCatalogItem(models.Pool{
		Chain:             "bsc",
		ProtocolVersion:   "v4",
		Address:           "0x7b1818047437a598480e552a60a6edd374a5ff6b8afd58aab9d07aff9dd90b31",
		Name:              "USDT/ARX",
		PoolMFeeRate:      0x800000,
		PoolFeePercentage: 838.8608,
		TotalFees:         42,
		CurrentPoolValue:  1000,
	}, poolCatalogOptions{Chain: "bsc", TimeframeMinutes: 5})

	if !item.FeeDynamic {
		t.Fatal("expected v4 dynamic fee flag to be marked dynamic")
	}
	if item.FeeTier != 0 || item.FeePercentage != 0 {
		t.Fatalf("dynamic fee output = tier %d pct %.4f, want zeros", item.FeeTier, item.FeePercentage)
	}
	if item.FeeRate != 4.2 {
		t.Fatalf("fee_rate = %.4f, want yield metric untouched", item.FeeRate)
	}
}
