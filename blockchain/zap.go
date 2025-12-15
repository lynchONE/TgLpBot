package blockchain

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// LiquidityZap ABI
const ZapABI = `[
	{
		"inputs": [
			{"internalType": "address", "name": "_router", "type": "address"}
		],
		"stateMutability": "nonpayable",
		"type": "constructor"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "address", "name": "user", "type": "address"},
			{"indexed": true, "internalType": "address", "name": "pair", "type": "address"},
			{"indexed": false, "internalType": "address", "name": "tokenIn", "type": "address"},
			{"indexed": false, "internalType": "uint256", "name": "amountIn", "type": "uint256"},
			{"indexed": false, "internalType": "uint256", "name": "liquidity", "type": "uint256"}
		],
		"name": "ZapIn",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "address", "name": "user", "type": "address"},
			{"indexed": true, "internalType": "address", "name": "pair", "type": "address"},
			{"indexed": false, "internalType": "address", "name": "tokenOut", "type": "address"},
			{"indexed": false, "internalType": "uint256", "name": "liquidity", "type": "uint256"},
			{"indexed": false, "internalType": "uint256", "name": "amountOut", "type": "uint256"}
		],
		"name": "ZapOut",
		"type": "event"
	},
	{
		"inputs": [
			{"internalType": "address", "name": "tokenIn", "type": "address"},
			{"internalType": "uint256", "name": "amountIn", "type": "uint256"},
			{"internalType": "address", "name": "pair", "type": "address"},
			{"internalType": "uint256", "name": "minLiquidity", "type": "uint256"},
			{"internalType": "uint256", "name": "deadline", "type": "uint256"}
		],
		"name": "zapIn",
		"outputs": [
			{"internalType": "uint256", "name": "liquidity", "type": "uint256"}
		],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "address", "name": "pair", "type": "address"},
			{"internalType": "uint256", "name": "liquidity", "type": "uint256"},
			{"internalType": "address", "name": "tokenOut", "type": "address"},
			{"internalType": "uint256", "name": "minAmountOut", "type": "uint256"},
			{"internalType": "uint256", "name": "deadline", "type": "uint256"}
		],
		"name": "zapOut",
		"outputs": [
			{"internalType": "uint256", "name": "amountOut", "type": "uint256"}
		],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "owner",
		"outputs": [
			{"internalType": "address", "name": "", "type": "address"}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "router",
		"outputs": [
			{"internalType": "contract IPancakeRouter02", "name": "", "type": "address"}
		],
		"stateMutability": "view",
		"type": "function"
	}
]`

// LiquidityZap represents the Zap contract
type LiquidityZap struct {
	contract *bind.BoundContract
	address  common.Address
}

// NewLiquidityZap creates a new LiquidityZap instance
func NewLiquidityZap(address common.Address, client *ethclient.Client) (*LiquidityZap, error) {
	parsed, err := abi.JSON(strings.NewReader(ZapABI))
	if err != nil {
		return nil, err
	}
	
	contract := bind.NewBoundContract(address, parsed, client, client, client)
	
	return &LiquidityZap{
		contract: contract,
		address:  address,
	}, nil
}

// ZapIn adds liquidity with a single token
func (z *LiquidityZap) ZapIn(
	opts *bind.TransactOpts,
	tokenIn common.Address,
	amountIn *big.Int,
	pair common.Address,
	minLiquidity *big.Int,
	deadline *big.Int,
) (*bind.BoundContract, error) {
	return z.contract, z.contract.Transact(opts, "zapIn", tokenIn, amountIn, pair, minLiquidity, deadline)
}

// ZapOut removes liquidity and receives a single token
func (z *LiquidityZap) ZapOut(
	opts *bind.TransactOpts,
	pair common.Address,
	liquidity *big.Int,
	tokenOut common.Address,
	minAmountOut *big.Int,
	deadline *big.Int,
) (*bind.BoundContract, error) {
	return z.contract, z.contract.Transact(opts, "zapOut", pair, liquidity, tokenOut, minAmountOut, deadline)
}

// Owner returns the owner address
func (z *LiquidityZap) Owner(opts *bind.CallOpts) (common.Address, error) {
	var result []interface{}
	err := z.contract.Call(opts, &result, "owner")
	if err != nil {
		return common.Address{}, err
	}
	return result[0].(common.Address), nil
}

// Router returns the router address
func (z *LiquidityZap) Router(opts *bind.CallOpts) (common.Address, error) {
	var result []interface{}
	err := z.contract.Call(opts, &result, "router")
	if err != nil {
		return common.Address{}, err
	}
	return result[0].(common.Address), nil
}

