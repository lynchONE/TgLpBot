package web_server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/strategy"
	"TgLpBot/service/txexec"
)

type taskWithdrawLiquidityRequest struct {
	InitData         string   `json:"initData"`
	TaskID           uint     `json:"taskId"`
	ExitPercent      *float64 `json:"exit_percent"`
	ExitPercentCamel *float64 `json:"exitPercent"`
}

type taskWithdrawLiquidityResponse struct {
	OK       bool     `json:"ok"`
	TaskID   uint     `json:"task_id"`
	TxHashes []string `json:"tx_hashes,omitempty"`
	Pending  bool     `json:"pending"`
	Message  string   `json:"message,omitempty"`
}

func (s *Server) handleTaskWithdrawLiquidity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "请求方法不允许", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req taskWithdrawLiquidityRequest
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

	if task.Status == models.StrategyStatusStopped {
		http.Error(w, "任务已停止", http.StatusBadRequest)
		return
	}
	if partialExit {
		if task.Status == models.StrategyStatusStopping {
			http.Error(w, "任务正在停止中，不能提交部分撤仓", http.StatusConflict)
			return
		}
		if strings.TrimSpace(task.ExitPendingAction) != "" {
			http.Error(w, "任务已有撤仓/再平衡流程处理中，不能提交部分撤仓", http.StatusConflict)
			return
		}
	}

	currentLiq := strings.TrimSpace(task.CurrentLiquidity)
	if currentLiq == "" || currentLiq == "0" {
		http.Error(w, "没有可撤出的流动性", http.StatusBadRequest)
		return
	}

	// Use txexec to serialize by wallet — TryRunTask launches a goroutine,
	// so we cannot write the HTTP response inside the callback.
	// Instead, return "pending" immediately and let the goroutine finish in the background.
	userID := user.ID
	taskID := req.TaskID
	exec := txexec.Default()
	ok, err := exec.TryRunTask(task.UserID, task.WalletID, task.WalletAddress, func(_ string) {
		liqSvc := liquidity.NewLiquidityService()
		var txHashes []string
		var exitErr error
		if partialExit {
			txHashes, exitErr = liqSvc.ExitTaskToUSDTWithOptions(userID, task, false, liquidity.TxOptions{ExitPercent: exitPercent})
		} else {
			txHashes, exitErr = liqSvc.WithdrawTaskLiquidityOnlyWithOptions(userID, task, liquidity.TxOptions{ExitPercent: exitPercent})
		}

		if exitErr != nil {
			log.Printf("[WebAPI] withdraw_liquidity failed: task_id=%d err=%v txHashes=%v", taskID, exitErr, txHashes)
			status := models.StrategyStatusError
			if partialExit {
				status = models.StrategyStatusRunning
			}
			_ = taskService.Update(userID, taskID, map[string]interface{}{
				"status":        status,
				"error_message": "撤出流动性失败: " + exitErr.Error(),
			})
			if s != nil && s.Realtime != nil {
				s.Realtime.InvalidateUser(userID)
			}
			return
		}

		status := models.StrategyStatusRunning
		if !partialExit {
			status = models.StrategyStatusStopped
		}
		updates := map[string]interface{}{
			"status":        status,
			"error_message": "",
		}
		if !partialExit {
			updates["current_liquidity"] = "0"
		}
		_ = taskService.Update(userID, taskID, updates)

		if s != nil && s.Realtime != nil {
			s.Realtime.InvalidateUser(userID)
		}
	})

	if err != nil {
		http.Error(w, "提交撤出流动性失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "钱包正在处理其他交易，请稍后再试", http.StatusConflict)
		return
	}

	message := "取回流动性请求已提交"
	if partialExit {
		message = fmt.Sprintf("已提交 %.4g%% 部分撤仓，撤出的资产会兑换为稳定币，任务会保留剩余仓位", exitPercentValue)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(taskWithdrawLiquidityResponse{
		OK:      true,
		TaskID:  req.TaskID,
		Pending: true,
		Message: message,
	})
}

func formatWithdrawMessage(err error) string {
	if err == nil {
		return "流动性已撤出，未自动兑换为稳定币"
	}
	return "撤出流动性时发生错误: " + err.Error()
}
