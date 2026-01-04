package pool

import (
	"fmt"
	"math/big"
)

const (
	uniswapMinTick = -887272
	uniswapMaxTick = 887272
)

var (
	q96        = new(big.Int).Lsh(big.NewInt(1), 96)
	q128       = new(big.Int).Lsh(big.NewInt(1), 128)
	maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
)

type tickMulConst struct {
	mask  int32
	value *big.Int
}

var tickMulConsts = []tickMulConst{
	{0x1, mustHexBig("0xfffcb933bd6fad37aa2d162d1a594001")},
	{0x2, mustHexBig("0xfff97272373d413259a46990580e213a")},
	{0x4, mustHexBig("0xfff2e50f5f656932ef12357cf3c7fdcc")},
	{0x8, mustHexBig("0xffe5caca7e10e4e61c3624eaa0941cd0")},
	{0x10, mustHexBig("0xffcb9843d60f6159c9db58835c926644")},
	{0x20, mustHexBig("0xff973b41fa98c081472e6896dfb254c0")},
	{0x40, mustHexBig("0xff2ea16466c96a3843ec78b326b52861")},
	{0x80, mustHexBig("0xfe5dee046a99a2a811c461f1969c3053")},
	{0x100, mustHexBig("0xfcbe86c7900a88aedcffc83b479aa3a4")},
	{0x200, mustHexBig("0xf987a7253ac413176f2b074cf7815e54")},
	{0x400, mustHexBig("0xf3392b0822b70005940c7a398e4b70f3")},
	{0x800, mustHexBig("0xe7159475a2c29b7443b29c7fa6e889d9")},
	{0x1000, mustHexBig("0xd097f3bdfd2022b8845ad8f792aa5825")},
	{0x2000, mustHexBig("0xa9f746462d870fdf8a65dc1f90e061e5")},
	{0x4000, mustHexBig("0x70d869a156d2a1b890bb3df62baf32f7")},
	{0x8000, mustHexBig("0x31be135f97d08fd981231505542fcfa6")},
	{0x10000, mustHexBig("0x9aa508b5b7a84e1c677de54f3e99bc9")},
	{0x20000, mustHexBig("0x5d6af8dedb81196699c329225ee604")},
	{0x40000, mustHexBig("0x2216e584f5fa1ea926041bedfe98")},
	{0x80000, mustHexBig("0x48a170391f7dc42444e8fa2")},
}

func mustHexBig(s string) *big.Int {
	v, ok := new(big.Int).SetString(trim0x(s), 16)
	if !ok {
		panic("invalid hex big int: " + s)
	}
	return v
}

func trim0x(s string) string {
	if len(s) >= 2 && (s[0:2] == "0x" || s[0:2] == "0X") {
		return s[2:]
	}
	return s
}

func mulShift128(a, b *big.Int) *big.Int {
	// floor(a*b / 2^128)
	prod := new(big.Int).Mul(a, b)
	return prod.Rsh(prod, 128)
}

// SqrtRatioAtTick returns sqrtPriceX96 = sqrt(1.0001^tick) * 2^96 (rounded up),
// equivalent to Uniswap V3 TickMath.getSqrtRatioAtTick.
func SqrtRatioAtTick(tick int32) (*big.Int, error) {
	if tick < uniswapMinTick || tick > uniswapMaxTick {
		return nil, fmt.Errorf("tick out of range: %d", tick)
	}

	absTick := tick
	if absTick < 0 {
		absTick = -absTick
	}

	ratio := new(big.Int).Lsh(big.NewInt(1), 128) // 1<<128
	for _, c := range tickMulConsts {
		if absTick&c.mask != 0 {
			ratio = mulShift128(ratio, c.value)
		}
	}

	if tick > 0 {
		ratio = new(big.Int).Div(maxUint256, ratio)
	}

	// (ratio >> 32) + (ratio % 2^32 == 0 ? 0 : 1)
	sqrtPriceX96 := new(big.Int).Rsh(ratio, 32)
	lowMask := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 32), big.NewInt(1))
	if new(big.Int).And(ratio, lowMask).Sign() != 0 {
		sqrtPriceX96.Add(sqrtPriceX96, big.NewInt(1))
	}
	return sqrtPriceX96, nil
}

func mulDivFloor(a, b, denom *big.Int) *big.Int {
	if denom.Sign() == 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Div(new(big.Int).Mul(a, b), denom)
}

