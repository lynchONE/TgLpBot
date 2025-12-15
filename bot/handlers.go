package bot

import (
	"TgLpBot/database"
	"TgLpBot/models"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleStart handles the /start command
func (b *Bot) handleStart(message *tgbotapi.Message, user *models.User) {
	text := fmt.Sprintf(`👋 Welcome to *LP Bot*, %s!

I can help you manage liquidity on BSC (Binance Smart Chain).

*Features:*
• 💼 Manage your wallets
• 💰 Add liquidity with USDT
• 📤 Remove liquidity to USDT
• ⚙️ Configure LP parameters
• 📊 Track your transactions

Use /help to see all available commands.

⚠️ *Security Notice:*
Your private keys are encrypted and stored securely. Never share your private keys with anyone!`, user.FirstName)
	
	b.sendMessage(message.Chat.ID, text)
}

// handleHelp handles the /help command
func (b *Bot) handleHelp(message *tgbotapi.Message, user *models.User) {
	text := `📚 *Available Commands:*

*Wallet Management:*
/wallet - Manage your wallets
/balance - Check wallet balances

*Liquidity Operations:*
/addlp - Add liquidity with USDT
/removelp - Remove liquidity to USDT
/config - Configure LP parameters

*Information:*
/transactions - View transaction history
/help - Show this help message

*How to use:*
1. First, create or import a wallet using /wallet
2. Configure your LP pool using /config
3. Add liquidity using /addlp
4. Remove liquidity using /removelp

For support, contact @yoursupport`
	
	b.sendMessage(message.Chat.ID, text)
}

// handleWallet handles the /wallet command
func (b *Bot) handleWallet(message *tgbotapi.Message, user *models.User) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Create Wallet", "create_wallet"),
			tgbotapi.NewInlineKeyboardButtonData("📥 Import Wallet", "import_wallet"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👀 View Wallets", "view_wallets"),
		),
	)
	
	text := "💼 *Wallet Management*\n\nChoose an option:"
	b.sendMessageWithKeyboard(message.Chat.ID, text, keyboard)
}

// handleBalance handles the /balance command
func (b *Bot) handleBalance(message *tgbotapi.Message, user *models.User) {
	wallets, err := b.walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		b.sendMessage(message.Chat.ID, "You don't have any wallets yet. Use /wallet to create one.")
		return
	}
	
	text := "💰 *Your Wallet Balances:*\n\n"
	
	for _, wallet := range wallets {
		text += fmt.Sprintf("*%s*\n", wallet.Name)
		text += fmt.Sprintf("`%s`\n", wallet.Address)
		
		// Get BNB balance
		// TODO: Implement balance fetching
		text += "BNB: Loading...\n"
		text += "USDT: Loading...\n"
		text += "\n"
	}
	
	b.sendMessage(message.Chat.ID, text)
}

// handleAddLP handles the /addlp command
func (b *Bot) handleAddLP(message *tgbotapi.Message, user *models.User) {
	// Check if user has a wallet
	wallets, err := b.walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		b.sendMessage(message.Chat.ID, "You don't have any wallets yet. Use /wallet to create one first.")
		return
	}
	
	// Set user state to expect pool address
	database.SetUserSession(user.TelegramID, "state", "awaiting_pool_address_add", 30*time.Minute)
	
	text := `💰 *Add Liquidity*

Please send me the LP pool contract address.

Example: \`0x...\`

Send /cancel to cancel this operation.`
	
	b.sendMessage(message.Chat.ID, text)
}

// handleRemoveLP handles the /removelp command
func (b *Bot) handleRemoveLP(message *tgbotapi.Message, user *models.User) {
	// Check if user has a wallet
	wallets, err := b.walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		b.sendMessage(message.Chat.ID, "You don't have any wallets yet. Use /wallet to create one first.")
		return
	}
	
	// Set user state to expect pool address
	database.SetUserSession(user.TelegramID, "state", "awaiting_pool_address_remove", 30*time.Minute)
	
	text := `📤 *Remove Liquidity*

Please send me the LP pool contract address.

Example: \`0x...\`

Send /cancel to cancel this operation.`
	
	b.sendMessage(message.Chat.ID, text)
}

// handleConfig handles the /config command
func (b *Bot) handleConfig(message *tgbotapi.Message, user *models.User) {
	// Set user state to expect pool address
	database.SetUserSession(user.TelegramID, "state", "awaiting_pool_address_config", 30*time.Minute)
	
	text := `⚙️ *Configure LP Parameters*

Please send me the LP pool contract address you want to configure.

Example: \`0x...\`

Send /cancel to cancel this operation.`
	
	b.sendMessage(message.Chat.ID, text)
}

// handleTransactions handles the /transactions command
func (b *Bot) handleTransactions(message *tgbotapi.Message, user *models.User) {
	var transactions []models.Transaction
	err := database.DB.Where("user_id = ?", user.ID).
		Order("created_at DESC").
		Limit(10).
		Find(&transactions).Error
	
	if err != nil {
		b.sendMessage(message.Chat.ID, "Error fetching transactions.")
		return
	}
	
	if len(transactions) == 0 {
		b.sendMessage(message.Chat.ID, "You don't have any transactions yet.")
		return
	}
	
	text := "📊 *Recent Transactions:*\n\n"
	
	for _, tx := range transactions {
		statusEmoji := "⏳"
		if tx.Status == models.TxStatusConfirmed {
			statusEmoji = "✅"
		} else if tx.Status == models.TxStatusFailed {
			statusEmoji = "❌"
		}
		
		typeText := string(tx.Type)
		text += fmt.Sprintf("%s *%s*\n", statusEmoji, typeText)
		text += fmt.Sprintf("Hash: `%s`\n", tx.TxHash[:10]+"..."+tx.TxHash[len(tx.TxHash)-8:])
		text += fmt.Sprintf("Time: %s\n\n", tx.CreatedAt.Format("2006-01-02 15:04"))
	}
	
	b.sendMessage(message.Chat.ID, text)
}

