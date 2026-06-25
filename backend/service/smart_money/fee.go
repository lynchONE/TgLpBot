package smart_money

import "strings"

const uniswapV4DynamicFeeFlag = 0x800000

func IsDynamicFeeTier(protocol string, feeTier *int) bool {
	if feeTier == nil {
		return false
	}
	return IsDynamicFeeTierValue(protocol, *feeTier)
}

func IsDynamicFeeTierValue(protocol string, feeTier int) bool {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol != "uniswap_v4" && protocol != "v4" {
		return false
	}
	return feeTier&uniswapV4DynamicFeeFlag != 0
}
