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
