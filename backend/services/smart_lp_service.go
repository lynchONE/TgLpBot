package services

import (
	"context"
	"fmt"
	"strings"
)

type SmartLPPoolKey struct {
	PoolVersion string
	PoolID      string
}

type SmartLPService struct {
	ch *ClickHouseService
}

func NewSmartLPService(ch *ClickHouseService) *SmartLPService {
	return &SmartLPService{ch: ch}
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

func smartLPPoolKey(poolVersion string, poolID string) string {
	return strings.ToLower(strings.TrimSpace(poolVersion)) + "|" + strings.ToLower(strings.TrimSpace(poolID))
}
