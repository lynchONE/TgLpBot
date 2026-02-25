package strategy

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func resolveV3PoolAddress(chain string, ctx context.Context, callTimeout time.Duration, npm common.Address, token0 common.Address, token1 common.Address, fee uint64) (common.Address, error) {
	chain = config.NormalizeChain(chain)
	if ctx == nil {
		ctx = context.Background()
	}
	if callTimeout <= 0 {
		callTimeout = 15 * time.Second
	}

	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return common.Address{}, err
	}

	var factories []common.Address
	seen := make(map[common.Address]struct{}, 8)
	addFactory := func(addr string) {
		addr = strings.TrimSpace(addr)
		if !common.IsHexAddress(addr) {
			return
		}
		a := common.HexToAddress(addr)
		if a == (common.Address{}) {
			return
		}
		if _, ok := seen[a]; ok {
			return
		}
		seen[a] = struct{}{}
		factories = append(factories, a)
	}

	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
			// Prefer the deployment matching this PositionManager (if configured).
			for _, dep := range cc.V3Deployments {
				if !common.IsHexAddress(dep.FactoryAddress) {
					continue
				}
				if common.IsHexAddress(dep.PositionManagerAddress) && npm == common.HexToAddress(dep.PositionManagerAddress) {
					addFactory(dep.FactoryAddress)
				}
			}
			// Then try all known factories for this chain.
			for _, dep := range cc.V3Deployments {
				addFactory(dep.FactoryAddress)
			}
		}
	}

	// Legacy fallback for BSC deployments if chain config is missing.
	if len(factories) == 0 && chain == "bsc" {
		addFactory("0x0BFbcf9fa4f9C56B0F40a671Ad40E0805A091865") // PancakeSwap V3
		addFactory("0xdB1d10011AD0Ff90774D0C6Bb92e5C5c8b4461F7") // Uniswap V3
	}

	for _, factory := range factories {
		callCtx, cancel := context.WithTimeout(ctx, callTimeout)
		pool, err := blockchain.GetV3PoolFromFactoryCtxWithClient(client, callCtx, factory, token0, token1, fee)
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
