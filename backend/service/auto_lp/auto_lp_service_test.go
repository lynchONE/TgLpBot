package auto_lp

import (
	"sort"
	"testing"
	"time"

	"TgLpBot/base/models"
)

func TestClassifyTrendMACross(t *testing.T) {
	t.Run("insufficient_points", func(t *testing.T) {
		trend, _, ok := classifyTrendMACross(100, 100, 3, 12, 0.3)
		if ok || trend != "UNKNOWN" {
			t.Fatalf("expected UNKNOWN/ok=false; got trend=%s ok=%v", trend, ok)
		}
	})

	t.Run("invalid_threshold", func(t *testing.T) {
		trend, _, ok := classifyTrendMACross(100, 100, 4, 12, 0)
		if ok || trend != "UNKNOWN" {
			t.Fatalf("expected UNKNOWN/ok=false; got trend=%s ok=%v", trend, ok)
		}
	})

	t.Run("uptrend", func(t *testing.T) {
		trend, crossPct, ok := classifyTrendMACross(101, 100, 4, 12, 0.3)
		if !ok || trend != "UPTREND" {
			t.Fatalf("expected UPTREND/ok=true; got trend=%s ok=%v cross=%.4f", trend, ok, crossPct)
		}
	})

	t.Run("downtrend", func(t *testing.T) {
		trend, crossPct, ok := classifyTrendMACross(99, 100, 4, 12, 0.3)
		if !ok || trend != "DOWNTREND" {
			t.Fatalf("expected DOWNTREND/ok=true; got trend=%s ok=%v cross=%.4f", trend, ok, crossPct)
		}
	})

	t.Run("sideways", func(t *testing.T) {
		trend, crossPct, ok := classifyTrendMACross(100.1, 100, 4, 12, 0.3)
		if !ok || trend != "SIDEWAYS" {
			t.Fatalf("expected SIDEWAYS/ok=true; got trend=%s ok=%v cross=%.4f", trend, ok, crossPct)
		}
	})
}

func TestAutoLPDev5Pct(t *testing.T) {
	if _, ok := autoLPDev5Pct(100, 100, 3); ok {
		t.Fatalf("expected ok=false for insufficient points")
	}
	got, ok := autoLPDev5Pct(99, 100, 4)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if got != -1 {
		t.Fatalf("expected -1; got %v", got)
	}
}

func TestAutoLPEntryGate(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		blocked, reason := autoLPEntryGate(false, "DOWNTREND", true, -10, true, 0.5)
		if blocked || reason != "" {
			t.Fatalf("expected not blocked; got blocked=%v reason=%q", blocked, reason)
		}
	})

	t.Run("trend_unknown", func(t *testing.T) {
		blocked, reason := autoLPEntryGate(true, "UNKNOWN", false, 0, true, 0.5)
		if !blocked || reason != "TREND_UNKNOWN" {
			t.Fatalf("expected TREND_UNKNOWN; got blocked=%v reason=%q", blocked, reason)
		}
	})

	t.Run("trend_down", func(t *testing.T) {
		blocked, reason := autoLPEntryGate(true, "DOWNTREND", true, 0, true, 0.5)
		if !blocked || reason != "TREND_DOWN" {
			t.Fatalf("expected TREND_DOWN; got blocked=%v reason=%q", blocked, reason)
		}
	})

	t.Run("dev5_unknown", func(t *testing.T) {
		blocked, reason := autoLPEntryGate(true, "SIDEWAYS", true, 0, false, 0.5)
		if !blocked || reason != "DEV5_UNKNOWN" {
			t.Fatalf("expected DEV5_UNKNOWN; got blocked=%v reason=%q", blocked, reason)
		}
	})

	t.Run("dev5_drop", func(t *testing.T) {
		blocked, reason := autoLPEntryGate(true, "SIDEWAYS", true, -0.6, true, 0.5)
		if !blocked || reason != "DEV5_DROP" {
			t.Fatalf("expected DEV5_DROP; got blocked=%v reason=%q", blocked, reason)
		}
	})

	t.Run("dev5_gate_disabled", func(t *testing.T) {
		blocked, reason := autoLPEntryGate(true, "SIDEWAYS", true, -99, true, 0)
		if blocked || reason != "" {
			t.Fatalf("expected not blocked when dev5 gate disabled; got blocked=%v reason=%q", blocked, reason)
		}
	})

	t.Run("allowed", func(t *testing.T) {
		blocked, reason := autoLPEntryGate(true, "SIDEWAYS", true, -0.4, true, 0.5)
		if blocked || reason != "" {
			t.Fatalf("expected not blocked; got blocked=%v reason=%q", blocked, reason)
		}
	})
}

func TestAutoLPAnalysisLess_FeeRate5mPctFirst(t *testing.T) {
	// FeeRate5mPct 高的应该排在前面
	highFeeRate := AutoLPAnalysis{FeeRate5mPct: 0.15, Score: 1}
	lowFeeRate := AutoLPAnalysis{FeeRate5mPct: 0.10, Score: 9999999}

	if !autoLPAnalysisLess(highFeeRate, lowFeeRate) {
		t.Fatalf("expected higher FeeRate5mPct to rank first regardless of score")
	}
	if autoLPAnalysisLess(lowFeeRate, highFeeRate) {
		t.Fatalf("expected lower FeeRate5mPct to not rank first even with higher score")
	}
}

