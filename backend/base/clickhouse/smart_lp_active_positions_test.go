package clickhouse

import (
	"math/big"
	"testing"
	"time"
)

func TestApplySmartLPActivePositionEvent_V3Lifecycle(t *testing.T) {
	key := buildSmartLPActivePositionKey("bsc", "v3", "0xpool", "0xwallet", "0xnpm", "123", -100, 100)
	now := time.Unix(200, 0).UTC()

	addEvent := SmartLPActivePositionEvent{
		Ts:              time.Unix(100, 0).UTC(),
		EventSeq:        1,
		Chain:           "bsc",
		PoolVersion:     "v3",
		PoolID:          "0xpool",
		WalletAddress:   "0xwallet",
		ContractAddress: "0xnpm",
		TokenID:         "123",
		Action:          "add",
		LiquidityDelta:  "100",
		TickLower:       -100,
		TickUpper:       100,
	}

	state, changed := applySmartLPActivePositionEvent(smartLPActivePositionState{}, addEvent, key, now)
	if !changed {
		t.Fatal("expected add event to change state")
	}
	if state.CurrentLiquidity == nil || state.CurrentLiquidity.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("expected liquidity 100, got %v", state.CurrentLiquidity)
	}
	if !state.IsActive {
		t.Fatal("expected state to remain active after add")
	}
	if !state.OpenedAt.Equal(addEvent.Ts) || !state.LastAddAt.Equal(addEvent.Ts) {
		t.Fatalf("expected opened/last_add to equal add ts, got opened=%v last_add=%v", state.OpenedAt, state.LastAddAt)
	}

	removeEvent := addEvent
	removeEvent.Ts = time.Unix(120, 0).UTC()
	removeEvent.EventSeq = 2
	removeEvent.Action = "remove"
	removeEvent.LiquidityDelta = "40"

	state, changed = applySmartLPActivePositionEvent(state, removeEvent, key, now)
	if !changed {
		t.Fatal("expected partial remove to change state")
	}
	if state.CurrentLiquidity.Cmp(big.NewInt(60)) != 0 {
		t.Fatalf("expected liquidity 60 after partial remove, got %s", state.CurrentLiquidity.String())
	}
	if !state.IsActive {
		t.Fatal("expected state to stay active after partial remove")
	}
	if !state.OpenedAt.Equal(addEvent.Ts) {
		t.Fatalf("expected opened_at to stay at first add, got %v", state.OpenedAt)
	}
	if !state.LastRemoveAt.Equal(removeEvent.Ts) {
		t.Fatalf("expected last_remove_at to update, got %v", state.LastRemoveAt)
	}

	closeEvent := removeEvent
	closeEvent.Ts = time.Unix(140, 0).UTC()
	closeEvent.EventSeq = 3
	closeEvent.LiquidityDelta = "60"

	state, changed = applySmartLPActivePositionEvent(state, closeEvent, key, now)
	if !changed {
		t.Fatal("expected final remove to change state")
	}
	if state.CurrentLiquidity.Cmp(big.NewInt(0)) != 0 {
		t.Fatalf("expected liquidity 0 after full close, got %s", state.CurrentLiquidity.String())
	}
	if state.IsActive {
		t.Fatal("expected state to be inactive after full close")
	}
	if !state.OpenedAt.Equal(smartLPActiveZeroTime) {
		t.Fatalf("expected opened_at reset on close, got %v", state.OpenedAt)
	}
}

func TestApplySmartLPActivePositionEvent_V4SignedDelta(t *testing.T) {
	key := buildSmartLPActivePositionKey("bsc", "v4", "0xpool", "0xwallet", "0xmanager", "999", -200, 200)
	now := time.Unix(300, 0).UTC()

	addEvent := SmartLPActivePositionEvent{
		Ts:              time.Unix(210, 0).UTC(),
		EventSeq:        10,
		Chain:           "bsc",
		PoolVersion:     "v4",
		PoolID:          "0xpool",
		WalletAddress:   "0xwallet",
		ContractAddress: "0xmanager",
		TokenID:         "999",
		Action:          "add",
		LiquidityDelta:  "50",
		TickLower:       -200,
		TickUpper:       200,
	}

	state, changed := applySmartLPActivePositionEvent(smartLPActivePositionState{}, addEvent, key, now)
	if !changed || state.CurrentLiquidity.Cmp(big.NewInt(50)) != 0 {
		t.Fatalf("expected v4 add to set liquidity 50, changed=%v state=%v", changed, state.CurrentLiquidity)
	}

	removeEvent := addEvent
	removeEvent.Ts = time.Unix(220, 0).UTC()
	removeEvent.EventSeq = 11
	removeEvent.Action = "remove"
	removeEvent.LiquidityDelta = "-20"

	state, changed = applySmartLPActivePositionEvent(state, removeEvent, key, now)
	if !changed {
		t.Fatal("expected v4 remove to change state")
	}
	if state.CurrentLiquidity.Cmp(big.NewInt(30)) != 0 {
		t.Fatalf("expected liquidity 30 after signed remove, got %s", state.CurrentLiquidity.String())
	}
	if !state.IsActive {
		t.Fatal("expected state to remain active after signed remove")
	}
}

func TestApplySmartLPActivePositionEvent_ClampsUnderflowAndSkipsOlderEvents(t *testing.T) {
	key := buildSmartLPActivePositionKey("bsc", "v3", "0xpool", "0xwallet", "0xnpm", "7", -50, 50)
	now := time.Unix(400, 0).UTC()

	removeFirst := SmartLPActivePositionEvent{
		Ts:              time.Unix(310, 0).UTC(),
		EventSeq:        20,
		Chain:           "bsc",
		PoolVersion:     "v3",
		PoolID:          "0xpool",
		WalletAddress:   "0xwallet",
		ContractAddress: "0xnpm",
		TokenID:         "7",
		Action:          "remove",
		LiquidityDelta:  "30",
		TickLower:       -50,
		TickUpper:       50,
	}

	state, changed := applySmartLPActivePositionEvent(smartLPActivePositionState{}, removeFirst, key, now)
	if !changed {
		t.Fatal("expected first remove to be recorded")
	}
	if state.CurrentLiquidity.Cmp(big.NewInt(0)) != 0 || state.IsActive {
		t.Fatalf("expected underflowing remove to clamp to inactive zero, state=%+v", state)
	}

	olderAdd := removeFirst
	olderAdd.EventSeq = 19
	olderAdd.Action = "add"
	olderAdd.LiquidityDelta = "10"

	next, changed := applySmartLPActivePositionEvent(state, olderAdd, key, now)
	if changed {
		t.Fatal("expected older event to be ignored")
	}
	if next.LastEventSeq != state.LastEventSeq {
		t.Fatalf("expected state to remain unchanged on older event, got seq=%d", next.LastEventSeq)
	}
}
