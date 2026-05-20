package web_server

import (
	"TgLpBot/base/models"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"TgLpBot/service/strategy"
)

type taskUpdateModeRequest struct {
	InitData string `json:"initData"`
	TaskID   uint   `json:"taskId"`
	TaskMode string `json:"taskMode"`
}

type taskUpdateModeResponse struct {
	OK               bool      `json:"ok"`
	TaskID           uint      `json:"task_id"`
	TaskMode         string    `json:"task_mode"`
	Paused           bool      `json:"paused"`
	OutOfRangeMode   string    `json:"out_of_range_mode"`
	RebalanceEnabled bool      `json:"rebalance_enabled"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (s *Server) handleTaskUpdateMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "请求方法不允许", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req taskUpdateModeRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "请求 JSON 格式无效", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	req.TaskMode = strings.TrimSpace(req.TaskMode)
	if req.TaskID == 0 {
		http.Error(w, "缺少 taskId", http.StatusBadRequest)
		return
	}

	requestedMode := models.NormalizeStrategyTaskMode(req.TaskMode)
	if requestedMode == "" {
		http.Error(w, "invalid taskMode", http.StatusBadRequest)
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
	if status, msg := requireModulePermission(check, models.AccessModulePositions); status != 0 {
		http.Error(w, msg, status)
		return
	}

	taskService := strategy.NewStrategyTaskService()
	task, err := taskService.GetByID(user.ID, req.TaskID)
	if err != nil {
		http.Error(w, "任务不存在", http.StatusNotFound)
		return
	}

	now := time.Now()
	outOfRangeMode := models.ResolveStrategyOutOfRangeMode(task)
	paused := false
	switch requestedMode {
	case models.StrategyTaskModePause:
		paused = true
	default:
		resolvedMode := models.NormalizeStrategyOutOfRangeMode(requestedMode)
		if resolvedMode == "" {
			http.Error(w, "invalid taskMode", http.StatusBadRequest)
			return
		}
		outOfRangeMode = resolvedMode
	}

	updates := map[string]interface{}{
		"paused":             paused,
		"out_of_range_since": nil,
		"out_of_range_mode":  string(outOfRangeMode),
		"rebalance_enabled":  models.RebalanceEnabledForOutOfRangeMode(outOfRangeMode),
	}
	if paused {
		updates["paused_at"] = &now
	} else {
		updates["paused_at"] = nil
	}

	if err := taskService.Update(user.ID, req.TaskID, updates); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if s != nil && s.Realtime != nil {
		s.Realtime.InvalidateUser(user.ID)
	}

	taskMode := string(outOfRangeMode)
	if paused {
		taskMode = models.StrategyTaskModePause
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(taskUpdateModeResponse{
		OK:               true,
		TaskID:           req.TaskID,
		TaskMode:         taskMode,
		Paused:           paused,
		OutOfRangeMode:   string(outOfRangeMode),
		RebalanceEnabled: models.RebalanceEnabledForOutOfRangeMode(outOfRangeMode),
		UpdatedAt:        now,
	})
}
