package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBarkServer = "https://api.day.app"

type BarkConfig struct {
	Server  string
	Key     string
	Group   string
	Sound   string
	Icon    string
	OpenURL string
}

func barkEndpointWithConfig(title string, body string, cfg BarkConfig) (string, bool) {
	key := strings.Trim(strings.TrimSpace(cfg.Key), "/")
	if key == "" {
		return "", false
	}

	server := strings.TrimRight(strings.TrimSpace(cfg.Server), "/")
	if server == "" {
		server = defaultBarkServer
	}

	endpoint := server + "/" + url.PathEscape(key) + "/" + url.PathEscape(title) + "/" + url.PathEscape(body)

	q := url.Values{}
	if v := strings.TrimSpace(cfg.Group); v != "" {
		q.Set("group", v)
	}
	if v := strings.TrimSpace(cfg.Sound); v != "" {
		q.Set("sound", v)
	}
	if v := strings.TrimSpace(cfg.Icon); v != "" {
		q.Set("icon", v)
	}
	if v := strings.TrimSpace(cfg.OpenURL); v != "" {
		q.Set("url", v)
	}

	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	return endpoint, true
}

// SendBarkWithConfig sends a Bark push notification when cfg.Key is present.
// It is a best-effort helper: callers may ignore errors.
func SendBarkWithConfig(title string, body string, cfg BarkConfig) error {
	endpoint, ok := barkEndpointWithConfig(title, body, cfg)
	if !ok {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request failed: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Avoid logging secrets; Bark key is embedded in URL path.
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return fmt.Errorf("unexpected status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	return nil
}
