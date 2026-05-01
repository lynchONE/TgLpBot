package liquidity

import (
	"TgLpBot/base/config"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestAppendStableBudgetDustAssetRecordsUnspentStableBudget(t *testing.T) {
	t.Parallel()

	stable := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	cc := config.ChainConfig{StableSymbol: "USDT"}
	requested := big.NewInt(500_000_000_000_000_000)
	spent := big.NewInt(463_000_000_000_000_000)

	got := appendStableBudgetDustAsset(nil, cc, stable, requested, spent)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Symbol != "USDT" {
		t.Fatalf("symbol = %q, want USDT", got[0].Symbol)
	}
	if got[0].Address != stable.Hex() {
		t.Fatalf("address = %q, want %q", got[0].Address, stable.Hex())
	}
	if got[0].Amount != "37000000000000000" {
		t.Fatalf("amount = %q, want 37000000000000000", got[0].Amount)
	}
}

func TestAppendStableBudgetDustAssetSubtractsAlreadyRecordedStableDust(t *testing.T) {
	t.Parallel()

	stable := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	cc := config.ChainConfig{StableSymbol: "USDT"}
	requested := big.NewInt(500_000_000_000_000_000)
	spent := big.NewInt(463_000_000_000_000_000)
	recorded := big.NewInt(7_000_000_000_000_000)

	got := appendStableBudgetDustAsset(nil, cc, stable, requested, spent, recorded)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Amount != "30000000000000000" {
		t.Fatalf("amount = %q, want 30000000000000000", got[0].Amount)
	}
}

func TestAppendStableBudgetDustAssetSkipsWhenFullySpent(t *testing.T) {
	t.Parallel()

	stable := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	cc := config.ChainConfig{StableSymbol: "USDT"}
	requested := big.NewInt(500_000_000_000_000_000)

	got := appendStableBudgetDustAsset(nil, cc, stable, requested, requested)
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}
