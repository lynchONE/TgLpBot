package smart_money

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/pricing"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"gorm.io/gorm"
)

type PositionMetadata struct {
	PoolAddress   string
	Token0Address string
	Token1Address string
	Token0Symbol  string
	Token1Symbol  string
	FeeTier       *int
	TickLower     *int
	TickUpper     *int
}

func (m *PositionMetadata) TradingPair() string {
	if m == nil {
		return ""
	}
	left := strings.TrimSpace(m.Token0Symbol)
	right := strings.TrimSpace(m.Token1Symbol)
	switch {
	case left != "" && right != "":
		return left + "/" + right
	case left != "":
		return left
	case right != "":
		return right
	default:
		return ""
	}
}

var (
	positionMetadataCache sync.Map
	positionRepairMu      sync.Mutex
	lastStateRepairAt     time.Time
)

type irreversiblePositionMetadataError struct {
	reason string
	err    error
}

func (e *irreversiblePositionMetadataError) Error() string {
	if e == nil {
		return ""
	}
	return e.reason
}

func (e *irreversiblePositionMetadataError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

const (
	smartMoneyStateRepairInterval = 30 * time.Second
	smartMoneyStateRepairLookback = 6 * time.Hour
	smartMoneyStateRepairLimit    = 100
)

func RepairPositions(ctx context.Context, repo *Repository) error {
	if repo == nil {
		return nil
	}
	positionRepairMu.Lock()
	defer positionRepairMu.Unlock()

	positions, err := repo.ListPositionsNeedingMetadataRepair(ctx, invalidSmartMoneyPoolIdentifiers())
	if err != nil {
		return err
	}

	for _, pos := range positions {
		if _, err := RepairPositionMetadata(ctx, repo, pos); err != nil {
			var invalidErr *irreversiblePositionMetadataError
			if errors.As(err, &invalidErr) {
				if markErr := repo.MarkLPPositionMetadataInvalid(ctx, pos.ID, invalidErr.Error()); markErr != nil {
					log.Printf("[SmartMoney] mark invalid metadata failed: id=%d nft=%d protocol=%s err=%v", pos.ID, pos.NftTokenID, pos.Protocol, markErr)
				} else {
					log.Printf("[SmartMoney] skipped invalid position metadata: id=%d nft=%d protocol=%s reason=%s", pos.ID, pos.NftTokenID, pos.Protocol, invalidErr.reason)
				}
				if strings.EqualFold(strings.TrimSpace(pos.Status), "open") && shouldCloseInvalidPosition(invalidErr) {
					if active, activeErr := repo.EnsureActivePositionFromPosition(ctx, &pos); activeErr != nil {
						log.Printf("[SmartMoney] load invalid active position failed: id=%d nft=%d protocol=%s err=%v", pos.ID, pos.NftTokenID, pos.Protocol, activeErr)
					} else if active != nil {
						if changed, closeErr := closeSmartMoneyPosition(ctx, repo, active, &pos, time.Now()); closeErr != nil {
							log.Printf("[SmartMoney] close invalid position failed: id=%d nft=%d protocol=%s err=%v", pos.ID, pos.NftTokenID, pos.Protocol, closeErr)
						} else if changed {
							log.Printf("[SmartMoney] closed invalid position: id=%d nft=%d protocol=%s", pos.ID, pos.NftTokenID, pos.Protocol)
						}
					}
				}
				continue
			}
			log.Printf("[SmartMoney] repair position metadata failed: id=%d nft=%d protocol=%s err=%v", pos.ID, pos.NftTokenID, pos.Protocol, err)
		}
	}

	now := time.Now()
	if lastStateRepairAt.IsZero() || now.Sub(lastStateRepairAt) >= smartMoneyStateRepairInterval {
		lastStateRepairAt = now
		repaired, err := repairRecentOpenPositionStates(ctx, repo, now)
		if err != nil {
			log.Printf("[SmartMoney] repair open position states failed: %v", err)
		} else if repaired > 0 {
			log.Printf("[SmartMoney] repaired stale open positions: %d", repaired)
		}
	}
	return nil
}

func shouldCloseInvalidPosition(err *irreversiblePositionMetadataError) bool {
	if err == nil {
		return false
	}
	return !strings.Contains(strings.ToLower(strings.TrimSpace(err.reason)), "belongs to")
}

func repairRecentOpenPositionStates(ctx context.Context, repo *Repository, now time.Time) (int, error) {
	if repo == nil {
		return 0, nil
	}

	positions, err := repo.ListRecentOpenPositionsForStateRepair(ctx, now.Add(-smartMoneyStateRepairLookback), smartMoneyStateRepairLimit)
	if err != nil {
		return 0, err
	}

	repaired := 0
	for _, pos := range positions {
		changed, err := repairOpenPositionState(ctx, repo, &pos, now)
		if err != nil {
			log.Printf("[SmartMoney] repair open position state failed: id=%d nft=%d protocol=%s err=%v", pos.ID, pos.NftTokenID, pos.Protocol, err)
			continue
		}
		if changed {
			repaired++
		}
	}
	return repaired, nil
}

func repairOpenPositionState(ctx context.Context, repo *Repository, pos *models.SmartMoneyLPPosition, now time.Time) (bool, error) {
	if repo == nil || pos == nil {
		return false, nil
	}

	positionRef := BuildPositionRefFromPosition(pos)
	if positionRef == "" {
		return false, nil
	}

	active, err := repo.GetActivePositionByRef(ctx, positionRef)
	if err != nil {
		return false, err
	}
	if active == nil {
		active, err = repo.EnsureActivePositionFromPosition(ctx, pos)
		if err != nil {
			return false, err
		}
	}
	if active == nil {
		return false, nil
	}

	applyManagerSnapshotToActive(active)
	liveLiquidity, liveErr := repo.loadCurrentLiquiditySnapshot(nil, active)
	if liveErr != nil {
		if isInvalidTokenIDError(liveErr) || isV4PoolKeyNotSetError(liveErr) {
			return closeSmartMoneyPosition(ctx, repo, active, pos, now)
		}
		return false, liveErr
	}
	if liveLiquidity == nil {
		return false, nil
	}

	if liveLiquidity.Sign() > 0 {
		updates := map[string]interface{}{
			"current_liquidity": liveLiquidity.String(),
			"is_active":         true,
			"closed_at":         nil,
		}
		return false, database.DB.WithContext(ctx).
			Model(&models.SmartMoneyActivePosition{}).
			Where("id = ?", active.ID).
			Updates(updates).Error
	}

	return closeSmartMoneyPosition(ctx, repo, active, pos, now)
}

func closeSmartMoneyPosition(ctx context.Context, repo *Repository, active *models.SmartMoneyActivePosition, pos *models.SmartMoneyLPPosition, now time.Time) (bool, error) {
	if repo == nil || active == nil || pos == nil {
		return false, nil
	}
	closedAt := smartMoneyClosedAt(active, pos, now)

	err := repo.WithTx(ctx, func(tx *gorm.DB) error {
		activeUpdates := map[string]interface{}{
			"current_liquidity": "0",
			"is_active":         false,
			"closed_at":         &closedAt,
		}
		if err := tx.Model(&models.SmartMoneyActivePosition{}).
			Where("id = ?", active.ID).
			Updates(activeUpdates).Error; err != nil {
			return err
		}

		positionUpdates := map[string]interface{}{
			"status":    "closed",
			"closed_at": &closedAt,
		}
		return tx.Model(&models.SmartMoneyLPPosition{}).
			Where("id = ?", pos.ID).
			Updates(positionUpdates).Error
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func smartMoneyClosedAt(active *models.SmartMoneyActivePosition, pos *models.SmartMoneyLPPosition, fallback time.Time) time.Time {
	if active != nil && active.LastRemoveAt != nil && !active.LastRemoveAt.IsZero() {
		return *active.LastRemoveAt
	}
	if pos != nil && pos.ClosedAt != nil && !pos.ClosedAt.IsZero() {
		return *pos.ClosedAt
	}
	return fallback
}

func RepairPositionMetadata(ctx context.Context, repo *Repository, pos models.SmartMoneyLPPosition) (models.SmartMoneyLPPosition, error) {
	if !shouldRepairPositionMetadata(pos) {
		return pos, nil
	}
	meta, err := ResolvePositionMetadata(ctx, pos.ChainID, pos.Protocol, pos.NftTokenID)
	if err != nil {
		return pos, err
	}

	patched, updates := applyPositionMetadata(pos, meta)
	if repo != nil && len(updates) > 0 {
		updates["metadata_status"] = ""
		updates["metadata_error"] = ""
		if err := repo.UpdateLPPositionMetadata(ctx, pos.ID, updates); err != nil {
			return pos, err
		}
	}
	return patched, nil
}

func EnrichLPEvent(ctx context.Context, event *models.SmartMoneyLPEvent) error {
	if event == nil || event.NftTokenID == nil || *event.NftTokenID == 0 {
		return nil
	}
	meta, err := ResolvePositionMetadata(ctx, event.ChainID, event.Protocol, *event.NftTokenID)
	if err != nil {
		return err
	}
	applyEventMetadata(event, meta)
	return nil
}

// ComputeEventAmountUSD estimates the total USD value of an LP event
// by querying real token prices via OKX / GeckoTerminal.
func ComputeEventAmountUSD(ctx context.Context, event *models.SmartMoneyLPEvent) {
	if event == nil {
		return
	}
	addr0 := strings.TrimSpace(event.Token0Address)
	addr1 := strings.TrimSpace(event.Token1Address)
	if addr0 == "" && addr1 == "" {
		return
	}

	chain := smartMoneyChainName(event.ChainID)
	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return
	}

	dec0 := readTokenDecimalsWithClient(client, addr0)
	dec1 := readTokenDecimalsWithClient(client, addr1)

	amt0 := weiStringToFloat(event.Token0Amount, dec0)
	amt1 := weiStringToFloat(event.Token1Amount, dec1)
	if amt0 <= 0 && amt1 <= 0 {
		return
	}

	// Collect addresses that need pricing
	network := smartMoneyChainSlugForPricing(event.ChainID)
	addrs := make([]string, 0, 2)
	if addr0 != "" && amt0 > 0 {
		addrs = append(addrs, addr0)
	}
	if addr1 != "" && amt1 > 0 {
		addrs = append(addrs, addr1)
	}
	if len(addrs) == 0 {
		return
	}

	prices, err := smTokenPriceService.GetUSDPrices(network, addrs)
	if err != nil {
		log.Printf("[SmartMoney] ComputeEventAmountUSD: price lookup failed chain=%s: %v", network, err)
	}

	usd0 := amt0 * prices[strings.ToLower(addr0)]
	usd1 := amt1 * prices[strings.ToLower(addr1)]
	totalUSD := usd0 + usd1

	if totalUSD > 0 {
		s := fmt.Sprintf("%.4f", totalUSD)
		event.TotalUSD = &s
	}
	if usd0 > 0 {
		s := fmt.Sprintf("%.4f", usd0)
		event.Token0AmountUSD = &s
	}
	if usd1 > 0 {
		s := fmt.Sprintf("%.4f", usd1)
		event.Token1AmountUSD = &s
	}
}

var smTokenPriceService = pricing.DefaultTokenPriceService()

func readTokenDecimalsWithClient(client *ethclient.Client, addr string) int {
	if !common.IsHexAddress(addr) {
		return 18
	}
	dec, err := blockchain.GetTokenDecimalsWithClient(client, common.HexToAddress(addr))
	if err != nil {
		return 18
	}
	return int(dec)
}

func weiStringToFloat(raw string, decimals int) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return 0
	}
	amt, ok := new(big.Float).SetString(raw)
	if !ok {
		return 0
	}
	divisor := new(big.Float).SetFloat64(math.Pow(10, float64(decimals)))
	result, _ := new(big.Float).Quo(amt, divisor).Float64()
	return result
}

func smartMoneyChainSlugForPricing(chainID int) string {
	switch chainID {
	case 8453:
		return "base"
	default:
		return "bsc"
	}
}

func ResolvePositionMetadata(ctx context.Context, chainID int, protocol string, nftTokenID uint64) (*PositionMetadata, error) {
	protocol = strings.TrimSpace(strings.ToLower(protocol))
	if nftTokenID == 0 {
		return nil, fmt.Errorf("nft token id is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	cacheKey := fmt.Sprintf("%d:%s:%d", chainID, protocol, nftTokenID)
	if cached, ok := positionMetadataCache.Load(cacheKey); ok {
		if meta, ok := cached.(PositionMetadata); ok {
			return clonePositionMetadata(meta), nil
		}
	}

	chain := smartMoneyChainName(chainID)
	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return nil, err
	}

	var meta *PositionMetadata
	switch protocol {
	case "pancake_v3", "uniswap_v3":
		meta, err = resolveV3PositionMetadata(ctx, chain, protocol, nftTokenID, client)
	case "uniswap_v4":
		meta, err = resolveV4PositionMetadata(ctx, chain, nftTokenID, client)
	default:
		return nil, fmt.Errorf("unsupported smart money protocol: %s", protocol)
	}
	if err != nil {
		return nil, err
	}

	positionMetadataCache.Store(cacheKey, *meta)
	return clonePositionMetadata(*meta), nil
}

func resolveV3PositionMetadata(ctx context.Context, chain string, protocol string, nftTokenID uint64, client *ethclient.Client) (*PositionMetadata, error) {
	positionManager, err := resolveV3PositionManagerAddress(chain, protocol)
	if err != nil {
		return nil, err
	}
	pm, err := blockchain.NewV3PositionManager(positionManager, client)
	if err != nil {
		return nil, err
	}

	tokenID := new(big.Int).SetUint64(nftTokenID)
	pos, err := pm.Positions(nil, tokenID)
	if err != nil {
		if isInvalidTokenIDError(err) {
			if alternateProtocol, owner, ok := findV3PositionAlternateProtocol(ctx, chain, protocol, nftTokenID, client); ok {
				return nil, &irreversiblePositionMetadataError{
					reason: fmt.Sprintf("v3 token id %d belongs to %s position manager owner=%s, stored protocol=%s", nftTokenID, alternateProtocol, owner.Hex(), protocol),
					err:    err,
				}
			}
			return nil, &irreversiblePositionMetadataError{
				reason: fmt.Sprintf("v3 token id %d is not valid for protocol %s position manager", nftTokenID, protocol),
				err:    err,
			}
		}
		return nil, fmt.Errorf("read v3 position %d: %w", nftTokenID, err)
	}

	poolAddr, err := resolveV3PoolAddress(ctx, chain, client, positionManager, pos.Token0, pos.Token1, pos.Fee)
	if err != nil {
		return nil, err
	}

	feeTier := int(pos.Fee)
	tickLower := pos.TickLower
	tickUpper := pos.TickUpper

	return &PositionMetadata{
		PoolAddress:   strings.ToLower(poolAddr.Hex()),
		Token0Address: strings.ToLower(pos.Token0.Hex()),
		Token1Address: strings.ToLower(pos.Token1.Hex()),
		Token0Symbol:  readTokenSymbolWithClient(client, pos.Token0),
		Token1Symbol:  readTokenSymbolWithClient(client, pos.Token1),
		FeeTier:       &feeTier,
		TickLower:     &tickLower,
		TickUpper:     &tickUpper,
	}, nil
}

func findV3PositionAlternateProtocol(ctx context.Context, chain string, currentProtocol string, nftTokenID uint64, client *ethclient.Client) (string, common.Address, bool) {
	currentProtocol = strings.ToLower(strings.TrimSpace(currentProtocol))
	tokenID := new(big.Int).SetUint64(nftTokenID)
	for _, candidate := range []string{"pancake_v3", "uniswap_v3"} {
		if candidate == currentProtocol {
			continue
		}
		manager, err := resolveV3PositionManagerAddress(chain, candidate)
		if err != nil {
			continue
		}
		pm, err := blockchain.NewV3PositionManager(manager, client)
		if err != nil {
			continue
		}
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		owner, err := pm.OwnerOf(&bind.CallOpts{Context: callCtx}, tokenID)
		cancel()
		if err == nil && owner != (common.Address{}) {
			return candidate, owner, true
		}
	}
	return "", common.Address{}, false
}

func resolveV4PositionMetadata(ctx context.Context, chain string, nftTokenID uint64, client *ethclient.Client) (*PositionMetadata, error) {
	positionManager, err := resolveV4PositionManagerAddress(chain)
	if err != nil {
		return nil, err
	}
	pm, err := blockchain.NewV4PositionManager(positionManager, client)
	if err != nil {
		return nil, err
	}

	tokenID := new(big.Int).SetUint64(nftTokenID)
	raw, rawErr := pm.PositionInfoPacked(nil, tokenID)
	if rawErr != nil {
		if isInvalidTokenIDError(rawErr) {
			return nil, &irreversiblePositionMetadataError{
				reason: fmt.Sprintf("v4 token id %d is not valid for position manager", nftTokenID),
				err:    rawErr,
			}
		}
		return nil, fmt.Errorf("read v4 positionInfo for %d: %w", nftTokenID, rawErr)
	}

	poolID25, packedTickLower, packedTickUpper, err := decodeV4PositionInfo(raw)
	if err != nil {
		return nil, err
	}
	if isZeroV4PoolID25(poolID25) {
		return nil, &irreversiblePositionMetadataError{
			reason: fmt.Sprintf("v4 token id %d has empty pool id", nftTokenID),
		}
	}

	fullPoolID, token0, token1, fee, _, _, err := blockchain.GetUniswapV4PoolKeyFromPositionManagerBytes25Ctx(ctx, positionManager, poolID25)
	if err != nil {
		if isV4PoolKeyNotSetError(err) {
			return nil, &irreversiblePositionMetadataError{
				reason: fmt.Sprintf("v4 token id %d pool key is not registered", nftTokenID),
				err:    err,
			}
		}
		return nil, err
	}

	tickLower := packedTickLower
	tickUpper := packedTickUpper
	feeTier := int(fee)

	return &PositionMetadata{
		PoolAddress:   strings.ToLower(fullPoolID.Hex()),
		Token0Address: strings.ToLower(token0.Hex()),
		Token1Address: strings.ToLower(token1.Hex()),
		Token0Symbol:  readTokenSymbolWithClient(client, token0),
		Token1Symbol:  readTokenSymbolWithClient(client, token1),
		FeeTier:       &feeTier,
		TickLower:     &tickLower,
		TickUpper:     &tickUpper,
	}, nil
}

func resolveV3PositionManagerAddress(chain string, protocol string) (common.Address, error) {
	chain = config.NormalizeChain(chain)
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
			for _, dep := range cc.V3Deployments {
				name := strings.ToLower(strings.TrimSpace(dep.Name))
				switch protocol {
				case "pancake_v3":
					if strings.Contains(name, "pancake") && common.IsHexAddress(dep.PositionManagerAddress) {
						return common.HexToAddress(dep.PositionManagerAddress), nil
					}
				case "uniswap_v3":
					if strings.Contains(name, "uniswap") && common.IsHexAddress(dep.PositionManagerAddress) {
						return common.HexToAddress(dep.PositionManagerAddress), nil
					}
				}
			}
		}

		switch protocol {
		case "pancake_v3":
			if common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
				return common.HexToAddress(config.AppConfig.PancakeV3PositionManagerAddress), nil
			}
		case "uniswap_v3":
			if common.IsHexAddress(config.AppConfig.UniswapV3PositionManagerAddress) {
				return common.HexToAddress(config.AppConfig.UniswapV3PositionManagerAddress), nil
			}
		}
	}

	return common.Address{}, fmt.Errorf("position manager not configured for protocol %s", protocol)
}

