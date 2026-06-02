package exchange

import (
	"TgLpBot/base/config"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// OKXDexService handles OKX DEX API interactions
type OKXDexService struct {
	apiURL     string
	apiKey     string
	secretKey  string
	passphrase string
	client     *http.Client
}

const (
	okxAdvancedInfoCacheTTL   = 10 * time.Minute
	okxApproveSpenderCacheTTL = 24 * time.Hour
)

var (
	okxReadGroup           singleflight.Group
	okxAdvancedInfoCache   sync.Map
	okxApproveSpenderCache sync.Map
)

type okxAdvancedInfoCacheEntry struct {
	value     MarketTokenAdvancedInfoResponse
	expiresAt time.Time
}

type okxApproveSpenderCacheEntry struct {
	value     string
	expiresAt time.Time
}

// NewOKXDexService creates a new OKX DEX service
func NewOKXDexService() *OKXDexService {
	return &OKXDexService{
		apiURL:     config.AppConfig.OKXDexAPIURL,
		apiKey:     config.AppConfig.OKXAPIKey,
		secretKey:  config.AppConfig.OKXSecretKey,
		passphrase: config.AppConfig.OKXPassphrase,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *OKXDexService) isV6() bool {
	return strings.Contains(s.apiURL, "/api/v6/") || strings.Contains(s.apiURL, "/api/v6")
}

func (s *OKXDexService) chainQueryKey() string {
	// OKX DEX aggregator uses "chainId" on v5 endpoints and "chainIndex" on v6.
	// Detect by API base URL to keep backward compatibility.
	if s.isV6() {
		return "chainIndex"
	}
	return "chainId"
}

func (s *OKXDexService) slippageQueryKey() string {
	// OKX DEX aggregator uses "slippage" on v5 endpoints and "slippagePercent" on v6.
	if s.isV6() {
		return "slippagePercent"
	}
	return "slippage"
}

func (s *OKXDexService) slippageQueryValue(slippage string) string {
	slippage = strings.TrimSpace(slippage)
	if slippage == "" {
		return ""
	}
	if !s.isV6() {
		return slippage
	}

	// Existing callers pass slippage as a decimal fraction (e.g. 0.005 for 0.5%).
	// v6 expects percent value (e.g. 0.5).
	rat, ok := new(big.Rat).SetString(slippage)
	if !ok {
		return slippage
	}
	rat.Mul(rat, big.NewRat(100, 1))
	if rat.Cmp(big.NewRat(0, 1)) < 0 {
		rat = big.NewRat(0, 1)
	}
	if rat.Cmp(big.NewRat(100, 1)) > 0 {
		rat = big.NewRat(100, 1)
	}
	out := rat.FloatString(6)
	out = strings.TrimRight(out, "0")
	out = strings.TrimRight(out, ".")
	if out == "" {
		return "0"
	}
	return out
}

// SwapRequest represents a swap request
type SwapRequest struct {
	ChainID                        string `json:"chainId"`
	FromTokenAddress               string `json:"fromTokenAddress"`
	ToTokenAddress                 string `json:"toTokenAddress"`
	Amount                         string `json:"amount"`
	Slippage                       string `json:"slippage"`
	UserWalletAddress              string `json:"userWalletAddress"`
	FeePercent                     string `json:"feePercent,omitempty"`
	FromTokenReferrerWalletAddress string `json:"fromTokenReferrerWalletAddress,omitempty"`
	ToTokenReferrerWalletAddress   string `json:"toTokenReferrerWalletAddress,omitempty"`
}

// SwapResponse represents a swap response
type SwapResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data []struct {
		Tx struct {
			From     string `json:"from"`
			To       string `json:"to"`
			Data     string `json:"data"`
			Value    string `json:"value"`
			Gas      string `json:"gas"`
			GasPrice string `json:"gasPrice"`
		} `json:"tx"`
		RouterResult struct {
			FromTokenAmount string          `json:"fromTokenAmount"`
			ToTokenAmount   string          `json:"toTokenAmount"`
			DexRouterList   json.RawMessage `json:"dexRouterList"`
		} `json:"routerResult"`
	} `json:"data"`
}

type OKXAPIError struct {
	Endpoint string
	Code     string
	Msg      string
}

type MarketTokenAdvancedInfoRequest struct {
	ChainIndex           string
	TokenContractAddress string
}

type MarketTokenAdvancedInfo struct {
	ChainIndex                   string   `json:"chainIndex"`
	TokenContractAddress         string   `json:"tokenContractAddress"`
	TotalFee                     string   `json:"totalFee"`
	LPBurnedPercent              string   `json:"lpBurnedPercent"`
	IsInternal                   bool     `json:"isInternal"`
	ProtocolID                   string   `json:"protocolId"`
	Progress                     string   `json:"progress"`
	TokenTags                    []string `json:"tokenTags"`
	CreateTime                   string   `json:"createTime"`
	CreatorAddress               string   `json:"creatorAddress"`
	DevRugPullTokenCount         string   `json:"devRugPullTokenCount"`
	DevCreateTokenCount          string   `json:"devCreateTokenCount"`
	DevLaunchedTokenCount        string   `json:"devLaunchedTokenCount"`
	RiskControlLevel             string   `json:"riskControlLevel"`
	Top10HoldPercent             string   `json:"top10HoldPercent"`
	DevHoldingPercent            string   `json:"devHoldingPercent"`
	BundleHoldingPercent         string   `json:"bundleHoldingPercent"`
	SuspiciousHoldingPercent     string   `json:"suspiciousHoldingPercent"`
	SniperHoldingPercent         string   `json:"sniperHoldingPercent"`
	SnipersClearAddressCount     string   `json:"snipersClearAddressCount"`
	SnipersTotal                 string   `json:"snipersTotal"`
	InsiderNetworkHoldPercent    string   `json:"insiderNetworkHoldPercent"`
	InsiderNetworkAddressCount   string   `json:"insiderNetworkAddressCount"`
	PhishingActivitiesCount      string   `json:"phishingActivitiesCount"`
	BlackListActivitiesCount     string   `json:"blackListActivitiesCount"`
	ContractCreatorRiskTokenRate string   `json:"contractCreatorRiskTokenRate"`
}

type MarketTokenAdvancedInfoResponse struct {
	Code string                    `json:"code"`
	Msg  string                    `json:"msg"`
	Data []MarketTokenAdvancedInfo `json:"data"`
}

func (r *MarketTokenAdvancedInfoResponse) UnmarshalJSON(body []byte) error {
	var raw struct {
		Code string          `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	r.Code = raw.Code
	r.Msg = raw.Msg

	data := bytes.TrimSpace(raw.Data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		r.Data = []MarketTokenAdvancedInfo{}
		return nil
	}
	if bytes.HasPrefix(data, []byte("[")) {
		return json.Unmarshal(data, &r.Data)
	}
	if bytes.HasPrefix(data, []byte("{")) {
		var item MarketTokenAdvancedInfo
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		r.Data = []MarketTokenAdvancedInfo{item}
		return nil
	}
	return fmt.Errorf("unexpected advanced-info data shape")
}

func (e *OKXAPIError) Error() string {
	if e == nil {
		return "OKX API error"
	}
	if strings.TrimSpace(e.Endpoint) == "" {
		return fmt.Sprintf("OKX API error: %s (code=%s)", e.Msg, e.Code)
	}
	return fmt.Sprintf("OKX API error: %s (code=%s endpoint=%s)", e.Msg, e.Code, e.Endpoint)
}

func (s *OKXDexService) marketAPIURL() string {
	base := strings.TrimSpace(s.apiURL)
	if base == "" {
		return "https://web3.okx.com/api/v6/dex/market"
	}
	base = strings.TrimRight(base, "/")
	base = strings.Replace(base, "https://www.okx.com/", "https://web3.okx.com/", 1)
	replacer := strings.NewReplacer(
		"/api/v6/dex/aggregator", "/api/v6/dex/market",
		"/api/v5/dex/aggregator", "/api/v5/dex/market",
	)
	next := replacer.Replace(base)
	if next != base {
		return next
	}
	if strings.Contains(base, "/api/v6/dex/market") || strings.Contains(base, "/api/v5/dex/market") {
		return base
	}
	return "https://web3.okx.com/api/v6/dex/market"
}

func normalizeOKXSwapFeePercent(raw string, chainID string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	rat, ok := new(big.Rat).SetString(raw)
	if !ok || rat.Sign() <= 0 {
		return ""
	}

	max := big.NewRat(3, 1)
	if strings.TrimSpace(chainID) == "501" {
		max = big.NewRat(10, 1)
	}
	if rat.Cmp(max) > 0 {
		rat = max
	}

	scale := big.NewInt(1_000_000_000)
	scaled := new(big.Rat).Mul(rat, new(big.Rat).SetInt(scale))
	units := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	if units.Sign() <= 0 {
		units.SetInt64(1)
	}

	integral := new(big.Int).Quo(new(big.Int).Set(units), scale)
	fractional := new(big.Int).Mod(units, scale)
	if fractional.Sign() == 0 {
		return integral.String()
	}

	fracText := fractional.String()
	if len(fracText) < 9 {
		fracText = strings.Repeat("0", 9-len(fracText)) + fracText
	}
	fracText = strings.TrimRight(fracText, "0")
	return integral.String() + "." + fracText
}

func (s *OKXDexService) swapFeeParams(req SwapRequest) (string, string, string, error) {
	feePercent := normalizeOKXSwapFeePercent(req.FeePercent, req.ChainID)
	fromReferrer := strings.TrimSpace(req.FromTokenReferrerWalletAddress)
	toReferrer := strings.TrimSpace(req.ToTokenReferrerWalletAddress)

	if config.AppConfig != nil {
		if feePercent == "" {
			feePercent = normalizeOKXSwapFeePercent(config.AppConfig.OKXSwapFeePercent, req.ChainID)
		}
		if fromReferrer == "" && toReferrer == "" {
			recipient := strings.TrimSpace(config.AppConfig.OKXSwapFeeRecipient)
			switch strings.ToLower(strings.TrimSpace(config.AppConfig.OKXSwapFeeToken)) {
			case "from":
				fromReferrer = recipient
			default:
				toReferrer = recipient
			}
		}
	}

	if feePercent == "" {
		return "", "", "", nil
	}
	if fromReferrer == "" && toReferrer == "" {
		return "", "", "", fmt.Errorf("OKX swap fee recipient is required when feePercent is set")
	}
	if fromReferrer != "" && toReferrer != "" {
		return "", "", "", fmt.Errorf("OKX swap fee supports only one referrer wallet address")
	}
	return feePercent, fromReferrer, toReferrer, nil
}

func (s *OKXDexService) swapEndpoint(req SwapRequest) (string, error) {
	query := url.Values{}
	query.Set(s.chainQueryKey(), strings.TrimSpace(req.ChainID))
	query.Set("fromTokenAddress", strings.TrimSpace(req.FromTokenAddress))
	query.Set("toTokenAddress", strings.TrimSpace(req.ToTokenAddress))
	query.Set("amount", strings.TrimSpace(req.Amount))
	query.Set(s.slippageQueryKey(), s.slippageQueryValue(req.Slippage))
	query.Set("userWalletAddress", strings.TrimSpace(req.UserWalletAddress))

	feePercent, fromReferrer, toReferrer, err := s.swapFeeParams(req)
	if err != nil {
		return "", err
	}
	if feePercent != "" {
		query.Set("feePercent", feePercent)
		if fromReferrer != "" {
			query.Set("fromTokenReferrerWalletAddress", fromReferrer)
		}
		if toReferrer != "" {
			query.Set("toTokenReferrerWalletAddress", toReferrer)
		}
	}

	return fmt.Sprintf("%s/swap?%s", strings.TrimRight(s.apiURL, "/"), query.Encode()), nil
}

// GetSwapData gets swap transaction data
func (s *OKXDexService) GetSwapData(req SwapRequest) (*SwapResponse, error) {
	endpoint, err := s.swapEndpoint(req)
	if err != nil {
		return nil, err
	}

	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX API] 请求 URL: %s", endpoint)
	}

	httpReq, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	s.addHeaders(httpReq, "", timestamp)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX API] 响应原始数据: %s", string(body))
	}

	var swapResp SwapResponse
	if err := json.Unmarshal(body, &swapResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if swapResp.Code != "0" {
		return nil, &OKXAPIError{Endpoint: "swap", Code: swapResp.Code, Msg: swapResp.Msg}
	}

	// 打印详细响应信息
	if config.AppConfig != nil && config.AppConfig.OKXDebug && len(swapResp.Data) > 0 {
		tx := swapResp.Data[0].Tx
		log.Printf("[OKX API] 响应详情:")
		log.Printf("  tx.from: %s", tx.From)
		log.Printf("  tx.to: %s", tx.To)
		log.Printf("  tx.value: %s", tx.Value)
		log.Printf("  tx.gas: %s", tx.Gas)
		log.Printf("  tx.data 长度: %d", len(tx.Data))
		log.Printf("  routerResult.fromTokenAmount: %s", swapResp.Data[0].RouterResult.FromTokenAmount)
		log.Printf("  routerResult.toTokenAmount: %s", swapResp.Data[0].RouterResult.ToTokenAmount)
	}

	return &swapResp, nil
}

func okxReadCacheBaseKey(prefix string, parts ...string) string {
	clean := make([]string, 0, len(parts)+1)
	clean = append(clean, strings.TrimSpace(prefix))
	for _, part := range parts {
		clean = append(clean, strings.TrimSpace(part))
	}
	return strings.Join(clean, "|")
}

func cloneMarketTokenAdvancedInfoResponse(resp MarketTokenAdvancedInfoResponse) *MarketTokenAdvancedInfoResponse {
	rows := make([]MarketTokenAdvancedInfo, len(resp.Data))
	for i := range resp.Data {
		rows[i] = resp.Data[i]
		rows[i].TokenTags = append([]string(nil), resp.Data[i].TokenTags...)
	}
	return &MarketTokenAdvancedInfoResponse{Code: resp.Code, Msg: resp.Msg, Data: rows}
}

func okxAdvancedInfoCacheKey(baseURL string, req MarketTokenAdvancedInfoRequest) string {
	chainIndex := strings.TrimSpace(req.ChainIndex)
	tokenAddress := strings.ToLower(strings.TrimSpace(req.TokenContractAddress))
	if chainIndex == "" || tokenAddress == "" {
		return ""
	}
	return okxReadCacheBaseKey("advanced-info", strings.TrimRight(baseURL, "/"), chainIndex, tokenAddress)
}

func okxApproveSpenderCacheKey(baseURL, chainID, tokenAddress string) string {
	chainID = strings.TrimSpace(chainID)
	tokenAddress = strings.ToLower(strings.TrimSpace(tokenAddress))
	if chainID == "" || tokenAddress == "" {
		return ""
	}
	return okxReadCacheBaseKey("approve-transaction", strings.TrimRight(baseURL, "/"), chainID, tokenAddress)
}

func (s *OKXDexService) GetMarketTokenAdvancedInfo(ctx context.Context, req MarketTokenAdvancedInfoRequest) (*MarketTokenAdvancedInfoResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	chainIndex := strings.TrimSpace(req.ChainIndex)
	tokenAddress := strings.ToLower(strings.TrimSpace(req.TokenContractAddress))
	if chainIndex == "" {
		return nil, fmt.Errorf("chainIndex is required")
	}
	if tokenAddress == "" {
		return nil, fmt.Errorf("tokenContractAddress is required")
	}

	normalized := MarketTokenAdvancedInfoRequest{
		ChainIndex:           chainIndex,
		TokenContractAddress: tokenAddress,
	}
	baseURL := s.marketAPIURL()
	cacheKey := okxAdvancedInfoCacheKey(baseURL, normalized)
	now := time.Now()
	if raw, ok := okxAdvancedInfoCache.Load(cacheKey); ok {
		if entry, ok := raw.(okxAdvancedInfoCacheEntry); ok && entry.expiresAt.After(now) {
			return cloneMarketTokenAdvancedInfoResponse(entry.value), nil
		}
		okxAdvancedInfoCache.Delete(cacheKey)
	}

	fetchedAny, err, _ := okxReadGroup.Do("advanced-info-fetch|"+cacheKey, func() (any, error) {
		return s.getMarketTokenAdvancedInfoUncached(ctx, baseURL, normalized)
	})
	if err != nil {
		return nil, err
	}
	fetched, ok := fetchedAny.(*MarketTokenAdvancedInfoResponse)
	if !ok || fetched == nil {
		return nil, fmt.Errorf("unexpected OKX advanced-info response")
	}
	okxAdvancedInfoCache.Store(cacheKey, okxAdvancedInfoCacheEntry{value: *cloneMarketTokenAdvancedInfoResponse(*fetched), expiresAt: now.Add(okxAdvancedInfoCacheTTL)})
	return cloneMarketTokenAdvancedInfoResponse(*fetched), nil
}

func (s *OKXDexService) getMarketTokenAdvancedInfoUncached(ctx context.Context, baseURL string, req MarketTokenAdvancedInfoRequest) (*MarketTokenAdvancedInfoResponse, error) {
	query := url.Values{}
	query.Set("chainIndex", strings.TrimSpace(req.ChainIndex))
	query.Set("tokenContractAddress", strings.ToLower(strings.TrimSpace(req.TokenContractAddress)))

	endpoint := fmt.Sprintf("%s/token/advanced-info?%s", baseURL, query.Encode())
	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX Market] advanced-info request URL: %s", endpoint)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	s.addHeaders(httpReq, "", timestamp)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OKX market/token/advanced-info http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX Market] advanced-info raw response: %s", string(respBody))
	}

	var out MarketTokenAdvancedInfoResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if out.Code != "0" {
		return nil, &OKXAPIError{Endpoint: "market/token/advanced-info", Code: out.Code, Msg: out.Msg}
	}
	return &out, nil
}

// addHeaders adds authentication headers to the request
func (s *OKXDexService) addHeaders(req *http.Request, body, timestamp string) {
	message := timestamp + req.Method + req.URL.RequestURI() + body
	mac := hmac.New(sha256.New, []byte(s.secretKey))
	mac.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req.Header.Set("OK-ACCESS-KEY", s.apiKey)
	req.Header.Set("OK-ACCESS-SIGN", signature)
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", s.passphrase)
	req.Header.Set("Content-Type", "application/json")
}

// ApproveTransactionResponse represents the response from /approve-transaction API
type ApproveTransactionResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data []struct {
		DexContractAddress string `json:"dexContractAddress"` // OKX approve 目标地址
	} `json:"data"`
}

// GetApproveSpender 获取 OKX DEX 的 approve 目标地址
// 这个地址是需要 approve 代币的 spender，每个链可能不同，需要动态获取
func (s *OKXDexService) GetApproveSpender(chainID string, tokenAddress string) (string, error) {
	chainID = strings.TrimSpace(chainID)
	tokenAddress = strings.ToLower(strings.TrimSpace(tokenAddress))
	cacheKey := okxApproveSpenderCacheKey(s.apiURL, chainID, tokenAddress)
	now := time.Now()
	if cacheKey != "" {
		if raw, ok := okxApproveSpenderCache.Load(cacheKey); ok {
			if entry, ok := raw.(okxApproveSpenderCacheEntry); ok && entry.expiresAt.After(now) {
				return entry.value, nil
			}
			okxApproveSpenderCache.Delete(cacheKey)
		}
	}

	fetchedAny, err, _ := okxReadGroup.Do("approve-spender-fetch|"+cacheKey, func() (any, error) {
		return s.getApproveSpenderUncached(chainID, tokenAddress)
	})
	if err != nil {
		return "", err
	}
	approveSpender, ok := fetchedAny.(string)
	if !ok || strings.TrimSpace(approveSpender) == "" {
		return "", fmt.Errorf("unexpected OKX approve spender response")
	}
	if cacheKey != "" {
		okxApproveSpenderCache.Store(cacheKey, okxApproveSpenderCacheEntry{
			value:     approveSpender,
			expiresAt: now.Add(okxApproveSpenderCacheTTL),
		})
	}
	return approveSpender, nil
}

func (s *OKXDexService) getApproveSpenderUncached(chainID string, tokenAddress string) (string, error) {
	url := fmt.Sprintf("%s/approve-transaction?%s=%s&tokenContractAddress=%s&approveAmount=1",
		s.apiURL, s.chainQueryKey(), chainID, tokenAddress)

	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	s.addHeaders(httpReq, "", timestamp)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var approveResp ApproveTransactionResponse
	if err := json.Unmarshal(body, &approveResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if approveResp.Code != "0" {
		return "", &OKXAPIError{Endpoint: "approve-transaction", Code: approveResp.Code, Msg: approveResp.Msg}
	}

	if len(approveResp.Data) == 0 || approveResp.Data[0].DexContractAddress == "" {
		return "", fmt.Errorf("OKX API returned empty approve address")
	}

	return approveResp.Data[0].DexContractAddress, nil
}