// handleText handles text messages based on user state
func (b *Bot) handleText(message *tgbotapi.Message, user *models.User) {
	// Check if user wants to cancel
	if message.Text == "/cancel" {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(message.Chat.ID, "Operation cancelled.")
		return
	}
	
	// Get user state
	state, err := database.GetUserSession(user.TelegramID, "state")
	if err != nil {
		b.sendMessage(message.Chat.ID, "Please use a command to start. Type /help for available commands.")
		return
	}
	
	switch state {
	case "awaiting_private_key":
		b.handlePrivateKeyInput(message, user)
	case "awaiting_wallet_name":
		b.handleWalletNameInput(message, user)
	case "awaiting_pool_address_add":
		b.handlePoolAddressForAdd(message, user)
	case "awaiting_pool_address_remove":
		b.handlePoolAddressForRemove(message, user)
	case "awaiting_pool_address_config":
		b.handlePoolAddressForConfig(message, user)
	case "awaiting_usdt_amount":
		b.handleUSDTAmountInput(message, user)
	case "awaiting_lp_amount":
		b.handleLPAmountInput(message, user)
	case "awaiting_slippage":
		b.handleSlippageInput(message, user)
	default:
		b.sendMessage(message.Chat.ID, "Please use a command to start. Type /help for available commands.")
	}
}

// handleCreateWallet handles wallet creation callback
func (b *Bot) handleCreateWallet(query *tgbotapi.CallbackQuery, user *models.User) {
	// Answer callback
	callback := tgbotapi.NewCallback(query.ID, "Creating wallet...")
	b.api.Send(callback)
	
	// Create wallet
	wallet, err := b.walletService.CreateWallet(user.ID, "Wallet "+strconv.Itoa(int(time.Now().Unix())))
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, "Error creating wallet. Please try again.")
		return
	}
	
	text := fmt.Sprintf(`✅ *Wallet Created Successfully!*

*Address:* \`%s\`

*Name:* %s

⚠️ *Important:* Please backup your wallet! You can export the private key later if needed.

Use /balance to check your wallet balance.`, wallet.Address, wallet.Name)
	
	b.sendMessage(query.Message.Chat.ID, text)
}

// handleImportWallet handles wallet import callback
func (b *Bot) handleImportWallet(query *tgbotapi.CallbackQuery, user *models.User) {
	// Answer callback
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)
	
	// Set user state
	database.SetUserSession(user.TelegramID, "state", "awaiting_private_key", 10*time.Minute)
	
	text := `📥 *Import Wallet*

Please send me your private key (without 0x prefix).

⚠️ *Security Notice:*
• Your private key will be encrypted before storage
• Delete your message after sending
• Never share your private key with anyone else

Send /cancel to cancel this operation.`
	
	b.sendMessage(query.Message.Chat.ID, text)
}

// handleViewWallets handles view wallets callback
func (b *Bot) handleViewWallets(query *tgbotapi.CallbackQuery, user *models.User) {
	// Answer callback
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)
	
	wallets, err := b.walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		b.sendMessage(query.Message.Chat.ID, "You don't have any wallets yet.")
		return
	}
	
	text := "💼 *Your Wallets:*\n\n"
	
	for i, wallet := range wallets {
		defaultMark := ""
		if wallet.IsDefault {
			defaultMark = " ⭐"
		}
		text += fmt.Sprintf("%d. *%s*%s\n", i+1, wallet.Name, defaultMark)
		text += fmt.Sprintf("   `%s`\n\n", wallet.Address)
	}
	
	b.sendMessage(query.Message.Chat.ID, text)
}

// handleSetDefaultWallet handles set default wallet callback
func (b *Bot) handleSetDefaultWallet(query *tgbotapi.CallbackQuery, user *models.User) {
	// Parse wallet ID from callback data
	parts := strings.Split(query.Data, "_")
	if len(parts) < 3 {
		return
	}
	
	walletID, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		return
	}
	
	err = b.walletService.SetDefaultWallet(user.ID, uint(walletID))
	if err != nil {
		callback := tgbotapi.NewCallback(query.ID, "Error setting default wallet")
		b.api.Send(callback)
		return
	}
	
	callback := tgbotapi.NewCallback(query.ID, "✅ Default wallet updated")
	b.api.Send(callback)
}

// handleDeleteWallet handles delete wallet callback
func (b *Bot) handleDeleteWallet(query *tgbotapi.CallbackQuery, user *models.User) {
	// Parse wallet ID from callback data
	parts := strings.Split(query.Data, "_")
	if len(parts) < 3 {
		return
	}
	
	walletID, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		return
	}
	
	err = b.walletService.DeleteWallet(user.ID, uint(walletID))
	if err != nil {
		callback := tgbotapi.NewCallback(query.ID, "Error deleting wallet")
		b.api.Send(callback)
		return
	}
	
	callback := tgbotapi.NewCallback(query.ID, "✅ Wallet deleted")
	b.api.Send(callback)
}

