package smart_money_golden_dog

import (
	"TgLpBot/service/pool"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type fakeRows struct {
	err error
}

func (r *fakeRows) Next() bool                       { return false }
func (r *fakeRows) Scan(dest ...any) error           { return nil }
func (r *fakeRows) ScanStruct(dest any) error        { return nil }
func (r *fakeRows) ColumnTypes() []driver.ColumnType { return nil }
func (r *fakeRows) Totals(dest ...any) error         { return nil }
func (r *fakeRows) Columns() []string                { return nil }
func (r *fakeRows) Close() error                     { return nil }
func (r *fakeRows) Err() error                       { return r.err }

type fakeCHConn struct {
	lastQuery string
	lastArgs  []any
	rows      driver.Rows
	err       error
}

func (c *fakeCHConn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	c.lastQuery = query
	c.lastArgs = args
	return c.rows, c.err
}

func TestQueryGoldenDogPoolPositions_BuildsExpectedQueryAndArgs(t *testing.T) {
	conn := &fakeCHConn{rows: &fakeRows{}}
	_, err := queryGoldenDogPoolPositions(context.Background(), conn, "bsc", 10, 200)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !strings.Contains(conn.lastQuery, "count() AS active_position_count") {
		t.Fatalf("unexpected query: %s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "action IN ('add', 'remove')") {
		t.Fatalf("unexpected query: %s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "HAVING net_liquidity > 0") {
		t.Fatalf("unexpected query: %s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "GROUP BY pool_version, pool_id, contract_address, token_id, wallet_address, tick_lower, tick_upper") {
		t.Fatalf("unexpected query: %s", conn.lastQuery)
	}
	if strings.Contains(conn.lastQuery, "HAVING wallet_count >= ?") {
		t.Fatalf("unexpected query: %s", conn.lastQuery)
	}

	if len(conn.lastArgs) != 3 {
		t.Fatalf("expected 3 args, got %d", len(conn.lastArgs))
	}
	if _, ok := conn.lastArgs[0].(time.Time); !ok {
		t.Fatalf("expected arg0 to be time.Time, got %T", conn.lastArgs[0])
	}
	if conn.lastArgs[1] != "bsc" {
		t.Fatalf("expected arg1 to be chain=bsc, got %#v", conn.lastArgs[1])
	}
	if conn.lastArgs[2] != uint64(200) {
		t.Fatalf("expected arg2 to be limit=200, got %#v", conn.lastArgs[2])
	}
}

func TestAggregateGoldenDogPairAlerts_MergesPoolsByPairWithoutPositionDedupe(t *testing.T) {
	rows := []goldenDogPoolPositionRow{
		{
			PoolVersion:   "v3",
			PoolID:        "0xpool-a",
			PositionCount: 2,
		},
		{
			PoolVersion:   "v3",
			PoolID:        "0xpool-b",
			PositionCount: 2,
		},
		{
			PoolVersion:   "v3",
			PoolID:        "0xpool-c",
			PositionCount: 1,
		},
	}

	resolve := func(chain string, poolVersion string, poolID string) (*pool.PoolInfo, error) {
		switch poolID {
		case "0xpool-a", "0xpool-b":
			return &pool.PoolInfo{
				Token0:       "0x0000000000000000000000000000000000000011",
				Token1:       "0x0000000000000000000000000000000000000022",
				Token0Symbol: "USDT",
				Token1Symbol: "BULLA",
			}, nil
		case "0xpool-c":
			return &pool.PoolInfo{
				Token0:       "0x0000000000000000000000000000000000000033",
				Token1:       "0x0000000000000000000000000000000000000044",
				Token0Symbol: "USDT",
				Token1Symbol: "DRAGON",
			}, nil
		default:
			return nil, nil
		}
	}

	alerts := aggregateGoldenDogPairAlerts("bsc", rows, 3, resolve)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].AlertScope != "pair" {
		t.Fatalf("expected alert scope pair, got %q", alerts[0].AlertScope)
	}
	if alerts[0].PairLabel != "USDT/BULLA" {
		t.Fatalf("expected pair label USDT/BULLA, got %q", alerts[0].PairLabel)
	}
	if alerts[0].WalletCount != 4 {
		t.Fatalf("expected wallet count 4, got %d", alerts[0].WalletCount)
	}
	if alerts[0].AlertKey == "" {
		t.Fatalf("expected non-empty alert key")
	}
}
