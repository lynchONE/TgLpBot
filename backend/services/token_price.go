package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type TokenPriceService struct {
	client *http.Client
	ttl    time.Duration

	mu    sync.RWMutex
	cache map[string]cachedTokenPrice
}

type cachedTokenPrice struct {
	priceUSD float64
	expires  time.Time
}

func NewTokenPriceService() *TokenPriceService {
	return &TokenPriceService{
		client: &http.Client{Timeout: 12 * time.Second},
		ttl:    30 * time.Second,
		cache:  make(map[string]cachedTokenPrice),
	}
}

func (s *TokenPriceService) GetUSDPrices(network string, tokenAddresses []string) (map[string]float64, error) {
	network = strings.TrimSpace(strings.ToLower(network))
	if network == "" {
		network = "bsc"
	}

	now := time.Now()
	out := make(map[string]float64, len(tokenAddresses))

	missing := make([]string, 0, len(tokenAddresses))
	seen := make(map[string]struct{}, len(tokenAddresses))
	for _, a := range tokenAddresses {
		addr := strings.ToLower(strings.TrimSpace(a))
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}

		s.mu.RLock()
		c, ok := s.cache[addr]
		s.mu.RUnlock()
		if ok && c.expires.After(now) {
			out[addr] = c.priceUSD
			continue
		}
		missing = append(missing, addr)
	}

	if len(missing) == 0 {
		return out, nil
	}

	fetched, err := s.fetchGeckoTokenPrices(network, missing)
	if err != nil {
		return out, err
	}

	s.mu.Lock()
	for addr, p := range fetched {
		s.cache[addr] = cachedTokenPrice{priceUSD: p, expires: now.Add(s.ttl)}
		out[addr] = p
	}
	s.mu.Unlock()

	return out, nil
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
		return nil, fmt.Errorf("geckoterminal token_price api error: status=%d body=%s", resp.StatusCode, string(body))
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

	// Missing prices are treated as 0.
	for _, a := range tokenAddresses {
		addr := strings.ToLower(strings.TrimSpace(a))
		if _, ok := out[addr]; !ok {
			out[addr] = 0
		}
	}

	return out, nil
}
