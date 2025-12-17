package blockchain

import (
	"context"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
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
	if Client == nil {
		return common.Address{}, fmt.Errorf("blockchain client not initialized")
	}

	parsed, err := abi.JSON(strings.NewReader(v3FactoryABI))
	if err != nil {
		return common.Address{}, err
	}

	data, err := parsed.Pack("getPool", tokenA, tokenB, uint32(fee))
	if err != nil {
		return common.Address{}, err
	}

	msg := ethereum.CallMsg{To: &factory, Data: data}
	raw, err := Client.CallContract(context.Background(), msg, nil)
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
