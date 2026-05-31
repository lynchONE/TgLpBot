package pricing

import (
	"TgLpBot/base/config"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type stubRoundTripper struct {
	fn func(req *http.Request) (*http.Response, error)
}

func (s stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if s.fn == nil {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"attributes":{"token_prices":{}}}}`)),
			Header:     make(http.Header),
		}, nil
	}
	return s.fn(req)
}

func TestGetUSDPrices_StableFallback_NoNetworkCall(t *testing.T) {
	svc := NewTokenPriceService()
	called := 0
	svc.client = &http.Client{Transport: stubRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
		called++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"attributes":{"token_prices":{}}}}`)),
			Header:     make(http.Header),
		}, nil
	}}}

	prices, err := svc.GetUSDPrices("bsc", []string{"0x55d398326f99059ff775485246999027b3197955"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if prices["0x55d398326f99059ff775485246999027b3197955"] != 1 {
		t.Fatalf("expected stable fallback price=1, got %+v", prices)
	}
	if called != 0 {
		t.Fatalf("expected no network call for stable fallback, called=%d", called)
	}
}

func TestGetUSDPrices_UsesStaleCacheOnRateLimit(t *testing.T) {
	svc := NewTokenPriceService()
	token := "0x1111111111111111111111111111111111111111"
	now := time.Now()
	svc.cache[tokenPriceCacheKey("bsc", token)] = cachedTokenPrice{
		priceUSD: 2.5,
		expires:  now.Add(-time.Minute),
		staleTil: now.Add(10 * time.Minute),
	}
	svc.client = &http.Client{Transport: stubRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader(`{"status":{"error_code":429}}`)),
			Header:     make(http.Header),
		}, nil
	}}}

	prices, err := svc.GetUSDPrices("bsc", []string{token})
	if err == nil {
		t.Fatalf("expected rate-limit error, got nil")
	}
	if prices[token] != 2.5 {
		t.Fatalf("expected stale cache price 2.5, got %+v", prices)
	}
}

func TestGetUSDPrices_CacheIsNetworkScoped(t *testing.T) {
	svc := NewTokenPriceService()
	token := "0x1111111111111111111111111111111111111111"
	now := time.Now()
	svc.cache[tokenPriceCacheKey("base", token)] = cachedTokenPrice{
		priceUSD: 9.9,
		expires:  now.Add(time.Minute),
		staleTil: now.Add(10 * time.Minute),
	}

	called := 0
	svc.client = &http.Client{Transport: stubRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
		called++
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader(`{"status":{"error_code":429}}`)),
			Header:     make(http.Header),
		}, nil
	}}}

	prices, err := svc.GetUSDPrices("bsc", []string{token})
	if err == nil {
		t.Fatalf("expected rate-limit error, got nil")
	}
	if prices[token] == 9.9 {
		t.Fatalf("unexpectedly reused base cache for bsc: %+v", prices)
	}
	if called == 0 {
		t.Fatalf("expected network call because bsc cache was empty")
	}
}

