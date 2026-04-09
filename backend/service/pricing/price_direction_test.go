package pricing

import (
	"math"
	"testing"

	"TgLpBot/base/models"
)

func TestBuildPriceDisplayUsesNonQuoteTokenAsBase(t *testing.T) {
	t.Parallel()

	task := &models.StrategyTask{
		Chain:        "bsc",
		Token0Symbol: "WBNB",
		Token1Symbol: "JELLY",
	}

	price, base, quote, ok := BuildPriceDisplay(task, 6932)
	if !ok {
		t.Fatal("BuildPriceDisplay returned ok=false")
	}
	if base != "JELLY" || quote != "WBNB" {
		t.Fatalf("BuildPriceDisplay base/quote = %s/%s, want JELLY/WBNB", base, quote)
	}
	if math.Abs(price-0.5) > 0.01 {
		t.Fatalf("BuildPriceDisplay price = %.6f, want about 0.5", price)
	}
}

func TestBuildRangeDisplayUsesNonQuoteTokenAsBase(t *testing.T) {
	t.Parallel()

	task := &models.StrategyTask{
		Chain:        "bsc",
		Token0Symbol: "WBNB",
		Token1Symbol: "JELLY",
	}

	lower, upper, base, quote, ok := BuildRangeDisplay(task, 0, 6932)
	if !ok {
		t.Fatal("BuildRangeDisplay returned ok=false")
	}
	if base != "JELLY" || quote != "WBNB" {
		t.Fatalf("BuildRangeDisplay base/quote = %s/%s, want JELLY/WBNB", base, quote)
	}
	if !(lower < upper) {
		t.Fatalf("BuildRangeDisplay lower/upper = %.6f/%.6f, want lower < upper", lower, upper)
	}
	if math.Abs(lower-0.5) > 0.01 || math.Abs(upper-1.0) > 0.01 {
		t.Fatalf("BuildRangeDisplay lower/upper = %.6f/%.6f, want about 0.5/1.0", lower, upper)
	}
}

func TestTickPercentagesFromStablePercentagesTreatsWrappedNativeAsQuoteSide(t *testing.T) {
	t.Parallel()

	task := &models.StrategyTask{
		Chain:        "bsc",
		Token0Symbol: "WBNB",
		Token1Symbol: "JELLY",
	}

	lower, upper := TickPercentagesFromStablePercentages(task, 10, 10)
	if math.Abs(lower-9.090909) > 0.0001 {
		t.Fatalf("tick lower = %.6f, want about 9.090909", lower)
	}
	if math.Abs(upper-11.111111) > 0.0001 {
		t.Fatalf("tick upper = %.6f, want about 11.111111", upper)
	}
}
