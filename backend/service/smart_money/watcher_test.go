package smart_money

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestParseModifyLiquidityParsesSaltAndDefaultsAmounts(t *testing.T) {
	t.Parallel()

	w := &Watcher{}
	tokenID := uint64(42)
	vlog := types.Log{
		Address: common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Topics: []common.Hash{
			TopicModifyLiquidity,
			common.HexToHash("0x1234"),
			common.BytesToHash(common.LeftPadBytes(common.HexToAddress("0x2222222222222222222222222222222222222222").Bytes(), 32)),
		},
		Data: joinWords(
			encodeSignedWord(-120),
			encodeSignedWord(120),
			encodeSignedWord(5000),
			encodeUnsignedWord(new(big.Int).SetUint64(tokenID)),
		),
	}

	event, err := w.parseModifyLiquidity(vlog)
	if err != nil {
		t.Fatalf("parseModifyLiquidity() error = %v", err)
	}
	if event == nil {
		t.Fatal("parseModifyLiquidity() returned nil event")
	}
	if event.Protocol != "uniswap_v4" {
		t.Fatalf("expected protocol uniswap_v4, got %q", event.Protocol)
	}
	if event.EventType != "add" {
		t.Fatalf("expected add event, got %q", event.EventType)
	}
	if event.NftTokenID == nil || *event.NftTokenID != tokenID {
		t.Fatalf("expected nft token id %d, got %v", tokenID, event.NftTokenID)
	}
	if event.TickLower == nil || *event.TickLower != -120 {
		t.Fatalf("expected tick lower -120, got %v", event.TickLower)
	}
	if event.TickUpper == nil || *event.TickUpper != 120 {
		t.Fatalf("expected tick upper 120, got %v", event.TickUpper)
	}
	if event.Token0Amount != "0" || event.Token1Amount != "0" {
		t.Fatalf("expected zero default amounts, got token0=%q token1=%q", event.Token0Amount, event.Token1Amount)
	}
	if want := strings.ToLower(vlog.Address.Hex()); event.PoolAddress != want {
		t.Fatalf("expected pool address %q, got %q", want, event.PoolAddress)
	}
}

func TestParseModifyLiquidityDetectsRemove(t *testing.T) {
	t.Parallel()

	w := &Watcher{}
	tokenID := uint64(7)
	vlog := types.Log{
		Address: common.HexToAddress("0x3333333333333333333333333333333333333333"),
		Topics: []common.Hash{
			TopicModifyLiquidity,
			common.HexToHash("0x4567"),
			common.BytesToHash(common.LeftPadBytes(common.HexToAddress("0x4444444444444444444444444444444444444444").Bytes(), 32)),
		},
		Data: joinWords(
			encodeSignedWord(-240),
			encodeSignedWord(240),
			encodeSignedWord(-9000),
			encodeUnsignedWord(new(big.Int).SetUint64(tokenID)),
		),
	}

	event, err := w.parseModifyLiquidity(vlog)
	if err != nil {
		t.Fatalf("parseModifyLiquidity() error = %v", err)
	}
	if event.EventType != "remove" {
		t.Fatalf("expected remove event, got %q", event.EventType)
	}
	if event.NftTokenID == nil || *event.NftTokenID != tokenID {
		t.Fatalf("expected nft token id %d, got %v", tokenID, event.NftTokenID)
	}
}

func joinWords(words ...[]byte) []byte {
	total := 0
	for _, word := range words {
		total += len(word)
	}
	out := make([]byte, 0, total)
	for _, word := range words {
		out = append(out, word...)
	}
	return out
}

func encodeUnsignedWord(v *big.Int) []byte {
	if v == nil {
		return make([]byte, 32)
	}
	return v.FillBytes(make([]byte, 32))
}

func encodeSignedWord(v int64) []byte {
	n := big.NewInt(v)
	if n.Sign() < 0 {
		n = new(big.Int).Add(n, new(big.Int).Lsh(big.NewInt(1), 256))
	}
	return n.FillBytes(make([]byte, 32))
}
