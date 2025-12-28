package bot

import (
	"TgLpBot/config"
	"TgLpBot/database"
	"TgLpBot/models"
	"TgLpBot/services"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	autoMenuMsgIDKey  = "autolp_menu_msg_id"
	autoMenuViewKey   = "autolp_menu_view"
	autoMenuViewCfg   = "config"
	autoMenuViewStrat = "strategy"
)

// handleAuto shows per-user AutoLP config & controls.
func (b *Bot) handleAuto(message *tgbotapi.Message, user *models.User) {
	if user == nil {
		return
	}
	_ = database.SetUserSession(user.TelegramID, autoMenuViewKey, autoMenuViewCfg, 24*time.Hour)
	b.openAutoMenu(message.Chat.ID, user, "")
}

func (b *Bot) openAutoMenu(chatID int64, user *models.User, notice string) {
	text, keyboard, view := b.buildAutoMenu(user, notice)
	msg, err := b.sendTaskCardMessage(chatID, text, keyboard)
	if err != nil || msg.MessageID == 0 {
		return
	}
	_ = database.SetUserSession(user.TelegramID, autoMenuMsgIDKey, fmt.Sprintf("%d", msg.MessageID), 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, autoMenuViewKey, view, 24*time.Hour)
}

func (b *Bot) refreshAutoMenuFromSession(chatID int64, user *models.User, notice string) {
	if user == nil {
		return
	}
	msgIDStr, _ := database.GetUserSession(user.TelegramID, autoMenuMsgIDKey)
	msgID, _ := strconv.Atoi(strings.TrimSpace(msgIDStr))
	if msgID == 0 {
		b.openAutoMenu(chatID, user, notice)
		return
	}
	if err := b.refreshAutoMenu(chatID, msgID, user, notice); err != nil {
		b.openAutoMenu(chatID, user, notice)
	}
}

func (b *Bot) refreshAutoMenu(chatID int64, messageID int, user *models.User, notice string) error {
	if user == nil {
		return nil
	}
	text, keyboard, view := b.buildAutoMenu(user, notice)
	if err := b.editMessageText(chatID, messageID, text); err != nil {
		log.Printf("[AutoLP] edit menu text failed: %v", err)
		return err
	}
	if err := b.editMessageReplyMarkup(chatID, messageID, keyboard); err != nil {
		log.Printf("[AutoLP] edit menu keyboard failed: %v", err)
		// Not fatal; the text is updated.
	}
	_ = database.SetUserSession(user.TelegramID, autoMenuMsgIDKey, fmt.Sprintf("%d", messageID), 24*time.Hour)
	_ = database.SetUserSession(user.TelegramID, autoMenuViewKey, view, 24*time.Hour)
	return nil
}

func autoInputPrompt(state string) string {
	switch strings.TrimSpace(state) {
	case "awaiting_auto_total_amount":
		return "⏳ 请输入 AutoLP 总投入（USDT），例如：`200`"
	case "awaiting_auto_max_tasks":
		return "⏳ 请输入 AutoLP 最大任务数（整数，>=1），例如：`3`"
	case "awaiting_auto_take_profit":
		return "⏳ 请输入盈利多少 USDT 关闭 AutoLP 并撤出自动仓位（0 表示不启用），例如：`100` 或 `0`"
	case "awaiting_auto_stop_loss":
		return "⏳ 请输入亏损多少 USDT 关闭 AutoLP 并撤出自动仓位（0 表示不启用），例如：`50` 或 `0`"
	default:
		return ""
	}
}

