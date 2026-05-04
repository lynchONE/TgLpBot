package bot

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/strategy"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func parseTaskID(prefix, data string) (uint, error) {
	idStr := strings.TrimPrefix(data, prefix)
	id64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id64 == 0 {
		return 0, fmt.Errorf("invalid task id")
	}
	return uint(id64), nil
}

func isTaskAutoRefreshActive(chatID int64, messageID int) bool {
	key := fmt.Sprintf("%d_%d", chatID, messageID)
	autoRefreshMutex.RLock()
	session := autoRefreshSessions[key]
	active := session != nil && session.Active
	autoRefreshMutex.RUnlock()
	return active
}

func taskDeleteConfirmKeyboard(taskID uint) tgbotapi.InlineKeyboardMarkup {
	idStr := fmt.Sprintf("%d", taskID)
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚠️ 确认删除", "task_confirm_delete_"+idStr),
			tgbotapi.NewInlineKeyboardButtonData("取消", "task_cancel_delete_"+idStr),
		),
	)
}

func (b *Bot) handleTaskView(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	taskID, err := parseTaskID("task_view_", query.Data)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "无效的任务ID")
		return
	}
	task, err := b.taskService.GetByID(user.ID, taskID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("获取任务失败：%v", err))
		return
	}

	// Send task card with refresh UI
	msg, err := b.sendTaskCardMessage(query.Message.Chat.ID, b.formatTaskCardWithRefresh(task), b.taskKeyboardWithRefresh(task))
	if err == nil && msg.MessageID != 0 {
		// Start auto-refresh for this message
		b.startTaskAutoRefresh(query.Message.Chat.ID, msg.MessageID, task.ID, user.ID)
	}
}

func (b *Bot) handleTaskDelete(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	chatID := user.TelegramID
	messageID := 0
	if query.Message != nil {
		chatID = query.Message.Chat.ID
		messageID = query.Message.MessageID
		b.stopTaskAutoRefresh(query.Message.Chat.ID, query.Message.MessageID)
	}

	taskID, err := parseTaskID("task_delete_", query.Data)
	if err != nil {
		b.sendMessage(chatID, "无效的任务ID")
		return
	}
	task, err := b.taskService.GetByID(user.ID, taskID)
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("获取任务失败：%v", err))
		return
	}
	stableSym, _, _ := stableSymbolForChain(task.Chain)

	text := fmt.Sprintf("⚠️ *确认删除任务 #%d？*\n\n删除后将从列表中移除（不可恢复）。\n\n注意：删除不会撤出链上流动性/兑换 %s；如需撤仓请先点击“停止任务”，或自行在钱包里撤仓。", task.ID, stableSym)
	if messageID != 0 {
		_ = b.editMessageText(chatID, messageID, text)
		_ = b.editMessageReplyMarkup(chatID, messageID, taskDeleteConfirmKeyboard(task.ID))
		return
	}

	b.sendMessageWithKeyboard(chatID, text, taskDeleteConfirmKeyboard(task.ID))
}

func (b *Bot) handleTaskCancelDelete(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, "已取消"))
	chatID := user.TelegramID
	messageID := 0
	if query.Message != nil {
		chatID = query.Message.Chat.ID
		messageID = query.Message.MessageID
	}
	taskID, err := parseTaskID("task_cancel_delete_", query.Data)
	if err != nil {
		b.sendMessage(chatID, "无效的任务ID")
		return
	}
	task, err := b.taskService.GetByID(user.ID, taskID)
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("获取任务失败：%v", err))
		return
	}

	if messageID != 0 {
		if task.Status == models.StrategyStatusRunning || task.Status == models.StrategyStatusWaiting || task.Status == models.StrategyStatusStopping {
			_ = b.editMessageText(chatID, messageID, b.formatTaskCardWithRefresh(task))
			_ = b.editMessageReplyMarkup(chatID, messageID, b.taskKeyboardWithRefresh(task))
			b.startTaskAutoRefresh(chatID, messageID, task.ID, user.ID)
			return
		}

		_ = b.editMessageText(chatID, messageID, b.formatTaskCard(task))
		_ = b.editMessageReplyMarkup(chatID, messageID, b.taskKeyboard(task))
		return
	}
	b.sendMessageWithKeyboard(chatID, b.formatTaskCard(task), b.taskKeyboard(task))
}

