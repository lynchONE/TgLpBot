package blockchain

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Minimal UniswapV3/PancakeV3 NonfungiblePositionManager surface for reading position info and approvals.
const v3PositionManagerABI = `[
  {
    "inputs": [
      { "internalType": "uint256", "name": "tokenId", "type": "uint256" }
    ],
    "name": "getApproved",
    "outputs": [
      { "internalType": "address", "name": "", "type": "address" }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      { "internalType": "address", "name": "to", "type": "address" },
      { "internalType": "uint256", "name": "tokenId", "type": "uint256" }
    ],
    "name": "approve",
    "outputs": [],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [
      {
        "components": [
          { "internalType": "uint256", "name": "tokenId", "type": "uint256" },
          { "internalType": "uint128", "name": "liquidity", "type": "uint128" },
          { "internalType": "uint256", "name": "amount0Min", "type": "uint256" },
          { "internalType": "uint256", "name": "amount1Min", "type": "uint256" },
          { "internalType": "uint256", "name": "deadline", "type": "uint256" }
        ],
        "internalType": "struct INonfungiblePositionManager.DecreaseLiquidityParams",
        "name": "params",
        "type": "tuple"
      }
    ],
    "name": "decreaseLiquidity",
    "outputs": [
      { "internalType": "uint256", "name": "amount0", "type": "uint256" },
      { "internalType": "uint256", "name": "amount1", "type": "uint256" }
    ],
    "stateMutability": "payable",
    "type": "function"
  },
  {
    "inputs": [
      {
        "components": [
          { "internalType": "uint256", "name": "tokenId", "type": "uint256" },
          { "internalType": "address", "name": "recipient", "type": "address" },
          { "internalType": "uint128", "name": "amount0Max", "type": "uint128" },
          { "internalType": "uint128", "name": "amount1Max", "type": "uint128" }
        ],
        "internalType": "struct INonfungiblePositionManager.CollectParams",
        "name": "params",
        "type": "tuple"
      }
    ],
    "name": "collect",
    "outputs": [
      { "internalType": "uint256", "name": "amount0", "type": "uint256" },
      { "internalType": "uint256", "name": "amount1", "type": "uint256" }
    ],
    "stateMutability": "payable",
    "type": "function"
  },
  {
    "inputs": [
      { "internalType": "uint256", "name": "tokenId", "type": "uint256" }
    ],
    "name": "burn",
    "outputs": [],
    "stateMutability": "payable",
    "type": "function"
  },
  {
    "inputs": [
      { "internalType": "uint256", "name": "tokenId", "type": "uint256" }
    ],
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
  }
]`

type V3PositionManager struct {
	contract *bind.BoundContract
	address  common.Address
}

type V3DecreaseLiquidityParams struct {
	TokenId    *big.Int `abi:"tokenId"`
	Liquidity  *big.Int `abi:"liquidity"` // uint128
	Amount0Min *big.Int `abi:"amount0Min"`
	Amount1Min *big.Int `abi:"amount1Min"`
	Deadline   *big.Int `abi:"deadline"`
}

type V3CollectParams struct {
	TokenId    *big.Int       `abi:"tokenId"`
	Recipient  common.Address `abi:"recipient"`
	Amount0Max *big.Int       `abi:"amount0Max"` // uint128
	Amount1Max *big.Int       `abi:"amount1Max"` // uint128
}

func NewV3PositionManager(address common.Address, client *ethclient.Client) (*V3PositionManager, error) {
	parsed, err := abi.JSON(strings.NewReader(v3PositionManagerABI))
	if err != nil {
		return nil, err
	}
	contract := bind.NewBoundContract(address, parsed, client, client, client)
	return &V3PositionManager{contract: contract, address: address}, nil
}

func (m *V3PositionManager) GetApproved(opts *bind.CallOpts, tokenId *big.Int) (common.Address, error) {
	var result []interface{}
	if err := m.contract.Call(opts, &result, "getApproved", tokenId); err != nil {
		return common.Address{}, err
	}
	if len(result) < 1 {
		return common.Address{}, fmt.Errorf("unexpected getApproved return length: %d", len(result))
	}
	addr, ok := result[0].(common.Address)
	if !ok {
		return common.Address{}, fmt.Errorf("unexpected getApproved type: %T", result[0])
	}
	return addr, nil
}

func (m *V3PositionManager) Approve(opts *bind.TransactOpts, to common.Address, tokenId *big.Int) (*types.Transaction, error) {
	return m.contract.Transact(opts, "approve", to, tokenId)
}

func (m *V3PositionManager) DecreaseLiquidity(
	opts *bind.TransactOpts,
	params V3DecreaseLiquidityParams,
) (*types.Transaction, error) {
	return m.contract.Transact(opts, "decreaseLiquidity", params)
}

func (m *V3PositionManager) Collect(
	opts *bind.TransactOpts,
	params V3CollectParams,
) (*types.Transaction, error) {
	return m.contract.Transact(opts, "collect", params)
}

func (m *V3PositionManager) Burn(opts *bind.TransactOpts, tokenId *big.Int) (*types.Transaction, error) {
	return m.contract.Transact(opts, "burn", tokenId)
}

func (m *V3PositionManager) PositionTokensAndLiquidity(opts *bind.CallOpts, tokenId *big.Int) (common.Address, common.Address, *big.Int, error) {
	var result []interface{}
	if err := m.contract.Call(opts, &result, "positions", tokenId); err != nil {
		return common.Address{}, common.Address{}, nil, err
	}
	// token0 at index 2, token1 at index 3, liquidity at index 7
	if len(result) < 8 {
		return common.Address{}, common.Address{}, nil, fmt.Errorf("unexpected positions return length: %d", len(result))
	}
	token0, ok0 := result[2].(common.Address)
	token1, ok1 := result[3].(common.Address)
	liq, okL := result[7].(*big.Int)
	if !ok0 || !ok1 || !okL || liq == nil {
		return common.Address{}, common.Address{}, nil, fmt.Errorf("unexpected positions types: token0=%T token1=%T liquidity=%T", result[2], result[3], result[7])
	}
	return token0, token1, liq, nil
}
