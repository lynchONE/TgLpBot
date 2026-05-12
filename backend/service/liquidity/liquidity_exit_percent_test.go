package liquidity

import (
	"math"
	"math/big"
	"strings"
	"testing"
)

func TestValidateExitPercent(t *testing.T) {
	full, partial, err := ValidateExitPercent(nil)
	if err != nil {
		t.Fatalf("nil percent returned error: %v", err)
	}
	if full != 100 || partial {
		t.Fatalf("nil percent = (%v,%v), want (100,false)", full, partial)
	}

	tests := []struct {
		name        string
		value       float64
		wantPartial bool
		wantErr     bool
	}{
		{name: "partial", value: 25, wantPartial: true},
		{name: "full", value: 100, wantPartial: false},
		{name: "zero", value: 0, wantErr: true},
		{name: "negative", value: -1, wantErr: true},
		{name: "too high", value: 100.01, wantErr: true},
		{name: "nan", value: math.NaN(), wantErr: true},
		{name: "inf", value: math.Inf(1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.value
			_, partial, err := ValidateExitPercent(&v)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if partial != tt.wantPartial {
				t.Fatalf("partial = %v, want %v", partial, tt.wantPartial)
			}
		})
	}
}

func TestExitLiquidityForPercent(t *testing.T) {
	tests := []struct {
		name    string
		current string
		percent float64
		want    string
	}{
		{name: "quarter", current: "100000", percent: 25, want: "25000"},
		{name: "full", current: "100000", percent: 100, want: "100000"},
		{name: "round down but at least one", current: "3", percent: 1, want: "1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current, ok := new(big.Int).SetString(tt.current, 10)
			if !ok {
				t.Fatalf("bad current test value %q", tt.current)
			}
			got, err := exitLiquidityForPercent(current, tt.percent)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != tt.want {
				t.Fatalf("exitLiquidityForPercent(%s,%v) = %s, want %s", tt.current, tt.percent, got.String(), tt.want)
			}
		})
	}
}

func TestWithdrawTaskLiquidityOnlyRejectsPartialExit(t *testing.T) {
	svc := &LiquidityService{}
	percent := 25.0
	_, err := svc.WithdrawTaskLiquidityOnlyWithOptions(1, nil, TxOptions{ExitPercent: &percent})
	if err == nil {
		t.Fatal("expected partial exit to be rejected by liquidity-only withdraw")
	}
	if !strings.Contains(err.Error(), "Partial exits must use ExitTaskToUSDTWithOptions") &&
		!strings.Contains(err.Error(), "partial exits must use ExitTaskToUSDTWithOptions") {
		t.Fatalf("unexpected error: %v", err)
	}
}
