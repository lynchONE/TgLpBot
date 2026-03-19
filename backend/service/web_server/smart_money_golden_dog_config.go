package web_server

import (
	"TgLpBot/base/models"
	smgd "TgLpBot/service/smart_money_golden_dog"
	userSvc "TgLpBot/service/user"
	"encoding/json"
	"net/http"
	"strings"
)

type smartMoneyGoldenDogConfigEnvelope struct {
	OK             bool                              `json:"ok"`
	Config         *models.SmartMoneyGoldenDogConfig `json:"config,omitempty"`
	BarkEnabled    bool                              `json:"bark_enabled"`
	BarkConfigured bool                              `json:"bark_configured"`
	BarkReady      bool                              `json:"bark_ready"`
}

func (s *Server) handleSmartMoneyGoldenDogConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetSmartMoneyGoldenDogConfig(w, r)
	case http.MethodPost:
		s.handlePostSmartMoneyGoldenDogConfig(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetSmartMoneyGoldenDogConfig(w http.ResponseWriter, r *http.Request) {
	user, _, ok := authenticateSmartMoneyGoldenDogUser(w, initDataFromQuery(r))
	if !ok {
		return
	}

	repo := smgd.NewRepository()
	cfg, err := repo.GetOrCreateConfig(r.Context(), user.ID, normalizeSmartMoneyGoldenDogChain(r.URL.Query().Get("chain")))
	if err != nil {
		http.Error(w, "failed to load config", http.StatusInternalServerError)
		return
	}

	barkStatus, err := smgd.ResolveUserBarkStatus(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "failed to load bark status", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, smartMoneyGoldenDogConfigEnvelope{
		OK:             true,
		Config:         cfg,
		BarkEnabled:    barkStatus.Enabled,
		BarkConfigured: barkStatus.Configured,
		BarkReady:      barkStatus.Ready,
	})
}

func (s *Server) handlePostSmartMoneyGoldenDogConfig(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req struct {
		InitData        string `json:"initData"`
		Chain           string `json:"chain"`
		Enabled         *bool  `json:"enabled"`
		MinWallets      *int   `json:"min_wallets"`
		WindowMinutes   *int   `json:"window_minutes"`
		CooldownMinutes *int   `json:"cooldown_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	user, _, ok := authenticateSmartMoneyGoldenDogUser(w, strings.TrimSpace(req.InitData))
	if !ok {
		return
	}

	updates := make(map[string]any)
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.MinWallets != nil {
		updates["min_wallets"] = clampSmartMoneyGoldenDogMinWallets(*req.MinWallets)
	}
	if req.WindowMinutes != nil {
		updates["window_minutes"] = clampSmartMoneyGoldenDogWindowMinutes(*req.WindowMinutes)
	}
	if req.CooldownMinutes != nil {
		updates["cooldown_minutes"] = clampSmartMoneyGoldenDogCooldownMinutes(*req.CooldownMinutes)
	}

	repo := smgd.NewRepository()
	cfg, err := repo.UpdateConfig(r.Context(), user.ID, normalizeSmartMoneyGoldenDogChain(req.Chain), updates)
	if err != nil {
		http.Error(w, "failed to save config", http.StatusInternalServerError)
		return
	}

	barkStatus, err := smgd.ResolveUserBarkStatus(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "failed to load bark status", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, smartMoneyGoldenDogConfigEnvelope{
		OK:             true,
		Config:         cfg,
		BarkEnabled:    barkStatus.Enabled,
		BarkConfigured: barkStatus.Configured,
		BarkReady:      barkStatus.Ready,
	})
}

func authenticateSmartMoneyGoldenDogUser(w http.ResponseWriter, initData string) (*models.User, userSvc.AccessCheck, bool) {
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return nil, userSvc.AccessCheck{}, false
	}

	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg, status)
		return nil, userSvc.AccessCheck{}, false
	}
	if status != 0 {
		http.Error(w, msg, status)
		return nil, userSvc.AccessCheck{}, false
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		http.Error(w, msg, status)
		return nil, userSvc.AccessCheck{}, false
	}
	return user, check, true
}

func normalizeSmartMoneyGoldenDogChain(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "base":
		return "base"
	default:
		return "bsc"
	}
}

func clampSmartMoneyGoldenDogMinWallets(value int) int {
	if value < 1 {
		return smgd.DefaultMinWallets
	}
	if value > 100 {
		return 100
	}
	return value
}

func clampSmartMoneyGoldenDogWindowMinutes(value int) int {
	if value < 1 {
		return smgd.DefaultWindowMinutes
	}
	if value > 1440 {
		return 1440
	}
	return value
}

func clampSmartMoneyGoldenDogCooldownMinutes(value int) int {
	if value < 0 {
		return smgd.DefaultCooldownMinutes
	}
	if value > 10080 {
		return 10080
	}
	return value
}
