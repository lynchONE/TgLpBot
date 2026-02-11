package web_server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"TgLpBot/base/database"
	"TgLpBot/base/models"

	"github.com/ethereum/go-ethereum/common"
	"gorm.io/gorm"
)

type smartMoneyFollowConfigGetResponse struct {
	Config models.SmartMoneyFollowConfig `json:"config"`
}

type smartMoneyFollowConfigUpsertRequest struct {
	InitData string `json:"initData"`

	Chain         string `json:"chain"`
	WalletAddress string `json:"wallet_address"`

	Enabled            *bool    `json:"enabled,omitempty"`
	MaxTotalAmountUSDT *float64 `json:"max_total_amount_usdt,omitempty"`
	PerTradeAmountUSDT *float64 `json:"per_trade_amount_usdt,omitempty"`

	DelayMinSeconds *int `json:"delay_min_seconds,omitempty"`
	DelayMaxSeconds *int `json:"delay_max_seconds,omitempty"`
}

type smartMoneyFollowConfigUpsertResponse struct {
	Config models.SmartMoneyFollowConfig `json:"config"`
}

type smartMoneyFollowConfigsResponse struct {
	Chain        string                          `json:"chain"`
	EnabledOnly  bool                            `json:"enabled_only"`
	EnabledCount int64                           `json:"enabled_count"`
	Configs      []models.SmartMoneyFollowConfig `json:"configs"`
}

func normalizeChain(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "bsc"
	}
	return v
}

func normalizeWalletAddress(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" || !common.IsHexAddress(v) {
		return "", false
	}
	return strings.ToLower(common.HexToAddress(v).Hex()), true
}

