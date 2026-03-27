package assets

import (
	"TgLpBot/base/convert"
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"testing"
	"time"
)

func mustWeiString(t *testing.T, amount float64) string {
	t.Helper()
	value, err := convert.FloatUSDTToWei(amount)
	if err != nil {
		t.Fatalf("convert %.8f to wei: %v", amount, err)
	}
	return value.String()
}

func mustNegativeWeiString(t *testing.T, amount float64) string {
	t.Helper()
	value, err := convert.FloatUSDTToWei(amount)
	if err != nil {
		t.Fatalf("convert negative %.8f to wei: %v", amount, err)
	}
	return "-" + value.String()
}

func locTime(year int, month time.Month, day, hour, minute int) *time.Time {
	value := time.Date(year, month, day, hour, minute, 0, 0, timeutil.Location())
	return &value
}

func TestBuildUserLPStatsFromTrades(t *testing.T) {
	timeutil.Init()
	now := time.Date(2026, time.March, 21, 16, 20, 0, 0, timeutil.Location())

	trades := []userLPTradeRow{
		{
			ID:           1,
			UserID:       7,
			PoolID:       "pool-a",
			Token0Symbol: "USDT",
			Token1Symbol: "PAA",
			Chain:        "bsc",
			ProfitUSDT:   mustWeiString(t, 2),
			ClosedAt:     locTime(2026, time.March, 21, 9, 0),
		},
		{
			ID:           2,
			UserID:       7,
			PoolID:       "pool-b",
			Token0Symbol: "USDT",
			Token1Symbol: "BOOK",
			Chain:        "bsc",
			ProfitUSDT:   mustWeiString(t, 2),
			ClosedAt:     locTime(2026, time.March, 21, 10, 0),
		},
		{
			ID:           3,
			UserID:       7,
			PoolID:       "pool-c",
			Token0Symbol: "USDT",
			Token1Symbol: "AAA",
			Chain:        "bsc",
			ProfitUSDT:   mustWeiString(t, 10),
			ClosedAt:     locTime(2026, time.March, 20, 11, 0),
		},
		{
			ID:           4,
			UserID:       7,
			PoolID:       "pool-c",
			Token0Symbol: "USDT",
			Token1Symbol: "AAA",
			Chain:        "bsc",
			ProfitUSDT:   mustNegativeWeiString(t, 3),
			ClosedAt:     locTime(2026, time.March, 19, 12, 0),
		},
		{
			ID:           5,
			UserID:       7,
			PoolID:       "pool-d",
			Token0Symbol: "USDT",
			Token1Symbol: "ZERO",
			Chain:        "bsc",
			ProfitUSDT:   "0",
			ClosedAt:     locTime(2026, time.March, 18, 13, 0),
		},
		{
			ID:           6,
			UserID:       7,
			PoolID:       "pool-old",
			Token0Symbol: "USDT",
			Token1Symbol: "OLD",
			Chain:        "bsc",
			ProfitUSDT:   mustWeiString(t, 99),
			ClosedAt:     locTime(2026, time.February, 18, 8, 0),
		},
	}

	stats := buildUserLPStatsFromTrades(trades, now)

	if got, want := stats.Today.RealizedPnLUSD, 4.0; got != want {
		t.Fatalf("today realized pnl = %.2f, want %.2f", got, want)
	}
	if got, want := stats.Today.ClosedCount, 2; got != want {
		t.Fatalf("today closed count = %d, want %d", got, want)
	}
	if got, want := stats.Today.WinCount, 2; got != want {
		t.Fatalf("today win count = %d, want %d", got, want)
	}
	if got, want := len(stats.TodayPools), 2; got != want {
		t.Fatalf("today pools = %d, want %d", got, want)
	}
	if got, want := stats.TodayPools[0].ProfitUSD, 2.0; got != want {
		t.Fatalf("first today pool pnl = %.2f, want %.2f", got, want)
	}
	if got, want := stats.TodayPools[1].ProfitUSD, 2.0; got != want {
		t.Fatalf("second today pool pnl = %.2f, want %.2f", got, want)
	}

	if got, want := len(stats.Windows), 3; got != want {
		t.Fatalf("window size = %d, want %d", got, want)
	}
	if got, want := stats.Windows[0].Days, 1; got != want {
		t.Fatalf("first window days = %d, want %d", got, want)
	}
	if got, want := stats.Windows[0].RealizedPnLUSD, 10.0; got != want {
		t.Fatalf("1d pnl = %.2f, want %.2f", got, want)
	}
	if got, want := stats.Windows[1].RealizedPnLUSD, 7.0; got != want {
		t.Fatalf("7d pnl = %.2f, want %.2f", got, want)
	}
	if got, want := stats.Windows[1].ClosedCount, 3; got != want {
		t.Fatalf("7d closed count = %d, want %d", got, want)
	}
	if got, want := stats.Windows[1].WinCount, 1; got != want {
		t.Fatalf("7d win count = %d, want %d", got, want)
	}
	if got, want := stats.Windows[1].LossCount, 1; got != want {
		t.Fatalf("7d loss count = %d, want %d", got, want)
	}
	if got, want := stats.Windows[1].BreakEvenCount, 1; got != want {
		t.Fatalf("7d break-even count = %d, want %d", got, want)
	}
	if got, want := stats.Windows[2].RealizedPnLUSD, 7.0; got != want {
		t.Fatalf("30d pnl = %.2f, want %.2f", got, want)
	}

	if got, want := len(stats.DailyHistory), 3; got != want {
		t.Fatalf("daily history size = %d, want %d", got, want)
	}
	if got, want := stats.DailyHistory[0].Day, "2026-03-18"; got != want {
		t.Fatalf("first daily day = %s, want %s", got, want)
	}
	if got, want := stats.DailyHistory[0].RealizedPnLUSD, 0.0; got != want {
		t.Fatalf("first daily pnl = %.2f, want %.2f", got, want)
	}
	if got, want := stats.DailyHistory[1].RealizedPnLUSD, -3.0; got != want {
		t.Fatalf("second daily pnl = %.2f, want %.2f", got, want)
	}
	if got, want := stats.DailyHistory[2].RealizedPnLUSD, 10.0; got != want {
		t.Fatalf("third daily pnl = %.2f, want %.2f", got, want)
	}
}

