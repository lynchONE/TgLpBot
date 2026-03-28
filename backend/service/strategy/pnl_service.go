package strategy

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"context"
	"fmt"
	"log"
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

var (
	q96        = new(big.Int).Lsh(big.NewInt(1), 96)
	q128       = new(big.Int).Lsh(big.NewInt(1), 128)
	modUint256 = new(big.Int).Lsh(big.NewInt(1), 256)
)

type cachedV3FeeGrowthGlobals struct {
	global0   *big.Int
	global1   *big.Int
	updatedAt time.Time
	expires   time.Time
}

type cachedV3TickFeeGrowthOutside struct {
	fee0        *big.Int
	fee1        *big.Int
	initialized bool
	updatedAt   time.Time
	expires     time.Time
}

type cachedV4FeeGrowthGlobals struct {
	global0   *big.Int
	global1   *big.Int
	updatedAt time.Time
	expires   time.Time
}

type cachedV4TickFeeGrowthOutside struct {
	fee0      *big.Int
	fee1      *big.Int
	updatedAt time.Time
	expires   time.Time
}

type PnLService struct {
	v3FeeMu        sync.RWMutex
	v3FeeCache     map[string]cachedV3FeeGrowthGlobals
	v3TickFeeMu    sync.RWMutex
	v3TickFeeCache map[string]cachedV3TickFeeGrowthOutside

	v4FeeMu        sync.RWMutex
	v4FeeCache     map[string]cachedV4FeeGrowthGlobals
	v4TickFeeMu    sync.RWMutex
	v4TickFeeCache map[string]cachedV4TickFeeGrowthOutside
}

func NewPnLService() *PnLService {
	return &PnLService{
		v3FeeCache:     make(map[string]cachedV3FeeGrowthGlobals),
		v3TickFeeCache: make(map[string]cachedV3TickFeeGrowthOutside),
		v4FeeCache:     make(map[string]cachedV4FeeGrowthGlobals),
		v4TickFeeCache: make(map[string]cachedV4TickFeeGrowthOutside),
	}
}

type PnLInfo struct {
	InitialCostUSDT   float64
	NetInvestedUSDT   float64
	CurrentValueUSDT  float64
	AbsolutePnLUSDT   float64
	UnclaimedFeesUSDT float64 // Included in CurrentValueUSDT
	HoldingsUSDT      float64 // Current position value (excluding fees)
	DustToken0        float64
	DustToken1        float64
	DustValueUSDT     float64
}

// GetTaskPnL calculates PnL for a task
func (s *PnLService) GetTaskPnL(task *models.StrategyTask) (*PnLInfo, error) {
	// 1. Get Initial Cost
	initialCost, rec, err := s.getInitialCost(task)
	if err != nil {
		return nil, fmt.Errorf("get initial cost failed: %w", err)
	}

	// 2. Get Current Value (V3 or V4)
	var currentValue, unclaimedFees, holdingsValue float64
	var sqrtPriceX96 *big.Int

	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	if version == "v4" {
		currentValue, unclaimedFees, holdingsValue, sqrtPriceX96, err = s.getV4CurrentValue(task)
	} else {
		currentValue, unclaimedFees, holdingsValue, sqrtPriceX96, err = s.getV3CurrentValue(task)
	}

	if err != nil {
		return nil, fmt.Errorf("get current value failed: %w", err)
	}

	dust0 := big.NewInt(0)
	dust1 := big.NewInt(0)
	if rec != nil {
		if v, perr := convert.ParseBigInt(rec.OpenDust0); perr == nil && v != nil {
			dust0 = v
		}
		if v, perr := convert.ParseBigInt(rec.OpenDust1); perr == nil && v != nil {
			dust1 = v
		}
	}

	dustValueUSDT := 0.0
	dustUSDTValue := 0.0
	if sqrtPriceX96 != nil && (dust0.Sign() > 0 || dust1.Sign() > 0) {
		dustValueUSDT, _, _ = s.calculateUSDTValue(
			task,
			dust0, dust1,
			big.NewInt(0), big.NewInt(0),
			big.NewInt(0), big.NewInt(0),
			sqrtPriceX96,
		)
	}

	openSpentWei := big.NewInt(0)
	if rec != nil {
		if v, perr := convert.ParseBigInt(rec.OpenUSDTSpent); perr == nil && v != nil && v.Sign() > 0 {
			openSpentWei = v
		}
	}
	expectedWei, _ := convert.FloatUSDTToWei(task.AmountUSDT)
	if expectedWei == nil {
		expectedWei = big.NewInt(0)
	}

	_, _, stableDecimals := stableTokenForChain(task.Chain)
	dustUSDTWei := big.NewInt(0) // internal USD(1e18) representation
	addStableDust := func(raw *big.Int) {
		if raw == nil || raw.Sign() <= 0 {
			return
		}
		scaled, err := convert.ScaleDecimals(raw, stableDecimals, 18)
		if err != nil || scaled == nil {
			// Fallback: keep raw units (best-effort) to avoid breaking the PnL view.
			dustUSDTWei.Add(dustUSDTWei, raw)
			return
		}
		dustUSDTWei.Add(dustUSDTWei, scaled)
	}
	if dust0.Sign() > 0 && isPrimaryStableToken(task.Chain, task.Token0Symbol, task.Token0Address) {
		addStableDust(dust0)
	}
	if dust1.Sign() > 0 && isPrimaryStableToken(task.Chain, task.Token1Symbol, task.Token1Address) {
		addStableDust(dust1)
	}

	if dust0.Sign() > 0 && isPrimaryStableToken(task.Chain, task.Token0Symbol, task.Token0Address) {
		dustUSDTValue += weiToFloat(dust0, pricing.GetTokenDecimals(task.Chain, task.Token0Address))
	}
	if dust1.Sign() > 0 && isPrimaryStableToken(task.Chain, task.Token1Symbol, task.Token1Address) {
		dustUSDTValue += weiToFloat(dust1, pricing.GetTokenDecimals(task.Chain, task.Token1Address))
	}

	// NetInvestedUSDT aims to reflect the USDT amount actually locked in the position.
	// For non-USDT dust, it should always be excluded (since it was bought with USDT spent).
	// For USDT dust, OpenUSDTSpent is usually derived from wallet USDT delta and already excludes refunded USDT dust.
	// But if OpenUSDTSpent was recorded via fallback (e.g., due to RPC lag), we must subtract USDT dust too.
	excludeUSDTReturnedDust := true
	if dustUSDTWei.Sign() > 0 && openSpentWei.Sign() > 0 && expectedWei.Sign() > 0 {
		// Tolerance: 0.001 USDT (1e15 wei) to cover DB rounding and float->wei conversions.
		const tolWeiStr = "1000000000000000"
		tolWei, _ := new(big.Int).SetString(tolWeiStr, 10)

		sum := new(big.Int).Add(openSpentWei, dustUSDTWei)
		sumDiff := new(big.Int).Sub(sum, expectedWei)
		if sumDiff.Sign() < 0 {
			sumDiff.Neg(sumDiff)
		}
		spentDiff := new(big.Int).Sub(openSpentWei, expectedWei)
		if spentDiff.Sign() < 0 {
			spentDiff.Neg(spentDiff)
		}

		// If openSpent + dustUSDT ~= expected: openSpent likely already excluded the refunded USDT dust.
		if tolWei != nil && sumDiff.Cmp(tolWei) <= 0 {
			excludeUSDTReturnedDust = true
		} else if tolWei != nil && spentDiff.Cmp(tolWei) <= 0 {
			// If openSpent ~= expected while dustUSDT > 0, OpenUSDTSpent likely includes the dust (fallback record).
			excludeUSDTReturnedDust = false
		}
	}

	dustToSubtract := dustValueUSDT
	if excludeUSDTReturnedDust {
		dustToSubtract = dustValueUSDT - dustUSDTValue
		if dustToSubtract < 0 {
			dustToSubtract = 0
		}
	}

	netInvested := initialCost - dustToSubtract
	if netInvested < 0 {
		netInvested = 0
	}

	dec0 := pricing.DefaultTokenDecimals
	dec1 := pricing.DefaultTokenDecimals
	if dust0.Sign() > 0 {
		dec0 = pricing.GetTokenDecimals(task.Chain, task.Token0Address)
	}
	if dust1.Sign() > 0 {
		dec1 = pricing.GetTokenDecimals(task.Chain, task.Token1Address)
	}

	return &PnLInfo{
		InitialCostUSDT:   initialCost,
		NetInvestedUSDT:   netInvested,
		CurrentValueUSDT:  currentValue,
		AbsolutePnLUSDT:   currentValue - netInvested,
		UnclaimedFeesUSDT: unclaimedFees,
		HoldingsUSDT:      holdingsValue,
		DustToken0:        weiToFloat(dust0, dec0),
		DustToken1:        weiToFloat(dust1, dec1),
		DustValueUSDT:     dustValueUSDT,
	}, nil
}

