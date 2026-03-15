package blockchain

import (
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Minimal Uniswap V4 PositionManager ABI surface for modifying liquidity positions.
const v4PositionManagerABI = `[
  {
    "inputs": [
      {
        "components": [
          { "internalType": "address", "name": "currency0", "type": "address" },
          { "internalType": "address", "name": "currency1", "type": "address" },
          { "internalType": "uint24", "name": "fee", "type": "uint24" },
          { "internalType": "int24", "name": "tickSpacing", "type": "int24" },
          { "internalType": "address", "name": "hooks", "type": "address" }
        ],
        "internalType": "struct PoolKey",
        "name": "key",
        "type": "tuple"
      },
      { "internalType": "uint160", "name": "sqrtPriceX96", "type": "uint160" }
    ],
    "name": "initializePool",
    "outputs": [{ "internalType": "int24", "name": "", "type": "int24" }],
    "stateMutability": "payable",
    "type": "function"
  },
  {
    "inputs": [
      { "internalType": "bytes", "name": "unlockData", "type": "bytes" },
      { "internalType": "uint256", "name": "deadline", "type": "uint256" }
    ],
    "name": "modifyLiquidities",
    "outputs": [],
    "stateMutability": "payable",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "nextTokenId",
    "outputs": [{ "internalType": "uint256", "name": "", "type": "uint256" }],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{ "internalType": "uint256", "name": "tokenId", "type": "uint256" }],
    "name": "positions",
    "outputs": [
      { "internalType": "uint96", "name": "nonce", "type": "uint96" },
      { "internalType": "address", "name": "operator", "type": "address" },
      { "internalType": "address", "name": "token0", "type": "address" },
      { "internalType": "address", "name": "token1", "type": "address" },
      { "internalType": "uint24", "name": "fee", "type": "uint24" },
      { "internalType": "int24", "name": "tickLower", "type": "int24" },
      { "internalType": "int24", "name": "tickUpper", "type": "int24" },
      { "internalType": "uint128", "name": "liquidity", "type": "uint128" },
      { "internalType": "uint256", "name": "feeGrowthInside0LastX128", "type": "uint256" },
      { "internalType": "uint256", "name": "feeGrowthInside1LastX128", "type": "uint256" },
      { "internalType": "uint128", "name": "tokensOwed0", "type": "uint128" },
      { "internalType": "uint128", "name": "tokensOwed1", "type": "uint128" }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{ "internalType": "uint256", "name": "tokenId", "type": "uint256" }],
    "name": "positionInfo",
    "outputs": [{ "internalType": "uint256", "name": "info", "type": "uint256" }],
    "stateMutability": "view",
    "type": "function"
  }
]`

type V4PositionManager struct {
	contract *bind.BoundContract
	address  common.Address
}

type V4PoolKey struct {
	Currency0   common.Address `abi:"currency0"`
	Currency1   common.Address `abi:"currency1"`
	Fee         *big.Int       `abi:"fee"`
	TickSpacing *big.Int       `abi:"tickSpacing"`
	Hooks       common.Address `abi:"hooks"`
}

type V4PositionInfo struct {
	Token0                   common.Address
	Token1                   common.Address
	Fee                      uint64
	TickLower                int
	TickUpper                int
	Liquidity                *big.Int
	FeeGrowthInside0LastX128 *big.Int
	FeeGrowthInside1LastX128 *big.Int
	TokensOwed0              *big.Int
	TokensOwed1              *big.Int
	PoolId25                 string
	HasSubscriber            bool
	PositionRaw              []interface{}
}

func NewV4PositionManager(address common.Address, client *ethclient.Client) (*V4PositionManager, error) {
	parsed, err := abi.JSON(strings.NewReader(v4PositionManagerABI))
	if err != nil {
		return nil, err
	}
	rc := wrapRPCRetryClient(client)
	contract := bind.NewBoundContract(address, parsed, rc, rc, rc)
	return &V4PositionManager{contract: contract, address: address}, nil
}

func (m *V4PositionManager) ModifyLiquidities(opts *bind.TransactOpts, unlockData []byte, deadline *big.Int) (*types.Transaction, error) {
	return m.contract.Transact(opts, "modifyLiquidities", unlockData, deadline)
}

func (m *V4PositionManager) InitializePool(opts *bind.TransactOpts, key V4PoolKey, sqrtPriceX96 *big.Int) (*types.Transaction, error) {
	return m.contract.Transact(opts, "initializePool", key, sqrtPriceX96)
}

func (m *V4PositionManager) NextTokenID(opts *bind.CallOpts) (*big.Int, error) {
	var result []interface{}
	if err := m.contract.Call(opts, &result, "nextTokenId"); err != nil {
		return nil, err
	}
	if len(result) < 1 {
		return nil, fmt.Errorf("unexpected nextTokenId return length: %d", len(result))
	}
	v, ok := result[0].(*big.Int)
	if !ok || v == nil {
		return nil, fmt.Errorf("unexpected nextTokenId return type: %T", result[0])
	}
	return v, nil
}

func (m *V4PositionManager) Positions(opts *bind.CallOpts, tokenId *big.Int) (*V4PositionInfo, error) {
	var result []interface{}
	if err := m.contract.Call(opts, &result, "positions", tokenId); err != nil {
		return nil, err
	}
	if len(result) < 12 {
		return nil, fmt.Errorf("unexpected positions return length: %d", len(result))
	}

	token0, ok0 := result[2].(common.Address)
	token1, ok1 := result[3].(common.Address)
	feeBI, okFee := result[4].(*big.Int)
	tickLowerBI, okTL := result[5].(*big.Int)
	tickUpperBI, okTU := result[6].(*big.Int)
	liq, okL := result[7].(*big.Int)
	feeGrowth0, okFG0 := result[8].(*big.Int)
	feeGrowth1, okFG1 := result[9].(*big.Int)
	owed0, okO0 := result[10].(*big.Int)
	owed1, okO1 := result[11].(*big.Int)

	if !ok0 || !ok1 {
		return nil, fmt.Errorf("unexpected positions token types: token0=%T token1=%T", result[2], result[3])
	}

	fee := uint64(0)
	if okFee && feeBI != nil {
		fee = feeBI.Uint64()
	}
	if !okTL || tickLowerBI == nil || !okTU || tickUpperBI == nil {
		return nil, fmt.Errorf("unexpected positions tick types: tickLower=%T tickUpper=%T", result[5], result[6])
	}
	if !okL || liq == nil {
		liq = big.NewInt(0)
	}
	if !okFG0 || feeGrowth0 == nil {
		feeGrowth0 = big.NewInt(0)
	}
	if !okFG1 || feeGrowth1 == nil {
		feeGrowth1 = big.NewInt(0)
	}
	if !okO0 || owed0 == nil {
		log.Printf("[V4PM] TokensOwed0 解析失败: tokenId=%s okO0=%v type=%T value=%v", tokenId.String(), okO0, result[10], result[10])
		owed0 = big.NewInt(0)
	}
	if !okO1 || owed1 == nil {
		log.Printf("[V4PM] TokensOwed1 解析失败: tokenId=%s okO1=%v type=%T value=%v", tokenId.String(), okO1, result[11], result[11])
		owed1 = big.NewInt(0)
	}

	return &V4PositionInfo{
		Token0:                   token0,
		Token1:                   token1,
		Fee:                      fee,
		TickLower:                int(tickLowerBI.Int64()),
		TickUpper:                int(tickUpperBI.Int64()),
		Liquidity:                liq,
		FeeGrowthInside0LastX128: feeGrowth0,
		FeeGrowthInside1LastX128: feeGrowth1,
		TokensOwed0:              owed0,
		TokensOwed1:              owed1,
		PositionRaw:              result,
	}, nil
}

func (m *V4PositionManager) PositionInfoPacked(opts *bind.CallOpts, tokenId *big.Int) (*big.Int, error) {
	var result []interface{}
	if err := m.contract.Call(opts, &result, "positionInfo", tokenId); err != nil {
		return nil, err
	}
	if len(result) < 1 {
		return nil, fmt.Errorf("unexpected positionInfo return length: %d", len(result))
	}
	raw, ok := result[0].(*big.Int)
	if !ok || raw == nil {
		return nil, fmt.Errorf("unexpected positionInfo return type: %T", result[0])
	}
	return raw, nil
}
