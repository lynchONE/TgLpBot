package blockchain

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

//go:embed bytecode/zap_simple_bytecode.hex
var zapSimpleBytecodeHex string

const zapSimpleAdminABI = `[
  {
    "inputs": [
      { "internalType": "address", "name": "_v3PositionManager", "type": "address" },
      { "internalType": "address", "name": "_v4PositionManager", "type": "address" }
    ],
    "name": "setPositionManagers",
    "outputs": [],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [
      { "internalType": "address[]", "name": "positionManagers", "type": "address[]" },
      { "internalType": "bool", "name": "trusted", "type": "bool" }
    ],
    "name": "setTrustedV3PositionManagers",
    "outputs": [],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [
      { "internalType": "address", "name": "_wrappedNative", "type": "address" }
    ],
    "name": "setWrappedNative",
    "outputs": [],
    "stateMutability": "nonpayable",
    "type": "function"
  }
]`

func zapSimpleBytecode() ([]byte, error) {
	v := strings.TrimSpace(zapSimpleBytecodeHex)
	if v == "" {
		return nil, fmt.Errorf("zap simple bytecode is empty")
	}
	b := common.FromHex(v)
	if len(b) == 0 {
		return nil, fmt.Errorf("zap simple bytecode decode failed")
	}
	return b, nil
}

// DeployZapSimple deploys a new ZapSimple contract instance (no constructor args).
// The caller is responsible for waiting for mining and for calling admin setup methods.
func DeployZapSimple(auth *bind.TransactOpts, client *ethclient.Client) (common.Address, *types.Transaction, error) {
	if auth == nil {
		return common.Address{}, nil, fmt.Errorf("auth is nil")
	}
	if client == nil {
		return common.Address{}, nil, fmt.Errorf("client is nil")
	}

	bytecode, err := zapSimpleBytecode()
	if err != nil {
		return common.Address{}, nil, err
	}

	parsed, err := abi.JSON(strings.NewReader(ZapSimpleABI))
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("parse ZapSimple ABI failed: %w", err)
	}

	rc := wrapRPCRetryClient(client)
	addr, tx, _, err := bind.DeployContract(auth, parsed, bytecode, rc)
	if err != nil {
		return common.Address{}, nil, err
	}
	return addr, tx, nil
}

func zapSimpleAdminContract(zapAddr common.Address, client *ethclient.Client) (*bind.BoundContract, error) {
	if client == nil {
		return nil, fmt.Errorf("client is nil")
	}
	parsed, err := abi.JSON(strings.NewReader(zapSimpleAdminABI))
	if err != nil {
		return nil, fmt.Errorf("parse ZapSimple admin ABI failed: %w", err)
	}
	rc := wrapRPCRetryClient(client)
	return bind.NewBoundContract(zapAddr, parsed, rc, rc, rc), nil
}

func ZapSimpleSetPositionManagers(
	auth *bind.TransactOpts,
	client *ethclient.Client,
	zapAddr common.Address,
	v3PositionManager common.Address,
	v4PositionManager common.Address,
) (*types.Transaction, error) {
	if auth == nil {
		return nil, fmt.Errorf("auth is nil")
	}
	c, err := zapSimpleAdminContract(zapAddr, client)
	if err != nil {
		return nil, err
	}
	return c.Transact(auth, "setPositionManagers", v3PositionManager, v4PositionManager)
}

func ZapSimpleSetTrustedV3PositionManagers(
	auth *bind.TransactOpts,
	client *ethclient.Client,
	zapAddr common.Address,
	positionManagers []common.Address,
	trusted bool,
) (*types.Transaction, error) {
	if auth == nil {
		return nil, fmt.Errorf("auth is nil")
	}
	if len(positionManagers) == 0 {
		return nil, nil
	}
	c, err := zapSimpleAdminContract(zapAddr, client)
	if err != nil {
		return nil, err
	}
	return c.Transact(auth, "setTrustedV3PositionManagers", positionManagers, trusted)
}

func ZapSimpleSetWrappedNative(
	auth *bind.TransactOpts,
	client *ethclient.Client,
	zapAddr common.Address,
	wrappedNative common.Address,
) (*types.Transaction, error) {
	if auth == nil {
		return nil, fmt.Errorf("auth is nil")
	}
	if wrappedNative == (common.Address{}) {
		return nil, fmt.Errorf("wrapped native is zero")
	}
	c, err := zapSimpleAdminContract(zapAddr, client)
	if err != nil {
		return nil, err
	}
	return c.Transact(auth, "setWrappedNative", wrappedNative)
}
