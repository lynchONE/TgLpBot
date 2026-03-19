package smart_money_golden_dog

import (
	"TgLpBot/base/models"
	"testing"
	"time"
)

func TestPairSignalsForConfigCountsDistinctWalletsByPair(t *testing.T) {
	now := time.Now()
	events := []models.SmartMoneyLPEvent{
		{
			WalletAddress: "0x00000000000000000000000000000000000000a1",
			Token0Address: "0x0000000000000000000000000000000000000011",
			Token1Address: "0x0000000000000000000000000000000000000022",
			Token0Symbol:  "AAA",
			Token1Symbol:  "BBB",
			TxTimestamp:   now.Add(-2 * time.Minute),
		},
		{
			WalletAddress: "0x00000000000000000000000000000000000000b2",
			Token0Address: "0x0000000000000000000000000000000000000011",
			Token1Address: "0x0000000000000000000000000000000000000022",
			Token0Symbol:  "AAA",
			Token1Symbol:  "BBB",
			TxTimestamp:   now.Add(-3 * time.Minute),
		},
		{
			WalletAddress: "0x00000000000000000000000000000000000000a1",
			Token0Address: "0x0000000000000000000000000000000000000022",
			Token1Address: "0x0000000000000000000000000000000000000011",
			Token0Symbol:  "BBB",
			Token1Symbol:  "AAA",
			TxTimestamp:   now.Add(-1 * time.Minute),
		},
		{
			WalletAddress: "0x00000000000000000000000000000000000000c3",
			Token0Address: "0x0000000000000000000000000000000000000011",
			Token1Address: "0x0000000000000000000000000000000000000022",
			Token0Symbol:  "AAA",
			Token1Symbol:  "BBB",
			TxTimestamp:   now.Add(-4 * time.Minute),
		},
	}

	signals := pairSignalsForConfig(buildPairBuckets(events), now, models.SmartMoneyGoldenDogConfig{
		MinWallets:    3,
		WindowMinutes: 10,
	})
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].WalletCount != 3 {
		t.Fatalf("expected wallet count 3, got %d", signals[0].WalletCount)
	}
	if signals[0].Label != "AAA/BBB" {
		t.Fatalf("expected canonical label AAA/BBB, got %q", signals[0].Label)
	}
}

func TestCooldownActive(t *testing.T) {
	now := time.Now()
	state := &models.SmartMoneyGoldenDogAlertState{
		LastNotifiedAt: now.Add(-5 * time.Minute),
	}

	if !cooldownActive(state, now, 10) {
		t.Fatal("expected cooldown to be active")
	}
	if cooldownActive(state, now, 3) {
		t.Fatal("expected cooldown to be inactive")
	}
}
