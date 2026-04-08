package trade

import (
	"math/big"
	"testing"
)

func TestEffectiveOpenSpentForPnL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		open   string
		credit string
		want   string
	}{
		{name: "no credit", open: "1000", credit: "0", want: "1000"},
		{name: "subtract credit", open: "1000", credit: "250", want: "750"},
		{name: "cap at zero", open: "1000", credit: "5000", want: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			open, _ := new(big.Int).SetString(tt.open, 10)
			credit, _ := new(big.Int).SetString(tt.credit, 10)
			got := effectiveOpenSpentForPnL(open, credit)
			if got == nil || got.String() != tt.want {
				t.Fatalf("effectiveOpenSpentForPnL(%s, %s) = %v, want %s", tt.open, tt.credit, got, tt.want)
			}
		})
	}
}
