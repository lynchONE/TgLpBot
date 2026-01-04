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

const v3PoolFeeGrowthABI = `[
  {
    "inputs": [],
    "name": "feeGrowthGlobal0X128",
    "outputs": [
      { "internalType": "uint256", "name": "", "type": "uint256" }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "feeGrowthGlobal1X128",
    "outputs": [
      { "internalType": "uint256", "name": "", "type": "uint256" }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      { "internalType": "int24", "name": "tick", "type": "int24" }
    ],
    "name": "ticks",
    "outputs": [
      { "internalType": "uint128", "name": "liquidityGross", "type": "uint128" },
      { "internalType": "int128", "name": "liquidityNet", "type": "int128" },
      { "internalType": "uint256", "name": "feeGrowthOutside0X128", "type": "uint256" },
      { "internalType": "uint256", "name": "feeGrowthOutside1X128", "type": "uint256" },
      { "internalType": "int56", "name": "tickCumulativeOutside", "type": "int56" },
      { "internalType": "uint160", "name": "secondsPerLiquidityOutsideX128", "type": "uint160" },
      { "internalType": "uint32", "name": "secondsOutside", "type": "uint32" },
      { "internalType": "bool", "name": "initialized", "type": "bool" }
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

// GetV3PoolCurrentTickAtBlock returns the tick from slot0() at a given block number.
func GetV3PoolCurrentTickAtBlock(poolAddress common.Address, blockNumber uint64) (int, error) {
	if Client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}
	if blockNumber == 0 {
		return 0, fmt.Errorf("block number not set")
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
	block := new(big.Int).SetUint64(blockNumber)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := callContractWithRetryAtBlock(ctx, msg, block)
	if err != nil {
		return 0, fmt.Errorf("call slot0 failed: %w", err)
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

// GetV3PoolFee returns the fee tier from a UniswapV3/PancakeV3-style pool.
func GetV3PoolFee(poolAddress common.Address) (uint32, error) {
	if Client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}

	parsedABI, err := abi.JSON(strings.NewReader(v3PoolABI))
	if err != nil {
		return 0, fmt.Errorf("parse pool ABI failed: %w", err)
	}

	data, err := parsedABI.Pack("fee")
	if err != nil {
		return 0, fmt.Errorf("pack fee failed: %w", err)
	}

	msg := ethereum.CallMsg{To: &poolAddress, Data: data}
	raw, err := Client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return 0, fmt.Errorf("call fee failed: %w", err)
	}

	out, err := parsedABI.Unpack("fee", raw)
	if err != nil {
		return 0, fmt.Errorf("unpack fee failed: %w", err)
	}

	if len(out) != 1 {
		return 0, fmt.Errorf("unexpected fee return length: %d", len(out))
	}

	// fee is uint24 in Solidity, which unpacks to *big.Int
	if feeBig, ok := out[0].(*big.Int); ok {
		return uint32(feeBig.Uint64()), nil
	}

	return 0, fmt.Errorf("unexpected fee type: %T", out[0])
}

// GetV3PoolFeeGrowthGlobals returns feeGrowthGlobal0X128 and feeGrowthGlobal1X128 from the pool.
func GetV3PoolFeeGrowthGlobals(poolAddress common.Address) (*big.Int, *big.Int, error) {
	if Client == nil {
		return nil, nil, fmt.Errorf("blockchain client not initialized")
	}

	parsedABI, err := abi.JSON(strings.NewReader(v3PoolFeeGrowthABI))
	if err != nil {
		return nil, nil, fmt.Errorf("parse pool ABI failed: %w", err)
	}

	callUint256 := func(method string) (*big.Int, error) {
		data, err := parsedABI.Pack(method)
		if err != nil {
			return nil, fmt.Errorf("pack %s failed: %w", method, err)
		}
		msg := ethereum.CallMsg{To: &poolAddress, Data: data}
		raw, err := Client.CallContract(context.Background(), msg, nil)
		if err != nil {
			return nil, fmt.Errorf("call %s failed: %w", method, err)
		}
		out, err := parsedABI.Unpack(method, raw)
		if err != nil {
			return nil, fmt.Errorf("unpack %s failed: %w", method, err)
		}
		if len(out) != 1 {
			return nil, fmt.Errorf("unexpected %s return length: %d", method, len(out))
		}
		v, ok := out[0].(*big.Int)
		if !ok || v == nil {
			return nil, fmt.Errorf("unexpected %s type: %T", method, out[0])
		}
		return v, nil
	}

	g0, err := callUint256("feeGrowthGlobal0X128")
	if err != nil {
		return nil, nil, err
	}
	g1, err := callUint256("feeGrowthGlobal1X128")
	if err != nil {
		return nil, nil, err
	}
	return g0, g1, nil
}

// GetV3PoolTickFeeGrowthOutside returns feeGrowthOutside0/1 for a given tick.
func GetV3PoolTickFeeGrowthOutside(poolAddress common.Address, tick int) (*big.Int, *big.Int, bool, error) {
	if Client == nil {
		return nil, nil, false, fmt.Errorf("blockchain client not initialized")
	}

	parsedABI, err := abi.JSON(strings.NewReader(v3PoolFeeGrowthABI))
	if err != nil {
		return nil, nil, false, fmt.Errorf("parse pool ABI failed: %w", err)
	}

	data, err := parsedABI.Pack("ticks", big.NewInt(int64(tick)))
	if err != nil {
		return nil, nil, false, fmt.Errorf("pack ticks failed: %w", err)
	}
	msg := ethereum.CallMsg{To: &poolAddress, Data: data}
	raw, err := Client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return nil, nil, false, fmt.Errorf("call ticks failed: %w", err)
	}
	out, err := parsedABI.Unpack("ticks", raw)
	if err != nil {
		return nil, nil, false, fmt.Errorf("unpack ticks failed: %w", err)
	}
	if len(out) < 4 {
		return nil, nil, false, fmt.Errorf("unexpected ticks return length: %d", len(out))
	}

	fee0, ok0 := out[2].(*big.Int)
	fee1, ok1 := out[3].(*big.Int)
	if !ok0 || fee0 == nil {
		fee0 = big.NewInt(0)
	}
	if !ok1 || fee1 == nil {
		fee1 = big.NewInt(0)
	}

	initialized := false
	if len(out) >= 8 {
		if b, ok := out[7].(bool); ok {
			initialized = b
		}
	}

	return fee0, fee1, initialized, nil
}
