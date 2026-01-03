package bot

import (
	"TgLpBot/models"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	defaultCleanCount          = 50
	maxCleanCount              = 300
	cleanResultAutoDeleteDelay = 4 * time.Second
)

// handleClean handles the hidden /clean command.
// It tries to delete recent messages in the current private chat.
func (b *Bot) handleClean(message *tgbotapi.Message, user *models.User) {
	_ = user
	if message == nil || message.Chat == nil {
		return
	}
	if message.Chat.Type != "private" {
		// Avoid destructive behavior in groups/supergroups.
		b.sendMessage(message.Chat.ID, "⚠️ /clean 仅支持与机器人私聊使用。")
		return
	}

	count := defaultCleanCount
	if args := strings.Fields(strings.TrimSpace(message.CommandArguments())); len(args) > 0 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			count = v
		}
	}
	if count > maxCleanCount {
		count = maxCleanCount
	}

	startID := message.MessageID
	if startID <= 0 || count <= 0 {
		return
	}

	messageIDs := make([]int, 0, count)
	for i := 0; i < count; i++ {
		id := startID - i
		if id <= 0 {
			break
		}
		messageIDs = append(messageIDs, id)
	}

	deleted := b.tryDeleteMessages(message.Chat.ID, messageIDs)

	// Send a short confirmation and auto-delete it to keep the chat clean.
	confirm := fmt.Sprintf("🧹 已尝试清理最近 %d 条消息（成功 %d 条）。\n\nTelegram 限制：仅能删除 48 小时内消息。", len(messageIDs), deleted)
	sent, err := b.sendMessage(message.Chat.ID, confirm)
	if err == nil && sent.MessageID != 0 {
		go func(chatID int64, msgID int) {
			time.Sleep(cleanResultAutoDeleteDelay)
			_, _ = b.api.Request(tgbotapi.NewDeleteMessage(chatID, msgID))
		}(sent.Chat.ID, sent.MessageID)
	}
}

func (b *Bot) tryDeleteMessages(chatID int64, messageIDs []int) int {
	if len(messageIDs) == 0 {
		return 0
	}

	const batchSize = 100
	bulkSupported := true

	deleted := 0
	consecutiveFailures := 0

	for i := 0; i < len(messageIDs); i += batchSize {
		end := i + batchSize
		if end > len(messageIDs) {
			end = len(messageIDs)
		}
		batch := messageIDs[i:end]

		if bulkSupported {
			if err := b.deleteMessagesBatch(chatID, batch); err == nil {
				deleted += len(batch)
				consecutiveFailures = 0
				continue
			} else if tgErr, ok := err.(*tgbotapi.Error); ok && tgErr.Code == 404 {
				// deleteMessages not supported by this Bot API server; fallback to per-message deletion.
				bulkSupported = false
			}
		}

		for _, msgID := range batch {
			if ok := b.deleteMessageOnce(chatID, msgID); ok {
				deleted++
				consecutiveFailures = 0
			} else {
				consecutiveFailures++
				// Heuristic: once we hit many consecutive failures, we're likely outside deletion window.
				if consecutiveFailures >= 25 {
					return deleted
				}
			}
		}
	}

	return deleted
}

func (b *Bot) deleteMessagesBatch(chatID int64, messageIDs []int) error {
	params := tgbotapi.Params{}
	params.AddNonZero64("chat_id", chatID)
	if err := params.AddInterface("message_ids", messageIDs); err != nil {
		return err
	}

	_, err := b.api.MakeRequest("deleteMessages", params)
	if err == nil {
		return nil
	}
	// Handle flood limits with a single retry.
	if tgErr, ok := err.(*tgbotapi.Error); ok && tgErr.Code == 429 && tgErr.ResponseParameters.RetryAfter > 0 {
		time.Sleep(time.Duration(tgErr.ResponseParameters.RetryAfter) * time.Second)
		_, err2 := b.api.MakeRequest("deleteMessages", params)
		return err2
	}
	return err
}

func (b *Bot) deleteMessageOnce(chatID int64, messageID int) bool {
	if messageID <= 0 {
		return false
	}

	_, err := b.api.Request(tgbotapi.NewDeleteMessage(chatID, messageID))
	if err == nil {
		return true
	}
	// Handle flood limits with a single retry.
	if tgErr, ok := err.(*tgbotapi.Error); ok && tgErr.Code == 429 && tgErr.ResponseParameters.RetryAfter > 0 {
		time.Sleep(time.Duration(tgErr.ResponseParameters.RetryAfter) * time.Second)
		_, err2 := b.api.Request(tgbotapi.NewDeleteMessage(chatID, messageID))
		return err2 == nil
	}
	return false
}
