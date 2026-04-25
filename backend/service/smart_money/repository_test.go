package smart_money

import (
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
