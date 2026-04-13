package user

import "testing"

func TestRebalanceTimeoutUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		updates map[string]interface{}
		want    int
		wantOK  bool
	}{
		{
			name:    "missing key",
			updates: map[string]interface{}{"slippage_tolerance": 0.5},
			want:    0,
			wantOK:  false,
		},
		{
			name:    "int value",
			updates: map[string]interface{}{"rebalance_timeout": 0},
			want:    0,
			wantOK:  true,
		},
		{
			name:    "float value",
			updates: map[string]interface{}{"rebalance_timeout": float64(15)},
			want:    15,
			wantOK:  true,
		},
		{
			name:    "unsupported type",
			updates: map[string]interface{}{"rebalance_timeout": "20"},
			want:    0,
			wantOK:  false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := rebalanceTimeoutUpdate(tc.updates)
			if ok != tc.wantOK {
				t.Fatalf("rebalanceTimeoutUpdate(...) ok = %v, want %v", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Fatalf("rebalanceTimeoutUpdate(...) = %d, want %d", got, tc.want)
			}
		})
	}
}
