package web_server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizeGeckoPoolAddressAcceptsV4PoolID(t *testing.T) {
	raw := "0x26a8e4591b7a0efcd45a577ad0d54aa64a99efaf2546ad4d5b0454c99eb70eab"
	got, ok := normalizeGeckoPoolAddress(raw)
	if !ok {
		t.Fatal("expected v4-style 64-byte pool id to be valid")
	}
	if got != raw {
		t.Fatalf("unexpected normalized pool id: %s", got)
	}
}

func TestFetchGeckoTokenCandlesSkipsUnavailablePreferredPool(t *testing.T) {
	const (
		tokenAddress     = "0x1111111111111111111111111111111111111111"
		preferredPool    = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		workingPool      = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		workingPoolLower = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)

	var ohlcvRequests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/networks/bsc/tokens/" + tokenAddress + "/pools":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": [
					{"id":"bsc_` + workingPoolLower + `","attributes":{"address":"` + workingPool + `","reserve_in_usd":"1000"}}
				]
			}`))
		case "/networks/bsc/pools/" + preferredPool + "/ohlcv/minute":
			ohlcvRequests = append(ohlcvRequests, preferredPool)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errors":[{"status":"404","title":"Not Found"}]}`))
		case "/networks/bsc/pools/" + workingPoolLower + "/ohlcv/minute":
			ohlcvRequests = append(ohlcvRequests, workingPoolLower)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": {
					"attributes": {
						"ohlcv_list": [[1700000000,1,2,0.5,1.5,123]]
					}
				}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	oldBaseURL := geckoTerminalAPIBaseURL
	geckoTerminalAPIBaseURL = server.URL
	t.Cleanup(func() {
		geckoTerminalAPIBaseURL = oldBaseURL
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	candles, usedPool, err := fetchGeckoTokenCandles(ctx, server.Client(), "bsc", tokenAddress, preferredPool, geckoOHLCVParams{
		timeframe: "minute",
		aggregate: 1,
		step:      time.Minute,
	}, 10, "")
	if err != nil {
		t.Fatalf("fetchGeckoTokenCandles returned error: %v", err)
	}
	if usedPool != workingPoolLower {
		t.Fatalf("expected working pool %s, got %s", workingPoolLower, usedPool)
	}
	if len(candles) != 1 || candles[0].T != 1700000000 || candles[0].C != 1.5 {
		t.Fatalf("unexpected candles: %+v", candles)
	}
	if len(ohlcvRequests) != 2 || ohlcvRequests[0] != preferredPool || ohlcvRequests[1] != workingPoolLower {
		t.Fatalf("unexpected ohlcv request order: %+v", ohlcvRequests)
	}
}

func TestFetchGeckoTokenCandlesUsesPreferredPoolBeforeCandidates(t *testing.T) {
	const (
		tokenAddress     = "0x1111111111111111111111111111111111111111"
		preferredPool    = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		workingPoolLower = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	requestedPools := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/networks/bsc/tokens/" + tokenAddress + "/pools":
			requestedPools = true
			w.WriteHeader(http.StatusInternalServerError)
		case "/networks/bsc/pools/" + workingPoolLower + "/ohlcv/minute":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": {
					"attributes": {
						"ohlcv_list": [[1700000000,1,2,0.5,1.5,123]]
					}
				}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	oldBaseURL := geckoTerminalAPIBaseURL
	geckoTerminalAPIBaseURL = server.URL
	t.Cleanup(func() {
		geckoTerminalAPIBaseURL = oldBaseURL
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	candles, usedPool, err := fetchGeckoTokenCandles(ctx, server.Client(), "bsc", tokenAddress, preferredPool, geckoOHLCVParams{
		timeframe: "minute",
		aggregate: 1,
		step:      time.Minute,
	}, 10, "")
	if err != nil {
		t.Fatalf("fetchGeckoTokenCandles returned error: %v", err)
	}
	if usedPool != workingPoolLower {
		t.Fatalf("expected preferred pool %s, got %s", workingPoolLower, usedPool)
	}
	if len(candles) != 1 {
		t.Fatalf("unexpected candles: %+v", candles)
	}
	if requestedPools {
		t.Fatal("did not expect token pool candidates request when preferred pool succeeds")
	}
}
