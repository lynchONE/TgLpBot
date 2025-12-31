package services

import (
	"TgLpBot/database"
	"TgLpBot/models"
	"errors"
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

func rebalanceRetryDelay(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return 15 * time.Second
	case attempt == 2:
		return 30 * time.Second
	case attempt == 3:
		return 60 * time.Second
	case attempt == 4:
		return 120 * time.Second
	default:
		return 300 * time.Second
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
		"exit_pending_action":     action,
		"exit_pending_reason":     strings.TrimSpace(reason),
		"exit_retry_count":        0,
		"exit_next_retry_at":      nil,
		"exit_last_error":         "",
		"exit_give_up_at":         nil,
		"rebalance_pending":       false,
		"rebalance_retry_count":   0,
		"rebalance_next_retry_at": nil,
		"rebalance_last_error":    "",
		"error_message":           "",
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
	task.RebalancePending = false
	task.RebalanceRetryCount = 0
	task.RebalanceNextRetryAt = nil
	task.RebalanceLastError = ""

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

	txHashes, err := s.liquidityService.ExitTaskToUSDTWithOptions(task.UserID, task, true, TxOptions{GasMultiplier: task.ExitGasMultiplier})
	if err != nil {
		s.onExitAttemptFailed(task, attempt, err, txHashes)
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

// processRebalanceRetry handles re-entry retries after a successful exit.
// Returns true when the task processing should stop for this cycle.
func (s *StrategyService) processRebalanceRetry(task *models.StrategyTask) bool {
	if task == nil || !task.RebalancePending {
		return false
	}

	now := time.Now()
	if task.RebalanceNextRetryAt != nil && now.Before(*task.RebalanceNextRetryAt) {
		return true
	}

	attempt := task.RebalanceRetryCount + 1
	if err := s.attemptRebalanceEnter(task, now); err != nil {
		s.scheduleRebalanceRetry(task, attempt, err)
	}
	return true
}

func (s *StrategyService) markRebalancePending(task *models.StrategyTask, now time.Time) {
	if task == nil {
		return
	}

	updates := map[string]interface{}{
		"status":                  models.StrategyStatusRunning,
		"last_exit_time":          &now,
		"current_liquidity":       "0",
		"v3_token_id":             "",
		"v4_token_id":             "",
		"out_of_range_since":      nil,
		"rebalance_pending":       true,
		"rebalance_retry_count":   0,
		"rebalance_next_retry_at": nil,
		"rebalance_last_error":    "",
		"error_message":           "",
	}
	_ = database.DB.Model(task).Updates(updates).Error

	task.Status = models.StrategyStatusRunning
	task.LastExitTime = &now
	task.CurrentLiquidity = "0"
	task.V3TokenID = ""
	task.V4TokenID = ""
	task.OutOfRangeSince = nil
	task.RebalancePending = true
	task.RebalanceRetryCount = 0
	task.RebalanceNextRetryAt = nil
	task.RebalanceLastError = ""
	task.ErrorMessage = ""
}

func (s *StrategyService) attemptRebalanceEnter(task *models.StrategyTask, now time.Time) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}

	currentTick, err := s.getCurrentTick(task)
	if err != nil {
		log.Printf("[Strategy] 任务 #%d 撤出后获取当前 tick 失败: %v", task.ID, err)
		return fmt.Errorf("rebalance tick query failed: %w", err)
	}

	tickLower, tickUpper, err := s.calculateRangeFromPercentage(task, currentTick)
	if err != nil {
		log.Printf("[Strategy] 任务 #%d 计算新 tick 范围失败: %v", task.ID, err)
		return fmt.Errorf("rebalance range calc failed: %w", err)
	}

	task.TickLower = tickLower
	task.TickUpper = tickUpper

	_ = database.DB.Model(task).Updates(map[string]interface{}{
		"tick_lower": tickLower,
		"tick_upper": tickUpper,
	}).Error

	enterRes, err := s.liquidityService.EnterTaskFromUSDT(task.UserID, task)
	if err != nil {
		log.Printf("[Strategy] 任务 #%d 再平衡开仓失败: %v", task.ID, err)
		return fmt.Errorf("rebalance enter failed: %w", err)
	}

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
		"rebalance_pending":           false,
		"rebalance_retry_count":       0,
		"rebalance_next_retry_at":     nil,
		"rebalance_last_error":        "",
		"error_message":               "",
	}
	_ = database.DB.Model(task).Updates(updates).Error

	task.Status = models.StrategyStatusRunning
	task.TickLower = tickLower
	task.TickUpper = tickUpper
	task.LastRebalanceAt = &now
	task.LastExitTime = &now
	task.CurrentLiquidity = enterRes.CurrentLiquidity
	task.V3PositionManagerAddress = enterRes.V3PositionManagerAddress
	task.V3TokenID = enterRes.V3TokenID
	task.V4TokenID = enterRes.V4TokenID
	task.OutOfRangeSince = nil
	task.RebalancePending = false
	task.RebalanceRetryCount = 0
	task.RebalanceNextRetryAt = nil
	task.RebalanceLastError = ""
	task.ErrorMessage = ""

	if task.IsAuto {
		_ = NewAutoLPEventService().Record(task, models.AutoLPEventRebalance, "")
	}
	s.notify(task.UserID, fmt.Sprintf("✅ 再平衡完成！\n新 Tick 范围: %d - %d\n交易哈希: `%s`", tickLower, tickUpper, enterRes.TxHash))
	return nil
}

