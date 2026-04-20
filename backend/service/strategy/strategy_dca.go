package strategy

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/txexec"
	"fmt"
	"log"
	"strings"
	"time"
)

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

	// Task is being torn down — cancel any remaining batches.
	if strings.TrimSpace(task.ExitPendingAction) != "" || task.RebalancePending {
		s.cancelRemainingDCA(task, "任务正在停止或再平衡，剩余批次已取消")
		return
	}

	// Price left the range — cancel remaining batches.
	if !inRange {
		s.cancelRemainingDCA(task, "价格已跑出区间，剩余批次已取消")
		return
	}

	now := time.Now()
	if task.DCANextBatchAt == nil || now.Before(*task.DCANextBatchAt) {
		return
	}

	// Reentrancy guard — reuse the same inflight map other flows use so we don't double-submit.
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

	batchIdx := task.DCAExecutedCount // 0-based slot we're about to execute
	pct := pcts[batchIdx]
	amount := task.DCATotalAmountUSDT * pct / 100.0
	if amount <= 0 {
		s.cancelRemainingDCA(task, "分批金额计算异常，剩余批次已取消")
		return
	}

	// Mark inflight before dispatching so overlapping ticks bail out.
	s.inflightTasksMu.Lock()
	s.inflightTasks[task.ID] = now
	s.inflightTasksMu.Unlock()

	// Push next_batch_at forward to prevent re-entry from the next tick while this batch runs.
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
				log.Printf("[Strategy] DCA 任务 #%d 批次 %d panic: %v", taskID, batchIdx+1, r)
			}
		}()
		s.runDCABatchAttempt(taskID, userID, batchIdx, amount, len(pcts))
	})
	if err != nil {
		s.inflightTasksMu.Lock()
		delete(s.inflightTasks, task.ID)
		s.inflightTasksMu.Unlock()
		log.Printf("[Strategy] DCA schedule failed: task_id=%d err=%v", task.ID, err)
		// Reset the hold so next tick can try again.
		task.DCANextBatchAt = &now
		_ = database.DB.Model(task).Update("dca_next_batch_at", &now).Error
		return
	}
	if !ok2 {
		// Wallet busy — back off; the next tick will retry.
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
		// State changed between tick and execution — skip.
		return
	}
	if strings.TrimSpace(task.ExitPendingAction) != "" || task.RebalancePending {
		s.cancelRemainingDCA(&task, "任务正在停止或再平衡，剩余批次已取消")
		return
	}

	batchNum := batchIdx + 1
	log.Printf("[Strategy] DCA 任务 #%d 开始第 %d/%d 批：$%.4f", task.ID, batchNum, total, amountUSDT)
	s.notify(task.UserID, fmt.Sprintf("💧 分批加仓 %d/%d 开始：$%.2f", batchNum, total, amountUSDT))

	res, err := s.liquidityService.IncreaseLiquidityForTask(task.UserID, &task, amountUSDT)
	if err != nil {
		log.Printf("[Strategy] DCA 任务 #%d 第 %d 批失败: %v", task.ID, batchNum, err)
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
		log.Printf("[Strategy] DCA 任务 #%d 第 %d 批 DB 更新失败 (链上已成功): %v", task.ID, batchNum, err)
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
	if task.DCANextBatchAt == nil {
		return
	}
	_ = database.DB.Model(task).Update("dca_next_batch_at", nil).Error
	task.DCANextBatchAt = nil
	s.notify(task.UserID, fmt.Sprintf("⚠️ %s", reason))
	log.Printf("[Strategy] DCA 任务 #%d 已取消后续批次: %s", task.ID, reason)
}
