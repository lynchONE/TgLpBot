package web_server

import (
	"testing"

	"TgLpBot/service/exchange"
)

func TestNormalizeOKXBar(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{raw: "", want: "1m"},
		{raw: "1m", want: "1m"},
		{raw: "1H", want: "1H"},
		{raw: "4h", want: "4H"},
		{raw: "1d", want: "1D"},
		{raw: "1w", want: "1W"},
	}

	for _, tc := range cases {
		if got := normalizeOKXBar(tc.raw); got != tc.want {
			t.Fatalf("normalizeOKXBar(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestNormalizeTokenAddress(t *testing.T) {
	got, ok := normalizeTokenAddress("1111111111111111111111111111111111111111")
	if !ok {
		t.Fatal("expected address without 0x to be valid")
	}
	if got != "0x1111111111111111111111111111111111111111" {
		t.Fatalf("unexpected normalized address: %s", got)
	}

	if _, ok := normalizeTokenAddress("not-an-address"); ok {
		t.Fatal("expected invalid address")
	}
}

func TestGeckoTokenIDAddressExtractsAddressForWalletCandidates(t *testing.T) {
	got := geckoTokenIDAddress("bsc_1111111111111111111111111111111111111111")
	if got != "0x1111111111111111111111111111111111111111" {
		t.Fatalf("unexpected token address: %s", got)
	}
}

func TestTokenCandlesFromOKXRowsSortsAndSkipsInvalidRows(t *testing.T) {
	rows := []exchange.MarketCandle{
		{TimestampMS: 1700000060000, Open: 2, High: 3, Low: 1, Close: 2.5, Volume: 12, VolumeUSD: 30, Confirm: true},
		{TimestampMS: 0, Open: 1, High: 1, Low: 1, Close: 1},
		{TimestampMS: 1700000000000, Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 10, VolumeUSD: 15, Confirm: false},
	}

	candles := tokenCandlesFromOKXRows(rows)
	if len(candles) != 2 {
		t.Fatalf("expected two candles, got %+v", candles)
	}
	if candles[0].T != 1700000000 || candles[0].C != 1.5 || candles[0].Confirm {
		t.Fatalf("unexpected first candle: %+v", candles[0])
	}
	if candles[1].T != 1700000060 || candles[1].VUSD != 30 || !candles[1].Confirm {
		t.Fatalf("unexpected second candle: %+v", candles[1])
	}
}
