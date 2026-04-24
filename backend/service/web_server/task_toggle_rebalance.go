package web_server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"TgLpBot/base/models"
	"TgLpBot/service/strategy"
)

type taskToggleRebalanceRequest struct {
	InitData         string `json:"initData"`
	TaskID           uint   `json:"taskId"`
	RebalanceEnabled bool   `json:"rebalanceEnabled"`
}

type taskToggleRebalanceResponse struct {
	OK               bool      `json:"ok"`
	TaskID           uint      `json:"task_id"`
	RebalanceEnabled bool      `json:"rebalance_enabled"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (s *Server) handleTaskToggleRebalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "请求方法不允许", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req taskToggleRebalanceRequest
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

	taskService := strategy.NewStrategyTaskService()
	task, err := taskService.GetByID(user.ID, req.TaskID)
	if err != nil {
		http.Error(w, "任务不存在", http.StatusNotFound)
		return
	}

	outOfRangeMode := models.StrategyOutOfRangeModeExitAll
	if req.RebalanceEnabled {
		outOfRangeMode = models.StrategyOutOfRangeModeRebalanceAll
	} else if models.ResolveStrategyOutOfRangeMode(task) == models.StrategyOutOfRangeModeRebalanceUpExitDown {
		outOfRangeMode = models.StrategyOutOfRangeModeExitAll
	}

	updates := map[string]interface{}{
		"rebalance_enabled": req.RebalanceEnabled,
		"out_of_range_mode": string(outOfRangeMode),
	}
	if err := taskService.Update(user.ID, req.TaskID, updates); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if s != nil && s.Realtime != nil {
		s.Realtime.InvalidateUser(user.ID)
	}

	now := time.Now()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(taskToggleRebalanceResponse{
		OK:               true,
		TaskID:           req.TaskID,
		RebalanceEnabled: req.RebalanceEnabled,
		UpdatedAt:        now,
	})
}
