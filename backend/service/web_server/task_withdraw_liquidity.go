package web_server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/strategy"
	"TgLpBot/service/txexec"
)

type taskWithdrawLiquidityRequest struct {
	InitData string `json:"initData"`
	TaskID   uint   `json:"taskId"`
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
		http.Error(w, "任务已停止", http.StatusBadRequest)
		return
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
		txHashes, exitErr := liqSvc.WithdrawTaskLiquidityOnly(userID, task)

		if exitErr != nil {
			log.Printf("[WebAPI] withdraw_liquidity failed: task_id=%d err=%v txHashes=%v", taskID, exitErr, txHashes)
			_ = taskService.Update(userID, taskID, map[string]interface{}{
				"status":        models.StrategyStatusError,
				"error_message": "撤出流动性失败: " + exitErr.Error(),
			})
			if s != nil && s.Realtime != nil {
				s.Realtime.InvalidateUser(userID)
			}
			return
		}

		// Mark task as stopped after withdrawal
		updates := map[string]interface{}{
			"status":            models.StrategyStatusStopped,
			"current_liquidity": "0",
			"error_message":     "",
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(taskWithdrawLiquidityResponse{
		OK:      true,
		TaskID:  req.TaskID,
		Pending: true,
		Message: "取回流动性已提交，正在撤出流动性",
	})
}

func formatWithdrawMessage(err error) string {
	if err == nil {
		return "流动性已撤出，未自动兑换为稳定币"
	}
	return "撤出流动性时发生错误: " + err.Error()
}
