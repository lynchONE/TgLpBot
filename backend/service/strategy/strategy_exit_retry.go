package strategy

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/txexec"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

const (
	ExitActionManualStop     = "manual_stop"
	ExitActionStopLoss       = "stoploss"
	ExitActionOutOfRangeStop = "range_stop"
	ExitActionRebalance      = "rebalance"
	ExitActionSwitch         = "switch"
)

var exitRetrySchedule = []time.Duration{
	500 * time.Millisecond,
	1 * time.Second,
	2 * time.Second,
	3 * time.Second,
	5 * time.Second,
	10 * time.Second,
	30 * time.Second,
}

func exitRetryDelay(attempt int) time.Duration {
	if len(exitRetrySchedule) == 0 {
		return 30 * time.Second
	}
	if attempt <= 0 {
		return exitRetrySchedule[0]
	}
	idx := attempt - 1
	if idx >= len(exitRetrySchedule) {
		return exitRetrySchedule[len(exitRetrySchedule)-1]
	}
	return exitRetrySchedule[idx]
}

func shouldRetryExitError(err error) bool {
	if err == nil {
		return false
	}
	var swapErr *liquidity.SwapToUSDTError
	if errors.As(err, &swapErr) {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	if text == "" {
		return true
	}
	unretryableMarkers := []string{
		"task is nil",
		"wallet is nil",
		"failed to get wallet",
		"failed to get private key",
		"failed to parse private key",
		"stable address not set",
		"position manager address not configured",
		"blockchain client not initialized",
		"invalid v3 pool address",
		"invalid pool address",
		"task has no v3 tokenid",
		"task has no v4 tokenid",
		"missing/invalid current_liquidity",
		"uniswap_v4_pool_manager_address not set",
		"uniswap_v4_position_manager_address not set",
	}
	for _, marker := range unretryableMarkers {
		if strings.Contains(text, marker) {
			return false
		}
	}
	return true
}

func (s *StrategyService) scheduleExitRetryWake(taskID uint, userID uint, delay time.Duration) {
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
			log.Printf("[Strategy] load task for scheduled exit retry wake failed: task_id=%d user_id=%d err=%v", taskID, userID, err)
			return
		}
		s.processExitRetry(&task)
	}()
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
	if task.ExitGiveUpAt != nil && action != ExitActionManualStop {
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

	// 检查内存锁：如果任务已在执行中，跳过本次提交
	s.inflightTasksMu.Lock()
	if submitTime, exists := s.inflightTasks[task.ID]; exists {
		// 如果任务已在执行中超过10分钟，认为是异常情况，清理锁
		if now.Sub(submitTime) < 10*time.Minute {
			s.inflightTasksMu.Unlock()
			log.Printf("[Strategy] 任务 #%d 退出操作已在执行中，跳过重复提交", task.ID)
			return true
		}
		log.Printf("[Strategy] 任务 #%d 执行中锁超时(>10分钟)，清理旧锁", task.ID)
		delete(s.inflightTasks, task.ID)
	}
	s.inflightTasksMu.Unlock()

	exec := txexec.Default()
	if exec == nil {
		return true
	}

	// 先更新DB状态，设置一个较长的 exit_next_retry_at 防止重复触发
	// 如果DB更新失败，不提交任务
	lockUntil := now.Add(5 * time.Minute)
	if err := database.DB.Model(task).Update("exit_next_retry_at", &lockUntil).Error; err != nil {
		log.Printf("[Strategy] 任务 #%d 更新DB锁失败，跳过本次提交: %v", task.ID, err)
		return true
	}
	task.ExitNextRetryAt = &lockUntil

	// 设置内存锁
	s.inflightTasksMu.Lock()
	s.inflightTasks[task.ID] = now
	s.inflightTasksMu.Unlock()

	if ok, err := exec.TryRunTask(task.UserID, task.WalletID, task.WalletAddress, func(_ string) {
		defer func() {
			// 交易完成后清理内存锁
			s.inflightTasksMu.Lock()
			delete(s.inflightTasks, task.ID)
			s.inflightTasksMu.Unlock()
		}()
		s.runExitRetryAttempt(task.ID, task.UserID)
	}); err != nil {
		log.Printf("[Strategy] schedule exit retry failed: task_id=%d user_id=%d err=%v", task.ID, task.UserID, err)
		// 提交失败，清理内存锁
		s.inflightTasksMu.Lock()
		delete(s.inflightTasks, task.ID)
		s.inflightTasksMu.Unlock()
	} else if !ok {
		// Wallet is busy or global tx slots are full; 清理内存锁，下次重试
		s.inflightTasksMu.Lock()
		delete(s.inflightTasks, task.ID)
		s.inflightTasksMu.Unlock()
		// 恢复 exit_next_retry_at 为 nil，允许下次尝试
		delay := exitRetryDelay(task.ExitRetryCount + 1)
		nextAt := now.Add(delay)
		database.DB.Model(task).Update("exit_next_retry_at", &nextAt)
		task.ExitNextRetryAt = &nextAt
		s.scheduleExitRetryWake(task.ID, task.UserID, delay)
	}

	return true
}

