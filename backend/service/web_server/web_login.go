package web_server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/models"
	userSvc "TgLpBot/service/user"
)

type telegramLoginPayload struct {
	ID        int64
	FirstName string
	LastName  string
	Username  string
	PhotoURL  string
	AuthDate  int64
	Hash      string
}

type webLoginResponse struct {
	OK       bool            `json:"ok"`
	InitData string          `json:"initData,omitempty"`
	User     *webLoginUser   `json:"user,omitempty"`
	Access   *webLoginAccess `json:"access,omitempty"`
	Message  string          `json:"message,omitempty"`
	Meta     *webLoginMeta   `json:"meta,omitempty"`
}

type webLoginUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	PhotoURL  string `json:"photo_url,omitempty"`
}

type webLoginAccess struct {
	Allowed        bool   `json:"allowed"`
	IsAdmin        bool   `json:"is_admin"`
	MiniAppEnabled bool   `json:"mini_app_enabled"`
	Reason         string `json:"reason,omitempty"`
}

type webLoginMeta struct {
	AuthenticatedAt time.Time `json:"authenticated_at"`
}

func (s *Server) handleWebLogin(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("endpoint")))
	switch endpoint {
	case "", "telegram_login":
		s.handleTelegramLogin(w, r)
		return
	case "generate_code":
		s.handleGenerateLoginCode(w, r)
		return
	case "check_code":
		s.handleCheckLoginCode(w, r)
		return
	default:
		http.Error(w, "invalid endpoint", http.StatusBadRequest)
		return
	}
}

func (s *Server) handleTelegramLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if config.AppConfig == nil {
		http.Error(w, "config not loaded", http.StatusInternalServerError)
		return
	}
	botToken := strings.TrimSpace(config.AppConfig.TelegramBotToken)
	if botToken == "" {
		http.Error(w, "telegram bot token not configured", http.StatusInternalServerError)
		return
	}

	payload, checkMap, err := parseTelegramLoginPayload(r)
	if err != nil {
		http.Error(w, "invalid login payload", http.StatusBadRequest)
		return
	}
	if err := verifyTelegramLoginPayload(payload, checkMap, botToken); err != nil {
		http.Error(w, "invalid telegram auth", http.StatusUnauthorized)
		return
	}

	userService := userSvc.NewUserService()
	user, err := userService.GetOrCreateUser(
		payload.ID,
		payload.Username,
		payload.FirstName,
		payload.LastName,
		"",
	)
	if err != nil {
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return
	}

	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	initData, err := buildWebInitDataForUser(user, botToken)
	if err != nil {
		http.Error(w, "failed to build initData", http.StatusInternalServerError)
		return
	}

	resp := webLoginResponse{
		OK:       true,
		InitData: initData,
		User: &webLoginUser{
			ID:        payload.ID,
			FirstName: payload.FirstName,
			LastName:  payload.LastName,
			Username:  payload.Username,
			PhotoURL:  payload.PhotoURL,
		},
		Access: &webLoginAccess{
			Allowed:        true,
			IsAdmin:        check.IsAdmin,
			MiniAppEnabled: check.IsAdmin || (check.Access != nil && check.Access.MiniAppEnabled),
			Reason:         strings.TrimSpace(check.Reason),
		},
		Meta: &webLoginMeta{
			AuthenticatedAt: time.Now(),
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func parseTelegramLoginPayload(r *http.Request) (telegramLoginPayload, map[string]string, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16*1024))
	if err != nil {
		return telegramLoginPayload{}, nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()

	raw := map[string]any{}
	if err := dec.Decode(&raw); err != nil {
		return telegramLoginPayload{}, nil, err
	}

	fields := make(map[string]string, 8)
	readStringField := func(key string) string {
		v, ok := raw[key]
		if !ok {
			return ""
		}
		s := strings.TrimSpace(anyToString(v))
		if s != "" {
			fields[key] = s
		}
		return s
	}

	idRaw := readStringField("id")
	authDateRaw := readStringField("auth_date")
	hash := strings.TrimSpace(anyToString(raw["hash"]))

	id, _ := strconv.ParseInt(idRaw, 10, 64)
	authDate, _ := strconv.ParseInt(authDateRaw, 10, 64)

	payload := telegramLoginPayload{
		ID:        id,
		FirstName: readStringField("first_name"),
		LastName:  readStringField("last_name"),
		Username:  readStringField("username"),
		PhotoURL:  readStringField("photo_url"),
		AuthDate:  authDate,
		Hash:      hash,
	}
	return payload, fields, nil
}

func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case float32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case uint64:
		return strconv.FormatUint(t, 10)
	case uint:
		return strconv.FormatUint(uint64(t), 10)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func verifyTelegramLoginPayload(payload telegramLoginPayload, checkMap map[string]string, botToken string) error {
	if payload.ID == 0 {
		return fmt.Errorf("missing id")
	}
	if payload.AuthDate <= 0 {
		return fmt.Errorf("missing auth_date")
	}
	if strings.TrimSpace(payload.Hash) == "" {
		return fmt.Errorf("missing hash")
	}
	if time.Since(time.Unix(payload.AuthDate, 0)) > 24*time.Hour {
		return fmt.Errorf("auth data expired")
	}

	keys := make([]string, 0, len(checkMap))
	for k := range checkMap {
		if k == "hash" {
			continue
		}
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
		sb.WriteString(checkMap[k])
	}
	dataCheckString := sb.String()

	secret := sha256.Sum256([]byte(botToken))
	expected := hex.EncodeToString(hmacSHA256(secret[:], []byte(dataCheckString)))
	if !hmacEqualHex(expected, payload.Hash) {
		return fmt.Errorf("hash mismatch")
	}
	return nil
}

func hmacEqualHex(expected string, got string) bool {
	expectedBytes, err := hex.DecodeString(strings.TrimSpace(expected))
	if err != nil || len(expectedBytes) == 0 {
		return false
	}
	gotBytes, err := hex.DecodeString(strings.TrimSpace(got))
	if err != nil || len(gotBytes) == 0 {
		return false
	}
	return hmac.Equal(expectedBytes, gotBytes)
}

func buildWebInitDataForUser(user *models.User, botToken string) (string, error) {
	if user == nil {
		return "", fmt.Errorf("user is nil")
	}
	if strings.TrimSpace(botToken) == "" {
		return "", fmt.Errorf("bot token is empty")
	}

	webUser := TelegramWebAppUser{
		ID:           user.TelegramID,
		FirstName:    user.FirstName,
		LastName:     user.LastName,
		Username:     user.Username,
		LanguageCode: user.LanguageCode,
	}
	userJSON, err := json.Marshal(webUser)
	if err != nil {
		return "", err
	}

	queryID := fmt.Sprintf("web_login_%d_%d", user.TelegramID, time.Now().UnixNano())
	authDate := strconv.FormatInt(time.Now().Unix(), 10)

	fields := map[string]string{
		"auth_date": authDate,
		"query_id":  queryID,
		"user":      string(userJSON),
	}

	keys := make([]string, 0, len(fields))
	for k := range fields {
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
		sb.WriteString(fields[k])
	}
	dataCheckString := sb.String()

	secret := hmacSHA256([]byte("WebAppData"), []byte(botToken))
	hash := hex.EncodeToString(hmacSHA256(secret, []byte(dataCheckString)))

	values := url.Values{}
	values.Set("query_id", queryID)
	values.Set("user", string(userJSON))
	values.Set("auth_date", authDate)
	values.Set("hash", hash)

	return values.Encode(), nil
}