func resolveV4PositionManagerAddress(chain string) (common.Address, error) {
	chain = config.NormalizeChain(chain)
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok && common.IsHexAddress(cc.UniswapV4PositionManagerAddress) {
			return common.HexToAddress(cc.UniswapV4PositionManagerAddress), nil
		}
		if common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
			return common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress), nil
		}
	}
	return common.Address{}, fmt.Errorf("uniswap v4 position manager not configured")
}

func resolveV3PoolAddress(ctx context.Context, chain string, client *ethclient.Client, npm common.Address, token0 common.Address, token1 common.Address, fee uint64) (common.Address, error) {
	factories := make([]common.Address, 0, 4)
	seen := make(map[common.Address]struct{}, 4)
	addFactory := func(addr string) {
		addr = strings.TrimSpace(addr)
		if !common.IsHexAddress(addr) {
			return
		}
		factory := common.HexToAddress(addr)
		if factory == (common.Address{}) {
			return
		}
		if _, ok := seen[factory]; ok {
			return
		}
		seen[factory] = struct{}{}
		factories = append(factories, factory)
	}

	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
			for _, dep := range cc.V3Deployments {
				if common.IsHexAddress(dep.PositionManagerAddress) && npm == common.HexToAddress(dep.PositionManagerAddress) {
					addFactory(dep.FactoryAddress)
				}
			}
			for _, dep := range cc.V3Deployments {
				addFactory(dep.FactoryAddress)
			}
		}
	}

	if len(factories) == 0 && chain == "bsc" {
		addFactory("0x0BFbcf9fa4f9C56B0F40a671Ad40E0805A091865")
		addFactory("0xdB1d10011AD0Ff90774D0C6Bb92e5C5c8b4461F7")
	}

	for _, factory := range factories {
		callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		poolAddr, err := blockchain.GetV3PoolFromFactoryCtxWithClient(client, callCtx, factory, token0, token1, fee)
		cancel()
		if err != nil {
			continue
		}
		if poolAddr != (common.Address{}) {
			return poolAddr, nil
		}
	}

	return common.Address{}, fmt.Errorf("v3 pool not found")
}

