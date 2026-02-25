package blockchain

import (
	"TgLpBot/base/config"
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Minimal Uniswap V4 PoolManager ABI surface for reading current tick.
// Different deployments have used different method names; we try both `getSlot0` and `slot0`.
const uniswapV4PoolManagerABI = `[
  {
    "inputs": [{ "internalType": "bytes32", "name": "poolId", "type": "bytes32" }],
    "name": "getSlot0",
    "outputs": [
      { "internalType": "uint160", "name": "sqrtPriceX96", "type": "uint160" },
      { "internalType": "int24", "name": "tick", "type": "int24" },
      { "internalType": "uint24", "name": "protocolFee", "type": "uint24" },
      { "internalType": "uint24", "name": "lpFee", "type": "uint24" }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{ "internalType": "bytes32", "name": "poolId", "type": "bytes32" }],
    "name": "slot0",
    "outputs": [
      { "internalType": "uint160", "name": "sqrtPriceX96", "type": "uint160" },
      { "internalType": "int24", "name": "tick", "type": "int24" },
      { "internalType": "uint24", "name": "protocolFee", "type": "uint24" },
      { "internalType": "uint24", "name": "lpFee", "type": "uint24" }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{ "internalType": "bytes32", "name": "poolId", "type": "bytes32" }],
    "name": "poolKeys",
    "outputs": [
      { "internalType": "address", "name": "currency0", "type": "address" },
      { "internalType": "address", "name": "currency1", "type": "address" },
      { "internalType": "uint24", "name": "fee", "type": "uint24" },
      { "internalType": "int24", "name": "tickSpacing", "type": "int24" },
      { "internalType": "address", "name": "hooks", "type": "address" }
    ],
    "stateMutability": "view",
    "type": "function"
  }
]`

const uniswapV4StateViewABISingleArg = `[
  {
    "inputs": [
      { "internalType": "bytes32", "name": "poolId", "type": "bytes32" }
    ],
    "name": "getSlot0",
    "outputs": [
      { "internalType": "uint160", "name": "sqrtPriceX96", "type": "uint160" },
      { "internalType": "int24", "name": "tick", "type": "int24" },
      { "internalType": "uint24", "name": "protocolFee", "type": "uint24" },
      { "internalType": "uint24", "name": "lpFee", "type": "uint24" }
    ],
    "stateMutability": "view",
    "type": "function"
  }
]`

const uniswapV4PoolManagerInitializeEventABI = `[
  {
    "anonymous": false,
    "inputs": [
      { "indexed": true, "internalType": "bytes32", "name": "id", "type": "bytes32" },
      { "indexed": true, "internalType": "address", "name": "currency0", "type": "address" },
      { "indexed": true, "internalType": "address", "name": "currency1", "type": "address" },
      { "indexed": false, "internalType": "uint24", "name": "fee", "type": "uint24" },
      { "indexed": false, "internalType": "int24", "name": "tickSpacing", "type": "int24" },
      { "indexed": false, "internalType": "address", "name": "hooks", "type": "address" },
      { "indexed": false, "internalType": "uint160", "name": "sqrtPriceX96", "type": "uint160" },
      { "indexed": false, "internalType": "int24", "name": "tick", "type": "int24" }
    ],
    "name": "Initialize",
    "type": "event"
  }
]`

// Many V4 deployments don't expose PoolManager.poolKeys(poolId), but PositionManager stores poolKeys (keyed by bytes25(poolId)).
// This is the most reliable, low-cost way to resolve PoolKey components off-chain.
const uniswapV4PositionManagerPoolKeysABI = `[
  {
    "inputs": [{ "internalType": "bytes25", "name": "poolId", "type": "bytes25" }],
    "name": "poolKeys",
    "outputs": [
      { "internalType": "address", "name": "currency0", "type": "address" },
      { "internalType": "address", "name": "currency1", "type": "address" },
      { "internalType": "uint24", "name": "fee", "type": "uint24" },
      { "internalType": "int24", "name": "tickSpacing", "type": "int24" },
      { "internalType": "address", "name": "hooks", "type": "address" }
    ],
    "stateMutability": "view",
    "type": "function"
  }
]`

func v4DebugEnabled() bool {
	// Prefer config flag (ensures .env was actually loaded / config printed)
	if config.AppConfig != nil && config.AppConfig.UniswapV4Debug {
		return true
	}
	v := strings.TrimSpace(os.Getenv("UNISWAP_V4_DEBUG"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func v4Debugf(format string, args ...interface{}) {
	if v4DebugEnabled() {
		log.Printf("[V4] "+format, args...)
	}
}

func normalizePoolID(poolID string) (common.Hash, error) {
	poolID = strings.TrimSpace(poolID)
	if !strings.HasPrefix(poolID, "0x") && !strings.HasPrefix(poolID, "0X") {
		poolID = "0x" + poolID
	}
	if len(poolID) != 66 {
		return common.Hash{}, fmt.Errorf("invalid PoolId length: %d", len(poolID))
	}
	// Hex sanity check: common.HexToHash tolerates odd inputs; we prefer to fail fast.
	for _, c := range poolID[2:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return common.Hash{}, fmt.Errorf("invalid PoolId hex")
		}
	}
	return common.HexToHash(poolID), nil
}

func poolIDToBytes25(poolID string) ([25]byte, common.Hash, error) {
	var out [25]byte
	id, err := normalizePoolID(poolID)
	if err != nil {
		return out, common.Hash{}, err
	}
	b := id.Bytes()
	copy(out[:], b[:25])
	return out, id, nil
}

func computeUniswapV4PoolID(currency0, currency1 common.Address, fee uint64, tickSpacing int, hooks common.Address) (common.Hash, error) {
	uint24Ty, err := abi.NewType("uint24", "", nil)
	if err != nil {
		return common.Hash{}, err
	}
	int24Ty, err := abi.NewType("int24", "", nil)
	if err != nil {
		return common.Hash{}, err
	}
	addressTy, err := abi.NewType("address", "", nil)
	if err != nil {
		return common.Hash{}, err
	}

	encoded, err := abi.Arguments{
		{Type: addressTy},
		{Type: addressTy},
		{Type: uint24Ty},
		{Type: int24Ty},
		{Type: addressTy},
	}.Pack(currency0, currency1, new(big.Int).SetUint64(fee), big.NewInt(int64(tickSpacing)), hooks)
	if err != nil {
		return common.Hash{}, err
	}
	return common.BytesToHash(crypto.Keccak256(encoded)), nil
}

func isEthGetLogsRangeLimited(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// Alchemy Free tier: "eth_getLogs requests with up to a 10 block range"
	if strings.Contains(msg, "eth_getLogs") && strings.Contains(msg, "10 block range") {
		return true
	}
	return false
}

func filterLogsWithFallback(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	if Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	logs, err := Client.FilterLogs(ctx, query)
	if err == nil {
		return logs, nil
	}
	if !isEthGetLogsRangeLimited(err) {
		return nil, err
	}

	// Fallback to a public RPC for log queries (keeps the main RPC for tx/reads).
	fallbackURL := strings.TrimSpace(os.Getenv("BSC_RPC_URL_LOGS"))
	if fallbackURL == "" {
		fallbackURL = "https://bsc-dataseed.binance.org/"
	}
	fallbackClient, dialErr := ethclient.Dial(fallbackURL)
	if dialErr != nil {
		return nil, err
	}
	defer fallbackClient.Close()
	return fallbackClient.FilterLogs(ctx, query)
}

// GetUniswapV4PoolKeyFromPositionManager reads PositionManager.poolKeys(bytes25(poolId)).
// Note: the mapping may be unset (tickSpacing=0) until the first position is minted via this PositionManager.
func GetUniswapV4PoolKeyFromPositionManager(positionManager common.Address, poolID string) (common.Address, common.Address, uint64, int, common.Address, error) {
	return GetUniswapV4PoolKeyFromPositionManagerCtx(context.Background(), positionManager, poolID)
}

// GetUniswapV4PoolKeyFromPositionManagerCtx reads PositionManager.poolKeys(bytes25(poolId)) with a caller-provided context.
func GetUniswapV4PoolKeyFromPositionManagerCtx(ctx context.Context, positionManager common.Address, poolID string) (common.Address, common.Address, uint64, int, common.Address, error) {
	if Client == nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("blockchain client not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if (positionManager == common.Address{}) {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("uniswap v4 position manager address not configured")
	}

	poolId25, fullID, err := poolIDToBytes25(poolID)
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, err
	}

	parsedABI, err := abi.JSON(strings.NewReader(uniswapV4PositionManagerPoolKeysABI))
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("parse position manager ABI failed: %w", err)
	}

	data, err := parsedABI.Pack("poolKeys", poolId25)
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("pack positionManager.poolKeys failed: %w", err)
	}

	v4Debugf("posm poolKeys: PositionManager=%s PoolId=%s (bytes25=%x)", positionManager.Hex(), poolID, poolId25)
	msg := ethereum.CallMsg{To: &positionManager, Data: data}
	raw, err := Client.CallContract(ctx, msg, nil)
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("call positionManager.poolKeys failed: %w", err)
	}

	out, err := parsedABI.Unpack("poolKeys", raw)
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("unpack positionManager.poolKeys failed: %w", err)
	}
	if len(out) < 5 {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("unexpected positionManager.poolKeys return length: %d", len(out))
	}

	c0, _ := out[0].(common.Address)
	c1, _ := out[1].(common.Address)
	feeBig, _ := out[2].(*big.Int)
	tickSpacingBig, _ := out[3].(*big.Int)
	hooks, _ := out[4].(common.Address)

	var fee uint64
	if feeBig != nil {
		fee = feeBig.Uint64()
	}
	if tickSpacingBig == nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("unexpected tickSpacing type: %T", out[3])
	}
	tickSpacing := int(tickSpacingBig.Int64())

	// When the mapping isn't set yet, tickSpacing is 0 and currencies are zero.
	if tickSpacing == 0 || (c0 == common.Address{} && c1 == common.Address{}) {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("positionManager.poolKeys not set for PoolId=%s (need at least 1 mint via this PositionManager)", poolID)
	}

	// Sanity check: derived PoolId must match the input PoolId (avoid bytes25 collisions / wrong mapping).
	derivedID, derr := computeUniswapV4PoolID(c0, c1, fee, tickSpacing, hooks)
	if derr != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("compute poolId failed: %w", derr)
	}
	if derivedID != fullID {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("poolId mismatch: expected=%s derived=%s (PositionManager.poolKeys bytes25 collision?)", fullID.Hex(), derivedID.Hex())
	}

	v4Debugf("posm poolKeys: poolId=%s fullId=%s currency0=%s currency1=%s fee=%d tickSpacing=%d hooks=%s", poolID, fullID.Hex(), c0.Hex(), c1.Hex(), fee, tickSpacing, hooks.Hex())
	return c0, c1, fee, tickSpacing, hooks, nil
}