func (b *Bot) handleTaskConfirmDelete(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, "已删除"))
	chatID := user.TelegramID
	messageID := 0
	if query.Message != nil {
		chatID = query.Message.Chat.ID
		messageID = query.Message.MessageID
		b.stopTaskAutoRefresh(query.Message.Chat.ID, query.Message.MessageID)
	}

	taskID, err := parseTaskID("task_confirm_delete_", query.Data)
	if err != nil {
		b.sendMessage(chatID, "无效的任务ID")
		return
	}

	if err := b.taskService.Delete(user.ID, taskID); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("删除任务失败：%v", err))
		return
	}

	if messageID != 0 {
		_ = b.editMessageText(chatID, messageID, fmt.Sprintf("✅ 任务 #%d 已删除", taskID))
		_ = b.editMessageReplyMarkup(chatID, messageID, tgbotapi.NewInlineKeyboardMarkup())
		return
	}
	b.sendMessage(chatID, fmt.Sprintf("✅ 任务 #%d 已删除", taskID))
}

// handleTaskStopRefresh stops auto-refresh for a task card
func (b *Bot) handleTaskStopRefresh(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, "已停止自动刷新"))

	// Stop refresh
	b.stopTaskAutoRefresh(query.Message.Chat.ID, query.Message.MessageID)

	// Update keyboard to remove stop refresh button
	taskID, err := parseTaskID("task_stop_refresh_", query.Data)
	if err == nil {
		task, err := b.taskService.GetByID(user.ID, taskID)
		if err == nil {
			if err := b.editMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, b.taskKeyboard(task)); err != nil {
				log.Printf("[Bot] Failed to update task keyboard: %v", err)
			}
		}
	}
}

