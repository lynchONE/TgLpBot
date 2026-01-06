package bot

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"fmt"
	"strconv"
	"strings"
)

func (b *Bot) handleGlobalRebalanceTimeoutInput(messageChatID int64, user *models.User, text string) {
	seconds, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || seconds < 0 || seconds > 86400 {
		b.sendMessage(messageChatID, "数值无效。请输入 0-86400 之间的整数秒数，例如：`300`")
		return
	}
	_, err = b.configService.Update(user.ID, map[string]interface{}{
		"rebalance_timeout": seconds,
	})
	if err != nil {
		b.sendMessage(messageChatID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}
	database.ClearUserSession(user.TelegramID)
	b.sendMessage(messageChatID, fmt.Sprintf("✅ 已更新再平衡超时：%d 秒", seconds))
}

func (b *Bot) handleGlobalStopLossDelayInput(messageChatID int64, user *models.User, text string) {
	seconds, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || seconds < 0 || seconds > 86400 {
		b.sendMessage(messageChatID, "数值无效。请输入 0-86400 之间的整数秒数，例如：`0` 或 `10`")
		return
	}
	_, err = b.configService.Update(user.ID, map[string]interface{}{
		"stop_loss_delay_seconds": seconds,
	})
	if err != nil {
		b.sendMessage(messageChatID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}
	database.ClearUserSession(user.TelegramID)
	b.sendMessage(messageChatID, fmt.Sprintf("✅ 已更新秒止损阈值：%d 秒", seconds))
}

func (b *Bot) handleGlobalSlippageInput(messageChatID int64, user *models.User, text string) {
	value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(text, "%")), 64)
	if err != nil || value < 0 || value > 100 {
		b.sendMessage(messageChatID, "数值无效。请输入 0-100 之间的滑点百分比，例如：`0.5` 表示 0.5%")
		return
	}
	_, err = b.configService.Update(user.ID, map[string]interface{}{
		"slippage_tolerance": value,
	})
	if err != nil {
		b.sendMessage(messageChatID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}
	database.ClearUserSession(user.TelegramID)
	b.sendMessage(messageChatID, fmt.Sprintf("✅ 已更新滑点：%.2f%%", value))
}

func (b *Bot) handleGlobalResidualToleranceInput(messageChatID int64, user *models.User, text string) {
	value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(text, "%")), 64)
	if err != nil || value < 0 || value > 100 {
		b.sendMessage(messageChatID, "数值无效。请输入 0-100 之间的百分比，例如：`1` 表示 1%")
		return
	}
	_, err = b.configService.Update(user.ID, map[string]interface{}{
		"residual_tolerance": value,
	})
	if err != nil {
		b.sendMessage(messageChatID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}
	database.ClearUserSession(user.TelegramID)
	b.sendMessage(messageChatID, fmt.Sprintf("✅ 已更新剩余资产容忍度：%.2f%%", value))
}
