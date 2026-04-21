package web_server

import "testing"

func TestResolveOpenPositionExecutionFlagsDefaultsEnabled(t *testing.T) {
	t.Parallel()

	rebalanceEnabled, stopLossEnabled := resolveOpenPositionExecutionFlags(openPositionRequest{})
	if !rebalanceEnabled {
		t.Fatal("rebalanceEnabled = false, want true")
	}
	if !stopLossEnabled {
		t.Fatal("stopLossEnabled = false, want true")
	}
}

func TestResolveOpenPositionExecutionFlagsAppliesOverrides(t *testing.T) {
	t.Parallel()

	rebalanceEnabled := false
	stopLossEnabled := false
	gotRebalance, gotStopLoss := resolveOpenPositionExecutionFlags(openPositionRequest{
		RebalanceEnabled: &rebalanceEnabled,
		StopLossEnabled:  &stopLossEnabled,
	})
	if gotRebalance {
		t.Fatal("rebalanceEnabled = true, want false")
	}
	if gotStopLoss {
		t.Fatal("stopLossEnabled = true, want false")
	}
}
