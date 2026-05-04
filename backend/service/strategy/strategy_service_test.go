package strategy

import (
	"TgLpBot/base/models"
	"testing"
	"time"
)

func TestShouldDelayOutOfRangeHandling(t *testing.T) {
	t.Parallel()

	if ShouldDelayOutOfRangeHandling(nil) {
		t.Fatal("nil task should not delay out-of-range handling")
	}

	if !ShouldDelayOutOfRangeHandling(&models.StrategyTask{RangeActivationPending: true}) {
		t.Fatal("pending activation task should delay out-of-range handling")
	}

	if ShouldDelayOutOfRangeHandling(&models.StrategyTask{RangeActivationPending: false}) {
		t.Fatal("activated task should not delay out-of-range handling")
	}
}

func TestShouldMonitorPausedDCA(t *testing.T) {
	t.Parallel()

	nextBatchAt := time.Now()
	baseTask := models.StrategyTask{
		Paused:         true,
		Status:         models.StrategyStatusRunning,
		DCAEnabled:     true,
		DCANextBatchAt: &nextBatchAt,
	}

	if shouldMonitorPausedDCA(nil) {
		t.Fatal("nil task should not monitor paused DCA")
	}

	if !shouldMonitorPausedDCA(&baseTask) {
		t.Fatal("paused running task with queued DCA should be monitored")
	}

	notPaused := baseTask
	notPaused.Paused = false
	if shouldMonitorPausedDCA(&notPaused) {
		t.Fatal("non-paused task should not use paused DCA monitor path")
	}

	waiting := baseTask
	waiting.Status = models.StrategyStatusWaiting
	if shouldMonitorPausedDCA(&waiting) {
		t.Fatal("paused waiting task should not monitor DCA")
	}

	disabled := baseTask
	disabled.DCAEnabled = false
	if shouldMonitorPausedDCA(&disabled) {
		t.Fatal("paused task with disabled DCA should not be monitored")
	}

	noNextBatch := baseTask
	noNextBatch.DCANextBatchAt = nil
	if shouldMonitorPausedDCA(&noNextBatch) {
		t.Fatal("paused DCA task without queued next batch should not be monitored")
	}
}

func TestShouldStopOutOfRange(t *testing.T) {
	t.Parallel()

	if ShouldStopOutOfRange(nil) {
		t.Fatal("nil task should not stop out-of-range")
	}

	if ShouldStopOutOfRange(&models.StrategyTask{RebalanceEnabled: true}) {
		t.Fatal("rebalance-enabled task should not auto-stop out-of-range")
	}

	if !ShouldStopOutOfRange(&models.StrategyTask{RebalanceEnabled: false}) {
		t.Fatal("rebalance-disabled task should auto-stop out-of-range")
	}
}

func TestResolveOutOfRangeAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		task *models.StrategyTask
		isUp bool
		want OutOfRangeAction
	}{
		{
			name: "rebalance all on upward breakout",
			task: &models.StrategyTask{OutOfRangeMode: string(models.StrategyOutOfRangeModeRebalanceAll)},
			isUp: true,
			want: OutOfRangeActionRebalance,
		},
		{
			name: "exit all on upward breakout",
			task: &models.StrategyTask{OutOfRangeMode: string(models.StrategyOutOfRangeModeExitAll)},
			isUp: true,
			want: OutOfRangeActionExit,
		},
		{
			name: "rebalance up exit down upward breakout",
			task: &models.StrategyTask{OutOfRangeMode: string(models.StrategyOutOfRangeModeRebalanceUpExitDown)},
			isUp: true,
			want: OutOfRangeActionRebalance,
		},
		{
			name: "rebalance up exit down downward breakout",
			task: &models.StrategyTask{OutOfRangeMode: string(models.StrategyOutOfRangeModeRebalanceUpExitDown)},
			want: OutOfRangeActionExit,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ResolveOutOfRangeAction(tt.task, tt.isUp, !tt.isUp); got != tt.want {
				t.Fatalf("ResolveOutOfRangeAction() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPauseBlocksExitAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		action string
		want   bool
	}{
		{action: "", want: false},
		{action: ExitActionManualStop, want: false},
		{action: ExitActionSwitch, want: false},
		{action: ExitActionRebalance, want: true},
		{action: ExitActionStopLoss, want: true},
		{action: ExitActionOutOfRangeStop, want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.action, func(t *testing.T) {
			t.Parallel()

			if got := pauseBlocksExitAction(tt.action); got != tt.want {
				t.Fatalf("pauseBlocksExitAction(%q) = %v, want %v", tt.action, got, tt.want)
			}
		})
	}
}
