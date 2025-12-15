package blockchain

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// v3PoolABI is used for pool meta reads (token0/token1/fee). Some V3 forks return
// slightly different `slot0()` types, so `GetV3PoolCurrentTick` uses a minimal ABI
// to only decode the fields we need.
const v3PoolABI = `[
  {
    "inputs": [],
    "name": "slot0",
    "outputs": [
      { "internalType": "uint160", "name": "sqrtPriceX96", "type": "uint160" },
      { "internalType": "int24", "name": "tick", "type": "int24" },
      { "internalType": "uint16", "name": "observationIndex", "type": "uint16" },
      { "internalType": "uint16", "name": "observationCardinality", "type": "uint16" },
      { "internalType": "uint16", "name": "observationCardinalityNext", "type": "uint16" },
      { "internalType": "uint8", "name": "feeProtocol", "type": "uint8" },
      { "internalType": "bool", "name": "unlocked", "type": "bool" }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "token0",
    "outputs": [
      { "internalType": "address", "name": "", "type": "address" }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "token1",
    "outputs": [
      { "internalType": "address", "name": "", "type": "address" }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "fee",
    "outputs": [
      { "internalType": "uint24", "name": "", "type": "uint24" }
    ],
    "stateMutability": "view",
    "type": "function"
  }
]`

const v3PoolSlot0MinABI = `[
  {
    "inputs": [],
    "name": "slot0",
    "outputs": [
      { "internalType": "uint160", "name": "sqrtPriceX96", "type": "uint160" },
      { "internalType": "int24", "name": "tick", "type": "int24" }
    ],
    "stateMutability": "view",
    "type": "function"
  }
]`

// GetV3PoolCurrentTick returns the current tick from a UniswapV3/PancakeV3-style pool via slot0().
func GetV3PoolCurrentTick(poolAddress common.Address) (int, error) {
	if Client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}

	parsedABI, err := abi.JSON(strings.NewReader(v3PoolSlot0MinABI))
	if err != nil {
		return 0, fmt.Errorf("parse pool ABI failed: %w", err)
	}

	data, err := parsedABI.Pack("slot0")
	if err != nil {
		return 0, fmt.Errorf("pack slot0 failed: %w", err)
	}

	msg := ethereum.CallMsg{To: &poolAddress, Data: data}
	var raw []byte
	var callErr error
	for attempt := 1; attempt <= 3; attempt++ {
		raw, callErr = Client.CallContract(context.Background(), msg, nil)
		if callErr == nil {
			break
		}
		// RPCs (incl. Alchemy) occasionally return transient EOFs; retry a couple times.
		if !strings.Contains(callErr.Error(), "EOF") && !strings.Contains(callErr.Error(), "connection") {
			break
		}
		time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
	}
	if callErr != nil {
		return 0, fmt.Errorf("call slot0 failed: %w", callErr)
	}

	out, err := parsedABI.Unpack("slot0", raw)
	if err != nil {
		return 0, fmt.Errorf("unpack slot0 failed: %w", err)
	}
	if len(out) < 2 {
		return 0, fmt.Errorf("unexpected slot0 return length: %d", len(out))
	}

	tickBig, ok := out[1].(*big.Int)
	if !ok || tickBig == nil {
		return 0, fmt.Errorf("unexpected tick type: %T", out[1])
	}

	return int(tickBig.Int64()), nil
}

// GetV3PoolSlot0 returns the sqrtPriceX96 and current tick from a V3 pool.
func GetV3PoolSlot0(poolAddress common.Address) (*big.Int, int, error) {
	if Client == nil {
		return nil, 0, fmt.Errorf("blockchain client not initialized")
	}

	parsedABI, err := abi.JSON(strings.NewReader(v3PoolSlot0MinABI))
	if err != nil {
		return nil, 0, fmt.Errorf("parse pool ABI failed: %w", err)
	}

	data, err := parsedABI.Pack("slot0")
	if err != nil {
		return nil, 0, fmt.Errorf("pack slot0 failed: %w", err)
	}

	msg := ethereum.CallMsg{To: &poolAddress, Data: data}
	raw, err := Client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("call slot0 failed: %w", err)
	}

	out, err := parsedABI.Unpack("slot0", raw)
	if err != nil {
		return nil, 0, fmt.Errorf("unpack slot0 failed: %w", err)
	}
	if len(out) < 2 {
		return nil, 0, fmt.Errorf("unexpected slot0 return length: %d", len(out))
	}

	sqrtPriceX96, ok := out[0].(*big.Int)
	if !ok {
		return nil, 0, fmt.Errorf("unexpected sqrtPriceX96 type: %T", out[0])
	}
	tickBig, ok := out[1].(*big.Int)
	if !ok {
		return nil, 0, fmt.Errorf("unexpected tick type: %T", out[1])
	}

	return sqrtPriceX96, int(tickBig.Int64()), nil
}

// GetV3PoolTokens returns (token0, token1) from a UniswapV3/PancakeV3-style pool.
func GetV3PoolTokens(poolAddress common.Address) (common.Address, common.Address, error) {
	if Client == nil {
		return common.Address{}, common.Address{}, fmt.Errorf("blockchain client not initialized")
	}

	parsedABI, err := abi.JSON(strings.NewReader(v3PoolABI))
	if err != nil {
		return common.Address{}, common.Address{}, fmt.Errorf("parse pool ABI failed: %w", err)
	}

	callAddr := func(method string) (common.Address, error) {
		data, err := parsedABI.Pack(method)
		if err != nil {
			return common.Address{}, fmt.Errorf("pack %s failed: %w", method, err)
		}
		msg := ethereum.CallMsg{To: &poolAddress, Data: data}
		raw, err := Client.CallContract(context.Background(), msg, nil)
		if err != nil {
			return common.Address{}, fmt.Errorf("call %s failed: %w", method, err)
		}
		out, err := parsedABI.Unpack(method, raw)
		if err != nil {
			return common.Address{}, fmt.Errorf("unpack %s failed: %w", method, err)
		}
		if len(out) != 1 {
			return common.Address{}, fmt.Errorf("unexpected %s return length: %d", method, len(out))
		}
		if addr, ok := out[0].(common.Address); ok {
			return addr, nil
		}
		if b, ok := out[0].([20]byte); ok {
			return common.BytesToAddress(b[:]), nil
		}
		return common.Address{}, fmt.Errorf("unexpected %s return type: %T", method, out[0])
	}

	t0, err := callAddr("token0")
	if err != nil {
		return common.Address{}, common.Address{}, err
	}
	t1, err := callAddr("token1")
	if err != nil {
		return common.Address{}, common.Address{}, err
	}
	return t0, t1, nil
}
