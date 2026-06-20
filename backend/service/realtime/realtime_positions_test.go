package realtime

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"github.com/ethereum/go-ethereum/common"
	"math"
	"testing"
	"time"
)

func TestFinalizeTaskPnLViewMetricsRestoresInitialCostWhenCurrentValueExists(t *testing.T) {
	t.Parallel()

	metrics := finalizeTaskPnLViewMetrics(taskPnLViewMetrics{
		initialCost:  200,
		netInvested:  0,
		currentValue: 199.83,
		absolutePnL:  199.83,
		hasPnL:       true,
		dustTracked:  true,
	}, 200)

	if metrics.netInvested != 200 {
		t.Fatalf("netInvested = %.2f, want 200", metrics.netInvested)
	}
	if metrics.absolutePnL > -0.16 || metrics.absolutePnL < -0.18 {
		t.Fatalf("absolutePnL = %.6f, want about -0.17", metrics.absolutePnL)
	}
	if !metrics.hasPnL {
		t.Fatal("hasPnL = false, want true")
	}
}

func TestFinalizeTaskPnLViewMetricsKeepsZeroWhenNoCurrentValue(t *testing.T) {
	t.Parallel()

	metrics := finalizeTaskPnLViewMetrics(taskPnLViewMetrics{
		initialCost: 100,
		netInvested: 0,
		dustTracked: true,
	}, 100)

	if metrics.netInvested != 0 {
		t.Fatalf("netInvested = %.2f, want 0", metrics.netInvested)
	}
	if metrics.hasPnL {
		t.Fatal("hasPnL = true, want false")
	}
}

func TestFinalizeTaskPnLViewMetricsKeepsZeroAfterRecoveredPrincipal(t *testing.T) {
	t.Parallel()

	metrics := finalizeTaskPnLViewMetrics(taskPnLViewMetrics{
		initialCost:  100,
		netInvested:  0,
		recovered:    100,
		currentValue: 5,
		absolutePnL:  5,
		hasPnL:       true,
	}, 100)

	if metrics.netInvested != 0 {
		t.Fatalf("netInvested = %.2f, want 0", metrics.netInvested)
	}
}

