package strategy

import (
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/trade"
	"TgLpBot/service/txexec"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"
)

var dcaRetrySchedule = []time.Duration{
	500 * time.Millisecond,
	1 * time.Second,
	2 * time.Second,
	3 * time.Second,
	5 * time.Second,
	10 * time.Second,
}

func dcaRetryDelay(attempt int) (time.Duration, bool) {
	if attempt <= 0 {
		attempt = 1
	}
	idx := attempt - 1
	if idx < 0 || idx >= len(dcaRetrySchedule) {
		return 0, false
	}
	return dcaRetrySchedule[idx], true
}

func isRetryableDCASlippageError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	if text == "" {
		return false
	}
	markers := []string{
		"slippage",
		"price move",
		"price moved",
		"too little received",
		"insufficient_output_amount",
		"minimum amount",
		"maximum amount exceeded",
		"maximumamountexceeded",
		"0x31e30ad0",
	}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func (s *StrategyService) scheduleDCARetryWake(taskID uint, userID uint, delay time.Duration) {
	if taskID == 0 || userID == 0 {
		return
	}
	if delay < 0 {
		delay = 0
	}
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-timer.C:
		case <-s.stopChan:
			return
		}

		var task models.StrategyTask
		if err := database.DB.Where("id = ? AND user_id = ?", taskID, userID).First(&task).Error; err != nil {
			log.Printf("[Strategy] load task for scheduled DCA retry wake failed: task_id=%d user_id=%d err=%v", taskID, userID, err)
			return
		}
		if !task.DCAEnabled {
			return
		}

		currentTick, err := s.getCurrentTick(&task)
		if err != nil {
			log.Printf("[Strategy] scheduled DCA retry wake failed to load tick: task_id=%d err=%v", taskID, err)
			return
		}
		inRange := currentTick >= task.TickLower && currentTick <= task.TickUpper
		s.processDCABatch(&task, inRange)
	}()
}

// processDCABatch advances or cancels a task's DCA plan.
// Must be called inside StrategyService.handleRunningTask after currentTick/inRange
// have been computed (we reuse that work rather than duplicating RPC calls).
func (s *StrategyService) processDCABatch(task *models.StrategyTask, inRange bool) {
	if task == nil || !task.DCAEnabled {
		return
	}
	pcts, ok := ParseDCAPercentages(task.DCAPercentagesJSON)
	if !ok {
		return
	}
	if task.DCAExecutedCount >= len(pcts) {
		return
	}

	if strings.TrimSpace(task.ExitPendingAction) != "" || task.RebalancePending {
		s.cancelRemainingDCA(task, "任务正在停止或再平衡，剩余批次已取消")
		return
	}

	if !inRange {
		s.cancelRemainingDCA(task, "价格已跑出区间，剩余批次已取消")
		return
	}

	now := time.Now()
	if task.DCANextBatchAt == nil || now.Before(*task.DCANextBatchAt) {
		return
	}

	s.inflightTasksMu.Lock()
	if submitTime, exists := s.inflightTasks[task.ID]; exists {
		if now.Sub(submitTime) < 10*time.Minute {
			s.inflightTasksMu.Unlock()
			return
		}
		delete(s.inflightTasks, task.ID)
	}
	s.inflightTasksMu.Unlock()

	exec := txexec.Default()
	if exec == nil {
		return
	}

	batchIdx := task.DCAExecutedCount
	pct := pcts[batchIdx]
	amount := task.DCATotalAmountUSDT * pct / 100.0
	if amount <= 0 {
		s.cancelRemainingDCA(task, "分批金额计算异常，剩余批次已取消")
		return
	}

	s.inflightTasksMu.Lock()
	s.inflightTasks[task.ID] = now
	s.inflightTasksMu.Unlock()

	hold := now.Add(5 * time.Minute)
	task.DCANextBatchAt = &hold
	_ = database.DB.Model(task).Update("dca_next_batch_at", &hold).Error

	taskID := task.ID
	userID := task.UserID
	ok2, err := exec.TryRunTask(userID, task.WalletID, task.WalletAddress, func(_ string) {
		defer func() {
			s.inflightTasksMu.Lock()
			delete(s.inflightTasks, taskID)
			s.inflightTasksMu.Unlock()
			if r := recover(); r != nil {
				log.Printf("[Strategy] DCA task #%d batch %d panic: %v", taskID, batchIdx+1, r)
			}
		}()
		s.runDCABatchAttempt(taskID, userID, batchIdx, amount, len(pcts))
	})
	if err != nil {
		s.inflightTasksMu.Lock()
		delete(s.inflightTasks, task.ID)
		s.inflightTasksMu.Unlock()
		log.Printf("[Strategy] DCA schedule failed: task_id=%d err=%v", task.ID, err)
		task.DCANextBatchAt = &now
		_ = database.DB.Model(task).Update("dca_next_batch_at", &now).Error
		return
	}
	if !ok2 {
		s.inflightTasksMu.Lock()
		delete(s.inflightTasks, task.ID)
		s.inflightTasksMu.Unlock()
		retryAt := now.Add(15 * time.Second)
		task.DCANextBatchAt = &retryAt
		_ = database.DB.Model(task).Update("dca_next_batch_at", &retryAt).Error
	}
}

