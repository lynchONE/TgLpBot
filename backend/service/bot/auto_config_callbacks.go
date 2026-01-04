package bot

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleAutoConfigToggle(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if user == nil {
		return
	}
	if b.autoLPCfgService == nil {
		_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "AutoLP 配置服务未初始化。")
		return
	}

	_ = database.SetUserSession(user.TelegramID, autoMenuMsgIDKey, fmt.Sprintf("%d", query.Message.MessageID), 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, autoMenuViewKey, autoMenuViewCfg, 24*time.Hour)

	cfg, err := b.autoLPCfgService.GetOrCreate(user.ID)
	if err != nil {
		_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, fmt.Sprintf("❌ 获取 AutoLP 配置失败：%v", err))
		return
	}

	if cfg.Enabled {
		now := time.Now()
		if _, err := b.autoLPCfgService.Update(user.ID, map[string]interface{}{
			"enabled":          false,
			"last_disabled_at": now,
		}); err != nil {
			_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, fmt.Sprintf("❌ 关闭 AutoLP 失败：%v", err))
			return
		}
		notice := "已关闭 AutoLP，并开始撤出当前自动仓位。"
		if b.autoLPService == nil {
			notice = "⚠️ 已关闭 AutoLP，但服务未初始化，未能发起撤仓。"
		} else if err := b.autoLPService.RequestExitForAutoTasks(user.ID, "🛑 AutoLP 已关闭", 1.0); err != nil {
			notice = fmt.Sprintf("⚠️ 已关闭 AutoLP，但发起撤仓失败：%v", err)
		}
		_ = database.DeleteUserSession(user.TelegramID, "state")
		b.api.Send(tgbotapi.NewCallback(query.ID, "已关闭"))
		_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, notice)
		return
	}

	// Enabling
	if config.AppConfig == nil || !config.AppConfig.AutoLPEnabled {
		_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "⚠️ 系统未开启 AutoLP 扫描，暂无法开启。")
		return
	}
	if !config.AppConfig.AutoLPExecuteEnabled {
		_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "⚠️ 系统已关闭自动开仓总开关，暂无法开启。")
		return
	}

	// Require user authorization.
	if !b.checkUserAuthorized(query.Message.Chat.ID, user) {
		return
	}

	// Require a wallet.
	wallets, err := b.walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "您还没有任何钱包。请先使用 /wallet 导入一个。")
		return
	}

	if cfg.TotalAmountUSDT <= 0 {
		_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "请先设置 AutoLP 总投入（USDT）。")
		return
	}
	if cfg.MaxActiveTasks <= 0 {
		_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "请先设置 AutoLP 最大任务数（>=1）。")
		return
	}

	// Respect task quota when enabling config (hard check only for max tasks setting).
	check, _ := b.accessService.CheckUserAccess(user.ID, time.Now())
	if !check.IsAdmin && check.Access != nil && check.Access.MaxActiveTasks > 0 {
		if cfg.MaxActiveTasks > check.Access.MaxActiveTasks {
			_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, fmt.Sprintf("❌ 最大任务数不能超过您的任务上限 (%d)。", check.Access.MaxActiveTasks))
			return
		}
	}

	now := time.Now()
	if _, err := b.autoLPCfgService.Update(user.ID, map[string]interface{}{
		"enabled":          true,
		"last_enabled_at":  now,
		"last_disabled_at": nil,
	}); err != nil {
		_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, fmt.Sprintf("❌ 开启 AutoLP 失败：%v", err))
		return
	}
	_ = database.DeleteUserSession(user.TelegramID, "state")
	b.api.Send(tgbotapi.NewCallback(query.ID, "已开启"))
	_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "")
}

func (b *Bot) handleAutoConfigRefresh(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if user == nil {
		return
	}
	_ = database.SetUserSession(user.TelegramID, autoMenuMsgIDKey, fmt.Sprintf("%d", query.Message.MessageID), 24*time.Hour)
	_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "")
}

func (b *Bot) handleAutoConfigSetTotal(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if user == nil {
		return
	}
	_ = database.SetUserSession(user.TelegramID, autoMenuMsgIDKey, fmt.Sprintf("%d", query.Message.MessageID), 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, autoMenuViewKey, autoMenuViewCfg, 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, "state", "awaiting_auto_total_amount", 30*time.Minute)
	_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "")
}

func (b *Bot) handleAutoConfigSetMaxTasks(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if user == nil {
		return
	}
	_ = database.SetUserSession(user.TelegramID, autoMenuMsgIDKey, fmt.Sprintf("%d", query.Message.MessageID), 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, autoMenuViewKey, autoMenuViewCfg, 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, "state", "awaiting_auto_max_tasks", 30*time.Minute)
	_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "")
}

func (b *Bot) handleAutoConfigSetTakeProfit(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if user == nil {
		return
	}
	_ = database.SetUserSession(user.TelegramID, autoMenuMsgIDKey, fmt.Sprintf("%d", query.Message.MessageID), 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, autoMenuViewKey, autoMenuViewCfg, 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, "state", "awaiting_auto_take_profit", 30*time.Minute)
	_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "")
}

func (b *Bot) handleAutoConfigSetStopLoss(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if user == nil {
		return
	}
	_ = database.SetUserSession(user.TelegramID, autoMenuMsgIDKey, fmt.Sprintf("%d", query.Message.MessageID), 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, autoMenuViewKey, autoMenuViewCfg, 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, "state", "awaiting_auto_stop_loss", 30*time.Minute)
	_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "")
}

func (b *Bot) handleAutoViewStrategy(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if user == nil {
		return
	}
	_ = database.SetUserSession(user.TelegramID, autoMenuMsgIDKey, fmt.Sprintf("%d", query.Message.MessageID), 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, autoMenuViewKey, autoMenuViewStrat, 24*time.Hour)
	_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "")
}

func (b *Bot) handleAutoViewConfig(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if user == nil {
		return
	}
	_ = database.SetUserSession(user.TelegramID, autoMenuMsgIDKey, fmt.Sprintf("%d", query.Message.MessageID), 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, autoMenuViewKey, autoMenuViewCfg, 24*time.Hour)
	_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "")
}

func (b *Bot) handleAutoCancelInput(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if user == nil {
		return
	}
	_ = database.DeleteUserSession(user.TelegramID, "state")
	_ = database.SetUserSession(user.TelegramID, autoMenuMsgIDKey, fmt.Sprintf("%d", query.Message.MessageID), 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, autoMenuViewKey, autoMenuViewCfg, 24*time.Hour)
	_ = b.refreshAutoMenu(query.Message.Chat.ID, query.Message.MessageID, user, "")
}

func (b *Bot) handleAutoFixLegacyMenuMessageID(query *tgbotapi.CallbackQuery, user *models.User) {
	// Best-effort: keep compatibility if the stored msg id is lost.
	if user == nil || query == nil || query.Message == nil {
		return
	}
	msgIDStr, _ := database.GetUserSession(user.TelegramID, autoMenuMsgIDKey)
	if msgID, _ := strconv.Atoi(strings.TrimSpace(msgIDStr)); msgID <= 0 {
		_ = database.SetUserSession(user.TelegramID, autoMenuMsgIDKey, fmt.Sprintf("%d", query.Message.MessageID), 24*time.Hour)
	}
}
