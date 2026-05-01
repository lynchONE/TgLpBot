package liquidity

import (
	"TgLpBot/base/config"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

func wrappedNativeAddress(cc config.ChainConfig) (common.Address, bool) {
	if !common.IsHexAddress(cc.WrappedNativeAddress) {
		return common.Address{}, false
	}
	addr := common.HexToAddress(cc.WrappedNativeAddress)
	return addr, addr != (common.Address{})
}

func v4CurrencyMatchesFundingToken(cc config.ChainConfig, currency common.Address, token common.Address) bool {
	if currency == token {
		return true
	}
	if currency != (common.Address{}) {
		return false
	}
	wrapped, ok := wrappedNativeAddress(cc)
	return ok && token == wrapped
}

func v4CurrencyFundingToken(cc config.ChainConfig, currency common.Address) (common.Address, error) {
	if currency != (common.Address{}) {
		return currency, nil
	}
	wrapped, ok := wrappedNativeAddress(cc)
	if !ok {
		return common.Address{}, fmt.Errorf("wrapped native address not configured for chain=%s", config.NormalizeChain(cc.Chain))
	}
	return wrapped, nil
}

func poolContainsEntryCandidate(version string, token0 common.Address, token1 common.Address, candidate common.Address, cc config.ChainConfig) bool {
	if candidate == (common.Address{}) {
		return false
	}
	if token0 == candidate || token1 == candidate {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(version), "v4") {
		return v4CurrencyMatchesFundingToken(cc, token0, candidate) ||
			v4CurrencyMatchesFundingToken(cc, token1, candidate)
	}
	return false
}

func poolCurrencyMatchesSwapToken(chain string, currency common.Address, swapToken common.Address) bool {
	if currency == swapToken {
		return true
	}
	if currency != (common.Address{}) || config.AppConfig == nil {
		return false
	}
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok {
		return false
	}
	wrapped, ok := wrappedNativeAddress(cc)
	return ok && swapToken == wrapped
}
