package clickhouse

import (
	"TgLpBot/base/config"
	"TgLpBot/base/timeutil"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type ClickHouseService struct {
	Conn driver.Conn
}

func inferClickHouseProtocol(addr string) clickhouse.Protocol {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return clickhouse.HTTP
	}

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return clickhouse.HTTP
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		return clickhouse.HTTP
	}

	// Some deployments expose ClickHouse behind NodePort/NAT offsets (e.g. 19000->9000, 18123->8123).
	switch p % 10000 {
	case 9000, 9440:
		return clickhouse.Native
	case 8123, 8443:
		return clickhouse.HTTP
	default:
		return clickhouse.HTTP
	}
}

func NewClickHouseService(addr, db, user, password, protocol string, debug bool) (*ClickHouseService, error) {
	timeutil.Init()

	chProtocol := clickhouse.HTTP
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "":
		chProtocol = inferClickHouseProtocol(addr)
	case "native":
		chProtocol = clickhouse.Native
	case "http":
		chProtocol = clickhouse.HTTP
	default:
		return nil, fmt.Errorf("unsupported CLICKHOUSE_PROTOCOL=%q (expected: native|http)", protocol)
	}

	var transportFunc func(*http.Transport) (http.RoundTripper, error)
	if chProtocol == clickhouse.HTTP {
		transportFunc = func(t *http.Transport) (http.RoundTripper, error) {
			// Prevent stale keep-alive connections from causing intermittent `EOF` on POST
			// (common when ClickHouse is behind a LB/proxy with a short idle timeout).
			t.IdleConnTimeout = 25 * time.Second
			return t, nil
		}
	}

	maxOpenConns := 50
	maxIdleConns := 10
	dialTimeout := 60 * time.Second
	if config.AppConfig != nil {
		if config.AppConfig.ClickHouseMaxOpenConns > 0 {
			maxOpenConns = config.AppConfig.ClickHouseMaxOpenConns
		}
		if config.AppConfig.ClickHouseMaxIdleConns > 0 {
			maxIdleConns = config.AppConfig.ClickHouseMaxIdleConns
		}
		if config.AppConfig.ClickHouseDialTimeoutSeconds > 0 {
			dialTimeout = time.Duration(config.AppConfig.ClickHouseDialTimeoutSeconds) * time.Second
		}
	}
	if maxIdleConns < 0 {
		maxIdleConns = 0
	}
	if maxOpenConns <= 0 {
		maxOpenConns = maxIdleConns + 10
	}
	if maxOpenConns < maxIdleConns {
		maxOpenConns = maxIdleConns
	}
	if dialTimeout <= 0 {
		dialTimeout = 60 * time.Second
	}

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{
			Database: db,
			Username: user,
			Password: password,
		},
		Protocol:      chProtocol,
		TransportFunc: transportFunc,
		Debug:         debug,
		DialTimeout:   dialTimeout,
		MaxOpenConns:  maxOpenConns,
		MaxIdleConns:  maxIdleConns,
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

