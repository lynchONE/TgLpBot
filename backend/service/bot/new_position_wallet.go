package bot

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func shortWalletAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if len(addr) <= 18 {
		return addr
	}
	return addr[:10] + "..." + addr[len(addr)-8:]
}

func newPositionWalletKeyboard(wallets []models.Wallet, chain string) any {
	_ = chain
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, 6)
	cur := make([]tgbotapi.InlineKeyboardButton, 0, 2)

	for i, w := range wallets {
		name := strings.TrimSpace(w.Name)
		if name == "" {
			name = fmt.Sprintf("Wallet #%d", i+1)
		}
		label := fmt.Sprintf("%d. %s", i+1, name)
		cur = append(cur, tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("newpos_wallet_%d", w.ID)))
		if len(cur) >= 2 {
			rows = append(rows, cur)
			cur = make([]tgbotapi.InlineKeyboardButton, 0, 2)
		}
	}
	if len(cur) > 0 {
		rows = append(rows, cur)
	}
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("取消", "newpos_wallet_cancel"),
	})
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func (b *Bot) promptNewPositionWalletSelect(chatID int64, user *models.User, chain string) {
	if b == nil || user == nil {
		return
	}
	chain = config.NormalizeChain(chain)

	wallets, err := b.walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		b.sendMessage(chatID, "⚠️ 您还没有钱包，请先用 /wallet 导入。")
		return
	}

	_ = database.SetUserSession(user.TelegramID, sessionNewPositionChain, chain, 30*time.Minute)
	_ = database.SetUserSession(user.TelegramID, "state", sessionNewPositionWalletState, 30*time.Minute)

	lines := make([]string, 0, len(wallets)*5)
	lines = append(lines, fmt.Sprintf("💳 *创建新仓位*\n\n已选择链：*%s*\n\n请选择使用的钱包：\n", chainLabel(chain)))

	for i := range wallets {
		w := wallets[i]
		name := strings.TrimSpace(w.Name)
		if name == "" {
			name = fmt.Sprintf("Wallet #%d", i+1)
		}
		name = escapeTelegramMarkdown(name)
		defaultMark := ""
		if w.IsDefault {
			defaultMark = " ⭐"
		}

		nativeSym, nativeBal, stableSym, stableBal := b.getWalletBalancesForChain(chain, w.Address)
		lines = append(lines,
			fmt.Sprintf("*%d. %s*%s", i+1, name, defaultMark),
			fmt.Sprintf("`%s`", strings.TrimSpace(w.Address)),
			fmt.Sprintf("🪙 %s: %s", nativeSym, nativeBal),
			fmt.Sprintf("💵 %s: %s", stableSym, stableBal),
			"",
		)
	}
	lines = append(lines, "点击下方按钮选择：")
	text := strings.Join(lines, "\n")
	b.sendMessageWithKeyboard(chatID, text, newPositionWalletKeyboard(wallets, chain))
}

func (b *Bot) handleNewPositionWalletSelect(query *tgbotapi.CallbackQuery, user *models.User) {
	_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if query == nil || query.Message == nil || query.Message.Chat == nil || user == nil {
		return
	}

	data := strings.TrimSpace(query.Data)
	if data == "newpos_wallet_cancel" {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(query.Message.Chat.ID, "已取消。")
		return
	}

	idStr := strings.TrimPrefix(data, "newpos_wallet_")
	id64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id64 == 0 {
		b.sendMessage(query.Message.Chat.ID, "无效的钱包。")
		return
	}

	w, err := b.walletService.GetWalletByID(user.ID, uint(id64))
	if err != nil || w == nil {
		b.sendMessage(query.Message.Chat.ID, "钱包不存在或不属于你。")
		return
	}

	_ = database.SetUserSession(user.TelegramID, sessionNewPositionWalletID, fmt.Sprintf("%d", w.ID), 30*time.Minute)
	_ = database.SetUserSession(user.TelegramID, "state", "awaiting_pool_address", 30*time.Minute)

	// If user pasted pool id/address before selecting wallet, continue automatically.
	if pending, err := database.GetUserSession(user.TelegramID, sessionPendingPoolInput); err == nil && strings.TrimSpace(pending) != "" {
		_ = database.DeleteUserSession(user.TelegramID, sessionPendingPoolInput)
		msg := &tgbotapi.Message{
			Text: pending,
			Chat: query.Message.Chat,
		}
		b.handlePoolAddress(msg, user)
		return
	}

	chain, _ := database.GetUserSession(user.TelegramID, sessionNewPositionChain)
	chain = config.NormalizeChain(chain)
	name := strings.TrimSpace(w.Name)
	if name == "" {
		name = fmt.Sprintf("Wallet #%d", w.ID)
	}
	if chain != "" {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("已选择链：*%s*\n已选择钱包：*%s* (`%s`)\n\n请发送 V3 池子地址 (0x...) 或 V4 PoolId (0x...32 bytes)。\n\n发送 /cancel 取消。", chainLabel(chain), escapeTelegramMarkdown(name), shortWalletAddress(w.Address)))
		return
	}
	b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("已选择钱包：*%s* (`%s`)\n\n请发送 V3 池子地址 (0x...) 或 V4 PoolId (0x...32 bytes)。\n\n发送 /cancel 取消。", escapeTelegramMarkdown(name), shortWalletAddress(w.Address)))
}

func (b *Bot) handleNewPositionWalletText(message *tgbotapi.Message, user *models.User) {
	if message == nil || message.Chat == nil {
		return
	}
	b.sendMessage(message.Chat.ID, "请点击按钮选择要使用的钱包。")
}
