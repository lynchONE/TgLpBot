package web_server

import (
	"TgLpBot/base/config"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestLoadSmartMoneyV4Slot0_PrefersStateView(t *testing.T) {
	origCfg := config.AppConfig
	origStateView := smartMoneyGetV4Slot0ViaStateView
	origDirect := smartMoneyGetV4Slot0Direct
	t.Cleanup(func() {
		config.AppConfig = origCfg
		smartMoneyGetV4Slot0ViaStateView = origStateView
		smartMoneyGetV4Slot0Direct = origDirect
	})

	config.AppConfig = &config.Config{
		UniswapV4StateViewAddress: "0x1111111111111111111111111111111111111111",
	}

	stateCalled := 0
	directCalled := 0
	smartMoneyGetV4Slot0ViaStateView = func(stateView common.Address, poolManager common.Address, poolID string) (*big.Int, int, error) {
		stateCalled++
		return big.NewInt(123), 456, nil
	}
	smartMoneyGetV4Slot0Direct = func(poolManager common.Address, poolID string) (*big.Int, int, error) {
		directCalled++
		return nil, 0, fmt.Errorf("should not be called")
	}

	sqrtP, tick, err := loadSmartMoneyV4Slot0(common.HexToAddress("0x2222222222222222222222222222222222222222"), "0x01")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if sqrtP == nil || sqrtP.Cmp(big.NewInt(123)) != 0 || tick != 456 {
		t.Fatalf("unexpected slot0 result: sqrt=%v tick=%d", sqrtP, tick)
	}
	if stateCalled != 1 {
		t.Fatalf("expected state view call once, got %d", stateCalled)
	}
	if directCalled != 0 {
		t.Fatalf("expected direct call to be skipped, got %d", directCalled)
	}
}

func TestLoadSmartMoneyV4Slot0_FallsBackToDirect(t *testing.T) {
	origCfg := config.AppConfig
	origStateView := smartMoneyGetV4Slot0ViaStateView
	origDirect := smartMoneyGetV4Slot0Direct
	t.Cleanup(func() {
		config.AppConfig = origCfg
		smartMoneyGetV4Slot0ViaStateView = origStateView
		smartMoneyGetV4Slot0Direct = origDirect
	})

	config.AppConfig = &config.Config{
		UniswapV4StateViewAddress: "0x1111111111111111111111111111111111111111",
	}

	stateCalled := 0
	directCalled := 0
	smartMoneyGetV4Slot0ViaStateView = func(stateView common.Address, poolManager common.Address, poolID string) (*big.Int, int, error) {
		stateCalled++
		return nil, 0, fmt.Errorf("state view reverted")
	}
	smartMoneyGetV4Slot0Direct = func(poolManager common.Address, poolID string) (*big.Int, int, error) {
		directCalled++
		return big.NewInt(789), 321, nil
	}

	sqrtP, tick, err := loadSmartMoneyV4Slot0(common.HexToAddress("0x2222222222222222222222222222222222222222"), "0x01")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if sqrtP == nil || sqrtP.Cmp(big.NewInt(789)) != 0 || tick != 321 {
		t.Fatalf("unexpected slot0 result: sqrt=%v tick=%d", sqrtP, tick)
	}
	if stateCalled != 1 || directCalled != 1 {
		t.Fatalf("expected one state view call and one direct call, got state=%d direct=%d", stateCalled, directCalled)
	}
}
