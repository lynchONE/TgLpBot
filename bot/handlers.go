package bot

import (
	"TgLpBot/blockchain"
	"TgLpBot/database"
	"TgLpBot/models"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleCancel handles the /cancel command
func (b *Bot) handleCancel(message *tgbotapi.Message, user *models.User) {
	database.ClearUserSession(user.TelegramID)
	b.sendMessage(message.Chat.ID, "✅ 操作已取消。\n\n使用 /help 查看可用命令。")
}

// handleStart handles the /start command
func (b *Bot) handleStart(message *tgbotapi.Message, user *models.User) {
	text := fmt.Sprintf(`👋 欢迎使用 *LP 自动化机器人*, %s！

我可以帮助您在 BSC（币安智能链）上自动管理流动性仓位。

*功能特性：*
• 💼 管理您的钱包
• 📊 创建和管理流动性仓位
• 🔄 自动再平衡
• 🛡️ 止损保护
• ⚙️ 全局配置（滑点、止损阈值等）
• 📈 跟踪您的交易和仓位

使用 /help 查看所有可用命令。

⚠️ *安全提示：*
您的私钥已加密并安全存储。切勿与任何人分享您的私钥！`, user.FirstName)

	b.sendMessage(message.Chat.ID, text)
}

// handleHelp handles the /help command
func (b *Bot) handleHelp(message *tgbotapi.Message, user *models.User) {
	text := `📚 *可用命令：*

*钱包管理：*
/wallet - 管理您的钱包
/balance - 查看钱包余额

*仓位管理：*
/newposition - 创建新仓位
/positions - 查看我的仓位
/config - 全局配置（滑点、止损、再平衡等）

*信息查询：*
/transactions - 查看交易历史
/help - 显示此帮助信息

*使用方法：*
1. 首先，使用 /wallet 创建或导入钱包
2. 使用 /config 配置全局参数（滑点、止损阈值、再平衡超时）
3. 使用 /newposition 创建新仓位（输入池子地址、tick范围、投入金额）
4. 使用 /positions 查看和管理您的仓位

如需支持，请联系 @yoursupport`

	b.sendMessage(message.Chat.ID, text)
}

// handleWallet handles the /wallet command
func (b *Bot) handleWallet(message *tgbotapi.Message, user *models.User) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ 创建钱包", "create_wallet"),
			tgbotapi.NewInlineKeyboardButtonData("📥 导入钱包", "import_wallet"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👀 查看钱包", "view_wallets"),
		),
	)

	text := "💼 *钱包管理*\n\n请选择一个选项："
	b.sendMessageWithKeyboard(message.Chat.ID, text, keyboard)
}

// handleBalance handles the /balance command
func (b *Bot) handleBalance(message *tgbotapi.Message, user *models.User) {
	wallets, err := b.walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		b.sendMessage(message.Chat.ID, "您还没有任何钱包。使用 /wallet 创建一个。")
		return
	}

	b.sendMessage(message.Chat.ID, "⏳ 正在查询余额...")

	text := "💰 *您的钱包余额：*\n\n"

	for _, wallet := range wallets {
		defaultMark := ""
		if wallet.IsDefault {
			defaultMark = " ⭐"
		}
		text += fmt.Sprintf("*%s*%s\n", wallet.Name, defaultMark)
		text += fmt.Sprintf("`%s`\n", wallet.Address)

		// Get BNB balance
		bnbBalance, usdtBalance := b.getWalletBalances(wallet.Address)
		text += fmt.Sprintf("💎 BNB: %s\n", bnbBalance)
		text += fmt.Sprintf("💵 USDT: %s\n", usdtBalance)
		text += "\n"
	}

	b.sendMessage(message.Chat.ID, text)
}

// getWalletBalances returns formatted BNB and USDT balances
func (b *Bot) getWalletBalances(address string) (string, string) {
	bnbBalance := "查询失败"
	usdtBalance := "查询失败"

	// Get BNB balance
	if blockchain.Client != nil {
		addr := common.HexToAddress(address)
		balance, err := blockchain.GetBalance(addr)
		if err == nil {
			// Convert from wei to BNB (18 decimals)
			bnbFloat := new(big.Float).SetInt(balance)
			divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
			bnbFloat.Quo(bnbFloat, divisor)
			bnbBalance = fmt.Sprintf("%.6f", bnbFloat)
		}

		// Get USDT balance (BSC USDT: 0x55d398326f99059fF775485246999027B3197955)
		usdtAddr := common.HexToAddress("0x55d398326f99059fF775485246999027B3197955")
		usdtBal, err := blockchain.GetTokenBalance(usdtAddr, addr)
		if err == nil {
			// USDT has 18 decimals on BSC
			usdtFloat := new(big.Float).SetInt(usdtBal)
			divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
			usdtFloat.Quo(usdtFloat, divisor)
			usdtBalance = fmt.Sprintf("%.2f", usdtFloat)
		}
	}

	return bnbBalance, usdtBalance
}

