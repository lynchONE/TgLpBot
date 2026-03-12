package web_server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
)

type myTradeMarkersRequest struct {
	InitData  string `json:"initData"`
	Chain     string `json:"chain"`
	PoolID    string `json:"pool_id"`
	WindowSec int    `json:"window_sec"`
	StartTS   int64  `json:"start_ts"`
	EndTS     int64  `json:"end_ts"`
	BucketSec int    `json:"bucket_sec"`
}

type myTradeMarkerEvent struct {
	T            int64   `json:"t"`
	BucketT      int64   `json:"bucket_t"`
	Action       string  `json:"action"`
	TxHash       string  `json:"tx_hash,omitempty"`
	TxURL        string  `json:"tx_url,omitempty"`
	EstimatedUSD float64 `json:"estimated_usd"`
	IsMyTrade    bool    `json:"is_my_trade"`
}

type myTradeMarkersResponse struct {
	Events []myTradeMarkerEvent `json:"events"`
}

func bucketUnix(ts int64, bucketSec int) int64 {
	if ts <= 0 {
		return 0
	}
	if bucketSec <= 0 {
		bucketSec = 300
	}
	size := int64(bucketSec)
	return (ts / size) * size
}

func (s *Server) handleMyTradeMarkers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req myTradeMarkersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	user, status, msg := authenticateTelegramWebAppUser(req.InitData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	chain := config.NormalizeChain(req.Chain)
	poolID := strings.TrimSpace(req.PoolID)
	if poolID == "" {
		http.Error(w, "pool_id required", http.StatusBadRequest)
		return
	}

	windowSec := req.WindowSec
	if windowSec <= 0 || windowSec > 7*24*3600 {
		windowSec = 24 * 3600
	}
	bucketSec := req.BucketSec
	if bucketSec < 60 || bucketSec > 86400 {
		bucketSec = 300
	}
	rangeStart, rangeEnd := resolveUnixTimeRange(req.StartTS, req.EndTS, time.Duration(windowSec)*time.Second)
	queryStart := rangeStart.Add(-time.Duration(bucketSec) * time.Second)
	queryEnd := rangeEnd.Add(time.Duration(bucketSec) * time.Second)
	startBucket, endBucket := buildMarkerBucketBounds(rangeStart, rangeEnd, bucketSec)

	var records []models.TradeRecord
	q := database.DB.
		Where("user_id = ? AND pool_id = ?", user.ID, poolID).
		Where("(opened_at BETWEEN ? AND ?) OR (closed_at IS NOT NULL AND closed_at BETWEEN ? AND ?)", queryStart, queryEnd, queryStart, queryEnd).
		Order("opened_at ASC")
	if chain != "" {
		q = q.Where("chain = ?", chain)
	}
	if err := q.Limit(400).Find(&records).Error; err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	explorerBase := "https://bscscan.com"
	if chain == "base" {
		explorerBase = "https://basescan.org"
	}

	var events []myTradeMarkerEvent
	for _, rec := range records {
		openBucket := bucketUnix(rec.OpenedAt.Unix(), bucketSec)
		// open event
		openUSD := parseWeiToFloat(rec.OpenUSDTSpent)
		if openUSD > 0 && openBucket >= startBucket && openBucket <= endBucket {
			txURL := ""
			if rec.OpenTxHash != "" {
				txURL = explorerBase + "/tx/" + rec.OpenTxHash
			}
			events = append(events, myTradeMarkerEvent{
				T:            rec.OpenedAt.Unix(),
				BucketT:      openBucket,
				Action:       "add",
				TxHash:       rec.OpenTxHash,
				TxURL:        txURL,
				EstimatedUSD: openUSD,
				IsMyTrade:    true,
			})
		}

		// close event
		if rec.ClosedAt != nil && rec.Status == models.TradeStatusClosed {
			closeBucket := bucketUnix(rec.ClosedAt.Unix(), bucketSec)
			closeUSD := parseWeiToFloat(rec.CloseUSDTReceived)
			if closeUSD > 0 && closeBucket >= startBucket && closeBucket <= endBucket {
				txURL := ""
				if rec.CloseTxHash != "" {
					txURL = explorerBase + "/tx/" + rec.CloseTxHash
				}
				events = append(events, myTradeMarkerEvent{
					T:            rec.ClosedAt.Unix(),
					BucketT:      closeBucket,
					Action:       "remove",
					TxHash:       rec.CloseTxHash,
					TxURL:        txURL,
					EstimatedUSD: closeUSD,
					IsMyTrade:    true,
				})
			}
		}
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].T == events[j].T {
			return events[i].Action < events[j].Action
		}
		return events[i].T < events[j].T
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(myTradeMarkersResponse{Events: events})
}

func parseWeiToFloat(weiStr string) float64 {
	s := strings.TrimSpace(weiStr)
	if s == "" || s == "0" {
		return 0
	}
	// Simple conversion: treat as 18 decimal wei
	// For amounts < 10^15 digits, use float parsing
	if len(s) <= 15 {
		return 0
	}
	intPart := s[:len(s)-18]
	if intPart == "" {
		intPart = "0"
	}
	var result float64
	for _, c := range intPart {
		result = result*10 + float64(c-'0')
	}
	// Add fractional part (first 2 digits of the 18 decimals)
	if len(s) >= 18 {
		fracStr := s[len(s)-18:]
		if len(fracStr) >= 2 {
			frac := float64(fracStr[0]-'0')*0.1 + float64(fracStr[1]-'0')*0.01
			result += frac
		}
	}
	return result
}
