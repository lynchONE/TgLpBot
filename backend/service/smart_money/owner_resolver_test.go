package smart_money

import (
	"context"
	"strings"
	"testing"

	"TgLpBot/base/models"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestOwnerResolveKey(t *testing.T) {
	pm := common.HexToAddress("0xAbCdEf0000000000000000000000000000000001")
	key := ownerResolveKey(pm, 42)
	if key != "0xabcdef0000000000000000000000000000000001|42" {
		t.Fatalf("unexpected key: %s", key)
	}
	// Same position, different casing of the address must collapse to one key.
	if ownerResolveKey(common.HexToAddress(strings.ToUpper(pm.Hex()[2:])), 42) != key {
		t.Fatal("key should be case-insensitive on the position manager address")
	}
}

func TestDecodeOwnerOf(t *testing.T) {
	parsed, err := abi.JSON(strings.NewReader(erc721OwnerOfABI))
	if err != nil {
		t.Fatalf("parse abi: %v", err)
	}
	owner := common.HexToAddress("0x00000000000000000000000000000000000000aB")
	out, err := parsed.Methods["ownerOf"].Outputs.Pack(owner)
	if err != nil {
		t.Fatalf("pack ownerOf output: %v", err)
	}
	got, ok := decodeOwnerOf(parsed, out)
	if !ok || got != owner {
		t.Fatalf("decode mismatch: ok=%v got=%s want=%s", ok, got.Hex(), owner.Hex())
	}

	if _, ok := decodeOwnerOf(parsed, nil); ok {
		t.Fatal("empty data should not decode")
	}
	if _, ok := decodeOwnerOf(parsed, []byte{0x01, 0x02}); ok {
		t.Fatal("malformed data should not decode")
	}
}

func TestPositionManagerForEventV3(t *testing.T) {
	w := &Watcher{chainID: 56}
	event := &models.SmartMoneyLPEvent{Protocol: "pancake_v3"}
	vlog := types.Log{Address: common.HexToAddress("0x46A15B0b27311cedF172AB29E4f4766fbE7F4364")}
	pm, ok := w.positionManagerForEvent(event, vlog)
	if !ok || pm != vlog.Address {
		t.Fatalf("v3 position manager should be the emitting contract: ok=%v pm=%s", ok, pm.Hex())
	}
}

func TestResolveOwnersInPlaceNoopSafe(t *testing.T) {
	// nil client must not panic and must leave events untouched.
	tokenID := uint64(7)
	event := &models.SmartMoneyLPEvent{Protocol: "pancake_v3", NftTokenID: &tokenID}
	resolveOwnersInPlace(context.Background(), nil, []*pendingOwnerResolve{
		{event: event, positionManager: common.HexToAddress("0x1111111111111111111111111111111111111111"), blockNumber: 1},
	})
	if event.WalletAddress != "" {
		t.Fatalf("nil client should not set owner, got %q", event.WalletAddress)
	}
	// empty input is a no-op.
	resolveOwnersInPlace(context.Background(), nil, nil)
}
