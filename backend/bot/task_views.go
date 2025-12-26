package bot

import (
	"TgLpBot/blockchain"
	"TgLpBot/config"
	"TgLpBot/models"
	"TgLpBot/services"
	"fmt"
	"log"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func getCurrentTickForTask(task *models.StrategyTask) (int, error) {
	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	switch version {
	case "v4":
		if config.AppConfig == nil {
			return 0, fmt.Errorf("config not loaded")
		}
		if !common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
			return 0, fmt.Errorf("UNISWAP_V4_POOL_MANAGER_ADDRESS not set")
		}
		if !common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) {
			return 0, fmt.Errorf("UNISWAP_V4_STATE_VIEW_ADDRESS not configured")
		}
		stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
		poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
		return blockchain.GetUniswapV4PoolCurrentTickViaStateView(stateView, poolManager, task.PoolId)
	default:
		if !common.IsHexAddress(task.PoolId) {
			return 0, fmt.Errorf("invalid V3 pool address: %s", task.PoolId)
		}
		return blockchain.GetV3PoolCurrentTick(common.HexToAddress(task.PoolId))
	}
}

func buildPriceDisplayLines(task *models.StrategyTask) (string, string) {
	currentLine := "💵 当前价格：--"
	rangeLine := "💹 价格范围：--"

	if task == nil {
		return currentLine, rangeLine
	}

	currentTick, err := getCurrentTickForTask(task)
	if err != nil {
		log.Printf("[TaskView] Current tick query failed for task #%d: %v", task.ID, err)
	} else {
		price, base, quote, ok := services.BuildPriceDisplay(task, currentTick)
		if ok {
			currentLine = fmt.Sprintf(
				"💵 当前价格：1 %s ≈ %s %s",
				escapeTelegramMarkdown(base),
				services.FormatPriceValue(price),
				escapeTelegramMarkdown(quote),
			)
		}
	}

	lower, upper, _, quote, ok := services.BuildRangeDisplay(task, task.TickLower, task.TickUpper)
	if ok {
		rangeLine = fmt.Sprintf(
			"💹 价格范围：%s - %s %s",
			services.FormatPriceValue(lower),
			services.FormatPriceValue(upper),
			escapeTelegramMarkdown(quote),
		)
	}

	return currentLine, rangeLine
}

func formatTaskStatus(status models.StrategyStatus) (string, string) {
	switch status {
	case models.StrategyStatusRunning:
		return "🟢", "运行中"
	case models.StrategyStatusWaiting:
		return "🟡", "等待中"
	case models.StrategyStatusStopping:
		return "🟠", "停止中"
	case models.StrategyStatusStopped:
		return "⚪", "已停止"
	case models.StrategyStatusError:
		return "🔴", "错误"
	default:
		return "❔", string(status)
	}
}

func shortenHex(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 20 {
		return s
	}
	return s[:10] + "..." + s[len(s)-8:]
}

