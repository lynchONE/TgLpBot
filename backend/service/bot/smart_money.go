package bot

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"TgLpBot/service/smart_lp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type smartMoneyPoolCtx struct {
	PoolVersion    string  `json:"pool_version"`
	PoolID         string  `json:"pool_id"`
	AddedLiquidity float64 `json:"added_liquidity"`
	WalletCount    int     `json:"wallet_count"`
}

type smartMoneyMessageCtx struct {
	Chain          string              `json:"chain"`
	LookbackSec    int                 `json:"lookback_sec"`
	GeneratedAtSec int64               `json:"generated_at_sec"`
	Pools          []smartMoneyPoolCtx `json:"pools"`
}

const (
	smartMoneyDefaultLookback  = time.Hour
	smartMoneyDefaultPoolLimit = 3
	smartMoneyWalletsPerPage   = 5
	smartMoneyMaxEventsQuery   = 2000
	smartMoneyTelegramMaxChars = 3800
)

func (b *Bot) handleSmartMoney(message *tgbotapi.Message, user *models.User) {
	if config.AppConfig == nil || !config.AppConfig.SmartLPEnabled {
		b.sendMessage(message.Chat.ID, "SmartLP 未启用，无法查询 Smart Money 排名。")
		return
	}
	if b.smartLPService == nil {
		b.sendMessage(message.Chat.ID, "SmartLP 服务未初始化，无法查询 Smart Money 排名。")
		return
	}

	b.sendMessage(message.Chat.ID, "⏳ 正在统计最近 1 小时 Smart Money 加LP榜...")

	chain := "bsc"
	if config.AppConfig != nil {
		if v := strings.TrimSpace(config.AppConfig.AutoLPChain); v != "" {
			chain = strings.ToLower(v)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	ranks, err := b.smartLPService.GetTopAddedLiquidityPools(ctx, chain, smartMoneyDefaultLookback, smartMoneyDefaultPoolLimit)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 获取 Smart Money 数据失败：%v", err))
		return
	}
	if len(ranks) == 0 {
		b.sendMessage(message.Chat.ID, "最近 1 小时内暂无加 LP 记录。")
		return
	}

	smCtx := smartMoneyMessageCtx{
		Chain:          chain,
		LookbackSec:    int(smartMoneyDefaultLookback.Seconds()),
		GeneratedAtSec: time.Now().Unix(),
		Pools:          make([]smartMoneyPoolCtx, 0, len(ranks)),
	}
	for _, rank := range ranks {
		smCtx.Pools = append(smCtx.Pools, smartMoneyPoolCtx{
			PoolVersion:    strings.TrimSpace(rank.PoolVersion),
			PoolID:         strings.TrimSpace(rank.PoolID),
			AddedLiquidity: rank.AddedLiquidity,
			WalletCount:    rank.WalletCount,
		})
	}

	text, keyboard, _, _, renderErr := b.renderSmartMoneyMessage(context.Background(), &smCtx, -1, 0)
	if renderErr != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 渲染 Smart Money 消息失败：%v", renderErr))
		return
	}
	sent, err := b.sendMessageWithKeyboardRet(message.Chat.ID, text, keyboard)
	if err != nil {
		return
	}

	if raw, err := json.Marshal(smCtx); err == nil {
		_ = database.SetUserSession(user.TelegramID, smartMoneySessionKey(message.Chat.ID, sent.MessageID), string(raw), 2*time.Hour)
	}
}

func (b *Bot) lookupSmartMoneyPoolInfo(poolVersion, poolID string) (*pool.PoolInfo, error) {
	if b.poolService == nil {
		return nil, fmt.Errorf("pool service not initialized")
	}
	poolID = strings.TrimSpace(poolID)
	if poolID == "" {
		return nil, fmt.Errorf("pool id empty")
	}

	version := strings.ToLower(strings.TrimSpace(poolVersion))
	switch version {
	case "v4":
		return b.poolService.GetV4PoolInfo(poolID)
	case "v3":
		return b.poolService.GetPoolInfo(poolID)
	default:
		if isV4PoolId(poolID) {
			return b.poolService.GetV4PoolInfo(poolID)
		}
		return b.poolService.GetPoolInfo(poolID)
	}
}

