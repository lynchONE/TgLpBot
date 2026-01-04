package realtime

import "math/big"

var (
	q128       = new(big.Int).Lsh(big.NewInt(1), 128)
	maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	modUint256 = new(big.Int).Add(new(big.Int).Set(maxUint256), big.NewInt(1))
)

func cloneBig(v *big.Int) *big.Int {
	if v == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(v)
}

func addMod256(a, b *big.Int) *big.Int {
	sum := new(big.Int).Add(cloneBig(a), cloneBig(b))
	return sum.Mod(sum, modUint256)
}

func subMod256(a, b *big.Int) *big.Int {
	diff := new(big.Int).Sub(cloneBig(a), cloneBig(b))
	return diff.Mod(diff, modUint256)
}

func feeGrowthInside(currentTick, tickLower, tickUpper int, global, outsideLower, outsideUpper *big.Int) *big.Int {
	feeGlobal := cloneBig(global)
	lower := cloneBig(outsideLower)
	upper := cloneBig(outsideUpper)

	var below *big.Int
	if currentTick >= tickLower {
		below = lower
	} else {
		below = subMod256(feeGlobal, lower)
	}

	var above *big.Int
	if currentTick < tickUpper {
		above = upper
	} else {
		above = subMod256(feeGlobal, upper)
	}

	sum := addMod256(below, above)
	return subMod256(feeGlobal, sum)
}

func mulDivFloor(a, b, denom *big.Int) *big.Int {
	if a == nil || b == nil || denom == nil || denom.Sign() <= 0 {
		return big.NewInt(0)
	}
	num := new(big.Int).Mul(a, b)
	return new(big.Int).Div(num, denom)
}
