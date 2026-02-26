package txexec

import (
	"fmt"
	"strings"
	"sync"

	"TgLpBot/base/concurrency"
	"TgLpBot/base/config"
	"TgLpBot/service/wallet"

	"github.com/ethereum/go-ethereum/common"
)

// Executor provides per-wallet serialized execution for on-chain operations,
// while allowing concurrency across different wallets.
type Executor struct {
	limiter   *concurrency.KeyedLimiter
	walletSvc *wallet.WalletService
}

func NewExecutor(maxParallel int) *Executor {
	return &Executor{
		limiter:   concurrency.NewKeyedLimiter(maxParallel),
		walletSvc: wallet.NewWalletService(),
	}
}

func normalizeKey(addr string) string {
	return strings.ToLower(strings.TrimSpace(addr))
}

func (e *Executor) TryRunWallet(walletAddress string, fn func()) bool {
	if e == nil || e.limiter == nil || fn == nil {
		return false
	}
	key := normalizeKey(walletAddress)
	if !common.IsHexAddress(key) {
		return false
	}
	return e.limiter.TryRun(key, fn)
}

func (e *Executor) TryRunUser(userID uint, fn func(walletAddress string)) (bool, error) {
	if e == nil || fn == nil || userID == 0 {
		return false, nil
	}
	if e.walletSvc == nil {
		e.walletSvc = wallet.NewWalletService()
	}
	w, err := e.walletSvc.GetDefaultWallet(userID)
	if err != nil {
		return false, err
	}
	addr := strings.TrimSpace(w.Address)
	if !common.IsHexAddress(addr) {
		return false, fmt.Errorf("invalid wallet address: %s", addr)
	}
	ok := e.TryRunWallet(addr, func() { fn(addr) })
	return ok, nil
}

// TryRunTask runs a function serialized by the task's wallet address.
// Resolution rule: wallet_id/wallet_address -> default wallet (legacy tasks).
func (e *Executor) TryRunTask(userID uint, walletID uint, walletAddress string, fn func(walletAddress string)) (bool, error) {
	if e == nil || fn == nil || userID == 0 {
		return false, nil
	}
	if e.walletSvc == nil {
		e.walletSvc = wallet.NewWalletService()
	}
	w, err := e.walletSvc.ResolveTaskWallet(userID, walletID, walletAddress)
	if err != nil {
		return false, err
	}
	addr := strings.TrimSpace(w.Address)
	if !common.IsHexAddress(addr) {
		return false, fmt.Errorf("invalid wallet address: %s", addr)
	}
	ok := e.TryRunWallet(addr, func() { fn(addr) })
	return ok, nil
}

var (
	defaultOnce sync.Once
	defaultExec *Executor
)

func Default() *Executor {
	defaultOnce.Do(func() {
		max := 8
		if config.AppConfig != nil && config.AppConfig.WalletTxMaxParallel > 0 {
			max = config.AppConfig.WalletTxMaxParallel
		}
		defaultExec = NewExecutor(max)
	})
	return defaultExec
}
