package pool_sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPoolMBaseURL       = "https://mapi.poolm.xyz"
	defaultDexScreenerBaseURL = "https://api.dexscreener.com"
)

type PoolMClient struct {
	baseURL    string
	httpClient *http.Client
}

type PoolMStringList []string

func (s *PoolMStringList) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*s = nil
		return nil
	}
	if data[0] == '[' {
		var out []string
		if err := json.Unmarshal(data, &out); err != nil {
			return err
		}
		*s = out
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		single = strings.TrimSpace(single)
		if single == "" {
			*s = nil
		} else {
			*s = []string{single}
		}
		return nil
	}
	return fmt.Errorf("invalid string list: %s", string(data))
}

type poolMRateLimitBody struct {
	Error             string `json:"error"`
	RetryAfterSeconds int    `json:"retryAfter"`
	RetryAfter        int    `json:"retry_after"`
}

type PoolMTopFeesResponse struct {
	Success           bool            `json:"success"`
	Timeframe         string          `json:"timeframe"`
	RequestedProtocol PoolMStringList `json:"requested_protocol"`
	RequestedDex      PoolMStringList `json:"requested_dex"`
	RequestedChain    string          `json:"requested_chain"`
	TotalPools        int             `json:"total_pools"`
	Data              []PoolMFeePool  `json:"data"`
	Error             string          `json:"error"`
}

type PoolMFeePool struct {
	Chain           string `json:"chain"`
	ProtocolVersion string `json:"protocol_version"`
	Dex             string `json:"dex"`
	PoolAddress     string `json:"pool_address"`
	FactoryName     string `json:"factory_name"`
	FactoryAddress  string `json:"factory_address"`
	TradingPair     string `json:"trading_pair"`
	Token0Symbol    string `json:"token0_symbol"`
	Token1Symbol    string `json:"token1_symbol"`
	Token0Name      string `json:"token0_name"`
	Token1Name      string `json:"token1_name"`
	Token0Address   string `json:"token0_address"`
	Token1Address   string `json:"token1_address"`
	Token0Decimals  int    `json:"token0_decimals"`
	Token1Decimals  int    `json:"token1_decimals"`

	StableCoinSymbol string  `json:"stable_coin_symbol"`
	FeeRate          int     `json:"fee_rate"`
	FeePercentage    float64 `json:"fee_percentage"`

	TransactionCount     int     `json:"transaction_count"`
	TotalFees            float64 `json:"total_fees"`
	TotalVolume          float64 `json:"total_volume"`
	CurrentPoolValue     float64 `json:"current_pool_value"`
	CurrentToken0Balance float64 `json:"current_token0_balance"`
	CurrentToken1Balance float64 `json:"current_token1_balance"`
	CurrentTokenPrice    float64 `json:"current_token_price"`
	PriceDisplay         string  `json:"price_display"`
	LastSwapAt           string  `json:"last_swap_at"`
}

func NewPoolMClient(baseURL string) *PoolMClient {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultPoolMBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &PoolMClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *PoolMClient) TopFees(ctx context.Context, timeframeMinutes int, chain string, dex string) (*PoolMTopFeesResponse, error) {
	if timeframeMinutes <= 0 {
		return nil, fmt.Errorf("invalid timeframeMinutes: %d", timeframeMinutes)
	}
	chain = strings.ToLower(strings.TrimSpace(chain))
	dex = strings.ToLower(strings.TrimSpace(dex))
	if chain == "" {
		return nil, fmt.Errorf("chain is required")
	}
	if dex == "" {
		return nil, fmt.Errorf("dex is required")
	}

	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + fmt.Sprintf("/api/pools/top-fees/%d", timeframeMinutes)
	q := u.Query()
	q.Set("chain", chain)
	q.Set("dex", dex)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Origin", "https://poolm.xyz")
	req.Header.Set("Referer", "https://poolm.xyz/")
	req.Header.Set("Accept", "application/json")

	maxAttempts := 4
	var lastBody []byte
	var lastStatus int

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}

		lastBody = body
		lastStatus = resp.StatusCode

		if resp.StatusCode == http.StatusTooManyRequests {
			if attempt == maxAttempts {
				return nil, fmt.Errorf("poolm http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			}
			wait := poolMRetryAfter(resp, body)
			if wait <= 0 {
				wait = 5 * time.Second
			}
			wait += 500 * time.Millisecond
			log.Printf("[PoolSync] PoolM rate limited, retry in %s url=%s", wait.String(), req.URL.String())
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("poolm http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var out PoolMTopFeesResponse
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("decode poolm response: %w", err)
		}
		if !out.Success {
			if strings.TrimSpace(out.Error) != "" {
				return &out, fmt.Errorf("poolm error: %s", strings.TrimSpace(out.Error))
			}
			return &out, fmt.Errorf("poolm error: success=false")
		}
		return &out, nil
	}

	if lastStatus != 0 {
		return nil, fmt.Errorf("poolm http %d: %s", lastStatus, strings.TrimSpace(string(lastBody)))
	}
	return nil, fmt.Errorf("poolm request failed")
}

func poolMRetryAfter(resp *http.Response, body []byte) time.Duration {
	if resp != nil {
		ra := strings.TrimSpace(resp.Header.Get("Retry-After"))
		if ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
				return time.Duration(secs) * time.Second
			}
			if t, err := http.ParseTime(ra); err == nil {
				if d := time.Until(t); d > 0 {
					return d
				}
			}
		}
	}

	var parsed poolMRateLimitBody
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.RetryAfterSeconds > 0 {
			return time.Duration(parsed.RetryAfterSeconds) * time.Second
		}
		if parsed.RetryAfter > 0 {
			return time.Duration(parsed.RetryAfter) * time.Second
		}
	}
	return 10 * time.Second
}

