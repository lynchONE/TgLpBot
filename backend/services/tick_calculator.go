package services

import (
	"fmt"
	"math"
	"math/big"
)

// TickCalculator provides utilities for tick calculations
type TickCalculator struct{}

// NewTickCalculator creates a new tick calculator
func NewTickCalculator() *TickCalculator {
	return &TickCalculator{}
}

// CalculateTickFromPercentage calculates tick range based on percentage from current tick
// percentage: 价格范围百分比 (如 5 表示 ±5%)
// currentTick: 当前池子的 tick
// tickSpacing: 池子的 tick spacing
// Returns: tickLower, tickUpper
func (tc *TickCalculator) CalculateTickFromPercentage(currentTick int, percentage float64, tickSpacing int) (int, int) {
	// 计算价格变化对应的 tick 变化
	// price = 1.0001^tick
	// 对于 x% 的价格变化，tick 偏移量 = log(1 + x/100) / log(1.0001)

	// 使用对称算法：取上涨和下跌偏移量的平均值作为对称偏移
	priceMultiplierUpper := 1.0 + (percentage / 100.0)
	priceMultiplierLower := 1.0 - (percentage / 100.0)

	tickOffsetUpper := math.Abs(math.Log(priceMultiplierUpper) / math.Log(1.0001))
	tickOffsetLower := math.Abs(math.Log(priceMultiplierLower) / math.Log(1.0001))

	// 取平均值作为对称偏移量
	symmetricOffset := int((tickOffsetUpper + tickOffsetLower) / 2)

	// 确保偏移量至少是一个 tickSpacing
	if symmetricOffset < tickSpacing {
		symmetricOffset = tickSpacing
	}

	// 计算对称的 tick 范围
	tickUpper := currentTick + symmetricOffset
	tickLower := currentTick - symmetricOffset

	// 调整到 tick spacing 的倍数
	// 为了保持对称，两边都使用四舍五入到最近的 tickSpacing 倍数
	tickLower = tc.RoundToNearestTickSpacing(tickLower, tickSpacing)
	tickUpper = tc.RoundToNearestTickSpacing(tickUpper, tickSpacing)

	// 额外保护：确保 tickUpper > currentTick 且 tickLower < currentTick
	if tickUpper <= currentTick {
		tickUpper = tc.RoundUpToTickSpacing(currentTick+1, tickSpacing)
	}
	if tickLower >= currentTick {
		tickLower = tc.RoundDownToTickSpacing(currentTick-1, tickSpacing)
	}

	return tickLower, tickUpper
}

// CalculateTickFromPercentages calculates an ASYMMETRIC tick range based on separate lower/upper percentages.
// lowerPct/upperPct: 价格范围百分比 (如 5 表示 5%)
// Returns: tickLower, tickUpper
func (tc *TickCalculator) CalculateTickFromPercentages(currentTick int, lowerPct float64, upperPct float64, tickSpacing int) (int, int) {
	priceMultiplierUpper := 1.0 + (upperPct / 100.0)
	priceMultiplierLower := 1.0 - (lowerPct / 100.0)

	tickOffsetUpper := math.Abs(math.Log(priceMultiplierUpper) / math.Log(1.0001))
	tickOffsetLower := math.Abs(math.Log(priceMultiplierLower) / math.Log(1.0001))

	upperOffset := int(tickOffsetUpper)
	lowerOffset := int(tickOffsetLower)

	if upperOffset < tickSpacing {
		upperOffset = tickSpacing
	}
	if lowerOffset < tickSpacing {
		lowerOffset = tickSpacing
	}

	tickUpper := tc.RoundUpToTickSpacing(currentTick+upperOffset, tickSpacing)
	tickLower := tc.RoundDownToTickSpacing(currentTick-lowerOffset, tickSpacing)

	if tickUpper <= currentTick {
		tickUpper = tc.RoundUpToTickSpacing(currentTick+1, tickSpacing)
	}
	if tickLower >= currentTick {
		tickLower = tc.RoundDownToTickSpacing(currentTick-1, tickSpacing)
	}

	return tickLower, tickUpper
}

