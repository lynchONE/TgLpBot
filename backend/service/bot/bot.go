package bot

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/assets"
	"TgLpBot/service/exchange"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/pool"
	"TgLpBot/service/strategy"
	"TgLpBot/service/user"
	"TgLpBot/service/wallet"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot represents the Telegram bot.
type Bot struct {
	api              *tgbotapi.BotAPI
	accessService    *user.AccessService
	userService      *user.UserService
	walletService    *wallet.WalletService
	liquidityService *liquidity.LiquidityService
	okxService       *exchange.OKXDexService
	poolService      *pool.PoolService
	strategyService  *strategy.StrategyService
	configService    *user.GlobalConfigService
	taskService      *strategy.StrategyTaskService
	snapshotService  *wallet.BalanceSnapshotService
	assetService     *assets.Service
	pnlService       *strategy.PnLService
}

// NewBot creates a new bot instance.
func NewBot() (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(config.AppConfig.TelegramBotToken)
	if err != nil {
		return nil, err
	}

	api.Debug = false
	log.Printf("Authorized on account %s", api.Self.UserName)

	bot := &Bot{
		api:              api,
		accessService:    user.NewAccessService(),
		userService:      user.NewUserService(),
		walletService:    wallet.NewWalletService(),
		liquidityService: liquidity.NewLiquidityService(),
		okxService:       exchange.NewOKXDexService(),
		poolService:      pool.NewPoolService(),
		strategyService:  strategy.NewStrategyService(),
		configService:    user.NewGlobalConfigService(),
		taskService:      strategy.NewStrategyTaskService(),
		snapshotService:  wallet.NewBalanceSnapshotService(),
		assetService:     assets.NewService(),
		pnlService:       strategy.NewPnLService(),
	}

	bot.strategyService.SetNotifier(func(userID uint, message string) {
		user, err := bot.userService.GetUserByID(userID)
		if err == nil {
			bot.sendMessage(user.TelegramID, message)
		} else {
			log.Printf("Failed to notify user %d: %v", userID, err)
		}
	})
	bot.strategyService.SetTaskCardNotifier(func(userID uint, taskID uint) {
		user, err := bot.userService.GetUserByID(userID)
		if err != nil {
			log.Printf("Failed to notify task card user %d: %v", userID, err)
			return
		}
		task, err := bot.taskService.GetByID(userID, taskID)
		if err != nil {
			log.Printf("Failed to notify task card user %d task #%d: %v", userID, taskID, err)
			return
		}

		msg, err := bot.sendTaskCardMessage(user.TelegramID, bot.formatTaskCardWithRefresh(task), bot.taskKeyboardWithRefresh(task))
		if err == nil && msg.MessageID != 0 {
			bot.startTaskAutoRefresh(user.TelegramID, msg.MessageID, task.ID, userID)
		}
	})

	if err := bot.setCommands(); err != nil {
		log.Printf("Warning: Failed to set bot commands: %v", err)
	}
	if err := bot.setMenuButton(); err != nil {
		log.Printf("Warning: Failed to set bot menu button: %v", err)
	}

	return bot, nil
}

func (b *Bot) setCommands() error {
	if err := b.clearCommands(); err != nil {
		log.Printf("Warning: Failed to clear bot commands: %v", err)
	}

	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "开始使用机器人"},
		{Command: "positions", Description: "查看我的仓位"},
		{Command: "miniapp", Description: "打开小程序"},
		{Command: "config", Description: "全局配置"},
		{Command: "profit", Description: "余额走势"},
		{Command: "wallet", Description: "管理钱包"},
		{Command: "swap", Description: "零钱兑换"},
		{Command: "transactions", Description: "查看交易历史"},
	}

	cfg := tgbotapi.NewSetMyCommands(commands...)
	_, err := b.api.Request(cfg)
	if err != nil {
		return err
	}

	log.Println("Bot commands set successfully")
	return nil
}

