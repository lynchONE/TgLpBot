package blockchain

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ERC20 ABI
const ERC20ABI = `[
	{
		"constant": true,
		"inputs": [],
		"name": "name",
		"outputs": [{"name": "", "type": "string"}],
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "symbol",
		"outputs": [{"name": "", "type": "string"}],
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "decimals",
		"outputs": [{"name": "", "type": "uint8"}],
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "totalSupply",
		"outputs": [{"name": "", "type": "uint256"}],
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [{"name": "account", "type": "address"}],
		"name": "balanceOf",
		"outputs": [{"name": "", "type": "uint256"}],
		"type": "function"
	},
	{
		"constant": false,
		"inputs": [
			{"name": "recipient", "type": "address"},
			{"name": "amount", "type": "uint256"}
		],
		"name": "transfer",
		"outputs": [{"name": "", "type": "bool"}],
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [
			{"name": "owner", "type": "address"},
			{"name": "spender", "type": "address"}
		],
		"name": "allowance",
		"outputs": [{"name": "", "type": "uint256"}],
		"type": "function"
	},
	{
		"constant": false,
		"inputs": [
			{"name": "spender", "type": "address"},
			{"name": "amount", "type": "uint256"}
		],
		"name": "approve",
		"outputs": [{"name": "", "type": "bool"}],
		"type": "function"
	},
	{
		"constant": false,
		"inputs": [
			{"name": "sender", "type": "address"},
			{"name": "recipient", "type": "address"},
			{"name": "amount", "type": "uint256"}
		],
		"name": "transferFrom",
		"outputs": [{"name": "", "type": "bool"}],
		"type": "function"
	}
]`

// ERC20 represents an ERC20 token contract
type ERC20 struct {
	contract *bind.BoundContract
	address  common.Address
}

// NewERC20 creates a new ERC20 instance
func NewERC20(address common.Address, client *ethclient.Client) (*ERC20, error) {
	parsed, err := abi.JSON(strings.NewReader(ERC20ABI))
	if err != nil {
		return nil, err
	}

	contract := bind.NewBoundContract(address, parsed, client, client, client)

	return &ERC20{
		contract: contract,
		address:  address,
	}, nil
}

// Name returns the token name
func (e *ERC20) Name(opts *bind.CallOpts) (string, error) {
	var result []interface{}
	err := e.contract.Call(opts, &result, "name")
	if err != nil {
		return "", err
	}
	return result[0].(string), nil
}

// Symbol returns the token symbol
func (e *ERC20) Symbol(opts *bind.CallOpts) (string, error) {
	var result []interface{}
	err := e.contract.Call(opts, &result, "symbol")
	if err != nil {
		return "", err
	}
	return result[0].(string), nil
}

// Decimals returns the token decimals
func (e *ERC20) Decimals(opts *bind.CallOpts) (uint8, error) {
	var result []interface{}
	err := e.contract.Call(opts, &result, "decimals")
	if err != nil {
		return 0, err
	}
	return result[0].(uint8), nil
}

// TotalSupply returns the total supply
func (e *ERC20) TotalSupply(opts *bind.CallOpts) (*big.Int, error) {
	var result []interface{}
	err := e.contract.Call(opts, &result, "totalSupply")
	if err != nil {
		return nil, err
	}
	return result[0].(*big.Int), nil
}

// BalanceOf returns the balance of an account
func (e *ERC20) BalanceOf(opts *bind.CallOpts, account common.Address) (*big.Int, error) {
	var result []interface{}
	err := e.contract.Call(opts, &result, "balanceOf", account)
	if err != nil {
		return nil, err
	}
	return result[0].(*big.Int), nil
}

// Allowance returns the allowance
func (e *ERC20) Allowance(opts *bind.CallOpts, owner, spender common.Address) (*big.Int, error) {
	var result []interface{}
	err := e.contract.Call(opts, &result, "allowance", owner, spender)
	if err != nil {
		return nil, err
	}
	return result[0].(*big.Int), nil
}

// Approve creates an approval transaction
func (e *ERC20) Approve(opts *bind.TransactOpts, spender common.Address, amount *big.Int) (*types.Transaction, error) {
	return e.contract.Transact(opts, "approve", spender, amount)
}

// Transfer creates a transfer transaction
func (e *ERC20) Transfer(opts *bind.TransactOpts, recipient common.Address, amount *big.Int) (*types.Transaction, error) {
	return e.contract.Transact(opts, "transfer", recipient, amount)
}
