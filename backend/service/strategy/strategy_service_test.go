package strategy

import (
	"TgLpBot/base/models"
	"TgLpBot/service/pricing"
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

func TestShouldExitFollowDownside(t *testing.T) {
	t.Parallel()

	if ShouldExitFollowDownside(nil, true) {
		t.Fatal("nil task should not trigger follow downside guard")
	}
	if ShouldExitFollowDownside(&models.StrategyTask{IsFollow: false}, true) {
		t.Fatal("non-follow task should not use follow downside guard")
	}
	if ShouldExitFollowDownside(&models.StrategyTask{IsFollow: true}, false) {
		t.Fatal("follow task should not exit when not below range")
	}
	if !ShouldExitFollowDownside(&models.StrategyTask{IsFollow: true}, true) {
		t.Fatal("follow task should exit when price is below range")
	}
}

func TestFollowDownsideGuardUsesDisplayPriceDirection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		task        *models.StrategyTask
		currentTick int
		wantExit    bool
	}{
		{
			name:        "quote token1 below range exits",
			task:        &models.StrategyTask{IsFollow: true, Token0Symbol: "SIREN", Token1Symbol: "USDT"},
			currentTick: -1,
			wantExit:    true,
		},
		{
			name:        "quote token1 above range continues",
			task:        &models.StrategyTask{IsFollow: true, Token0Symbol: "SIREN", Token1Symbol: "USDT"},
			currentTick: 11,
			wantExit:    false,
		},
		{
			name:        "quote token0 raw above range exits as display price below range",
			task:        &models.StrategyTask{IsFollow: true, Token0Symbol: "WBNB", Token1Symbol: "JELLY"},
			currentTick: 11,
			wantExit:    true,
		},
		{
			name:        "quote token0 raw below range continues as display price above range",
			task:        &models.StrategyTask{IsFollow: true, Token0Symbol: "WBNB", Token1Symbol: "JELLY"},
			currentTick: -1,
			wantExit:    false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, _, isDown := pricing.PriceDirectionFromTicks(tt.task, 0, 10, tt.currentTick)
			if got := ShouldExitFollowDownside(tt.task, isDown); got != tt.wantExit {
				t.Fatalf("ShouldExitFollowDownside() = %v, want %v", got, tt.wantExit)
			}
		})
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