// GetUniswapV4PoolKeyFromInitializeEvent resolves a V4 PoolKey by reading the PoolManager.Initialize event.
// Many V4 deployments do NOT expose a poolKeys(poolId) view, so logs are the only reliable on-chain source.
func GetUniswapV4PoolKeyFromInitializeEvent(poolManager common.Address, poolID string) (common.Address, common.Address, uint64, int, common.Address, error) {
	if Client == nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("blockchain client not initialized")
	}
	if (poolManager == common.Address{}) {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("uniswap v4 pool manager address not configured")
	}

	parsedABI, err := abi.JSON(strings.NewReader(uniswapV4PoolManagerInitializeEventABI))
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("parse pool manager Initialize event ABI failed: %w", err)
	}

	event, ok := parsedABI.Events["Initialize"]
	if !ok {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("Initialize event not found in ABI")
	}

	id, err := normalizePoolID(poolID)
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, err
	}

	// topics: [InitializeSig, PoolId]
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(0),
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{{event.ID}, {id}},
	}

	callCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var logs []types.Log
	logs, err = filterLogsWithFallback(callCtx, query)
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("filter Initialize logs failed: %w", err)
	}
	if len(logs) == 0 {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("Initialize event not found for PoolId=%s", poolID)
	}

	// There should only be one Initialize per pool. Use the last one defensively.
	lg := logs[len(logs)-1]
	if len(lg.Topics) < 4 {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("unexpected Initialize topics length: %d", len(lg.Topics))
	}

	c0 := common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	c1 := common.BytesToAddress(lg.Topics[3].Bytes()[12:])

	out, err := parsedABI.Unpack("Initialize", lg.Data)
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("unpack Initialize event failed: %w", err)
	}
	if len(out) < 5 {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("unexpected Initialize data fields: %d", len(out))
	}

	feeBig, _ := out[0].(*big.Int)
	tickSpacingBig, _ := out[1].(*big.Int)
	hooks, _ := out[2].(common.Address)

	var fee uint64
	if feeBig != nil {
		fee = feeBig.Uint64()
	}
	if tickSpacingBig == nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("unexpected tickSpacing type: %T", out[1])
	}

	tickSpacing := int(tickSpacingBig.Int64())
	v4Debugf("Initialize: poolId=%s currency0=%s currency1=%s fee=%d tickSpacing=%d hooks=%s", poolID, c0.Hex(), c1.Hex(), fee, tickSpacing, hooks.Hex())
	return c0, c1, fee, tickSpacing, hooks, nil
}

