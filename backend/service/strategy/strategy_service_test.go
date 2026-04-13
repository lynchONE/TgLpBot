package strategy

import (
	"TgLpBot/base/models"
	"testing"
)

func TestShouldStopOutOfRangeImmediately(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		task              *models.StrategyTask
		isUp              bool
		isDown            bool
		wantImmediateStop bool
	}{
		{
			name:              "nil task",
			task:              nil,
			isUp:              true,
			wantImmediateStop: false,
		},
		{
			name: "rebalance enabled above range",
			task: &models.StrategyTask{
				RebalanceEnabled: true,
			},
			isUp:              true,
			wantImmediateStop: false,
		},
		{
			name: "rebalance disabled above range",
			task: &models.StrategyTask{
				RebalanceEnabled: false,
			},
			isUp:              true,
			wantImmediateStop: true,
		},
		{
			name: "rebalance disabled below range without stoploss",
			task: &models.StrategyTask{
				RebalanceEnabled: false,
				StopLossEnabled:  false,
			},
			isDown:            true,
			wantImmediateStop: true,
		},
		{
			name: "rebalance disabled below range with stoploss",
			task: &models.StrategyTask{
				RebalanceEnabled: false,
				StopLossEnabled:  true,
			},
			isDown:            true,
			wantImmediateStop: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ShouldStopOutOfRangeImmediately(tc.task, tc.isUp, tc.isDown)
			if got != tc.wantImmediateStop {
				t.Fatalf("ShouldStopOutOfRangeImmediately(...) = %v, want %v", got, tc.wantImmediateStop)
			}
		})
	}
}
