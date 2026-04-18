package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/pricing"
	userSvc "TgLpBot/service/user"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

const (
	openPositionSizingShareHardCap = 0.80
)

type OpenPositionSizingBuildOptions struct {
	CurrentTick int
}

type OpenPositionSizingInputs struct {
	Price                 float64 `json:"price,omitempty"`
	PriceBaseSymbol       string  `json:"price_base_symbol,omitempty"`
	PriceQuoteSymbol      string  `json:"price_quote_symbol,omitempty"`
	CurrentTick           int     `json:"current_tick"`
	TickLower             int     `json:"tick_lower"`
	TickUpper             int     `json:"tick_upper"`
	ActiveLiquidityUSD    float64 `json:"active_liquidity_usd,omitempty"`
	ActiveLiquiditySource string  `json:"active_liquidity_source,omitempty"`
	CapitalTotal          float64 `json:"capital_total,omitempty"`
	CapitalSource         string  `json:"capital_source,omitempty"`
	TargetShareMin        float64 `json:"target_share_min"`
	TargetShareMax        float64 `json:"target_share_max"`
	RiskCapUSD            float64 `json:"risk_cap_usd,omitempty"`
	RiskCapRatio          float64 `json:"risk_cap_ratio,omitempty"`
	EffectiveRiskCapUSD   float64 `json:"effective_risk_cap_usd,omitempty"`
}

type OpenPositionSizingAdvice struct {
	RecommendedPositions []OpenPositionSizingPosition `json:"recommended_positions"`
	Warnings             []string                     `json:"warnings,omitempty"`
	Inputs               *OpenPositionSizingInputs    `json:"inputs,omitempty"`
}

type OpenPositionSizingPosition struct {
	Mode           string                         `json:"mode"`
	LiquidityToAdd float64                        `json:"liquidity_to_add"`
	ExpectedShare  float64                        `json:"expected_share"`
	RiskExposure   float64                        `json:"risk_exposure"`
	Efficiency     string                         `json:"efficiency"`
	Calculation    *OpenPositionSizingCalculation `json:"calculation,omitempty"`
}

type OpenPositionSizingCalculation struct {
	TargetShareRequested      float64  `json:"target_share_requested"`
	TargetShareApplied        float64  `json:"target_share_applied"`
	TheoreticalLiquidityToAdd float64  `json:"theoretical_liquidity_to_add"`
	AppliedConstraints        []string `json:"applied_constraints,omitempty"`
	RiskExposureApproximation string   `json:"risk_exposure_approximation,omitempty"`
}

type openPositionSizingTarget struct {
	Mode   string
	Target float64
}

type openPositionSizingConstraint struct {
	Key   string
	Limit float64
}

func BuildOpenPositionSizingAdvice(
	task *models.StrategyTask,
	wallet *models.Wallet,
	opts OpenPositionSizingBuildOptions,
) (*OpenPositionSizingAdvice, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	if wallet == nil {
		return nil, fmt.Errorf("wallet is nil")
	}

	cfg, err := userSvc.NewGlobalConfigService().ResolveOpenPositionSizingConfig(task.UserID)
	if err != nil {
		return nil, err
	}

	inputs := OpenPositionSizingInputs{
		CurrentTick:    opts.CurrentTick,
		TickLower:      task.TickLower,
		TickUpper:      task.TickUpper,
		TargetShareMin: cfg.TargetShareMin,
		TargetShareMax: cfg.TargetShareMax,
		RiskCapUSD:     cfg.RiskCapUSD,
		RiskCapRatio:   cfg.RiskCapRatio,
	}

	var warnings []string
	if price, baseSymbol, quoteSymbol, ok := pricing.BuildPriceDisplay(task, opts.CurrentTick); ok && price > 0 {
		inputs.Price = roundValue(price, 8)
		inputs.PriceBaseSymbol = strings.TrimSpace(baseSymbol)
		inputs.PriceQuoteSymbol = strings.TrimSpace(quoteSymbol)
	}

	liquidityUSD, liquiditySource, err := resolveSizingLiquidityUSD(task.Chain, task.PoolId)
	if err != nil {
		log.Printf("[Liquidity] sizing: resolve liquidity failed: chain=%s pool=%s err=%v", task.Chain, task.PoolId, err)
		warnings = append(warnings, "当前区间活跃流动性暂时无法确认，已跳过加仓建议。")
	}
	if liquidityUSD > 0 {
		inputs.ActiveLiquidityUSD = roundMoney(liquidityUSD)
		inputs.ActiveLiquiditySource = liquiditySource
		if liquiditySource != "" && liquiditySource != "pool_snapshot.active_liquidity_usd" {
			warnings = append(warnings, fmt.Sprintf("活跃流动性已回退到 %s 估值来源。", liquiditySource))
		}
	}

	capitalTotal, capitalSource, err := resolveSizingCapitalTotal(task.Chain, wallet)
	if err != nil {
		log.Printf("[Liquidity] sizing: resolve stable capital failed: chain=%s wallet=%s err=%v", task.Chain, wallet.Address, err)
		warnings = append(warnings, "当前钱包 stable 余额暂时无法读取，已跳过加仓建议。")
	}
	if capitalTotal > 0 {
		inputs.CapitalTotal = roundMoney(capitalTotal)
		inputs.CapitalSource = capitalSource
	}

	advice := CalculateOpenPositionSizingAdvice(inputs)
	advice.Warnings = append(advice.Warnings, warnings...)
	advice.Warnings = dedupeSizingWarnings(advice.Warnings)
	return advice, nil
}

