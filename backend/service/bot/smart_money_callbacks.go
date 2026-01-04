package bot

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleSmartMoneyCallback(query *tgbotapi.CallbackQuery, user *models.User) {
	// Always answer callback to remove the loading state.
	_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, ""))

	if query == nil || query.Message == nil || user == nil {
		return
	}

	chatID := query.Message.Chat.ID
	messageID := query.Message.MessageID

	raw, err := database.GetUserSession(user.TelegramID, smartMoneySessionKey(chatID, messageID))
	if err != nil || strings.TrimSpace(raw) == "" {
		_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, "已过期，请重新发送 /smart_money"))
		return
	}

	var smCtx smartMoneyMessageCtx
	if err := json.Unmarshal([]byte(raw), &smCtx); err != nil {
		_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, "数据解析失败，请重新发送 /smart_money"))
		return
	}

	expandedPoolIdx := -1
	page := 0

	parts := strings.Split(strings.TrimSpace(query.Data), "_")
	if len(parts) >= 2 {
		switch parts[1] {
		case "show":
			if len(parts) >= 4 {
				if idx, err := strconv.Atoi(parts[2]); err == nil {
					expandedPoolIdx = idx
				}
				if p, err := strconv.Atoi(parts[3]); err == nil && p >= 0 {
					page = p
				}
			}
		case "hide":
			expandedPoolIdx = -1
			page = 0
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 22*time.Second)
	defer cancel()

	text, keyboard, _, _, renderErr := b.renderSmartMoneyMessage(ctx, &smCtx, expandedPoolIdx, page)
	if renderErr != nil {
		_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, "渲染失败，请稍后重试"))
		return
	}

	if err := b.editMessageText(chatID, messageID, text); err != nil {
		return
	}
	_ = b.editMessageReplyMarkup(chatID, messageID, keyboard)
}
