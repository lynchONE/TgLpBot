package web_server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleSmartMoneyGoldenDogConfig_MethodNotAllowed(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodPut, "/api/smart_money_golden_dog_config", nil)
	w := httptest.NewRecorder()
	srv.handleSmartMoneyGoldenDogConfig(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d body=%q", http.StatusMethodNotAllowed, w.Code, w.Body.String())
	}
}

func TestHandleSmartMoneyGoldenDogConfigPost_InvalidJSON(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/smart_money_golden_dog_config", strings.NewReader("{"))
	w := httptest.NewRecorder()
	srv.handleSmartMoneyGoldenDogConfig(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d body=%q", http.StatusBadRequest, w.Code, w.Body.String())
	}
}
