package bot

import (
	"TgLpBot/config"
	"TgLpBot/models"
	"TgLpBot/services"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot represents the Telegram bot
type Bot struct {
	api              *tgbotapi.BotAPI
	accessService    *services.AccessService
	userService      *services.UserService
	walletService    *services.WalletService
	liquidityService *services.LiquidityService
	okxService       *services.OKXDexService
	poolService      *services.PoolService
	strategyService  *services.StrategyService
	autoLPService    *services.AutoLPService
	smartLPMonitor   *services.SmartLPMonitor
	smartLPService   *services.SmartLPService
	autoLPCfgService *services.AutoLPUserConfigService
	configService    *services.GlobalConfigService
	taskService      *services.StrategyTaskService
	snapshotService  *services.BalanceSnapshotService
	pnlService       *services.PnLService
}

// NewBot creates a new bot instance
func NewBot(ch *services.ClickHouseService) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(config.AppConfig.TelegramBotToken)
	if err != nil {
		return nil, err
	}

	api.Debug = false
	log.Printf("Authorized on account %s", api.Self.UserName)

	bot := &Bot{
		api:              api,
		accessService:    services.NewAccessService(),
		userService:      services.NewUserService(),
		walletService:    services.NewWalletService(),
		liquidityService: services.NewLiquidityService(),
		okxService:       services.NewOKXDexService(),
		poolService:      services.NewPoolService(),
		strategyService:  services.NewStrategyService(),
		autoLPService:    services.NewAutoLPService(ch),
		smartLPMonitor:   services.NewSmartLPMonitor(ch),
		smartLPService:   services.NewSmartLPService(ch),
		autoLPCfgService: services.NewAutoLPUserConfigService(),
		configService:    services.NewGlobalConfigService(),
		taskService:      services.NewStrategyTaskService(),
		snapshotService:  services.NewBalanceSnapshotService(),
		pnlService:       services.NewPnLService(),
	}

	// Set Strategy Notifier
	bot.strategyService.SetNotifier(func(userID uint, message string) {
		user, err := bot.userService.GetUserByID(userID)
		if err == nil {
			bot.sendMessage(user.TelegramID, message)
		} else {
			log.Printf("Failed to notify user %d: %v", userID, err)
		}
	})

	// Set AutoLP Notifier (reuse the same user->telegram mapping)
	if bot.autoLPService != nil {
		bot.autoLPService.SetNotifier(func(userID uint, message string) {
			user, err := bot.userService.GetUserByID(userID)
			if err == nil {
				bot.sendMessage(user.TelegramID, message)
			} else {
				log.Printf("Failed to notify user %d: %v", userID, err)
			}
		})
	}

	// Set bot commands
	if err := bot.setCommands(); err != nil {
		log.Printf("Warning: Failed to set bot commands: %v", err)
	}
	if err := bot.setMenuButton(); err != nil {
		log.Printf("Warning: Failed to set bot menu button: %v", err)
	}

	return bot, nil
}

