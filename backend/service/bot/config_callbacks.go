package bot

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleConfigRebalanceTimeout(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	database.SetUserSession(user.TelegramID, "state", "awaiting_global_rebalance_timeout", 30*time.Minute)
	b.sendMessage(query.Message.Chat.ID, "⏱️ 请输入再平衡超时（秒），例如：`300`")
}

func (b *Bot) handleConfigStopLossToggle(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	cfg, err := b.configService.GetOrCreate(user.ID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 获取配置失败：%v", err))
		return
	}
	newValue := !cfg.StopLossEnabled
	_, err = b.configService.Update(user.ID, map[string]interface{}{
		"stop_loss_enabled": newValue,
	})
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}
	if newValue {
		b.sendMessage(query.Message.Chat.ID, "✅ 已开启秒止损")
	} else {
		b.sendMessage(query.Message.Chat.ID, "✅ 已关闭秒止损")
	}
}

func (b *Bot) handleConfigStopLossDelay(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	database.SetUserSession(user.TelegramID, "state", "awaiting_global_stop_loss_delay", 30*time.Minute)
	b.sendMessage(query.Message.Chat.ID, "⏲️ 请输入秒止损阈值（秒，0 表示立即触发），例如：`0` 或 `10`")
}

func (b *Bot) handleConfigSlippage(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	database.SetUserSession(user.TelegramID, "state", "awaiting_global_slippage", 30*time.Minute)
	b.sendMessage(query.Message.Chat.ID, "📊 请输入滑点（百分比），例如：`1` 表示 1%")
}

func (b *Bot) handleConfigReinvestToggle(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	cfg, err := b.configService.GetOrCreate(user.ID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 获取配置失败：%v", err))
		return
	}
	newValue := !cfg.AutoReinvest
	_, err = b.configService.Update(user.ID, map[string]interface{}{
		"auto_reinvest": newValue,
	})
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}
	if newValue {
		b.sendMessage(query.Message.Chat.ID, "✅ 已开启复投")
	} else {
		b.sendMessage(query.Message.Chat.ID, "✅ 已关闭复投")
	}
}

func (b *Bot) handleConfigResidualTolerance(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	database.SetUserSession(user.TelegramID, "state", "awaiting_global_residual_tolerance", 30*time.Minute)
	b.sendMessage(query.Message.Chat.ID, "🧾 请输入剩余资产容忍度（百分比），例如：`1` 表示最多允许 1% 的剩余资产未投入")
}

func (b *Bot) handleConfigExtraNotificationsToggle(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	cfg, err := b.configService.GetOrCreate(user.ID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 获取配置失败：%v", err))
		return
	}
	newValue := !cfg.ExtraNotificationsEnabled
	_, err = b.configService.Update(user.ID, map[string]interface{}{
		"extra_notifications_enabled": newValue,
	})
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}
	if newValue {
		b.sendMessage(query.Message.Chat.ID, "✅ 已开启日志通知（涨破/跌破/AutoLP候选池）")
	} else {
		b.sendMessage(query.Message.Chat.ID, "✅ 已关闭日志通知（涨破/跌破/AutoLP候选池）")
	}
}

func (b *Bot) handleConfigFilterChineseToggle(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	cfg, err := b.configService.GetOrCreate(user.ID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 获取配置失败：%v", err))
		return
	}
	newValue := !cfg.FilterChineseTokens
	_, err = b.configService.Update(user.ID, map[string]interface{}{
		"filter_chinese_tokens": newValue,
	})
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}
	if newValue {
		b.sendMessage(query.Message.Chat.ID, "✅ 已开启过滤中文代币（交易对含中文将不自动开仓）")
	} else {
		b.sendMessage(query.Message.Chat.ID, "✅ 已关闭过滤中文代币")
	}
}

func (b *Bot) handleConfigBarkToggle(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	cfg, err := b.configService.GetOrCreate(user.ID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 获取配置失败：%v", err))
		return
	}

	if strings.TrimSpace(cfg.BarkKeyEncrypted) == "" {
		database.SetUserSession(user.TelegramID, "state", "awaiting_global_bark_key", 30*time.Minute)
		b.sendMessage(query.Message.Chat.ID, "📲 尚未配置 Bark Key。\n\n请发送 Bark Key（或粘贴 day.app 链接）。\n发送 `clear` 可清除。\n发送 /cancel 取消。")
		return
	}

	newValue := !cfg.BarkEnabled
	_, err = b.configService.Update(user.ID, map[string]interface{}{
		"bark_enabled": newValue,
	})
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}
	if newValue {
		b.sendMessage(query.Message.Chat.ID, "✅ 已开启 Bark 通知")
	} else {
		b.sendMessage(query.Message.Chat.ID, "✅ 已关闭 Bark 通知")
	}
}

func (b *Bot) handleConfigBarkKey(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	database.SetUserSession(user.TelegramID, "state", "awaiting_global_bark_key", 30*time.Minute)
	b.sendMessage(query.Message.Chat.ID, "🔑 请输入 Bark Key（或粘贴 day.app 链接）。\n发送 `clear` 可清除。\n发送 /cancel 取消。")
}

