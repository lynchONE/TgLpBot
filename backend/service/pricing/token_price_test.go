package pricing

import (
	"io"
	"net/http"
	"strings"
	"testing"
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

func TestGetUSDPrices_ReturnsErrorWithoutStalePriceOnRateLimit(t *testing.T) {
	svc := NewTokenPriceService()
	token := "0x1111111111111111111111111111111111111111"
	called := 0
	svc.client = &http.Client{Transport: stubRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
		called++
		if called == 1 {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"data":{"attributes":{"token_prices":{"0x1111111111111111111111111111111111111111":"2.5"}}}}`)),
				Header:     make(http.Header),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader(`{"status":{"error_code":429}}`)),
			Header:     make(http.Header),
		}, nil
	}}}

	first, err := svc.GetUSDPrices("bsc", []string{token})
	if err != nil {
		t.Fatalf("expected first request to succeed, got %v", err)
	}
	if first[token] != 2.5 {
		t.Fatalf("expected first price=2.5, got %+v", first)
	}

	prices, err := svc.GetUSDPrices("bsc", []string{token})
	if err == nil {
		t.Fatalf("expected rate-limit error, got nil")
	}
	if _, ok := prices[token]; ok {
		t.Fatalf("expected no stale price on rate limit, got %+v", prices)
	}
}

func TestGetUSDPrices_DoesNotCacheSuccessfulPrice(t *testing.T) {
	svc := NewTokenPriceService()
	token := "0x1111111111111111111111111111111111111111"

	called := 0
	svc.client = &http.Client{Transport: stubRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
		called++
		price := "1.23"
		if called == 2 {
			price = "4.56"
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"attributes":{"token_prices":{"0x1111111111111111111111111111111111111111":"` + price + `"}}}}`)),
			Header:     make(http.Header),
		}, nil
	}}}

	first, err := svc.GetUSDPrices("bsc", []string{token})
	if err != nil {
		t.Fatalf("expected first request to succeed, got %v", err)
	}
	second, err := svc.GetUSDPrices("bsc", []string{token})
	if err != nil {
		t.Fatalf("expected second request to succeed, got %v", err)
	}
	if first[token] != 1.23 || second[token] != 4.56 {
		t.Fatalf("expected fresh prices from both requests, first=%+v second=%+v", first, second)
	}
	if called != 2 {
		t.Fatalf("expected no successful-price cache, called=%d", called)
	}
}

func TestGetUSDPrices_DoesNotCacheMissingPrice(t *testing.T) {
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
	if _, ok := first[token]; ok {
		t.Fatalf("expected first response to omit missing price, got %+v", first)
	}
	if _, ok := second[token]; ok {
		t.Fatalf("expected second response to omit missing price, got %+v", second)
	}
	if called != 2 {
		t.Fatalf("expected no missing-price cache, called=%d", called)
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

func TestFetchPrices_UsesGeckoBatchEndpoint(t *testing.T) {
	tokens := []string{
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
	}
	calls := 0
	svc := NewTokenPriceService()
	svc.client = &http.Client{Transport: stubRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != http.MethodGet {
			t.Fatalf("expected GET request, got %s", req.Method)
		}
		if !strings.Contains(req.URL.String(), "/api/v2/simple/networks/bsc/token_price/") {
			t.Fatalf("unexpected request url: %s", req.URL.String())
		}
		for _, token := range tokens {
			if !strings.Contains(req.URL.Path, token) {
				t.Fatalf("expected token %s in path: %s", token, req.URL.Path)
			}
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{"data":{"attributes":{"token_prices":{` +
				`"0x1111111111111111111111111111111111111111":"1.11",` +
				`"0x2222222222222222222222222222222222222222":"2.22"` +
				`}}}}`)),
			Header: make(http.Header),
		}, nil
	}}}

	prices, err := svc.fetchPrices("bsc", tokens)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one Gecko request, got %d", calls)
	}
	if prices[tokens[0]] != 1.11 || prices[tokens[1]] != 2.22 {
		t.Fatalf("unexpected prices: %+v", prices)
	}
}
