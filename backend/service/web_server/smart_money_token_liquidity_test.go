package web_server

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseSMTokenLiquidityTimeRangeUsesAbsoluteRange(t *testing.T) {
	values := url.Values{}
	values.Set("start_time", "2026-06-07T12:30:15+08:00")
	values.Set("end_time", "2026-06-07T15:45:45+08:00")

	startTime, endTime, windowHours, err := parseSMTokenLiquidityTimeRange(values)
	if err != nil {
		t.Fatalf("expected absolute time range: %v", err)
	}
	if got, want := startTime.Format("2006-01-02T15:04:05Z07:00"), "2026-06-07T04:30:15Z"; got != want {
		t.Fatalf("start time = %s, want %s", got, want)
	}
	if got, want := endTime.Format("2006-01-02T15:04:05Z07:00"), "2026-06-07T07:45:45Z"; got != want {
		t.Fatalf("end time = %s, want %s", got, want)
	}
	if windowHours != 4 {
		t.Fatalf("window hours = %d, want 4", windowHours)
	}
}

func TestParseSMTokenLiquidityTimeRangeRejectsInvalidAbsoluteRange(t *testing.T) {
	values := url.Values{}
	values.Set("start_time", "2026-06-07T15:45:45Z")
	values.Set("end_time", "2026-06-07T12:30:15Z")

	_, _, _, err := parseSMTokenLiquidityTimeRange(values)
	if err == nil {
		t.Fatal("expected invalid range error")
	}
}

func TestParseSMTokenLiquidityTimeRangeRejectsTooLargeRange(t *testing.T) {
	values := url.Values{}
	values.Set("start_time", "2026-06-01T00:00:00Z")
	values.Set("end_time", "2026-06-09T00:00:00Z")

	_, _, _, err := parseSMTokenLiquidityTimeRange(values)
	if err == nil {
		t.Fatal("expected too large range error")
	}
}

func TestSmartMoneyPoolLiquidityScanErrorMessageSanitizesCloudflareHTML(t *testing.T) {
	html := `<!DOCTYPE html><html><head><title>hotpool.ink | 504: Gateway time-out</title></head><body>Cloudflare Error code 504</body></html>`

	msg := smartMoneyPoolLiquidityScanErrorMessage(fmt.Errorf("price lookup failed: %s", html), 502)

	if strings.Contains(strings.ToLower(msg), "<html") || strings.Contains(strings.ToLower(msg), "cloudflare") {
		t.Fatalf("expected sanitized message, got %q", msg)
	}
	if !strings.Contains(msg, "扫描服务超时") {
		t.Fatalf("expected timeout message, got %q", msg)
	}
}

func TestWriteSmartMoneySSEEventWritesAndFlushes(t *testing.T) {
	rr := httptest.NewRecorder()

	err := writeSmartMoneySSEEvent(rr, rr, "candidate", map[string]interface{}{
		"wallet": "0x0000000000000000000000000000000000000001",
	})
	if err != nil {
		t.Fatalf("expected sse event to be written: %v", err)
	}
	body := rr.Body.String()
	if !strings.HasPrefix(body, "event: candidate\n") {
		t.Fatalf("expected candidate event prefix, got %q", body)
	}
	if !strings.Contains(body, `"wallet":"0x0000000000000000000000000000000000000001"`) {
		t.Fatalf("expected json data payload, got %q", body)
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Fatalf("expected sse event delimiter, got %q", body)
	}
	if !rr.Flushed {
		t.Fatal("expected sse event to flush")
	}
}

func TestWriteSmartMoneySSEEventRejectsEmptyName(t *testing.T) {
	rr := httptest.NewRecorder()

	err := writeSmartMoneySSEEvent(rr, rr, " ", map[string]interface{}{})
	if err == nil {
		t.Fatal("expected empty event name error")
	}
}
