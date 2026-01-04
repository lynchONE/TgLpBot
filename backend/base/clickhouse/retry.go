package clickhouse

import "strings"

func isRetryableClickHouseError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "eof"):
		return true
	case strings.Contains(msg, "connection reset"):
		return true
	case strings.Contains(msg, "broken pipe"):
		return true
	case strings.Contains(msg, "tls: use of closed connection"):
		return true
	case strings.Contains(msg, "context deadline exceeded"):
		return true
	case strings.Contains(msg, "context canceled"):
		return true
	default:
		return false
	}
}
