package web_server

import (
	"math"
	"strings"
)

const (
	hotPoolV4DynamicFeeFlag = 0x800000
	hotPoolMaxStaticFeeTier = 1000000
)

func normalizeHotPoolFee(protocolVersion string, feeTier int, feePercentage float64, dynamic bool) (int, float64, bool) {
	if dynamic || isHotPoolDynamicFee(protocolVersion, feeTier, feePercentage) {
		return 0, normalizeHotPoolFeePercentage(feePercentage), true
	}
	if feeTier < 0 || feeTier > hotPoolMaxStaticFeeTier {
		feeTier = 0
	}
	return feeTier, normalizeHotPoolFeePercentage(feePercentage), false
}

func isHotPoolDynamicFee(protocolVersion string, feeTier int, feePercentage float64) bool {
	if !strings.EqualFold(strings.TrimSpace(protocolVersion), "v4") {
		return false
	}
	if feeTier&hotPoolV4DynamicFeeFlag != 0 {
		return true
	}
	return feePercentage > 100
}

func normalizeHotPoolFeePercentage(feePercentage float64) float64 {
	if math.IsNaN(feePercentage) || math.IsInf(feePercentage, 0) || feePercentage < 0 || feePercentage > 100 {
		return 0
	}
	return feePercentage
}
