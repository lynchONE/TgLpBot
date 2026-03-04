package rpcpool

import (
	"TgLpBot/base/timeutil"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
)

func validateURLForTransport(raw string, transport string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("rpc url is empty")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid rpc url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid rpc url")
	}

	switch transport {
	case TransportHTTP:
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("rpc url scheme must be http/https")
		}
	case TransportWS:
		if u.Scheme != "ws" && u.Scheme != "wss" {
			return fmt.Errorf("rpc url scheme must be ws/wss")
		}
	default:
		return fmt.Errorf("invalid transport")
	}
	return nil
}

func truncateString(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func nextMonthStart(now time.Time) time.Time {
	loc := timeutil.Location()
	now = now.In(loc)
	y, m, _ := now.Date()
	thisMonth := time.Date(y, m, 1, 0, 0, 0, 0, loc)
	return thisMonth.AddDate(0, 1, 0)
}

func IsQuotaExhaustedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "cu limit exceeded") {
		return true
	}
	if strings.Contains(msg, "quota exceeded") {
		return true
	}
	if strings.Contains(msg, "monthly quota") {
		return true
	}
	// Some providers only expose HTTP status in the error string.
	if strings.Contains(msg, "429") {
		return true
	}
	if strings.Contains(msg, "too many requests") {
		return true
	}
	return false
}

// MaskURL returns a UI-safe masked representation of a URL.
// It keeps scheme+host and hides path/query fragments that may contain API keys.
func MaskURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "***"
	}
	if u.Scheme == "" || u.Host == "" {
		return "***"
	}
	return fmt.Sprintf("%s://%s/…", u.Scheme, u.Host)
}

type EthProber struct {
	DialTimeout time.Duration
	CallTimeout time.Duration
}

func (p *EthProber) Probe(ctx context.Context, rpcURL string, transport string) (time.Duration, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rpcURL = strings.TrimSpace(rpcURL)
	if rpcURL == "" {
		return 0, fmt.Errorf("rpc url is empty")
	}

	dialTimeout := p.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}
	callTimeout := p.CallTimeout
	if callTimeout <= 0 {
		callTimeout = 8 * time.Second
	}

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	client, err := ethclient.DialContext(dialCtx, rpcURL)
	cancel()
	if err != nil {
		return 0, err
	}
	defer client.Close()

	callCtx, cancelCall := context.WithTimeout(ctx, callTimeout)
	defer cancelCall()
	start := time.Now()
	_, err = client.BlockNumber(callCtx)
	return time.Since(start), err
}