func (b *Bot) handleConfigBarkServer(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	database.SetUserSession(user.TelegramID, "state", "awaiting_global_bark_server", 30*time.Minute)
	b.sendMessage(query.Message.Chat.ID, "🌐 请输入 Bark Server（例如：`https://api.day.app` 或自建服务地址）。\n发送 `default` 恢复默认。\n发送 /cancel 取消。")
}

func (b *Bot) handleConfigBarkGroup(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	database.SetUserSession(user.TelegramID, "state", "awaiting_global_bark_group", 30*time.Minute)
	b.sendMessage(query.Message.Chat.ID, "👥 请输入 Bark Group（分组，可为空）。\n发送 `clear` 清空分组。\n发送 /cancel 取消。")
}

func configDefaultChainKeyboard(chains []string, current string) any {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, 4)
	cur := make([]tgbotapi.InlineKeyboardButton, 0, 2)

	current = config.NormalizeChain(current)
	for _, c := range chains {
		ch := config.NormalizeChain(c)
		if ch == "" {
			continue
		}
		label := chainLabel(ch)
		if ch == current {
			label = "✅ " + label
		}
		cur = append(cur, tgbotapi.NewInlineKeyboardButtonData(label, "config_default_chain_set_"+ch))
		if len(cur) >= 2 {
			rows = append(rows, cur)
			cur = make([]tgbotapi.InlineKeyboardButton, 0, 2)
		}
	}
	if len(cur) > 0 {
		rows = append(rows, cur)
	}
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("取消", "config_default_chain_cancel"),
	})

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func (b *Bot) handleConfigMultiChainToggle(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	cfg, err := b.configService.GetOrCreate(user.ID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 获取配置失败：%v", err))
		return
	}

	newValue := !cfg.MultiChainEnabled
	if _, err := b.configService.Update(user.ID, map[string]interface{}{
		"multi_chain_enabled": newValue,
	}); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}

	// Clear chain-scoped sessions so the next flow picks up the new mode.
	_ = database.DeleteUserSession(user.TelegramID, sessionNewPositionChain)
	_ = database.DeleteUserSession(user.TelegramID, sessionWalletSwapChain)

	if newValue {
		b.sendMessage(query.Message.Chat.ID, "✅ 已开启多链模式（开仓/换币将提示选择链）")
	} else {
		effective := config.PickEnabledChain(cfg.DefaultChain)
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("✅ 已关闭多链模式（默认使用 %s）", chainLabel(effective)))
	}

	// Refresh menu.
	b.handleViewConfig(query, user)
}

func (b *Bot) handleConfigDefaultChain(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	cfg, err := b.configService.GetOrCreate(user.ID)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 获取配置失败：%v", err))
		return
	}

	chains := enabledChains()
	text := fmt.Sprintf("🔗 请选择默认链（关闭多链模式时生效）\n\n当前默认链：*%s*", chainLabel(cfg.DefaultChain))
	b.sendMessageWithKeyboard(query.Message.Chat.ID, text, configDefaultChainKeyboard(chains, cfg.DefaultChain))
}

func (b *Bot) handleConfigDefaultChainSelect(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if query == nil || query.Message == nil || query.Message.Chat == nil {
		return
	}

	data := strings.TrimSpace(query.Data)
	if data == "config_default_chain_cancel" {
		b.sendMessage(query.Message.Chat.ID, "✅ 已取消")
		return
	}

	chain := config.NormalizeChain(strings.TrimPrefix(data, "config_default_chain_set_"))
	if chain == "" {
		b.sendMessage(query.Message.Chat.ID, "❌ 无效的链")
		return
	}

	enabled := false
	for _, c := range enabledChains() {
		if config.NormalizeChain(c) == chain {
			enabled = true
			break
		}
	}
	if !enabled {
		b.sendMessage(query.Message.Chat.ID, "❌ 当前未启用该链，请检查 CHAINS 配置")
		return
	}

	if _, err := b.configService.Update(user.ID, map[string]interface{}{
		"default_chain": chain,
	}); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}

	// Clear chain-scoped sessions so the next flow picks up the new default.
	_ = database.DeleteUserSession(user.TelegramID, sessionNewPositionChain)
	_ = database.DeleteUserSession(user.TelegramID, sessionWalletSwapChain)

	b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("✅ 默认链已设置为 *%s*", chainLabel(chain)))
	b.handleViewConfig(query, user)
}

func (b *Bot) handleViewConfig(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	chatID := user.TelegramID
	if query.Message != nil {
		chatID = query.Message.Chat.ID
	}
	cfg, err := b.configService.GetOrCreate(user.ID)
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 获取配置失败：%v", err))
		return
	}

	text := formatGlobalConfigMenuText(cfg)
	keyboard := globalConfigKeyboard()
	if query.Message != nil {
		_ = b.editMessageText(query.Message.Chat.ID, query.Message.MessageID, text)
		_ = b.editMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, keyboard)
		return
	}
	b.sendMessageWithKeyboard(chatID, text, keyboard)
}

func boolToOnOff(v bool) string {
	if v {
		return "开启"
	}
	return "关闭"
}
