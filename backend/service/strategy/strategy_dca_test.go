package strategy

import (
	"errors"
	"testing"
	"time"
)

func TestDCARetryDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		attempt int
		want    time.Duration
		ok      bool
	}{
		{name: "first", attempt: 1, want: 500 * time.Millisecond, ok: true},
		{name: "second", attempt: 2, want: 1 * time.Second, ok: true},
		{name: "third", attempt: 3, want: 2 * time.Second, ok: true},
		{name: "fourth", attempt: 4, want: 3 * time.Second, ok: true},
		{name: "fifth", attempt: 5, want: 5 * time.Second, ok: true},
		{name: "sixth", attempt: 6, want: 10 * time.Second, ok: true},
		{name: "exhausted", attempt: 7, want: 0, ok: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := dcaRetryDelay(tc.attempt)
			if ok != tc.ok {
				t.Fatalf("dcaRetryDelay(%d) ok = %v, want %v", tc.attempt, ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("dcaRetryDelay(%d) = %v, want %v", tc.attempt, got, tc.want)
			}
		})
	}
}

func TestIsRetryableDCASlippageError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "slippage marker", err: errors.New("swap failed: slippage exceeded"), want: true},
		{name: "v4 max amount selector", err: errors.New("execution reverted: 0x31e30ad0"), want: true},
		{name: "price moved marker", err: errors.New("price moved out of tolerance"), want: true},
		{name: "unrelated error", err: errors.New("failed to get wallet"), want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isRetryableDCASlippageError(tc.err); got != tc.want {
				t.Fatalf("isRetryableDCASlippageError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
