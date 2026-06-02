package web_server

import (
	"context"
	"encoding/json"
	"errors"
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
var geckoPoolAddressRegex = regexp.MustCompile(`^(0x)?(?:[a-fA-F0-9]{40}|[a-fA-F0-9]{64})$`)
var geckoTerminalAPIBaseURL = "https://api.geckoterminal.com/api/v2"
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
	PoolAddress  string        `json:"pool_address,omitempty"`
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
		Relationships struct {
			BaseToken struct {
				Data struct {
					ID string `json:"id"`
				} `json:"data"`
			} `json:"base_token"`
			QuoteToken struct {
				Data struct {
					ID string `json:"id"`
				} `json:"data"`
			} `json:"quote_token"`
			Dex struct {
				Data struct {
					ID string `json:"id"`
				} `json:"data"`
			} `json:"dex"`
		} `json:"relationships"`
	} `json:"data"`
}

type geckoPoolResponse struct {
	Data struct {
		ID         string `json:"id"`
		Attributes struct {
			Address string `json:"address"`
		} `json:"attributes"`
		Relationships struct {
			BaseToken struct {
				Data struct {
					ID string `json:"id"`
				} `json:"data"`
			} `json:"base_token"`
			QuoteToken struct {
				Data struct {
					ID string `json:"id"`
				} `json:"data"`
			} `json:"quote_token"`
			Dex struct {
				Data struct {
					ID string `json:"id"`
				} `json:"data"`
			} `json:"dex"`
		} `json:"relationships"`
	} `json:"data"`
}

type geckoOHLCVResponse struct {
	Data struct {
		Attributes struct {
			OHLCVList [][]float64 `json:"ohlcv_list"`
		} `json:"attributes"`
	} `json:"data"`
}

type geckoPoolCandidate struct {
	address           string
	reserveUSD        float64
	dexID             string
	baseTokenAddress  string
	quoteTokenAddress string
}

type geckoPoolDetails struct {
	address           string
	dexID             string
	baseTokenAddress  string
	quoteTokenAddress string
}

type geckoHTTPError struct {
	scope  string
	status int
	body   string
}

