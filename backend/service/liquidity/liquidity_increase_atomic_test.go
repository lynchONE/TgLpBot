package liquidity

import (
	"strings"
	"testing"

	"TgLpBot/base/blockchain"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
)

func TestBuildAtomicIncreaseV3ParamsNormalizesEmptySwaps(t *testing.T) {
	t.Parallel()

	params := buildAtomicIncreaseV3Params(
		common.HexToAddress("0x0000000000000000000000000000000000000001"),
		common.HexToAddress("0x0000000000000000000000000000000000000002"),
		big.NewInt(123),
		common.HexToAddress("0x0000000000000000000000000000000000000003"),
		big.NewInt(456),
		blockchain.SwapParamsSimple{},
		blockchain.SwapParamsSimple{},
	)

	if params.EntrySwap.AmountIn == nil || params.EntrySwap.MinAmountOut == nil {
		t.Fatal("entry swap amounts should be normalized")
	}
	if params.RebalanceSwap.AmountIn == nil || params.RebalanceSwap.MinAmountOut == nil {
		t.Fatal("rebalance swap amounts should be normalized")
	}

	parsed, err := abi.JSON(strings.NewReader(blockchain.AtomicIncreaseZapABI))
	if err != nil {
		t.Fatalf("parse AtomicIncreaseZap ABI failed: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("pack zapIncreaseV3 panicked: %v", r)
		}
	}()
	if _, err := parsed.Pack("zapIncreaseV3", params); err != nil {
		t.Fatalf("pack zapIncreaseV3 failed: %v", err)
	}
}

func TestBuildAtomicIncreaseV4ParamsNormalizesEmptySwaps(t *testing.T) {
	t.Parallel()

	params := buildAtomicIncreaseV4Params(
		blockchain.PoolKeySimple{
			Currency0:   common.HexToAddress("0x0000000000000000000000000000000000000001"),
			Currency1:   common.HexToAddress("0x0000000000000000000000000000000000000002"),
			Fee:         big.NewInt(3000),
			TickSpacing: big.NewInt(60),
			Hooks:       common.HexToAddress("0x0000000000000000000000000000000000000003"),
		},
		common.HexToAddress("0x0000000000000000000000000000000000000004"),
		common.HexToAddress("0x0000000000000000000000000000000000000005"),
		big.NewInt(789),
		-120,
		120,
		big.NewInt(50),
		common.HexToAddress("0x0000000000000000000000000000000000000006"),
		big.NewInt(1000),
		blockchain.SwapParamsSimple{},
		blockchain.SwapParamsSimple{},
		big.NewInt(123456),
	)

	if params.EntrySwap.AmountIn == nil || params.EntrySwap.MinAmountOut == nil {
		t.Fatal("entry swap amounts should be normalized")
	}
	if params.RebalanceSwap.AmountIn == nil || params.RebalanceSwap.MinAmountOut == nil {
		t.Fatal("rebalance swap amounts should be normalized")
	}

	parsed, err := abi.JSON(strings.NewReader(blockchain.AtomicIncreaseZapABI))
	if err != nil {
		t.Fatalf("parse AtomicIncreaseZap ABI failed: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("pack zapIncreaseV4 panicked: %v", r)
		}
	}()
	if _, err := parsed.Pack("zapIncreaseV4", params); err != nil {
		t.Fatalf("pack zapIncreaseV4 failed: %v", err)
	}
}
