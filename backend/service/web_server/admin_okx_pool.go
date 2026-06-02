package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/okxpool"
	userSvc "TgLpBot/service/user"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type adminOKXPoolRequest struct {
	InitData string `json:"initData"`
	Action   string `json:"action"`

	ConfigID uint `json:"config_id,omitempty"`

	Name       string `json:"name,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	APIKey     string `json:"api_key,omitempty"`
	SecretKey  string `json:"secret_key,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
	SetCurrent bool   `json:"set_current,omitempty"`

	DisableUntilUnix int64  `json:"disable_until_unix,omitempty"`
	DisableNextMonth bool   `json:"disable_next_month,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

type adminOKXConfigDTO struct {
	ID uint `json:"id"`

	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	BaseURLMasked string `json:"base_url_masked"`
	APIKeyMasked  string `json:"api_key_masked"`

	IsCurrent bool `json:"is_current"`
	IsEnabled bool `json:"is_enabled"`

	Status         string     `json:"status"`
	DisabledUntil  *time.Time `json:"disabled_until,omitempty"`
	DisabledReason string     `json:"disabled_reason,omitempty"`

	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastCheckedAt       *time.Time `json:"last_checked_at,omitempty"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	LastLatencyMs       int64      `json:"last_latency_ms"`
	LastError           string     `json:"last_error,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type adminOKXPoolResponse struct {
	OK  bool      `json:"ok"`
	Now time.Time `json:"now"`

	EffectiveSource        string `json:"effective_source"`
	EffectiveConfigID      *uint  `json:"effective_config_id,omitempty"`
	EffectiveBaseURL       string `json:"effective_base_url"`
	EffectiveBaseURLMasked string `json:"effective_base_url_masked"`
	EffectiveAPIKeyMasked  string `json:"effective_api_key_masked"`

	EnvBaseURL       string `json:"env_base_url,omitempty"`
	EnvBaseURLMasked string `json:"env_base_url_masked,omitempty"`
	EnvAPIKeyMasked  string `json:"env_api_key_masked,omitempty"`

	Configs []adminOKXConfigDTO `json:"configs"`
}

var (
	adminOKXPoolGetOrCreateUser = func(parsed *TelegramWebAppInitData) (*models.User, error) {
		userService := userSvc.NewUserService()
		return userService.GetOrCreateUser(
			parsed.User.ID,
			parsed.User.Username,
			parsed.User.FirstName,
			parsed.User.LastName,
			parsed.User.LanguageCode,
		)
	}
	adminOKXPoolIsAdminUser = func(userID uint) bool {
		accessService := userSvc.NewAccessService()
		return accessService.IsAdminUser(userID)
	}
	adminOKXPoolManager = okxpool.Default
)

func (s *Server) handleAdminOKXPool(w http.ResponseWriter, r *http.Request) {
	initData := ""
	action := ""
	var req adminOKXPoolRequest

	switch r.Method {
	case http.MethodGet:
		initData = strings.TrimSpace(r.URL.Query().Get("initData"))
		if initData == "" {
			initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
		}
		action = "list"
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 24*1024)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)
		action = strings.ToLower(strings.TrimSpace(req.Action))
		if action == "" {
			action = "list"
		}
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

	user, err := adminOKXPoolGetOrCreateUser(parsed)
	if err != nil {
		http.Error(w, "load user failed", http.StatusInternalServerError)
		return
	}
	if !adminOKXPoolIsAdminUser(user.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	mgr := adminOKXPoolManager()
	if mgr == nil {
		http.Error(w, "okx pool not available", http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()

	switch action {
	case "list":
		writeOKXPoolList(w, ctx, mgr)
	case "add":
		_, err := mgr.AddConfig(ctx, okxInputFromAdminRequest(req))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeOKXPoolList(w, ctx, mgr)
	case "update":
		if req.ConfigID == 0 {
			http.Error(w, "config_id required", http.StatusBadRequest)
			return
		}
		if _, err := mgr.UpdateConfig(ctx, req.ConfigID, okxInputFromAdminRequest(req)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeOKXPoolList(w, ctx, mgr)
	case "rename":
		if req.ConfigID == 0 {
			http.Error(w, "config_id required", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if err := mgr.RenameConfig(ctx, req.ConfigID, req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeOKXPoolList(w, ctx, mgr)
	case "switch":
		if req.ConfigID == 0 {
			http.Error(w, "config_id required", http.StatusBadRequest)
			return
		}
		if err := mgr.SwitchCurrent(ctx, req.ConfigID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeOKXPoolList(w, ctx, mgr)
	case "disable":
		if req.ConfigID == 0 {
			http.Error(w, "config_id required", http.StatusBadRequest)
			return
		}
		if req.DisableNextMonth {
			if err := mgr.DisableUntilNextMonth(ctx, req.ConfigID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else if req.DisableUntilUnix > 0 {
			reason := strings.TrimSpace(req.Reason)
			if reason == "" {
				reason = okxpool.ReasonManual
			}
			until := time.Unix(req.DisableUntilUnix, 0)
			if err := mgr.DisableConfig(ctx, req.ConfigID, until, reason); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			if err := mgr.DisableEnabledFlag(ctx, req.ConfigID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		writeOKXPoolList(w, ctx, mgr)
	case "enable":
		if req.ConfigID == 0 {
			http.Error(w, "config_id required", http.StatusBadRequest)
			return
		}
		if err := mgr.EnableConfig(ctx, req.ConfigID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeOKXPoolList(w, ctx, mgr)
	case "delete":
		if req.ConfigID == 0 {
			http.Error(w, "config_id required", http.StatusBadRequest)
			return
		}
		if err := mgr.DeleteConfig(ctx, req.ConfigID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeOKXPoolList(w, ctx, mgr)
	case "check":
		if req.ConfigID == 0 {
			http.Error(w, "config_id required", http.StatusBadRequest)
			return
		}
		_ = mgr.CheckOne(ctx, req.ConfigID)
		writeOKXPoolList(w, ctx, mgr)
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
	}
}

func okxInputFromAdminRequest(req adminOKXPoolRequest) okxpool.Input {
	return okxpool.Input{
		Name:       strings.TrimSpace(req.Name),
		BaseURL:    strings.TrimSpace(req.BaseURL),
		APIKey:     strings.TrimSpace(req.APIKey),
		SecretKey:  strings.TrimSpace(req.SecretKey),
		Passphrase: strings.TrimSpace(req.Passphrase),
		SetCurrent: req.SetCurrent,
	}
}

func writeOKXPoolList(w http.ResponseWriter, ctx context.Context, mgr *okxpool.Manager) {
	now := time.Now()
	rows, err := mgr.ListAll(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]adminOKXConfigDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, adminOKXConfigDTOFromModel(row, now))
	}

	eff, err := mgr.Effective(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	env := okxpool.EnvFromConfig()
	var effID *uint
	if eff.Config != nil && eff.Config.ID > 0 {
		id := eff.Config.ID
		effID = &id
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(adminOKXPoolResponse{
		OK:                     true,
		Now:                    now,
		EffectiveSource:        string(eff.Source),
		EffectiveConfigID:      effID,
		EffectiveBaseURL:       eff.BaseURL,
		EffectiveBaseURLMasked: okxpool.MaskURL(eff.BaseURL),
		EffectiveAPIKeyMasked:  okxpool.MaskString(eff.APIKey),
		EnvBaseURL:             env.BaseURL,
		EnvBaseURLMasked:       okxpool.MaskURL(env.BaseURL),
		EnvAPIKeyMasked:        okxpool.MaskString(env.APIKey),
		Configs:                items,
	})
}

func adminOKXConfigDTOFromModel(row models.OKXAPIConfig, now time.Time) adminOKXConfigDTO {
	status := "available"
	if !row.IsEnabled || (row.DisabledUntil != nil && now.Before(*row.DisabledUntil)) {
		status = "unavailable"
	}
	return adminOKXConfigDTO{
		ID:                  row.ID,
		Name:                strings.TrimSpace(row.Name),
		BaseURL:             strings.TrimSpace(row.BaseURL),
		BaseURLMasked:       okxpool.MaskURL(row.BaseURL),
		APIKeyMasked:        okxpool.MaskString(row.APIKey),
		IsCurrent:           row.IsCurrent,
		IsEnabled:           row.IsEnabled,
		Status:              status,
		DisabledUntil:       row.DisabledUntil,
		DisabledReason:      row.DisabledReason,
		ConsecutiveFailures: row.ConsecutiveFailures,
		LastCheckedAt:       row.LastCheckedAt,
		LastSuccessAt:       row.LastSuccessAt,
		LastLatencyMs:       row.LastLatencyMs,
		LastError:           strings.TrimSpace(row.LastError),
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}
}
