package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
)

var tokenAddressRegex = regexp.MustCompile(`^(0x)?[a-fA-F0-9]{40}$`)
var geckoCandleBarMap = map[string]geckoOHLCVParams{
	"1m":  {timeframe: "minute", aggregate: 1, step: time.Minute},
	"3m":  {timeframe: "minute", aggregate: 3, step: 3 * time.Minute},
	"5m":  {timeframe: "minute", aggregate: 5, step: 5 * time.Minute},
	"15m": {timeframe: "minute", aggregate: 15, step: 15 * time.Minute},
	"30m": {timeframe: "minute", aggregate: 30, step: 30 * time.Minute},
	"1h":  {timeframe: "hour", aggregate: 1, step: time.Hour},
	"2h":  {timeframe: "hour", aggregate: 2, step: 2 * time.Hour},
	"4h":  {timeframe: "hour", aggregate: 4, step: 4 * time.Hour},
	"6h":  {timeframe: "hour", aggregate: 6, step: 6 * time.Hour},
	"12h": {timeframe: "hour", aggregate: 12, step: 12 * time.Hour},
	"1d":  {timeframe: "day", aggregate: 1, step: 24 * time.Hour},
	"1w":  {timeframe: "day", aggregate: 7, step: 7 * 24 * time.Hour},
}

type geckoOHLCVParams struct {
	timeframe string
	aggregate int
	step      time.Duration
}

type tokenCandle struct {
	T       int64   `json:"t"`
	O       float64 `json:"o"`
	H       float64 `json:"h"`
	L       float64 `json:"l"`
	C       float64 `json:"c"`
	V       float64 `json:"v"`
	VUSD    float64 `json:"v_usd"`
	Confirm bool    `json:"confirm"`
}

