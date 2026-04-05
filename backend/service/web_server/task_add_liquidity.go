package web_server

import (
	"encoding/json"
	"log"
	"math/big"
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

type taskAddLiquidityRunResult struct {
	res *liquidity.IncreaseLiquidityResult
	err error
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

	resultCh := make(chan taskAddLiquidityRunResult, 1)
	ok, tryErr := exec.TryRunTask(task.UserID, task.WalletID, task.WalletAddress, func(_ string) {
		liqSvc := liquidity.NewLiquidityService()
		increaseRes, increaseErr := liqSvc.IncreaseLiquidityForTask(userID, task, amountUSDT)
		if increaseErr != nil {
			log.Printf("[WebAPI] add_liquidity failed: task_id=%d err=%v", taskID, increaseErr)
			resultCh <- taskAddLiquidityRunResult{err: increaseErr}
			return
		}

		actualSpent := amountUSDT
		if increaseRes != nil && increaseRes.ActualStableSpent > 0 {
			actualSpent = increaseRes.ActualStableSpent
		}

		updates := map[string]interface{}{
			"amount_usdt": task.AmountUSDT + actualSpent,
		}
		if increaseRes != nil && increaseRes.CurrentLiquidity != "" {
			updates["current_liquidity"] = increaseRes.CurrentLiquidity
		}
		if increaseRes != nil && increaseRes.TickLower != nil && increaseRes.TickUpper != nil && *increaseRes.TickLower < *increaseRes.TickUpper {
			updates["tick_lower"] = *increaseRes.TickLower
			updates["tick_upper"] = *increaseRes.TickUpper
		}
		_ = taskService.Update(userID, taskID, updates)

		var deltaWei *big.Int
		if increaseRes != nil && increaseRes.ActualStableSpentWei != nil && increaseRes.ActualStableSpentWei.Sign() > 0 {
			deltaWei = increaseRes.ActualStableSpentWei
		} else if conv, convErr := convert.FloatUSDTToWei(actualSpent); convErr == nil && conv != nil && conv.Sign() > 0 {
			deltaWei = conv
		}

		if tradeErr := trade.NewTradeRecordService().ApplyAddLiquidityDelta(
			task,
			deltaWei,
			func() *big.Int {
				if increaseRes != nil {
					return increaseRes.GasSpentWei
				}
				return nil
			}(),
			func() *big.Int {
				if increaseRes != nil {
					return increaseRes.Dust0Wei
				}
				return nil
			}(),
			func() *big.Int {
				if increaseRes != nil {
					return increaseRes.Dust1Wei
				}
				return nil
			}(),
		); tradeErr != nil {
			log.Printf("[WebAPI] add_liquidity: update trade record failed: task_id=%d err=%v", taskID, tradeErr)
		}

		if s != nil && s.Realtime != nil {
			s.Realtime.InvalidateUser(userID)
		}
		resultCh <- taskAddLiquidityRunResult{res: increaseRes}
	})

	if tryErr != nil {
		http.Error(w, "failed to schedule add liquidity: "+tryErr.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "wallet is busy, please try again later", http.StatusConflict)
		return
	}

	var runRes taskAddLiquidityRunResult
	select {
	case runRes = <-resultCh:
		if runRes.err != nil {
			http.Error(w, "add liquidity failed: "+runRes.err.Error(), http.StatusBadRequest)
			return
		}
	case <-time.After(3 * time.Minute):
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(taskAddLiquidityResponse{
			OK:      true,
			TaskID:  req.TaskID,
			Pending: true,
			Message: "operation is still running, please refresh later",
		})
		return
	}

	resp := taskAddLiquidityResponse{
		OK:      true,
		TaskID:  req.TaskID,
		Pending: false,
		Message: "add liquidity succeeded",
	}
	if runRes.res != nil && strings.TrimSpace(runRes.res.TxHash) != "" {
		resp.TxHashes = []string{strings.TrimSpace(runRes.res.TxHash)}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