func smartMoneySessionKey(chatID int64, messageID int) string {
	return fmt.Sprintf("smartmoney_ctx_%d_%d", chatID, messageID)
}

type smartMoneyWalletSummary struct {
	WalletAddress string
	Events        []smart_lp.SmartLPEvent
	TotalValue    float64
	ValueSymbol   string
	LastBlock     uint64
}

func (b *Bot) renderSmartMoneyMessage(ctx context.Context, smCtx *smartMoneyMessageCtx, expandedPoolIdx int, walletPage int) (string, any, int, int, error) {
	if smCtx == nil {
		return "", nil, 0, 0, fmt.Errorf("smart money ctx is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if expandedPoolIdx < -1 {
		expandedPoolIdx = -1
	}

	lookback := time.Duration(smCtx.LookbackSec) * time.Second
	if lookback <= 0 {
		lookback = smartMoneyDefaultLookback
	}

	text := fmt.Sprintf("🧠 *Smart Money*（最近 %s 加 LP，按参与钱包数量排序）\n\n", lookback.Truncate(time.Minute))
	for i, pool := range smCtx.Pools {
		poolID := strings.TrimSpace(pool.PoolID)
		if poolID == "" {
			poolID = "-"
		}

		pair := "-"
		feeText := "-"
		if poolID != "-" {
			if info, err := b.lookupSmartMoneyPoolInfo(pool.PoolVersion, poolID); err == nil && info != nil {
				t0 := strings.TrimSpace(info.Token0Symbol)
				t1 := strings.TrimSpace(info.Token1Symbol)
				if t0 != "" || t1 != "" {
					pair = strings.TrimSpace(t0 + "/" + t1)
					if pair == "/" {
						pair = "-"
					} else {
						pair = escapeTelegramMarkdown(pair)
					}
				}
				feeText = fmt.Sprintf("%.4f%%", float64(info.Fee)/10000)
			}
		}

		prefix := " "
		if expandedPoolIdx == i {
			prefix = "👉"
		}

		text += fmt.Sprintf("%s *%d.* 池子：\n```\n%s\n```\n", prefix, i+1, poolID)
		text += fmt.Sprintf("💱 交易对：%s\n", pair)
		text += fmt.Sprintf("💵 费率：%s\n", feeText)
		text += fmt.Sprintf("👛 参与钱包：%d\n\n", pool.WalletCount)
	}

	totalWallets := 0
	totalPages := 0
	if expandedPoolIdx >= 0 && expandedPoolIdx < len(smCtx.Pools) {
		pool := smCtx.Pools[expandedPoolIdx]
		poolID := strings.TrimSpace(pool.PoolID)
		if poolID == "" {
			return text, b.smartMoneyKeyboard(smCtx, expandedPoolIdx, walletPage, 0), 0, 0, nil
		}

		info, err := b.lookupSmartMoneyPoolInfo(pool.PoolVersion, poolID)
		if err != nil || info == nil {
			text += "⚠️ 读取池子信息失败，无法展示钱包详情。\n"
			return text, b.smartMoneyKeyboard(smCtx, expandedPoolIdx, walletPage, 0), 0, 0, nil
		}

		queryCtx, cancel := context.WithTimeout(ctx, 18*time.Second)
		defer cancel()
		events, err := b.smartLPService.GetPoolAddEvents(queryCtx, smCtx.Chain, lookback, pool.PoolVersion, poolID, smartMoneyMaxEventsQuery)
		if err != nil {
			text += fmt.Sprintf("⚠️ 获取钱包参与详情失败：%v\n", err)
			return text, b.smartMoneyKeyboard(smCtx, expandedPoolIdx, walletPage, 0), 0, 0, nil
		}

		walletMap := make(map[string][]smart_lp.SmartLPEvent)
		for _, ev := range events {
			addr := strings.ToLower(strings.TrimSpace(ev.WalletAddress))
			if addr == "" {
				continue
			}
			walletMap[addr] = append(walletMap[addr], ev)
		}

		wallets := make([]smartMoneyWalletSummary, 0, len(walletMap))
		decimalsCache := make(map[string]int, 2)
		tickCache := make(map[uint64]int)

		tmpTask := &models.StrategyTask{
			PoolId:        poolID,
			PoolVersion:   strings.ToLower(strings.TrimSpace(pool.PoolVersion)),
			Token0Symbol:  strings.TrimSpace(info.Token0Symbol),
			Token1Symbol:  strings.TrimSpace(info.Token1Symbol),
			Token0Address: strings.TrimSpace(info.Token0),
			Token1Address: strings.TrimSpace(info.Token1),
		}

		for addr, evs := range walletMap {
			sortEventsInPlace(evs)
			sum := smartMoneyWalletSummary{
				WalletAddress: addr,
				Events:        evs,
				TotalValue:    0,
				ValueSymbol:   "",
				LastBlock:     0,
			}
			if len(evs) > 0 {
				sum.LastBlock = evs[0].BlockNumber
			}

			for _, ev := range evs {
				tick, ok := b.smartMoneyTickAtBlock(tmpTask, ev.BlockNumber, tickCache)
				if !ok {
					continue
				}
				val, sym, okVal := smartMoneyEstimateStableValue(tmpTask, &ev, tick, decimalsCache)
				if okVal {
					sum.TotalValue += val
					if sum.ValueSymbol == "" {
						sum.ValueSymbol = sym
					}
				}
			}
			wallets = append(wallets, sum)
		}

		sortWalletsInPlace(wallets)

		totalWallets = len(wallets)
		if totalWallets > 0 {
			totalPages = int(math.Ceil(float64(totalWallets) / float64(smartMoneyWalletsPerPage)))
		}
		if walletPage < 0 {
			walletPage = 0
		}
		if totalPages > 0 && walletPage >= totalPages {
			walletPage = totalPages - 1
		}

		start := walletPage * smartMoneyWalletsPerPage
		end := start + smartMoneyWalletsPerPage
		if start < 0 {
			start = 0
		}
		if end > totalWallets {
			end = totalWallets
		}

		text += fmt.Sprintf("🔍 *钱包详情*（池子 #%d｜第 %d/%d 页）\n", expandedPoolIdx+1, walletPage+1, maxInt(1, totalPages))
		if totalWallets == 0 {
			text += "暂无钱包参与记录。\n"
		} else {
			text += fmt.Sprintf("展示 %d-%d / %d 个钱包（最近 %s）\n\n", start+1, end, totalWallets, lookback.Truncate(time.Minute))
		}

		for _, w := range wallets[start:end] {
			displayValue := ""
			if w.TotalValue > 0 && w.ValueSymbol != "" {
				displayValue = fmt.Sprintf("｜合计≈ %.2f %s", w.TotalValue, escapeTelegramMarkdown(w.ValueSymbol))
			}
			text += fmt.Sprintf("👛 `%s`（%d 次）%s\n", w.WalletAddress, len(w.Events), displayValue)

			for _, ev := range w.Events {
				line := b.smartMoneyFormatEventLine(tmpTask, &ev, decimalsCache, tickCache)
				if line == "" {
					continue
				}
				if len(text)+len(line)+16 >= smartMoneyTelegramMaxChars {
					text += "…（消息过长已截断，可用翻页/收起重新展开查看）\n"
					goto doneWallets
				}
				text += "• " + line + "\n"
			}
			text += "\n"
		}
	doneWallets:
	}

	keyboard := b.smartMoneyKeyboard(smCtx, expandedPoolIdx, walletPage, totalPages)
	return text, keyboard, totalWallets, totalPages, nil
}

func (b *Bot) smartMoneyKeyboard(smCtx *smartMoneyMessageCtx, expandedPoolIdx int, walletPage int, totalPages int) any {
	if smCtx == nil || len(smCtx.Pools) == 0 {
		return tgbotapi.NewInlineKeyboardMarkup()
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, 3)

	// Pool selector row
	poolButtons := make([]tgbotapi.InlineKeyboardButton, 0, len(smCtx.Pools))
	for i := range smCtx.Pools {
		label := fmt.Sprintf("#%d", i+1)
		if expandedPoolIdx == i {
			label = "✅" + label
		}
		poolButtons = append(poolButtons, tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("smartmoney_show_%d_0", i)))
	}
	rows = append(rows, poolButtons)

	// Paging row (only when expanded)
	if expandedPoolIdx >= 0 && totalPages > 1 {
		btns := make([]tgbotapi.InlineKeyboardButton, 0, 2)
		if walletPage > 0 {
			btns = append(btns, tgbotapi.NewInlineKeyboardButtonData("⬅️ 上一页", fmt.Sprintf("smartmoney_show_%d_%d", expandedPoolIdx, walletPage-1)))
		}
		if walletPage+1 < totalPages {
			btns = append(btns, tgbotapi.NewInlineKeyboardButtonData("➡️ 下一页", fmt.Sprintf("smartmoney_show_%d_%d", expandedPoolIdx, walletPage+1)))
		}
		if len(btns) > 0 {
			rows = append(rows, btns)
		}
	}

	// Collapse row
	if expandedPoolIdx >= 0 {
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("收起", "smartmoney_hide"),
		})
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func sortEventsInPlace(evs []smart_lp.SmartLPEvent) {
	if len(evs) < 2 {
		return
	}
	sort.Slice(evs, func(i, j int) bool {
		if evs[i].BlockNumber != evs[j].BlockNumber {
			return evs[i].BlockNumber > evs[j].BlockNumber
		}
		return evs[i].LogIndex > evs[j].LogIndex
	})
}

