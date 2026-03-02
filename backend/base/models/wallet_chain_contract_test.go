package models

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestWalletChainContract_UniqueIndex(t *testing.T) {
	s, err := schema.Parse(&WalletChainContract{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}

	indexes := s.ParseIndexes()
	idx, ok := indexes["uniq_wallet_chain_kind"]
	if !ok {
		t.Fatalf("uniq_wallet_chain_kind index not found, have=%v", keys(indexes))
	}
	if idx.Class != "UNIQUE" {
		t.Fatalf("expected UNIQUE index, got class=%q", idx.Class)
	}
	if len(idx.Fields) != 3 {
		t.Fatalf("expected 3 index fields, got %d", len(idx.Fields))
	}

	got := []string{idx.Fields[0].DBName, idx.Fields[1].DBName, idx.Fields[2].DBName}
	want := []string{"wallet_id", "chain", "kind"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("index field[%d] mismatch: got=%q want=%q (all=%v)", i, got[i], want[i], got)
		}
	}
}

func keys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
