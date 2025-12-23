package bot

import (
	"TgLpBot/database"
	"TgLpBot/models"
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
	msgConfig := tgbotapi.NewMessage(query.Message.Chat.ID, b.formatTaskCardWithRefresh(task))
	msgConfig.ParseMode = "Markdown"
	msgConfig.ReplyMarkup = b.taskKeyboardWithRefresh(task)
	msgConfig.DisableWebPagePreview = true

	msg, err := b.api.Send(msgConfig)
	if err == nil && msg.MessageID != 0 {
		// Start auto-refresh for this message
		b.startTaskAutoRefresh(query.Message.Chat.ID, msg.MessageID, task.ID, user.ID)
	}
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
	taskID, err := parseTaskID("task_stop_", query.Data)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "无效的任务ID")
		return
	}
	task, err := b.taskService.GetByID(user.ID, taskID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("获取任务失败：%v", err))
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
			b.sendMessage(query.Message.Chat.ID, "🛑 已切换为手动停止：系统将继续撤出并兑换 USDT（最多重试 3 次）。")
		} else {
			b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("⏳ 正在撤出中（已失败 %d 次，最多 3 次），请稍候…", task.ExitRetryCount))
		}
		task, _ = b.taskService.GetByID(user.ID, taskID)
		b.sendMessageWithKeyboard(query.Message.Chat.ID, b.formatTaskCard(task), b.taskKeyboard(task))
		return
	}

	now := time.Now()

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

	loadingMsg, _ := b.sendMessage(query.Message.Chat.ID, "⏳ 正在撤出流动性并兑换成 USDT，请稍候...")
	defer func() {
		if loadingMsg.MessageID != 0 {
			b.api.Send(tgbotapi.NewDeleteMessage(loadingMsg.Chat.ID, loadingMsg.MessageID))
		}
	}()

	// Reset any previous give-up state when user manually stops again.
	if task.ExitGiveUpAt != nil || task.ExitRetryCount > 0 || strings.TrimSpace(task.ExitLastError) != "" || task.ExitNextRetryAt != nil {
		_ = b.taskService.Update(user.ID, taskID, map[string]interface{}{
			"exit_pending_action": "",
			"exit_pending_reason": "",
			"exit_retry_count":    0,
			"exit_next_retry_at":  nil,
			"exit_last_error":     "",
			"exit_give_up_at":     nil,
			"error_message":       "",
		})
		// Update in-memory task snapshot for this handler.
		task.ExitPendingAction = ""
		task.ExitPendingReason = ""
		task.ExitRetryCount = 0
		task.ExitNextRetryAt = nil
		task.ExitLastError = ""
		task.ExitGiveUpAt = nil
	}

	_ = b.taskService.Update(user.ID, taskID, map[string]interface{}{
		"status":        models.StrategyStatusStopping,
		"error_message": "",
	})

	txHashes, err := b.liquidityService.ExitTaskToUSDT(user.ID, task, true)
	if err != nil {
		nextAt := now.Add(10 * time.Second)
		_ = b.taskService.Update(user.ID, taskID, map[string]interface{}{
			"status":              models.StrategyStatusRunning,
			"exit_pending_action": "manual_stop",
			"exit_pending_reason": "🛑 手动停止",
			"exit_retry_count":    1,
			"exit_next_retry_at":  &nextAt,
			"exit_last_error":     fmt.Sprintf("%v", err),
			"exit_give_up_at":     nil,
			"error_message":       "",
		})
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 撤出失败：%v\n系统将自动重试（1/3），任务保持运行中。", err))
		task, _ = b.taskService.GetByID(user.ID, taskID)
		b.sendMessageWithKeyboard(query.Message.Chat.ID, b.formatTaskCard(task), b.taskKeyboard(task))
		return
	}

	// 准备交易链接文本（不直接发送）
	log.Printf("[Bot] Task #%d stopped, got %d transaction hashes", taskID, len(txHashes))
	var txLinksText string
	if len(txHashes) > 0 {
		txLinksText = "\n📝 *交易记录：*\n"
		for i, txInfo := range txHashes {
			// 格式：描述|哈希
			parts := strings.Split(txInfo, "|")
			if len(parts) == 2 {
				desc := parts[0]
				txHash := parts[1]
				log.Printf("[Bot] TX %d: %s - %s", i+1, desc, txHash)
				txLinksText += fmt.Sprintf("%d. **%s**\n   [查看交易](https://bscscan.com/tx/%s)\n", i+1, desc, txHash)
			} else {
				// 兼容旧格式（只有哈希）
				log.Printf("[Bot] TX %d: %s", i+1, txInfo)
				txLinksText += fmt.Sprintf("%d. [查看交易](https://bscscan.com/tx/%s)\n", i+1, txInfo)
			}
		}
		txLinksText += "\n"
	} else {
		log.Printf("[Bot] No transaction hashes returned from ExitTaskToUSDT")
	}

	updates := map[string]interface{}{
		"status":              models.StrategyStatusStopped,
		"current_liquidity":   "0",
		"out_of_range_since":  nil,
		"error_message":       "",
		"last_exit_time":      &now,
		"exit_pending_action": "",
		"exit_pending_reason": "",
		"exit_retry_count":    0,
		"exit_next_retry_at":  nil,
		"exit_last_error":     "",
		"exit_give_up_at":     nil,
	}
	if err := b.taskService.Update(user.ID, taskID, updates); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("停止任务失败：%v", err))
		return
	}

	// 编辑当前消息
	task, _ = b.taskService.GetByID(user.ID, taskID)
	finalText := "✅ *任务已停止* (流动性已撤出)\n" + txLinksText + b.formatTaskCard(task)

	log.Printf("[Bot] Final message for task #%d (txLinksText len=%d): %s", taskID, len(txLinksText), txLinksText)

	editMsg := tgbotapi.NewEditMessageText(
		query.Message.Chat.ID,
		query.Message.MessageID,
		finalText,
	)
	editMsg.ParseMode = "Markdown"
	editMsg.DisableWebPagePreview = true
	if resp, err := b.api.Send(editMsg); err != nil {
		log.Printf("[Bot] Failed to edit message for task #%d: %v", taskID, err)
	} else {
		log.Printf("[Bot] Message edited successfully for task #%d, msgID=%d", taskID, resp.MessageID)
	}

	// 更新按钮
	if err := b.editMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, b.taskKeyboard(task)); err != nil {
		log.Printf("[Bot] Failed to update task keyboard: %v", err)
	}
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
		b.formatTaskCard(task),
	)
	editMsg.ParseMode = "Markdown"
	editMsg.DisableWebPagePreview = true
	b.api.Send(editMsg)

	if err := b.editMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, b.taskKeyboard(task)); err != nil {
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
		b.formatTaskCard(task),
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

	promptMsg, _ := b.sendMessage(query.Message.Chat.ID, "⏱️ 请输入该任务的再平衡超时（秒），例如：`300`")
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

	loadingMsg, _ := b.sendMessage(query.Message.Chat.ID, "⏳ 正在兑换残余代币为 USDT，请稍候...")
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
		b.sendMessage(query.Message.Chat.ID, "✅ 残余资产已是 USDT 或无需兑换。")
	} else {
		var txLinksText string
		txLinksText = "✅ 残余代币已提交兑换：\n"
		for i, txInfo := range txHashes {
			parts := strings.Split(txInfo, "|")
			if len(parts) == 2 {
				desc := parts[0]
				txHash := parts[1]
				txLinksText += fmt.Sprintf("%d. **%s**\n   [查看交易](https://bscscan.com/tx/%s)\n", i+1, desc, txHash)
			} else {
				txLinksText += fmt.Sprintf("%d. [查看交易](https://bscscan.com/tx/%s)\n", i+1, txInfo)
			}
		}
		b.sendMessage(query.Message.Chat.ID, txLinksText)
	}

	task, _ = b.taskService.GetByID(user.ID, taskID)
	editMsg := tgbotapi.NewEditMessageText(
		query.Message.Chat.ID,
		query.Message.MessageID,
		b.formatTaskCard(task),
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
	taskID, err := parseTaskID("task_set_residual_", query.Data)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "无效的任务ID")
		return
	}
	database.SetUserSession(user.TelegramID, "task_edit_id", fmt.Sprintf("%d", taskID), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "state", "awaiting_task_residual_tolerance", 30*time.Minute)
	b.sendMessage(query.Message.Chat.ID, "🧾 请输入该任务的剩余资产容忍度（百分比），例如：`1` 表示最多允许 1% 的剩余资产未投入")
}
