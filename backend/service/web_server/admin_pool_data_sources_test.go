package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleAdminPoolDataSources_InvalidJSON(t *testing.T) {
	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/pool_data_sources", strings.NewReader("{"))
	rr := httptest.NewRecorder()

	srv.handleAdminPoolDataSources(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleAdminPoolDataSources_MissingInitData(t *testing.T) {
	old := config.AppConfig
	config.AppConfig = &config.Config{
		TelegramBotToken: "test",
	}
	defer func() { config.AppConfig = old }()

	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/pool_data_sources", strings.NewReader(`{"action":"list"}`))
	rr := httptest.NewRecorder()

	srv.handleAdminPoolDataSources(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleAdminPoolDataSources_NonAdmin(t *testing.T) {
	restore := stubAdminPoolDataSourcesAuth(t, false)
	defer restore()

	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/pool_data_sources", strings.NewReader(`{"action":"list"}`))
	rr := httptest.NewRecorder()

	srv.handleAdminPoolDataSources(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}
}

func TestHandleAdminPoolDataSources_InvalidAction(t *testing.T) {
	restore := stubAdminPoolDataSourcesAuth(t, true)
	defer restore()

	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/pool_data_sources", strings.NewReader(`{"action":"unknown"}`))
	rr := httptest.NewRecorder()

	srv.handleAdminPoolDataSources(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func stubAdminPoolDataSourcesAuth(t *testing.T, isAdmin bool) func() {
	t.Helper()
	oldConfig := config.AppConfig
	oldGetOrCreate := adminPoolDataSourcesGetOrCreateUser
	oldIsAdmin := adminPoolDataSourcesIsAdminUser

	config.AppConfig = &config.Config{
		TelegramBotToken:                 "test",
		TelegramWebAppAllowEmptyInitData: true,
	}
	adminPoolDataSourcesGetOrCreateUser = func(*TelegramWebAppInitData) (*models.User, error) {
		return &models.User{ID: 1, TelegramID: 1000000000, Username: "test"}, nil
	}
	adminPoolDataSourcesIsAdminUser = func(uint) bool {
		return isAdmin
	}

	return func() {
		config.AppConfig = oldConfig
		adminPoolDataSourcesGetOrCreateUser = oldGetOrCreate
		adminPoolDataSourcesIsAdminUser = oldIsAdmin
	}
}
