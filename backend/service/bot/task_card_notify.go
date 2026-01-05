package bot

import (
	"fmt"

	"TgLpBot/base/config"
	"TgLpBot/service/strategy"
	"TgLpBot/service/user"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// SendTaskCardForUser sends a task card message without starting a full bot loop.
func SendTaskCardForUser(userID uint, taskID uint) error {
	if config.AppConfig == nil || config.AppConfig.TelegramBotToken == "" {
		return fmt.Errorf("telegram bot token not configured")
	}

	api, err := tgbotapi.NewBotAPI(config.AppConfig.TelegramBotToken)
	if err != nil {
		return err
	}

	b := &Bot{
		api:         api,
		userService: user.NewUserService(),
		taskService: strategy.NewStrategyTaskService(),
		pnlService:  strategy.NewPnLService(),
	}

	u, err := b.userService.GetUserByID(userID)
	if err != nil {
		return err
	}
	task, err := b.taskService.GetByID(userID, taskID)
	if err != nil {
		return err
	}

	msg, err := b.sendTaskCardMessage(u.TelegramID, b.formatTaskCardWithRefresh(task), b.taskKeyboardWithRefresh(task))
	if err != nil {
		return err
	}
	if msg.MessageID != 0 {
		b.startTaskAutoRefresh(u.TelegramID, msg.MessageID, task.ID, userID)
	}
	return nil
}