func sortWalletsInPlace(ws []smartMoneyWalletSummary) {
	if len(ws) < 2 {
		return
	}
	sort.Slice(ws, func(i, j int) bool {
		if ws[i].TotalValue != ws[j].TotalValue {
			return ws[i].TotalValue > ws[j].TotalValue
		}
		if len(ws[i].Events) != len(ws[j].Events) {
			return len(ws[i].Events) > len(ws[j].Events)
		}
		return ws[i].LastBlock > ws[j].LastBlock
	})
}

func (b *Bot) smartMoneyTickAtBlock(task *models.StrategyTask, blockNumber uint64, cache map[uint64]int) (int, bool) {
	if task == nil || blockNumber == 0 {
		return 0, false
	}
	if cache != nil {
		if v, ok := cache[blockNumber]; ok {
			return v, true
		}
	}

	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	var tick int
	var err error
	switch version {
	case "v4":
		if config.AppConfig == nil || !common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) || !common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) {
			return 0, false
		}
		poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
		stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
		tick, err = blockchain.GetUniswapV4PoolCurrentTickViaStateViewAtBlock(stateView, poolManager, task.PoolId, blockNumber)
	default:
		if !common.IsHexAddress(task.PoolId) {
			return 0, false
		}
		tick, err = blockchain.GetV3PoolCurrentTickAtBlock(common.HexToAddress(task.PoolId), blockNumber)
	}
	if err != nil {
		return 0, false
	}
	if cache != nil {
		cache[blockNumber] = tick
	}
	return tick, true
}