type DexScreenerClient struct {
	baseURL    string
	httpClient *http.Client
}

type dexScreenerToken struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	Symbol  string `json:"symbol"`
}

type dexScreenerTxnsBucket struct {
	Buys    int `json:"buys"`
	Sells   int `json:"sells"`
	Buyers  int `json:"buyers"`
	Sellers int `json:"sellers"`
}

type dexScreenerTxns struct {
	M5  dexScreenerTxnsBucket `json:"m5"`
	H1  dexScreenerTxnsBucket `json:"h1"`
	H6  dexScreenerTxnsBucket `json:"h6"`
	H24 dexScreenerTxnsBucket `json:"h24"`
}

type dexScreenerVolume struct {
	M5  float64 `json:"m5"`
	H1  float64 `json:"h1"`
	H6  float64 `json:"h6"`
	H24 float64 `json:"h24"`
}

type dexScreenerPriceChange struct {
	M5  float64 `json:"m5"`
	H1  float64 `json:"h1"`
	H6  float64 `json:"h6"`
	H24 float64 `json:"h24"`
}

type dexScreenerLiquidity struct {
	USD   float64 `json:"usd"`
	Base  float64 `json:"base"`
	Quote float64 `json:"quote"`
}

type DexScreenerPair struct {
	ChainID       string                 `json:"chainId"`
	DexID         string                 `json:"dexId"`
	PairAddress   string                 `json:"pairAddress"`
	Labels        []string               `json:"labels"`
	BaseToken     dexScreenerToken       `json:"baseToken"`
	QuoteToken    dexScreenerToken       `json:"quoteToken"`
	PriceUSD      string                 `json:"priceUsd"`
	PriceNative   string                 `json:"priceNative"`
	Txns          dexScreenerTxns        `json:"txns"`
	Volume        dexScreenerVolume      `json:"volume"`
	PriceChange   dexScreenerPriceChange `json:"priceChange"`
	Liquidity     dexScreenerLiquidity   `json:"liquidity"`
	FDV           float64                `json:"fdv"`
	MarketCap     float64                `json:"marketCap"`
	PairCreatedAt int64                  `json:"pairCreatedAt"`
}

type dexScreenerPairsResponse struct {
	Pairs []DexScreenerPair `json:"pairs"`
}

func NewDexScreenerClient(baseURL string) *DexScreenerClient {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultDexScreenerBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &DexScreenerClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *DexScreenerClient) GetPair(ctx context.Context, chain string, pairAddress string) (*DexScreenerPair, error) {
	chain = strings.ToLower(strings.TrimSpace(chain))
	pairAddress = normalizePairAddress(pairAddress)
	if chain == "" {
		return nil, fmt.Errorf("chain is required")
	}
	if pairAddress == "" {
		return nil, fmt.Errorf("pair address is required")
	}

	endpoint := fmt.Sprintf("%s/latest/dex/pairs/%s/%s", c.baseURL, url.PathEscape(chain), url.PathEscape(pairAddress))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("dexscreener http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed dexScreenerPairsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	for i := range parsed.Pairs {
		p := parsed.Pairs[i]
		if normalizePairAddress(p.PairAddress) == pairAddress {
			return &p, nil
		}
	}
	if len(parsed.Pairs) > 0 {
		return &parsed.Pairs[0], nil
	}
	return nil, nil
}

func normalizePairAddress(raw string) string {
	addr := strings.TrimSpace(raw)
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, "0x") || strings.HasPrefix(addr, "0X") {
		addr = addr[2:]
	}
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" {
		return ""
	}
	return "0x" + addr
}
