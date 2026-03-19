package pool

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// PoolInfo represents information about a liquidity pool.
type PoolInfo struct {
	Address      string
	Exchange     string
	Token0       string
	Token1       string
	Token0Symbol string
	Token1Symbol string
	Fee          int
	TickSpacing  int
	CurrentTick  int // kept for backward compatibility; not queried anymore
	HooksAddress string
}

// PoolService handles pool-related operations.
type PoolService struct {
	// Pure on-chain reads (no external API dependency).
}

// NewPoolService creates a new pool service.
func NewPoolService() *PoolService {
	return &PoolService{}
}

func poolDebugEnabled() bool {
	v := strings.TrimSpace(os.Getenv("POOL_DEBUG"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func poolDebugf(format string, args ...interface{}) {
	if poolDebugEnabled() {
		log.Printf("[PoolService] "+format, args...)
	}
}

// GetPoolInfo retrieves information about a V3 pool from blockchain (default chain: bsc).
func (s *PoolService) GetPoolInfo(poolAddress string) (*PoolInfo, error) {
	return s.GetPoolInfoForChain("bsc", poolAddress)
}

// GetPoolInfoForChain retrieves information about a V3 pool from blockchain for a given chain.
func (s *PoolService) GetPoolInfoForChain(chain string, poolAddress string) (*PoolInfo, error) {
	poolAddress = strings.TrimSpace(poolAddress)
	if !strings.HasPrefix(poolAddress, "0x") && !strings.HasPrefix(poolAddress, "0X") {
		poolAddress = "0x" + poolAddress
	}
	chain = config.NormalizeChain(chain)

	poolDebugf("GetPoolInfo called: chain=%s pool=%s", chain, poolAddress)
	return s.getPoolInfoFromChain(chain, poolAddress)
}

// GetV4PoolInfo retrieves information about a Uniswap V4 pool using PoolId (legacy default chain).
// NOTE: V4 reads still use the legacy default client until v4_pool.go is refactored to be multi-chain.
func (s *PoolService) GetV4PoolInfo(poolId string) (*PoolInfo, error) {
	// Normalize PoolId
	poolId = strings.TrimSpace(poolId)
	if !strings.HasPrefix(poolId, "0x") && !strings.HasPrefix(poolId, "0X") {
		poolId = "0x" + poolId
	}

	poolDebugf("GetV4PoolInfo called for: %s", poolId)

	if blockchain.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}

	if config.AppConfig == nil || !common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		return nil, fmt.Errorf("V4 PositionManager address not configured")
	}

	// Read PoolKey from PositionManager.poolKeys(bytes25(poolId)).
	posm := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
	c0, c1, fee, tickSpacing, hooks, err := blockchain.GetUniswapV4PoolKeyFromPositionManager(posm, poolId)
	if err != nil {
		return nil, fmt.Errorf("read V4 PoolKey failed: %w", err)
	}

	// Token symbols
	token0Symbol, err := blockchain.GetTokenSymbol(c0)
	if err != nil {
		log.Printf("[PoolService] Warning: could not get token0 symbol: %v", err)
		token0Symbol = "UNKNOWN"
	}
	token1Symbol, err := blockchain.GetTokenSymbol(c1)
	if err != nil {
		log.Printf("[PoolService] Warning: could not get token1 symbol: %v", err)
		token1Symbol = "UNKNOWN"
	}

	poolDebugf("V4 Pool info retrieved from chain: Uniswap V4 %s/%s (fee: %d)", token0Symbol, token1Symbol, fee)

	return &PoolInfo{
		Address:      poolId,
		Exchange:     "Uniswap V4",
		Token0:       c0.Hex(),
		Token1:       c1.Hex(),
		Token0Symbol: token0Symbol,
		Token1Symbol: token1Symbol,
		Fee:          int(fee),
		TickSpacing:  tickSpacing,
		CurrentTick:  0,
		HooksAddress: hooks.Hex(),
	}, nil
}

// calculateTickSpacing calculates tick spacing based on fee tier.
func (s *PoolService) calculateTickSpacing(fee int) int {
	switch fee {
	case 100:
		return 1
	case 500:
		return 10
	case 2500:
		return 50
	case 3000:
		return 60
	case 10000:
		return 200
	default:
		return 60
	}
}

// getPoolInfoFromChain reads a V3 pool directly from chain.
func (s *PoolService) getPoolInfoFromChain(chain string, poolAddress string) (*PoolInfo, error) {
	chain = config.NormalizeChain(chain)
	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return nil, err
	}
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok {
		return nil, fmt.Errorf("chain config not found: %s", chain)
	}

	poolAddr := common.HexToAddress(poolAddress)
	if !common.IsHexAddress(poolAddress) {
		return nil, fmt.Errorf("invalid pool address: %s", poolAddress)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code, err := client.CodeAt(ctx, poolAddr, nil)
	if err != nil {
		return nil, fmt.Errorf("read pool bytecode failed (chain=%s pool=%s): %w", chain, poolAddress, err)
	}
	if len(code) == 0 {
		return nil, fmt.Errorf("pool address has no contract code on chain=%s: %s", chain, poolAddress)
	}

	// 1) token0/token1
	token0, token1, err := blockchain.GetV3PoolTokensWithClient(client, poolAddr)
	if err != nil {
		return nil, fmt.Errorf("read pool tokens failed: %w", err)
	}

	// 2) fee
	fee, err := blockchain.GetV3PoolFeeWithClient(client, poolAddr)
	if err != nil {
		return nil, fmt.Errorf("read pool fee failed: %w", err)
	}

	// 3) token symbols
	token0Symbol, err := blockchain.GetTokenSymbolWithClient(client, token0)
	if err != nil {
		log.Printf("[PoolService] Warning: could not get token0 symbol: %v", err)
		token0Symbol = "UNKNOWN"
	}

	token1Symbol, err := blockchain.GetTokenSymbolWithClient(client, token1)
	if err != nil {
		log.Printf("[PoolService] Warning: could not get token1 symbol: %v", err)
		token1Symbol = "UNKNOWN"
	}

	// 4) exchange by factory()
	exchange := "V3 Pool"
	factoryAddr, err := blockchain.GetV3PoolFactoryWithClient(client, poolAddr)
	if err != nil {
		log.Printf("[PoolService] Warning: read pool factory failed: %v", err)
	} else {
		exchange = s.determineExchangeFromFactory(factoryAddr, cc)
	}

	tickSpacing := s.calculateTickSpacing(int(fee))
	poolDebugf("Pool info retrieved from chain: %s %s/%s (fee: %d)", exchange, token0Symbol, token1Symbol, fee)

	return &PoolInfo{
		Address:      poolAddress,
		Exchange:     exchange,
		Token0:       token0.Hex(),
		Token1:       token1.Hex(),
		Token0Symbol: token0Symbol,
		Token1Symbol: token1Symbol,
		Fee:          int(fee),
		TickSpacing:  tickSpacing,
		CurrentTick:  0,
		HooksAddress: "0x0000000000000000000000000000000000000000",
	}, nil
}

func (s *PoolService) determineExchangeFromFactory(factoryAddr common.Address, cc config.ChainConfig) string {
	factoryHex := strings.ToLower(strings.TrimSpace(factoryAddr.Hex()))
	for _, dep := range cc.V3Deployments {
		want := strings.ToLower(strings.TrimSpace(dep.FactoryAddress))
		if !common.IsHexAddress(want) {
			continue
		}
		if factoryHex == strings.ToLower(want) {
			name := strings.TrimSpace(dep.Name)
			if name == "" {
				return "V3 Pool"
			}
			return name
		}
	}
	return "V3 Pool"
}
