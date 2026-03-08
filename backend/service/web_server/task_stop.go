package web_server

import (
	"encoding/json"
	"net/http"
	"strings"

	"TgLpBot/base/models"
	"TgLpBot/service/strategy"
	"TgLpBot/service/ws"
)

type taskStopRequest struct {
	InitData string `json:"initData"`
	TaskID   uint   `json:"taskId"`
}

type taskStopResponse struct {
	OK      bool   `json:"ok"`
	TaskID  uint   `json:"task_id"`
	Status  string `json:"status"`
	Pending bool   `json:"pending"`
	Message string `json:"message,omitempty"`
}

func (s *Server) handleTaskStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req taskStopRequest
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
	task, err := taskService.GetByID(user.ID, req.TaskID)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	if task.Status == models.StrategyStatusStopped {
		ws.SendProgress(user.ID, "close_position", req.TaskID, 3, 4, "done", "")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(taskStopResponse{
			OK:      true,
			TaskID:  req.TaskID,
			Status:  "stopped",
			Pending: false,
			Message: "\u4EFB\u52A1\u5DF2\u505C\u6B62",
		})
		return
	}

	if task.Status == models.StrategyStatusStopping {
		ws.SendProgress(user.ID, "close_position", req.TaskID, 1, 4, "active", "")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(taskStopResponse{
			OK:      true,
			TaskID:  req.TaskID,
			Status:  "stopping",
			Pending: true,
			Message: "\u4EFB\u52A1\u6B63\u5728\u505C\u6B62\u4E2D",
		})
		return
	}

	pendingAction := strings.TrimSpace(task.ExitPendingAction)
	if pendingAction != "" {
		updates := map[string]interface{}{
			"exit_pending_action":     strategy.ExitActionManualStop,
			"exit_pending_reason":     "\U0001F6D1 \u624B\u52A8\u505C\u6B62",
			"exit_retry_count":        0,
			"exit_next_retry_at":      nil,
			"exit_last_error":         "",
			"exit_give_up_at":         nil,
			"rebalance_pending":       false,
			"rebalance_retry_count":   0,
			"rebalance_next_retry_at": nil,
			"rebalance_last_error":    "",
			"error_message":           "",
			"paused":                  false,
		}
		if err := taskService.Update(user.ID, req.TaskID, updates); err != nil {
			http.Error(w, "failed to update task", http.StatusInternalServerError)
			return
		}
		if s != nil && s.Realtime != nil {
			s.Realtime.InvalidateUser(user.ID)
		}

		ws.SendProgress(user.ID, "close_position", req.TaskID, 1, 4, "active", "")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(taskStopResponse{
			OK:      true,
			TaskID:  req.TaskID,
			Status:  "stopping",
			Pending: true,
			Message: "\u4EFB\u52A1\u6B63\u5728\u64A4\u51FA\u4E2D",
		})
		return
	}

	currentLiq := strings.TrimSpace(task.CurrentLiquidity)
	poolVersion := strings.ToLower(strings.TrimSpace(task.PoolVersion))

	canExit := false
	switch poolVersion {
	case "v4":
		v4TokenId := strings.TrimSpace(task.V4TokenID)
		canExit = v4TokenId != "" && v4TokenId != "0" && currentLiq != "" && currentLiq != "0"
	default:
		v3TokenId := strings.TrimSpace(task.V3TokenID)
		canExit = v3TokenId != "" && v3TokenId != "0"
	}

	if !canExit {
		if task.RebalancePending && (currentLiq == "" || currentLiq == "0") {
			if err := taskService.Update(user.ID, req.TaskID, map[string]interface{}{
				"status":                  models.StrategyStatusStopped,
				"rebalance_pending":       false,
				"rebalance_retry_count":   0,
				"rebalance_next_retry_at": nil,
				"rebalance_last_error":    "",
				"error_message":           "",
			}); err != nil {
				http.Error(w, "failed to stop task", http.StatusInternalServerError)
				return
			}
			if s != nil && s.Realtime != nil {
				s.Realtime.InvalidateUser(user.ID)
			}
			ws.SendProgress(user.ID, "close_position", req.TaskID, 3, 4, "done", "")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(taskStopResponse{
				OK:      true,
				TaskID:  req.TaskID,
				Status:  "stopped",
				Pending: false,
				Message: "\u5DF2\u505C\u6B62:\u5F53\u524D\u5904\u4E8E\u518D\u5E73\u8861\u91CD\u8BD5\u4E2D\u4E14\u65E0\u53EF\u64A4\u51FA\u7684\u6D41\u52A8\u6027\u4ED3\u4F4D",
			})
			return
		}

		if task.Status != models.StrategyStatusRunning && (currentLiq == "" || currentLiq == "0") {
			if err := taskService.Update(user.ID, req.TaskID, map[string]interface{}{
				"status":        models.StrategyStatusStopped,
				"error_message": "",
			}); err != nil {
				http.Error(w, "failed to stop task", http.StatusInternalServerError)
				return
			}
			if s != nil && s.Realtime != nil {
				s.Realtime.InvalidateUser(user.ID)
			}
			ws.SendProgress(user.ID, "close_position", req.TaskID, 3, 4, "done", "")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(taskStopResponse{
				OK:      true,
				TaskID:  req.TaskID,
				Status:  "stopped",
				Pending: false,
				Message: "\u5DF2\u505C\u6B62:\u5F53\u524D\u65E0\u53EF\u64A4\u51FA\u7684\u6D41\u52A8\u6027\u4ED3\u4F4D",
			})
			return
		}

		http.Error(w, "cannot stop: missing position info", http.StatusBadRequest)
		return
	}

	updates := map[string]interface{}{
		"exit_pending_action":     strategy.ExitActionManualStop,
		"exit_pending_reason":     "\U0001F6D1 \u624B\u52A8\u505C\u6B62",
		"exit_retry_count":        0,
		"exit_next_retry_at":      nil,
		"exit_last_error":         "",
		"exit_give_up_at":         nil,
		"rebalance_pending":       false,
		"rebalance_retry_count":   0,
		"rebalance_next_retry_at": nil,
		"rebalance_last_error":    "",
		"error_message":           "",
		"paused":                  false,
	}
	if err := taskService.Update(user.ID, req.TaskID, updates); err != nil {
		http.Error(w, "failed to update task", http.StatusInternalServerError)
		return
	}

	if s != nil && s.Realtime != nil {
		s.Realtime.InvalidateUser(user.ID)
	}

	ws.SendProgress(user.ID, "close_position", req.TaskID, 1, 4, "active", "")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(taskStopResponse{
		OK:      true,
		TaskID:  req.TaskID,
		Status:  "stopping",
		Pending: true,
		Message: "\u5DF2\u53D1\u8D77\u505C\u6B62, \u6B63\u5728\u64A4\u51FA\u6D41\u52A8\u6027\u5E76\u5151\u6362\u6210 USDT",
	})
}
