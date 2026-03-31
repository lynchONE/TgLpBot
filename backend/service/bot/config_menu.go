package bot

import (
	"TgLpBot/base/models"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func globalConfigKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("铃铛 再平衡超时", "config_rebalance_timeout"),
			tgbotapi.NewInlineKeyboardButtonData("止损开关", "config_stop_loss_toggle"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("止损延迟", "config_stop_loss_delay"),
			tgbotapi.NewInlineKeyboardButtonData("滑点配置", "config_slippage"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("复投开关", "config_reinvest_toggle"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Bark 通知开关", "config_bark_toggle"),
			tgbotapi.NewInlineKeyboardButtonData("设置 Bark Key", "config_bark_key"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Bark Server", "config_bark_server"),
			tgbotapi.NewInlineKeyboardButtonData("Bark Group", "config_bark_group"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("日志通知", "config_extra_notifications_toggle"),
			tgbotapi.NewInlineKeyboardButtonData("过滤中文代币", "config_filter_chinese_toggle"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("多链模式", "config_multi_chain_toggle"),
			tgbotapi.NewInlineKeyboardButtonData("默认链", "config_default_chain"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("多钱包模式", "config_multi_wallet_toggle"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("刷新", "view_config"),
		),
	)
}

func formatGlobalConfigMenuText(cfg *models.GlobalConfig) string {
	if cfg == nil {
		return "⚙️ *全局配置*\n\n配置加载失败，请稍后重试。"
	}

	barkConfigured := strings.TrimSpace(cfg.BarkKeyEncrypted) != ""
	barkEnabled := cfg.BarkEnabled && barkConfigured
	barkStatus := boolToOnOff(barkEnabled)
	barkKeyStatus := "未配置"
	if barkConfigured {
		barkKeyStatus = "已配置"
	}
	barkServer := strings.TrimSpace(cfg.BarkServer)
	if barkServer == "" {
		barkServer = "https://api.day.app"
	}
	barkGroup := strings.TrimSpace(cfg.BarkGroup)
	if barkGroup == "" {
		barkGroup = "<空>"
	}

	return fmt.Sprintf(`⚙️ *全局配置*

*当前配置*
再平衡超时：%d 秒
止损开关：%s
止损延迟：%d 秒
滑点：%.2f%%
复投：%s
Bark 通知：%s（%s）
Bark Server：%s
Bark Group：%s
日志通知：%s
过滤中文代币：%s
多链模式：%s
多钱包模式：%s
默认链：%s

请选择要配置的选项：`,
		cfg.RebalanceTimeout,
		boolToOnOff(cfg.StopLossEnabled),
		cfg.StopLossDelaySeconds,
		cfg.SlippageTolerance,
		boolToOnOff(cfg.AutoReinvest),
		barkStatus,
		barkKeyStatus,
		barkServer,
		barkGroup,
		boolToOnOff(cfg.ExtraNotificationsEnabled),
		boolToOnOff(cfg.FilterChineseTokens),
		boolToOnOff(cfg.MultiChainEnabled),
		boolToOnOff(cfg.MultiWalletEnabled),
		chainLabel(cfg.DefaultChain),
	)
}
