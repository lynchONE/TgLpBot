package web_server

import (
	"encoding/json"
	"net/http"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	userSvc "TgLpBot/service/user"
)

type adminSystemConfigRequest struct {
	InitData string `json:"initData"`

	ZapPriceDeviationMaxPercent *float64 `json:"zap_price_deviation_max_percent,omitempty"`
	ZapMinPoolLiquidityUSD      *float64 `json:"zap_min_pool_liquidity_usd,omitempty"`
}

type adminSystemConfigResponse struct {
	OK                bool                    `json:"ok"`
	Config            *models.SystemConfig    `json:"config,omitempty"`
	ZapSafetyDefaults *models.ZapSafetyConfig `json:"zap_safety_defaults,omitempty"`
}

func (s *Server) handleAdminSystemConfig(w http.ResponseWriter, r *http.Request) {
	initData := ""

	switch r.Method {
	case http.MethodGet:
		initData = initDataFromQuery(r)
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var req adminSystemConfigRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)

		user, status, msg := authenticateTelegramWebAppUser(initData)
		if status != 0 {
			http.Error(w, msg, status)
			return
		}
		if _, status, msg, err := requireUserAccess(user.ID); err != nil || status != 0 {
			http.Error(w, msg, status)
			return
		}

		accessService := userSvc.NewAccessService()
		if !accessService.IsAdminUser(user.ID) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if config.AppConfig == nil || database.DB == nil {
			http.Error(w, "service unavailable", http.StatusInternalServerError)
			return
		}

		updates := make(map[string]interface{})
		if req.ZapPriceDeviationMaxPercent != nil {
			updates["ZapPriceDeviationMaxPercent"] = *req.ZapPriceDeviationMaxPercent
		}
		if req.ZapMinPoolLiquidityUSD != nil {
			updates["ZapMinPoolLiquidityUSD"] = *req.ZapMinPoolLiquidityUSD
		}

		sysConfigService := userSvc.NewSystemConfigService()
		cfg, err := func() (*models.SystemConfig, error) {
			if len(updates) == 0 {
				return sysConfigService.GetOrCreate()
			}
			return sysConfigService.Update(updates)
		}()
		if err != nil {
			http.Error(w, "failed to update system config", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(adminSystemConfigResponse{
			OK:                true,
			Config:            cfg,
			ZapSafetyDefaults: getZapSafetyDefaults(),
		})
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if _, status, msg, err := requireUserAccess(user.ID); err != nil || status != 0 {
		http.Error(w, msg, status)
		return
	}

	accessService := userSvc.NewAccessService()
	if !accessService.IsAdminUser(user.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if config.AppConfig == nil || database.DB == nil {
		http.Error(w, "service unavailable", http.StatusInternalServerError)
		return
	}

	sysConfigService := userSvc.NewSystemConfigService()
	cfg, err := sysConfigService.GetOrCreate()
	if err != nil {
		http.Error(w, "failed to load system config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(adminSystemConfigResponse{
		OK:                true,
		Config:            cfg,
		ZapSafetyDefaults: getZapSafetyDefaults(),
	})
}

func getZapSafetyDefaults() *models.ZapSafetyConfig {
	priceDeviationDefault := 1.0
	minLiquidityDefault := 1000.0
	if config.AppConfig != nil {
		if config.AppConfig.ZapPriceDeviationMaxPercent > 0 {
			priceDeviationDefault = config.AppConfig.ZapPriceDeviationMaxPercent
		}
		if config.AppConfig.ZapMinPoolLiquidityUSD > 0 {
			minLiquidityDefault = config.AppConfig.ZapMinPoolLiquidityUSD
		}
	}
	return &models.ZapSafetyConfig{
		PriceDeviationMaxPercent: priceDeviationDefault,
		MinPoolLiquidityUSD:      minLiquidityDefault,
	}
}