// RoundToNearestTickSpacing rounds a tick to the NEAREST valid tick spacing (四舍五入)
func (tc *TickCalculator) RoundToNearestTickSpacing(tick int, tickSpacing int) int {
	remainder := tick % tickSpacing
	if remainder == 0 {
		return tick
	}

	// 计算向下取整和向上取整的结果
	down := tick - remainder
	up := down + tickSpacing
	if tick < 0 {
		down = tick - remainder - tickSpacing
		up = tick - remainder
	}

	// 选择更接近的那个
	if math.Abs(float64(tick-down)) <= math.Abs(float64(tick-up)) {
		return down
	}
	return up
}

// RoundDownToTickSpacing rounds a tick DOWN to the nearest valid tick spacing
func (tc *TickCalculator) RoundDownToTickSpacing(tick int, tickSpacing int) int {
	remainder := tick % tickSpacing
	if remainder == 0 {
		return tick
	}
	// 如果是负数,需要特殊处理
	if tick < 0 {
		return tick - remainder - tickSpacing
	}
	return tick - remainder
}

// RoundUpToTickSpacing rounds a tick UP to the nearest valid tick spacing
func (tc *TickCalculator) RoundUpToTickSpacing(tick int, tickSpacing int) int {
	remainder := tick % tickSpacing
	if remainder == 0 {
		return tick
	}
	// 如果是负数,需要特殊处理
	if tick < 0 {
		return tick - remainder
	}
	return tick - remainder + tickSpacing
}

// RoundToTickSpacing rounds a tick to the nearest valid tick spacing (向下取整，保留用于兼容)
func (tc *TickCalculator) RoundToTickSpacing(tick int, tickSpacing int) int {
	return tc.RoundDownToTickSpacing(tick, tickSpacing)
}

// CalculatePriceFromTick calculates price from tick
// price = 1.0001^tick
func (tc *TickCalculator) CalculatePriceFromTick(tick int) float64 {
	return math.Pow(1.0001, float64(tick))
}

// CalculateTickFromPrice calculates tick from price
// tick = log(price) / log(1.0001)
func (tc *TickCalculator) CalculateTickFromPrice(price float64) int {
	if price <= 0 {
		return 0
	}
	return int(math.Log(price) / math.Log(1.0001))
}

// GetCurrentTickFromSqrtPriceX96 extracts current tick from sqrtPriceX96
// This is useful when we have slot0 data
func (tc *TickCalculator) GetCurrentTickFromSqrtPriceX96(sqrtPriceX96 *big.Int) int {
	// sqrtPriceX96 = sqrt(price) * 2^96
	// price = (sqrtPriceX96 / 2^96)^2
	// tick = log(price) / log(1.0001)

	// Convert to float for calculation
	sqrtPriceFloat := new(big.Float).SetInt(sqrtPriceX96)
	divisor := new(big.Float).SetInt(new(big.Int).Lsh(big.NewInt(1), 96)) // 2^96
	sqrtPriceFloat.Quo(sqrtPriceFloat, divisor)

	// Square to get price
	price := new(big.Float).Mul(sqrtPriceFloat, sqrtPriceFloat)
	priceFloat64, _ := price.Float64()

	return tc.CalculateTickFromPrice(priceFloat64)
}

// FormatTickRange formats tick range for display
func (tc *TickCalculator) FormatTickRange(tickLower, tickUpper int) string {
	return fmt.Sprintf("%d 到 %d", tickLower, tickUpper)
}

// ValidateTickRange validates if tick range is valid
func (tc *TickCalculator) ValidateTickRange(tickLower, tickUpper, tickSpacing int) error {
	if tickLower >= tickUpper {
		return fmt.Errorf("下限 tick 必须小于上限 tick")
	}

	if tickLower%tickSpacing != 0 {
		return fmt.Errorf("下限 tick (%d) 必须是 tick spacing (%d) 的倍数", tickLower, tickSpacing)
	}

	if tickUpper%tickSpacing != 0 {
		return fmt.Errorf("上限 tick (%d) 必须是 tick spacing (%d) 的倍数", tickUpper, tickSpacing)
	}

	// Uniswap V3/V4 tick 范围限制
	const maxTick = 887272
	const minTick = -887272

	if tickLower < minTick || tickUpper > maxTick {
		return fmt.Errorf("tick 范围必须在 %d 到 %d 之间", minTick, maxTick)
	}

	return nil
}
