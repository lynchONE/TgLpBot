package smart_money

import (
	"TgLpBot/base/config"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNormalizeTokenLiquidityCandidateQuery(t *testing.T) {
	query, err := NormalizeTokenLiquidityCandidateQuery(TokenLiquidityCandidateQuery{
		Chain:        "bsc",
		PoolAddress:  "0x00000000000000000000000000000000000000aa",
		MinAmountUSD: 500,
		WindowHours:  24,
		Limit:        20,
	})
	if err != nil {
		t.Fatalf("expected valid query: %v", err)
	}
	if query.ChainID != 56 {
		t.Fatalf("expected bsc chain id 56, got %d", query.ChainID)
	}

	_, err = NormalizeTokenLiquidityCandidateQuery(TokenLiquidityCandidateQuery{
		PoolAddress:  "bad",
		MinAmountUSD: 500,
		WindowHours:  24,
		Limit:        20,
	})
	if err == nil {
		t.Fatal("expected invalid pool address error")
	}

	_, err = NormalizeTokenLiquidityCandidateQuery(TokenLiquidityCandidateQuery{
		Provider:     "dexscreener",
		PoolAddress:  "0x00000000000000000000000000000000000000aa",
		MinAmountUSD: 500,
		WindowHours:  24,
		Limit:        20,
	})
	if err == nil || !strings.Contains(err.Error(), "provider is no longer supported") {
		t.Fatalf("expected provider removal error, got %v", err)
	}
}

func TestNormalizeTokenLiquidityCandidateQueryAcceptsAbsoluteRange(t *testing.T) {
	start := time.Date(2026, 6, 7, 12, 30, 15, 0, time.FixedZone("CST", 8*3600))
	end := start.Add(3*time.Hour + 20*time.Minute)
	query, err := NormalizeTokenLiquidityCandidateQuery(TokenLiquidityCandidateQuery{
		Chain:        "bsc",
		PoolAddress:  "0x00000000000000000000000000000000000000aa",
		MinAmountUSD: 500,
		StartTime:    start,
		EndTime:      end,
		Limit:        20,
	})
	if err != nil {
		t.Fatalf("expected absolute range query: %v", err)
	}
	if query.StartTime.Location() != time.UTC || query.EndTime.Location() != time.UTC {
		t.Fatal("expected absolute range to be normalized to UTC")
	}

	_, err = NormalizeTokenLiquidityCandidateQuery(TokenLiquidityCandidateQuery{
		PoolAddress:  "0x00000000000000000000000000000000000000aa",
		MinAmountUSD: 500,
		StartTime:    end,
		EndTime:      start,
		Limit:        20,
	})
	if err == nil || !strings.Contains(err.Error(), "end_time must be after start_time") {
		t.Fatalf("expected invalid absolute range error, got %v", err)
	}
}

func TestNewTokenLiquidityProviderFromConfigUsesRPCWithoutProviderConfig(t *testing.T) {
	provider, err := NewTokenLiquidityProviderFromConfig(&config.Config{})
	if err != nil {
		t.Fatalf("expected rpc provider without extra config: %v", err)
	}
	if _, ok := provider.(*RPCTokenLiquidityProvider); !ok {
		t.Fatalf("expected rpc provider, got %T", provider)
	}
}

func TestBitqueryBalanceDeltaUSD(t *testing.T) {
	amount, ok := bitqueryBalanceDeltaUSD(bitqueryTokenBalance{
		PreBalance:       "1000",
		PostBalance:      "800",
		PostBalanceInUSD: json.RawMessage(`"1600"`),
	})
	if !ok {
		t.Fatal("expected amount")
	}
	if amount != 400 {
		t.Fatalf("expected 400, got %f", amount)
	}

	_, ok = bitqueryBalanceDeltaUSD(bitqueryTokenBalance{
		PreBalance:       "800",
		PostBalance:      "1000",
		PostBalanceInUSD: json.RawMessage(`"2000"`),
	})
	if ok {
		t.Fatal("expected increasing balance to be excluded")
	}
}

func TestAggregateBitqueryCandidates(t *testing.T) {
	query := TokenLiquidityCandidateQuery{
		Chain:        "bsc",
		ChainID:      56,
		TokenAddress: "0x55d398326f99059ff775485246999027b3197955",
		MinAmountUSD: 300,
		WindowHours:  24,
		Limit:        10,
	}
	events := []bitqueryDEXPoolEvent{{
		Block:       bitqueryBlock{Time: "2026-06-08T00:00:00Z", Number: 1},
		Transaction: bitqueryTx{Hash: "0xabc"},
		PoolEvent: bitqueryPoolEvent{Pool: bitqueryPool{
			SmartContract: "0x00000000000000000000000000000000000000aa",
			CurrencyA:     bitqueryCurrency{Symbol: "ABC"},
			CurrencyB:     bitqueryCurrency{Symbol: "USDT"},
		}},
	}}
	balances := []bitqueryTransactionBalance{
		{
			Transaction: bitqueryTx{Hash: "0xabc"},
			TokenBalance: bitqueryTokenBalance{
				Address:          "0x0000000000000000000000000000000000000001",
				PreBalance:       "1000",
				PostBalance:      "800",
				PostBalanceInUSD: json.RawMessage(`"1600"`),
			},
		},
		{
			Transaction: bitqueryTx{Hash: "0xabc"},
			TokenBalance: bitqueryTokenBalance{
				Address:          "0x00000000000000000000000000000000000000aa",
				PreBalance:       "0",
				PostBalance:      "200",
				PostBalanceInUSD: json.RawMessage(`"400"`),
			},
		},
	}

	resp := aggregateBitqueryCandidates(query, events, balances)
	if len(resp.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(resp.Candidates))
	}
	candidate := resp.Candidates[0]
	if candidate.WalletAddress != "0x0000000000000000000000000000000000000001" {
		t.Fatalf("unexpected wallet %s", candidate.WalletAddress)
	}
	if candidate.MaxAmountUSD != 400 {
		t.Fatalf("expected max amount 400, got %f", candidate.MaxAmountUSD)
	}
}

func TestFilterLiquidityEventsByABIEvents(t *testing.T) {
	events := []bitqueryDEXPoolEvent{
		{Transaction: bitqueryTx{Hash: "0xadd"}},
		{Transaction: bitqueryTx{Hash: "0xswap"}},
	}
	abiEvents := []bitqueryEvent{{
		Transaction: bitqueryTx{Hash: "0xadd"},
		Log:         bitqueryLog{Signature: bitquerySignature{Name: "IncreaseLiquidity"}},
	}}

	filtered := filterLiquidityEventsByABIEvents(events, abiEvents)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 event, got %d", len(filtered))
	}
	if filtered[0].Transaction.Hash != "0xadd" {
		t.Fatalf("unexpected tx %s", filtered[0].Transaction.Hash)
	}
}