// GetUniswapV4PoolCurrentTick reads the current tick for a Uniswap V4 pool from the PoolManager.
func GetUniswapV4PoolCurrentTick(poolManager common.Address, poolID string) (int, error) {
	if Client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}
	if (poolManager == common.Address{}) {
		return 0, fmt.Errorf("uniswap v4 pool manager address not configured")
	}

	parsedABI, err := abi.JSON(strings.NewReader(uniswapV4PoolManagerABI))
	if err != nil {
		return 0, fmt.Errorf("parse pool manager ABI failed: %w", err)
	}

	id, err := normalizePoolID(poolID)
	if err != nil {
		return 0, err
	}

	v4Debugf("tick: PoolManager=%s PoolId=%s", poolManager.Hex(), poolID)
	code, codeErr := Client.CodeAt(context.Background(), poolManager, nil)
	if codeErr == nil {
		v4Debugf("tick: PoolManager code size=%d bytes", len(code))
	}
	if codeErr == nil && len(code) == 0 {
		return 0, fmt.Errorf("pool manager has no code at %s (wrong network or wrong address)", poolManager.Hex())
	}

	// Try getSlot0 first, then slot0 for compatibility.
	for _, method := range []string{"getSlot0", "slot0"} {
		data, packErr := parsedABI.Pack(method, id)
		if packErr != nil {
			continue
		}

		v4Debugf("tick: calling PoolManager.%s(bytes32) calldata=%s", method, common.Bytes2Hex(data))
		msg := ethereum.CallMsg{To: &poolManager, Data: data}
		raw, callErr := Client.CallContract(context.Background(), msg, nil)
		if callErr != nil {
			err = fmt.Errorf("%s reverted/failed for PoolId %s: %w", method, poolID, callErr)
			v4Debugf("tick: PoolManager.%s failed: %v", method, err)
			continue
		}

		out, unpackErr := parsedABI.Unpack(method, raw)
		if unpackErr != nil {
			err = unpackErr
			v4Debugf("tick: PoolManager.%s unpack failed: %v raw=%s", method, unpackErr, common.Bytes2Hex(raw))
			continue
		}
		if len(out) < 2 {
			return 0, fmt.Errorf("unexpected %s return length: %d", method, len(out))
		}

		tickBig, ok := out[1].(*big.Int)
		if !ok || tickBig == nil {
			return 0, fmt.Errorf("unexpected tick type: %T", out[1])
		}

		return int(tickBig.Int64()), nil
	}

	if err != nil {
		return 0, fmt.Errorf("call uniswap v4 slot0 failed (PoolManager=%s PoolId=%s): %w", poolManager.Hex(), poolID, err)
	}
	return 0, fmt.Errorf("no compatible slot0 method found on pool manager")
}

