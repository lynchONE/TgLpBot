package pool_sync

import "strings"

const (
	uniswapV4DynamicFeeFlag = 0x800000
	maxStaticPoolFeeTier    = 1000000
)

func normalizePoolSyncFee(protocolVersion string, feeTier int, feePercentage float64) (int, float64, bool) {
	if isPoolSyncDynamicFee(protocolVersion, feeTier, feePercentage) {
		return 0, 0, true
	}
	if feeTier < 0 {
		feeTier = 0
	}
	if feeTier > maxStaticPoolFeeTier {
		feeTier = 0
	}
	if feePercentage < 0 || feePercentage > 100 {
		feePercentage = 0
	}
	return feeTier, feePercentage, false
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
