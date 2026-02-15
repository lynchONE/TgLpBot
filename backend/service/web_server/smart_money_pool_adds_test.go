package web_server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func TestHandleSmartMoneyPoolAdds_MethodNotAllowed(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/smart_money_pool_adds", nil)
	w := httptest.NewRecorder()
	srv.handleSmartMoneyPoolAdds(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d body=%q", http.StatusMethodNotAllowed, w.Code, w.Body.String())
	}
}

func TestHandleSmartMoneyPoolAdds_MissingPoolParams(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/smart_money_pool_adds", nil)
	w := httptest.NewRecorder()
	srv.handleSmartMoneyPoolAdds(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d body=%q", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestHandleSmartMoneyPoolAdds_InvalidPoolVersion(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/smart_money_pool_adds?pool_version=v2&pool_id=0x0000000000000000000000000000000000000000", nil)
	w := httptest.NewRecorder()
	srv.handleSmartMoneyPoolAdds(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d body=%q", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestHandleSmartMoneyPoolAdds_InvalidPoolIDV3(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/smart_money_pool_adds?pool_version=v3&pool_id=not-an-addr", nil)
	w := httptest.NewRecorder()
	srv.handleSmartMoneyPoolAdds(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d body=%q", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestHandleSmartMoneyPoolAdds_InvalidPoolIDV4(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/smart_money_pool_adds?pool_version=v4&pool_id=0x1234", nil)
	w := httptest.NewRecorder()
	srv.handleSmartMoneyPoolAdds(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d body=%q", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestHandleSmartMoneyPoolAdds_ClickHouseNotConfigured(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/smart_money_pool_adds?pool_version=v3&pool_id=0x0000000000000000000000000000000000000000", nil)
	w := httptest.NewRecorder()
	srv.handleSmartMoneyPoolAdds(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected %d, got %d body=%q", http.StatusServiceUnavailable, w.Code, w.Body.String())
	}
}

type fakePoolAddsRows struct {
	idx  int
	data []smartMoneyPoolAddRow
	err  error
}

func (r *fakePoolAddsRows) Next() bool {
	if r == nil {
		return false
	}
	if r.idx >= len(r.data) {
		return false
	}
	r.idx++
	return true
}

func (r *fakePoolAddsRows) Scan(dest ...any) error {
	if r == nil {
		return nil
	}
	if r.idx <= 0 || r.idx > len(r.data) {
		return nil
	}
	row := r.data[r.idx-1]
	*dest[0].(*string) = row.WalletAddress
	*dest[1].(*string) = row.ContractAddr
	*dest[2].(*string) = row.TokenID
	*dest[3].(*int32) = row.TickLower
	*dest[4].(*int32) = row.TickUpper
	*dest[5].(*string) = row.Sum0
	*dest[6].(*string) = row.Sum1
	*dest[7].(*uint64) = row.EventCount
	*dest[8].(*time.Time) = row.LastAt
	return nil
}

func (r *fakePoolAddsRows) ScanStruct(dest any) error        { return nil }
func (r *fakePoolAddsRows) ColumnTypes() []driver.ColumnType { return nil }
func (r *fakePoolAddsRows) Totals(dest ...any) error         { return nil }
func (r *fakePoolAddsRows) Columns() []string                { return nil }
func (r *fakePoolAddsRows) Close() error                     { return nil }
func (r *fakePoolAddsRows) Err() error                       { return r.err }

func TestQuerySmartMoneyPoolAdds_UsesNetLiquidityFilterV3(t *testing.T) {
	conn := &fakeCHConn{
		rows: &fakePoolAddsRows{
			data: []smartMoneyPoolAddRow{
				{
					WalletAddress: "0xabc",
					ContractAddr:  "0xpm",
					TokenID:       "1",
					TickLower:     -100,
					TickUpper:     100,
					Sum0:          "100",
					Sum1:          "200",
					EventCount:    1,
					LastAt:        time.Unix(1, 0).UTC(),
				},
			},
		},
	}

	rows, err := querySmartMoneyPoolAdds(context.Background(), conn, "bsc", "v3", "0xpool", 2*time.Hour, 10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	if !strings.Contains(conn.lastQuery, "action IN ('add', 'remove')") {
		t.Fatalf("expected add/remove filter, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "liquidity_delta") || !strings.Contains(strings.ToLower(conn.lastQuery), "having") {
		t.Fatalf("expected liquidity HAVING filter, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "toInt256OrZero(amount0)") || !strings.Contains(conn.lastQuery, "toInt256OrZero(amount1)") {
		t.Fatalf("expected amount0/amount1 aggregation, got query=%s", conn.lastQuery)
	}
}

func TestQuerySmartMoneyPoolAdds_UsesNetLiquidityFilterV4(t *testing.T) {
	conn := &fakeCHConn{
		rows: &fakePoolAddsRows{
			data: []smartMoneyPoolAddRow{
				{
					WalletAddress: "0xabc",
					ContractAddr:  "0xpm",
					TokenID:       "",
					TickLower:     -100,
					TickUpper:     100,
					Sum0:          "0",
					Sum1:          "0",
					EventCount:    1,
					LastAt:        time.Unix(1, 0).UTC(),
				},
			},
		},
	}

	_, err := querySmartMoneyPoolAdds(context.Background(), conn, "bsc", "v4", "0xpool", 2*time.Hour, 10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// V4 uses signed liquidity_delta aggregation.
	if !strings.Contains(conn.lastQuery, "sum(toInt256OrZero(liquidity_delta))") {
		t.Fatalf("expected v4 net liquidity expression, got query=%s", conn.lastQuery)
	}
}
