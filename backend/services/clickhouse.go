package services

import (
	"TgLpBot/config"
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type ClickHouseService struct {
	Conn driver.Conn
}

func NewClickHouseService(addr, db, user, password string, debug bool) (*ClickHouseService, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{
			Database: db,
			Username: user,
			Password: password,
		},
		Protocol:     clickhouse.HTTP,
		Debug:        debug,
		MaxOpenConns: 20,
		MaxIdleConns: 5,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open clickhouse connection: %w", err)
	}

	ctx := context.Background()
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	service := &ClickHouseService{Conn: conn}
	if err := service.Migrate(ctx); err != nil {
		return nil, err
	}

	return service, nil
}

func (s *ClickHouseService) Migrate(ctx context.Context) error {
	if s == nil || s.Conn == nil {
		return fmt.Errorf("clickhouse not initialized")
	}

	if config.AppConfig != nil && config.AppConfig.ClickHouseResetAll {
		log.Println("⚠️  CLICKHOUSE_RESET_ALL=1: dropping all tables in current database...")
		if err := s.ResetAll(ctx); err != nil {
			return fmt.Errorf("reset clickhouse schema failed: %w", err)
		}
		log.Println("✅ ClickHouse reset completed")
	}

	queries := []string{
		`
		CREATE TABLE IF NOT EXISTS poolm_top_fees_raw (
			ts DateTime,
			chain LowCardinality(String),
			protocol_version LowCardinality(String),
			timeframe_minutes UInt16,
			pool_address String,
			factory_name LowCardinality(String),
			factory_address String,
			trading_pair String,
			token0_symbol LowCardinality(String),
			token1_symbol LowCardinality(String),
			token0_address String,
			token1_address String,
			token0_decimals UInt8,
			token1_decimals UInt8,
			stable_coin_symbol LowCardinality(String),
			fee_rate UInt32,
			fee_percentage Float64,
			transaction_count UInt32,
			total_fees Float64,
			total_volume Float64,
			current_pool_value Float64,
			current_token0_balance Float64,
			current_token1_balance Float64,
			current_token_price Float64,
			price_display String,
			last_swap_at DateTime,
			ingest_id UUID DEFAULT generateUUIDv4()
		) ENGINE = MergeTree
		PARTITION BY toDate(ts)
		ORDER BY (chain, protocol_version, timeframe_minutes, pool_address, ts)
		TTL ts + INTERVAL 24 HOUR
		`,
		`ALTER TABLE poolm_top_fees_raw MODIFY TTL ts + INTERVAL 24 HOUR`,
		`
		CREATE TABLE IF NOT EXISTS pools (
			id String,
			type LowCardinality(String),
			address String,
			name String,
			base_token_id String,
			quote_token_id String,
			dex_id LowCardinality(String),
			base_token_price_usd Float64,
			quote_token_price_usd Float64,
			base_token_price_native_currency Float64,
			quote_token_price_native_currency Float64,
			base_token_price_quote_token Float64,
			quote_token_price_base_token Float64,
			pool_created_at DateTime,
			fdv_usd Float64,
			market_cap_usd Float64,
			reserve_in_usd Float64,
			price_change_m5 Float64,
			price_change_h1 Float64,
			price_change_h6 Float64,
			price_change_h24 Float64,
			volume_m5 Float64,
			volume_h1 Float64,
			volume_h6 Float64,
			volume_h24 Float64,
			pool_fee_percentage Float64,
			fee_usd_m5 Float64,
			fee_usd_h1 Float64,
			fee_usd_h6 Float64,
			fee_usd_h24 Float64,
			fee_apr_m5 Float64,
			fee_apr_h1 Float64,
			fee_apr_h6 Float64,
			fee_apr_h24 Float64,
			transactions_h24_buys UInt32,
			transactions_h24_sells UInt32,
			transactions_h24_buyers UInt32,
			transactions_h24_sellers UInt32,
			updated_at DateTime
		) ENGINE = ReplacingMergeTree(updated_at)
		PARTITION BY tuple()
		ORDER BY id
		TTL updated_at + INTERVAL 24 HOUR
		`,
		`ALTER TABLE pools MODIFY TTL updated_at + INTERVAL 24 HOUR`,
		`
		CREATE TABLE IF NOT EXISTS auto_lp_analysis (
			ts DateTime,
			chain LowCardinality(String),
			protocol_version LowCardinality(String),
			pool_address String,
			trading_pair String,
			current_price Float64,
			ma_5 Float64,
			sigma_5 Float64,
			z_5 Float64,
			ma_60 Float64,
			sigma_60 Float64,
			z_60 Float64,
			state_5 LowCardinality(String),
			trend_60 LowCardinality(String),
			resonance LowCardinality(String),
			base_width_pct Float64,
			lower_width_pct Float64,
			upper_width_pct Float64,
			action LowCardinality(String),
			score Float64
		) ENGINE = MergeTree
		PARTITION BY toDate(ts)
		ORDER BY (chain, protocol_version, pool_address, ts)
		TTL ts + INTERVAL 24 HOUR
		`,
		`ALTER TABLE auto_lp_analysis MODIFY TTL ts + INTERVAL 24 HOUR`,
	}

	for _, q := range queries {
		if err := s.Conn.Exec(ctx, q); err != nil {
			return fmt.Errorf("clickhouse migrate failed: %w", err)
		}
	}

	return nil
}

var clickhouseIdent = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func (s *ClickHouseService) ResetAll(ctx context.Context) error {
	if s == nil || s.Conn == nil {
		return fmt.Errorf("clickhouse not initialized")
	}
	rows, err := s.Conn.Query(ctx, "SELECT name FROM system.tables WHERE database = currentDatabase()")
	if err != nil {
		return fmt.Errorf("list tables failed: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan table name failed: %w", err)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !clickhouseIdent.MatchString(name) {
			log.Printf("⚠️  Skip unexpected ClickHouse table name: %q", name)
			continue
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate tables failed: %w", err)
	}

	for _, name := range tables {
		q := fmt.Sprintf("DROP TABLE IF EXISTS `%s`", name)
		if err := s.Conn.Exec(ctx, q); err != nil {
			return fmt.Errorf("drop table %s failed: %w", name, err)
		}
	}
	return nil
}