func TestDisplayTaskAmountUSDT(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		task *models.StrategyTask
		want float64
	}{
		{name: "nil", task: nil, want: 0},
		{name: "regular amount", task: &models.StrategyTask{AmountUSDT: 500}, want: 500},
		{name: "dca total still pending", task: &models.StrategyTask{DCAEnabled: true, DCATotalAmountUSDT: 500, AmountUSDT: 400}, want: 500},
		{name: "dca current catches up", task: &models.StrategyTask{DCAEnabled: true, DCATotalAmountUSDT: 500, AmountUSDT: 500}, want: 500},
		{name: "dca current exceeds stale total", task: &models.StrategyTask{DCAEnabled: true, DCATotalAmountUSDT: 500, AmountUSDT: 600}, want: 600},
		{name: "net invested after partial exit", task: &models.StrategyTask{AmountUSDT: 1000}, want: 600},
		{name: "net invested after full principal recovered", task: &models.StrategyTask{AmountUSDT: 1000}, want: 0},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			metrics := taskPnLViewMetrics{}
			if tt.name == "net invested after partial exit" {
				metrics.netInvested = 600
			}
			if tt.name == "net invested after full principal recovered" {
				metrics.recovered = 1000
			}
			if got := displayTaskAmountUSDTWithMetrics(tt.task, metrics); got != tt.want {
				t.Fatalf("displayTaskAmountUSDTWithMetrics() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFollowStrategySummaryForTask(t *testing.T) {
	t.Parallel()

	enabled := true
	disabled := false

	tests := []struct {
		name  string
		task  *models.StrategyTask
		close *bool
		want  string
	}{
		{name: "nil task", want: ""},
		{name: "non-follow task", task: &models.StrategyTask{}, want: ""},
		{
			name:  "follow close enabled",
			task:  &models.StrategyTask{IsFollow: true},
			close: &enabled,
			want:  "目标撤仓跟随 / 下破保底撤出 / 上破继续跟随",
		},
		{
			name:  "follow close disabled",
			task:  &models.StrategyTask{IsFollow: true},
			close: &disabled,
			want:  "目标撤仓未开启 / 下破保底撤出 / 上破继续跟随",
		},
		{
			name: "follow close unknown",
			task: &models.StrategyTask{IsFollow: true},
			want: "目标撤仓未确认 / 下破保底撤出 / 上破继续跟随",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := followStrategySummaryForTask(tt.task, tt.close); got != tt.want {
				t.Fatalf("followStrategySummaryForTask() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSortRealtimePositionsByCreationTimeAscending(t *testing.T) {
	t.Parallel()

	early := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	middle := early.Add(2 * time.Hour)
	late := early.Add(4 * time.Hour)

	positions := []RealtimePosition{
		{Title: "late", PositionID: "3", RunningSince: &late, Totals: RealtimeTotals{TotalUSD: 10}},
		{Title: "early", PositionID: "1", RunningSince: &early, Totals: RealtimeTotals{TotalUSD: 1000}},
		{Title: "middle", PositionID: "2", RunningSince: &middle, Totals: RealtimeTotals{TotalUSD: 500}},
	}

	sortRealtimePositions(positions)

	if got := []string{positions[0].Title, positions[1].Title, positions[2].Title}; got[0] != "early" || got[1] != "middle" || got[2] != "late" {
		t.Fatalf("sortRealtimePositions order = %v, want [early middle late]", got)
	}
}

func TestSortRealtimePositionsKeepsMissingCreationTimeAfterCreatedPositions(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	positions := []RealtimePosition{
		{Title: "chain-only", PoolID: "0x1", PositionID: "1"},
		{Title: "created", PoolID: "0x2", PositionID: "2", RunningSince: &createdAt},
	}

	sortRealtimePositions(positions)

	if positions[0].Title != "created" || positions[1].Title != "chain-only" {
		t.Fatalf("sortRealtimePositions order = [%s %s], want [created chain-only]", positions[0].Title, positions[1].Title)
	}
}

func TestV4TaskCurrenciesAllowNativeCurrency(t *testing.T) {
	t.Parallel()

	c0, c1, ok := v4TaskCurrencies(&models.StrategyTask{
		PoolVersion:   "v4",
		Token0Address: common.Address{}.Hex(),
		Token1Address: "0x0000000000000000000000000000000000000001",
	})
	if !ok {
		t.Fatal("v4TaskCurrencies() ok = false, want true")
	}
	if c0 != (common.Address{}) {
		t.Fatalf("currency0 = %s, want native zero address", c0.Hex())
	}
	if c1 != common.HexToAddress("0x0000000000000000000000000000000000000001") {
		t.Fatalf("currency1 = %s", c1.Hex())
	}
}

func TestV4TaskCurrenciesRejectMissingOrDoubleNative(t *testing.T) {
	t.Parallel()

	if _, _, ok := v4TaskCurrencies(&models.StrategyTask{PoolVersion: "v4"}); ok {
		t.Fatal("empty token metadata accepted")
	}
	if _, _, ok := v4TaskCurrencies(&models.StrategyTask{
		PoolVersion:   "v4",
		Token0Address: common.Address{}.Hex(),
		Token1Address: common.Address{}.Hex(),
	}); ok {
		t.Fatal("double native currencies accepted")
	}
}

func TestV4CurrencyMetaUsesNativeSymbolForZeroAddress(t *testing.T) {
	oldConfig := config.AppConfig
	defer func() { config.AppConfig = oldConfig }()
	config.AppConfig = &config.Config{
		Chains: map[string]config.ChainConfig{
			"bsc": {
				Chain:               "bsc",
				WrappedNativeSymbol: "WBNB",
			},
		},
	}

	got := (&RealtimePositionsService{}).getV4CurrencyMeta("bsc", common.Address{})
	if got.symbol != "BNB" {
		t.Fatalf("symbol = %q, want BNB", got.symbol)
	}
	if got.decimals != 18 {
		t.Fatalf("decimals = %d, want 18", got.decimals)
	}
}

func TestDeriveRealtimePoolTokenPricesFromStableSide(t *testing.T) {
	t.Parallel()

	price0, price1 := deriveRealtimePoolTokenPrices(
		"bsc",
		common.HexToAddress("0x0000000000000000000000000000000000000001"),
		common.HexToAddress("0x55d398326f99059ff775485246999027b3197955"),
		realtimeTokenMeta{symbol: "TEST", decimals: 18},
		realtimeTokenMeta{symbol: "USDT", decimals: 18},
		6932,
		true,
		0,
		0,
	)

	if math.Abs(price0-1.9998) > 0.01 {
		t.Fatalf("price0 = %.6f, want about 2", price0)
	}
	if price1 != 1 {
		t.Fatalf("price1 = %.6f, want 1", price1)
	}
}

func TestDeriveRealtimePoolTokenPricesFromKnownTokenSide(t *testing.T) {
	t.Parallel()

	price0, price1 := deriveRealtimePoolTokenPrices(
		"bsc",
		common.HexToAddress("0x0000000000000000000000000000000000000001"),
		common.HexToAddress("0x0000000000000000000000000000000000000002"),
		realtimeTokenMeta{symbol: "WBNB", decimals: 18},
		realtimeTokenMeta{symbol: "TEST", decimals: 18},
		6932,
		true,
		700,
		0,
	)

	if price0 != 700 {
		t.Fatalf("price0 = %.6f, want 700", price0)
	}
	if math.Abs(price1-350.03) > 0.5 {
		t.Fatalf("price1 = %.6f, want about 350", price1)
	}
}

func TestBuildRealtimeDCAStatusPending(t *testing.T) {
	t.Parallel()

	next := time.Now().Add(time.Minute)
	task := &models.StrategyTask{
		DCAEnabled:         true,
		DCAPercentagesJSON: "[40,60]",
		DCAExecutedCount:   1,
		DCARetryCount:      2,
		DCANextBatchAt:     &next,
	}

	got := buildRealtimeDCAStatus(task)
	if got == nil {
		t.Fatal("buildRealtimeDCAStatus() = nil, want status")
	}
	if !got.Enabled || !got.PlanValid {
		t.Fatalf("status enabled/plan_valid = %v/%v, want true/true", got.Enabled, got.PlanValid)
	}
	if got.ExecutedCount != 1 || got.TotalCount != 2 || got.RetryCount != 2 {
		t.Fatalf("counts = executed:%d total:%d retry:%d, want 1/2/2", got.ExecutedCount, got.TotalCount, got.RetryCount)
	}
	if got.NextBatchAt != &next {
		t.Fatalf("NextBatchAt pointer mismatch")
	}
	if !got.Pending || got.Finished || got.Completed || got.Canceled {
		t.Fatalf("pending/finished/completed/canceled = %v/%v/%v/%v, want true/false/false/false", got.Pending, got.Finished, got.Completed, got.Canceled)
	}
}

func TestBuildRealtimeDCAStatusCompleted(t *testing.T) {
	t.Parallel()

	task := &models.StrategyTask{
		DCAEnabled:         true,
		DCAPercentagesJSON: "[40,60]",
		DCAExecutedCount:   2,
	}

	got := buildRealtimeDCAStatus(task)
	if got == nil {
		t.Fatal("buildRealtimeDCAStatus() = nil, want status")
	}
	if !got.PlanValid || got.TotalCount != 2 {
		t.Fatalf("plan = valid:%v total:%d, want true/2", got.PlanValid, got.TotalCount)
	}
	if got.Pending || !got.Finished || !got.Completed || got.Canceled {
		t.Fatalf("pending/finished/completed/canceled = %v/%v/%v/%v, want false/true/true/false", got.Pending, got.Finished, got.Completed, got.Canceled)
	}
}

func TestBuildRealtimeDCAStatusCanceled(t *testing.T) {
	t.Parallel()

	task := &models.StrategyTask{
		DCAEnabled:         true,
		DCAPercentagesJSON: "[40,30,30]",
		DCAExecutedCount:   1,
	}

	got := buildRealtimeDCAStatus(task)
	if got == nil {
		t.Fatal("buildRealtimeDCAStatus() = nil, want status")
	}
	if got.Pending || !got.Finished || got.Completed || !got.Canceled {
		t.Fatalf("pending/finished/completed/canceled = %v/%v/%v/%v, want false/true/false/true", got.Pending, got.Finished, got.Completed, got.Canceled)
	}
}

func TestBuildRealtimeDCAStatusInvalidPlan(t *testing.T) {
	t.Parallel()

	task := &models.StrategyTask{
		DCAEnabled:         true,
		DCAPercentagesJSON: "bad",
		DCAExecutedCount:   1,
	}

	got := buildRealtimeDCAStatus(task)
	if got == nil {
		t.Fatal("buildRealtimeDCAStatus() = nil, want status")
	}
	if got.PlanValid {
		t.Fatal("PlanValid = true, want false")
	}
	if got.TotalCount != 0 {
		t.Fatalf("TotalCount = %d, want 0", got.TotalCount)
	}
	if got.ExecutedCount != 1 {
		t.Fatalf("ExecutedCount = %d, want 1", got.ExecutedCount)
	}
}