func (b *Bot) handleTaskStop(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, "正在停止任务..."))
	chatID := user.TelegramID
	messageID := 0
	if query.Message != nil && query.Message.Chat != nil {
		chatID = query.Message.Chat.ID
		messageID = query.Message.MessageID
	}
	taskID, err := parseTaskID("task_stop_", query.Data)
	if err != nil {
		b.sendMessage(chatID, "无效的任务ID")
		return
	}
	task, err := b.taskService.GetByID(user.ID, taskID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "task not found") {
			_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, "任务不存在"))
			text := fmt.Sprintf("⚠️ 任务 #%d 不存在（可能已删除或已结束）。\n\n请返回 /tasks 列表刷新。", taskID)
			if messageID != 0 {
				b.stopTaskAutoRefresh(chatID, messageID)
				_ = b.editMessageReplyMarkup(chatID, messageID, tgbotapi.NewInlineKeyboardMarkup())
				_ = b.editMessageText(chatID, messageID, text)
			} else {
				b.sendMessage(chatID, text)
			}
			return
		}
		b.sendMessage(chatID, fmt.Sprintf("获取任务失败：%v", err))
		return
	}

	if task.Status == models.StrategyStatusStopped {
		b.sendMessageWithKeyboard(query.Message.Chat.ID, b.formatTaskCard(task), b.taskKeyboard(task))
		return
	}
	if task.Status == models.StrategyStatusStopping {
		b.sendMessage(query.Message.Chat.ID, "⏳ 任务正在停止中，请稍候...")
		b.sendMessageWithKeyboard(query.Message.Chat.ID, b.formatTaskCard(task), b.taskKeyboard(task))
		return
	}

	// If there's already a pending exit retry, don't start another on-chain exit in the bot handler.
	if strings.TrimSpace(task.ExitPendingAction) != "" && task.ExitGiveUpAt == nil {
		// Allow switching a pending rebalance/stoploss exit into a manual stop (so it won't re-enter after exit).
		if strings.TrimSpace(task.ExitPendingAction) != "manual_stop" {
			_ = b.taskService.Update(user.ID, taskID, map[string]interface{}{
				"exit_pending_action": "manual_stop",
				"exit_pending_reason": "🛑 手动停止",
				"exit_next_retry_at":  nil, // retry ASAP in strategy loop
				"exit_give_up_at":     nil,
				"error_message":       "",
			})
			stableSym, _, _ := stableSymbolForChain(task.Chain)
			b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("🛑 已切换为手动停止：系统将继续撤出并兑换 %s（最多重试 3 次）。", stableSym))
		} else {
			b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("⏳ 正在撤出中（已失败 %d 次，最多 3 次），请稍候…", task.ExitRetryCount))
		}
		task, _ = b.taskService.GetByID(user.ID, taskID)
		b.sendMessageWithKeyboard(query.Message.Chat.ID, b.formatTaskCard(task), b.taskKeyboard(task))
		return
	}

	currentLiq := strings.TrimSpace(task.CurrentLiquidity)
	poolVersion := strings.ToLower(strings.TrimSpace(task.PoolVersion))

	canExit := false
	switch poolVersion {
	case "v4":
		// V4 需要 tokenId 和 liquidity
		v4TokenId := strings.TrimSpace(task.V4TokenID)
		canExit = v4TokenId != "" && v4TokenId != "0" && currentLiq != "" && currentLiq != "0"
	default:
		// V3 只需要 tokenId
		v3TokenId := strings.TrimSpace(task.V3TokenID)
		canExit = v3TokenId != "" && v3TokenId != "0"
	}

	// If we are not currently in a running position (e.g. waiting), we allow a pure "stop" when there's no recorded liquidity.
	if !canExit {
		if task.RebalancePending && (currentLiq == "" || currentLiq == "0") {
			if err := b.taskService.Update(user.ID, taskID, map[string]interface{}{
				"status":                  models.StrategyStatusStopped,
				"rebalance_pending":       false,
				"rebalance_retry_count":   0,
				"rebalance_next_retry_at": nil,
				"rebalance_last_error":    "",
				"error_message":           "",
			}); err != nil {
				b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("停止任务失败：%v", err))
				return
			}
			b.sendMessage(query.Message.Chat.ID, "✅ 已停止：当前处于再平衡重试中且无可撤出的流动性仓位")
			task, _ = b.taskService.GetByID(user.ID, taskID)
			b.sendMessageWithKeyboard(query.Message.Chat.ID, b.formatTaskCard(task), b.taskKeyboard(task))
			return
		}

		if task.Status != models.StrategyStatusRunning && (currentLiq == "" || currentLiq == "0") {
			if err := b.taskService.Update(user.ID, taskID, map[string]interface{}{
				"status":        models.StrategyStatusStopped,
				"error_message": "",
			}); err != nil {
				b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("停止任务失败：%v", err))
				return
			}
			b.sendMessage(query.Message.Chat.ID, "✅ 已停止：当前无可撤出的流动性仓位")
			task, _ = b.taskService.GetByID(user.ID, taskID)
			b.sendMessageWithKeyboard(query.Message.Chat.ID, b.formatTaskCard(task), b.taskKeyboard(task))
			return
		}

		b.sendMessage(query.Message.Chat.ID, "❌ 无法停止：缺少仓位信息（tokenId / liquidity），无法撤出并兑换稳定币。")
		b.sendMessageWithKeyboard(query.Message.Chat.ID, b.formatTaskCard(task), b.taskKeyboard(task))
		return
	}

	// Reset any previous give-up state when user manually stops again, then rely on the strategy loop to exit in background.
	updates := map[string]interface{}{
		"status":                     models.StrategyStatusStopping,
		"out_of_range_since":         nil,
		"error_message":              "",
		"exit_pending_action":        "manual_stop",
		"exit_pending_reason":        "🛑 手动停止",
		"exit_retry_count":           0,
		"exit_next_retry_at":         nil, // retry ASAP in strategy loop
		"exit_last_error":            "",
		"exit_give_up_at":            nil,
		"rebalance_pending":          false,
		"rebalance_retry_count":      0,
		"rebalance_next_retry_at":    nil,
		"rebalance_last_error":       "",
		"switch_target_pool_id":      "",
		"switch_target_pool_version": "",
	}
	if err := b.taskService.Update(user.ID, taskID, updates); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("停止任务失败：%v", err))
		return
	}

	stableSym, _, _ := stableSymbolForChain(task.Chain)
	b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("🛑 已提交手动停止：后台将撤出流动性并兑换成 %s（最多重试 3 次）。", stableSym))

	task, _ = b.taskService.GetByID(user.ID, taskID)
	finalText := "🛑 *已提交停止请求* (后台处理中)\n\n" + rewriteRebalanceTimeoutText(b.formatTaskCard(task))
	editMsg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, finalText)
	editMsg.ParseMode = "Markdown"
	editMsg.DisableWebPagePreview = true
	_, _ = b.api.Send(editMsg)
	_ = b.editMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, b.taskKeyboard(task))
}

