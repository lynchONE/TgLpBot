package bot

import (
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

// handlePrivateKeyInput handles private key input for wallet import
func (b *Bot) handlePrivateKeyInput(message *tgbotapi.Message, user *models.User) {
	privateKey := strings.TrimSpace(message.Text)
	
	// Delete user's message for security
	deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, message.MessageID)
	b.api.Send(deleteMsg)
	
	// Validate private key format
	if len(privateKey) != 64 {
		b.sendMessage(message.Chat.ID, "Invalid private key format. Please send a 64-character hex string.")
		return
	}
	
	// Store private key temporarily
	database.SetUserSession(user.TelegramID, "temp_private_key", privateKey, 10*time.Minute)
	database.SetUserSession(user.TelegramID, "state", "awaiting_wallet_name", 10*time.Minute)
	
	text := "Please enter a name for this wallet:"
	b.sendMessage(message.Chat.ID, text)
}

// handleWalletNameInput handles wallet name input
func (b *Bot) handleWalletNameInput(message *tgbotapi.Message, user *models.User) {
	walletName := strings.TrimSpace(message.Text)
	
	if walletName == "" {
		b.sendMessage(message.Chat.ID, "Wallet name cannot be empty. Please try again.")
		return
	}
	
	// Get stored private key
	privateKey, err := database.GetUserSession(user.TelegramID, "temp_private_key")
	if err != nil {
		b.sendMessage(message.Chat.ID, "Session expired. Please start over with /wallet")
		database.ClearUserSession(user.TelegramID)
		return
	}
	
	// Import wallet
	wallet, err := b.walletService.ImportWallet(user.ID, privateKey, walletName)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("Error importing wallet: %v", err))
		database.ClearUserSession(user.TelegramID)
		return
	}
	
	// Clear session
	database.ClearUserSession(user.TelegramID)
	
	text := fmt.Sprintf(`✅ *Wallet Imported Successfully!*

*Address:* \`%s\`
*Name:* %s

Use /balance to check your wallet balance.`, wallet.Address, wallet.Name)
	
	b.sendMessage(message.Chat.ID, text)
}

// handlePoolAddressForAdd handles pool address input for adding liquidity
func (b *Bot) handlePoolAddressForAdd(message *tgbotapi.Message, user *models.User) {
	poolAddress := strings.TrimSpace(message.Text)
	
	// Validate address
	if !common.IsHexAddress(poolAddress) {
		b.sendMessage(message.Chat.ID, "Invalid pool address. Please send a valid BSC address.")
		return
	}
	
	// Store pool address
	database.SetUserSession(user.TelegramID, "pool_address", poolAddress, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "state", "awaiting_usdt_amount", 30*time.Minute)
	
	text := `How much USDT do you want to use for adding liquidity?

Please enter the amount (e.g., 100):

Send /cancel to cancel this operation.`
	
	b.sendMessage(message.Chat.ID, text)
}

// handlePoolAddressForRemove handles pool address input for removing liquidity
func (b *Bot) handlePoolAddressForRemove(message *tgbotapi.Message, user *models.User) {
	poolAddress := strings.TrimSpace(message.Text)
	
	// Validate address
	if !common.IsHexAddress(poolAddress) {
		b.sendMessage(message.Chat.ID, "Invalid pool address. Please send a valid BSC address.")
		return
	}
	
	// Store pool address
	database.SetUserSession(user.TelegramID, "pool_address", poolAddress, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "state", "awaiting_lp_amount", 30*time.Minute)
	
	text := `How much LP tokens do you want to remove?

Please enter the amount (e.g., 0.5):

Send /cancel to cancel this operation.`
	
	b.sendMessage(message.Chat.ID, text)
}

// handlePoolAddressForConfig handles pool address input for configuration
func (b *Bot) handlePoolAddressForConfig(message *tgbotapi.Message, user *models.User) {
	poolAddress := strings.TrimSpace(message.Text)
	
	// Validate address
	if !common.IsHexAddress(poolAddress) {
		b.sendMessage(message.Chat.ID, "Invalid pool address. Please send a valid BSC address.")
		return
	}
	
	// TODO: Fetch pool info and create/update config
	
	text := fmt.Sprintf(`⚙️ *LP Configuration*

Pool: \`%s\`

Configuration saved! You can now use /addlp and /removelp with this pool.`, poolAddress)
	
	database.ClearUserSession(user.TelegramID)
	b.sendMessage(message.Chat.ID, text)
}