func (b *Bot) clearCommands() error {
	cfg := tgbotapi.NewDeleteMyCommandsWithScope(tgbotapi.NewBotCommandScopeDefault())
	_, err := b.api.Request(cfg)
	return err
}

type chatMenuButton struct {
	Type   string      `json:"type"`
	Text   string      `json:"text,omitempty"`
	WebApp *webAppInfo `json:"web_app,omitempty"`
}

func (b *Bot) setMenuButton() error {
	if config.AppConfig == nil {
		return nil
	}

	mode := strings.ToLower(strings.TrimSpace(config.AppConfig.TelegramMenuButtonMode))
	if mode == "" {
		mode = "commands"
	}

	var btn any
	switch mode {
	case "web_app":
		url := strings.TrimSpace(config.AppConfig.TelegramWebAppURL)
		if url == "" {
			log.Println("TELEGRAM_WEBAPP_URL not set; fallback to commands menu button")
			btn = chatMenuButton{Type: "commands"}
		} else {
			btn = chatMenuButton{
				Type:   "web_app",
				Text:   "实时仓位",
				WebApp: &webAppInfo{URL: url},
			}
		}
	case "default":
		btn = chatMenuButton{Type: "default"}
	case "commands":
		btn = chatMenuButton{Type: "commands"}
	default:
		log.Printf("Unknown TELEGRAM_MENU_BUTTON_MODE=%q; fallback to commands", mode)
		btn = chatMenuButton{Type: "commands"}
	}

	params := tgbotapi.Params{}
	if err := params.AddInterface("menu_button", btn); err != nil {
		return err
	}
	_, err := b.api.MakeRequest("setChatMenuButton", params)
	return err
}

// Start starts the bot.
func (b *Bot) Start() {
	b.strategyService.Start()
	if b.snapshotService != nil {
		b.snapshotService.Start()
	}
	if b.assetService != nil {
		b.assetService.Start()
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)
	for update := range updates {
		if update.Message != nil {
			b.handleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			b.handleCallbackQuery(update.CallbackQuery)
		}
	}
}

func (b *Bot) handleMessage(message *tgbotapi.Message) {
	user, err := b.userService.GetOrCreateUser(
		message.From.ID,
		message.From.UserName,
		message.From.FirstName,
		message.From.LastName,
		message.From.LanguageCode,
	)
	if err != nil {
		log.Printf("Error getting/creating user: %v", err)
		b.sendMessage(message.Chat.ID, "处理您的请求时出错，请重试。")
		return
	}

	if message.IsCommand() {
		b.handleCommand(message, user)
		return
	}

	b.handleText(message, user)
}

func (b *Bot) handleCommand(message *tgbotapi.Message, user *models.User) {
	cmd := message.Command()
	if cmd != "start" && cmd != "help" && cmd != "cancel" && cmd != "weblogin" {
		if !b.checkUserAuthorized(message.Chat.ID, user) {
			return
		}
	}

	switch cmd {
	case "start":
		b.handleStart(message, user)
	case "help":
		b.handleHelp(message, user)
	case "clean":
		b.handleClean(message, user)
	case "wallet":
		b.handleWallet(message, user)
	case "newposition":
		b.handleNewPosition(message, user)
	case "positions":
		b.handlePositions(message, user)
	case "miniapp":
		b.handleMiniApp(message, user)
	case "config":
		b.handleConfig(message, user)
	case "balance":
		b.handleBalance(message, user)
	case "transactions":
		b.handleTransactions(message, user)
	case "profit":
		b.handleProfit(message, user)
	case "swap":
		b.handleSwapToUSDT(message, user)
	case "cancel":
		b.handleCancel(message, user)
	case "admin":
		b.handleAdmin(message, user)
	case "weblogin":
		b.handleWebLogin(message, user)
	default:
		b.sendMessage(message.Chat.ID, "未知命令。使用 /help 查看可用命令。")
	}
}

