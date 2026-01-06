package bot

import (
	"TgLpBot/base/models"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleSwapToUSDT(message *tgbotapi.Message, user *models.User) {
	if message == nil || message.Chat == nil {
		return
	}
	if !b.checkUserAuthorized(message.Chat.ID, user) {
		return
	}
	b.promptWalletSwapToUSDT(message.Chat.ID, user)
}

func (b *Bot) handleWalletSwapToUSDTPrompt(query *tgbotapi.CallbackQuery, user *models.User) {
	_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if query == nil || query.Message == nil || query.Message.Chat == nil {
		return
	}
	if !b.checkUserAuthorized(query.Message.Chat.ID, user) {
		return
	}
	b.promptWalletSwapToUSDT(query.Message.Chat.ID, user)
}

func (b *Bot) promptWalletSwapToUSDT(chatID int64, user *models.User) {
	wallet, err := b.walletService.GetDefaultWallet(user.ID)
	if err != nil {
		b.sendMessage(chatID, "您还没有默认钱包。请先使用 /wallet 导入钱包并设置默认钱包。")
		return
	}

	slippage := 0.5
	if b.configService != nil {
		if cfg, err := b.configService.GetOrCreate(user.ID); err == nil && cfg != nil && cfg.SlippageTolerance > 0 {
			slippage = cfg.SlippageTolerance
		}
	}

	text := fmt.Sprintf(`🔄 *一键兑换为 USDT*

将使用默认钱包：
`+"`%s`"+`

把钱包里识别到的「非 BNB/WBNB、非稳定币」代币尽可能兑换成 USDT。

- 稳定币：USDT/USDC/BUSD
- 不会动用 BNB/WBNB（Gas 资产）
- 识别范围：历史仓位涉及的代币（token0/token1）
- 会产生链上 Gas 费

当前滑点：%.2f%%

是否继续？`, wallet.Address, slippage)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ 确认兑换", "wallet_swap_to_usdt_confirm"),
			tgbotapi.NewInlineKeyboardButtonData("❌ 取消", "wallet_swap_to_usdt_cancel"),
		),
	)
	b.sendMessageWithKeyboard(chatID, text, keyboard)
}

func (b *Bot) handleWalletSwapToUSDTConfirm(query *tgbotapi.CallbackQuery, user *models.User) {
	_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, "开始兑换..."))
	if query == nil || query.Message == nil || query.Message.Chat == nil {
		return
	}
	if !b.checkUserAuthorized(query.Message.Chat.ID, user) {
		return
	}

	chatID := query.Message.Chat.ID
	userID := user.ID

	go func() {
		loadingMsg, _ := b.sendMessage(chatID, "⏳ 正在兑换，请稍候...")
		defer func() {
			if loadingMsg.MessageID != 0 {
				_, _ = b.api.Send(tgbotapi.NewDeleteMessage(chatID, loadingMsg.MessageID))
			}
		}()

		slippage := 0.5
		if b.configService != nil {
			if cfg, err := b.configService.GetOrCreate(userID); err == nil && cfg != nil && cfg.SlippageTolerance > 0 {
				slippage = cfg.SlippageTolerance
			}
		}

		report, err := b.liquidityService.SwapWalletOtherTokensToUSDT(userID, slippage)
		if err != nil {
			b.sendMessage(chatID, fmt.Sprintf("❌ 兑换失败：%v", err))
			return
		}
		if report == nil {
			b.sendMessage(chatID, "❌ 兑换失败：空返回")
			return
		}

		if len(report.Swapped) == 0 && len(report.Failed) == 0 {
			b.sendMessage(chatID, "✅ 未检测到需要兑换的代币（仅检查历史仓位涉及代币，且已排除 BNB/WBNB 与稳定币）。")
			return
		}

		var sb strings.Builder
		if len(report.Swapped) > 0 {
			sb.WriteString("✅ 已提交兑换：\n")
			for i, txInfo := range report.Swapped {
				parts := strings.Split(txInfo, "|")
				if len(parts) == 2 {
					desc := parts[0]
					txHash := parts[1]
					sb.WriteString(fmt.Sprintf("%d. **%s**\n   [查看交易](https://bscscan.com/tx/%s)\n", i+1, desc, txHash))
				} else {
					sb.WriteString(fmt.Sprintf("%d. [查看交易](https://bscscan.com/tx/%s)\n", i+1, txInfo))
				}
			}
		}

		if len(report.Failed) > 0 {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("⚠️ 部分代币兑换失败：\n")
			const maxShow = 10
			for i, fail := range report.Failed {
				if i >= maxShow {
					sb.WriteString(fmt.Sprintf("... 还有 %d 个失败未展示\n", len(report.Failed)-maxShow))
					break
				}
				parts := strings.SplitN(fail, "|", 2)
				if len(parts) == 2 {
					sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n", i+1, parts[0], parts[1]))
				} else {
					sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, fail))
				}
			}
		}

		b.sendMessage(chatID, sb.String())
	}()
}

func (b *Bot) handleWalletSwapToUSDTCancel(query *tgbotapi.CallbackQuery, user *models.User) {
	_ = user
	_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, "已取消"))
}