func (b *Bot) handleTaskToggleReinvest(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	taskID, err := parseTaskID("task_toggle_reinvest_", query.Data)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "无效的任务ID")
		return
	}
	task, err := b.taskService.GetByID(user.ID, taskID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("获取任务失败：%v", err))
		return
	}
	newValue := !task.AutoReinvest
	if err := b.taskService.Update(user.ID, taskID, map[string]interface{}{
		"auto_reinvest": newValue,
	}); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("更新任务失败：%v", err))
		return
	}
	task.AutoReinvest = newValue

	// Use EditMessageText to update inplace
	editMsg := tgbotapi.NewEditMessageText(
		query.Message.Chat.ID,
		query.Message.MessageID,
		rewriteRebalanceTimeoutText(b.formatTaskCard(task)),
	)
	editMsg.ParseMode = "Markdown"
	editMsg.DisableWebPagePreview = true
	b.api.Send(editMsg)

	if err := b.editMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, b.taskKeyboard(task)); err != nil {
		log.Printf("[Bot] Failed to update task keyboard: %v", err)
	}
}

func (b *Bot) handleTaskTogglePause(query *tgbotapi.CallbackQuery, user *models.User) {
	taskID, err := parseTaskID("task_toggle_pause_", query.Data)
	if err != nil {
		b.api.Send(tgbotapi.NewCallback(query.ID, "无效的任务ID"))
		b.sendMessage(query.Message.Chat.ID, "无效的任务ID")
		return
	}
	task, err := b.taskService.GetByID(user.ID, taskID)
	if err != nil {
		b.api.Send(tgbotapi.NewCallback(query.ID, "获取任务失败"))
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("获取任务失败：%v", err))
		return
	}

	newValue := !task.Paused
	now := time.Now()

	updates := map[string]interface{}{
		"paused":             newValue,
		"out_of_range_since": nil,
	}
	if newValue {
		updates["paused_at"] = &now
		switch strings.TrimSpace(task.ExitPendingAction) {
		case strategy.ExitActionRebalance, strategy.ExitActionStopLoss, strategy.ExitActionOutOfRangeStop:
			updates["exit_pending_action"] = ""
			updates["exit_pending_reason"] = ""
			updates["exit_retry_count"] = 0
			updates["exit_next_retry_at"] = nil
			updates["exit_last_error"] = ""
			updates["exit_give_up_at"] = nil
			updates["rebalance_pending"] = false
			updates["rebalance_retry_count"] = 0
			updates["rebalance_next_retry_at"] = nil
			updates["rebalance_last_error"] = ""
			updates["error_message"] = ""
		}
	} else {
		updates["paused_at"] = nil
	}

	if err := b.taskService.Update(user.ID, taskID, updates); err != nil {
		b.api.Send(tgbotapi.NewCallback(query.ID, "更新失败"))
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("更新任务失败：%v", err))
		return
	}

	task.Paused = newValue
	if newValue {
		task.PausedAt = &now
	} else {
		task.PausedAt = nil
	}
	task.OutOfRangeSince = nil
	if newValue {
		switch strings.TrimSpace(task.ExitPendingAction) {
		case strategy.ExitActionRebalance, strategy.ExitActionStopLoss, strategy.ExitActionOutOfRangeStop:
			task.ExitPendingAction = ""
			task.ExitPendingReason = ""
			task.ExitRetryCount = 0
			task.ExitNextRetryAt = nil
			task.ExitLastError = ""
			task.ExitGiveUpAt = nil
			task.RebalancePending = false
			task.RebalanceRetryCount = 0
			task.RebalanceNextRetryAt = nil
			task.RebalanceLastError = ""
			task.ErrorMessage = ""
		}
	}

	cbText := "✅ 已恢复"
	if newValue {
		cbText = "⏸️ 已暂停"
	}
	b.api.Send(tgbotapi.NewCallback(query.ID, cbText))

	useRefresh := false
	if query.Message != nil {
		useRefresh = isTaskAutoRefreshActive(query.Message.Chat.ID, query.Message.MessageID)
	}

	var cardText string
	var keyboard any
	if useRefresh {
		cardText = b.formatTaskCardWithRefresh(task)
		keyboard = b.taskKeyboardWithRefresh(task)
	} else {
		cardText = rewriteRebalanceTimeoutText(b.formatTaskCard(task))
		keyboard = b.taskKeyboard(task)
	}

	editMsg := tgbotapi.NewEditMessageText(
		query.Message.Chat.ID,
		query.Message.MessageID,
		rewriteRebalanceTimeoutText(cardText),
	)
	editMsg.ParseMode = "Markdown"
	editMsg.DisableWebPagePreview = true
	b.api.Send(editMsg)

	if err := b.editMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, keyboard); err != nil {
		log.Printf("[Bot] Failed to update task keyboard: %v", err)
	}
}

