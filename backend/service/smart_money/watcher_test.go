package smart_money

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestDetectProtocolUsesConfiguredManagers(t *testing.T) {
	t.Cleanup(func() {
		config.AppConfig = nil
	})

	pancake := common.HexToAddress("0x1111111111111111111111111111111111111111")
	uniswap := common.HexToAddress("0x2222222222222222222222222222222222222222")
	config.AppConfig = &config.Config{
		EnabledChains: []string{"bsc"},
		Chains: map[string]config.ChainConfig{
			"bsc": {
				Chain: "bsc",
				V3Deployments: []config.V3DeploymentConfig{
					{Name: "PancakeSwap V3", PositionManagerAddress: pancake.Hex()},
					{Name: "Uniswap V3", PositionManagerAddress: uniswap.Hex()},
				},
			},
		},
	}

	w := &Watcher{chainID: 56}

	protocol, err := w.detectProtocol(pancake)
	if err != nil {
		t.Fatalf("detect pancake protocol: %v", err)
	}
	if protocol != "pancake_v3" {
		t.Fatalf("expected pancake_v3, got %q", protocol)
	}

	protocol, err = w.detectProtocol(uniswap)
	if err != nil {
		t.Fatalf("detect uniswap protocol: %v", err)
	}
	if protocol != "uniswap_v3" {
		t.Fatalf("expected uniswap_v3, got %q", protocol)
	}

	if _, err := w.detectProtocol(common.HexToAddress("0x3333333333333333333333333333333333333333")); err == nil {
		t.Fatal("expected unknown position manager to return error")
	}
}

