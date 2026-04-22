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
