package smart_money_follow

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"testing"
	"time"
)

func ensureTestChainConfig(t *testing.T) {
	t.Helper()
	config.AppConfig = &config.Config{
		Chains: map[string]config.ChainConfig{
			"bsc": {ChainID: 56},
		},
	}
}

func TestCalculateFollowAmountFixed(t *testing.T) {
	cfg := &models.SmartMoneyFollowConfig{
		AmountMode:      models.SmartMoneyFollowAmountModeFixed,
		FixedAmountUSDT: 25,
	}
	amount, err := CalculateFollowAmount(cfg, &models.SmartMoneyLPEvent{})
	if err != nil {
		t.Fatalf("CalculateFollowAmount returned error: %v", err)
	}
	if amount != 25 {
		t.Fatalf("amount = %v, want 25", amount)
	}
}

func TestCalculateFollowAmountRatio(t *testing.T) {
	total := "120.50"
	cfg := &models.SmartMoneyFollowConfig{
		AmountMode: models.SmartMoneyFollowAmountModeRatio,
		Ratio:      0.5,
	}
	amount, err := CalculateFollowAmount(cfg, &models.SmartMoneyLPEvent{TotalUSD: &total})
	if err != nil {
		t.Fatalf("CalculateFollowAmount returned error: %v", err)
	}
	if amount != 60.25 {
		t.Fatalf("amount = %v, want 60.25", amount)
	}
}

func TestCalculateFollowAmountRatioRequiresEventUSD(t *testing.T) {
	cfg := &models.SmartMoneyFollowConfig{
		AmountMode: models.SmartMoneyFollowAmountModeRatio,
		Ratio:      1,
	}
	if _, err := CalculateFollowAmount(cfg, &models.SmartMoneyLPEvent{}); err == nil {
		t.Fatal("expected missing USD amount error")
	}
}

func TestNormalizeSaveInputRejectsInvalidDelay(t *testing.T) {
	ensureTestChainConfig(t)
	_, err := NormalizeSaveInput(SaveConfigInput{
		Chain:               "bsc",
		TargetWalletAddress: "0x0000000000000000000000000000000000000001",
		AmountMode:          models.SmartMoneyFollowAmountModeFixed,
		FixedAmountUSDT:     10,
		DelayMode:           models.SmartMoneyFollowDelayModeFixed,
		DelaySeconds:        maxFollowDelaySeconds + 1,
	})
	if err == nil {
		t.Fatal("expected invalid delay error")
	}
}

func TestNormalizeSaveInputRejectsInvalidExecutionWalletAddress(t *testing.T) {
	ensureTestChainConfig(t)
	_, err := NormalizeSaveInput(SaveConfigInput{
		Chain:               "bsc",
		TargetWalletAddress: "0x0000000000000000000000000000000000000001",
		ExecutionWalletAddr: "not-wallet",
		AmountMode:          models.SmartMoneyFollowAmountModeFixed,
		FixedAmountUSDT:     10,
		DelayMode:           models.SmartMoneyFollowDelayModeImmediate,
	})
	if err == nil {
		t.Fatal("expected invalid execution wallet address error")
	}
}

func TestNormalizeSaveInputAcceptsWalletGroup(t *testing.T) {
	ensureTestChainConfig(t)
	got, err := NormalizeSaveInput(SaveConfigInput{
		Chain: "bsc",
		TargetWallets: []string{
			"0x0000000000000000000000000000000000000001",
			"0x0000000000000000000000000000000000000002",
			"0x0000000000000000000000000000000000000001",
		},
		AmountMode:           models.SmartMoneyFollowAmountModeFixed,
		FixedAmountUSDT:      10,
		DelayMode:            models.SmartMoneyFollowDelayModeImmediate,
		TriggerMode:          models.SmartMoneyFollowTriggerModeThreshold,
		TriggerMinWallets:    2,
		TriggerWindowSeconds: 600,
	})
	if err != nil {
		t.Fatalf("NormalizeSaveInput returned error: %v", err)
	}
	if len(got.TargetWallets) != 2 {
		t.Fatalf("wallet count = %d, want 2", len(got.TargetWallets))
	}
	if got.TargetWalletAddress != "0x0000000000000000000000000000000000000001" {
		t.Fatalf("primary wallet = %s", got.TargetWalletAddress)
	}
	if got.TriggerMode != models.SmartMoneyFollowTriggerModeThreshold {
		t.Fatalf("trigger mode = %s", got.TriggerMode)
	}
}

