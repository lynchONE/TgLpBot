package smart_lp

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestSmartLPTokenIDFromV4Salt(t *testing.T) {
	hash := common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000004d2")
	if got := smartLPTokenIDFromV4Salt(hash); got != "1234" {
		t.Fatalf("expected 1234, got %s", got)
	}

	raw := [32]byte{}
	raw[31] = 5
	if got := smartLPTokenIDFromV4Salt(raw); got != "5" {
		t.Fatalf("expected 5, got %s", got)
	}

	if got := smartLPTokenIDFromV4Salt(big.NewInt(99)); got != "99" {
		t.Fatalf("expected 99, got %s", got)
	}

	if got := smartLPTokenIDFromV4Salt(common.Hash{}); got != "" {
		t.Fatalf("expected empty token id for zero salt, got %s", got)
	}
}
