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
		case "/networks/bsc/pools/" + preferredPool:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errors":[{"status":"404","title":"Not Found"}]}`))
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
	candles, usedPool, usedToken, err := fetchGeckoTokenCandles(ctx, server.Client(), "bsc", tokenAddress, preferredPool, geckoOHLCVParams{
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
	if usedToken != tokenAddress {
		t.Fatalf("expected used token %s, got %s", tokenAddress, usedToken)
	}
	if len(candles) != 1 || candles[0].T != 1700000000 || candles[0].C != 1.5 {
		t.Fatalf("unexpected candles: %+v", candles)
	}
	if len(ohlcvRequests) != 2 || ohlcvRequests[0] != preferredPool || ohlcvRequests[1] != workingPoolLower {
		t.Fatalf("unexpected ohlcv request order: %+v", ohlcvRequests)
	}
}

func TestFetchGeckoTokenCandlesSkipsEmptyPreferredPool(t *testing.T) {
	const (
		tokenAddress     = "0x1111111111111111111111111111111111111111"
		preferredPool    = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		emptyPool        = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		workingPool      = "0xcccccccccccccccccccccccccccccccccccccccc"
		workingPoolLower = "0xcccccccccccccccccccccccccccccccccccccccc"
	)

	var ohlcvRequests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/networks/bsc/tokens/" + tokenAddress + "/pools":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": [
					{"id":"bsc_` + emptyPool + `","attributes":{"address":"` + emptyPool + `","reserve_in_usd":"2000"}},
					{"id":"bsc_` + workingPoolLower + `","attributes":{"address":"` + workingPool + `","reserve_in_usd":"1000"}}
				]
			}`))
		case "/networks/bsc/pools/" + preferredPool:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errors":[{"status":"404","title":"Not Found"}]}`))
		case "/networks/bsc/pools/" + preferredPool + "/ohlcv/minute":
			ohlcvRequests = append(ohlcvRequests, preferredPool)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"attributes":{"ohlcv_list":[]}}}`))
		case "/networks/bsc/pools/" + emptyPool + "/ohlcv/minute":
			ohlcvRequests = append(ohlcvRequests, emptyPool)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"attributes":{"ohlcv_list":[]}}}`))
		case "/networks/bsc/pools/" + workingPoolLower + "/ohlcv/minute":
			ohlcvRequests = append(ohlcvRequests, workingPoolLower)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": {
					"attributes": {
						"ohlcv_list": [[1700000060,2,3,1.5,2.5,456]]
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
	candles, usedPool, _, err := fetchGeckoTokenCandles(ctx, server.Client(), "bsc", tokenAddress, preferredPool, geckoOHLCVParams{
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
	if len(candles) != 1 || candles[0].T != 1700000060 || candles[0].C != 2.5 {
		t.Fatalf("unexpected candles: %+v", candles)
	}
	wantRequests := []string{preferredPool, emptyPool, workingPoolLower}
	if len(ohlcvRequests) != len(wantRequests) {
		t.Fatalf("unexpected ohlcv requests: %+v", ohlcvRequests)
	}
	for i := range wantRequests {
		if ohlcvRequests[i] != wantRequests[i] {
			t.Fatalf("unexpected ohlcv request order: %+v", ohlcvRequests)
		}
	}
}

func TestFetchGeckoTokenCandlesUsesNonQuoteTokenAndPreferredDexCandidates(t *testing.T) {
	const (
		requestedToken = "0x8ac76a51cc950d9822d68b83fe1ad97b32cd580d"
		mkrToken       = "0x5f0da599bb2cccfcf6fdfd7d81743b6020864350"
		v4PoolID       = "0xd9e0f045d0dcaef3400639e62543259cdea1e6e87d8faff23e20a5a2fe48d707"
		v2Pool         = "0xd446a0fac0abd96797efd1b7fa2243223ee5edc6"
		v4Pool         = "0xce38c279cdfedfcbd0fdc25ce8d2a8aa2aa73afd754853a8e3d9b79f60105540"
		usdcToken      = "0x8ac76a51cc950d9822d68b83fe1ad97b32cd580d"
		wbnbToken      = "0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c"
	)

	var tokenPoolsPath string
	var ohlcvRequests []string
	var ohlcvTokens []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/networks/bsc/pools/" + v4PoolID:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": {
					"id":"bsc_` + v4PoolID + `",
					"attributes":{"address":"` + v4PoolID + `"},
					"relationships": {
						"base_token":{"data":{"id":"bsc_` + mkrToken + `"}},
						"quote_token":{"data":{"id":"bsc_` + usdcToken + `"}},
						"dex":{"data":{"id":"uniswap-v4-bsc"}}
					}
				}
			}`))
		case "/networks/bsc/tokens/" + mkrToken + "/pools":
			tokenPoolsPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": [
					{
						"id":"bsc_` + v2Pool + `",
						"attributes":{"address":"` + v2Pool + `","reserve_in_usd":"6000"},
						"relationships": {
							"base_token":{"data":{"id":"bsc_` + mkrToken + `"}},
							"quote_token":{"data":{"id":"bsc_` + wbnbToken + `"}},
							"dex":{"data":{"id":"pancakeswap_v2"}}
						}
					},
					{
						"id":"bsc_` + v4Pool + `",
						"attributes":{"address":"` + v4Pool + `","reserve_in_usd":"800"},
						"relationships": {
							"base_token":{"data":{"id":"bsc_` + mkrToken + `"}},
							"quote_token":{"data":{"id":"bsc_` + usdcToken + `"}},
							"dex":{"data":{"id":"uniswap-v4-bsc"}}
						}
					}
				]
			}`))
		case "/networks/bsc/pools/" + v4PoolID + "/ohlcv/minute":
			ohlcvRequests = append(ohlcvRequests, v4PoolID)
			ohlcvTokens = append(ohlcvTokens, r.URL.Query().Get("token"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"attributes":{"ohlcv_list":[]}}}`))
		case "/networks/bsc/pools/" + v4Pool + "/ohlcv/minute":
			ohlcvRequests = append(ohlcvRequests, v4Pool)
			ohlcvTokens = append(ohlcvTokens, r.URL.Query().Get("token"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": {
					"attributes": {
						"ohlcv_list": [[1700000120,10,12,9,11,789]]
					}
				}
			}`))
		case "/networks/bsc/pools/" + v2Pool + "/ohlcv/minute":
			t.Fatal("expected v4 candidate to be tried before higher-reserve v2 pool")
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
	candles, usedPool, usedToken, err := fetchGeckoTokenCandles(ctx, server.Client(), "bsc", requestedToken, v4PoolID, geckoOHLCVParams{
		timeframe: "minute",
		aggregate: 1,
		step:      time.Minute,
	}, 10, "")
	if err != nil {
		t.Fatalf("fetchGeckoTokenCandles returned error: %v", err)
	}
	if tokenPoolsPath != "/networks/bsc/tokens/"+mkrToken+"/pools" {
		t.Fatalf("expected token pools request for MKR, got %s", tokenPoolsPath)
	}
	if usedToken != mkrToken {
		t.Fatalf("expected used token %s, got %s", mkrToken, usedToken)
	}
	if usedPool != v4Pool {
		t.Fatalf("expected v4 candidate pool %s, got %s", v4Pool, usedPool)
	}
	if len(candles) != 1 || candles[0].T != 1700000120 || candles[0].C != 11 {
		t.Fatalf("unexpected candles: %+v", candles)
	}
	wantRequests := []string{v4PoolID, v4Pool}
	if len(ohlcvRequests) != len(wantRequests) {
		t.Fatalf("unexpected ohlcv requests: %+v", ohlcvRequests)
	}
	for i := range wantRequests {
		if ohlcvRequests[i] != wantRequests[i] {
			t.Fatalf("unexpected ohlcv request order: %+v", ohlcvRequests)
		}
		if ohlcvTokens[i] != mkrToken {
			t.Fatalf("expected ohlcv token %s, got requests=%+v tokens=%+v", mkrToken, ohlcvRequests, ohlcvTokens)
		}
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
		case "/networks/bsc/pools/" + preferredPool:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errors":[{"status":"404","title":"Not Found"}]}`))
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
	candles, usedPool, _, err := fetchGeckoTokenCandles(ctx, server.Client(), "bsc", tokenAddress, preferredPool, geckoOHLCVParams{
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
