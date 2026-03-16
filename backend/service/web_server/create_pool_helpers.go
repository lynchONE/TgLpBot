package web_server

import (
	"fmt"
	"math/big"
	"strings"

	poolSvc "TgLpBot/service/pool"
)

var (
	createPoolUniV3FeeTiers = map[uint64]struct{}{
		100: {}, 500: {}, 3000: {}, 10000: {},
	}
	createPoolPcsV3FeeTiers = map[uint64]struct{}{
		100: {}, 500: {}, 2500: {}, 10000: {},
	}
)

func createPoolSupportsFeeTier(protocol string, feeTier uint64) bool {
	switch protocol {
	case createPoolProtocolUniV3:
		_, ok := createPoolUniV3FeeTiers[feeTier]
		return ok
	case createPoolProtocolPcsV3:
		_, ok := createPoolPcsV3FeeTiers[feeTier]
		return ok
	case createPoolProtocolUniV4:
		return feeTier > 0
	default:
		return false
	}
}

func createPoolHumanAmount(amount *big.Int, decimals int) string {
	if amount == nil || amount.Sign() <= 0 {
		return ""
	}
	return balanceToDecimalString(amount, decimals)
}

func createPoolCanonicalPriceFromAB(raw string, tokenAIsToken0 bool) (*big.Float, string, error) {
	value, err := poolSvc.ParseDecimalToFloat(raw)
	if err != nil {
		return nil, "", err
	}
	canonical, err := poolSvc.HumanPriceFromBaseQuote(value, tokenAIsToken0)
	if err != nil {
		return nil, "", err
	}
	return canonical, formatCreatePoolDecimal(canonical, 12), nil
}

func createPoolPriceABFromCanonical(raw *big.Float, tokenAIsToken0 bool) (string, error) {
	if raw == nil || raw.Sign() <= 0 {
		return "", fmt.Errorf("price must be positive")
	}
	value := new(big.Float).SetPrec(256).Set(raw)
	if !tokenAIsToken0 {
		var err error
		value, err = poolSvc.HumanPriceFromBaseQuote(raw, false)
		if err != nil {
			return "", err
		}
	}
	return formatCreatePoolDecimal(value, 12), nil
}

func createPoolAlignRangeTicks(minTick, maxTick, tickSpacing int) (int, int, error) {
	tickLower, err := poolSvc.AlignTickDown(minTick, tickSpacing)
	if err != nil {
		return 0, 0, err
	}
	tickUpper, err := poolSvc.AlignTickUp(maxTick, tickSpacing)
	if err != nil {
		return 0, 0, err
	}
	if tickLower >= tickUpper {
		return 0, 0, fmt.Errorf("custom range collapsed after tick alignment")
	}
	return tickLower, tickUpper, nil
}

