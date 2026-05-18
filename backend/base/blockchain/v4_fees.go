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
	"github.com/ethereum/go-ethereum/ethclient"
)

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

func GetV4PoolFeeGrowthGlobals(stateView, poolManager common.Address, poolID string) (*big.Int, *big.Int, error) {
	return GetV4PoolFeeGrowthGlobalsWithClient(Client, stateView, poolManager, poolID)
}

func GetV4PoolFeeGrowthGlobalsWithClient(client *ethclient.Client, stateView, poolManager common.Address, poolID string) (*big.Int, *big.Int, error) {
	return getV4PoolFeeGrowthGlobalsWithClientAtBlock(client, stateView, poolManager, poolID, nil)
}

func GetV4PoolFeeGrowthGlobalsAtBlock(stateView, poolManager common.Address, poolID string, blockNumber uint64) (*big.Int, *big.Int, error) {
	return GetV4PoolFeeGrowthGlobalsAtBlockWithClient(Client, stateView, poolManager, poolID, blockNumber)
}

func GetV4PoolFeeGrowthGlobalsAtBlockWithClient(client *ethclient.Client, stateView, poolManager common.Address, poolID string, blockNumber uint64) (*big.Int, *big.Int, error) {
	if blockNumber == 0 {
		return nil, nil, fmt.Errorf("block number not set")
	}
	block := new(big.Int).SetUint64(blockNumber)
	return getV4PoolFeeGrowthGlobalsWithClientAtBlock(client, stateView, poolManager, poolID, block)
}

func getV4PoolFeeGrowthGlobalsAtBlock(stateView, poolManager common.Address, poolID string, block *big.Int) (*big.Int, *big.Int, error) {
	return getV4PoolFeeGrowthGlobalsWithClientAtBlock(Client, stateView, poolManager, poolID, block)
}

func getV4PoolFeeGrowthGlobalsWithClientAtBlock(client *ethclient.Client, stateView, poolManager common.Address, poolID string, block *big.Int) (*big.Int, *big.Int, error) {
	if client == nil {
		return nil, nil, fmt.Errorf("blockchain client not initialized")
	}
	if stateView == (common.Address{}) {
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

	data, err := parsedABI.Pack("getFeeGrowthGlobals", id)
	if err != nil {
		return nil, nil, fmt.Errorf("pack getFeeGrowthGlobals failed: %w", err)
	}

	msg := ethereum.CallMsg{To: &stateView, Data: data}
	callCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var raw []byte
	if block != nil {
		raw, err = callContractWithRetryAtBlock(client, callCtx, msg, block)
	} else {
		raw, err = callContractWithRetry(client, callCtx, msg)
	}
	if err != nil {
		v4Debugf("getFeeGrowthGlobals failed: %v", err)
		return nil, nil, fmt.Errorf("call getFeeGrowthGlobals failed: %w", err)
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

func GetV4TickFeeGrowthOutside(stateView, poolManager common.Address, poolID string, tick int) (*big.Int, *big.Int, error) {
	return GetV4TickFeeGrowthOutsideWithClient(Client, stateView, poolManager, poolID, tick)
}

func GetV4TickFeeGrowthOutsideWithClient(client *ethclient.Client, stateView, poolManager common.Address, poolID string, tick int) (*big.Int, *big.Int, error) {
	return getV4TickFeeGrowthOutsideWithClientAtBlock(client, stateView, poolManager, poolID, tick, nil)
}

func GetV4TickFeeGrowthOutsideAtBlock(stateView, poolManager common.Address, poolID string, tick int, blockNumber uint64) (*big.Int, *big.Int, error) {
	return GetV4TickFeeGrowthOutsideAtBlockWithClient(Client, stateView, poolManager, poolID, tick, blockNumber)
}

func GetV4TickFeeGrowthOutsideAtBlockWithClient(client *ethclient.Client, stateView, poolManager common.Address, poolID string, tick int, blockNumber uint64) (*big.Int, *big.Int, error) {
	if blockNumber == 0 {
		return nil, nil, fmt.Errorf("block number not set")
	}
	block := new(big.Int).SetUint64(blockNumber)
	return getV4TickFeeGrowthOutsideWithClientAtBlock(client, stateView, poolManager, poolID, tick, block)
}

func getV4TickFeeGrowthOutsideAtBlock(stateView, poolManager common.Address, poolID string, tick int, block *big.Int) (*big.Int, *big.Int, error) {
	return getV4TickFeeGrowthOutsideWithClientAtBlock(Client, stateView, poolManager, poolID, tick, block)
}

func getV4TickFeeGrowthOutsideWithClientAtBlock(client *ethclient.Client, stateView, poolManager common.Address, poolID string, tick int, block *big.Int) (*big.Int, *big.Int, error) {
	if client == nil {
		return nil, nil, fmt.Errorf("blockchain client not initialized")
	}
	if stateView == (common.Address{}) {
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

	data, err := parsedABI.Pack("getTickFeeGrowthOutside", id, big.NewInt(int64(tick)))
	if err == nil {
		msg := ethereum.CallMsg{To: &stateView, Data: data}
		callCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		var raw []byte
		var callErr error
		if block != nil {
			raw, callErr = callContractWithRetryAtBlock(client, callCtx, msg, block)
		} else {
			raw, callErr = callContractWithRetry(client, callCtx, msg)
		}
		cancel()
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

	data, err = parsedABI.Pack("getTickInfo", id, big.NewInt(int64(tick)))
	if err != nil {
		return nil, nil, fmt.Errorf("pack getTickInfo failed: %w", err)
	}

	msg := ethereum.CallMsg{To: &stateView, Data: data}
	callCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var raw []byte
	if block != nil {
		raw, err = callContractWithRetryAtBlock(client, callCtx, msg, block)
	} else {
		raw, err = callContractWithRetry(client, callCtx, msg)
	}
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "revert") {
			v4Debugf("getTickInfo reverted for tick %d: %v, returning zeros", tick, err)
			return big.NewInt(0), big.NewInt(0), nil
		}
		v4Debugf("getTickInfo failed for tick %d: %v", tick, err)
		return nil, nil, fmt.Errorf("call getTickInfo failed for tick %d: %w", tick, err)
	}

	out, err := parsedABI.Unpack("getTickInfo", raw)
	if err != nil {
		return nil, nil, fmt.Errorf("unpack getTickInfo failed: %w", err)
	}
	if len(out) < 4 {
		return nil, nil, fmt.Errorf("unexpected getTickInfo return length: %d", len(out))
	}

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
