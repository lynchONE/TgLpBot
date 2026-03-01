package web_server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"TgLpBot/base/config"
	userSvc "TgLpBot/service/user"
)

// handleAdminUserAccess handles:
//
//	GET  /api/admin/user_access?initData=...&userId=123  – fetch a user's access record
//	POST /api/admin/user_access                          – update fields (e.g. smartMoneyEnabled)
func (s *Server) handleAdminUserAccess(w http.ResponseWriter, r *http.Request) {
	if config.AppConfig == nil {
		http.Error(w, "config not loaded", http.StatusInternalServerError)
		return
	}

	var initData string
	var targetUserID uint
	var smartMoneyEnabled *bool

	switch r.Method {
	case http.MethodGet:
		initData = strings.TrimSpace(r.URL.Query().Get("initData"))
		if initData == "" {
			initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
		}
		userRaw := strings.TrimSpace(r.URL.Query().Get("userId"))
		if userRaw == "" {
			userRaw = strings.TrimSpace(r.URL.Query().Get("user_id"))
		}
		if userRaw != "" {
			if n, err := strconv.ParseUint(userRaw, 10, 64); err == nil {
				targetUserID = uint(n)
			}
		}

	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var req struct {
			InitData          string `json:"initData"`
			UserID            uint   `json:"userId"`
			SmartMoneyEnabled *bool  `json:"smartMoneyEnabled"`
		}
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)
		targetUserID = req.UserID
		smartMoneyEnabled = req.SmartMoneyEnabled

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if targetUserID == 0 {
		http.Error(w, "missing userId", http.StatusBadRequest)
		return
	}

	parsed, err := ParseTelegramWebAppInitData(initData, config.AppConfig.TelegramBotToken)
	if err != nil {
		if errors.Is(err, ErrMissingInitData) {
			http.Error(w, "missing initData", http.StatusBadRequest)
		} else {
			http.Error(w, "invalid initData", http.StatusUnauthorized)
		}
		return
	}

	userService := userSvc.NewUserService()
	callerUser, err := userService.GetOrCreateUser(
		parsed.User.ID,
		parsed.User.Username,
		parsed.User.FirstName,
		parsed.User.LastName,
		parsed.User.LanguageCode,
	)
	if err != nil {
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return
	}

	accessService := userSvc.NewAccessService()
	if !accessService.IsAdminUser(callerUser.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if r.Method == http.MethodGet {
		access, err := accessService.GetUserAccessWithUser(targetUserID)
		if err != nil {
			http.Error(w, "user access not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(access)
		return
	}

	// POST: apply updates
	if smartMoneyEnabled == nil {
		http.Error(w, "no fields to update", http.StatusBadRequest)
		return
	}

	access, err := accessService.UpdateUserAccess(callerUser.ID, targetUserID, userSvc.UpdateUserAccessInput{
		SmartMoneyEnabled: smartMoneyEnabled,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(access)
}
