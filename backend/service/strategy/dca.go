package strategy

import (
	"encoding/json"
	"fmt"
	"math"
)

const (
	DCAMinBatchCount      = 2
	DCAMaxBatchCount      = 5
	DCAMinBatchPercent    = 5.0
	DCASumTolerance       = 0.01
	DCAMinIntervalSeconds = 10
	DCAMaxIntervalSeconds = 600
)

// NormalizeDCAPercentages validates and rounds a batch percentage slice.
// The slice must have 2–5 entries, each >= 5%, summing to ~100 (within 0.01 tolerance).
// Returns a copy rounded to 4 decimals for deterministic JSON storage.
func NormalizeDCAPercentages(arr []float64) ([]float64, error) {
	n := len(arr)
	if n < DCAMinBatchCount || n > DCAMaxBatchCount {
		return nil, fmt.Errorf("分批次数必须在 %d-%d 之间，当前为 %d", DCAMinBatchCount, DCAMaxBatchCount, n)
	}
	out := make([]float64, n)
	sum := 0.0
	for i, v := range arr {
		if !isFinite(v) || v < DCAMinBatchPercent {
			return nil, fmt.Errorf("第 %d 批百分比 %.2f 无效（必须 >= %.0f%%）", i+1, v, DCAMinBatchPercent)
		}
		rounded := math.Round(v*10000) / 10000
		out[i] = rounded
		sum += rounded
	}
	if math.Abs(sum-100.0) > DCASumTolerance {
		return nil, fmt.Errorf("分批百分比之和必须等于 100，当前为 %.4f", sum)
	}
	return out, nil
}

// ParseDCAPercentages decodes the JSON-encoded batch plan stored on GlobalConfig / StrategyTask.
// Returns (nil, false) on any malformed input so callers can fall back safely.
func ParseDCAPercentages(js string) ([]float64, bool) {
	if js == "" {
		return nil, false
	}
	var arr []float64
	if err := json.Unmarshal([]byte(js), &arr); err != nil {
		return nil, false
	}
	if len(arr) < DCAMinBatchCount {
		return nil, false
	}
	return arr, true
}

// NormalizeDCAInterval clamps the interval into the supported range.
func NormalizeDCAInterval(seconds int) (int, error) {
	if seconds < DCAMinIntervalSeconds || seconds > DCAMaxIntervalSeconds {
		return 0, fmt.Errorf("批次间隔必须在 %d-%d 秒之间，当前为 %d", DCAMinIntervalSeconds, DCAMaxIntervalSeconds, seconds)
	}
	return seconds, nil
}

// MarshalDCAPercentages serialises a normalised slice back to the JSON form used for storage.
func MarshalDCAPercentages(arr []float64) (string, error) {
	b, err := json.Marshal(arr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}
