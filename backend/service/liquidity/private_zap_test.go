package liquidity

import (
	"TgLpBot/base/models"
	"testing"
)

func TestPrivateZapCacheKey_NormalizesChain(t *testing.T) {
	key := privateZapCacheKey("BASE", 123)
	want := "private_zap:binding:base:123:zap_simple"
	if key != want {
		t.Fatalf("unexpected cache key: got=%s want=%s", key, want)
	}
}

func TestPrivateZapCacheScanPattern_NormalizesChain(t *testing.T) {
	pattern := privateZapCacheScanPattern("BSC")
	want := "private_zap:binding:bsc:*"
	if pattern != want {
		t.Fatalf("unexpected cache pattern: got=%s want=%s", pattern, want)
	}
}

func TestIsPrivateZapBindingUsable(t *testing.T) {
	tests := []struct {
		name    string
		binding models.WalletChainContract
		want    bool
	}{
		{
			name: "valid",
			binding: models.WalletChainContract{
				ContractAddress: "0x1111111111111111111111111111111111111111",
			},
			want: true,
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
