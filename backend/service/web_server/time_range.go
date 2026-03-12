package web_server

import (
	"strconv"
	"strings"
	"time"
)

const maxMarkerQuerySpan = 15 * 24 * time.Hour

func parseUnixSecondsQuery(q map[string][]string, key string) int64 {
	raw := strings.TrimSpace("")
	if q != nil {
		if values := q[key]; len(values) > 0 {
			raw = strings.TrimSpace(values[0])
		}
	}
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

func resolveUnixTimeRange(startTS int64, endTS int64, fallbackWindow time.Duration) (time.Time, time.Time) {
	if fallbackWindow <= 0 {
		fallbackWindow = 12 * time.Hour
	}

	if startTS > 0 && endTS > 0 {
		start := time.Unix(startTS, 0).UTC()
		end := time.Unix(endTS, 0).UTC()
		if end.Before(start) {
			start, end = end, start
		}
		if span := end.Sub(start); span > maxMarkerQuerySpan {
			start = end.Add(-maxMarkerQuerySpan)
		}
		return start, end
	}

	end := time.Now().UTC()
	start := end.Add(-fallbackWindow)
	return start, end
}

func durationSeconds(start time.Time, end time.Time) int {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	if end.Before(start) {
		start, end = end, start
	}
	return int(end.Sub(start).Seconds())
}