func readTokenSymbolWithClient(client *ethclient.Client, token common.Address) string {
	if token == (common.Address{}) {
		return ""
	}
	symbol, err := blockchain.GetTokenSymbolWithClient(client, token)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(symbol)
}

func decodeV4PositionInfo(raw *big.Int) ([25]byte, int, int, error) {
	var poolID25 [25]byte
	if raw == nil {
		return poolID25, 0, 0, fmt.Errorf("v4 positionInfo missing")
	}

	mask24 := big.NewInt(0xFFFFFF)
	tickLowerRaw := new(big.Int).And(new(big.Int).Rsh(raw, 8), mask24).Int64()
	tickUpperRaw := new(big.Int).And(new(big.Int).Rsh(raw, 32), mask24).Int64()

	poolID := new(big.Int).Rsh(raw, 56)
	copy(poolID25[:], poolID.FillBytes(make([]byte, 25)))

	return poolID25, decodeSignedInt24(tickLowerRaw), decodeSignedInt24(tickUpperRaw), nil
}

func decodeSignedInt24(v int64) int {
	if v&0x800000 != 0 {
		v -= 1 << 24
	}
	return int(v)
}

func isZeroV4PoolID25(poolID25 [25]byte) bool {
	for _, b := range poolID25 {
		if b != 0 {
			return false
		}
	}
	return true
}

