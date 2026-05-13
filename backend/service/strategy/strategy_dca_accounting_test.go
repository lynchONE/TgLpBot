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

func TestRecoveredUSDTFromOpenRecord(t *testing.T) {
	t.Parallel()

	rec := &models.TradeRecord{
		CloseUSDTReceived: "400000000000000000000",
	}
	got, err := recoveredUSDTFromOpenRecord(rec)
	if err != nil {
		t.Fatalf("recoveredUSDTFromOpenRecord() error = %v", err)
	}
	if math.Abs(got-400) > 0.000001 {
		t.Fatalf("recoveredUSDTFromOpenRecord() = %.6f, want 400", got)
	}

	rec.CloseUSDTReceived = ""
	got, err = recoveredUSDTFromOpenRecord(rec)
	if err != nil {
		t.Fatalf("recoveredUSDTFromOpenRecord(empty) error = %v", err)
	}
	if got != 0 {
		t.Fatalf("recoveredUSDTFromOpenRecord(empty) = %.6f, want 0", got)
	}
}

func TestTaskPnLSubtractsRecoveredUSDT(t *testing.T) {
	t.Parallel()

	initialCost := 1000.0
	recoveredUSDT, err := recoveredUSDTFromOpenRecord(&models.TradeRecord{
		CloseUSDTReceived: "400000000000000000000",
	})
	if err != nil {
		t.Fatalf("recoveredUSDTFromOpenRecord() error = %v", err)
	}
	netInvested := initialCost - recoveredUSDT
	if netInvested < 0 {
		netInvested = 0
	}

	if math.Abs(netInvested-600) > 0.000001 {
		t.Fatalf("netInvested = %.6f, want 600", netInvested)
	}
}