func (b *Bot) handleTaskToggleStopLoss(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	taskID, err := parseTaskID("task_toggle_stoploss_", query.Data)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "无效的任务ID")
		return
	}
	task, err := b.taskService.GetByID(user.ID, taskID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("获取任务失败：%v", err))
		return
	}
	newValue := !task.StopLossEnabled
	if err := b.taskService.Update(user.ID, taskID, map[string]interface{}{
		"stop_loss_enabled": newValue,
	}); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("更新任务失败：%v", err))
		return
	}
	task.StopLossEnabled = newValue

	// Use EditMessageText to update inplace
	editMsg := tgbotapi.NewEditMessageText(
		query.Message.Chat.ID,
		query.Message.MessageID,
		rewriteRebalanceTimeoutText(b.formatTaskCard(task)),
	)
	editMsg.ParseMode = "Markdown"
	editMsg.DisableWebPagePreview = true
	b.api.Send(editMsg)

	if err := b.editMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, b.taskKeyboard(task)); err != nil {
		log.Printf("[Bot] Failed to update task keyboard: %v", err)
	}
}

func (b *Bot) handleTaskSetSlippage(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	taskID, err := parseTaskID("task_set_slippage_", query.Data)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "无效的任务ID")
		return
	}
	database.SetUserSession(user.TelegramID, "task_edit_id", fmt.Sprintf("%d", taskID), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "state", "awaiting_task_slippage", 30*time.Minute)
	b.sendMessage(query.Message.Chat.ID, "📊 请输入该任务的滑点（百分比），例如：`1` 表示 1%")
}

func (b *Bot) handleTaskSetRange(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	taskID, err := parseTaskID("task_set_range_", query.Data)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "无效的任务ID")
		return
	}

	task, err := b.taskService.GetByID(user.ID, taskID)
	if err != nil || task == nil {
		b.sendMessage(query.Message.Chat.ID, "任务不存在或已删除。")
		return
	}
	if task.Status == models.StrategyStatusStopped {
		b.sendMessage(query.Message.Chat.ID, "该任务已停止，无法修改区间。")
		return
	}

	database.SetUserSession(user.TelegramID, "task_edit_id", fmt.Sprintf("%d", taskID), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "task_card_msg_id", fmt.Sprintf("%d", query.Message.MessageID), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "state", "awaiting_task_range", 30*time.Minute)

	promptMsg, _ := b.sendMessage(query.Message.Chat.ID, "🎯 请输入新的区间（百分比），例如：`5` 表示 ±5%，或 `1 3` 表示下 1% 上 3%（修改后下次再平衡生效）")
	if promptMsg.MessageID != 0 {
		database.SetUserSession(user.TelegramID, "prompt_msg_id", fmt.Sprintf("%d", promptMsg.MessageID), 30*time.Minute)
	}
}

