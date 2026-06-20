package smart_money_follow

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	poolsvc "TgLpBot/service/pool"
	"errors"
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

func TestNormalizeSaveInputRejectsInvalidRangeShiftGrids(t *testing.T) {
	ensureTestChainConfig(t)
	_, err := NormalizeSaveInput(SaveConfigInput{
		Chain:               "bsc",
		TargetWalletAddress: "0x0000000000000000000000000000000000000001",
		AmountMode:          models.SmartMoneyFollowAmountModeFixed,
		FixedAmountUSDT:     10,
		DelayMode:           models.SmartMoneyFollowDelayModeImmediate,
		RangeShiftGrids:     maxFollowRangeShiftGrids + 1,
	})
	if err == nil {
		t.Fatal("expected invalid range shift grids error")
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

func TestShiftFollowRangeByGrids(t *testing.T) {
	lower, upper, shifted, err := shiftFollowRangeByGrids(100, 500, 100, 1)
	if err != nil {
		t.Fatalf("shiftFollowRangeByGrids returned error: %v", err)
	}
	if !shifted || lower != 200 || upper != 600 {
		t.Fatalf("shifted range = %d-%d shifted=%v, want 200-600 true", lower, upper, shifted)
	}
}

func TestShiftFollowRangeByGridsSkipsNarrowRange(t *testing.T) {
	lower, upper, shifted, err := shiftFollowRangeByGrids(100, 300, 100, 1)
	if err != nil {
		t.Fatalf("shiftFollowRangeByGrids returned error: %v", err)
	}
	if shifted || lower != 100 || upper != 300 {
		t.Fatalf("range = %d-%d shifted=%v, want unchanged false", lower, upper, shifted)
	}
}

func TestShiftFollowRangeByGridsRejectsOutOfFullRange(t *testing.T) {
	_, maxTick, err := poolsvc.FullRangeTicks(200)
	if err != nil {
		t.Fatalf("FullRangeTicks returned error: %v", err)
	}
	_, _, _, err = shiftFollowRangeByGrids(maxTick-600, maxTick, 200, 1)
	if err == nil {
		t.Fatal("expected shifted range outside full range error")
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

func TestFollowOpenEventAction(t *testing.T) {
	tests := []struct {
		name                  string
		existingSameEventOpen bool
		hasOpenTask           bool
		hasOpeningJob         bool
		want                  string
	}{
		{name: "same event existing open remains open", existingSameEventOpen: true, hasOpenTask: true, want: models.SmartMoneyFollowJobActionOpen},
		{name: "existing mapping becomes add liquidity", hasOpenTask: true, want: models.SmartMoneyFollowJobActionAddLiquidity},
		{name: "pending open becomes add liquidity", hasOpeningJob: true, want: models.SmartMoneyFollowJobActionAddLiquidity},
		{name: "first event opens", want: models.SmartMoneyFollowJobActionOpen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := followOpenEventAction(tt.existingSameEventOpen, tt.hasOpenTask, tt.hasOpeningJob)
			if got != tt.want {
				t.Fatalf("action = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestEffectiveFollowConfigsKeepsLatestForSameWalletSet(t *testing.T) {
	oldTime := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Minute)
	configs := []models.SmartMoneyFollowConfig{
		{
			ID:                   1,
			Chain:                "bsc",
			TargetWalletAddress:  "0x0000000000000000000000000000000000000001",
			TargetWallets:        models.StringArray{"0x0000000000000000000000000000000000000001", "0x0000000000000000000000000000000000000002"},
			ExecutionWalletID:    11,
			ExecutionWalletIDs:   models.UintArray{11},
			ExecutionWalletMode:  models.SmartMoneyFollowExecutionWalletModeFixed,
			TriggerMode:          models.SmartMoneyFollowTriggerModeAny,
			TriggerWindowSeconds: 300,
			TriggerMinWallets:    1,
			UpdatedAt:            oldTime,
		},
		{
			ID:                   2,
			Chain:                "bsc",
			TargetWalletAddress:  "0x0000000000000000000000000000000000000002",
			TargetWallets:        models.StringArray{"0x0000000000000000000000000000000000000002", "0x0000000000000000000000000000000000000001"},
			ExecutionWalletID:    21,
			ExecutionWalletIDs:   models.UintArray{21, 22},
			ExecutionWalletMode:  models.SmartMoneyFollowExecutionWalletModeRandom,
			TriggerMode:          models.SmartMoneyFollowTriggerModeAny,
			TriggerWindowSeconds: 300,
			TriggerMinWallets:    1,
			UpdatedAt:            newTime,
		},
	}

	got := effectiveFollowConfigs(configs)
	if len(got) != 1 {
		t.Fatalf("effective config count = %d, want 1", len(got))
	}
	if got[0].ID != 2 {
		t.Fatalf("effective config id = %d, want 2", got[0].ID)
	}
	if got[0].ExecutionWalletID != 21 {
		t.Fatalf("execution wallet id = %d, want 21", got[0].ExecutionWalletID)
	}
}

func TestEffectiveFollowConfigsKeepsDifferentTriggerModes(t *testing.T) {
	wallets := models.StringArray{"0x0000000000000000000000000000000000000001", "0x0000000000000000000000000000000000000002"}
	configs := []models.SmartMoneyFollowConfig{
		{
			ID:                  1,
			Chain:               "bsc",
			TargetWalletAddress: "0x0000000000000000000000000000000000000001",
			TargetWallets:       wallets,
			TriggerMode:         models.SmartMoneyFollowTriggerModeAny,
		},
		{
			ID:                  2,
			Chain:               "bsc",
			TargetWalletAddress: "0x0000000000000000000000000000000000000001",
			TargetWallets:       wallets,
			TriggerMode:         models.SmartMoneyFollowTriggerModeThreshold,
		},
	}

	got := effectiveFollowConfigs(configs)
	if len(got) != 2 {
		t.Fatalf("effective config count = %d, want 2", len(got))
	}
}

func TestEffectiveFollowConfigsDisabledLatestSupersedesEnabledOld(t *testing.T) {
	oldTime := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Minute)
	configs := []models.SmartMoneyFollowConfig{
		{
			ID:                  1,
			Chain:               "bsc",
			TargetWalletAddress: "0x0000000000000000000000000000000000000001",
			TargetWallets:       models.StringArray{"0x0000000000000000000000000000000000000001"},
			TriggerMode:         models.SmartMoneyFollowTriggerModeAny,
			Enabled:             true,
			UpdatedAt:           oldTime,
		},
		{
			ID:                  2,
			Chain:               "bsc",
			TargetWalletAddress: "0x0000000000000000000000000000000000000001",
			TargetWallets:       models.StringArray{"0x0000000000000000000000000000000000000001"},
			TriggerMode:         models.SmartMoneyFollowTriggerModeAny,
			Enabled:             false,
			UpdatedAt:           newTime,
		},
	}

	got := effectiveFollowConfigs(configs)
	if len(got) != 1 {
		t.Fatalf("effective config count = %d, want 1", len(got))
	}
	if got[0].ID != 2 {
		t.Fatalf("effective config id = %d, want 2", got[0].ID)
	}
	if got[0].Enabled {
		t.Fatal("expected disabled latest config to supersede enabled old config")
	}
}

func TestRetryableFollowSlippageError(t *testing.T) {
	if !isRetryableFollowSlippageError(errors.New("swap failed: slippage exceeded")) {
		t.Fatal("expected slippage error to be retryable")
	}
	if isRetryableFollowSlippageError(errors.New("insufficient USDT balance")) {
		t.Fatal("expected balance error to be non-retryable")
	}
}

func TestFollowRetrySlippagePercent(t *testing.T) {
	if got := followRetrySlippagePercent(0, 0); got != 0.5 {
		t.Fatalf("attempt 0 slippage = %v, want 0.5", got)
	}
	if got := followRetrySlippagePercent(2, 1); got != 4 {
		t.Fatalf("base 2 attempt 1 slippage = %v, want 4", got)
	}
	prev := followRetrySlippagePercent(0.5, 0)
	for attempt := 1; attempt <= maxFollowJobRetryCount+3; attempt++ {
		got := followRetrySlippagePercent(0.5, attempt)
		if got < prev {
			t.Fatalf("slippage decreased at attempt %d: %v < %v", attempt, got, prev)
		}
		if got > 10 {
			t.Fatalf("slippage exceeded cap at attempt %d: %v", attempt, got)
		}
		prev = got
	}
}
