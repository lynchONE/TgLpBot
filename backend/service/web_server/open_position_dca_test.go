package web_server

import (
	"testing"

	"TgLpBot/base/models"
)

func TestResolveDCAPlanDisablesSplitBelowThreshold(t *testing.T) {
	t.Parallel()

	cfg := &models.GlobalConfig{
		DCAEnabled:            true,
		DCAPercentagesJSON:    "[40,60]",
		DCAIntervalSeconds:    30,
		DCAMinSplitAmountUSDT: 300,
	}

	enabled, percentages, interval, err := resolveDCAPlan(cfg, openPositionRequest{Amount: 250})
	if err != nil {
		t.Fatalf("resolveDCAPlan returned error: %v", err)
	}
	if enabled {
		t.Fatal("enabled = true, want false when amount is below threshold")
	}
	if percentages != nil {
		t.Fatalf("percentages = %#v, want nil when DCA disabled", percentages)
	}
	if interval != 0 {
		t.Fatalf("interval = %v, want 0 when DCA disabled", interval)
	}
}

func TestResolveDCAPlanKeepsSplitAtThreshold(t *testing.T) {
	t.Parallel()

	cfg := &models.GlobalConfig{
		DCAEnabled:            true,
		DCAPercentagesJSON:    "[40,60]",
		DCAIntervalSeconds:    12.3456,
		DCAMinSplitAmountUSDT: 250,
	}

	enabled, percentages, interval, err := resolveDCAPlan(cfg, openPositionRequest{Amount: 250})
	if err != nil {
		t.Fatalf("resolveDCAPlan returned error: %v", err)
	}
	if !enabled {
		t.Fatal("enabled = false, want true at threshold")
	}
	if len(percentages) != 2 || percentages[0] != 40 || percentages[1] != 60 {
		t.Fatalf("percentages = %#v, want [40 60]", percentages)
	}
	if interval != 12.346 {
		t.Fatalf("interval = %v, want 12.346", interval)
	}
}

func TestResolveDCAPlanRejectsInvalidThreshold(t *testing.T) {
	t.Parallel()

	cfg := &models.GlobalConfig{
		DCAEnabled:            true,
		DCAPercentagesJSON:    "[50,50]",
		DCAIntervalSeconds:    30,
		DCAMinSplitAmountUSDT: -1,
	}

	_, _, _, err := resolveDCAPlan(cfg, openPositionRequest{Amount: 500})
	if err == nil {
		t.Fatal("expected error for invalid negative threshold")
	}
}
