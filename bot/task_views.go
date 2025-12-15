package bot

import (
	"TgLpBot/models"
	"fmt"
	"log"
	"math"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// tickToPrice 将 tick 转换为价格
// 价格 = 1.0001 ^ tick
func tickToPrice(tick int) float64 {
	return math.Pow(1.0001, float64(tick))
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
	pair := task.Token0Symbol + "/" + task.Token1Symbol
	if strings.TrimSpace(pair) == "/" {
		pair = "-"
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

	// 计算价格范围（始终显示非 USDT 币种以 USDT 计价）
	// tick 表示 token1/token0 的价格
	priceLower := tickToPrice(task.TickLower)
	priceUpper := tickToPrice(task.TickUpper)

	// 判断哪个是 USDT
	var priceRangeInfo string
	token0Upper := strings.ToUpper(strings.TrimSpace(task.Token0Symbol))
	token1Upper := strings.ToUpper(strings.TrimSpace(task.Token1Symbol))

	log.Printf("[TaskView] Task #%d: token0=%s token1=%s tickLower=%d tickUpper=%d priceLower=%.6f priceUpper=%.6f",
		task.ID, token0Upper, token1Upper, task.TickLower, task.TickUpper, priceLower, priceUpper)

	if token0Upper == "USDT" {
		// token0 是 USDT，price = token1/USDT，需要取倒数
		if priceLower > 0 && priceUpper > 0 {
			priceInUSDTLower := 1.0 / priceUpper
			priceInUSDTUpper := 1.0 / priceLower
			log.Printf("[TaskView] Task #%d: token0=USDT, inverted price range: %.6f - %.6f", task.ID, priceInUSDTLower, priceInUSDTUpper)
			priceRangeInfo = fmt.Sprintf("\n💹 价格范围：%.6f - %.6f USDT", priceInUSDTLower, priceInUSDTUpper)
		} else {
			priceRangeInfo = "\n💹 价格范围：计算错误"
		}
	} else if token1Upper == "USDT" {
		// token1 是 USDT，price = USDT/token0，直接使用
		log.Printf("[TaskView] Task #%d: token1=USDT, direct price range: %.6f - %.6f", task.ID, priceLower, priceUpper)
		priceRangeInfo = fmt.Sprintf("\n💹 价格范围：%.6f - %.6f USDT", priceLower, priceUpper)
	} else {
		// 都不是 USDT，显示原始 tick 价格
		priceRangeInfo = fmt.Sprintf("\n💹 价格范围：%.6f - %.6f", priceLower, priceUpper)
	}

	return fmt.Sprintf(`%s *任务 #%d* (%s)

🏦 交易所：%s
💱 交易对：%s
🔗 池子：`+"`%s`"+`%s

📊 Tick 范围：%d → %d%s
💰 金额：%.2f USDT

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
		task.Exchange,
		pair,
		shortenHex(task.PoolId),
		positionInfo,
		task.TickLower,
		task.TickUpper,
		priceRangeInfo,
		task.AmountUSDT,
		task.ReopenDelaySeconds,
		task.SlippageTolerance,
		boolToOnOff(task.StopLossEnabled),
		task.StopLossDelaySeconds,
		boolToOnOff(task.AutoReinvest),
		task.ResidualTolerance,
	)
}

func (b *Bot) taskKeyboard(task *models.StrategyTask) tgbotapi.InlineKeyboardMarkup {
	idStr := fmt.Sprintf("%d", task.ID)

	stopText := "🛑 停止任务"
	if task.Status == models.StrategyStatusStopped {
		stopText = "✅ 已停止"
	} else if task.Status == models.StrategyStatusStopping {
		stopText = "⏳ 停止中"
	}

	stopLossText := fmt.Sprintf("⚡ 秒止损：%s", boolToOnOff(task.StopLossEnabled))
	reinvestText := fmt.Sprintf("🔁 复投：%s", boolToOnOff(task.AutoReinvest))
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(stopText, "task_stop_"+idStr),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(stopLossText, "task_toggle_stoploss_"+idStr),
			tgbotapi.NewInlineKeyboardButtonData(reinvestText, "task_toggle_reinvest_"+idStr),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("⏱️ 再平衡超时 (%ds)", task.ReopenDelaySeconds), "task_set_rebalance_"+idStr),
		),
	)
}
