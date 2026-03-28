package smart_money_golden_dog

import "testing"

func TestFreshPoolSelectColumnsIncludeActiveLiquidityUSD(t *testing.T) {
	for _, column := range freshPoolSelectColumns {
		if column == "active_liquidity_usd" {
			return
		}
	}
	t.Fatal("expected fresh pool query to select active_liquidity_usd for active fee rate filtering")
}
