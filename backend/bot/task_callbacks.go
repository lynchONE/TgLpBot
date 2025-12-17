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
	b.sendMessageWithKeyboard(query.Message.Chat.ID, b.formatTaskCard(task), b.taskKeyboard(task))
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

	_ = b.taskService.Update(user.ID, taskID, map[string]interface{}{
		"status":        models.StrategyStatusStopping,
		"error_message": "",
	})

	txHashes, err := b.liquidityService.ExitTaskToUSDT(user.ID, task)
	if err != nil {
		_ = b.taskService.Update(user.ID, taskID, map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": fmt.Sprintf("manual stop exit failed: %v", err),
		})
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 停止任务失败：%v", err))
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
		"status":             models.StrategyStatusStopped,
		"current_liquidity":  "0",
		"out_of_range_since": nil,
		"error_message":      "",
		"last_exit_time":     &now,
	}
	if err := b.taskService.Update(user.ID, taskID, updates); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("停止任务失败：%v", err))
		return
	}

	// 编辑当前消息
	task, _ = b.taskService.GetByID(user.ID, taskID)
	finalText := "✅ *任务已停止* (流动性已撤出)\n" + txLinksText + b.formatTaskCard(task)

	editMsg := tgbotapi.NewEditMessageText(
		query.Message.Chat.ID,
		query.Message.MessageID,
		finalText,
	)
	editMsg.ParseMode = "Markdown"
	editMsg.DisableWebPagePreview = true
	b.api.Send(editMsg)

	// 更新按钮
	editKeyboard := tgbotapi.NewEditMessageReplyMarkup(
		query.Message.Chat.ID,
		query.Message.MessageID,
		b.taskKeyboard(task),
	)
	b.api.Send(editKeyboard)
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

	editKeyboard := tgbotapi.NewEditMessageReplyMarkup(
		query.Message.Chat.ID,
		query.Message.MessageID,
		b.taskKeyboard(task),
	)
	b.api.Send(editKeyboard)
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

	editKeyboard := tgbotapi.NewEditMessageReplyMarkup(
		query.Message.Chat.ID,
		query.Message.MessageID,
		b.taskKeyboard(task),
	)
	b.api.Send(editKeyboard)
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
