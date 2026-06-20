package blockchain

import (
	"context"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// DefaultMulticall3Address is the canonical Multicall3 deployment address, which
// is identical across most EVM chains (BSC 56, Base 8453, ...).
// See https://github.com/mds1/multicall.
var DefaultMulticall3Address = common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")

const multicall3ABI = `[
  {
    "inputs": [
      {
        "components": [
          { "internalType": "address", "name": "target", "type": "address" },
          { "internalType": "bool", "name": "allowFailure", "type": "bool" },
          { "internalType": "bytes", "name": "callData", "type": "bytes" }
        ],
        "internalType": "struct Multicall3.Call3[]",
        "name": "calls",
        "type": "tuple[]"
      }
    ],
    "name": "aggregate3",
    "outputs": [
      {
        "components": [
          { "internalType": "bool", "name": "success", "type": "bool" },
          { "internalType": "bytes", "name": "returnData", "type": "bytes" }
        ],
        "internalType": "struct Multicall3.Result[]",
        "name": "returnData",
        "type": "tuple[]"
      }
    ],
    "stateMutability": "view",
    "type": "function"
  }
]`

// Multicall3Call is a single sub-call within an aggregate3 batch.
type Multicall3Call struct {
	Target       common.Address `abi:"target"`
	AllowFailure bool           `abi:"allowFailure"`
	CallData     []byte         `abi:"callData"`
}

// Multicall3Result is the per-call outcome returned by aggregate3.
type Multicall3Result struct {
	Success    bool   `abi:"success"`
	ReturnData []byte `abi:"returnData"`
}

// Aggregate3 batches multiple read-only calls into a single eth_call via the
// Multicall3 contract. allowFailure is honoured per-call, so a reverting
// sub-call yields Success=false rather than failing the whole batch. Pass a
// zero multicallAddr to use DefaultMulticall3Address.
func Aggregate3(ctx context.Context, client *ethclient.Client, multicallAddr common.Address, calls []Multicall3Call) ([]Multicall3Result, error) {
	if client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if len(calls) == 0 {
		return nil, nil
	}
	if multicallAddr == (common.Address{}) {
		multicallAddr = DefaultMulticall3Address
	}
	parsed, err := abi.JSON(strings.NewReader(multicall3ABI))
	if err != nil {
		return nil, err
	}
	input, err := parsed.Pack("aggregate3", calls)
	if err != nil {
		return nil, fmt.Errorf("pack aggregate3: %w", err)
	}

	rc := wrapRPCRetryClient(client)
	output, err := rc.CallContract(ctx, ethereum.CallMsg{To: &multicallAddr, Data: input}, nil)
	if err != nil {
		return nil, err
	}

	var decoded struct {
		ReturnData []Multicall3Result
	}
	if err := parsed.UnpackIntoInterface(&decoded, "aggregate3", output); err != nil {
		return nil, fmt.Errorf("unpack aggregate3: %w", err)
	}
	return decoded.ReturnData, nil
}
