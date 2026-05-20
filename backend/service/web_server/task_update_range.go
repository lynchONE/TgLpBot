package web_server

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"

	"TgLpBot/base/models"
	"TgLpBot/service/pricing"
	"TgLpBot/service/strategy"
)

type taskUpdateRangeRequest struct {
	InitData      string   `json:"initData"`
	TaskID        uint     `json:"taskId"`
	RangeLowerPct float64  `json:"range_lower_pct"`
	RangeUpperPct float64  `json:"range_upper_pct"`
	AmountUSDT    *float64 `json:"amount_usdt,omitempty"`
}

type taskUpdateRangeResponse struct {
	OK            bool      `json:"ok"`
	TaskID        uint      `json:"task_id"`
	RangeLowerPct float64   `json:"range_lower_pct"`
	RangeUpperPct float64   `json:"range_upper_pct"`
	AmountUSDT    float64   `json:"amount_usdt,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (s *Server) handleTaskUpdateRange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "请求方法不允许", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req taskUpdateRangeRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "请求 JSON 格式无效", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	if req.TaskID == 0 {
		http.Error(w, "缺少 taskId", http.StatusBadRequest)
		return
	}
	if req.RangeLowerPct <= 0 || req.RangeUpperPct <= 0 || req.RangeLowerPct >= 100 || req.RangeUpperPct >= 100 {
		http.Error(w, "invalid range", http.StatusBadRequest)
		return
	}
	if req.AmountUSDT != nil {
		amt := *req.AmountUSDT
		if !isValidPositiveAmount(amt) {
			http.Error(w, "invalid amount", http.StatusBadRequest)
			return
		}
	}

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
	if status, msg := requireModulePermission(check, models.AccessModulePositions); status != 0 {
		http.Error(w, msg, status)
		return
	}

	taskService := strategy.NewStrategyTaskService()
	task, err := taskService.GetByID(user.ID, req.TaskID)
	if err != nil || task == nil {
		http.Error(w, "任务不存在", http.StatusNotFound)
		return
	}
	if task.Status == models.StrategyStatusStopped {
		http.Error(w, "任务已停止", http.StatusConflict)
		return
	}

	tickLowerPct, tickUpperPct := pricing.TickPercentagesFromStablePercentages(task, req.RangeLowerPct, req.RangeUpperPct)
	if tickLowerPct <= 0 || tickUpperPct <= 0 || tickLowerPct >= 100 || tickUpperPct >= 100 {
		http.Error(w, "invalid range", http.StatusBadRequest)
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"range_percentage":       (tickLowerPct + tickUpperPct) / 2.0,
		"range_lower_percentage": tickLowerPct,
		"range_upper_percentage": tickUpperPct,
	}
	if req.AmountUSDT != nil {
		updates["amount_usdt"] = *req.AmountUSDT
	}
	if err := taskService.Update(user.ID, req.TaskID, updates); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if s != nil && s.Realtime != nil {
		s.Realtime.InvalidateUser(user.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	amountUSDT := task.AmountUSDT
	if req.AmountUSDT != nil {
		amountUSDT = *req.AmountUSDT
	}
	_ = json.NewEncoder(w).Encode(taskUpdateRangeResponse{
		OK:            true,
		TaskID:        req.TaskID,
		RangeLowerPct: req.RangeLowerPct,
		RangeUpperPct: req.RangeUpperPct,
		AmountUSDT:    amountUSDT,
		UpdatedAt:     now,
	})
}

func isValidPositiveAmount(v float64) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return false
	}
	return v > 0
}