func TestGetUSDPrices_CachesMissingPriceBriefly(t *testing.T) {
	svc := NewTokenPriceService()
	token := "0x1111111111111111111111111111111111111111"
	called := 0
	svc.client = &http.Client{Transport: stubRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
		called++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"attributes":{"token_prices":{}}}}`)),
			Header:     make(http.Header),
		}, nil
	}}}

	first, err := svc.GetUSDPrices("bsc", []string{token})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	second, err := svc.GetUSDPrices("bsc", []string{token})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if first[token] != 0 || second[token] != 0 {
		t.Fatalf("expected missing price to remain zero, got first=%+v second=%+v", first, second)
	}
	if called != 1 {
		t.Fatalf("expected missing price cache to avoid second network call, called=%d", called)
	}
}

func TestFetchGeckoTokenPrices_ReturnsProviderHTTPError(t *testing.T) {
	svc := NewTokenPriceService()
	svc.client = &http.Client{Transport: stubRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader(`{"status":{"error_code":429,"error_message":"rate limited"}}`)),
			Header:     make(http.Header),
		}, nil
	}}}

	_, err := svc.fetchGeckoTokenPrices("bsc", []string{"0x1111111111111111111111111111111111111111"})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	httpErr, ok := err.(*ProviderHTTPError)
	if !ok {
		t.Fatalf("expected ProviderHTTPError, got %T (%v)", err, err)
	}
	if httpErr.Status != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", httpErr.Status)
	}
}

func TestFetchOKXTokenPrices_UsesMarketPricePostEndpoint(t *testing.T) {
	oldConfig := config.AppConfig
	config.AppConfig = &config.Config{
		OKXDexAPIURL:  "https://www.okx.com/api/v6/dex/aggregator",
		OKXAPIKey:     "test-key",
		OKXSecretKey:  "test-secret",
		OKXPassphrase: "test-pass",
		Chains: map[string]config.ChainConfig{
			"bsc": {Chain: "bsc", ChainID: 56},
		},
	}
	t.Cleanup(func() {
		config.AppConfig = oldConfig
	})

	svc := NewTokenPriceService()
	svc.client = &http.Client{Transport: stubRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", req.Method)
		}
		if got := req.URL.String(); got != "https://web3.okx.com/api/v6/dex/market/price" {
			t.Fatalf("unexpected request url: %s", got)
		}
		if ct := req.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("expected application/json, got %s", ct)
		}
		if req.Header.Get("OK-ACCESS-SIGN") == "" {
			t.Fatalf("expected OK-ACCESS-SIGN header")
		}

		rawBody, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		var payload []map[string]string
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			t.Fatalf("failed to parse request body: %v", err)
		}
		if len(payload) != 1 {
			t.Fatalf("expected single-item payload, got %+v", payload)
		}
		if payload[0]["chainIndex"] != "56" {
			t.Fatalf("expected chainIndex=56, got %+v", payload[0])
		}
		if payload[0]["tokenContractAddress"] != "0x1111111111111111111111111111111111111111" {
			t.Fatalf("unexpected tokenContractAddress: %+v", payload[0])
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[{"price":"1.23"}]}`)),
			Header:     make(http.Header),
		}, nil
	}}}

	prices, err := svc.fetchOKXTokenPrices("bsc", []string{"0x1111111111111111111111111111111111111111"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if prices["0x1111111111111111111111111111111111111111"] != 1.23 {
		t.Fatalf("expected price 1.23, got %+v", prices)
	}
}

func TestFetchOKXTokenPrices_BatchesMultipleTokensInOneRequest(t *testing.T) {
	oldConfig := config.AppConfig
	config.AppConfig = &config.Config{
		OKXDexAPIURL:  "https://www.okx.com/api/v6/dex/aggregator",
		OKXAPIKey:     "test-key",
		OKXSecretKey:  "test-secret",
		OKXPassphrase: "test-pass",
		Chains: map[string]config.ChainConfig{
			"bsc": {Chain: "bsc", ChainID: 56},
		},
	}
	t.Cleanup(func() {
		config.AppConfig = oldConfig
	})

	tokens := []string{
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
	}
	calls := 0
	svc := NewTokenPriceService()
	svc.client = &http.Client{Transport: stubRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
		calls++
		rawBody, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		var payload []map[string]string
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			t.Fatalf("failed to parse request body: %v", err)
		}
		if len(payload) != len(tokens) {
			t.Fatalf("expected batched payload len=%d, got %+v", len(tokens), payload)
		}
		for i, token := range tokens {
			if payload[i]["chainIndex"] != "56" {
				t.Fatalf("expected chainIndex=56, got %+v", payload[i])
			}
			if payload[i]["tokenContractAddress"] != token {
				t.Fatalf("unexpected tokenContractAddress: %+v", payload[i])
			}
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{"code":"0","data":[` +
				`{"tokenContractAddress":"0x1111111111111111111111111111111111111111","price":"1.11"},` +
				`{"tokenContractAddress":"0x2222222222222222222222222222222222222222","price":"2.22"}` +
				`]}`)),
			Header: make(http.Header),
		}, nil
	}}}

	prices, err := svc.fetchOKXTokenPrices("bsc", tokens)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one OKX request, got %d", calls)
	}
	if prices[tokens[0]] != 1.11 || prices[tokens[1]] != 2.22 {
		t.Fatalf("unexpected prices: %+v", prices)
	}
}
