package strategy

import (
	"TgLpBot/base/models"
	"testing"
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
