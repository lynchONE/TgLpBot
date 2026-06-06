package web_server

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"TgLpBot/service/exchange"
)

var tokenAddressRegex = regexp.MustCompile(`^(0x)?[a-fA-F0-9]{40}$`)
var okxCandleBarMap = map[string]string{
	"1m":  "1m",
	"3m":  "3m",
	"5m":  "5m",
	"15m": "15m",
	"30m": "30m",
	"1h":  "1H",
	"2h":  "2H",
	"4h":  "4H",
	"6h":  "6H",
	"12h": "12H",
	"1d":  "1D",
	"1w":  "1W",
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
	PoolAddress  string        `json:"pool_address,omitempty"`
	Bar          string        `json:"bar"`
	Limit        int           `json:"limit"`
	Candles      []tokenCandle `json:"candles"`
	Source       string        `json:"source"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

func normalizeOKXBar(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return "1m"
	}
	if v, ok := okxCandleBarMap[key]; ok {
		return v
	}
	return ""
}

func normalizeTokenAddress(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if !tokenAddressRegex.MatchString(raw) {
		return "", false
	}
	if !strings.HasPrefix(raw, "0x") && !strings.HasPrefix(raw, "0X") {
		raw = "0x" + raw
	}
	return strings.ToLower(raw), true
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

func tokenCandlesFromOKXRows(rows []exchange.MarketCandle) []tokenCandle {
	candles := make([]tokenCandle, 0, len(rows))
	for _, row := range rows {
		ts := row.TimestampMS / 1000
		if ts <= 0 {
			continue
		}
		candles = append(candles, tokenCandle{
			T:       ts,
			O:       sanitizeFloat(row.Open),
			H:       sanitizeFloat(row.High),
			L:       sanitizeFloat(row.Low),
			C:       sanitizeFloat(row.Close),
			V:       sanitizeFloat(row.Volume),
			VUSD:    sanitizeFloat(row.VolumeUSD),
			Confirm: row.Confirm,
		})
	}
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].T < candles[j].T
	})
	return candles
}

func fetchOKXTokenCandles(ctx context.Context, chain string, tokenAddress string, bar string, limit int, before string, after string) ([]tokenCandle, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok || cc.ChainID <= 0 {
		return nil, fmt.Errorf("invalid chain")
	}

	okxSvc := exchange.NewOKXDexService()
	resp, err := okxSvc.GetMarketCandlesWithContext(ctx, exchange.MarketCandlesRequest{
		ChainIndex:           strconv.FormatInt(cc.ChainID, 10),
		TokenContractAddress: tokenAddress,
		Bar:                  bar,
		Limit:                limit,
		Before:               before,
		After:                after,
	})
	if err != nil {
		return nil, err
	}
	return tokenCandlesFromOKXRows(resp.Rows), nil
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

	tokenAddress, ok := normalizeTokenAddress(query.Get("token_address"))
	if !ok {
		http.Error(w, "invalid token_address", http.StatusBadRequest)
		return
	}

	bar := normalizeOKXBar(query.Get("bar"))
	if bar == "" {
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

	candles, err := fetchOKXTokenCandles(ctx, chain, tokenAddress, bar, limit, before, after)
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
		Source:       "okx",
		UpdatedAt:    timeutil.Now(),
	})
}
