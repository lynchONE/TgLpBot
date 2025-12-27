package bot

import (
	"TgLpBot/database"
	"TgLpBot/models"
	"TgLpBot/services"
	"errors"
	"fmt"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleConfirmPosition handles the confirm position button callback
func (b *Bot) handleConfirmPosition(query *tgbotapi.CallbackQuery, user *models.User) {
	// Answer callback
	callback := tgbotapi.NewCallback(query.ID, "正在创建仓位...")
	b.api.Send(callback)

	// Get stored data
	poolAddress, _ := database.GetUserSession(user.TelegramID, "pool_address")
	poolVersion, _ := database.GetUserSession(user.TelegramID, "pool_version")
	poolExchange, _ := database.GetUserSession(user.TelegramID, "pool_exchange")
	token0Symbol, _ := database.GetUserSession(user.TelegramID, "pool_token0")
	token1Symbol, _ := database.GetUserSession(user.TelegramID, "pool_token1")
	feeStr, _ := database.GetUserSession(user.TelegramID, "pool_fee")
	tickSpacingStr, _ := database.GetUserSession(user.TelegramID, "pool_tick_spacing")
	rangePctStr, _ := database.GetUserSession(user.TelegramID, "tick_percentage")
	tickLowerStr, _ := database.GetUserSession(user.TelegramID, "tick_lower")
	tickUpperStr, _ := database.GetUserSession(user.TelegramID, "tick_upper")
	amountStr, _ := database.GetUserSession(user.TelegramID, "position_amount")

	tickLower, _ := strconv.Atoi(tickLowerStr)
	tickUpper, _ := strconv.Atoi(tickUpperStr)
	amount, _ := strconv.ParseFloat(amountStr, 64)
	fee, _ := strconv.Atoi(feeStr)
	tickSpacing, _ := strconv.Atoi(tickSpacingStr)
	rangePct, _ := strconv.ParseFloat(rangePctStr, 64)

	cfg, cfgErr := b.configService.GetOrCreate(user.ID)
	if cfgErr != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 获取全局配置失败: %v", cfgErr))
		return
	}

	// Clear session
	database.ClearUserSession(user.TelegramID)

	// Create Strategy Task
	task := &models.StrategyTask{
		UserID:               user.ID,
		PoolId:               poolAddress,
		PoolVersion:          poolVersion,
		Exchange:             poolExchange,
		Token0Symbol:         token0Symbol,
		Token1Symbol:         token1Symbol,
		Fee:                  fee,
		TickSpacing:          tickSpacing,
		TickLower:            tickLower,
		TickUpper:            tickUpper,
		RangePercentage:      rangePct,
		AmountUSDT:           amount,
		CurrentLiquidity:     "0", // Will be updated after zap in
		ReopenDelaySeconds:   cfg.RebalanceTimeout,
		SlippageTolerance:    cfg.SlippageTolerance,
		AutoReinvest:         cfg.AutoReinvest,
		ResidualTolerance:    cfg.ResidualTolerance,
		StopLossEnabled:      cfg.StopLossEnabled,
		StopLossDelaySeconds: cfg.StopLossDelaySeconds,
		Status:               models.StrategyStatusRunning,
		LastCheckTime:        time.Now(),
	}

	if err := b.strategyService.CreateTask(task); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 创建任务失败: %v", err))
		return
	}

	b.sendMessage(query.Message.Chat.ID, "⛓️ 任务已创建，正在准备开仓...")

	enterRes, err := b.liquidityService.EnterTaskFromUSDT(user.ID, task)
	if err != nil {
		var swapErr *services.EntrySwapRequiredError
		if errors.As(err, &swapErr) {
			b.promptEntrySwap(query.Message.Chat.ID, task, swapErr.TokenSymbol)
			return
		}
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
	b.sendMessage(query.Message.Chat.ID, "✅ 任务已开始监控。\n\n使用 /positions 查看所有任务。")
}

// handleCancelPosition handles the cancel position button callback
func (b *Bot) handleCancelPosition(query *tgbotapi.CallbackQuery, user *models.User) {
	// Answer callback
	callback := tgbotapi.NewCallback(query.ID, "已取消")
	b.api.Send(callback)

	// Clear session
	database.ClearUserSession(user.TelegramID)

	b.sendMessage(query.Message.Chat.ID, "❌ 仓位创建已取消。\n\n使用 /newposition 重新开始。")
}

// handleBackToInput handles the back to input button callback
func (b *Bot) handleBackToInput(query *tgbotapi.CallbackQuery, user *models.User) {
	// Answer callback
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	// Reset state to awaiting_tick_range
	database.SetUserSession(user.TelegramID, "state", "awaiting_tick_range", 30*time.Minute) // 30 minutes

	text := `🔙 *返回输入*

请重新输入百分比范围和投入金额：

*格式选项：*
1️⃣ 使用百分比范围: '5 100' (表示当前价格 ±5%, 投入 100 USDT)

发送 /cancel 取消此操作。`

	b.sendMessage(query.Message.Chat.ID, text)
}
