package web_server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type fakeMarkerRows struct {
	idx  int
	data []smartMoneyPoolMarkerRow
	err  error
}

func (r *fakeMarkerRows) Next() bool {
	if r == nil {
		return false
	}
	if r.idx >= len(r.data) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeMarkerRows) Scan(dest ...any) error {
	if r == nil || r.idx <= 0 || r.idx > len(r.data) {
		return nil
	}
	row := r.data[r.idx-1]
	*dest[0].(*time.Time) = row.Ts
	*dest[1].(*uint64) = row.EventSeq
	*dest[2].(*string) = row.WalletAddress
	*dest[3].(*string) = row.Action
	*dest[4].(*string) = row.TokenID
	*dest[5].(*string) = row.ContractAddr
	*dest[6].(*string) = row.Amount0
	*dest[7].(*string) = row.Amount1
	*dest[8].(*string) = row.LiquidityDelta
	*dest[9].(*int32) = row.TickLower
	*dest[10].(*int32) = row.TickUpper
	*dest[11].(*string) = row.TxHash
	*dest[12].(*uint64) = row.BlockNumber
	*dest[13].(*uint32) = row.LogIndex
	return nil
}

func (r *fakeMarkerRows) ScanStruct(dest any) error        { return nil }
func (r *fakeMarkerRows) ColumnTypes() []driver.ColumnType { return nil }
func (r *fakeMarkerRows) Totals(dest ...any) error         { return nil }
func (r *fakeMarkerRows) Columns() []string                { return nil }
func (r *fakeMarkerRows) Close() error                     { return nil }
func (r *fakeMarkerRows) Err() error                       { return r.err }

type fakeMarkerSummaryRows struct {
	idx  int
	data []smartMoneyPoolMarkerSummary
	err  error
}

func (r *fakeMarkerSummaryRows) Next() bool {
	if r == nil {
		return false
	}
	if r.idx >= len(r.data) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeMarkerSummaryRows) Scan(dest ...any) error {
	if r == nil || r.idx <= 0 || r.idx > len(r.data) {
		return nil
	}
	row := r.data[r.idx-1]
	*dest[0].(*uint64) = row.TotalEvents
	*dest[1].(*uint64) = row.AddCount
	*dest[2].(*uint64) = row.RemoveCount
	*dest[3].(*uint64) = row.WalletCount
	return nil
}

func (r *fakeMarkerSummaryRows) ScanStruct(dest any) error        { return nil }
func (r *fakeMarkerSummaryRows) ColumnTypes() []driver.ColumnType { return nil }
func (r *fakeMarkerSummaryRows) Totals(dest ...any) error         { return nil }
func (r *fakeMarkerSummaryRows) Columns() []string                { return nil }
func (r *fakeMarkerSummaryRows) Close() error                     { return nil }
func (r *fakeMarkerSummaryRows) Err() error                       { return r.err }