func (b *Bot) handleTaskSetRebalanceTimeout(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	taskID, err := parseTaskID("task_set_rebalance_", query.Data)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "无效的任务ID")
		return
	}

	// Store task ID
	database.SetUserSession(user.TelegramID, "task_edit_id", fmt.Sprintf("%d", taskID), 30*time.Minute)
	// Store original task card message ID so we can edit it later
	database.SetUserSession(user.TelegramID, "task_card_msg_id", fmt.Sprintf("%d", query.Message.MessageID), 30*time.Minute)

	database.SetUserSession(user.TelegramID, "state", "awaiting_task_rebalance_timeout", 30*time.Minute)

	promptMsg, _ := b.sendMessage(query.Message.Chat.ID, "⏱️ 请输入该任务的再平衡超时（秒），`-1` 表示立即执行，例如：`-1` 或 `10`")
	// Store prompt message ID to delete it later
	if promptMsg.MessageID != 0 {
		database.SetUserSession(user.TelegramID, "prompt_msg_id", fmt.Sprintf("%d", promptMsg.MessageID), 30*time.Minute)
	}
}

func (b *Bot) handleTaskSwapDust(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, "正在兑换残余..."))
	taskID, err := parseTaskID("task_swap_dust_", query.Data)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "无效的任务ID")
		return
	}
	task, err := b.taskService.GetByID(user.ID, taskID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("获取任务失败：%v", err))
		return
	}
	stableSym, _, _ := stableSymbolForChain(task.Chain)

	loadingMsg, _ := b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("⏳ 正在兑换残余代币为 %s，请稍候...", stableSym))
	defer func() {
		if loadingMsg.MessageID != 0 {
			b.api.Send(tgbotapi.NewDeleteMessage(loadingMsg.Chat.ID, loadingMsg.MessageID))
		}
	}()

	txHashes, err := b.liquidityService.SwapTaskDustToUSDT(user.ID, task)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 兑换残余失败：%v", err))
		task, _ = b.taskService.GetByID(user.ID, taskID)
		b.sendMessageWithKeyboard(query.Message.Chat.ID, b.formatTaskCard(task), b.taskKeyboard(task))
		return
	}

	if len(txHashes) == 0 {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("✅ 残余资产已是 %s 或无需兑换。", stableSym))
	} else {
		var txLinksText string
		txLinksText = "✅ 残余代币已提交兑换：\n"
		for i, txInfo := range txHashes {
			parts := strings.Split(txInfo, "|")
			if len(parts) == 2 {
				desc := parts[0]
				txHash := parts[1]
				txLinksText += fmt.Sprintf("%d. **%s**\n   [查看交易](%s)\n", i+1, desc, explorerTxURL(task.Chain, txHash))
			} else {
				txLinksText += fmt.Sprintf("%d. [查看交易](%s)\n", i+1, explorerTxURL(task.Chain, txInfo))
			}
		}
		b.sendMessage(query.Message.Chat.ID, txLinksText)
	}

	task, _ = b.taskService.GetByID(user.ID, taskID)
	editMsg := tgbotapi.NewEditMessageText(
		query.Message.Chat.ID,
		query.Message.MessageID,
		rewriteRebalanceTimeoutText(b.formatTaskCard(task)),
	)
	editMsg.ParseMode = "Markdown"
	editMsg.DisableWebPagePreview = true
	b.api.Send(editMsg)

	if err := b.editMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, b.taskKeyboard(task)); err != nil {
		log.Printf("[Bot] Failed to update task keyboard: %v", err)
	}
}

func (b *Bot) handleTaskSetStopLossDelay(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	taskID, err := parseTaskID("task_set_stoploss_delay_", query.Data)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "无效的任务ID")
		return
	}
	database.SetUserSession(user.TelegramID, "task_edit_id", fmt.Sprintf("%d", taskID), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "state", "awaiting_task_stop_loss_delay", 30*time.Minute)
	b.sendMessage(query.Message.Chat.ID, "⏲️ 请输入该任务的秒止损阈值（秒，0 表示立即触发），例如：`0` 或 `10`")
}

func (b *Bot) handleTaskSetResidualTolerance(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	database.ClearUserSession(user.TelegramID)
	b.sendMessage(query.Message.Chat.ID, "该配置已下线，不再进行剩余资产容忍度校验。")
}
