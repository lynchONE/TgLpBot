package bot

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/pricing"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) taskIDFromSession(user *models.User) (uint, error) {
	idStr, err := database.GetUserSession(user.TelegramID, "task_edit_id")
	if err != nil || strings.TrimSpace(idStr) == "" {
		return 0, fmt.Errorf("task id missing")
	}
	id64, err := strconv.ParseUint(strings.TrimSpace(idStr), 10, 64)
	if err != nil || id64 == 0 {
		return 0, fmt.Errorf("invalid task id")
	}
	return uint(id64), nil
}

func (b *Bot) handleTaskSlippageInput(chatID int64, user *models.User, text string) {
	taskID, err := b.taskIDFromSession(user)
	if err != nil {
		b.sendMessage(chatID, "会话已过期，请重新打开任务卡片。")
		database.ClearUserSession(user.TelegramID)
		return
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(text, "%")), 64)
	if err != nil || value < 0 || value > 100 {
		b.sendMessage(chatID, "数值无效。请输入 0-100 之间的滑点百分比，例如：`0.5` 表示 0.5%")
		return
	}
	if err := b.taskService.Update(user.ID, taskID, map[string]interface{}{
		"slippage_tolerance": value,
	}); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("更新任务失败：%v", err))
		return
	}
	database.ClearUserSession(user.TelegramID)
	task, _ := b.taskService.GetByID(user.ID, taskID)
	b.sendMessageWithKeyboard(chatID, b.formatTaskCard(task), b.taskKeyboard(task))
}

func (b *Bot) handleTaskRebalanceTimeoutInput(message *tgbotapi.Message, user *models.User) {
	text := message.Text
	chatID := message.Chat.ID

	taskID, err := b.taskIDFromSession(user)
	if err != nil {
		b.sendMessage(chatID, "会话已过期，请重新打开任务卡片。")
		database.ClearUserSession(user.TelegramID)
		return
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || seconds < 0 || seconds > 86400 {
		b.sendMessage(chatID, "数值无效。请输入 0-86400 之间的整数秒数，例如：`300`")
		return
	}

	// Delete user's message
	b.api.Send(tgbotapi.NewDeleteMessage(chatID, message.MessageID))

	// Delete prompt message if exists
	if promptMsgIDStr, err := database.GetUserSession(user.TelegramID, "prompt_msg_id"); err == nil && promptMsgIDStr != "" {
		if promptMsgID, _ := strconv.Atoi(promptMsgIDStr); promptMsgID != 0 {
			b.api.Send(tgbotapi.NewDeleteMessage(chatID, promptMsgID))
		}
	}

	if err := b.taskService.Update(user.ID, taskID, map[string]interface{}{
		"reopen_delay_seconds": seconds,
	}); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("更新任务失败：%v", err))
		return
	}
	database.ClearUserSession(user.TelegramID)

	task, _ := b.taskService.GetByID(user.ID, taskID)

	// Update original task card inplace if possible
	if cardMsgIDStr, err := database.GetUserSession(user.TelegramID, "task_card_msg_id"); err == nil && cardMsgIDStr != "" {
		if cardMsgID, _ := strconv.Atoi(cardMsgIDStr); cardMsgID != 0 {
			editMsg := tgbotapi.NewEditMessageText(
				chatID,
				cardMsgID,
				b.formatTaskCard(task),
			)
			editMsg.ParseMode = "Markdown"
			editMsg.DisableWebPagePreview = true
			b.api.Send(editMsg)

			if err := b.editMessageReplyMarkup(chatID, cardMsgID, b.taskKeyboard(task)); err != nil {
				log.Printf("[Bot] Failed to update task keyboard: %v", err)
			}
			return
		}
	}

	// Fallback if no card ID found (shouldn't happen with new flow)
	b.sendMessageWithKeyboard(chatID, b.formatTaskCard(task), b.taskKeyboard(task))
}

