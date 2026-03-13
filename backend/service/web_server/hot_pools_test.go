package web_server

import (
	"TgLpBot/base/config"
	"testing"
)

func TestNormalizeHotPoolTokenAddress(t *testing.T) {
	got := normalizeHotPoolTokenAddress("0x55d398326f99059ff775485246999027b3197955")
	if got != "0x55d398326f99059ff775485246999027b3197955" {
		t.Fatalf("unexpected normalized address: %s", got)
	}
}

func TestNormalizeHotPoolTokenAddress_Invalid(t *testing.T) {
	if got := normalizeHotPoolTokenAddress("not-an-address"); got != "" {
		t.Fatalf("expected empty result, got %s", got)
	}
}

func TestBuildHotPoolsCacheKey_NormalizesIncludePoolsOrder(t *testing.T) {
	a := buildHotPoolsCacheKey(
		"bsc",
		5,
		50,
		"fees",
		"pancakeswap",
		"",
		[]string{"0xbbb", "0xaaa"},
	)
	b := buildHotPoolsCacheKey(
		"bsc",
		5,
		50,
		"fees",
		"pancakeswap",
		"",
		[]string{"0xaaa", "0xbbb"},
	)
	if a != b {
		t.Fatalf("expected stable cache key, got %s != %s", a, b)
	}
}

func TestResolveHotPoolDisplayToken_PrefersNonBaseLikeToken(t *testing.T) {
	prevCfg := config.AppConfig
	config.AppConfig = &config.Config{
		Chains: map[string]config.ChainConfig{
			"bsc": {
				Chain:                "bsc",
				StableAddress:        "0x55d398326f99059ff775485246999027b3197955",
				USDTAddress:          "0x55d398326f99059ff775485246999027b3197955",
				WrappedNativeAddress: "0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c",
			},
		},
	}
	defer func() {
		config.AppConfig = prevCfg
	}()

	addr, symbol := resolveHotPoolDisplayToken(
		"bsc",
		"USDT/ABC",
		"0x55d398326f99059ff775485246999027b3197955",
		"0x1111111111111111111111111111111111111111",
	)
	if addr != "0x1111111111111111111111111111111111111111" || symbol != "ABC" {
		t.Fatalf("unexpected display token: addr=%s symbol=%s", addr, symbol)
	}
}

func TestResolveHotPoolDisplayToken_PrefersAltOverWrappedNative(t *testing.T) {
	prevCfg := config.AppConfig
	config.AppConfig = &config.Config{
		Chains: map[string]config.ChainConfig{
			"bsc": {
				Chain:                "bsc",
				WrappedNativeAddress: "0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c",
			},
		},
	}
	defer func() {
		config.AppConfig = prevCfg
	}()

	addr, symbol := resolveHotPoolDisplayToken(
		"bsc",
		"WBNB/XYZ",
		"0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c",
		"0x2222222222222222222222222222222222222222",
	)
	if addr != "0x2222222222222222222222222222222222222222" || symbol != "XYZ" {
		t.Fatalf("unexpected display token: addr=%s symbol=%s", addr, symbol)
	}
}
