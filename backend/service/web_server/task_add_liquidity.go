package web_server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"TgLpBot/base/convert"
	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/strategy"
	"TgLpBot/service/trade"
	"TgLpBot/service/txexec"
)

type taskAddLiquidityRequest struct {
	InitData   string  `json:"initData"`
	TaskID     uint    `json:"taskId"`
	AmountUSDT float64 `json:"amountUsdt"`
}

type taskAddLiquidityResponse struct {
	OK       bool     `json:"ok"`
	TaskID   uint     `json:"task_id"`
	TxHashes []string `json:"tx_hashes,omitempty"`
	Pending  bool     `json:"pending"`
	Message  string   `json:"message,omitempty"`
}

func (s *Server) handleTaskAddLiquidity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req taskAddLiquidityRequest
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
	if req.AmountUSDT <= 0 {
		http.Error(w, "amountUsdt must be positive", http.StatusBadRequest)
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

	if task.Status == models.StrategyStatusStopped || task.Status == models.StrategyStatusStopping {
		http.Error(w, "task is stopped or stopping", http.StatusBadRequest)
		return
	}

	// Validate that the task has an existing position to increase
	hasV3 := strings.TrimSpace(task.V3TokenID) != "" && strings.TrimSpace(task.V3TokenID) != "0"
	hasV4 := strings.TrimSpace(task.V4TokenID) != "" && strings.TrimSpace(task.V4TokenID) != "0"
	if !hasV3 && !hasV4 {
		http.Error(w, "task has no existing position, cannot add liquidity", http.StatusBadRequest)
		return
	}

	userID := user.ID
	taskID := req.TaskID
	amountUSDT := req.AmountUSDT
	exec := txexec.Default()

	// Use a channel to capture the result so errors are returned to the caller.
	resultCh := make(chan error, 1)
	ok, tryErr := exec.TryRunTask(task.UserID, task.WalletID, task.WalletAddress, func(_ string) {
		liqSvc := liquidity.NewLiquidityService()
		increaseRes, increaseErr := liqSvc.IncreaseLiquidityForTask(userID, task, amountUSDT)

		if increaseErr != nil {
			log.Printf("[WebAPI] add_liquidity (increase) failed: task_id=%d err=%v", taskID, increaseErr)
			resultCh <- increaseErr
			return
		}

		// Update task with new liquidity info (no tokenId change since we're increasing existing position)
		updates := map[string]interface{}{
			"amount_usdt": task.AmountUSDT + amountUSDT,
		}
		if increaseRes != nil && increaseRes.CurrentLiquidity != "" {
			updates["current_liquidity"] = increaseRes.CurrentLiquidity
		}
		_ = taskService.Update(userID, taskID, updates)

		// Update TradeRecord.OpenUSDTSpent so PnL calculations reflect the additional investment
		deltaWei, convErr := convert.FloatUSDTToWei(amountUSDT)
		if convErr == nil && deltaWei != nil && deltaWei.Sign() > 0 {
			if tradeErr := trade.NewTradeRecordService().AddToOpenUSDTSpent(task, deltaWei); tradeErr != nil {
				log.Printf("[WebAPI] add_liquidity: update trade record failed: task_id=%d err=%v", taskID, tradeErr)
			}
		}

		if s != nil && s.Realtime != nil {
			s.Realtime.InvalidateUser(userID)
		}
		resultCh <- nil
	})

	if tryErr != nil {
		http.Error(w, "failed to schedule add liquidity: "+tryErr.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "wallet is busy, please try again later", http.StatusConflict)
		return
	}

	// Wait for the goroutine to finish (up to 3 minutes) so we can return the real result.
	select {
	case opErr := <-resultCh:
		if opErr != nil {
			http.Error(w, "补充流动性失败: "+opErr.Error(), http.StatusBadRequest)
			return
		}
	case <-time.After(3 * time.Minute):
		// Still running — return a pending response
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(taskAddLiquidityResponse{
			OK:      true,
			TaskID:  req.TaskID,
			Pending: true,
			Message: "操作时间较长，请稍后刷新查看结果",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(taskAddLiquidityResponse{
		OK:      true,
		TaskID:  req.TaskID,
		Pending: false,
		Message: "补充流动性成功",
	})
}
