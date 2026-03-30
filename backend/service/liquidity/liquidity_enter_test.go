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

func TestEnsurePoolHasLiquidityRejectsNilOrZero(t *testing.T) {
	t.Parallel()

	cases := []*big.Int{nil, big.NewInt(0)}
	for _, liq := range cases {
		err := ensurePoolHasLiquidity("v3", liq)
		if err == nil {
			t.Fatalf("expected zero liquidity to be rejected, liq=%v", liq)
		}
		var safetyErr *ZapSafetyError
		if !errors.As(err, &safetyErr) {
			t.Fatalf("expected ZapSafetyError, got %T", err)
		}
	}
}

func TestEnsurePoolHasLiquidityAllowsPositive(t *testing.T) {
	t.Parallel()

	if err := ensurePoolHasLiquidity("v4", big.NewInt(1)); err != nil {
		t.Fatalf("expected positive liquidity to pass, got %v", err)
	}
}

func TestEvaluateLiquidityRiskRejectsBelowConfiguredMin(t *testing.T) {
	t.Parallel()

	err := evaluateLiquidityRisk(403, 500, 150, OpenPositionRiskOptions{})
	if err == nil {
		t.Fatal("expected below-min liquidity to be rejected")
	}
	if err.Code != "pool_liquidity_too_low" {
		t.Fatalf("unexpected code: %s", err.Code)
	}
	if err.MinLiquidityUSD != 500 {
		t.Fatalf("expected min liquidity 500, got %.2f", err.MinLiquidityUSD)
	}
}

func TestEvaluateLiquidityRiskRequiresAckInWarningBand(t *testing.T) {
	t.Parallel()

	err := evaluateLiquidityRisk(650, 0, 180, OpenPositionRiskOptions{
		RequireLiquidityAck: true,
	})
	if err == nil {
		t.Fatal("expected warning-band liquidity to require risk acknowledgement")
	}
	if err.Code != "pool_liquidity_warning" {
		t.Fatalf("unexpected code: %s", err.Code)
	}
	if !err.RiskAckRequired {
		t.Fatal("expected risk acknowledgement to be required")
	}
}

func TestEvaluateLiquidityRiskAllowsAckedWarningBand(t *testing.T) {
	t.Parallel()

	err := evaluateLiquidityRisk(650, 0, 180, OpenPositionRiskOptions{
		AckLiquidityRisk:    true,
		RequireLiquidityAck: true,
	})
	if err != nil {
		t.Fatalf("expected acknowledged warning-band liquidity to pass, got %v", err)
	}
}
