package services

import (
	"TgLpBot/config"
	"TgLpBot/models"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

const stableUnknown = -1

var stableSymbols = map[string]struct{}{
	"USDT": {},
	"USDC": {},
	"BUSD": {},
	"DAI":  {},
}

// stableSideFromTask returns which token is the stable quote.
// -1: unknown, 0: token0, 1: token1.
func stableSideFromTask(task *models.StrategyTask) int {
	if task == nil {
		return stableUnknown
	}

	if config.AppConfig != nil && common.IsHexAddress(config.AppConfig.USDTAddress) {
		usdtAddr := strings.ToLower(strings.TrimSpace(config.AppConfig.USDTAddress))
		if usdtAddr != "" {
			t0 := strings.ToLower(strings.TrimSpace(task.Token0Address))
			t1 := strings.ToLower(strings.TrimSpace(task.Token1Address))
			if common.IsHexAddress(t0) && t0 == usdtAddr {
				return 0
			}
			if common.IsHexAddress(t1) && t1 == usdtAddr {
				return 1
			}
		}
	}

	sym0 := strings.ToUpper(strings.TrimSpace(task.Token0Symbol))
	sym1 := strings.ToUpper(strings.TrimSpace(task.Token1Symbol))
	if _, ok := stableSymbols[sym0]; ok {
		return 0
	}
	if _, ok := stableSymbols[sym1]; ok {
		return 1
	}
	return stableUnknown
}

// priceDirectionFromTicks returns out-of-range direction in stable price terms.
// isAbove/isBelow are based on raw ticks; priceUp/priceDown map to stable price direction.
func priceDirectionFromTicks(task *models.StrategyTask, tickLower, tickUpper, currentTick int) (isAbove bool, isBelow bool, priceUp bool, priceDown bool) {
	isAbove = currentTick > tickUpper
	isBelow = currentTick < tickLower
	priceUp = isAbove
	priceDown = isBelow

	if stableSideFromTask(task) == 0 {
		priceUp = isBelow
		priceDown = isAbove
	}
	return
}
