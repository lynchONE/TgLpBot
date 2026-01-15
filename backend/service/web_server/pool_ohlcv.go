package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"TgLpBot/base/timeutil"
)

var poolAddressRegex = regexp.MustCompile(`^(0x)?[a-fA-F0-9]{40}$|^(0x)?[a-fA-F0-9]{64}$`)
var chainSlugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type poolOHLCVCandle struct {
	T int64   `json:"t"`
	O float64 `json:"o"`
	H float64 `json:"h"`
	L float64 `json:"l"`
	C float64 `json:"c"`
	V float64 `json:"v"`
}

type poolOHLCVTokenMeta struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	Symbol  string `json:"symbol"`
}

type poolOHLCVEnvelope struct {
	Chain       string              `json:"chain"`
	PoolAddress string              `json:"pool_address"`
	Timeframe   string              `json:"timeframe"`
	Aggregate   int                 `json:"aggregate"`
	Limit       int                 `json:"limit"`
	Base        *poolOHLCVTokenMeta `json:"base,omitempty"`
	Quote       *poolOHLCVTokenMeta `json:"quote,omitempty"`
	Candles     []poolOHLCVCandle   `json:"candles"`
	Source      string              `json:"source"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

func normalizeGeckoTerminalNetwork(chain string) string {
	v := strings.ToLower(strings.TrimSpace(chain))
	switch v {
	case "", "bsc", "bnb":
		return "bsc"
	case "eth", "ethereum":
		return "eth"
	}
	return v
}

func (s *Server) handlePoolOHLCV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	initData := initDataFromQuery(r)
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	query := r.URL.Query()

	chain := normalizeGeckoTerminalNetwork(query.Get("chain"))
	if !chainSlugRegex.MatchString(chain) {
		http.Error(w, "invalid chain", http.StatusBadRequest)
		return
	}
	poolAddress := strings.TrimSpace(query.Get("pool_address"))
	if !poolAddressRegex.MatchString(poolAddress) {
		http.Error(w, "invalid pool_address", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(poolAddress, "0x") && !strings.HasPrefix(poolAddress, "0X") {
		poolAddress = "0x" + poolAddress
	}
	poolAddress = strings.ToLower(poolAddress)

	timeframe := strings.ToLower(strings.TrimSpace(query.Get("timeframe")))
	switch timeframe {
	case "", "minute", "hour", "day":
	default:
		http.Error(w, "invalid timeframe", http.StatusBadRequest)
		return
	}
	if timeframe == "" {
		timeframe = "minute"
	}

	aggregate := 0
	if v := strings.TrimSpace(query.Get("aggregate")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			aggregate = n
		}
	}
	if aggregate <= 0 {
		switch timeframe {
		case "minute":
			aggregate = 5
		default:
			aggregate = 1
		}
	}
	if aggregate < 1 {
		aggregate = 1
	}
	if aggregate > 1440 {
		aggregate = 1440
	}

	limit := 0
	if v := strings.TrimSpace(query.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit <= 0 {
		limit = 200
	}
	if limit < 10 {
		limit = 10
	}
	if limit > 500 {
		limit = 500
	}

	beforeTimestamp := int64(0)
	if v := strings.TrimSpace(query.Get("before_timestamp")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			beforeTimestamp = n
		}
	}

	upstream := fmt.Sprintf(
		"https://api.geckoterminal.com/api/v2/networks/%s/pools/%s/ohlcv/%s?aggregate=%d&limit=%d",
		chain,
		poolAddress,
		timeframe,
		aggregate,
		limit,
	)
	if beforeTimestamp > 0 {
		upstream += fmt.Sprintf("&before_timestamp=%d", beforeTimestamp)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		http.Error(w, string(body), http.StatusBadGateway)
		return
	}

	type geckoOHLCVResponse struct {
		Data struct {
			Attributes struct {
				OHLCVList [][]float64 `json:"ohlcv_list"`
			} `json:"attributes"`
		} `json:"data"`
		Meta struct {
			Base  *poolOHLCVTokenMeta `json:"base"`
			Quote *poolOHLCVTokenMeta `json:"quote"`
		} `json:"meta"`
	}

	var parsed geckoOHLCVResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	candles := make([]poolOHLCVCandle, 0, len(parsed.Data.Attributes.OHLCVList))
	for _, row := range parsed.Data.Attributes.OHLCVList {
		if len(row) < 6 {
			continue
		}
		ts := int64(row[0])
		if ts <= 0 {
			continue
		}
		candles = append(candles, poolOHLCVCandle{
			T: ts,
			O: row[1],
			H: row[2],
			L: row[3],
			C: row[4],
			V: row[5],
		})
	}

	sort.Slice(candles, func(i, j int) bool {
		return candles[i].T < candles[j].T
	})

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(poolOHLCVEnvelope{
		Chain:       chain,
		PoolAddress: poolAddress,
		Timeframe:   timeframe,
		Aggregate:   aggregate,
		Limit:       limit,
		Base:        parsed.Meta.Base,
		Quote:       parsed.Meta.Quote,
		Candles:     candles,
		Source:      "geckoterminal",
		UpdatedAt:   timeutil.Now(),
	})
}
