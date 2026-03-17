package web_server

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/service/pool"
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

type smartMoneyPositionRef struct {
	PositionKey     string
	WalletAddress   string
	PoolVersion     string
	PoolID          string
	ContractAddress string
	TokenID         string
	TickLower       int
	TickUpper       int
	LastEventSeq    uint64
	OpenedAt        *time.Time
}

type smartMoneyResolvedPosition struct {
	PoolVersion string
	PoolID      string
	PositionID  string

	ContractAddress string
	Exchange        string
	Pair            string
	FeePct          float64

	CurrentTick int
	TickLower   int
	TickUpper   int
	InRange     bool

	Liquidity string

	Token0       string
	Token1       string
	Token0Symbol string
	Token1Symbol string
	Token0Dec    int
	Token1Dec    int

	Amount0 float64
	Amount1 float64

	ClaimableFee0    float64
	ClaimableFee1    float64
	ClaimableFeesUSD float64
	FeeStatus        string
	FeeError         string

	LegacyFallback bool
}

type smartMoneyPositionResolver struct {
	metaCache *smartMoneyTokenMetaCache

	v3Managers []common.Address
	pmMu       sync.Mutex
	pmCache    map[common.Address]*blockchain.V3PositionManager

	v4PMAddr     common.Address
	v4PoolManger common.Address

	snapshotOnce  sync.Once
	snapshotBlock uint64
	snapshotErr   error
}

func newSmartMoneyPositionResolver(metaCache *smartMoneyTokenMetaCache) (*smartMoneyPositionResolver, []string) {
	if metaCache == nil {
		metaCache = newSmartMoneyTokenMetaCache()
	}

	resolver := &smartMoneyPositionResolver{
		metaCache: metaCache,
		pmCache:   make(map[common.Address]*blockchain.V3PositionManager),
	}

	warnings := make([]string, 0, 4)
	if blockchain.Client == nil {
		return resolver, append(warnings, "blockchain client not initialized")
	}

	if config.AppConfig != nil {
		if common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
			resolver.v3Managers = append(resolver.v3Managers, common.HexToAddress(config.AppConfig.PancakeV3PositionManagerAddress))
		}
		if common.IsHexAddress(config.AppConfig.UniswapV3PositionManagerAddress) {
			resolver.v3Managers = append(resolver.v3Managers, common.HexToAddress(config.AppConfig.UniswapV3PositionManagerAddress))
		}
		if common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
			resolver.v4PMAddr = common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
		}
		if common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
			resolver.v4PoolManger = common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
		}
	}

	for _, addr := range resolver.v3Managers {
		if pm := resolver.getV3Manager(addr); pm == nil {
			warnings = append(warnings, fmt.Sprintf("init V3 position manager failed: %s", addr.Hex()))
		}
	}

	return resolver, warnings
}

func (r *smartMoneyPositionResolver) canResolveV4() bool {
	return blockchain.Client != nil && r != nil && r.v4PMAddr != (common.Address{}) && r.v4PoolManger != (common.Address{})
}

func (r *smartMoneyPositionResolver) getV3Manager(addr common.Address) *blockchain.V3PositionManager {
	if r == nil || blockchain.Client == nil || addr == (common.Address{}) {
		return nil
	}

	r.pmMu.Lock()
	defer r.pmMu.Unlock()

	if pm, ok := r.pmCache[addr]; ok {
		return pm
	}

	pm, err := blockchain.NewV3PositionManager(addr, blockchain.Client)
	if err != nil {
		r.pmCache[addr] = nil
		return nil
	}
	r.pmCache[addr] = pm
	return pm
}

func (r *smartMoneyPositionResolver) Resolve(ctx context.Context, ref smartMoneyPositionRef) (*smartMoneyResolvedPosition, error) {
	if r == nil {
		return nil, fmt.Errorf("resolver not initialized")
	}

	switch strings.ToLower(strings.TrimSpace(ref.PoolVersion)) {
	case "v3":
		return r.resolveV3(ctx, ref)
	case "v4":
		return r.resolveV4(ctx, ref)
	default:
		return nil, fmt.Errorf("unsupported pool version: %s", ref.PoolVersion)
	}
}

