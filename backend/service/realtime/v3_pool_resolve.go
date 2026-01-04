package realtime

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func resolveV3PoolAddress(ctx context.Context, callTimeout time.Duration, npm common.Address, token0 common.Address, token1 common.Address, fee uint64) (common.Address, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if callTimeout <= 0 {
		callTimeout = 15 * time.Second
	}
	var factories []common.Address
	pancakeFactory := common.HexToAddress("0x0BFbcf9fa4f9C56B0F40a671Ad40E0805A091865")
	uniswapFactory := common.HexToAddress("0xdB1d10011AD0Ff90774D0C6Bb92e5C5c8b4461F7")

	if config.AppConfig != nil {
		if common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) && npm == common.HexToAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
			factories = append(factories, pancakeFactory)
		} else if common.IsHexAddress(config.AppConfig.UniswapV3PositionManagerAddress) && npm == common.HexToAddress(config.AppConfig.UniswapV3PositionManagerAddress) {
			factories = append(factories, uniswapFactory)
		}
	}
	if len(factories) == 0 {
		factories = append(factories, pancakeFactory, uniswapFactory)
	}

	for _, factory := range factories {
		callCtx, cancel := context.WithTimeout(ctx, callTimeout)
		pool, err := blockchain.GetV3PoolFromFactoryCtx(callCtx, factory, token0, token1, fee)
		cancel()
		if err != nil {
			continue
		}
		if pool != (common.Address{}) {
			return pool, nil
		}
	}
	return common.Address{}, fmt.Errorf("v3 pool not found")
}