func (b *Bot) buildAutoMenu(user *models.User, notice string) (string, any, string) {
	if config.AppConfig == nil {
		return "配置未加载，无法显示 AutoLP 信息。", nil, autoMenuViewCfg
	}
	if b.autoLPCfgService == nil {
		return "AutoLP 配置服务未初始化。", nil, autoMenuViewCfg
	}
	if user == nil {
		return "", nil, autoMenuViewCfg
	}

	view := autoMenuViewCfg
	if v, _ := database.GetUserSession(user.TelegramID, autoMenuViewKey); strings.TrimSpace(v) != "" {
		view = strings.TrimSpace(v)
	}
	if view != autoMenuViewCfg && view != autoMenuViewStrat {
		view = autoMenuViewCfg
	}

	cfg, err := b.autoLPCfgService.GetOrCreate(user.ID)
	if err != nil {
		return fmt.Sprintf("❌ 获取 AutoLP 配置失败：%v", err), nil, autoMenuViewCfg
	}

	state, _ := database.GetUserSession(user.TelegramID, "state")
	prompt := autoInputPrompt(state)
	inInput := prompt != ""

	switch view {
	case autoMenuViewStrat:
		text := b.autoStrategyText()
		kb := b.autoStrategyKeyboard()
		return text, kb, view
	default:
		perTask := 0.0
		if cfg.MaxActiveTasks > 0 {
			perTask = cfg.TotalAmountUSDT / float64(cfg.MaxActiveTasks)
		}

		tp := "未设置"
		if cfg.TakeProfitUSDT > 0 {
			tp = fmt.Sprintf("%.2f USDT", cfg.TakeProfitUSDT)
		}
		sl := "未设置"
		if cfg.StopLossUSDT > 0 {
			sl = fmt.Sprintf("%.2f USDT", cfg.StopLossUSDT)
		}

		noticeLine := ""
		if strings.TrimSpace(notice) != "" {
			noticeLine = strings.TrimSpace(notice) + "\n\n"
		}

		promptLine := ""
		if prompt != "" {
			promptLine = "\n\n" + prompt + "\n发送 /cancel 取消操作，或点下方「取消设置」。"
		}

		statsBlock := b.autoStatsText(user, cfg)

		text := fmt.Sprintf(`🤖 *AutoLP 自动开仓配置*

%s*状态*：%s
*总投入*：%.2f USDT
*最大任务数*：%d
*单仓投入*：%.2f USDT（总投入/最大任务数）
*盈利关闭*：%s
*亏损关闭*：%s

%s
达到最大任务数后将不再开新仓，需等有仓位撤仓后才会继续开新仓。
盈利/亏损触发后会关闭 AutoLP，并自动撤出当前自动仓位。%s`,
			noticeLine,
			boolToOnOff(cfg.Enabled),
			cfg.TotalAmountUSDT,
			cfg.MaxActiveTasks,
			perTask,
			tp,
			sl,
			statsBlock,
			promptLine,
		)

		kb := b.autoConfigKeyboard(cfg.Enabled, inInput)
		return text, kb, view
	}
}