func (r *smartMoneyPositionResolver) getSnapshotBlock(ctx context.Context) (uint64, error) {
	if r == nil {
		return 0, fmt.Errorf("resolver not initialized")
	}
	if blockchain.Client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}

	r.snapshotOnce.Do(func() {
		callCtx := ctx
		if callCtx == nil {
			callCtx = context.Background()
		}
		callCtx, cancel := context.WithTimeout(callCtx, 8*time.Second)
		defer cancel()

		r.snapshotBlock, r.snapshotErr = blockchain.Client.BlockNumber(callCtx)
	})

	return r.snapshotBlock, r.snapshotErr
}

func (r *smartMoneyPositionResolver) resolveV3(ctx context.Context, ref smartMoneyPositionRef) (*smartMoneyResolvedPosition, error) {
	if blockchain.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}

	tokenID, ok := new(big.Int).SetString(strings.TrimSpace(ref.TokenID), 10)
	if !ok || tokenID == nil || tokenID.Sign() <= 0 {
		return nil, fmt.Errorf("invalid token_id")
	}

	snapshotBlock, snapshotErr := r.getSnapshotBlock(ctx)
	useSnapshot := snapshotErr == nil && snapshotBlock > 0

	npmOrder := make([]common.Address, 0, len(r.v3Managers)+1)
	if common.IsHexAddress(ref.ContractAddress) {
		addr := common.HexToAddress(ref.ContractAddress)
		npmOrder = append(npmOrder, addr)
	} else {
		seen := make(map[common.Address]struct{}, len(r.v3Managers))
		for _, addr := range r.v3Managers {
			if _, ok := seen[addr]; ok {
				continue
			}
			npmOrder = append(npmOrder, addr)
			seen[addr] = struct{}{}
		}
	}
	if len(npmOrder) == 0 {
		return nil, fmt.Errorf("no V3 position manager available")
	}

	var (
		info    *blockchain.V3PositionInfo
		usedNPM common.Address
		lastErr error
	)
	for _, npmAddr := range npmOrder {
		pm := r.getV3Manager(npmAddr)
		if pm == nil {
			continue
		}
		callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		opts := &bind.CallOpts{Context: callCtx}
		if useSnapshot {
			opts.BlockNumber = new(big.Int).SetUint64(snapshotBlock)
		}
		pos, err := pm.Positions(opts, tokenID)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		if pos == nil || pos.Liquidity == nil || pos.Liquidity.Sign() <= 0 {
			continue
		}
		if poolErr := validateSmartMoneyV3PositionPool(ref.PoolID, pos); poolErr != nil {
			lastErr = poolErr
			continue
		}
		info = pos
		usedNPM = npmAddr
		break
	}
	if info == nil {
		return nil, lastErr
	}

	if !useSnapshot {
		snapshotBlock = 0
	}
	return r.buildV3ResolvedPosition(ctx, ref, usedNPM, tokenID.String(), info, snapshotBlock)
}

func validateSmartMoneyV3PositionPool(poolID string, pos *blockchain.V3PositionInfo) error {
	if pos == nil {
		return fmt.Errorf("position info missing")
	}
	if !common.IsHexAddress(poolID) {
		return nil
	}

	poolAddr := common.HexToAddress(poolID)
	poolToken0, poolToken1, err := blockchain.GetV3PoolTokens(poolAddr)
	if err != nil {
		return fmt.Errorf("read V3 pool tokens failed: %w", err)
	}
	poolFee, err := blockchain.GetV3PoolFee(poolAddr)
	if err != nil {
		return fmt.Errorf("read V3 pool fee failed: %w", err)
	}

	if poolToken0 != pos.Token0 || poolToken1 != pos.Token1 || uint64(poolFee) != pos.Fee {
		return fmt.Errorf(
			"V3 tokenId/pool mismatch: pool=%s poolToken0=%s poolToken1=%s poolFee=%d posToken0=%s posToken1=%s posFee=%d",
			poolAddr.Hex(),
			poolToken0.Hex(),
			poolToken1.Hex(),
			poolFee,
			pos.Token0.Hex(),
			pos.Token1.Hex(),
			pos.Fee,
		)
	}
	return nil
}

