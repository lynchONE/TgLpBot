package web_server

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"

	"TgLpBot/base/database"
	"TgLpBot/base/models"
	autoLP "TgLpBot/service/auto_lp"
	"TgLpBot/service/pricing"
	"TgLpBot/service/strategy"
)

type autoLPPnLCurveRequest struct {
	InitData string `json:"initData"`
}

type autoLPPnLCurvePoint struct {
	T     int64   `json:"t"`
	Value float64 `json:"value"`
}

type autoLPPnLCurveEvent struct {
	Type string `json:"type"`
	T    int64  `json:"t"`

	TradeID uint   `json:"trade_id"`
	TaskID  uint   `json:"task_id"`
	Pair    string `json:"pair"`

	OpenUSDT float64 `json:"open_usdt,omitempty"`

	ProfitUSDT    float64 `json:"profit_usdt,omitempty"`
	ProfitPct     float64 `json:"profit_pct,omitempty"`
	CumProfitUSDT float64 `json:"cum_profit_usdt,omitempty"`
}

type autoLPPnLCurveResponse struct {
	OK bool `json:"ok"`

	WindowLabel string     `json:"window_label"`
	WindowStart *time.Time `json:"window_start,omitempty"`
	WindowEnd   *time.Time `json:"window_end,omitempty"`

	RealizedProfitUSDT   float64 `json:"realized_profit_usdt"`
	UnrealizedProfitUSDT float64 `json:"unrealized_profit_usdt"`
	TotalProfitUSDT      float64 `json:"total_profit_usdt"`

	TradesCount int  `json:"trades_count"`
	Truncated   bool `json:"truncated,omitempty"`

	Events         []autoLPPnLCurveEvent `json:"events"`
	SeriesRealized []autoLPPnLCurvePoint `json:"series_realized"`
	SeriesTotal    []autoLPPnLCurvePoint `json:"series_total"`

	UpdatedAt time.Time `json:"updated_at"`
	Warnings  []string  `json:"warnings,omitempty"`
}

type autoLPPnLCurveTradeRow struct {
	ID            uint                     `gorm:"column:id"`
	TaskID        uint                     `gorm:"column:task_id"`
	Status        models.TradeRecordStatus `gorm:"column:status"`
	Token0Symbol  string                   `gorm:"column:token0_symbol"`
	Token1Symbol  string                   `gorm:"column:token1_symbol"`
	OpenedAt      time.Time                `gorm:"column:opened_at"`
	ClosedAt      *time.Time               `gorm:"column:closed_at"`
	OpenUSDTSpent string                   `gorm:"column:open_usdt_spent"`
	ProfitUSDT    string                   `gorm:"column:profit_usdt"`
	ProfitPct     float64                  `gorm:"column:profit_pct"`
}

type autoLPPnLCurveOpenTradeRow struct {
	ID              uint      `gorm:"column:id"`
	TaskID          uint      `gorm:"column:task_id"`
	OpenGasSpentWei string    `gorm:"column:open_gas_spent_wei"`
	OpenedAt        time.Time `gorm:"column:opened_at"`
}

func weiStrToFloat18(weiStr string) (float64, bool) {
	weiStr = strings.TrimSpace(weiStr)
	if weiStr == "" {
		weiStr = "0"
	}
	v, ok := new(big.Int).SetString(weiStr, 10)
	if !ok {
		return 0, false
	}
	r := new(big.Rat).SetInt(v)
	r.Quo(r, new(big.Rat).SetInt(big.NewInt(1e18)))
	f, _ := r.Float64()
	return f, true
}

func resolveAutoLPWindow(cfg *models.AutoLPUserConfig) (*time.Time, *time.Time, string) {
	if cfg == nil {
		return nil, nil, "全部历史"
	}

	start := cfg.LastEnabledAt
	var end *time.Time
	label := ""

	if cfg.Enabled {
		now := time.Now()
		end = &now
		if start != nil {
			label = "本次开启至今"
		}
	} else if cfg.LastDisabledAt != nil {
		end = cfg.LastDisabledAt
		if start != nil {
			label = "上次开启"
		}
	} else if start != nil {
		now := time.Now()
		end = &now
		label = "最近开启至今"
	}

	if start == nil {
		label = "全部历史"
	}

	return start, end, label
}