func (b *Bot) autoConfigKeyboard(enabled bool, inInput bool) any {
	toggleText := "开启 AutoLP"
	if enabled {
		toggleText = "关闭 AutoLP"
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, 5)
	if inInput {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("取消设置", "auto_cfg_cancel_input"),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(toggleText, "auto_cfg_toggle"),
		tgbotapi.NewInlineKeyboardButtonData("刷新", "auto_cfg_refresh"),
		tgbotapi.NewInlineKeyboardButtonData("当前策略", "auto_view_strategy"),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("设置总投入", "auto_cfg_set_total"),
		tgbotapi.NewInlineKeyboardButtonData("设置最大任务数", "auto_cfg_set_max_tasks"),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("设置盈利关闭", "auto_cfg_set_take_profit"),
		tgbotapi.NewInlineKeyboardButtonData("设置亏损关闭", "auto_cfg_set_stop_loss"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func (b *Bot) autoStrategyKeyboard() any {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("返回设置", "auto_view_config"),
			tgbotapi.NewInlineKeyboardButtonData("刷新", "auto_cfg_refresh"),
		),
	)
}

func (b *Bot) autoStrategyText() string {
	return `📌 *当前策略（V1）*

*怎么选池子*
• 从手续费榜单抓取 5/15/60/360 分钟数据
• 只看 5 分钟维度做“硬筛”：TVL、费率、费用率(5m手续费/TVL)、手续费、成交量必须达标（阈值由服务端配置）
• 对通过硬筛的池子计算 Z5 / Z60，并按评分选 Top（评分主要由 5m 手续费与 TVL 决定，共振需满足费用率/成交量/|Z60|阈值）

*怎么制定开仓策略*
• 状态机（基于 Z5）：急涨 / 震荡 / 温和上涨 才会作为候选开仓
• 区间宽度：按状态选择不同总宽度（震荡更窄、温和上涨适中、急涨更宽）
• 非对称区间：根据共振/状态决定下/上区间比例（急涨偏向上方，震荡对称）
• Tick 计算会按池子的 tickSpacing 取整

*何时撤仓（自动任务）*
• 暴跌：Z5 < -3 触发撤仓（可提高 Gas 倍数）
• 量价衰减：60m 成交量低于过去 24h 均值 × 阈值
• 热度消失：5m 交易笔数连续下降 N 次
• 另外仍复用原有任务监控逻辑：出区间后按配置执行再平衡/止损

说明：上述阈值/宽度参数来自服务端配置；你在 /auto 里设置的“总投入/最大任务数/盈亏关闭”用于控制每个用户的自动开新仓，并在触发盈亏关闭时撤出当前自动仓位。`
}

func (b *Bot) autoStatsText(user *models.User, cfg *models.AutoLPUserConfig) string {
	if user == nil || cfg == nil {
		return ""
	}
	if !cfg.Enabled {
		return "📊 *本次 Auto 任务执行*\nAutoLP 已关闭，暂无运行统计。"
	}

	stats, err := services.NewAutoLPStatsService().GetUserStats(user.ID, cfg)
	if err != nil || stats == nil {
		return "📊 *本次 Auto 任务执行*\n统计暂不可用"
	}

	windowLine := "全部历史"
	if stats.WindowStart != nil {
		cst := time.FixedZone("CST", 8*60*60)
		startStr := stats.WindowStart.In(cst).Format("01-02 15:04")
		endStr := "至今"
		if stats.WindowEnd != nil {
			endStr = stats.WindowEnd.In(cst).Format("01-02 15:04")
		}
		label := strings.TrimSpace(stats.WindowLabel)
		if label != "" && label != "全部历史" {
			windowLine = fmt.Sprintf("%s（%s - %s）", label, startStr, endStr)
		} else {
			windowLine = fmt.Sprintf("%s - %s", startStr, endStr)
		}
	} else if strings.TrimSpace(stats.WindowLabel) != "" {
		windowLine = strings.TrimSpace(stats.WindowLabel)
	}

	bestLine := "无"
	if strings.TrimSpace(stats.BestPair) != "" {
		bestLine = fmt.Sprintf("%s（%s USDT）", escapeTelegramMarkdown(stats.BestPair), formatWei(stats.BestProfit))
	}

	worstLine := "无"
	if strings.TrimSpace(stats.WorstPair) != "" {
		worstLine = fmt.Sprintf("%s（%s USDT）", escapeTelegramMarkdown(stats.WorstPair), formatWei(stats.WorstProfit))
	}

	return fmt.Sprintf(`📊 *本次 Auto 任务执行*
*统计周期*：%s
*开仓次数*：%d
*再平衡次数*：%d
*撤退卫士*：%d
*Gas 消耗*：%s USDT
*累计收益*：%s USDT
*盈利交易对*：%s
*亏损交易对*：%s`,
		windowLine,
		stats.OpenCount,
		stats.RebalanceCount,
		stats.GuardCount,
		formatWei(stats.GasUSDT),
		formatWei(stats.ProfitUSDT),
		bestLine,
		worstLine,
	)
}
