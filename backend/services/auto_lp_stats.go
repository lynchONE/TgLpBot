package services

import (
	"TgLpBot/database"
	"TgLpBot/models"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type AutoLPStats struct {
	WindowLabel string     `json:"window_label"`
	WindowStart *time.Time `json:"window_start,omitempty"`
	WindowEnd   *time.Time `json:"window_end,omitempty"`

	OpenCount      int64 `json:"open_count"`
	RebalanceCount int64 `json:"rebalance_count"`
	GuardCount     int64 `json:"guard_count"`

	GasUSDTWei    string `json:"gas_usdt_wei"`
	ProfitUSDTWei string `json:"profit_usdt_wei"`

	BestPair           string `json:"best_pair"`
	BestProfitUSDTWei  string `json:"best_profit_usdt_wei"`
	WorstPair          string `json:"worst_pair"`
	WorstProfitUSDTWei string `json:"worst_profit_usdt_wei"`
}

type AutoLPStatsService struct{}

func NewAutoLPStatsService() *AutoLPStatsService {
	return &AutoLPStatsService{}
}

func (s *AutoLPStatsService) GetUserStats(userID uint, cfg *models.AutoLPUserConfig) (*AutoLPStats, error) {
	if database.DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	start, end, label := resolveAutoLPStatsWindow(cfg)
	stats := &AutoLPStats{
		WindowLabel:   label,
		WindowStart:   start,
		WindowEnd:     end,
		GasUSDTWei:    "0",
		ProfitUSDTWei: "0",
	}

	openCount, err := s.countEvents(userID, models.AutoLPEventOpen, start, end)
	if err != nil {
		return nil, err
	}
	rebalanceCount, err := s.countEvents(userID, models.AutoLPEventRebalance, start, end)
	if err != nil {
		return nil, err
	}
	guardCount, err := s.countEvents(userID, models.AutoLPEventGuardExit, start, end)
	if err != nil {
		return nil, err
	}

	stats.OpenCount = openCount
	stats.RebalanceCount = rebalanceCount
	stats.GuardCount = guardCount

	profit, gas, err := s.sumProfitAndGas(userID, start, end)
	if err != nil {
		return nil, err
	}
	stats.ProfitUSDTWei = profit
	stats.GasUSDTWei = gas

	bestPair, bestProfit, err := s.pickExtremePair(userID, start, end, true)
	if err != nil {
		return nil, err
	}
	worstPair, worstProfit, err := s.pickExtremePair(userID, start, end, false)
	if err != nil {
		return nil, err
	}
	stats.BestPair = bestPair
	stats.BestProfitUSDTWei = bestProfit
	stats.WorstPair = worstPair
	stats.WorstProfitUSDTWei = worstProfit

	return stats, nil
}

func resolveAutoLPStatsWindow(cfg *models.AutoLPUserConfig) (*time.Time, *time.Time, string) {
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

func (s *AutoLPStatsService) countEvents(userID uint, eventType models.AutoLPEventType, start *time.Time, end *time.Time) (int64, error) {
	if database.DB == nil {
		return 0, nil
	}

	q := database.DB.Model(&models.AutoLPEvent{}).
		Where("user_id = ? AND event_type = ?", userID, eventType)
	q = applyTimeWindow(q, "created_at", start, end)

	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *AutoLPStatsService) sumProfitAndGas(userID uint, start *time.Time, end *time.Time) (string, string, error) {
	type row struct {
		Profit string `gorm:"column:profit"`
		Gas    string `gorm:"column:gas"`
	}
	out := row{}

	query := `
		SELECT
			COALESCE(SUM(CAST(tr.profit_usdt AS DECIMAL(65,0))), 0) AS profit,
			COALESCE(SUM(CAST(tr.total_gas_usdt AS DECIMAL(65,0))), 0) AS gas
		FROM trade_records tr
		JOIN strategy_tasks st ON st.id = tr.task_id
		WHERE tr.user_id = ? AND st.is_auto = 1 AND tr.status = ?
	`
	args := []any{userID, models.TradeStatusClosed}

	query, args = appendTimeWindow(query, args, "tr.closed_at", start, end)

	if err := database.DB.Raw(query, args...).Scan(&out).Error; err != nil {
		return "0", "0", err
	}

	profit := strings.TrimSpace(out.Profit)
	if profit == "" {
		profit = "0"
	}
	gas := strings.TrimSpace(out.Gas)
	if gas == "" {
		gas = "0"
	}

	return profit, gas, nil
}

func (s *AutoLPStatsService) pickExtremePair(userID uint, start *time.Time, end *time.Time, wantBest bool) (string, string, error) {
	type row struct {
		Token0Symbol string `gorm:"column:token0_symbol"`
		Token1Symbol string `gorm:"column:token1_symbol"`
		Profit       string `gorm:"column:profit"`
	}
	out := row{}

	comp := ">"
	order := "DESC"
	if !wantBest {
		comp = "<"
		order = "ASC"
	}

	query := fmt.Sprintf(`
		SELECT tr.token0_symbol, tr.token1_symbol, tr.profit_usdt AS profit
		FROM trade_records tr
		JOIN strategy_tasks st ON st.id = tr.task_id
		WHERE tr.user_id = ? AND st.is_auto = 1 AND tr.status = ?
		  AND CAST(tr.profit_usdt AS DECIMAL(65,0)) %s 0
	`, comp)
	args := []any{userID, models.TradeStatusClosed}

	query, args = appendTimeWindow(query, args, "tr.closed_at", start, end)
	query = query + fmt.Sprintf(" ORDER BY CAST(tr.profit_usdt AS DECIMAL(65,0)) %s LIMIT 1", order)

	res := database.DB.Raw(query, args...).Scan(&out)
	if res.Error != nil {
		return "", "", res.Error
	}
	if res.RowsAffected == 0 {
		return "", "", nil
	}

	pair := strings.TrimSpace(out.Token0Symbol) + "/" + strings.TrimSpace(out.Token1Symbol)
	if strings.TrimSpace(pair) == "/" {
		pair = ""
	}
	profit := strings.TrimSpace(out.Profit)
	if profit == "" {
		profit = "0"
	}

	return pair, profit, nil
}

func applyTimeWindow(q *gorm.DB, column string, start *time.Time, end *time.Time) *gorm.DB {
	if start != nil {
		q = q.Where(fmt.Sprintf("%s >= ?", column), *start)
	}
	if end != nil {
		q = q.Where(fmt.Sprintf("%s <= ?", column), *end)
	}
	return q
}

func appendTimeWindow(query string, args []any, column string, start *time.Time, end *time.Time) (string, []any) {
	if start != nil {
		query += fmt.Sprintf(" AND %s >= ?", column)
		args = append(args, *start)
	}
	if end != nil {
		query += fmt.Sprintf(" AND %s <= ?", column)
		args = append(args, *end)
	}
	return query, args
}