func (s *PnLService) getInitialCost(task *models.StrategyTask) (float64, *models.TradeRecord, error) {
	var rec models.TradeRecord
	err := database.DB.
		Where("user_id = ? AND task_id = ? AND status = ?", task.UserID, task.ID, models.TradeStatusOpen).
		Order("opened_at DESC").
		First(&rec).Error

	if err != nil {
		// Fallback to task.AmountUSDT if no record found (legacy or error)
		return task.AmountUSDT, nil, nil
	}

	amountWei, err := convert.ParseBigInt(rec.OpenUSDTSpent)
	if err != nil {
		return 0, nil, err
	}
	if amountWei == nil || amountWei.Sign() <= 0 {
		return task.AmountUSDT, &rec, nil
	}
	return weiToFloat(amountWei, 18), &rec, nil
}

func (s *PnLService) getV3CurrentValue(task *models.StrategyTask) (totalVal, feesVal, holdingsVal float64, sqrtPriceX96 *big.Int, err error) {
	// 1. Get Position Info (Liquidity + Fees)
	if task.V3TokenID == "" || task.V3TokenID == "0" {
		return 0, 0, 0, nil, fmt.Errorf("no V3 token ID")
	}
	tokenId, _ := new(big.Int).SetString(task.V3TokenID, 10)

	chain := config.NormalizeChain(task.Chain)
	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return 0, 0, 0, nil, err
	}

	pmAddrStr := strings.TrimSpace(task.V3PositionManagerAddress)
	if !common.IsHexAddress(pmAddrStr) && config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
			if common.IsHexAddress(cc.DefaultV3PositionManagerAddress) {
				pmAddrStr = strings.TrimSpace(cc.DefaultV3PositionManagerAddress)
			} else {
				for _, dep := range cc.V3Deployments {
					if common.IsHexAddress(dep.PositionManagerAddress) {
						pmAddrStr = strings.TrimSpace(dep.PositionManagerAddress)
						break
					}
				}
			}
		}
	}
	if !common.IsHexAddress(pmAddrStr) {
		ex := strings.ToLower(strings.TrimSpace(task.Exchange))
		if strings.Contains(ex, "pancake") && config.AppConfig != nil && common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
			pmAddrStr = strings.TrimSpace(config.AppConfig.PancakeV3PositionManagerAddress)
		} else if strings.Contains(ex, "uniswap") && config.AppConfig != nil && common.IsHexAddress(config.AppConfig.UniswapV3PositionManagerAddress) {
			pmAddrStr = strings.TrimSpace(config.AppConfig.UniswapV3PositionManagerAddress)
		}
	}
	if !common.IsHexAddress(pmAddrStr) {
		return 0, 0, 0, nil, fmt.Errorf("V3 position manager address missing")
	}
	pmAddress := common.HexToAddress(pmAddrStr)
	pm, err := blockchain.NewV3PositionManager(pmAddress, client)
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("init V3 PM failed: %w", err)
	}

	snapshotBlock := uint64(0)
	blockCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	latestBlock, blockErr := client.BlockNumber(blockCtx)
	cancel()
	if blockErr != nil {
		log.Printf("[PnL] V3 snapshot block read failed: tokenId=%s err=%v", task.V3TokenID, blockErr)
	} else {
		snapshotBlock = latestBlock
	}

	var pos *blockchain.V3PositionInfo
	if snapshotBlock > 0 {
		pos, err = pm.Positions(&bind.CallOpts{BlockNumber: new(big.Int).SetUint64(snapshotBlock)}, tokenId)
	} else {
		pos, err = pm.Positions(nil, tokenId)
	}
	if err != nil {
		// Check ownerOf to see if it exists/burned
		return 0, 0, 0, nil, fmt.Errorf("read positions failed: %w", err)
	}

	// 2. Resolve pool address (factory-derived is the source of truth).
	poolAddr := common.Address{}
	if common.IsHexAddress(task.PoolId) {
		poolAddr = common.HexToAddress(task.PoolId)
	}
	if resolved, rErr := resolveV3PoolAddress(chain, nil, 10*time.Second, pmAddress, pos.Token0, pos.Token1, pos.Fee); rErr == nil && resolved != (common.Address{}) {
		poolAddr = resolved
	}
	if poolAddr == (common.Address{}) {
		return 0, 0, 0, nil, fmt.Errorf("V3 pool address missing")
	}

	// 3. Get Current Price (Slot0)
	currentTick := 0
	if snapshotBlock > 0 {
		sqrtPriceX96, currentTick, err = blockchain.GetV3PoolSlot0AtBlockWithClient(client, poolAddr, snapshotBlock)
	} else {
		sqrtPriceX96, currentTick, err = blockchain.GetV3PoolSlot0WithClient(client, poolAddr)
	}
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("get slot0 failed: %w", err)
	}

	// 4. Calculate Amounts from Liquidity
	sqrtPriceA, _ := pool.SqrtRatioAtTick(int32(pos.TickLower))
	sqrtPriceB, _ := pool.SqrtRatioAtTick(int32(pos.TickUpper))

	amount0, amount1 := pool.AmountsForLiquidity(sqrtPriceX96, sqrtPriceA, sqrtPriceB, pos.Liquidity)

	fees0 := big.NewInt(0)
	fees1 := big.NewInt(0)
	if snapshotBlock > 0 {
		if fee0, fee1, feeErr := pool.CalcV3UnclaimedFeesAtBlock(poolAddr, currentTick, pos, snapshotBlock); feeErr == nil && fee0 != nil && fee1 != nil {
			fees0 = fee0
			fees1 = fee1
			log.Printf("[PnL] V3 consistent snapshot fees tokenId=%s fees0=%s fees1=%s block=%d", task.V3TokenID, fees0.String(), fees1.String(), snapshotBlock)
		} else if feeErr != nil {
			log.Printf("[PnL] V3 consistent snapshot fee calc failed: tokenId=%s err=%v", task.V3TokenID, feeErr)
		}
	} else if fee0, fee1, usedStale, age, feeErr := s.calcV3UnclaimedFeesCached(chain, poolAddr, currentTick, pos); fee0 != nil && fee1 != nil {
		fees0 = fee0
		fees1 = fee1
		if feeErr != nil {
			if usedStale {
				log.Printf("[PnL] V3 fee cache fallback (%ds) tokenId=%s err=%v", int(age.Seconds()), task.V3TokenID, feeErr)
			} else {
				log.Printf("[PnL] V3 fee calculation failed: tokenId=%s err=%v", task.V3TokenID, feeErr)
			}
		}
	} else if feeErr != nil {
		log.Printf("[PnL] V3 鎵嬬画璐硅绠楀け璐? tokenId=%s err=%v", task.V3TokenID, feeErr)
	}

	// 5. Total Amounts = Position + Unclaimed Fees
	total0 := new(big.Int).Add(amount0, fees0)
	total1 := new(big.Int).Add(amount1, fees1)

	log.Printf("[PnL] V3 鎵嬬画璐? tokenId=%s owed0=%s owed1=%s amount0=%s amount1=%s",
		task.V3TokenID, fees0.String(), fees1.String(), amount0.String(), amount1.String())

	// 6. Convert to USDT
	// Helper to determine price:
	// If Token0 is USDT: Value = Total0 + Total1 * Price(Token1 in USDT)
	// If Token1 is USDT: Value = Total1 + Total0 * Price(Token0 in USDT)

	// Price0 = 1 (if USDT), Price1 = ?
	// sqrtPriceX96 is sqrt(token1/token0) * 2^96
	// priceT1_per_T0 = (sqrtPriceX96 / 2^96)^2

	usdtVal, feesUsdt, holdUsdt := s.calculateUSDTValue(task, total0, total1, fees0, fees1, amount0, amount1, sqrtPriceX96)
	return usdtVal, feesUsdt, holdUsdt, sqrtPriceX96, nil
}

