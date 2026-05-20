package web_server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/webloginstore"
)

func (s *Server) handleGenerateLoginCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	code, err := webloginstore.GenerateCode()
	if err != nil {
		http.Error(w, "failed to generate code", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"code":        code,
		"expires_in":  int(webloginstore.CodeTTL.Seconds()),
		"instruction": fmt.Sprintf("请在 Telegram 中向 Bot 发送: /weblogin %s", code),
	})
}

func (s *Server) handleCheckLoginCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	entry, confirmed := webloginstore.Check(req.Code)
	if entry == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"status":  "expired",
			"message": "验证码不存在或已过期",
		})
		return
	}
	if !confirmed {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     false,
			"status": "pending",
		})
		return
	}

	if config.AppConfig == nil {
		http.Error(w, "config not loaded", http.StatusInternalServerError)
		return
	}
	botToken := strings.TrimSpace(config.AppConfig.TelegramBotToken)

	user := entry.User
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
	enabledModules, err := enabledModulesForAccessCheck(check)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	initData, err := buildWebInitDataForUser(user, botToken)
	if err != nil {
		log.Printf("web_login_code: buildWebInitDataForUser error: %v", err)
		http.Error(w, "failed to build initData", http.StatusInternalServerError)
		return
	}

	resp := webLoginResponse{
		OK:       true,
		InitData: initData,
		User: &webLoginUser{
			ID:        user.TelegramID,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Username:  user.Username,
			PhotoURL:  entry.PhotoURL,
		},
		Access: &webLoginAccess{
			Allowed:        true,
			IsAdmin:        check.IsAdmin,
			MiniAppEnabled: check.IsAdmin || (check.Access != nil && check.Access.MiniAppEnabled),
			EnabledModules: enabledModules,
			ModuleCatalog:  models.AccessModuleCatalog(),
			Reason:         strings.TrimSpace(check.Reason),
		},
		Meta: &webLoginMeta{
			AuthenticatedAt: time.Now(),
		},
	}
	writeJSON(w, http.StatusOK, resp)
}
