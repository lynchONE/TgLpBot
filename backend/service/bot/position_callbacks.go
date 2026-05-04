package bot

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/strategy"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleConfirmPosition handles the confirm position button callback
func (b *Bot) handleConfirmPosition(query *tgbotapi.CallbackQuery, user *models.User) {
	// Answer callback
	callback := tgbotapi.NewCallback(query.ID, "正在创建仓位...")
	b.api.Send(callback)

	// Get stored data
	chain, chainErr := resolveNewPositionChain(user.ID, user.TelegramID)
	if chainErr != nil {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(query.Message.Chat.ID, "会话已过期或未选择链。请重新使用 /newposition 并先选择链。")
		return
	}
	poolAddress, _ := database.GetUserSession(user.TelegramID, "pool_address")
	poolVersion, _ := database.GetUserSession(user.TelegramID, "pool_version")
	poolExchange, _ := database.GetUserSession(user.TelegramID, "pool_exchange")
	token0Symbol, _ := database.GetUserSession(user.TelegramID, "pool_token0")
	token1Symbol, _ := database.GetUserSession(user.TelegramID, "pool_token1")
	token0Addr, _ := database.GetUserSession(user.TelegramID, "pool_token0_address")
	token1Addr, _ := database.GetUserSession(user.TelegramID, "pool_token1_address")
	hooksAddr, _ := database.GetUserSession(user.TelegramID, "pool_hooks_address")
	if strings.TrimSpace(hooksAddr) == "" {
		hooksAddr = "0x0000000000000000000000000000000000000000"
	}
	feeStr, _ := database.GetUserSession(user.TelegramID, "pool_fee")
	tickSpacingStr, _ := database.GetUserSession(user.TelegramID, "pool_tick_spacing")
	rangePctStr, _ := database.GetUserSession(user.TelegramID, "tick_percentage")
	rangeLowerPctStr, _ := database.GetUserSession(user.TelegramID, "tick_lower_percentage")
	rangeUpperPctStr, _ := database.GetUserSession(user.TelegramID, "tick_upper_percentage")
	tickLowerStr, _ := database.GetUserSession(user.TelegramID, "tick_lower")
	tickUpperStr, _ := database.GetUserSession(user.TelegramID, "tick_upper")
	amountStr, _ := database.GetUserSession(user.TelegramID, "position_amount")

	tickLower, _ := strconv.Atoi(tickLowerStr)
	tickUpper, _ := strconv.Atoi(tickUpperStr)
	amount, _ := strconv.ParseFloat(amountStr, 64)
	fee, _ := strconv.Atoi(feeStr)
	tickSpacing, _ := strconv.Atoi(tickSpacingStr)
	rangePct, _ := strconv.ParseFloat(rangePctStr, 64)
	rangeLowerPct, _ := strconv.ParseFloat(rangeLowerPctStr, 64)
	rangeUpperPct, _ := strconv.ParseFloat(rangeUpperPctStr, 64)
	if rangeLowerPct <= 0 || rangeUpperPct <= 0 {
		rangeLowerPct = 0
		rangeUpperPct = 0
	}

	cfg, cfgErr := b.configService.GetOrCreate(user.ID)
	if cfgErr != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("获取全局配置失败: %v", cfgErr))
		return
	}

	slippage := cfg.SlippageTolerance
	if slippageStr, err := database.GetUserSession(user.TelegramID, "position_slippage"); err == nil {
		if v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(slippageStr, "%")), 64); err == nil && v >= 0 && v <= 100 {
			slippage = v
		}
	}

	// Resolve wallet selection (or default wallet) before clearing session.
	selectedWallet, werr := b.resolveNewPositionWallet(user.ID, user.TelegramID)
	if werr != nil || selectedWallet == nil {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您还没有钱包，请先用 /wallet 导入。")
		database.ClearUserSession(user.TelegramID)
		return
	}

	// Clear session
	database.ClearUserSession(user.TelegramID)

	// Create Strategy Task
	task := &models.StrategyTask{
		UserID:               user.ID,
		Chain:                chain,
		PoolId:               poolAddress,
		PoolVersion:          poolVersion,
		Exchange:             poolExchange,
		WalletID:             selectedWallet.ID,
		WalletAddress:        selectedWallet.Address,
		Token0Symbol:         token0Symbol,
		Token1Symbol:         token1Symbol,
		Token0Address:        token0Addr,
		Token1Address:        token1Addr,
		HooksAddress:         hooksAddr,
		Fee:                  fee,
		TickSpacing:          tickSpacing,
		TickLower:            tickLower,
		TickUpper:            tickUpper,
		RangePercentage:      rangePct,
		RangeLowerPercentage: rangeLowerPct,
		RangeUpperPercentage: rangeUpperPct,
		AmountUSDT:           amount,
		CurrentLiquidity:     "0", // Will be updated after zap in
		ReopenDelaySeconds:   strategy.NormalizeRebalanceTimeout(cfg.RebalanceTimeout),
		SlippageTolerance:    slippage,
		AutoReinvest:         cfg.AutoReinvest,
		RebalanceEnabled:     false,
		OutOfRangeMode:       string(models.StrategyOutOfRangeModeExitAll),
		Paused:               true,
		Status:               models.StrategyStatusRunning,
		LastCheckTime:        time.Now(),
	}
	now := time.Now()
	task.PausedAt = &now

	if err := b.strategyService.CreateTask(task); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("创建任务失败: %v", err))
		return
	}

	b.sendMessage(query.Message.Chat.ID, "任务已创建，正在准备开仓...")

	enterRes, err := b.liquidityService.EnterTaskFromUSDT(user.ID, task)
	if err != nil {
		var swapErr *liquidity.EntrySwapRequiredError
		if errors.As(err, &swapErr) {
			b.promptEntrySwap(query.Message.Chat.ID, task, swapErr.TokenSymbol)
			return
		}
		_ = database.DB.Model(task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": fmt.Sprintf("enter failed: %v", err),
		}).Error
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("开仓失败：%v", err))
		b.sendMessageWithKeyboard(query.Message.Chat.ID, b.formatTaskCard(task), b.taskKeyboard(task))
		return
	}

	if err := b.applyEnterResult(task, enterRes); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("更新任务失败：%v", err))
		return
	}

	b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("开仓成功！交易哈希：`%s`", enterRes.TxHash))
	if msg, err := b.sendTaskCardMessage(query.Message.Chat.ID, b.formatTaskCardWithRefresh(task), b.taskKeyboardWithRefresh(task)); err == nil && msg.MessageID != 0 {
		b.startTaskAutoRefresh(query.Message.Chat.ID, msg.MessageID, task.ID, user.ID)
	}
	b.sendMessage(query.Message.Chat.ID, "任务已开始监控。\n\n使用 /positions 查看所有任务。")
}

