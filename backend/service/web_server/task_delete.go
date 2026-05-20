package web_server

import (
	"TgLpBot/base/models"
	"encoding/json"
	"net/http"
	"strings"

	"TgLpBot/service/strategy"
)

type taskDeleteRequest struct {
	InitData string `json:"initData"`
	TaskID   uint   `json:"taskId"`
}

type taskDeleteResponse struct {
	OK      bool   `json:"ok"`
	TaskID  uint   `json:"task_id"`
	Message string `json:"message,omitempty"`
}

func (s *Server) handleTaskDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "请求方法不允许", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req taskDeleteRequest
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
	if status, msg := requireModulePermission(check, models.AccessModulePositions); status != 0 {
		http.Error(w, msg, status)
		return
	}

	taskService := strategy.NewStrategyTaskService()
	if err := taskService.Delete(user.ID, req.TaskID); err != nil {
		http.Error(w, "delete task failed", http.StatusInternalServerError)
		return
	}

	if s != nil && s.Realtime != nil {
		s.Realtime.InvalidateUser(user.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(taskDeleteResponse{
		OK:      true,
		TaskID:  req.TaskID,
		Message: "任务已删除",
	})
}
