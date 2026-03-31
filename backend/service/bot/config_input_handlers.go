package bot

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/base/security"
	"fmt"
	"net/url"
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
	database.ClearUserSession(user.TelegramID)
	b.sendMessage(messageChatID, "该配置已下线，不再进行剩余资产容忍度校验。")
}

func (b *Bot) handleGlobalZapLossToleranceInput(messageChatID int64, user *models.User, text string) {
	database.ClearUserSession(user.TelegramID)
	b.sendMessage(messageChatID, "该配置已下线，不再进行开仓亏损校验。")
}

func normalizeBarkKeyInput(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}

	// Support pasting a full URL, e.g. https://api.day.app/<key>/title/body
	candidate := s
	if !strings.Contains(candidate, "://") && strings.Contains(candidate, "/") && strings.Contains(candidate, ".") {
		candidate = "https://" + candidate
	}
	if u, err := url.Parse(candidate); err == nil && u.Host != "" {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}

	return strings.Trim(s, "/")
}

func (b *Bot) handleGlobalBarkKeyInput(messageChatID int64, user *models.User, text string) {
	raw := strings.TrimSpace(text)
	if raw == "" || strings.EqualFold(raw, "clear") {
		_, err := b.configService.Update(user.ID, map[string]interface{}{
			"bark_key_encrypted": "",
			"bark_enabled":       false,
		})
		if err != nil {
			b.sendMessage(messageChatID, fmt.Sprintf("❌ 更新配置失败：%v", err))
			return
		}
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(messageChatID, "✅ 已清除 Bark Key，并关闭 Bark 通知")
		return
	}

	if config.AppConfig == nil {
		b.sendMessage(messageChatID, "❌ 配置未加载，请稍后重试")
		return
	}
	keyBytes, err := security.DecodeHexKey32(config.AppConfig.EncryptionKey)
	if err != nil {
		b.sendMessage(messageChatID, fmt.Sprintf("❌ 加密密钥不可用：%v", err))
		return
	}

	key := normalizeBarkKeyInput(raw)
	if key == "" {
		b.sendMessage(messageChatID, "❌ Bark Key 无效，请重新输入")
		return
	}

	enc, err := security.EncryptAESGCMToHex(keyBytes, []byte(key))
	if err != nil {
		b.sendMessage(messageChatID, fmt.Sprintf("❌ 保存失败（加密错误）：%v", err))
		return
	}

	_, err = b.configService.Update(user.ID, map[string]interface{}{
		"bark_key_encrypted": enc,
		"bark_enabled":       true,
	})
	if err != nil {
		b.sendMessage(messageChatID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}
	database.ClearUserSession(user.TelegramID)
	b.sendMessage(messageChatID, "✅ 已保存 Bark Key，并开启 Bark 通知")
}

func normalizeBarkServerInput(input string) (string, bool) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", false
	}

	// Accept bare host like api.day.app
	candidate := s
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	u, err := url.Parse(candidate)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}

	out := u.Scheme + "://" + u.Host
	path := strings.TrimRight(u.Path, "/")
	if path != "" && path != "/" {
		out += path
	}
	return strings.TrimRight(out, "/"), true
}

func (b *Bot) handleGlobalBarkServerInput(messageChatID int64, user *models.User, text string) {
	raw := strings.TrimSpace(text)
	if raw == "" {
		b.sendMessage(messageChatID, "数值无效。请输入 Bark Server，例如：`https://api.day.app`")
		return
	}
	if strings.EqualFold(raw, "default") || strings.EqualFold(raw, "clear") {
		_, err := b.configService.Update(user.ID, map[string]interface{}{
			"bark_server": "",
		})
		if err != nil {
			b.sendMessage(messageChatID, fmt.Sprintf("❌ 更新配置失败：%v", err))
			return
		}
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(messageChatID, "✅ 已恢复 Bark Server 默认值（https://api.day.app）")
		return
	}

	server, ok := normalizeBarkServerInput(raw)
	if !ok {
		b.sendMessage(messageChatID, "数值无效。请输入 Bark Server，例如：`https://api.day.app` 或自建服务地址")
		return
	}

	_, err := b.configService.Update(user.ID, map[string]interface{}{
		"bark_server": server,
	})
	if err != nil {
		b.sendMessage(messageChatID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}
	database.ClearUserSession(user.TelegramID)
	b.sendMessage(messageChatID, fmt.Sprintf("✅ 已更新 Bark Server：%s", server))
}

func (b *Bot) handleGlobalBarkGroupInput(messageChatID int64, user *models.User, text string) {
	raw := strings.TrimSpace(text)
	if raw == "" || strings.EqualFold(raw, "clear") {
		_, err := b.configService.Update(user.ID, map[string]interface{}{
			"bark_group": "",
		})
		if err != nil {
			b.sendMessage(messageChatID, fmt.Sprintf("❌ 更新配置失败：%v", err))
			return
		}
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(messageChatID, "✅ 已清空 Bark Group")
		return
	}

	// Basic length guard to avoid abuse
	r := []rune(raw)
	if len(r) > 50 {
		b.sendMessage(messageChatID, "数值过长。Bark Group 建议不超过 50 个字符")
		return
	}

	_, err := b.configService.Update(user.ID, map[string]interface{}{
		"bark_group": raw,
	})
	if err != nil {
		b.sendMessage(messageChatID, fmt.Sprintf("❌ 更新配置失败：%v", err))
		return
	}
	database.ClearUserSession(user.TelegramID)
	b.sendMessage(messageChatID, fmt.Sprintf("✅ 已更新 Bark Group：%s", raw))
}