func (s *StrategyService) scheduleRebalanceRetry(task *models.StrategyTask, attempt int, err error) {
	if task == nil {
		return
	}

	now := time.Now()
	delay := rebalanceRetryDelay(attempt)
	nextAt := now.Add(delay)
	errText := strings.TrimSpace(fmt.Sprintf("%v", err))

	updates := map[string]interface{}{
		"status":                  models.StrategyStatusRunning,
		"rebalance_pending":       true,
		"rebalance_retry_count":   attempt,
		"rebalance_next_retry_at": &nextAt,
		"rebalance_last_error":    errText,
		"error_message":           "",
	}
	_ = database.DB.Model(task).Updates(updates).Error

	task.Status = models.StrategyStatusRunning
	task.RebalancePending = true
	task.RebalanceRetryCount = attempt
	task.RebalanceNextRetryAt = &nextAt
	task.RebalanceLastError = errText
	task.ErrorMessage = ""

	s.notify(task.UserID, fmt.Sprintf("❌ 再平衡失败（%d 次）：%v\n将在 %ds 后重试，任务保持运行中。", attempt, err, int(delay.Seconds())))
}

func (s *StrategyService) onExitAttemptFailed(task *models.StrategyTask, attempt int, err error, txHashes []string) {
	if task == nil {
		return
	}

	now := time.Now()
	errText := strings.TrimSpace(fmt.Sprintf("%v", err))
	swapFailed := false
	if err != nil {
		var swapErr *SwapToUSDTError
		if errors.As(err, &swapErr) {
			swapFailed = true
		}
	}
	txText := ""
	if len(txHashes) > 0 {
		txText = "\n📝 *交易记录：*\n"
		for i, txInfo := range txHashes {
			parts := strings.Split(txInfo, "|")
			if len(parts) == 2 {
				desc := parts[0]
				txHash := strings.TrimSpace(parts[1])
				if txHash != "" {
					txText += fmt.Sprintf("%d. **%s**\n   [查看交易](https://bscscan.com/tx/%s)\n", i+1, desc, txHash)
				} else {
					txText += fmt.Sprintf("%d. **%s**\n", i+1, desc)
				}
			} else {
				txHash := strings.TrimSpace(txInfo)
				if txHash != "" {
					txText += fmt.Sprintf("%d. [查看交易](https://bscscan.com/tx/%s)\n", i+1, txHash)
				}
			}
		}
	}

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

		if swapFailed {
			s.notify(task.UserID, fmt.Sprintf("❌ 已撤出流动性，但兑换 USDT 连续失败 %d 次，已停止自动重试。\n任务保持运行中，可稍后手动停止再试。\n请到钱包手动把剩余 token 兑换成 USDT。\n最后错误：%v%s", attempt, err, txText))
		} else {
			s.notify(task.UserID, fmt.Sprintf("❌ 撤出/兑换连续失败 %d 次，已停止自动重试。\n任务保持运行中，可稍后手动停止再试。\n如果已撤出流动性但兑换失败，请到钱包手动把剩余 token 兑换成 USDT。\n最后错误：%v%s", attempt, err, txText))
		}
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

	if swapFailed {
		s.notify(task.UserID, fmt.Sprintf("⚠️ 已撤出流动性，但兑换 USDT 失败（%d/%d）：%v\n将在 %ds 后重试兑换。%s", attempt, exitMaxAttempts, err, int(exitRetryDelay(attempt).Seconds()), txText))
	} else {
		s.notify(task.UserID, fmt.Sprintf("❌ 撤出/兑换失败（%d/%d）：%v\n将在 %ds 后重试。%s", attempt, exitMaxAttempts, err, int(exitRetryDelay(attempt).Seconds()), txText))
	}
}

