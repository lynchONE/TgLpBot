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
		{name: "binance", input: "Binance", expected: "binance"},
		{name: "zero x removed", input: "0x", wantErr: true},
		{name: "lifi removed", input: "lifi", wantErr: true},
		{name: "li fi removed", input: "LI.FI", wantErr: true},
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

	got, err = parseDecimalAmountToBigInt("198.160099413555617348", 18)
	if err != nil {
		t.Fatalf("unexpected high precision error: %v", err)
	}
	if got.String() != "198160099413555617348" {
		t.Fatalf("high precision amount = %s, want 198160099413555617348", got.String())
	}

	got, err = parseDecimalAmountToBigInt("1.230000", 2)
	if err != nil {
		t.Fatalf("unexpected trailing zero precision error: %v", err)
	}
	if got.String() != "123" {
		t.Fatalf("trimmed precision amount = %s, want 123", got.String())
	}

	if _, err := parseDecimalAmountToBigInt("0", 18); err == nil {
		t.Fatalf("expected zero amount error")
	}
	if _, err := parseDecimalAmountToBigInt("bad", 18); err == nil {
		t.Fatalf("expected invalid amount error")
	}
	if _, err := parseDecimalAmountToBigInt("1.001", 2); err == nil {
		t.Fatalf("expected excess precision error")
	}
}

func TestFormatWalletSwapRawAmount(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		decimals int
		expected string
	}{
		{name: "eighteen decimals", raw: "198160099413555617348", decimals: 18, expected: "198.160099413555617348"},
		{name: "small", raw: "12", decimals: 18, expected: "0.000000000000000012"},
		{name: "whole", raw: "123000000", decimals: 6, expected: "123"},
		{name: "zero", raw: "0", decimals: 18, expected: "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, ok := new(big.Int).SetString(tt.raw, 10)
			if !ok {
				t.Fatalf("invalid raw test amount")
			}
			if got := formatWalletSwapRawAmount(raw, tt.decimals); got != tt.expected {
				t.Fatalf("format = %q, want %q", got, tt.expected)
			}
		})
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
