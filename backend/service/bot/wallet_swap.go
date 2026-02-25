package bot

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const sessionWalletSwapChain = "wallet_swap_chain"

func walletSwapChainKeyboard(chains []string) any {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, 4)
	cur := make([]tgbotapi.InlineKeyboardButton, 0, 2)

	for _, c := range chains {
		ch := config.NormalizeChain(c)
		if ch == "" {
			continue
		}
		cur = append(cur, tgbotapi.NewInlineKeyboardButtonData(chainLabel(ch), "wallet_swap_chain_"+ch))
		if len(cur) >= 2 {
			rows = append(rows, cur)
			cur = make([]tgbotapi.InlineKeyboardButton, 0, 2)
		}
	}
	if len(cur) > 0 {
		rows = append(rows, cur)
	}
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("取消", "wallet_swap_to_usdt_cancel"),
	})
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func walletSwapConfirmKeyboard(chain string) any {
	chain = config.NormalizeChain(chain)
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("确认", "wallet_swap_to_usdt_confirm_"+chain),
			tgbotapi.NewInlineKeyboardButtonData("取消", "wallet_swap_to_usdt_cancel"),
		),
	)
}

// handleSwapToUSDT handles the /swap command (swap wallet tokens to USDT).
func (b *Bot) handleSwapToUSDT(message *tgbotapi.Message, user *models.User) {
	if message == nil || message.Chat == nil {
		return
	}

	if !b.checkUserAuthorized(message.Chat.ID, user) {
		return
	}

	if _, err := b.walletService.GetDefaultWallet(user.ID); err != nil {
		b.sendMessage(message.Chat.ID, "请先使用 /wallet 导入钱包，然后再执行零钱兑换。")
		return
	}

	if cfg, err := b.configService.GetOrCreate(user.ID); err == nil && cfg != nil && !cfg.MultiChainEnabled {
		chain := config.PickEnabledChain(cfg.DefaultChain)
		_ = database.SetUserSession(user.TelegramID, sessionWalletSwapChain, chain, 30*time.Minute)
		b.sendWalletSwapPreview(message.Chat.ID, user, chain)
		return
	}

	chains := enabledChains()
	if len(chains) > 1 {
		b.sendMessageWithKeyboard(message.Chat.ID, "*零钱兑换*\n\n请选择链：", walletSwapChainKeyboard(chains))
		return
	}

	chain := config.NormalizeChain(chains[0])
	_ = database.SetUserSession(user.TelegramID, sessionWalletSwapChain, chain, 30*time.Minute)
	b.sendWalletSwapPreview(message.Chat.ID, user, chain)
}

func (b *Bot) handleWalletSwapToUSDTPrompt(query *tgbotapi.CallbackQuery, user *models.User) {
	_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if query == nil || query.Message == nil || query.Message.Chat == nil {
		return
	}

	if !b.checkUserAuthorized(query.Message.Chat.ID, user) {
		return
	}

	if _, err := b.walletService.GetDefaultWallet(user.ID); err != nil {
		b.sendMessage(query.Message.Chat.ID, "请先使用 /wallet 导入钱包，然后再执行零钱兑换。")
		return
	}

	if cfg, err := b.configService.GetOrCreate(user.ID); err == nil && cfg != nil && !cfg.MultiChainEnabled {
		chain := config.PickEnabledChain(cfg.DefaultChain)
		_ = database.SetUserSession(user.TelegramID, sessionWalletSwapChain, chain, 30*time.Minute)
		b.sendWalletSwapPreview(query.Message.Chat.ID, user, chain)
		return
	}

	chains := enabledChains()
	if len(chains) > 1 {
		b.sendMessageWithKeyboard(query.Message.Chat.ID, "*零钱兑换*\n\n请选择链：", walletSwapChainKeyboard(chains))
		return
	}

	chain := config.NormalizeChain(chains[0])
	_ = database.SetUserSession(user.TelegramID, sessionWalletSwapChain, chain, 30*time.Minute)
	b.sendWalletSwapPreview(query.Message.Chat.ID, user, chain)
}

