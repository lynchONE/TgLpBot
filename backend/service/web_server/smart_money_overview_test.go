package web_server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"TgLpBot/service/smart_lp"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type fakeRows struct {
	idx  int
	data []smartMoneyCashflowRow
	err  error
}

func (r *fakeRows) Next() bool {
	if r == nil {
		return false
	}
	if r.idx >= len(r.data) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	if r == nil {
		return nil
	}
	if r.idx <= 0 || r.idx > len(r.data) {
		return nil
	}
	row := r.data[r.idx-1]
	// Matches querySmartMoneyCashflows scan order.
	*dest[0].(*string) = row.WalletAddress
	*dest[1].(*string) = row.PoolVersion
	*dest[2].(*string) = row.PoolID
	*dest[3].(*string) = row.Action
	*dest[4].(*string) = row.Sum0
	*dest[5].(*string) = row.Sum1
	*dest[6].(*uint64) = row.EventCount
	return nil
}

func (r *fakeRows) ScanStruct(dest any) error { return nil }
func (r *fakeRows) ColumnTypes() []driver.ColumnType {
	return nil
}
func (r *fakeRows) Totals(dest ...any) error { return nil }
func (r *fakeRows) Columns() []string        { return nil }
func (r *fakeRows) Close() error             { return nil }
func (r *fakeRows) Err() error               { return r.err }

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

func TestQuerySmartMoneyCashflows_ParsesRowsAndUsesNetAmounts(t *testing.T) {
	conn := &fakeCHConn{
		rows: &fakeRows{
			data: []smartMoneyCashflowRow{
				{
					WalletAddress: "0xabc",
					PoolVersion:   "v3",
					PoolID:        "0xpool",
					Action:        "add",
					Sum0:          "100",
					Sum1:          "200",
					EventCount:    3,
				},
			},
		},
	}

	pools := []smart_lp.SmartLPPoolKey{
		{PoolVersion: "v3", PoolID: "0xpool"},
	}
	wallets := []string{"0xabc"}

	rows, err := querySmartMoneyCashflows(context.Background(), conn, "bsc", pools, wallets, 24*time.Hour)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].WalletAddress != "0xabc" || rows[0].Sum0 != "100" || rows[0].Sum1 != "200" || rows[0].EventCount != 3 {
		t.Fatalf("unexpected row: %+v", rows[0])
	}

	if !strings.Contains(conn.lastQuery, "net_amount0") || !strings.Contains(conn.lastQuery, "net_amount1") {
		t.Fatalf("expected query to reference net_amount columns, got: %s", conn.lastQuery)
	}
	if len(conn.lastArgs) == 0 {
		t.Fatalf("expected query args")
	}
}

func TestHandleSmartMoneyOverview_MethodNotAllowed(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/smart_money_overview", nil)
	w := httptest.NewRecorder()
	srv.handleSmartMoneyOverview(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d body=%q", http.StatusMethodNotAllowed, w.Code, w.Body.String())
	}
}

func TestHandleSmartMoneyOverview_ClickHouseNotConfigured(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/smart_money_overview", nil)
	w := httptest.NewRecorder()
	srv.handleSmartMoneyOverview(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected %d, got %d body=%q", http.StatusServiceUnavailable, w.Code, w.Body.String())
	}
}
