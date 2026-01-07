package auto_lp

import (
	"sort"
	"testing"
	"time"

	"TgLpBot/base/models"
)

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
