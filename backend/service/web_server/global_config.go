package web_server

import (
	"encoding/json"
	"net/http"
	"strings"

	"TgLpBot/base/models"
	userSvc "TgLpBot/service/user"
)

type globalConfigResponse struct {
	OK     bool                 `json:"ok"`
	Config *models.GlobalConfig `json:"config,omitempty"`
}

func (s *Server) handleGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	initDataRaw, ok := raw["initData"]
	if !ok {
		http.Error(w, "missing initData", http.StatusBadRequest)
		return
	}
	var initData string
	if err := json.Unmarshal(initDataRaw, &initData); err != nil {
		http.Error(w, "invalid initData", http.StatusBadRequest)
		return
	}
	initData = strings.TrimSpace(initData)

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

	cfgService := userSvc.NewGlobalConfigService()

	cfg, err := cfgService.GetOrCreate(user.ID)
	if err != nil {
		http.Error(w, "failed to load config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(globalConfigResponse{
		OK:     true,
		Config: cfg,
	})
}