func (s *StrategyService) runExitRetryAttempt(taskID uint, userID uint) {
	if taskID == 0 || userID == 0 {
		return
	}

	var task models.StrategyTask
	if err := database.DB.Where("id = ? AND user_id = ?", taskID, userID).First(&task).Error; err != nil {
		log.Printf("[Strategy] load task for exit retry failed: task_id=%d user_id=%d err=%v", taskID, userID, err)
		return
	}

	action := strings.TrimSpace(task.ExitPendingAction)
	if action == "" {
		return
	}

	// After we give up, only a manual stop can reset and retry again.
	if task.ExitGiveUpAt != nil && action != ExitActionManualStop {
		return
	}

	now := time.Now()
	// 注意：不再检查 ExitNextRetryAt，因为 processExitRetry 已经检查过了，
	// 而且这里的任务是刚从DB加载的，ExitNextRetryAt 是我们刚设置的5分钟锁，
	// 如果在这里检查会导致直接返回而不执行撤退操作。

	attempt := task.ExitRetryCount + 1
	pendingReason := strings.TrimSpace(task.ExitPendingReason)
	reason := pendingReason
	if reason == "" {
		reason = "撤出流动性"
	}

	if attempt == 1 {
		if task.ExitLiquidityRemoved {
			s.notify(task.UserID, fmt.Sprintf("%s，正在兑换成 USDT...", reason))
		} else {
			s.notify(task.UserID, fmt.Sprintf("%s，正在撤出流动性并兑换成 USDT...", reason))
		}
	} else {
		if task.ExitLiquidityRemoved {
			s.notify(task.UserID, fmt.Sprintf("🔄 %s失败，正在第 %d 次重试兑换 USDT...", reason, attempt))
		} else {
			s.notify(task.UserID, fmt.Sprintf("🔄 %s失败，正在第 %d 次重试撤出并兑换 USDT...", reason, attempt))
		}
	}

	txHashes, err := s.liquidityService.ExitTaskToUSDTWithOptions(task.UserID, &task, true, liquidity.TxOptions{GasMultiplier: task.ExitGasMultiplier})
	if err != nil {
		s.onExitAttemptFailed(&task, attempt, err, txHashes)
		return
	}

	// Success: clear retry state and continue with post-exit action.
	s.clearExitRetryState(&task)

	switch action {
	case ExitActionRebalance:
		s.executeRebalanceAfterExit(&task, now)
	case ExitActionStopLoss:
		s.finishStopAfterExit(&task, now, reason, txHashes)
	case ExitActionOutOfRangeStop:
		s.finishStopAfterExit(&task, now, reason, txHashes)
	case ExitActionSwitch:
		s.executeSwitchAfterExit(&task, now, reason)
	case ExitActionManualStop:
		title := "🛑 手动停止"
		if pendingReason != "" {
			title = pendingReason
		}
		s.finishStopAfterExit(&task, now, title, txHashes)
	default:
		log.Printf("[Strategy] 任务 #%d 撤出成功，但未知 exit_pending_action=%q，已清理重试状态", task.ID, action)
	}
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

	// 检查内存锁：如果任务已在执行中，跳过本次提交
	s.inflightTasksMu.Lock()
	if submitTime, exists := s.inflightTasks[task.ID]; exists {
		// 如果任务已在执行中超过10分钟，认为是异常情况，清理锁
		if now.Sub(submitTime) < 10*time.Minute {
			s.inflightTasksMu.Unlock()
			log.Printf("[Strategy] 任务 #%d 再平衡开仓操作已在执行中，跳过重复提交", task.ID)
			return true
		}
		log.Printf("[Strategy] 任务 #%d 执行中锁超时(>10分钟)，清理旧锁", task.ID)
		delete(s.inflightTasks, task.ID)
	}
	s.inflightTasksMu.Unlock()

	exec := txexec.Default()
	if exec == nil {
		return true
	}

	// 先更新DB状态，设置一个较长的 rebalance_next_retry_at 防止重复触发
	// 如果DB更新失败，不提交任务
	lockUntil := now.Add(5 * time.Minute)
	if err := database.DB.Model(task).Update("rebalance_next_retry_at", &lockUntil).Error; err != nil {
		log.Printf("[Strategy] 任务 #%d 更新再平衡DB锁失败，跳过本次提交: %v", task.ID, err)
		return true
	}
	task.RebalanceNextRetryAt = &lockUntil

	// 设置内存锁
	s.inflightTasksMu.Lock()
	s.inflightTasks[task.ID] = now
	s.inflightTasksMu.Unlock()

	if ok, err := exec.TryRunTask(task.UserID, task.WalletID, task.WalletAddress, func(_ string) {
		defer func() {
			// 交易完成后清理内存锁
			s.inflightTasksMu.Lock()
			delete(s.inflightTasks, task.ID)
			s.inflightTasksMu.Unlock()
		}()
		s.runRebalanceRetryAttempt(task.ID, task.UserID)
	}); err != nil {
		log.Printf("[Strategy] schedule rebalance retry failed: task_id=%d user_id=%d err=%v", task.ID, task.UserID, err)
		// 提交失败，清理内存锁
		s.inflightTasksMu.Lock()
		delete(s.inflightTasks, task.ID)
		s.inflightTasksMu.Unlock()
	} else if !ok {
		// Wallet is busy or global tx slots are full; 清理内存锁，下次重试
		s.inflightTasksMu.Lock()
		delete(s.inflightTasks, task.ID)
		s.inflightTasksMu.Unlock()
		// 恢复 rebalance_next_retry_at 为 nil，允许下次尝试
		database.DB.Model(task).Update("rebalance_next_retry_at", nil)
		task.RebalanceNextRetryAt = nil
	}
	return true
}

