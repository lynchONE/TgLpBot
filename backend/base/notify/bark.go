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
	Call    string
	Level   string
}

func barkEndpointWithConfig(title string, body string, cfg BarkConfig) (string, bool) {
	key := strings.Trim(strings.TrimSpace(cfg.Key), "/")
	if key == "" {
		return "", false
	}

	server := barkServerBaseForKey(cfg.Server, key)

	endpoint := server + "/" + url.PathEscape(key) + "/" + url.PathEscape(title) + "/" + url.PathEscape(body)

	q := url.Values{}
	if v := strings.TrimSpace(cfg.Group); v != "" {
		q.Set("group", v)
	}
	if v := strings.TrimSpace(cfg.Sound); v != "" {
		q.Set("sound", v)
	}
	if v := strings.TrimSpace(cfg.Call); v != "" {
		q.Set("call", v)
	}
	if v := strings.TrimSpace(cfg.Level); v != "" {
		q.Set("level", v)
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

func NormalizeBarkServer(server string) (string, bool) {
	s := strings.TrimSpace(server)
	if s == "" {
		return "", true
	}

	candidate := s
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	u, err := url.Parse(candidate)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}

	out := u.Scheme + "://" + u.Host
	path := strings.TrimRight(u.Path, "/")
	if path != "" && path != "/" && !isOfficialBarkHost(u.Hostname()) {
		out += path
	}
	return strings.TrimRight(out, "/"), true
}

func barkServerBaseForKey(server string, key string) string {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	if server == "" {
		return defaultBarkServer
	}

	u, err := url.Parse(server)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return server
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, segment := range segments {
		if segment == key {
			if i == 0 {
				u.Path = ""
			} else {
				u.Path = "/" + strings.Join(segments[:i], "/")
			}
			u.RawPath = ""
			u.RawQuery = ""
			u.Fragment = ""
			return strings.TrimRight(u.String(), "/")
		}
	}

	return server
}

func isOfficialBarkHost(host string) bool {
	return strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(host), "."), "api.day.app")
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