func (e *geckoHTTPError) Error() string {
	if e == nil {
		return ""
	}
	body := strings.TrimSpace(e.body)
	if body == "" {
		return fmt.Sprintf("geckoterminal %s http %d", e.scope, e.status)
	}
	return fmt.Sprintf("geckoterminal %s http %d: %s", e.scope, e.status, body)
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

func geckoAPIURL(format string, args ...any) string {
	base := strings.TrimRight(strings.TrimSpace(geckoTerminalAPIBaseURL), "/")
	return base + fmt.Sprintf(format, args...)
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

func normalizeGeckoPoolAddress(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if !geckoPoolAddressRegex.MatchString(raw) {
		return "", false
	}
	if !strings.HasPrefix(raw, "0x") && !strings.HasPrefix(raw, "0X") {
		raw = "0x" + raw
	}
	return strings.ToLower(raw), true
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

func geckoPoolIDAddress(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	idx := strings.LastIndex(id, "_")
	if idx >= 0 && idx+1 < len(id) {
		id = id[idx+1:]
	}
	addr, ok := normalizeGeckoPoolAddress(id)
	if !ok {
		return ""
	}
	return addr
}

func parseGeckoFloat(raw string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0
	}
	return sanitizeFloat(v)
}

func fetchGeckoPoolDetails(ctx context.Context, client *http.Client, chain string, poolAddress string) (geckoPoolDetails, error) {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	poolAddress, ok := normalizeGeckoPoolAddress(poolAddress)
	if !ok {
		return geckoPoolDetails{}, fmt.Errorf("invalid geckoterminal pool address")
	}
	network := geckoNetworkForChain(chain)
	endpoint := geckoAPIURL(
		"/networks/%s/pools/%s",
		url.PathEscape(network),
		url.PathEscape(poolAddress),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return geckoPoolDetails{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return geckoPoolDetails{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return geckoPoolDetails{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return geckoPoolDetails{}, &geckoHTTPError{scope: "pool details", status: resp.StatusCode, body: string(body)}
	}

	var parsed geckoPoolResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return geckoPoolDetails{}, err
	}
	addr := strings.TrimSpace(parsed.Data.Attributes.Address)
	if addr == "" {
		addr = geckoPoolIDAddress(parsed.Data.ID)
	}
	normalized, ok := normalizeGeckoPoolAddress(addr)
	if !ok {
		return geckoPoolDetails{}, fmt.Errorf("geckoterminal did not return usable pool address for %s", poolAddress)
	}
	return geckoPoolDetails{
		address:           normalized,
		dexID:             strings.ToLower(strings.TrimSpace(parsed.Data.Relationships.Dex.Data.ID)),
		baseTokenAddress:  geckoTokenIDAddress(parsed.Data.Relationships.BaseToken.Data.ID),
		quoteTokenAddress: geckoTokenIDAddress(parsed.Data.Relationships.QuoteToken.Data.ID),
	}, nil
}

func geckoPrimaryQuoteTokenAddresses(chain string) map[string]struct{} {
	out := make(map[string]struct{})
	add := func(raw string) {
		addr, ok := normalizeTokenAddress(raw)
		if ok {
			out[addr] = struct{}{}
		}
	}

	chain = strings.ToLower(strings.TrimSpace(chain))
	switch chain {
	case "base":
		add("0x4200000000000000000000000000000000000006") // WETH
		add("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913") // USDC
		add("0xd9aAEc86B65D86f6A7B5B1b0c42FFA531710b6CA") // USDbC
		add("0xfde4C96c8593536E31F229EA8f37b2ADa2699bb2") // USDT
	case "eth", "ethereum":
		add("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2") // WETH
		add("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48") // USDC
		add("0xdAC17F958D2ee523a2206206994597C13D831ec7") // USDT
		add("0x6B175474E89094C44Da98b954EedeAC495271d0F") // DAI
	default:
		add("0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c") // WBNB
		add("0x55d398326f99059fF775485246999027B3197955") // USDT
		add("0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d") // USDC
		add("0xe9e7CEA3DedcA5984780Bafc599bD69ADd087D56") // BUSD
		add("0x8fFf93E810a2eDaaFc326eDEE51071DA9d398E83") // BRL
	}
	return out
}

func pickGeckoKlineTokenAddress(chain string, requestedTokenAddress string, pool geckoPoolDetails) string {
	requestedTokenAddress, _ = normalizeTokenAddress(requestedTokenAddress)
	base := pool.baseTokenAddress
	quote := pool.quoteTokenAddress
	if base == "" || quote == "" {
		return requestedTokenAddress
	}
	quoteTokens := geckoPrimaryQuoteTokenAddresses(chain)
	_, baseIsQuote := quoteTokens[base]
	_, quoteIsQuote := quoteTokens[quote]
	switch {
	case baseIsQuote && !quoteIsQuote:
		return quote
	case quoteIsQuote && !baseIsQuote:
		return base
	case requestedTokenAddress == base || requestedTokenAddress == quote:
		return requestedTokenAddress
	default:
		return base
	}
}

func fetchGeckoPoolCandidates(ctx context.Context, client *http.Client, chain string, tokenAddress string) ([]geckoPoolCandidate, error) {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	network := geckoNetworkForChain(chain)
	endpoint := geckoAPIURL(
		"/networks/%s/tokens/%s/pools?page=1",
		url.PathEscape(network),
		url.PathEscape(strings.ToLower(strings.TrimSpace(tokenAddress))),
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &geckoHTTPError{scope: "token pools", status: resp.StatusCode, body: string(body)}
	}

	var parsed geckoTokenPoolsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	candidates := make([]geckoPoolCandidate, 0, len(parsed.Data))
	seen := make(map[string]struct{}, len(parsed.Data))
	for _, row := range parsed.Data {
		addr := strings.TrimSpace(row.Attributes.Address)
		if addr == "" {
			addr = geckoPoolIDAddress(row.ID)
		}
		normalized, ok := normalizeGeckoPoolAddress(addr)
		if !ok {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		reserve := parseGeckoFloat(row.Attributes.ReserveInUSD)
		candidates = append(candidates, geckoPoolCandidate{
			address:           normalized,
			reserveUSD:        reserve,
			dexID:             strings.ToLower(strings.TrimSpace(row.Relationships.Dex.Data.ID)),
			baseTokenAddress:  geckoTokenIDAddress(row.Relationships.BaseToken.Data.ID),
			quoteTokenAddress: geckoTokenIDAddress(row.Relationships.QuoteToken.Data.ID),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].reserveUSD > candidates[j].reserveUSD
	})
	if len(candidates) == 0 {
		return nil, fmt.Errorf("geckoterminal did not return usable pools for token %s", tokenAddress)
	}
	return candidates, nil
}

func fetchGeckoBestPoolAddress(ctx context.Context, client *http.Client, chain string, tokenAddress string) (string, error) {
	candidates, err := fetchGeckoPoolCandidates(ctx, client, chain, tokenAddress)
	if err != nil {
		return "", err
	}
	return candidates[0].address, nil
}

func fetchGeckoPoolCandles(ctx context.Context, client *http.Client, chain string, poolAddress string, tokenAddress string, barParams geckoOHLCVParams, limit int, before string) ([]tokenCandle, error) {
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
	if tokenAddress != "" {
		values.Set("token", strings.ToLower(strings.TrimSpace(tokenAddress)))
	}
	if before != "" {
		values.Set("before_timestamp", before)
	}
	endpoint := geckoAPIURL(
		"/networks/%s/pools/%s/ohlcv/%s?%s",
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
		return nil, &geckoHTTPError{scope: "ohlcv", status: resp.StatusCode, body: string(body)}
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

func shouldTryNextGeckoPool(err error) bool {
	var geckoErr *geckoHTTPError
	if errors.As(err, &geckoErr) {
		return geckoErr.status == http.StatusBadRequest || geckoErr.status == http.StatusNotFound
	}
	return false
}

func appendUniqueGeckoPoolCandidate(candidates []geckoPoolCandidate, seen map[string]struct{}, candidate geckoPoolCandidate) []geckoPoolCandidate {
	normalized, ok := normalizeGeckoPoolAddress(candidate.address)
	if !ok {
		return candidates
	}
	if _, ok := seen[normalized]; ok {
		return candidates
	}
	seen[normalized] = struct{}{}
	candidate.address = normalized
	return append(candidates, candidate)
}

func geckoPoolCandidateHasToken(candidate geckoPoolCandidate, tokenAddress string) bool {
	return tokenAddress != "" && (candidate.baseTokenAddress == tokenAddress || candidate.quoteTokenAddress == tokenAddress)
}

func geckoPoolCandidatePairsWithPrimaryQuote(chain string, candidate geckoPoolCandidate, tokenAddress string) bool {
	if !geckoPoolCandidateHasToken(candidate, tokenAddress) {
		return false
	}
	quoteTokens := geckoPrimaryQuoteTokenAddresses(chain)
	if candidate.baseTokenAddress == tokenAddress {
		_, ok := quoteTokens[candidate.quoteTokenAddress]
		return ok
	}
	if candidate.quoteTokenAddress == tokenAddress {
		_, ok := quoteTokens[candidate.baseTokenAddress]
		return ok
	}
	return false
}

func sortGeckoPoolCandidatesForKline(chain string, candidates []geckoPoolCandidate, tokenAddress string, preferredDexID string) {
	tokenAddress, _ = normalizeTokenAddress(tokenAddress)
	preferredDexID = strings.ToLower(strings.TrimSpace(preferredDexID))
	score := func(candidate geckoPoolCandidate) int {
		score := 0
		if preferredDexID != "" && strings.EqualFold(candidate.dexID, preferredDexID) {
			score += 100
		}
		if geckoPoolCandidatePairsWithPrimaryQuote(chain, candidate, tokenAddress) {
			score += 20
		}
		if geckoPoolCandidateHasToken(candidate, tokenAddress) {
			score += 5
		}
		return score
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftScore := score(candidates[i])
		rightScore := score(candidates[j])
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return candidates[i].reserveUSD > candidates[j].reserveUSD
	})
}

func fetchGeckoTokenCandles(ctx context.Context, client *http.Client, chain string, tokenAddress string, preferredPoolAddress string, barParams geckoOHLCVParams, limit int, before string) ([]tokenCandle, string, string, error) {
	tokenAddress, _ = normalizeTokenAddress(tokenAddress)
	klineTokenAddress := tokenAddress
	preferredDexID := ""
	preferredPoolAddress, hasPreferredPool := normalizeGeckoPoolAddress(preferredPoolAddress)
	if hasPreferredPool {
		if details, err := fetchGeckoPoolDetails(ctx, client, chain, preferredPoolAddress); err == nil {
			klineTokenAddress = pickGeckoKlineTokenAddress(chain, tokenAddress, details)
			preferredDexID = details.dexID
			preferredPoolAddress = details.address
		}
		candles, err := fetchGeckoPoolCandles(ctx, client, chain, preferredPoolAddress, klineTokenAddress, barParams, limit, before)
		if err == nil && len(candles) > 0 {
			return candles, preferredPoolAddress, klineTokenAddress, nil
		}
		if err == nil {
			err = fmt.Errorf("geckoterminal returned empty candles for pool %s", preferredPoolAddress)
		}
		if !shouldTryNextGeckoPool(err) && !isEmptyGeckoCandlesError(err) {
			return nil, preferredPoolAddress, klineTokenAddress, err
		}
	}

	poolCandidates, poolErr := fetchGeckoPoolCandidates(ctx, client, chain, klineTokenAddress)
	if poolErr != nil {
		return nil, "", klineTokenAddress, poolErr
	}
	sortGeckoPoolCandidatesForKline(chain, poolCandidates, klineTokenAddress, preferredDexID)
	candidates := make([]geckoPoolCandidate, 0, 8)
	seen := make(map[string]struct{}, len(poolCandidates)+1)
	if hasPreferredPool {
		seen[preferredPoolAddress] = struct{}{}
	}
	for _, candidate := range poolCandidates {
		candidates = appendUniqueGeckoPoolCandidate(candidates, seen, candidate)
		if len(candidates) >= 8 {
			break
		}
	}
	if len(candidates) == 0 {
		if poolErr != nil {
			return nil, "", klineTokenAddress, poolErr
		}
		return nil, "", klineTokenAddress, fmt.Errorf("geckoterminal did not return usable pools for token %s", klineTokenAddress)
	}

	var lastErr error
	for _, candidate := range candidates {
		candles, err := fetchGeckoPoolCandles(ctx, client, chain, candidate.address, klineTokenAddress, barParams, limit, before)
		if err == nil && len(candles) > 0 {
			return candles, candidate.address, klineTokenAddress, nil
		}
		if err == nil {
			err = fmt.Errorf("geckoterminal returned empty candles for pool %s", candidate.address)
		}
		lastErr = err
		if !shouldTryNextGeckoPool(err) && !isEmptyGeckoCandlesError(err) {
			return nil, candidate.address, klineTokenAddress, err
		}
	}
	if lastErr != nil {
		return nil, "", klineTokenAddress, lastErr
	}
	return nil, "", klineTokenAddress, fmt.Errorf("geckoterminal did not return candles for token %s", klineTokenAddress)
}

func isEmptyGeckoCandlesError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "returned empty candles")
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
		normalizedPoolAddress, ok := normalizeGeckoPoolAddress(poolAddress)
		if !ok {
			http.Error(w, "invalid pool_address", http.StatusBadRequest)
			return
		}
		poolAddress = normalizedPoolAddress
	}

	candles, usedPoolAddress, usedTokenAddress, err := fetchGeckoTokenCandles(ctx, nil, chain, tokenAddress, poolAddress, barParams, limit, before)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, tokenCandlesEnvelope{
		Chain:        chain,
		TokenAddress: usedTokenAddress,
		PoolAddress:  usedPoolAddress,
		Bar:          bar,
		Limit:        limit,
		Candles:      candles,
		Source:       "geckoterminal",
		UpdatedAt:    timeutil.Now(),
	})
}
