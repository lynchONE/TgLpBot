package liquidity

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	"TgLpBot/service/pool"
)

func mustBigIntFromString(t *testing.T, raw string) *big.Int {
	t.Helper()
	v, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		t.Fatalf("invalid big.Int: %s", raw)
	}
	return v
}

func TestEstimateV4LiquidityForAmountsZeroForSingleSidedInRange(t *testing.T) {
	t.Parallel()

	sqrtPriceX96, err := pool.SqrtRatioAtTick(48345)
	if err != nil {
		t.Fatalf("SqrtRatioAtTick() error = %v", err)
	}

	liq, err := estimateV4LiquidityForAmounts(sqrtPriceX96, 35200, 52800, mustBigIntFromString(t, "9971648607554963294"), big.NewInt(0))
	if err != nil {
		t.Fatalf("estimateV4LiquidityForAmounts() error = %v", err)
	}
	if liq == nil || liq.Sign() != 0 {
		t.Fatalf("expected zero liquidity, got %v", liq)
	}
}

func TestEstimateV4LiquidityForAmountsPositiveForTwoSidedInRange(t *testing.T) {
	t.Parallel()

	sqrtPriceX96, err := pool.SqrtRatioAtTick(48345)
	if err != nil {
		t.Fatalf("SqrtRatioAtTick() error = %v", err)
	}

	liq, err := estimateV4LiquidityForAmounts(
		sqrtPriceX96,
		35200,
		52800,
		mustBigIntFromString(t, "5000000000000000000"),
		mustBigIntFromString(t, "500000000000000000000"),
	)
	if err != nil {
		t.Fatalf("estimateV4LiquidityForAmounts() error = %v", err)
	}
	if liq == nil || liq.Sign() <= 0 {
		t.Fatalf("expected positive liquidity, got %v", liq)
	}
}

func TestEvmRevertHintCannotUpdateEmptyPosition(t *testing.T) {
	t.Parallel()

	hint := evmRevertHint(errors.New("execution reverted: 0xaefeb924"))
	if !strings.Contains(hint, "CannotUpdateEmptyPosition") {
		t.Fatalf("expected CannotUpdateEmptyPosition hint, got %q", hint)
	}
}
