package trade

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
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

func TestRealizedProfitUSDTFromBalanceSnapshots(t *testing.T) {
	t.Parallel()

	got, ok, err := RealizedProfitUSDTFromBalanceSnapshots(&models.TradeRecord{
		OpenStableBefore: "1000",
		CloseStableAfter: "1125",
		TotalGasUSDT:     "5",
		ProfitUSDT:       "-999",
	})
	if err != nil {
		t.Fatalf("RealizedProfitUSDTFromBalanceSnapshots returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected balance snapshot profit to be available")
	}
	if got == nil || got.String() != "120" {
		t.Fatalf("snapshot profit = %v, want 120", got)
	}
}

func TestRealizedProfitUSDTFromBalanceSnapshotsFallsBackWhenBaselineMissing(t *testing.T) {
	t.Parallel()

	got, ok, err := RealizedProfitUSDTFromBalanceSnapshots(&models.TradeRecord{
		OpenStableBefore: "0",
		CloseStableAfter: "1125",
		TotalGasUSDT:     "5",
		ProfitUSDT:       "120",
	})
	if err != nil {
		t.Fatalf("RealizedProfitUSDTFromBalanceSnapshots returned error: %v", err)
	}
	if ok {
		t.Fatalf("snapshot profit available = true, value=%v; want unavailable", got)
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

func TestOpenDustAssetsEncodeParseMerge(t *testing.T) {
	t.Parallel()

	addr := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	assets := MergeOpenDustAssets(
		[]models.TradeRecordDustAsset{
			DustAsset("usdc", addr, big.NewInt(100)),
		},
		[]models.TradeRecordDustAsset{
			{Symbol: "USDC", Address: "0x00000000000000000000000000000000000000AA", Amount: "250"},
			{Symbol: "ZERO", Address: "0x00000000000000000000000000000000000000bb", Amount: "0"},
		},
	)

	if len(assets) != 1 {
		t.Fatalf("len(assets) = %d, want 1: %#v", len(assets), assets)
	}
	if assets[0].Symbol != "USDC" {
		t.Fatalf("symbol = %q, want USDC", assets[0].Symbol)
	}
	if assets[0].Address != addr.Hex() {
		t.Fatalf("address = %q, want %q", assets[0].Address, addr.Hex())
	}
	if assets[0].Amount != "350" {
		t.Fatalf("amount = %q, want 350", assets[0].Amount)
	}

	encoded := EncodeOpenDustAssets(assets)
	parsed := ParseOpenDustAssets(encoded)
	if len(parsed) != 1 || parsed[0].Amount != "350" || parsed[0].Address != addr.Hex() {
		t.Fatalf("ParseOpenDustAssets(%q) = %#v", encoded, parsed)
	}
}
