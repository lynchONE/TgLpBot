package liquidity

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	"TgLpBot/base/blockchain"
	"TgLpBot/service/pool"
	"github.com/ethereum/go-ethereum/common"
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

func TestEvmRevertHintMaximumAmountExceeded(t *testing.T) {
	t.Parallel()

	hint := evmRevertHint(errors.New("execution reverted: 0x31e30ad0"))
	if !strings.Contains(hint, "MaximumAmountExceeded") {
		t.Fatalf("expected MaximumAmountExceeded hint, got %q", hint)
	}
}

func TestPickIncreasePositionRangePrefersOnchain(t *testing.T) {
	t.Parallel()

	lower, upper, synced := pickIncreasePositionRange(-100, 100, -120, 120)
	if lower != -120 || upper != 120 {
		t.Fatalf("range = %d/%d, want -120/120", lower, upper)
	}
	if !synced {
		t.Fatal("expected synced=true")
	}
}

func TestPickIncreasePositionRangeFallsBackToTaskWhenOnchainInvalid(t *testing.T) {
	t.Parallel()

	lower, upper, synced := pickIncreasePositionRange(-100, 100, 0, 0)
	if lower != -100 || upper != 100 {
		t.Fatalf("range = %d/%d, want -100/100", lower, upper)
	}
	if synced {
		t.Fatal("expected synced=false")
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

func TestEvaluateLiquidityRiskAllowsWarningBandWithoutAckRequirement(t *testing.T) {
	t.Parallel()

	err := evaluateLiquidityRisk(650, 0, 180, OpenPositionRiskOptions{})
	if err != nil {
		t.Fatalf("expected warning-band liquidity to pass without forced acknowledgement, got %v", err)
	}
}

func TestEvaluateLiquidityRiskRejectsExcessOpenAmountWithoutAckRequirement(t *testing.T) {
	t.Parallel()

	err := evaluateLiquidityRisk(650, 0, 220, OpenPositionRiskOptions{})
	if err == nil {
		t.Fatal("expected excess open amount to be rejected")
	}
	if err.Code != "pool_liquidity_warning" {
		t.Fatalf("unexpected code: %s", err.Code)
	}
	if err.RiskAckRequired {
		t.Fatal("expected excess open amount rejection to not require acknowledgement")
	}
}

func TestSwapOutputDustRatioBpsUsesSwapOutputSide(t *testing.T) {
	t.Parallel()

	sim := &blockchain.ZapResultSimple{
		Amount0Used: big.NewInt(200),
		Amount1Used: big.NewInt(20),
		Dust0:       big.NewInt(800),
		Dust1:       big.NewInt(80),
	}

	if got := swapOutputDustRatioBps(sim, true); got != 8000 {
		t.Fatalf("zeroForOne ratio = %d, want 8000", got)
	}
	if got := swapOutputDustRatioBps(sim, false); got != 8000 {
		t.Fatalf("oneForZero ratio = %d, want 8000", got)
	}
}

func TestReduceOneSidedSwapAmountScalesByUsedOutputShare(t *testing.T) {
	t.Parallel()

	next, ok := reduceOneSidedSwapAmount(
		big.NewInt(1000),
		&blockchain.ZapResultSimple{
			Amount1Used: big.NewInt(25),
			Dust1:       big.NewInt(75),
		},
		true,
	)
	if !ok {
		t.Fatal("expected reduction to be accepted")
	}
	if next.Cmp(big.NewInt(250)) != 0 {
		t.Fatalf("reduced swap = %s, want 250", next.String())
	}
}

func TestBuildZapInV4ParamsPreservesCoreFields(t *testing.T) {
	t.Parallel()

	poolKey := blockchain.PoolKeySimple{
		Currency0:   common.HexToAddress("0x0000000000000000000000000000000000000001"),
		Currency1:   common.HexToAddress("0x0000000000000000000000000000000000000002"),
		Fee:         big.NewInt(3000),
		TickSpacing: big.NewInt(60),
		Hooks:       common.HexToAddress("0x0000000000000000000000000000000000000003"),
	}
	swap := blockchain.SwapParamsSimple{
		Target:       common.HexToAddress("0x0000000000000000000000000000000000000004"),
		TokenIn:      poolKey.Currency0,
		TokenOut:     poolKey.Currency1,
		AmountIn:     big.NewInt(123),
		MinAmountOut: big.NewInt(456),
		CallData:     []byte{0xaa, 0xbb},
	}

	params := buildZapInV4Params(
		poolKey,
		common.HexToAddress("0x0000000000000000000000000000000000000005"),
		common.HexToAddress("0x0000000000000000000000000000000000000006"),
		common.HexToAddress("0x0000000000000000000000000000000000000007"),
		-120,
		120,
		big.NewInt(1000),
		big.NewInt(0),
		big.NewInt(50),
		swap,
		big.NewInt(789),
	)

	if params.PoolKey.Currency0 != poolKey.Currency0 || params.PoolKey.Currency1 != poolKey.Currency1 {
		t.Fatalf("pool key currencies not preserved: %+v", params.PoolKey)
	}
	if params.Swap.Target != swap.Target || params.Swap.TokenIn != swap.TokenIn || params.Swap.TokenOut != swap.TokenOut {
		t.Fatalf("swap addresses not preserved: %+v", params.Swap)
	}
	if params.Swap.AmountIn.Cmp(swap.AmountIn) != 0 || params.Swap.MinAmountOut.Cmp(swap.MinAmountOut) != 0 {
		t.Fatalf("swap amounts not preserved: %+v", params.Swap)
	}
	if params.TickLower.Cmp(big.NewInt(-120)) != 0 || params.TickUpper.Cmp(big.NewInt(120)) != 0 {
		t.Fatalf("tick range not preserved: lower=%s upper=%s", params.TickLower.String(), params.TickUpper.String())
	}
	if params.SqrtPriceX96.Cmp(big.NewInt(789)) != 0 {
		t.Fatalf("sqrtPriceX96 = %s, want 789", params.SqrtPriceX96.String())
	}
}

func TestPickRecordedOpenDustPrefersParsedDustWhenSignalsDiverge(t *testing.T) {
	t.Parallel()

	got := pickRecordedOpenDust(big.NewInt(1000), big.NewInt(90))
	if got.Cmp(big.NewInt(90)) != 0 {
		t.Fatalf("pickRecordedOpenDust() = %s, want 90", got.String())
	}
}

func TestPickRecordedOpenDustPrefersWalletWhenSignalsClose(t *testing.T) {
	t.Parallel()

	got := pickRecordedOpenDust(big.NewInt(95), big.NewInt(90))
	if got.Cmp(big.NewInt(95)) != 0 {
		t.Fatalf("pickRecordedOpenDust() = %s, want 95", got.String())
	}
}

func TestPickRecordedOpenDustFallsBackToParsedDust(t *testing.T) {
	t.Parallel()

	got := pickRecordedOpenDust(big.NewInt(0), big.NewInt(99))
	if got.Cmp(big.NewInt(99)) != 0 {
		t.Fatalf("pickRecordedOpenDust() = %s, want 99", got.String())
	}
}

func TestPickRecordedOpenDustReturnsZeroWhenMissing(t *testing.T) {
	t.Parallel()

	got := pickRecordedOpenDust(nil, nil)
	if got.Sign() != 0 {
		t.Fatalf("pickRecordedOpenDust() = %s, want 0", got.String())
	}
}

func TestShouldRetryDustReadWhenSignalsDiverge(t *testing.T) {
	t.Parallel()

	if !shouldRetryDustRead(big.NewInt(1000), big.NewInt(90)) {
		t.Fatal("shouldRetryDustRead() = false, want true")
	}
}
