package blockchain

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	v4PoolsSlot       = uint64(6)
	v4PositionsOffset = uint64(6)
)

const v4ExtsloadABI = `[
  {
    "inputs": [
      { "internalType": "bytes32", "name": "startSlot", "type": "bytes32" },
      { "internalType": "uint256", "name": "nSlots", "type": "uint256" }
    ],
    "name": "extsload",
    "outputs": [{ "internalType": "bytes32[]", "name": "values", "type": "bytes32[]" }],
    "stateMutability": "view",
    "type": "function"
  }
]`

type V4PositionState struct {
	Liquidity                *big.Int
	FeeGrowthInside0LastX128 *big.Int
	FeeGrowthInside1LastX128 *big.Int
}

type v4PackedPositionInfo struct {
	PoolId25      []byte
	TickLower     int
	TickUpper     int
	HasSubscriber bool
}

var (
	v4ExtsloadOnce   sync.Once
	v4ExtsloadParsed abi.ABI
	v4ExtsloadErr    error
)

func GetV4PositionInfo(positionManager common.Address, poolManager common.Address, poolID string, tokenId *big.Int) (*V4PositionInfo, error) {
	if Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if tokenId == nil || tokenId.Sign() <= 0 {
		return nil, fmt.Errorf("invalid tokenId")
	}
	if positionManager == (common.Address{}) {
		return nil, fmt.Errorf("positionManager missing")
	}

	pm, err := NewV4PositionManager(positionManager, Client)
	if err != nil {
		return nil, err
	}

	pos, posErr := pm.Positions(nil, tokenId)
	if posErr == nil && pos != nil {
		return pos, nil
	}

	raw, infoErr := pm.PositionInfoPacked(nil, tokenId)
	if infoErr != nil {
		if posErr != nil {
			return nil, fmt.Errorf("positions failed: %v; positionInfo failed: %w", posErr, infoErr)
		}
		return nil, infoErr
	}

	packed, err := decodeV4PackedPositionInfo(raw)
	if err != nil {
		return nil, err
	}

	pos = &V4PositionInfo{
		TickLower:     packed.TickLower,
		TickUpper:     packed.TickUpper,
		Liquidity:     big.NewInt(0),
		TokensOwed0:   big.NewInt(0),
		TokensOwed1:   big.NewInt(0),
		PoolId25:      hex.EncodeToString(packed.PoolId25),
		HasSubscriber: packed.HasSubscriber,
		PositionRaw:   []interface{}{raw},
	}

	if poolID != "" {
		if c0, c1, fee, _, _, keyErr := GetUniswapV4PoolKeyFromPositionManager(positionManager, poolID); keyErr == nil {
			pos.Token0 = c0
			pos.Token1 = c1
			pos.Fee = fee
		}
	}

	if posErr != nil {
		log.Printf("[V4PM] positions() failed for tokenId=%s: %v; using positionInfo+extsload", tokenId.String(), posErr)
	}

	if poolID == "" || poolManager == (common.Address{}) {
		return pos, fmt.Errorf("poolId/poolManager missing for V4 position state")
	}

	fullPoolID, perr := parsePoolID(poolID)
	if perr != nil {
		return pos, perr
	}
	if len(packed.PoolId25) == 25 {
		if !bytes.Equal(fullPoolID.Bytes()[:25], packed.PoolId25) {
			log.Printf("[V4PM] poolId mismatch: poolId=%s packed25=%s", fullPoolID.Hex(), hex.EncodeToString(packed.PoolId25))
		}
	}

	state, stErr := getV4PositionStateViaExtsload(poolManager, fullPoolID, positionManager, packed.TickLower, packed.TickUpper, tokenId)
	if stErr != nil {
		return pos, stErr
	}
	pos.Liquidity = state.Liquidity
	pos.FeeGrowthInside0LastX128 = state.FeeGrowthInside0LastX128
	pos.FeeGrowthInside1LastX128 = state.FeeGrowthInside1LastX128
	return pos, nil
}

func decodeV4PackedPositionInfo(raw *big.Int) (*v4PackedPositionInfo, error) {
	if raw == nil {
		return nil, fmt.Errorf("positionInfo missing")
	}
	mask24 := big.NewInt(0xFFFFFF)

	tickLowerRaw := new(big.Int).And(new(big.Int).Rsh(raw, 8), mask24).Int64()
	tickUpperRaw := new(big.Int).And(new(big.Int).Rsh(raw, 32), mask24).Int64()

	poolId := new(big.Int).Rsh(raw, 56)
	poolIdBytes := poolId.FillBytes(make([]byte, 25))

	hasSub := new(big.Int).And(raw, big.NewInt(0xFF)).Uint64() != 0

	return &v4PackedPositionInfo{
		PoolId25:      poolIdBytes,
		TickLower:     decodeSignedInt24(tickLowerRaw),
		TickUpper:     decodeSignedInt24(tickUpperRaw),
		HasSubscriber: hasSub,
	}, nil
}

func decodeSignedInt24(v int64) int {
	if v&0x800000 != 0 {
		v = v - (1 << 24)
	}
	return int(v)
}

