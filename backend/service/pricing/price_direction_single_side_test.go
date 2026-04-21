package pricing

import (
	"TgLpBot/base/models"
	"math"
	"testing"
)

func TestStablePercentagesFromTickPercentagesAllowsSingleSideWhenQuoteIsToken1(t *testing.T) {
	task := &models.StrategyTask{
		Token0Symbol: "BASED",
		Token1Symbol: "USDT",
	}

	lower, upper := StablePercentagesFromTickPercentages(task, 0, 12.5)
	if lower != 0 {
		t.Fatalf("expected lower to stay 0, got %v", lower)
	}
	if upper != 12.5 {
		t.Fatalf("expected upper to stay 12.5, got %v", upper)
	}
}

func TestStablePercentagesFromTickPercentagesAllowsSingleSideWhenQuoteIsToken0(t *testing.T) {
	task := &models.StrategyTask{
		Token0Symbol: "USDT",
		Token1Symbol: "BASED",
	}

	lower, upper := StablePercentagesFromTickPercentages(task, 0, 12.5)
	expectedLower := (12.5 / 112.5) * 100.0
	if math.Abs(lower-expectedLower) > 1e-9 {
		t.Fatalf("expected lower %v, got %v", expectedLower, lower)
	}
	if upper != 0 {
		t.Fatalf("expected upper to stay 0, got %v", upper)
	}
}
