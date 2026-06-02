package pricing

import (
	"TgLpBot/base/config"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	rateLimitCooldown time.Duration

	mu               sync.RWMutex
	rateLimitedUntil time.Time

	fetchGroup singleflight.Group
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
		rateLimitCooldown: 90 * time.Second,
	}
}

var (
	defaultTokenPriceService     *TokenPriceService
	defaultTokenPriceServiceOnce sync.Once
)

func DefaultTokenPriceService() *TokenPriceService {
	defaultTokenPriceServiceOnce.Do(func() {
		defaultTokenPriceService = NewTokenPriceService()
	})
	return defaultTokenPriceService
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

	missing := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		if p, ok := defaultFallbackPrice(network, addr); ok {
			out[addr] = p
			continue
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
		return out, &ProviderHTTPError{Provider: "geckoterminal", Status: http.StatusTooManyRequests}
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
			}
		}
		return out, err
	}

	for _, addr := range missing {
		if p, ok := fetched[addr]; ok && p > 0 {
			out[addr] = p
			continue
		}
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

		const chunkSize = 25
		var geckoErr error
		for start := 0; start < len(addresses); start += chunkSize {
			end := start + chunkSize
			if end > len(addresses) {
				end = len(addresses)
			}
			batch := addresses[start:end]
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

		} else if network == "bsc" {
			// Backward-compatible fallback for legacy single-chain config values.
			usdt := strings.ToLower(strings.TrimSpace(config.AppConfig.USDTAddress))
			usdc := strings.ToLower(strings.TrimSpace(config.AppConfig.USDCAddress))
			busd := strings.ToLower(strings.TrimSpace(config.AppConfig.BUSDAddress))
			if addr == usdt || addr == usdc || addr == busd {
				return 1, true
			}
		}
	}

	if network == "bsc" {
		if p, ok := defaultBSCStableAddresses[addr]; ok {
			return p, true
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
