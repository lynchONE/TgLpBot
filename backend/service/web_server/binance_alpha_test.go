package web_server

import (
	"encoding/json"
	"testing"
)

func TestEnrichHotPoolBinanceAlphaWithIndexAddsBadgeForNonStableToken(t *testing.T) {
	items := []HotPoolResponse{
		{
			Chain:         "bsc",
			PoolAddress:   "0x1111111111111111111111111111111111111111",
			TradingPair:   "QAIT/USDT",
			Token0Address: "0x4d41a5d412f4ef44a35b9f53b06db65ede249493",
			Token0Symbol:  "QAIT",
			Token1Address: "0x55d398326f99059ff775485246999027b3197955",
			Token1Symbol:  "USDT",
			Badges:        json.RawMessage(`[{"label":"Hot","tip":"Hot pool"}]`),
		},
	}
	index := map[string]binanceAlphaTokenInfo{
		"56:0x4d41a5d412f4ef44a35b9f53b06db65ede249493": {
			ChainID:         "56",
			ChainName:       "BSC",
			ContractAddress: "0x4d41a5d412f4ef44a35b9f53b06db65ede249493",
			Symbol:          "QAIT",
			AlphaID:         "ALPHA_980",
		},
	}

	enrichHotPoolBinanceAlphaWithIndex("bsc", items, index)

	var badges []map[string]string
	if err := json.Unmarshal(items[0].Badges, &badges); err != nil {
		t.Fatalf("unmarshal badges: %v", err)
	}
	if len(badges) != 2 {
		t.Fatalf("badge count = %d, want 2; raw=%s", len(badges), string(items[0].Badges))
	}
	if badges[1]["label"] != binanceAlphaBadgeLabel {
		t.Fatalf("last badge label = %q, want %q", badges[1]["label"], binanceAlphaBadgeLabel)
	}
	if badges[1]["tip"] != "币安 Alpha · QAIT · ALPHA_980 · BSC" {
		t.Fatalf("last badge tip = %q", badges[1]["tip"])
	}
}

func TestEnrichHotPoolBinanceAlphaWithIndexSkipsStableTokenMatch(t *testing.T) {
	items := []HotPoolResponse{
		{
			Chain:         "bsc",
			Token0Address: "0x4d41a5d412f4ef44a35b9f53b06db65ede249493",
			Token0Symbol:  "QAIT",
			Token1Address: "0x55d398326f99059ff775485246999027b3197955",
			Token1Symbol:  "USDT",
			Badges:        json.RawMessage(`[]`),
		},
	}
	index := map[string]binanceAlphaTokenInfo{
		"56:0x55d398326f99059ff775485246999027b3197955": {
			ChainID:         "56",
			ContractAddress: "0x55d398326f99059ff775485246999027b3197955",
			Symbol:          "USDT",
		},
	}

	enrichHotPoolBinanceAlphaWithIndex("bsc", items, index)

	var badges []map[string]string
	if err := json.Unmarshal(items[0].Badges, &badges); err != nil {
		t.Fatalf("unmarshal badges: %v", err)
	}
	if len(badges) != 0 {
		t.Fatalf("badge count = %d, want 0; raw=%s", len(badges), string(items[0].Badges))
	}
}

func TestAppendHotPoolBadgeDoesNotDuplicateBinanceAlpha(t *testing.T) {
	raw := json.RawMessage(`[{"text":"币安 Alpha","tip":"old"}]`)

	got := appendHotPoolBadge(raw, binanceAlphaBadgeLabel, "new")

	if string(got) != string(raw) {
		t.Fatalf("appendHotPoolBadge duplicated existing badge: got=%s want=%s", string(got), string(raw))
	}
}
