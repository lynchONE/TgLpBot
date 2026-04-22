package web_server

import "testing"

func TestResolveOpenPositionRebalanceEnabledDefaultsEnabled(t *testing.T) {
	t.Parallel()

	rebalanceEnabled := resolveOpenPositionRebalanceEnabled(openPositionRequest{})
	if !rebalanceEnabled {
		t.Fatal("rebalanceEnabled = false, want true")
	}
}

func TestResolveOpenPositionRebalanceEnabledAppliesOverrides(t *testing.T) {
	t.Parallel()

	rebalanceEnabled := false
	gotRebalance := resolveOpenPositionRebalanceEnabled(openPositionRequest{
		RebalanceEnabled: &rebalanceEnabled,
	})
	if gotRebalance {
		t.Fatal("rebalanceEnabled = true, want false")
	}
}
