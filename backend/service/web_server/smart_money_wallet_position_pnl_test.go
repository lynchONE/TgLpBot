package web_server

import (
	"testing"
	"time"
)

func TestApplySmartMoneyWalletPositionPnLEstimatesCompleteReplay(t *testing.T) {
	start := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	positions := []smartMoneyWalletLPPosition{
		{
			PoolVersion:      "v4",
			PoolID:           "0xpool",
			PositionID:       "909",
			ContractAddress:  "0xpoolmanager",
			TickLower:        -120,
			TickUpper:        120,
			Liquidity:        "60",
			Token0:           "0xt0",
			Token1:           "0xt1",
			Token0Dec:        0,
			Token1Dec:        0,
			PositionUSD:      60,
			ClaimableFeesUSD: 10,
		},
	}
	history := []smartMoneyWalletPositionHistoryRow{
		{
			Ts:             start,
			EventSeq:       1,
			PoolVersion:    "v4",
			PoolID:         "0xpool",
			Action:         "add",
			TokenID:        "909",
			Amount0:        "50",
			Amount1:        "50",
			LiquidityDelta: "100",
			TickLower:      -120,
			TickUpper:      120,
			BlockNumber:    100,
			LogIndex:       1,
		},
		{
			Ts:             start.Add(10 * time.Minute),
			EventSeq:       2,
			PoolVersion:    "v4",
			PoolID:         "0xpool",
			Action:         "remove",
			TokenID:        "909",
			Amount0:        "20",
			Amount1:        "20",
			LiquidityDelta: "-40",
			TickLower:      -120,
			TickUpper:      120,
			BlockNumber:    101,
			LogIndex:       1,
		},
	}
	prices := map[string]float64{
		"0xt0": 1,
		"0xt1": 1,
	}

	applySmartMoneyWalletPositionPnLEstimates(positions, history, prices)

	if !positions[0].HasPnL {
		t.Fatalf("expected pnl estimate to be available")
	}
	if positions[0].CurrentValueUSD != 70 {
		t.Fatalf("expected current value 70, got %v", positions[0].CurrentValueUSD)
	}
	if positions[0].CostBasisUSD != 60 {
		t.Fatalf("expected cost basis 60, got %v", positions[0].CostBasisUSD)
	}
	if positions[0].AbsolutePnLUSD != 10 {
		t.Fatalf("expected pnl 10, got %v", positions[0].AbsolutePnLUSD)
	}
	if positions[0].RunningSince == nil || !positions[0].RunningSince.Equal(start) {
		t.Fatalf("expected running since %s, got %#v", start, positions[0].RunningSince)
	}
}

func TestApplySmartMoneyWalletPositionPnLEstimatesRejectsIncompleteReplay(t *testing.T) {
	positions := []smartMoneyWalletLPPosition{
		{
			PoolVersion:      "v4",
			PoolID:           "0xpool",
			PositionID:       "909",
			ContractAddress:  "0xpoolmanager",
			TickLower:        -120,
			TickUpper:        120,
			Liquidity:        "100",
			Token0:           "0xt0",
			Token1:           "0xt1",
			Token0Dec:        0,
			Token1Dec:        0,
			PositionUSD:      80,
			ClaimableFeesUSD: 5,
		},
	}
	history := []smartMoneyWalletPositionHistoryRow{
		{
			Ts:             time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC),
			EventSeq:       1,
			PoolVersion:    "v4",
			PoolID:         "0xpool",
			Action:         "add",
			TokenID:        "909",
			Amount0:        "40",
			Amount1:        "40",
			LiquidityDelta: "60",
			TickLower:      -120,
			TickUpper:      120,
			BlockNumber:    100,
			LogIndex:       1,
		},
	}

	applySmartMoneyWalletPositionPnLEstimates(positions, history, map[string]float64{"0xt0": 1, "0xt1": 1})

	if positions[0].CurrentValueUSD != 85 {
		t.Fatalf("expected current value 85, got %v", positions[0].CurrentValueUSD)
	}
	if positions[0].HasPnL {
		t.Fatalf("expected incomplete replay to suppress pnl")
	}
	if positions[0].RunningSince != nil {
		t.Fatalf("expected incomplete replay to suppress running_since, got %#v", positions[0].RunningSince)
	}
}

func TestApplySmartMoneyWalletPositionPnLEstimatesUsesV4LegacyAlias(t *testing.T) {
	start := time.Date(2026, 3, 17, 8, 0, 0, 0, time.UTC)
	positions := []smartMoneyWalletLPPosition{
		{
			PoolVersion:     "v4",
			PoolID:          "0xpool",
			PositionID:      "909",
			ContractAddress: "0xpoolmanager",
			TickLower:       -300,
			TickUpper:       300,
			Liquidity:       "50",
			Token0:          "0xt0",
			Token1:          "0xt1",
			Token0Dec:       0,
			Token1Dec:       0,
			PositionUSD:     50,
		},
	}
	history := []smartMoneyWalletPositionHistoryRow{
		{
			Ts:             start,
			EventSeq:       1,
			PoolVersion:    "v4",
			PoolID:         "0xpool",
			Action:         "add",
			TokenID:        "",
			Amount0:        "25",
			Amount1:        "25",
			LiquidityDelta: "50",
			TickLower:      -300,
			TickUpper:      300,
			BlockNumber:    100,
			LogIndex:       1,
		},
	}

	applySmartMoneyWalletPositionPnLEstimates(positions, history, map[string]float64{"0xt0": 1, "0xt1": 1})

	if !positions[0].HasPnL {
		t.Fatalf("expected legacy v4 alias to produce pnl")
	}
	if positions[0].RunningSince == nil || !positions[0].RunningSince.Equal(start) {
		t.Fatalf("expected running since %s, got %#v", start, positions[0].RunningSince)
	}
}
