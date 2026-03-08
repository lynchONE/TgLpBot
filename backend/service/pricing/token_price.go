package pricing

import (
	"TgLpBot/base/config"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type TokenPriceService struct {
	client *http.Client
	ttl    time.Duration
	stale  time.Duration

	rateLimitCooldown time.Duration

	mu               sync.RWMutex
	cache            map[string]cachedTokenPrice
	rateLimitedUntil time.Time

	fetchGroup singleflight.Group
}

type cachedTokenPrice struct {
	priceUSD float64
	expires  time.Time
	staleTil time.Time
}

type ProviderHTTPError struct {
	Provider string
	Status   int
	Body     string
}

func (e *ProviderHTTPError) Error() string {
	if e == nil {
		return "provider http error"
	}
	provider := strings.TrimSpace(e.Provider)
	if provider == "" {
		provider = "provider"
	}
	if e.Status > 0 {
		return fmt.Sprintf("%s token_price api error: status=%d", provider, e.Status)
	}
	return fmt.Sprintf("%s token_price api error", provider)
}

func (e *ProviderHTTPError) IsRateLimit() bool {
	return e != nil && e.Status == http.StatusTooManyRequests
}

func NewTokenPriceService() *TokenPriceService {
	return &TokenPriceService{
		client:            &http.Client{Timeout: 12 * time.Second},
		ttl:               75 * time.Second,
		stale:             30 * time.Minute,
		rateLimitCooldown: 90 * time.Second,
		cache:             make(map[string]cachedTokenPrice),
	}
}

func (s *TokenPriceService) GetUSDPrices(network string, tokenAddresses []string) (map[string]float64, error) {
	network = strings.TrimSpace(strings.ToLower(network))
	if network == "" {
		network = "bsc"
	}

	addresses := normalizeTokenAddresses(tokenAddresses)
	if len(addresses) == 0 {
		return map[string]float64{}, nil
	}

	now := time.Now()
	out := make(map[string]float64, len(addresses))
	stalePrices := make(map[string]float64, len(addresses))

	missing := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		if p, ok := defaultFallbackPrice(network, addr); ok {
			out[addr] = p
			s.putCache(addr, p, now)
			continue
		}

		fresh, stale, hasFresh, hasStale := s.getCachedPrice(addr, now)
		if hasFresh {
			out[addr] = fresh
			continue
		}
		if hasStale {
			stalePrices[addr] = stale
		}
		missing = append(missing, addr)
	}

	if len(missing) == 0 {
		return out, nil
	}

	s.mu.RLock()
	cooldownUntil := s.rateLimitedUntil
	s.mu.RUnlock()
	if cooldownUntil.After(now) {
		for _, addr := range missing {
			if p, ok := stalePrices[addr]; ok {
				out[addr] = p
				continue
			}
			out[addr] = 0
		}
		return out, &ProviderHTTPError{Provider: "okx/gecko", Status: http.StatusTooManyRequests}
	}

	fetched, err := s.fetchPrices(network, missing)
	if err != nil {
		var httpErr *ProviderHTTPError
		if errors.As(err, &httpErr) && httpErr.IsRateLimit() {
			s.mu.Lock()
			s.rateLimitedUntil = time.Now().Add(s.rateLimitCooldown)
			s.mu.Unlock()
		}
		for _, addr := range missing {
			if p, ok := fetched[addr]; ok && p > 0 {
				out[addr] = p
				s.putCache(addr, p, now)
				continue
			}
			if p, ok := stalePrices[addr]; ok {
				out[addr] = p
				continue
			}
			out[addr] = 0
		}
		return out, err
	}

	for _, addr := range missing {
		if p, ok := fetched[addr]; ok && p > 0 {
			out[addr] = p
			s.putCache(addr, p, now)
			continue
		}
		if p, ok := stalePrices[addr]; ok {
			out[addr] = p
			continue
		}
		out[addr] = 0
	}

	return out, nil
}

