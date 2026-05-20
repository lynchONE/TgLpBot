package web_server

import (
	"TgLpBot/base/models"
	"TgLpBot/base/notify"
	smgd "TgLpBot/service/smart_money_golden_dog"
	userSvc "TgLpBot/service/user"
	"encoding/json"
	"net/http"
	"strings"
)

type smartMoneyGoldenDogConfigEnvelope struct {
	OK                   bool                              `json:"ok"`
	Config               *models.SmartMoneyGoldenDogConfig `json:"config,omitempty"`
	BarkEnabled          bool                              `json:"bark_enabled"`
	BarkConfigured       bool                              `json:"bark_configured"`
	BarkReady            bool                              `json:"bark_ready"`
	AvailableIntensities []smgd.BarkIntensityOption        `json:"available_intensities"`
}

type smartMoneyGoldenDogMessageEnvelope struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

type smartMoneyGoldenDogWalletModePayload struct {
	Enabled              *bool                      `json:"enabled"`
	MinWallets           *int                       `json:"min_wallets"`
	WindowMinutes        *int                       `json:"window_minutes"`
	CooldownMinutes      *int                       `json:"cooldown_minutes"`
	MinTotalAmountUSD    *float64                   `json:"min_total_amount_usd"`
	Intensity            *string                    `json:"intensity"`
	IntensityMode        *string                    `json:"intensity_mode"`
	AmountIntensityTiers []smgd.AmountIntensityTier `json:"amount_intensity_tiers"`
}

type smartMoneyGoldenDogPoolModePayload struct {
	Enabled                 *bool    `json:"enabled"`
	CooldownMinutes         *int     `json:"cooldown_minutes"`
	MinTotalFees            *float64 `json:"min_total_fees"`
	MinTransactionCount     *int     `json:"min_transaction_count"`
	MinTVL                  *float64 `json:"min_tvl"`
	MinVolume               *float64 `json:"min_volume"`
	MinFeeRate              *float64 `json:"min_fee_rate"`
	MinActiveLiquidityRatio *float64 `json:"min_active_liquidity_ratio"`
	Intensity               *string  `json:"intensity"`
}

type smartMoneyGoldenDogUpdateRequest struct {
	InitData string `json:"initData"`
	Chain    string `json:"chain"`

	WalletMode *smartMoneyGoldenDogWalletModePayload `json:"wallet_mode"`
	PoolMode   *smartMoneyGoldenDogPoolModePayload   `json:"pool_mode"`

	Enabled                     *bool                      `json:"enabled"`
	MinWallets                  *int                       `json:"min_wallets"`
	WindowMinutes               *int                       `json:"window_minutes"`
	CooldownMinutes             *int                       `json:"cooldown_minutes"`
	WalletMinTotalAmountUSD     *float64                   `json:"wallet_min_total_amount_usd"`
	WalletIntensity             *string                    `json:"wallet_intensity"`
	WalletIntensityMode         *string                    `json:"wallet_intensity_mode"`
	WalletAmountIntensityTiers  []smgd.AmountIntensityTier `json:"wallet_amount_intensity_tiers"`
	PoolEnabled                 *bool                      `json:"pool_enabled"`
	PoolCooldownMinutes         *int                       `json:"pool_cooldown_minutes"`
	PoolMinTotalFees            *float64                   `json:"pool_min_total_fees"`
	PoolMinTransactionCount     *int                       `json:"pool_min_transaction_count"`
	PoolMinTVL                  *float64                   `json:"pool_min_tvl"`
	PoolMinVolume               *float64                   `json:"pool_min_volume"`
	PoolMinFeeRate              *float64                   `json:"pool_min_fee_rate"`
	PoolMinActiveLiquidityRatio *float64                   `json:"pool_min_active_liquidity_ratio"`
	PoolIntensity               *string                    `json:"pool_intensity"`
}

