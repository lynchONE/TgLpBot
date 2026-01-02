package bot

import (
	"TgLpBot/config"
	"TgLpBot/models"
	"TgLpBot/services"
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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

	ranks, err := b.smartLPService.GetTopAddedLiquidityPools(ctx, chain, time.Hour, 3)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 获取 Smart Money 数据失败：%v", err))
		return
	}
	if len(ranks) == 0 {
		b.sendMessage(message.Chat.ID, "最近 1 小时内暂无加 LP 记录。")
		return
	}

	text := "🧠 *Smart Money*（最近 1 小时加 LP 排名，按参与钱包数量排序）\n\n"
	for i, rank := range ranks {
		poolID := strings.TrimSpace(rank.PoolID)
		if poolID == "" {
			poolID = "-"
		}

		pair := "-"
		feeText := "-"
		if poolID != "-" {
			if info, err := b.lookupSmartMoneyPoolInfo(rank.PoolVersion, poolID); err == nil && info != nil {
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

		text += fmt.Sprintf("%d. 池子地址：\n```\n%s\n```\n", i+1, poolID)
		text += fmt.Sprintf("💱 交易对：%s\n", pair)
		text += fmt.Sprintf("💵 费率：%s\n", feeText)
		text += fmt.Sprintf("👛 参与钱包：%d\n\n", rank.WalletCount)
	}

	b.sendMessage(message.Chat.ID, text)
}

func (b *Bot) lookupSmartMoneyPoolInfo(poolVersion, poolID string) (*services.PoolInfo, error) {
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