// GetUniswapV4PoolSlot0 reads slot0 for a Uniswap V4 pool from the PoolManager.
// Returns sqrtPriceX96 and tick. Tries `getSlot0` then `slot0` for compatibility.
func GetUniswapV4PoolSlot0(poolManager common.Address, poolID string) (*big.Int, int, error) {
	if Client == nil {
		return nil, 0, fmt.Errorf("blockchain client not initialized")
	}
	if (poolManager == common.Address{}) {
		return nil, 0, fmt.Errorf("uniswap v4 pool manager address not configured")
	}

	parsedABI, err := abi.JSON(strings.NewReader(uniswapV4PoolManagerABI))
	if err != nil {
		return nil, 0, fmt.Errorf("parse pool manager ABI failed: %w", err)
	}

	id, err := normalizePoolID(poolID)
	if err != nil {
		return nil, 0, err
	}

	v4Debugf("slot0: PoolManager=%s PoolId=%s", poolManager.Hex(), poolID)

	var lastErr error
	for _, method := range []string{"getSlot0", "slot0"} {
		data, packErr := parsedABI.Pack(method, id)
		if packErr != nil {
			continue
		}

		v4Debugf("slot0: calling PoolManager.%s(bytes32) calldata=%s", method, common.Bytes2Hex(data))
		msg := ethereum.CallMsg{To: &poolManager, Data: data}
		raw, callErr := Client.CallContract(context.Background(), msg, nil)
		if callErr != nil {
			lastErr = fmt.Errorf("%s reverted/failed for PoolId %s: %w", method, poolID, callErr)
			v4Debugf("slot0: PoolManager.%s failed: %v", method, lastErr)
			continue
		}

		out, unpackErr := parsedABI.Unpack(method, raw)
		if unpackErr != nil {
			lastErr = unpackErr
			v4Debugf("slot0: PoolManager.%s unpack failed: %v raw=%s", method, unpackErr, common.Bytes2Hex(raw))
			continue
		}
		if len(out) < 2 {
			return nil, 0, fmt.Errorf("unexpected %s return length: %d", method, len(out))
		}

		sqrtPriceX96, ok0 := out[0].(*big.Int)
		tickBig, ok1 := out[1].(*big.Int)
		if !ok0 || sqrtPriceX96 == nil || !ok1 || tickBig == nil {
			return nil, 0, fmt.Errorf("unexpected slot0 return types: sqrt=%T tick=%T", out[0], out[1])
		}
		return sqrtPriceX96, int(tickBig.Int64()), nil
	}

	if lastErr != nil {
		return nil, 0, fmt.Errorf("call uniswap v4 slot0 failed (PoolManager=%s PoolId=%s): %w", poolManager.Hex(), poolID, lastErr)
	}
	return nil, 0, fmt.Errorf("no compatible slot0 method found on pool manager")
}

