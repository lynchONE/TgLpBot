package web_server

import (
	"encoding/json"
	"net/http"
	"strings"

	userSvc "TgLpBot/service/user"
)

type meRequest struct {
	InitData string `json:"initData"`
}

type meResponse struct {
	UserID            uint `json:"user_id"`
	IsAdmin           bool `json:"is_admin"`
	SmartMoneyEnabled bool `json:"smart_money_enabled"`
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

	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
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

	accessService := userSvc.NewAccessService()
	smartMoneyEnabled := false
	if check.IsAdmin {
		smartMoneyEnabled = true
	} else if check.Access != nil {
		smartMoneyEnabled = check.Access.SmartMoneyEnabled
	}
	resp := meResponse{
		UserID:            user.ID,
		IsAdmin:           accessService.IsAdminUser(user.ID),
		SmartMoneyEnabled: smartMoneyEnabled,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
