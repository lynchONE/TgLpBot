package web_server

import (
	"encoding/json"
	"net/http"
	"strings"

	"TgLpBot/base/models"
	"TgLpBot/service/strategy"
)

type taskTriggerRebalanceRequest struct {
	InitData string `json:"initData"`
	TaskID   uint   `json:"taskId"`
}

type taskTriggerRebalanceResponse struct {
	OK      bool   `json:"ok"`
	TaskID  uint   `json:"task_id"`
	Pending bool   `json:"pending"`
	Message string `json:"message,omitempty"`
}

func (s *Server) handleTaskTriggerRebalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "请求方法不允许", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req taskTriggerRebalanceRequest
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

	if task.Status == models.StrategyStatusStopped || task.Status == models.StrategyStatusStopping {
		http.Error(w, "任务已停止或正在停止中", http.StatusBadRequest)
		return
	}

	// Trigger rebalance by setting exit_pending_action to "rebalance"
	currentLiq := strings.TrimSpace(task.CurrentLiquidity)
	if currentLiq == "" || currentLiq == "0" {
		http.Error(w, "没有可再平衡的流动性", http.StatusBadRequest)
		return
	}

	updates := map[string]interface{}{
		"exit_pending_action": strategy.ExitActionRebalance,
		"exit_pending_reason": "🔄 手动触发再平衡",
		"exit_retry_count":    0,
		"exit_next_retry_at":  nil,
		"exit_last_error":     "",
		"exit_give_up_at":     nil,
		"exit_gas_multiplier": 1.0,
		"out_of_range_since":  nil,
		"paused":              false,
		"paused_at":           nil,
		"error_message":       "",
	}
	if err := taskService.Update(user.ID, req.TaskID, updates); err != nil {
		http.Error(w, "触发再平衡失败："+err.Error(), http.StatusInternalServerError)
		return
	}

	if s != nil && s.Realtime != nil {
		s.Realtime.InvalidateUser(user.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(taskTriggerRebalanceResponse{
		OK:      true,
		TaskID:  req.TaskID,
		Pending: true,
		Message: "再平衡已触发，正在撤出并重新开仓",
	})
}
