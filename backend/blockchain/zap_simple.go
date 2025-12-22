package blockchain

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ZapSimpleABI is the ABI for the simplified Zap contract
const ZapSimpleABI = `[
  {
    "inputs": [],
    "stateMutability": "nonpayable",
    "type": "constructor"
  },
  {
    "anonymous": false,
    "inputs": [
      { "indexed": true, "internalType": "address", "name": "user", "type": "address" },
      { "indexed": true, "internalType": "address", "name": "pool", "type": "address" },
      { "indexed": true, "internalType": "uint256", "name": "tokenId", "type": "uint256" },
      { "indexed": false, "internalType": "uint256", "name": "amount0", "type": "uint256" },
      { "indexed": false, "internalType": "uint256", "name": "amount1", "type": "uint256" },
      { "indexed": false, "internalType": "uint128", "name": "liquidity", "type": "uint128" }
    ],
    "name": "ZapInV3",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      { "indexed": true, "internalType": "address", "name": "user", "type": "address" },
      { "indexed": true, "internalType": "bytes32", "name": "poolId", "type": "bytes32" },
      { "indexed": true, "internalType": "uint256", "name": "tokenId", "type": "uint256" },
      { "indexed": false, "internalType": "uint256", "name": "amount0", "type": "uint256" },
      { "indexed": false, "internalType": "uint256", "name": "amount1", "type": "uint256" },
      { "indexed": false, "internalType": "uint128", "name": "liquidity", "type": "uint128" }
    ],
    "name": "ZapInV4",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      { "indexed": true, "internalType": "address", "name": "user", "type": "address" },
      { "indexed": true, "internalType": "uint256", "name": "tokenId", "type": "uint256" },
      { "indexed": false, "internalType": "uint256", "name": "amount0", "type": "uint256" },
      { "indexed": false, "internalType": "uint256", "name": "amount1", "type": "uint256" }
    ],
    "name": "ZapOutV3",
    "type": "event"
  },
  {
    "inputs": [
      { "internalType": "address", "name": "pool", "type": "address" },
      { "internalType": "int24", "name": "tickLower", "type": "int24" },
      { "internalType": "int24", "name": "tickUpper", "type": "int24" },
      { "internalType": "uint256", "name": "amount0In", "type": "uint256" },
      { "internalType": "uint256", "name": "amount1In", "type": "uint256" }
    ],
    "name": "calculateOptimalSwap",
    "outputs": [
      { "internalType": "bool", "name": "zeroForOne", "type": "bool" },
      { "internalType": "uint256", "name": "amountToSwap", "type": "uint256" }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      {
        "components": [
          { "internalType": "address", "name": "pool", "type": "address" },
          { "internalType": "address", "name": "positionManager", "type": "address" },
          { "internalType": "address", "name": "token0", "type": "address" },
          { "internalType": "address", "name": "token1", "type": "address" },
          { "internalType": "int24", "name": "tickLower", "type": "int24" },
          { "internalType": "int24", "name": "tickUpper", "type": "int24" },
          { "internalType": "address", "name": "recipient", "type": "address" },
          { "internalType": "uint256", "name": "amount0In", "type": "uint256" },
          { "internalType": "uint256", "name": "amount1In", "type": "uint256" },
          { "internalType": "uint256", "name": "slippageBps", "type": "uint256" },
          {
            "components": [
              { "internalType": "address", "name": "target", "type": "address" },
              { "internalType": "address", "name": "approveTarget", "type": "address" },
              { "internalType": "address", "name": "tokenIn", "type": "address" },
              { "internalType": "address", "name": "tokenOut", "type": "address" },
              { "internalType": "uint256", "name": "amountIn", "type": "uint256" },
              { "internalType": "uint256", "name": "minAmountOut", "type": "uint256" },
              { "internalType": "bytes", "name": "callData", "type": "bytes" }
            ],
            "internalType": "struct ZapSimple.SwapParams",
            "name": "swap",
            "type": "tuple"
          }
        ],
        "internalType": "struct ZapSimple.ZapInV3Params",
        "name": "params",
        "type": "tuple"
      }
    ],
    "name": "zapInV3",
    "outputs": [
      {
        "components": [
          { "internalType": "uint256", "name": "tokenId", "type": "uint256" },
          { "internalType": "uint128", "name": "liquidity", "type": "uint128" },
          { "internalType": "uint256", "name": "amount0Used", "type": "uint256" },
          { "internalType": "uint256", "name": "amount1Used", "type": "uint256" },
          { "internalType": "uint256", "name": "dust0", "type": "uint256" },
          { "internalType": "uint256", "name": "dust1", "type": "uint256" }
        ],
        "internalType": "struct ZapSimple.ZapResult",
        "name": "result",
        "type": "tuple"
      }
    ],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [
      { "internalType": "address", "name": "positionManager", "type": "address" },
      { "internalType": "uint256", "name": "tokenId", "type": "uint256" },
      { "internalType": "address", "name": "recipient", "type": "address" },
      { "internalType": "uint256", "name": "amount0Min", "type": "uint256" },
      { "internalType": "uint256", "name": "amount1Min", "type": "uint256" }
    ],
    "name": "zapOutV3",
    "outputs": [
      { "internalType": "uint256", "name": "amount0", "type": "uint256" },
      { "internalType": "uint256", "name": "amount1", "type": "uint256" }
    ],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [
      {
        "components": [
          {
            "components": [
              { "internalType": "address", "name": "currency0", "type": "address" },
              { "internalType": "address", "name": "currency1", "type": "address" },
              { "internalType": "uint24", "name": "fee", "type": "uint24" },
              { "internalType": "int24", "name": "tickSpacing", "type": "int24" },
              { "internalType": "address", "name": "hooks", "type": "address" }
            ],
            "internalType": "struct ZapSimple.PoolKey",
            "name": "poolKey",
            "type": "tuple"
          },
          { "internalType": "address", "name": "stateView", "type": "address" },
          { "internalType": "address", "name": "positionManager", "type": "address" },
          { "internalType": "int24", "name": "tickLower", "type": "int24" },
          { "internalType": "int24", "name": "tickUpper", "type": "int24" },
          { "internalType": "address", "name": "recipient", "type": "address" },
          { "internalType": "uint256", "name": "amount0In", "type": "uint256" },
          { "internalType": "uint256", "name": "amount1In", "type": "uint256" },
          { "internalType": "uint256", "name": "slippageBps", "type": "uint256" },
          {
            "components": [
              { "internalType": "address", "name": "target", "type": "address" },
              { "internalType": "address", "name": "approveTarget", "type": "address" },
              { "internalType": "address", "name": "tokenIn", "type": "address" },
              { "internalType": "address", "name": "tokenOut", "type": "address" },
              { "internalType": "uint256", "name": "amountIn", "type": "uint256" },
              { "internalType": "uint256", "name": "minAmountOut", "type": "uint256" },
              { "internalType": "bytes", "name": "callData", "type": "bytes" }
            ],
            "internalType": "struct ZapSimple.SwapParams",
            "name": "swap",
            "type": "tuple"
          },
          { "internalType": "uint160", "name": "sqrtPriceX96", "type": "uint160" },
          { "internalType": "uint256", "name": "maxDustBps", "type": "uint256" }
        ],
        "internalType": "struct ZapSimple.ZapInV4Params",
        "name": "params",
        "type": "tuple"
      }
    ],
    "name": "zapInV4",
    "outputs": [
      {
        "components": [
          { "internalType": "uint256", "name": "tokenId", "type": "uint256" },
          { "internalType": "uint128", "name": "liquidity", "type": "uint128" },
          { "internalType": "uint256", "name": "amount0Used", "type": "uint256" },
          { "internalType": "uint256", "name": "amount1Used", "type": "uint256" },
          { "internalType": "uint256", "name": "dust0", "type": "uint256" },
          { "internalType": "uint256", "name": "dust1", "type": "uint256" }
        ],
        "internalType": "struct ZapSimple.ZapResult",
        "name": "result",
        "type": "tuple"
      }
    ],
    "stateMutability": "nonpayable",
    "type": "function"
  }
]`

