package bot

import (
	"TgLpBot/blockchain"
	"TgLpBot/config"
	"TgLpBot/models"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// tickToPrice 将 tick 转换为价格
// 价格 = 1.0001 ^ tick
func tickToPrice(tick int) float64 {
	return math.Pow(1.0001, float64(tick))
}

var stableSymbols = map[string]struct{}{
	"USDT": {},
	"USDC": {},
	"BUSD": {},
	"DAI":  {},
}

func isStableSymbol(symbol string) bool {
	_, ok := stableSymbols[strings.ToUpper(strings.TrimSpace(symbol))]
	return ok
}

func formatPriceValue(price float64) string {
	if math.IsNaN(price) || math.IsInf(price, 0) {
		return "--"
	}
	abs := math.Abs(price)
	switch {
	case abs >= 1000:
		return fmt.Sprintf("%.2f", price)
	case abs >= 100:
		return fmt.Sprintf("%.3f", price)
	case abs >= 1:
		return fmt.Sprintf("%.4f", price)
	case abs >= 0.01:
		return fmt.Sprintf("%.6f", price)
	default:
		return fmt.Sprintf("%.8f", price)
	}
}

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

func formatCurrentPriceInfo(task *models.StrategyTask) string {
	if task == nil {
		return "💵 当前价格：--"
	}

	currentTick, err := getCurrentTickForTask(task)
	if err != nil {
		log.Printf("[TaskView] Current tick query failed for task #%d: %v", task.ID, err)
		return "💵 当前价格：--"
	}

	price := tickToPrice(currentTick)
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return "💵 当前价格：--"
	}

	token0 := strings.TrimSpace(task.Token0Symbol)
	token1 := strings.TrimSpace(task.Token1Symbol)
	token0Upper := strings.ToUpper(token0)
	token1Upper := strings.ToUpper(token1)

	base := token0
	quote := token1
	if isStableSymbol(token0Upper) {
		if price == 0 {
			return "💵 当前价格：--"
		}
		price = 1.0 / price
		base = token1
		quote = token0
	} else if isStableSymbol(token1Upper) {
		base = token0
		quote = token1
	} else {
		return "💵 当前价格：--"
	}

	if strings.TrimSpace(base) == "" {
		base = "-"
	}
	if strings.TrimSpace(quote) == "" {
		quote = "-"
	}
	return fmt.Sprintf("💵 当前价格：1 %s ≈ %s %s", base, formatPriceValue(price), quote)
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
				dustParts = append(dustParts, fmt.Sprintf("%.4f %s", pnl.DustToken0, task.Token0Symbol))
			}
			if pnl.DustToken1 > 0 {
				dustParts = append(dustParts, fmt.Sprintf("%.4f %s", pnl.DustToken1, task.Token1Symbol))
			}
			if len(dustParts) > 0 {
				dustLine = fmt.Sprintf("\n🧹 开仓残余：%s (≈%.2f USDT)", strings.Join(dustParts, " + "), pnl.DustValueUSDT)
			}

			// 使用 InitialCostUSDT（实际投入的 USDT）与交易历史保持一致
			actualInvested := pnl.InitialCostUSDT
			if actualInvested <= 0 {
				actualInvested = task.AmountUSDT
			}

			amountLine = fmt.Sprintf(
				"📊 资产状况：\n💵 当前价值：%.2f USDT\n📈 绝对盈亏：%s%.2f USDT %s\n🎁 未领手续费：%.2f USDT%s\n💰 实际投入：%.2f USDT (预期 %.2f USDT)",
				pnl.CurrentValueUSDT,
				sign, pnl.AbsolutePnLUSDT, emojiStr,
				pnl.UnclaimedFeesUSDT,
				dustLine,
				actualInvested,
				task.AmountUSDT,
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

	// 计算价格范围（始终显示非 USDT 币种以 USDT 计价）
	// tick 表示 token1/token0 的价格
	priceLower := tickToPrice(task.TickLower)
	priceUpper := tickToPrice(task.TickUpper)

	// 判断哪个是 USDT
	var priceRangeInfo string
	token0Upper := strings.ToUpper(strings.TrimSpace(task.Token0Symbol))
	token1Upper := strings.ToUpper(strings.TrimSpace(task.Token1Symbol))
	quoteSymbol := "USDT"
	if isStableSymbol(token0Upper) {
		quoteSymbol = token0Upper
	} else if isStableSymbol(token1Upper) {
		quoteSymbol = token1Upper
	}

	log.Printf("[TaskView] Task #%d: token0=%s token1=%s priceLower=%.6f priceUpper=%.6f",
		task.ID, token0Upper, token1Upper, priceLower, priceUpper)

	if math.IsNaN(priceLower) || math.IsInf(priceLower, 0) || math.IsNaN(priceUpper) || math.IsInf(priceUpper, 0) {
		priceRangeInfo = "💹 价格范围：--"
	} else if isStableSymbol(token0Upper) {
		// token0 是 USDT，price = token1/USDT，需要取倒数
		if priceLower > 0 && priceUpper > 0 {
			priceInUSDTLower := 1.0 / priceUpper
			priceInUSDTUpper := 1.0 / priceLower
			if priceInUSDTLower > priceInUSDTUpper {
				priceInUSDTLower, priceInUSDTUpper = priceInUSDTUpper, priceInUSDTLower
			}
			log.Printf("[TaskView] Task #%d: token0=USDT, inverted price range: %.6f - %.6f", task.ID, priceInUSDTLower, priceInUSDTUpper)
			priceRangeInfo = fmt.Sprintf("💹 价格范围：%s - %s %s", formatPriceValue(priceInUSDTLower), formatPriceValue(priceInUSDTUpper), quoteSymbol)
		} else {
			priceRangeInfo = "💹 价格范围：计算错误"
		}
	} else if isStableSymbol(token1Upper) {
		// token1 是 USDT，price = USDT/token0，直接使用
		if priceLower > priceUpper {
			priceLower, priceUpper = priceUpper, priceLower
		}
		log.Printf("[TaskView] Task #%d: token1=USDT, direct price range: %.6f - %.6f", task.ID, priceLower, priceUpper)
		priceRangeInfo = fmt.Sprintf("💹 价格范围：%s - %s %s", formatPriceValue(priceLower), formatPriceValue(priceUpper), quoteSymbol)
	} else {
		// 都不是稳定币，避免误导
		priceRangeInfo = "💹 价格范围：--"
	}

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
		task.Exchange,
		pair,
		shortenHex(task.PoolId),
		positionInfo,
		formatCurrentPriceInfo(task),
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