func TestNormalizeSaveInputNormalizesExecutionWalletAddress(t *testing.T) {
	ensureTestChainConfig(t)
	got, err := NormalizeSaveInput(SaveConfigInput{
		Chain:               "bsc",
		TargetWalletAddress: "0x0000000000000000000000000000000000000001",
		ExecutionWalletAddr: "0x00000000000000000000000000000000000000AA",
		AmountMode:          models.SmartMoneyFollowAmountModeFixed,
		FixedAmountUSDT:     10,
		DelayMode:           models.SmartMoneyFollowDelayModeImmediate,
	})
	if err != nil {
		t.Fatalf("NormalizeSaveInput returned error: %v", err)
	}
	if got.ExecutionWalletAddr != "0x00000000000000000000000000000000000000aa" {
		t.Fatalf("execution wallet address = %s", got.ExecutionWalletAddr)
	}
}

func TestNormalizeSaveInputRejectsThresholdAboveWalletCount(t *testing.T) {
	ensureTestChainConfig(t)
	_, err := NormalizeSaveInput(SaveConfigInput{
		Chain:                "bsc",
		TargetWallets:        []string{"0x0000000000000000000000000000000000000001"},
		AmountMode:           models.SmartMoneyFollowAmountModeFixed,
		FixedAmountUSDT:      10,
		DelayMode:            models.SmartMoneyFollowDelayModeImmediate,
		TriggerMode:          models.SmartMoneyFollowTriggerModeThreshold,
		TriggerMinWallets:    2,
		TriggerWindowSeconds: 60,
	})
	if err == nil {
		t.Fatal("expected threshold wallet count error")
	}
}

func TestTargetPositionRefForThresholdIgnoresWallet(t *testing.T) {
	lower := -100
	upper := 100
	cfg := &models.SmartMoneyFollowConfig{TriggerMode: models.SmartMoneyFollowTriggerModeThreshold}
	eventA := &models.SmartMoneyLPEvent{
		WalletAddress: "0x0000000000000000000000000000000000000001",
		ChainID:       56,
		Protocol:      "pancake_v3",
		PoolAddress:   "0x00000000000000000000000000000000000000aa",
		TickLower:     &lower,
		TickUpper:     &upper,
		TxTimestamp:   time.Now(),
	}
	eventB := *eventA
	eventB.WalletAddress = "0x0000000000000000000000000000000000000002"
	refA := targetPositionRefForFollowJob(cfg, eventA)
	refB := targetPositionRefForFollowJob(cfg, &eventB)
	if refA == "" {
		t.Fatal("expected threshold position ref")
	}
	if refA != refB {
		t.Fatalf("threshold refs differ: %s vs %s", refA, refB)
	}
}

func TestFollowJobEventIDsIncludesTriggerEvents(t *testing.T) {
	got := followJobAndAttemptEventIDs([]models.SmartMoneyFollowJob{
		{EventID: 10, TriggerEventIDs: models.StringArray{"9", "10", "bad", "0"}},
		{EventID: 11, TriggerEventIDs: models.StringArray{"8"}},
	}, []models.SmartMoneyFollowAttempt{
		{EventID: 12},
	})
	want := []uint{12, 11, 10, 9, 8}
	if len(got) != len(want) {
		t.Fatalf("event id count = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event ids = %v, want %v", got, want)
		}
	}
}
