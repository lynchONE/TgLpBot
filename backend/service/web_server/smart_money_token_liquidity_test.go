package web_server

import (
	"net/url"
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
