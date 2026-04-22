package models

import "testing"

func TestStrategyTaskCreateOverrideUpdates(t *testing.T) {
	t.Parallel()

	task := &StrategyTask{
		ReopenDelaySeconds: 0,
		SlippageTolerance:  0,
		ResidualTolerance:  0,
		ZapLossTolerance:   0,
		RebalanceEnabled:   false,
	}

	updates := task.CreateOverrideUpdates()
	if got := updates["reopen_delay_seconds"]; got != 0 {
		t.Fatalf("reopen_delay_seconds = %#v, want 0", got)
	}
	if got := updates["slippage_tolerance"]; got != 0 {
		t.Fatalf("slippage_tolerance = %#v, want 0", got)
	}
	if got := updates["residual_tolerance"]; got != 0 {
		t.Fatalf("residual_tolerance = %#v, want 0", got)
	}
	if got := updates["zap_loss_tolerance"]; got != 0 {
		t.Fatalf("zap_loss_tolerance = %#v, want 0", got)
	}
	if got := updates["rebalance_enabled"]; got != false {
		t.Fatalf("rebalance_enabled = %#v, want false", got)
	}
}

func TestStrategyTaskCreateOverrideUpdatesSkipsNonZeroDefaults(t *testing.T) {
	t.Parallel()

	task := &StrategyTask{
		ReopenDelaySeconds: 120,
		SlippageTolerance:  0.5,
		ResidualTolerance:  1.0,
		ZapLossTolerance:   0.5,
		RebalanceEnabled:   true,
		DCAEnabled:         true,
		DCAIntervalSeconds: 15,
		DCAExecutedCount:   1,
	}

	updates := task.CreateOverrideUpdates()
	if len(updates) != 0 {
		t.Fatalf("len(updates) = %d, want 0", len(updates))
	}
}
