package services

import (
	"TgLpBot/blockchain"
	"TgLpBot/config"
	"fmt"
	"log"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// PoolInfo represents information about a liquidity pool
type PoolInfo struct {
	Address      string
	Exchange     string
	Token0       string
	Token1       string
	Token0Symbol string
	Token1Symbol string
	Fee          int
	TickSpacing  int
	CurrentTick  int // 保留字段以兼容现有代码，但不再查询
	HooksAddress string
}

// PoolService handles pool-related operations
type PoolService struct {
	graphAPI *TheGraphAPI
}

// NewPoolService creates a new pool service
func NewPoolService() *PoolService {
	return &PoolService{
		graphAPI: NewTheGraphAPI(),
	}
}

// GetPoolInfo retrieves information about a pool using The Graph API
// Supports PancakeSwap V3, Uniswap V3, and Uniswap V4
func (s *PoolService) GetPoolInfo(poolAddress string) (*PoolInfo, error) {
	// Normalize pool address/ID
	poolAddress = strings.TrimSpace(poolAddress)
	if !strings.HasPrefix(poolAddress, "0x") && !strings.HasPrefix(poolAddress, "0X") {
		poolAddress = "0x" + poolAddress
	}

	log.Printf("[PoolService] GetPoolInfo called for: %s", poolAddress)

	// Query from The Graph API (default to BSC network)
	poolData, err := s.graphAPI.QueryPool("bsc", poolAddress)
	if err != nil {
		return nil, fmt.Errorf("查询池子信息失败: %w", err)
	}

	// Determine exchange based on protocol and factory
	exchange := s.determineExchangeFromProtocol(poolData.Protocol, poolData.Factory)

	// Calculate tick spacing based on fee if not available
	tickSpacing := s.calculateTickSpacing(poolData.Fee)

	log.Printf("[PoolService] Pool info retrieved: %s %s/%s", exchange, poolData.InputToken.Symbol, poolData.OutputToken.Symbol)

	return &PoolInfo{
		Address:      poolData.Pool,
		Exchange:     exchange,
		Token0:       poolData.InputToken.Address,
		Token1:       poolData.OutputToken.Address,
		Token0Symbol: poolData.InputToken.Symbol,
		Token1Symbol: poolData.OutputToken.Symbol,
		Fee:          poolData.Fee,
		TickSpacing:  tickSpacing,
		CurrentTick:  0, // 不再查询当前 tick
		HooksAddress: "0x0000000000000000000000000000000000000000",
	}, nil
}

// GetV4PoolInfo retrieves information about a Uniswap V4 pool using PoolId
// Uses The Graph token API for symbols, and reads PositionManager.poolKeys(bytes25(poolId)) for fee/tickSpacing/hooks (authoritative).
func (s *PoolService) GetV4PoolInfo(poolId string) (*PoolInfo, error) {
	// Normalize PoolId
	poolId = strings.TrimSpace(poolId)
	if !strings.HasPrefix(poolId, "0x") && !strings.HasPrefix(poolId, "0X") {
		poolId = "0x" + poolId
	}

	log.Printf("[PoolService] GetV4PoolInfo called for: %s", poolId)

	// 1) Fetch token symbols via The Graph token API
	poolData, err := s.graphAPI.QueryPool("bsc", poolId)
	if err != nil {
		return nil, fmt.Errorf("查询 V4 池子信息失败: %w", err)
	}
	exchange := s.determineExchangeFromProtocol(poolData.Protocol, poolData.Factory)

	// 2) Resolve PoolKey components via on-chain PositionManager.poolKeys(bytes25(poolId)) (preferred)
	var (
		c0, c1      common.Address
		fee         = poolData.Fee
		tickSpacing = s.calculateTickSpacing(poolData.Fee)
		hooks       = common.HexToAddress("0x0000000000000000000000000000000000000000")
	)

	if blockchain.Client != nil && config.AppConfig != nil && common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		posm := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
		c0On, c1On, feeOn, tickSpacingOn, hooksOn, kErr := blockchain.GetUniswapV4PoolKeyFromPositionManager(posm, poolId)
		if kErr != nil {
			log.Printf("[PoolService] Warning: resolve V4 PoolKey via PositionManager.poolKeys failed: %v", kErr)
		} else {
			c0, c1 = c0On, c1On
			fee = int(feeOn)
			tickSpacing = tickSpacingOn
			hooks = hooksOn
		}
	}

	// Prefer on-chain currency0/currency1 if available; otherwise use API order.
	token0Addr := poolData.InputToken.Address
	token1Addr := poolData.OutputToken.Address
	token0Symbol := poolData.InputToken.Symbol
	token1Symbol := poolData.OutputToken.Symbol

	if (c0 != common.Address{}) && (c1 != common.Address{}) {
		token0Addr = c0.Hex()
		token1Addr = c1.Hex()

		// Re-map symbols to match currency0/currency1
		apiT0 := common.HexToAddress(poolData.InputToken.Address)
		apiT1 := common.HexToAddress(poolData.OutputToken.Address)
		switch {
		case apiT0 == c0 && apiT1 == c1:
			// keep as-is
		case apiT0 == c1 && apiT1 == c0:
			token0Symbol, token1Symbol = poolData.OutputToken.Symbol, poolData.InputToken.Symbol
		default:
			// Fallback to on-chain symbol when API tokens don't match currencies.
			if sym0, symErr := blockchain.GetTokenSymbol(c0); symErr == nil && sym0 != "" {
				token0Symbol = sym0
			}
			if sym1, symErr := blockchain.GetTokenSymbol(c1); symErr == nil && sym1 != "" {
				token1Symbol = sym1
			}
		}
	}

	return &PoolInfo{
		Address:      poolId,
		Exchange:     exchange,
		Token0:       token0Addr,
		Token1:       token1Addr,
		Token0Symbol: token0Symbol,
		Token1Symbol: token1Symbol,
		Fee:          fee,
		TickSpacing:  tickSpacing,
		CurrentTick:  0,
		HooksAddress: hooks.Hex(),
	}, nil
}

// determineExchangeFromProtocol determines the exchange name from protocol field and factory address
func (s *PoolService) determineExchangeFromProtocol(protocol, factory string) string {
	protocol = strings.ToLower(protocol)
	factory = strings.ToLower(factory)

	// Check Factory Address
	if factory == strings.ToLower("0x0BFbcf9fa4f9C56B0F40a671Ad40E0805A091865") {
		return "PancakeSwap V3"
	}

	switch {
	case strings.Contains(protocol, "pancakeswap"):
		return "PancakeSwap V3"
	case strings.Contains(protocol, "uniswap_v4"):
		return "Uniswap V4"
	case strings.Contains(protocol, "uniswap_v3"):
		return "Uniswap V3"
	case strings.Contains(protocol, "uniswap"):
		return "Uniswap"
	default:
		return protocol
	}
}

// calculateTickSpacing calculates tick spacing based on fee tier
func (s *PoolService) calculateTickSpacing(fee int) int {
	// Standard tick spacing for common fee tiers
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
		// Default to 60 for unknown fee tiers
		return 60
	}
}