func (r *smartMoneyPositionResolver) buildV3ResolvedPosition(ctx context.Context, ref smartMoneyPositionRef, npm common.Address, positionID string, info *blockchain.V3PositionInfo, snapshotBlock uint64) (*smartMoneyResolvedPosition, error) {
	if info == nil {
		return nil, fmt.Errorf("position info missing")
	}

	out := &smartMoneyResolvedPosition{
		PoolVersion:     "v3",
		PoolID:          strings.ToLower(strings.TrimSpace(ref.PoolID)),
		PositionID:      positionID,
		ContractAddress: strings.ToLower(npm.Hex()),
		Exchange:        v3ExchangeLabel(npm),
		TickLower:       info.TickLower,
		TickUpper:       info.TickUpper,
		Liquidity:       info.Liquidity.String(),
		Token0:          strings.ToLower(strings.TrimSpace(info.Token0.Hex())),
		Token1:          strings.ToLower(strings.TrimSpace(info.Token1.Hex())),
		FeeStatus:       "skipped",
	}

	if info.Fee > 0 {
		out.FeePct = float64(info.Fee) / 10000.0
	}

	out.Token0Dec = r.metaCache.Decimals(out.Token0)
	out.Token1Dec = r.metaCache.Decimals(out.Token1)
	out.Token0Symbol = r.metaCache.Symbol(out.Token0)
	out.Token1Symbol = r.metaCache.Symbol(out.Token1)
	out.Pair = smartMoneyPairLabel(out.Token0Symbol, out.Token1Symbol)

	if common.IsHexAddress(out.PoolID) {
		poolAddr := common.HexToAddress(out.PoolID)
		var (
			sqrtP       *big.Int
			currentTick int
			slotErr     error
		)
		if snapshotBlock > 0 {
			sqrtP, currentTick, slotErr = blockchain.GetV3PoolSlot0AtBlock(poolAddr, snapshotBlock)
		} else {
			slotCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			sqrtP, currentTick, slotErr = getV3Slot0WithTimeout(slotCtx, poolAddr)
			cancel()
		}
		if slotErr != nil {
			out.FeeStatus = "error"
			out.FeeError = truncateErr(slotErr, 120)
			return out, nil
		}

		out.CurrentTick = currentTick
		out.InRange = currentTick >= info.TickLower && currentTick <= info.TickUpper

		if sqrtP != nil {
			sqrtA, _ := pool.SqrtRatioAtTick(int32(info.TickLower))
			sqrtB, _ := pool.SqrtRatioAtTick(int32(info.TickUpper))
			amt0Raw, amt1Raw := pool.AmountsForLiquidity(sqrtP, sqrtA, sqrtB, info.Liquidity)
			out.Amount0 = sanitizeFloat(amountToFloat(amountToString(amt0Raw), out.Token0Dec))
			out.Amount1 = sanitizeFloat(amountToFloat(amountToString(amt1Raw), out.Token1Dec))
		}

		var fee0, fee1 *big.Int
		var feeErr error
		if snapshotBlock > 0 {
			fee0, fee1, feeErr = pool.CalcV3UnclaimedFeesAtBlock(poolAddr, currentTick, info, snapshotBlock)
		} else {
			fee0, fee1, feeErr = pool.CalcV3UnclaimedFees(poolAddr, currentTick, info)
		}
		if feeErr != nil {
			out.FeeStatus = "error"
			out.FeeError = truncateErr(feeErr, 120)
		} else {
			out.ClaimableFee0 = sanitizeFloat(amountToFloat(amountToString(fee0), out.Token0Dec))
			out.ClaimableFee1 = sanitizeFloat(amountToFloat(amountToString(fee1), out.Token1Dec))
			out.FeeStatus = "ok"
		}
	}

	return out, nil
}

