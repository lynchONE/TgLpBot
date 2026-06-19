package strategy

import (
	"math"
	"testing"
)

func TestRetryGasMultiplier(t *testing.T) {
	cases := []struct {
		attempt int
		want    float64
	}{
		{-1, 1.0},
		{0, 1.0},
		{1, 1.25},
		{2, 1.5},
		{8, 3.0},  // 1 + 0.25*8 == 3.0 (exactly at cap)
		{9, 3.0},  // would be 3.25, capped
		{99, 3.0}, // capped
	}
	for _, tc := range cases {
		if got := retryGasMultiplier(tc.attempt); got != tc.want {
			t.Errorf("retryGasMultiplier(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}

	// Monotonic non-decreasing.
	prev := retryGasMultiplier(0)
	for a := 1; a <= 20; a++ {
		cur := retryGasMultiplier(a)
		if cur < prev {
			t.Fatalf("retryGasMultiplier not monotonic at attempt=%d: %v < %v", a, cur, prev)
		}
		if cur > 3.0 {
			t.Fatalf("retryGasMultiplier(%d)=%v exceeds cap 3.0", a, cur)
		}
		prev = cur
	}
}

func TestDCARetrySlippagePercent(t *testing.T) {
	const eps = 1e-9
	cases := []struct {
		base    float64
		attempt int
		want    float64
	}{
		{0, 0, 0.5},     // base<=0 -> default 0.5; attempt<=0 -> base
		{-3, 0, 0.5},    // negative base -> default 0.5
		{0, 3, 4.0},     // 0.5 * 2^3
		{0.5, 0, 0.5},   // attempt<=0 -> base
		{0.5, 1, 1.0},   // 0.5 * 2
		{0.5, 2, 2.0},   // 0.5 * 4
		{0.5, 5, 10.0},  // 0.5*32=16 -> capped at 10
		{1.0, 0, 1.0},   // attempt<=0 -> base
		{12.0, 1, 12.0}, // base above cap: widened 24 -> cap 10 -> never below base -> 12
	}
	for _, tc := range cases {
		got := dcaRetrySlippagePercent(tc.base, tc.attempt)
		if math.Abs(got-tc.want) > eps {
			t.Errorf("dcaRetrySlippagePercent(%v, %d) = %v, want %v", tc.base, tc.attempt, got, tc.want)
		}
	}

	// Never below the (sanitized) base, and never above 10 for sane bases.
	for a := 0; a <= 10; a++ {
		got := dcaRetrySlippagePercent(0.5, a)
		if got < 0.5-eps {
			t.Fatalf("dcaRetrySlippagePercent(0.5,%d)=%v below base", a, got)
		}
		if got > 10+eps {
			t.Fatalf("dcaRetrySlippagePercent(0.5,%d)=%v above cap", a, got)
		}
	}
}
