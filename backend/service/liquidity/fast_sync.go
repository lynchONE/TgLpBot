package liquidity

import (
	"fmt"
	"math/big"
	"time"
)

func (s *LiquidityService) fastSyncDurations() []time.Duration {
	return []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1 * time.Second,
	}
}

func (s *LiquidityService) fastAllowanceDurations() []time.Duration {
	return []time.Duration{
		150 * time.Millisecond,
		250 * time.Millisecond,
		400 * time.Millisecond,
		600 * time.Millisecond,
		1 * time.Second,
	}
}

func waitBigIntAtLeast(
	initial *big.Int,
	initialErr error,
	want *big.Int,
	delays []time.Duration,
	read func() (*big.Int, error),
) (*big.Int, error) {
	current := cloneBig(initial)
	if current == nil {
		current = big.NewInt(0)
	}
	if want == nil || want.Sign() <= 0 || (initialErr == nil && current.Cmp(want) >= 0) {
		return current, initialErr
	}

	lastErr := initialErr
	for _, delay := range delays {
		time.Sleep(delay)
		next, err := read()
		if next == nil {
			next = big.NewInt(0)
		}
		current = next
		lastErr = err
		if err == nil && current.Cmp(want) >= 0 {
			return current, nil
		}
	}
	if lastErr != nil {
		return current, lastErr
	}
	return current, fmt.Errorf("value still below target: have=%s want>=%s", current.String(), want.String())
}
