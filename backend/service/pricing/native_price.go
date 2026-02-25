package pricing

import (
	"TgLpBot/base/config"
	"log"
	"strings"
	"sync"
	"time"
)

type cachedNativePrice struct {
	price   float64
	expires time.Time
}

var nativePriceCache = struct {
	mu    sync.RWMutex
	cache map[string]cachedNativePrice
}{
	cache: make(map[string]cachedNativePrice),
}

var nativePriceTokenSvc = NewTokenPriceService()

// GetNativePriceUSD returns the native gas token price in USD (best effort).
//
// - bsc => BNB price
// - base => ETH price
//
// It prefers chain-scoped WrappedNative token prices via GeckoTerminal, and falls back
// to reasonable constants when unavailable.
func GetNativePriceUSD(chain string) float64 {
	chain = config.NormalizeChain(chain)
	now := time.Now()

	nativePriceCache.mu.RLock()
	if c, ok := nativePriceCache.cache[chain]; ok && c.price > 0 && c.expires.After(now) {
		v := c.price
		nativePriceCache.mu.RUnlock()
		return v
	}
	nativePriceCache.mu.RUnlock()

	price := 0.0

	// Legacy fast-path for BSC (on-chain pool read, cached internally).
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

	ttl := 20 * time.Second
	nativePriceCache.mu.Lock()
	nativePriceCache.cache[chain] = cachedNativePrice{price: price, expires: now.Add(ttl)}
	nativePriceCache.mu.Unlock()

	return price
}