func isInvalidTokenIDError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "invalid token id")
}

func isV4PoolKeyNotSetError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "positionmanager.poolkeys not set")
}

func applyPositionMetadata(pos models.SmartMoneyLPPosition, meta *PositionMetadata) (models.SmartMoneyLPPosition, map[string]interface{}) {
	patched := pos
	updates := make(map[string]interface{})

	applyString := func(target *string, column string, value string) {
		value = strings.TrimSpace(value)
		if value == "" || strings.TrimSpace(*target) == value {
			return
		}
		*target = value
		updates[column] = value
	}
	applyIntPtr := func(target **int, column string, value *int) {
		if value == nil {
			return
		}
		if *target != nil && **target == *value {
			return
		}
		copyValue := *value
		*target = &copyValue
		updates[column] = copyValue
	}

	applyString(&patched.PoolAddress, "pool_address", meta.PoolAddress)
	applyString(&patched.Token0Address, "token0_address", meta.Token0Address)
	applyString(&patched.Token1Address, "token1_address", meta.Token1Address)
	applyString(&patched.Token0Symbol, "token0_symbol", meta.Token0Symbol)
	applyString(&patched.Token1Symbol, "token1_symbol", meta.Token1Symbol)
	applyIntPtr(&patched.FeeTier, "fee_tier", meta.FeeTier)
	applyIntPtr(&patched.TickLower, "tick_lower", meta.TickLower)
	applyIntPtr(&patched.TickUpper, "tick_upper", meta.TickUpper)

	return patched, updates
}

