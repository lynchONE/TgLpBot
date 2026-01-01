package bot

import (
	"TgLpBot/config"
	"TgLpBot/models"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// formatTaskCardWithRefresh formats task card with refresh timestamp
func (b *Bot) formatTaskCardWithRefresh(task *models.StrategyTask) string {
	baseCard := b.formatTaskCard(task)
	// 使用中国时间 UTC+8
	cst := time.FixedZone("CST", 8*60*60)
	now := time.Now().In(cst).Format("15:04:05")
	return baseCard + fmt.Sprintf("\n\n🔄 最后更新：%s (自动刷新中)", now)
}

func (b *Bot) formatTaskCardWithRefreshExpired(task *models.StrategyTask) string {
	baseCard := b.formatTaskCard(task)
	// 使用中国时间 UTC+8
	cst := time.FixedZone("CST", 8*60*60)
	now := time.Now().In(cst).Format("15:04:05")
	return baseCard + fmt.Sprintf("\n\n⏸️ 自动刷新已停止（超过 30 分钟）\n🔄 最后更新：%s\n请重新查看仓位信息以重新开始自动刷新。", now)
}

// taskKeyboardWithRefresh adds a refresh control button
func (b *Bot) taskKeyboardWithRefresh(task *models.StrategyTask) any {
	idStr := fmt.Sprintf("%d", task.ID)

	stopText := "🛑 停止任务"
	if task.Status == models.StrategyStatusStopped {
		stopText = "✅ 已停止"
	} else if task.Status == models.StrategyStatusStopping {
		stopText = "⏳ 停止中"
	}

	stopLossText := fmt.Sprintf("⚡ 秒止损：%s", boolToOnOff(task.StopLossEnabled))
	reinvestText := fmt.Sprintf("🔁 复投：%s", boolToOnOff(task.AutoReinvest))
	base := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(stopText, "task_stop_"+idStr),
			tgbotapi.NewInlineKeyboardButtonData("⏸️ 停止刷新", "task_stop_refresh_"+idStr),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(stopLossText, "task_toggle_stoploss_"+idStr),
			tgbotapi.NewInlineKeyboardButtonData(reinvestText, "task_toggle_reinvest_"+idStr),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("⏱️ 再平衡超时 (%ds)", task.ReopenDelaySeconds), "task_set_rebalance_"+idStr),
			tgbotapi.NewInlineKeyboardButtonData("🧹 兑换残余", "task_swap_dust_"+idStr),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑️ 删除任务", "task_delete_"+idStr),
		),
	)

	if config.AppConfig == nil || strings.TrimSpace(config.AppConfig.TelegramWebAppURL) == "" {
		return base
	}
	return newInlineKeyboardMarkupWithWebAppRow(base, "实时仓位", config.AppConfig.TelegramWebAppURL)
}
