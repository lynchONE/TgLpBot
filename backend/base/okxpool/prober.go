package okxpool

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPProber struct {
	Timeout time.Duration
	Client  *http.Client
}

func (p *HTTPProber) Probe(ctx context.Context, cfg EffectiveConfig) (time.Duration, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return 0, fmt.Errorf("okx base_url is empty")
	}
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.SecretKey) == "" || strings.TrimSpace(cfg.Passphrase) == "" {
		return 0, fmt.Errorf("okx credentials incomplete")
	}

	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	endpoint := buildProbeEndpoint(cfg.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	addHeaders(req, cfg, "")

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return latency, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return latency, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return latency, fmt.Errorf("okx probe http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if !bytes.Contains(body, []byte(`"code":"0"`)) && !bytes.Contains(body, []byte(`"code":0`)) {
		return latency, fmt.Errorf("okx probe non-success response: %s", strings.TrimSpace(string(body)))
	}
	return latency, nil
}

func buildProbeEndpoint(baseURL string) string {
	baseURL = normalizeBaseURL(baseURL)
	query := url.Values{}
	if strings.Contains(baseURL, "/api/v6/") || strings.Contains(baseURL, "/api/v6") {
		query.Set("chainIndex", "56")
	} else {
		query.Set("chainId", "56")
	}
	query.Set("tokenContractAddress", "0x55d398326f99059fF775485246999027B3197955")
	query.Set("approveAmount", "1")
	return fmt.Sprintf("%s/approve-transaction?%s", baseURL, query.Encode())
}

func addHeaders(req *http.Request, cfg EffectiveConfig, body string) {
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	message := timestamp + req.Method + req.URL.RequestURI() + body
	mac := hmac.New(sha256.New, []byte(cfg.SecretKey))
	mac.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req.Header.Set("OK-ACCESS-KEY", cfg.APIKey)
	req.Header.Set("OK-ACCESS-SIGN", signature)
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", cfg.Passphrase)
	req.Header.Set("Content-Type", "application/json")
}
