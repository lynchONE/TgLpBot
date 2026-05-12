package bot

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"strings"
	"time"

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

我可以帮助您在已启用链（如 BSC/Base）上自动管理流动性仓位。

*功能特性：*
• 💼 管理您的钱包
• 📊 创建和管理流动性仓位
• 🔄 自动再平衡
? ??? ????
? ?? ???????????????
• 📈 跟踪您的交易和仓位

使用 /help 查看所有可用命令。

⚠️ *安全提示：*
您的私钥已加密并安全存储。切勿与任何人分享您的私钥！`, user.FirstName)

	// 添加授权状态
	text += b.formatAccessStatus(user)

	b.sendMessage(message.Chat.ID, text)
}

// handleHelp handles the /help command
func (b *Bot) handleHelp(message *tgbotapi.Message, user *models.User) {
	text := `📚 *可用命令：*

*钱包管理：*
/wallet - 管理您的钱包
/balance - 查看钱包余额
/swap - 零钱兑换

*仓位管理：*
/newposition - 创建新仓位
/positions - 查看我的仓位
/config - ????????????????

*信息查询：*
/profit - 余额走势
/transactions - 查看交易历史
/miniapp - 打开小程序
/help - 显示此帮助信息

*使用方法：*
1. 首先，使用 /wallet 导入钱包
2. ?? /config ????????????????????
3. 使用 /newposition 创建新仓位（输入池子地址、tick范围、投入金额）
4. 使用 /positions 查看和管理您的仓位

	如需支持，请联系 @yoursupport`

	if config.AppConfig != nil {
		url := strings.TrimSpace(config.AppConfig.TelegramWebAppURL)
		if isValidWebAppURL(url) {
			// 仅对有 MiniApp 权限的用户展示 WebApp 按钮
			hasMiniApp, _ := b.accessService.CheckMiniAppAccess(user.ID)
			if !hasMiniApp {
				b.sendMessage(message.Chat.ID, text)
				return
			}

			msg := tgbotapi.NewMessage(message.Chat.ID, text)
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = newWebAppInlineKeyboardMarkup("实时仓位", url)
			if _, err := b.api.Send(msg); err != nil {
				log.Printf("Error sending help message with webapp button: %v", err)
			}
			return
		}
	}

	b.sendMessage(message.Chat.ID, text)
}

// handleMiniApp handles the /miniapp command
func (b *Bot) handleMiniApp(message *tgbotapi.Message, user *models.User) {
	// 检查 MiniApp 权限（基础授权已在 command 入口统一检查）
	hasAccess, reason := b.accessService.CheckMiniAppAccess(user.ID)
	if !hasAccess {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 您没有 MiniApp 权限\n\n原因：%s\n\n请联系管理员开通。", reason))
		return
	}

	if config.AppConfig == nil {
		b.sendMessage(message.Chat.ID, "小程序入口未配置，请联系管理员。")
		return
	}

	url := strings.TrimSpace(config.AppConfig.TelegramWebAppURL)
	if !isValidWebAppURL(url) {
		b.sendMessage(message.Chat.ID, "小程序入口链接无效，请联系管理员更新配置。")
		return
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, "请点击下方按钮打开小程序。")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = newWebAppInlineKeyboardMarkup("打开小程序", url)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Error sending miniapp message with webapp button: %v", err)
	}
}

// handleWallet handles the /wallet command
func (b *Bot) handleWallet(message *tgbotapi.Message, user *models.User) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📥 导入钱包", "import_wallet"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👀 查看钱包", "view_wallets"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🪙 零钱兑换", "wallet_swap_to_usdt"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📈 余额走势", "view_profit"),
		),
	)

	text := "💼 *钱包管理*\n\n"
	if wallet, err := b.walletService.GetDefaultWallet(user.ID); err == nil {
		text += fmt.Sprintf("💰 *当前默认钱包：* *%s* ⭐\n`%s`\n%s\n\n", wallet.Name, wallet.Address, b.formatWalletBalanceLines(wallet.Address))
	} else {
		text += "当前还没有导入钱包。\n\n"
	}
	text += "请选择一个选项："
	b.sendMessageWithKeyboard(message.Chat.ID, text, keyboard)
}

// handleBalance handles the /balance command
func (b *Bot) handleBalance(message *tgbotapi.Message, user *models.User) {
	wallets, err := b.walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		b.sendMessage(message.Chat.ID, "您还没有任何钱包。使用 /wallet 导入一个。")
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

		text += b.formatWalletBalanceLines(wallet.Address)
		text += "\n\n"
	}

	b.sendMessage(message.Chat.ID, text)
}

// formatWalletBalanceLines formats wallet balances across enabled chains.
func (b *Bot) formatWalletBalanceLines(address string) string {
	chains := enabledChains()
	seen := make(map[string]struct{}, len(chains))
	lines := make([]string, 0, len(chains))

	for _, chain := range chains {
		normalized := config.NormalizeChain(chain)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}

		nativeSym, nativeBal, stableSym, stableBal := b.getWalletBalancesForChain(normalized, address)
		lines = append(lines, fmt.Sprintf("⛓ %s\n💎 %s: %s\n💵 %s: %s", chainLabel(normalized), nativeSym, nativeBal, stableSym, stableBal))
	}

	if len(lines) == 0 {
		nativeSym, nativeBal, stableSym, stableBal := b.getWalletBalancesForChain("bsc", address)
		lines = append(lines, fmt.Sprintf("⛓ %s\n💎 %s: %s\n💵 %s: %s", chainLabel("bsc"), nativeSym, nativeBal, stableSym, stableBal))
	}

	return strings.Join(lines, "\n")
}

// handleNewPosition handles the /newposition command
func (b *Bot) handleNewPosition(message *tgbotapi.Message, user *models.User) {
	// 检查授权
	if !b.checkUserAuthorized(message.Chat.ID, user) {
		return
	}

	// 检查任务额度
	check, _ := b.accessService.CheckUserAccess(user.ID, time.Now())
	if !check.IsAdmin && check.Access != nil {
		taskCount, _ := b.accessService.CountUserActiveTasks(user.ID)
		if taskCount >= int64(check.Access.MaxActiveTasks) {
			b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 您已达到活跃任务数量上限 (%d)。\n\n请先停止其他任务，或联系管理员提升额度。", check.Access.MaxActiveTasks))
			return
		}
	}

	// Check if user has a wallet
	wallets, err := b.walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		b.sendMessage(message.Chat.ID, "您还没有任何钱包。请先使用 /wallet 导入一个。")
		return
	}

	// If user disables multi-chain selection, always use their default chain and skip the chain prompt.
	if cfg, err := b.configService.GetOrCreate(user.ID); err == nil && cfg != nil && !cfg.MultiChainEnabled {
		chain := config.PickEnabledChain(cfg.DefaultChain)
		_ = database.SetUserSession(user.TelegramID, sessionNewPositionChain, chain, 30*time.Minute)
		_ = database.SetUserSession(user.TelegramID, "state", "awaiting_pool_address", 30*time.Minute)

		text := fmt.Sprintf("📊 *创建新仓位*（%s）\n\n请发送流动性池合约地址。\n\n示例：`0x...`\n\n发送 /cancel 取消此操作。", chainLabel(chain))
		b.sendMessage(message.Chat.ID, text)
		return
	}

	// Multi-chain: pick chain first when more than one chain is enabled.
	chains := enabledChains()
	if len(chains) > 1 {
		_ = database.SetUserSession(user.TelegramID, "state", sessionNewPositionState, 30*time.Minute)
		b.sendMessageWithKeyboard(message.Chat.ID, "📊 *确认池子所属链*\n\n请选择链：", newPositionChainKeyboard(chains))
		return
	}
	// Single chain: store it for chain-scoped RPC calls (pool info / tick / balances).
	_ = database.SetUserSession(user.TelegramID, sessionNewPositionChain, config.NormalizeChain(chains[0]), 30*time.Minute)

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
		// Send task card with auto-refresh
		msg, err := b.sendTaskCardMessage(message.Chat.ID, b.formatTaskCardWithRefresh(&task), b.taskKeyboardWithRefresh(&task))
		if err == nil && msg.MessageID != 0 {
			// Start auto-refresh for this message
			b.startTaskAutoRefresh(message.Chat.ID, msg.MessageID, task.ID, user.ID)
		}
	}
}

// handleConfig handles the /config command
func (b *Bot) handleConfig(message *tgbotapi.Message, user *models.User) {
	cfg, err := b.configService.GetOrCreate(user.ID)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 获取配置失败：%v", err))
		return
	}
	b.sendMessageWithKeyboard(message.Chat.ID, formatGlobalConfigMenuText(cfg), globalConfigKeyboard())
}

// handleTransactions handles the /transactions command
func (b *Bot) handleTransactions(message *tgbotapi.Message, user *models.User) {
	var records []models.TradeRecord
	err := database.DB.Where("user_id = ?", user.ID).
		Order("opened_at DESC").
		Limit(10).
		Find(&records).Error

	if err != nil {
		b.sendMessage(message.Chat.ID, "获取交易记录时出错。")
		return
	}

	if len(records) == 0 {
		b.sendMessage(message.Chat.ID, "您还没有任何交易记录。")
		return
	}

	text := "📊 *最近的开/撤仓记录：*\n\n"

	for i, rec := range records {
		statusEmoji := "📝"
		statusText := string(rec.Status)
		switch rec.Status {
		case models.TradeStatusOpen:
			statusEmoji = "🟢"
			statusText = "进行中"
		case models.TradeStatusClosed:
			statusEmoji = "✅"
			statusText = "已结束"
		case models.TradeStatusOrphaned:
			statusEmoji = "⚠️"
			statusText = "记录缺失"
		case models.TradeStatusAborted:
			statusEmoji = "⚠️"
			statusText = "已中止"
		}

		pair := strings.TrimSpace(rec.Token0Symbol) + "/" + strings.TrimSpace(rec.Token1Symbol)
		if strings.TrimSpace(pair) == "/" {
			pair = "-"
		}

		poolId := strings.TrimSpace(rec.PoolId)
		if poolId == "" {
			poolId = "-"
		}
		exchange := strings.TrimSpace(rec.Exchange)
		if exchange == "" {
			exchange = "-"
		}

		loc := timeutil.Location()
		openTime := rec.OpenedAt.In(loc).Format("01-02 15:04")
		stableSym, _, _ := stableSymbolForChain(rec.Chain)
		nativeSym := nativeSymbolForChain(rec.Chain)
		text += fmt.Sprintf("%d. %s *%s* (%s)\n", i+1, statusEmoji, exchange, statusText)
		text += fmt.Sprintf("🕒 开仓：%s\n", openTime)
		text += fmt.Sprintf("🏊 %s | 池子合约：`%s`\n", pair, poolId)
		text += fmt.Sprintf("🟢 投入：%s %s | Gas：%s %s\n", formatWei(rec.OpenUSDTSpent), stableSym, formatWeiDecimals(rec.OpenGasSpentWei, 6), nativeSym)

		if rec.ClosedAt != nil {
			text += fmt.Sprintf("🔴 撤出：%s %s | Gas：%s %s\n", formatWei(rec.CloseUSDTReceived), stableSym, formatWeiDecimals(rec.CloseGasSpentWei, 6), nativeSym)
			text += fmt.Sprintf("⛽ 总Gas费：%s %s\n", formatWei(rec.TotalGasUSDT), stableSym)
			text += fmt.Sprintf("📈 收益：%s %s (%.2f%%) _已扣除Gas_\n", formatWei(rec.ProfitUSDT), stableSym, rec.ProfitPct)
		}

		var links []string
		if strings.TrimSpace(rec.OpenTxHash) != "" {
			links = append(links, fmt.Sprintf("[开仓Tx](%s)", explorerTxURL(rec.Chain, rec.OpenTxHash)))
		}
		if strings.TrimSpace(rec.CloseTxHash) != "" {
			links = append(links, fmt.Sprintf("[撤仓Tx](%s)", explorerTxURL(rec.Chain, rec.CloseTxHash)))
		}
		if len(links) > 0 {
			text += "🔗 " + strings.Join(links, " | ") + "\n"
		}

		text += "\n"
	}

	b.sendMessage(message.Chat.ID, text)
}

// formatWei helper to format 18-decimal wei strings into human readable decimals.
func formatWei(weiStr string) string {
	return formatWeiDecimals(weiStr, 2)
}

func formatWeiDecimals(weiStr string, decimals int) string {
	weiStr = strings.TrimSpace(weiStr)
	if weiStr == "" {
		weiStr = "0"
	}
	v, ok := new(big.Int).SetString(weiStr, 10)
	if !ok {
		return "0"
	}
	r := new(big.Rat).SetInt(v)
	denom := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	r.Quo(r, denom)
	return r.FloatString(decimals)
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
	ensureChainForPoolInput := func(poolInput string) bool {
		chain, _ := database.GetUserSession(user.TelegramID, sessionNewPositionChain)
		if strings.TrimSpace(chain) != "" {
			return true
		}

		if cfg, err := b.configService.GetOrCreate(user.ID); err == nil && cfg != nil && !cfg.MultiChainEnabled {
			chain := config.PickEnabledChain(cfg.DefaultChain)
			_ = database.SetUserSession(user.TelegramID, sessionNewPositionChain, chain, 30*time.Minute)
			return true
		}

		chains := enabledChains()
		if len(chains) == 1 {
			_ = database.SetUserSession(
				user.TelegramID,
				sessionNewPositionChain,
				config.NormalizeChain(chains[0]),
				30*time.Minute,
			)
			return true
		}

		_ = database.SetUserSession(user.TelegramID, sessionPendingPoolInput, strings.TrimSpace(poolInput), 30*time.Minute)
		_ = database.SetUserSession(user.TelegramID, "state", sessionNewPositionState, 30*time.Minute)
		b.sendMessageWithKeyboard(
			message.Chat.ID,
			"📊 *创建新仓位*\n\n已检测到池子地址/PoolId，请先选择链：",
			newPositionChainKeyboard(chains),
		)
		return false
	}
	ensureWalletForPoolInput := func(poolInput string) bool {
		if raw, _ := database.GetUserSession(user.TelegramID, sessionNewPositionWalletID); strings.TrimSpace(raw) != "" {
			return true
		}

		wallets, err := b.walletService.GetUserWallets(user.ID)
		if err != nil || len(wallets) == 0 {
			b.sendMessage(message.Chat.ID, "⚠️ 您还没有钱包，请先使用 /wallet 导入。")
			database.ClearUserSession(user.TelegramID)
			return false
		}

		cfg, _ := b.configService.GetOrCreate(user.ID)
		if cfg != nil && cfg.MultiWalletEnabled && len(wallets) > 1 {
			chain, _ := database.GetUserSession(user.TelegramID, sessionNewPositionChain)
			chain = config.NormalizeChain(chain)
			_ = database.SetUserSession(user.TelegramID, sessionPendingPoolInput, strings.TrimSpace(poolInput), 30*time.Minute)
			b.promptNewPositionWalletSelect(message.Chat.ID, user, chain)
			return false
		}

		// Single-wallet mode (or only one wallet): use default wallet and continue.
		defaultWallet := wallets[0]
		for _, w := range wallets {
			if w.IsDefault {
				defaultWallet = w
				break
			}
		}
		_ = database.SetUserSession(user.TelegramID, sessionNewPositionWalletID, fmt.Sprintf("%d", defaultWallet.ID), 30*time.Minute)
		return true
	}

	// If no state, check if input looks like a pool address or PoolId
	if state == "" {
		text := strings.TrimSpace(message.Text)
		// Check if it's a valid pool identifier (V3 address or V4 PoolId)
		if isPoolIdentifier(text) {
			if !b.checkUserAuthorized(message.Chat.ID, user) {
				return
			}
			if !ensureChainForPoolInput(text) {
				return
			}
			if !ensureWalletForPoolInput(text) {
				return
			}
			// Auto-detect pool input, treat as new position
			_ = database.SetUserSession(user.TelegramID, "state", "awaiting_pool_address", 30*time.Minute)
			b.handlePoolAddress(message, user)
			return
		}
		// Not a pool identifier and no state
		b.sendMessage(message.Chat.ID, "💡 *提示：*\n\n直接发送池子地址或 PoolId 即可开始创建仓位。\n\n支持：\n• V3 池子地址（如 0x...，40位）\n• V4 PoolId（如 0x...，64位）\n\n或使用 /help 查看可用命令。")
		return
	}

	if state != "awaiting_auth_code" {
		if !b.checkUserAuthorized(message.Chat.ID, user) {
			return
		}
	}

	switch state {
	case "awaiting_private_key":
		b.handlePrivateKeyInput(message, user)
	case "awaiting_wallet_name":
		b.handleWalletNameInput(message, user)
	case sessionNewPositionState:
		b.handleNewPositionChainText(message, user)
	case sessionNewPositionWalletState:
		b.handleNewPositionWalletText(message, user)
	case "awaiting_pool_address":
		b.handlePoolAddress(message, user)
	case "awaiting_tick_range":
		b.handleTickRange(message, user)
	case "awaiting_amount":
		b.handlePositionAmount(message, user)
	// Global config inputs
	case "awaiting_global_rebalance_timeout":
		b.handleGlobalRebalanceTimeoutInput(message.Chat.ID, user, message.Text)
	case "awaiting_global_slippage":
		b.handleGlobalSlippageInput(message.Chat.ID, user, message.Text)
	case "awaiting_global_residual_tolerance":
		b.handleGlobalResidualToleranceInput(message.Chat.ID, user, message.Text)
	case "awaiting_global_zap_loss_tolerance":
		b.handleGlobalZapLossToleranceInput(message.Chat.ID, user, message.Text)
	case "awaiting_global_bark_key":
		b.handleGlobalBarkKeyInput(message.Chat.ID, user, message.Text)
	case "awaiting_global_bark_server":
		b.handleGlobalBarkServerInput(message.Chat.ID, user, message.Text)
	case "awaiting_global_bark_group":
		b.handleGlobalBarkGroupInput(message.Chat.ID, user, message.Text)
	// Task config inputs
	case "awaiting_task_slippage":
		b.handleTaskSlippageInput(message.Chat.ID, user, message.Text)
	case "awaiting_task_range":
		b.handleTaskRangeInput(message, user)
	case "awaiting_task_rebalance_timeout":
		b.handleTaskRebalanceTimeoutInput(message, user)
	case "awaiting_task_stop_loss_delay":
		b.handleTaskStopLossDelayInput(message.Chat.ID, user, message.Text)
	case "awaiting_task_partial_exit":
		b.handleTaskPartialExitInput(message, user)
	case "awaiting_task_residual_tolerance":
		b.handleTaskResidualToleranceInput(message.Chat.ID, user, message.Text)
	// 授权码和管理员相关输入
	case "awaiting_auth_code":
		b.handleRedeemCode(message, user)
	case "awaiting_auth_code_params":
		b.handleAuthCodeParamsInput(message, user)
	case "awaiting_announcement":
		b.handleAnnouncementInput(message, user)
	case "awaiting_code_edit_params":
		b.handleCodeEditInput(message, user)
	case "awaiting_user_search":
		b.handleUserSearchInput(message, user)
	case "awaiting_user_edit_params":
		b.handleUserEditInput(message, user)
	default:
		// Unknown state, check if it's a pool identifier
		text := strings.TrimSpace(message.Text)
		if isPoolIdentifier(text) {
			if !ensureChainForPoolInput(text) {
				return
			}
			_ = database.SetUserSession(user.TelegramID, "state", "awaiting_pool_address", 30*time.Minute)
			b.handlePoolAddress(message, user)
			return
		}
		b.sendMessage(message.Chat.ID, "💡 *提示：*\n\n直接发送池子地址或 PoolId 即可开始创建仓位。\n\n或使用 /help 查看可用命令。")
	}
}

// handleCreateWallet handles wallet creation callback
func (b *Bot) handleCreateWallet(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "暂不支持创建钱包")
	b.api.Send(callback)
	b.sendMessage(query.Message.Chat.ID, "当前仅支持导入钱包。请使用 /wallet 选择“导入钱包”。")
	return
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
		b.sendMessage(query.Message.Chat.ID, "您还没有任何钱包。请先导入一个钱包。")
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

		text += fmt.Sprintf("*%d. %s*%s\n", i+1, wallet.Name, defaultMark)
		text += fmt.Sprintf("📍 `%s`\n", wallet.Address)
		text += b.formatWalletBalanceLines(wallet.Address)
		text += "\n\n"

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
