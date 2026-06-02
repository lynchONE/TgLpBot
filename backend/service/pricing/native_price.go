package pricing

import (
	"TgLpBot/base/config"
	"log"
	"strings"
)

var nativePriceTokenSvc = DefaultTokenPriceService()

// GetNativePriceUSD returns the native gas token price in USD (best effort).
//
// - bsc => BNB price
// - base => ETH price
//
// It prefers chain-scoped WrappedNative token prices via GeckoTerminal, and falls back
// to reasonable constants when unavailable.
func GetNativePriceUSD(chain string) float64 {
	chain = config.NormalizeChain(chain)

	price := 0.0

	// Legacy fast-path for BSC (on-chain pool read).
	if chain == "bsc" {
		if p := GetBNBPriceUSDT(); p > 0 {
			price = p
		}
	}

	// General path: GeckoTerminal token_price for WrappedNative.
	if price <= 0 && config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
			addr := strings.ToLower(strings.TrimSpace(cc.WrappedNativeAddress))
			if addr != "" && nativePriceTokenSvc != nil {
				prices, err := nativePriceTokenSvc.GetUSDPrices(chain, []string{addr})
				if err != nil {
					log.Printf("[Pricing] Warning: fetch native price via gecko failed: chain=%s err=%v", chain, err)
				}
				if p := prices[addr]; p > 0 {
					price = p
				}
			}
		}
	}

	// Best-effort static fallbacks (avoid 0 which breaks PnL/gas accounting).
	if price <= 0 {
		switch chain {
		case "base":
			price = 2500.0
		case "bsc":
			price = 700.0
		default:
			price = 1000.0
		}
	}

	return price
}
