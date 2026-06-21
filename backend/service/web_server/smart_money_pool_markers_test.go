package web_server

import (
	"TgLpBot/base/models"
	"testing"
	"time"
)

func TestReplaySmartMoneyMarkerEstimates_MatchesDifferentPositionsIndependently(t *testing.T) {
	t.Parallel()

	nftA := uint64(101)
	nftB := uint64(202)
	baseTime := time.Unix(1_710_000_000, 0).UTC()
	total100 := "100"
	total200 := "200"
	total130 := "130"
	total150 := "150"

	events := []models.SmartMoneyLPEvent{
		{WalletAddress: "0xabc", PoolAddress: "0xpool", EventType: "add", NftTokenID: &nftA, TxHash: "0xadd-a", LogIndex: 1, TxTimestamp: baseTime, TotalUSD: &total100},
		{WalletAddress: "0xabc", PoolAddress: "0xpool", EventType: "add", NftTokenID: &nftB, TxHash: "0xadd-b", LogIndex: 2, TxTimestamp: baseTime.Add(time.Minute), TotalUSD: &total200},
		{WalletAddress: "0xabc", PoolAddress: "0xpool", EventType: "remove", NftTokenID: &nftA, TxHash: "0xrm-a", LogIndex: 3, TxTimestamp: baseTime.Add(2 * time.Minute), TotalUSD: &total130},
		{WalletAddress: "0xabc", PoolAddress: "0xpool", EventType: "remove", NftTokenID: &nftB, TxHash: "0xrm-b", LogIndex: 4, TxTimestamp: baseTime.Add(3 * time.Minute), TotalUSD: &total150},
	}

	targetKeys := map[string]struct{}{
		smartMoneyMarkerPositionKey(&events[2]): {},
		smartMoneyMarkerPositionKey(&events[3]): {},
	}

	estimates, warnings := replaySmartMoneyMarkerEstimates(events, targetKeys)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	first := estimates[smartMoneyMarkerEventID(&events[2])]
	if first.MatchedOpenTxHash != "0xadd-a" {
		t.Fatalf("expected remove A to match 0xadd-a, got %q", first.MatchedOpenTxHash)
	}
	if first.MatchedOpenT != baseTime.Unix() {
		t.Fatalf("expected remove A matched_open_t=%d, got %d", baseTime.Unix(), first.MatchedOpenT)
	}
	if first.EstimatedCostUSD != 100 {
		t.Fatalf("expected remove A cost 100, got %.2f", first.EstimatedCostUSD)
	}
	if first.EstimatedRealizedPnlUSD != 30 {
		t.Fatalf("expected remove A pnl 30, got %.2f", first.EstimatedRealizedPnlUSD)
	}

	second := estimates[smartMoneyMarkerEventID(&events[3])]
	if second.MatchedOpenTxHash != "0xadd-b" {
		t.Fatalf("expected remove B to match 0xadd-b, got %q", second.MatchedOpenTxHash)
	}
	if second.MatchedOpenT != events[1].TxTimestamp.Unix() {
		t.Fatalf("expected remove B matched_open_t=%d, got %d", events[1].TxTimestamp.Unix(), second.MatchedOpenT)
	}
	if second.EstimatedCostUSD != 200 {
		t.Fatalf("expected remove B cost 200, got %.2f", second.EstimatedCostUSD)
	}
	if second.EstimatedRealizedPnlUSD != -50 {
		t.Fatalf("expected remove B pnl -50, got %.2f", second.EstimatedRealizedPnlUSD)
	}
}

func TestReplaySmartMoneyMarkerEstimates_FallsBackToTickRangeWithoutNFT(t *testing.T) {
	t.Parallel()

	baseTime := time.Unix(1_710_100_000, 0).UTC()
	total300 := "300"
	total330 := "330"
	tickLower := -120
	tickUpper := 120

	events := []models.SmartMoneyLPEvent{
		{WalletAddress: "0xabc", PoolAddress: "0xpool", EventType: "add", TickLower: &tickLower, TickUpper: &tickUpper, TxHash: "0xadd", LogIndex: 1, TxTimestamp: baseTime, TotalUSD: &total300},
		{WalletAddress: "0xabc", PoolAddress: "0xpool", EventType: "remove", TickLower: &tickLower, TickUpper: &tickUpper, TxHash: "0xrm", LogIndex: 2, TxTimestamp: baseTime.Add(time.Minute), TotalUSD: &total330},
	}

	targetKeys := map[string]struct{}{
		smartMoneyMarkerPositionKey(&events[1]): {},
	}

	estimates, warnings := replaySmartMoneyMarkerEstimates(events, targetKeys)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	estimate, ok := estimates[smartMoneyMarkerEventID(&events[1])]
	if !ok {
		t.Fatalf("expected range fallback estimate for remove event")
	}
	if estimate.MatchedOpenTxHash != "0xadd" {
		t.Fatalf("expected matched open tx 0xadd, got %q", estimate.MatchedOpenTxHash)
	}
	if estimate.EstimatedCostUSD != 300 {
		t.Fatalf("expected cost 300, got %.2f", estimate.EstimatedCostUSD)
	}
	if estimate.EstimatedRealizedPnlUSD != 30 {
		t.Fatalf("expected pnl 30, got %.2f", estimate.EstimatedRealizedPnlUSD)
	}
}

