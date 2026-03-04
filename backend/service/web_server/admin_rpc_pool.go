package web_server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/rpcpool"
	userSvc "TgLpBot/service/user"
)

type adminRPCPoolRequest struct {
	InitData string `json:"initData"`
	Action   string `json:"action"` // list | add | switch | disable | enable

	EndpointID uint   `json:"endpoint_id,omitempty"`
	Chain      string `json:"chain,omitempty"`
	Transport  string `json:"transport,omitempty"` // http | ws
	URL        string `json:"url,omitempty"`
	SetCurrent bool   `json:"set_current,omitempty"`

	DisableUntilUnix int64  `json:"disable_until_unix,omitempty"` // seconds
	DisableNextMonth bool   `json:"disable_next_month,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

type adminRPCEndpointDTO struct {
	ID        uint   `json:"id"`
	Chain     string `json:"chain"`
	Transport string `json:"transport"`

	URL       string `json:"url"`
	URLMasked string `json:"url_masked"`

	IsCurrent bool `json:"is_current"`

	Status         string     `json:"status"` // available | unavailable
	DisabledUntil  *time.Time `json:"disabled_until,omitempty"`
	DisabledReason string     `json:"disabled_reason,omitempty"`

	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastCheckedAt       *time.Time `json:"last_checked_at,omitempty"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	LastLatencyMs       int64      `json:"last_latency_ms"`
	LastError           string     `json:"last_error,omitempty"`
}

type adminRPCPoolGroup struct {
	Chain     string `json:"chain"`
	Transport string `json:"transport"`

	EffectiveSource    string `json:"effective_source"`
	EffectiveURL       string `json:"effective_url"`
	EffectiveURLMasked string `json:"effective_url_masked"`
	EffectiveEndpoint  *uint  `json:"effective_endpoint_id,omitempty"`

	EnvURL       string `json:"env_url,omitempty"`
	EnvURLMasked string `json:"env_url_masked,omitempty"`

	Endpoints []adminRPCEndpointDTO `json:"endpoints"`
}

type adminRPCPoolResponse struct {
	OK     bool                `json:"ok"`
	Now    time.Time           `json:"now"`
	Groups []adminRPCPoolGroup `json:"groups"`
}

func (s *Server) handleAdminRPCPool(w http.ResponseWriter, r *http.Request) {
	initData := ""
	action := ""
	var req adminRPCPoolRequest

	switch r.Method {
	case http.MethodGet:
		initData = strings.TrimSpace(r.URL.Query().Get("initData"))
		if initData == "" {
			initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
		}
		action = "list"
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
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

	userService := userSvc.NewUserService()
	user, err := userService.GetOrCreateUser(
		parsed.User.ID,
		parsed.User.Username,
		parsed.User.FirstName,
		parsed.User.LastName,
		parsed.User.LanguageCode,
	)
	if err != nil {
		http.Error(w, "load user failed", http.StatusInternalServerError)
		return
	}

	accessService := userSvc.NewAccessService()
	if !accessService.IsAdminUser(user.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	mgr := rpcpool.Default()
	if mgr == nil {
		http.Error(w, "rpc pool not available", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	switch action {
	case "list":
		writeRPCPoolList(w, ctx)
		return
	case "add":
		chain := strings.TrimSpace(req.Chain)
		transport := strings.TrimSpace(req.Transport)
		url := strings.TrimSpace(req.URL)
		_, err := mgr.AddEndpoint(ctx, chain, transport, url, req.SetCurrent)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeRPCPoolList(w, ctx)
		return
	case "switch":
		if req.EndpointID == 0 {
			http.Error(w, "endpoint_id required", http.StatusBadRequest)
			return
		}
		if err := mgr.SwitchCurrent(ctx, req.EndpointID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeRPCPoolList(w, ctx)
		return
	case "disable":
		if req.EndpointID == 0 {
			http.Error(w, "endpoint_id required", http.StatusBadRequest)
			return
		}
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			reason = rpcpool.ReasonManual
		}
		if req.DisableNextMonth {
			if err := mgr.DisableUntilNextMonth(ctx, req.EndpointID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else if req.DisableUntilUnix > 0 {
			until := time.Unix(req.DisableUntilUnix, 0)
			if err := mgr.DisableEndpoint(ctx, req.EndpointID, until, reason); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "disable_next_month or disable_until_unix required", http.StatusBadRequest)
			return
		}
		writeRPCPoolList(w, ctx)
		return
	case "enable":
		if req.EndpointID == 0 {
			http.Error(w, "endpoint_id required", http.StatusBadRequest)
			return
		}
		if err := mgr.EnableEndpoint(ctx, req.EndpointID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeRPCPoolList(w, ctx)
		return
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
		return
	}
}

func writeRPCPoolList(w http.ResponseWriter, ctx context.Context) {
	mgr := rpcpool.Default()
	now := time.Now()
	store := rpcpool.NewGormStore()

	type pair struct {
		chain     string
		transport string
	}
	pairs := []pair{
		{chain: "bsc", transport: rpcpool.TransportHTTP},
		{chain: "bsc", transport: rpcpool.TransportWS},
		{chain: "base", transport: rpcpool.TransportHTTP},
		{chain: "base", transport: rpcpool.TransportWS},
	}

	groups := make([]adminRPCPoolGroup, 0, len(pairs))
	for _, p := range pairs {
		eff, _ := mgr.EffectiveURL(ctx, p.chain, p.transport)
		envURL := strings.TrimSpace(rpcpool.EnvFromConfig(p.chain, p.transport))

		dbList, _ := store.List(ctx, config.NormalizeChain(p.chain), rpcpool.NormalizeTransport(p.transport))
		items := make([]adminRPCEndpointDTO, 0, len(dbList))
		for _, ep := range dbList {
			status := "available"
			if ep.DisabledUntil != nil && now.Before(*ep.DisabledUntil) {
				status = "unavailable"
			}
			items = append(items, adminRPCEndpointDTO{
				ID:                  ep.ID,
				Chain:               ep.Chain,
				Transport:           ep.Transport,
				URL:                 ep.URL,
				URLMasked:           rpcpool.MaskURL(ep.URL),
				IsCurrent:           ep.IsCurrent,
				Status:              status,
				DisabledUntil:       ep.DisabledUntil,
				DisabledReason:      ep.DisabledReason,
				ConsecutiveFailures: ep.ConsecutiveFailures,
				LastCheckedAt:       ep.LastCheckedAt,
				LastSuccessAt:       ep.LastSuccessAt,
				LastLatencyMs:       ep.LastLatencyMs,
				LastError:           ep.LastError,
			})
		}

		var effID *uint
		if eff.Endpoint != nil && eff.Endpoint.ID > 0 {
			id := eff.Endpoint.ID
			effID = &id
		}

		groups = append(groups, adminRPCPoolGroup{
			Chain:              p.chain,
			Transport:          p.transport,
			EffectiveSource:    string(eff.Source),
			EffectiveURL:       eff.URL,
			EffectiveURLMasked: rpcpool.MaskURL(eff.URL),
			EffectiveEndpoint:  effID,
			EnvURL:             envURL,
			EnvURLMasked:       rpcpool.MaskURL(envURL),
			Endpoints:          items,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(adminRPCPoolResponse{
		OK:     true,
		Now:    now,
		Groups: groups,
	})
}