func smartMoneyEstimateStableValue(task *models.StrategyTask, ev *smart_lp.SmartLPEvent, currentTick int, decimalsCache map[string]int) (float64, string, bool) {
	if task == nil || ev == nil {
		return 0, "", false
	}

	dec0 := smartMoneyGetDecimals(task.Token0Address, decimalsCache)
	dec1 := smartMoneyGetDecimals(task.Token1Address, decimalsCache)
	amt0 := smartMoneyAmountToFloat(ev.Amount0, dec0)
	amt1 := smartMoneyAmountToFloat(ev.Amount1, dec1)

	price, _, quoteSym, okPrice := pricing.BuildPriceDisplay(task, currentTick)
	if !okPrice || price <= 0 {
		return 0, "", false
	}

	side := pricing.StableSideFromTask(task)
	switch side {
	case 0:
		// token0 is stable quote (e.g., USDT)
		return amt0 + amt1*price, strings.TrimSpace(task.Token0Symbol), true
	case 1:
		// token1 is stable quote
		return amt1 + amt0*price, strings.TrimSpace(task.Token1Symbol), true
	default:
		// Unknown stable side; fall back to "quote token" value.
		return amt1 + amt0*price, strings.TrimSpace(quoteSym), true
	}
}

func smartMoneyGetDecimals(addr string, cache map[string]int) int {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" || !common.IsHexAddress(addr) {
		return 18
	}
	if cache != nil {
		if v, ok := cache[addr]; ok {
			return v
		}
	}
	dec, err := blockchain.GetTokenDecimals(common.HexToAddress(addr))
	if err != nil || dec == 0 {
		if cache != nil {
			cache[addr] = 18
		}
		return 18
	}
	if cache != nil {
		cache[addr] = int(dec)
	}
	return int(dec)
}

