package smart_money

import (
	"TgLpBot/base/models"
	"testing"
	"time"
)

func TestSortPoolAggRowsPrioritizesRecentOperations(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	rows := []PoolAggRow{
		{PoolAddress: "0xoldsmall", LatestEventAt: now.Add(-5 * time.Hour), TotalPositionAmountUSD: 100},
		{PoolAddress: "0xrecentlarge", LatestEventAt: now.Add(-90 * time.Minute), TotalPositionAmountUSD: 1000},
		{PoolAddress: "0xoldlarge", LatestEventAt: now.Add(-130 * time.Minute), TotalPositionAmountUSD: 9999},
		{PoolAddress: "0xrecentnew", LatestEventAt: now.Add(-5 * time.Minute), TotalPositionAmountUSD: 10},
	}

	sortPoolAggRows(rows, now)

	want := []string{"0xrecentnew", "0xrecentlarge", "0xoldlarge", "0xoldsmall"}
	for i, row := range rows {
		if row.PoolAddress != want[i] {
			t.Fatalf("row %d = %s, want %s; rows=%v", i, row.PoolAddress, want[i], rows)
		}
	}
}

func TestSortPoolAggRowsFallsBackToAmountOutsideRecentWindow(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	rows := []PoolAggRow{
		{PoolAddress: "0xoldnewer", LatestEventAt: now.Add(-3 * time.Hour), TotalPositionAmountUSD: 100},
		{PoolAddress: "0xoldlarger", LatestEventAt: now.Add(-4 * time.Hour), TotalPositionAmountUSD: 500},
		{PoolAddress: "0xboundary", LatestEventAt: now.Add(-2 * time.Hour), TotalPositionAmountUSD: 1},
	}

	sortPoolAggRows(rows, now)

	want := []string{"0xboundary", "0xoldlarger", "0xoldnewer"}
	for i, row := range rows {
		if row.PoolAddress != want[i] {
			t.Fatalf("row %d = %s, want %s; rows=%v", i, row.PoolAddress, want[i], rows)
		}
	}
}

func TestBuildPoolFeeHeatmapRowsSortsByFee(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	feeSmall := "5"
	feeLarge := "20"
	amount := "1000"

	rows := buildPoolFeeHeatmapRows([]models.SmartMoneyActivePosition{
		{
			PositionRef:   "small",
			PoolAddress:   "0xsmall",
			WalletAddress: "0x0000000000000000000000000000000000000001",
			FeeUSD:        &feeSmall,
			NetTotalUSD:   &amount,
			OpenedAt:      now.Add(-time.Hour),
			Token0Symbol:  "A",
			Token1Symbol:  "B",
			Token0Address: "0xa",
			Token1Address: "0xb",
			Protocol:      "pancake_v3",
			ChainID:       56,
			IsActive:      true,
		},
		{
			PositionRef:   "large",
			PoolAddress:   "0xlarge",
			WalletAddress: "0x0000000000000000000000000000000000000002",
			FeeUSD:        &feeLarge,
			NetTotalUSD:   &amount,
			OpenedAt:      now.Add(-time.Hour),
			Token0Symbol:  "C",
			Token1Symbol:  "D",
			Token0Address: "0xc",
			Token1Address: "0xd",
			Protocol:      "uniswap_v3",
			ChainID:       56,
			IsActive:      true,
		},
	}, PoolFeeHeatmapOptions{WindowSeconds: 60, Sort: "fee", Now: now})

	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want 2", len(rows))
	}
	if rows[0].PoolAddress != "0xlarge" || rows[1].PoolAddress != "0xsmall" {
		t.Fatalf("pool order = [%s %s], want [0xlarge 0xsmall]", rows[0].PoolAddress, rows[1].PoolAddress)
	}
	if rows[0].SampleStatus != "ok" {
		t.Fatalf("SampleStatus = %s, want ok", rows[0].SampleStatus)
	}
}

func TestBuildPoolFeeHeatmapRowsSortsByNormalizedRate(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	fee := "10"
	smallAmount := "1000"
	largeAmount := "10000"

	rows := buildPoolFeeHeatmapRows([]models.SmartMoneyActivePosition{
		{
			PositionRef:   "large-capital",
			PoolAddress:   "0xlargecapital",
			WalletAddress: "0x0000000000000000000000000000000000000001",
			FeeUSD:        &fee,
			NetTotalUSD:   &largeAmount,
			OpenedAt:      now.Add(-time.Hour),
			Token0Symbol:  "A",
			Token1Symbol:  "B",
			Token0Address: "0xa",
			Token1Address: "0xb",
			Protocol:      "pancake_v3",
			ChainID:       56,
			IsActive:      true,
		},
		{
			PositionRef:   "small-capital",
			PoolAddress:   "0xsmallcapital",
			WalletAddress: "0x0000000000000000000000000000000000000002",
			FeeUSD:        &fee,
			NetTotalUSD:   &smallAmount,
			OpenedAt:      now.Add(-time.Hour),
			Token0Symbol:  "C",
			Token1Symbol:  "D",
			Token0Address: "0xc",
			Token1Address: "0xd",
			Protocol:      "uniswap_v3",
			ChainID:       56,
			IsActive:      true,
		},
	}, PoolFeeHeatmapOptions{WindowSeconds: 60, Sort: "rate", Now: now})

	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want 2", len(rows))
	}
	if rows[0].PoolAddress != "0xsmallcapital" || rows[1].PoolAddress != "0xlargecapital" {
		t.Fatalf("pool order = [%s %s], want [0xsmallcapital 0xlargecapital]", rows[0].PoolAddress, rows[1].PoolAddress)
	}
	if rows[0].FeeRatePer1KUSDWindow <= rows[1].FeeRatePer1KUSDWindow {
		t.Fatalf("rate order invalid: %.8f <= %.8f", rows[0].FeeRatePer1KUSDWindow, rows[1].FeeRatePer1KUSDWindow)
	}
}

func TestBuildPoolFeeHeatmapRowsReportsMissingAmount(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	fee := "10"

	rows := buildPoolFeeHeatmapRows([]models.SmartMoneyActivePosition{
		{
			PositionRef:   "missing-amount",
			PoolAddress:   "0xmissing",
			WalletAddress: "0x0000000000000000000000000000000000000001",
			FeeUSD:        &fee,
			OpenedAt:      now.Add(-time.Hour),
			Token0Symbol:  "A",
			Token1Symbol:  "B",
			Token0Address: "0xa",
			Token1Address: "0xb",
			Protocol:      "pancake_v3",
			ChainID:       56,
			IsActive:      true,
		},
	}, PoolFeeHeatmapOptions{WindowSeconds: 60, Sort: "rate", Now: now})

	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].SampleStatus != "insufficient" {
		t.Fatalf("SampleStatus = %s, want insufficient", rows[0].SampleStatus)
	}
	if rows[0].MissingAmountCount != 1 || rows[0].RatePositionCount != 0 {
		t.Fatalf("missing/rate counts = %d/%d, want 1/0", rows[0].MissingAmountCount, rows[0].RatePositionCount)
	}
}
