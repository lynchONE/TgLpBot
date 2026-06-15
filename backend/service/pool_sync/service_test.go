package pool_sync

import (
	"TgLpBot/base/models"
	"testing"
)

func TestReplacementScopesFromRowsGroupsByChain(t *testing.T) {
	scopes, err := replacementScopesFromRows([]models.Pool{
		{ID: "pool-b", Chain: "bsc"},
		{ID: "pool-a", Chain: "bsc"},
		{ID: "pool-c", Chain: "base"},
		{ID: "pool-a", Chain: "bsc"},
	})
	if err != nil {
		t.Fatalf("replacementScopesFromRows() error = %v", err)
	}
	if len(scopes) != 2 {
		t.Fatalf("len(scopes) = %d, want 2", len(scopes))
	}
	if scopes[0].chain != "base" || len(scopes[0].ids) != 1 || scopes[0].ids[0] != "pool-c" {
		t.Fatalf("base scope = %+v", scopes[0])
	}
	if scopes[1].chain != "bsc" || len(scopes[1].ids) != 2 || scopes[1].ids[0] != "pool-a" || scopes[1].ids[1] != "pool-b" {
		t.Fatalf("bsc scope = %+v", scopes[1])
	}
}

func TestReplacementScopesFromRowsRejectsMissingChain(t *testing.T) {
	_, err := replacementScopesFromRows([]models.Pool{{ID: "pool-a"}})
	if err == nil {
		t.Fatalf("expected missing chain error")
	}
}
