package web_server

import (
	"TgLpBot/base/models"
	"testing"
)

func TestResolveOpenPositionTaskModeDefaultsExitAll(t *testing.T) {
	t.Parallel()

	mode, paused := resolveOpenPositionTaskMode(openPositionRequest{})
	if mode != models.StrategyOutOfRangeModeExitAll {
		t.Fatalf("mode = %q, want %q", mode, models.StrategyOutOfRangeModeExitAll)
	}
	if paused {
		t.Fatal("paused = true, want false")
	}
}

func TestResolveOpenPositionTaskModeSupportsLegacyToggle(t *testing.T) {
	t.Parallel()

	enabled := true
	mode, paused := resolveOpenPositionTaskMode(openPositionRequest{
		RebalanceEnabled: &enabled,
	})
	if mode != models.StrategyOutOfRangeModeRebalanceAll {
		t.Fatalf("mode = %q, want %q", mode, models.StrategyOutOfRangeModeRebalanceAll)
	}
	if paused {
		t.Fatal("paused = true, want false")
	}
}

func TestResolveOpenPositionTaskModeSupportsPause(t *testing.T) {
	t.Parallel()

	mode, paused := resolveOpenPositionTaskMode(openPositionRequest{
		TaskMode: models.StrategyTaskModePause,
	})
	if mode != models.StrategyOutOfRangeModeExitAll {
		t.Fatalf("mode = %q, want %q", mode, models.StrategyOutOfRangeModeExitAll)
	}
	if !paused {
		t.Fatal("paused = false, want true")
	}
}
