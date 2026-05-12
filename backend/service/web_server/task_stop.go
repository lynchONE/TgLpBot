package web_server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/strategy"
	"TgLpBot/service/txexec"
)

type taskStopRequest struct {
	InitData         string   `json:"initData"`
	TaskID           uint     `json:"taskId"`
	ExitPercent      *float64 `json:"exit_percent"`
	ExitPercentCamel *float64 `json:"exitPercent"`
}

type taskStopResponse struct {
	OK      bool   `json:"ok"`
	TaskID  uint   `json:"task_id"`
	Status  string `json:"status"`
	Pending bool   `json:"pending"`
	Message string `json:"message,omitempty"`
}

func requestExitPercent(snake *float64, camel *float64) (*float64, error) {
	if snake != nil && camel != nil && *snake != *camel {
		return nil, fmt.Errorf("exit_percent and exitPercent conflict")
	}
	if snake != nil {
		value := *snake
		return &value, nil
	}
	if camel != nil {
		value := *camel
		return &value, nil
	}
	return nil, nil
}

func (s *Server) handleTaskStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "请求方法不允许", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req taskStopRequest
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

	exitPercent, err := requestExitPercent(req.ExitPercent, req.ExitPercentCamel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	exitPercentValue, partialExit, err := liquidity.ValidateExitPercent(exitPercent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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

	if task.Status == models.StrategyStatusStopped {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(taskStopResponse{
			OK:      true,
			TaskID:  req.TaskID,
			Status:  "stopped",
			Pending: false,
			Message: "任务已停止",
		})
		return
	}

	if task.Status == models.StrategyStatusStopping {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(taskStopResponse{
			OK:      true,
			TaskID:  req.TaskID,
			Status:  "stopping",
			Pending: true,
			Message: "任务正在停止中",
		})
		return
	}

	pendingAction := strings.TrimSpace(task.ExitPendingAction)
	if pendingAction != "" {
		if partialExit {
			http.Error(w, "任务已有撤仓/再平衡流程处理中，不能提交部分撤仓", http.StatusConflict)
			return
		}
		updates := map[string]interface{}{
			"exit_pending_action":     strategy.ExitActionManualStop,
			"exit_pending_reason":     "🛑 手动停止",
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

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(taskStopResponse{
			OK:      true,
			TaskID:  req.TaskID,
			Status:  "stopping",
			Pending: true,
			Message: "任务正在撤出中",
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
				http.Error(w, "停止任务失败", http.StatusInternalServerError)
				return
			}
			if s != nil && s.Realtime != nil {
				s.Realtime.InvalidateUser(user.ID)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(taskStopResponse{
				OK:      true,
				TaskID:  req.TaskID,
				Status:  "stopped",
				Pending: false,
				Message: "已停止:当前处于再平衡重试中且无可撤出的流动性仓位",
			})
			return
		}

		if task.Status != models.StrategyStatusRunning && (currentLiq == "" || currentLiq == "0") {
			if err := taskService.Update(user.ID, req.TaskID, map[string]interface{}{
				"status":        models.StrategyStatusStopped,
				"error_message": "",
			}); err != nil {
				http.Error(w, "停止任务失败", http.StatusInternalServerError)
				return
			}
			if s != nil && s.Realtime != nil {
				s.Realtime.InvalidateUser(user.ID)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(taskStopResponse{
				OK:      true,
				TaskID:  req.TaskID,
				Status:  "stopped",
				Pending: false,
				Message: "已停止:当前无可撤出的流动性仓位",
			})
			return
		}

		http.Error(w, "无法停止：缺少仓位信息", http.StatusBadRequest)
		return
	}

	if partialExit {
		userID := user.ID
		taskID := req.TaskID
		exec := txexec.Default()
		ok, err := exec.TryRunTask(task.UserID, task.WalletID, task.WalletAddress, func(_ string) {
			liqSvc := liquidity.NewLiquidityService()
			txHashes, exitErr := liqSvc.ExitTaskToUSDTWithOptions(userID, task, false, liquidity.TxOptions{ExitPercent: exitPercent})
			if exitErr != nil {
				_ = taskService.Update(userID, taskID, map[string]interface{}{
					"status":        models.StrategyStatusRunning,
					"error_message": "部分撤仓失败: " + exitErr.Error(),
				})
				if s != nil && s.Realtime != nil {
					s.Realtime.InvalidateUser(userID)
				}
				return
			}
			_ = txHashes
			_ = taskService.Update(userID, taskID, map[string]interface{}{
				"status":             models.StrategyStatusRunning,
				"error_message":      "",
				"exit_retry_count":   0,
				"exit_next_retry_at": nil,
				"exit_last_error":    "",
				"exit_give_up_at":    nil,
			})
			if s != nil && s.Realtime != nil {
				s.Realtime.InvalidateUser(userID)
			}
		})
		if err != nil {
			http.Error(w, "提交部分撤仓失败："+err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "钱包正在处理其他交易，请稍后再试", http.StatusConflict)
			return
		}
		if s != nil && s.Realtime != nil {
			s.Realtime.InvalidateUser(user.ID)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(taskStopResponse{
			OK:      true,
			TaskID:  req.TaskID,
			Status:  string(task.Status),
			Pending: true,
			Message: fmt.Sprintf("已提交 %.4g%% 部分撤仓，撤出的资产会兑换为稳定币，任务会保留剩余仓位", exitPercentValue),
		})
		return
	}

	updates := map[string]interface{}{
		"exit_pending_action":     strategy.ExitActionManualStop,
		"exit_pending_reason":     "🛑 手动停止",
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(taskStopResponse{
		OK:      true,
		TaskID:  req.TaskID,
		Status:  "stopping",
		Pending: true,
		Message: "已发起停止, 正在撤出流动性并兑换成 USDT",
	})
}
