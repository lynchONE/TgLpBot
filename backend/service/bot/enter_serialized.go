package bot

import (
	"errors"
	"fmt"
	"time"

	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/txexec"
)

// errWalletBusy is returned by enterTaskSerialized when the task's wallet is already
// executing another on-chain operation. It is a transient condition, not a failure:
// callers should surface a "try again" message and must NOT mark the task as errored.
var errWalletBusy = errors.New("wallet busy")

// enterTaskSerialized runs EnterTaskFromUSDT under the per-wallet serializer (txexec) so a
// manual open cannot race a strategy-driven exit/DCA/rebalance on the same wallet and collide
// on the nonce. The underlying enter error (including *liquidity.EntrySwapRequiredError) is
// propagated unchanged so existing errors.As branches keep working.
func (b *Bot) enterTaskSerialized(userID uint, task *models.StrategyTask) (*liquidity.EnterResult, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}

	type enterOutcome struct {
		res *liquidity.EnterResult
		err error
	}
	resultCh := make(chan enterOutcome, 1)

	ok, tryErr := txexec.Default().TryRunTask(userID, task.WalletID, task.WalletAddress, func(_ string) {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- enterOutcome{err: fmt.Errorf("开仓执行 panic: %v", r)}
			}
		}()
		res, err := b.liquidityService.EnterTaskFromUSDT(userID, task)
		resultCh <- enterOutcome{res: res, err: err}
	})
	if tryErr != nil {
		return nil, tryErr
	}
	if !ok {
		return nil, errWalletBusy
	}

	select {
	case out := <-resultCh:
		return out.res, out.err
	case <-time.After(10 * time.Minute):
		return nil, fmt.Errorf("开仓执行超时")
	}
}