func applyEventMetadata(event *models.SmartMoneyLPEvent, meta *PositionMetadata) {
	if event == nil || meta == nil {
		return
	}
	if value := strings.TrimSpace(meta.PoolAddress); value != "" {
		event.PoolAddress = value
	}
	if value := strings.TrimSpace(meta.Token0Address); value != "" {
		event.Token0Address = value
	}
	if value := strings.TrimSpace(meta.Token1Address); value != "" {
		event.Token1Address = value
	}
	if value := strings.TrimSpace(meta.Token0Symbol); value != "" {
		event.Token0Symbol = value
	}
	if value := strings.TrimSpace(meta.Token1Symbol); value != "" {
		event.Token1Symbol = value
	}
	if meta.FeeTier != nil {
		feeTier := *meta.FeeTier
		event.FeeTier = &feeTier
	}
	if event.TickLower == nil && meta.TickLower != nil {
		tickLower := *meta.TickLower
		event.TickLower = &tickLower
	}
	if event.TickUpper == nil && meta.TickUpper != nil {
		tickUpper := *meta.TickUpper
		event.TickUpper = &tickUpper
	}
}

func shouldRepairPositionMetadata(pos models.SmartMoneyLPPosition) bool {
	if pos.NftTokenID == 0 {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(pos.MetadataStatus), "invalid") {
		return false
	}
	if isKnownSmartMoneyPoolIdentifier(pos.PoolAddress) {
		return true
	}
	return strings.TrimSpace(pos.Token0Symbol) == "" ||
		strings.TrimSpace(pos.Token1Symbol) == "" ||
		pos.FeeTier == nil ||
		pos.TickLower == nil ||
		pos.TickUpper == nil
}