func (s *StrategyService) runDCABatchAttempt(taskID uint, userID uint, batchIdx int, amountUSDT float64, total int) {
	var task models.StrategyTask
	if err := database.DB.Where("id = ? AND user_id = ?", taskID, userID).First(&task).Error; err != nil {
		log.Printf("[Strategy] DCA load task failed: task_id=%d err=%v", taskID, err)
		return
	}
	if !task.DCAEnabled || task.DCAExecutedCount != batchIdx {
		return
	}
	if strings.TrimSpace(task.ExitPendingAction) != "" || task.RebalancePending {
		s.cancelRemainingDCA(&task, "任务正在停止或再平衡，剩余批次已取消")
		return
	}

	batchNum := batchIdx + 1
	if task.DCARetryCount > 0 {
		log.Printf("[Strategy] DCA task #%d retry %d/%d for batch %d/%d: $%.4f", task.ID, task.DCARetryCount, len(dcaRetrySchedule), batchNum, total, amountUSDT)
		s.notify(task.UserID, fmt.Sprintf("🔁 分批加仓 %d/%d 重试 %d/%d 开始：$%.2f", batchNum, total, task.DCARetryCount, len(dcaRetrySchedule), amountUSDT))
	} else {
		log.Printf("[Strategy] DCA task #%d start batch %d/%d: $%.4f", task.ID, batchNum, total, amountUSDT)
		s.notify(task.UserID, fmt.Sprintf("🧧 分批加仓 %d/%d 开始：$%.2f", batchNum, total, amountUSDT))
	}

	res, err := s.liquidityService.IncreaseLiquidityForTask(task.UserID, &task, amountUSDT)
	if err != nil {
		log.Printf("[Strategy] DCA task #%d batch %d failed: %v", task.ID, batchNum, err)
		if isRetryableDCASlippageError(err) {
			nextAttempt := task.DCARetryCount + 1
			if delay, ok := dcaRetryDelay(nextAttempt); ok {
				nextAt := time.Now().Add(delay)
				updates := map[string]interface{}{
					"dca_retry_count":   nextAttempt,
					"dca_next_batch_at": &nextAt,
				}
				if updateErr := database.DB.Model(&task).Updates(updates).Error; updateErr != nil {
					log.Printf("[Strategy] DCA task #%d batch %d failed to persist retry state: %v", task.ID, batchNum, updateErr)
					s.cancelRemainingDCA(&task, fmt.Sprintf("分批加仓第 %d 批记录重试状态失败：%v", batchNum, updateErr))
					return
				}
				task.DCARetryCount = nextAttempt
				task.DCANextBatchAt = &nextAt
				s.notify(task.UserID, fmt.Sprintf("⚠️ 分批加仓 %d/%d 因滑点不足失败，将在 %s 后重试（%d/%d）：%v", batchNum, total, formatDCAInterval(delay.Seconds()), nextAttempt, len(dcaRetrySchedule), err))
				s.scheduleDCARetryWake(task.ID, task.UserID, delay)
				return
			}
			s.cancelRemainingDCA(&task, fmt.Sprintf("分批加仓第 %d 批因滑点连续重试 %d 次后仍失败，已终止：%v", batchNum, len(dcaRetrySchedule), err))
			return
		}
		s.cancelRemainingDCA(&task, fmt.Sprintf("分批加仓第 %d 批失败：%v", batchNum, err))
		return
	}

	spent := amountUSDT
	if res != nil && res.ActualStableSpent > 0 {
		spent = res.ActualStableSpent
	}

	now := time.Now()
	newExecuted := batchIdx + 1
	updates := map[string]interface{}{
		"dca_executed_count": newExecuted,
		"dca_retry_count":    0,
		"amount_usdt":        task.AmountUSDT + spent,
	}
	if res != nil && res.CurrentLiquidity != "" {
		updates["current_liquidity"] = res.CurrentLiquidity
	}
	if newExecuted >= total {
		updates["dca_next_batch_at"] = nil
	} else {
		next := now.Add(time.Duration(task.DCAIntervalSeconds * float64(time.Second)))
		updates["dca_next_batch_at"] = &next
	}
	if err := database.DB.Model(&task).Updates(updates).Error; err != nil {
		log.Printf("[Strategy] DCA task #%d batch %d DB update failed after on-chain success: %v", task.ID, batchNum, err)
	}

	var deltaWei *big.Int
	if res != nil && res.ActualStableSpentWei != nil && res.ActualStableSpentWei.Sign() > 0 {
		deltaWei = res.ActualStableSpentWei
	} else if conv, convErr := convert.FloatUSDTToWei(spent); convErr == nil && conv != nil && conv.Sign() > 0 {
		deltaWei = conv
	}
	extraDust := []models.TradeRecordDustAsset(nil)
	if res != nil {
		extraDust = res.ExtraDust
	}
	if tradeErr := trade.NewTradeRecordService().ApplyAddLiquidityDelta(
		&task,
		deltaWei,
		func() *big.Int {
			if res != nil {
				return res.GasSpentWei
			}
			return nil
		}(),
		func() *big.Int {
			if res != nil {
				return res.Dust0Wei
			}
			return nil
		}(),
		func() *big.Int {
			if res != nil {
				return res.Dust1Wei
			}
			return nil
		}(),
		extraDust...,
	); tradeErr != nil {
		log.Printf("[Strategy] DCA task #%d batch %d failed to update trade record: %v", task.ID, batchNum, tradeErr)
	}

	if newExecuted >= total {
		s.notify(task.UserID, fmt.Sprintf("✅ 分批加仓完成：共 %d/%d 批，累计投入约 $%.2f", newExecuted, total, task.AmountUSDT+spent))
	} else {
		s.notify(task.UserID, fmt.Sprintf("✅ 分批加仓 %d/%d 完成（本批 $%.2f），下一批 %s 后执行", newExecuted, total, spent, formatDCAInterval(task.DCAIntervalSeconds)))
	}
}

// formatDCAInterval renders an interval like "30s" or "300ms" depending on magnitude.
func formatDCAInterval(seconds float64) string {
	if seconds < 1 {
		ms := int(seconds*1000 + 0.5)
		return fmt.Sprintf("%dms", ms)
	}
	if seconds == float64(int(seconds)) {
		return fmt.Sprintf("%ds", int(seconds))
	}
	return fmt.Sprintf("%.1fs", seconds)
}

func (s *StrategyService) cancelRemainingDCA(task *models.StrategyTask, reason string) {
	if task == nil {
		return
	}
	if task.DCANextBatchAt == nil && task.DCARetryCount == 0 {
		return
	}

	if err := database.DB.Model(task).Updates(map[string]interface{}{
		"dca_next_batch_at": nil,
		"dca_retry_count":   0,
	}).Error; err != nil {
		log.Printf("[Strategy] DCA task #%d failed to clear remaining batches: %v", task.ID, err)
	}
	task.DCANextBatchAt = nil
	task.DCARetryCount = 0
	s.notify(task.UserID, fmt.Sprintf("ℹ️ %s", reason))
	log.Printf("[Strategy] DCA task #%d cancelled remaining batches: %s", task.ID, reason)
}
