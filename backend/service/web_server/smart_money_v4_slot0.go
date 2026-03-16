package web_server

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

var (
	smartMoneyGetV4Slot0ViaStateView = blockchain.GetUniswapV4PoolSlot0ViaStateView
	smartMoneyGetV4Slot0Direct       = blockchain.GetUniswapV4PoolSlot0
)

func loadSmartMoneyV4Slot0(poolManager common.Address, poolID string) (*big.Int, int, error) {
	if poolManager == (common.Address{}) {
		return nil, 0, fmt.Errorf("uniswap v4 pool manager address not configured")
	}

	var stateViewErr error
	if config.AppConfig != nil && common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) {
		stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
		sqrtP, currentTick, err := smartMoneyGetV4Slot0ViaStateView(stateView, poolManager, poolID)
		if err == nil {
			return sqrtP, currentTick, nil
		}
		stateViewErr = err
	}

	sqrtP, currentTick, err := smartMoneyGetV4Slot0Direct(poolManager, poolID)
	if err == nil {
		return sqrtP, currentTick, nil
	}
	if stateViewErr != nil {
		return nil, 0, fmt.Errorf("state view getSlot0 failed: %v; pool manager fallback failed: %w", stateViewErr, err)
	}
	return nil, 0, err
}
