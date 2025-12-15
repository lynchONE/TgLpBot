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

// Minimal Uniswap V4 PositionManager ABI surface for modifying liquidity positions.
const v4PositionManagerABI = `[
  {
    "inputs": [
      { "internalType": "bytes", "name": "unlockData", "type": "bytes" },
      { "internalType": "uint256", "name": "deadline", "type": "uint256" }
    ],
    "name": "modifyLiquidities",
    "outputs": [],
    "stateMutability": "payable",
    "type": "function"
  }
]`

type V4PositionManager struct {
	contract *bind.BoundContract
	address  common.Address
}

func NewV4PositionManager(address common.Address, client *ethclient.Client) (*V4PositionManager, error) {
	parsed, err := abi.JSON(strings.NewReader(v4PositionManagerABI))
	if err != nil {
		return nil, err
	}
	contract := bind.NewBoundContract(address, parsed, client, client, client)
	return &V4PositionManager{contract: contract, address: address}, nil
}

func (m *V4PositionManager) ModifyLiquidities(opts *bind.TransactOpts, unlockData []byte, deadline *big.Int) (*types.Transaction, error) {
	return m.contract.Transact(opts, "modifyLiquidities", unlockData, deadline)
}
