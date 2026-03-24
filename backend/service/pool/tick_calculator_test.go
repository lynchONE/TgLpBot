package pool

import "testing"

func TestCalculateTickFromPercentagesBestFitClampsUpperBound(t *testing.T) {
	t.Parallel()

	tc := NewTickCalculator()
	_, maxTick, err := FullRangeTicks(200)
	if err != nil {
		t.Fatalf("FullRangeTicks() error = %v", err)
	}

	tickLower, tickUpper := tc.CalculateTickFromPercentagesBestFit(maxTick-20, 1, 1, 200)
	if err := tc.ValidateTickRange(tickLower, tickUpper, 200); err != nil {
		t.Fatalf("ValidateTickRange() error = %v, range=[%d,%d]", err, tickLower, tickUpper)
	}
	if tickUpper != maxTick {
		t.Fatalf("expected upper tick to clamp to %d, got %d", maxTick, tickUpper)
	}
}

func TestCalculateTickFromPercentagesBestFitClampsLowerBound(t *testing.T) {
	t.Parallel()

	tc := NewTickCalculator()
	minTick, _, err := FullRangeTicks(200)
	if err != nil {
		t.Fatalf("FullRangeTicks() error = %v", err)
	}

	tickLower, tickUpper := tc.CalculateTickFromPercentagesBestFit(minTick+20, 1, 1, 200)
	if err := tc.ValidateTickRange(tickLower, tickUpper, 200); err != nil {
		t.Fatalf("ValidateTickRange() error = %v, range=[%d,%d]", err, tickLower, tickUpper)
	}
	if tickLower != minTick {
		t.Fatalf("expected lower tick to clamp to %d, got %d", minTick, tickLower)
	}
}

func TestNormalizeTickRangeExpandsCollapsedRangeNearUpperBound(t *testing.T) {
	t.Parallel()

	tc := NewTickCalculator()
	_, maxTick, err := FullRangeTicks(200)
	if err != nil {
		t.Fatalf("FullRangeTicks() error = %v", err)
	}

	tickLower, tickUpper, err := tc.NormalizeTickRange(maxTick+200, maxTick+400, 200)
	if err != nil {
		t.Fatalf("NormalizeTickRange() error = %v", err)
	}
	if err := tc.ValidateTickRange(tickLower, tickUpper, 200); err != nil {
		t.Fatalf("ValidateTickRange() error = %v, range=[%d,%d]", err, tickLower, tickUpper)
	}
	if tickUpper != maxTick {
		t.Fatalf("expected upper tick to clamp to %d, got %d", maxTick, tickUpper)
	}
}
