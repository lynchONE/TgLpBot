package blockchain

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// OkxSwapTx represents an arbitrary swap transaction payload returned by OKX DEX /swap API.
// It is used for validation/logging and for passing around tx call data.
type OkxSwapTx struct {
	To    common.Address `abi:"to"`
	Value *big.Int       `abi:"value"`
	Data  []byte         `abi:"data"`
}
