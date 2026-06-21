package liquidity

import (
	"math"
	"math/big"
	"testing"

	"TgLpBot/base/config"
	poolmath "TgLpBot/service/pool"

	"github.com/ethereum/go-ethereum/common"
)

func TestLiquidityGuardHexIdentifierAllowsV4PoolID(t *testing.T) {
	v4PoolID := "0x12a67c47baf61c0d8d43870a1115ac683f1e5907c2653f08c69bd5ef794a23aa"
	if !isLiquidityGuardHexIdentifier(v4PoolID) {
		t.Fatalf("expected v4 pool id to be accepted")
	}

	if !isLiquidityGuardHexIdentifier("0x1111111111111111111111111111111111111111") {
		t.Fatalf("expected v3 pool address to be accepted")
	}

	if isLiquidityGuardHexIdentifier("0x1234") {
		t.Fatalf("expected short hex to be rejected")
	}
}

func TestLiquidityAmountsUSDValueUsesPoolPriceWhenToken0IsStable(t *testing.T) {
	cc := config.ChainConfig{
		Chain:         "bsc",
		StableAddress: "0x55d398326f99059ff775485246999027b3197955",
		USDTAddress:   "0x55d398326f99059ff775485246999027b3197955",
	}
	state := &openPositionGuardState{
		Token0:         common.HexToAddress(cc.USDTAddress),
		Token1:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Token0Symbol:   "USDT",
		Token1Symbol:   "SIREN",
		Token0Decimals: 18,
		Token1Decimals: 18,
	}

	sqrtPrice, err := poolmath.SqrtPriceX96FromHumanPrice(big.NewFloat(25), 18, 18)
	if err != nil {
		t.Fatalf("SqrtPriceX96FromHumanPrice() error = %v", err)
	}
	state.SqrtPriceX96 = sqrtPrice

	amount0 := new(big.Int).Mul(big.NewInt(100), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	amount1 := new(big.Int).Mul(big.NewInt(2500), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	got, err := liquidityAmountsUSDValueFromPoolPrice("bsc", state, cc, amount0, amount1)
	if err != nil {
		t.Fatalf("liquidityAmountsUSDValueFromPoolPrice() error = %v", err)
	}
	if math.Abs(got-200) > 0.0001 {
		t.Fatalf("liquidity USD = %.8f, want 200", got)
	}
}

func TestEstimateActiveLiquidityUSDFromChainStateUsesCurrentTickBin(t *testing.T) {
	cc := config.ChainConfig{
		Chain:         "bsc",
		StableAddress: "0x55d398326f99059ff775485246999027b3197955",
		USDTAddress:   "0x55d398326f99059ff775485246999027b3197955",
	}
	sqrtPrice, err := poolmath.SqrtRatioAtTick(0)
	if err != nil {
		t.Fatalf("SqrtRatioAtTick() error = %v", err)
	}

	state := &openPositionGuardState{
		Chain:          "bsc",
		Token0:         common.HexToAddress(cc.USDTAddress),
		Token1:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Token0Symbol:   "USDT",
		Token1Symbol:   "SIREN",
		Token0Decimals: 18,
		Token1Decimals: 18,
		SqrtPriceX96:   sqrtPrice,
		CurrentTick:    0,
		TickSpacing:    60,
		RawLiquidity:   new(big.Int).Exp(big.NewInt(10), big.NewInt(20), nil),
	}

	got, source, err := estimateActiveLiquidityUSDFromChainState("bsc", state, cc)
	if err != nil {
		t.Fatalf("estimateActiveLiquidityUSDFromChainState() error = %v", err)
	}
	if got <= 0 {
		t.Fatalf("estimated liquidity USD = %.8f, want positive", got)
	}
	if source != "onchain.active_liquidity_tick_bin" {
		t.Fatalf("source = %q, want onchain.active_liquidity_tick_bin", source)
	}
}
