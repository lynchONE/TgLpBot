package exchange

import "testing"

func TestMarketAPIURL_RewritesAggregatorBase(t *testing.T) {
	svc := &OKXDexService{apiURL: "https://www.okx.com/api/v6/dex/aggregator"}
	got := svc.marketAPIURL()
	want := "https://www.okx.com/api/v6/dex/market"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeMarketCandlesRows_ParsesOfficialOrder(t *testing.T) {
	rows := normalizeMarketCandlesRows([][]string{
		{"1710000000000", "1.0", "1.2", "0.9", "1.1", "123.45", "234.56", "1"},
	})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	row := rows[0]
	if row.TimestampMS != 1710000000000 {
		t.Fatalf("unexpected timestamp: %+v", row)
	}
	if row.Open != 1.0 || row.High != 1.2 || row.Low != 0.9 || row.Close != 1.1 {
		t.Fatalf("unexpected OHLC values: %+v", row)
	}
	if row.Volume != 123.45 || row.VolumeUSD != 234.56 {
		t.Fatalf("unexpected volume values: %+v", row)
	}
	if !row.Confirm {
		t.Fatalf("expected confirm=true, got %+v", row)
	}
}