func (s *StrategyService) clearExitRetryState(task *models.StrategyTask) {
	if task == nil {
		return
	}

	updates := map[string]interface{}{
		"exit_pending_action": "",
		"exit_pending_reason": "",
		"exit_gas_multiplier": 1.0,
		"exit_retry_count":    0,
		"exit_next_retry_at":  nil,
		"exit_last_error":     "",
		"exit_give_up_at":     nil,
	}
	_ = database.DB.Model(task).Updates(updates).Error

	task.ExitPendingAction = ""
	task.ExitPendingReason = ""
	task.ExitGasMultiplier = 1.0
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

	s.markRebalancePending(task, now)
	s.notify(task.UserID, "🔄 再平衡撤出已完成，正在按新价格重新开仓...")
	if err := s.attemptRebalanceEnter(task, now); err != nil {
		s.scheduleRebalanceRetry(task, 1, err)
	}
}

func (s *StrategyService) finishStopAfterExit(task *models.StrategyTask, now time.Time, title string, txHashes []string) {
	if task == nil {
		return
	}

	updates := map[string]interface{}{
		"status":                  models.StrategyStatusStopped,
		"last_exit_time":          &now,
		"current_liquidity":       "0",
		"out_of_range_since":      nil,
		"rebalance_pending":       false,
		"rebalance_retry_count":   0,
		"rebalance_next_retry_at": nil,
		"rebalance_last_error":    "",
		"error_message":           "",
	}
	database.DB.Model(task).Updates(updates)

	msg := fmt.Sprintf("✅ %s 完成，任务已停止。", title)
	if len(txHashes) > 0 {
		msg += "\n📝 *交易记录：*\n"
		hasSwapTx := false
		for i, txInfo := range txHashes {
			parts := strings.Split(txInfo, "|")
			if len(parts) == 2 {
				desc := parts[0]
				txHash := parts[1]
				msg += fmt.Sprintf("%d. **%s**\n   [查看交易](https://bscscan.com/tx/%s)\n", i+1, desc, txHash)
				if strings.Contains(desc, "→USDT") || strings.Contains(desc, "->USDT") {
					hasSwapTx = true
				}
			} else {
				msg += fmt.Sprintf("%d. [查看交易](https://bscscan.com/tx/%s)\n", i+1, txInfo)
				if strings.Contains(txInfo, "→USDT") || strings.Contains(txInfo, "->USDT") {
					hasSwapTx = true
				}
			}
		}
		if !hasSwapTx {
			msg += "\nℹ️ 本次未产生兑换交易：钱包中该池子的非 USDT 代币余额为 0（无需兑换）。"
		}
	}
	s.notify(task.UserID, msg)
}