func smartMoneyAmountToFloat(amountStr string, decimals int) float64 {
	amountStr = strings.TrimSpace(amountStr)
	if amountStr == "" {
		return 0
	}
	i, ok := new(big.Int).SetString(amountStr, 10)
	if !ok || i.Sign() == 0 {
		return 0
	}
	if decimals < 0 {
		decimals = 0
	}

	f := new(big.Float).SetInt(i)
	div := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	f.Quo(f, div)
	v, _ := f.Float64()
	return v
}

func (b *Bot) smartMoneyFormatEventLine(task *models.StrategyTask, ev *smart_lp.SmartLPEvent, decimalsCache map[string]int, tickCache map[uint64]int) string {
	if task == nil || ev == nil {
		return ""
	}
	if ev.TickLower == 0 && ev.TickUpper == 0 {
		return ""
	}

	tick, ok := b.smartMoneyTickAtBlock(task, ev.BlockNumber, tickCache)
	if !ok {
		return ""
	}

	tc := pool.NewTickCalculator()
	lowerTickPct, upperTickPct := tc.CalculatePercentagesFromTicks(tick, ev.TickLower, ev.TickUpper)
	lowerStablePct, upperStablePct := pricing.StablePercentagesFromTickPercentages(task, lowerTickPct, upperTickPct)
	if lowerStablePct <= 0 || upperStablePct <= 0 {
		lowerStablePct = lowerTickPct
		upperStablePct = upperTickPct
	}

	value, sym, okVal := smartMoneyEstimateStableValue(task, ev, tick, decimalsCache)
	amountText := "-"
	if okVal && value > 0 && strings.TrimSpace(sym) != "" {
		amountText = fmt.Sprintf("≈%.2f %s", value, escapeTelegramMarkdown(sym))
	}

	rangeText := fmt.Sprintf("L %.2f%% / U %.2f%%", lowerStablePct, upperStablePct)

	txHash := strings.TrimSpace(ev.TxHash)
	if txHash == "" {
		txHash = "-"
	} else if !strings.HasPrefix(txHash, "0x") && !strings.HasPrefix(txHash, "0X") {
		txHash = "0x" + txHash
	}

	return fmt.Sprintf("%s ｜ %s ｜ tx `%s` ｜ block %d", amountText, rangeText, txHash, ev.BlockNumber)
}