// setCommands sets the bot command menu
func (b *Bot) setCommands() error {
	if err := b.clearCommands(); err != nil {
		log.Printf("Warning: Failed to clear bot commands: %v", err)
	}

	commands := []tgbotapi.BotCommand{
		{
			Command:     "start",
			Description: "开始使用机器人",
		},
		{
			Command:     "auto",
			Description: "AutoLP 自动开仓配置",
		},
		{
			Command:     "positions",
			Description: "查看我的仓位",
		},
		{
			Command:     "config",
			Description: "全局配置",
		},
		{
			Command:     "profit",
			Description: "余额走势",
		},
		{
			Command:     "wallet",
			Description: "管理钱包",
		},
		{
			Command:     "transactions",
			Description: "查看交易历史",
		},
		{
			Command:     "smart_money",
			Description: "Smart Money 加LP榜",
		},
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
	// 只清除默认 scope 的命令
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

// Start starts the bot
func (b *Bot) Start() {
	// Start strategy service
	b.strategyService.Start()
	if b.autoLPService != nil {
		b.autoLPService.Start()
	}
	if b.smartLPMonitor != nil {
		b.smartLPMonitor.Start()
	}
	if b.snapshotService != nil {
		b.snapshotService.Start()
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

// handleMessage handles incoming messages
func (b *Bot) handleMessage(message *tgbotapi.Message) {
	// Get or create user
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

	// Handle commands
	if message.IsCommand() {
		b.handleCommand(message, user)
		return
	}

	// Handle text messages based on user state
	b.handleText(message, user)
}

// handleCommand handles bot commands
func (b *Bot) handleCommand(message *tgbotapi.Message, user *models.User) {
	switch message.Command() {
	case "start":
		b.handleStart(message, user)
	case "auto":
		b.handleAuto(message, user)
	case "help":
		b.handleHelp(message, user)
	case "wallet":
		b.handleWallet(message, user)
	case "newposition":
		b.handleNewPosition(message, user)
	case "positions":
		b.handlePositions(message, user)
	case "config":
		b.handleConfig(message, user)
	case "balance":
		b.handleBalance(message, user)
	case "transactions":
		b.handleTransactions(message, user)
	case "profit":
		b.handleProfit(message, user)
	case "cancel":
		b.handleCancel(message, user)
	case "admin":
		b.handleAdmin(message, user)
	case "smart_money":
		b.handleSmartMoney(message, user)
	default:
		b.sendMessage(message.Chat.ID, "未知命令。使用 /help 查看可用命令。")
	}
}

// sendMessage sends a text message
// sendMessage sends a text message
func (b *Bot) sendMessage(chatID int64, text string) (tgbotapi.Message, error) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	// msg.DisableWebPagePreview = true // 保持一致性，如果之前没有这里也不加，或者加上。原代码没有 disable preview
	if sentMsg, err := b.api.Send(msg); err != nil {
		// Fallback: if markdown entities are invalid, resend as plain text to avoid losing notifications.
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

// sendMessageWithKeyboard sends a message with inline keyboard
func (b *Bot) sendMessageWithKeyboard(chatID int64, text string, replyMarkup any) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = replyMarkup
	if _, err := b.api.Send(msg); err != nil {
		if strings.Contains(err.Error(), "can't parse entities") {
			msg.ParseMode = ""
			if _, err2 := b.api.Send(msg); err2 == nil {
				return
			} else {
				log.Printf("Error sending message (Markdown): %v; fallback plain text failed: %v", err, err2)
				return
			}
		}
		log.Printf("Error sending message: %v", err)
	}
}

func (b *Bot) editMessageText(chatID int64, messageID int, text string) error {
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.DisableWebPagePreview = true

	if _, err := b.api.Send(editMsg); err != nil {
		// Ignore no-op edits.
		if strings.Contains(err.Error(), "message is not modified") {
			return nil
		}
		// Fallback: if markdown entities are invalid, resend as plain text.
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

// handleCallbackQuery handles callback queries from inline keyboards
func (b *Bot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	// Get user
	user, err := b.userService.GetUserByTelegramID(query.From.ID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		return
	}

	// Handle different callback actions
	switch {
	// Admin callbacks
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
	// Wallet callbacks
	case query.Data == "create_wallet":
		b.handleCreateWallet(query, user)
	case query.Data == "import_wallet":
		b.handleImportWallet(query, user)
	case query.Data == "view_wallets":
		b.handleViewWallets(query, user)
	case strings.HasPrefix(query.Data, "set_wallet_"):
		b.handleSetDefaultWallet(query, user)
	case strings.HasPrefix(query.Data, "delete_wallet_"):
		b.handleDeleteWallet(query, user)
	case strings.HasPrefix(query.Data, "confirm_delete_"):
		b.handleConfirmDeleteWallet(query, user)
	case query.Data == "back_to_wallets":
		b.handleViewWallets(query, user)
	// Position confirmation callbacks
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
	// AutoLP config callbacks
	case query.Data == "auto_cfg_toggle":
		b.handleAutoConfigToggle(query, user)
	case query.Data == "auto_cfg_refresh":
		b.handleAutoConfigRefresh(query, user)
	case query.Data == "auto_cfg_set_total":
		b.handleAutoConfigSetTotal(query, user)
	case query.Data == "auto_cfg_set_max_tasks":
		b.handleAutoConfigSetMaxTasks(query, user)
	case query.Data == "auto_cfg_set_take_profit":
		b.handleAutoConfigSetTakeProfit(query, user)
	case query.Data == "auto_cfg_set_stop_loss":
		b.handleAutoConfigSetStopLoss(query, user)
	case query.Data == "auto_cfg_cancel_input":
		b.handleAutoCancelInput(query, user)
	case query.Data == "auto_view_strategy":
		b.handleAutoViewStrategy(query, user)
	case query.Data == "auto_view_config":
		b.handleAutoViewConfig(query, user)
	// Global config callbacks
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
	case query.Data == "view_config":
		b.handleViewConfig(query, user)
	// Task management callbacks
	case strings.HasPrefix(query.Data, "task_view_"):
		b.handleTaskView(query, user)
	case strings.HasPrefix(query.Data, "task_stop_refresh_"):
		b.handleTaskStopRefresh(query, user)
	case strings.HasPrefix(query.Data, "task_stop_"):
		b.handleTaskStop(query, user)
	case strings.HasPrefix(query.Data, "task_toggle_reinvest_"):
		b.handleTaskToggleReinvest(query, user)
	case strings.HasPrefix(query.Data, "task_toggle_stoploss_"):
		b.handleTaskToggleStopLoss(query, user)
	case strings.HasPrefix(query.Data, "task_set_slippage_"):
		b.handleTaskSetSlippage(query, user)
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
		// Answer callback to remove loading state
		callback := tgbotapi.NewCallback(query.ID, "")
		b.api.Send(callback)
	}
}

// Stop stops the bot
func (b *Bot) Stop() {
	b.strategyService.Stop()
	if b.autoLPService != nil {
		b.autoLPService.Stop()
	}
	if b.smartLPMonitor != nil {
		b.smartLPMonitor.Stop()
	}
	if b.snapshotService != nil {
		b.snapshotService.Stop()
	}
	b.api.StopReceivingUpdates()
}