func (s *StrategyService) runRebalanceRetryAttempt(taskID uint, userID uint) {
	if taskID == 0 || userID == 0 {
		return
	}

	var task models.StrategyTask
	if err := database.DB.Where("id = ? AND user_id = ?", taskID, userID).First(&task).Error; err != nil {
		log.Printf("[Strategy] load task for rebalance retry failed: task_id=%d user_id=%d err=%v", taskID, userID, err)
		return
	}
	if !task.RebalancePending {
		return
	}

	// 注意：不再检查 RebalanceNextRetryAt，因为 processRebalanceRetry 已经检查过了，
	// 而且这里的任务是刚从DB加载的，RebalanceNextRetryAt 是我们刚设置的5分钟锁。

	now := time.Now()
	attempt := task.RebalanceRetryCount + 1
	if err := s.attemptRebalanceEnter(&task, now); err != nil {
		s.scheduleRebalanceRetry(&task, attempt, err)
	}
}

func (s *StrategyService) markRebalancePending(task *models.StrategyTask, now time.Time) {
	if task == nil {
		return
	}

	updates := map[string]interface{}{
		"status":                   models.StrategyStatusRunning,
		"last_exit_time":           &now,
		"current_liquidity":        "0",
		"v3_token_id":              "",
		"v4_token_id":              "",
		"out_of_range_since":       nil,
		"range_activation_pending": false,
		"rebalance_pending":        true,
		"rebalance_retry_count":    0,
		"rebalance_next_retry_at":  func() *time.Time { t := now.Add(5 * time.Minute); return &t }(), // 防止竞态条件导致重复触发
		"rebalance_last_error":     "",
		"error_message":            "",
	}
	_ = database.DB.Model(task).Updates(updates).Error

	task.Status = models.StrategyStatusRunning
	task.LastExitTime = &now
	task.CurrentLiquidity = "0"
	task.V3TokenID = ""
	task.V4TokenID = ""
	task.OutOfRangeSince = nil
	task.RangeActivationPending = false
	task.RebalancePending = true
	task.RebalanceRetryCount = 0
	nextRetryAt := now.Add(5 * time.Minute)
	task.RebalanceNextRetryAt = &nextRetryAt
	task.RebalanceLastError = ""
	task.ErrorMessage = ""
}

