package smart_money

import (
	"TgLpBot/base/models"
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

func TestGroupLPEventsByPosition(t *testing.T) {
	t.Parallel()

	mkEvent := func(wallet string, nft uint64, tx string, logIndex int) *models.SmartMoneyLPEvent {
		nftCopy := nft
		return &models.SmartMoneyLPEvent{
			WalletAddress: wallet,
			ChainID:       56,
			Protocol:      "uniswap_v3",
			NftTokenID:    &nftCopy,
			TxHash:        tx,
			BlockNumber:   100,
			LogIndex:      logIndex,
		}
	}

	events := []*models.SmartMoneyLPEvent{
		mkEvent("0x1111111111111111111111111111111111111111", 1, "0xaaa", 9),
		mkEvent("0x1111111111111111111111111111111111111111", 1, "0xbbb", 3),
		mkEvent("0x2222222222222222222222222222222222222222", 2, "0xccc", 5),
	}

	grouped := groupLPEventsByPosition(events)
	if len(grouped) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(grouped))
	}
	if len(grouped[0]) != 2 {
		t.Fatalf("expected first group size 2, got %d", len(grouped[0]))
	}
	if grouped[0][0].LogIndex != 3 || grouped[0][1].LogIndex != 9 {
		t.Fatalf("expected same-position group sorted by log index, got [%d, %d]",
			grouped[0][0].LogIndex, grouped[0][1].LogIndex)
	}
	if len(grouped[1]) != 1 || grouped[1][0].NftTokenID == nil || *grouped[1][0].NftTokenID != 2 {
		t.Fatalf("expected second group to contain nft=2 event, got %+v", grouped[1])
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
