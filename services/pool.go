package services

import (
	"TgLpBot/blockchain"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
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
}

// PoolService handles pool-related operations
type PoolService struct {
	client *blockchain.Client
}

// NewPoolService creates a new pool service
func NewPoolService() *PoolService {
	return &PoolService{
		client: blockchain.GetClient(),
	}
}

// Uniswap V3 Pool ABI (minimal)
const uniswapV3PoolABI = `[
	{
		"inputs": [],
		"name": "token0",
		"outputs": [{"internalType": "address", "name": "", "type": "address"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "token1",
		"outputs": [{"internalType": "address", "name": "", "type": "address"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "fee",
		"outputs": [{"internalType": "uint24", "name": "", "type": "uint24"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "tickSpacing",
		"outputs": [{"internalType": "int24", "name": "", "type": "int24"}],
		"stateMutability": "view",
		"type": "function"
	}
]`

// GetPoolInfo retrieves information about a pool
func (s *PoolService) GetPoolInfo(poolAddress string) (*PoolInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}

	// Parse pool ABI
	poolABI, err := abi.JSON(strings.NewReader(uniswapV3PoolABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse pool ABI: %w", err)
	}

	poolAddr := common.HexToAddress(poolAddress)

	// Get token0
	var token0Result []interface{}
	token0Data, err := poolABI.Pack("token0")
	if err != nil {
		return nil, fmt.Errorf("failed to pack token0 call: %w", err)
	}
	token0Result, err = s.client.CallContract(poolAddr, token0Data)
	if err != nil {
		return nil, fmt.Errorf("failed to get token0: %w", err)
	}
	token0 := token0Result[0].(common.Address).Hex()

	// Get token1
	var token1Result []interface{}
	token1Data, err := poolABI.Pack("token1")
	if err != nil {
		return nil, fmt.Errorf("failed to pack token1 call: %w", err)
	}
	token1Result, err = s.client.CallContract(poolAddr, token1Data)
	if err != nil {
		return nil, fmt.Errorf("failed to get token1: %w", err)
	}
	token1 := token1Result[0].(common.Address).Hex()

	// Get fee
	var feeResult []interface{}
	feeData, err := poolABI.Pack("fee")
	if err != nil {
		return nil, fmt.Errorf("failed to pack fee call: %w", err)
	}
	feeResult, err = s.client.CallContract(poolAddr, feeData)
	if err != nil {
		return nil, fmt.Errorf("failed to get fee: %w", err)
	}
	fee := int(feeResult[0].(*big.Int).Int64())

	// Get tickSpacing
	var tickSpacingResult []interface{}
	tickSpacingData, err := poolABI.Pack("tickSpacing")
	if err != nil {
		return nil, fmt.Errorf("failed to pack tickSpacing call: %w", err)
	}
	tickSpacingResult, err = s.client.CallContract(poolAddr, tickSpacingData)
	if err != nil {
		return nil, fmt.Errorf("failed to get tickSpacing: %w", err)
	}
	tickSpacing := int(tickSpacingResult[0].(*big.Int).Int64())

	// Get token symbols
	erc20Service := NewERC20Service()
	token0Symbol, _ := erc20Service.GetSymbol(token0)
	token1Symbol, _ := erc20Service.GetSymbol(token1)

	// Determine exchange based on pool characteristics
	exchange := s.determineExchange(fee, tickSpacing)

	return &PoolInfo{
		Address:      poolAddress,
		Exchange:     exchange,
		Token0:       token0,
		Token1:       token1,
		Token0Symbol: token0Symbol,
		Token1Symbol: token1Symbol,
		Fee:          fee,
		TickSpacing:  tickSpacing,
	}, nil
}

// determineExchange determines the exchange based on pool characteristics
func (s *PoolService) determineExchange(fee, tickSpacing int) string {
	// PancakeSwap V3 on BSC
	// Fee tiers: 100 (0.01%), 500 (0.05%), 2500 (0.25%), 10000 (1%)
	// Tick spacing: 1, 10, 50, 200
	
	switch {
	case fee == 100 && tickSpacing == 1:
		return "PancakeSwap V3"
	case fee == 500 && tickSpacing == 10:
		return "PancakeSwap V3"
	case fee == 2500 && tickSpacing == 50:
		return "PancakeSwap V3"
	case fee == 10000 && tickSpacing == 200:
		return "PancakeSwap V3"
	default:
		return "Unknown DEX"
	}
}

// ERC20Service handles ERC20 token operations
type ERC20Service struct {
	client *blockchain.Client
}

// NewERC20Service creates a new ERC20 service
func NewERC20Service() *ERC20Service {
	return &ERC20Service{
		client: blockchain.GetClient(),
	}
}

const erc20ABI = `[
	{
		"inputs": [],
		"name": "symbol",
		"outputs": [{"internalType": "string", "name": "", "type": "string"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "decimals",
		"outputs": [{"internalType": "uint8", "name": "", "type": "uint8"}],
		"stateMutability": "view",
		"type": "function"
	}
]`

// GetSymbol gets the symbol of an ERC20 token
func (s *ERC20Service) GetSymbol(tokenAddress string) (string, error) {
	if s.client == nil {
		return "", fmt.Errorf("blockchain client not initialized")
	}

	tokenABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return "", fmt.Errorf("failed to parse ERC20 ABI: %w", err)
	}

	tokenAddr := common.HexToAddress(tokenAddress)

	symbolData, err := tokenABI.Pack("symbol")
	if err != nil {
		return "", fmt.Errorf("failed to pack symbol call: %w", err)
	}

	result, err := s.client.CallContract(tokenAddr, symbolData)
	if err != nil {
		return "", fmt.Errorf("failed to get symbol: %w", err)
	}

	if len(result) == 0 {
		return "UNKNOWN", nil
	}

	return result[0].(string), nil
}

// GetDecimals gets the decimals of an ERC20 token
func (s *ERC20Service) GetDecimals(tokenAddress string) (uint8, error) {
	if s.client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}

	tokenABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return 0, fmt.Errorf("failed to parse ERC20 ABI: %w", err)
	}

	tokenAddr := common.HexToAddress(tokenAddress)

	decimalsData, err := tokenABI.Pack("decimals")
	if err != nil {
		return 0, fmt.Errorf("failed to pack decimals call: %w", err)
	}

	result, err := s.client.CallContract(tokenAddr, decimalsData)
	if err != nil {
		return 0, fmt.Errorf("failed to get decimals: %w", err)
	}

	if len(result) == 0 {
		return 18, nil // Default to 18 decimals
	}

	return result[0].(uint8), nil
}

