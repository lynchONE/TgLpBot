package pool

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// CalcV4UnclaimedFeesFromGrowths computes V4 unclaimed fees from already-fetched fee growth values.
// This math must stay aligned across realtime positions, strategy PnL and smart money lookups.
func CalcV4UnclaimedFeesFromGrowths(
	currentTick int,
	pos *blockchain.V4PositionInfo,
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
	if inside0.Cmp(global0) > 0 || inside1.Cmp(global1) > 0 {
		return owed0, owed1, fmt.Errorf(
			"inconsistent V4 fee snapshot: inside exceeds global inside0=%s global0=%s inside1=%s global1=%s",
			inside0.String(),
			global0.String(),
			inside1.String(),
			global1.String(),
		)
	}

	delta0 := new(big.Int).Sub(inside0, last0)
	delta1 := new(big.Int).Sub(inside1, last1)
	if delta0.Sign() < 0 || delta1.Sign() < 0 {
		return owed0, owed1, fmt.Errorf(
			"inconsistent V4 fee snapshot: inside0=%s last0=%s inside1=%s last1=%s",
			inside0.String(),
			last0.String(),
			inside1.String(),
			last1.String(),
		)
	}

	extra0 := mulDivFloor(delta0, pos.Liquidity, q128)
	extra1 := mulDivFloor(delta1, pos.Liquidity, q128)

	owed0.Add(owed0, extra0)
	owed1.Add(owed1, extra1)
	return owed0, owed1, nil
}

func CalcV4UnclaimedFeesAtBlock(stateView, poolManager common.Address, poolID string, currentTick int, pos *blockchain.V4PositionInfo, blockNumber uint64) (*big.Int, *big.Int, error) {
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
	if stateView == (common.Address{}) {
		return owed0, owed1, fmt.Errorf("V4 StateView address not configured")
	}
	if poolManager == (common.Address{}) {
		return owed0, owed1, fmt.Errorf("V4 PoolManager address not configured")
	}

	global0, global1, err := blockchain.GetV4PoolFeeGrowthGlobalsAtBlock(stateView, poolManager, poolID, blockNumber)
	if err != nil {
		log.Printf("[V4Fees] read feeGrowthGlobal at block=%d failed: %v", blockNumber, err)
		return owed0, owed1, fmt.Errorf("read V4 feeGrowthGlobal failed: %w", err)
	}

	lower0, lower1, err := blockchain.GetV4TickFeeGrowthOutsideAtBlock(stateView, poolManager, poolID, pos.TickLower, blockNumber)
	if err != nil {
		log.Printf("[V4Fees] read tickLower feeGrowthOutside at block=%d failed: %v", blockNumber, err)
		return owed0, owed1, fmt.Errorf("read V4 tickLower feeGrowthOutside failed: %w", err)
	}

	upper0, upper1, err := blockchain.GetV4TickFeeGrowthOutsideAtBlock(stateView, poolManager, poolID, pos.TickUpper, blockNumber)
	if err != nil {
		log.Printf("[V4Fees] read tickUpper feeGrowthOutside at block=%d failed: %v", blockNumber, err)
		return owed0, owed1, fmt.Errorf("read V4 tickUpper feeGrowthOutside failed: %w", err)
	}

	return CalcV4UnclaimedFeesFromGrowths(currentTick, pos, global0, global1, lower0, lower1, upper0, upper1)
}

// CalcV4UnclaimedFees fetches the current V4 fee growth values and computes unclaimed fees.
func CalcV4UnclaimedFees(poolID string, currentTick int, pos *blockchain.V4PositionInfo) (*big.Int, *big.Int, error) {
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

	if config.AppConfig == nil {
		return owed0, owed1, fmt.Errorf("config not loaded")
	}
	if config.AppConfig.UniswapV4StateViewAddress == "" {
		return owed0, owed1, fmt.Errorf("V4 StateView address not configured")
	}
	if config.AppConfig.UniswapV4PoolManagerAddress == "" {
		return owed0, owed1, fmt.Errorf("V4 PoolManager address not configured")
	}

	stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
	poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)

	global0, global1, err := blockchain.GetV4PoolFeeGrowthGlobals(stateView, poolManager, poolID)
	if err != nil {
		log.Printf("[V4Fees] read feeGrowthGlobal failed: %v", err)
		return owed0, owed1, fmt.Errorf("read V4 feeGrowthGlobal failed: %w", err)
	}

	lower0, lower1, err := blockchain.GetV4TickFeeGrowthOutside(stateView, poolManager, poolID, pos.TickLower)
	if err != nil {
		log.Printf("[V4Fees] read tickLower feeGrowthOutside failed: %v", err)
		return owed0, owed1, fmt.Errorf("read V4 tickLower feeGrowthOutside failed: %w", err)
	}

	upper0, upper1, err := blockchain.GetV4TickFeeGrowthOutside(stateView, poolManager, poolID, pos.TickUpper)
	if err != nil {
		log.Printf("[V4Fees] read tickUpper feeGrowthOutside failed: %v", err)
		return owed0, owed1, fmt.Errorf("read V4 tickUpper feeGrowthOutside failed: %w", err)
	}

	fees0, fees1, calcErr := CalcV4UnclaimedFeesFromGrowths(currentTick, pos, global0, global1, lower0, lower1, upper0, upper1)
	if calcErr != nil {
		return owed0, owed1, calcErr
	}

	extra0 := new(big.Int).Sub(cloneBig(fees0), cloneBig(pos.TokensOwed0))
	extra1 := new(big.Int).Sub(cloneBig(fees1), cloneBig(pos.TokensOwed1))
	log.Printf(
		"[V4Fees] computed tokenId fees liquidity=%s extra0=%s extra1=%s owed0=%s owed1=%s",
		pos.Liquidity.String(),
		extra0.String(),
		extra1.String(),
		fees0.String(),
		fees1.String(),
	)

	return fees0, fees1, nil
}
