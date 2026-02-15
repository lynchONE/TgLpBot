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

// Permit2 is Uniswap's shared approval/transfer helper contract.
// Address is the same across chains: https://docs.uniswap.org/contracts/permit2/overview
var Permit2Address = common.HexToAddress("0x000000000022D473030F116dDEE9F6B43aC78BA3")

var (
	maxUint160 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 160), big.NewInt(1))
	maxUint48  = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 48), big.NewInt(1))
)

// Minimal Permit2 AllowanceTransfer ABI surface.
const permit2ABI = `[
  {
    "inputs": [
      { "internalType": "address", "name": "user", "type": "address" },
      { "internalType": "address", "name": "token", "type": "address" },
      { "internalType": "address", "name": "spender", "type": "address" }
    ],
    "name": "allowance",
    "outputs": [
      { "internalType": "uint160", "name": "amount", "type": "uint160" },
      { "internalType": "uint48", "name": "expiration", "type": "uint48" },
      { "internalType": "uint48", "name": "nonce", "type": "uint48" }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      { "internalType": "address", "name": "token", "type": "address" },
      { "internalType": "address", "name": "spender", "type": "address" },
      { "internalType": "uint160", "name": "amount", "type": "uint160" },
      { "internalType": "uint48", "name": "expiration", "type": "uint48" }
    ],
    "name": "approve",
    "outputs": [],
    "stateMutability": "nonpayable",
    "type": "function"
  }
]`

type Permit2 struct {
	contract *bind.BoundContract
	address  common.Address
}

type Permit2Allowance struct {
	Amount     *big.Int
	Expiration *big.Int
	Nonce      *big.Int
}

func NewPermit2(address common.Address, client *ethclient.Client) (*Permit2, error) {
	parsed, err := abi.JSON(strings.NewReader(permit2ABI))
	if err != nil {
		return nil, err
	}
	contract := bind.NewBoundContract(address, parsed, client, client, client)
	return &Permit2{contract: contract, address: address}, nil
}

func (p *Permit2) Allowance(opts *bind.CallOpts, owner common.Address, token common.Address, spender common.Address) (*Permit2Allowance, error) {
	var result []interface{}
	if err := p.contract.Call(opts, &result, "allowance", owner, token, spender); err != nil {
		return nil, err
	}
	if len(result) < 3 {
		return nil, fmt.Errorf("unexpected allowance return length: %d", len(result))
	}
	amount, okA := result[0].(*big.Int)
	exp, okE := result[1].(*big.Int)
	nonce, okN := result[2].(*big.Int)
	if !okA || !okE || !okN {
		return nil, fmt.Errorf("unexpected allowance return types: amount=%T exp=%T nonce=%T", result[0], result[1], result[2])
	}
	if amount == nil {
		amount = big.NewInt(0)
	}
	if exp == nil {
		exp = big.NewInt(0)
	}
	if nonce == nil {
		nonce = big.NewInt(0)
	}
	return &Permit2Allowance{Amount: amount, Expiration: exp, Nonce: nonce}, nil
}

func (p *Permit2) Approve(opts *bind.TransactOpts, token common.Address, spender common.Address, amount *big.Int, expiration *big.Int) (*types.Transaction, error) {
	if amount == nil {
		amount = big.NewInt(0)
	}
	if expiration == nil {
		expiration = big.NewInt(0)
	}
	if amount.Sign() < 0 || amount.Cmp(maxUint160) > 0 {
		return nil, fmt.Errorf("permit2 approve amount out of range for uint160: %s", amount.String())
	}
	if expiration.Sign() < 0 || expiration.Cmp(maxUint48) > 0 {
		return nil, fmt.Errorf("permit2 approve expiration out of range for uint48: %s", expiration.String())
	}
	return p.contract.Transact(opts, "approve", token, spender, amount, expiration)
}