// handleUSDTAmountInput handles USDT amount input
func (b *Bot) handleUSDTAmountInput(message *tgbotapi.Message, user *models.User) {
	amountStr := strings.TrimSpace(message.Text)
	
	// Parse amount
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		b.sendMessage(message.Chat.ID, "Invalid amount. Please enter a positive number.")
		return
	}
	
	// Store amount
	database.SetUserSession(user.TelegramID, "usdt_amount", amountStr, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "state", "awaiting_slippage", 30*time.Minute)
	
	text := `What slippage tolerance do you want to use?

Please enter the percentage (e.g., 0.5 for 0.5%):

Recommended: 0.5 - 1.0

Send /cancel to cancel this operation.`
	
	b.sendMessage(message.Chat.ID, text)
}

// handleLPAmountInput handles LP amount input
func (b *Bot) handleLPAmountInput(message *tgbotapi.Message, user *models.User) {
	amountStr := strings.TrimSpace(message.Text)
	
	// Parse amount
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		b.sendMessage(message.Chat.ID, "Invalid amount. Please enter a positive number.")
		return
	}
	
	// Store amount
	database.SetUserSession(user.TelegramID, "lp_amount", amountStr, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "state", "awaiting_slippage", 30*time.Minute)
	
	text := `What slippage tolerance do you want to use?

Please enter the percentage (e.g., 0.5 for 0.5%):

Recommended: 0.5 - 1.0

Send /cancel to cancel this operation.`
	
	b.sendMessage(message.Chat.ID, text)
}

// handleSlippageInput handles slippage input
func (b *Bot) handleSlippageInput(message *tgbotapi.Message, user *models.User) {
	slippageStr := strings.TrimSpace(message.Text)
	
	// Parse slippage
	slippage, err := strconv.ParseFloat(slippageStr, 64)
	if err != nil || slippage <= 0 || slippage > 50 {
		b.sendMessage(message.Chat.ID, "Invalid slippage. Please enter a number between 0 and 50.")
		return
	}
	
	// Get stored data
	poolAddress, _ := database.GetUserSession(user.TelegramID, "pool_address")
	
	// Check if this is for adding or removing liquidity
	usdtAmountStr, errAdd := database.GetUserSession(user.TelegramID, "usdt_amount")
	lpAmountStr, errRemove := database.GetUserSession(user.TelegramID, "lp_amount")
	
	if errAdd == nil && usdtAmountStr != "" {
		// Adding liquidity
		b.executeAddLiquidity(message.Chat.ID, user, poolAddress, usdtAmountStr, slippage)
	} else if errRemove == nil && lpAmountStr != "" {
		// Removing liquidity
		b.executeRemoveLiquidity(message.Chat.ID, user, poolAddress, lpAmountStr, slippage)
	} else {
		b.sendMessage(message.Chat.ID, "Session expired. Please start over.")
	}
	
	// Clear session
	database.ClearUserSession(user.TelegramID)
}

// executeAddLiquidity executes the add liquidity operation
func (b *Bot) executeAddLiquidity(chatID int64, user *models.User, poolAddress, amountStr string, slippage float64) {
	b.sendMessage(chatID, "⏳ Processing your request...")
	
	// Parse amount to wei (USDT has 18 decimals on BSC)
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		b.sendMessage(chatID, "Error parsing amount.")
		return
	}
	
	// Convert to wei (assuming 18 decimals)
	amountWei := new(big.Float).Mul(big.NewFloat(amount), big.NewFloat(1e18))
	amountBigInt, _ := amountWei.Int(nil)
	
	// Execute add liquidity
	txHash, err := b.liquidityService.AddLiquidityWithUSDT(user.ID, poolAddress, amountBigInt, slippage)
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ Error adding liquidity: %v", err))
		return
	}
	
	text := fmt.Sprintf(`✅ *Liquidity Added!*

Transaction Hash: \`%s\`

You can check the transaction status on BSCScan:
https://bscscan.com/tx/%s

Use /transactions to view your transaction history.`, txHash, txHash)
	
	b.sendMessage(chatID, text)
}

// executeRemoveLiquidity executes the remove liquidity operation
func (b *Bot) executeRemoveLiquidity(chatID int64, user *models.User, poolAddress, amountStr string, slippage float64) {
	b.sendMessage(chatID, "⏳ Processing your request...")
	
	// Parse amount to wei
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		b.sendMessage(chatID, "Error parsing amount.")
		return
	}
	
	// Convert to wei (assuming 18 decimals)
	amountWei := new(big.Float).Mul(big.NewFloat(amount), big.NewFloat(1e18))
	amountBigInt, _ := amountWei.Int(nil)
	
	// Execute remove liquidity
	txHash, err := b.liquidityService.RemoveLiquidityToUSDT(user.ID, poolAddress, amountBigInt, slippage)
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ Error removing liquidity: %v", err))
		return
	}
	
	text := fmt.Sprintf(`✅ *Liquidity Removed!*

Transaction Hash: \`%s\`

You can check the transaction status on BSCScan:
https://bscscan.com/tx/%s

Use /transactions to view your transaction history.`, txHash, txHash)
	
	b.sendMessage(chatID, text)
}

