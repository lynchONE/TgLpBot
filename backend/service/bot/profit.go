package bot

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"bytes"
	"fmt"
	"math"
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
	"github.com/wcharczuk/go-chart/v2/drawing"
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
		b.sendMessage(chatID, "您还没有任何钱包。请先使用 /wallet 导入钱包。")
		return
	}

	// Ensure today's snapshot exists.
	if b.snapshotService != nil {
		_ = b.snapshotService.CaptureTodayForUser(user.ID)
	}

	now := time.Now()
	days := make([]string, 0, 7)
	dates := make([]time.Time, 0, 7)
	for i := 6; i >= 0; i-- {
		t := now.AddDate(0, 0, -i)
		days = append(days, t.Format("2006-01-02"))
		dates = append(dates, t)
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
	present := make([]bool, 0, 7)
	missing := 0
	for _, d := range days {
		s, ok := byDay[d]
		if !ok {
			values = append(values, 0)
			present = append(present, false)
			missing++
			continue
		}
		values = append(values, weiToFloat64(strings.TrimSpace(s.USDTBalanceWei)))
		present = append(present, true)
	}

	var (
		minY       float64
		maxY       float64
		hasAnyData bool
	)
	for i := range values {
		if !present[i] {
			continue
		}
		v := values[i]
		if !hasAnyData {
			minY, maxY = v, v
			hasAnyData = true
			continue
		}
		minY = math.Min(minY, v)
		maxY = math.Max(maxY, v)
	}
	if !hasAnyData {
		b.sendMessage(chatID, "暂无余额走势数据（将从今天开始自动记录）。")
		return
	}

	// 定义现代化配色方案 - 深色主题
	darkBg := drawing.Color{R: 26, G: 31, B: 46, A: 255}            // 深蓝灰背景 #1a1f2e
	accentGreen := drawing.Color{R: 0, G: 212, B: 170, A: 255}      // 主色：青绿色 #00d4aa
	accentGreenLight := drawing.Color{R: 0, G: 255, B: 204, A: 255} // 亮青绿色 #00ffcc
	gridColor := drawing.Color{R: 42, G: 49, B: 66, A: 255}         // 网格线颜色 #2a3142
	textColor := drawing.Color{R: 160, G: 174, B: 192, A: 255}      // 文字颜色 #a0aec0

	const (
		strokeWidth = 3.5
		dotWidth    = 6.0
	)

	// 主数据线样式 - 青绿色渐变
	seriesStyle := chart.Style{
		StrokeColor: accentGreen,
		StrokeWidth: strokeWidth,
		DotColor:    accentGreenLight,
		DotWidth:    dotWidth,
		FillColor:   accentGreen.WithAlpha(40), // 半透明填充
	}

	segments := make([]chart.Series, 0, 2)
	segX := make([]time.Time, 0, 7)
	segY := make([]float64, 0, 7)
	flushSeg := func() {
		if len(segX) == 0 {
			return
		}
		segments = append(segments, chart.TimeSeries{
			Style:   seriesStyle,
			XValues: append([]time.Time(nil), segX...),
			YValues: append([]float64(nil), segY...),
		})
		segX = segX[:0]
		segY = segY[:0]
	}
	for i := range dates {
		if !present[i] {
			flushSeg()
			continue
		}
		segX = append(segX, dates[i])
		segY = append(segY, values[i])
	}
	flushSeg()

	yPad := math.Max(1.0, maxY*0.05) // 增加Y轴边距
	yMin := minY - yPad
	if yMin < 0 {
		yMin = 0
	}
	yMax := maxY + yPad
	if yMax <= yMin {
		yMax = yMin + 1
	}

	xTicks := make([]chart.Tick, 0, len(dates))
	for i := range dates {
		xTicks = append(xTicks, chart.Tick{
			Value: chart.TimeToFloat64(dates[i]),
			Label: dates[i].Format("01-02"),
		})
	}

	// 获取最后一个有效值用于标注
	var lastValue float64
	var lastDate time.Time
	for i := len(values) - 1; i >= 0; i-- {
		if present[i] {
			lastValue = values[i]
			lastDate = dates[i]
			break
		}
	}

	// 计算7日变动
	var firstValue float64
	hasFirst := false
	for i := 0; i < len(values); i++ {
		if present[i] {
			firstValue = values[i]
			hasFirst = true
			break
		}
	}
	changePercent := 0.0
	if hasFirst && firstValue > 0 {
		changePercent = ((lastValue - firstValue) / firstValue) * 100
	}

	// 创建当前余额标注
	annotations := []chart.Value2{
		{
			XValue: chart.TimeToFloat64(lastDate),
			YValue: lastValue,
			Label:  fmt.Sprintf("$%.2f", lastValue),
		},
	}

	// 添加最新值标注系列
	annotationSeries := chart.AnnotationSeries{
		Name:        "余额标注",
		Annotations: annotations,
		Style: chart.Style{
			FontSize:    12,
			FontColor:   accentGreenLight,
			StrokeColor: accentGreen,
			FillColor:   darkBg,
			Padding:     chart.Box{Top: 5, Left: 5, Right: 5, Bottom: 5},
		},
	}

	// 合并所有系列
	allSeries := append(segments, annotationSeries)

	graph := chart.Chart{
		Width:  800,
		Height: 450,
		Background: chart.Style{
			FillColor: darkBg,
		},
		Canvas: chart.Style{
			FillColor: darkBg,
		},
		YAxis: chart.YAxis{
			Range: &chart.ContinuousRange{Min: yMin, Max: yMax},
			ValueFormatter: func(v interface{}) string {
				return chart.FloatValueFormatterWithFormat(v, "$%.2f")
			},
			Style: chart.Style{
				FontColor:   textColor,
				StrokeColor: gridColor,
			},
			GridMajorStyle: chart.Style{
				StrokeColor: gridColor,
				StrokeWidth: 1,
			},
		},
		XAxis: chart.XAxis{
			Ticks: xTicks,
			Style: chart.Style{
				FontColor:   textColor,
				StrokeColor: gridColor,
			},
			GridMajorStyle: chart.Style{
				StrokeColor: gridColor,
				StrokeWidth: 1,
			},
		},
		Series: allSeries,
	}
	if f := getChartCJKFont(); f != nil {
		graph.Font = f
	}

	// 生成变动提示
	changeNote := ""
	if hasFirst && firstValue > 0 {
		if changePercent >= 0 {
			changeNote = fmt.Sprintf("\n📊 7日变动: +%.2f%%", changePercent)
		} else {
			changeNote = fmt.Sprintf("\n📊 7日变动: %.2f%%", changePercent)
		}
	}

	buf := bytes.NewBuffer(nil)
	if err := graph.Render(chart.PNG, buf); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("生成走势图失败：%v", err))
		return
	}

	missingNote := ""
	if missing > 0 {
		missingNote = fmt.Sprintf("\n⚠️ 缺少 %d 天数据", missing)
	}

	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{
		Name:  "profit.png",
		Bytes: buf.Bytes(),
	})
	photo.Caption = fmt.Sprintf("📈 *余额走势*\n钱包：`%s`%s%s", shortenHex(wallet.Address), changeNote, missingNote)
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