func encodeSignedInt24(v int) ([]byte, error) {
	if v < -8388608 || v > 8388607 {
		return nil, fmt.Errorf("int24 out of range: %d", v)
	}
	val := int64(v)
	if val < 0 {
		val = (1 << 24) + val
	}
	return []byte{byte(val >> 16), byte(val >> 8), byte(val)}, nil
}

func getV4PositionStateViaExtsload(poolManager common.Address, poolID common.Hash, owner common.Address, tickLower, tickUpper int, tokenId *big.Int) (*V4PositionState, error) {
	if poolManager == (common.Address{}) {
		return nil, fmt.Errorf("poolManager missing")
	}
	if tokenId == nil || tokenId.Sign() <= 0 {
		return nil, fmt.Errorf("invalid tokenId")
	}

	positionKey, err := buildV4PositionKey(owner, tickLower, tickUpper, tokenId)
	if err != nil {
		return nil, err
	}

	stateSlot := v4PoolStateSlot(poolID)
	positionMappingSlot := addUint256ToHash(stateSlot, v4PositionsOffset)
	positionSlot := crypto.Keccak256Hash(positionKey.Bytes(), positionMappingSlot.Bytes())

	slots, err := v4ExtsloadSlots(poolManager, positionSlot, 3)
	if err != nil {
		return nil, err
	}
	if len(slots) < 3 {
		return nil, fmt.Errorf("extsload returned %d slots, expected 3", len(slots))
	}

	liquidity := new(big.Int).SetBytes(slots[0].Bytes())
	fee0 := new(big.Int).SetBytes(slots[1].Bytes())
	fee1 := new(big.Int).SetBytes(slots[2].Bytes())

	return &V4PositionState{
		Liquidity:                liquidity,
		FeeGrowthInside0LastX128: fee0,
		FeeGrowthInside1LastX128: fee1,
	}, nil
}

func buildV4PositionKey(owner common.Address, tickLower, tickUpper int, tokenId *big.Int) (common.Hash, error) {
	lower, err := encodeSignedInt24(tickLower)
	if err != nil {
		return common.Hash{}, err
	}
	upper, err := encodeSignedInt24(tickUpper)
	if err != nil {
		return common.Hash{}, err
	}
	salt := tokenId.FillBytes(make([]byte, 32))

	buf := make([]byte, 0, 58)
	buf = append(buf, owner.Bytes()...)
	buf = append(buf, lower...)
	buf = append(buf, upper...)
	buf = append(buf, salt...)

	return crypto.Keccak256Hash(buf), nil
}

func v4PoolStateSlot(poolID common.Hash) common.Hash {
	slot := make([]byte, 32)
	slot[31] = byte(v4PoolsSlot)
	return crypto.Keccak256Hash(poolID.Bytes(), slot)
}

func addUint256ToHash(h common.Hash, offset uint64) common.Hash {
	val := new(big.Int).SetBytes(h.Bytes())
	val.Add(val, new(big.Int).SetUint64(offset))
	return common.BytesToHash(val.FillBytes(make([]byte, 32)))
}

func parsePoolID(poolID string) (common.Hash, error) {
	s := strings.TrimSpace(poolID)
	if s == "" {
		return common.Hash{}, fmt.Errorf("poolId empty")
	}
	if !strings.HasPrefix(s, "0x") {
		s = "0x" + s
	}
	raw, err := hexutil.Decode(s)
	if err != nil {
		return common.Hash{}, fmt.Errorf("invalid poolId: %w", err)
	}
	if len(raw) > 32 {
		return common.Hash{}, fmt.Errorf("poolId length invalid: %d", len(raw))
	}
	buf := make([]byte, 32)
	copy(buf[32-len(raw):], raw)
	return common.BytesToHash(buf), nil
}

func v4ExtsloadSlots(poolManager common.Address, startSlot common.Hash, nSlots uint64) ([]common.Hash, error) {
	if Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	contract, err := v4ExtsloadContract(poolManager, Client)
	if err != nil {
		return nil, err
	}
	var result []interface{}
	if err := contract.Call(nil, &result, "extsload", startSlot, new(big.Int).SetUint64(nSlots)); err != nil {
		return nil, err
	}
	if len(result) < 1 {
		return nil, fmt.Errorf("extsload returned empty result")
	}

	switch v := result[0].(type) {
	case []common.Hash:
		return v, nil
	case [][32]byte:
		hashes := make([]common.Hash, len(v))
		for i := range v {
			hashes[i] = common.BytesToHash(v[i][:])
		}
		return hashes, nil
	default:
		return nil, fmt.Errorf("unexpected extsload return type: %T", result[0])
	}
}

func v4ExtsloadContract(address common.Address, client *ethclient.Client) (*bind.BoundContract, error) {
	v4ExtsloadOnce.Do(func() {
		v4ExtsloadParsed, v4ExtsloadErr = abi.JSON(strings.NewReader(v4ExtsloadABI))
	})
	if v4ExtsloadErr != nil {
		return nil, v4ExtsloadErr
	}
	return bind.NewBoundContract(address, v4ExtsloadParsed, client, client, client), nil
}
