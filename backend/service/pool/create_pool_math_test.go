package pool

import (
	"math/big"
	"testing"
)

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

func TestAlignTickSpacing(t *testing.T) {
	down, err := AlignTickDown(125, 60)
	if err != nil {
		t.Fatalf("AlignTickDown failed: %v", err)
	}
	if down != 120 {
		t.Fatalf("expected down=120, got %d", down)
	}

	up, err := AlignTickUp(125, 60)
	if err != nil {
		t.Fatalf("AlignTickUp failed: %v", err)
	}
	if up != 180 {
		t.Fatalf("expected up=180, got %d", up)
	}
}

func TestAmountsForSingleSidedLiquidity(t *testing.T) {
	sqrtP, err := SqrtRatioAtTick(0)
	if err != nil {
		t.Fatalf("SqrtRatioAtTick price failed: %v", err)
	}
	sqrtA, err := SqrtRatioAtTick(-60)
	if err != nil {
		t.Fatalf("SqrtRatioAtTick lower failed: %v", err)
	}
	sqrtB, err := SqrtRatioAtTick(60)
	if err != nil {
		t.Fatalf("SqrtRatioAtTick upper failed: %v", err)
	}
	liq := LiquidityForAmount0(sqrtP, sqrtB, bigFromString(t, "1000000"))
	if liq == nil || liq.Sign() <= 0 {
		t.Fatalf("expected liquidity > 0, got %v", liq)
	}
	amount0, amount1 := AmountsForLiquidity(sqrtP, sqrtA, sqrtB, liq)
	if amount0.Sign() <= 0 || amount1.Sign() <= 0 {
		t.Fatalf("expected both amounts > 0, got amount0=%s amount1=%s", amount0.String(), amount1.String())
	}
}

func bigFromString(t *testing.T, raw string) *big.Int {
	t.Helper()
	v, ok := new(big.Int).SetString(raw, 10)
	if !ok || v == nil {
		t.Fatalf("invalid big int: %s", raw)
	}
	return v
}