type smartMoneyGoldenDogTestRequest struct {
	InitData  string `json:"initData"`
	Chain     string `json:"chain"`
	Mode      string `json:"mode"`
	Intensity string `json:"intensity"`
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

func (s *Server) handleSmartMoneyGoldenDogTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req smartMoneyGoldenDogTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	user, _, ok := authenticateSmartMoneyGoldenDogUser(w, strings.TrimSpace(req.InitData))
	if !ok {
		return
	}

	barkStatus, err := smgd.ResolveUserBarkStatus(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "failed to load bark status", http.StatusInternalServerError)
		return
	}
	if !barkStatus.Ready {
		http.Error(w, "bark not ready", http.StatusBadRequest)
		return
	}

	mode := normalizeSmartMoneyGoldenDogMode(req.Mode)
	chain := strings.ToUpper(normalizeSmartMoneyGoldenDogChain(req.Chain))
	intensity := smgd.NormalizeBarkIntensity(req.Intensity)
	title, body := buildSmartMoneyGoldenDogTestMessage(mode, chain, intensity)
	if err := notify.SendBarkWithConfig(title, body, smgd.BarkConfigForIntensity(barkStatus.Config, intensity)); err != nil {
		http.Error(w, "failed to send bark test", http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, smartMoneyGoldenDogMessageEnvelope{
		OK:      true,
		Message: "测试通知已发送",
	})
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

	s.writeSmartMoneyGoldenDogConfigEnvelope(w, r, user.ID, cfg)
}

func (s *Server) handlePostSmartMoneyGoldenDogConfig(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req smartMoneyGoldenDogUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	user, _, ok := authenticateSmartMoneyGoldenDogUser(w, strings.TrimSpace(req.InitData))
	if !ok {
		return
	}

	chain := normalizeSmartMoneyGoldenDogChain(req.Chain)
	repo := smgd.NewRepository()
	current, err := repo.GetOrCreateConfig(r.Context(), user.ID, chain)
	if err != nil {
		http.Error(w, "failed to load config", http.StatusInternalServerError)
		return
	}

	updates := make(map[string]any)
	applySmartMoneyGoldenDogFlatUpdates(updates, &req)
	applySmartMoneyGoldenDogNestedUpdates(updates, req.WalletMode, req.PoolMode)

	preview := *current
	applySmartMoneyGoldenDogPreview(&preview, updates)
	if preview.PoolEnabled && !smgd.HasPoolThresholds(preview) {
		http.Error(w, "pool mode requires at least one threshold", http.StatusBadRequest)
		return
	}

	cfg, err := repo.UpdateConfig(r.Context(), user.ID, chain, updates)
	if err != nil {
		http.Error(w, "failed to save config", http.StatusInternalServerError)
		return
	}

	s.writeSmartMoneyGoldenDogConfigEnvelope(w, r, user.ID, cfg)
}

func (s *Server) writeSmartMoneyGoldenDogConfigEnvelope(w http.ResponseWriter, r *http.Request, userID uint, cfg *models.SmartMoneyGoldenDogConfig) {
	barkStatus, err := smgd.ResolveUserBarkStatus(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to load bark status", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, smartMoneyGoldenDogConfigEnvelope{
		OK:                   true,
		Config:               cfg,
		BarkEnabled:          barkStatus.Enabled,
		BarkConfigured:       barkStatus.Configured,
		BarkReady:            barkStatus.Ready,
		AvailableIntensities: smgd.BarkIntensityOptions(),
	})
}

