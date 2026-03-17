package pool

import (
	"TgLpBot/base/blockchain"
	"math/big"
	"testing"
)

func TestCalcV4UnclaimedFeesFromGrowthsPositiveDelta(t *testing.T) {
	pos := &blockchain.V4PositionInfo{
		TickLower:                100,
		TickUpper:                200,
		Liquidity:                big.NewInt(2),
		FeeGrowthInside0LastX128: new(big.Int).Set(q128),
		FeeGrowthInside1LastX128: big.NewInt(0),
		TokensOwed0:              big.NewInt(3),
		TokensOwed1:              big.NewInt(5),
	}

	global0 := new(big.Int).Mul(new(big.Int).Set(q128), big.NewInt(3))
	global1 := new(big.Int).Mul(new(big.Int).Set(q128), big.NewInt(2))
	zero := big.NewInt(0)

	fees0, fees1, err := CalcV4UnclaimedFeesFromGrowths(150, pos, global0, global1, zero, zero, zero, zero)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fees0.Cmp(big.NewInt(7)) != 0 {
		t.Fatalf("fees0 = %s, want 7", fees0.String())
	}
	if fees1.Cmp(big.NewInt(9)) != 0 {
		t.Fatalf("fees1 = %s, want 9", fees1.String())
	}
}

func TestCalcV4UnclaimedFeesFromGrowthsRejectsInconsistentSnapshot(t *testing.T) {
	pos := &blockchain.V4PositionInfo{
		TickLower:                100,
		TickUpper:                200,
		Liquidity:                big.NewInt(10),
		FeeGrowthInside0LastX128: big.NewInt(20),
		FeeGrowthInside1LastX128: big.NewInt(10),
		TokensOwed0:              big.NewInt(11),
		TokensOwed1:              big.NewInt(12),
	}

	fees0, fees1, err := CalcV4UnclaimedFeesFromGrowths(
		150,
		pos,
		big.NewInt(5), big.NewInt(0),
		big.NewInt(7), big.NewInt(0),
		big.NewInt(0), big.NewInt(0),
	)
	if err == nil {
		t.Fatal("expected inconsistent snapshot error")
	}
	if fees0.Cmp(big.NewInt(11)) != 0 {
		t.Fatalf("fees0 = %s, want 11", fees0.String())
	}
	if fees1.Cmp(big.NewInt(12)) != 0 {
		t.Fatalf("fees1 = %s, want 12", fees1.String())
	}
}