func CalculateOpenPositionSizingAdvice(inputs OpenPositionSizingInputs) *OpenPositionSizingAdvice {
	normalized, warnings := normalizeOpenPositionSizingInputs(inputs)
	advice := &OpenPositionSizingAdvice{
		RecommendedPositions: []OpenPositionSizingPosition{},
		Warnings:             warnings,
		Inputs:               &normalized,
	}

	if normalized.ActiveLiquidityUSD <= 0 {
		advice.Warnings = append(advice.Warnings, "当前区间活跃流动性缺失，无法生成建议金额。")
		advice.Warnings = dedupeSizingWarnings(advice.Warnings)
		return advice
	}
	if normalized.CapitalTotal <= 0 {
		advice.Warnings = append(advice.Warnings, "当前钱包可用 stable 资金缺失，无法生成建议金额。")
		advice.Warnings = dedupeSizingWarnings(advice.Warnings)
		return advice
	}

	constraints, effectiveRiskCap := buildSizingConstraints(normalized)
	normalized.EffectiveRiskCapUSD = roundMoney(effectiveRiskCap)
	advice.Inputs = &normalized

	targets := []openPositionSizingTarget{
		{Mode: "conservative", Target: 0.20},
		{Mode: "neutral", Target: 0.40},
		{Mode: "aggressive", Target: 0.65},
	}

	appliedTargets := make(map[string]float64, len(targets))
	for _, target := range targets {
		requestedTarget := clampShare(target.Target, normalized.TargetShareMin, normalized.TargetShareMax)
		appliedTarget := requestedTarget
		efficiency := sizingEfficiency(appliedTarget, requestedTarget)
		if requestedTarget > openPositionSizingShareHardCap {
			appliedTarget = openPositionSizingShareHardCap
			efficiency = "low"
			advice.Warnings = append(advice.Warnings, fmt.Sprintf("%s 档目标占比超过 80%%，已截断到 80%%，属于低效率区间。", target.Mode))
		}

		theoreticalLiquidity := reverseLiquidityForShare(normalized.ActiveLiquidityUSD, appliedTarget)
		finalLiquidity := theoreticalLiquidity
		appliedConstraints := make([]string, 0, len(constraints))
		if effectiveRiskCap > 0 && finalLiquidity > effectiveRiskCap {
			finalLiquidity = effectiveRiskCap
			for _, constraint := range constraints {
				if constraint.Limit > 0 && almostEqual(constraint.Limit, effectiveRiskCap) {
					appliedConstraints = append(appliedConstraints, constraint.Key)
				}
			}
		}

		expectedShare := shareFromLiquidity(normalized.ActiveLiquidityUSD, finalLiquidity)
		advice.RecommendedPositions = append(advice.RecommendedPositions, OpenPositionSizingPosition{
			Mode:           target.Mode,
			LiquidityToAdd: roundMoney(finalLiquidity),
			ExpectedShare:  roundValue(expectedShare, 6),
			RiskExposure:   roundMoney(finalLiquidity),
			Efficiency:     efficiency,
			Calculation: &OpenPositionSizingCalculation{
				TargetShareRequested:      roundValue(requestedTarget, 6),
				TargetShareApplied:        roundValue(appliedTarget, 6),
				TheoreticalLiquidityToAdd: roundMoney(theoreticalLiquidity),
				AppliedConstraints:        appliedConstraints,
				RiskExposureApproximation: "conservative_single_side_amount",
			},
		})
		appliedTargets[target.Mode] = roundValue(appliedTarget, 6)
	}

	if len(uniqueSizingTargets(appliedTargets)) < len(targets) {
		advice.Warnings = append(advice.Warnings, "目标占比范围较窄，部分档位建议已经收敛为同一目标。")
	}

	advice.Warnings = dedupeSizingWarnings(advice.Warnings)
	return advice
}

func normalizeOpenPositionSizingInputs(inputs OpenPositionSizingInputs) (OpenPositionSizingInputs, []string) {
	out := inputs
	var warnings []string

	if out.TargetShareMin <= 0 {
		out.TargetShareMin = 0.20
		warnings = append(warnings, "目标占比下限缺失，已回退到 20%。")
	}
	if out.TargetShareMax <= 0 {
		out.TargetShareMax = 0.65
		warnings = append(warnings, "目标占比上限缺失，已回退到 65%。")
	}
	if out.TargetShareMin > out.TargetShareMax {
		out.TargetShareMin, out.TargetShareMax = out.TargetShareMax, out.TargetShareMin
		warnings = append(warnings, "目标占比配置顺序无效，已自动交换上下限。")
	}
	if out.RiskCapUSD <= 0 {
		out.RiskCapUSD = 500
		warnings = append(warnings, "固定单仓风险上限缺失，已回退到 500U。")
	}
	if out.RiskCapRatio <= 0 {
		out.RiskCapRatio = 0.20
		warnings = append(warnings, "单仓风险比例上限缺失，已回退到 20%。")
	}

	out.ActiveLiquidityUSD = roundMoney(out.ActiveLiquidityUSD)
	out.CapitalTotal = roundMoney(out.CapitalTotal)
	out.TargetShareMin = roundValue(out.TargetShareMin, 6)
	out.TargetShareMax = roundValue(out.TargetShareMax, 6)
	out.RiskCapUSD = roundMoney(out.RiskCapUSD)
	out.RiskCapRatio = roundValue(out.RiskCapRatio, 6)

	return out, warnings
}

