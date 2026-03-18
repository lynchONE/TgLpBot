package web_server

import (
	"TgLpBot/base/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleAdminPrivateZap_InvalidJSON(t *testing.T) {
	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/private_zap", strings.NewReader("{"))
	rr := httptest.NewRecorder()

	srv.handleAdminPrivateZap(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleAdminPrivateZap_ConfigNotLoaded(t *testing.T) {
	old := config.AppConfig
	config.AppConfig = nil
	defer func() { config.AppConfig = old }()

	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/private_zap", strings.NewReader(`{"initData":"x","action":"list"}`))
	rr := httptest.NewRecorder()

	srv.handleAdminPrivateZap(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestHandleAdminPrivateZap_MissingInitData(t *testing.T) {
	old := config.AppConfig
	config.AppConfig = &config.Config{
		TelegramBotToken: "test",
	}
	defer func() { config.AppConfig = old }()

	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/private_zap", strings.NewReader(`{"action":"list"}`))
	rr := httptest.NewRecorder()

	srv.handleAdminPrivateZap(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}
