package web_server

import (
	"TgLpBot/service/exchange"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const smOKXDeFiCacheTTL = 90 * time.Second

var smOKXDeFiCache = struct {
	sync.Mutex
	overview map[string]smOKXDeFiOverviewCacheEntry
	detail   map[string]smOKXDeFiDetailCacheEntry
}{
	overview: make(map[string]smOKXDeFiOverviewCacheEntry),
	detail:   make(map[string]smOKXDeFiDetailCacheEntry),
}

type smOKXDeFiOverviewCacheEntry struct {
	expiresAt time.Time
	payload   smOKXDeFiOverviewResponse
}

type smOKXDeFiDetailCacheEntry struct {
	expiresAt time.Time
	payload   smOKXDeFiDetailResponse
}

type smOKXDeFiOverviewResponse struct {
	Source        string                     `json:"source"`
	Status        string                     `json:"status"`
	WalletAddress string                     `json:"wallet_address"`
	ChainIndexes  []string                   `json:"chain_indexes"`
	ChainNames    []string                   `json:"chain_names"`
	TotalValue    string                     `json:"total_value,omitempty"`
	TotalValueUSD *float64                   `json:"total_value_usd,omitempty"`
	Chains        []smOKXDeFiChainSummary    `json:"chains"`
	Platforms     []smOKXDeFiPlatformSummary `json:"platforms"`
	UpdatedAt     string                     `json:"updated_at"`
	CacheHit      bool                       `json:"cache_hit"`
	Warnings      []string                   `json:"warnings,omitempty"`
}

type smOKXDeFiDetailResponse struct {
	Source             string                   `json:"source"`
	Status             string                   `json:"status"`
	WalletAddress      string                   `json:"wallet_address"`
	AnalysisPlatformID string                   `json:"analysis_platform_id"`
	ChainIndex         string                   `json:"chain_index,omitempty"`
	ChainName          string                   `json:"chain_name,omitempty"`
	ChainIndexes       []string                 `json:"chain_indexes"`
	Platform           smOKXDeFiPlatformSummary `json:"platform"`
	Totals             smOKXDeFiDetailTotals    `json:"totals"`
	Investments        []smOKXDeFiInvestment    `json:"investments"`
	Positions          []smOKXDeFiPosition      `json:"positions"`
	UpdatedAt          string                   `json:"updated_at"`
	CacheHit           bool                     `json:"cache_hit"`
	Warnings           []string                 `json:"warnings,omitempty"`
}

type smOKXDeFiDetailTotals struct {
	TotalValue    string   `json:"total_value,omitempty"`
	TotalValueUSD *float64 `json:"total_value_usd,omitempty"`
	Fee           string   `json:"fee,omitempty"`
	FeeUSD        *float64 `json:"fee_usd,omitempty"`
}

type smOKXDeFiChainSummary struct {
	ChainIndex    string   `json:"chain_index"`
	ChainName     string   `json:"chain_name"`
	TotalValue    string   `json:"total_value,omitempty"`
	TotalValueUSD *float64 `json:"total_value_usd,omitempty"`
	TokenCount    int      `json:"token_count,omitempty"`
	PlatformCount int      `json:"platform_count,omitempty"`
}

type smOKXDeFiPlatformSummary struct {
	AnalysisPlatformID string                  `json:"analysis_platform_id"`
	PlatformName       string                  `json:"platform_name"`
	PlatformLogoURL    string                  `json:"platform_logo_url,omitempty"`
	ChainIndex         string                  `json:"chain_index,omitempty"`
	ChainName          string                  `json:"chain_name,omitempty"`
	TotalValue         string                  `json:"total_value,omitempty"`
	TotalValueUSD      *float64                `json:"total_value_usd,omitempty"`
	PositionAmount     string                  `json:"position_amount,omitempty"`
	PositionAmountUSD  *float64                `json:"position_amount_usd,omitempty"`
	Fee                string                  `json:"fee,omitempty"`
	FeeUSD             *float64                `json:"fee_usd,omitempty"`
	RangeText          string                  `json:"range_text,omitempty"`
	NetworkBalances    []smOKXDeFiChainSummary `json:"network_balances,omitempty"`
	HoldingCount       int                     `json:"holding_count,omitempty"`
}

type smOKXDeFiInvestment struct {
	InvestmentID      string              `json:"investment_id,omitempty"`
	Name              string              `json:"name"`
	Type              string              `json:"type,omitempty"`
	ChainIndex        string              `json:"chain_index,omitempty"`
	ChainName         string              `json:"chain_name,omitempty"`
	PositionAmount    string              `json:"position_amount,omitempty"`
	PositionAmountUSD *float64            `json:"position_amount_usd,omitempty"`
	Fee               string              `json:"fee,omitempty"`
	FeeUSD            *float64            `json:"fee_usd,omitempty"`
	RangeText         string              `json:"range_text,omitempty"`
	Tokens            []smOKXDeFiToken    `json:"tokens,omitempty"`
	Positions         []smOKXDeFiPosition `json:"positions,omitempty"`
}

type smOKXDeFiPosition struct {
	PositionID        string           `json:"position_id,omitempty"`
	Name              string           `json:"name"`
	PoolName          string           `json:"pool_name,omitempty"`
	ChainIndex        string           `json:"chain_index,omitempty"`
	ChainName         string           `json:"chain_name,omitempty"`
	PositionAmount    string           `json:"position_amount,omitempty"`
	PositionAmountUSD *float64         `json:"position_amount_usd,omitempty"`
	Fee               string           `json:"fee,omitempty"`
	FeeUSD            *float64         `json:"fee_usd,omitempty"`
	RangeText         string           `json:"range_text,omitempty"`
	TickLower         string           `json:"tick_lower,omitempty"`
	TickUpper         string           `json:"tick_upper,omitempty"`
	PriceLower        string           `json:"price_lower,omitempty"`
	PriceUpper        string           `json:"price_upper,omitempty"`
	Token0Symbol      string           `json:"token0_symbol,omitempty"`
	Token1Symbol      string           `json:"token1_symbol,omitempty"`
	Tokens            []smOKXDeFiToken `json:"tokens,omitempty"`
}

type smOKXDeFiToken struct {
	Symbol       string   `json:"symbol,omitempty"`
	TokenAddress string   `json:"token_address,omitempty"`
	Amount       string   `json:"amount,omitempty"`
	Value        string   `json:"value,omitempty"`
	ValueUSD     *float64 `json:"value_usd,omitempty"`
}

func (s *Server) handleSMDeFiOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	walletAddress := okxDeFiNormalizeWalletAddress(r.URL.Query().Get("address"))
	if walletAddress == "" {
		walletAddress = okxDeFiNormalizeWalletAddress(r.URL.Query().Get("wallet"))
	}
	if walletAddress == "" {
		walletAddress = okxDeFiNormalizeWalletAddress(r.URL.Query().Get("wallet_address"))
	}
	if walletAddress == "" {
		jsonError(w, "valid wallet address is required", http.StatusBadRequest)
		return
	}

	chainIndexes := okxDeFiRequestedChainIndexes(r)
	cacheKey := okxDeFiCacheKey("overview", walletAddress, "", chainIndexes)
	if payload, ok := okxDeFiGetOverviewCache(cacheKey); ok {
		payload.CacheHit = true
		jsonOK(w, payload)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	resp, err := exchange.NewOKXDexService().GetDeFiUserAssetPlatformList(ctx, exchange.DeFiUserAssetPlatformListRequest{
		WalletAddressList: okxDeFiWalletAddressRequests(walletAddress, chainIndexes),
	})
	if err != nil {
		jsonError(w, "OKX DeFi overview failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	payload, err := okxDeFiNormalizeOverview(walletAddress, chainIndexes, resp.Data, time.Now().UTC())
	if err != nil {
		jsonError(w, "OKX DeFi overview parse failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	okxDeFiSetOverviewCache(cacheKey, payload)
	jsonOK(w, payload)
}

func (s *Server) handleSMDeFiDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	walletAddress := okxDeFiNormalizeWalletAddress(q.Get("address"))
	if walletAddress == "" {
		walletAddress = okxDeFiNormalizeWalletAddress(q.Get("wallet"))
	}
	if walletAddress == "" {
		walletAddress = okxDeFiNormalizeWalletAddress(q.Get("wallet_address"))
	}
	if walletAddress == "" {
		jsonError(w, "valid wallet address is required", http.StatusBadRequest)
		return
	}

	platformID := strings.TrimSpace(q.Get("analysis_platform_id"))
	if platformID == "" {
		platformID = strings.TrimSpace(q.Get("analysisPlatformId"))
	}
	if platformID == "" {
		jsonError(w, "analysis_platform_id is required", http.StatusBadRequest)
		return
	}

	chainIndexes := okxDeFiRequestedChainIndexes(r)
	requestedChainIndex := okxDeFiFirstRequestedChainIndex(q.Get("chain_index"), q.Get("chainIndex"), q.Get("chain_id"), q.Get("chainId"))
	if requestedChainIndex != "" {
		chainIndexes = []string{requestedChainIndex}
	}

	cacheKey := okxDeFiCacheKey("detail", walletAddress, platformID+"|"+requestedChainIndex, chainIndexes)
	if payload, ok := okxDeFiGetDetailCache(cacheKey); ok {
		payload.CacheHit = true
		jsonOK(w, payload)
		return
	}

	platformReq := exchange.DeFiPlatformRequest{AnalysisPlatformID: platformID}
	if requestedChainIndex != "" {
		platformReq.ChainIndex = requestedChainIndex
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	resp, err := exchange.NewOKXDexService().GetDeFiUserAssetPlatformDetail(ctx, exchange.DeFiUserAssetPlatformDetailRequest{
		WalletAddressList: okxDeFiWalletAddressRequests(walletAddress, chainIndexes),
		PlatformList:      []exchange.DeFiPlatformRequest{platformReq},
	})
	if err != nil {
		jsonError(w, "OKX DeFi detail failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	payload, err := okxDeFiNormalizeDetail(walletAddress, platformID, requestedChainIndex, chainIndexes, resp.Data, time.Now().UTC())
	if err != nil {
		jsonError(w, "OKX DeFi detail parse failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	okxDeFiSetDetailCache(cacheKey, payload)
	jsonOK(w, payload)
}

func okxDeFiWalletAddressRequests(walletAddress string, chainIndexes []string) []exchange.DeFiWalletAddressRequest {
	out := make([]exchange.DeFiWalletAddressRequest, 0, len(chainIndexes))
	for _, chainIndex := range chainIndexes {
		out = append(out, exchange.DeFiWalletAddressRequest{
			ChainIndex:    chainIndex,
			WalletAddress: walletAddress,
		})
	}
	return out
}

func okxDeFiNormalizeWalletAddress(value string) string {
	addr := strings.TrimSpace(value)
	if len(addr) != 42 {
		return ""
	}
	if !strings.HasPrefix(addr, "0x") && !strings.HasPrefix(addr, "0X") {
		return ""
	}
	for _, c := range addr[2:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return ""
		}
	}
	return "0x" + strings.ToLower(addr[2:])
}

func okxDeFiRequestedChainIndexes(r *http.Request) []string {
	if r == nil {
		return okxDeFiDefaultChainIndexes()
	}
	q := r.URL.Query()
	values := make([]string, 0, 8)
	for _, key := range []string{"chain_index", "chainIndex", "chain_id", "chainId"} {
		values = append(values, q[key]...)
	}
	if rawChain := strings.TrimSpace(q.Get("chain")); rawChain != "" && rawChain != "all" {
		values = append(values, rawChain)
	}
	out := okxDeFiNormalizeChainIndexes(values)
	if len(out) == 0 {
		return okxDeFiDefaultChainIndexes()
	}
	return out
}

func okxDeFiDefaultChainIndexes() []string {
	return []string{"56", "8453", "1", "42161", "10", "137", "43114"}
}

func okxDeFiNormalizeChainIndexes(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			chainIndex := okxDeFiChainIndex(part)
			if chainIndex == "" {
				continue
			}
			if _, ok := seen[chainIndex]; ok {
				continue
			}
			seen[chainIndex] = struct{}{}
			out = append(out, chainIndex)
		}
	}
	return out
}

func okxDeFiFirstRequestedChainIndex(values ...string) string {
	indexes := okxDeFiNormalizeChainIndexes(values)
	if len(indexes) == 0 {
		return ""
	}
	return indexes[0]
}

func okxDeFiChainIndex(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	switch value {
	case "bsc", "bnb", "bnbchain", "bnb_chain", "binance-smart-chain":
		return "56"
	case "base":
		return "8453"
	case "eth", "ethereum":
		return "1"
	case "arb", "arbitrum", "arbitrum-one":
		return "42161"
	case "op", "optimism":
		return "10"
	case "polygon", "matic":
		return "137"
	case "avax", "avalanche":
		return "43114"
	default:
		if _, err := strconv.ParseInt(value, 10, 64); err == nil {
			return value
		}
		return ""
	}
}

func okxDeFiChainName(chainIndex string) string {
	switch strings.TrimSpace(chainIndex) {
	case "56":
		return "BSC"
	case "8453":
		return "Base"
	case "1":
		return "Ethereum"
	case "42161":
		return "Arbitrum"
	case "10":
		return "Optimism"
	case "137":
		return "Polygon"
	case "43114":
		return "Avalanche"
	default:
		if strings.TrimSpace(chainIndex) == "" {
			return ""
		}
		return "Chain " + strings.TrimSpace(chainIndex)
	}
}

func okxDeFiCacheKey(kind string, walletAddress string, suffix string, chainIndexes []string) string {
	copied := append([]string(nil), chainIndexes...)
	sort.Strings(copied)
	return kind + "|" + strings.ToLower(strings.TrimSpace(walletAddress)) + "|" + strings.Join(copied, ",") + "|" + strings.TrimSpace(suffix)
}

func okxDeFiGetOverviewCache(key string) (smOKXDeFiOverviewResponse, bool) {
	now := time.Now()
	smOKXDeFiCache.Lock()
	defer smOKXDeFiCache.Unlock()
	entry, ok := smOKXDeFiCache.overview[key]
	if !ok {
		return smOKXDeFiOverviewResponse{}, false
	}
	if now.After(entry.expiresAt) {
		delete(smOKXDeFiCache.overview, key)
		return smOKXDeFiOverviewResponse{}, false
	}
	return entry.payload, true
}

func okxDeFiSetOverviewCache(key string, payload smOKXDeFiOverviewResponse) {
	payload.CacheHit = false
	smOKXDeFiCache.Lock()
	smOKXDeFiCache.overview[key] = smOKXDeFiOverviewCacheEntry{
		expiresAt: time.Now().Add(smOKXDeFiCacheTTL),
		payload:   payload,
	}
	smOKXDeFiCache.Unlock()
}

func okxDeFiGetDetailCache(key string) (smOKXDeFiDetailResponse, bool) {
	now := time.Now()
	smOKXDeFiCache.Lock()
	defer smOKXDeFiCache.Unlock()
	entry, ok := smOKXDeFiCache.detail[key]
	if !ok {
		return smOKXDeFiDetailResponse{}, false
	}
	if now.After(entry.expiresAt) {
		delete(smOKXDeFiCache.detail, key)
		return smOKXDeFiDetailResponse{}, false
	}
	return entry.payload, true
}

func okxDeFiSetDetailCache(key string, payload smOKXDeFiDetailResponse) {
	payload.CacheHit = false
	smOKXDeFiCache.Lock()
	smOKXDeFiCache.detail[key] = smOKXDeFiDetailCacheEntry{
		expiresAt: time.Now().Add(smOKXDeFiCacheTTL),
		payload:   payload,
	}
	smOKXDeFiCache.Unlock()
}

func okxDeFiNormalizeOverview(walletAddress string, chainIndexes []string, raw json.RawMessage, updatedAt time.Time) (smOKXDeFiOverviewResponse, error) {
	root, err := okxDeFiDecodeRaw(raw)
	if err != nil {
		return smOKXDeFiOverviewResponse{}, err
	}
	dataMaps := okxDeFiMapsFromValue(root)
	platformMaps := okxDeFiCollectMapsByKeys(root, okxDeFiPlatformArrayKeys())
	if len(platformMaps) == 0 {
		platformMaps = okxDeFiCollectLooksLikePlatform(root)
	}
	chainMaps := okxDeFiCollectMapsByKeys(root, okxDeFiChainArrayKeys())
	walletPlatformMaps := okxDeFiCollectMapsByKeys(root, okxDeFiWalletPlatformArrayKeys())

	totalValue, totalValueUSD := okxDeFiFirstAmount(dataMaps, okxDeFiTotalValueKeys()...)
	if totalValue == "" && len(walletPlatformMaps) > 0 {
		totalValue, totalValueUSD = okxDeFiFirstAmount(walletPlatformMaps, okxDeFiTotalValueKeys()...)
	}
	chains := okxDeFiNormalizeChains(chainMaps)
	platforms := okxDeFiNormalizePlatforms(platformMaps)
	warnings := make([]string, 0, 2)
	if len(platforms) == 0 {
		warnings = append(warnings, "OKX DeFi platform list returned no platform data")
	}
	if totalValue != "" && totalValueUSD == nil {
		warnings = append(warnings, "OKX DeFi total value is not numeric")
	}

	return smOKXDeFiOverviewResponse{
		Source:        "okx_defi",
		Status:        "ok",
		WalletAddress: walletAddress,
		ChainIndexes:  append([]string(nil), chainIndexes...),
		ChainNames:    okxDeFiChainNames(chainIndexes),
		TotalValue:    totalValue,
		TotalValueUSD: totalValueUSD,
		Chains:        chains,
		Platforms:     platforms,
		UpdatedAt:     updatedAt.Format(time.RFC3339),
		Warnings:      warnings,
	}, nil
}

func okxDeFiNormalizeDetail(walletAddress string, platformID string, requestedChainIndex string, chainIndexes []string, raw json.RawMessage, updatedAt time.Time) (smOKXDeFiDetailResponse, error) {
	root, err := okxDeFiDecodeRaw(raw)
	if err != nil {
		return smOKXDeFiDetailResponse{}, err
	}
	platformMaps := okxDeFiCollectMapsByKeys(root, okxDeFiPlatformArrayKeys())
	if len(platformMaps) == 0 {
		platformMaps = okxDeFiCollectLooksLikePlatform(root)
	}
	if len(platformMaps) == 0 {
		dataMaps := okxDeFiMapsFromValue(root)
		platformMaps = append(platformMaps, dataMaps...)
	}

	selectedPlatform := okxDeFiSelectPlatformMap(platformMaps, platformID, requestedChainIndex)
	platform := okxDeFiPlatformSummary(selectedPlatform)
	if platform.AnalysisPlatformID == "" {
		platform.AnalysisPlatformID = platformID
	}
	if requestedChainIndex != "" && platform.ChainIndex == "" {
		platform.ChainIndex = requestedChainIndex
		platform.ChainName = okxDeFiChainName(requestedChainIndex)
	}

	positionMaps := okxDeFiCollectMapsByKeysWithContext(selectedPlatform, okxDeFiPositionArrayKeys())
	if len(positionMaps) == 0 {
		positionMaps = okxDeFiCollectMapsByKeysWithContext(root, okxDeFiPositionArrayKeys())
	}
	if len(positionMaps) == 0 && okxDeFiLooksLikePosition(selectedPlatform) {
		positionMaps = append(positionMaps, selectedPlatform)
	}

	investmentMaps := okxDeFiCollectMapsByKeysWithContext(selectedPlatform, okxDeFiInvestmentArrayKeys())
	if len(investmentMaps) == 0 {
		investmentMaps = okxDeFiCollectMapsByKeysWithContext(root, okxDeFiInvestmentArrayKeys())
	}

	positions := okxDeFiNormalizePositions(positionMaps)
	investments := okxDeFiNormalizeInvestments(investmentMaps)
	totalValue := platform.TotalValue
	totalValueUSD := platform.TotalValueUSD
	if totalValue == "" || totalValueUSD == nil {
		totalValue, totalValueUSD = okxDeFiFirstAmount(okxDeFiMapsFromValue(root), okxDeFiTotalValueKeys()...)
	}
	feeValue, feeUSD := okxDeFiFeeAmount(selectedPlatform)

	warnings := make([]string, 0, 2)
	if len(positions) == 0 && len(investments) == 0 {
		warnings = append(warnings, "OKX DeFi detail returned no investment or position rows")
	}
	if totalValue != "" && totalValueUSD == nil {
		warnings = append(warnings, "OKX DeFi detail total value is not numeric")
	}

	return smOKXDeFiDetailResponse{
		Source:             "okx_defi",
		Status:             "ok",
		WalletAddress:      walletAddress,
		AnalysisPlatformID: platformID,
		ChainIndex:         platform.ChainIndex,
		ChainName:          platform.ChainName,
		ChainIndexes:       append([]string(nil), chainIndexes...),
		Platform:           platform,
		Totals: smOKXDeFiDetailTotals{
			TotalValue:    totalValue,
			TotalValueUSD: totalValueUSD,
			Fee:           feeValue,
			FeeUSD:        feeUSD,
		},
		Investments: investments,
		Positions:   positions,
		UpdatedAt:   updatedAt.Format(time.RFC3339),
		Warnings:    warnings,
	}, nil
}

func okxDeFiDecodeRaw(raw json.RawMessage) (interface{}, error) {
	data := bytes.TrimSpace(raw)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return []interface{}{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var out interface{}
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func okxDeFiMapsFromValue(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	case map[string]interface{}:
		return []map[string]interface{}{typed}
	default:
		return nil
	}
}

func okxDeFiCollectMapsByKeys(value interface{}, keys map[string]struct{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0)
	var walk func(interface{})
	walk = func(current interface{}) {
		switch typed := current.(type) {
		case map[string]interface{}:
			for key, value := range typed {
				if _, ok := keys[strings.ToLower(strings.TrimSpace(key))]; ok {
					out = append(out, okxDeFiMapsFromValue(value)...)
					continue
				}
				switch value.(type) {
				case map[string]interface{}, []interface{}:
					walk(value)
				}
			}
		case []interface{}:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	return out
}

func okxDeFiCollectMapsByKeysWithContext(value interface{}, keys map[string]struct{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0)
	var walk func(interface{}, string)
	walk = func(current interface{}, inheritedChainIndex string) {
		switch typed := current.(type) {
		case map[string]interface{}:
			chainIndex := okxDeFiString(typed, "chainIndex", "chainId", "networkChainId")
			if chainIndex == "" {
				chainIndex = inheritedChainIndex
			}
			for key, value := range typed {
				if _, ok := keys[strings.ToLower(strings.TrimSpace(key))]; ok {
					for _, item := range okxDeFiMapsFromValue(value) {
						if chainIndex != "" && okxDeFiString(item, "chainIndex", "chainId", "networkChainId") == "" {
							item = okxDeFiCloneMap(item)
							item["chainIndex"] = chainIndex
						}
						out = append(out, item)
					}
					continue
				}
				switch value.(type) {
				case map[string]interface{}, []interface{}:
					walk(value, chainIndex)
				}
			}
		case []interface{}:
			for _, item := range typed {
				walk(item, inheritedChainIndex)
			}
		}
	}
	walk(value, "")
	return out
}

func okxDeFiCloneMap(item map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(item)+1)
	for key, value := range item {
		out[key] = value
	}
	return out
}

func okxDeFiCollectLooksLikePlatform(value interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0)
	var walk func(interface{})
	walk = func(current interface{}) {
		switch typed := current.(type) {
		case map[string]interface{}:
			if okxDeFiLooksLikePlatform(typed) {
				out = append(out, typed)
			}
			for _, value := range typed {
				switch value.(type) {
				case map[string]interface{}, []interface{}:
					walk(value)
				}
			}
		case []interface{}:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	return out
}

func okxDeFiLooksLikePlatform(m map[string]interface{}) bool {
	return okxDeFiString(m, "analysisPlatformId", "platformId", "platformName", "platformLogoUrl", "platformLogo") != "" ||
		len(okxDeFiMapsFromValue(okxDeFiAny(m, "platformList"))) > 0
}

func okxDeFiLooksLikePosition(m map[string]interface{}) bool {
	return okxDeFiString(m, "positionId", "positionName", "positionValue", "positionValueUsd", "currencyAmount", "liquidityPoolToken", "tickLower", "tickUpper", "lowerPrice", "upperPrice") != ""
}

func okxDeFiPlatformArrayKeys() map[string]struct{} {
	return okxDeFiKeySet("platformList", "assetPlatformList", "platformAssetList", "platformAssets", "platformInfoList")
}

func okxDeFiWalletPlatformArrayKeys() map[string]struct{} {
	return okxDeFiKeySet("walletIdPlatformList", "walletIdPlatformDetailList")
}

func okxDeFiChainArrayKeys() map[string]struct{} {
	return okxDeFiKeySet("networkBalanceVoList", "networkBalanceList", "networkList", "chainList", "chainBalanceList", "networkHoldVoList")
}

func okxDeFiPositionArrayKeys() map[string]struct{} {
	return okxDeFiKeySet("positionList", "positions", "lpPositionList", "poolPositionList", "holdingList", "holdList", "poolTokenPositionList", "investmentTokenPositionList")
}

func okxDeFiInvestmentArrayKeys() map[string]struct{} {
	return okxDeFiKeySet("investmentList", "investments", "investmentTokenList", "investTokenBalanceVoList", "productList", "assetList")
}

func okxDeFiTokenArrayKeys() map[string]struct{} {
	return okxDeFiKeySet("tokenList", "tokens", "tokenAssets", "assetTokenList", "underlyingTokenList", "underlyingTokens", "defiTokenInfo", "defiTokenInfoList", "unclaimFeesDefiTokenInfo")
}

func okxDeFiFeeTokenArrayKeys() map[string]struct{} {
	return okxDeFiKeySet("unclaimFeesDefiTokenInfo", "claimableFeeDefiTokenInfo", "feeTokenList", "unclaimedFeeTokenList", "rewardTokenList")
}

func okxDeFiKeySet(keys ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		out[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
	}
	return out
}

func okxDeFiSelectPlatformMap(platforms []map[string]interface{}, platformID string, chainIndex string) map[string]interface{} {
	for _, platform := range platforms {
		id := okxDeFiString(platform, "analysisPlatformId", "platformId", "platformID", "id")
		if id != platformID {
			continue
		}
		if chainIndex == "" || okxDeFiString(platform, "chainIndex", "chainId") == chainIndex {
			return platform
		}
	}
	for _, platform := range platforms {
		if okxDeFiString(platform, "analysisPlatformId", "platformId", "platformID", "id") == platformID {
			return platform
		}
	}
	if len(platforms) > 0 {
		return platforms[0]
	}
	return map[string]interface{}{}
}

func okxDeFiNormalizePlatforms(platformMaps []map[string]interface{}) []smOKXDeFiPlatformSummary {
	out := make([]smOKXDeFiPlatformSummary, 0, len(platformMaps))
	seen := make(map[string]struct{}, len(platformMaps))
	for _, item := range platformMaps {
		summary := okxDeFiPlatformSummary(item)
		if summary.AnalysisPlatformID == "" && summary.PlatformName == "" {
			continue
		}
		key := strings.ToLower(summary.AnalysisPlatformID + "|" + summary.PlatformName + "|" + summary.ChainIndex)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, summary)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := okxDeFiDerefFloat(out[i].TotalValueUSD)
		right := okxDeFiDerefFloat(out[j].TotalValueUSD)
		return left > right
	})
	return out
}

func okxDeFiPlatformSummary(item map[string]interface{}) smOKXDeFiPlatformSummary {
	chainIndex := okxDeFiString(item, "chainIndex", "chainId")
	networks := okxDeFiNormalizeChains(okxDeFiCollectMapsByKeys(item, okxDeFiChainArrayKeys()))
	if chainIndex == "" && len(networks) == 1 {
		chainIndex = networks[0].ChainIndex
	}
	totalValue, totalValueUSD := okxDeFiAmount(item, okxDeFiTotalValueKeys()...)
	if totalValue == "" && len(networks) == 1 {
		totalValue = networks[0].TotalValue
		totalValueUSD = networks[0].TotalValueUSD
	}
	positionValue, positionValueUSD := okxDeFiAmount(item, okxDeFiPositionValueKeys()...)
	if positionValue == "" && totalValue != "" {
		positionValue = totalValue
		positionValueUSD = totalValueUSD
	}
	feeValue, feeUSD := okxDeFiFeeAmount(item)
	holdingCount := len(okxDeFiCollectMapsByKeys(item, okxDeFiPositionArrayKeys()))
	if holdingCount == 0 {
		holdingCount = len(okxDeFiCollectMapsByKeys(item, okxDeFiInvestmentArrayKeys()))
	}
	return smOKXDeFiPlatformSummary{
		AnalysisPlatformID: okxDeFiString(item, "analysisPlatformId", "platformId", "platformID", "id"),
		PlatformName:       okxDeFiString(item, "platformName", "name", "projectName", "protocolName"),
		PlatformLogoURL:    okxDeFiString(item, "platformLogoUrl", "platformLogoURL", "platformLogo", "logoUrl", "logo"),
		ChainIndex:         chainIndex,
		ChainName:          okxDeFiChainName(chainIndex),
		TotalValue:         totalValue,
		TotalValueUSD:      totalValueUSD,
		PositionAmount:     positionValue,
		PositionAmountUSD:  positionValueUSD,
		Fee:                feeValue,
		FeeUSD:             feeUSD,
		RangeText:          okxDeFiRangeText(item),
		NetworkBalances:    networks,
		HoldingCount:       holdingCount,
	}
}

func okxDeFiNormalizeChains(chainMaps []map[string]interface{}) []smOKXDeFiChainSummary {
	out := make([]smOKXDeFiChainSummary, 0, len(chainMaps))
	seen := make(map[string]struct{}, len(chainMaps))
	for _, item := range chainMaps {
		chainIndex := okxDeFiString(item, "chainIndex", "chainId", "networkChainId")
		if chainIndex == "" {
			chainIndex = okxDeFiChainIndex(okxDeFiString(item, "chain", "network"))
		}
		if chainIndex == "" {
			continue
		}
		totalValue, totalValueUSD := okxDeFiAmount(item, okxDeFiTotalValueKeys()...)
		key := chainIndex + "|" + totalValue
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		name := okxDeFiString(item, "chainName", "networkName", "network")
		if name == "" {
			name = okxDeFiChainName(chainIndex)
		}
		out = append(out, smOKXDeFiChainSummary{
			ChainIndex:    chainIndex,
			ChainName:     name,
			TotalValue:    totalValue,
			TotalValueUSD: totalValueUSD,
			TokenCount:    okxDeFiInt(item, "tokenCount", "assetCount"),
			PlatformCount: okxDeFiInt(item, "platformCount"),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return okxDeFiDerefFloat(out[i].TotalValueUSD) > okxDeFiDerefFloat(out[j].TotalValueUSD)
	})
	return out
}

func okxDeFiNormalizeInvestments(investmentMaps []map[string]interface{}) []smOKXDeFiInvestment {
	out := make([]smOKXDeFiInvestment, 0, len(investmentMaps))
	seen := make(map[string]struct{}, len(investmentMaps))
	for _, item := range investmentMaps {
		name := okxDeFiString(item, "investmentName", "name", "productName", "poolName", "positionName", "liquidityPoolToken")
		investmentID := okxDeFiString(item, "investmentId", "id", "productId")
		if name == "" && investmentID == "" && !okxDeFiLooksLikePosition(item) {
			continue
		}
		chainIndex := okxDeFiString(item, "chainIndex", "chainId")
		positionValue, positionValueUSD := okxDeFiAmount(item, okxDeFiPositionValueKeys()...)
		feeValue, feeUSD := okxDeFiFeeAmount(item)
		positions := okxDeFiNormalizePositions(okxDeFiCollectMapsByKeys(item, okxDeFiPositionArrayKeys()))
		tokens := okxDeFiNormalizeTokens(okxDeFiCollectMapsByKeys(item, okxDeFiTokenArrayKeys()))
		key := strings.ToLower(investmentID + "|" + name + "|" + chainIndex)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, smOKXDeFiInvestment{
			InvestmentID:      investmentID,
			Name:              okxDeFiDisplayName(name, investmentID, "OKX DeFi investment"),
			Type:              okxDeFiString(item, "type", "investmentType", "productType"),
			ChainIndex:        chainIndex,
			ChainName:         okxDeFiChainName(chainIndex),
			PositionAmount:    positionValue,
			PositionAmountUSD: positionValueUSD,
			Fee:               feeValue,
			FeeUSD:            feeUSD,
			RangeText:         okxDeFiRangeText(item),
			Tokens:            tokens,
			Positions:         positions,
		})
	}
	return out
}

func okxDeFiNormalizePositions(positionMaps []map[string]interface{}) []smOKXDeFiPosition {
	out := make([]smOKXDeFiPosition, 0, len(positionMaps))
	seen := make(map[string]struct{}, len(positionMaps))
	for _, item := range positionMaps {
		name := okxDeFiString(item, "positionName", "name", "investmentName", "poolName", "productName", "liquidityPoolToken")
		positionID := okxDeFiString(item, "positionId", "id", "tokenId", "nftTokenId")
		if name == "" && positionID == "" && !okxDeFiLooksLikePosition(item) {
			continue
		}
		chainIndex := okxDeFiString(item, "chainIndex", "chainId")
		positionValue, positionValueUSD := okxDeFiAmount(item, okxDeFiPositionValueKeys()...)
		feeValue, feeUSD := okxDeFiFeeAmount(item)
		key := strings.ToLower(positionID + "|" + name + "|" + chainIndex + "|" + positionValue)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, smOKXDeFiPosition{
			PositionID:        positionID,
			Name:              okxDeFiDisplayName(name, positionID, "OKX DeFi position"),
			PoolName:          okxDeFiString(item, "poolName", "pairName", "liquidityPoolToken"),
			ChainIndex:        chainIndex,
			ChainName:         okxDeFiChainName(chainIndex),
			PositionAmount:    positionValue,
			PositionAmountUSD: positionValueUSD,
			Fee:               feeValue,
			FeeUSD:            feeUSD,
			RangeText:         okxDeFiRangeText(item),
			TickLower:         okxDeFiString(item, "tickLower", "lowerTick"),
			TickUpper:         okxDeFiString(item, "tickUpper", "upperTick"),
			PriceLower:        okxDeFiString(item, "priceLower", "lowerPrice", "minPrice"),
			PriceUpper:        okxDeFiString(item, "priceUpper", "upperPrice", "maxPrice"),
			Token0Symbol:      okxDeFiString(item, "token0Symbol", "tokenASymbol"),
			Token1Symbol:      okxDeFiString(item, "token1Symbol", "tokenBSymbol"),
			Tokens:            okxDeFiNormalizeTokens(okxDeFiCollectMapsByKeys(item, okxDeFiTokenArrayKeys())),
		})
	}
	return out
}

func okxDeFiNormalizeTokens(tokenMaps []map[string]interface{}) []smOKXDeFiToken {
	out := make([]smOKXDeFiToken, 0, len(tokenMaps))
	seen := make(map[string]struct{}, len(tokenMaps))
	for _, item := range tokenMaps {
		symbol := okxDeFiString(item, "symbol", "tokenSymbol", "coinSymbol")
		address := okxDeFiString(item, "tokenContractAddress", "tokenAddress", "contractAddress")
		amount := okxDeFiString(item, "amount", "balance", "tokenAmount")
		value, valueUSD := okxDeFiAmount(item, okxDeFiTotalValueKeys()...)
		if symbol == "" && address == "" && amount == "" && value == "" {
			continue
		}
		key := strings.ToLower(symbol + "|" + address + "|" + amount + "|" + value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, smOKXDeFiToken{
			Symbol:       symbol,
			TokenAddress: address,
			Amount:       amount,
			Value:        value,
			ValueUSD:     valueUSD,
		})
	}
	return out
}

func okxDeFiDisplayName(name string, id string, label string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	id = strings.TrimSpace(id)
	if id != "" {
		return label + " " + id
	}
	return label
}

func okxDeFiRangeText(item map[string]interface{}) string {
	if text := okxDeFiString(item, "range", "rangeText", "priceRange"); text != "" {
		return text
	}
	lower := okxDeFiString(item, "priceLower", "lowerPrice", "minPrice")
	upper := okxDeFiString(item, "priceUpper", "upperPrice", "maxPrice")
	if lower != "" || upper != "" {
		return strings.TrimSpace(lower + " - " + upper)
	}
	tickLower := okxDeFiString(item, "tickLower", "lowerTick")
	tickUpper := okxDeFiString(item, "tickUpper", "upperTick")
	if tickLower != "" || tickUpper != "" {
		return strings.TrimSpace("tick " + tickLower + " - " + tickUpper)
	}
	return ""
}

func okxDeFiTotalValueKeys() []string {
	return []string{"totalValue", "totalValueUsd", "totalValueUSD", "totalAssets", "assetValue", "assetValueUsd", "currencyAmount", "value", "valueUsd", "usdValue", "amountUsd"}
}

func okxDeFiPositionValueKeys() []string {
	return []string{"positionValue", "positionValueUsd", "positionAmount", "positionAmountUsd", "liquidityValue", "liquidityValueUsd", "currencyAmount", "amountUsd", "totalValue", "value"}
}

func okxDeFiFeeValueKeys() []string {
	return []string{"fee", "feeValue", "feeUsd", "feeUSD", "unclaimedFee", "unclaimedFeeUsd", "unclaimedFeeUSD", "claimableFee", "claimableFeeUsd", "rewardValue", "rewardUsd", "totalFee", "totalFeeUsd", "unclaimFee", "unclaimFeeUsd"}
}

func okxDeFiFirstAmount(items []map[string]interface{}, keys ...string) (string, *float64) {
	for _, item := range items {
		raw, value := okxDeFiAmount(item, keys...)
		if raw != "" {
			return raw, value
		}
	}
	return "", nil
}

func okxDeFiAmount(item map[string]interface{}, keys ...string) (string, *float64) {
	raw := okxDeFiString(item, keys...)
	return raw, okxDeFiFloat(raw)
}

func okxDeFiFeeAmount(item map[string]interface{}) (string, *float64) {
	raw, value := okxDeFiAmount(item, okxDeFiFeeValueKeys()...)
	if raw != "" {
		return raw, value
	}
	feeTokens := okxDeFiCollectMapsByKeys(item, okxDeFiFeeTokenArrayKeys())
	if len(feeTokens) == 0 {
		return "", nil
	}
	return okxDeFiSumAmounts(feeTokens, okxDeFiTotalValueKeys()...)
}

func okxDeFiSumAmounts(items []map[string]interface{}, keys ...string) (string, *float64) {
	var total float64
	seenNumeric := false
	for _, item := range items {
		_, value := okxDeFiAmount(item, keys...)
		if value == nil {
			continue
		}
		total += *value
		seenNumeric = true
	}
	if !seenNumeric {
		return "", nil
	}
	return strconv.FormatFloat(total, 'f', -1, 64), &total
}

func okxDeFiFloat(raw string) *float64 {
	value := strings.TrimSpace(strings.ReplaceAll(raw, ",", ""))
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return nil
	}
	return &parsed
}

func okxDeFiDerefFloat(value *float64) float64 {
	if value == nil {
		return math.Inf(-1)
	}
	return *value
}

func okxDeFiInt(item map[string]interface{}, keys ...string) int {
	raw := okxDeFiString(item, keys...)
	if raw == "" {
		return 0
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	return parsed
}

func okxDeFiString(item map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value, ok := okxDeFiLookup(item, key)
		if !ok {
			continue
		}
		text := okxDeFiValueString(value)
		if text != "" {
			return text
		}
	}
	return ""
}

func okxDeFiAny(item map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		value, ok := okxDeFiLookup(item, key)
		if ok {
			return value
		}
	}
	return nil
}

func okxDeFiLookup(item map[string]interface{}, key string) (interface{}, bool) {
	if item == nil {
		return nil, false
	}
	if value, ok := item[key]; ok {
		return value, true
	}
	target := strings.ToLower(strings.TrimSpace(key))
	for itemKey, value := range item {
		if strings.ToLower(strings.TrimSpace(itemKey)) == target {
			return value, true
		}
	}
	return nil, false
}

func okxDeFiValueString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func okxDeFiChainNames(chainIndexes []string) []string {
	out := make([]string, 0, len(chainIndexes))
	for _, chainIndex := range chainIndexes {
		name := okxDeFiChainName(chainIndex)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}
