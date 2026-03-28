package pool

import (
	"TgLpBot/base/blockchain"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

func CalcV3UnclaimedFeesFromGrowths(
	currentTick int,
	pos *blockchain.V3PositionInfo,
	global0, global1 *big.Int,
	lower0, lower1 *big.Int,
	upper0, upper1 *big.Int,
) (*big.Int, *big.Int, error) {
	if pos == nil {
		return big.NewInt(0), big.NewInt(0), fmt.Errorf("position info missing")
	}

	owed0 := cloneBig(pos.TokensOwed0)
	owed1 := cloneBig(pos.TokensOwed1)

	if pos.Liquidity == nil || pos.Liquidity.Sign() == 0 {
		return owed0, owed1, nil
	}
	if pos.FeeGrowthInside0LastX128 == nil || pos.FeeGrowthInside1LastX128 == nil {
		return owed0, owed1, fmt.Errorf("position feeGrowthInside last missing")
	}
	if global0 == nil || global1 == nil || lower0 == nil || lower1 == nil || upper0 == nil || upper1 == nil {
		return owed0, owed1, fmt.Errorf("fee growth inputs missing")
	}

	inside0 := feeGrowthInside(currentTick, pos.TickLower, pos.TickUpper, global0, lower0, upper0)
	inside1 := feeGrowthInside(currentTick, pos.TickLower, pos.TickUpper, global1, lower1, upper1)

	last0 := cloneBig(pos.FeeGrowthInside0LastX128)
	last1 := cloneBig(pos.FeeGrowthInside1LastX128)
	// Uniswap V3 uses unchecked uint256 subtraction here. Both feeGrowthInside
	// and feeGrowthInsideLastX128 can wrap modulo 2^256, so current < last is valid.
	delta0 := subMod256(inside0, last0)
	delta1 := subMod256(inside1, last1)

	extra0 := mulDivFloor(delta0, pos.Liquidity, q128)
	extra1 := mulDivFloor(delta1, pos.Liquidity, q128)
	owed0.Add(owed0, extra0)
	owed1.Add(owed1, extra1)
	return owed0, owed1, nil
}

func CalcV3UnclaimedFeesAtBlock(poolAddr common.Address, currentTick int, pos *blockchain.V3PositionInfo, blockNumber uint64) (*big.Int, *big.Int, error) {
	if pos == nil {
		return big.NewInt(0), big.NewInt(0), fmt.Errorf("position info missing")
	}

	owed0 := cloneBig(pos.TokensOwed0)
	owed1 := cloneBig(pos.TokensOwed1)

	if pos.Liquidity == nil || pos.Liquidity.Sign() == 0 {
		return owed0, owed1, nil
	}
	if poolAddr == (common.Address{}) {
		return owed0, owed1, fmt.Errorf("pool address missing")
	}

	global0, global1, err := blockchain.GetV3PoolFeeGrowthGlobalsAtBlock(poolAddr, blockNumber)
	if err != nil {
		return owed0, owed1, fmt.Errorf("read feeGrowthGlobal failed: %w", err)
	}
	lower0, lower1, _, err := blockchain.GetV3PoolTickFeeGrowthOutsideAtBlock(poolAddr, pos.TickLower, blockNumber)
	if err != nil {
		return owed0, owed1, fmt.Errorf("read tickLower feeGrowthOutside failed: %w", err)
	}
	upper0, upper1, _, err := blockchain.GetV3PoolTickFeeGrowthOutsideAtBlock(poolAddr, pos.TickUpper, blockNumber)
	if err != nil {
		return owed0, owed1, fmt.Errorf("read tickUpper feeGrowthOutside failed: %w", err)
	}

	return CalcV3UnclaimedFeesFromGrowths(currentTick, pos, global0, global1, lower0, lower1, upper0, upper1)
}

func CalcV3UnclaimedFees(poolAddr common.Address, currentTick int, pos *blockchain.V3PositionInfo) (*big.Int, *big.Int, error) {
	if pos == nil {
		return big.NewInt(0), big.NewInt(0), fmt.Errorf("position info missing")
	}

	owed0 := cloneBig(pos.TokensOwed0)
	owed1 := cloneBig(pos.TokensOwed1)

	if pos.Liquidity == nil || pos.Liquidity.Sign() == 0 {
		return owed0, owed1, nil
	}
	if poolAddr == (common.Address{}) {
		return owed0, owed1, fmt.Errorf("pool address missing")
	}
	if pos.FeeGrowthInside0LastX128 == nil || pos.FeeGrowthInside1LastX128 == nil {
		return owed0, owed1, fmt.Errorf("position feeGrowthInside last missing")
	}

	global0, global1, err := blockchain.GetV3PoolFeeGrowthGlobals(poolAddr)
	if err != nil {
		return owed0, owed1, fmt.Errorf("read feeGrowthGlobal failed: %w", err)
	}
	lower0, lower1, _, err := blockchain.GetV3PoolTickFeeGrowthOutside(poolAddr, pos.TickLower)
	if err != nil {
		return owed0, owed1, fmt.Errorf("read tickLower feeGrowthOutside failed: %w", err)
	}
	upper0, upper1, _, err := blockchain.GetV3PoolTickFeeGrowthOutside(poolAddr, pos.TickUpper)
	if err != nil {
		return owed0, owed1, fmt.Errorf("read tickUpper feeGrowthOutside failed: %w", err)
	}

	return CalcV3UnclaimedFeesFromGrowths(currentTick, pos, global0, global1, lower0, lower1, upper0, upper1)
}

func feeGrowthInside(currentTick, tickLower, tickUpper int, global, outsideLower, outsideUpper *big.Int) *big.Int {
	feeGlobal := cloneBig(global)
	lower := cloneBig(outsideLower)
	upper := cloneBig(outsideUpper)

	// Uniswap V3 fee growth inside:
	// feeGrowthBelow = (tickCurrent >= tickLower) ? feeGrowthOutsideLower : feeGrowthGlobal - feeGrowthOutsideLower
	// feeGrowthAbove = (tickCurrent < tickUpper) ? feeGrowthOutsideUpper : feeGrowthGlobal - feeGrowthOutsideUpper
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

var modUint256 = new(big.Int).Add(new(big.Int).Set(maxUint256), big.NewInt(1))