func normalizeTokenAddresses(tokenAddresses []string) []string {
	seen := make(map[string]struct{}, len(tokenAddresses))
	out := make([]string, 0, len(tokenAddresses))
	for _, a := range tokenAddresses {
		addr := strings.ToLower(strings.TrimSpace(a))
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	return out
}

func (s *TokenPriceService) getCachedPrice(addr string, now time.Time) (fresh float64, stale float64, hasFresh bool, hasStale bool) {
	s.mu.RLock()
	c, ok := s.cache[addr]
	s.mu.RUnlock()
	if !ok || c.priceUSD <= 0 {
		return 0, 0, false, false
	}
	if c.expires.After(now) {
		return c.priceUSD, c.priceUSD, true, true
	}
	if c.staleTil.After(now) {
		return 0, c.priceUSD, false, true
	}
	return 0, 0, false, false
}

func (s *TokenPriceService) putCache(addr string, price float64, now time.Time) {
	if strings.TrimSpace(addr) == "" || price <= 0 {
		return
	}
	s.mu.Lock()
	s.cache[addr] = cachedTokenPrice{priceUSD: price, expires: now.Add(s.ttl), staleTil: now.Add(s.stale)}
	s.mu.Unlock()
}

func (s *TokenPriceService) fetchPrices(network string, tokenAddresses []string) (map[string]float64, error) {
	addresses := normalizeTokenAddresses(tokenAddresses)
	if len(addresses) == 0 {
		return map[string]float64{}, nil
	}

	sorted := append([]string(nil), addresses...)
	sort.Strings(sorted)
	key := network + "|" + strings.Join(sorted, ",")
	v, err, _ := s.fetchGroup.Do(key, func() (any, error) {
		out := make(map[string]float64, len(addresses))

		// Primary: OKX DEX market API
		if okxAvailable() {
			okxPrices, okxErr := s.fetchOKXTokenPrices(network, addresses)
			if okxErr != nil {
				log.Printf("[TokenPrice] OKX fetch failed (chain=%s): %v", network, okxErr)
			}
			for addr, price := range okxPrices {
				if price > 0 {
					out[addr] = price
				}
			}
		}

		// Fallback: GeckoTerminal for any missing tokens
		var missing []string
		for _, addr := range addresses {
			if _, ok := out[addr]; !ok {
				missing = append(missing, addr)
			}
		}
		if len(missing) == 0 {
			return out, nil
		}

		const chunkSize = 25
		var geckoErr error
		for start := 0; start < len(missing); start += chunkSize {
			end := start + chunkSize
			if end > len(missing) {
				end = len(missing)
			}
			batch := missing[start:end]
			part, ferr := s.fetchGeckoTokenPrices(network, batch)
			if ferr != nil {
				geckoErr = ferr
			}
			for addr, price := range part {
				if price > 0 {
					out[addr] = price
				}
			}
		}
		return out, geckoErr
	})
	if err != nil {
		if partial, ok := v.(map[string]float64); ok {
			return partial, err
		}
		return map[string]float64{}, err
	}
	data, ok := v.(map[string]float64)
	if !ok || data == nil {
		return map[string]float64{}, nil
	}
	out := make(map[string]float64, len(data))
	for addr, p := range data {
		out[addr] = p
	}
	return out, nil
}

var defaultBSCStableAddresses = map[string]float64{
	"0x55d398326f99059ff775485246999027b3197955": 1,
	"0x8ac76a51cc950d9822d68b83fe1ad97b32cd580d": 1,
	"0xe9e7cea3dedca5984780bafc599bd69add087d56": 1,
	"0x1af3f329e8be154074d8769d1ffabf0a3ef00b1d": 1,
}

func defaultFallbackPrice(network string, addr string) (float64, bool) {
	network = strings.ToLower(strings.TrimSpace(network))
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" {
		return 0, false
	}

	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(network); ok {
			stables := []string{
				cc.StableAddress,
				cc.USDCAddress,
				cc.BUSDAddress,
			}
			for _, stable := range stables {
				stable = strings.ToLower(strings.TrimSpace(stable))
				if stable == "" {
					continue
				}
				if addr == stable {
					return 1, true
				}
			}

			wrapped := strings.ToLower(strings.TrimSpace(cc.WrappedNativeAddress))
			if wrapped != "" && addr == wrapped {
				// Avoid recursive calls into token_price via GetNativePriceUSD; keep static fallbacks here.
				switch network {
				case "bsc":
					if p := GetBNBPriceUSDT(); p > 0 {
						return p, true
					}
				case "base":
					return 2500, true
				default:
					return 1000, true
				}
			}
		} else if network == "bsc" {
			// Backward-compatible fallback for legacy single-chain config values.
			usdt := strings.ToLower(strings.TrimSpace(config.AppConfig.USDTAddress))
			usdc := strings.ToLower(strings.TrimSpace(config.AppConfig.USDCAddress))
			busd := strings.ToLower(strings.TrimSpace(config.AppConfig.BUSDAddress))
			if addr == usdt || addr == usdc || addr == busd {
				return 1, true
			}
			wbnb := strings.ToLower(strings.TrimSpace(config.AppConfig.WBNBAddress))
			if wbnb != "" && addr == wbnb {
				if p := GetBNBPriceUSDT(); p > 0 {
					return p, true
				}
			}
		}
	}

	if network == "bsc" {
		if p, ok := defaultBSCStableAddresses[addr]; ok {
			return p, true
		}
		if addr == "0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c" {
			if p := GetBNBPriceUSDT(); p > 0 {
				return p, true
			}
		}
	}
	return 0, false
}

type geckoTokenPriceResponse struct {
	Data struct {
		Attributes struct {
			TokenPrices map[string]string `json:"token_prices"`
		} `json:"attributes"`
	} `json:"data"`
}