func liquidityForAmount0(sqrtA, sqrtB, amount0 *big.Int) *big.Int {
	if sqrtA.Cmp(sqrtB) > 0 {
		sqrtA, sqrtB = sqrtB, sqrtA
	}
	intermediate := mulDivFloor(sqrtA, sqrtB, q96)
	return mulDivFloor(amount0, intermediate, new(big.Int).Sub(sqrtB, sqrtA))
}

func liquidityForAmount1(sqrtA, sqrtB, amount1 *big.Int) *big.Int {
	if sqrtA.Cmp(sqrtB) > 0 {
		sqrtA, sqrtB = sqrtB, sqrtA
	}
	return mulDivFloor(amount1, q96, new(big.Int).Sub(sqrtB, sqrtA))
}

func amount0ForLiquidity(sqrtA, sqrtB, liquidity *big.Int) *big.Int {
	if sqrtA.Cmp(sqrtB) > 0 {
		sqrtA, sqrtB = sqrtB, sqrtA
	}
	if liquidity == nil || liquidity.Sign() == 0 {
		return big.NewInt(0)
	}
	if sqrtA.Sign() == 0 || sqrtB.Sign() == 0 {
		return big.NewInt(0)
	}
	numerator1 := new(big.Int).Lsh(new(big.Int).Set(liquidity), 96)
	numerator2 := new(big.Int).Sub(new(big.Int).Set(sqrtB), sqrtA)
	numerator := new(big.Int).Mul(numerator1, numerator2)
	denom := new(big.Int).Mul(new(big.Int).Set(sqrtB), sqrtA)
	if denom.Sign() == 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Div(numerator, denom)
}

func amount1ForLiquidity(sqrtA, sqrtB, liquidity *big.Int) *big.Int {
	if sqrtA.Cmp(sqrtB) > 0 {
		sqrtA, sqrtB = sqrtB, sqrtA
	}
	if liquidity == nil || liquidity.Sign() == 0 {
		return big.NewInt(0)
	}
	numerator := new(big.Int).Mul(new(big.Int).Set(liquidity), new(big.Int).Sub(new(big.Int).Set(sqrtB), sqrtA))
	if q96.Sign() == 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Div(numerator, q96)
}

// LiquidityForAmounts returns the maximum liquidity for given token amounts, equivalent to LiquidityAmounts.getLiquidityForAmounts.
func LiquidityForAmounts(sqrtP, sqrtA, sqrtB, amount0, amount1 *big.Int) *big.Int {
	if sqrtA.Cmp(sqrtB) > 0 {
		sqrtA, sqrtB = sqrtB, sqrtA
	}
	if sqrtP.Cmp(sqrtA) <= 0 {
		return liquidityForAmount0(sqrtA, sqrtB, amount0)
	}
	if sqrtP.Cmp(sqrtB) < 0 {
		l0 := liquidityForAmount0(sqrtP, sqrtB, amount0)
		l1 := liquidityForAmount1(sqrtA, sqrtP, amount1)
		if l0.Cmp(l1) < 0 {
			return l0
		}
		return l1
	}
	return liquidityForAmount1(sqrtA, sqrtB, amount1)
}

// AmountsForLiquidity returns token0/token1 amounts for given liquidity, equivalent to LiquidityAmounts.getAmountsForLiquidity.
func AmountsForLiquidity(sqrtP, sqrtA, sqrtB, liquidity *big.Int) (*big.Int, *big.Int) {
	if sqrtA.Cmp(sqrtB) > 0 {
		sqrtA, sqrtB = sqrtB, sqrtA
	}
	if liquidity == nil || liquidity.Sign() == 0 {
		return big.NewInt(0), big.NewInt(0)
	}

	switch {
	case sqrtP.Cmp(sqrtA) <= 0:
		return amount0ForLiquidity(sqrtA, sqrtB, liquidity), big.NewInt(0)
	case sqrtP.Cmp(sqrtB) < 0:
		a0 := amount0ForLiquidity(sqrtP, sqrtB, liquidity)
		a1 := amount1ForLiquidity(sqrtA, sqrtP, liquidity)
		return a0, a1
	default:
		return big.NewInt(0), amount1ForLiquidity(sqrtA, sqrtB, liquidity)
	}
}