func (b *Bot) handleWalletSwapChainSelect(query *tgbotapi.CallbackQuery, user *models.User) {
	_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if query == nil || query.Message == nil || query.Message.Chat == nil {
		return
	}

	if !b.checkUserAuthorized(query.Message.Chat.ID, user) {
		return
	}

	chain := config.NormalizeChain(strings.TrimPrefix(strings.TrimSpace(query.Data), "wallet_swap_chain_"))
	if chain == "" {
		b.sendMessage(query.Message.Chat.ID, "无效的链。")
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
		b.sendMessage(query.Message.Chat.ID, "当前未启用该链，请检查 CHAINS 配置。")
		return
	}

	_ = database.SetUserSession(user.TelegramID, sessionWalletSwapChain, chain, 30*time.Minute)
	b.sendWalletSwapPreview(query.Message.Chat.ID, user, chain)
}

func (b *Bot) handleWalletSwapToUSDTConfirm(query *tgbotapi.CallbackQuery, user *models.User) {
	_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, "正在兑换..."))
	if query == nil || query.Message == nil || query.Message.Chat == nil {
		return
	}

	if !b.checkUserAuthorized(query.Message.Chat.ID, user) {
		return
	}

	if _, err := b.walletService.GetDefaultWallet(user.ID); err != nil {
		b.sendMessage(query.Message.Chat.ID, "请先使用 /wallet 导入钱包，然后再执行零钱兑换。")
		return
	}

	chain := ""
	if strings.HasPrefix(query.Data, "wallet_swap_to_usdt_confirm_") {
		chain = strings.TrimPrefix(query.Data, "wallet_swap_to_usdt_confirm_")
	}
	chain = config.NormalizeChain(chain)
	if chain == "" {
		if s, err := database.GetUserSession(user.TelegramID, sessionWalletSwapChain); err == nil {
			chain = config.NormalizeChain(s)
		}
	}
	if chain == "" {
		chains := enabledChains()
		if len(chains) > 0 {
			chain = config.NormalizeChain(chains[0])
		}
	}
	if chain == "" {
		chain = "bsc"
	}

	cfg, _ := b.configService.GetOrCreate(user.ID)
	slippage := 0.5
	if cfg != nil && cfg.SlippageTolerance > 0 {
		slippage = cfg.SlippageTolerance
	}

	loadingMsg, _ := b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("⏳ 正在执行零钱兑换 (%s)...\nslippage=%.2f%%", chainLabel(chain), slippage))
	defer func() {
		if loadingMsg.MessageID != 0 {
			_, _ = b.api.Send(tgbotapi.NewDeleteMessage(loadingMsg.Chat.ID, loadingMsg.MessageID))
		}
	}()

	report, err := b.liquidityService.SwapWalletOtherTokensToUSDTForChain(user.ID, chain, slippage)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 零钱兑换失败: %v", err))
		return
	}
	if report == nil {
		b.sendMessage(query.Message.Chat.ID, "❌ 零钱兑换失败: empty result")
		return
	}

	if len(report.Swapped) == 0 && len(report.Failed) == 0 {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("✅ 未发现需要兑换的代币 (%s)。", chainLabel(chain)))
		return
	}

	// Keep message size bounded (Telegram limit is 4096 chars).
	const maxLines = 30
	trimList := func(in []string) ([]string, int) {
		if len(in) <= maxLines {
			return in, 0
		}
		return in[:maxLines], len(in) - maxLines
	}

	swapped, swappedMore := trimList(report.Swapped)
	failed, failedMore := trimList(report.Failed)

	text := fmt.Sprintf("🪙 *零钱兑换结果* (%s)\n\n候选代币数: %d\n成功: %d\n失败: %d\n",
		chainLabel(chain), report.CandidateCnt, len(report.Swapped), len(report.Failed))

	if len(swapped) > 0 {
		text += "\n✅ *成功交易:*\n"
		for i, item := range swapped {
			parts := strings.Split(item, "|")
			if len(parts) == 2 {
				desc := strings.TrimSpace(parts[0])
				txHash := strings.TrimSpace(parts[1])
				link := explorerTxURL(chain, txHash)
				if link != "" {
					text += fmt.Sprintf("%d. %s\n   [查看交易](%s)\n", i+1, desc, link)
				} else {
					text += fmt.Sprintf("%d. %s | %s\n", i+1, desc, txHash)
				}
			} else {
				text += fmt.Sprintf("%d. %s\n", i+1, item)
			}
		}
		if swappedMore > 0 {
			text += fmt.Sprintf("... 还有 %d 条\n", swappedMore)
		}
	}

	if len(failed) > 0 {
		text += "\n❌ *失败:*\n"
		for i, item := range failed {
			text += fmt.Sprintf("%d. %s\n", i+1, item)
		}
		if failedMore > 0 {
			text += fmt.Sprintf("... 还有 %d 条\n", failedMore)
		}
	}

	msg := tgbotapi.NewMessage(query.Message.Chat.ID, text)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true
	if _, sendErr := b.api.Send(msg); sendErr != nil {
		log.Printf("[Bot] send swap report failed: %v", sendErr)
	}
}

