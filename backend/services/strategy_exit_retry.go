package services

import (
	"TgLpBot/database"
	"TgLpBot/models"
	"fmt"
	"log"
	"strings"
	"time"
)

const (
	exitActionManualStop = "manual_stop"
	exitActionStopLoss   = "stoploss"
	exitActionRebalance  = "rebalance"

	exitMaxAttempts = 3
)

func exitRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 10 * time.Second
	case 2:
		return 30 * time.Second
	default:
		return 60 * time.Second
	}
}

func (s *StrategyService) requestExitToUSDT(task *models.StrategyTask, action string, reason string) {
	if task == nil {
		return
	}

	action = strings.TrimSpace(action)
	if action == "" {
		return
	}

	// After we give up, only a manual stop can reset and retry again.
	if task.ExitGiveUpAt != nil && action != exitActionManualStop {
		return
	}

	// If already pending, do nothing.
	if strings.TrimSpace(task.ExitPendingAction) != "" {
		return
	}

	updates := map[string]interface{}{
		"exit_pending_action": action,
		"exit_pending_reason": strings.TrimSpace(reason),
		"exit_retry_count":    0,
		"exit_next_retry_at":  nil,
		"exit_last_error":     "",
		"exit_give_up_at":     nil,
		"error_message":       "",
	}
	if err := database.DB.Model(task).Updates(updates).Error; err != nil {
		log.Printf("[Strategy] 任务 #%d 设置撤出重试状态失败: %v", task.ID, err)
		return
	}

	// Update in-memory task for this cycle.
	task.ExitPendingAction = action
	task.ExitPendingReason = strings.TrimSpace(reason)
	task.ExitRetryCount = 0
	task.ExitNextRetryAt = nil
	task.ExitLastError = ""
	task.ExitGiveUpAt = nil

	_ = s.processExitRetry(task)
}

// processExitRetry handles a pending exit flow (with max retry attempts).
// Returns true when the task processing should stop for this cycle.
func (s *StrategyService) processExitRetry(task *models.StrategyTask) bool {
	if task == nil {
		return false
	}

	action := strings.TrimSpace(task.ExitPendingAction)
	if action == "" {
		return false
	}

	now := time.Now()
	if task.ExitNextRetryAt != nil && now.Before(*task.ExitNextRetryAt) {
		return true
	}

	if task.ExitRetryCount >= exitMaxAttempts {
		s.giveUpExitRetry(task, fmt.Errorf("max attempts reached"))
		return true
	}

	attempt := task.ExitRetryCount + 1
	reason := strings.TrimSpace(task.ExitPendingReason)
	if reason == "" {
		reason = "撤出流动性"
	}

	if attempt == 1 {
		s.notify(task.UserID, fmt.Sprintf("%s，正在撤出流动性并兑换成 USDT...", reason))
	} else {
		s.notify(task.UserID, fmt.Sprintf("🔄 %s失败，正在第 %d/%d 次重试撤出并兑换 USDT...", reason, attempt, exitMaxAttempts))
	}

	txHashes, err := s.liquidityService.ExitTaskToUSDT(task.UserID, task, true)
	if err != nil {
		s.onExitAttemptFailed(task, attempt, err)
		return true
	}

	// Success: clear retry state and continue with post-exit action.
	s.clearExitRetryState(task)

	switch action {
	case exitActionRebalance:
		s.executeRebalanceAfterExit(task, now)
	case exitActionStopLoss:
		s.finishStopAfterExit(task, now, reason, txHashes)
	case exitActionManualStop:
		s.finishStopAfterExit(task, now, "🛑 手动停止", txHashes)
	default:
		// Unknown action, keep task running.
		log.Printf("[Strategy] 任务 #%d 撤出成功，但未知 exit_pending_action=%q，已清理重试状态", task.ID, action)
	}

	return true
}

func (s *StrategyService) onExitAttemptFailed(task *models.StrategyTask, attempt int, err error) {
	if task == nil {
		return
	}

	now := time.Now()
	errText := strings.TrimSpace(fmt.Sprintf("%v", err))

	if attempt >= exitMaxAttempts {
		updates := map[string]interface{}{
			"status":              models.StrategyStatusRunning,
			"exit_pending_action": "",
			"exit_pending_reason": "",
			"exit_retry_count":    attempt,
			"exit_next_retry_at":  nil,
			"exit_last_error":     errText,
			"exit_give_up_at":     &now,
			"error_message":       "",
		}
		_ = database.DB.Model(task).Updates(updates).Error

		task.Status = models.StrategyStatusRunning
		task.ExitPendingAction = ""
		task.ExitPendingReason = ""
		task.ExitRetryCount = attempt
		task.ExitNextRetryAt = nil
		task.ExitLastError = errText
		task.ExitGiveUpAt = &now

		s.notify(task.UserID, fmt.Sprintf("❌ 撤出连续失败 %d 次，已停止自动重试。\n任务保持运行中，可稍后手动停止再试。\n最后错误：%v", attempt, err))
		return
	}

	nextAt := now.Add(exitRetryDelay(attempt))
	updates := map[string]interface{}{
		"status":             models.StrategyStatusRunning,
		"exit_retry_count":   attempt,
		"exit_next_retry_at": &nextAt,
		"exit_last_error":    errText,
		"error_message":      "",
	}
	_ = database.DB.Model(task).Updates(updates).Error

	task.Status = models.StrategyStatusRunning
	task.ExitRetryCount = attempt
	task.ExitNextRetryAt = &nextAt
	task.ExitLastError = errText

	s.notify(task.UserID, fmt.Sprintf("❌ 撤出失败（%d/%d）：%v\n将在 %ds 后重试。", attempt, exitMaxAttempts, err, int(exitRetryDelay(attempt).Seconds())))
}

