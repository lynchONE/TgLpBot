package liquidity

import (
	"TgLpBot/base/config"
	"log"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
)

func normalizeZapGasLimitMultiplier(v float64) float64 {
	if v <= 0 {
		return 1
	}
	if v > 10 {
		return 10
	}
	return v
}

func zapGasLimitSettings() (mult float64, minLimit uint64, maxLimit uint64) {
	mult = 1.30
	if config.AppConfig != nil {
		if config.AppConfig.ZapGasLimitMultiplier > 0 {
			mult = config.AppConfig.ZapGasLimitMultiplier
		}
		minLimit = config.AppConfig.ZapGasLimitMin
		maxLimit = config.AppConfig.ZapGasLimitMax
	}
	mult = normalizeZapGasLimitMultiplier(mult)
	return mult, minLimit, maxLimit
}

func applyGasLimitMultiplier(base uint64, mult float64) uint64 {
	if base == 0 || mult == 1 {
		return base
	}
	withMult := uint64(float64(base) * mult)
	if withMult < base {
		return base
	}
	return withMult
}

func tuneZapTxGasLimit(label string, auth *bind.TransactOpts, buildTx func(*bind.TransactOpts) (*types.Transaction, error)) {
	if auth == nil || buildTx == nil {
		return
	}
	if auth.GasLimit > 0 {
		log.Printf("[Liquidity] %s gasLimit: using explicit gasLimit=%d", label, auth.GasLimit)
		return
	}

	mult, minLimit, maxLimit := zapGasLimitSettings()

	preview := *auth
	preview.NoSend = true
	tx, err := buildTx(&preview)
	if err != nil {
		if minLimit > 0 {
			auth.GasLimit = minLimit
			log.Printf("[Liquidity] %s gasLimit: EstimateGas failed, fallback to min=%d (mult=%.4f max=%d): %v",
				label, minLimit, mult, maxLimit, err)
			return
		}
		log.Printf("[Liquidity] %s gasLimit: EstimateGas failed, using node estimation (mult=%.4f min=%d max=%d): %v",
			label, mult, minLimit, maxLimit, err)
		return
	}

	estimated := tx.Gas()
	gasLimit := applyGasLimitMultiplier(estimated, mult)
	if minLimit > 0 && gasLimit < minLimit {
		gasLimit = minLimit
	}
	if maxLimit > 0 && gasLimit > maxLimit {
		gasLimit = maxLimit
	}
	if gasLimit > 0 {
		auth.GasLimit = gasLimit
	}

	log.Printf("[Liquidity] %s gasLimit: estimated=%d final=%d (mult=%.4f min=%d max=%d)",
		label, estimated, gasLimit, mult, minLimit, maxLimit)
}
