package bot

import (
	"TgLpBot/models"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func globalConfigKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏱️ 再平衡超时", "config_rebalance_timeout"),
			tgbotapi.NewInlineKeyboardButtonData("⚡ 秒止损开关", "config_stop_loss_toggle"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏲️ 秒止损阈值", "config_stop_loss_delay"),
			tgbotapi.NewInlineKeyboardButtonData("📊 滑点配置", "config_slippage"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔁 复投开关", "config_reinvest_toggle"),
			tgbotapi.NewInlineKeyboardButtonData("🧾 剩余资产容忍度", "config_residual_tolerance"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📝 日志通知", "config_extra_notifications_toggle"),
			tgbotapi.NewInlineKeyboardButtonData("🔄 刷新", "view_config"),
		),
	)
}

func formatGlobalConfigMenuText(cfg *models.GlobalConfig) string {
	if cfg == nil {
		return "⚙️ *全局配置*\n\n❌ 获取配置失败，请稍后重试。"
	}

	return fmt.Sprintf(`⚙️ *全局配置*

*当前配置：*
⏱️ 再平衡超时：%d 秒
⚡ 秒止损：%s
⏲️ 秒止损阈值：%d 秒
📊 滑点：%.2f%%
🔁 复投：%s
🧾 剩余资产容忍度：%.2f%%
📝 日志通知：%s

请选择要配置的选项：`,
		cfg.RebalanceTimeout,
		boolToOnOff(cfg.StopLossEnabled),
		cfg.StopLossDelaySeconds,
		cfg.SlippageTolerance,
		boolToOnOff(cfg.AutoReinvest),
		cfg.ResidualTolerance,
		boolToOnOff(cfg.ExtraNotificationsEnabled),
	)
}