// GetUniswapV4PoolCurrentTickViaStateView reads the current tick using a StateView helper contract.
func GetUniswapV4PoolCurrentTickViaStateView(stateView common.Address, poolManager common.Address, poolID string) (int, error) {
	if Client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}
	if (stateView == common.Address{}) {
		return 0, fmt.Errorf("uniswap v4 state view address not configured")
	}
	if (poolManager == common.Address{}) {
		return 0, fmt.Errorf("uniswap v4 pool manager address not configured")
	}

	id, err := normalizePoolID(poolID)
	if err != nil {
		return 0, err
	}

	v4Debugf("stateview tick: StateView=%s PoolManager=%s PoolId=%s", stateView.Hex(), poolManager.Hex(), poolID)
	if v4DebugEnabled() {
		code, codeErr := Client.CodeAt(context.Background(), stateView, nil)
		if codeErr == nil {
			v4Debugf("stateview tick: StateView code size=%d bytes", len(code))
		}
	}

	parsedABI, err := abi.JSON(strings.NewReader(uniswapV4StateViewABISingleArg))
	if err != nil {
		return 0, fmt.Errorf("parse state view ABI failed: %w", err)
	}

	data, err := parsedABI.Pack("getSlot0", id)
	if err != nil {
		return 0, fmt.Errorf("pack state view getSlot0 failed: %w", err)
	}

	v4Debugf("stateview tick: calling getSlot0(bytes32) calldata=%s", common.Bytes2Hex(data))
	msg := ethereum.CallMsg{To: &stateView, Data: data}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := callContractWithRetry(Client, ctx, msg)
	if err != nil {
		return 0, fmt.Errorf("call state view getSlot0 failed: %w", err)
	}

	out, err := parsedABI.Unpack("getSlot0", raw)
	if err != nil {
		return 0, fmt.Errorf("unpack state view getSlot0 failed: %w", err)
	}
	if len(out) < 2 {
		return 0, fmt.Errorf("unexpected state view getSlot0 return length: %d", len(out))
	}

	sqrtPriceX96, ok0 := out[0].(*big.Int)
	tickBig, ok := out[1].(*big.Int)
	if !ok0 || sqrtPriceX96 == nil || !ok || tickBig == nil {
		return 0, fmt.Errorf("unexpected state view slot0 return types: sqrt=%T tick=%T", out[0], out[1])
	}
	if sqrtPriceX96.Sign() == 0 {
		return 0, fmt.Errorf("pool not initialized (sqrtPriceX96=0)")
	}
	return int(tickBig.Int64()), nil
}