func (r *smartMoneyPositionResolver) resolveV4(ctx context.Context, ref smartMoneyPositionRef) (*smartMoneyResolvedPosition, error) {
	if !r.canResolveV4() {
		return nil, fmt.Errorf("V4 position manager not configured")
	}

	tokenID, ok := new(big.Int).SetString(strings.TrimSpace(ref.TokenID), 10)
	if !ok || tokenID == nil || tokenID.Sign() <= 0 {
		return nil, fmt.Errorf("invalid token_id")
	}

	stateView := common.Address{}
	if config.AppConfig != nil && common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) {
		stateView = common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
	}

	snapshotBlock, snapshotErr := r.getSnapshotBlock(ctx)
	useSnapshot := snapshotErr == nil && snapshotBlock > 0 && stateView != (common.Address{})

	var (
		pos    *blockchain.V4PositionInfo
		posErr error
	)
	if useSnapshot {
		pos, posErr = blockchain.GetV4PositionInfoAtBlock(r.v4PMAddr, r.v4PoolManger, ref.PoolID, tokenID, snapshotBlock)
	} else {
		pos, posErr = blockchain.GetV4PositionInfo(r.v4PMAddr, r.v4PoolManger, ref.PoolID, tokenID)
	}
	if pos == nil {
		return nil, posErr
	}
	if pos.Liquidity == nil || pos.Liquidity.Sign() <= 0 {
		return nil, posErr
	}

	out := &smartMoneyResolvedPosition{
		PoolVersion:     "v4",
		PoolID:          strings.ToLower(strings.TrimSpace(ref.PoolID)),
		PositionID:      tokenID.String(),
		ContractAddress: strings.ToLower(r.v4PMAddr.Hex()),
		Exchange:        "Uniswap V4",
		TickLower:       pos.TickLower,
		TickUpper:       pos.TickUpper,
		Liquidity:       pos.Liquidity.String(),
		Token0:          strings.ToLower(strings.TrimSpace(pos.Token0.Hex())),
		Token1:          strings.ToLower(strings.TrimSpace(pos.Token1.Hex())),
		FeeStatus:       "skipped",
	}

	if pos.Fee > 0 {
		out.FeePct = float64(pos.Fee) / 10000.0
	}

	out.Token0Dec = r.metaCache.Decimals(out.Token0)
	out.Token1Dec = r.metaCache.Decimals(out.Token1)
	out.Token0Symbol = r.metaCache.Symbol(out.Token0)
	out.Token1Symbol = r.metaCache.Symbol(out.Token1)
	out.Pair = smartMoneyPairLabel(out.Token0Symbol, out.Token1Symbol)

	var (
		sqrtP       *big.Int
		currentTick int
		slotErr     error
	)
	if useSnapshot {
		sqrtP, currentTick, slotErr = blockchain.GetUniswapV4PoolSlot0ViaStateViewAtBlock(stateView, r.v4PoolManger, out.PoolID, snapshotBlock)
	} else {
		sqrtP, currentTick, slotErr = loadSmartMoneyV4Slot0(r.v4PoolManger, out.PoolID)
	}
	if slotErr != nil {
		out.FeeStatus = "error"
		out.FeeError = truncateErr(slotErr, 120)
		if posErr != nil && out.FeeError == "" {
			out.FeeError = truncateErr(posErr, 120)
		}
		return out, nil
	}

	out.CurrentTick = currentTick
	out.InRange = currentTick >= pos.TickLower && currentTick <= pos.TickUpper

	if sqrtP != nil {
		sqrtA, _ := pool.SqrtRatioAtTick(int32(pos.TickLower))
		sqrtB, _ := pool.SqrtRatioAtTick(int32(pos.TickUpper))
		amt0Raw, amt1Raw := pool.AmountsForLiquidity(sqrtP, sqrtA, sqrtB, pos.Liquidity)
		out.Amount0 = sanitizeFloat(amountToFloat(amountToString(amt0Raw), out.Token0Dec))
		out.Amount1 = sanitizeFloat(amountToFloat(amountToString(amt1Raw), out.Token1Dec))
	}

	var fee0, fee1 *big.Int
	var feeErr error
	if useSnapshot {
		fee0, fee1, feeErr = pool.CalcV4UnclaimedFeesAtBlock(stateView, r.v4PoolManger, out.PoolID, currentTick, pos, snapshotBlock)
	} else {
		fee0, fee1, feeErr = pool.CalcV4UnclaimedFees(out.PoolID, currentTick, pos)
	}
	if feeErr != nil {
		out.FeeStatus = "error"
		out.FeeError = truncateErr(feeErr, 120)
	} else {
		out.ClaimableFee0 = sanitizeFloat(amountToFloat(amountToString(fee0), out.Token0Dec))
		out.ClaimableFee1 = sanitizeFloat(amountToFloat(amountToString(fee1), out.Token1Dec))
		out.FeeStatus = "ok"
	}

	return out, nil
}