func (s *PnLService) getV4CurrentValue(task *models.StrategyTask) (totalVal, feesVal, holdingsVal float64, sqrtPriceX96 *big.Int, err error) {
	// Best-effort: read fees from V4 PositionManager; fallback to 0.
	liqStr := task.CurrentLiquidity
	if liqStr == "" {
		liqStr = "0"
	}
	liquidity, _ := new(big.Int).SetString(liqStr, 10)
	if liquidity == nil {
		liquidity = big.NewInt(0)
	}
	fees0 := big.NewInt(0)
	fees1 := big.NewInt(0)

	var v4pos *blockchain.V4PositionInfo
	snapshotBlock := uint64(0)

	if common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		tokenId, parseErr := convert.ParseBigInt(task.V4TokenID)
		if parseErr == nil && tokenId.Sign() > 0 {
			blockCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			latestBlock, blockErr := blockchain.Client.BlockNumber(blockCtx)
			cancel()
			if blockErr != nil {
				log.Printf("[PnL] V4 snapshot block read failed: tokenId=%s err=%v", task.V4TokenID, blockErr)
			} else {
				snapshotBlock = latestBlock
			}
			v4pmAddr := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
			poolMgr := common.Address{}
			if common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
				poolMgr = common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
			}
			var pos *blockchain.V4PositionInfo
			var posErr error
			if snapshotBlock > 0 {
				pos, posErr = blockchain.GetV4PositionInfoAtBlock(v4pmAddr, poolMgr, task.PoolId, tokenId, snapshotBlock)
			} else {
				pos, posErr = blockchain.GetV4PositionInfo(v4pmAddr, poolMgr, task.PoolId, tokenId)
			}
			if posErr != nil {
				log.Printf("[PnL] V4 position info read failed: tokenId=%s err=%v", task.V4TokenID, posErr)
			}
			if pos != nil {
				v4pos = pos
				if pos.Liquidity != nil {
					liquidity = pos.Liquidity
				}
			}
		}
	}

	// 1. Get Current Price (Slot0 via StateView)
	if config.AppConfig.UniswapV4StateViewAddress == "" || config.AppConfig.UniswapV4PoolManagerAddress == "" {
		return 0, 0, 0, nil, fmt.Errorf("V4 config missing")
	}
	stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
	poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
	currentTick := 0

	if snapshotBlock > 0 {
		sqrtPriceX96, currentTick, err = blockchain.GetUniswapV4PoolSlot0ViaStateViewAtBlock(stateView, poolManager, task.PoolId, snapshotBlock)
	} else {
		sqrtPriceX96, currentTick, err = blockchain.GetUniswapV4PoolSlot0ViaStateView(stateView, poolManager, task.PoolId)
	}
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("get V4 slot0 failed: %w", err)
	}

	// 灏濊瘯璁＄畻瀹炴椂鎵嬬画璐癸紙濡傛灉鏈変粨浣嶄俊鎭級
	if v4pos != nil && v4pos.Liquidity != nil && v4pos.Liquidity.Sign() > 0 {
		if snapshotBlock > 0 {
			realFees0, realFees1, feeErr := pool.CalcV4UnclaimedFeesAtBlock(stateView, poolManager, task.PoolId, currentTick, v4pos, snapshotBlock)
			if feeErr == nil && realFees0 != nil && realFees1 != nil {
				fees0 = realFees0
				fees1 = realFees1
				log.Printf("[PnL] V4 consistent snapshot fees tokenId=%s fees0=%s fees1=%s block=%d", task.V4TokenID, fees0.String(), fees1.String(), snapshotBlock)
			} else if feeErr != nil {
				log.Printf("[PnL] V4 consistent snapshot fee calc failed: tokenId=%s err=%v", task.V4TokenID, feeErr)
			}
		} else if realFees0, realFees1, usedStale, age, feeErr := s.calcV4UnclaimedFeesCachedUnified(stateView, poolManager, task.PoolId, currentTick, v4pos); realFees0 != nil && realFees1 != nil {
			fees0 = realFees0
			fees1 = realFees1
			if feeErr == nil {
				log.Printf("[PnL] V4 realtime fees tokenId=%s fees0=%s fees1=%s", task.V4TokenID, fees0.String(), fees1.String())
			} else if usedStale {
				log.Printf("[PnL] V4 fee cache fallback (%ds) tokenId=%s err=%v", int(age.Seconds()), task.V4TokenID, feeErr)
			} else {
				log.Printf("[PnL] V4 fee calculation failed: tokenId=%s err=%v", task.V4TokenID, feeErr)
			}
		} else if feeErr != nil {
			log.Printf("[PnL] V4 fee calculation failed: %v", feeErr)
		}
	}

	// 2. Calculate Amounts
	tickLower := int32(task.TickLower)
	tickUpper := int32(task.TickUpper)
	if v4pos != nil && (v4pos.TickLower != 0 || v4pos.TickUpper != 0) && v4pos.TickLower < v4pos.TickUpper {
		tickLower = int32(v4pos.TickLower)
		tickUpper = int32(v4pos.TickUpper)
	}
	sqrtPriceA, _ := pool.SqrtRatioAtTick(tickLower)
	sqrtPriceB, _ := pool.SqrtRatioAtTick(tickUpper)

	amount0, amount1 := pool.AmountsForLiquidity(sqrtPriceX96, sqrtPriceA, sqrtPriceB, liquidity)

	// 3. Convert to USDT (include owed fees)
	total0 := new(big.Int).Add(amount0, fees0)
	total1 := new(big.Int).Add(amount1, fees1)
	usdtVal, feesUsdt, holdUsdt := s.calculateUSDTValue(task, total0, total1, fees0, fees1, amount0, amount1, sqrtPriceX96)

	return usdtVal, feesUsdt, holdUsdt, sqrtPriceX96, nil
}

