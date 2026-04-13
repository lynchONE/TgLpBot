package liquidity

import (
	"math"
	"math/big"
)

const v4PriceMoveToleranceFloorPct = 1.0

func V4PriceMoveTolerancePercent(slippagePercent float64) float64 {
	effective := slippagePercent
	if math.IsNaN(effective) || math.IsInf(effective, 0) || effective <= 0 {
		effective = v4PriceMoveToleranceFloorPct
	}
	if effective < v4PriceMoveToleranceFloorPct {
		effective = v4PriceMoveToleranceFloorPct
	}
	if effective > 100 {
		effective = 100
	}
	return effective
}

func V4PriceMoveToleranceBps(slippagePercent float64) *big.Int {
	bps := int64(math.Round(V4PriceMoveTolerancePercent(slippagePercent) * 100))
	if bps < 0 {
		bps = 0
	}
	if bps > 10000 {
		bps = 10000
	}
	return big.NewInt(bps)
}
