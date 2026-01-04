package auto_lp

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

const defaultPoolMBaseURL = "https://mapi.poolm.xyz"

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

func NewPoolMClient(baseURL string) *PoolMClient {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultPoolMBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &PoolMClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
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
	Raw               json.RawMessage `json:"-"`
}

type PoolMFeePool struct {
	Chain           string `json:"chain"`
	ProtocolVersion string `json:"protocol_version"`
	Dex             string `json:"dex"`

	// For V3: pool address (0x...20 bytes). For V4: PoolId (0x...32 bytes).
	PoolAddress string `json:"pool_address"`

	FactoryName    string `json:"factory_name"`
	FactoryAddress string `json:"factory_address"`
	TradingPair    string `json:"trading_pair"`

	Token0Symbol   string `json:"token0_symbol"`
	Token1Symbol   string `json:"token1_symbol"`
	Token0Name     string `json:"token0_name"`
	Token1Name     string `json:"token1_name"`
	Token0Address  string `json:"token0_address"`
	Token1Address  string `json:"token1_address"`
	Token0Decimals int    `json:"token0_decimals"`
	Token1Decimals int    `json:"token1_decimals"`

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

	// Required headers to avoid 403 from PoolM.
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
			wait = wait + 500*time.Millisecond
			log.Printf("[PoolM] 触发限流(429)：第 %d/%d 次请求将等待 %s 后重试 url=%s", attempt, maxAttempts, wait.String(), req.URL.String())

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
		out.Raw = body
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
				d := time.Until(t)
				if d > 0 {
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
