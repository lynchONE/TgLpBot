package bot

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) sendTaskCardMessage(chatID int64, text string, replyMarkup any) (tgbotapi.Message, error) {
	text = rewriteRebalanceTimeoutText(text)
	replyMarkup = rewriteReplyMarkupRebalanceTimeout(replyMarkup)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = replyMarkup
	msg.DisableWebPagePreview = true

	sent, err := b.api.Send(msg)
	if err == nil {
		return sent, nil
	}
	log.Printf("[Bot] Send task card failed (markdown, len=%d): %v", len(text), err)

	msg.ParseMode = ""
	sent, err = b.api.Send(msg)
	if err == nil {
		return sent, nil
	}
	log.Printf("[Bot] Send task card failed (plain, len=%d): %v", len(text), err)

	msg.ReplyMarkup = nil
	sent, err = b.api.Send(msg)
	if err == nil {
		return sent, nil
	}
	log.Printf("[Bot] Send task card failed (plain/no keyboard, len=%d): %v", len(text), err)

	return tgbotapi.Message{}, err
}
