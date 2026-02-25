package web_server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlePositions_InvalidEndpoint(t *testing.T) {
	srv := NewServer(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/positions?endpoint=bad", nil)
	rr := httptest.NewRecorder()

	srv.handlePositions(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleTaskAction_InvalidAction(t *testing.T) {
	srv := NewServer(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/task_action?action=bad", nil)
	rr := httptest.NewRecorder()

	srv.handleTaskAction(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleGetPools_InvalidEndpoint(t *testing.T) {
	srv := NewServer(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/pools?endpoint=bad", nil)
	rr := httptest.NewRecorder()

	srv.handleGetPools(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}
