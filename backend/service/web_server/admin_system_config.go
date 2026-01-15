package web_server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	userSvc "TgLpBot/service/user"
)

type adminSystemConfigRequest struct {
	InitData string `json:"initData"`

	// 可选更新字段 - 硬筛阈值
	AutoLPMinPoolValueUSD     *float64 `json:"autolp_min_pool_value_usd,omitempty"`
	AutoLPMinFeePercentage    *float64 `json:"autolp_min_fee_percentage,omitempty"`
	AutoLPMaxFeePercentage    *float64 `json:"autolp_max_fee_percentage,omitempty"`
	AutoLPMinFeeRate5m        *float64 `json:"autolp_min_fee_rate_5m,omitempty"`
	AutoLPMinTotalFees5m      *float64 `json:"autolp_min_total_fees_5m,omitempty"`
	AutoLPMinTotalVolume5m    *float64 `json:"autolp_min_total_volume_5m,omitempty"`
	AutoLPMinTx5m             *int     `json:"autolp_min_tx_5m,omitempty"`
	AutoLPFilterChineseTokens *bool    `json:"autolp_filter_chinese_tokens,omitempty"`

	// 可选更新字段 - 进场门禁
	AutoLPTrendFilterEnabled     *bool    `json:"autolp_trend_filter_enabled,omitempty"`
	AutoLPEntryTrendCrossPercent *float64 `json:"autolp_entry_trend_cross_pct,omitempty"`
	AutoLPEntryBlockDev5Percent  *float64 `json:"autolp_entry_block_dev5_pct,omitempty"`

	// 可选更新字段 - 宽度策略
	AutoLPWidthSidewaysPercent       *float64 `json:"autolp_width_sideways_percent,omitempty"`
	AutoLPWidthMildUptrendPercent    *float64 `json:"autolp_width_mild_uptrend_percent,omitempty"`
	AutoLPWidthRapidPumpPercent      *float64 `json:"autolp_width_rapid_pump_percent,omitempty"`
	AutoLPFirstOpenFixedWidthEnabled *bool    `json:"autolp_first_open_fixed_width_enabled,omitempty"`
	AutoLPFirstOpenFixedWidthPercent *float64 `json:"autolp_first_open_fixed_width_percent,omitempty"`

	// 可选更新字段 - 退出卫士
	AutoLPGuardVolumeDropPercent    *float64 `json:"autolp_guard_volume_drop_percent,omitempty"`
	AutoLPGuardPriceDropPercent     *float64 `json:"autolp_guard_price_drop_percent,omitempty"`
	AutoLPGuardTxDropPercent        *float64 `json:"autolp_guard_tx_drop_percent,omitempty"`
	AutoLPGuardLowFeeRate5m         *float64 `json:"autolp_guard_low_fee_rate_5m,omitempty"`
	AutoLPGuardVolumeDropPercentLow *float64 `json:"autolp_guard_volume_drop_percent_low,omitempty"`
	AutoLPGuardCooldownSeconds      *int     `json:"autolp_guard_cooldown_seconds,omitempty"`
}

type adminSystemConfigResponse struct {
	OK     bool                 `json:"ok"`
	Config *models.SystemConfig `json:"config,omitempty"`
	// 环境变量默认值（供前端显示参考）
	Defaults            *models.HardFilterConfig  `json:"defaults,omitempty"`
	WidthGuardDefaults  *models.WidthGuardConfig  `json:"width_guard_defaults,omitempty"`
	EntrySignalDefaults *models.EntrySignalConfig `json:"entry_signal_defaults,omitempty"`
}

