package web_server

import (
	"TgLpBot/base/models"
	"TgLpBot/service/smart_money_golden_dog"
	"testing"
)

func TestApplySmartMoneyGoldenDogNestedUpdatesOverridesFlatFields(t *testing.T) {
	updates := map[string]any{}
	req := &smartMoneyGoldenDogUpdateRequest{
		Enabled:         boolPtr(true),
		MinWallets:      intPtr(2),
		WalletIntensity: stringPtr(smart_money_golden_dog.BarkIntensityRing),
		PoolEnabled:     boolPtr(false),
	}

	applySmartMoneyGoldenDogFlatUpdates(updates, req)
	applySmartMoneyGoldenDogNestedUpdates(updates,
		&smartMoneyGoldenDogWalletModePayload{
			Enabled:    boolPtr(false),
			MinWallets: intPtr(5),
			Intensity:  stringPtr(smart_money_golden_dog.BarkIntensityPersistentRing),
		},
		&smartMoneyGoldenDogPoolModePayload{
			Enabled:             boolPtr(true),
			MinTransactionCount: intPtr(12),
		},
	)

	if got := updates["enabled"]; got != false {
		t.Fatalf("expected nested wallet enabled=false, got %#v", got)
	}
	if got := updates["min_wallets"]; got != 5 {
		t.Fatalf("expected nested min_wallets=5, got %#v", got)
	}
	if got := updates["wallet_intensity"]; got != smart_money_golden_dog.BarkIntensityPersistentRing {
		t.Fatalf("expected persistent wallet intensity, got %#v", got)
	}
	if got := updates["pool_enabled"]; got != true {
		t.Fatalf("expected nested pool enabled=true, got %#v", got)
	}
	if got := updates["pool_min_transaction_count"]; got != 12 {
		t.Fatalf("expected pool_min_transaction_count=12, got %#v", got)
	}
}

func TestApplySmartMoneyGoldenDogPreviewKeepsPoolThresholdValidationFriendly(t *testing.T) {
	cfg := &models.SmartMoneyGoldenDogConfig{}
	applySmartMoneyGoldenDogPreview(cfg, map[string]any{
		"pool_enabled":                    true,
		"pool_min_total_fees":             1500.0,
		"pool_min_active_liquidity_ratio": 0.25,
		"pool_intensity":                  smart_money_golden_dog.BarkIntensityCriticalRing,
	})

	if !cfg.PoolEnabled {
		t.Fatal("expected pool mode enabled")
	}
	if !smart_money_golden_dog.HasPoolThresholds(*cfg) {
		t.Fatal("expected pool thresholds to be detected")
	}
	if cfg.PoolIntensity != smart_money_golden_dog.BarkIntensityCriticalRing {
		t.Fatalf("expected critical intensity, got %q", cfg.PoolIntensity)
	}
}

func boolPtr(v bool) *bool       { return &v }
func intPtr(v int) *int          { return &v }
func stringPtr(v string) *string { return &v }
