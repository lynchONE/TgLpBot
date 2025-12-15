package bot

import (
	"TgLpBot/config"
	"TgLpBot/services"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot represents the Telegram bot
type Bot struct {
	api              *tgbotapi.BotAPI
	userService      *services.UserService
	walletService    *services.WalletService
	liquidityService *services.LiquidityService
	okxService       *services.OKXDexService
}

// NewBot creates a new bot instance
func NewBot() (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(config.AppConfig.TelegramBotToken)
	if err != nil {
		return nil, err
	}
	
	api.Debug = false
	log.Printf("Authorized on account %s", api.Self.UserName)
	
	return &Bot{
		api:              api,
		userService:      services.NewUserService(),
		walletService:    services.NewWalletService(),
		liquidityService: services.NewLiquidityService(),
		okxService:       services.NewOKXDexService(),
	}, nil
}

// Start starts the bot
func (b *Bot) Start() {
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
		b.sendMessage(message.Chat.ID, "Error processing your request. Please try again.")
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
	case "help":
		b.handleHelp(message, user)
	case "wallet":
		b.handleWallet(message, user)
	case "addlp":
		b.handleAddLP(message, user)
	case "removelp":
		b.handleRemoveLP(message, user)
	case "config":
		b.handleConfig(message, user)
	case "balance":
		b.handleBalance(message, user)
	case "transactions":
		b.handleTransactions(message, user)
	default:
		b.sendMessage(message.Chat.ID, "Unknown command. Use /help to see available commands.")
	}
}

// sendMessage sends a text message
func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Error sending message: %v", err)
	}
}

// sendMessageWithKeyboard sends a message with inline keyboard
func (b *Bot) sendMessageWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Error sending message: %v", err)
	}
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
	case query.Data == "create_wallet":
		b.handleCreateWallet(query, user)
	case query.Data == "import_wallet":
		b.handleImportWallet(query, user)
	case query.Data == "view_wallets":
		b.handleViewWallets(query, user)
	case query.Data[:10] == "set_wallet":
		b.handleSetDefaultWallet(query, user)
	case query.Data[:13] == "delete_wallet":
		b.handleDeleteWallet(query, user)
	default:
		// Answer callback to remove loading state
		callback := tgbotapi.NewCallback(query.ID, "")
		b.api.Send(callback)
	}
}

// Stop stops the bot
func (b *Bot) Stop() {
	b.api.StopReceivingUpdates()
}

