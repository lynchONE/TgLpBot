package realtime

import (
	"TgLpBot/base/models"
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
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := displayTaskAmountUSDT(tt.task); got != tt.want {
				t.Fatalf("displayTaskAmountUSDT() = %v, want %v", got, tt.want)
			}
		})
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