func (s *StrategyService) attemptRebalanceEnter(task *models.StrategyTask, now time.Time) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}

	// Reload editable config fields so user updates can affect the next (in-flight) rebalance.
	var freshCfg models.StrategyTask
	if err := database.DB.
		Select("range_percentage", "range_lower_percentage", "range_upper_percentage", "slippage_tolerance").
		Where("id = ? AND user_id = ?", task.ID, task.UserID).
		First(&freshCfg).Error; err == nil {
		task.RangePercentage = freshCfg.RangePercentage
		task.RangeLowerPercentage = freshCfg.RangeLowerPercentage
		task.RangeUpperPercentage = freshCfg.RangeUpperPercentage
		task.SlippageTolerance = freshCfg.SlippageTolerance
	}

	switching := strings.TrimSpace(task.SwitchTargetPoolId) != "" && strings.TrimSpace(task.SwitchTargetPoolVersion) != ""

	if task.TickSpacing <= 0 || strings.TrimSpace(task.Token0Address) == "" || strings.TrimSpace(task.Token1Address) == "" {
		if err := s.refreshTaskPoolMeta(task); err != nil {
			return fmt.Errorf("load pool info failed: %w", err)
		}
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
		"exit_liquidity_removed":      false,
		"v3_position_manager_address": enterRes.V3PositionManagerAddress,
		"v3_token_id":                 enterRes.V3TokenID,
		"v4_token_id":                 enterRes.V4TokenID,
		"out_of_range_since":          nil,
		"range_activation_pending":    false,
		"rebalance_pending":           false,
		"rebalance_retry_count":       0,
		"rebalance_next_retry_at":     nil,
		"rebalance_last_error":        "",
		"error_message":               "",
	}

	// 调试日志：记录即将保存的TokenID
	log.Printf("[Strategy] 任务 #%d 开仓成功，准备保存: V3TokenID=%s, V4TokenID=%s, V3PM=%s",
		task.ID, enterRes.V3TokenID, enterRes.V4TokenID, enterRes.V3PositionManagerAddress)

	if dbErr := database.DB.Model(task).Updates(updates).Error; dbErr != nil {
		// 链上交易已成功，DB保存失败只记录警告，不触发重试
		log.Printf("[Strategy] ⚠️ 任务 #%d 保存开仓结果失败 (链上交易已成功): %v", task.ID, dbErr)

		// 兜底：至少把关键字段（TokenID/状态/重试标志）写入 DB，避免任务被误判为未开仓而重复开仓。
		criticalUpdates := map[string]interface{}{
			"status":                      models.StrategyStatusRunning,
			"tick_lower":                  tickLower,
			"tick_upper":                  tickUpper,
			"last_rebalance_at":           &now,
			"last_exit_time":              &now,
			"current_liquidity":           enterRes.CurrentLiquidity,
			"exit_liquidity_removed":      false,
			"v3_position_manager_address": enterRes.V3PositionManagerAddress,
			"v3_token_id":                 enterRes.V3TokenID,
			"v4_token_id":                 enterRes.V4TokenID,
			"out_of_range_since":          nil,
			"range_activation_pending":    false,
			"rebalance_pending":           false,
			"rebalance_retry_count":       0,
			"rebalance_next_retry_at":     nil,
			"rebalance_last_error":        "",
			"error_message":               "",
		}
		if cErr := database.DB.Model(task).Updates(criticalUpdates).Error; cErr != nil {
			log.Printf("[Strategy] ⚠️ 任务 #%d 兜底写入关键字段仍失败: %v", task.ID, cErr)
		}
	}

	task.Status = models.StrategyStatusRunning
	task.TickLower = tickLower
	task.TickUpper = tickUpper
	task.LastRebalanceAt = &now
	task.LastExitTime = &now
	task.CurrentLiquidity = enterRes.CurrentLiquidity
	task.ExitLiquidityRemoved = false
	task.V3PositionManagerAddress = enterRes.V3PositionManagerAddress
	task.V3TokenID = enterRes.V3TokenID
	task.V4TokenID = enterRes.V4TokenID
	task.OutOfRangeSince = nil
	task.RangeActivationPending = false
	task.RebalancePending = false
	task.RebalanceRetryCount = 0
	task.RebalanceNextRetryAt = nil
	task.RebalanceLastError = ""
	task.ErrorMessage = ""

	if switching {
		_ = database.DB.Model(task).Updates(map[string]interface{}{
			"switch_target_pool_version":   "",
			"switch_target_pool_id":        "",
			"switch_target_tick_lower_pct": 0,
			"switch_target_tick_upper_pct": 0,
		}).Error
		task.SwitchTargetPoolVersion = ""
		task.SwitchTargetPoolId = ""
		task.SwitchTargetTickLowerPct = 0
		task.SwitchTargetTickUpperPct = 0
	}

	title := "✅ 再平衡完成！"
	if switching {
		title = "✅ 切换完成！"
	}
	s.notify(task.UserID, fmt.Sprintf("%s\n新 Tick 范围: %d - %d\n交易哈希: `%s`", title, tickLower, tickUpper, enterRes.TxHash))
	s.notifyTaskCard(task.UserID, task.ID)
	return nil
}