func (s *StrategyService) clearExitRetryState(task *models.StrategyTask) {
	if task == nil {
		return
	}

	updates := map[string]interface{}{
		"exit_pending_action": "",
		"exit_pending_reason": "",
		"exit_retry_count":    0,
		"exit_next_retry_at":  nil,
		"exit_last_error":     "",
		"exit_give_up_at":     nil,
	}
	_ = database.DB.Model(task).Updates(updates).Error

	task.ExitPendingAction = ""
	task.ExitPendingReason = ""
	task.ExitRetryCount = 0
	task.ExitNextRetryAt = nil
	task.ExitLastError = ""
	task.ExitGiveUpAt = nil
}

func (s *StrategyService) giveUpExitRetry(task *models.StrategyTask, err error) {
	if task == nil {
		return
	}

	now := time.Now()
	errText := strings.TrimSpace(fmt.Sprintf("%v", err))

	updates := map[string]interface{}{
		"status":              models.StrategyStatusRunning,
		"exit_pending_action": "",
		"exit_pending_reason": "",
		"exit_next_retry_at":  nil,
		"exit_last_error":     errText,
		"exit_give_up_at":     &now,
		"error_message":       "",
	}
	_ = database.DB.Model(task).Updates(updates).Error

	task.Status = models.StrategyStatusRunning
	task.ExitPendingAction = ""
	task.ExitPendingReason = ""
	task.ExitNextRetryAt = nil
	task.ExitLastError = errText
	task.ExitGiveUpAt = &now
}

func (s *StrategyService) executeRebalanceAfterExit(task *models.StrategyTask, now time.Time) {
	if task == nil {
		return
	}

	currentTick, err := s.getCurrentTick(task)
	if err != nil {
		log.Printf("[Strategy] 任务 #%d 撤出后获取当前 tick 失败: %v", task.ID, err)
		s.notify(task.UserID, fmt.Sprintf("❌ 再平衡撤出完成，但获取 tick 失败：%v。任务已暂停。", err))
		database.DB.Model(task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": fmt.Sprintf("rebalance tick query failed after exit: %v", err),
		})
		return
	}

	// 2. Calculate New Range
	tickLower, tickUpper, err := s.calculateRangeFromPercentage(task, currentTick)
	if err != nil {
		log.Printf("[Strategy] 任务 #%d 计算新 tick 范围失败: %v", task.ID, err)
		s.notify(task.UserID, fmt.Sprintf("❌ 再平衡计算范围失败: %v。任务已暂停。", err))
		database.DB.Model(task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": fmt.Sprintf("rebalance range calc failed: %v", err),
		})
		return
	}

	// 3. Update Task Params
	task.TickLower = tickLower
	task.TickUpper = tickUpper
	// Must clear these to avoid confusion in Enter
	task.V3TokenID = ""
	task.V4TokenID = ""
	task.CurrentLiquidity = "0"

	// 4. Enter Immediately
	s.notify(task.UserID, "🔄 再平衡撤出已完成，正在按新价格重新开仓...")
	enterRes, err := s.liquidityService.EnterTaskFromUSDT(task.UserID, task)
	if err != nil {
		log.Printf("[Strategy] 任务 #%d 再平衡开仓失败: %v", task.ID, err)
		s.notify(task.UserID, fmt.Sprintf("❌ 再平衡开仓失败: %v。任务已暂停。", err))
		database.DB.Model(task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": fmt.Sprintf("rebalance enter failed: %v", err),
			"tick_lower":    tickLower,
			"tick_upper":    tickUpper,
		})
		return
	}

	// 5. Update Task Success
	updates := map[string]interface{}{
		"status":                      models.StrategyStatusRunning,
		"tick_lower":                  tickLower,
		"tick_upper":                  tickUpper,
		"last_rebalance_at":           &now,
		"last_exit_time":              &now,
		"current_liquidity":           enterRes.CurrentLiquidity,
		"v3_position_manager_address": enterRes.V3PositionManagerAddress,
		"v3_token_id":                 enterRes.V3TokenID,
		"v4_token_id":                 enterRes.V4TokenID,
		"out_of_range_since":          nil,
		"error_message":               "",
	}
	database.DB.Model(task).Updates(updates)
	s.notify(task.UserID, fmt.Sprintf("✅ 再平衡完成！\n新 Tick 范围: %d - %d\n交易哈希: `%s`", tickLower, tickUpper, enterRes.TxHash))
}

func (s *StrategyService) finishStopAfterExit(task *models.StrategyTask, now time.Time, title string, txHashes []string) {
	if task == nil {
		return
	}

	updates := map[string]interface{}{
		"status":             models.StrategyStatusStopped,
		"last_exit_time":     &now,
		"current_liquidity":  "0",
		"out_of_range_since": nil,
		"error_message":      "",
	}
	database.DB.Model(task).Updates(updates)

	msg := fmt.Sprintf("✅ %s 完成，任务已停止。", title)
	if len(txHashes) > 0 {
		msg += "\n📝 *交易记录：*\n"
		for i, txInfo := range txHashes {
			parts := strings.Split(txInfo, "|")
			if len(parts) == 2 {
				desc := parts[0]
				txHash := parts[1]
				msg += fmt.Sprintf("%d. **%s**\n   [查看交易](https://bscscan.com/tx/%s)\n", i+1, desc, txHash)
			} else {
				msg += fmt.Sprintf("%d. [查看交易](https://bscscan.com/tx/%s)\n", i+1, txInfo)
			}
		}
	}
	s.notify(task.UserID, msg)
}
