package web_server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"TgLpBot/base/models"
)

func TestHandleAdminAccess_InvalidJSON(t *testing.T) {
	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/admin_access", strings.NewReader("{"))
	rr := httptest.NewRecorder()

	srv.handleAdminAccess(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleAdminAccess_NonAdmin(t *testing.T) {
	restore := stubAdminWorkbenchAuth(http.StatusForbidden, "forbidden")
	defer restore()

	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/admin_access", strings.NewReader(`{"action":"list","initData":"x"}`))
	rr := httptest.NewRecorder()

	srv.handleAdminAccess(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}
}

func TestHandleAdminAuthCodes_InvalidAction(t *testing.T) {
	restore := stubAdminWorkbenchAuth(0, "")
	defer restore()

	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/admin_auth_codes", strings.NewReader(`{"action":"unknown","initData":"x"}`))
	rr := httptest.NewRecorder()

	srv.handleAdminAuthCodes(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleAdminAuthCodes_UpdateRequiresCodeID(t *testing.T) {
	restore := stubAdminWorkbenchAuth(0, "")
	defer restore()

	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/admin_auth_codes", strings.NewReader(`{"action":"update","initData":"x"}`))
	rr := httptest.NewRecorder()

	srv.handleAdminAuthCodes(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleAdminAnnouncements_PublishRequiresContent(t *testing.T) {
	restore := stubAdminWorkbenchAuth(0, "")
	defer restore()

	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/admin_announcements", strings.NewReader(`{"action":"publish","initData":"x","title":"t"}`))
	rr := httptest.NewRecorder()

	srv.handleAdminAnnouncements(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func stubAdminWorkbenchAuth(status int, msg string) func() {
	old := authenticateAdminWebAppUserForAdminHandlers
	authenticateAdminWebAppUserForAdminHandlers = func(string) (*models.User, int, string) {
		if status != 0 {
			return nil, status, msg
		}
		return &models.User{ID: 1, TelegramID: 1000000000, Username: "admin"}, 0, ""
	}
	return func() {
		authenticateAdminWebAppUserForAdminHandlers = old
	}
}
