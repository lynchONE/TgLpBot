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

//go:embed bytecode/atomic_increase_zap_bytecode.hex
var atomicIncreaseZapBytecodeHex string

const atomicIncreaseZapAdminABI = `[
  {
    "inputs": [
      { "internalType": "address", "name": "_okxSwapRouter", "type": "address" },
      { "internalType": "address", "name": "_okxTokenApprove", "type": "address" },
      { "internalType": "address", "name": "_v3PositionManager", "type": "address" },
      { "internalType": "address", "name": "_v4PositionManager", "type": "address" }
    ],
    "name": "setTrustedAddresses",
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
      { "internalType": "address[]", "name": "targets", "type": "address[]" },
      { "internalType": "bool", "name": "trusted", "type": "bool" }
    ],
    "name": "setTrustedSwapTargets",
    "outputs": [],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [
      { "internalType": "address[]", "name": "targets", "type": "address[]" },
      { "internalType": "bool", "name": "trusted", "type": "bool" }
    ],
    "name": "setTrustedApproveTargets",
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

func atomicIncreaseZapBytecode() ([]byte, error) {
	v := strings.TrimSpace(atomicIncreaseZapBytecodeHex)
	if v == "" {
		return nil, fmt.Errorf("atomic increase zap bytecode is empty")
	}
	b := common.FromHex(v)
	if len(b) == 0 {
		return nil, fmt.Errorf("atomic increase zap bytecode decode failed")
	}
	return b, nil
}

func DeployAtomicIncreaseZap(auth *bind.TransactOpts, client *ethclient.Client) (common.Address, *types.Transaction, error) {
	if auth == nil {
		return common.Address{}, nil, fmt.Errorf("auth is nil")
	}
	if client == nil {
		return common.Address{}, nil, fmt.Errorf("client is nil")
	}

	bytecode, err := atomicIncreaseZapBytecode()
	if err != nil {
		return common.Address{}, nil, err
	}

	parsed, err := abi.JSON(strings.NewReader(AtomicIncreaseZapABI))
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("parse AtomicIncreaseZap ABI failed: %w", err)
	}

	rc := wrapRPCRetryClient(client)
	addr, tx, _, err := bind.DeployContract(auth, parsed, bytecode, rc)
	if err != nil {
		return common.Address{}, nil, err
	}
	return addr, tx, nil
}

func atomicIncreaseZapAdminContract(zapAddr common.Address, client *ethclient.Client) (*bind.BoundContract, error) {
	if client == nil {
		return nil, fmt.Errorf("client is nil")
	}
	parsed, err := abi.JSON(strings.NewReader(atomicIncreaseZapAdminABI))
	if err != nil {
		return nil, fmt.Errorf("parse AtomicIncreaseZap admin ABI failed: %w", err)
	}
	rc := wrapRPCRetryClient(client)
	return bind.NewBoundContract(zapAddr, parsed, rc, rc, rc), nil
}

func AtomicIncreaseZapSetTrustedAddresses(
	auth *bind.TransactOpts,
	client *ethclient.Client,
	zapAddr common.Address,
	okxSwapRouter common.Address,
	okxTokenApprove common.Address,
	v3PositionManager common.Address,
	v4PositionManager common.Address,
) (*types.Transaction, error) {
	if auth == nil {
		return nil, fmt.Errorf("auth is nil")
	}
	c, err := atomicIncreaseZapAdminContract(zapAddr, client)
	if err != nil {
		return nil, err
	}
	return c.Transact(auth, "setTrustedAddresses", okxSwapRouter, okxTokenApprove, v3PositionManager, v4PositionManager)
}

func AtomicIncreaseZapSetTrustedV3PositionManagers(
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
	c, err := atomicIncreaseZapAdminContract(zapAddr, client)
	if err != nil {
		return nil, err
	}
	return c.Transact(auth, "setTrustedV3PositionManagers", positionManagers, trusted)
}

func AtomicIncreaseZapSetTrustedSwapTargets(
	auth *bind.TransactOpts,
	client *ethclient.Client,
	zapAddr common.Address,
	targets []common.Address,
	trusted bool,
) (*types.Transaction, error) {
	if auth == nil {
		return nil, fmt.Errorf("auth is nil")
	}
	if len(targets) == 0 {
		return nil, nil
	}
	c, err := atomicIncreaseZapAdminContract(zapAddr, client)
	if err != nil {
		return nil, err
	}
	return c.Transact(auth, "setTrustedSwapTargets", targets, trusted)
}

func AtomicIncreaseZapSetTrustedApproveTargets(
	auth *bind.TransactOpts,
	client *ethclient.Client,
	zapAddr common.Address,
	targets []common.Address,
	trusted bool,
) (*types.Transaction, error) {
	if auth == nil {
		return nil, fmt.Errorf("auth is nil")
	}
	if len(targets) == 0 {
		return nil, nil
	}
	c, err := atomicIncreaseZapAdminContract(zapAddr, client)
	if err != nil {
		return nil, err
	}
	return c.Transact(auth, "setTrustedApproveTargets", targets, trusted)
}

func AtomicIncreaseZapSetWrappedNative(
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
	c, err := atomicIncreaseZapAdminContract(zapAddr, client)
	if err != nil {
		return nil, err
	}
	return c.Transact(auth, "setWrappedNative", wrappedNative)
}