func applySmartMoneyGoldenDogFlatUpdates(updates map[string]any, req *smartMoneyGoldenDogUpdateRequest) {
	if req == nil {
		return
	}
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
	if req.WalletMinTotalAmountUSD != nil {
		updates["wallet_min_total_amount_usd"] = clampSmartMoneyGoldenDogMetricFloat(*req.WalletMinTotalAmountUSD)
	}
	if req.WalletIntensity != nil {
		updates["wallet_intensity"] = smgd.NormalizeBarkIntensity(*req.WalletIntensity)
	}
	if req.WalletIntensityMode != nil {
		updates["wallet_intensity_mode"] = smgd.NormalizeWalletIntensityMode(*req.WalletIntensityMode)
	}
	if req.WalletAmountIntensityTiers != nil {
		updates["wallet_amount_intensity_tiers"] = smgd.EncodeAmountIntensityTiers(req.WalletAmountIntensityTiers)
	}
	if req.PoolEnabled != nil {
		updates["pool_enabled"] = *req.PoolEnabled
	}
	if req.PoolCooldownMinutes != nil {
		updates["pool_cooldown_minutes"] = clampSmartMoneyGoldenDogCooldownMinutes(*req.PoolCooldownMinutes)
	}
	if req.PoolMinTotalFees != nil {
		updates["pool_min_total_fees"] = clampSmartMoneyGoldenDogMetricFloat(*req.PoolMinTotalFees)
	}
	if req.PoolMinTransactionCount != nil {
		updates["pool_min_transaction_count"] = clampSmartMoneyGoldenDogMetricCount(*req.PoolMinTransactionCount)
	}
	if req.PoolMinTVL != nil {
		updates["pool_min_tvl"] = clampSmartMoneyGoldenDogMetricFloat(*req.PoolMinTVL)
	}
	if req.PoolMinVolume != nil {
		updates["pool_min_volume"] = clampSmartMoneyGoldenDogMetricFloat(*req.PoolMinVolume)
	}
	if req.PoolMinFeeRate != nil {
		updates["pool_min_fee_rate"] = clampSmartMoneyGoldenDogMetricFloat(*req.PoolMinFeeRate)
	}
	if req.PoolMinActiveLiquidityRatio != nil {
		updates["pool_min_active_liquidity_ratio"] = clampSmartMoneyGoldenDogMetricFloat(*req.PoolMinActiveLiquidityRatio)
	}
	if req.PoolIntensity != nil {
		updates["pool_intensity"] = smgd.NormalizeBarkIntensity(*req.PoolIntensity)
	}
}

func applySmartMoneyGoldenDogNestedUpdates(updates map[string]any, walletMode *smartMoneyGoldenDogWalletModePayload, poolMode *smartMoneyGoldenDogPoolModePayload) {
	if walletMode != nil {
		if walletMode.Enabled != nil {
			updates["enabled"] = *walletMode.Enabled
		}
		if walletMode.MinWallets != nil {
			updates["min_wallets"] = clampSmartMoneyGoldenDogMinWallets(*walletMode.MinWallets)
		}
		if walletMode.WindowMinutes != nil {
			updates["window_minutes"] = clampSmartMoneyGoldenDogWindowMinutes(*walletMode.WindowMinutes)
		}
		if walletMode.CooldownMinutes != nil {
			updates["cooldown_minutes"] = clampSmartMoneyGoldenDogCooldownMinutes(*walletMode.CooldownMinutes)
		}
		if walletMode.MinTotalAmountUSD != nil {
			updates["wallet_min_total_amount_usd"] = clampSmartMoneyGoldenDogMetricFloat(*walletMode.MinTotalAmountUSD)
		}
		if walletMode.Intensity != nil {
			updates["wallet_intensity"] = smgd.NormalizeBarkIntensity(*walletMode.Intensity)
		}
		if walletMode.IntensityMode != nil {
			updates["wallet_intensity_mode"] = smgd.NormalizeWalletIntensityMode(*walletMode.IntensityMode)
		}
		if walletMode.AmountIntensityTiers != nil {
			updates["wallet_amount_intensity_tiers"] = smgd.EncodeAmountIntensityTiers(walletMode.AmountIntensityTiers)
		}
	}

	if poolMode != nil {
		if poolMode.Enabled != nil {
			updates["pool_enabled"] = *poolMode.Enabled
		}
		if poolMode.CooldownMinutes != nil {
			updates["pool_cooldown_minutes"] = clampSmartMoneyGoldenDogCooldownMinutes(*poolMode.CooldownMinutes)
		}
		if poolMode.MinTotalFees != nil {
			updates["pool_min_total_fees"] = clampSmartMoneyGoldenDogMetricFloat(*poolMode.MinTotalFees)
		}
		if poolMode.MinTransactionCount != nil {
			updates["pool_min_transaction_count"] = clampSmartMoneyGoldenDogMetricCount(*poolMode.MinTransactionCount)
		}
		if poolMode.MinTVL != nil {
			updates["pool_min_tvl"] = clampSmartMoneyGoldenDogMetricFloat(*poolMode.MinTVL)
		}
		if poolMode.MinVolume != nil {
			updates["pool_min_volume"] = clampSmartMoneyGoldenDogMetricFloat(*poolMode.MinVolume)
		}
		if poolMode.MinFeeRate != nil {
			updates["pool_min_fee_rate"] = clampSmartMoneyGoldenDogMetricFloat(*poolMode.MinFeeRate)
		}
		if poolMode.MinActiveLiquidityRatio != nil {
			updates["pool_min_active_liquidity_ratio"] = clampSmartMoneyGoldenDogMetricFloat(*poolMode.MinActiveLiquidityRatio)
		}
		if poolMode.Intensity != nil {
			updates["pool_intensity"] = smgd.NormalizeBarkIntensity(*poolMode.Intensity)
		}
	}
}