func (b *Bot) sendMessage(chatID int64, text string) (tgbotapi.Message, error) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if sentMsg, err := b.api.Send(msg); err != nil {
		if strings.Contains(err.Error(), "can't parse entities") {
			msg.ParseMode = ""
			if sentMsg2, err2 := b.api.Send(msg); err2 == nil {
				return sentMsg2, nil
			}
		}
		log.Printf("Error sending message: %v", err)
		return tgbotapi.Message{}, err
	} else {
		return sentMsg, nil
	}
}

func (b *Bot) sendMessageWithKeyboard(chatID int64, text string, replyMarkup any) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = replyMarkup
	if _, err := b.api.Send(msg); err != nil {
		if strings.Contains(err.Error(), "can't parse entities") {
			msg.ParseMode = ""
			if _, err2 := b.api.Send(msg); err2 == nil {
				return
			}
		}
		log.Printf("Error sending message: %v", err)
	}
}

func (b *Bot) sendMessageWithKeyboardRet(chatID int64, text string, replyMarkup any) (tgbotapi.Message, error) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = replyMarkup
	if sentMsg, err := b.api.Send(msg); err != nil {
		if strings.Contains(err.Error(), "can't parse entities") {
			msg.ParseMode = ""
			if sentMsg2, err2 := b.api.Send(msg); err2 == nil {
				return sentMsg2, nil
			} else {
				log.Printf("Error sending message (Markdown): %v; fallback plain text failed: %v", err, err2)
				return tgbotapi.Message{}, err2
			}
		}
		log.Printf("Error sending message: %v", err)
		return tgbotapi.Message{}, err
	} else {
		return sentMsg, nil
	}
}

func (b *Bot) editMessageText(chatID int64, messageID int, text string) error {
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.DisableWebPagePreview = true

	if _, err := b.api.Send(editMsg); err != nil {
		if strings.Contains(err.Error(), "message is not modified") {
			return nil
		}
		if strings.Contains(err.Error(), "can't parse entities") {
			editMsg.ParseMode = ""
			if _, err2 := b.api.Send(editMsg); err2 == nil {
				return nil
			} else {
				log.Printf("Error editing message (Markdown): %v; fallback plain text failed: %v", err, err2)
				return err2
			}
		}
		return err
	}
	return nil
}

func (b *Bot) editMessageReplyMarkup(chatID int64, messageID int, replyMarkup any) error {
	params := tgbotapi.Params{}
	params.AddFirstValid("chat_id", chatID, "")
	params.AddNonZero("message_id", messageID)
	if err := params.AddInterface("reply_markup", replyMarkup); err != nil {
		return err
	}
	_, err := b.api.MakeRequest("editMessageReplyMarkup", params)
	return err
}

