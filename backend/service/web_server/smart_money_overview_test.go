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

type fakeEventTrendRows struct {
	idx  int
	data []smartMoneyEventTrendRow
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

func (r *fakeEventTrendRows) Next() bool {
	if r == nil {
		return false
	}
	if r.idx >= len(r.data) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeEventTrendRows) Scan(dest ...any) error {
	if r == nil {
		return nil
	}
	if r.idx <= 0 || r.idx > len(r.data) {
		return nil
	}
	row := r.data[r.idx-1]
	*dest[0].(*int32) = row.HoursAgo
	*dest[1].(*uint64) = row.AddEvents
	*dest[2].(*uint64) = row.RemoveEvents
	*dest[3].(*uint64) = row.DistinctWallet
	return nil
}

func (r *fakeEventTrendRows) ScanStruct(dest any) error { return nil }
func (r *fakeEventTrendRows) ColumnTypes() []driver.ColumnType {
	return nil
}
func (r *fakeEventTrendRows) Totals(dest ...any) error { return nil }
func (r *fakeEventTrendRows) Columns() []string        { return nil }
func (r *fakeEventTrendRows) Close() error             { return nil }
func (r *fakeEventTrendRows) Err() error               { return r.err }

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

func TestQuerySmartMoneyEventTrend_BuildsHourlyAggregationQuery(t *testing.T) {
	conn := &fakeCHConn{
		rows: &fakeEventTrendRows{
			data: []smartMoneyEventTrendRow{
				{HoursAgo: 2, AddEvents: 5, RemoveEvents: 1, DistinctWallet: 3},
			},
		},
	}

	pools := []smart_lp.SmartLPPoolKey{
		{PoolVersion: "v3", PoolID: "0xpool"},
	}

	rows, err := querySmartMoneyEventTrend(context.Background(), conn, "bsc", pools, 24*time.Hour)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].HoursAgo != 2 || rows[0].AddEvents != 5 || rows[0].RemoveEvents != 1 || rows[0].DistinctWallet != 3 {
		t.Fatalf("unexpected row: %+v", rows[0])
	}

	if !strings.Contains(conn.lastQuery, "hours_ago") || !strings.Contains(conn.lastQuery, "sum(if(action='add'") {
		t.Fatalf("unexpected trend query: %s", conn.lastQuery)
	}
}

func TestBuildSmartMoneySummary_AggregatesMetrics(t *testing.T) {
	pools := []smartMoneyOverviewPool{{PoolID: "a"}, {PoolID: "b"}}
	wallets := []smartMoneyOverviewWallet{
		{WalletAddress: "0x1", InUSDT24h: 200, OutUSDT24h: 100, PnLUSDT24h: 100, EventCount24h: 4, EventCount1h: 1},
		{WalletAddress: "0x2", InUSDT24h: 80, OutUSDT24h: 100, PnLUSDT24h: -20, EventCount24h: 2, EventCount1h: 0},
		{WalletAddress: "0x3", InUSDT24h: 0, OutUSDT24h: 0, PnLUSDT24h: 0, EventCount24h: 1, EventCount1h: 1},
	}

	summary := buildSmartMoneySummary(pools, wallets, 2)
	if summary.PoolCount != 2 || summary.WalletCount != 3 {
		t.Fatalf("unexpected counts: %+v", summary)
	}
	if summary.TotalInUSDT24h != 280 || summary.TotalOutUSDT24h != 200 || summary.TotalPnLUSDT24h != 80 {
		t.Fatalf("unexpected totals: %+v", summary)
	}
	if summary.PositiveWallets24h != 1 || summary.NegativeWallets24h != 1 || summary.ZeroWallets24h != 1 {
		t.Fatalf("unexpected wallet buckets: %+v", summary)
	}
	if summary.MissingPriceTokenCnt != 2 {
		t.Fatalf("unexpected missing token count: %+v", summary)
	}
	if summary.CoverageRatio24h <= 0.66 || summary.CoverageRatio24h > 0.67 {
		t.Fatalf("unexpected coverage ratio: %+v", summary)
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
