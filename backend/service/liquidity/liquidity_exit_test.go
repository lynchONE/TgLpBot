package liquidity

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"TgLpBot/service/pool"
)

// expectedAmounts mirrors what exitMinAmounts uses internally, so the tests assert the
// slippage application and guards rather than re-deriving the Uniswap liquidity math.
func expectedAmounts(t *testing.T, tickLower, tickUpper int, liq *big.Int) (*big.Int, *big.Int) {
	t.Helper()
	sqrtP, err := pool.SqrtRatioAtTick(int32((tickLower + tickUpper) / 2))
	if err != nil {
		t.Fatalf("sqrtP: %v", err)
	}
	sqrtA, _ := pool.SqrtRatioAtTick(int32(tickLower))
	sqrtB, _ := pool.SqrtRatioAtTick(int32(tickUpper))
	a0, a1 := pool.AmountsForLiquidity(sqrtP, sqrtA, sqrtB, liq)
	return a0, a1
}

func sqrtPAtMid(t *testing.T, tickLower, tickUpper int) *big.Int {
	t.Helper()
	sqrtP, err := pool.SqrtRatioAtTick(int32((tickLower + tickUpper) / 2))
	if err != nil {
		t.Fatalf("sqrtP: %v", err)
	}
	return sqrtP
}

func TestExitMinAmounts_Guards(t *testing.T) {
	liq := big.NewInt(1_000_000_000_000_000_000)
	sqrtP := sqrtPAtMid(t, -100, 100)

	cases := []struct {
		name     string
		sqrt     *big.Int
		lower    int
		upper    int
		liq      *big.Int
		slippage float64
	}{
		{"nil price", nil, -100, 100, liq, 0.5},
		{"zero price", big.NewInt(0), -100, 100, liq, 0.5},
		{"nil liquidity", sqrtP, -100, 100, nil, 0.5},
		{"zero liquidity", sqrtP, -100, 100, big.NewInt(0), 0.5},
		{"inverted ticks", sqrtP, 100, -100, liq, 0.5},
		{"equal ticks", sqrtP, 100, 100, liq, 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			min0, min1 := exitMinAmounts(tc.sqrt, tc.lower, tc.upper, tc.liq, tc.slippage)
			if min0.Sign() != 0 || min1.Sign() != 0 {
				t.Fatalf("expected (0,0), got (%s,%s)", min0, min1)
			}
		})
	}
}

func TestExitMinAmounts_ZeroSlippageEqualsExpected(t *testing.T) {
	lower, upper := -600, 600
	liq := big.NewInt(5_000_000_000_000_000_000)
	exp0, exp1 := expectedAmounts(t, lower, upper, liq)
	sqrtP := sqrtPAtMid(t, lower, upper)

	min0, min1 := exitMinAmounts(sqrtP, lower, upper, liq, 0)
	if min0.Cmp(exp0) != 0 || min1.Cmp(exp1) != 0 {
		t.Fatalf("slippage=0 should equal expected: got (%s,%s) want (%s,%s)", min0, min1, exp0, exp1)
	}
	if exp0.Sign() <= 0 || exp1.Sign() <= 0 {
		t.Fatalf("expected in-range amounts to be positive, got (%s,%s)", exp0, exp1)
	}
}

func TestExitMinAmounts_FullSlippageIsZero(t *testing.T) {
	lower, upper := -600, 600
	liq := big.NewInt(5_000_000_000_000_000_000)
	sqrtP := sqrtPAtMid(t, lower, upper)

	min0, min1 := exitMinAmounts(sqrtP, lower, upper, liq, 100)
	if min0.Sign() != 0 || min1.Sign() != 0 {
		t.Fatalf("slippage=100 should yield (0,0), got (%s,%s)", min0, min1)
	}
}

func TestExitMinAmounts_AppliesSlippageBps(t *testing.T) {
	lower, upper := -600, 600
	liq := big.NewInt(5_000_000_000_000_000_000)
	exp0, exp1 := expectedAmounts(t, lower, upper, liq)
	sqrtP := sqrtPAtMid(t, lower, upper)

	// 0.5% -> keep 9950 bps.
	min0, min1 := exitMinAmounts(sqrtP, lower, upper, liq, 0.5)
	want0 := new(big.Int).Div(new(big.Int).Mul(exp0, big.NewInt(9950)), big.NewInt(10000))
	want1 := new(big.Int).Div(new(big.Int).Mul(exp1, big.NewInt(9950)), big.NewInt(10000))
	if min0.Cmp(want0) != 0 || min1.Cmp(want1) != 0 {
		t.Fatalf("0.5%% slippage: got (%s,%s) want (%s,%s)", min0, min1, want0, want1)
	}
}

func TestExitMinAmounts_Monotonic(t *testing.T) {
	lower, upper := -600, 600
	liq := big.NewInt(5_000_000_000_000_000_000)
	sqrtP := sqrtPAtMid(t, lower, upper)

	tighter0, tighter1 := exitMinAmounts(sqrtP, lower, upper, liq, 1)
	looser0, looser1 := exitMinAmounts(sqrtP, lower, upper, liq, 5)
	if looser0.Cmp(tighter0) > 0 || looser1.Cmp(tighter1) > 0 {
		t.Fatalf("higher slippage must not increase min: 1%%=(%s,%s) 5%%=(%s,%s)", tighter0, tighter1, looser0, looser1)
	}
}

func TestExitMinAmounts_PriceBelowRangeIsToken0Only(t *testing.T) {
	lower, upper := -100, 100
	liq := big.NewInt(1_000_000_000_000_000_000)
	// Price well below the range: all liquidity sits in token0, so amount1 (and its min) is 0.
	sqrtP, err := pool.SqrtRatioAtTick(int32(-300))
	if err != nil {
		t.Fatalf("sqrtP: %v", err)
	}
	min0, min1 := exitMinAmounts(sqrtP, lower, upper, liq, 0.5)
	if min1.Sign() != 0 {
		t.Fatalf("price below range: amount1Min should be 0, got %s", min1)
	}
	if min0.Sign() <= 0 {
		t.Fatalf("price below range: amount0Min should be positive, got %s", min0)
	}
}

func TestExitMinAmounts_PriceAboveRangeIsToken1Only(t *testing.T) {
	lower, upper := -100, 100
	liq := big.NewInt(1_000_000_000_000_000_000)
	// Price well above the range: all liquidity sits in token1, so amount0 (and its min) is 0.
	sqrtP, err := pool.SqrtRatioAtTick(int32(300))
	if err != nil {
		t.Fatalf("sqrtP: %v", err)
	}
	min0, min1 := exitMinAmounts(sqrtP, lower, upper, liq, 0.5)
	if min0.Sign() != 0 {
		t.Fatalf("price above range: amount0Min should be 0, got %s", min0)
	}
	if min1.Sign() <= 0 {
		t.Fatalf("price above range: amount1Min should be positive, got %s", min1)
	}
}

func TestExtractTxHashesFindsHashInsideLabel(t *testing.T) {
	want := common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")
	got := extractTxHashes([]string{
		"撤出流动性" + want.Hex(),
		"兑换 Token0->USDT|" + want.Hex(),
		"not-a-hash",
	})
	if len(got) != 1 {
		t.Fatalf("extractTxHashes len=%d want 1 (%v)", len(got), got)
	}
	if got[0] != want {
		t.Fatalf("extractTxHashes[0]=%s want %s", got[0].Hex(), want.Hex())
	}
}
