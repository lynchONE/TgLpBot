package liquidity

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestOKXTokenAddressParam_UsesLowercasePseudoForNative(t *testing.T) {
	got := okxTokenAddressParam(common.HexToAddress("0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"))
	want := "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	if got != want {
		t.Fatalf("okxTokenAddressParam() = %q, want %q", got, want)
	}
}

func TestNativeBalanceDelta_AddsBackGasCost(t *testing.T) {
	before := big.NewInt(1000)
	after := big.NewInt(950)
	gasCost := big.NewInt(100)

	got := nativeBalanceDelta(before, after, gasCost)
	if got.Cmp(big.NewInt(50)) != 0 {
		t.Fatalf("nativeBalanceDelta() = %s, want 50", got.String())
	}
}

func TestNativeBalanceDelta_ClampsNegativeResult(t *testing.T) {
	before := big.NewInt(1000)
	after := big.NewInt(900)
	gasCost := big.NewInt(0)

	got := nativeBalanceDelta(before, after, gasCost)
	if got.Sign() != 0 {
		t.Fatalf("nativeBalanceDelta() = %s, want 0", got.String())
	}
}