func (s *PnLService) calculateUSDTValue(
	task *models.StrategyTask,
	total0, total1, fees0, fees1, hold0, hold1, sqrtPriceX96 *big.Int,
) (totalUSDT, feesUSDT, holdUSDT float64) {
	token0Symbol := strings.ToUpper(strings.TrimSpace(task.Token0Symbol))
	token1Symbol := strings.ToUpper(strings.TrimSpace(task.Token1Symbol))
	isStable0 := pricing.IsStableSymbol(token0Symbol) || pricing.IsStableAddress(task.Chain, task.Token0Address)
	isStable1 := pricing.IsStableSymbol(token1Symbol) || pricing.IsStableAddress(task.Chain, task.Token1Address)

	dec0 := pricing.GetTokenDecimals(task.Chain, task.Token0Address)
	dec1 := pricing.GetTokenDecimals(task.Chain, task.Token1Address)
	if dec0 <= 0 {
		dec0 = pricing.DefaultTokenDecimals
	}
	if dec1 <= 0 {
		dec1 = pricing.DefaultTokenDecimals
	}

	// Determine price relation (human units).
	// Uniswap: sqrtPriceX96 = sqrt(token1/token0) * 2^96, where token amounts are in raw units.
	// So: priceRaw = token1_units per 1 token0_unit = (sqrtX96 / 2^96)^2.
	// Convert to human units: priceHuman = priceRaw * 10^(dec0-dec1).
	priceToken1PerToken0 := 0.0
	if sqrtPriceX96 != nil && sqrtPriceX96.Sign() > 0 {
		p := new(big.Float).SetInt(sqrtPriceX96)
		q := new(big.Float).SetInt(q96)
		p.Quo(p, q)
		p.Mul(p, p) // priceRaw = token1_units per 1 token0_unit
		priceRaw, _ := p.Float64()
		priceToken1PerToken0 = priceRaw * math.Pow(10, float64(dec0-dec1))
	}

	// Convert raw units to human amounts
	t0 := weiToFloat(total0, dec0)
	t1 := weiToFloat(total1, dec1)
	f0 := weiToFloat(fees0, dec0)
	f1 := weiToFloat(fees1, dec1)
	h0 := weiToFloat(hold0, dec0)
	h1 := weiToFloat(hold1, dec1)

	if isStable0 && !isStable1 {
		// Token0 is stable. Value = T0 + T1 * (Price of T1 in stable)
		// priceToken1PerToken0 means 1 T0 = P T1. So Price of T1 in T0 = 1/P.
		priceT1InUSDT := 0.0
		if priceToken1PerToken0 > 0 {
			priceT1InUSDT = 1.0 / priceToken1PerToken0
		}

		totalUSDT = t0 + t1*priceT1InUSDT
		feesUSDT = f0 + f1*priceT1InUSDT
		holdUSDT = h0 + h1*priceT1InUSDT
	} else if isStable1 && !isStable0 {
		// Token1 is stable. Value = T1 + T0 * (Price of T0 in stable)
		// priceToken1PerToken0 means 1 T0 = P T1. Since T1 is stable, P is Price of T0 in stable terms.
		priceT0InUSDT := priceToken1PerToken0

		totalUSDT = t1 + t0*priceT0InUSDT
		feesUSDT = f1 + f0*priceT0InUSDT
		holdUSDT = h1 + h0*priceT0InUSDT
	} else if isStable0 && isStable1 {
		totalUSDT = t0 + t1
		feesUSDT = f0 + f1
		holdUSDT = h0 + h1
	} else {
		// Neither side is stable. Try to estimate value via the chain native gas token price (WBNB/WETH...).
		chain := config.NormalizeChain(task.Chain)
		wrappedSym := ""
		wrappedAddr := ""
		if config.AppConfig != nil {
			if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
				wrappedSym = strings.ToUpper(strings.TrimSpace(cc.WrappedNativeSymbol))
				wrappedAddr = strings.TrimSpace(cc.WrappedNativeAddress)
			}
		}
		nativeSym := strings.TrimPrefix(wrappedSym, "W")
		if nativeSym == "" {
			switch chain {
			case "base":
				wrappedSym = "WETH"
				nativeSym = "ETH"
			default:
				wrappedSym = "WBNB"
				nativeSym = "BNB"
			}
		}
		isWBNB0 := token0Symbol == wrappedSym || token0Symbol == nativeSym
		isWBNB1 := token1Symbol == wrappedSym || token1Symbol == nativeSym
		if common.IsHexAddress(wrappedAddr) {
			if strings.EqualFold(strings.TrimSpace(task.Token0Address), wrappedAddr) {
				isWBNB0 = true
			}
			if strings.EqualFold(strings.TrimSpace(task.Token1Address), wrappedAddr) {
				isWBNB1 = true
			}
		}

		// Native price in USD/USDT (best-effort).
		bnbPriceUSDT := pricing.GetNativePriceUSD(chain)

		if isWBNB0 && !isWBNB1 {
			// Token0 is WBNB, price is in WBNB terms
			// Value = T0_WBNB * BNB_USDT_Price + T1 * (T1_price_in_WBNB * BNB_USDT_Price)
			// priceToken1PerToken0 = amount of T1 per 1 WBNB
			// So T1 price in WBNB = 1 / priceToken1PerToken0
			priceT1InWBNB := 0.0
			if priceToken1PerToken0 > 0 {
				priceT1InWBNB = 1.0 / priceToken1PerToken0
			}
			totalUSDT = t0*bnbPriceUSDT + t1*priceT1InWBNB*bnbPriceUSDT
			feesUSDT = f0*bnbPriceUSDT + f1*priceT1InWBNB*bnbPriceUSDT
			holdUSDT = h0*bnbPriceUSDT + h1*priceT1InWBNB*bnbPriceUSDT
			log.Printf("[PnL] 闈炵ǔ瀹氬竵瀵?%s/%s: 浣跨敤 WBNB(Token0) 浠锋牸浼扮畻, bnbPrice=%.2f", token0Symbol, token1Symbol, bnbPriceUSDT)
		} else if isWBNB1 && !isWBNB0 {
			// Token1 is WBNB
			// priceToken1PerToken0 = amount of WBNB per 1 T0
			// So T0 price in WBNB = priceToken1PerToken0
			priceT0InWBNB := priceToken1PerToken0
			totalUSDT = t1*bnbPriceUSDT + t0*priceT0InWBNB*bnbPriceUSDT
			feesUSDT = f1*bnbPriceUSDT + f0*priceT0InWBNB*bnbPriceUSDT
			holdUSDT = h1*bnbPriceUSDT + h0*priceT0InWBNB*bnbPriceUSDT
			log.Printf("[PnL] 闈炵ǔ瀹氬竵瀵?%s/%s: 浣跨敤 WBNB(Token1) 浠锋牸浼扮畻, bnbPrice=%.2f", token0Symbol, token1Symbol, bnbPriceUSDT)
		} else {
			// Neither is WBNB or stable, cannot estimate
			log.Printf("[PnL] 璀﹀憡: 鏃犳硶浼扮畻闈炵ǔ瀹氬竵瀵?%s/%s 鐨?USDT 浠峰€?(Task #%d)", token0Symbol, token1Symbol, task.ID)
			return 0, 0, 0
		}
	}

	return totalUSDT, feesUSDT, holdUSDT
}

