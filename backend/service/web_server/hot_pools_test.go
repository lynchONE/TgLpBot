package web_server

import "testing"

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