func applySmartMoneyGoldenDogPreview(cfg *models.SmartMoneyGoldenDogConfig, updates map[string]any) {
	if cfg == nil || len(updates) == 0 {
		return
	}

	for key, value := range updates {
		switch key {
		case "enabled":
			cfg.Enabled = value.(bool)
		case "min_wallets":
			cfg.MinWallets = value.(int)
		case "window_minutes":
			cfg.WindowMinutes = value.(int)
		case "cooldown_minutes":
			cfg.CooldownMinutes = value.(int)
		case "wallet_min_total_amount_usd":
			cfg.WalletMinTotalAmountUSD = value.(float64)
		case "wallet_intensity":
			cfg.WalletIntensity = value.(string)
		case "wallet_intensity_mode":
			cfg.WalletIntensityMode = value.(string)
		case "wallet_amount_intensity_tiers":
			cfg.WalletAmountIntensityTiers = value.(string)
		case "pool_enabled":
			cfg.PoolEnabled = value.(bool)
		case "pool_cooldown_minutes":
			cfg.PoolCooldownMinutes = value.(int)
		case "pool_min_total_fees":
			cfg.PoolMinTotalFees = value.(float64)
		case "pool_min_transaction_count":
			cfg.PoolMinTransactionCount = value.(int)
		case "pool_min_tvl":
			cfg.PoolMinTVL = value.(float64)
		case "pool_min_volume":
			cfg.PoolMinVolume = value.(float64)
		case "pool_min_fee_rate":
			cfg.PoolMinFeeRate = value.(float64)
		case "pool_min_active_liquidity_ratio":
			cfg.PoolMinActiveLiquidityRatio = value.(float64)
		case "pool_intensity":
			cfg.PoolIntensity = value.(string)
		}
	}
}

func buildSmartMoneyGoldenDogTestMessage(mode string, chain string, intensity string) (string, string) {
	modeLabel := "聪明钱聚集"
	if mode == "pool" {
		modeLabel = "池子参数"
	}
	title := "金狗通知测试"
	body := modeLabel + "模式测试已触发 | 链: " + chain + " | 强度: " + smartMoneyGoldenDogIntensityLabel(intensity)
	return title, body
}

func smartMoneyGoldenDogIntensityLabel(value string) string {
	switch smgd.NormalizeBarkIntensity(value) {
	case smgd.BarkIntensityPersistentRing:
		return "持续响铃"
	case smgd.BarkIntensityCriticalRing:
		return "静音强提醒"
	default:
		return "响铃"
	}
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
	if status, msg := requireModulePermission(check, models.AccessModuleSmartMoney); status != 0 {
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

func normalizeSmartMoneyGoldenDogMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pool", "pool_mode", "pool_metric":
		return "pool"
	default:
		return "wallet"
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

func clampSmartMoneyGoldenDogMetricFloat(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1_000_000_000_000 {
		return 1_000_000_000_000
	}
	return value
}

func clampSmartMoneyGoldenDogMetricCount(value int) int {
	if value < 0 {
		return 0
	}
	if value > 1_000_000_000 {
		return 1_000_000_000
	}
	return value
}