// handleNewPosition handles the /newposition command
func (b *Bot) handleNewPosition(message *tgbotapi.Message, user *models.User) {
	// Check if user has a wallet
	wallets, err := b.walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		b.sendMessage(message.Chat.ID, "您还没有任何钱包。请先使用 /wallet 创建一个。")
		return
	}

	// Set user state to expect pool address
	database.SetUserSession(user.TelegramID, "state", "awaiting_pool_address", 30*time.Minute)

	text := "📊 *创建新仓位*\n\n请发送流动性池合约地址。\n\n示例：`0x...`\n\n发送 /cancel 取消此操作。"

	b.sendMessage(message.Chat.ID, text)
}

// handlePositions handles the /positions command
func (b *Bot) handlePositions(message *tgbotapi.Message, user *models.User) {
	tasks, err := b.taskService.ListActive(user.ID, 10)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 获取任务列表失败：%v", err))
		return
	}
	if len(tasks) == 0 {
		b.sendMessage(message.Chat.ID, "📊 *我的仓位*\n\n当前没有运行中的任务。\n\n使用 /newposition 创建一个。")
		return
	}

	b.sendMessage(message.Chat.ID, fmt.Sprintf("📊 *我的仓位*\n\n共 %d 个任务：", len(tasks)))
	for i := range tasks {
		task := tasks[i]
		b.sendMessageWithKeyboard(message.Chat.ID, b.formatTaskCard(&task), b.taskKeyboard(&task))
	}
}

// handleConfig handles the /config command
func (b *Bot) handleConfig(message *tgbotapi.Message, user *models.User) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏱️ 再平衡超时", "config_rebalance_timeout"),
			tgbotapi.NewInlineKeyboardButtonData("⚡ 秒止损开关", "config_stop_loss_toggle"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏲️ 秒止损阈值", "config_stop_loss_delay"),
			tgbotapi.NewInlineKeyboardButtonData("📊 滑点配置", "config_slippage"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔁 复投开关", "config_reinvest_toggle"),
			tgbotapi.NewInlineKeyboardButtonData("🧾 剩余资产容忍度", "config_residual_tolerance"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👀 查看当前配置", "view_config"),
		),
	)

	text := "⚙️ *全局配置*\n\n请选择要配置的选项："
	b.sendMessageWithKeyboard(message.Chat.ID, text, keyboard)
}

// handleTransactions handles the /transactions command
func (b *Bot) handleTransactions(message *tgbotapi.Message, user *models.User) {
	var transactions []models.Transaction
	err := database.DB.Where("user_id = ?", user.ID).
		Order("created_at DESC").
		Limit(10).
		Find(&transactions).Error

	if err != nil {
		b.sendMessage(message.Chat.ID, "获取交易记录时出错。")
		return
	}

	if len(transactions) == 0 {
		b.sendMessage(message.Chat.ID, "您还没有任何交易记录。")
		return
	}

	text := "📊 *最近的交易：*\n\n"

	// Preload related tasks for richer pool display.
	taskByID := make(map[uint]models.StrategyTask)
	var taskIDs []uint
	for _, tx := range transactions {
		if tx.TaskID != 0 {
			taskIDs = append(taskIDs, tx.TaskID)
		}
	}
	if len(taskIDs) > 0 {
		seen := make(map[uint]struct{}, len(taskIDs))
		uniqueIDs := make([]uint, 0, len(taskIDs))
		for _, id := range taskIDs {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			uniqueIDs = append(uniqueIDs, id)
		}

		var tasks []models.StrategyTask
		if err := database.DB.Where("user_id = ? AND id IN ?", user.ID, uniqueIDs).Find(&tasks).Error; err == nil {
			for i := range tasks {
				taskByID[tasks[i].ID] = tasks[i]
			}
		}
	}

	for _, tx := range transactions {
		// Emoji based on type
		actionEmoji := "📝"
		actionName := "未知"
		switch tx.Type {
		case models.TxTypeAddLiquidity:
			actionEmoji = "🟢"
			actionName = "加仓"
		case models.TxTypeRemoveLiquidity:
			actionEmoji = "🔴"
			actionName = "撤仓"
		case models.TxTypeSwap:
			actionEmoji = "🔄"
			actionName = "兑换"
		case models.TxTypeApprove:
			actionEmoji = "🔐"
			actionName = "授权"
		}

		// Format time
		timeStr := tx.CreatedAt.Format("01-02 15:04")

		// Format amount
		// Assuming AmountIn or AmountOut is the main value.
		// For LP Add, we spent AmountIn (USDT).
		// For LP Remove, we got AmountOut (USDT).
		amountStr := "0"
		if tx.AmountIn != "" && tx.AmountIn != "0" {
			// Convert wei to ether-like float for display if needed, but string might be raw wei.
			// Currently our models.Transaction stores raw string. We should format it.
			// Since we don't have easy utils here, let's just use a simple divider if it looks large.
			if len(tx.AmountIn) > 6 {
				// Rough approximation or assume input was already formatted?
				// In liquidity_enter.go we stored amount0In.String() which is raw wei (big.Int).
				// We need to parse and format.
				amountStr = formatWei(tx.AmountIn)
			} else {
				amountStr = tx.AmountIn
			}
		} else if tx.AmountOut != "" && tx.AmountOut != "0" {
			amountStr = formatWei(tx.AmountOut)
		}

		poolInfo := formatTxPoolInfo(&tx, taskByID)

		text += fmt.Sprintf("🕒 %s | %s %s | %s U\n", timeStr, actionEmoji, actionName, amountStr)
		text += fmt.Sprintf("🏊 %s\n", poolInfo)
		if strings.TrimSpace(tx.TxHash) != "" {
			text += fmt.Sprintf("🔗 [查看交易](https://bscscan.com/tx/%s)\n", tx.TxHash)
		}
		text += "\n"
	}

	b.sendMessage(message.Chat.ID, text)
}

