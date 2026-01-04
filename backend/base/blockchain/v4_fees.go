package blockchain

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// V4 StateView ABI for feeGrowth queries
const uniswapV4StateViewFeeGrowthABI = `[
  {
    "inputs": [
      { "internalType": "bytes32", "name": "poolId", "type": "bytes32" }
    ],
    "name": "getFeeGrowthGlobals",
    "outputs": [
      { "internalType": "uint256", "name": "feeGrowthGlobal0", "type": "uint256" },
      { "internalType": "uint256", "name": "feeGrowthGlobal1", "type": "uint256" }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      { "internalType": "bytes32", "name": "poolId", "type": "bytes32" },
      { "internalType": "int24", "name": "tick", "type": "int24" }
    ],
    "name": "getTickFeeGrowthOutside",
    "outputs": [
      { "internalType": "uint256", "name": "feeGrowthOutside0X128", "type": "uint256" },
      { "internalType": "uint256", "name": "feeGrowthOutside1X128", "type": "uint256" }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      { "internalType": "bytes32", "name": "poolId", "type": "bytes32" },
      { "internalType": "int24", "name": "tick", "type": "int24" }
    ],
    "name": "getTickInfo",
    "outputs": [
      { "internalType": "uint128", "name": "liquidityGross", "type": "uint128" },
      { "internalType": "int128", "name": "liquidityNet", "type": "int128" },
      { "internalType": "uint256", "name": "feeGrowthOutside0X128", "type": "uint256" },
      { "internalType": "uint256", "name": "feeGrowthOutside1X128", "type": "uint256" }
    ],
    "stateMutability": "view",
    "type": "function"
  }
]`

// GetV4PoolFeeGrowthGlobals 获取 V4 池子的全局手续费增长
func GetV4PoolFeeGrowthGlobals(stateView, poolManager common.Address, poolID string) (*big.Int, *big.Int, error) {
	if Client == nil {
		return nil, nil, fmt.Errorf("blockchain client not initialized")
	}
	if (stateView == common.Address{}) {
		return nil, nil, fmt.Errorf("stateView address not set")
	}

	id, err := normalizePoolID(poolID)
	if err != nil {
		return nil, nil, err
	}

	parsedABI, err := abi.JSON(strings.NewReader(uniswapV4StateViewFeeGrowthABI))
	if err != nil {
		return nil, nil, fmt.Errorf("parse StateView ABI failed: %w", err)
	}

	// 首先尝试 getFeeGrowthGlobals 方法
	data, err := parsedABI.Pack("getFeeGrowthGlobals", id)
	if err != nil {
		return nil, nil, fmt.Errorf("pack getFeeGrowthGlobals failed: %w", err)
	}

	msg := ethereum.CallMsg{To: &stateView, Data: data}
	raw, err := Client.CallContract(context.Background(), msg, nil)
	if err != nil {
		// 某些 StateView 实现可能不支持这个方法
		v4Debugf("getFeeGrowthGlobals failed: %v, returning zeros", err)
		return big.NewInt(0), big.NewInt(0), nil
	}

	out, err := parsedABI.Unpack("getFeeGrowthGlobals", raw)
	if err != nil {
		return nil, nil, fmt.Errorf("unpack getFeeGrowthGlobals failed: %w", err)
	}
	if len(out) < 2 {
		return nil, nil, fmt.Errorf("unexpected getFeeGrowthGlobals return length: %d", len(out))
	}

	fg0, ok0 := out[0].(*big.Int)
	fg1, ok1 := out[1].(*big.Int)
	if !ok0 || fg0 == nil {
		fg0 = big.NewInt(0)
	}
	if !ok1 || fg1 == nil {
		fg1 = big.NewInt(0)
	}

	return fg0, fg1, nil
}

// GetV4TickFeeGrowthOutside 获取 V4 指定 tick 的 feeGrowthOutside
func GetV4TickFeeGrowthOutside(stateView, poolManager common.Address, poolID string, tick int) (*big.Int, *big.Int, error) {
	if Client == nil {
		return nil, nil, fmt.Errorf("blockchain client not initialized")
	}
	if (stateView == common.Address{}) {
		return nil, nil, fmt.Errorf("stateView address not set")
	}

	id, err := normalizePoolID(poolID)
	if err != nil {
		return nil, nil, err
	}

	parsedABI, err := abi.JSON(strings.NewReader(uniswapV4StateViewFeeGrowthABI))
	if err != nil {
		return nil, nil, fmt.Errorf("parse StateView ABI failed: %w", err)
	}

	// 首先尝试 getTickFeeGrowthOutside 方法
	data, err := parsedABI.Pack("getTickFeeGrowthOutside", id, big.NewInt(int64(tick)))
	if err == nil {
		msg := ethereum.CallMsg{To: &stateView, Data: data}
		raw, callErr := Client.CallContract(context.Background(), msg, nil)
		if callErr == nil {
			out, unpackErr := parsedABI.Unpack("getTickFeeGrowthOutside", raw)
			if unpackErr == nil && len(out) >= 2 {
				fg0, ok0 := out[0].(*big.Int)
				fg1, ok1 := out[1].(*big.Int)
				if !ok0 || fg0 == nil {
					fg0 = big.NewInt(0)
				}
				if !ok1 || fg1 == nil {
					fg1 = big.NewInt(0)
				}
				return fg0, fg1, nil
			}
		}
	}

	// 如果 getTickFeeGrowthOutside 不可用，尝试 getTickInfo
	data, err = parsedABI.Pack("getTickInfo", id, big.NewInt(int64(tick)))
	if err != nil {
		return nil, nil, fmt.Errorf("pack getTickInfo failed: %w", err)
	}

	msg := ethereum.CallMsg{To: &stateView, Data: data}
	raw, err := Client.CallContract(context.Background(), msg, nil)
	if err != nil {
		// tick 未初始化时返回 0
		v4Debugf("getTickInfo failed for tick %d: %v, returning zeros", tick, err)
		return big.NewInt(0), big.NewInt(0), nil
	}

	out, err := parsedABI.Unpack("getTickInfo", raw)
	if err != nil {
		return nil, nil, fmt.Errorf("unpack getTickInfo failed: %w", err)
	}
	if len(out) < 4 {
		return nil, nil, fmt.Errorf("unexpected getTickInfo return length: %d", len(out))
	}

	// getTickInfo 返回: liquidityGross, liquidityNet, feeGrowthOutside0X128, feeGrowthOutside1X128
	fg0, ok0 := out[2].(*big.Int)
	fg1, ok1 := out[3].(*big.Int)
	if !ok0 || fg0 == nil {
		fg0 = big.NewInt(0)
	}
	if !ok1 || fg1 == nil {
		fg1 = big.NewInt(0)
	}

	return fg0, fg1, nil
}