// handleCancelPosition handles the cancel position button callback
func (b *Bot) handleCancelPosition(query *tgbotapi.CallbackQuery, user *models.User) {
	// Answer callback
	callback := tgbotapi.NewCallback(query.ID, "已取消")
	b.api.Send(callback)

	// Clear session
	database.ClearUserSession(user.TelegramID)

	b.sendMessage(query.Message.Chat.ID, "仓位创建已取消。\n\n使用 /newposition 重新开始。")
}

// handleBackToInput handles the back to input button callback
func (b *Bot) handleBackToInput(query *tgbotapi.CallbackQuery, user *models.User) {
	// Answer callback
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	// Reset state to awaiting_tick_range
	database.SetUserSession(user.TelegramID, "state", "awaiting_tick_range", 30*time.Minute) // 30 minutes

	chain, _ := database.GetUserSession(user.TelegramID, sessionNewPositionChain)
	stableSym, _, _ := stableSymbolForChain(chain)
	text := fmt.Sprintf(
		"🔁 *返回输入*\n\n请重新输入百分比范围和投入金额：\n\n"+
			"1) `100 5`（投入 100 %s，价格上下 5%%）\n"+
			"2) `100 1 3`（投入 100 %s，下 1%% 上 3%%）\n\n"+
			"可选滑点：末尾追加 `s=0.5`\n发送 /cancel 取消。",
		stableSym,
		stableSym,
	)

	b.sendMessage(query.Message.Chat.ID, text)
}
