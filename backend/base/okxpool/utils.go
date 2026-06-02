package okxpool

import (
	"TgLpBot/base/config"
	"TgLpBot/base/security"
	"TgLpBot/base/timeutil"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	SourceDB  Source = "db"
	SourceEnv Source = "env"
)

const (
	ReasonQuotaExhausted = "quota_exhausted"
	ReasonRateLimited    = "rate_limited"
	ReasonHealthFail     = "health_fail"
	ReasonAuthFail       = "auth_fail"
	ReasonManual         = "manual"
)

type Source string

func validateBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("okx base_url is empty")
	}
	if len(raw) > 512 {
		return fmt.Errorf("okx base_url too long (max 512 chars)")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid okx base_url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("okx base_url scheme must be http/https")
	}
	if u.Host == "" {
		return fmt.Errorf("invalid okx base_url")
	}
	return nil
}

func normalizeBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func deriveNameFromURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || strings.TrimSpace(u.Host) == "" {
		return ""
	}
	host := strings.TrimSpace(u.Host)
	if len(host) > 80 {
		host = host[:80]
	}
	return host
}

func normalizeName(name string, baseURL string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = deriveNameFromURL(baseURL)
	}
	if len(name) > 80 {
		return "", fmt.Errorf("okx config name too long (max 80 chars)")
	}
	return name, nil
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

func encryptionKey() ([]byte, error) {
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	key, err := security.DecodeHexKey32(config.AppConfig.EncryptionKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func encryptSecret(raw string) (string, error) {
	key, err := encryptionKey()
	if err != nil {
		return "", err
	}
	return security.EncryptAESGCMToHex(key, []byte(strings.TrimSpace(raw)))
}

func decryptSecret(cipherHex string) (string, error) {
	key, err := encryptionKey()
	if err != nil {
		return "", err
	}
	plain, err := security.DecryptAESGCMHex(key, cipherHex)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(plain)), nil
}

func MaskString(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "..." + s[len(s)-4:]
}

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
	return fmt.Sprintf("%s://%s/...", u.Scheme, u.Host)
}

func IsRateLimitedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "request too fast") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "ratelimit") ||
		strings.Contains(msg, "429") {
		return true
	}
	return false
}

func IsQuotaExhaustedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "quota exceeded") ||
		strings.Contains(msg, "monthly quota") ||
		strings.Contains(msg, "daily quota") ||
		strings.Contains(msg, "daily limit") ||
		strings.Contains(msg, "exceeded quota") ||
		strings.Contains(msg, "insufficient quota") ||
		strings.Contains(msg, "credit exhausted") {
		return true
	}
	if strings.Contains(msg, "429") && (strings.Contains(msg, "quota") || strings.Contains(msg, "credit") || strings.Contains(msg, "limit exceeded")) {
		return true
	}
	return false
}

func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "401") ||
		strings.Contains(msg, "403") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "invalid api") ||
		strings.Contains(msg, "invalid sign") ||
		strings.Contains(msg, "invalid passphrase") ||
		strings.Contains(msg, "permission")
}
