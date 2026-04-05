package blockchain

import (
	_ "embed"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

//go:embed abi/atomic_increase_zap_abi.json
var AtomicIncreaseZapABI string

type FundingParamsSimple struct {
	Token  common.Address `abi:"token"`
	Amount *big.Int       `abi:"amount"`
}

type ZapIncreaseV3ParamsSimple struct {
	Pool            common.Address      `abi:"pool"`
	PositionManager common.Address      `abi:"positionManager"`
	TokenId         *big.Int            `abi:"tokenId"`
	Funding         FundingParamsSimple `abi:"funding"`
	EntrySwap       SwapParamsSimple    `abi:"entrySwap"`
	RebalanceSwap   SwapParamsSimple    `abi:"rebalanceSwap"`
}

type ZapIncreaseV4ParamsSimple struct {
	PoolKey         PoolKeySimple       `abi:"poolKey"`
	StateView       common.Address      `abi:"stateView"`
	PositionManager common.Address      `abi:"positionManager"`
	TokenId         *big.Int            `abi:"tokenId"`
	TickLower       *big.Int            `abi:"tickLower"`
	TickUpper       *big.Int            `abi:"tickUpper"`
	SlippageBps     *big.Int            `abi:"slippageBps"`
	Funding         FundingParamsSimple `abi:"funding"`
	EntrySwap       SwapParamsSimple    `abi:"entrySwap"`
	RebalanceSwap   SwapParamsSimple    `abi:"rebalanceSwap"`
	SqrtPriceX96    *big.Int            `abi:"sqrtPriceX96"`
}

type AtomicIncreaseZap struct {
	contract *bind.BoundContract
	address  common.Address
}

func NewAtomicIncreaseZap(address common.Address, client *ethclient.Client) (*AtomicIncreaseZap, error) {
	parsed, err := abi.JSON(strings.NewReader(AtomicIncreaseZapABI))
	if err != nil {
		return nil, err
	}
	rc := wrapRPCRetryClient(client)
	contract := bind.NewBoundContract(address, parsed, rc, rc, rc)
	return &AtomicIncreaseZap{contract: contract, address: address}, nil
}

func (z *AtomicIncreaseZap) Address() common.Address {
	return z.address
}

func (z *AtomicIncreaseZap) ZapIncreaseV3(opts *bind.TransactOpts, params ZapIncreaseV3ParamsSimple) (*types.Transaction, error) {
	return z.contract.Transact(opts, "zapIncreaseV3", params)
}

func (z *AtomicIncreaseZap) ZapIncreaseV4(opts *bind.TransactOpts, params ZapIncreaseV4ParamsSimple) (*types.Transaction, error) {
	return z.contract.Transact(opts, "zapIncreaseV4", params)
}

func (z *AtomicIncreaseZap) SimulateZapIncreaseV3(opts *bind.CallOpts, params ZapIncreaseV3ParamsSimple) (*ZapResultSimple, error) {
	var out []interface{}
	if err := z.contract.Call(opts, &out, "zapIncreaseV3", params); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty zapIncreaseV3 call result")
	}
	return decodeZapResultSimple(out[0])
}

func (z *AtomicIncreaseZap) SimulateZapIncreaseV4(opts *bind.CallOpts, params ZapIncreaseV4ParamsSimple) (*ZapResultSimple, error) {
	var out []interface{}
	if err := z.contract.Call(opts, &out, "zapIncreaseV4", params); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty zapIncreaseV4 call result")
	}
	return decodeZapResultSimple(out[0])
}
