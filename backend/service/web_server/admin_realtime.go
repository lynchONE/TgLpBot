package web_server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/service/realtime"
	userSvc "TgLpBot/service/user"
)

type adminRealtimeUsersRequest struct {
	InitData string `json:"initData"`
	Limit    int    `json:"limit"`
}

type adminRealtimeUsersResponse struct {
	Users []realtime.AdminActiveUser `json:"users"`
	Total int                        `json:"total"`
}

type adminRealtimePositionsRequest struct {
	InitData string `json:"initData"`
	UserID   uint   `json:"userId"`
}

func (s *Server) handleAdminRealtimeUsers(w http.ResponseWriter, r *http.Request) {
	initData := ""
	limit := 0

	switch r.Method {
	case http.MethodGet:
		initData = strings.TrimSpace(r.URL.Query().Get("initData"))
		if initData == "" {
			initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
		}
		limitRaw := strings.TrimSpace(r.URL.Query().Get("limit"))
		if limitRaw != "" {
			if n, err := strconv.Atoi(limitRaw); err == nil {
				limit = n
			}
		}
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var req adminRealtimeUsersRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)
		limit = req.Limit
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
	if !accessService.IsAdminUser(user.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	adminService := realtime.NewAdminRealtimeService()
	users, err := adminService.ListActiveTaskUsers(limit)
	if err != nil {
		http.Error(w, "failed to load users", http.StatusInternalServerError)
		return
	}

	resp := adminRealtimeUsersResponse{
		Users: users,
		Total: len(users),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleAdminRealtimePositions(w http.ResponseWriter, r *http.Request) {
	initData := ""
	var userID uint

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
				userID = uint(n)
			}
		}
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var req adminRealtimePositionsRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)
		userID = req.UserID
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if userID == 0 {
		http.Error(w, "missing userId", http.StatusBadRequest)
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
	if !accessService.IsAdminUser(user.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	resp, err := s.Realtime.GetForUser(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp.IsAdmin = true

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
