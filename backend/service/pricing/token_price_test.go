package pricing

import (
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
	svc.cache[token] = cachedTokenPrice{
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