func parseBoolQuery(v string, def bool) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func parseIntQueryRange(v string, def int, min int, max int) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func clampInt(v int, min int, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func (s *Server) handleSmartMoneyFollowConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleSmartMoneyFollowConfigGet(w, r)
		return
	case http.MethodPost:
		s.handleSmartMoneyFollowConfigUpsert(w, r)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func (s *Server) handleSmartMoneyFollowConfigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	chain := normalizeChain(query.Get("chain"))
	enabledOnly := parseBoolQuery(query.Get("enabled_only"), true)
	limit := parseIntQueryRange(query.Get("limit"), 100, 1, 500)

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

	resp := smartMoneyFollowConfigsResponse{
		Chain:       chain,
		EnabledOnly: enabledOnly,
		Configs:     make([]models.SmartMoneyFollowConfig, 0),
	}

	countQ := database.DB.Model(&models.SmartMoneyFollowConfig{}).
		Where("user_id = ? AND chain = ? AND enabled = ?", user.ID, chain, true)
	if err := countQ.Count(&resp.EnabledCount).Error; err != nil {
		http.Error(w, "failed to count follow configs", http.StatusInternalServerError)
		return
	}

	q := database.DB.Where("user_id = ? AND chain = ?", user.ID, chain)
	if enabledOnly {
		q = q.Where("enabled = ?", true)
	}

	if err := q.Order("updated_at DESC").Limit(limit).Find(&resp.Configs).Error; err != nil {
		http.Error(w, "failed to load follow configs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleSmartMoneyFollowConfigGet(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	chain := normalizeChain(query.Get("chain"))

	walletAddr, ok := normalizeWalletAddress(query.Get("wallet_address"))
	if !ok {
		http.Error(w, "invalid wallet_address", http.StatusBadRequest)
		return
	}

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

	resp := smartMoneyFollowConfigGetResponse{
		Config: models.SmartMoneyFollowConfig{
			UserID:          user.ID,
			Chain:           chain,
			WalletAddress:   walletAddr,
			Enabled:         false,
			DelayMinSeconds: 0,
			DelayMaxSeconds: 60,
		},
	}

	var cfg models.SmartMoneyFollowConfig
	err = database.DB.Where("user_id = ? AND chain = ? AND wallet_address = ?", user.ID, chain, walletAddr).First(&cfg).Error
	if err == nil {
		resp.Config = cfg
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "failed to load follow config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleSmartMoneyFollowConfigUpsert(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req smartMoneyFollowConfigUpsertRequest
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	chain := normalizeChain(req.Chain)
	walletAddr, ok := normalizeWalletAddress(req.WalletAddress)
	if !ok {
		http.Error(w, "invalid wallet_address", http.StatusBadRequest)
		return
	}

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

	now := time.Now()

	var cfg models.SmartMoneyFollowConfig
	err = database.DB.Where("user_id = ? AND chain = ? AND wallet_address = ?", user.ID, chain, walletAddr).First(&cfg).Error
	hasExisting := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "failed to load follow config", http.StatusInternalServerError)
		return
	}

	enabled := cfg.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	maxTotal := 0.0
	perTrade := 0.0
	delayMin := 0
	delayMax := 60
	if hasExisting {
		maxTotal = cfg.MaxTotalAmountUSDT
		perTrade = cfg.PerTradeAmountUSDT
		delayMin = cfg.DelayMinSeconds
		delayMax = cfg.DelayMaxSeconds
	}

	if req.MaxTotalAmountUSDT != nil {
		if *req.MaxTotalAmountUSDT < 0 {
			http.Error(w, "max_total_amount_usdt must be >= 0", http.StatusBadRequest)
			return
		}
		maxTotal = *req.MaxTotalAmountUSDT
	}
	if req.PerTradeAmountUSDT != nil {
		if *req.PerTradeAmountUSDT < 0 {
			http.Error(w, "per_trade_amount_usdt must be >= 0", http.StatusBadRequest)
			return
		}
		perTrade = *req.PerTradeAmountUSDT
	}
	if req.DelayMinSeconds != nil {
		delayMin = clampInt(*req.DelayMinSeconds, 0, 60)
	}
	if req.DelayMaxSeconds != nil {
		delayMax = clampInt(*req.DelayMaxSeconds, 0, 60)
	}
	if delayMax < delayMin {
		delayMax, delayMin = delayMin, delayMax
	}
	if maxTotal > 0 && perTrade > maxTotal {
		http.Error(w, "per_trade_amount_usdt cannot exceed max_total_amount_usdt", http.StatusBadRequest)
		return
	}

	if hasExisting {
		prevEnabled := cfg.Enabled
		cfg.Enabled = enabled
		cfg.MaxTotalAmountUSDT = maxTotal
		cfg.PerTradeAmountUSDT = perTrade
		cfg.DelayMinSeconds = delayMin
		cfg.DelayMaxSeconds = delayMax

		if enabled {
			if !prevEnabled || cfg.LastEnabledAt == nil || cfg.LastEnabledAt.IsZero() {
				cfg.LastEnabledAt = &now
			}
			cfg.LastDisabledAt = nil
		} else {
			if prevEnabled {
				cfg.LastDisabledAt = &now
			}
		}

		if err := database.DB.Save(&cfg).Error; err != nil {
			http.Error(w, "failed to save follow config", http.StatusInternalServerError)
			return
		}
	} else {
		cfg = models.SmartMoneyFollowConfig{
			UserID:             user.ID,
			Chain:              chain,
			WalletAddress:      walletAddr,
			Enabled:            enabled,
			MaxTotalAmountUSDT: maxTotal,
			PerTradeAmountUSDT: perTrade,
			DelayMinSeconds:    delayMin,
			DelayMaxSeconds:    delayMax,
		}
		if enabled {
			cfg.LastEnabledAt = &now
		}
		if err := database.DB.Create(&cfg).Error; err != nil {
			http.Error(w, "failed to save follow config", http.StatusInternalServerError)
			return
		}
	}

	// Cancel pending jobs when disabled (does not close existing positions).
	if !enabled {
		_ = database.DB.Model(&models.SmartMoneyFollowJob{}).
			Where("user_id = ? AND chain = ? AND wallet_address = ? AND status = ?", user.ID, chain, walletAddr, "pending").
			Update("status", "canceled").Error
	}

	// Reload final config for response.
	var out models.SmartMoneyFollowConfig
	if err := database.DB.Where("user_id = ? AND chain = ? AND wallet_address = ?", user.ID, chain, walletAddr).First(&out).Error; err != nil {
		http.Error(w, "failed to load saved follow config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(smartMoneyFollowConfigUpsertResponse{Config: out})
}
