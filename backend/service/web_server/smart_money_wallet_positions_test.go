package web_server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleSmartMoneyWalletPositions_MethodNotAllowed(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/smart_money_wallet_positions", nil)
	w := httptest.NewRecorder()
	srv.handleSmartMoneyWalletPositions(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d body=%q", http.StatusMethodNotAllowed, w.Code, w.Body.String())
	}
}

func TestHandleSmartMoneyWalletPositions_ClickHouseNotConfigured(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/smart_money_wallet_positions?wallet_address=0x0000000000000000000000000000000000000001", nil)
	w := httptest.NewRecorder()
	srv.handleSmartMoneyWalletPositions(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected %d, got %d body=%q", http.StatusServiceUnavailable, w.Code, w.Body.String())
	}
}
