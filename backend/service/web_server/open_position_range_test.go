package web_server

import (
	"TgLpBot/base/models"
	"testing"
)

func TestResolveOpenPositionRangeAllowsSingleSidedTickRange(t *testing.T) {
	lower := -448
	upper := -392
	task := &models.StrategyTask{
		Token0Symbol: "BASED",
		Token1Symbol: "USDT",
	}
	req := openPositionRequest{
		RangeInputMode: openPositionRangeInputTick,
		TickLower:      &lower,
		TickUpper:      &upper,
	}

	resolved, errResp, status := resolveOpenPositionRange(task, req, -504, 56)
	if errResp != nil || status != 0 {
		t.Fatalf("expected single-sided tick range to resolve, got err=%v status=%d", errResp, status)
	}
	if resolved.TickLower != lower || resolved.TickUpper != upper {
		t.Fatalf("expected ticks [%d,%d], got [%d,%d]", lower, upper, resolved.TickLower, resolved.TickUpper)
	}
	if resolved.TickLowerPct != 0 {
		t.Fatalf("expected lower pct to be 0 for single-sided upper range, got %v", resolved.TickLowerPct)
	}
	if resolved.TickUpperPct <= 0 {
		t.Fatalf("expected upper pct to stay positive, got %v", resolved.TickUpperPct)
	}
}
