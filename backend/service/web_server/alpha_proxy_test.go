package web_server

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadAlphaOverviewReturnsBothSources(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data":
			_, _ = w.Write([]byte(`{"airdrops":[{"token":"NES"}]}`))
		case "/stability":
			_, _ = w.Write([]byte(`{"items":[{"n":"NES/USDT","st":"red:unstable"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	got := loadAlphaOverview(context.Background(), upstream.Client(), []alphaOverviewSource{
		{Name: "data", URL: upstream.URL + "/data"},
		{Name: "stability", URL: upstream.URL + "/stability"},
	})

	if len(got.Data) == 0 {
		t.Fatalf("expected data payload")
	}
	if len(got.Stability) == 0 {
		t.Fatalf("expected stability payload")
	}
	if len(got.Errors) != 0 {
		t.Fatalf("expected no errors, got %+v", got.Errors)
	}
}

func TestLoadAlphaOverviewKeepsPartialSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data":
			_, _ = w.Write([]byte(`{"airdrops":[{"token":"NES"}]}`))
		case "/stability":
			http.Error(w, "upstream failed", http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	got := loadAlphaOverview(context.Background(), upstream.Client(), []alphaOverviewSource{
		{Name: "data", URL: upstream.URL + "/data"},
		{Name: "stability", URL: upstream.URL + "/stability"},
	})

	if len(got.Data) == 0 {
		t.Fatalf("expected data payload")
	}
	if len(got.Stability) != 0 {
		t.Fatalf("expected empty stability payload, got %s", string(got.Stability))
	}
	if got.Errors["stability"] == "" {
		t.Fatalf("expected stability error, got %+v", got.Errors)
	}
}

func TestFetchAlphaOverviewSourceUsesBrowserHeadersByDefault(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != alphaOverviewBrowserUA {
			t.Fatalf("unexpected user-agent: %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json, text/plain, */*" {
			t.Fatalf("unexpected accept: %q", got)
		}
		if got := r.Header.Get("Accept-Encoding"); got != "gzip, deflate, br" {
			t.Fatalf("unexpected accept-encoding: %q", got)
		}
		if got := r.Header.Get("Sec-Fetch-Site"); got != "same-origin" {
			t.Fatalf("unexpected sec-fetch-site: %q", got)
		}

		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		_, _ = gz.Write([]byte(`{"airdrops":[{"token":"NES"}]}`))
	}))
	defer upstream.Close()

	t.Setenv("ALPHA_FETCH_PROFILE", "")
	got, err := fetchAlphaOverviewSource(context.Background(), upstream.Client(), alphaOverviewSource{
		Name: "data",
		URL:  upstream.URL,
	})
	if err != nil {
		t.Fatalf("fetch alpha source: %v", err)
	}
	if string(got) != `{"airdrops":[{"token":"NES"}]}` {
		t.Fatalf("unexpected payload: %s", string(got))
	}
}

func TestFetchAlphaOverviewSourceCanUsePostmanHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "PostmanRuntime/7.51.1" {
			t.Fatalf("unexpected user-agent: %q", got)
		}
		if got := r.Header.Get("Accept"); got != "*/*" {
			t.Fatalf("unexpected accept: %q", got)
		}
		if got := r.Header.Get("Postman-Token"); got == "" {
			t.Fatalf("expected postman token header")
		}

		_, _ = w.Write([]byte(`{"airdrops":[{"token":"NES"}]}`))
	}))
	defer upstream.Close()

	t.Setenv("ALPHA_FETCH_PROFILE", "postman")
	got, err := fetchAlphaOverviewSource(context.Background(), upstream.Client(), alphaOverviewSource{
		Name: "data",
		URL:  upstream.URL,
	})
	if err != nil {
		t.Fatalf("fetch alpha source: %v", err)
	}
	if string(got) != `{"airdrops":[{"token":"NES"}]}` {
		t.Fatalf("unexpected payload: %s", string(got))
	}
}

func TestHandleAlphaOverviewRejectsPost(t *testing.T) {
	srv := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/api/alpha", nil)
	rr := httptest.NewRecorder()

	srv.handleAlphaOverview(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

func TestAlphaOverviewResponseMarshalsRawPayloads(t *testing.T) {
	resp := alphaOverviewResponse{
		Data:      json.RawMessage(`{"ok":true}`),
		Stability: json.RawMessage(`{"items":[]}`),
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if string(b) != `{"data":{"ok":true},"stability":{"items":[]}}` {
		t.Fatalf("unexpected response json: %s", string(b))
	}
}
