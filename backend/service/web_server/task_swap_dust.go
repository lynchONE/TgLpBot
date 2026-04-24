package web_server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"TgLpBot/service/liquidity"
	"TgLpBot/service/strategy"
	"TgLpBot/service/txexec"
)

type taskSwapDustRequest struct {
	InitData string `json:"initData"`
	TaskID   uint   `json:"taskId"`
}

type taskSwapDustResponse struct {
	OK       bool     `json:"ok"`
	TaskID   uint     `json:"task_id"`
	TxHashes []string `json:"tx_hashes,omitempty"`
	Message  string   `json:"message,omitempty"`
}

func (s *Server) handleTaskSwapDust(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "请求方法不允许", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req taskSwapDustRequest
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

	userID := user.ID
	taskID := req.TaskID
	exec := txexec.Default()
	ok, err := exec.TryRunTask(task.UserID, task.WalletID, task.WalletAddress, func(_ string) {
		liqSvc := liquidity.NewLiquidityService()
		txHashes, swapErr := liqSvc.SwapTaskDustToUSDT(userID, task)

		if swapErr != nil {
			log.Printf("[WebAPI] swap_dust failed: task_id=%d err=%v txHashes=%v", taskID, swapErr, txHashes)
		}

		if s != nil && s.Realtime != nil {
			s.Realtime.InvalidateUser(userID)
		}
	})

	if err != nil {
		http.Error(w, "提交兑换残余失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "钱包正在处理其他交易，请稍后再试", http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(taskSwapDustResponse{
		OK:      true,
		TaskID:  req.TaskID,
		Message: "兑换残余已提交，正在处理中",
	})
}

func formatSwapDustMessage(err error) string {
	if err == nil {
		return "残余已兑换为 USDT"
	}
	return "兑换残余时发生错误: " + err.Error()
}