func (b *Bot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	user, err := b.userService.GetUserByTelegramID(query.From.ID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		return
	}

	chatID := int64(query.From.ID)
	if query.Message != nil && query.Message.Chat != nil && query.Message.Chat.ID != 0 {
		chatID = query.Message.Chat.ID
	}
	if !b.checkUserAuthorized(chatID, user) {
		callback := tgbotapi.NewCallback(query.ID, "")
		_, _ = b.api.Request(callback)
		return
	}

	switch {
	case query.Data == "admin_auth_codes":
		b.handleAdminAuthCodes(query, user)
	case query.Data == "admin_create_code":
		b.handleAdminCreateCode(query, user)
	case strings.HasPrefix(query.Data, "admin_quick_code_"):
		b.handleAdminQuickCode(query, user)
	case query.Data == "admin_custom_code":
		b.handleAdminCustomCode(query, user)
	case query.Data == "admin_users":
		b.handleAdminUsers(query, user)
	case query.Data == "admin_user_search":
		b.handleAdminUserSearch(query, user)
	case strings.HasPrefix(query.Data, "admin_users_page_"):
		page, _ := strconv.Atoi(strings.TrimPrefix(query.Data, "admin_users_page_"))
		b.handleAdminUsersPage(query, user, page)
	case strings.HasPrefix(query.Data, "admin_user_edit_"):
		b.handleAdminUserEdit(query, user)
	case strings.HasPrefix(query.Data, "admin_user_"):
		b.handleAdminUserDetail(query, user)
	case strings.HasPrefix(query.Data, "admin_revoke_"):
		b.handleAdminUserRevoke(query, user)
	case strings.HasPrefix(query.Data, "admin_restore_"):
		b.handleAdminUserRestore(query, user)
	case query.Data == "admin_announcement":
		b.handleAdminAnnouncement(query, user)
	case query.Data == "admin_announce_normal":
		b.handleAdminAnnounceNormal(query, user)
	case query.Data == "admin_announce_pinned":
		b.handleAdminAnnouncePinned(query, user)
	case query.Data == "admin_back":
		b.handleAdminBack(query, user)
	case strings.HasPrefix(query.Data, "admin_code_edit_"):
		b.handleAdminCodeEdit(query, user)
	case strings.HasPrefix(query.Data, "admin_code_disable_"):
		b.handleAdminCodeDisable(query, user)
	case strings.HasPrefix(query.Data, "admin_code_enable_"):
		b.handleAdminCodeEnable(query, user)
	case strings.HasPrefix(query.Data, "admin_code_"):
		b.handleAdminCodeDetail(query, user)
	case query.Data == "create_wallet":
		b.handleCreateWallet(query, user)
	case query.Data == "import_wallet":
		b.handleImportWallet(query, user)
	case query.Data == "view_wallets":
		b.handleViewWallets(query, user)
	case strings.HasPrefix(query.Data, "newpos_chain_"):
		b.handleNewPositionChainSelect(query, user)
	case strings.HasPrefix(query.Data, "newpos_wallet_"):
		b.handleNewPositionWalletSelect(query, user)
	case strings.HasPrefix(query.Data, "set_wallet_"):
		b.handleSetDefaultWallet(query, user)
	case strings.HasPrefix(query.Data, "delete_wallet_"):
		b.handleDeleteWallet(query, user)
	case strings.HasPrefix(query.Data, "confirm_delete_"):
		b.handleConfirmDeleteWallet(query, user)
	case query.Data == "back_to_wallets":
		b.handleViewWallets(query, user)
	case query.Data == "wallet_swap_to_usdt":
		b.handleWalletSwapToUSDTPrompt(query, user)
	case strings.HasPrefix(query.Data, "wallet_swap_chain_"):
		b.handleWalletSwapChainSelect(query, user)
	case strings.HasPrefix(query.Data, "wallet_swap_to_usdt_confirm_"):
		b.handleWalletSwapToUSDTConfirm(query, user)
	case query.Data == "wallet_swap_to_usdt_confirm":
		b.handleWalletSwapToUSDTConfirm(query, user)
	case query.Data == "wallet_swap_to_usdt_cancel":
		b.handleWalletSwapToUSDTCancel(query, user)
	case query.Data == "confirm_position":
		b.handleConfirmPosition(query, user)
	case query.Data == "cancel_position":
		b.handleCancelPosition(query, user)
	case query.Data == "back_to_input":
		b.handleBackToInput(query, user)
	case strings.HasPrefix(query.Data, "entry_swap_allow_"):
		b.handleEntrySwapAllow(query, user)
	case strings.HasPrefix(query.Data, "entry_swap_cancel_"):
		b.handleEntrySwapCancel(query, user)
	case query.Data == "config_rebalance_timeout":
		b.handleConfigRebalanceTimeout(query, user)
	case query.Data == "config_stop_loss_toggle":
		b.handleConfigStopLossToggle(query, user)
	case query.Data == "config_stop_loss_delay":
		b.handleConfigStopLossDelay(query, user)
	case query.Data == "config_slippage":
		b.handleConfigSlippage(query, user)
	case query.Data == "config_reinvest_toggle":
		b.handleConfigReinvestToggle(query, user)
	case query.Data == "config_residual_tolerance":
		b.handleConfigResidualTolerance(query, user)
	case query.Data == "config_zap_loss_tolerance":
		b.handleConfigZapLossTolerance(query, user)
	case query.Data == "config_bark_toggle":
		b.handleConfigBarkToggle(query, user)
	case query.Data == "config_bark_key":
		b.handleConfigBarkKey(query, user)
	case query.Data == "config_bark_server":
		b.handleConfigBarkServer(query, user)
	case query.Data == "config_bark_group":
		b.handleConfigBarkGroup(query, user)
	case query.Data == "config_extra_notifications_toggle":
		b.handleConfigExtraNotificationsToggle(query, user)
	case query.Data == "config_filter_chinese_toggle":
		b.handleConfigFilterChineseToggle(query, user)
	case query.Data == "config_multi_chain_toggle":
		b.handleConfigMultiChainToggle(query, user)
	case query.Data == "config_multi_wallet_toggle":
		b.handleConfigMultiWalletToggle(query, user)
	case query.Data == "config_default_chain":
		b.handleConfigDefaultChain(query, user)
	case strings.HasPrefix(query.Data, "config_default_chain_set_") || query.Data == "config_default_chain_cancel":
		b.handleConfigDefaultChainSelect(query, user)
	case query.Data == "view_config":
		b.handleViewConfig(query, user)
	case strings.HasPrefix(query.Data, "task_view_"):
		b.handleTaskView(query, user)
	case strings.HasPrefix(query.Data, "task_stop_refresh_"):
		b.handleTaskStopRefresh(query, user)
	case strings.HasPrefix(query.Data, "task_stop_"):
		b.handleTaskStop(query, user)
	case strings.HasPrefix(query.Data, "task_delete_"):
		b.handleTaskDelete(query, user)
	case strings.HasPrefix(query.Data, "task_confirm_delete_"):
		b.handleTaskConfirmDelete(query, user)
	case strings.HasPrefix(query.Data, "task_cancel_delete_"):
		b.handleTaskCancelDelete(query, user)
	case strings.HasPrefix(query.Data, "task_toggle_reinvest_"):
		b.handleTaskToggleReinvest(query, user)
	case strings.HasPrefix(query.Data, "task_toggle_pause_"):
		b.handleTaskTogglePause(query, user)
	case strings.HasPrefix(query.Data, "task_toggle_stoploss_"):
		b.handleTaskToggleStopLoss(query, user)
	case strings.HasPrefix(query.Data, "task_set_slippage_"):
		b.handleTaskSetSlippage(query, user)
	case strings.HasPrefix(query.Data, "task_set_range_"):
		b.handleTaskSetRange(query, user)
	case strings.HasPrefix(query.Data, "task_set_rebalance_"):
		b.handleTaskSetRebalanceTimeout(query, user)
	case strings.HasPrefix(query.Data, "task_swap_dust_"):
		b.handleTaskSwapDust(query, user)
	case strings.HasPrefix(query.Data, "task_set_stoploss_delay_"):
		b.handleTaskSetStopLossDelay(query, user)
	case strings.HasPrefix(query.Data, "task_set_residual_"):
		b.handleTaskSetResidualTolerance(query, user)
	case query.Data == "view_profit":
		b.handleViewProfit(query, user)
	default:
		callback := tgbotapi.NewCallback(query.ID, "")
		b.api.Send(callback)
	}
}

// Stop stops the bot.
func (b *Bot) Stop() {
	b.strategyService.Stop()
	if b.snapshotService != nil {
		b.snapshotService.Stop()
	}
	if b.assetService != nil {
		b.assetService.Stop()
	}
	b.api.StopReceivingUpdates()
}