func TestQuerySmartMoneyPoolMarkerEvents_UsesPoolAndActionFilters(t *testing.T) {
	conn := &fakeCHConn{
		rows: &fakeMarkerRows{
			data: []smartMoneyPoolMarkerRow{
				{
					Ts:             time.Unix(1700000000, 0).UTC(),
					EventSeq:       9,
					WalletAddress:  "0xabc",
					Action:         "add",
					Amount0:        "100",
					Amount1:        "200",
					LiquidityDelta: "50",
					TickLower:      -100,
					TickUpper:      100,
					TxHash:         "0xtx",
					BlockNumber:    10,
					LogIndex:       3,
				},
			},
		},
	}

	start := time.Unix(1700000000, 0).UTC()
	end := start.Add(6 * time.Hour)
	rows, err := querySmartMoneyPoolMarkerEvents(context.Background(), conn, "bsc", "v3", "0xpool", 300, start, end, 10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if !strings.Contains(conn.lastQuery, "action IN ('add', 'remove')") {
		t.Fatalf("expected action filter, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "pool_version = ? AND pool_id = ?") {
		t.Fatalf("expected pool filter, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "intDiv(toInt64(toUnixTimestamp(ts)), ?) * ? >= ?") {
		t.Fatalf("expected bucketed start filter, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "intDiv(toInt64(toUnixTimestamp(ts)), ?) * ? <= ?") {
		t.Fatalf("expected bucketed end filter, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "net_amount0") || !strings.Contains(conn.lastQuery, "net_amount1") {
		t.Fatalf("expected net amount fallback in query, got query=%s", conn.lastQuery)
	}
}

func TestQuerySmartMoneyPoolMarkerSummary_UsesFullWindowCounts(t *testing.T) {
	conn := &fakeCHConn{
		rows: &fakeMarkerSummaryRows{
			data: []smartMoneyPoolMarkerSummary{
				{
					TotalEvents: 312,
					AddCount:    60,
					RemoveCount: 252,
					WalletCount: 16,
				},
			},
		},
	}

	start := time.Unix(1700000000, 0).UTC()
	end := start.Add(24 * time.Hour)
	summary, err := querySmartMoneyPoolMarkerSummary(context.Background(), conn, "bsc", "v3", "0xpool", 300, start, end)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if summary.TotalEvents != 312 || summary.AddCount != 60 || summary.RemoveCount != 252 || summary.WalletCount != 16 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if !strings.Contains(conn.lastQuery, "countIf(action = 'add') AS add_count") {
		t.Fatalf("expected add count query, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "countIf(action = 'remove') AS remove_count") {
		t.Fatalf("expected remove count query, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "uniqExact(wallet_address) AS wallet_count") {
		t.Fatalf("expected wallet count query, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "intDiv(toInt64(toUnixTimestamp(ts)), ?) * ? >= ?") {
		t.Fatalf("expected bucketed start filter, got query=%s", conn.lastQuery)
	}
	if !strings.Contains(conn.lastQuery, "intDiv(toInt64(toUnixTimestamp(ts)), ?) * ? <= ?") {
		t.Fatalf("expected bucketed end filter, got query=%s", conn.lastQuery)
	}
}

func TestApplyMarkerPnLEstimates_ComputesRemovePnLFromMatchedAdds(t *testing.T) {
	rows := []smartMoneyPoolMarkerRow{
		{
			Ts:             time.Unix(1700000000, 0).UTC(),
			EventSeq:       10,
			WalletAddress:  "0xabc",
			Action:         "add",
			TokenID:        "1",
			ContractAddr:   "0xnpm",
			Amount0:        "50",
			Amount1:        "50",
			LiquidityDelta: "100",
			TickLower:      -120,
			TickUpper:      120,
			TxHash:         "0xadd",
			BlockNumber:    100,
			LogIndex:       1,
		},
		{
			Ts:             time.Unix(1700000600, 0).UTC(),
			EventSeq:       11,
			WalletAddress:  "0xabc",
			Action:         "remove",
			TokenID:        "1",
			ContractAddr:   "0xnpm",
			Amount0:        "25",
			Amount1:        "20",
			LiquidityDelta: "40",
			TickLower:      -120,
			TickUpper:      120,
			TxHash:         "0xremove",
			BlockNumber:    101,
			LogIndex:       1,
		},
	}

	out := applyMarkerPnLEstimates(rows, rows, 0, 0, 2, 1)
	est, ok := out[buildMarkerEventID("0xremove", 11, 1)]
	if !ok {
		t.Fatalf("expected remove estimate")
	}
	if !est.HasPnLEstimate {
		t.Fatalf("expected has_pnl_estimate=true")
	}
	if est.CostBasisUSD != 60 {
		t.Fatalf("expected cost basis 60, got %v", est.CostBasisUSD)
	}
	if est.PnLEstimateUSD != 10 {
		t.Fatalf("expected pnl 10, got %v", est.PnLEstimateUSD)
	}
}

func TestApplyMarkerPnLEstimates_SkipsRemoveWithoutTrackedCost(t *testing.T) {
	rows := []smartMoneyPoolMarkerRow{
		{
			Ts:             time.Unix(1700000600, 0).UTC(),
			EventSeq:       11,
			WalletAddress:  "0xabc",
			Action:         "remove",
			TokenID:        "",
			ContractAddr:   "0xpoolmanager",
			Amount0:        "25",
			Amount1:        "20",
			LiquidityDelta: "-40",
			TickLower:      -120,
			TickUpper:      120,
			TxHash:         "0xremove",
			BlockNumber:    101,
			LogIndex:       1,
		},
	}

	out := applyMarkerPnLEstimates(rows, rows, 0, 0, 2, 1)
	if _, ok := out[buildMarkerEventID("0xremove", 11, 1)]; ok {
		t.Fatalf("expected remove without prior add to be skipped")
	}
}
