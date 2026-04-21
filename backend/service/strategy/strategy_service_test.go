package strategy

import (
	"TgLpBot/base/models"
	"testing"
)

func TestShouldMonitorOutOfRangeOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		task            *models.StrategyTask
		isUp            bool
		isDown          bool
		wantMonitorOnly bool
	}{
		{
			name:            "nil task",
			task:            nil,
			isUp:            true,
			wantMonitorOnly: false,
		},
		{
			name: "rebalance enabled above range",
			task: &models.StrategyTask{
				RebalanceEnabled: true,
			},
			isUp:            true,
			wantMonitorOnly: false,
		},
		{
			name: "rebalance disabled above range",
			task: &models.StrategyTask{
				RebalanceEnabled: false,
			},
			isUp:            true,
			wantMonitorOnly: true,
		},
		{
			name: "rebalance disabled below range without stoploss",
			task: &models.StrategyTask{
				RebalanceEnabled: false,
				StopLossEnabled:  false,
			},
			isDown:          true,
			wantMonitorOnly: true,
		},
		{
			name: "rebalance disabled below range with stoploss",
			task: &models.StrategyTask{
				RebalanceEnabled: false,
				StopLossEnabled:  true,
			},
			isDown:          true,
			wantMonitorOnly: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ShouldMonitorOutOfRangeOnly(tc.task, tc.isUp, tc.isDown)
			if got != tc.wantMonitorOnly {
				t.Fatalf("ShouldMonitorOutOfRangeOnly(...) = %v, want %v", got, tc.wantMonitorOnly)
			}
		})
	}
}
