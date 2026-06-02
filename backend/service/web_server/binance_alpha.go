package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	binanceAlphaTokenListURL     = "https://www.binance.com/bapi/defi/v1/public/wallet-direct/buw/wallet/cex/alpha/all/token/list"
	binanceAlphaTokenListTTL     = 30 * time.Minute
	binanceAlphaTokenListTimeout = 3 * time.Second
	binanceAlphaBadgeLabel       = "币安 Alpha"
)

type binanceAlphaTokenInfo struct {
	ChainID         string
	ChainName       string
	ContractAddress string
	Name            string
	Symbol          string
	AlphaID         string
}

type binanceAlphaCacheSnapshot struct {
	tokens    map[string]binanceAlphaTokenInfo
	expiresAt time.Time
}

var (
	binanceAlphaCacheMu sync.Mutex
	binanceAlphaCache   binanceAlphaCacheSnapshot
)

type binanceAlphaTokenListResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    []struct {
		ChainID         string `json:"chainId"`
		ChainName       string `json:"chainName"`
		ContractAddress string `json:"contractAddress"`
		Name            string `json:"name"`
		Symbol          string `json:"symbol"`
		AlphaID         string `json:"alphaId"`
	} `json:"data"`
}

func normalizeBinanceAlphaChainID(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "56", "bsc", "bnb", "bnb smart chain", "binance smart chain", "binance smartchain":
		return "56"
	case "8453", "base":
		return "8453"
	default:
		return ""
	}
}

func normalizeBinanceAlphaListChainID(chainID string, chainName string) string {
	if normalized := normalizeBinanceAlphaChainID(chainID); normalized != "" {
		return normalized
	}
	return normalizeBinanceAlphaChainID(chainName)
}

func binanceAlphaTokenKey(chain string, address string) string {
	chainID := normalizeBinanceAlphaChainID(chain)
	addr := normalizeCatalogHex(address)
	if chainID == "" || !isBinanceAlphaEVMAddress(addr) {
		return ""
	}
	return chainID + ":" + addr
}

func isBinanceAlphaEVMAddress(address string) bool {
	return strings.HasPrefix(address, "0x") && len(address) == 42
}

func loadBinanceAlphaTokenIndex(ctx context.Context) (map[string]binanceAlphaTokenInfo, error) {
	now := time.Now()
	binanceAlphaCacheMu.Lock()
	defer binanceAlphaCacheMu.Unlock()

	if binanceAlphaCache.tokens != nil && binanceAlphaCache.expiresAt.After(now) {
		return binanceAlphaCache.tokens, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, binanceAlphaTokenListTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, binanceAlphaTokenListURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "TgLpBot/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance alpha token list http %d", resp.StatusCode)
	}

	var parsed binanceAlphaTokenListResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Code != "" && parsed.Code != "000000" {
		return nil, fmt.Errorf("binance alpha token list code=%s message=%s", parsed.Code, strings.TrimSpace(parsed.Message))
	}

	index := make(map[string]binanceAlphaTokenInfo, len(parsed.Data))
	for _, item := range parsed.Data {
		chainID := normalizeBinanceAlphaListChainID(item.ChainID, item.ChainName)
		addr := normalizeCatalogHex(item.ContractAddress)
		if chainID == "" || !isBinanceAlphaEVMAddress(addr) {
			continue
		}
		index[chainID+":"+addr] = binanceAlphaTokenInfo{
			ChainID:         chainID,
			ChainName:       strings.TrimSpace(item.ChainName),
			ContractAddress: addr,
			Name:            strings.TrimSpace(item.Name),
			Symbol:          strings.TrimSpace(item.Symbol),
			AlphaID:         strings.TrimSpace(item.AlphaID),
		}
	}

	binanceAlphaCache = binanceAlphaCacheSnapshot{
		tokens:    index,
		expiresAt: now.Add(binanceAlphaTokenListTTL),
	}
	return index, nil
}

func (s *Server) enrichHotPoolBinanceAlpha(ctx context.Context, chain string, items []HotPoolResponse) {
	if len(items) == 0 {
		return
	}
	index, err := loadBinanceAlphaTokenIndex(ctx)
	if err != nil {
		log.Printf("[BinanceAlpha] token list unavailable: %v", err)
		return
	}
	enrichHotPoolBinanceAlphaWithIndex(chain, items, index)
}

func enrichHotPoolBinanceAlphaWithIndex(chain string, items []HotPoolResponse, index map[string]binanceAlphaTokenInfo) {
	if len(items) == 0 || len(index) == 0 {
		return
	}
	for i := range items {
		for _, target := range tokenRiskTargetsForPoolItem(items[i], chain) {
			key := binanceAlphaTokenKey(target.Chain, target.Address)
			if key == "" {
				continue
			}
			alpha, ok := index[key]
			if !ok {
				continue
			}
			items[i].Badges = appendHotPoolBadge(items[i].Badges, binanceAlphaBadgeLabel, binanceAlphaBadgeTip(alpha))
			break
		}
	}
}

func binanceAlphaBadgeTip(alpha binanceAlphaTokenInfo) string {
	parts := []string{binanceAlphaBadgeLabel}
	if alpha.Symbol != "" {
		parts = append(parts, alpha.Symbol)
	}
	if alpha.AlphaID != "" {
		parts = append(parts, alpha.AlphaID)
	}
	if alpha.ChainName != "" {
		parts = append(parts, alpha.ChainName)
	}
	return strings.Join(parts, " · ")
}

func appendHotPoolBadge(raw json.RawMessage, label string, tip string) json.RawMessage {
	label = strings.TrimSpace(label)
	tip = strings.TrimSpace(tip)
	if label == "" {
		return raw
	}
	if tip == "" {
		tip = label
	}

	var badges []any
	if len(raw) > 0 && json.Valid(raw) {
		_ = json.Unmarshal(raw, &badges)
	}
	for _, badge := range badges {
		if strings.EqualFold(hotPoolBadgeText(badge), label) {
			return raw
		}
	}
	badges = append(badges, map[string]string{
		"label": label,
		"tip":   tip,
	})
	out, err := json.Marshal(badges)
	if err != nil {
		return raw
	}
	return json.RawMessage(out)
}

func hotPoolBadgeText(value any) string {
	switch item := value.(type) {
	case string:
		return strings.TrimSpace(item)
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%g", item))
	case map[string]any:
		for _, key := range []string{"label", "text", "title", "name", "badge", "content", "value", "type", "tip"} {
			if text := hotPoolBadgeScalarText(item[key]); text != "" {
				return text
			}
		}
	}
	return ""
}

func hotPoolBadgeScalarText(value any) string {
	switch item := value.(type) {
	case string:
		return strings.TrimSpace(item)
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%g", item))
	default:
		return ""
	}
}
