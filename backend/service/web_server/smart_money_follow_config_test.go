package web_server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleSmartMoneyFollowConfig_MethodNotAllowed(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodPut, "/api/smart_money_follow_config", nil)
	w := httptest.NewRecorder()
	srv.handleSmartMoneyFollowConfig(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d body=%q", http.StatusMethodNotAllowed, w.Code, w.Body.String())
	}
}

func TestHandleSmartMoneyFollowConfigGet_InvalidWallet(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/smart_money_follow_config?wallet_address=not-an-addr", nil)
	w := httptest.NewRecorder()
	srv.handleSmartMoneyFollowConfig(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d body=%q", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestHandleSmartMoneyFollowConfigPost_InvalidJSON(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/smart_money_follow_config", strings.NewReader("{"))
	w := httptest.NewRecorder()
	srv.handleSmartMoneyFollowConfig(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d body=%q", http.StatusBadRequest, w.Code, w.Body.String())
	}
}
