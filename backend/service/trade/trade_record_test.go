package trade

import (
	"TgLpBot/base/config"
	"math/big"
	"testing"
)

func TestBalanceAfterSpend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		before string
		spent  string
		want   string
	}{
		{name: "basic", before: "1000", spent: "250", want: "750"},
		{name: "floor at zero", before: "1000", spent: "5000", want: "0"},
		{name: "nil-like zero", before: "0", spent: "0", want: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			before, _ := new(big.Int).SetString(tt.before, 10)
			spent, _ := new(big.Int).SetString(tt.spent, 10)
			got := balanceAfterSpend(before, spent)
			if got == nil || got.String() != tt.want {
				t.Fatalf("balanceAfterSpend(%s, %s) = %v, want %s", tt.before, tt.spent, got, tt.want)
			}
		})
	}
}

func TestTradeProfitUSDT(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		closeRecv    string
		openSpent    string
		totalGasUSDT string
		want         string
	}{
		{name: "profit", closeRecv: "1200", openSpent: "1000", totalGasUSDT: "10", want: "190"},
		{name: "loss", closeRecv: "979", openSpent: "1000", totalGasUSDT: "0", want: "-21"},
		{name: "loss with gas", closeRecv: "979", openSpent: "1000", totalGasUSDT: "2", want: "-23"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			closeRecv, _ := new(big.Int).SetString(tt.closeRecv, 10)
			openSpent, _ := new(big.Int).SetString(tt.openSpent, 10)
			totalGasUSDT, _ := new(big.Int).SetString(tt.totalGasUSDT, 10)
			got := tradeProfitUSDT(closeRecv, openSpent, totalGasUSDT)
			if got == nil || got.String() != tt.want {
				t.Fatalf("tradeProfitUSDT(%s, %s, %s) = %v, want %s", tt.closeRecv, tt.openSpent, tt.totalGasUSDT, got, tt.want)
			}
		})
	}
}

func TestStableAmountTo18(t *testing.T) {
	previous := config.AppConfig
	config.AppConfig = &config.Config{
		Chains: map[string]config.ChainConfig{
			"base": {Chain: "base", StableDecimals: 6},
		},
	}
	t.Cleanup(func() {
		config.AppConfig = previous
	})

	got := stableAmountTo18("base", big.NewInt(123456789))
	want := "123456789000000000000"
	if got == nil || got.String() != want {
		t.Fatalf("stableAmountTo18(base, 123456789) = %v, want %s", got, want)
	}

	got = stableAmountTo18("bsc", big.NewInt(1234567890000000000))
	want = "1234567890000000000"
	if got == nil || got.String() != want {
		t.Fatalf("stableAmountTo18(bsc, 1234567890000000000) = %v, want %s", got, want)
	}
}
