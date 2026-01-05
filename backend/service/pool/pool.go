package pool

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
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
	// 使用纯链上查询，不再依赖外部 API
}

// NewPoolService creates a new pool service
func NewPoolService() *PoolService {
	return &PoolService{}
}

// GetPoolInfo retrieves information about a V3 pool from blockchain
// Supports PancakeSwap V3, Uniswap V3 on BSC
func (s *PoolService) GetPoolInfo(poolAddress string) (*PoolInfo, error) {
	// Normalize pool address
	poolAddress = strings.TrimSpace(poolAddress)
	if !strings.HasPrefix(poolAddress, "0x") && !strings.HasPrefix(poolAddress, "0X") {
		poolAddress = "0x" + poolAddress
	}

	log.Printf("[PoolService] GetPoolInfo called for: %s", poolAddress)

	// 直接使用链上查询
	return s.getPoolInfoFromChain(poolAddress)
}

// GetV4PoolInfo retrieves information about a Uniswap V4 pool using PoolId
// Reads PositionManager.poolKeys(bytes25(poolId)) for complete pool information
func (s *PoolService) GetV4PoolInfo(poolId string) (*PoolInfo, error) {
	// Normalize PoolId
	poolId = strings.TrimSpace(poolId)
	if !strings.HasPrefix(poolId, "0x") && !strings.HasPrefix(poolId, "0X") {
		poolId = "0x" + poolId
	}

	log.Printf("[PoolService] GetV4PoolInfo called for: %s", poolId)

	if blockchain.Client == nil {
		return nil, fmt.Errorf("区块链客户端未初始化")
	}

	if config.AppConfig == nil || !common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		return nil, fmt.Errorf("V4 PositionManager 地址未配置")
	}

	// 从 PositionManager 读取 PoolKey
	posm := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
	c0, c1, fee, tickSpacing, hooks, err := blockchain.GetUniswapV4PoolKeyFromPositionManager(posm, poolId)
	if err != nil {
		return nil, fmt.Errorf("读取 V4 PoolKey 失败: %w", err)
	}

	// 获取代币符号
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

	log.Printf("[PoolService] V4 Pool info retrieved from chain: Uniswap V4 %s/%s (fee: %d)", token0Symbol, token1Symbol, fee)

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

// getPoolInfoFromChain 从链上直接读取 V3 池子信息（备用方案）
func (s *PoolService) getPoolInfoFromChain(poolAddress string) (*PoolInfo, error) {
	if blockchain.Client == nil {
		return nil, fmt.Errorf("区块链客户端未初始化")
	}

	poolAddr := common.HexToAddress(poolAddress)

	// 1. 读取 token0 和 token1
	token0, token1, err := blockchain.GetV3PoolTokens(poolAddr)
	if err != nil {
		return nil, fmt.Errorf("读取池子代币地址失败: %w", err)
	}

	// 2. 读取手续费率
	fee, err := blockchain.GetV3PoolFee(poolAddr)
	if err != nil {
		return nil, fmt.Errorf("读取池子手续费失败: %w", err)
	}

	// 3. 获取代币符号
	token0Symbol, err := blockchain.GetTokenSymbol(token0)
	if err != nil {
		log.Printf("[PoolService] Warning: could not get token0 symbol: %v", err)
		token0Symbol = "UNKNOWN"
	}

	token1Symbol, err := blockchain.GetTokenSymbol(token1)
	if err != nil {
		log.Printf("[PoolService] Warning: could not get token1 symbol: %v", err)
		token1Symbol = "UNKNOWN"
	}

	// 4. 通过池子的 factory() 方法判断是哪个交易所
	exchange := s.determineExchangeFromFactory(poolAddr)

	tickSpacing := s.calculateTickSpacing(int(fee))

	log.Printf("[PoolService] Pool info retrieved from chain: %s %s/%s (fee: %d)", exchange, token0Symbol, token1Symbol, fee)

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

// determineExchangeFromFactory 通过读取池子的 factory() 方法来精确判断交易所
func (s *PoolService) determineExchangeFromFactory(poolAddr common.Address) string {
	// 直接从池子读取它的 factory 地址
	factoryAddr, err := blockchain.GetV3PoolFactory(poolAddr)
	if err != nil {
		log.Printf("[PoolService] Warning: 无法读取池子的 factory 地址: %v", err)
		return "V3 Pool"
	}

	// 已知的工厂合约地址 (BSC 主网)
	pancakeFactory := common.HexToAddress("0x0BFbcf9fa4f9C56B0F40a671Ad40E0805A091865")
	uniswapFactory := common.HexToAddress("0xdB1d10011AD0Ff90774D0C6Bb92e5C5c8b4461F7")

	log.Printf("[PoolService] Pool factory 地址: %s", factoryAddr.Hex())

	// 根据 factory 地址判断交易所
	if factoryAddr == pancakeFactory {
		log.Printf("[PoolService] 识别为 PancakeSwap V3")
		return "PancakeSwap V3"
	}
	if factoryAddr == uniswapFactory {
		log.Printf("[PoolService] 识别为 Uniswap V3")
		return "Uniswap V3"
	}

	// 未知的工厂
	log.Printf("[PoolService] Warning: 未知的工厂地址 %s，返回默认值", factoryAddr.Hex())
	return "V3 Pool"
}
