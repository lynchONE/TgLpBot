package web_server

import (
	"encoding/json"
	"net/http"
	"strings"

	"TgLpBot/base/database"
	"TgLpBot/base/models"
	autoLP "TgLpBot/service/auto_lp"
)

type autoLPConfigRequest struct {
	InitData           string `json:"initData"`
	GuardCompareToPeak *bool  `json:"guard_compare_to_peak,omitempty"`
}

type autoLPConfigResponse struct {
	OK     bool                     `json:"ok"`
	Config *models.AutoLPUserConfig `json:"config,omitempty"`
}

func (s *Server) handleAutoLPConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req autoLPConfigRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	if database.DB == nil {
		http.Error(w, "database not initialized", http.StatusInternalServerError)
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
	if status, msg := requireAutoModePermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	cfgService := autoLP.NewAutoLPUserConfigService()
	cfg, err := cfgService.GetOrCreate(user.ID)
	if err != nil {
		http.Error(w, "failed to load autolp config", http.StatusInternalServerError)
		return
	}

	if req.GuardCompareToPeak != nil {
		cfg, err = cfgService.Update(user.ID, map[string]interface{}{
			"GuardCompareToPeak": *req.GuardCompareToPeak,
		})
		if err != nil {
			http.Error(w, "failed to update autolp config", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(autoLPConfigResponse{
		OK:     true,
		Config: cfg,
	})
}
