package strategy

import "testing"

func TestNormalizeRebalanceTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   int
		want int
	}{
		{in: 0, want: -1},
		{in: -1, want: -1},
		{in: 10, want: 10},
	}

	for _, tc := range tests {
		if got := NormalizeRebalanceTimeout(tc.in); got != tc.want {
			t.Fatalf("NormalizeRebalanceTimeout(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestFormatDelayTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   int
		want string
	}{
		{in: -1, want: "立即"},
		{in: 0, want: "立即"},
		{in: 10, want: "10 秒"},
		{in: 120, want: "2 分钟"},
	}

	for _, tc := range tests {
		if got := FormatDelayTime(tc.in); got != tc.want {
			t.Fatalf("FormatDelayTime(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