func TestParseModifyLiquidityParsesSaltAndDefaultsAmounts(t *testing.T) {
	t.Parallel()

	w := &Watcher{}
	tokenID := uint64(42)
	poolID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	vlog := types.Log{
		Address: common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Topics: []common.Hash{
			TopicModifyLiquidity,
			poolID,
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
	if event.NftTokenID != nil {
		t.Fatalf("expected nil nft token id for non-position-manager sender, got %v", event.NftTokenID)
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
	if want := strings.ToLower(poolID.Hex()); event.PoolAddress != want {
		t.Fatalf("expected pool id %q, got %q", want, event.PoolAddress)
	}
}

func TestParseModifyLiquidityDetectsRemove(t *testing.T) {
	t.Parallel()

	w := &Watcher{}
	tokenID := uint64(7)
	poolID := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	vlog := types.Log{
		Address: common.HexToAddress("0x3333333333333333333333333333333333333333"),
		Topics: []common.Hash{
			TopicModifyLiquidity,
			poolID,
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
	if event.NftTokenID != nil {
		t.Fatalf("expected nil nft token id for non-position-manager sender, got %v", event.NftTokenID)
	}
	if event.PoolAddress != strings.ToLower(poolID.Hex()) {
		t.Fatalf("expected pool id %q, got %q", strings.ToLower(poolID.Hex()), event.PoolAddress)
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

func TestBuildNativeTransferEventsByBlock_SkipsExcludedLPTx(t *testing.T) {
	t.Parallel()

	blockTime := time.Date(2026, time.March, 29, 12, 0, 0, 0, time.UTC)
	blocks := []*blockSnapshot{
		{
			Number:    100,
			Timestamp: blockTime,
			Transactions: []blockTransaction{
				{
					Hash:  common.HexToHash("0x1"),
					From:  "0x1111111111111111111111111111111111111111",
					To:    "0x2222222222222222222222222222222222222222",
					Value: "1000000000000000000",
					Input: "0x", // pure value transfer
				},
				{
					Hash:  common.HexToHash("0x2"),
					From:  "0x1111111111111111111111111111111111111111",
					To:    "0x3333333333333333333333333333333333333333",
					Value: "2000000000000000000",
					Input: "0x", // pure value transfer
				},
			},
		},
	}
	activeWallets := map[string]struct{}{
		"0x1111111111111111111111111111111111111111": {},
		"0x2222222222222222222222222222222222222222": {},
	}
	excluded := map[uint64]map[string]struct{}{
		100: {
			"0x0000000000000000000000000000000000000000000000000000000000000002": {},
		},
	}

	eventsByBlock := buildNativeTransferEventsByBlock(blocks, 56, activeWallets, excluded)
	events := eventsByBlock[100]
	if got, want := len(events), 2; got != want {
		t.Fatalf("native transfer events = %d, want %d", got, want)
	}
	if got, want := events[0].Direction, models.SmartMoneyTransferDirectionOut; got != want {
		t.Fatalf("first direction = %s, want %s", got, want)
	}
	if got, want := events[0].WalletAddress, "0x1111111111111111111111111111111111111111"; got != want {
		t.Fatalf("first wallet = %s, want %s", got, want)
	}
	if got, want := events[1].Direction, models.SmartMoneyTransferDirectionIn; got != want {
		t.Fatalf("second direction = %s, want %s", got, want)
	}
	if got, want := events[1].WalletAddress, "0x2222222222222222222222222222222222222222"; got != want {
		t.Fatalf("second wallet = %s, want %s", got, want)
	}
	for _, event := range events {
		if event.TxHash != "0x0000000000000000000000000000000000000000000000000000000000000001" {
			t.Fatalf("unexpected tx hash in persisted native events: %s", event.TxHash)
		}
	}
}

func TestBuildERC20TransferEventsFromLogs_BuildsDirectionalEvents(t *testing.T) {
	t.Parallel()

	blockTime := time.Date(2026, time.March, 29, 12, 30, 0, 0, time.UTC)
	activeWallets := map[string]struct{}{
		"0x1111111111111111111111111111111111111111": {},
		"0x2222222222222222222222222222222222222222": {},
	}
	blockTimeByNumber := map[uint64]time.Time{88: blockTime}
	logs := []types.Log{
		{
			Address: common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			Topics: []common.Hash{
				TopicTransfer,
				common.BytesToHash(common.LeftPadBytes(common.HexToAddress("0x1111111111111111111111111111111111111111").Bytes(), 32)),
				common.BytesToHash(common.LeftPadBytes(common.HexToAddress("0x2222222222222222222222222222222222222222").Bytes(), 32)),
			},
			Data:        encodeUnsignedWord(big.NewInt(123)),
			TxHash:      common.HexToHash("0x3"),
			BlockNumber: 88,
			Index:       7,
		},
		{
			Address: common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			Topics: []common.Hash{
				TopicTransfer,
				common.BytesToHash(common.LeftPadBytes(common.HexToAddress("0x1111111111111111111111111111111111111111").Bytes(), 32)),
				common.BytesToHash(common.LeftPadBytes(common.HexToAddress("0x3333333333333333333333333333333333333333").Bytes(), 32)),
			},
			Data:        encodeUnsignedWord(big.NewInt(999)),
			TxHash:      common.HexToHash("0x4"),
			BlockNumber: 88,
			Index:       8,
		},
	}
	excluded := map[uint64]map[string]struct{}{
		88: {
			"0x0000000000000000000000000000000000000000000000000000000000000004": {},
		},
	}

	outEvents := buildERC20TransferEventsFromLogs(logs, 56, models.SmartMoneyTransferDirectionOut, activeWallets, blockTimeByNumber, excluded, nil, nil)
	if got, want := len(outEvents), 1; got != want {
		t.Fatalf("out events = %d, want %d", got, want)
	}
	if got, want := outEvents[0].WalletAddress, "0x1111111111111111111111111111111111111111"; got != want {
		t.Fatalf("out wallet = %s, want %s", got, want)
	}
	if got, want := outEvents[0].AmountRaw, "123"; got != want {
		t.Fatalf("out amount raw = %s, want %s", got, want)
	}

	inEvents := buildERC20TransferEventsFromLogs(logs, 56, models.SmartMoneyTransferDirectionIn, activeWallets, blockTimeByNumber, excluded, nil, nil)
	if got, want := len(inEvents), 1; got != want {
		t.Fatalf("in events = %d, want %d", got, want)
	}
	if got, want := inEvents[0].WalletAddress, "0x2222222222222222222222222222222222222222"; got != want {
		t.Fatalf("in wallet = %s, want %s", got, want)
	}
	if got, want := inEvents[0].TxTimestamp, blockTime; !got.Equal(want) {
		t.Fatalf("in timestamp = %v, want %v", got, want)
	}
}

func TestExpandUserWalletTransferEvents_ExpandsPerTrackedWallet(t *testing.T) {
	t.Parallel()

	events := []*models.SmartMoneyWalletTransferEvent{
		{
			WalletAddress: "0x1111111111111111111111111111111111111111",
			ChainID:       56,
			Direction:     models.SmartMoneyTransferDirectionIn,
			AssetType:     models.SmartMoneyTransferAssetERC20,
			TokenAddress:  "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			AmountRaw:     "123",
			AmountDecimal: 123,
			AmountUSD:     123,
			TxHash:        "0xabc",
			BlockNumber:   88,
			LogIndex:      7,
			TxTimestamp:   time.Date(2026, time.March, 29, 13, 0, 0, 0, time.UTC),
		},
	}
	walletRefsByAddress := map[string][]UserWalletRef{
		"0x1111111111111111111111111111111111111111": {
			{UserID: 1, WalletID: 11, WalletAddress: "0x1111111111111111111111111111111111111111"},
			{UserID: 2, WalletID: 22, WalletAddress: "0x1111111111111111111111111111111111111111"},
		},
	}

	out := expandUserWalletTransferEvents(events, walletRefsByAddress, "bsc")
	if got, want := len(out), 2; got != want {
		t.Fatalf("user wallet transfer events = %d, want %d", got, want)
	}
	if got, want := out[0].WalletID, uint(11); got != want {
		t.Fatalf("first wallet id = %d, want %d", got, want)
	}
	if got, want := out[1].UserID, uint(2); got != want {
		t.Fatalf("second user id = %d, want %d", got, want)
	}
	if got, want := out[0].Chain, "bsc"; got != want {
		t.Fatalf("chain = %s, want %s", got, want)
	}
	if got, want := out[0].TxHash, "0xabc"; got != want {
		t.Fatalf("tx hash = %s, want %s", got, want)
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
