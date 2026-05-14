package web_server

import "testing"

func TestRequestAddLiquiditySlippage(t *testing.T) {
	value := 0.8
	got, err := requestAddLiquiditySlippage(&value, nil)
	if err != nil {
		t.Fatalf("requestAddLiquiditySlippage returned error: %v", err)
	}
	if got == nil || *got != value {
		t.Fatalf("slippage = %v, want %v", got, value)
	}
}

func TestRequestAddLiquiditySlippageRejectsConflict(t *testing.T) {
	left := 0.5
	right := 0.8
	if _, err := requestAddLiquiditySlippage(&left, &right); err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestRequestAddLiquiditySlippageRejectsOutOfRange(t *testing.T) {
	value := 100.1
	if _, err := requestAddLiquiditySlippage(&value, nil); err == nil {
		t.Fatal("expected range error")
	}
}

func TestRequestAddLiquiditySlippageAllowsDefault(t *testing.T) {
	got, err := requestAddLiquiditySlippage(nil, nil)
	if err != nil {
		t.Fatalf("requestAddLiquiditySlippage returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("slippage = %v, want nil", got)
	}
}