func (s *TokenPriceService) fetchGeckoTokenPrices(network string, tokenAddresses []string) (map[string]float64, error) {
	if len(tokenAddresses) == 0 {
		return map[string]float64{}, nil
	}
	tokenAddresses = normalizeTokenAddresses(tokenAddresses)
	if len(tokenAddresses) == 0 {
		return map[string]float64{}, nil
	}

	joined := strings.Join(tokenAddresses, ",")
	escaped := url.PathEscape(joined)
	u := fmt.Sprintf("https://api.geckoterminal.com/api/v2/simple/networks/%s/token_price/%s", network, escaped)

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		bodyText := strings.TrimSpace(string(body))
		if len(bodyText) > 320 {
			bodyText = bodyText[:320]
		}
		return nil, &ProviderHTTPError{Provider: "geckoterminal", Status: resp.StatusCode, Body: bodyText}
	}

	var parsed geckoTokenPriceResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	out := make(map[string]float64, len(tokenAddresses))
	for addr, raw := range parsed.Data.Attributes.TokenPrices {
		addr = strings.ToLower(strings.TrimSpace(addr))
		if addr == "" {
			continue
		}
		if raw == "" {
			continue
		}
		// Gecko may return numeric-as-string.
		f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			continue
		}
		out[addr] = f
	}

	return out, nil
}

// ── OKX DEX Market current-price ──

func okxAvailable() bool {
	return config.AppConfig != nil &&
		strings.TrimSpace(config.AppConfig.OKXAPIKey) != "" &&
		strings.TrimSpace(config.AppConfig.OKXSecretKey) != ""
}

func okxMarketBaseURL() string {
	base := strings.TrimSpace(config.AppConfig.OKXDexAPIURL)
	if base == "" {
		return "https://web3.okx.com/api/v6/dex/market"
	}
	base = strings.TrimRight(base, "/")
	replacer := strings.NewReplacer(
		"/api/v6/dex/aggregator", "/api/v6/dex/market",
		"/api/v5/dex/aggregator", "/api/v5/dex/market",
	)
	next := replacer.Replace(base)
	if next != base {
		return next
	}
	return "https://web3.okx.com/api/v6/dex/market"
}

func okxIsV6() bool {
	base := strings.TrimSpace(config.AppConfig.OKXDexAPIURL)
	return base == "" || strings.Contains(base, "/v6/")
}

func okxChainQueryKey() string {
	if okxIsV6() {
		return "chainIndex"
	}
	return "chainId"
}

func okxSignRequest(req *http.Request) {
	apiKey := strings.TrimSpace(config.AppConfig.OKXAPIKey)
	secretKey := strings.TrimSpace(config.AppConfig.OKXSecretKey)
	passphrase := strings.TrimSpace(config.AppConfig.OKXPassphrase)

	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	message := timestamp + req.Method + req.URL.RequestURI()
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req.Header.Set("OK-ACCESS-KEY", apiKey)
	req.Header.Set("OK-ACCESS-SIGN", signature)
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", passphrase)
	req.Header.Set("Content-Type", "application/json")
}

type okxCurrentPriceResponse struct {
	Code string `json:"code"`
	Data []struct {
		Price string `json:"price"`
	} `json:"data"`
}

func (s *TokenPriceService) fetchOKXTokenPrices(network string, tokenAddresses []string) (map[string]float64, error) {
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	cc, ok := config.AppConfig.GetChainConfig(network)
	if !ok || cc.ChainID == 0 {
		return nil, fmt.Errorf("unsupported chain for OKX: %s", network)
	}
	chainIndex := strconv.FormatInt(cc.ChainID, 10)
	baseURL := okxMarketBaseURL()
	chainKey := okxChainQueryKey()

	out := make(map[string]float64, len(tokenAddresses))
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	for _, addr := range tokenAddresses {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			price, err := s.fetchOKXSinglePrice(baseURL, chainKey, chainIndex, addr)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			if price > 0 {
				mu.Lock()
				out[addr] = price
				mu.Unlock()
			}
		}(addr)
	}
	wg.Wait()

	return out, firstErr
}

func (s *TokenPriceService) fetchOKXSinglePrice(baseURL, chainKey, chainIndex, tokenAddr string) (float64, error) {
	query := url.Values{}
	query.Set(chainKey, chainIndex)
	query.Set("tokenContractAddress", tokenAddr)
	endpoint := fmt.Sprintf("%s/current-price?%s", baseURL, query.Encode())

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	okxSignRequest(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return 0, &ProviderHTTPError{Provider: "okx", Status: resp.StatusCode}
	}

	var parsed okxCurrentPriceResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, err
	}
	if parsed.Code != "0" {
		return 0, fmt.Errorf("okx current-price code=%s", parsed.Code)
	}
	if len(parsed.Data) == 0 || strings.TrimSpace(parsed.Data[0].Price) == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(parsed.Data[0].Price), 64)
	if err != nil {
		return 0, nil
	}
	return f, nil
}
