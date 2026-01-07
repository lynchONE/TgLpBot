package web_server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"TgLpBot/base/config"
)

type TelegramWebAppUser struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Username     string `json:"username"`
	LanguageCode string `json:"language_code"`
}

type TelegramWebAppInitData struct {
	QueryID  string             `json:"query_id"`
	User     TelegramWebAppUser `json:"user"`
	AuthDate int64              `json:"auth_date"`
}

var ErrMissingInitData = errors.New("missing initData")

func ParseTelegramWebAppInitData(initData string, botToken string) (*TelegramWebAppInitData, error) {
	initData = strings.TrimSpace(initData)
	if initData == "" {
		if config.AppConfig != nil && config.AppConfig.TelegramWebAppAllowEmptyInitData {
			return debugTelegramWebAppInitData(), nil
		}
		return nil, ErrMissingInitData
	}
	return VerifyTelegramWebAppInitData(initData, botToken)
}

func VerifyTelegramWebAppInitData(initData string, botToken string) (*TelegramWebAppInitData, error) {
	initData = strings.TrimSpace(initData)
	botToken = strings.TrimSpace(botToken)
	if initData == "" || botToken == "" {
		return nil, fmt.Errorf("missing initData or bot token")
	}

	values, err := url.ParseQuery(initData)
	if err != nil {
		return nil, fmt.Errorf("parse initData failed: %w", err)
	}

	hash := strings.TrimSpace(values.Get("hash"))
	if hash == "" {
		return nil, fmt.Errorf("missing hash")
	}
	values.Del("hash")

	// Build data_check_string: sort keys, join with "\n", use URL-decoded values.
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(values.Get(k))
	}
	dataCheckString := sb.String()

	secretKey := hmacSHA256([]byte("WebAppData"), []byte(botToken))
	expected := hex.EncodeToString(hmacSHA256(secretKey, []byte(dataCheckString)))

	if !hmac.Equal([]byte(expected), []byte(hash)) {
		return nil, fmt.Errorf("hash mismatch")
	}

	// Basic expiry check.
	authDateRaw := strings.TrimSpace(values.Get("auth_date"))
	if authDateRaw != "" {
		if ts, err := strconv.ParseInt(authDateRaw, 10, 64); err == nil && ts > 0 {
			if time.Since(time.Unix(ts, 0)) > 24*time.Hour {
				return nil, fmt.Errorf("initData expired")
			}
		}
	}

	out := &TelegramWebAppInitData{
		QueryID: strings.TrimSpace(values.Get("query_id")),
	}
	if authDateRaw != "" {
		out.AuthDate, _ = strconv.ParseInt(authDateRaw, 10, 64)
	}

	userRaw := values.Get("user")
	if strings.TrimSpace(userRaw) != "" {
		_ = json.Unmarshal([]byte(userRaw), &out.User)
	}

	if out.User.ID == 0 {
		return nil, fmt.Errorf("missing user")
	}

	return out, nil
}

func debugTelegramWebAppInitData() *TelegramWebAppInitData {
	userID := int64(0)
	username := ""
	if config.AppConfig != nil {
		userID = config.AppConfig.TelegramWebAppDebugUserID
		username = strings.TrimSpace(config.AppConfig.TelegramWebAppDebugUsername)
	}
	if userID == 0 {
		userID = 1000000000
	}
	if username == "" {
		username = "local_debug"
	}
	return &TelegramWebAppInitData{
		QueryID:  "local_debug",
		AuthDate: time.Now().Unix(),
		User: TelegramWebAppUser{
			ID:           userID,
			Username:     username,
			FirstName:    "Local",
			LastName:     "Debug",
			LanguageCode: "en",
		},
	}
}

func hmacSHA256(key []byte, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}