// SwapParamsSimple OKX swap 参数
type SwapParamsSimple struct {
	Target        common.Address `abi:"target"`
	ApproveTarget common.Address `abi:"approveTarget"`
	TokenIn       common.Address `abi:"tokenIn"`
	TokenOut      common.Address `abi:"tokenOut"`
	AmountIn      *big.Int       `abi:"amountIn"`
	MinAmountOut  *big.Int       `abi:"minAmountOut"`
	CallData      []byte         `abi:"callData"`
}

// PoolKeySimple V4 PoolKey
type PoolKeySimple struct {
	Currency0   common.Address `abi:"currency0"`
	Currency1   common.Address `abi:"currency1"`
	Fee         *big.Int       `abi:"fee"`         // uint24 -> *big.Int for ABI compatibility
	TickSpacing *big.Int       `abi:"tickSpacing"` // int24 -> *big.Int for ABI compatibility
	Hooks       common.Address `abi:"hooks"`
}

// ZapInV3ParamsSimple V3 开仓参数
type ZapInV3ParamsSimple struct {
	Pool            common.Address   `abi:"pool"`
	PositionManager common.Address   `abi:"positionManager"`
	Token0          common.Address   `abi:"token0"`
	Token1          common.Address   `abi:"token1"`
	TickLower       *big.Int         `abi:"tickLower"` // int24
	TickUpper       *big.Int         `abi:"tickUpper"` // int24
	Recipient       common.Address   `abi:"recipient"`
	Amount0In       *big.Int         `abi:"amount0In"`
	Amount1In       *big.Int         `abi:"amount1In"`
	SlippageBps     *big.Int         `abi:"slippageBps"`
	Swap            SwapParamsSimple `abi:"swap"`
}

