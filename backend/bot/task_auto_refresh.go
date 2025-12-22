package bot

import (
	"fmt"
	"log"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// AutoRefreshSession tracks auto-refresh sessions for task cards
type AutoRefreshSession struct {
	ChatID    int64
	MessageID int
	TaskID    uint
	UserID    uint
	StopChan  chan struct{}
	Active    bool
}

var (
	// Map: chatID_messageID -> session
	autoRefreshSessions = make(map[string]*AutoRefreshSession)
	autoRefreshMutex    sync.RWMutex
)

// startTaskAutoRefresh starts auto-refreshing a task card
func (b *Bot) startTaskAutoRefresh(chatID int64, messageID int, taskID, userID uint) {
	key := fmt.Sprintf("%d_%d", chatID, messageID)

	autoRefreshMutex.Lock()
	// Stop any existing session for the same task (keep only the latest card updating)
	for k, existing := range autoRefreshSessions {
		if existing.Active && existing.UserID == userID && existing.TaskID == taskID {
			close(existing.StopChan)
			existing.Active = false
			delete(autoRefreshSessions, k)
		}
	}

	session := &AutoRefreshSession{
		ChatID:    chatID,
		MessageID: messageID,
		TaskID:    taskID,
		UserID:    userID,
		StopChan:  make(chan struct{}),
		Active:    true,
	}
	autoRefreshSessions[key] = session
	autoRefreshMutex.Unlock()

	go b.autoRefreshLoop(session)
	log.Printf("[Bot] Started auto-refresh for task #%d, chat %d, msg %d", taskID, chatID, messageID)
}

// stopTaskAutoRefresh stops auto-refreshing
func (b *Bot) stopTaskAutoRefresh(chatID int64, messageID int) {
	key := fmt.Sprintf("%d_%d", chatID, messageID)

	autoRefreshMutex.Lock()
	defer autoRefreshMutex.Unlock()

	if session, ok := autoRefreshSessions[key]; ok && session.Active {
		close(session.StopChan)
		session.Active = false
		delete(autoRefreshSessions, key)
		log.Printf("[Bot] Stopped auto-refresh for chat %d, msg %d", chatID, messageID)
	}
}

// autoRefreshLoop refreshes task card every 30 seconds
func (b *Bot) autoRefreshLoop(session *AutoRefreshSession) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-session.StopChan:
			log.Printf("[Bot] Auto-refresh loop stopped for task #%d", session.TaskID)
			return
		case <-ticker.C:
			b.refreshTaskCard(session)
		}
	}
}

// refreshTaskCard updates the task card message
func (b *Bot) refreshTaskCard(session *AutoRefreshSession) {
	task, err := b.taskService.GetByID(session.UserID, session.TaskID)
	if err != nil {
		log.Printf("[Bot] Failed to get task #%d for refresh: %v", session.TaskID, err)
		return
	}

	// Update message text
	editMsg := tgbotapi.NewEditMessageText(
		session.ChatID,
		session.MessageID,
		b.formatTaskCardWithRefresh(task),
	)
	editMsg.ParseMode = "Markdown"
	editMsg.DisableWebPagePreview = true

	if _, err := b.api.Send(editMsg); err != nil {
		log.Printf("[Bot] Failed to refresh task card: %v", err)
		// If message not found or too old, stop refreshing
		if err.Error() == "Bad Request: message to edit not found" || err.Error() == "Bad Request: message is not modified" {
			b.stopTaskAutoRefresh(session.ChatID, session.MessageID)
		}
		return
	}

	// Update keyboard
	editKeyboard := tgbotapi.NewEditMessageReplyMarkup(
		session.ChatID,
		session.MessageID,
		b.taskKeyboardWithRefresh(task),
	)
	b.api.Send(editKeyboard)

	log.Printf("[Bot] Refreshed task #%d card", session.TaskID)
}
