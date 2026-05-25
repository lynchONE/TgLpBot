package smart_money

import (
	"strings"
	"testing"
)

func TestNormalizeWatchActivityQuery(t *testing.T) {
	query := normalizeWatchActivityQuery(WatchActivityQuery{
		WalletAddress: " 0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD ",
		Page:          -1,
		Size:          500,
	})

	if query.ChainID != 56 {
		t.Fatalf("ChainID = %d, want 56", query.ChainID)
	}
	if query.Page != 1 {
		t.Fatalf("Page = %d, want 1", query.Page)
	}
	if query.Size != 20 {
		t.Fatalf("Size = %d, want 20", query.Size)
	}
	if query.WalletAddress != "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd" {
		t.Fatalf("WalletAddress = %q, want lower-case trimmed address", query.WalletAddress)
	}
}

func TestBuildWatchActivitySQLScopesUserChainWalletEventTypesAndPagination(t *testing.T) {
	query := normalizeWatchActivityQuery(WatchActivityQuery{
		UserID:        42,
		ChainID:       56,
		WalletAddress: "0x1111111111111111111111111111111111111111",
		Page:          3,
		Size:          25,
	})
	sql, args := buildWatchActivitySelectSQL(query)

	requiredFragments := []string{
		"FROM sm_lp_events",
		"INNER JOIN smart_money_user_watch_wallets smww",
		"smww.wallet_address = sm_lp_events.wallet_address",
		"smww.chain = ?",
		"smww.user_id = ?",
		"sm_lp_events.chain_id = ?",
		"sm_lp_events.event_type IN (?, ?)",
		"sm_lp_events.wallet_address = ?",
		"ORDER BY sm_lp_events.tx_timestamp DESC, sm_lp_events.id DESC",
		"LIMIT ? OFFSET ?",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL missing %q:\n%s", fragment, sql)
		}
	}

	wantArgs := []any{
		"bsc",
		uint(42),
		56,
		"add",
		"remove",
		"0x1111111111111111111111111111111111111111",
		25,
		50,
	}
	if len(args) != len(wantArgs) {
		t.Fatalf("args len = %d, want %d: %#v", len(args), len(wantArgs), args)
	}
	for index := range wantArgs {
		if args[index] != wantArgs[index] {
			t.Fatalf("args[%d] = %#v, want %#v; all args=%#v", index, args[index], wantArgs[index], args)
		}
	}
}
