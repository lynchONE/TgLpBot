package web_server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeAdminSmartMoneyLeaderboardRequest_CanonicalFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/admin?endpoint=assets_smart_money_leaderboard", strings.NewReader(`{"initData":"abc","days":1,"metric":"pnl","page":2,"page_size":20,"keyword":"tag","force_refresh":true}`))
	rr := httptest.NewRecorder()

	got, ok := decodeAdminSmartMoneyLeaderboardRequest(rr, req)
	if !ok {
		t.Fatalf("expected decode success, got status %d", rr.Code)
	}
	if got.InitData != "abc" {
		t.Fatalf("initData = %q, want %q", got.InitData, "abc")
	}
	if got.PageSize != 20 {
		t.Fatalf("pageSize = %d, want %d", got.PageSize, 20)
	}
	if !got.ForceRefresh {
		t.Fatalf("forceRefresh = false, want true")
	}
}

func TestDecodeAdminSmartMoneyLeaderboardRequest_CamelCaseAndUnknownFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/admin?endpoint=assets_smart_money_leaderboard", strings.NewReader(`{"initData":"abc","days":1,"metric":"yield_rate","page":1,"pageSize":15,"keyword":"alpha","forceRefresh":true,"tab":"leaderboard"}`))
	rr := httptest.NewRecorder()

	got, ok := decodeAdminSmartMoneyLeaderboardRequest(rr, req)
	if !ok {
		t.Fatalf("expected decode success, got status %d", rr.Code)
	}
	if got.PageSize != 15 {
		t.Fatalf("pageSize = %d, want %d", got.PageSize, 15)
	}
	if !got.ForceRefresh {
		t.Fatalf("forceRefresh = false, want true")
	}
	if got.Metric != "yield_rate" {
		t.Fatalf("metric = %q, want %q", got.Metric, "yield_rate")
	}
}
