package pricing

import (
	"TgLpBot/base/config"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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

	mu                          sync.RWMutex
	rateLimitedUntil            time.Time
	dexScreenerRateLimitedUntil time.Time

	fetchGroup singleflight.Group
}

const (
	tokenPriceProviderGecko       = "geckoterminal"
	tokenPriceProviderDexScreener = "dexscreener"
)

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

	fetched, err := s.fetchPrices(network, missing)
	if err != nil {
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

		geckoErr := s.fetchProviderBatches(
			tokenPriceProviderGecko,
			25,
			network,
			addresses,
			out,
			s.fetchGeckoTokenPrices,
		)
		missing := missingTokenPrices(addresses, out)
		dexErr := s.fetchProviderBatches(
			tokenPriceProviderDexScreener,
			dexScreenerBatchSize,
			network,
			missing,
			out,
			s.fetchDexScreenerTokenPrices,
		)
		missing = missingTokenPrices(addresses, out)
		if len(missing) == 0 {
			return out, nil
		}
		return out, errors.Join(geckoErr, dexErr)
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

func (s *TokenPriceService) fetchProviderBatches(
	provider string,
	chunkSize int,
	network string,
	addresses []string,
	out map[string]float64,
	fetch func(string, []string) (map[string]float64, error),
) error {
	if len(addresses) == 0 {
		return nil
	}
	if s.isProviderRateLimited(provider) {
		return &ProviderHTTPError{Provider: provider, Status: http.StatusTooManyRequests}
	}
	if chunkSize <= 0 {
		return fmt.Errorf("%s token_price batch size is invalid", provider)
	}

	var providerErr error
	for start := 0; start < len(addresses); start += chunkSize {
		end := start + chunkSize
		if end > len(addresses) {
			end = len(addresses)
		}
		batch := addresses[start:end]
		part, err := fetch(network, batch)
		if err != nil {
			providerErr = errors.Join(providerErr, err)
			if isProviderRateLimitError(err, provider) {
				s.markProviderRateLimited(provider)
				break
			}
			continue
		}
		for addr, price := range part {
			if isUsablePrice(price) {
				out[addr] = price
			}
		}
	}
	return providerErr
}

func missingTokenPrices(addresses []string, prices map[string]float64) []string {
	missing := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		if p, ok := prices[addr]; ok && isUsablePrice(p) {
			continue
		}
		missing = append(missing, addr)
	}
	return missing
}

func isUsablePrice(price float64) bool {
	return price > 0 && !math.IsNaN(price) && !math.IsInf(price, 0)
}

func isProviderRateLimitError(err error, provider string) bool {
	if err == nil {
		return false
	}
	var httpErr *ProviderHTTPError
	if !errors.As(err, &httpErr) || !httpErr.IsRateLimit() {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(httpErr.Provider), provider)
}

func (s *TokenPriceService) isProviderRateLimited(provider string) bool {
	if s == nil {
		return false
	}
	now := time.Now()
	s.mu.RLock()
	defer s.mu.RUnlock()
	switch provider {
	case tokenPriceProviderGecko:
		return s.rateLimitedUntil.After(now)
	case tokenPriceProviderDexScreener:
		return s.dexScreenerRateLimitedUntil.After(now)
	default:
		return false
	}
}

func (s *TokenPriceService) markProviderRateLimited(provider string) {
	if s == nil {
		return
	}
	until := time.Now().Add(s.rateLimitCooldown)
	s.mu.Lock()
	defer s.mu.Unlock()
	switch provider {
	case tokenPriceProviderGecko:
		s.rateLimitedUntil = until
	case tokenPriceProviderDexScreener:
		s.dexScreenerRateLimitedUntil = until
	}
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

type dexScreenerTokenPair struct {
	ChainID   string `json:"chainId"`
	BaseToken struct {
		Address string `json:"address"`
	} `json:"baseToken"`
	PriceUSD  string `json:"priceUsd"`
	Liquidity struct {
		USD float64 `json:"usd"`
	} `json:"liquidity"`
}

const dexScreenerBatchSize = 30

func (s *TokenPriceService) fetchDexScreenerTokenPrices(network string, tokenAddresses []string) (map[string]float64, error) {
	tokenAddresses = normalizeTokenAddresses(tokenAddresses)
	if len(tokenAddresses) == 0 {
		return map[string]float64{}, nil
	}
	chainID := dexScreenerChainID(network)
	if chainID == "" {
		return map[string]float64{}, fmt.Errorf("unsupported dexscreener chain: %s", network)
	}

	lookup := make(map[string]struct{}, len(tokenAddresses))
	for _, addr := range tokenAddresses {
		lookup[addr] = struct{}{}
	}

	endpoint := fmt.Sprintf(
		"https://api.dexscreener.com/tokens/v1/%s/%s",
		url.PathEscape(chainID),
		url.PathEscape(strings.Join(tokenAddresses, ",")),
	)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyText := strings.TrimSpace(string(body))
		if len(bodyText) > 320 {
			bodyText = bodyText[:320]
		}
		return nil, &ProviderHTTPError{Provider: tokenPriceProviderDexScreener, Status: resp.StatusCode, Body: bodyText}
	}

	var pairs []dexScreenerTokenPair
	if err := json.Unmarshal(body, &pairs); err != nil {
		return nil, err
	}

	out := make(map[string]float64, len(tokenAddresses))
	bestLiquidity := make(map[string]float64, len(tokenAddresses))
	for _, pair := range pairs {
		if !strings.EqualFold(strings.TrimSpace(pair.ChainID), chainID) {
			continue
		}
		addr := strings.ToLower(strings.TrimSpace(pair.BaseToken.Address))
		if _, ok := lookup[addr]; !ok {
			continue
		}
		price, err := strconv.ParseFloat(strings.TrimSpace(pair.PriceUSD), 64)
		if err != nil || !isUsablePrice(price) {
			continue
		}
		if existing, ok := bestLiquidity[addr]; ok && existing > pair.Liquidity.USD {
			continue
		}
		bestLiquidity[addr] = pair.Liquidity.USD
		out[addr] = price
	}
	return out, nil
}

func dexScreenerChainID(chain string) string {
	switch strings.ToLower(strings.TrimSpace(chain)) {
	case "", "bsc", "bnb":
		return "bsc"
	case "base":
		return "base"
	case "eth", "ethereum":
		return "ethereum"
	default:
		return strings.ToLower(strings.TrimSpace(chain))
	}
}