type tokenCandlesEnvelope struct {
	Chain        string        `json:"chain"`
	TokenAddress string        `json:"token_address"`
	Bar          string        `json:"bar"`
	Limit        int           `json:"limit"`
	Candles      []tokenCandle `json:"candles"`
	Source       string        `json:"source"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type geckoTokenPoolsResponse struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			Address      string `json:"address"`
			ReserveInUSD string `json:"reserve_in_usd"`
		} `json:"attributes"`
	} `json:"data"`
}

type geckoOHLCVResponse struct {
	Data struct {
		Attributes struct {
			OHLCVList [][]float64 `json:"ohlcv_list"`
		} `json:"attributes"`
	} `json:"data"`
}

func normalizeGeckoBar(raw string) (string, geckoOHLCVParams, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		key = "1m"
	}
	v, ok := geckoCandleBarMap[key]
	if !ok {
		return "", geckoOHLCVParams{}, false
	}
	return key, v, true
}

func isDigitsOnly(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return false
		}
	}
	return true
}

func geckoNetworkForChain(chain string) string {
	chain = strings.ToLower(strings.TrimSpace(chain))
	switch chain {
	case "", "bsc", "bnb":
		return "bsc"
	case "base":
		return "base"
	case "eth", "ethereum":
		return "eth"
	default:
		return chain
	}
}

func geckoTokenIDAddress(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	idx := strings.LastIndex(id, "_")
	if idx >= 0 && idx+1 < len(id) {
		id = id[idx+1:]
	}
	id = strings.ToLower(strings.TrimSpace(id))
	if tokenAddressRegex.MatchString(id) {
		if !strings.HasPrefix(id, "0x") {
			id = "0x" + id
		}
		return id
	}
	return ""
}

func parseGeckoFloat(raw string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0
	}
	return sanitizeFloat(v)
}

func fetchGeckoBestPoolAddress(ctx context.Context, client *http.Client, chain string, tokenAddress string) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	network := geckoNetworkForChain(chain)
	endpoint := fmt.Sprintf(
		"https://api.geckoterminal.com/api/v2/networks/%s/tokens/%s/pools?page=1",
		url.PathEscape(network),
		url.PathEscape(strings.ToLower(strings.TrimSpace(tokenAddress))),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("geckoterminal token pools http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed geckoTokenPoolsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}

	bestAddress := ""
	bestReserve := -1.0
	for _, row := range parsed.Data {
		addr := strings.ToLower(strings.TrimSpace(row.Attributes.Address))
		if addr == "" {
			addr = geckoTokenIDAddress(row.ID)
		}
		if addr == "" {
			continue
		}
		reserve := parseGeckoFloat(row.Attributes.ReserveInUSD)
		if reserve > bestReserve {
			bestReserve = reserve
			bestAddress = addr
		}
	}
	if bestAddress == "" {
		return "", fmt.Errorf("geckoterminal did not return pools for token %s", tokenAddress)
	}
	return bestAddress, nil
}

func fetchGeckoPoolCandles(ctx context.Context, client *http.Client, chain string, poolAddress string, barParams geckoOHLCVParams, limit int, before string) ([]tokenCandle, error) {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	if barParams.timeframe == "" || barParams.aggregate <= 0 {
		return nil, fmt.Errorf("invalid geckoterminal ohlcv params")
	}
	network := geckoNetworkForChain(chain)
	values := url.Values{}
	values.Set("aggregate", strconv.Itoa(barParams.aggregate))
	values.Set("limit", strconv.Itoa(limit))
	values.Set("currency", "usd")
	if before != "" {
		values.Set("before_timestamp", before)
	}
	endpoint := fmt.Sprintf(
		"https://api.geckoterminal.com/api/v2/networks/%s/pools/%s/ohlcv/%s?%s",
		url.PathEscape(network),
		url.PathEscape(strings.ToLower(strings.TrimSpace(poolAddress))),
		url.PathEscape(barParams.timeframe),
		values.Encode(),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("geckoterminal ohlcv http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed geckoOHLCVResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	candles := make([]tokenCandle, 0, len(parsed.Data.Attributes.OHLCVList))
	now := time.Now()
	for _, row := range parsed.Data.Attributes.OHLCVList {
		if len(row) < 6 {
			continue
		}
		ts := int64(row[0])
		if ts <= 0 {
			continue
		}
		confirm := time.Unix(ts, 0).Add(barParams.step).Before(now)
		candles = append(candles, tokenCandle{
			T:       ts,
			O:       sanitizeFloat(row[1]),
			H:       sanitizeFloat(row[2]),
			L:       sanitizeFloat(row[3]),
			C:       sanitizeFloat(row[4]),
			V:       sanitizeFloat(row[5]),
			Confirm: confirm,
		})
	}
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].T < candles[j].T
	})
	return candles, nil
}

func (s *Server) handleTokenCandles(w http.ResponseWriter, r *http.Request) {
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
	if status, msg := requireModulePermission(check, models.AccessModuleGMGNKline); status != 0 {
		http.Error(w, msg, status)
		return
	}

	query := r.URL.Query()
	chain := config.NormalizeChain(query.Get("chain"))
	if chain == "" {
		chain = config.PickEnabledChain("bsc")
	}
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok || cc.ChainID <= 0 {
		http.Error(w, "invalid chain", http.StatusBadRequest)
		return
	}

	tokenAddress := strings.TrimSpace(query.Get("token_address"))
	if !tokenAddressRegex.MatchString(tokenAddress) {
		http.Error(w, "invalid token_address", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(tokenAddress, "0x") && !strings.HasPrefix(tokenAddress, "0X") {
		tokenAddress = "0x" + tokenAddress
	}
	tokenAddress = strings.ToLower(tokenAddress)

	bar, barParams, ok := normalizeGeckoBar(query.Get("bar"))
	if !ok {
		http.Error(w, "invalid bar", http.StatusBadRequest)
		return
	}

	limit := 240
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil {
			limit = n
		}
	}
	if limit <= 0 {
		limit = 240
	}
	if limit > 299 {
		limit = 299
	}

	before := strings.TrimSpace(query.Get("before"))
	if before != "" && !isDigitsOnly(before) {
		http.Error(w, "invalid before", http.StatusBadRequest)
		return
	}
	after := strings.TrimSpace(query.Get("after"))
	if after != "" && !isDigitsOnly(after) {
		http.Error(w, "invalid after", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	poolAddress := strings.TrimSpace(query.Get("pool_address"))
	if poolAddress != "" {
		if !tokenAddressRegex.MatchString(poolAddress) {
			http.Error(w, "invalid pool_address", http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(poolAddress, "0x") && !strings.HasPrefix(poolAddress, "0X") {
			poolAddress = "0x" + poolAddress
		}
		poolAddress = strings.ToLower(poolAddress)
	} else {
		var poolErr error
		poolAddress, poolErr = fetchGeckoBestPoolAddress(ctx, nil, chain, tokenAddress)
		if poolErr != nil {
			http.Error(w, poolErr.Error(), http.StatusBadGateway)
			return
		}
	}

	candles, err := fetchGeckoPoolCandles(ctx, nil, chain, poolAddress, barParams, limit, before)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, tokenCandlesEnvelope{
		Chain:        chain,
		TokenAddress: tokenAddress,
		Bar:          bar,
		Limit:        limit,
		Candles:      candles,
		Source:       "geckoterminal",
		UpdatedAt:    timeutil.Now(),
	})
}