// ZapInV4ParamsSimple V4 开仓参数 (与合约 ZapInV4Params 匹配)
type ZapInV4ParamsSimple struct {
	PoolKey         PoolKeySimple    `abi:"poolKey"`
	StateView       common.Address   `abi:"stateView"`
	PositionManager common.Address   `abi:"positionManager"`
	TickLower       *big.Int         `abi:"tickLower"` // int24 -> *big.Int for ABI compatibility
	TickUpper       *big.Int         `abi:"tickUpper"` // int24 -> *big.Int for ABI compatibility
	Recipient       common.Address   `abi:"recipient"`
	Amount0In       *big.Int         `abi:"amount0In"`
	Amount1In       *big.Int         `abi:"amount1In"`
	SlippageBps     *big.Int         `abi:"slippageBps"`
	Swap            SwapParamsSimple `abi:"swap"`
	SqrtPriceX96    *big.Int         `abi:"sqrtPriceX96"` // uint160 -> *big.Int, 从 Go 传入的当前价格
	MaxDustBps      *big.Int         `abi:"maxDustBps"`   // 最大 dust 容忍度 (100 = 1%)
}

// ZapResultSimple 结果
type ZapResultSimple struct {
	TokenId     *big.Int `abi:"tokenId"`
	Liquidity   *big.Int `abi:"liquidity"` // uint128
	Amount0Used *big.Int `abi:"amount0Used"`
	Amount1Used *big.Int `abi:"amount1Used"`
	Dust0       *big.Int `abi:"dust0"`
	Dust1       *big.Int `abi:"dust1"`
}

// ZapSimple 简化版 Zap 合约绑定
type ZapSimple struct {
	contract *bind.BoundContract
	address  common.Address
}

// NewZapSimple 创建新的 ZapSimple 实例
func NewZapSimple(address common.Address, client *ethclient.Client) (*ZapSimple, error) {
	parsed, err := abi.JSON(strings.NewReader(ZapSimpleABI))
	if err != nil {
		return nil, err
	}
	contract := bind.NewBoundContract(address, parsed, client, client, client)
	return &ZapSimple{contract: contract, address: address}, nil
}

// Address 返回合约地址
func (z *ZapSimple) Address() common.Address {
	return z.address
}

// CalculateOptimalSwap 计算最优 swap 数量
func (z *ZapSimple) CalculateOptimalSwap(
	pool common.Address,
	tickLower, tickUpper *big.Int,
	amount0In, amount1In *big.Int,
) (zeroForOne bool, amountToSwap *big.Int, err error) {
	var out []interface{}
	err = z.contract.Call(nil, &out, "calculateOptimalSwap", pool, tickLower, tickUpper, amount0In, amount1In)
	if err != nil {
		return false, nil, err
	}
	if len(out) < 2 {
		return false, nil, nil
	}
	zeroForOne, _ = out[0].(bool)
	amountToSwap, _ = out[1].(*big.Int)
	return zeroForOne, amountToSwap, nil
}

// ZapInV3 V3 开仓
func (z *ZapSimple) ZapInV3(opts *bind.TransactOpts, params ZapInV3ParamsSimple) (*types.Transaction, error) {
	return z.contract.Transact(opts, "zapInV3", params)
}

// ZapInV4 V4 开仓
func (z *ZapSimple) ZapInV4(opts *bind.TransactOpts, params ZapInV4ParamsSimple) (*types.Transaction, error) {
	return z.contract.Transact(opts, "zapInV4", params)
}

// ZapOutV3 V3 撤仓
func (z *ZapSimple) ZapOutV3(opts *bind.TransactOpts, positionManager common.Address, tokenId *big.Int, recipient common.Address, amount0Min *big.Int, amount1Min *big.Int) (*types.Transaction, error) {
	return z.contract.Transact(opts, "zapOutV3", positionManager, tokenId, recipient, amount0Min, amount1Min)
}
