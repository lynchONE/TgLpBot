package liquidity

import (
	"TgLpBot/base/config"
	"testing"
)

func TestRequiredPrivateZapVersion_DefaultsTo1(t *testing.T) {
	if got := requiredPrivateZapVersion(config.ChainConfig{}); got != 1 {
		t.Fatalf("expected default version 1, got %d", got)
	}
	if got := requiredPrivateZapVersion(config.ChainConfig{PrivateZapVersion: 0}); got != 1 {
		t.Fatalf("expected default version 1, got %d", got)
	}
	if got := requiredPrivateZapVersion(config.ChainConfig{PrivateZapVersion: -10}); got != 1 {
		t.Fatalf("expected default version 1, got %d", got)
	}
}

func TestRequiredPrivateZapVersion_UsesConfiguredValue(t *testing.T) {
	if got := requiredPrivateZapVersion(config.ChainConfig{PrivateZapVersion: 2}); got != 2 {
		t.Fatalf("expected version 2, got %d", got)
	}
	if got := requiredPrivateZapVersion(config.ChainConfig{PrivateZapVersion: 100}); got != 100 {
		t.Fatalf("expected version 100, got %d", got)
	}
}

func TestPrivateZapCacheKey_NormalizesChainAndIncludesVersion(t *testing.T) {
	key := privateZapCacheKey("BASE", 123, 2)
	want := "private_zap:binding:base:123:zap_simple:v2"
	if key != want {
		t.Fatalf("unexpected cache key: got=%s want=%s", key, want)
	}
}

func TestPrivateZapCacheKey_DefaultVersion(t *testing.T) {
	key := privateZapCacheKey("bsc", 1, 0)
	want := "private_zap:binding:bsc:1:zap_simple:v1"
	if key != want {
		t.Fatalf("unexpected cache key: got=%s want=%s", key, want)
	}
}
