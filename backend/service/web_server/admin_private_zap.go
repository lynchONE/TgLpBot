package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/service/liquidity"
	userSvc "TgLpBot/service/user"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type adminPrivateZapRequest struct {
	InitData string `json:"initData"`
	Action   string `json:"action"` // list | invalidate
	Chain    string `json:"chain,omitempty"`
	Kind     string `json:"kind,omitempty"`
}

type adminPrivateZapResponse struct {
	OK     bool                                    `json:"ok"`
	Chains []string                                `json:"chains,omitempty"`
	Kinds  []string                                `json:"kinds,omitempty"`
	Result *liquidity.PrivateZapInvalidationResult `json:"result,omitempty"`
}

func (s *Server) handleAdminPrivateZap(w http.ResponseWriter, r *http.Request) {
	initData := ""
	action := "list"
	var req adminPrivateZapRequest

	switch r.Method {
	case http.MethodGet:
		initData = strings.TrimSpace(r.URL.Query().Get("initData"))
		if initData == "" {
			initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
		}
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)
		if raw := strings.ToLower(strings.TrimSpace(req.Action)); raw != "" {
			action = raw
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

	resp := adminPrivateZapResponse{
		OK:     true,
		Chains: liquidity.EnabledPrivateZapChains(),
		Kinds:  liquidity.SupportedPrivateZapKinds(),
	}

	switch action {
	case "list":
		// no-op
	case "invalidate":
		chain := config.NormalizeChain(req.Chain)
		if chain == "" {
			http.Error(w, "chain required", http.StatusBadRequest)
			return
		}
		liq := liquidity.NewLiquidityService()
		result, err := liq.InvalidatePrivateZapBindingsByChain(r.Context(), chain, req.Kind)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp.Result = result
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