func createPoolQuoteSingleSided(plan *createPoolPlan) ([]string, error) {
	if plan == nil || plan.sqrtPriceX96 == nil || plan.tickLower >= plan.tickUpper {
		return nil, nil
	}
	if (plan.amountAUnits == nil || plan.amountAUnits.Sign() <= 0) && (plan.amountBUnits == nil || plan.amountBUnits.Sign() <= 0) {
		return nil, nil
	}
	if plan.amountAUnits != nil && plan.amountAUnits.Sign() > 0 && plan.amountBUnits != nil && plan.amountBUnits.Sign() > 0 {
		return nil, nil
	}

	sqrtLower, err := poolSvc.SqrtRatioAtTick(int32(plan.tickLower))
	if err != nil {
		return nil, err
	}
	sqrtUpper, err := poolSvc.SqrtRatioAtTick(int32(plan.tickUpper))
	if err != nil {
		return nil, err
	}

	var warnings []string
	switch {
	case plan.amountAUnits != nil && plan.amountAUnits.Sign() > 0:
		plan.singleSidedInput = "token_a"
		plan.singleInputAmount = new(big.Int).Set(plan.amountAUnits)
		plan.singleInputAmountText = createPoolHumanAmount(plan.amountAUnits, plan.tokenA.Decimals)
		if plan.tokenAIsToken0 {
			plan.singleInputToken = plan.token0.Address
			if plan.sqrtPriceX96.Cmp(sqrtUpper) >= 0 {
				warnings = append(warnings, fmt.Sprintf("当前价格已高于区间上沿，单独输入 %s 时会优先换成 %s", plan.tokenA.Symbol, plan.tokenB.Symbol))
				plan.mirrorAmountBUnits = big.NewInt(0)
				plan.mirrorAmountB = "0"
			} else {
				liq := poolSvc.LiquidityForAmount0(maxBig(plan.sqrtPriceX96, sqrtLower), sqrtUpper, plan.amountAUnits)
				_, amount1 := poolSvc.AmountsForLiquidity(plan.sqrtPriceX96, sqrtLower, sqrtUpper, liq)
				plan.mirrorAmountBUnits = cloneBigInt(amount1)
				plan.mirrorAmountB = createPoolHumanAmount(amount1, plan.tokenB.Decimals)
			}
			plan.mirrorAmountAUnits = cloneBigInt(plan.amountAUnits)
			plan.mirrorAmountA = createPoolHumanAmount(plan.amountAUnits, plan.tokenA.Decimals)
		} else {
			plan.singleInputToken = plan.token1.Address
			if plan.sqrtPriceX96.Cmp(sqrtLower) <= 0 {
				warnings = append(warnings, fmt.Sprintf("当前价格已低于区间下沿，单独输入 %s 时会优先换成 %s", plan.tokenA.Symbol, plan.tokenB.Symbol))
				plan.mirrorAmountBUnits = big.NewInt(0)
				plan.mirrorAmountB = "0"
			} else {
				liq := poolSvc.LiquidityForAmount1(sqrtLower, minBig(plan.sqrtPriceX96, sqrtUpper), plan.amountAUnits)
				amount0, _ := poolSvc.AmountsForLiquidity(plan.sqrtPriceX96, sqrtLower, sqrtUpper, liq)
				plan.mirrorAmountBUnits = cloneBigInt(amount0)
				plan.mirrorAmountB = createPoolHumanAmount(amount0, plan.tokenB.Decimals)
			}
			plan.mirrorAmountAUnits = cloneBigInt(plan.amountAUnits)
			plan.mirrorAmountA = createPoolHumanAmount(plan.amountAUnits, plan.tokenA.Decimals)
		}
	case plan.amountBUnits != nil && plan.amountBUnits.Sign() > 0:
		plan.singleSidedInput = "token_b"
		plan.singleInputAmount = new(big.Int).Set(plan.amountBUnits)
		plan.singleInputAmountText = createPoolHumanAmount(plan.amountBUnits, plan.tokenB.Decimals)
		if plan.tokenAIsToken0 {
			plan.singleInputToken = plan.token1.Address
			if plan.sqrtPriceX96.Cmp(sqrtLower) <= 0 {
				warnings = append(warnings, fmt.Sprintf("当前价格已低于区间下沿，单独输入 %s 时会优先换成 %s", plan.tokenB.Symbol, plan.tokenA.Symbol))
				plan.mirrorAmountAUnits = big.NewInt(0)
				plan.mirrorAmountA = "0"
			} else {
				liq := poolSvc.LiquidityForAmount1(sqrtLower, minBig(plan.sqrtPriceX96, sqrtUpper), plan.amountBUnits)
				amount0, _ := poolSvc.AmountsForLiquidity(plan.sqrtPriceX96, sqrtLower, sqrtUpper, liq)
				plan.mirrorAmountAUnits = cloneBigInt(amount0)
				plan.mirrorAmountA = createPoolHumanAmount(amount0, plan.tokenA.Decimals)
			}
			plan.mirrorAmountBUnits = cloneBigInt(plan.amountBUnits)
			plan.mirrorAmountB = createPoolHumanAmount(plan.amountBUnits, plan.tokenB.Decimals)
		} else {
			plan.singleInputToken = plan.token0.Address
			if plan.sqrtPriceX96.Cmp(sqrtUpper) >= 0 {
				warnings = append(warnings, fmt.Sprintf("当前价格已高于区间上沿，单独输入 %s 时会优先换成 %s", plan.tokenB.Symbol, plan.tokenA.Symbol))
				plan.mirrorAmountAUnits = big.NewInt(0)
				plan.mirrorAmountA = "0"
			} else {
				liq := poolSvc.LiquidityForAmount0(maxBig(plan.sqrtPriceX96, sqrtLower), sqrtUpper, plan.amountBUnits)
				_, amount1 := poolSvc.AmountsForLiquidity(plan.sqrtPriceX96, sqrtLower, sqrtUpper, liq)
				plan.mirrorAmountAUnits = cloneBigInt(amount1)
				plan.mirrorAmountA = createPoolHumanAmount(amount1, plan.tokenA.Decimals)
			}
			plan.mirrorAmountBUnits = cloneBigInt(plan.amountBUnits)
			plan.mirrorAmountB = createPoolHumanAmount(plan.amountBUnits, plan.tokenB.Decimals)
		}
	}

	if plan.singleSidedInput != "" {
		plan.mirrorAmountSource = "range_ratio"
	}
	return warnings, nil
}

func minBig(a, b *big.Int) *big.Int {
	if a == nil {
		return cloneBigInt(b)
	}
	if b == nil {
		return cloneBigInt(a)
	}
	if a.Cmp(b) <= 0 {
		return cloneBigInt(a)
	}
	return cloneBigInt(b)
}

func maxBig(a, b *big.Int) *big.Int {
	if a == nil {
		return cloneBigInt(b)
	}
	if b == nil {
		return cloneBigInt(a)
	}
	if a.Cmp(b) >= 0 {
		return cloneBigInt(a)
	}
	return cloneBigInt(b)
}

func cloneBigInt(v *big.Int) *big.Int {
	if v == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(v)
}

func createPoolQuoteDirection(plan *createPoolPlan, zeroForOne bool) (string, string, string) {
	if plan == nil {
		return "", "", ""
	}
	switch {
	case zeroForOne && plan.tokenAIsToken0:
		return "a_to_b", plan.tokenA.Symbol, plan.tokenB.Symbol
	case zeroForOne && !plan.tokenAIsToken0:
		return "b_to_a", plan.tokenB.Symbol, plan.tokenA.Symbol
	case !zeroForOne && plan.tokenAIsToken0:
		return "b_to_a", plan.tokenB.Symbol, plan.tokenA.Symbol
	default:
		return "a_to_b", plan.tokenA.Symbol, plan.tokenB.Symbol
	}
}

func createPoolValidateSingleInput(plan *createPoolPlan) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}
	hasA := plan.amountAUnits != nil && plan.amountAUnits.Sign() > 0
	hasB := plan.amountBUnits != nil && plan.amountBUnits.Sign() > 0
	switch plan.amountMode {
	case createPoolAmountModeDual:
		if !hasA || !hasB {
			return fmt.Errorf("dual_exact requires both amount_a and amount_b")
		}
	case createPoolAmountModeSingle:
		if hasA == hasB {
			return fmt.Errorf("single_auto_swap requires exactly one of amount_a or amount_b")
		}
	default:
		return fmt.Errorf("unsupported amount mode")
	}
	return nil
}

func createPoolRangeSummary(raw string) string {
	return strings.TrimSpace(raw)
}
