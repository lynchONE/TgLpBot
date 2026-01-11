package web_server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"TgLpBot/service/strategy"
)

type taskPauseRequest struct {
	InitData string `json:"initData"`
	TaskID   uint   `json:"taskId"`
	Paused   bool   `json:"paused"`
}

type taskPauseResponse struct {
	OK        bool      `json:"ok"`
	TaskID    uint      `json:"task_id"`
	Paused    bool      `json:"paused"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Server) handleTaskPause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req taskPauseRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	if req.TaskID == 0 {
		http.Error(w, "missing taskId", http.StatusBadRequest)
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
	if _, err := taskService.GetByID(user.ID, req.TaskID); err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"paused":             req.Paused,
		"out_of_range_since": nil,
	}
	if req.Paused {
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(taskPauseResponse{
		OK:        true,
		TaskID:    req.TaskID,
		Paused:    req.Paused,
		UpdatedAt: now,
	})
}
