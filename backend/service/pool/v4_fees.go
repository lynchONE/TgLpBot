package pool

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// CalcV4UnclaimedFees 计算 V4 仓位的实时未领手续费
// 返回 (fees0, fees1, error)
// 注意：V4 的手续费计算与 V3 类似，但需要通过 StateView 合约获取池子的 feeGrowth 数据
func CalcV4UnclaimedFees(poolID string, currentTick int, pos *blockchain.V4PositionInfo) (*big.Int, *big.Int, error) {
	if pos == nil {
		return big.NewInt(0), big.NewInt(0), fmt.Errorf("position info missing")
	}

	owed0 := cloneBig(pos.TokensOwed0)
	owed1 := cloneBig(pos.TokensOwed1)

	// 如果没有流动性，只返回已记录的 owed 手续费
	if pos.Liquidity == nil || pos.Liquidity.Sign() == 0 {
		return owed0, owed1, nil
	}
	if pos.FeeGrowthInside0LastX128 == nil || pos.FeeGrowthInside1LastX128 == nil {
		return owed0, owed1, fmt.Errorf("position feeGrowthInside last missing")
	}

	// 检查配置
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

	// 获取池子的全局 feeGrowth
	global0, global1, err := blockchain.GetV4PoolFeeGrowthGlobals(stateView, poolManager, poolID)
	if err != nil {
		log.Printf("[V4Fees] 获取 feeGrowthGlobal 失败: %v", err)
		return owed0, owed1, fmt.Errorf("read V4 feeGrowthGlobal failed: %w", err)
	}

	// 获取 tickLower 和 tickUpper 的 feeGrowthOutside
	lower0, lower1, err := blockchain.GetV4TickFeeGrowthOutside(stateView, poolManager, poolID, pos.TickLower)
	if err != nil {
		log.Printf("[V4Fees] 获取 tickLower feeGrowthOutside 失败: %v", err)
		return owed0, owed1, fmt.Errorf("read V4 tickLower feeGrowthOutside failed: %w", err)
	}

	upper0, upper1, err := blockchain.GetV4TickFeeGrowthOutside(stateView, poolManager, poolID, pos.TickUpper)
	if err != nil {
		log.Printf("[V4Fees] 获取 tickUpper feeGrowthOutside 失败: %v", err)
		return owed0, owed1, fmt.Errorf("read V4 tickUpper feeGrowthOutside failed: %w", err)
	}

	// 计算 feeGrowthInside
	inside0 := feeGrowthInside(currentTick, pos.TickLower, pos.TickUpper, global0, lower0, upper0)
	inside1 := feeGrowthInside(currentTick, pos.TickLower, pos.TickUpper, global1, lower1, upper1)
	if inside0.Cmp(global0) > 0 || inside1.Cmp(global1) > 0 {
		return owed0, owed1, fmt.Errorf("invalid feeGrowthInside (pool_id=%s)", poolID)
	}

	// 计算增量
	last0 := cloneBig(pos.FeeGrowthInside0LastX128)
	last1 := cloneBig(pos.FeeGrowthInside1LastX128)

	delta0 := new(big.Int).Sub(inside0, last0)
	if delta0.Sign() < 0 {
		delta0 = big.NewInt(0)
	}
	delta1 := new(big.Int).Sub(inside1, last1)
	if delta1.Sign() < 0 {
		delta1 = big.NewInt(0)
	}

	// 计算实际手续费：delta * liquidity / 2^128
	extra0 := mulDivFloor(delta0, pos.Liquidity, q128)
	extra1 := mulDivFloor(delta1, pos.Liquidity, q128)

	owed0.Add(owed0, extra0)
	owed1.Add(owed1, extra1)

	log.Printf("[V4Fees] 手续费计算: tokenId liquidity=%s extra0=%s extra1=%s owed0=%s owed1=%s",
		pos.Liquidity.String(), extra0.String(), extra1.String(), owed0.String(), owed1.String())

	return owed0, owed1, nil
}
