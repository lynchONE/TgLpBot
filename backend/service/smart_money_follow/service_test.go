package smart_money_follow

import (
	"TgLpBot/base/models"
	"testing"
)

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
