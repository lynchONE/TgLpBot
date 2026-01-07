package web_server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"TgLpBot/base/config"
	userSvc "TgLpBot/service/user"
)

type meRequest struct {
	InitData string `json:"initData"`
}

type meResponse struct {
	UserID  uint `json:"user_id"`
	IsAdmin bool `json:"is_admin"`
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	initData := ""
	switch r.Method {
	case http.MethodGet:
		initData = strings.TrimSpace(r.URL.Query().Get("initData"))
		if initData == "" {
			initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
		}
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var req meRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if config.AppConfig == nil {
		http.Error(w, "config not loaded", http.StatusInternalServerError)
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
	user, err := userService.GetOrCreateUser(
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
	resp := meResponse{
		UserID:  user.ID,
		IsAdmin: accessService.IsAdminUser(user.ID),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