// GetUniswapV4PoolCurrentTickViaStateViewAtBlock reads the tick using a StateView helper contract at a given block number.
func GetUniswapV4PoolCurrentTickViaStateViewAtBlock(stateView common.Address, poolManager common.Address, poolID string, blockNumber uint64) (int, error) {
	if Client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}
	if blockNumber == 0 {
		return 0, fmt.Errorf("block number not set")
	}
	if (stateView == common.Address{}) {
		return 0, fmt.Errorf("uniswap v4 state view address not configured")
	}
	if (poolManager == common.Address{}) {
		return 0, fmt.Errorf("uniswap v4 pool manager address not configured")
	}

	id, err := normalizePoolID(poolID)
	if err != nil {
		return 0, err
	}

	parsedABI, err := abi.JSON(strings.NewReader(uniswapV4StateViewABISingleArg))
	if err != nil {
		return 0, fmt.Errorf("parse state view ABI failed: %w", err)
	}

	data, err := parsedABI.Pack("getSlot0", id)
	if err != nil {
		return 0, fmt.Errorf("pack state view getSlot0 failed: %w", err)
	}

	msg := ethereum.CallMsg{To: &stateView, Data: data}
	block := new(big.Int).SetUint64(blockNumber)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := callContractWithRetryAtBlock(Client, ctx, msg, block)
	if err != nil {
		return 0, fmt.Errorf("call state view getSlot0 failed: %w", err)
	}

	out, err := parsedABI.Unpack("getSlot0", raw)
	if err != nil {
		return 0, fmt.Errorf("unpack state view getSlot0 failed: %w", err)
	}
	if len(out) < 2 {
		return 0, fmt.Errorf("unexpected state view getSlot0 return length: %d", len(out))
	}

	sqrtPriceX96, ok0 := out[0].(*big.Int)
	tickBig, ok := out[1].(*big.Int)
	if !ok0 || sqrtPriceX96 == nil || !ok || tickBig == nil {
		return 0, fmt.Errorf("unexpected state view slot0 return types: sqrt=%T tick=%T", out[0], out[1])
	}
	if sqrtPriceX96.Sign() == 0 {
		return 0, fmt.Errorf("pool not initialized (sqrtPriceX96=0)")
	}
	return int(tickBig.Int64()), nil
}

// GetUniswapV4PoolSlot0ViaStateView reads slot0 using a StateView helper contract.
func GetUniswapV4PoolSlot0ViaStateView(stateView common.Address, poolManager common.Address, poolID string) (*big.Int, int, error) {
	if Client == nil {
		return nil, 0, fmt.Errorf("blockchain client not initialized")
	}
	if (stateView == common.Address{}) {
		return nil, 0, fmt.Errorf("uniswap v4 state view address not configured")
	}
	if (poolManager == common.Address{}) {
		return nil, 0, fmt.Errorf("uniswap v4 pool manager address not configured")
	}

	id, err := normalizePoolID(poolID)
	if err != nil {
		return nil, 0, err
	}

	v4Debugf("stateview slot0: StateView=%s PoolManager=%s PoolId=%s", stateView.Hex(), poolManager.Hex(), poolID)

	parsedABI, err := abi.JSON(strings.NewReader(uniswapV4StateViewABISingleArg))
	if err != nil {
		return nil, 0, fmt.Errorf("parse state view ABI failed: %w", err)
	}

	data, err := parsedABI.Pack("getSlot0", id)
	if err != nil {
		return nil, 0, fmt.Errorf("pack state view getSlot0 failed: %w", err)
	}

	v4Debugf("stateview slot0: calling getSlot0(bytes32) calldata=%s", common.Bytes2Hex(data))
	msg := ethereum.CallMsg{To: &stateView, Data: data}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := callContractWithRetry(Client, ctx, msg)
	if err != nil {
		return nil, 0, fmt.Errorf("call state view getSlot0 failed: %w", err)
	}

	out, err := parsedABI.Unpack("getSlot0", raw)
	if err != nil {
		return nil, 0, fmt.Errorf("unpack state view getSlot0 failed: %w", err)
	}
	if len(out) < 2 {
		return nil, 0, fmt.Errorf("unexpected state view getSlot0 return length: %d", len(out))
	}

	sqrtPriceX96, ok0 := out[0].(*big.Int)
	tickBig, ok1 := out[1].(*big.Int)
	if !ok0 || sqrtPriceX96 == nil || !ok1 || tickBig == nil {
		return nil, 0, fmt.Errorf("unexpected state view slot0 return types: sqrt=%T tick=%T", out[0], out[1])
	}
	if sqrtPriceX96.Sign() == 0 {
		return nil, 0, fmt.Errorf("pool not initialized (sqrtPriceX96=0)")
	}
	return sqrtPriceX96, int(tickBig.Int64()), nil
}