func (b *Bot) handleWalletSwapToUSDTCancel(query *tgbotapi.CallbackQuery, user *models.User) {
	_, _ = b.api.Send(tgbotapi.NewCallback(query.ID, "已取消"))
	if query == nil || query.Message == nil || query.Message.Chat == nil {
		return
	}
	_ = database.DeleteUserSession(user.TelegramID, sessionWalletSwapChain)
	b.sendMessage(query.Message.Chat.ID, "已取消零钱兑换。")
}

func (b *Bot) sendWalletSwapPreview(chatID int64, user *models.User, chain string) {
	chain = config.NormalizeChain(chain)
	if chain == "" {
		b.sendMessage(chatID, "无效的链。")
		return
	}

	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok {
		b.sendMessage(chatID, fmt.Sprintf("链配置缺失：%s", chain))
		return
	}
	stableSymbol := strings.TrimSpace(cc.StableSymbol)
	if stableSymbol == "" {
		stableSymbol = "USDT"
	}

	cfg, _ := b.configService.GetOrCreate(user.ID)
	slippage := 0.5
	if cfg != nil && cfg.SlippageTolerance > 0 {
		slippage = cfg.SlippageTolerance
	}

	minValueUSDT := 1.0
	loadingMsg, _ := b.sendMessage(chatID, fmt.Sprintf("⏳ 正在扫描可兑换代币 (%s, >= %.2f %s)...", chainLabel(chain), minValueUSDT, stableSymbol))
	if loadingMsg.MessageID != 0 {
		defer func() {
			_, _ = b.api.Send(tgbotapi.NewDeleteMessage(loadingMsg.Chat.ID, loadingMsg.MessageID))
		}()
	}

	tokens, err := b.liquidityService.ScanWalletTokensForSwapForChain(user.ID, chain, minValueUSDT)
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 扫描失败: %v", err))
		return
	}
	if len(tokens) == 0 {
		b.sendMessage(chatID, fmt.Sprintf("✅ 未发现需要兑换的代币 (%s)。", chainLabel(chain)))
		return
	}

	const maxPreview = 20
	n := len(tokens)
	if n > maxPreview {
		n = maxPreview
	}

	text := fmt.Sprintf("🪙 *零钱兑换预览* (%s)\n\n将把以下代币兑换为 %s。\nslippage=%.2f%%\n\n",
		chainLabel(chain), stableSymbol, slippage)
	for i := 0; i < n; i++ {
		t := tokens[i]
		text += fmt.Sprintf("%d. %s: %s (~%.2f %s)\n", i+1, strings.TrimSpace(t.Symbol), strings.TrimSpace(t.Balance), t.ValueUSDT, stableSymbol)
	}
	if len(tokens) > maxPreview {
		text += fmt.Sprintf("... 还有 %d 个代币未展示\n", len(tokens)-maxPreview)
	}

	b.sendMessageWithKeyboard(chatID, text+"\n确认执行？", walletSwapConfirmKeyboard(chain))
}
