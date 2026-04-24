package realtime

import (
	"TgLpBot/base/models"
	"testing"
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
