package bot

import (
	"TgLpBot/database"
	"TgLpBot/models"
	"bytes"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/golang/freetype/truetype"
	chart "github.com/wcharczuk/go-chart/v2"
)

var (
	chartCJKFontOnce sync.Once
	chartCJKFont     *truetype.Font
)

func getChartCJKFont() *truetype.Font {
	chartCJKFontOnce.Do(func() {
		var candidates []string

		if p := strings.TrimSpace(os.Getenv("TGLPBOT_CHART_FONT")); p != "" {
			candidates = append(candidates, p)
		}

		// Try alongside the executable (useful for deployments that ship a font file).
		if exe, err := os.Executable(); err == nil && exe != "" {
			exeDir := filepath.Dir(exe)
			candidates = append(candidates,
				filepath.Join(exeDir, "fonts", "simhei.ttf"),
				filepath.Join(exeDir, "fonts", "SimHei.ttf"),
			)
		}

		switch runtime.GOOS {
		case "windows":
			candidates = append(candidates,
				`C:\Windows\Fonts\simhei.ttf`,
				`C:\Windows\Fonts\STXIHEI.TTF`,
			)
		case "linux":
			candidates = append(candidates,
				`/usr/share/fonts/truetype/wqy/wqy-zenhei.ttf`,
				`/usr/share/fonts/truetype/arphic/uming.ttf`,
				`/usr/share/fonts/truetype/noto/NotoSansSC-Regular.ttf`,
				`/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.otf`,
			)
		case "darwin":
			candidates = append(candidates,
				`/System/Library/Fonts/STHeiti Light.ttc`,
				`/System/Library/Fonts/PingFang.ttc`,
			)
		}

		for _, p := range candidates {
			b, err := os.ReadFile(p)
			if err != nil || len(b) == 0 {
				continue
			}
			f, err := truetype.Parse(b)
			if err != nil {
				continue
			}
			chartCJKFont = f
			return
		}
	})
	return chartCJKFont
}

func (b *Bot) handleProfit(message *tgbotapi.Message, user *models.User) {
	b.sendProfitChart(message.Chat.ID, user)
}

func (b *Bot) handleViewProfit(query *tgbotapi.CallbackQuery, user *models.User) {
	b.api.Send(tgbotapi.NewCallback(query.ID, ""))
	if query.Message == nil {
		return
	}
	b.sendProfitChart(query.Message.Chat.ID, user)
}

func (b *Bot) sendProfitChart(chatID int64, user *models.User) {
	if user == nil {
		return
	}
	if database.DB == nil {
		b.sendMessage(chatID, "数据库未初始化，无法查询余额走势。")
		return
	}

	wallet, err := b.walletService.GetDefaultWallet(user.ID)
	if err != nil {
		b.sendMessage(chatID, "您还没有任何钱包。请先使用 /wallet 创建或导入钱包。")
		return
	}

	// Ensure today's snapshot exists.
	if b.snapshotService != nil {
		_ = b.snapshotService.CaptureTodayForUser(user.ID)
	}

	now := time.Now()
	days := make([]string, 0, 7)
	labels := make([]string, 0, 7)
	for i := 6; i >= 0; i-- {
		t := now.AddDate(0, 0, -i)
		days = append(days, t.Format("2006-01-02"))
		labels = append(labels, t.Format("01-02"))
	}

	var snaps []models.WalletBalanceSnapshot
	if err := database.DB.
		Where("user_id = ? AND wallet_address = ? AND day IN ?", user.ID, wallet.Address, days).
		Find(&snaps).Error; err != nil {
		b.sendMessage(chatID, "获取余额走势数据失败。")
		return
	}

	byDay := make(map[string]models.WalletBalanceSnapshot, len(snaps))
	for _, s := range snaps {
		byDay[strings.TrimSpace(s.Day)] = s
	}

	values := make([]float64, 0, 7)
	missing := 0
	for _, d := range days {
		s, ok := byDay[d]
		if !ok {
			values = append(values, 0)
			missing++
			continue
		}
		values = append(values, weiToFloat64(strings.TrimSpace(s.USDTBalanceWei)))
	}

	bars := make([]chart.Value, 0, 7)
	for i := range labels {
		bars = append(bars, chart.Value{
			Value: values[i],
			Label: labels[i],
		})
	}

	graph := chart.BarChart{
		Title:      "近7天余额走势",
		TitleStyle: chart.Shown(),
		Height:     420,
		BarWidth:   48,
		Bars:       bars,
	}
	if f := getChartCJKFont(); f != nil {
		graph.Font = f
	}

	buf := bytes.NewBuffer(nil)
	if err := graph.Render(chart.PNG, buf); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("生成走势图失败：%v", err))
		return
	}

	note := ""
	if missing > 0 {
		note = fmt.Sprintf("\n⚠️ 缺少 %d 天数据（将从今天开始自动记录）", missing)
	}

	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{
		Name:  "profit.png",
		Bytes: buf.Bytes(),
	})
	photo.Caption = fmt.Sprintf("📈 *余额走势*\n钱包：`%s`%s", shortenHex(wallet.Address), note)
	photo.ParseMode = "Markdown"
	photo.DisableNotification = true
	b.api.Send(photo)
}

func weiToFloat64(weiStr string) float64 {
	weiStr = strings.TrimSpace(weiStr)
	if weiStr == "" {
		return 0
	}
	v, ok := new(big.Int).SetString(weiStr, 10)
	if !ok {
		return 0
	}
	r := new(big.Rat).SetInt(v)
	denom := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	r.Quo(r, denom)
	f, _ := new(big.Float).SetRat(r).Float64()
	return f
}
