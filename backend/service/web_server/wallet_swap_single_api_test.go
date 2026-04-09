package web_server

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestOKXWalletSwapTokenAddress_UsesLowercasePseudoForNative(t *testing.T) {
	got := okxWalletSwapTokenAddress(nativePseudoTokenAddress, common.HexToAddress(nativePseudoTokenAddress))
	if got != nativePseudoTokenAddress {
		t.Fatalf("okxWalletSwapTokenAddress() = %q, want %q", got, nativePseudoTokenAddress)
	}
}

func TestTokenDecimals_Returns18ForNativePseudo(t *testing.T) {
	got := tokenDecimals(nil, common.HexToAddress(nativePseudoTokenAddress))
	if got != 18 {
		t.Fatalf("tokenDecimals() = %d, want 18", got)
	}
}
