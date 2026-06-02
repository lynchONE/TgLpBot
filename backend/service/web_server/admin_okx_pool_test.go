package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/okxpool"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleAdminOKXPool_InvalidJSON(t *testing.T) {
	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/okx_pool", strings.NewReader("{"))
	rr := httptest.NewRecorder()

	srv.handleAdminOKXPool(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleAdminOKXPool_MissingInitData(t *testing.T) {
	old := config.AppConfig
	config.AppConfig = &config.Config{
		TelegramBotToken: "test",
	}
	t.Cleanup(func() { config.AppConfig = old })

	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/okx_pool", strings.NewReader(`{"action":"list"}`))
	rr := httptest.NewRecorder()

	srv.handleAdminOKXPool(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleAdminOKXPool_NonAdmin(t *testing.T) {
	restore := stubAdminOKXPoolAuth(t, false)
	defer restore()

	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/okx_pool", strings.NewReader(`{"action":"list"}`))
	rr := httptest.NewRecorder()

	srv.handleAdminOKXPool(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}
}

func TestHandleAdminOKXPool_InvalidAction(t *testing.T) {
	restore := stubAdminOKXPoolAuth(t, true)
	defer restore()

	oldManager := adminOKXPoolManager
	adminOKXPoolManager = func() *okxpool.Manager {
		return okxpool.NewManager(nil, nil)
	}
	defer func() { adminOKXPoolManager = oldManager }()

	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/okx_pool", strings.NewReader(`{"action":"unknown"}`))
	rr := httptest.NewRecorder()

	srv.handleAdminOKXPool(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func stubAdminOKXPoolAuth(t *testing.T, isAdmin bool) func() {
	t.Helper()
	oldConfig := config.AppConfig
	oldGetOrCreate := adminOKXPoolGetOrCreateUser
	oldIsAdmin := adminOKXPoolIsAdminUser

	config.AppConfig = &config.Config{
		TelegramBotToken:                 "test",
		TelegramWebAppAllowEmptyInitData: true,
	}
	adminOKXPoolGetOrCreateUser = func(*TelegramWebAppInitData) (*models.User, error) {
		return &models.User{ID: 1, TelegramID: 1000000000, Username: "test"}, nil
	}
	adminOKXPoolIsAdminUser = func(uint) bool {
		return isAdmin
	}

	return func() {
		config.AppConfig = oldConfig
		adminOKXPoolGetOrCreateUser = oldGetOrCreate
		adminOKXPoolIsAdminUser = oldIsAdmin
	}
}
