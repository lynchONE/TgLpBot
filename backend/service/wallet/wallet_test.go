package wallet

import (
	"testing"

	"TgLpBot/base/config"
	"TgLpBot/base/models"
)

func TestFilterBusinessWalletsExcludesConfiguredAdminWallet(t *testing.T) {
	oldConfig := config.AppConfig
	t.Cleanup(func() { config.AppConfig = oldConfig })

	const adminAddress = "0x1111111111111111111111111111111111111111"
	config.AppConfig = &config.Config{AdminWalletAddress: adminAddress}

	wallets := []models.Wallet{
		{ID: 1, Address: adminAddress, Name: "admin"},
		{ID: 2, Address: "0x2222222222222222222222222222222222222222", Name: "trading"},
	}

	got := filterBusinessWallets(wallets)
	if len(got) != 1 {
		t.Fatalf("expected 1 business wallet, got %d", len(got))
	}
	if got[0].ID != 2 {
		t.Fatalf("expected wallet 2 to remain, got wallet %d", got[0].ID)
	}
}
