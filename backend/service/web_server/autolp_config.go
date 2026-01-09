package web_server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	autoLP "TgLpBot/service/auto_lp"
	userSvc "TgLpBot/service/user"
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
	if config.AppConfig == nil {
		http.Error(w, "config not loaded", http.StatusInternalServerError)
		return
	}
	if database.DB == nil {
		http.Error(w, "database not initialized", http.StatusInternalServerError)
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