// formatWei helper to format large numbers roughly to 2 decimals
func formatWei(weiStr string) string {
	f := new(big.Float)
	f.SetString(weiStr)
	// Assume 18 decimals for display simplicity
	f.Quo(f, big.NewFloat(1e18))
	return fmt.Sprintf("%.2f", f)
}

func formatTxPoolInfo(tx *models.Transaction, taskByID map[uint]models.StrategyTask) string {
	if tx != nil && tx.TaskID != 0 {
		if task, ok := taskByID[tx.TaskID]; ok {
			return formatTaskPoolInfo(&task)
		}
	}

	inAddr := strings.TrimSpace(tx.TokenInAddress)
	outAddr := strings.TrimSpace(tx.TokenOutAddress)
	if inAddr == "" {
		inAddr = "-"
	} else {
		inAddr = shortenHex(inAddr)
	}
	if outAddr == "" {
		outAddr = "-"
	} else {
		outAddr = shortenHex(outAddr)
	}
	return fmt.Sprintf("输入：`%s` → 输出：`%s`", inAddr, outAddr)
}

func formatTaskPoolInfo(task *models.StrategyTask) string {
	pair := formatPair(task.Token0Symbol, task.Token1Symbol)

	poolAddr := strings.TrimSpace(task.PoolId)
	if poolAddr == "" {
		poolAddr = "-"
	}
	exchange := strings.TrimSpace(task.Exchange)
	if exchange == "" {
		exchange = "-"
	}

	return strings.Join([]string{
		pair,
		fmt.Sprintf("池子合约：`%s`", poolAddr),
		fmt.Sprintf("交易所：%s", exchange),
	}, " | ")
}

func formatPair(sym0, sym1 string) string {
	s0 := strings.TrimSpace(sym0)
	s1 := strings.TrimSpace(sym1)
	if s0 == "" && s1 == "" {
		return "-"
	}
	if s0 == "" {
		s0 = "-"
	}
	if s1 == "" {
		s1 = "-"
	}
	return s0 + "/" + s1
}

// isPoolIdentifier checks if the input is a valid pool identifier
// Supports: V3 pool address (40 hex chars) or V4 PoolId (64 hex chars)
func isPoolIdentifier(text string) bool {
	text = strings.TrimSpace(text)
	// Remove 0x prefix if present
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		text = text[2:]
	}
	// Check if it's valid hex
	for _, c := range text {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	// V3 address: 40 hex chars (20 bytes)
	// V4 PoolId: 64 hex chars (32 bytes)
	return len(text) == 40 || len(text) == 64
}