// GetUniswapV4PoolTickSpacing reads tickSpacing from PoolManager.poolKeys(poolId).
func GetUniswapV4PoolTickSpacing(poolManager common.Address, poolID string) (int, error) {
	if Client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}
	if (poolManager == common.Address{}) {
		return 0, fmt.Errorf("uniswap v4 pool manager address not configured")
	}

	// Try poolKeys(poolId) first (some deployments have it).
	_, _, _, tickSpacing, _, err := GetUniswapV4PoolKey(poolManager, poolID)
	if err == nil {
		return tickSpacing, nil
	}
	// Fallback to Initialize event (recommended for deployments without poolKeys mapping).
	_, _, _, tickSpacing, _, initErr := GetUniswapV4PoolKeyFromInitializeEvent(poolManager, poolID)
	if initErr != nil {
		return 0, err
	}
	return tickSpacing, nil
}

// GetUniswapV4PoolKey reads PoolManager.poolKeys(poolId).
// If the PoolId isn't stored, many implementations return zero addresses (not a revert).
func GetUniswapV4PoolKey(poolManager common.Address, poolID string) (common.Address, common.Address, uint64, int, common.Address, error) {
	if Client == nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("blockchain client not initialized")
	}
	if (poolManager == common.Address{}) {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("uniswap v4 pool manager address not configured")
	}

	parsedABI, err := abi.JSON(strings.NewReader(uniswapV4PoolManagerABI))
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("parse pool manager ABI failed: %w", err)
	}

	id, err := normalizePoolID(poolID)
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, err
	}
	v4Debugf("tickSpacing: PoolManager=%s PoolId=%s", poolManager.Hex(), poolID)
	data, err := parsedABI.Pack("poolKeys", id)
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("pack poolKeys failed: %w", err)
	}

	v4Debugf("tickSpacing: calling PoolManager.poolKeys(bytes32) calldata=%s", common.Bytes2Hex(data))
	msg := ethereum.CallMsg{To: &poolManager, Data: data}
	raw, err := Client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("call poolKeys failed: %w", err)
	}

	out, err := parsedABI.Unpack("poolKeys", raw)
	if err != nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("unpack poolKeys failed: %w", err)
	}
	if len(out) < 5 {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("unexpected poolKeys return length: %d", len(out))
	}

	c0, _ := out[0].(common.Address)
	c1, _ := out[1].(common.Address)
	feeBig, _ := out[2].(*big.Int)
	tickSpacingBig, ok := out[3].(*big.Int)
	hooks, _ := out[4].(common.Address)

	var fee uint64
	if feeBig != nil {
		fee = feeBig.Uint64()
	}

	if !ok || tickSpacingBig == nil {
		return common.Address{}, common.Address{}, 0, 0, common.Address{}, fmt.Errorf("unexpected tickSpacing type: %T", out[3])
	}

	tickSpacing := int(tickSpacingBig.Int64())
	v4Debugf("tickSpacing: poolKeys currency0=%s currency1=%s fee=%d tickSpacing=%d hooks=%s", c0.Hex(), c1.Hex(), fee, tickSpacing, hooks.Hex())
	return c0, c1, fee, tickSpacing, hooks, nil
}