// GetBNBPriceUSDT 浠?PancakeSwap V3 WBNB/USDT 姹犲瓙鑾峰彇 BNB 瀹炴椂浠锋牸
func (s *PnLService) GetBNBPriceUSDT() float64 {
	return pricing.GetBNBPriceUSDT()
}

func weiToFloat(wei *big.Int, decimals int) float64 {
	f := new(big.Float).SetInt(wei)
	div := new(big.Float).SetFloat64(math.Pow(10, float64(decimals)))
	f.Quo(f, div)
	val, _ := f.Float64()
	return val
}

func stableTokenForChain(chain string) (symbol string, addr string, decimals int) {
	chain = config.NormalizeChain(chain)
	symbol = "USDT"
	decimals = 18

	if config.AppConfig == nil {
		return
	}
	if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
		if strings.TrimSpace(cc.StableSymbol) != "" {
			symbol = strings.TrimSpace(cc.StableSymbol)
		}
		addr = strings.TrimSpace(cc.StableAddress)
		if cc.StableDecimals > 0 {
			decimals = cc.StableDecimals
		}
		return
	}

	// Backward-compatible fallback for legacy single-chain config.
	addr = strings.TrimSpace(config.AppConfig.USDTAddress)
	return
}

func isPrimaryStableToken(chain, symbol, addr string) bool {
	stableSym, stableAddr, _ := stableTokenForChain(chain)
	if stableSym != "" && strings.EqualFold(strings.TrimSpace(symbol), stableSym) {
		return true
	}
	if common.IsHexAddress(stableAddr) && strings.EqualFold(strings.TrimSpace(addr), stableAddr) {
		return true
	}
	// Last-resort fallback: treat USDT symbol as stable.
	if strings.EqualFold(strings.TrimSpace(symbol), "USDT") {
		return true
	}
	return false
}

