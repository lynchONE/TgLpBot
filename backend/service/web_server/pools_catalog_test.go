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