func TestApplyUserSnapshotPnL(t *testing.T) {
	timeutil.Init()
	now := time.Date(2026, time.March, 21, 16, 20, 0, 0, timeutil.Location())

	base := UserLPStatsResponse{
		Windows: []UserLPWindowStats{
			{Days: 1, ClosedCount: 1, WinCount: 1, WinRate: 1, AvgPnLUSD: 5, RealizedPnLUSD: 5},
			{Days: 7, ClosedCount: 2, WinCount: 1, LossCount: 1, WinRate: 0.5, AvgPnLUSD: 43.5, RealizedPnLUSD: 87},
			{Days: 30, ClosedCount: 2, WinCount: 1, LossCount: 1, WinRate: 0.5, AvgPnLUSD: 43.5, RealizedPnLUSD: 87},
		},
		Today: UserLPWindowStats{
			ClosedCount:    1,
			WinCount:       1,
			WinRate:        1,
			AvgPnLUSD:      5,
			RealizedPnLUSD: 5,
		},
		DailyHistory: []UserLPDailyPoint{
			{Day: "2026-03-18", RealizedPnLUSD: 80, ClosedCount: 1, WinCount: 1},
			{Day: "2026-03-19", RealizedPnLUSD: 7, ClosedCount: 1, LossCount: 1},
		},
	}

	snapshots := []models.UserAssetDailySnapshot{
		{SnapshotDay: "2026-03-17", TotalUSD: 100},
		{SnapshotDay: "2026-03-18", TotalUSD: 102},
		{SnapshotDay: "2026-03-19", TotalUSD: 99},
		{SnapshotDay: "2026-03-20", TotalUSD: 109},
	}
	liveTotalUSD := 112.0

	stats := applyUserSnapshotPnL(base, snapshots, &liveTotalUSD, now)

	if got, want := stats.Today.RealizedPnLUSD, 3.0; got != want {
		t.Fatalf("today snapshot pnl = %.2f, want %.2f", got, want)
	}
	if got, want := stats.Today.AvgPnLUSD, 3.0; got != want {
		t.Fatalf("today avg pnl = %.2f, want %.2f", got, want)
	}

	if got, want := len(stats.DailyHistory), 3; got != want {
		t.Fatalf("daily history size = %d, want %d", got, want)
	}
	if got, want := stats.DailyHistory[0].Day, "2026-03-18"; got != want {
		t.Fatalf("first history day = %s, want %s", got, want)
	}
	if got, want := stats.DailyHistory[0].RealizedPnLUSD, 2.0; got != want {
		t.Fatalf("2026-03-18 pnl = %.2f, want %.2f", got, want)
	}
	if got, want := stats.DailyHistory[1].RealizedPnLUSD, -3.0; got != want {
		t.Fatalf("2026-03-19 pnl = %.2f, want %.2f", got, want)
	}
	if got, want := stats.DailyHistory[2].RealizedPnLUSD, 10.0; got != want {
		t.Fatalf("2026-03-20 pnl = %.2f, want %.2f", got, want)
	}

	if got, want := stats.Windows[0].RealizedPnLUSD, 10.0; got != want {
		t.Fatalf("1d pnl = %.2f, want %.2f", got, want)
	}
	if got, want := stats.Windows[0].AvgPnLUSD, 10.0; got != want {
		t.Fatalf("1d avg pnl = %.2f, want %.2f", got, want)
	}
	if got, want := stats.Windows[1].RealizedPnLUSD, 9.0; got != want {
		t.Fatalf("7d pnl = %.2f, want %.2f", got, want)
	}
	if got, want := stats.Windows[1].AvgPnLUSD, 4.5; got != want {
		t.Fatalf("7d avg pnl = %.2f, want %.2f", got, want)
	}
	if got, want := stats.Windows[2].RealizedPnLUSD, 9.0; got != want {
		t.Fatalf("30d pnl = %.2f, want %.2f", got, want)
	}
}
