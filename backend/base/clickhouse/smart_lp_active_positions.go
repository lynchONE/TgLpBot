package clickhouse

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strings"
	"time"
)

const smartLPActivePositionsTable = "smart_lp_active_positions"

var smartLPActiveZeroTime = time.Unix(0, 0).UTC()

type SmartLPActivePositionEvent struct {
	Ts              time.Time
	EventSeq        uint64
	Chain           string
	PoolVersion     string
	PoolID          string
	WalletAddress   string
	Action          string
	TokenID         string
	LiquidityDelta  string
	TickLower       int32
	TickUpper       int32
	BlockNumber     uint64
	ContractAddress string
	Source          string
}

type smartLPActivePositionState struct {
	PositionKey      string
	Chain            string
	PoolVersion      string
	PoolID           string
	WalletAddress    string
	ContractAddress  string
	TokenID          string
	TickLower        int32
	TickUpper        int32
	CurrentLiquidity *big.Int
	IsActive         bool
	OpenedAt         time.Time
	LastAddAt        time.Time
	LastRemoveAt     time.Time
	LastEventSeq     uint64
	LastEventBlock   uint64
	LastEventTs      time.Time
	Source           string
	UpdatedAt        time.Time
}

func smartLPActivePositionCreateTableSQL() string {
	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			position_key String,
			chain LowCardinality(String),
			pool_version LowCardinality(String),
			pool_id String,
			wallet_address String,
			contract_address String,
			token_id String,
			tick_lower Int32,
			tick_upper Int32,
			current_liquidity String,
			is_active UInt8,
			opened_at DateTime,
			last_add_at DateTime,
			last_remove_at DateTime,
			last_event_seq UInt64,
			last_event_block UInt64,
			last_event_ts DateTime,
			source LowCardinality(String),
			updated_at DateTime,
			ingest_id UUID DEFAULT generateUUIDv4()
		) ENGINE = ReplacingMergeTree(last_event_seq)
		PARTITION BY tuple()
		ORDER BY (chain, pool_version, pool_id, wallet_address, position_key)
	`, smartLPActivePositionsTable)
}

func buildSmartLPActivePositionKey(chain string, poolVersion string, poolID string, wallet string, contractAddress string, tokenID string, tickLower int32, tickUpper int32) string {
	return strings.ToLower(strings.TrimSpace(chain)) + "|" +
		strings.ToLower(strings.TrimSpace(poolVersion)) + "|" +
		strings.ToLower(strings.TrimSpace(poolID)) + "|" +
		strings.ToLower(strings.TrimSpace(wallet)) + "|" +
		strings.ToLower(strings.TrimSpace(contractAddress)) + "|" +
		strings.TrimSpace(tokenID) + "|" +
		fmt.Sprintf("%d", tickLower) + "|" +
		fmt.Sprintf("%d", tickUpper)
}

func normalizeSmartLPActivePositionEvent(ev SmartLPActivePositionEvent) (SmartLPActivePositionEvent, string, bool) {
	ev.Chain = strings.ToLower(strings.TrimSpace(ev.Chain))
	ev.PoolVersion = strings.ToLower(strings.TrimSpace(ev.PoolVersion))
	ev.PoolID = strings.ToLower(strings.TrimSpace(ev.PoolID))
	ev.WalletAddress = strings.ToLower(strings.TrimSpace(ev.WalletAddress))
	ev.ContractAddress = strings.ToLower(strings.TrimSpace(ev.ContractAddress))
	ev.TokenID = strings.TrimSpace(ev.TokenID)
	ev.Action = strings.ToLower(strings.TrimSpace(ev.Action))
	ev.Source = strings.TrimSpace(ev.Source)
	if ev.Ts.IsZero() {
		ev.Ts = time.Now().UTC()
	} else {
		ev.Ts = ev.Ts.UTC()
	}

	if ev.EventSeq == 0 || ev.Chain == "" || ev.PoolVersion == "" || ev.PoolID == "" || ev.WalletAddress == "" {
		return SmartLPActivePositionEvent{}, "", false
	}
	if ev.Action != "add" && ev.Action != "remove" {
		return SmartLPActivePositionEvent{}, "", false
	}
	key := buildSmartLPActivePositionKey(ev.Chain, ev.PoolVersion, ev.PoolID, ev.WalletAddress, ev.ContractAddress, ev.TokenID, ev.TickLower, ev.TickUpper)
	if key == "" {
		return SmartLPActivePositionEvent{}, "", false
	}
	return ev, key, true
}

func parseSmartLPActiveLiquidityDelta(poolVersion string, action string, raw string) (*big.Int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	delta, ok := new(big.Int).SetString(raw, 10)
	if !ok || delta == nil || delta.Sign() == 0 {
		return nil, false
	}
	if strings.ToLower(strings.TrimSpace(poolVersion)) != "v4" && strings.ToLower(strings.TrimSpace(action)) == "remove" {
		delta = new(big.Int).Neg(delta)
	}
	return delta, true
}

func smartLPActiveTimeOrZero(t time.Time) time.Time {
	if t.IsZero() {
		return smartLPActiveZeroTime
	}
	return t.UTC()
}

func applySmartLPActivePositionEvent(state smartLPActivePositionState, ev SmartLPActivePositionEvent, key string, now time.Time) (smartLPActivePositionState, bool) {
	if state.PositionKey != "" && ev.EventSeq <= state.LastEventSeq {
		return state, false
	}
	delta, ok := parseSmartLPActiveLiquidityDelta(ev.PoolVersion, ev.Action, ev.LiquidityDelta)
	if !ok {
		return state, false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	current := big.NewInt(0)
	if state.CurrentLiquidity != nil {
		current = new(big.Int).Set(state.CurrentLiquidity)
	}
	nextLiquidity := new(big.Int).Add(current, delta)
	if nextLiquidity.Sign() < 0 {
		nextLiquidity = big.NewInt(0)
	}

	openedAt := state.OpenedAt
	lastAddAt := state.LastAddAt
	lastRemoveAt := state.LastRemoveAt

	if delta.Sign() > 0 {
		if current.Sign() <= 0 || !openedAt.After(smartLPActiveZeroTime) {
			openedAt = ev.Ts
		}
		lastAddAt = ev.Ts
	} else {
		lastRemoveAt = ev.Ts
		if nextLiquidity.Sign() <= 0 {
			openedAt = smartLPActiveZeroTime
		}
	}

	next := smartLPActivePositionState{
		PositionKey:      key,
		Chain:            ev.Chain,
		PoolVersion:      ev.PoolVersion,
		PoolID:           ev.PoolID,
		WalletAddress:    ev.WalletAddress,
		ContractAddress:  ev.ContractAddress,
		TokenID:          ev.TokenID,
		TickLower:        ev.TickLower,
		TickUpper:        ev.TickUpper,
		CurrentLiquidity: nextLiquidity,
		IsActive:         nextLiquidity.Sign() > 0,
		OpenedAt:         smartLPActiveTimeOrZero(openedAt),
		LastAddAt:        smartLPActiveTimeOrZero(lastAddAt),
		LastRemoveAt:     smartLPActiveTimeOrZero(lastRemoveAt),
		LastEventSeq:     ev.EventSeq,
		LastEventBlock:   ev.BlockNumber,
		LastEventTs:      ev.Ts,
		Source:           ev.Source,
		UpdatedAt:        now,
	}
	return next, true
}

func (s *ClickHouseService) ensureSmartLPActivePositions(ctx context.Context) error {
	if s == nil || s.Conn == nil {
		return fmt.Errorf("clickhouse not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.Conn.Exec(ctx, smartLPActivePositionCreateTableSQL()); err != nil {
		return fmt.Errorf("clickhouse create smart_lp_active_positions failed: %w", err)
	}

	alterQueries := []string{
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS position_key String`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS chain LowCardinality(String)`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS pool_version LowCardinality(String)`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS pool_id String`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS wallet_address String`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS contract_address String`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS token_id String`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS tick_lower Int32 DEFAULT 0`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS tick_upper Int32 DEFAULT 0`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS current_liquidity String DEFAULT '0'`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS is_active UInt8 DEFAULT 0`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS opened_at DateTime DEFAULT toDateTime(0)`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS last_add_at DateTime DEFAULT toDateTime(0)`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS last_remove_at DateTime DEFAULT toDateTime(0)`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS last_event_seq UInt64 DEFAULT 0`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS last_event_block UInt64 DEFAULT 0`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS last_event_ts DateTime DEFAULT toDateTime(0)`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS source LowCardinality(String) DEFAULT ''`,
		`ALTER TABLE smart_lp_active_positions ADD COLUMN IF NOT EXISTS updated_at DateTime DEFAULT now()`,
	}
	for _, q := range alterQueries {
		if err := s.Conn.Exec(ctx, q); err != nil {
			return fmt.Errorf("clickhouse alter smart_lp_active_positions failed: %w", err)
		}
	}

	var count uint64
	if err := s.Conn.QueryRow(ctx, fmt.Sprintf("SELECT count() FROM %s", smartLPActivePositionsTable)).Scan(&count); err != nil {
		return fmt.Errorf("clickhouse count smart_lp_active_positions rows failed: %w", err)
	}
	if count > 0 {
		return nil
	}

	log.Printf("[ClickHouse] backfilling %s from %s (last 2 days)", smartLPActivePositionsTable, smartLPEventsTable)
	if _, err := s.ReconcileSmartLPActivePositionsFromRecentEvents(ctx, 48*time.Hour); err != nil {
		return err
	}
	return nil
}

func (s *ClickHouseService) loadSmartLPActivePositionStates(ctx context.Context, keys []string) (map[string]smartLPActivePositionState, error) {
	out := make(map[string]smartLPActivePositionState)
	if s == nil || s.Conn == nil || len(keys) == 0 {
		return out, nil
	}
	placeholders := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		placeholders = append(placeholders, "?")
		args = append(args, key)
	}
	if len(placeholders) == 0 {
		return out, nil
	}

	q := fmt.Sprintf(`
		SELECT
			position_key,
			argMax(chain, tuple(last_event_seq, updated_at)) AS latest_chain,
			argMax(pool_version, tuple(last_event_seq, updated_at)) AS latest_pool_version,
			argMax(pool_id, tuple(last_event_seq, updated_at)) AS latest_pool_id,
			argMax(wallet_address, tuple(last_event_seq, updated_at)) AS latest_wallet_address,
			argMax(contract_address, tuple(last_event_seq, updated_at)) AS latest_contract_address,
			argMax(token_id, tuple(last_event_seq, updated_at)) AS latest_token_id,
			argMax(tick_lower, tuple(last_event_seq, updated_at)) AS latest_tick_lower,
			argMax(tick_upper, tuple(last_event_seq, updated_at)) AS latest_tick_upper,
			argMax(current_liquidity, tuple(last_event_seq, updated_at)) AS latest_current_liquidity,
			argMax(is_active, tuple(last_event_seq, updated_at)) AS latest_is_active,
			argMax(opened_at, tuple(last_event_seq, updated_at)) AS latest_opened_at,
			argMax(last_add_at, tuple(last_event_seq, updated_at)) AS latest_last_add_at,
			argMax(last_remove_at, tuple(last_event_seq, updated_at)) AS latest_last_remove_at,
			max(last_event_seq) AS latest_last_event_seq,
			argMax(last_event_block, tuple(last_event_seq, updated_at)) AS latest_last_event_block,
			argMax(last_event_ts, tuple(last_event_seq, updated_at)) AS latest_last_event_ts,
			argMax(source, tuple(last_event_seq, updated_at)) AS latest_source,
			argMax(updated_at, tuple(last_event_seq, updated_at)) AS latest_updated_at
		FROM %s
		WHERE position_key IN (%s)
		GROUP BY position_key
	`, smartLPActivePositionsTable, strings.Join(placeholders, ","))

	rows, err := s.Conn.Query(ctx, q, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			state            smartLPActivePositionState
			currentLiquidity string
			isActive         uint8
		)
		if err := rows.Scan(
			&state.PositionKey,
			&state.Chain,
			&state.PoolVersion,
			&state.PoolID,
			&state.WalletAddress,
			&state.ContractAddress,
			&state.TokenID,
			&state.TickLower,
			&state.TickUpper,
			&currentLiquidity,
			&isActive,
			&state.OpenedAt,
			&state.LastAddAt,
			&state.LastRemoveAt,
			&state.LastEventSeq,
			&state.LastEventBlock,
			&state.LastEventTs,
			&state.Source,
			&state.UpdatedAt,
		); err != nil {
			return out, err
		}
		state.Chain = strings.ToLower(strings.TrimSpace(state.Chain))
		state.PoolVersion = strings.ToLower(strings.TrimSpace(state.PoolVersion))
		state.PoolID = strings.ToLower(strings.TrimSpace(state.PoolID))
		state.WalletAddress = strings.ToLower(strings.TrimSpace(state.WalletAddress))
		state.ContractAddress = strings.ToLower(strings.TrimSpace(state.ContractAddress))
		state.TokenID = strings.TrimSpace(state.TokenID)
		state.Source = strings.TrimSpace(state.Source)
		state.IsActive = isActive == 1
		state.CurrentLiquidity = big.NewInt(0)
		if parsed, ok := new(big.Int).SetString(strings.TrimSpace(currentLiquidity), 10); ok && parsed != nil {
			state.CurrentLiquidity = parsed
		}
		out[state.PositionKey] = state
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, nil
}

func (s *ClickHouseService) insertSmartLPActivePositionStates(ctx context.Context, states []smartLPActivePositionState) error {
	if s == nil || s.Conn == nil || len(states) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	batch, err := s.PrepareBatch(ctx, fmt.Sprintf(`INSERT INTO %s (
		position_key, chain, pool_version, pool_id, wallet_address, contract_address, token_id,
		tick_lower, tick_upper, current_liquidity, is_active, opened_at, last_add_at, last_remove_at,
		last_event_seq, last_event_block, last_event_ts, source, updated_at
	)`, smartLPActivePositionsTable))
	if err != nil {
		return err
	}
	defer func() { _ = batch.Abort() }()

	for _, state := range states {
		currentLiquidity := "0"
		if state.CurrentLiquidity != nil {
			currentLiquidity = state.CurrentLiquidity.String()
		}
		isActive := uint8(0)
		if state.IsActive {
			isActive = 1
		}
		if err := batch.Append(
			state.PositionKey,
			state.Chain,
			state.PoolVersion,
			state.PoolID,
			state.WalletAddress,
			state.ContractAddress,
			state.TokenID,
			state.TickLower,
			state.TickUpper,
			currentLiquidity,
			isActive,
			smartLPActiveTimeOrZero(state.OpenedAt),
			smartLPActiveTimeOrZero(state.LastAddAt),
			smartLPActiveTimeOrZero(state.LastRemoveAt),
			state.LastEventSeq,
			state.LastEventBlock,
			smartLPActiveTimeOrZero(state.LastEventTs),
			state.Source,
			smartLPActiveTimeOrZero(state.UpdatedAt),
		); err != nil {
			return err
		}
	}
	return batch.Send()
}

func (s *ClickHouseService) UpsertSmartLPActivePositionsFromEvents(ctx context.Context, events []SmartLPActivePositionEvent) error {
	if s == nil || s.Conn == nil || len(events) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	s.smartLPStateMu.Lock()
	defer s.smartLPStateMu.Unlock()

	type keyedEvent struct {
		key string
		ev  SmartLPActivePositionEvent
	}
	keyed := make([]keyedEvent, 0, len(events))
	keySet := make(map[string]struct{}, len(events))
	for _, raw := range events {
		ev, key, ok := normalizeSmartLPActivePositionEvent(raw)
		if !ok {
			continue
		}
		keyed = append(keyed, keyedEvent{key: key, ev: ev})
		keySet[key] = struct{}{}
	}
	if len(keyed) == 0 {
		return nil
	}

	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	states, err := s.loadSmartLPActivePositionStates(ctx, keys)
	if err != nil {
		return fmt.Errorf("load smart_lp_active_positions state failed: %w", err)
	}

	sort.Slice(keyed, func(i, j int) bool {
		if keyed[i].ev.EventSeq == keyed[j].ev.EventSeq {
			if keyed[i].key == keyed[j].key {
				return keyed[i].ev.BlockNumber < keyed[j].ev.BlockNumber
			}
			return keyed[i].key < keyed[j].key
		}
		return keyed[i].ev.EventSeq < keyed[j].ev.EventSeq
	})

	now := time.Now().UTC()
	touched := make(map[string]smartLPActivePositionState, len(keys))
	for _, item := range keyed {
		next, changed := applySmartLPActivePositionEvent(states[item.key], item.ev, item.key, now)
		if !changed {
			continue
		}
		states[item.key] = next
		touched[item.key] = next
	}
	if len(touched) == 0 {
		return nil
	}

	updates := make([]smartLPActivePositionState, 0, len(touched))
	for _, state := range touched {
		updates = append(updates, state)
	}
	sort.Slice(updates, func(i, j int) bool {
		if updates[i].LastEventSeq == updates[j].LastEventSeq {
			return updates[i].PositionKey < updates[j].PositionKey
		}
		return updates[i].LastEventSeq < updates[j].LastEventSeq
	})
	if err := s.insertSmartLPActivePositionStates(ctx, updates); err != nil {
		return fmt.Errorf("insert smart_lp_active_positions failed: %w", err)
	}
	return nil
}

func (s *ClickHouseService) ReconcileSmartLPActivePositionsFromRecentEvents(ctx context.Context, window time.Duration) (int, error) {
	if s == nil || s.Conn == nil {
		return 0, fmt.Errorf("clickhouse not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if window <= 0 {
		window = 48 * time.Hour
	}
	seconds := int(window.Seconds())
	if seconds <= 0 {
		seconds = 48 * 3600
	}

	s.smartLPStateMu.Lock()
	defer s.smartLPStateMu.Unlock()

	q := fmt.Sprintf(`
		SELECT
			dedup_ts AS ts,
			dedup_event_seq AS event_seq,
			dedup_chain AS chain,
			dedup_pool_version AS pool_version,
			dedup_pool_id AS pool_id,
			dedup_wallet_address AS wallet_address,
			dedup_action AS action,
			dedup_token_id AS token_id,
			dedup_liquidity_delta AS liquidity_delta,
			dedup_tick_lower AS tick_lower,
			dedup_tick_upper AS tick_upper,
			dedup_block_number AS block_number,
			dedup_contract_address AS contract_address,
			dedup_source AS source
		FROM (
			SELECT
				argMax(ts, event_seq) AS dedup_ts,
				max(event_seq) AS dedup_event_seq,
				argMax(chain, event_seq) AS dedup_chain,
				argMax(pool_version, event_seq) AS dedup_pool_version,
				argMax(pool_id, event_seq) AS dedup_pool_id,
				argMax(wallet_address, event_seq) AS dedup_wallet_address,
				argMax(action, event_seq) AS dedup_action,
				argMax(token_id, event_seq) AS dedup_token_id,
				argMax(liquidity_delta, event_seq) AS dedup_liquidity_delta,
				argMax(tick_lower, event_seq) AS dedup_tick_lower,
				argMax(tick_upper, event_seq) AS dedup_tick_upper,
				argMax(block_number, event_seq) AS dedup_block_number,
				argMax(contract_address, event_seq) AS dedup_contract_address,
				argMax(source, event_seq) AS dedup_source,
				tx_hash,
				log_index
			FROM %s
			WHERE ts >= now() - INTERVAL %d SECOND
				AND action IN ('add', 'remove')
				AND pool_version != ''
				AND pool_id != ''
				AND wallet_address != ''
			GROUP BY tx_hash, log_index
		)
		ORDER BY event_seq ASC
	`, smartLPEventsTable, seconds)

	rows, err := s.Conn.Query(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("query recent smart_lp_events for reconcile failed: %w", err)
	}
	defer rows.Close()

	states := make(map[string]smartLPActivePositionState)
	now := time.Now().UTC()
	for rows.Next() {
		var ev SmartLPActivePositionEvent
		if err := rows.Scan(
			&ev.Ts,
			&ev.EventSeq,
			&ev.Chain,
			&ev.PoolVersion,
			&ev.PoolID,
			&ev.WalletAddress,
			&ev.Action,
			&ev.TokenID,
			&ev.LiquidityDelta,
			&ev.TickLower,
			&ev.TickUpper,
			&ev.BlockNumber,
			&ev.ContractAddress,
			&ev.Source,
		); err != nil {
			return 0, err
		}
		ev, key, ok := normalizeSmartLPActivePositionEvent(ev)
		if !ok {
			continue
		}
		next, changed := applySmartLPActivePositionEvent(states[key], ev, key, now)
		if !changed {
			continue
		}
		states[key] = next
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(states) == 0 {
		return 0, nil
	}

	updates := make([]smartLPActivePositionState, 0, len(states))
	for _, state := range states {
		updates = append(updates, state)
	}
	sort.Slice(updates, func(i, j int) bool {
		if updates[i].LastEventSeq == updates[j].LastEventSeq {
			return updates[i].PositionKey < updates[j].PositionKey
		}
		return updates[i].LastEventSeq < updates[j].LastEventSeq
	})

	if err := s.insertSmartLPActivePositionStates(ctx, updates); err != nil {
		return 0, fmt.Errorf("reconcile smart_lp_active_positions insert failed: %w", err)
	}
	return len(updates), nil
}