func (s *PnLService) getV3FeeGrowthGlobalsCached(chain string, poolAddress common.Address) (*big.Int, *big.Int, bool, time.Duration, error) {
	if (poolAddress == common.Address{}) {
		return nil, nil, false, 0, fmt.Errorf("empty pool address")
	}

	chain = config.NormalizeChain(chain)
	now := time.Now()
	key := chain + "|" + strings.ToLower(poolAddress.Hex())

	s.v3FeeMu.RLock()
	if c, ok := s.v3FeeCache[key]; ok && c.global0 != nil && c.global1 != nil && c.expires.After(now) {
		g0 := new(big.Int).Set(c.global0)
		g1 := new(big.Int).Set(c.global1)
		s.v3FeeMu.RUnlock()
		return g0, g1, false, 0, nil
	}

	var stale0 *big.Int
	var stale1 *big.Int
	var staleAt time.Time
	if c, ok := s.v3FeeCache[key]; ok && c.global0 != nil && c.global1 != nil {
		stale0 = new(big.Int).Set(c.global0)
		stale1 = new(big.Int).Set(c.global1)
		staleAt = c.updatedAt
	}
	s.v3FeeMu.RUnlock()

	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return nil, nil, false, 0, err
	}
	g0, g1, err := blockchain.GetV3PoolFeeGrowthGlobalsWithClient(client, poolAddress)
	if err == nil && g0 != nil && g1 != nil {
		s.v3FeeMu.Lock()
		s.v3FeeCache[key] = cachedV3FeeGrowthGlobals{
			global0:   new(big.Int).Set(g0),
			global1:   new(big.Int).Set(g1),
			updatedAt: now,
			expires:   now.Add(2 * time.Second),
		}
		s.v3FeeMu.Unlock()
		return g0, g1, false, 0, nil
	}

	if stale0 != nil && stale1 != nil && !staleAt.IsZero() && now.Sub(staleAt) <= 30*time.Second {
		return stale0, stale1, true, now.Sub(staleAt), err
	}
	return nil, nil, false, 0, err
}

func (s *PnLService) getV3TickFeeGrowthOutsideCached(chain string, poolAddress common.Address, tick int) (*big.Int, *big.Int, bool, bool, time.Duration, error) {
	if (poolAddress == common.Address{}) {
		return nil, nil, false, false, 0, fmt.Errorf("empty pool address")
	}

	chain = config.NormalizeChain(chain)
	now := time.Now()
	key := chain + "|" + strings.ToLower(poolAddress.Hex()) + "|" + strconv.Itoa(tick)

	s.v3TickFeeMu.RLock()
	if c, ok := s.v3TickFeeCache[key]; ok && c.fee0 != nil && c.fee1 != nil && c.expires.After(now) {
		f0 := new(big.Int).Set(c.fee0)
		f1 := new(big.Int).Set(c.fee1)
		initialized := c.initialized
		s.v3TickFeeMu.RUnlock()
		return f0, f1, initialized, false, 0, nil
	}

	var stale0 *big.Int
	var stale1 *big.Int
	var staleInit bool
	var staleAt time.Time
	if c, ok := s.v3TickFeeCache[key]; ok && c.fee0 != nil && c.fee1 != nil {
		stale0 = new(big.Int).Set(c.fee0)
		stale1 = new(big.Int).Set(c.fee1)
		staleInit = c.initialized
		staleAt = c.updatedAt
	}
	s.v3TickFeeMu.RUnlock()

	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return nil, nil, false, false, 0, err
	}
	f0, f1, initialized, err := blockchain.GetV3PoolTickFeeGrowthOutsideWithClient(client, poolAddress, tick)
	if err == nil && f0 != nil && f1 != nil {
		s.v3TickFeeMu.Lock()
		s.v3TickFeeCache[key] = cachedV3TickFeeGrowthOutside{
			fee0:        new(big.Int).Set(f0),
			fee1:        new(big.Int).Set(f1),
			initialized: initialized,
			updatedAt:   now,
			expires:     now.Add(20 * time.Second),
		}
		s.v3TickFeeMu.Unlock()
		return f0, f1, initialized, false, 0, nil
	}

	if stale0 != nil && stale1 != nil && !staleAt.IsZero() && now.Sub(staleAt) <= 2*time.Minute {
		return stale0, stale1, staleInit, true, now.Sub(staleAt), err
	}
	return nil, nil, false, false, 0, err
}

func (s *PnLService) calcV3UnclaimedFeesCached(chain string, poolAddr common.Address, currentTick int, pos *blockchain.V3PositionInfo) (*big.Int, *big.Int, bool, time.Duration, error) {
	if pos == nil {
		return nil, nil, false, 0, fmt.Errorf("position info missing")
	}

	if pos.Liquidity == nil || pos.Liquidity.Sign() == 0 {
		return cloneBig(pos.TokensOwed0), cloneBig(pos.TokensOwed1), false, 0, nil
	}
	if poolAddr == (common.Address{}) {
		return nil, nil, false, 0, fmt.Errorf("pool address missing")
	}

	global0, global1, staleG, ageG, errG := s.getV3FeeGrowthGlobalsCached(chain, poolAddr)
	if errG != nil && (global0 == nil || global1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read feeGrowthGlobal failed: %w", errG)
	}
	lower0, lower1, _, staleL, ageL, errL := s.getV3TickFeeGrowthOutsideCached(chain, poolAddr, pos.TickLower)
	if errL != nil && (lower0 == nil || lower1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read tickLower feeGrowthOutside failed: %w", errL)
	}
	upper0, upper1, _, staleU, ageU, errU := s.getV3TickFeeGrowthOutsideCached(chain, poolAddr, pos.TickUpper)
	if errU != nil && (upper0 == nil || upper1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read tickUpper feeGrowthOutside failed: %w", errU)
	}

	usedStale := staleG || staleL || staleU
	age := time.Duration(0)
	if staleG && ageG > age {
		age = ageG
	}
	if staleL && ageL > age {
		age = ageL
	}
	if staleU && ageU > age {
		age = ageU
	}

	fees0, fees1, calcErr := pool.CalcV3UnclaimedFeesFromGrowths(currentTick, pos, global0, global1, lower0, lower1, upper0, upper1)
	if calcErr != nil {
		return nil, nil, usedStale, age, calcErr
	}

	var errOut error
	if usedStale {
		if errG != nil {
			errOut = errG
		} else if errL != nil {
			errOut = errL
		} else if errU != nil {
			errOut = errU
		}
	}
	return fees0, fees1, usedStale, age, errOut
}

