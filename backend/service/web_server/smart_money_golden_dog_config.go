package web_server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"TgLpBot/base/database"
	"TgLpBot/base/models"

	"gorm.io/gorm"
)

type smartMoneyGoldenDogConfigGetResponse struct {
	Config models.SmartMoneyGoldenDogConfig `json:"config"`
}

type smartMoneyGoldenDogConfigUpsertRequest struct {
	InitData string `json:"initData"`

	Chain string `json:"chain"`

	Enabled         *bool `json:"enabled,omitempty"`
	MinWallets      *int  `json:"min_wallets,omitempty"`
	WindowMinutes   *int  `json:"window_minutes,omitempty"`
	CooldownMinutes *int  `json:"cooldown_minutes,omitempty"`
}

type smartMoneyGoldenDogConfigUpsertResponse struct {
	Config models.SmartMoneyGoldenDogConfig `json:"config"`
}

func clampIntRange(v int, min int, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func (s *Server) handleSmartMoneyGoldenDogConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleSmartMoneyGoldenDogConfigGet(w, r)
		return
	case http.MethodPost:
		s.handleSmartMoneyGoldenDogConfigUpsert(w, r)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func (s *Server) handleSmartMoneyGoldenDogConfigGet(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	chain := normalizeChain(query.Get("chain"))

	initData := initDataFromQuery(r)
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
	if status, msg := requireSmartMoneyPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	resp := smartMoneyGoldenDogConfigGetResponse{
		Config: models.SmartMoneyGoldenDogConfig{
			UserID:          user.ID,
			Chain:           chain,
			Enabled:         false,
			MinWallets:      3,
			WindowMinutes:   10,
			CooldownMinutes: 30,
		},
	}

	var cfg models.SmartMoneyGoldenDogConfig
	err = database.DB.Where("user_id = ? AND chain = ?", user.ID, chain).First(&cfg).Error
	if err == nil {
		resp.Config = cfg
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "failed to load golden-dog config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleSmartMoneyGoldenDogConfigUpsert(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req smartMoneyGoldenDogConfigUpsertRequest
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	chain := normalizeChain(req.Chain)
	user, status, msg := authenticateTelegramWebAppUser(strings.TrimSpace(req.InitData))
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
	if status, msg := requireSmartMoneyPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	var cfg models.SmartMoneyGoldenDogConfig
	err = database.DB.Where("user_id = ? AND chain = ?", user.ID, chain).First(&cfg).Error
	hasExisting := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "failed to load golden-dog config", http.StatusInternalServerError)
		return
	}

	enabled := cfg.Enabled
	minWallets := 3
	windowMinutes := 10
	cooldownMinutes := 30
	if hasExisting {
		minWallets = cfg.MinWallets
		windowMinutes = cfg.WindowMinutes
		cooldownMinutes = cfg.CooldownMinutes
	}

	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.MinWallets != nil {
		minWallets = clampIntRange(*req.MinWallets, 2, 100)
	}
	if req.WindowMinutes != nil {
		windowMinutes = clampIntRange(*req.WindowMinutes, 1, 180)
	}
	if req.CooldownMinutes != nil {
		cooldownMinutes = clampIntRange(*req.CooldownMinutes, 0, 24*60)
	}

	now := time.Now()
	if hasExisting {
		cfg.Enabled = enabled
		cfg.MinWallets = minWallets
		cfg.WindowMinutes = windowMinutes
		cfg.CooldownMinutes = cooldownMinutes
		cfg.UpdatedAt = now
		if err := database.DB.Save(&cfg).Error; err != nil {
			http.Error(w, "failed to save golden-dog config", http.StatusInternalServerError)
			return
		}
	} else {
		cfg = models.SmartMoneyGoldenDogConfig{
			UserID:          user.ID,
			Chain:           chain,
			Enabled:         enabled,
			MinWallets:      minWallets,
			WindowMinutes:   windowMinutes,
			CooldownMinutes: cooldownMinutes,
		}
		if err := database.DB.Create(&cfg).Error; err != nil {
			http.Error(w, "failed to save golden-dog config", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(smartMoneyGoldenDogConfigUpsertResponse{Config: cfg})
}
