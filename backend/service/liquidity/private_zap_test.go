package liquidity

import (
	"TgLpBot/base/models"
	"testing"
)

func TestPrivateZapCacheKey_NormalizesChain(t *testing.T) {
	key := privateZapCacheKey("BASE", 123)
	want := "private_zap:binding:v2:base:123:zap_simple"
	if key != want {
		t.Fatalf("unexpected cache key: got=%s want=%s", key, want)
	}
}

func TestPrivateZapCacheScanPattern_NormalizesChain(t *testing.T) {
	pattern := privateZapCacheScanPattern("BSC")
	want := "private_zap:binding:v2:bsc:*"
	if pattern != want {
		t.Fatalf("unexpected cache pattern: got=%s want=%s", pattern, want)
	}
}

func TestPrivateContractCacheKey_AtomicIndependentPrefix(t *testing.T) {
	key := privateContractCacheKey("BASE", 123, walletChainContractKindAtomicIncreaseZap)
	want := "private_atomic_increase_zap:binding:v2:base:123:atomic_increase_zap"
	if key != want {
		t.Fatalf("unexpected atomic cache key: got=%s want=%s", key, want)
	}
}

func TestIsPrivateZapBindingUsable(t *testing.T) {
	tests := []struct {
		name    string
		binding models.WalletChainContract
		want    bool
	}{
		{
			name: "legacy valid without required version",
			binding: models.WalletChainContract{
				ContractAddress: "0x1111111111111111111111111111111111111111",
			},
			want: false,
		},
		{
			name: "ready",
			binding: models.WalletChainContract{
				Status:          walletChainContractStatusReady,
				ContractAddress: "0x1111111111111111111111111111111111111111",
				Version:         privateZapSimpleBindingVersion,
			},
			want: true,
		},
		{
			name: "deployed pending config",
			binding: models.WalletChainContract{
				Status:          walletChainContractStatusDeployed,
				ContractAddress: "0x1111111111111111111111111111111111111111",
				Version:         privateZapSimpleBindingVersion,
			},
			want: false,
		},
		{
			name: "empty",
			binding: models.WalletChainContract{
				ContractAddress: "",
			},
			want: false,
		},
		{
			name: "invalid",
			binding: models.WalletChainContract{
				ContractAddress: "not-an-address",
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPrivateZapBindingUsable(tc.binding); got != tc.want {
				t.Fatalf("unexpected usable result: got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestIsPrivateZapBindingConfigPending(t *testing.T) {
	tests := []struct {
		name    string
		binding models.WalletChainContract
		want    bool
	}{
		{
			name: "pending deployed",
			binding: models.WalletChainContract{
				Status:          walletChainContractStatusDeployed,
				ContractAddress: "0x1111111111111111111111111111111111111111",
				Version:         privateZapSimpleBindingVersion,
			},
			want: true,
		},
		{
			name: "legacy ready",
			binding: models.WalletChainContract{
				ContractAddress: "0x1111111111111111111111111111111111111111",
			},
			want: false,
		},
		{
			name: "ready",
			binding: models.WalletChainContract{
				Status:          walletChainContractStatusReady,
				ContractAddress: "0x1111111111111111111111111111111111111111",
				Version:         privateZapSimpleBindingVersion,
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPrivateZapBindingConfigPending(tc.binding); got != tc.want {
				t.Fatalf("unexpected pending result: got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestIsPrivateContractBindingUsable_AtomicVersionIndependent(t *testing.T) {
	binding := models.WalletChainContract{
		Status:          walletChainContractStatusReady,
		ContractAddress: "0x1111111111111111111111111111111111111111",
		Version:         privateAtomicIncreaseZapBindingVersion,
	}
	if !isPrivateContractBindingUsable(binding, privateAtomicIncreaseZapBindingVersion) {
		t.Fatalf("expected atomic binding to be usable")
	}
	if isPrivateContractBindingUsable(binding, privateAtomicIncreaseZapBindingVersion+1) {
		t.Fatalf("expected atomic binding to be unusable for higher required version")
	}
}
