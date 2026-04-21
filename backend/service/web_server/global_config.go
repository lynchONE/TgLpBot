package web_server

import (
	"encoding/json"
	"net/http"
	"strings"

	"TgLpBot/base/models"
	"TgLpBot/service/strategy"
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

	// Check if this is a save request (has "action" field set to "save")
	var actionStr string
	if actionRaw, ok := raw["action"]; ok {
		_ = json.Unmarshal(actionRaw, &actionStr)
	}

	if strings.TrimSpace(actionStr) == "save" {
		updates := buildGlobalConfigUpdates(raw)
		if len(updates) > 0 {
			cfg, err := cfgService.Update(user.ID, updates)
			if err != nil {
				http.Error(w, "failed to save config", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(globalConfigResponse{OK: true, Config: cfg})
			return
		}
	}

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

// buildGlobalConfigUpdates extracts known config fields from the raw JSON body.
func buildGlobalConfigUpdates(raw map[string]json.RawMessage) map[string]interface{} {
	updates := make(map[string]interface{})

	setBool := func(jsonKey, dbKey string) {
		if v, ok := raw[jsonKey]; ok {
			var b bool
			if json.Unmarshal(v, &b) == nil {
				updates[dbKey] = b
			}
		}
	}
	setInt := func(jsonKey, dbKey string) {
		if v, ok := raw[jsonKey]; ok {
			var n int
			if json.Unmarshal(v, &n) == nil {
				updates[dbKey] = n
			}
		}
	}
	setFloat := func(jsonKey, dbKey string) {
		if v, ok := raw[jsonKey]; ok {
			var f float64
			if json.Unmarshal(v, &f) == nil {
				updates[dbKey] = f
			}
		}
	}
	setString := func(jsonKey, dbKey string) {
		if v, ok := raw[jsonKey]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				updates[dbKey] = strings.TrimSpace(s)
			}
		}
	}

	setInt("rebalance_timeout", "rebalance_timeout")
	if v, ok := updates["rebalance_timeout"].(int); ok {
		updates["rebalance_timeout"] = strategy.NormalizeRebalanceTimeout(v)
	}
	setFloat("stop_loss_threshold", "stop_loss_threshold")
	setBool("stop_loss_enabled", "stop_loss_enabled")
	setInt("stop_loss_delay_seconds", "stop_loss_delay_seconds")
	setFloat("slippage_tolerance", "slippage_tolerance")
	setBool("auto_reinvest", "auto_reinvest")
	setFloat("residual_tolerance", "residual_tolerance")
	setFloat("zap_loss_tolerance", "zap_loss_tolerance")
	setBool("extra_notifications_enabled", "extra_notifications_enabled")
	setBool("filter_chinese_tokens", "filter_chinese_tokens")
	setBool("multi_chain_enabled", "multi_chain_enabled")
	setString("default_chain", "default_chain")
	setBool("multi_wallet_enabled", "multi_wallet_enabled")
	setBool("bark_enabled", "bark_enabled")
	setString("bark_server", "bark_server")
	setString("bark_group", "bark_group")
	setFloat("open_position_target_share_min", "open_position_target_share_min")
	setFloat("open_position_target_share_max", "open_position_target_share_max")
	setFloat("open_position_risk_cap_usd", "open_position_risk_cap_usd")
	setFloat("open_position_risk_cap_ratio", "open_position_risk_cap_ratio")

	setBool("dca_enabled", "dca_enabled")
	if v, ok := raw["dca_interval_seconds"]; ok {
		var n float64
		if json.Unmarshal(v, &n) == nil {
			if normalized, err := strategy.NormalizeDCAInterval(n); err == nil {
				updates["dca_interval_seconds"] = normalized
			}
		}
	}
	if v, ok := raw["dca_percentages"]; ok {
		var arr []float64
		if json.Unmarshal(v, &arr) == nil {
			if normalized, err := strategy.NormalizeDCAPercentages(arr); err == nil {
				if s, merr := strategy.MarshalDCAPercentages(normalized); merr == nil {
					updates["dca_percentages_json"] = s
				}
			}
		}
	}
	if v, ok := raw["dca_min_split_amount_usdt"]; ok {
		var n float64
		if json.Unmarshal(v, &n) == nil {
			if normalized, err := strategy.NormalizeDCAMinSplitAmountUSDT(n); err == nil {
				updates["dca_min_split_amount_usdt"] = normalized
			}
		}
	}

	return updates
}