func (s *StrategyService) scheduleRebalanceRetry(task *models.StrategyTask, attempt int, err error) {
	if task == nil {
		return
	}

	actionName := "再平衡"
	if strings.TrimSpace(task.SwitchTargetPoolId) != "" && strings.TrimSpace(task.SwitchTargetPoolVersion) != "" {
		actionName = "切换开仓"
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

	s.notify(task.UserID, fmt.Sprintf("❌ %s失败（%d 次）：%v\n将在 %ds 后重试，任务保持运行中。", actionName, attempt, err, int(delay.Seconds())))
}

func (s *StrategyService) onExitAttemptFailed(task *models.StrategyTask, attempt int, err error, txHashes []string) {
	if task == nil {
		return
	}

	now := time.Now()
	errText := strings.TrimSpace(fmt.Sprintf("%v", err))
	swapFailed := false
	if err != nil {
		var swapErr *liquidity.SwapToUSDTError
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
					txText += fmt.Sprintf("%d. **%s**\n   [查看交易](%s)\n", i+1, desc, explorerTxURL(task.Chain, txHash))
				} else {
					txText += fmt.Sprintf("%d. **%s**\n", i+1, desc)
				}
			} else {
				txHash := strings.TrimSpace(txInfo)
				if txHash != "" {
					txText += fmt.Sprintf("%d. [查看交易](%s)\n", i+1, explorerTxURL(task.Chain, txHash))
				}
			}
		}
	}

	if !shouldRetryExitError(err) {
		s.giveUpExitRetry(task, err)
		if swapFailed {
			s.notify(task.UserID, fmt.Sprintf("❌ 已撤出流动性，但兑换 USDT 失败且该错误不会自动重试。\n任务保持运行中，请到钱包手动处理剩余 token。\n最后错误：%v%s", err, txText))
		} else {
			s.notify(task.UserID, fmt.Sprintf("❌ 撤出/兑换失败，且该错误不会自动重试。\n任务保持运行中，请确认钱包、授权和链配置后再手动重试。\n最后错误：%v%s", err, txText))
		}
		return
	}

	delay := exitRetryDelay(attempt)
	nextAt := now.Add(delay)
	updates := map[string]interface{}{
		"status":             models.StrategyStatusRunning,
		"exit_retry_count":   attempt,
		"exit_next_retry_at": &nextAt,
		"exit_last_error":    errText,
		"exit_give_up_at":    nil,
		"error_message":      "",
	}
	_ = database.DB.Model(task).Updates(updates).Error

	task.Status = models.StrategyStatusRunning
	task.ExitRetryCount = attempt
	task.ExitNextRetryAt = &nextAt
	task.ExitLastError = errText
	task.ExitGiveUpAt = nil

	s.scheduleExitRetryWake(task.ID, task.UserID, delay)

	if swapFailed {
		s.notify(task.UserID, fmt.Sprintf("⚠️ 已撤出流动性，但兑换 USDT 失败（第 %d 次）：%v\n将在 %s 后重试兑换。%s", attempt, err, delay.String(), txText))
	} else {
		s.notify(task.UserID, fmt.Sprintf("❌ 撤出/兑换失败（第 %d 次）：%v\n将在 %s 后重试。%s", attempt, err, delay.String(), txText))
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

func (s *StrategyService) executeSwitchAfterExit(task *models.StrategyTask, now time.Time, reason string) {
	if task == nil {
		return
	}

	targetPoolVersion := strings.ToLower(strings.TrimSpace(task.SwitchTargetPoolVersion))
	targetPoolID := strings.TrimSpace(task.SwitchTargetPoolId)
	if targetPoolVersion == "" || targetPoolID == "" {
		title := "🔁 切换失败：缺少目标池"
		if strings.TrimSpace(reason) != "" {
			title = strings.TrimSpace(reason)
		}
		s.finishStopAfterExit(task, now, title, nil)
		return
	}

	lowerPct := task.SwitchTargetTickLowerPct
	upperPct := task.SwitchTargetTickUpperPct
	if lowerPct <= 0 || upperPct <= 0 || lowerPct >= 100 || upperPct >= 100 {
		if task.RangeLowerPercentage > 0 && task.RangeUpperPercentage > 0 {
			lowerPct = task.RangeLowerPercentage
			upperPct = task.RangeUpperPercentage
		} else if task.RangePercentage > 0 {
			lowerPct = task.RangePercentage
			upperPct = task.RangePercentage
		}
	}
	const maxPct = 99.0
	if lowerPct > maxPct {
		lowerPct = maxPct
	}
	if upperPct > maxPct {
		upperPct = maxPct
	}
	if lowerPct <= 0 || upperPct <= 0 || lowerPct >= 100 || upperPct >= 100 {
		lowerPct = 1.0
		upperPct = 1.0
	}

	task.PoolVersion = targetPoolVersion
	task.PoolId = targetPoolID
	task.Exchange = ""
	task.Token0Symbol = ""
	task.Token1Symbol = ""
	task.Token0Address = ""
	task.Token1Address = ""
	task.HooksAddress = "0x0000000000000000000000000000000000000000"
	task.Fee = 0
	task.TickSpacing = 0
	task.RangeLowerPercentage = lowerPct
	task.RangeUpperPercentage = upperPct
	task.RangePercentage = (lowerPct + upperPct) / 2.0

	updates := map[string]interface{}{
		"pool_version":                task.PoolVersion,
		"pool_id":                     task.PoolId,
		"exchange":                    task.Exchange,
		"token0_symbol":               task.Token0Symbol,
		"token1_symbol":               task.Token1Symbol,
		"token0_address":              task.Token0Address,
		"token1_address":              task.Token1Address,
		"hooks_address":               task.HooksAddress,
		"fee":                         task.Fee,
		"tick_spacing":                task.TickSpacing,
		"range_percentage":            task.RangePercentage,
		"range_lower_percentage":      task.RangeLowerPercentage,
		"range_upper_percentage":      task.RangeUpperPercentage,
		"tick_lower":                  0,
		"tick_upper":                  0,
		"status":                      models.StrategyStatusRunning,
		"last_exit_time":              &now,
		"current_liquidity":           "0",
		"exit_liquidity_removed":      false,
		"v3_position_manager_address": "",
		"v3_token_id":                 "",
		"v4_token_id":                 "",
		"out_of_range_since":          nil,
		"range_activation_pending":    false,
		"rebalance_pending":           true,
		"rebalance_retry_count":       0,
		"rebalance_next_retry_at":     func() *time.Time { t := now.Add(5 * time.Minute); return &t }(), // 防止竞态条件导致重复触发
		"rebalance_last_error":        "",
		"error_message":               "",
	}
	_ = database.DB.Model(task).Updates(updates).Error

	task.Status = models.StrategyStatusRunning
	task.LastExitTime = &now
	task.CurrentLiquidity = "0"
	task.ExitLiquidityRemoved = false
	task.V3PositionManagerAddress = ""
	task.V3TokenID = ""
	task.V4TokenID = ""
	task.OutOfRangeSince = nil
	task.RangeActivationPending = false
	task.RebalancePending = true
	task.RebalanceRetryCount = 0
	switchNextRetryAt := now.Add(5 * time.Minute)
	task.RebalanceNextRetryAt = &switchNextRetryAt
	task.RebalanceLastError = ""
	task.ErrorMessage = ""

	msg := "🔁 切换撤出已完成，正在按新池子重新开仓..."
	if strings.TrimSpace(reason) != "" {
		msg = fmt.Sprintf("%s，正在按新池子重新开仓...", strings.TrimSpace(reason))
	}
	s.notify(task.UserID, msg)
	if err := s.attemptRebalanceEnter(task, now); err != nil {
		s.scheduleRebalanceRetry(task, 1, err)
	}
}

func (s *StrategyService) finishStopAfterExit(task *models.StrategyTask, now time.Time, title string, txHashes []string) {
	if task == nil {
		return
	}

	updates := map[string]interface{}{
		"status":                   models.StrategyStatusStopped,
		"last_exit_time":           &now,
		"current_liquidity":        "0",
		"out_of_range_since":       nil,
		"range_activation_pending": false,
		"rebalance_pending":        false,
		"rebalance_retry_count":    0,
		"rebalance_next_retry_at":  nil,
		"rebalance_last_error":     "",
		"error_message":            "",
	}
	database.DB.Model(task).Updates(updates)

	task.Status = models.StrategyStatusStopped
	task.LastExitTime = &now
	task.CurrentLiquidity = "0"
	task.OutOfRangeSince = nil
	task.RangeActivationPending = false
	task.RebalancePending = false
	task.RebalanceRetryCount = 0
	task.RebalanceNextRetryAt = nil
	task.RebalanceLastError = ""
	task.ErrorMessage = ""

	msg := fmt.Sprintf("✅ %s 完成，任务已停止。", title)
	if len(txHashes) > 0 {
		msg += "\n📝 *交易记录：*\n"
		hasSwapTx := false
		for i, txInfo := range txHashes {
			parts := strings.Split(txInfo, "|")
			if len(parts) == 2 {
				desc := parts[0]
				txHash := parts[1]
				msg += fmt.Sprintf("%d. **%s**\n   [查看交易](%s)\n", i+1, desc, explorerTxURL(task.Chain, txHash))
				if strings.Contains(desc, "→USDT") || strings.Contains(desc, "->USDT") {
					hasSwapTx = true
				}
			} else {
				msg += fmt.Sprintf("%d. [查看交易](%s)\n", i+1, explorerTxURL(task.Chain, txInfo))
				if strings.Contains(txInfo, "→USDT") || strings.Contains(txInfo, "->USDT") {
					hasSwapTx = true
				}
			}
		}
		if !hasSwapTx {
			msg += "\nℹ️ 本次未产生兑换交易：钱包中该池子的非 USDT 代币余额为 0，或均为小额（<1 USDT）已跳过兑换。"
		}
	}
	s.notify(task.UserID, msg)
}