func resolveSizingLiquidityUSD(chain string, poolID string) (float64, string, error) {
	liquidityUSD, source, err := ResolvePoolLiquidityUSDWithSource(chain, poolID)
	if err != nil {
		return 0, source, err
	}
	if liquidityUSD <= 0 {
		return 0, source, fmt.Errorf("liquidity usd is unavailable")
	}
	return liquidityUSD, source, nil
}

func resolveSizingCapitalTotal(chain string, wallet *models.Wallet) (float64, string, error) {
	if wallet == nil {
		return 0, "", fmt.Errorf("wallet is nil")
	}
	normalizedChain := config.NormalizeChain(chain)
	if config.AppConfig == nil {
		return 0, "", fmt.Errorf("config not loaded")
	}
	cc, ok := config.AppConfig.GetChainConfig(normalizedChain)
	if !ok {
		return 0, "", fmt.Errorf("chain config not found")
	}
	if !common.IsHexAddress(cc.StableAddress) {
		return 0, "", fmt.Errorf("stable token not configured")
	}
	if !common.IsHexAddress(wallet.Address) {
		return 0, "", fmt.Errorf("wallet address is invalid")
	}

	client, _, err := blockchain.GetEVMClient(normalizedChain)
	if err != nil {
		return 0, "", err
	}
	balance, err := blockchain.GetTokenBalanceWithClient(
		client,
		common.HexToAddress(cc.StableAddress),
		common.HexToAddress(wallet.Address),
	)
	if err != nil {
		return 0, "wallet_balance.stable_token", err
	}
	stableDecimals := cc.StableDecimals
	if stableDecimals <= 0 {
		stableDecimals = 18
	}
	return amountToFloat(balance, stableDecimals), "wallet_balance.stable_token", nil
}

func buildSizingConstraints(inputs OpenPositionSizingInputs) ([]openPositionSizingConstraint, float64) {
	constraints := make([]openPositionSizingConstraint, 0, 3)
	if inputs.RiskCapUSD > 0 {
		constraints = append(constraints, openPositionSizingConstraint{
			Key:   "risk_cap_usd",
			Limit: inputs.RiskCapUSD,
		})
	}
	if inputs.CapitalTotal > 0 && inputs.RiskCapRatio > 0 {
		constraints = append(constraints, openPositionSizingConstraint{
			Key:   "risk_cap_ratio",
			Limit: inputs.CapitalTotal * inputs.RiskCapRatio,
		})
	}
	if inputs.CapitalTotal > 0 {
		constraints = append(constraints, openPositionSizingConstraint{
			Key:   "capital_total",
			Limit: inputs.CapitalTotal,
		})
	}

	effective := 0.0
	for _, constraint := range constraints {
		if constraint.Limit <= 0 {
			continue
		}
		if effective <= 0 || constraint.Limit < effective {
			effective = constraint.Limit
		}
	}
	return constraints, effective
}

func clampShare(value float64, minShare float64, maxShare float64) float64 {
	if value < minShare {
		return minShare
	}
	if value > maxShare {
		return maxShare
	}
	return value
}

func reverseLiquidityForShare(activeLiquidity float64, targetShare float64) float64 {
	if activeLiquidity <= 0 || targetShare <= 0 || targetShare >= 1 {
		return 0
	}
	return activeLiquidity * (targetShare / (1 - targetShare))
}

func shareFromLiquidity(activeLiquidity float64, userLiquidity float64) float64 {
	if activeLiquidity <= 0 || userLiquidity <= 0 {
		return 0
	}
	return userLiquidity / (activeLiquidity + userLiquidity)
}

func sizingEfficiency(appliedTarget float64, requestedTarget float64) string {
	if requestedTarget > openPositionSizingShareHardCap {
		return "low"
	}
	if appliedTarget <= 0.5 {
		return "high"
	}
	return "medium"
}

func uniqueSizingTargets(values map[string]float64) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		key := fmt.Sprintf("%.6f", value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func dedupeSizingWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(warnings))
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		text := strings.TrimSpace(warning)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		out = append(out, text)
	}
	return out
}

func roundMoney(value float64) float64 {
	return roundValue(value, 2)
}

func roundValue(value float64, precision int) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if precision < 0 {
		precision = 0
	}
	scale := math.Pow10(precision)
	return math.Round(value*scale) / scale
}

func almostEqual(left float64, right float64) bool {
	return math.Abs(left-right) <= 1e-9
}
