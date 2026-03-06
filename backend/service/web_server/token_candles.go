package web_server

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"TgLpBot/base/config"
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
	if status, msg := requireMiniAppPermission(check); status != 0 {
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

	okxSvc := exchange.NewOKXDexService()
	resp, err := okxSvc.GetMarketCandles(exchange.MarketCandlesRequest{
		ChainIndex:           strconv.FormatInt(cc.ChainID, 10),
		TokenContractAddress: tokenAddress,
		Bar:                  bar,
		Limit:                limit,
		Before:               before,
		After:                after,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	candles := make([]tokenCandle, 0, len(resp.Rows))
	for _, row := range resp.Rows {
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
