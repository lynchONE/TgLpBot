package web_server

import (
	"encoding/json"
	"net/http"
	"strings"

	smfollow "TgLpBot/service/smart_money_follow"
)

type smartMoneyAutoFollowSaveRequest struct {
	ID                   uint     `json:"id"`
	Chain                string   `json:"chain"`
	TaskName             string   `json:"task_name"`
	TargetWalletAddress  string   `json:"target_wallet_address"`
	TargetWallets        []string `json:"target_wallet_addresses"`
	ExecutionWalletID    uint     `json:"execution_wallet_id"`
	ExecutionWalletAddr  string   `json:"execution_wallet_address"`
	ExecutionWalletIDs   []uint   `json:"execution_wallet_ids"`
	ExecutionWalletMode  string   `json:"execution_wallet_mode"`
	TriggerMode          string   `json:"trigger_mode"`
	TriggerMinWallets    int      `json:"trigger_min_wallets"`
	TriggerWindowSeconds int      `json:"trigger_window_seconds"`
	Enabled              bool     `json:"enabled"`
	AmountMode           string   `json:"amount_mode"`
	FixedAmountUSDT      float64  `json:"fixed_amount_usdt"`
	Ratio                float64  `json:"ratio"`
	DelayMode            string   `json:"delay_mode"`
	DelaySeconds         int      `json:"delay_seconds"`
	FollowClose          bool     `json:"follow_close"`
	RangeShiftGrids      int      `json:"range_shift_grids"`
	NotifyEnabled        bool     `json:"notify_enabled"`
	NotifyIntensity      string   `json:"notify_intensity"`
	TakeProfitUSDT       float64  `json:"take_profit_usdt"`
	StopLossUSDT         float64  `json:"stop_loss_usdt"`
}

type smartMoneyAutoFollowRequest struct {
	InitData string                          `json:"initData"`
	Action   string                          `json:"action"`
	Config   smartMoneyAutoFollowSaveRequest `json:"config"`
	ID       uint                            `json:"id"`
	Chain    string                          `json:"chain"`
}

func (s *Server) handleSmartMoneyAutoFollow(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetSmartMoneyAutoFollow(w, r)
	case http.MethodPost:
		s.handlePostSmartMoneyAutoFollow(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetSmartMoneyAutoFollow(w http.ResponseWriter, r *http.Request) {
	user, _, ok := authenticateSmartMoneyGoldenDogUser(w, initDataFromQuery(r))
	if !ok {
		return
	}
	envelope, err := smartMoneyFollowService().ListEnvelope(r.Context(), user.ID, r.URL.Query().Get("chain"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, envelope)
}

func (s *Server) handlePostSmartMoneyAutoFollow(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var req smartMoneyAutoFollowRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	user, _, ok := authenticateSmartMoneyGoldenDogUser(w, strings.TrimSpace(req.InitData))
	if !ok {
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "save", "":
		input := smfollow.SaveConfigInput{
			ID:                   req.Config.ID,
			Chain:                firstSmartMoneyAutoFollowChain(req.Config.Chain, req.Chain),
			TaskName:             req.Config.TaskName,
			TargetWalletAddress:  req.Config.TargetWalletAddress,
			TargetWallets:        req.Config.TargetWallets,
			ExecutionWalletID:    req.Config.ExecutionWalletID,
			ExecutionWalletAddr:  req.Config.ExecutionWalletAddr,
			ExecutionWalletIDs:   req.Config.ExecutionWalletIDs,
			ExecutionWalletMode:  req.Config.ExecutionWalletMode,
			TriggerMode:          req.Config.TriggerMode,
			TriggerMinWallets:    req.Config.TriggerMinWallets,
			TriggerWindowSeconds: req.Config.TriggerWindowSeconds,
			Enabled:              req.Config.Enabled,
			AmountMode:           req.Config.AmountMode,
			FixedAmountUSDT:      req.Config.FixedAmountUSDT,
			Ratio:                req.Config.Ratio,
			DelayMode:            req.Config.DelayMode,
			DelaySeconds:         req.Config.DelaySeconds,
			FollowClose:          req.Config.FollowClose,
			RangeShiftGrids:      req.Config.RangeShiftGrids,
			NotifyEnabled:        req.Config.NotifyEnabled,
			NotifyIntensity:      req.Config.NotifyIntensity,
			TakeProfitUSDT:       req.Config.TakeProfitUSDT,
			StopLossUSDT:         req.Config.StopLossUSDT,
		}
		cfg, err := smartMoneyFollowService().SaveConfig(r.Context(), user.ID, input)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"config": cfg,
		})
	case "delete":
		id := req.ID
		if id == 0 {
			id = req.Config.ID
		}
		if err := smartMoneyFollowService().DeleteConfig(r.Context(), user.ID, id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "delete_logs", "clear_logs":
		result, err := smartMoneyFollowService().DeleteLogs(r.Context(), user.ID, firstSmartMoneyAutoFollowChain(req.Chain, req.Config.Chain))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":               true,
			"deleted_jobs":     result.DeletedJobs,
			"deleted_attempts": result.DeletedAttempts,
		})
	case "recalculate_pnl", "recalc_pnl":
		id := req.ID
		if id == 0 {
			id = req.Config.ID
		}
		result, err := smartMoneyFollowService().RecalculateConfigPnL(r.Context(), user.ID, id, firstSmartMoneyAutoFollowChain(req.Chain, req.Config.Chain))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"status":    result.Status,
			"reason":    result.Reason,
			"triggered": result.Triggered,
			"reenabled": result.Reenabled,
		})
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
	}
}

func firstSmartMoneyAutoFollowChain(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return "bsc"
}

func smartMoneyFollowService() *smfollow.Service {
	if smFollowSvc == nil {
		smFollowSvc = smfollow.NewService()
	}
	return smFollowSvc
}
