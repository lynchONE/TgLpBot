package web_server

import (
	"math/big"
	"testing"

	"TgLpBot/base/models"
)

func TestNormalizeLimitOrderProvider(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{name: "empty uses best", input: "", expected: models.WalletSwapLimitOrderProviderBest},
		{name: "best", input: " BEST ", expected: models.WalletSwapLimitOrderProviderBest},
		{name: "okx", input: "OKX", expected: "okx"},
		{name: "zero x", input: "0x", expected: "0x"},
		{name: "lifi alias", input: "lifi", expected: "li.fi"},
		{name: "li fi", input: "LI.FI", expected: "li.fi"},
		{name: "unsupported", input: "abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeLimitOrderProvider(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Fatalf("provider = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestParseDecimalAmountToBigInt(t *testing.T) {
	got, err := parseDecimalAmountToBigInt("1.234567", 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != "1234567" {
		t.Fatalf("amount = %s, want 1234567", got.String())
	}

	if _, err := parseDecimalAmountToBigInt("0", 18); err == nil {
		t.Fatalf("expected zero amount error")
	}
	if _, err := parseDecimalAmountToBigInt("bad", 18); err == nil {
		t.Fatalf("expected invalid amount error")
	}
}

func TestTargetToAmountFromPrice(t *testing.T) {
	fromAmount := new(big.Int).Mul(big.NewInt(2), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	got, err := targetToAmountFromPrice(fromAmount, "2500", 6, 18)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != "5000000000" {
		t.Fatalf("target amount = %s, want 5000000000", got.String())
	}

	if _, err := targetToAmountFromPrice(fromAmount, "0", 6, 18); err == nil {
		t.Fatalf("expected zero price error")
	}
	if _, err := targetToAmountFromPrice(fromAmount, "bad", 6, 18); err == nil {
		t.Fatalf("expected invalid price error")
	}
}