func normalizePoolIDKey(poolID string) string {
	poolIDKey := strings.ToLower(strings.TrimSpace(poolID))
	if poolIDKey != "" && !strings.HasPrefix(poolIDKey, "0x") {
		poolIDKey = "0x" + poolIDKey
	}
	return poolIDKey
}

func (s *PnLService) getV4FeeGrowthGlobalsCached(stateView, poolManager common.Address, poolID string) (*big.Int, *big.Int, bool, time.Duration, error) {
	if (stateView == common.Address{}) || (poolManager == common.Address{}) {
		return nil, nil, false, 0, fmt.Errorf("V4 stateView/poolManager missing")
	}

	now := time.Now()
	key := strings.ToLower(stateView.Hex()) + "|" + strings.ToLower(poolManager.Hex()) + "|" + normalizePoolIDKey(poolID)

	s.v4FeeMu.RLock()
	if c, ok := s.v4FeeCache[key]; ok && c.global0 != nil && c.global1 != nil && c.expires.After(now) {
		g0 := new(big.Int).Set(c.global0)
		g1 := new(big.Int).Set(c.global1)
		s.v4FeeMu.RUnlock()
		return g0, g1, false, 0, nil
	}

	var stale0 *big.Int
	var stale1 *big.Int
	var staleAt time.Time
	if c, ok := s.v4FeeCache[key]; ok && c.global0 != nil && c.global1 != nil {
		stale0 = new(big.Int).Set(c.global0)
		stale1 = new(big.Int).Set(c.global1)
		staleAt = c.updatedAt
	}
	s.v4FeeMu.RUnlock()

	g0, g1, err := blockchain.GetV4PoolFeeGrowthGlobals(stateView, poolManager, poolID)
	if err == nil && g0 != nil && g1 != nil {
		s.v4FeeMu.Lock()
		s.v4FeeCache[key] = cachedV4FeeGrowthGlobals{
			global0:   new(big.Int).Set(g0),
			global1:   new(big.Int).Set(g1),
			updatedAt: now,
			expires:   now.Add(2 * time.Second),
		}
		s.v4FeeMu.Unlock()
		return g0, g1, false, 0, nil
	}

	if stale0 != nil && stale1 != nil && !staleAt.IsZero() && now.Sub(staleAt) <= 30*time.Second {
		return stale0, stale1, true, now.Sub(staleAt), err
	}
	return nil, nil, false, 0, err
}

func (s *PnLService) getV4TickFeeGrowthOutsideCached(stateView, poolManager common.Address, poolID string, tick int) (*big.Int, *big.Int, bool, time.Duration, error) {
	if (stateView == common.Address{}) || (poolManager == common.Address{}) {
		return nil, nil, false, 0, fmt.Errorf("V4 stateView/poolManager missing")
	}

	now := time.Now()
	key := strings.ToLower(stateView.Hex()) + "|" + strings.ToLower(poolManager.Hex()) + "|" + normalizePoolIDKey(poolID) + "|" + strconv.Itoa(tick)

	s.v4TickFeeMu.RLock()
	if c, ok := s.v4TickFeeCache[key]; ok && c.fee0 != nil && c.fee1 != nil && c.expires.After(now) {
		f0 := new(big.Int).Set(c.fee0)
		f1 := new(big.Int).Set(c.fee1)
		s.v4TickFeeMu.RUnlock()
		return f0, f1, false, 0, nil
	}

	var stale0 *big.Int
	var stale1 *big.Int
	var staleAt time.Time
	if c, ok := s.v4TickFeeCache[key]; ok && c.fee0 != nil && c.fee1 != nil {
		stale0 = new(big.Int).Set(c.fee0)
		stale1 = new(big.Int).Set(c.fee1)
		staleAt = c.updatedAt
	}
	s.v4TickFeeMu.RUnlock()

	f0, f1, err := blockchain.GetV4TickFeeGrowthOutside(stateView, poolManager, poolID, tick)
	if err == nil && f0 != nil && f1 != nil {
		s.v4TickFeeMu.Lock()
		s.v4TickFeeCache[key] = cachedV4TickFeeGrowthOutside{
			fee0:      new(big.Int).Set(f0),
			fee1:      new(big.Int).Set(f1),
			updatedAt: now,
			expires:   now.Add(20 * time.Second),
		}
		s.v4TickFeeMu.Unlock()
		return f0, f1, false, 0, nil
	}

	if stale0 != nil && stale1 != nil && !staleAt.IsZero() && now.Sub(staleAt) <= 2*time.Minute {
		return stale0, stale1, true, now.Sub(staleAt), err
	}
	return nil, nil, false, 0, err
}

