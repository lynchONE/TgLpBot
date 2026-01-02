package services

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type SmartLPPoolKey struct {
	PoolVersion string
	PoolID      string
}

type SmartLPRank struct {
	PoolVersion    string
	PoolID         string
	AddedLiquidity float64
	WalletCount    int
}

type SmartLPService struct {
	ch *ClickHouseService
}

func NewSmartLPService(ch *ClickHouseService) *SmartLPService {
	return &SmartLPService{ch: ch}
}

func (s *SmartLPService) GetTopAddedLiquidityPools(ctx context.Context, chain string, lookback time.Duration, limit int) ([]SmartLPRank, error) {
	out := make([]SmartLPRank, 0)
	if s == nil || s.ch == nil || s.ch.Conn == nil {
		return out, fmt.Errorf("clickhouse not initialized")
	}
	if limit <= 0 {
		return out, nil
	}
	if lookback <= 0 {
		lookback = time.Hour
	}
	seconds := int(lookback.Seconds())
	if seconds <= 0 {
		seconds = 3600
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	chainFilter := ""
	args := make([]interface{}, 0, 1)
	if chain != "" {
		chainFilter = "AND chain = ?"
		args = append(args, chain)
	}

	q := fmt.Sprintf(`
		SELECT pool_version, pool_id,
			sum(toFloat64OrZero(liquidity_delta)) AS added_liquidity,
			uniqExact(wallet_address) AS wallet_count
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND action = 'add'
			AND pool_version != '' AND pool_id != ''
			%s
		GROUP BY pool_version, pool_id
		ORDER BY wallet_count DESC, added_liquidity DESC
		LIMIT %d
	`, seconds, chainFilter, limit)

	rows, err := s.ch.Conn.Query(ctx, q, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		var pv string
		var pid string
		var added float64
		var wallets uint64
		if err := rows.Scan(&pv, &pid, &added, &wallets); err != nil {
			return out, err
		}
		out = append(out, SmartLPRank{
			PoolVersion:    pv,
			PoolID:         pid,
			AddedLiquidity: added,
			WalletCount:    int(wallets),
		})
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	return out, nil
}

func (s *SmartLPService) GetActiveWalletCounts(ctx context.Context, pools []SmartLPPoolKey) (map[string]int, error) {
	out := make(map[string]int)
	if s == nil || s.ch == nil || s.ch.Conn == nil {
		return out, nil
	}
	if len(pools) == 0 {
		return out, nil
	}

	placeholders := make([]string, 0, len(pools))
	args := make([]interface{}, 0, len(pools)*2)
	for _, p := range pools {
		pv := strings.ToLower(strings.TrimSpace(p.PoolVersion))
		pid := strings.ToLower(strings.TrimSpace(p.PoolID))
		if pv == "" || pid == "" {
			continue
		}
		placeholders = append(placeholders, "(?, ?)")
		args = append(args, pv, pid)
	}
	if len(placeholders) == 0 {
		return out, nil
	}

	q := fmt.Sprintf(`
		SELECT pool_version, pool_id, countIf(last_action = 'add') AS wallet_count
		FROM (
			SELECT pool_version, pool_id, wallet_address, argMax(action, event_seq) AS last_action
			FROM smart_lp_events
			WHERE ts >= now() - INTERVAL 15 DAY
				AND (pool_version, pool_id) IN (%s)
			GROUP BY pool_version, pool_id, wallet_address
		)
		GROUP BY pool_version, pool_id
	`, strings.Join(placeholders, ","))

	rows, err := s.ch.Conn.Query(ctx, q, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		var pv string
		var pid string
		var cnt uint64
		if err := rows.Scan(&pv, &pid, &cnt); err != nil {
			return out, err
		}
		key := smartLPPoolKey(pv, pid)
		out[key] = int(cnt)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	return out, nil
}

// GetRecentAddWalletCounts 统计每个池子在最近指定时间窗口内添加LP的不同钱包数量
// lookback: 时间窗口，例如 10 * time.Minute
func (s *SmartLPService) GetRecentAddWalletCounts(ctx context.Context, pools []SmartLPPoolKey, lookback time.Duration) (map[string]int, error) {
	out := make(map[string]int)
	if s == nil || s.ch == nil || s.ch.Conn == nil {
		return out, nil
	}
	if len(pools) == 0 {
		return out, nil
	}
	if lookback <= 0 {
		lookback = 10 * time.Minute // 默认10分钟
	}
	seconds := int(lookback.Seconds())
	if seconds <= 0 {
		seconds = 600 // 默认10分钟
	}

	placeholders := make([]string, 0, len(pools))
	args := make([]interface{}, 0, len(pools)*2)
	for _, p := range pools {
		pv := strings.ToLower(strings.TrimSpace(p.PoolVersion))
		pid := strings.ToLower(strings.TrimSpace(p.PoolID))
		if pv == "" || pid == "" {
			continue
		}
		placeholders = append(placeholders, "(?, ?)")
		args = append(args, pv, pid)
	}
	if len(placeholders) == 0 {
		return out, nil
	}

	// 查询最近lookback时间内有add动作的不同钱包数量
	q := fmt.Sprintf(`
		SELECT pool_version, pool_id, uniqExact(wallet_address) AS wallet_count
		FROM smart_lp_events
		WHERE ts >= now() - INTERVAL %d SECOND
			AND action = 'add'
			AND (pool_version, pool_id) IN (%s)
		GROUP BY pool_version, pool_id
	`, seconds, strings.Join(placeholders, ","))

	rows, err := s.ch.Conn.Query(ctx, q, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		var pv string
		var pid string
		var cnt uint64
		if err := rows.Scan(&pv, &pid, &cnt); err != nil {
			return out, err
		}
		key := smartLPPoolKey(pv, pid)
		out[key] = int(cnt)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	return out, nil
}

func smartLPPoolKey(poolVersion string, poolID string) string {
	return strings.ToLower(strings.TrimSpace(poolVersion)) + "|" + strings.ToLower(strings.TrimSpace(poolID))
}
