package strategy

import (
	"TgLpBot/base/models"
	"math"
	"testing"
)

func TestNextDCARecordedAmountUSDTClampsStaleTotal(t *testing.T) {
	t.Parallel()

	task := &models.StrategyTask{
		DCAEnabled:         true,
		DCATotalAmountUSDT: 1000,
		AmountUSDT:         1000,
	}
	if got := nextDCARecordedAmountUSDT(task, 500); got != 1000 {
		t.Fatalf("nextDCARecordedAmountUSDT() = %.2f, want 1000", got)
	}
}

func TestNextDCARecordedAmountUSDTKeepsActualBelowPlan(t *testing.T) {
	t.Parallel()

	task := &models.StrategyTask{
		DCAEnabled:         true,
		DCATotalAmountUSDT: 1000,
		AmountUSDT:         500,
	}
	if got := nextDCARecordedAmountUSDT(task, 463.83); math.Abs(got-963.83) > 0.000001 {
		t.Fatalf("nextDCARecordedAmountUSDT() = %.2f, want 963.83", got)
	}
}

func TestExpectedOpenBudgetUSDTUsesExecutedDCAPlan(t *testing.T) {
	t.Parallel()

	task := &models.StrategyTask{
		DCAEnabled:         true,
		DCATotalAmountUSDT: 1000,
		DCAPercentagesJSON: "[50,50]",
		DCAExecutedCount:   2,
		AmountUSDT:         963.83,
	}
	if got := expectedOpenBudgetUSDT(task); got != 1000 {
		t.Fatalf("expectedOpenBudgetUSDT() = %.2f, want 1000", got)
	}

	task.DCAExecutedCount = 1
	if got := expectedOpenBudgetUSDT(task); got != 500 {
		t.Fatalf("expectedOpenBudgetUSDT() = %.2f, want 500", got)
	}
}
