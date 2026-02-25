package bot

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) applyEnterResult(task *models.StrategyTask, enterRes *liquidity.EnterResult) error {
	if task == nil || enterRes == nil {
		return fmt.Errorf("task or enter result is nil")
	}

	updates := map[string]interface{}{
		"current_liquidity":      enterRes.CurrentLiquidity,
		"exit_liquidity_removed": false,
		"error_message":          "",
		"status":                 models.StrategyStatusRunning,
	}

	v3TokenId := strings.TrimSpace(enterRes.V3TokenID)
	if v3TokenId != "" && v3TokenId != "0" {
		updates["v3_position_manager_address"] = enterRes.V3PositionManagerAddress
		updates["v3_token_id"] = enterRes.V3TokenID
	}

	v4TokenId := strings.TrimSpace(enterRes.V4TokenID)
	if v4TokenId != "" && v4TokenId != "0" {
		updates["v4_token_id"] = enterRes.V4TokenID
	}

	if err := database.DB.Model(task).Updates(updates).Error; err != nil {
		return err
	}

	task.CurrentLiquidity = enterRes.CurrentLiquidity
	task.ExitLiquidityRemoved = false
	task.Status = models.StrategyStatusRunning
	task.ErrorMessage = ""

	if v3TokenId != "" && v3TokenId != "0" {
		task.V3PositionManagerAddress = enterRes.V3PositionManagerAddress
		task.V3TokenID = enterRes.V3TokenID
	}
	if v4TokenId != "" && v4TokenId != "0" {
		task.V4TokenID = enterRes.V4TokenID
	}

	return nil
}

func taskHasPosition(task *models.StrategyTask) bool {
	if task == nil {
		return false
	}
	if strings.TrimSpace(task.CurrentLiquidity) != "" && strings.TrimSpace(task.CurrentLiquidity) != "0" {
		return true
	}
	v3TokenId := strings.TrimSpace(task.V3TokenID)
	if v3TokenId != "" && v3TokenId != "0" {
		return true
	}
	v4TokenId := strings.TrimSpace(task.V4TokenID)
	if v4TokenId != "" && v4TokenId != "0" {
		return true
	}
	return false
}

func (b *Bot) promptEntrySwap(chatID int64, task *models.StrategyTask, tokenSymbol string) {
	if task == nil {
		return
	}

	tokenSymbol = strings.TrimSpace(tokenSymbol)
	if tokenSymbol == "" {
		tokenSymbol = "目标代币"
	}

	_ = database.DB.Model(task).Updates(map[string]interface{}{
		"status":        models.StrategyStatusWaiting,
		"error_message": "",
	}).Error
	task.Status = models.StrategyStatusWaiting

	stableSym, _, _ := stableSymbolForChain(task.Chain)
	if strings.TrimSpace(stableSym) == "" {
		stableSym = "USDT"
	}
	text := fmt.Sprintf("检测到该池子不包含 %s，需要先将 %s 兑换为 %s 才能开仓。\n\n是否允许？", stableSym, stableSym, tokenSymbol)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("✅ 允许 %s→%s", stableSym, tokenSymbol), fmt.Sprintf("entry_swap_allow_%d", task.ID)),
			tgbotapi.NewInlineKeyboardButtonData("❌ 取消开仓", fmt.Sprintf("entry_swap_cancel_%d", task.ID)),
		),
	)

	b.sendMessageWithKeyboard(chatID, text, keyboard)
}

func (b *Bot) handleEntrySwapAllow(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, "正在兑换并开仓..."))

	taskID, err := parseTaskID("entry_swap_allow_", query.Data)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "无效的任务ID")
		return
	}

	task, err := b.taskService.GetByID(user.ID, taskID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("获取任务失败：%v", err))
		return
	}
	if taskHasPosition(task) {
		b.sendMessage(query.Message.Chat.ID, "该任务已开仓，无需重复操作。")
		return
	}

	_ = database.DB.Model(task).Updates(map[string]interface{}{
		"allow_entry_swap": true,
		"status":           models.StrategyStatusWaiting,
		"error_message":    "",
	}).Error
	task.AllowEntrySwap = true
	task.Status = models.StrategyStatusWaiting

	enterRes, err := b.liquidityService.EnterTaskFromUSDT(user.ID, task)
	if err != nil {
		_ = database.DB.Model(task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": fmt.Sprintf("enter failed: %v", err),
		}).Error
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 开仓失败：%v", err))
		b.sendMessageWithKeyboard(query.Message.Chat.ID, b.formatTaskCard(task), b.taskKeyboard(task))
		return
	}

	if err := b.applyEnterResult(task, enterRes); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("更新任务失败：%v", err))
		return
	}

	b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("✅ 开仓成功！交易哈希：`%s`", enterRes.TxHash))

	if msg, err := b.sendTaskCardMessage(query.Message.Chat.ID, b.formatTaskCardWithRefresh(task), b.taskKeyboardWithRefresh(task)); err == nil && msg.MessageID != 0 {
		b.startTaskAutoRefresh(query.Message.Chat.ID, msg.MessageID, task.ID, user.ID)
	}
}

func (b *Bot) handleEntrySwapCancel(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, "已取消"))

	taskID, err := parseTaskID("entry_swap_cancel_", query.Data)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "无效的任务ID")
		return
	}

	task, err := b.taskService.GetByID(user.ID, taskID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("获取任务失败：%v", err))
		return
	}
	if taskHasPosition(task) {
		b.sendMessage(query.Message.Chat.ID, "该任务已开仓，无法取消。")
		return
	}

	_ = database.DB.Model(task).Updates(map[string]interface{}{
		"status":        models.StrategyStatusStopped,
		"error_message": "entry swap canceled by user",
	}).Error
	task.Status = models.StrategyStatusStopped
	task.ErrorMessage = "entry swap canceled by user"

	b.sendMessage(query.Message.Chat.ID, "已取消开仓，任务已停止。")
}
