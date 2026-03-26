package smart_money_golden_dog

import (
	"TgLpBot/base/models"
	"TgLpBot/base/notify"
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

func TestPoolSignalsForConfigAppliesAllConfiguredThresholds(t *testing.T) {
	now := time.Now()
	// Pool AAA: fees=4200, tvl=250000 → feeRate=1.68%
	//           fees=4200, activeLiqUSD=11053 → activeRate=38.0%
	// Pool BBB: fees=5000, tvl=300000 → feeRate=1.667%
	//           fees=5000, activeLiqUSD=25000 → activeRate=20.0%
	pools := []models.Pool{
		{
			Address:            "0x0000000000000000000000000000000000000aaa",
			Name:               "AAA/USDT",
			TotalFees:          4200,
			TransactionCount:   120,
			CurrentPoolValue:   250000,
			TotalVolume:        1200000,
			ActiveLiquidityUSD: 11053,
			UpdatedAt:          now,
		},
		{
			Address:            "0x0000000000000000000000000000000000000bbb",
			Name:               "BBB/USDT",
			TotalFees:          5000,
			TransactionCount:   80,
			CurrentPoolValue:   300000,
			TotalVolume:        900000,
			ActiveLiquidityUSD: 25000,
			UpdatedAt:          now,
		},
	}

	signals := poolSignalsForConfig(pools, models.SmartMoneyGoldenDogConfig{
		PoolMinTotalFees:            3000,
		PoolMinTransactionCount:     100,
		PoolMinTVL:                  200000,
		PoolMinVolume:               1000000,
		PoolMinFeeRate:              1.5,  // 百分比：>=1.5%
		PoolMinActiveLiquidityRatio: 30.0, // 百分比：>=30%
	})
	if len(signals) != 1 {
		t.Fatalf("expected 1 pool signal, got %d", len(signals))
	}
	if signals[0].Address != "0x0000000000000000000000000000000000000aaa" {
		t.Fatalf("unexpected pool address %q", signals[0].Address)
	}
}

func TestBarkConfigForIntensityMapsPersistentAndCritical(t *testing.T) {
	base := notify.BarkConfig{Key: "abc", Sound: "alarm"}

	persistent := BarkConfigForIntensity(base, BarkIntensityPersistentRing)
	if persistent.Call != "1" {
		t.Fatalf("expected call=1 for persistent ring, got %q", persistent.Call)
	}
	if persistent.Level != "" {
		t.Fatalf("expected empty level for persistent ring, got %q", persistent.Level)
	}

	critical := BarkConfigForIntensity(base, BarkIntensityCriticalRing)
	if critical.Level != "critical" {
		t.Fatalf("expected level=critical, got %q", critical.Level)
	}
	if critical.Call != "" {
		t.Fatalf("expected empty call for critical ring, got %q", critical.Call)
	}
}
