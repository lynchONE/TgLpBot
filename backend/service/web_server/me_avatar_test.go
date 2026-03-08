package web_server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleMeAvatar_MethodNotAllowed(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/me/avatar", nil)
	w := httptest.NewRecorder()

	srv.handleMeAvatar(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestDetectImageContentType_FallsBackToExtension(t *testing.T) {
	got := detectImageContentType("https://example.com/avatar.png", "application/octet-stream", []byte("png"))
	if got != "image/png" {
		t.Fatalf("expected image/png, got %q", got)
	}
}
