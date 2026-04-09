package realtime

import "testing"

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
