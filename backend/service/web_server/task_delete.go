package web_server

import (
	"encoding/json"
	"net/http"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/service/strategy"
	userSvc "TgLpBot/service/user"
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
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req taskDeleteRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	if initData == "" {
		http.Error(w, "missing initData", http.StatusBadRequest)
		return
	}
	if req.TaskID == 0 {
		http.Error(w, "missing taskId", http.StatusBadRequest)
		return
	}
	if config.AppConfig == nil {
		http.Error(w, "config not loaded", http.StatusInternalServerError)
		return
	}

	parsed, err := VerifyTelegramWebAppInitData(initData, config.AppConfig.TelegramBotToken)
	if err != nil {
		http.Error(w, "invalid initData", http.StatusUnauthorized)
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
		Message: "\u4EFB\u52A1\u5DF2\u5220\u9664",
	})
}