func (s *Server) handleAdminSystemConfig(w http.ResponseWriter, r *http.Request) {
	initData := ""

	switch r.Method {
	case http.MethodGet:
		initData = strings.TrimSpace(r.URL.Query().Get("initData"))
		if initData == "" {
			initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
		}
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var req adminSystemConfigRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "无效的 JSON 请求体", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)

		// 处理更新请求
		if config.AppConfig == nil {
			http.Error(w, "配置未加载", http.StatusInternalServerError)
			return
		}
		if database.DB == nil {
			http.Error(w, "数据库未初始化", http.StatusInternalServerError)
			return
		}

		parsed, err := ParseTelegramWebAppInitData(initData, config.AppConfig.TelegramBotToken)
		if err != nil {
			if errors.Is(err, ErrMissingInitData) {
				http.Error(w, "缺少 initData", http.StatusBadRequest)
			} else {
				http.Error(w, "无效的 initData", http.StatusUnauthorized)
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
			http.Error(w, "加载用户失败", http.StatusInternalServerError)
			return
		}

		accessService := userSvc.NewAccessService()
		if !accessService.IsAdminUser(user.ID) {
			http.Error(w, "权限不足", http.StatusForbidden)
			return
		}

		// 构建更新 map
		updates := make(map[string]interface{})
		if req.AutoLPMinPoolValueUSD != nil {
			updates["AutoLPMinPoolValueUSD"] = *req.AutoLPMinPoolValueUSD
		}
		if req.AutoLPMinFeePercentage != nil {
			updates["AutoLPMinFeePercentage"] = *req.AutoLPMinFeePercentage
		}
		if req.AutoLPMaxFeePercentage != nil {
			updates["AutoLPMaxFeePercentage"] = *req.AutoLPMaxFeePercentage
		}
		if req.AutoLPMinFeeRate5m != nil {
			updates["AutoLPMinFeeRate5m"] = *req.AutoLPMinFeeRate5m
		}
		if req.AutoLPMinTotalFees5m != nil {
			updates["AutoLPMinTotalFees5m"] = *req.AutoLPMinTotalFees5m
		}
		if req.AutoLPMinTotalVolume5m != nil {
			updates["AutoLPMinTotalVolume5m"] = *req.AutoLPMinTotalVolume5m
		}
		if req.AutoLPMinTx5m != nil {
			updates["AutoLPMinTx5m"] = *req.AutoLPMinTx5m
		}
		if req.AutoLPFilterChineseTokens != nil {
			updates["AutoLPFilterChineseTokens"] = *req.AutoLPFilterChineseTokens
		}
		// 进场门禁
		if req.AutoLPTrendFilterEnabled != nil {
			updates["AutoLPTrendFilterEnabled"] = *req.AutoLPTrendFilterEnabled
		}
		if req.AutoLPEntryTrendCrossPercent != nil {
			updates["AutoLPEntryTrendCrossPercent"] = *req.AutoLPEntryTrendCrossPercent
		}
		if req.AutoLPEntryBlockDev5Percent != nil {
			updates["AutoLPEntryBlockDev5Percent"] = *req.AutoLPEntryBlockDev5Percent
		}
		// 宽度策略
		if req.AutoLPWidthSidewaysPercent != nil {
			updates["AutoLPWidthSidewaysPercent"] = *req.AutoLPWidthSidewaysPercent
		}
		if req.AutoLPWidthMildUptrendPercent != nil {
			updates["AutoLPWidthMildUptrendPercent"] = *req.AutoLPWidthMildUptrendPercent
		}
		if req.AutoLPWidthRapidPumpPercent != nil {
			updates["AutoLPWidthRapidPumpPercent"] = *req.AutoLPWidthRapidPumpPercent
		}
		if req.AutoLPFirstOpenFixedWidthEnabled != nil {
			updates["AutoLPFirstOpenFixedWidthEnabled"] = *req.AutoLPFirstOpenFixedWidthEnabled
		}
		if req.AutoLPFirstOpenFixedWidthPercent != nil {
			updates["AutoLPFirstOpenFixedWidthPercent"] = *req.AutoLPFirstOpenFixedWidthPercent
		}
		// 退出卫士
		if req.AutoLPGuardVolumeDropPercent != nil {
			updates["AutoLPGuardVolumeDropPercent"] = *req.AutoLPGuardVolumeDropPercent
		}
		if req.AutoLPGuardPriceDropPercent != nil {
			updates["AutoLPGuardPriceDropPercent"] = *req.AutoLPGuardPriceDropPercent
		}
		if req.AutoLPGuardTxDropPercent != nil {
			updates["AutoLPGuardTxDropPercent"] = *req.AutoLPGuardTxDropPercent
		}
		if req.AutoLPGuardLowFeeRate5m != nil {
			updates["AutoLPGuardLowFeeRate5m"] = *req.AutoLPGuardLowFeeRate5m
		}
		if req.AutoLPGuardVolumeDropPercentLow != nil {
			updates["AutoLPGuardVolumeDropPercentLow"] = *req.AutoLPGuardVolumeDropPercentLow
		}
		if req.AutoLPGuardCooldownSeconds != nil {
			updates["AutoLPGuardCooldownSeconds"] = *req.AutoLPGuardCooldownSeconds
		}

		if len(updates) > 0 {
			sysConfigService := userSvc.NewSystemConfigService()
			cfg, err := sysConfigService.Update(updates)
			if err != nil {
				http.Error(w, "更新系统配置失败", http.StatusInternalServerError)
				return
			}

			defaults := getEnvDefaults()
			widthGuardDefaults := getWidthGuardDefaults()
			entrySignalDefaults := getEntrySignalDefaults()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(adminSystemConfigResponse{
				OK:                  true,
				Config:              cfg,
				Defaults:            defaults,
				WidthGuardDefaults:  widthGuardDefaults,
				EntrySignalDefaults: entrySignalDefaults,
			})
			return
		}

		// 如果没有更新字段，继续返回当前配置
	default:
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}

	// GET 请求或无更新的 POST 请求
	if config.AppConfig == nil {
		http.Error(w, "配置未加载", http.StatusInternalServerError)
		return
	}
	if database.DB == nil {
		http.Error(w, "数据库未初始化", http.StatusInternalServerError)
		return
	}

	parsed, err := ParseTelegramWebAppInitData(initData, config.AppConfig.TelegramBotToken)
	if err != nil {
		if errors.Is(err, ErrMissingInitData) {
			http.Error(w, "缺少 initData", http.StatusBadRequest)
		} else {
			http.Error(w, "无效的 initData", http.StatusUnauthorized)
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
		http.Error(w, "加载用户失败", http.StatusInternalServerError)
		return
	}

	accessService := userSvc.NewAccessService()
	if !accessService.IsAdminUser(user.ID) {
		http.Error(w, "权限不足", http.StatusForbidden)
		return
	}

	sysConfigService := userSvc.NewSystemConfigService()
	cfg, err := sysConfigService.GetOrCreate()
	if err != nil {
		http.Error(w, "加载系统配置失败", http.StatusInternalServerError)
		return
	}

	defaults := getEnvDefaults()
	widthGuardDefaults := getWidthGuardDefaults()
	entrySignalDefaults := getEntrySignalDefaults()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(adminSystemConfigResponse{
		OK:                  true,
		Config:              cfg,
		Defaults:            defaults,
		WidthGuardDefaults:  widthGuardDefaults,
		EntrySignalDefaults: entrySignalDefaults,
	})
}

// getEnvDefaults 获取环境变量默认值
func getEnvDefaults() *models.HardFilterConfig {
	if config.AppConfig == nil {
		return nil
	}
	return &models.HardFilterConfig{
		MinPoolValueUSD:     config.AppConfig.AutoLPMinPoolValueUSD,
		MinFeePercentage:    config.AppConfig.AutoLPMinFeePercentage,
		MaxFeePercentage:    config.AppConfig.AutoLPMaxFeePercentage,
		MinFeeRate5m:        config.AppConfig.AutoLPMinFeeRate5m,
		MinTotalFees5m:      config.AppConfig.AutoLPMinTotalFees5m,
		MinTotalVolume5m:    config.AppConfig.AutoLPMinTotalVolume5m,
		MinTx5m:             config.AppConfig.AutoLPMinTx5m,
		FilterChineseTokens: config.AppConfig.AutoLPFilterChineseTokens,
	}
}

// getWidthGuardDefaults 获取宽度和退出卫士环境变量默认值
func getWidthGuardDefaults() *models.WidthGuardConfig {
	if config.AppConfig == nil {
		return nil
	}
	return &models.WidthGuardConfig{
		WidthSidewaysPercent:       config.AppConfig.AutoLPWidthSidewaysPercent,
		WidthMildUptrendPercent:    config.AppConfig.AutoLPWidthMildUptrendPercent,
		WidthRapidPumpPercent:      config.AppConfig.AutoLPWidthRapidPumpPercent,
		FirstOpenFixedWidthEnabled: false,
		FirstOpenFixedWidthPercent: config.AppConfig.AutoLPBaseWidthPercentage,
		GuardVolumeDropPercent:     config.AppConfig.AutoLPGuardVolumeDropPercent,
		GuardPriceDropPercent:      config.AppConfig.AutoLPGuardPriceDropPercent,
		GuardTxDropPercent:         config.AppConfig.AutoLPGuardTxDropPercent,
		GuardLowFeeRate5m:          config.AppConfig.AutoLPGuardLowFeeRate5m,
		GuardVolumeDropPercentLow:  config.AppConfig.AutoLPGuardVolumeDropPercentLow,
		GuardCooldownSeconds:       config.AppConfig.AutoLPGuardCooldownSeconds,
	}
}

func getEntrySignalDefaults() *models.EntrySignalConfig {
	if config.AppConfig == nil {
		return nil
	}
	return &models.EntrySignalConfig{
		TrendFilterEnabled:     config.AppConfig.AutoLPTrendFilterEnabled,
		EntryTrendCrossPercent: config.AppConfig.AutoLPEntryTrendCrossPercent,
		EntryBlockDev5Percent:  config.AppConfig.AutoLPEntryBlockDev5Percent,
	}
}