// handleText handles text messages based on user state
func (b *Bot) handleText(message *tgbotapi.Message, user *models.User) {
	// Check if user wants to cancel
	if message.Text == "/cancel" {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(message.Chat.ID, "✅ 操作已取消。\n\n使用 /help 查看可用命令。")
		return
	}

	// Get user state
	state, _ := database.GetUserSession(user.TelegramID, "state")

	// If no state, check if input looks like a pool address or PoolId
	if state == "" {
		text := strings.TrimSpace(message.Text)
		// Check if it's a valid pool identifier (V3 address or V4 PoolId)
		if isPoolIdentifier(text) {
			// Auto-detect pool input, treat as new position
			database.SetUserSession(user.TelegramID, "state", "awaiting_pool_address", 30*time.Minute)
			b.handlePoolAddress(message, user)
			return
		}
		// Not a pool identifier and no state
		b.sendMessage(message.Chat.ID, "💡 *提示：*\n\n直接发送池子地址或 PoolId 即可开始创建仓位。\n\n支持：\n• V3 池子地址（如 0x...，40位）\n• V4 PoolId（如 0x...，64位）\n\n或使用 /help 查看可用命令。")
		return
	}

	switch state {
	case "awaiting_private_key":
		b.handlePrivateKeyInput(message, user)
	case "awaiting_wallet_name":
		b.handleWalletNameInput(message, user)
	case "awaiting_pool_address":
		b.handlePoolAddress(message, user)
	case "awaiting_tick_range":
		b.handleTickRange(message, user)
	case "awaiting_amount":
		b.handlePositionAmount(message, user)
	// Global config inputs
	case "awaiting_global_rebalance_timeout":
		b.handleGlobalRebalanceTimeoutInput(message.Chat.ID, user, message.Text)
	case "awaiting_global_stop_loss_delay":
		b.handleGlobalStopLossDelayInput(message.Chat.ID, user, message.Text)
	case "awaiting_global_slippage":
		b.handleGlobalSlippageInput(message.Chat.ID, user, message.Text)
	case "awaiting_global_residual_tolerance":
		b.handleGlobalResidualToleranceInput(message.Chat.ID, user, message.Text)
	// Task config inputs
	case "awaiting_task_slippage":
		b.handleTaskSlippageInput(message.Chat.ID, user, message.Text)
	case "awaiting_task_rebalance_timeout":
		b.handleTaskRebalanceTimeoutInput(message, user)
	case "awaiting_task_stop_loss_delay":
		b.handleTaskStopLossDelayInput(message.Chat.ID, user, message.Text)
	case "awaiting_task_residual_tolerance":
		b.handleTaskResidualToleranceInput(message.Chat.ID, user, message.Text)
	default:
		// Unknown state, check if it's a pool identifier
		text := strings.TrimSpace(message.Text)
		if isPoolIdentifier(text) {
			database.SetUserSession(user.TelegramID, "state", "awaiting_pool_address", 30*time.Minute)
			b.handlePoolAddress(message, user)
			return
		}
		b.sendMessage(message.Chat.ID, "💡 *提示：*\n\n直接发送池子地址或 PoolId 即可开始创建仓位。\n\n或使用 /help 查看可用命令。")
	}
}

// handleCreateWallet handles wallet creation callback
func (b *Bot) handleCreateWallet(query *tgbotapi.CallbackQuery, user *models.User) {
	// Answer callback
	callback := tgbotapi.NewCallback(query.ID, "正在创建钱包...")
	b.api.Send(callback)

	// Create wallet
	wallet, err := b.walletService.CreateWallet(user.ID, "钱包 "+strconv.Itoa(int(time.Now().Unix())))
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "创建钱包时出错。请重试。")
		return
	}

	text := fmt.Sprintf("✅ *钱包创建成功！*\n\n*地址：* `%s`\n\n*名称：* %s\n\n⚠️ *重要提示：* 请备份您的钱包！如需要，您可以稍后导出私钥。\n\n使用 /balance 查看您的钱包余额。", wallet.Address, wallet.Name)

	b.sendMessage(query.Message.Chat.ID, text)
}

// handleImportWallet handles wallet import callback
func (b *Bot) handleImportWallet(query *tgbotapi.CallbackQuery, user *models.User) {
	// Answer callback
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	// Set user state
	database.SetUserSession(user.TelegramID, "state", "awaiting_private_key", 10*time.Minute)

	text := `📥 *导入钱包*

请发送您的私钥（不带 0x 前缀）。

⚠️ *安全提示：*
• 您的私钥在存储前会被加密
• 发送后请删除您的消息
• 切勿与任何人分享您的私钥

发送 /cancel 取消此操作。`

	b.sendMessage(query.Message.Chat.ID, text)
}

