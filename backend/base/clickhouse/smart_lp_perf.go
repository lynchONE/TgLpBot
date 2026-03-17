package clickhouse

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

const smartLPEventsTable = "smart_lp_events"
const smartLPRollupTable = "smart_lp_rollup_5m"
const smartLPRollupView = "smart_lp_rollup_5m_mv"

func smartLPEventsCreateTableSQL(tableName string) string {
	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			ts DateTime,
			event_seq UInt64,
			chain LowCardinality(String),
			pool_version LowCardinality(String),
			pool_id String,
			wallet_address String,
			action LowCardinality(String),
			token_id String,
			amount0 String,
			amount1 String,
			net_amount0 String,
			net_amount1 String,
			liquidity_delta String,
			tick_lower Int32,
			tick_upper Int32,
			tx_hash String,
			block_number UInt64,
			log_index UInt32,
			contract_address String,
			source LowCardinality(String),
			ingest_id UUID DEFAULT generateUUIDv4()
		) ENGINE = ReplacingMergeTree(ts)
		PARTITION BY toDate(ts)
		ORDER BY (chain, pool_version, pool_id, wallet_address, ts, event_seq, tx_hash, log_index)
		TTL ts + INTERVAL 2 DAY
	`, tableName)
}

func smartLPRollupCreateTableSQL() string {
	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			bucket DateTime,
			chain LowCardinality(String),
			pool_version LowCardinality(String),
			pool_id String,
			wallet_address String,
			action LowCardinality(String),
			event_count UInt64,
			sum0 Int256,
			sum1 Int256,
			add_liquidity Float64,
			net_liquidity Int256
		) ENGINE = MergeTree
		PARTITION BY toDate(bucket)
		ORDER BY (chain, pool_version, pool_id, wallet_address, bucket, action)
		TTL bucket + INTERVAL 2 DAY
	`, smartLPRollupTable)
}

func smartLPRollupMVSQL() string {
	return fmt.Sprintf(`
		CREATE MATERIALIZED VIEW %s TO %s AS
		SELECT
			toStartOfInterval(ts, INTERVAL 5 MINUTE) AS bucket,
			chain,
			pool_version,
			pool_id,
			wallet_address,
			action,
			count() AS event_count,
			sum(toInt256OrZero(if(net_amount0 != '' AND net_amount0 != '0', net_amount0, amount0))) AS sum0,
			sum(toInt256OrZero(if(net_amount1 != '' AND net_amount1 != '0', net_amount1, amount1))) AS sum1,
			sumIf(toFloat64OrZero(liquidity_delta), action = 'add') AS add_liquidity,
			sum(
				if(pool_version = 'v4',
					toInt256OrZero(liquidity_delta),
					if(action = 'add', toInt256OrZero(liquidity_delta), -toInt256OrZero(liquidity_delta))
				)
			) AS net_liquidity
		FROM %s
		WHERE action IN ('add', 'remove')
			AND pool_version != ''
			AND pool_id != ''
			AND wallet_address != ''
		GROUP BY bucket, chain, pool_version, pool_id, wallet_address, action
	`, smartLPRollupView, smartLPRollupTable, smartLPEventsTable)
}

func smartLPRollupBackfillSQL() string {
	return fmt.Sprintf(`
		INSERT INTO %s (
			bucket, chain, pool_version, pool_id, wallet_address, action,
			event_count, sum0, sum1, add_liquidity, net_liquidity
		)
		SELECT
			toStartOfInterval(ts, INTERVAL 5 MINUTE) AS bucket,
			chain,
			pool_version,
			pool_id,
			wallet_address,
			action,
			count() AS event_count,
			sum(toInt256OrZero(if(net_amount0 != '' AND net_amount0 != '0', net_amount0, amount0))) AS sum0,
			sum(toInt256OrZero(if(net_amount1 != '' AND net_amount1 != '0', net_amount1, amount1))) AS sum1,
			sumIf(toFloat64OrZero(liquidity_delta), action = 'add') AS add_liquidity,
			sum(
				if(pool_version = 'v4',
					toInt256OrZero(liquidity_delta),
					if(action = 'add', toInt256OrZero(liquidity_delta), -toInt256OrZero(liquidity_delta))
				)
			) AS net_liquidity
		FROM %s
		WHERE ts >= now() - INTERVAL 2 DAY
			AND action IN ('add', 'remove')
			AND pool_version != ''
			AND pool_id != ''
			AND wallet_address != ''
		GROUP BY bucket, chain, pool_version, pool_id, wallet_address, action
	`, smartLPRollupTable, smartLPEventsTable)
}

