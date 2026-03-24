package pool

import (
	"fmt"
	"math"
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
	return tc.CalculateTickFromPercentages(currentTick, percentage, percentage, tickSpacing)
}

// CalculateTickFromPercentages calculates an ASYMMETRIC tick range based on separate lower/upper percentages.
// lowerPct/upperPct: 价格范围百分比 (如 5 表示 5%)
// Returns: tickLower, tickUpper
func (tc *TickCalculator) CalculateTickFromPercentages(currentTick int, lowerPct float64, upperPct float64, tickSpacing int) (int, int) {
	if tickSpacing <= 0 {
		return currentTick - 1, currentTick + 1
	}

	priceMultiplierUpper := 1.0 + (upperPct / 100.0)
	priceMultiplierLower := 1.0 - (lowerPct / 100.0)

	tickOffsetUpper := math.Abs(math.Log(priceMultiplierUpper) / math.Log(1.0001))
	tickOffsetLower := math.Abs(math.Log(priceMultiplierLower) / math.Log(1.0001))

	upperOffset := int(math.Ceil(tickOffsetUpper))
	lowerOffset := int(math.Ceil(tickOffsetLower))

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

	if normalizedLower, normalizedUpper, err := tc.NormalizeTickRange(tickLower, tickUpper, tickSpacing); err == nil {
		return normalizedLower, normalizedUpper
	}

	return tickLower, tickUpper
}

// CalculateTickFromPercentagesBestFit chooses the closest valid ticks to the target percentages.
// This minimizes distortion when currentTick isn't aligned to tickSpacing.
func (tc *TickCalculator) CalculateTickFromPercentagesBestFit(currentTick int, lowerPct float64, upperPct float64, tickSpacing int) (int, int) {
	if tickSpacing <= 0 || lowerPct <= 0 || upperPct <= 0 {
		return tc.CalculateTickFromPercentages(currentTick, lowerPct, upperPct, tickSpacing)
	}

	priceMultiplierUpper := 1.0 + (upperPct / 100.0)
	priceMultiplierLower := 1.0 - (lowerPct / 100.0)
	if priceMultiplierUpper <= 0 || priceMultiplierLower <= 0 {
		return tc.CalculateTickFromPercentages(currentTick, lowerPct, upperPct, tickSpacing)
	}

	tickOffsetUpper := math.Abs(math.Log(priceMultiplierUpper) / math.Log(1.0001))
	tickOffsetLower := math.Abs(math.Log(priceMultiplierLower) / math.Log(1.0001))
	if tickOffsetUpper < 1 {
		tickOffsetUpper = 1
	}
	if tickOffsetLower < 1 {
		tickOffsetLower = 1
	}

	targetLower := currentTick - int(math.Round(tickOffsetLower))
	targetUpper := currentTick + int(math.Round(tickOffsetUpper))

	lowerCandidates := []int{
		tc.RoundDownToTickSpacing(targetLower, tickSpacing),
		tc.RoundUpToTickSpacing(targetLower, tickSpacing),
	}
	upperCandidates := []int{
		tc.RoundDownToTickSpacing(targetUpper, tickSpacing),
		tc.RoundUpToTickSpacing(targetUpper, tickSpacing),
	}

	bestLower := lowerCandidates[0]
	bestUpper := upperCandidates[0]
	bestScore := math.Inf(1)

	for _, l := range lowerCandidates {
		for _, u := range upperCandidates {
			if l >= u || l >= currentTick || u <= currentTick {
				continue
			}
			effLower, effUpper := tc.CalculatePercentagesFromTicks(currentTick, l, u)
			if effLower <= 0 || effUpper <= 0 {
				continue
			}
			score := math.Abs(effLower-lowerPct) + math.Abs(effUpper-upperPct)
			if lowerPct < upperPct && effLower > effUpper {
				score += 1000
			}
			if lowerPct > upperPct && effLower < effUpper {
				score += 1000
			}
			if score < bestScore {
				bestScore = score
				bestLower = l
				bestUpper = u
			}
		}
	}

	if math.IsInf(bestScore, 1) {
		return tc.CalculateTickFromPercentages(currentTick, lowerPct, upperPct, tickSpacing)
	}

	if normalizedLower, normalizedUpper, err := tc.NormalizeTickRange(bestLower, bestUpper, tickSpacing); err == nil {
		return normalizedLower, normalizedUpper
	}

	return tc.CalculateTickFromPercentages(currentTick, lowerPct, upperPct, tickSpacing)
}

// CalculatePercentagesFromTicks estimates lower/upper percentage widths from a tick range.
func (tc *TickCalculator) CalculatePercentagesFromTicks(currentTick, tickLower, tickUpper int) (float64, float64) {
	price := tc.CalculatePriceFromTick(currentTick)
	if price <= 0 {
		return 0, 0
	}

	lowerPrice := tc.CalculatePriceFromTick(tickLower)
	upperPrice := tc.CalculatePriceFromTick(tickUpper)
	if lowerPrice <= 0 || upperPrice <= 0 {
		return 0, 0
	}

	lowerPct := (1.0 - (lowerPrice / price)) * 100.0
	upperPct := ((upperPrice / price) - 1.0) * 100.0
	if lowerPct < 0 {
		lowerPct = 0
	}
	if upperPct < 0 {
		upperPct = 0
	}
	return lowerPct, upperPct
}

// CalculatePriceFromTick calculates price from tick
// price = 1.0001^tick
func (tc *TickCalculator) CalculatePriceFromTick(tick int) float64 {
	return math.Pow(1.0001, float64(tick))
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

// NormalizeTickRange aligns and clamps ticks to the nearest valid on-chain range.
func (tc *TickCalculator) NormalizeTickRange(tickLower, tickUpper, tickSpacing int) (int, int, error) {
	if tickSpacing <= 0 {
		return 0, 0, fmt.Errorf("invalid tick spacing: %d", tickSpacing)
	}

	minTick, maxTick, err := FullRangeTicks(tickSpacing)
	if err != nil {
		return 0, 0, err
	}

	tickLower = tc.RoundDownToTickSpacing(tickLower, tickSpacing)
	tickUpper = tc.RoundUpToTickSpacing(tickUpper, tickSpacing)

	if tickLower < minTick {
		tickLower = minTick
	}
	if tickUpper > maxTick {
		tickUpper = maxTick
	}

	if tickLower >= tickUpper {
		switch {
		case tickLower < maxTick:
			tickUpper = tickLower + tickSpacing
		case tickUpper > minTick:
			tickLower = tickUpper - tickSpacing
		default:
			return 0, 0, fmt.Errorf("tick range collapsed for spacing %d", tickSpacing)
		}

		if tickLower < minTick {
			tickLower = minTick
		}
		if tickUpper > maxTick {
			tickUpper = maxTick
		}
	}

	if tickLower >= tickUpper {
		return 0, 0, fmt.Errorf("tick range collapsed after normalization")
	}

	return tickLower, tickUpper, nil
}

// ValidateTickRange validates if tick range is valid
func (tc *TickCalculator) ValidateTickRange(tickLower, tickUpper, tickSpacing int) error {
	if tickSpacing <= 0 {
		return fmt.Errorf("invalid tick spacing: %d", tickSpacing)
	}

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
	minTick, maxTick, err := FullRangeTicks(tickSpacing)
	if err != nil {
		return err
	}

	if tickLower < minTick || tickUpper > maxTick {
		return fmt.Errorf("tick 范围必须在 %d 到 %d 之间", minTick, maxTick)
	}

	return nil
}