func (s *ClickHouseService) PrepareBatch(ctx context.Context, query string) (driver.Batch, error) {
	if s == nil || s.Conn == nil {
		return nil, fmt.Errorf("clickhouse not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	batch, err := s.Conn.PrepareBatch(ctx, query)
	if err == nil {
		return batch, nil
	}
	if !isRetryableClickHouseError(err) {
		return nil, err
	}

	// Retry once on transient network errors (e.g. stale HTTP keep-alive causing EOF).
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}
	return s.Conn.PrepareBatch(ctx, query)
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
			requested_chain LowCardinality(String),
			protocol_version LowCardinality(String),
			dex LowCardinality(String),
			timeframe_minutes UInt16,
			timeframe_label String,
			requested_protocols Array(String),
			requested_dexes Array(String),
			total_pools UInt32,
			response_success UInt8,
			response_error String,
			pool_address String,
			factory_name LowCardinality(String),
			factory_address String,
			trading_pair String,
			token0_symbol LowCardinality(String),
			token1_symbol LowCardinality(String),
			token0_name String,
			token1_name String,
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
		`ALTER TABLE poolm_top_fees_raw ADD COLUMN IF NOT EXISTS requested_chain LowCardinality(String)`,
		`ALTER TABLE poolm_top_fees_raw ADD COLUMN IF NOT EXISTS dex LowCardinality(String)`,
		`ALTER TABLE poolm_top_fees_raw ADD COLUMN IF NOT EXISTS timeframe_label String`,
		`ALTER TABLE poolm_top_fees_raw ADD COLUMN IF NOT EXISTS requested_protocols Array(String)`,
		`ALTER TABLE poolm_top_fees_raw ADD COLUMN IF NOT EXISTS requested_dexes Array(String)`,
		`ALTER TABLE poolm_top_fees_raw ADD COLUMN IF NOT EXISTS total_pools UInt32`,
		`ALTER TABLE poolm_top_fees_raw ADD COLUMN IF NOT EXISTS response_success UInt8`,
		`ALTER TABLE poolm_top_fees_raw ADD COLUMN IF NOT EXISTS response_error String`,
		`ALTER TABLE poolm_top_fees_raw ADD COLUMN IF NOT EXISTS token0_name String`,
		`ALTER TABLE poolm_top_fees_raw ADD COLUMN IF NOT EXISTS token1_name String`,
		`
		CREATE TABLE IF NOT EXISTS poolm_top_fees_realtime (
			ts DateTime,
			chain LowCardinality(String),
			protocol_version LowCardinality(String),
			timeframe_minutes UInt16,
			dex LowCardinality(String),
			pool_address String,
			factory_name LowCardinality(String),
			trading_pair String,
			fee_percentage Float64,
			transaction_count UInt32,
			total_fees Float64,
			total_volume Float64,
			current_pool_value Float64,
			price_display String,
			last_swap_at DateTime
		) ENGINE = MergeTree
		PARTITION BY tuple()
		ORDER BY (chain, timeframe_minutes, protocol_version, pool_address)
		TTL ts + INTERVAL 2 HOUR
		`,
		`ALTER TABLE poolm_top_fees_realtime MODIFY TTL ts + INTERVAL 2 HOUR`,
		`ALTER TABLE poolm_top_fees_realtime ADD COLUMN IF NOT EXISTS factory_name LowCardinality(String)`,
		`ALTER TABLE poolm_top_fees_realtime ADD COLUMN IF NOT EXISTS transaction_count UInt32`,
		`ALTER TABLE poolm_top_fees_realtime ADD COLUMN IF NOT EXISTS token0_address String`,
		`ALTER TABLE poolm_top_fees_realtime ADD COLUMN IF NOT EXISTS token1_address String`,
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
		`
		CREATE TABLE IF NOT EXISTS smart_lp_events (
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
		ORDER BY (tx_hash, log_index)
		TTL ts + INTERVAL 15 DAY
		`,
		`ALTER TABLE smart_lp_events ADD COLUMN IF NOT EXISTS net_amount0 String DEFAULT '0'`,
		`ALTER TABLE smart_lp_events ADD COLUMN IF NOT EXISTS net_amount1 String DEFAULT '0'`,
		`ALTER TABLE smart_lp_events ADD COLUMN IF NOT EXISTS tick_lower Int32 DEFAULT 0`,
		`ALTER TABLE smart_lp_events ADD COLUMN IF NOT EXISTS tick_upper Int32 DEFAULT 0`,
		`ALTER TABLE smart_lp_events MODIFY TTL ts + INTERVAL 15 DAY`,
		`
		CREATE TABLE IF NOT EXISTS smart_lp_scan_state (
			id UInt8,
			last_block UInt64,
			updated_at DateTime
		) ENGINE = ReplacingMergeTree(updated_at)
		ORDER BY id
		`,
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
