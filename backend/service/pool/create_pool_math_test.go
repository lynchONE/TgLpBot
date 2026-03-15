package pool

import "testing"

func TestFullRangeTicks(t *testing.T) {
	lower, upper, err := FullRangeTicks(60)
	if err != nil {
		t.Fatalf("FullRangeTicks failed: %v", err)
	}
	if lower != -887220 || upper != 887220 {
		t.Fatalf("unexpected ticks: lower=%d upper=%d", lower, upper)
	}
}

func TestSqrtPriceX96FromHumanPrice(t *testing.T) {
	price, err := ParseDecimalToFloat("1")
	if err != nil {
		t.Fatalf("ParseDecimalToFloat failed: %v", err)
	}
	sqrt, err := SqrtPriceX96FromHumanPrice(price, 18, 18)
	if err != nil {
		t.Fatalf("SqrtPriceX96FromHumanPrice failed: %v", err)
	}
	if sqrt.Cmp(q96) != 0 {
		t.Fatalf("expected q96, got %s", sqrt.String())
	}
}

func TestTickFromHumanPrice(t *testing.T) {
	tick, err := TickFromHumanPrice(1, 18, 18)
	if err != nil {
		t.Fatalf("TickFromHumanPrice failed: %v", err)
	}
	if tick != 0 {
		t.Fatalf("expected tick 0, got %d", tick)
	}
}

func TestDecimalToUnits(t *testing.T) {
	units, err := DecimalToUnits("1.23", 6)
	if err != nil {
		t.Fatalf("DecimalToUnits failed: %v", err)
	}
	if units.String() != "1230000" {
		t.Fatalf("unexpected units: %s", units.String())
	}
}
