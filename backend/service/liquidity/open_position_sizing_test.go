package liquidity

import (
	"strings"
	"testing"
)

func TestCalculateOpenPositionSizingAdviceCapsHighTargetShare(t *testing.T) {
	advice := CalculateOpenPositionSizingAdvice(OpenPositionSizingInputs{
		ActiveLiquidityUSD: 1000,
		TargetShareMin:     0.90,
		TargetShareMax:     0.95,
	})

	if len(advice.RecommendedPositions) != 3 {
		t.Fatalf("expected 3 positions, got %d", len(advice.RecommendedPositions))
	}

	aggressive := advice.RecommendedPositions[2]
	if aggressive.Efficiency != "low" {
		t.Fatalf("expected aggressive efficiency to be low, got %s", aggressive.Efficiency)
	}
	if aggressive.Calculation == nil {
		t.Fatalf("expected calculation details")
	}
	if aggressive.Calculation.TargetShareApplied != 0.8 {
		t.Fatalf("expected aggressive target share applied to be 0.8, got %f", aggressive.Calculation.TargetShareApplied)
	}
	if aggressive.LiquidityToAdd != 4000 {
		t.Fatalf("expected aggressive liquidity to add 4000, got %f", aggressive.LiquidityToAdd)
	}

	foundWarning := false
	for _, warning := range advice.Warnings {
		if strings.Contains(warning, "80%") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatalf("expected 80%% efficiency warning, got %#v", advice.Warnings)
	}
}

func TestCalculateOpenPositionSizingAdviceUsesPoolLiquidityOnly(t *testing.T) {
	advice := CalculateOpenPositionSizingAdvice(OpenPositionSizingInputs{
		ActiveLiquidityUSD: 1000,
		CapitalTotal:       2000,
		TargetShareMin:     0.20,
		TargetShareMax:     0.65,
		RiskCapUSD:         500,
		RiskCapRatio:       0.20,
	})

	if got := advice.Inputs.EffectiveRiskCapUSD; got != 0 {
		t.Fatalf("expected effective risk cap to stay unused, got %f", got)
	}

	conservative := advice.RecommendedPositions[0]
	if conservative.LiquidityToAdd != 250 {
		t.Fatalf("expected conservative liquidity to add 250, got %f", conservative.LiquidityToAdd)
	}
	if conservative.ExpectedShare != 0.2 {
		t.Fatalf("expected conservative share 0.2, got %f", conservative.ExpectedShare)
	}

	neutral := advice.RecommendedPositions[1]
	if neutral.LiquidityToAdd != 666.67 {
		t.Fatalf("expected neutral liquidity to add 666.67, got %f", neutral.LiquidityToAdd)
	}
	if neutral.ExpectedShare != 0.4 {
		t.Fatalf("expected neutral share 0.4, got %f", neutral.ExpectedShare)
	}
	if neutral.Calculation == nil {
		t.Fatalf("expected calculation details")
	}
	if neutral.Calculation.TheoreticalLiquidityToAdd != 666.67 {
		t.Fatalf("expected theoretical liquidity 666.67, got %f", neutral.Calculation.TheoreticalLiquidityToAdd)
	}
	if len(neutral.Calculation.AppliedConstraints) != 0 {
		t.Fatalf("expected no applied constraints, got %#v", neutral.Calculation.AppliedConstraints)
	}

	aggressive := advice.RecommendedPositions[2]
	if aggressive.LiquidityToAdd != 1857.14 {
		t.Fatalf("expected aggressive liquidity to add 1857.14, got %f", aggressive.LiquidityToAdd)
	}
	if aggressive.ExpectedShare != 0.65 {
		t.Fatalf("expected aggressive share 0.65, got %f", aggressive.ExpectedShare)
	}
}

func TestCalculateOpenPositionSizingAdviceDoesNotRequireCapitalTotal(t *testing.T) {
	advice := CalculateOpenPositionSizingAdvice(OpenPositionSizingInputs{
		ActiveLiquidityUSD: 500,
		TargetShareMin:     0.20,
		TargetShareMax:     0.65,
	})

	if len(advice.RecommendedPositions) != 3 {
		t.Fatalf("expected 3 positions, got %d", len(advice.RecommendedPositions))
	}
	if advice.RecommendedPositions[1].LiquidityToAdd != 333.33 {
		t.Fatalf("expected neutral liquidity to add 333.33, got %f", advice.RecommendedPositions[1].LiquidityToAdd)
	}
}

func TestCalculateOpenPositionSizingAdviceWarnsWhenTargetsCollapse(t *testing.T) {
	advice := CalculateOpenPositionSizingAdvice(OpenPositionSizingInputs{
		ActiveLiquidityUSD: 1000,
		TargetShareMin:     0.55,
		TargetShareMax:     0.60,
	})

	if len(advice.RecommendedPositions) != 3 {
		t.Fatalf("expected 3 positions, got %d", len(advice.RecommendedPositions))
	}
	if advice.RecommendedPositions[0].Calculation == nil || advice.RecommendedPositions[1].Calculation == nil {
		t.Fatalf("expected calculation details")
	}
	if advice.RecommendedPositions[0].Calculation.TargetShareApplied != 0.55 {
		t.Fatalf("expected conservative target 0.55, got %f", advice.RecommendedPositions[0].Calculation.TargetShareApplied)
	}
	if advice.RecommendedPositions[1].Calculation.TargetShareApplied != 0.55 {
		t.Fatalf("expected neutral target 0.55, got %f", advice.RecommendedPositions[1].Calculation.TargetShareApplied)
	}

	foundWarning := false
	for _, warning := range advice.Warnings {
		if strings.Contains(warning, "收敛") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatalf("expected overlap warning, got %#v", advice.Warnings)
	}
}