func querySmartMoneyWalletRecentPositionRefs(ctx context.Context, conn smartMoneyClickHouseQueryer, chain string, wallet string, window time.Duration, limit int) ([]smartMoneyPositionRef, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if conn == nil {
		return nil, fmt.Errorf("clickhouse not initialized")
	}
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	if wallet == "" || limit <= 0 {
		return []smartMoneyPositionRef{}, nil
	}
	if limit > 500 {
		limit = 500
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	seconds := int(window.Seconds())
	if seconds <= 0 {
		seconds = 86400
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	chainFilter := ""
	args := make([]any, 0, 2)
	args = append(args, wallet)
	if chain != "" {
		chainFilter = "AND lowerUTF8(chain) = ?"
		args = append(args, chain)
	}

	q := fmt.Sprintf(`
		SELECT
			position_key,
			pool_version,
			pool_id,
			contract_address,
			token_id,
			tick_lower,
			tick_upper,
			opened_at,
			last_event_seq
		FROM (
			SELECT
				position_key,
				latest_pool_version AS pool_version,
				latest_pool_id AS pool_id,
				latest_contract_address AS contract_address,
				latest_token_id AS token_id,
				latest_tick_lower AS tick_lower,
				latest_tick_upper AS tick_upper,
				latest_opened_at AS opened_at,
				latest_last_add_at AS last_add_at,
				latest_is_active AS is_active,
				latest_last_event_seq AS last_event_seq
			FROM (
				SELECT
					position_key,
					argMax(pool_version, tuple(last_event_seq, updated_at)) AS latest_pool_version,
					argMax(pool_id, tuple(last_event_seq, updated_at)) AS latest_pool_id,
					argMax(contract_address, tuple(last_event_seq, updated_at)) AS latest_contract_address,
					argMax(token_id, tuple(last_event_seq, updated_at)) AS latest_token_id,
					argMax(tick_lower, tuple(last_event_seq, updated_at)) AS latest_tick_lower,
					argMax(tick_upper, tuple(last_event_seq, updated_at)) AS latest_tick_upper,
					argMax(opened_at, tuple(last_event_seq, updated_at)) AS latest_opened_at,
					argMax(last_add_at, tuple(last_event_seq, updated_at)) AS latest_last_add_at,
					argMax(is_active, tuple(last_event_seq, updated_at)) AS latest_is_active,
					max(last_event_seq) AS latest_last_event_seq
				FROM smart_lp_active_positions
				WHERE lowerUTF8(wallet_address) = ?
					AND last_add_at >= now() - INTERVAL %d SECOND
					%s
				GROUP BY position_key
			)
		)
		WHERE is_active = 1
			AND token_id != ''
		ORDER BY last_add_at DESC, last_event_seq DESC
		LIMIT %d
	`, seconds, chainFilter, limit)

	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]smartMoneyPositionRef, 0, limit)
	for rows.Next() {
		var (
			positionKey string
			poolVersion string
			poolID      string
			contract    string
			tokenID     string
			tickLower   int32
			tickUpper   int32
			openedAt    time.Time
			last        uint64
		)
		if err := rows.Scan(&positionKey, &poolVersion, &poolID, &contract, &tokenID, &tickLower, &tickUpper, &openedAt, &last); err != nil {
			return nil, err
		}
		var openedAtPtr *time.Time
		if openedAt.After(time.Unix(0, 0).UTC()) {
			open := openedAt.UTC()
			openedAtPtr = &open
		}
		out = append(out, smartMoneyPositionRef{
			PositionKey:     strings.TrimSpace(positionKey),
			WalletAddress:   wallet,
			PoolVersion:     strings.ToLower(strings.TrimSpace(poolVersion)),
			PoolID:          strings.ToLower(strings.TrimSpace(poolID)),
			ContractAddress: strings.ToLower(strings.TrimSpace(contract)),
			TokenID:         strings.TrimSpace(tokenID),
			TickLower:       int(tickLower),
			TickUpper:       int(tickUpper),
			LastEventSeq:    last,
			OpenedAt:        openedAtPtr,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func querySmartMoneyWalletLegacyV4Pools(ctx context.Context, conn smartMoneyClickHouseQueryer, chain string, wallet string, window time.Duration, limit int) ([]smartMoneyWalletV4PoolRef, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if conn == nil {
		return nil, fmt.Errorf("clickhouse not initialized")
	}
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	if wallet == "" {
		return []smartMoneyWalletV4PoolRef{}, nil
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	seconds := int(window.Seconds())
	if seconds <= 0 {
		seconds = 86400
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	chainFilter := ""
	args := make([]any, 0, 2)
	args = append(args, wallet)
	if chain != "" {
		chainFilter = "AND lowerUTF8(chain) = ?"
		args = append(args, chain)
	}

	q := fmt.Sprintf(`
		SELECT
			position_key,
			pool_id,
			tick_lower,
			tick_upper
		FROM (
			SELECT
				position_key,
				latest_pool_id AS pool_id,
				latest_token_id AS token_id,
				latest_tick_lower AS tick_lower,
				latest_tick_upper AS tick_upper,
				latest_last_add_at AS last_add_at,
				latest_is_active AS is_active
			FROM (
				SELECT
					position_key,
					argMax(pool_id, tuple(last_event_seq, updated_at)) AS latest_pool_id,
					argMax(token_id, tuple(last_event_seq, updated_at)) AS latest_token_id,
					argMax(tick_lower, tuple(last_event_seq, updated_at)) AS latest_tick_lower,
					argMax(tick_upper, tuple(last_event_seq, updated_at)) AS latest_tick_upper,
					argMax(last_add_at, tuple(last_event_seq, updated_at)) AS latest_last_add_at,
					argMax(is_active, tuple(last_event_seq, updated_at)) AS latest_is_active
				FROM smart_lp_active_positions
				WHERE lowerUTF8(wallet_address) = ?
					AND pool_version = 'v4'
					AND last_add_at >= now() - INTERVAL %d SECOND
					%s
				GROUP BY position_key
			)
		)
		WHERE is_active = 1
			AND token_id = ''
		ORDER BY last_add_at DESC
		LIMIT %d
	`, seconds, chainFilter, limit)

	rows, err := conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]smartMoneyWalletV4PoolRef, 0, limit)
	for rows.Next() {
		var (
			positionKey string
			poolID      string
			tickLower   int32
			tickUpper   int32
		)
		if err := rows.Scan(&positionKey, &poolID, &tickLower, &tickUpper); err != nil {
			return nil, err
		}
		out = append(out, smartMoneyWalletV4PoolRef{
			PositionKey: strings.TrimSpace(positionKey),
			PoolID:      strings.ToLower(strings.TrimSpace(poolID)),
			TickLower:   int(tickLower),
			TickUpper:   int(tickUpper),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanSmartMoneyV4FallbackRefs(ctx context.Context, walletAddr string, pools []smartMoneyWalletV4PoolRef, limit int) ([]smartMoneyPositionRef, error) {
	if blockchain.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	if !common.IsHexAddress(walletAddr) || !common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		return []smartMoneyPositionRef{}, nil
	}
	if config.AppConfig.V4NFTScanFromBlock == 0 || len(pools) == 0 {
		return []smartMoneyPositionRef{}, nil
	}

	wallet := common.HexToAddress(walletAddr)
	v4pmAddr := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)

	fullBy25 := make(map[string]string, len(pools))
	for _, poolRef := range pools {
		key, ok := poolID25Key(poolRef.PoolID)
		if !ok {
			continue
		}
		if _, exists := fullBy25[key]; !exists {
			fullBy25[key] = strings.ToLower(strings.TrimSpace(poolRef.PoolID))
		}
	}
	if len(fullBy25) == 0 {
		return []smartMoneyPositionRef{}, nil
	}

	owned, err := scanERC721OwnedTokenIDsCtx(ctx, v4pmAddr, wallet, config.AppConfig.V4NFTScanFromBlock)
	if err != nil {
		return nil, err
	}
	if len(owned) == 0 {
		return []smartMoneyPositionRef{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	pm, err := blockchain.NewV4PositionManager(v4pmAddr, blockchain.Client)
	if err != nil {
		return nil, err
	}

	out := make([]smartMoneyPositionRef, 0, len(owned))
	for _, tid := range owned {
		tokenID, ok := new(big.Int).SetString(strings.TrimSpace(tid), 10)
		if !ok || tokenID == nil || tokenID.Sign() <= 0 {
			continue
		}
		raw, infoErr := pm.PositionInfoPacked(&bind.CallOpts{Context: ctx}, tokenID)
		if infoErr != nil || raw == nil {
			continue
		}
		decoded, decodeErr := decodeV4PackedPositionInfo(raw)
		if decodeErr != nil || decoded == nil {
			continue
		}
		poolID := fullBy25[strings.ToLower(strings.TrimSpace(decoded.PoolId25))]
		if poolID == "" {
			continue
		}
		out = append(out, smartMoneyPositionRef{
			WalletAddress: walletAddr,
			PoolVersion:   "v4",
			PoolID:        poolID,
			TokenID:       tokenID.String(),
			TickLower:     decoded.TickLower,
			TickUpper:     decoded.TickUpper,
		})
		if len(out) >= limit {
			break
		}
	}

	return out, nil
}

func findSmartMoneyV4FallbackRef(refs []smartMoneyPositionRef, poolID string, tickLower, tickUpper int) (smartMoneyPositionRef, bool) {
	poolID = strings.ToLower(strings.TrimSpace(poolID))
	var (
		single smartMoneyPositionRef
		count  int
	)

	for _, ref := range refs {
		if strings.ToLower(strings.TrimSpace(ref.PoolID)) != poolID {
			continue
		}
		if ref.TickLower == tickLower && ref.TickUpper == tickUpper {
			return ref, true
		}
		single = ref
		count++
	}

	if count == 1 {
		return single, true
	}
	return smartMoneyPositionRef{}, false
}

func smartMoneyPairLabel(sym0, sym1 string) string {
	pair := strings.TrimSpace(strings.TrimSpace(sym0) + "/" + strings.TrimSpace(sym1))
	if pair == "/" {
		return ""
	}
	return pair
}