func (b *Bot) formatTaskCard(task *models.StrategyTask) string {
	emoji, statusText := formatTaskStatus(task.Status)
	exchange := strings.TrimSpace(task.Exchange)
	if exchange == "" {
		exchange = "-"
	}
	exchange = escapeTelegramMarkdown(exchange)

	pair := task.Token0Symbol + "/" + task.Token1Symbol
	if strings.TrimSpace(pair) == "/" {
		pair = "-"
	}
	pair = escapeTelegramMarkdown(pair)

	// Display actual invested amount (USDT delta) if we have an open trade record.
	// Calculate PnL
	amountLine := fmt.Sprintf("初始投入：%.2f USDT", task.AmountUSDT)
	if b.pnlService != nil {
		pnl, err := b.pnlService.GetTaskPnL(task)
		if err != nil {
			log.Printf("[TaskView] Get PnL failed for task #%d: %v", task.ID, err)
			// Fallback to simpler display
			amountLine += "\n(获取实时收益失败)"
		} else {
			// Format PnL
			sign := "+"
			if pnl.AbsolutePnLUSDT < 0 {
				sign = ""
			}
			emojiStr := "🟢"
			if pnl.AbsolutePnLUSDT < 0 {
				emojiStr = "🔴"
			}

			dustLine := ""
			dustParts := make([]string, 0, 2)
			if pnl.DustToken0 > 0 {
				dustParts = append(dustParts, fmt.Sprintf("%.4f %s", pnl.DustToken0, escapeTelegramMarkdown(task.Token0Symbol)))
			}
			if pnl.DustToken1 > 0 {
				dustParts = append(dustParts, fmt.Sprintf("%.4f %s", pnl.DustToken1, escapeTelegramMarkdown(task.Token1Symbol)))
			}
			if len(dustParts) > 0 {
				dustLine = fmt.Sprintf("\n🧹 开仓残余：%s (≈%.2f USDT)", strings.Join(dustParts, " + "), pnl.DustValueUSDT)
			}

			// 使用 NetInvestedUSDT（净投入 = 实际支出 - 残余价值）更准确反映仓位内金额
			actualInvested := pnl.NetInvestedUSDT
			if actualInvested <= 0 {
				actualInvested = task.AmountUSDT
			}

			amountLine = fmt.Sprintf(
				"📊 资产状况：\n📈 绝对盈亏：%s%.2f USDT %s\n💵 当前价值：%.2f USDT\n🎁 未领手续费：%.2f USDT\n💰 实际投入：%.2f USDT (预期 %.2f USDT)%s",
				sign, pnl.AbsolutePnLUSDT, emojiStr,
				pnl.HoldingsUSDT,
				pnl.UnclaimedFeesUSDT,
				actualInvested,
				task.AmountUSDT,
				dustLine,
			)
		}
	}

	// 构建头寸 ID 信息
	positionInfo := ""
	v3TokenId := strings.TrimSpace(task.V3TokenID)
	v4TokenId := strings.TrimSpace(task.V4TokenID)

	if v3TokenId != "" && v3TokenId != "0" {
		positionInfo = fmt.Sprintf("\n🎫 头寸 ID：`%s`", v3TokenId)
	} else if v4TokenId != "" && v4TokenId != "0" {
		positionInfo = fmt.Sprintf("\n🎫 头寸 ID：`%s`", v4TokenId)
	}

	currentPriceInfo, priceRangeInfo := buildPriceDisplayLines(task)

	rangePctText := ""
	if task.RangePercentage > 0 {
		rangePctText = fmt.Sprintf(" (±%.2f%%)", task.RangePercentage)
	}

	return fmt.Sprintf(`%s *任务 #%d* (%s)

🏦 交易所：%s
💱 交易对：%s
🔗 池子：`+"`%s`"+`%s

%s
%s%s
💰 %s

⚙️ 策略配置：
⏱️ 再平衡超时：%d 秒
📊 滑点：%.2f%%
⚡ 秒止损：%s
⏲️ 秒止损阈值：%d 秒
🔁 复投：%s
🧾 剩余资产容忍度：%.2f%%`,
		emoji,
		task.ID,
		statusText,
		exchange,
		pair,
		shortenHex(task.PoolId),
		positionInfo,
		currentPriceInfo,
		priceRangeInfo,
		rangePctText,
		amountLine,
		task.ReopenDelaySeconds,
		task.SlippageTolerance,
		boolToOnOff(task.StopLossEnabled),
		task.StopLossDelaySeconds,
		boolToOnOff(task.AutoReinvest),
		task.ResidualTolerance,
	)
}

func (b *Bot) taskKeyboard(task *models.StrategyTask) any {
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
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(stopLossText, "task_toggle_stoploss_"+idStr),
			tgbotapi.NewInlineKeyboardButtonData(reinvestText, "task_toggle_reinvest_"+idStr),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("⏱️ 再平衡超时 (%ds)", task.ReopenDelaySeconds), "task_set_rebalance_"+idStr),
			tgbotapi.NewInlineKeyboardButtonData("🧹 兑换残余", "task_swap_dust_"+idStr),
		),
	)

	if config.AppConfig == nil || strings.TrimSpace(config.AppConfig.TelegramWebAppURL) == "" {
		return base
	}
	return newInlineKeyboardMarkupWithWebAppRow(base, "实时仓位", config.AppConfig.TelegramWebAppURL)
}
