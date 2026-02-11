package smart_money_golden_dog

import (
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

func TestQueryGoldenDogPools_BuildsExpectedQueryAndArgs(t *testing.T) {
	conn := &fakeCHConn{rows: &fakeRows{}}
	_, err := queryGoldenDogPools(context.Background(), conn, "bsc", 10, 3, 50)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !strings.Contains(conn.lastQuery, "uniqExact(wallet_address)") {
		t.Fatalf("unexpected query: %s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "action = 'add'") {
		t.Fatalf("unexpected query: %s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "HAVING wallet_count >= ?") {
		t.Fatalf("unexpected query: %s", conn.lastQuery)
	}

	if len(conn.lastArgs) != 4 {
		t.Fatalf("expected 4 args, got %d", len(conn.lastArgs))
	}
	if _, ok := conn.lastArgs[0].(time.Time); !ok {
		t.Fatalf("expected arg0 to be time.Time, got %T", conn.lastArgs[0])
	}
	if conn.lastArgs[1] != "bsc" {
		t.Fatalf("expected arg1 to be chain=bsc, got %#v", conn.lastArgs[1])
	}
	if conn.lastArgs[2] != uint64(3) {
		t.Fatalf("expected arg2 to be minWallets=3, got %#v", conn.lastArgs[2])
	}
	if conn.lastArgs[3] != uint64(50) {
		t.Fatalf("expected arg3 to be limit=50, got %#v", conn.lastArgs[3])
	}
}