func invalidSmartMoneyPoolIdentifiers() []string {
	seen := make(map[string]struct{}, 4)
	add := func(raw string, out *[]string) {
		raw = strings.TrimSpace(strings.ToLower(raw))
		if raw == "" || !common.IsHexAddress(raw) {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		*out = append(*out, raw)
	}

	out := make([]string, 0, 4)
	if config.AppConfig != nil {
		add(config.AppConfig.PancakeV3PositionManagerAddress, &out)
		add(config.AppConfig.UniswapV3PositionManagerAddress, &out)
		add(config.AppConfig.UniswapV4PoolManagerAddress, &out)
		for _, chain := range config.AppConfig.EnabledChains {
			if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
				for _, dep := range cc.V3Deployments {
					add(dep.PositionManagerAddress, &out)
				}
				add(cc.UniswapV4PoolManagerAddress, &out)
			}
		}
	}
	return out
}

func isKnownSmartMoneyPoolIdentifier(poolAddress string) bool {
	poolAddress = strings.TrimSpace(strings.ToLower(poolAddress))
	if poolAddress == "" {
		return false
	}
	for _, candidate := range invalidSmartMoneyPoolIdentifiers() {
		if poolAddress == candidate {
			return true
		}
	}
	return false
}

func smartMoneyChainName(chainID int) string {
	switch chainID {
	case 8453:
		return "base"
	default:
		return "bsc"
	}
}

func clonePositionMetadata(meta PositionMetadata) *PositionMetadata {
	out := meta
	if meta.FeeTier != nil {
		feeTier := *meta.FeeTier
		out.FeeTier = &feeTier
	}
	if meta.TickLower != nil {
		tickLower := *meta.TickLower
		out.TickLower = &tickLower
	}
	if meta.TickUpper != nil {
		tickUpper := *meta.TickUpper
		out.TickUpper = &tickUpper
	}
	return &out
}