func TestAutoLPAnalysisLess_ScoreTieBreak(t *testing.T) {
	a := AutoLPAnalysis{FeeRate5mPct: 0.15, Score: 10}
	b := AutoLPAnalysis{FeeRate5mPct: 0.15, Score: 20}

	if autoLPAnalysisLess(a, b) {
		t.Fatalf("expected higher score to rank first when FeeRate5mPct ties")
	}
	if !autoLPAnalysisLess(b, a) {
		t.Fatalf("expected higher score to rank first when FeeRate5mPct ties")
	}
}

func TestScoreCandidate_FeeRate5mPctDominates(t *testing.T) {
	fees5m := 150.0
	tvl := 250_000.0
	state := "SIDEWAYS"
	res := "NONE"

	sLow := scoreCandidate(0.10, fees5m, tvl, res, state)
	sHigh := scoreCandidate(0.15, fees5m, tvl, res, state)

	if sHigh <= sLow {
		t.Fatalf("expected higher FeeRate5mPct score to be larger: high=%v low=%v", sHigh, sLow)
	}
}

func TestSortAnalyses_FeeRate5mPctPrimary(t *testing.T) {
	analyses := []AutoLPAnalysis{
		{FeeRate5mPct: 0.05, Score: 999},
		{FeeRate5mPct: 0.15, Score: 1},
		{FeeRate5mPct: 0.20, Score: -100},
		{FeeRate5mPct: 0.15, Score: 2},
	}
	sort.Slice(analyses, func(i, j int) bool {
		return autoLPAnalysisLess(analyses[i], analyses[j])
	})

	if got := analyses[0].FeeRate5mPct; got != 0.20 {
		t.Fatalf("expected highest FeeRate5mPct first; got %v", got)
	}
	if got := analyses[1].FeeRate5mPct; got != 0.15 {
		t.Fatalf("expected second tier FeeRate5mPct 0.15; got %v", got)
	}
	// 相同 FeeRate5mPct 时按 Score 降序
	if got := analyses[1].Score; got != 2 {
		t.Fatalf("expected higher score within same FeeRate5mPct; got %v", got)
	}
	if got := analyses[len(analyses)-1].FeeRate5mPct; got != 0.05 {
		t.Fatalf("expected lowest FeeRate5mPct last; got %v", got)
	}
}

func TestAutoLPResolveSwitchCooldownSeconds_Default(t *testing.T) {
	cfg := models.AutoLPUserConfig{}
	if got := autoLPResolveSwitchCooldownSeconds(cfg); got != 300 {
		t.Fatalf("autoLPResolveSwitchCooldownSeconds(default)=%d want=300", got)
	}
}

func TestAutoLPResolveSwitchCooldownSeconds_Custom(t *testing.T) {
	cfg := models.AutoLPUserConfig{SwitchCooldownSeconds: 600}
	if got := autoLPResolveSwitchCooldownSeconds(cfg); got != 600 {
		t.Fatalf("autoLPResolveSwitchCooldownSeconds(custom)=%d want=600", got)
	}
}

func TestAutoLPTopCandidate(t *testing.T) {
	analyses := []AutoLPAnalysis{
		{Action: "SKIP", TradingPair: "A"},
		{Action: "CANDIDATE", TradingPair: "B"},
		{Action: "CANDIDATE", TradingPair: "C"},
	}
	got, ok := autoLPTopCandidate(analyses)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if got.TradingPair != "B" {
		t.Fatalf("expected top candidate B; got %q", got.TradingPair)
	}
}

func TestAutoLPSelectWorstTaskForSwitch(t *testing.T) {
	tasks := []models.StrategyTask{
		{ID: 1, PoolVersion: "v3", PoolId: "0xaaa", V3TokenID: "1"},
		{ID: 2, PoolVersion: "v3", PoolId: "0xbbb", V3TokenID: "2"},
	}
	analysisByPool := map[string]AutoLPAnalysis{
		autoLPPoolKey("v3", "0xaaa"): {FeeRate5mPct: 1.2},
		autoLPPoolKey("v3", "0xbbb"): {FeeRate5mPct: 0.5},
	}
	worst, worstYield := autoLPSelectWorstTaskForSwitch(tasks, analysisByPool)
	if worst == nil {
		t.Fatalf("expected worst task")
	}
	if worst.ID != 2 {
		t.Fatalf("expected worst task id=2; got %d", worst.ID)
	}
	if worstYield != 0.5 {
		t.Fatalf("expected worstYield=0.5; got %v", worstYield)
	}
}

func TestAutoLPSelectWorstTaskForSwitch_IgnoresIneligible(t *testing.T) {
	tasks := []models.StrategyTask{
		{ID: 1, PoolVersion: "v3", PoolId: "0xaaa", V3TokenID: "1", ExitPendingAction: "stoploss"},
		{ID: 2, PoolVersion: "v3", PoolId: "0xbbb", V3TokenID: "2"},
	}
	analysisByPool := map[string]AutoLPAnalysis{
		autoLPPoolKey("v3", "0xaaa"): {FeeRate5mPct: 0.1},
		autoLPPoolKey("v3", "0xbbb"): {FeeRate5mPct: 0.2},
	}
	worst, _ := autoLPSelectWorstTaskForSwitch(tasks, analysisByPool)
	if worst == nil || worst.ID != 2 {
		t.Fatalf("expected ineligible task to be ignored and pick id=2; got %+v", worst)
	}
}

func TestAutoLPWithinSwitchCooldown(t *testing.T) {
	now := time.Now()

	last := now.Add(-299 * time.Second)
	if !autoLPWithinSwitchCooldown(now, &last, 300) {
		t.Fatalf("expected within cooldown")
	}

	last2 := now.Add(-301 * time.Second)
	if autoLPWithinSwitchCooldown(now, &last2, 300) {
		t.Fatalf("expected outside cooldown")
	}
}
