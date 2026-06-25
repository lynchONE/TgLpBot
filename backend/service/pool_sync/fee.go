package pool_sync

import (
	"math"
	"strings"
)

const (
	uniswapV4DynamicFeeFlag = 0x800000
	maxStaticPoolFeeTier    = 1000000
)

func normalizePoolSyncFee(protocolVersion string, feeTier int, feePercentage float64) (int, float64, bool) {
	if isPoolSyncDynamicFee(protocolVersion, feeTier, feePercentage) {
		return 0, normalizePoolSyncFeePercentage(feePercentage), true
	}
	if feeTier < 0 {
		feeTier = 0
	}
	if feeTier > maxStaticPoolFeeTier {
		feeTier = 0
	}
	return feeTier, normalizePoolSyncFeePercentage(feePercentage), false
}

func isPoolSyncDynamicFee(protocolVersion string, feeTier int, feePercentage float64) bool {
	if !strings.EqualFold(strings.TrimSpace(protocolVersion), "v4") {
		return false
	}
	if feeTier&uniswapV4DynamicFeeFlag != 0 {
		return true
	}
	return feePercentage > 100
}

func normalizePoolSyncFeePercentage(feePercentage float64) float64 {
	if math.IsNaN(feePercentage) || math.IsInf(feePercentage, 0) || feePercentage < 0 || feePercentage > 100 {
		return 0
	}
	return feePercentage
}
