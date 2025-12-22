package services

import (
	"context"
	"fmt"

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
	// Pools Table - Using ReplacingMergeTree for updates
	// Ordering by ID allows efficient deduplication for specific pools
	query := `
	CREATE TABLE IF NOT EXISTS pools (
		id String,
		type String,
		address String,
		name String,
		
		base_token_id String,
		quote_token_id String,
		dex_id String,

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

		pool_fee_percentage Float64 DEFAULT 0,
		fee_usd_m5 Float64 DEFAULT 0,
		fee_usd_h1 Float64 DEFAULT 0,
		fee_usd_h6 Float64 DEFAULT 0,
		fee_usd_h24 Float64 DEFAULT 0,
		fee_apr_m5 Float64 DEFAULT 0,
		fee_apr_h1 Float64 DEFAULT 0,
		fee_apr_h6 Float64 DEFAULT 0,
		fee_apr_h24 Float64 DEFAULT 0,
		
		transactions_h24_buys UInt32,
		transactions_h24_sells UInt32,
		transactions_h24_buyers UInt32,
		transactions_h24_sellers UInt32,

		updated_at DateTime DEFAULT now()
	) ENGINE = ReplacingMergeTree(updated_at)
	ORDER BY id
	`
	if err := s.Conn.Exec(ctx, query); err != nil {
		return fmt.Errorf("failed to create pools table: %w", err)
	}

	alterQueries := []string{
		`ALTER TABLE pools ADD COLUMN IF NOT EXISTS pool_fee_percentage Float64 DEFAULT 0`,
		`ALTER TABLE pools ADD COLUMN IF NOT EXISTS fee_usd_m5 Float64 DEFAULT 0`,
		`ALTER TABLE pools ADD COLUMN IF NOT EXISTS fee_usd_h1 Float64 DEFAULT 0`,
		`ALTER TABLE pools ADD COLUMN IF NOT EXISTS fee_usd_h6 Float64 DEFAULT 0`,
		`ALTER TABLE pools ADD COLUMN IF NOT EXISTS fee_usd_h24 Float64 DEFAULT 0`,
		`ALTER TABLE pools ADD COLUMN IF NOT EXISTS fee_apr_m5 Float64 DEFAULT 0`,
		`ALTER TABLE pools ADD COLUMN IF NOT EXISTS fee_apr_h1 Float64 DEFAULT 0`,
		`ALTER TABLE pools ADD COLUMN IF NOT EXISTS fee_apr_h6 Float64 DEFAULT 0`,
		`ALTER TABLE pools ADD COLUMN IF NOT EXISTS fee_apr_h24 Float64 DEFAULT 0`,
	}

	for _, q := range alterQueries {
		if err := s.Conn.Exec(ctx, q); err != nil {
			return fmt.Errorf("failed to alter pools table: %w", err)
		}
	}

	return nil
}
