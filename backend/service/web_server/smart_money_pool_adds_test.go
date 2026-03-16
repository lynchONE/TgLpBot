package web_server

import (
	"context"
	"fmt"
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

func TestQuerySmartMoneyPoolAddsStable_UsesDedupAndTokenKeyV3(t *testing.T) {
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

	rows, err := querySmartMoneyPoolAddsStable(context.Background(), conn, "bsc", "v3", "0xpool", 2*time.Hour, 10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if !strings.Contains(conn.lastQuery, "GROUP BY tx_hash, log_index") {
		t.Fatalf("expected event dedup grouping, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "concat('token:', token_id)") {
		t.Fatalf("expected token-first position key, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "ANY INNER JOIN") {
		t.Fatalf("expected active-state join, got query=%s", conn.lastQuery)
	}
	if strings.Contains(conn.lastQuery, "max(ts) AS ts") {
		t.Fatalf("expected dedup timestamp alias to avoid WHERE conflicts, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "max(ts) AS event_ts") {
		t.Fatalf("expected dedup timestamp alias event_ts, got query=%s", conn.lastQuery)
	}
	if strings.Contains(conn.lastQuery, "argMax(action, event_seq) AS action") {
		t.Fatalf("expected dedup action alias to avoid WHERE conflicts, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "argMax(action, event_seq) AS dedup_action") {
		t.Fatalf("expected dedup action alias, got query=%s", conn.lastQuery)
	}
	if strings.Contains(conn.lastQuery, "argMax(wallet_address, event_seq) AS wallet_address") {
		t.Fatalf("expected dedup wallet alias to avoid WHERE conflicts, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "argMax(wallet_address, event_seq) AS dedup_wallet_address") {
		t.Fatalf("expected dedup wallet alias, got query=%s", conn.lastQuery)
	}
	if strings.Contains(conn.lastQuery, "WHERE action = 'add'") {
		t.Fatalf("expected recent adds to use sumIf/countIf instead of WHERE action filter, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "countIf(dedup_action = 'add') > 0") {
		t.Fatalf("expected recent adds HAVING countIf filter, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "sumIf(toInt256OrZero(amount0), dedup_action = 'add')") {
		t.Fatalf("expected sumIf with dedup_action, got query=%s", conn.lastQuery)
	}
}

func TestQuerySmartMoneyPoolAddsStable_UsesSignedLiquidityV4(t *testing.T) {
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

	_, err := querySmartMoneyPoolAddsStable(context.Background(), conn, "bsc", "v4", "0xpool", 2*time.Hour, 10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(conn.lastQuery, "sum(toInt256OrZero(liquidity_delta))") {
		t.Fatalf("expected v4 signed liquidity expression, got query=%s", conn.lastQuery)
	}
}

type fakePoolAddResolver struct {
	positions map[string]*smartMoneyResolvedPosition
}

func (r *fakePoolAddResolver) Resolve(_ context.Context, ref smartMoneyPositionRef) (*smartMoneyResolvedPosition, error) {
	if r == nil {
		return nil, nil
	}
	key := strings.ToLower(strings.TrimSpace(ref.PoolVersion)) + "|" +
		strings.ToLower(strings.TrimSpace(ref.PoolID)) + "|" +
		strings.ToLower(strings.TrimSpace(ref.WalletAddress)) + "|" +
		strings.TrimSpace(ref.TokenID) + "|" +
		fmt.Sprintf("%d", ref.TickLower) + "|" +
		fmt.Sprintf("%d", ref.TickUpper)
	return r.positions[key], nil
}

func TestResolveActiveSmartMoneyPoolAddRows_FiltersClosedRows(t *testing.T) {
	resolver := &fakePoolAddResolver{
		positions: map[string]*smartMoneyResolvedPosition{
			"v3|0xpool|0xwallet1|101|-120|120": {
				PoolVersion:     "v3",
				PoolID:          "0xpool",
				PositionID:      "101",
				ContractAddress: "0xpm-live",
			},
		},
	}

	rows := []smartMoneyPoolAddsWalletRow{
		{
			WalletAddress: "0xwallet1",
			TokenID:       "101",
			NPMAddress:    "0xpm-old",
			TickLower:     -120,
			TickUpper:     120,
		},
		{
			WalletAddress: "0xwallet2",
			TokenID:       "202",
			NPMAddress:    "0xpm-old",
			TickLower:     -200,
			TickUpper:     200,
		},
	}

	active, resolved, warnings := resolveActiveSmartMoneyPoolAddRows(
		context.Background(),
		"v3",
		"0xpool",
		rows,
		resolver,
		nil,
	)
	if len(active) != 1 {
		t.Fatalf("expected 1 active row, got %d", len(active))
	}
	if len(resolved) != 1 || resolved[0] == nil || resolved[0].PositionID != "101" {
		t.Fatalf("unexpected resolved positions: %#v", resolved)
	}
	if active[0].WalletAddress != "0xwallet1" {
		t.Fatalf("expected wallet1 to remain, got %#v", active[0])
	}
	if active[0].NPMAddress != "0xpm-live" {
		t.Fatalf("expected live contract address to overwrite row, got %s", active[0].NPMAddress)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "filtered 1 stale rows") {
		t.Fatalf("expected stale-row warning, got %#v", warnings)
	}
}

func TestResolveActiveSmartMoneyPoolAddRows_UsesV4FallbackTokenID(t *testing.T) {
	resolver := &fakePoolAddResolver{
		positions: map[string]*smartMoneyResolvedPosition{
			"v4|0xpool|0xwallet1|909|-300|300": {
				PoolVersion: "v4",
				PoolID:      "0xpool",
				PositionID:  "909",
			},
		},
	}

	rows := []smartMoneyPoolAddsWalletRow{
		{
			WalletAddress: "0xwallet1",
			TokenID:       "",
			TickLower:     -300,
			TickUpper:     300,
		},
	}

	fallbackLoader := func(_ context.Context, walletAddr string, pools []smartMoneyWalletV4PoolRef, limit int) ([]smartMoneyPositionRef, error) {
		if walletAddr != "0xwallet1" {
			t.Fatalf("unexpected wallet: %s", walletAddr)
		}
		if len(pools) != 1 || pools[0].PoolID != "0xpool" {
			t.Fatalf("unexpected pools: %#v", pools)
		}
		if limit != 20 {
			t.Fatalf("unexpected limit: %d", limit)
		}
		return []smartMoneyPositionRef{
			{
				WalletAddress: "0xwallet1",
				PoolVersion:   "v4",
				PoolID:        "0xpool",
				TokenID:       "909",
				TickLower:     -300,
				TickUpper:     300,
			},
		}, nil
	}

	active, resolved, warnings := resolveActiveSmartMoneyPoolAddRows(
		context.Background(),
		"v4",
		"0xpool",
		rows,
		resolver,
		fallbackLoader,
	)
	if len(active) != 1 {
		t.Fatalf("expected 1 active row, got %d", len(active))
	}
	if active[0].TokenID != "909" {
		t.Fatalf("expected token_id to be backfilled, got %#v", active[0])
	}
	if len(resolved) != 1 || resolved[0] == nil || resolved[0].PositionID != "909" {
		t.Fatalf("unexpected resolved positions: %#v", resolved)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
}
