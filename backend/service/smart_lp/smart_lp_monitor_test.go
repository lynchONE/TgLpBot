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

func TestMergeWatchedWalletSet_SkipsBlockedAndDuplicates(t *testing.T) {
	dst := map[string]struct{}{
		"0x00000000000000000000000000000000000000aa": {},
	}
	blocked := map[string]struct{}{
		"0x00000000000000000000000000000000000000bb": {},
	}

	added := mergeWatchedWalletSet(dst, blocked, []string{
		"0x00000000000000000000000000000000000000AA",
		"0x00000000000000000000000000000000000000BB",
		"0x00000000000000000000000000000000000000cc",
		"not-an-address",
	})

	if added != 1 {
		t.Fatalf("expected exactly one wallet to be added, got %d", added)
	}
	if _, ok := dst["0x00000000000000000000000000000000000000cc"]; !ok {
		t.Fatal("expected normalized active wallet to be merged into watch set")
	}
	if _, ok := dst["0x00000000000000000000000000000000000000bb"]; ok {
		t.Fatal("expected blocked wallet to stay excluded")
	}
}

func TestSmartLPPositionLookupNotFound(t *testing.T) {
	if !smartLPPositionLookupNotFound(assertErr("execution reverted: ERC721: invalid token ID")) {
		t.Fatal("expected invalid token id error to be classified as not found")
	}
	if smartLPPositionLookupNotFound(assertErr("context deadline exceeded")) {
		t.Fatal("expected timeout to stay retryable instead of not found")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