func normalizeDDL(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func smartLPEventsDDLUpToDate(createQuery string) bool {
	norm := normalizeDDL(createQuery)
	return strings.Contains(norm, "order by (chain, pool_version, pool_id, wallet_address, ts, event_seq, tx_hash, log_index)") &&
		strings.Contains(norm, "ttl ts + interval 2 day")
}

func (s *ClickHouseService) ensureSmartLPSchema(ctx context.Context) error {
	if s == nil || s.Conn == nil {
		return fmt.Errorf("clickhouse not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.ensureSmartLPEventsSchema(ctx); err != nil {
		return err
	}
	if err := s.ensureSmartLPRollup(ctx); err != nil {
		return err
	}
	if err := s.ensureSmartLPActivePositions(ctx); err != nil {
		return err
	}
	return nil
}

func (s *ClickHouseService) ensureSmartLPEventsSchema(ctx context.Context) error {
	var exists uint8
	if err := s.Conn.QueryRow(ctx, "EXISTS TABLE smart_lp_events").Scan(&exists); err != nil {
		return fmt.Errorf("clickhouse check smart_lp_events exists failed: %w", err)
	}
	if exists == 0 {
		return nil
	}

	var createQuery string
	if err := s.Conn.QueryRow(ctx, "SHOW CREATE TABLE smart_lp_events").Scan(&createQuery); err != nil {
		return fmt.Errorf("clickhouse show create smart_lp_events failed: %w", err)
	}
	if smartLPEventsDDLUpToDate(createQuery) {
		return nil
	}

	log.Printf("[ClickHouse] rebuilding %s with 2-day TTL and query-oriented ORDER BY", smartLPEventsTable)
	_ = s.Conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", smartLPRollupView))

	tempName := fmt.Sprintf("%s_rebuild_%d", smartLPEventsTable, time.Now().Unix())
	backupName := fmt.Sprintf("%s_backup_%d", smartLPEventsTable, time.Now().Unix())
	if err := s.Conn.Exec(ctx, smartLPEventsCreateTableSQL(tempName)); err != nil {
		return fmt.Errorf("clickhouse create rebuilt smart_lp_events failed: %w", err)
	}

	copyQ := fmt.Sprintf(`
		INSERT INTO %s (
			ts, event_seq, chain, pool_version, pool_id, wallet_address, action,
			token_id, amount0, amount1, net_amount0, net_amount1, liquidity_delta,
			tick_lower, tick_upper, tx_hash, block_number, log_index, contract_address, source
		)
		SELECT
			ts, event_seq, chain, pool_version, pool_id, wallet_address, action,
			token_id, amount0, amount1, net_amount0, net_amount1, liquidity_delta,
			tick_lower, tick_upper, tx_hash, block_number, log_index, contract_address, source
		FROM %s
		WHERE ts >= now() - INTERVAL 2 DAY
	`, tempName, smartLPEventsTable)
	if err := s.Conn.Exec(ctx, copyQ); err != nil {
		_ = s.Conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tempName))
		return fmt.Errorf("clickhouse backfill rebuilt smart_lp_events failed: %w", err)
	}

	renameQ := fmt.Sprintf("RENAME TABLE %s TO %s, %s TO %s", smartLPEventsTable, backupName, tempName, smartLPEventsTable)
	if err := s.Conn.Exec(ctx, renameQ); err != nil {
		_ = s.Conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tempName))
		return fmt.Errorf("clickhouse swap smart_lp_events failed: %w", err)
	}
	_ = s.Conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", backupName))
	return nil
}

func (s *ClickHouseService) ensureSmartLPRollup(ctx context.Context) error {
	if err := s.Conn.Exec(ctx, smartLPRollupCreateTableSQL()); err != nil {
		return fmt.Errorf("clickhouse create smart_lp rollup table failed: %w", err)
	}
	if err := s.Conn.Exec(ctx, fmt.Sprintf("ALTER TABLE %s MODIFY TTL bucket + INTERVAL 2 DAY", smartLPRollupTable)); err != nil {
		return fmt.Errorf("clickhouse alter smart_lp rollup ttl failed: %w", err)
	}

	if err := s.Conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", smartLPRollupView)); err != nil {
		return fmt.Errorf("clickhouse drop smart_lp rollup mv failed: %w", err)
	}
	if err := s.Conn.Exec(ctx, smartLPRollupMVSQL()); err != nil {
		return fmt.Errorf("clickhouse create smart_lp rollup mv failed: %w", err)
	}

	var count uint64
	if err := s.Conn.QueryRow(ctx, fmt.Sprintf("SELECT count() FROM %s", smartLPRollupTable)).Scan(&count); err != nil {
		return fmt.Errorf("clickhouse count smart_lp rollup rows failed: %w", err)
	}
	if count > 0 {
		return nil
	}

	log.Printf("[ClickHouse] backfilling %s from %s (last 2 days)", smartLPRollupTable, smartLPEventsTable)
	if err := s.Conn.Exec(ctx, smartLPRollupBackfillSQL()); err != nil {
		return fmt.Errorf("clickhouse backfill smart_lp rollup failed: %w", err)
	}
	return nil
}
