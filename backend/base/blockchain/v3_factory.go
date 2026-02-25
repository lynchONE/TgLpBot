package blockchain

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

const v3FactoryABI = `[
  {
    "inputs": [
      { "internalType": "address", "name": "tokenA", "type": "address" },
      { "internalType": "address", "name": "tokenB", "type": "address" },
      { "internalType": "uint24", "name": "fee", "type": "uint24" }
    ],
    "name": "getPool",
    "outputs": [
      { "internalType": "address", "name": "pool", "type": "address" }
    ],
    "stateMutability": "view",
    "type": "function"
  }
]`

func GetV3PoolFromFactory(factory common.Address, tokenA common.Address, tokenB common.Address, fee uint64) (common.Address, error) {
	return GetV3PoolFromFactoryCtxWithClient(Client, context.Background(), factory, tokenA, tokenB, fee)
}

func GetV3PoolFromFactoryCtx(ctx context.Context, factory common.Address, tokenA common.Address, tokenB common.Address, fee uint64) (common.Address, error) {
	return GetV3PoolFromFactoryCtxWithClient(Client, ctx, factory, tokenA, tokenB, fee)
}

func GetV3PoolFromFactoryWithClient(client *ethclient.Client, factory common.Address, tokenA common.Address, tokenB common.Address, fee uint64) (common.Address, error) {
	return GetV3PoolFromFactoryCtxWithClient(client, context.Background(), factory, tokenA, tokenB, fee)
}

func GetV3PoolFromFactoryCtxWithClient(client *ethclient.Client, ctx context.Context, factory common.Address, tokenA common.Address, tokenB common.Address, fee uint64) (common.Address, error) {
	if client == nil {
		return common.Address{}, fmt.Errorf("blockchain client not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	parsed, err := abi.JSON(strings.NewReader(v3FactoryABI))
	if err != nil {
		return common.Address{}, err
	}

	data, err := parsed.Pack("getPool", tokenA, tokenB, new(big.Int).SetUint64(fee))
	if err != nil {
		return common.Address{}, err
	}

	msg := ethereum.CallMsg{To: &factory, Data: data}
	raw, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return common.Address{}, err
	}

	out, err := parsed.Unpack("getPool", raw)
	if err != nil {
		return common.Address{}, err
	}
	if len(out) != 1 {
		return common.Address{}, fmt.Errorf("unexpected getPool return length: %d", len(out))
	}
	if addr, ok := out[0].(common.Address); ok {
		return addr, nil
	}
	if b, ok := out[0].([20]byte); ok {
		return common.BytesToAddress(b[:]), nil
	}
	return common.Address{}, fmt.Errorf("unexpected getPool return type: %T", out[0])
}

// GetV3PoolFactory returns the factory address of a V3 pool by calling pool.factory()
func GetV3PoolFactory(poolAddress common.Address) (common.Address, error) {
	return GetV3PoolFactoryWithClient(Client, poolAddress)
}

func GetV3PoolFactoryWithClient(client *ethclient.Client, poolAddress common.Address) (common.Address, error) {
	if client == nil {
		return common.Address{}, fmt.Errorf("blockchain client not initialized")
	}

	const factoryABI = `[{
		"inputs": [],
		"name": "factory",
		"outputs": [{"internalType": "address", "name": "", "type": "address"}],
		"stateMutability": "view",
		"type": "function"
	}]`

	parsed, err := abi.JSON(strings.NewReader(factoryABI))
	if err != nil {
		return common.Address{}, fmt.Errorf("parse factory ABI failed: %w", err)
	}

	data, err := parsed.Pack("factory")
	if err != nil {
		return common.Address{}, fmt.Errorf("pack factory failed: %w", err)
	}

	msg := ethereum.CallMsg{To: &poolAddress, Data: data}
	raw, err := client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return common.Address{}, fmt.Errorf("call factory failed: %w", err)
	}

	out, err := parsed.Unpack("factory", raw)
	if err != nil {
		return common.Address{}, fmt.Errorf("unpack factory failed: %w", err)
	}

	if len(out) != 1 {
		return common.Address{}, fmt.Errorf("unexpected factory return length: %d", len(out))
	}

	if addr, ok := out[0].(common.Address); ok {
		return addr, nil
	}
	if b, ok := out[0].([20]byte); ok {
		return common.BytesToAddress(b[:]), nil
	}

	return common.Address{}, fmt.Errorf("unexpected factory return type: %T", out[0])
}
