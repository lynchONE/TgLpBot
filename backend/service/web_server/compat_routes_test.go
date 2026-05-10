package web_server

import (
	"TgLpBot/base/config"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlePositions_InvalidEndpoint(t *testing.T) {
	srv := NewServer()
	req := httptest.NewRequest(http.MethodGet, "/api/positions?endpoint=bad", nil)
	rr := httptest.NewRecorder()

	srv.handlePositions(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleTaskAction_InvalidAction(t *testing.T) {
	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/task_action?action=bad", nil)
	rr := httptest.NewRecorder()

	srv.handleTaskAction(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleGetPools_InvalidEndpoint(t *testing.T) {
	srv := NewServer()
	req := httptest.NewRequest(http.MethodGet, "/api/pools?endpoint=bad", nil)
	rr := httptest.NewRecorder()

	srv.handleGetPools(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleSMCompat_InvalidEndpoint(t *testing.T) {
	srv := NewServer()
	req := httptest.NewRequest(http.MethodGet, "/api/sm?endpoint=bad", nil)
	rr := httptest.NewRecorder()

	srv.handleSMCompat(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleSMUploadCompat_InvalidEndpoint(t *testing.T) {
	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/sm_upload?endpoint=bad", nil)
	rr := httptest.NewRecorder()

	srv.handleSMUploadCompat(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestParseAllowedSmartMoneyAvatarURL(t *testing.T) {
	oldConfig := config.AppConfig
	t.Cleanup(func() {
		config.AppConfig = oldConfig
	})
	config.AppConfig = &config.Config{
		MinIOPublicBaseURL: "https://cdn.example.com",
	}

	if _, ok := parseAllowedSmartMoneyAvatarURL("https://cdn.example.com/avatar/smart-money/a.png"); !ok {
		t.Fatalf("expected avatar URL to be allowed")
	}
	if _, ok := parseAllowedSmartMoneyAvatarURL("https://evil.example.com/avatar/smart-money/a.png"); ok {
		t.Fatalf("expected foreign host to be rejected")
	}
	if _, ok := parseAllowedSmartMoneyAvatarURL("https://cdn.example.com/not-avatar/a.png"); ok {
		t.Fatalf("expected non-avatar path to be rejected")
	}
}
