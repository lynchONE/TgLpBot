package web_server

import (
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/auto_lp"
	userSvc "TgLpBot/service/user"
)

type adminAutoLPStatsRequest struct {
	InitData string `json:"initData"`
	UserID   uint   `json:"userId"`
}

type adminAutoLPStatsResponse struct {
	Config    *models.AutoLPUserConfig   `json:"config"`
	Stats     *auto_lp.AutoLPStats       `json:"stats"`
	Formatted *adminAutoLPStatsFormatted `json:"formatted,omitempty"`
}

type adminAutoLPStatsFormatted struct {
	GasUSDT         string `json:"gas_usdt"`
	ProfitUSDT      string `json:"profit_usdt"`
	BestProfitUSDT  string `json:"best_profit_usdt"`
	WorstProfitUSDT string `json:"worst_profit_usdt"`
}

type adminAutoLPDisableRequest struct {
	InitData      string  `json:"initData"`
	UserID        uint    `json:"userId"`
	Reason        string  `json:"reason,omitempty"`
	GasMultiplier float64 `json:"gasMultiplier,omitempty"`
}

func formatWeiDecimals(weiStr string, decimals int) string {
	weiStr = strings.TrimSpace(weiStr)
	if weiStr == "" {
		weiStr = "0"
	}
	v, ok := new(big.Int).SetString(weiStr, 10)
	if !ok {
		return "0"
	}
	r := new(big.Rat).SetInt(v)
	denom := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	r.Quo(r, denom)
	return r.FloatString(decimals)
}

func (s *Server) handleAdminAutoLPStats(w http.ResponseWriter, r *http.Request) {
	initData := ""
	var userID uint

	switch r.Method {
	case http.MethodGet:
		initData = strings.TrimSpace(r.URL.Query().Get("initData"))
		if initData == "" {
			initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
		}
		userRaw := strings.TrimSpace(r.URL.Query().Get("userId"))
		if userRaw == "" {
			userRaw = strings.TrimSpace(r.URL.Query().Get("user_id"))
		}
		if userRaw != "" {
			if n, err := strconv.ParseUint(userRaw, 10, 64); err == nil {
				userID = uint(n)
			}
		}
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var req adminAutoLPStatsRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)
		userID = req.UserID
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if userID == 0 {
		http.Error(w, "missing userId", http.StatusBadRequest)
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
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return
	}

	accessService := userSvc.NewAccessService()
	if !accessService.IsAdminUser(user.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	adminService := auto_lp.NewAdminAutoLPService()
	cfg, stats, err := adminService.GetUserStats(userID)
	if err != nil {
		http.Error(w, "failed to load stats", http.StatusInternalServerError)
		return
	}

	var formatted *adminAutoLPStatsFormatted
	if stats != nil {
		formatted = &adminAutoLPStatsFormatted{
			GasUSDT:         formatWeiDecimals(stats.GasUSDTWei, 2),
			ProfitUSDT:      formatWeiDecimals(stats.ProfitUSDTWei, 2),
			BestProfitUSDT:  formatWeiDecimals(stats.BestProfitUSDTWei, 2),
			WorstProfitUSDT: formatWeiDecimals(stats.WorstProfitUSDTWei, 2),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(adminAutoLPStatsResponse{
		Config:    cfg,
		Stats:     stats,
		Formatted: formatted,
	})
}

func (s *Server) handleAdminAutoLPDisable(w http.ResponseWriter, r *http.Request) {
	initData := ""
	var userID uint
	reason := ""
	gasMultiplier := 1.0

	switch r.Method {
	case http.MethodGet:
		initData = strings.TrimSpace(r.URL.Query().Get("initData"))
		if initData == "" {
			initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
		}
		userRaw := strings.TrimSpace(r.URL.Query().Get("userId"))
		if userRaw == "" {
			userRaw = strings.TrimSpace(r.URL.Query().Get("user_id"))
		}
		if userRaw != "" {
			if n, err := strconv.ParseUint(userRaw, 10, 64); err == nil {
				userID = uint(n)
			}
		}
		reason = strings.TrimSpace(r.URL.Query().Get("reason"))
		if v := strings.TrimSpace(r.URL.Query().Get("gasMultiplier")); v != "" {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				gasMultiplier = n
			}
		}
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var req adminAutoLPDisableRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)
		userID = req.UserID
		reason = strings.TrimSpace(req.Reason)
		if req.GasMultiplier > 0 {
			gasMultiplier = req.GasMultiplier
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if userID == 0 {
		http.Error(w, "missing userId", http.StatusBadRequest)
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
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return
	}

	accessService := userSvc.NewAccessService()
	if !accessService.IsAdminUser(user.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	adminService := auto_lp.NewAdminAutoLPService()
	result, err := adminService.DisableUserAutoLP(userID, reason, gasMultiplier)
	if err != nil {
		http.Error(w, "failed to disable autolp", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