func (b *Bot) handleTaskRangeInput(message *tgbotapi.Message, user *models.User) {
	chatID := message.Chat.ID
	text := strings.TrimSpace(message.Text)

	taskID, err := b.taskIDFromSession(user)
	if err != nil {
		b.sendMessage(chatID, "会话已过期，请重新打开任务卡片。")
		database.ClearUserSession(user.TelegramID)
		return
	}

	fields := strings.Fields(text)
	var stableLowerPctReq float64
	var stableUpperPctReq float64

	switch len(fields) {
	case 1:
		pct, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(fields[0], "%")), 64)
		if err != nil || pct <= 0 || pct >= 100 {
			b.sendMessage(chatID, "区间无效。请输入 0-100 之间的百分比，例如：`5` 表示 ±5%")
			return
		}
		stableLowerPctReq = pct
		stableUpperPctReq = pct
	case 2:
		lowPct, err1 := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(fields[0], "%")), 64)
		upPct, err2 := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(fields[1], "%")), 64)
		if err1 != nil || err2 != nil || lowPct <= 0 || upPct <= 0 || lowPct >= 100 || upPct >= 100 {
			b.sendMessage(chatID, "区间无效。请输入 0-100 之间的百分比，例如：`1 3` 表示下 1% 上 3%")
			return
		}
		stableLowerPctReq = lowPct
		stableUpperPctReq = upPct
	default:
		b.sendMessage(chatID, "格式无效。请输入：\n1) `5`（表示 ±5%）\n2) `1 3`（表示下 1% 上 3%）")
		return
	}

	task, err := b.taskService.GetByID(user.ID, taskID)
	if err != nil || task == nil {
		b.sendMessage(chatID, "任务不存在或已删除。")
		database.ClearUserSession(user.TelegramID)
		return
	}

	tickLowerPct, tickUpperPct := pricing.TickPercentagesFromStablePercentages(task, stableLowerPctReq, stableUpperPctReq)
	if tickLowerPct <= 0 || tickUpperPct <= 0 || tickLowerPct >= 100 || tickUpperPct >= 100 {
		b.sendMessage(chatID, "区间无效。请检查输入百分比。")
		return
	}

	// Delete user's message
	b.api.Send(tgbotapi.NewDeleteMessage(chatID, message.MessageID))

	// Delete prompt message if exists
	if promptMsgIDStr, err := database.GetUserSession(user.TelegramID, "prompt_msg_id"); err == nil && promptMsgIDStr != "" {
		if promptMsgID, _ := strconv.Atoi(promptMsgIDStr); promptMsgID != 0 {
			b.api.Send(tgbotapi.NewDeleteMessage(chatID, promptMsgID))
		}
	}

	updates := map[string]interface{}{
		"range_percentage":       (tickLowerPct + tickUpperPct) / 2.0,
		"range_lower_percentage": tickLowerPct,
		"range_upper_percentage": tickUpperPct,
	}
	if err := b.taskService.Update(user.ID, taskID, updates); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("更新任务失败：%v", err))
		return
	}

	task, _ = b.taskService.GetByID(user.ID, taskID)

	b.sendMessage(chatID, "✅ 区间已更新（下次再平衡生效）")

	// Update original task card inplace if possible
	cardMsgIDStr, _ := database.GetUserSession(user.TelegramID, "task_card_msg_id")

	database.ClearUserSession(user.TelegramID)

	if strings.TrimSpace(cardMsgIDStr) != "" {
		if cardMsgID, _ := strconv.Atoi(cardMsgIDStr); cardMsgID != 0 {
			editMsg := tgbotapi.NewEditMessageText(
				chatID,
				cardMsgID,
				b.formatTaskCard(task),
			)
			editMsg.ParseMode = "Markdown"
			editMsg.DisableWebPagePreview = true
			b.api.Send(editMsg)

			if err := b.editMessageReplyMarkup(chatID, cardMsgID, b.taskKeyboard(task)); err != nil {
				log.Printf("[Bot] Failed to update task keyboard: %v", err)
			}
			return
		}
	}

	b.sendMessageWithKeyboard(chatID, b.formatTaskCard(task), b.taskKeyboard(task))
}

func (b *Bot) handleTaskStopLossDelayInput(chatID int64, user *models.User, text string) {
	taskID, err := b.taskIDFromSession(user)
	if err != nil {
		b.sendMessage(chatID, "会话已过期，请重新打开任务卡片。")
		database.ClearUserSession(user.TelegramID)
		return
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || seconds < 0 || seconds > 86400 {
		b.sendMessage(chatID, "数值无效。请输入 0-86400 之间的整数秒数，例如：`0` 或 `10`")
		return
	}
	if err := b.taskService.Update(user.ID, taskID, map[string]interface{}{
		"stop_loss_delay_seconds": seconds,
	}); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("更新任务失败：%v", err))
		return
	}
	database.ClearUserSession(user.TelegramID)
	task, _ := b.taskService.GetByID(user.ID, taskID)
	b.sendMessageWithKeyboard(chatID, b.formatTaskCard(task), b.taskKeyboard(task))
}

func (b *Bot) handleTaskResidualToleranceInput(chatID int64, user *models.User, text string) {
	taskID, err := b.taskIDFromSession(user)
	if err != nil {
		b.sendMessage(chatID, "会话已过期，请重新打开任务卡片。")
		database.ClearUserSession(user.TelegramID)
		return
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(text, "%")), 64)
	if err != nil || value < 0 || value > 100 {
		b.sendMessage(chatID, "数值无效。请输入 0-100 之间的百分比，例如：`1` 表示 1%")
		return
	}
	if err := b.taskService.Update(user.ID, taskID, map[string]interface{}{
		"residual_tolerance": value,
	}); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("更新任务失败：%v", err))
		return
	}
	database.ClearUserSession(user.TelegramID)
	task, _ := b.taskService.GetByID(user.ID, taskID)
	b.sendMessageWithKeyboard(chatID, b.formatTaskCard(task), b.taskKeyboard(task))
}
