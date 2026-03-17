package web_server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type fakePositionRefRows struct {
	idx  int
	data []smartMoneyPositionRef
	err  error
}

func (r *fakePositionRefRows) Next() bool {
	if r == nil || r.idx >= len(r.data) {
		return false
	}
	r.idx++
	return true
}

func (r *fakePositionRefRows) Scan(dest ...any) error {
	if r == nil || r.idx <= 0 || r.idx > len(r.data) {
		return nil
	}
	row := r.data[r.idx-1]
	*dest[0].(*string) = row.PoolVersion
	*dest[1].(*string) = row.PoolID
	*dest[2].(*string) = row.ContractAddress
	*dest[3].(*string) = row.TokenID
	*dest[4].(*int32) = int32(row.TickLower)
	*dest[5].(*int32) = int32(row.TickUpper)
	if row.OpenedAt != nil {
		*dest[6].(*time.Time) = row.OpenedAt.UTC()
	} else {
		*dest[6].(*time.Time) = time.Unix(0, 0).UTC()
	}
	*dest[7].(*uint64) = row.LastEventSeq
	return nil
}

func (r *fakePositionRefRows) ScanStruct(dest any) error        { return nil }
func (r *fakePositionRefRows) ColumnTypes() []driver.ColumnType { return nil }
func (r *fakePositionRefRows) Totals(dest ...any) error         { return nil }
func (r *fakePositionRefRows) Columns() []string                { return nil }
func (r *fakePositionRefRows) Close() error                     { return nil }
func (r *fakePositionRefRows) Err() error                       { return r.err }

type fakeV4PoolRows struct {
	idx  int
	data []string
	err  error
}

func (r *fakeV4PoolRows) Next() bool {
	if r == nil || r.idx >= len(r.data) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeV4PoolRows) Scan(dest ...any) error {
	if r == nil || r.idx <= 0 || r.idx > len(r.data) {
		return nil
	}
	*dest[0].(*string) = r.data[r.idx-1]
	return nil
}

func (r *fakeV4PoolRows) ScanStruct(dest any) error        { return nil }
func (r *fakeV4PoolRows) ColumnTypes() []driver.ColumnType { return nil }
func (r *fakeV4PoolRows) Totals(dest ...any) error         { return nil }
func (r *fakeV4PoolRows) Columns() []string                { return nil }
func (r *fakeV4PoolRows) Close() error                     { return nil }
func (r *fakeV4PoolRows) Err() error                       { return r.err }

func TestQuerySmartMoneyWalletRecentPositionRefs_IncludesUnifiedTokenRefs(t *testing.T) {
	startedAt := time.Unix(123, 0).UTC()
	conn := &fakeCHConn{
		rows: &fakePositionRefRows{
			data: []smartMoneyPositionRef{
				{
					PoolVersion:     "v3",
					PoolID:          "0xpoolv3",
					ContractAddress: "0xnpm",
					TokenID:         "123",
					TickLower:       -100,
					TickUpper:       100,
					LastEventSeq:    11,
					OpenedAt:        &startedAt,
				},
				{
					PoolVersion:     "v4",
					PoolID:          "0xpoolv4",
					ContractAddress: "0xpoolmanager",
					TokenID:         "456",
					TickLower:       -200,
					TickUpper:       200,
					LastEventSeq:    12,
				},
			},
		},
	}

	rows, err := querySmartMoneyWalletRecentPositionRefs(context.Background(), conn, "bsc", "0xabc", 24*time.Hour, 10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].WalletAddress != "0xabc" {
		t.Fatalf("expected wallet address to be injected, got %s", rows[0].WalletAddress)
	}
	if rows[0].OpenedAt == nil || !rows[0].OpenedAt.Equal(startedAt) {
		t.Fatalf("expected opened_at to be propagated, got %#v", rows[0].OpenedAt)
	}
	if !strings.Contains(conn.lastQuery, "FROM smart_lp_active_positions") {
		t.Fatalf("expected active-state source, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "token_id != ''") || !strings.Contains(conn.lastQuery, "is_active = 1") {
		t.Fatalf("expected active token position filters, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "GROUP BY position_key") {
		t.Fatalf("expected position-key grouping, got query=%s", conn.lastQuery)
	}
}

func TestQuerySmartMoneyWalletLegacyV4Pools_FiltersEmptyTokenID(t *testing.T) {
	conn := &fakeCHConn{
		rows: &fakeV4PoolRows{
			data: []string{"0xpool1"},
		},
	}

	pools, err := querySmartMoneyWalletLegacyV4Pools(context.Background(), conn, "bsc", "0xabc", 24*time.Hour, 10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(pools) != 1 || pools[0].PoolID != "0xpool1" {
		t.Fatalf("unexpected pools: %#v", pools)
	}
	if !strings.Contains(conn.lastQuery, "FROM smart_lp_active_positions") || !strings.Contains(conn.lastQuery, "pool_version = 'v4'") || !strings.Contains(conn.lastQuery, "token_id = ''") {
		t.Fatalf("expected legacy v4 active-state filter, got query=%s", conn.lastQuery)
	}
}

func TestFindSmartMoneyV4FallbackRef_PrefersExactTickMatch(t *testing.T) {
	refs := []smartMoneyPositionRef{
		{PoolID: "0xpool", TokenID: "1", TickLower: -100, TickUpper: 100},
		{PoolID: "0xpool", TokenID: "2", TickLower: -200, TickUpper: 200},
	}

	match, ok := findSmartMoneyV4FallbackRef(refs, "0xpool", -200, 200)
	if !ok {
		t.Fatal("expected exact fallback match")
	}
	if match.TokenID != "2" {
		t.Fatalf("expected token 2, got %s", match.TokenID)
	}
}

func TestFindSmartMoneyV4FallbackRef_UsesSinglePoolCandidate(t *testing.T) {
	refs := []smartMoneyPositionRef{
		{PoolID: "0xpool", TokenID: "7", TickLower: -100, TickUpper: 100},
	}

	match, ok := findSmartMoneyV4FallbackRef(refs, "0xpool", -50, 50)
	if !ok {
		t.Fatal("expected single pool candidate fallback")
	}
	if match.TokenID != "7" {
		t.Fatalf("expected token 7, got %s", match.TokenID)
	}
}
