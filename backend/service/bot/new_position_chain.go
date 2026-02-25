package bot

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func newPositionChainKeyboard(chains []string) any {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, 4)
	cur := make([]tgbotapi.InlineKeyboardButton, 0, 2)

	for _, c := range chains {
		ch := config.NormalizeChain(c)
		if ch == "" {
			continue
		}
		cur = append(cur, tgbotapi.NewInlineKeyboardButtonData(chainLabel(ch), "newpos_chain_"+ch))
		if len(cur) >= 2 {
			rows = append(rows, cur)
			cur = make([]tgbotapi.InlineKeyboardButton, 0, 2)
		}
	}
	if len(cur) > 0 {
		rows = append(rows, cur)
	}
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("取消", "newpos_chain_cancel"),
	})

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func (b *Bot) handleNewPositionChainSelect(query *tgbotapi.CallbackQuery, user *models.User) {
	_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if query == nil || query.Message == nil || query.Message.Chat == nil {
		return
	}

	data := strings.TrimSpace(query.Data)
	if data == "newpos_chain_cancel" {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(query.Message.Chat.ID, "已取消。")
		return
	}

	chain := config.NormalizeChain(strings.TrimPrefix(data, "newpos_chain_"))
	if chain == "" {
		b.sendMessage(query.Message.Chat.ID, "无效的链。")
		return
	}

	enabled := false
	for _, c := range enabledChains() {
		if config.NormalizeChain(c) == chain {
			enabled = true
			break
		}
	}
	if !enabled {
		b.sendMessage(query.Message.Chat.ID, "当前未启用该链，请检查 CHAINS 配置。")
		return
	}

	_ = database.SetUserSession(user.TelegramID, sessionNewPositionChain, chain, 30*time.Minute)
	_ = database.SetUserSession(user.TelegramID, "state", "awaiting_pool_address", 30*time.Minute)

	// If user pasted pool id/address before selecting chain, continue automatically.
	if pending, err := database.GetUserSession(user.TelegramID, sessionPendingPoolInput); err == nil && strings.TrimSpace(pending) != "" {
		_ = database.DeleteUserSession(user.TelegramID, sessionPendingPoolInput)
		msg := &tgbotapi.Message{
			Text: pending,
			Chat: query.Message.Chat,
		}
		b.handlePoolAddress(msg, user)
		return
	}

	text := fmt.Sprintf(
		"已选择链：*%s*\n\n请发送 V3 池子地址（0x...）或 V4 PoolId（0x...，32字节）。\n\n发送 /cancel 取消此操作。",
		chainLabel(chain),
	)
	b.sendMessage(query.Message.Chat.ID, text)
}

func (b *Bot) handleNewPositionChainText(message *tgbotapi.Message, user *models.User) {
	if message == nil || message.Chat == nil {
		return
	}

	chain := config.NormalizeChain(message.Text)
	if chain == "" {
		b.sendMessage(message.Chat.ID, "请输入正确的链（如 bsc / base），或直接点击按钮。")
		return
	}

	enabled := false
	for _, c := range enabledChains() {
		if config.NormalizeChain(c) == chain {
			enabled = true
			break
		}
	}
	if !enabled {
		b.sendMessage(message.Chat.ID, "当前未启用该链，请检查 CHAINS 配置。")
		return
	}

	_ = database.SetUserSession(user.TelegramID, sessionNewPositionChain, chain, 30*time.Minute)
	_ = database.SetUserSession(user.TelegramID, "state", "awaiting_pool_address", 30*time.Minute)

	// If user pasted pool id/address before selecting chain, continue automatically.
	if pending, err := database.GetUserSession(user.TelegramID, sessionPendingPoolInput); err == nil && strings.TrimSpace(pending) != "" {
		_ = database.DeleteUserSession(user.TelegramID, sessionPendingPoolInput)
		msg := &tgbotapi.Message{
			Text: pending,
			Chat: message.Chat,
		}
		b.handlePoolAddress(msg, user)
		return
	}

	text := fmt.Sprintf(
		"已选择链：*%s*\n\n请发送 V3 池子地址（0x...）或 V4 PoolId（0x...，32字节）。\n\n发送 /cancel 取消此操作。",
		chainLabel(chain),
	)
	b.sendMessage(message.Chat.ID, text)
}