// handleViewWallets handles view wallets callback
func (b *Bot) handleViewWallets(query *tgbotapi.CallbackQuery, user *models.User) {
	// Answer callback
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	wallets, err := b.walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		b.sendMessage(query.Message.Chat.ID, "您还没有任何钱包。请先创建或导入一个钱包。")
		return
	}

	text := "💼 *您的钱包：*\n\n"

	// Build keyboard rows for each wallet
	var keyboardRows [][]tgbotapi.InlineKeyboardButton

	for i, wallet := range wallets {
		defaultMark := ""
		if wallet.IsDefault {
			defaultMark = " ⭐ (默认)"
		}

		// Get balances
		bnbBalance, usdtBalance := b.getWalletBalances(wallet.Address)

		text += fmt.Sprintf("*%d. %s*%s\n", i+1, wallet.Name, defaultMark)
		text += fmt.Sprintf("📍 `%s`\n", wallet.Address)
		text += fmt.Sprintf("💎 BNB: %s\n", bnbBalance)
		text += fmt.Sprintf("💵 USDT: %s\n\n", usdtBalance)

		// Add buttons for this wallet
		var buttons []tgbotapi.InlineKeyboardButton
		if !wallet.IsDefault {
			buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("⭐ 设为默认 #%d", i+1),
				fmt.Sprintf("set_wallet_%d", wallet.ID),
			))
		}
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("🗑️ 删除 #%d", i+1),
			fmt.Sprintf("delete_wallet_%d", wallet.ID),
		))
		keyboardRows = append(keyboardRows, buttons)
	}

	text += "💡 *提示：* ⭐ 标记的是默认钱包，用于所有交易操作。"

	keyboard := tgbotapi.NewInlineKeyboardMarkup(keyboardRows...)
	b.sendMessageWithKeyboard(query.Message.Chat.ID, text, keyboard)
}

// handleSetDefaultWallet handles set default wallet callback
func (b *Bot) handleSetDefaultWallet(query *tgbotapi.CallbackQuery, user *models.User) {
	// Parse wallet ID from callback data
	parts := strings.Split(query.Data, "_")
	if len(parts) < 3 {
		callback := tgbotapi.NewCallback(query.ID, "无效的操作")
		b.api.Send(callback)
		return
	}

	walletID, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		callback := tgbotapi.NewCallback(query.ID, "无效的钱包ID")
		b.api.Send(callback)
		return
	}

	err = b.walletService.SetDefaultWallet(user.ID, uint(walletID))
	if err != nil {
		callback := tgbotapi.NewCallback(query.ID, "设置默认钱包时出错")
		b.api.Send(callback)
		return
	}

	callback := tgbotapi.NewCallback(query.ID, "✅ 默认钱包已更新")
	b.api.Send(callback)

	// Refresh wallet list
	b.handleViewWallets(query, user)
}

// handleDeleteWallet handles delete wallet callback - shows confirmation
func (b *Bot) handleDeleteWallet(query *tgbotapi.CallbackQuery, user *models.User) {
	// Parse wallet ID from callback data
	parts := strings.Split(query.Data, "_")
	if len(parts) < 3 {
		callback := tgbotapi.NewCallback(query.ID, "无效的操作")
		b.api.Send(callback)
		return
	}

	walletID := parts[2]

	// Show confirmation
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚠️ 确认删除", fmt.Sprintf("confirm_delete_%s", walletID)),
			tgbotapi.NewInlineKeyboardButtonData("❌ 取消", "back_to_wallets"),
		),
	)

	b.sendMessageWithKeyboard(query.Message.Chat.ID, "⚠️ *确认删除钱包？*\n\n删除后将无法恢复，请确保已备份私钥！", keyboard)
}

// handleConfirmDeleteWallet handles confirmed wallet deletion
func (b *Bot) handleConfirmDeleteWallet(query *tgbotapi.CallbackQuery, user *models.User) {
	// Parse wallet ID from callback data
	parts := strings.Split(query.Data, "_")
	if len(parts) < 3 {
		callback := tgbotapi.NewCallback(query.ID, "无效的操作")
		b.api.Send(callback)
		return
	}

	walletID, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		callback := tgbotapi.NewCallback(query.ID, "无效的钱包ID")
		b.api.Send(callback)
		return
	}

	err = b.walletService.DeleteWallet(user.ID, uint(walletID))
	if err != nil {
		callback := tgbotapi.NewCallback(query.ID, "删除钱包时出错")
		b.api.Send(callback)
		return
	}

	callback := tgbotapi.NewCallback(query.ID, "✅ 钱包已删除")
	b.api.Send(callback)

	b.sendMessage(query.Message.Chat.ID, "✅ 钱包已成功删除。")

	// Refresh wallet list
	b.handleViewWallets(query, user)
}
