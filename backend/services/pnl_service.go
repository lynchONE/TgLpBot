package services

import (
	"TgLpBot/blockchain"
	"TgLpBot/config"
	"TgLpBot/database"
	"TgLpBot/models"
	"fmt"
	"log"
	"math"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

type PnLService struct{}

func NewPnLService() *PnLService {
	return &PnLService{}
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
		if v, perr := parseBigInt(rec.OpenDust0); perr == nil && v != nil {
			dust0 = v
		}
		if v, perr := parseBigInt(rec.OpenDust1); perr == nil && v != nil {
			dust1 = v
		}
	}

	dustValueUSDT := 0.0
	if sqrtPriceX96 != nil && (dust0.Sign() > 0 || dust1.Sign() > 0) {
		dustValueUSDT, _, _ = s.calculateUSDTValue(
			task,
			dust0, dust1,
			big.NewInt(0), big.NewInt(0),
			big.NewInt(0), big.NewInt(0),
			sqrtPriceX96,
		)
	}

	netInvested := initialCost - dustValueUSDT
	if netInvested < 0 {
		netInvested = 0
	}

	return &PnLInfo{
		InitialCostUSDT:   initialCost,
		NetInvestedUSDT:   netInvested,
		CurrentValueUSDT:  currentValue,
		AbsolutePnLUSDT:   currentValue - netInvested,
		UnclaimedFeesUSDT: unclaimedFees,
		HoldingsUSDT:      holdingsValue,
		DustToken0:        weiToFloat(dust0, 18),
		DustToken1:        weiToFloat(dust1, 18),
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

	amountWei, err := parseBigInt(rec.OpenUSDTSpent)
	if err != nil {
		return 0, nil, err
	}
	return weiToFloat(amountWei, 18), &rec, nil
}

func (s *PnLService) getV3CurrentValue(task *models.StrategyTask) (totalVal, feesVal, holdingsVal float64, sqrtPriceX96 *big.Int, err error) {
	// 1. Get Position Info (Liquidity + Fees)
	if task.V3TokenID == "" || task.V3TokenID == "0" {
		return 0, 0, 0, nil, fmt.Errorf("no V3 token ID")
	}
	tokenId, _ := new(big.Int).SetString(task.V3TokenID, 10)

	pmAddress := common.HexToAddress(task.V3PositionManagerAddress)
	pm, err := blockchain.NewV3PositionManager(pmAddress, blockchain.Client)
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("init V3 PM failed: %w", err)
	}

	pos, err := pm.Positions(nil, tokenId)
	if err != nil {
		// Check ownerOf to see if it exists/burned
		return 0, 0, 0, nil, fmt.Errorf("read positions failed: %w", err)
	}

	// 2. Get Current Price (Slot0)
	currentTick := 0
	poolAddr := common.HexToAddress(task.PoolId)
	sqrtPriceX96, currentTick, err = blockchain.GetV3PoolSlot0(poolAddr)
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("get slot0 failed: %w", err)
	}

	// 3. Calculate Amounts from Liquidity
	sqrtPriceA, _ := SqrtRatioAtTick(int32(pos.TickLower))
	sqrtPriceB, _ := SqrtRatioAtTick(int32(pos.TickUpper))

	amount0, amount1 := AmountsForLiquidity(sqrtPriceX96, sqrtPriceA, sqrtPriceB, pos.Liquidity)

	fees0 := cloneBig(pos.TokensOwed0)
	fees1 := cloneBig(pos.TokensOwed1)
	if fee0, fee1, feeErr := calcV3UnclaimedFees(poolAddr, currentTick, pos); feeErr == nil {
		fees0 = fee0
		fees1 = fee1
	} else {
		log.Printf("[PnL] V3 手续费计算失败: tokenId=%s err=%v", task.V3TokenID, feeErr)
	}

	// 4. Total Amounts = Position + Unclaimed Fees
	total0 := new(big.Int).Add(amount0, fees0)
	total1 := new(big.Int).Add(amount1, fees1)

	log.Printf("[PnL] V3 手续费: tokenId=%s owed0=%s owed1=%s amount0=%s amount1=%s",
		task.V3TokenID, fees0.String(), fees1.String(), amount0.String(), amount1.String())

	// 5. Convert to USDT
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

	if common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		tokenId, parseErr := parseBigInt(task.V4TokenID)
		if parseErr == nil && tokenId.Sign() > 0 {
			v4pmAddr := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
			v4pm, pmErr := blockchain.NewV4PositionManager(v4pmAddr, blockchain.Client)
			if pmErr == nil {
				pos, posErr := v4pm.Positions(nil, tokenId)
				if posErr == nil && pos != nil {
					v4pos = pos
					if pos.TokensOwed0 != nil {
						fees0 = pos.TokensOwed0
					}
					if pos.TokensOwed1 != nil {
						fees1 = pos.TokensOwed1
					}
					if liquidity.Sign() == 0 && pos.Liquidity != nil && pos.Liquidity.Sign() > 0 {
						liquidity = pos.Liquidity
					}
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

	sqrtPriceX96, currentTick, err := blockchain.GetUniswapV4PoolSlot0ViaStateView(stateView, poolManager, task.PoolId)
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("get V4 slot0 failed: %w", err)
	}

	// 尝试计算实时手续费（如果有仓位信息）
	if v4pos != nil && v4pos.Liquidity != nil && v4pos.Liquidity.Sign() > 0 {
		if realFees0, realFees1, feeErr := calcV4UnclaimedFees(task.PoolId, currentTick, v4pos); feeErr == nil {
			fees0 = realFees0
			fees1 = realFees1
			log.Printf("[PnL] V4 实时手续费: tokenId=%s fees0=%s fees1=%s", task.V4TokenID, fees0.String(), fees1.String())
		} else {
			log.Printf("[PnL] V4 手续费计算失败: %v，使用 TokensOwed", feeErr)
		}
	}

	// 2. Calculate Amounts
	tickLower := int32(task.TickLower)
	tickUpper := int32(task.TickUpper)
	sqrtPriceA, _ := SqrtRatioAtTick(tickLower)
	sqrtPriceB, _ := SqrtRatioAtTick(tickUpper)

	amount0, amount1 := AmountsForLiquidity(sqrtPriceX96, sqrtPriceA, sqrtPriceB, liquidity)

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

	// Determine price relation
	// sqrtPriceX96 = sqrt(token1/token0) * 2^96
	// price1to0 = (sqrtPriceX96 / 2^96)^2  (1 token0 = X token1) -> WRONG.
	// Uniswap: price = token1/token0. So 1 token0 = price token1.

	p := new(big.Float).SetInt(sqrtPriceX96)
	q := new(big.Float).SetInt(q96)
	p.Quo(p, q)
	p.Mul(p, p) // price = (sqrtX96/Q96)^2. Represents amount of Token1 per 1 Token0.

	priceToken1PerToken0, _ := p.Float64()

	// Convert wei to float
	t0 := weiToFloat(total0, 18) // Assume 18 decimals for now (should fetch from token metadata ideally)
	t1 := weiToFloat(total1, 18)
	f0 := weiToFloat(fees0, 18)
	f1 := weiToFloat(fees1, 18)
	h0 := weiToFloat(hold0, 18)
	h1 := weiToFloat(hold1, 18)

	if token0Symbol == "USDT" {
		// Token0 is USDT. Value = T0 + T1 * (Price of T1 in T0)
		// priceToken1PerToken0 means 1 T0 = P T1. So Price of T1 in T0 = 1/P.
		priceT1InUSDT := 0.0
		if priceToken1PerToken0 > 0 {
			priceT1InUSDT = 1.0 / priceToken1PerToken0
		}

		totalUSDT = t0 + t1*priceT1InUSDT
		feesUSDT = f0 + f1*priceT1InUSDT
		holdUSDT = h0 + h1*priceT1InUSDT
	} else if token1Symbol == "USDT" {
		// Token1 is USDT. Value = T1 + T0 * (Price of T0 in T1)
		// priceToken1PerToken0 means 1 T0 = P T1. Since T1 is USDT, P is Price of T0 in USDT.
		priceT0InUSDT := priceToken1PerToken0

		totalUSDT = t1 + t0*priceT0InUSDT
		feesUSDT = f1 + f0*priceT0InUSDT
		holdUSDT = h1 + h0*priceT0InUSDT
	} else {
		// Neither is USDT (e.g. ETH/BTC). Fallback or use DB price?
		// For simplicity, return 0 or rely on task.AmountUSDT estimate if logic fails.
		// User specifically mentioned USDT pairs.
		log.Printf("Warning: Neither token is USDT for PnL calc. Task #%d", task.ID)
		return 0, 0, 0
	}

	return totalUSDT, feesUSDT, holdUSDT
}

func weiToFloat(wei *big.Int, decimals int) float64 {
	f := new(big.Float).SetInt(wei)
	div := new(big.Float).SetFloat64(math.Pow(10, float64(decimals)))
	f.Quo(f, div)
	val, _ := f.Float64()
	return val
}