func (s *Server) handleAutoLPPnLCurve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var req autoLPPnLCurveRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	if database.DB == nil {
		http.Error(w, "database not initialized", http.StatusInternalServerError)
		return
	}

	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}
	if status, msg := requireAutoModePermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	cfg, err := autoLP.NewAutoLPUserConfigService().GetOrCreate(user.ID)
	if err != nil {
		http.Error(w, "failed to load autolp config", http.StatusInternalServerError)
		return
	}

	start, end, label := resolveAutoLPWindow(cfg)

	// If AutoLP is disabled, extend window_end to include closes of trades opened during the run.
	if start != nil && !cfg.Enabled && cfg.LastDisabledAt != nil {
		type row struct {
			MaxClosedAt *time.Time `gorm:"column:max_closed_at"`
		}
		out := row{}
		q := `
			SELECT MAX(tr.closed_at) AS max_closed_at
			FROM trade_records tr
			JOIN strategy_tasks st ON st.id = tr.task_id
			WHERE tr.user_id = ? AND st.is_auto = 1
			  AND tr.opened_at >= ? AND tr.closed_at IS NOT NULL
		`
		if err := database.DB.WithContext(ctx).Raw(q, user.ID, *start).Scan(&out).Error; err == nil {
			if out.MaxClosedAt != nil && (end == nil || out.MaxClosedAt.After(*end)) {
				end = out.MaxClosedAt
			}
		}
	}

	const tradeLimit = 400
	var rows []autoLPPnLCurveTradeRow

	tradeQ := database.DB.WithContext(ctx).
		Table("trade_records tr").
		Select(strings.Join([]string{
			"tr.id",
			"tr.task_id",
			"tr.status",
			"tr.token0_symbol",
			"tr.token1_symbol",
			"tr.opened_at",
			"tr.closed_at",
			"tr.open_usdt_spent",
			"tr.profit_usdt",
			"tr.profit_pct",
		}, ", ")).
		Joins("JOIN strategy_tasks st ON st.id = tr.task_id").
		Where("tr.user_id = ? AND st.is_auto = 1", user.ID)

	if start != nil {
		tradeQ = tradeQ.Where("tr.opened_at >= ?", *start)
	}
	if end != nil {
		tradeQ = tradeQ.Where("tr.opened_at <= ?", *end)
	}

	// Fetch 1 extra row to detect truncation.
	if err := tradeQ.Order("tr.opened_at DESC").Limit(tradeLimit + 1).Scan(&rows).Error; err != nil {
		http.Error(w, "failed to query trade records", http.StatusInternalServerError)
		return
	}

	truncated := len(rows) > tradeLimit
	if truncated {
		rows = rows[:tradeLimit]
	}

	// Reverse to roughly chronological by open time.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}

	baselineRealized := 0.0
	var earliestOpenedAt *time.Time
	if truncated && len(rows) > 0 {
		t := rows[0].OpenedAt
		earliestOpenedAt = &t

		type sumRow struct {
			Profit string `gorm:"column:profit"`
		}
		out := sumRow{}

		q := `
			SELECT COALESCE(SUM(CAST(tr.profit_usdt AS DECIMAL(65,0))), 0) AS profit
			FROM trade_records tr
			JOIN strategy_tasks st ON st.id = tr.task_id
			WHERE tr.user_id = ? AND st.is_auto = 1 AND tr.status = ?
		`
		args := []any{user.ID, models.TradeStatusClosed}

		if start != nil {
			q += " AND tr.opened_at >= ?"
			args = append(args, *start)
		}
		q += " AND tr.opened_at < ?"
		args = append(args, *earliestOpenedAt)

		if err := database.DB.WithContext(ctx).Raw(q, args...).Scan(&out).Error; err == nil {
			if v, ok := weiStrToFloat18(out.Profit); ok {
				baselineRealized = v
			}
		}
	}

	events := make([]autoLPPnLCurveEvent, 0, len(rows)*2)
	for _, tr := range rows {
		pair := strings.TrimSpace(tr.Token0Symbol) + "/" + strings.TrimSpace(tr.Token1Symbol)
		if strings.TrimSpace(pair) == "/" {
			pair = ""
		}

		openUSDT, _ := weiStrToFloat18(tr.OpenUSDTSpent)
		events = append(events, autoLPPnLCurveEvent{
			Type:     "open",
			T:        tr.OpenedAt.Unix(),
			TradeID:  tr.ID,
			TaskID:   tr.TaskID,
			Pair:     pair,
			OpenUSDT: openUSDT,
		})

		if tr.Status == models.TradeStatusClosed && tr.ClosedAt != nil {
			profitUSDT, _ := weiStrToFloat18(tr.ProfitUSDT)
			events = append(events, autoLPPnLCurveEvent{
				Type:       "close",
				T:          tr.ClosedAt.Unix(),
				TradeID:    tr.ID,
				TaskID:     tr.TaskID,
				Pair:       pair,
				ProfitUSDT: profitUSDT,
				ProfitPct:  tr.ProfitPct,
			})
		}
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].T != events[j].T {
			return events[i].T < events[j].T
		}
		// Open marker first when timestamps collide.
		if events[i].Type != events[j].Type {
			return events[i].Type == "open"
		}
		return events[i].TradeID < events[j].TradeID
	})

	seriesStartAt := time.Now().Unix()
	if start != nil {
		seriesStartAt = start.Unix()
	} else if len(events) > 0 {
		seriesStartAt = events[0].T
	}

	cumRealized := baselineRealized
	seriesRealized := make([]autoLPPnLCurvePoint, 0, 1+len(events))
	seriesRealized = append(seriesRealized, autoLPPnLCurvePoint{T: seriesStartAt, Value: cumRealized})

	for i := range events {
		if events[i].Type != "close" {
			continue
		}
		cumRealized += events[i].ProfitUSDT
		events[i].CumProfitUSDT = cumRealized
		seriesRealized = append(seriesRealized, autoLPPnLCurvePoint{T: events[i].T, Value: cumRealized})
	}

	// Compute floating PnL for open auto trades in the current window.
	unrealized := 0.0
	warnings := make([]string, 0, 2)

	var openTrades []autoLPPnLCurveOpenTradeRow
	openQ := database.DB.WithContext(ctx).
		Table("trade_records tr").
		Select("tr.id, tr.task_id, tr.open_gas_spent_wei, tr.opened_at").
		Joins("JOIN strategy_tasks st ON st.id = tr.task_id").
		Where("tr.user_id = ? AND st.is_auto = 1 AND tr.status = ?", user.ID, models.TradeStatusOpen)
	if start != nil {
		openQ = openQ.Where("tr.opened_at >= ?", *start)
	}
	if end != nil {
		openQ = openQ.Where("tr.opened_at <= ?", *end)
	}
	_ = openQ.Scan(&openTrades).Error

	if len(openTrades) > 0 {
		taskIDs := make([]uint, 0, len(openTrades))
		taskByID := make(map[uint]models.StrategyTask, len(openTrades))
		for _, ot := range openTrades {
			if ot.TaskID > 0 {
				taskIDs = append(taskIDs, ot.TaskID)
			}
		}

		var tasks []models.StrategyTask
		if len(taskIDs) > 0 {
			_ = database.DB.WithContext(ctx).
				Where("user_id = ? AND id IN ?", user.ID, taskIDs).
				Find(&tasks).Error
		}
		for _, t := range tasks {
			taskByID[t.ID] = t
		}

		pnlSvc := strategy.NewPnLService()
		bnbPriceUSDT := pricing.GetBNBPriceUSDT()

		for _, ot := range openTrades {
			task, ok := taskByID[ot.TaskID]
			if !ok {
				continue
			}
			info, perr := pnlSvc.GetTaskPnL(&task)
			if perr != nil || info == nil {
				warnings = append(warnings, "部分未平仓任务浮动盈亏计算失败（请稍后重试）")
				continue
			}

			pnl := info.AbsolutePnLUSDT
			if gasWei, ok := new(big.Int).SetString(strings.TrimSpace(ot.OpenGasSpentWei), 10); ok && gasWei != nil && gasWei.Sign() > 0 {
				gasBNB, _ := weiStrToFloat18(gasWei.String())
				pnl -= gasBNB * bnbPriceUSDT
			}
			unrealized += pnl
		}
	}

	total := cumRealized + unrealized

	seriesTotal := make([]autoLPPnLCurvePoint, 0, len(seriesRealized)+1)
	seriesTotal = append(seriesTotal, seriesRealized...)
	nowT := time.Now().Unix()
	if len(seriesTotal) == 0 || seriesTotal[len(seriesTotal)-1].T != nowT {
		seriesTotal = append(seriesTotal, autoLPPnLCurvePoint{T: nowT, Value: total})
	} else {
		seriesTotal[len(seriesTotal)-1].Value = total
	}

	resp := autoLPPnLCurveResponse{
		OK: true,

		WindowLabel: label,
		WindowStart: start,
		WindowEnd:   end,

		RealizedProfitUSDT:   cumRealized,
		UnrealizedProfitUSDT: unrealized,
		TotalProfitUSDT:      total,

		TradesCount: len(rows),
		Truncated:   truncated,

		Events:         events,
		SeriesRealized: seriesRealized,
		SeriesTotal:    seriesTotal,

		UpdatedAt: time.Now(),
		Warnings:  warnings,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