func (s *PnLService) calcV4UnclaimedFeesCached(stateView, poolManager common.Address, poolID string, currentTick int, pos *blockchain.V4PositionInfo) (*big.Int, *big.Int, bool, time.Duration, error) {
	if pos == nil {
		return nil, nil, false, 0, fmt.Errorf("position info missing")
	}
	owed0 := cloneBig(pos.TokensOwed0)
	owed1 := cloneBig(pos.TokensOwed1)

	if pos.Liquidity == nil || pos.Liquidity.Sign() == 0 {
		return owed0, owed1, false, 0, nil
	}
	if pos.FeeGrowthInside0LastX128 == nil || pos.FeeGrowthInside1LastX128 == nil {
		return nil, nil, false, 0, fmt.Errorf("position feeGrowthInside last missing")
	}

	global0, global1, staleG, ageG, errG := s.getV4FeeGrowthGlobalsCached(stateView, poolManager, poolID)
	if errG != nil && (global0 == nil || global1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read V4 feeGrowthGlobal failed: %w", errG)
	}
	lower0, lower1, staleL, ageL, errL := s.getV4TickFeeGrowthOutsideCached(stateView, poolManager, poolID, pos.TickLower)
	if errL != nil && (lower0 == nil || lower1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read V4 tickLower feeGrowthOutside failed: %w", errL)
	}
	upper0, upper1, staleU, ageU, errU := s.getV4TickFeeGrowthOutsideCached(stateView, poolManager, poolID, pos.TickUpper)
	if errU != nil && (upper0 == nil || upper1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read V4 tickUpper feeGrowthOutside failed: %w", errU)
	}

	usedStale := staleG || staleL || staleU
	age := time.Duration(0)
	if staleG && ageG > age {
		age = ageG
	}
	if staleL && ageL > age {
		age = ageL
	}
	if staleU && ageU > age {
		age = ageU
	}

	inside0 := feeGrowthInside(currentTick, pos.TickLower, pos.TickUpper, global0, lower0, upper0)
	inside1 := feeGrowthInside(currentTick, pos.TickLower, pos.TickUpper, global1, lower1, upper1)
	if inside0.Cmp(global0) > 0 || inside1.Cmp(global1) > 0 {
		return nil, nil, usedStale, age, fmt.Errorf("invalid feeGrowthInside (pool_id=%s)", poolID)
	}

	last0 := cloneBig(pos.FeeGrowthInside0LastX128)
	last1 := cloneBig(pos.FeeGrowthInside1LastX128)

	delta0 := new(big.Int).Sub(inside0, last0)
	if delta0.Sign() < 0 {
		delta0 = big.NewInt(0)
	}
	delta1 := new(big.Int).Sub(inside1, last1)
	if delta1.Sign() < 0 {
		delta1 = big.NewInt(0)
	}

	extra0 := mulDivFloor(delta0, pos.Liquidity, q128)
	extra1 := mulDivFloor(delta1, pos.Liquidity, q128)
	owed0.Add(owed0, extra0)
	owed1.Add(owed1, extra1)

	var err error
	if usedStale {
		if errG != nil {
			err = errG
		} else if errL != nil {
			err = errL
		} else if errU != nil {
			err = errU
		}
	}
	return owed0, owed1, usedStale, age, err
}

func (s *PnLService) calcV4UnclaimedFeesCachedUnified(stateView, poolManager common.Address, poolID string, currentTick int, pos *blockchain.V4PositionInfo) (*big.Int, *big.Int, bool, time.Duration, error) {
	if pos == nil {
		return nil, nil, false, 0, fmt.Errorf("position info missing")
	}

	owed0 := cloneBig(pos.TokensOwed0)
	owed1 := cloneBig(pos.TokensOwed1)

	if pos.Liquidity == nil || pos.Liquidity.Sign() == 0 {
		return owed0, owed1, false, 0, nil
	}
	if pos.FeeGrowthInside0LastX128 == nil || pos.FeeGrowthInside1LastX128 == nil {
		return nil, nil, false, 0, fmt.Errorf("position feeGrowthInside last missing")
	}

	global0, global1, staleG, ageG, errG := s.getV4FeeGrowthGlobalsCached(stateView, poolManager, poolID)
	if errG != nil && (global0 == nil || global1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read V4 feeGrowthGlobal failed: %w", errG)
	}
	lower0, lower1, staleL, ageL, errL := s.getV4TickFeeGrowthOutsideCached(stateView, poolManager, poolID, pos.TickLower)
	if errL != nil && (lower0 == nil || lower1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read V4 tickLower feeGrowthOutside failed: %w", errL)
	}
	upper0, upper1, staleU, ageU, errU := s.getV4TickFeeGrowthOutsideCached(stateView, poolManager, poolID, pos.TickUpper)
	if errU != nil && (upper0 == nil || upper1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read V4 tickUpper feeGrowthOutside failed: %w", errU)
	}

	usedStale := staleG || staleL || staleU
	age := time.Duration(0)
	if staleG && ageG > age {
		age = ageG
	}
	if staleL && ageL > age {
		age = ageL
	}
	if staleU && ageU > age {
		age = ageU
	}

	fees0, fees1, calcErr := pool.CalcV4UnclaimedFeesFromGrowths(currentTick, pos, global0, global1, lower0, lower1, upper0, upper1)
	if calcErr != nil {
		return nil, nil, usedStale, age, calcErr
	}

	var err error
	if usedStale {
		if errG != nil {
			err = errG
		} else if errL != nil {
			err = errL
		} else if errU != nil {
			err = errU
		}
	}
	return fees0, fees1, usedStale, age, err
}

func mulDivFloor(a, b, denom *big.Int) *big.Int {
	if denom == nil || denom.Sign() == 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Div(new(big.Int).Mul(a, b), denom)
}

func feeGrowthInside(currentTick, tickLower, tickUpper int, global, outsideLower, outsideUpper *big.Int) *big.Int {
	feeGlobal := cloneBig(global)
	lower := cloneBig(outsideLower)
	upper := cloneBig(outsideUpper)

	var below *big.Int
	if currentTick >= tickLower {
		below = lower
	} else {
		below = subMod256(feeGlobal, lower)
	}

	var above *big.Int
	if currentTick < tickUpper {
		above = upper
	} else {
		above = subMod256(feeGlobal, upper)
	}

	sum := addMod256(below, above)
	return subMod256(feeGlobal, sum)
}

func addMod256(a, b *big.Int) *big.Int {
	sum := new(big.Int).Add(cloneBig(a), cloneBig(b))
	return sum.Mod(sum, modUint256)
}

func subMod256(a, b *big.Int) *big.Int {
	diff := new(big.Int).Sub(cloneBig(a), cloneBig(b))
	return diff.Mod(diff, modUint256)
}
