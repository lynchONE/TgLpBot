package bot

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (b *Bot) handleAutoTotalAmountInput(chatID int64, user *models.User, text string) {
	if user == nil || b.autoLPCfgService == nil {
		return
	}

	v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.ToUpper(text), "USDT")), 64)
	if err != nil || v <= 0 || v > 1_000_000 {
		b.refreshAutoMenuFromSession(chatID, user, "❌ 数值无效。请输入 0-1000000 之间的 USDT 数值，例如：`200`")
		return
	}
	if _, err := b.autoLPCfgService.Update(user.ID, map[string]interface{}{
		"total_amount_usdt": v,
	}); err != nil {
		b.refreshAutoMenuFromSession(chatID, user, fmt.Sprintf("❌ 更新 AutoLP 总投入失败：%v", err))
		return
	}
	_ = database.DeleteUserSession(user.TelegramID, "state")
	b.refreshAutoMenuFromSession(chatID, user, "")
}

func (b *Bot) handleAutoMaxTasksInput(chatID int64, user *models.User, text string) {
	if user == nil || b.autoLPCfgService == nil {
		return
	}

	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n < 1 || n > 100 {
		b.refreshAutoMenuFromSession(chatID, user, "❌ 数值无效。请输入 1-100 之间的整数，例如：`3`")
		return
	}

	check, _ := b.accessService.CheckUserAccess(user.ID, time.Now())
	if !check.IsAdmin && check.Access != nil && check.Access.MaxActiveTasks > 0 && n > check.Access.MaxActiveTasks {
		b.refreshAutoMenuFromSession(chatID, user, fmt.Sprintf("❌ 最大任务数不能超过您的任务上限 (%d)。", check.Access.MaxActiveTasks))
		return
	}

	if _, err := b.autoLPCfgService.Update(user.ID, map[string]interface{}{
		"max_active_tasks": n,
	}); err != nil {
		b.refreshAutoMenuFromSession(chatID, user, fmt.Sprintf("❌ 更新 AutoLP 最大任务数失败：%v", err))
		return
	}
	_ = database.DeleteUserSession(user.TelegramID, "state")
	b.refreshAutoMenuFromSession(chatID, user, "")
}

func (b *Bot) handleAutoTakeProfitInput(chatID int64, user *models.User, text string) {
	if user == nil || b.autoLPCfgService == nil {
		return
	}

	raw := strings.TrimSpace(strings.TrimSuffix(strings.ToUpper(text), "USDT"))
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 || v > 1_000_000 {
		b.refreshAutoMenuFromSession(chatID, user, "❌ 数值无效。请输入 0-1000000 之间的 USDT 数值，例如：`100` 或 `0`")
		return
	}
	if _, err := b.autoLPCfgService.Update(user.ID, map[string]interface{}{
		"take_profit_usdt": v,
	}); err != nil {
		b.refreshAutoMenuFromSession(chatID, user, fmt.Sprintf("❌ 更新盈利关闭失败：%v", err))
		return
	}
	_ = database.DeleteUserSession(user.TelegramID, "state")
	b.refreshAutoMenuFromSession(chatID, user, "")
}

func (b *Bot) handleAutoStopLossInput(chatID int64, user *models.User, text string) {
	if user == nil || b.autoLPCfgService == nil {
		return
	}

	raw := strings.TrimSpace(strings.TrimSuffix(strings.ToUpper(text), "USDT"))
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 || v > 1_000_000 {
		b.refreshAutoMenuFromSession(chatID, user, "❌ 数值无效。请输入 0-1000000 之间的 USDT 数值，例如：`50` 或 `0`")
		return
	}
	if _, err := b.autoLPCfgService.Update(user.ID, map[string]interface{}{
		"stop_loss_usdt": v,
	}); err != nil {
		b.refreshAutoMenuFromSession(chatID, user, fmt.Sprintf("❌ 更新亏损关闭失败：%v", err))
		return
	}
	_ = database.DeleteUserSession(user.TelegramID, "state")
	b.refreshAutoMenuFromSession(chatID, user, "")
}

func (b *Bot) handleAutoSwitchThresholdInput(chatID int64, user *models.User, text string) {
	if user == nil || b.autoLPCfgService == nil {
		return
	}

	raw := strings.TrimSpace(strings.TrimSuffix(text, "%"))
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 || v > 1000 {
		b.refreshAutoMenuFromSession(chatID, user, "❌ 数值无效。请输入 0-1000 之间的百分比，例如：`20` 或 `0`")
		return
	}
	if _, err := b.autoLPCfgService.Update(user.ID, map[string]interface{}{
		"switch_min_improvement_pct": v,
	}); err != nil {
		b.refreshAutoMenuFromSession(chatID, user, fmt.Sprintf("❌ 更新换池阈值失败：%v", err))
		return
	}
	_ = database.DeleteUserSession(user.TelegramID, "state")
	b.refreshAutoMenuFromSession(chatID, user, "")
}
