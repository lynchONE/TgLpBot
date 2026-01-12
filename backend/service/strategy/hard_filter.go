package strategy

import (
	"context"
)

// HardFilterResult 硬筛检查结果
type HardFilterResult struct {
	Passed     bool
	FailReason string
	Metrics    HardFilterMetrics
}

// HardFilterMetrics 池子硬筛指标
type HardFilterMetrics struct {
	TVLUSD        float64
	FeePercentage float64
	FeeRate5mPct  float64
	TotalFees5m   float64
	TotalVolume5m float64
	TxCount5m     int
	TradingPair   string
}

// HardFilterChecker 硬筛检查函数类型
type HardFilterChecker func(ctx context.Context, poolAddress string, protocolVersion string) (*HardFilterResult, error)

// globalHardFilterChecker 全局硬筛检查函数
var globalHardFilterChecker HardFilterChecker

// SetHardFilterChecker 设置全局硬筛检查函数
func SetHardFilterChecker(fn HardFilterChecker) {
	globalHardFilterChecker = fn
}

// GetHardFilterChecker 获取全局硬筛检查函数
func GetHardFilterChecker() HardFilterChecker {
	return globalHardFilterChecker
}