func TestReplaySmartMoneyMarkerEstimates_PricesPartialRemoveByLiquidityShare(t *testing.T) {
	t.Parallel()

	nftID := uint64(88)
	baseTime := time.Unix(1_710_150_000, 0).UTC()
	total100 := "100"
	total60 := "60"
	total100Out := "100"
	total120Out := "120"

	events := []models.SmartMoneyLPEvent{
		{WalletAddress: "0xabc", PoolAddress: "0xpool", EventType: "add", NftTokenID: &nftID, TxHash: "0xadd-1", LogIndex: 1, TxTimestamp: baseTime, LiquidityDelta: "100", TotalUSD: &total100},
		{WalletAddress: "0xabc", PoolAddress: "0xpool", EventType: "add", NftTokenID: &nftID, TxHash: "0xadd-2", LogIndex: 2, TxTimestamp: baseTime.Add(time.Minute), LiquidityDelta: "50", TotalUSD: &total60},
		{WalletAddress: "0xabc", PoolAddress: "0xpool", EventType: "remove", NftTokenID: &nftID, TxHash: "0xrm", LogIndex: 3, TxTimestamp: baseTime.Add(2 * time.Minute), LiquidityDelta: "-75", TotalUSD: &total100Out},
		{WalletAddress: "0xabc", PoolAddress: "0xpool", EventType: "remove", NftTokenID: &nftID, TxHash: "0xrm-2", LogIndex: 4, TxTimestamp: baseTime.Add(3 * time.Minute), LiquidityDelta: "-75", TotalUSD: &total120Out},
	}

	targetKeys := map[string]struct{}{
		smartMoneyMarkerPositionKey(&events[2]): {},
		smartMoneyMarkerPositionKey(&events[3]): {},
	}

	estimates, warnings := replaySmartMoneyMarkerEstimates(events, targetKeys)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	estimate, ok := estimates[smartMoneyMarkerEventID(&events[2])]
	if !ok {
		t.Fatalf("expected estimate for partial remove")
	}
	if estimate.MatchedOpenTxHash != "0xadd-1" {
		t.Fatalf("expected matched open tx 0xadd-1, got %q", estimate.MatchedOpenTxHash)
	}
	if estimate.EstimatedCostUSD != 80 {
		t.Fatalf("expected proportional cost 80, got %.2f", estimate.EstimatedCostUSD)
	}
	if estimate.EstimatedRealizedPnlUSD != 20 {
		t.Fatalf("expected pnl 20, got %.2f", estimate.EstimatedRealizedPnlUSD)
	}
	if estimate.EstimatedRealizedPnlPct == nil || *estimate.EstimatedRealizedPnlPct != 25 {
		t.Fatalf("expected pnl pct 25, got %v", estimate.EstimatedRealizedPnlPct)
	}

	secondEstimate, ok := estimates[smartMoneyMarkerEventID(&events[3])]
	if !ok {
		t.Fatalf("expected estimate for follow-up remove")
	}
	if secondEstimate.EstimatedCostUSD != 80 {
		t.Fatalf("expected remaining proportional cost 80, got %.2f", secondEstimate.EstimatedCostUSD)
	}
	if secondEstimate.EstimatedRealizedPnlUSD != 40 {
		t.Fatalf("expected second pnl 40, got %.2f", secondEstimate.EstimatedRealizedPnlUSD)
	}
	if secondEstimate.EstimatedRealizedPnlPct == nil || *secondEstimate.EstimatedRealizedPnlPct != 50 {
		t.Fatalf("expected second pnl pct 50, got %v", secondEstimate.EstimatedRealizedPnlPct)
	}
}

func TestReplaySmartMoneyMarkerEstimates_SkipsAmbiguousPositionCycle(t *testing.T) {
	t.Parallel()

	nftID := uint64(7)
	baseTime := time.Unix(1_710_200_000, 0).UTC()
	total100 := "100"
	total40 := "40"
	total160 := "160"

	events := []models.SmartMoneyLPEvent{
		{WalletAddress: "0xabc", PoolAddress: "0xpool", EventType: "add", NftTokenID: &nftID, TxHash: "0xadd-1", LogIndex: 1, TxTimestamp: baseTime, TotalUSD: &total100},
		{WalletAddress: "0xabc", PoolAddress: "0xpool", EventType: "add", NftTokenID: &nftID, TxHash: "0xadd-2", LogIndex: 2, TxTimestamp: baseTime.Add(time.Minute), TotalUSD: &total40},
		{WalletAddress: "0xabc", PoolAddress: "0xpool", EventType: "remove", NftTokenID: &nftID, TxHash: "0xrm", LogIndex: 3, TxTimestamp: baseTime.Add(2 * time.Minute), TotalUSD: &total160},
	}

	targetKeys := map[string]struct{}{
		smartMoneyMarkerPositionKey(&events[2]): {},
	}

	estimates, warnings := replaySmartMoneyMarkerEstimates(events, targetKeys)
	if len(estimates) != 0 {
		t.Fatalf("expected no estimate for ambiguous cycle, got %+v", estimates)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warning for ambiguous cycle")
	}
}
