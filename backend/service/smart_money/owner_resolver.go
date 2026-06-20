package smart_money

import (
	"context"
	"log"
	"math/big"
	"strconv"
	"strings"

	"TgLpBot/base/blockchain"
	"TgLpBot/base/models"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ownerOf(uint256) is identical for V3/V4 position-manager NFTs (standard
// ERC721), so one minimal ABI is reused to pack calldata and decode results.
const erc721OwnerOfABI = `[{"inputs":[{"internalType":"uint256","name":"tokenId","type":"uint256"}],"name":"ownerOf","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"}]`

// pendingOwnerResolve is an LP event whose real NFT owner must be resolved
// on-chain because its transaction sender (tx.From) is not a monitored wallet
// (e.g. liquidity added via a zap/aggregator/contract wallet on behalf of one).
type pendingOwnerResolve struct {
	event           *models.SmartMoneyLPEvent
	positionManager common.Address
	blockNumber     uint64
}

func ownerResolveKey(pm common.Address, tokenID uint64) string {
	return strings.ToLower(pm.Hex()) + "|" + strconv.FormatUint(tokenID, 10)
}

// positionManagerForEvent returns the NFT position-manager contract that holds
// the tokenId for the given event. For V3 the event is emitted by the position
// manager itself (vlog.Address); for V4 it is the configured V4 position manager.
func (w *Watcher) positionManagerForEvent(event *models.SmartMoneyLPEvent, vlog types.Log) (common.Address, bool) {
	if event == nil {
		return common.Address{}, false
	}
	if strings.Contains(strings.ToLower(event.Protocol), "v4") {
		pm, err := resolveV4PositionManagerAddress(smartMoneyChainName(int(w.chainID)))
		if err != nil {
			return common.Address{}, false
		}
		return pm, true
	}
	return vlog.Address, true
}

// resolveOwnersInPlace resolves the real NFT owner for each pending event and
// writes it (lowercased) into event.WalletAddress. Targets are de-duplicated by
// (positionManager, tokenId) and resolved in a single Multicall3 eth_call;
// sub-calls that revert (e.g. a position burned in the same tx) fall back to the
// per-event resolver (historical/current ownerOf + receipt Transfer). Events
// that cannot be attributed keep an empty WalletAddress.
func resolveOwnersInPlace(ctx context.Context, client *ethclient.Client, pending []*pendingOwnerResolve) {
	if client == nil || len(pending) == 0 {
		return
	}
	parsed, err := abi.JSON(strings.NewReader(erc721OwnerOfABI))
	if err != nil {
		log.Printf("[SmartMoney Watcher] owner resolver: parse abi failed: %v", err)
		return
	}

	// De-duplicate by (positionManager, tokenId); a single ownerOf result is
	// shared by every event touching the same position in this poll round.
	order := make([]string, 0, len(pending))
	byKey := make(map[string][]*pendingOwnerResolve, len(pending))
	for _, p := range pending {
		if p == nil || p.event == nil || p.event.NftTokenID == nil || *p.event.NftTokenID == 0 {
			continue
		}
		if p.positionManager == (common.Address{}) {
			continue
		}
		key := ownerResolveKey(p.positionManager, *p.event.NftTokenID)
		if _, ok := byKey[key]; !ok {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], p)
	}
	if len(order) == 0 {
		return
	}

	calls := make([]blockchain.Multicall3Call, 0, len(order))
	packedKeys := make([]string, 0, len(order))
	fallbackKeys := make([]string, 0)
	for _, key := range order {
		head := byKey[key][0]
		data, err := parsed.Pack("ownerOf", new(big.Int).SetUint64(*head.event.NftTokenID))
		if err != nil {
			fallbackKeys = append(fallbackKeys, key)
			continue
		}
		calls = append(calls, blockchain.Multicall3Call{
			Target:       head.positionManager,
			AllowFailure: true,
			CallData:     data,
		})
		packedKeys = append(packedKeys, key)
	}

	resolved := make(map[string]string, len(order))
	if len(calls) > 0 {
		results, err := blockchain.Aggregate3(ctx, client, common.Address{}, calls)
		if err != nil {
			log.Printf("[SmartMoney Watcher] owner resolver: multicall failed (%d calls), falling back per-event: %v", len(calls), err)
			fallbackKeys = append(fallbackKeys, packedKeys...)
		} else {
			for i, key := range packedKeys {
				if i >= len(results) {
					fallbackKeys = append(fallbackKeys, key)
					continue
				}
				res := results[i]
				owner, ok := decodeOwnerOf(parsed, res.ReturnData)
				if !res.Success || !ok || owner == (common.Address{}) {
					fallbackKeys = append(fallbackKeys, key)
					continue
				}
				resolved[key] = strings.ToLower(owner.Hex())
			}
		}
	}

	for _, key := range fallbackKeys {
		if _, ok := resolved[key]; ok {
			continue
		}
		if owner, ok := resolveOwnerSingle(ctx, client, byKey[key][0]); ok {
			resolved[key] = owner
		}
	}

	for key, owner := range resolved {
		for _, p := range byKey[key] {
			p.event.WalletAddress = owner
		}
	}
}

func decodeOwnerOf(parsed abi.ABI, data []byte) (common.Address, bool) {
	if len(data) == 0 {
		return common.Address{}, false
	}
	vals, err := parsed.Unpack("ownerOf", data)
	if err != nil || len(vals) == 0 {
		return common.Address{}, false
	}
	owner, ok := vals[0].(common.Address)
	return owner, ok
}

// resolveOwnerSingle resolves a single position's owner via the existing
// per-event resolvers (historical ownerOf -> receipt ERC721 Transfer ->
// current ownerOf). Used as a fallback for Multicall3 sub-calls that revert.
func resolveOwnerSingle(ctx context.Context, client *ethclient.Client, p *pendingOwnerResolve) (string, bool) {
	if p == nil || p.event == nil {
		return "", false
	}
	var (
		owner common.Address
		err   error
	)
	if strings.Contains(strings.ToLower(p.event.Protocol), "v4") {
		owner, _, err = resolveV4LiquidityOwner(ctx, client, p.positionManager, p.event, p.blockNumber)
	} else {
		owner, _, err = resolveV3LiquidityOwner(ctx, client, p.positionManager, p.event, p.blockNumber)
	}
	if err != nil || owner == (common.Address{}) {
		return "", false
	}
	return strings.ToLower(owner.Hex()), true
}
