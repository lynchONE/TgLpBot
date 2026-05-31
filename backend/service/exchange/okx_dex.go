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
	"sort"
	"strconv"
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
	okxBasicInfoCacheTTL      = 24 * time.Hour
	okxBasicInfoMissCacheTTL  = 30 * time.Minute
	okxAdvancedInfoCacheTTL   = 10 * time.Minute
	okxCandlesLiveCacheTTL    = 10 * time.Second
	okxCandlesHistoryCacheTTL = 2 * time.Minute
	okxBalanceCacheTTL        = 3 * time.Second
	okxApproveSpenderCacheTTL = 24 * time.Hour
)

var (
	okxReadGroup           singleflight.Group
	okxBasicInfoCache      sync.Map
	okxAdvancedInfoCache   sync.Map
	okxCandlesCache        sync.Map
	okxDeFiCache           sync.Map
	okxBalanceCache        sync.Map
	okxApproveSpenderCache sync.Map
)

type okxBasicInfoCacheEntry struct {
	value     MarketTokenBasicInfo
	expiresAt time.Time
	missing   bool
}

type okxAdvancedInfoCacheEntry struct {
	value     MarketTokenAdvancedInfoResponse
	expiresAt time.Time
}

type okxCandlesCacheEntry struct {
	value     MarketCandlesResponse
	expiresAt time.Time
}

type okxBalanceCacheEntry struct {
	value     AllTokenBalancesResponse
	expiresAt time.Time
}

type okxDeFiCacheEntry struct {
	value     DeFiUserAssetResponse
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

type MarketCandlesRequest struct {
	ChainIndex           string
	TokenContractAddress string
	Bar                  string
	Limit                int
	Before               string
	After                string
}

type MarketTokenBasicInfoRequest struct {
	ChainIndex           string `json:"chainIndex"`
	TokenContractAddress string `json:"tokenContractAddress"`
}

type MarketTokenAdvancedInfoRequest struct {
	ChainIndex           string
	TokenContractAddress string
}

type MarketTokenBasicInfo struct {
	ChainIndex           string `json:"chainIndex"`
	TokenContractAddress string `json:"tokenContractAddress"`
	TokenSymbol          string `json:"tokenSymbol"`
	TokenName            string `json:"tokenName"`
	TokenLogoURL         string `json:"tokenLogoUrl"`
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

type MarketCandle struct {
	TimestampMS int64
	Open        float64
	High        float64
	Low         float64
	Close       float64
	Volume      float64
	VolumeUSD   float64
	Confirm     bool
}

type MarketCandlesResponse struct {
	Code string         `json:"code"`
	Msg  string         `json:"msg"`
	Data [][]string     `json:"data"`
	Rows []MarketCandle `json:"-"`
}

type MarketTokenBasicInfoResponse struct {
	Code string                 `json:"code"`
	Msg  string                 `json:"msg"`
	Data []MarketTokenBasicInfo `json:"data"`
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

// TokenBalance represents a token balance from OKX balance API
type TokenBalance struct {
	TokenContractAddress string `json:"tokenContractAddress"`
	Symbol               string `json:"symbol"`
	Balance              string `json:"balance"`
	TokenPrice           string `json:"tokenPrice"`
	TokenType            string `json:"tokenType"`
}

// AllTokenBalancesResponse represents the response from OKX balance API
type AllTokenBalancesResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data []struct {
		TokenAssets []TokenBalance `json:"tokenAssets"`
	} `json:"data"`
}

type DeFiWalletAddressRequest struct {
	ChainIndex    string `json:"chainIndex"`
	WalletAddress string `json:"walletAddress"`
}

type DeFiUserAssetPlatformListRequest struct {
	WalletAddressList []DeFiWalletAddressRequest `json:"walletAddressList"`
}

type DeFiPlatformRequest struct {
	AnalysisPlatformID string `json:"analysisPlatformId"`
	ChainIndex         string `json:"chainIndex,omitempty"`
}

type DeFiUserAssetPlatformDetailRequest struct {
	WalletAddressList []DeFiWalletAddressRequest `json:"walletAddressList"`
	PlatformList      []DeFiPlatformRequest      `json:"platformList"`
}

type DeFiUserAssetResponse struct {
	Code string          `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func (r *DeFiUserAssetResponse) UnmarshalJSON(body []byte) error {
	var raw struct {
		Code json.RawMessage `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	code := strings.TrimSpace(string(raw.Code))
	if len(raw.Code) > 0 {
		if err := json.Unmarshal(raw.Code, &r.Code); err != nil {
			r.Code = strings.Trim(code, `"`)
		}
	}
	r.Msg = raw.Msg
	r.Data = raw.Data
	return nil
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

func (s *OKXDexService) defiUserAssetAPIURL() string {
	base := strings.TrimSpace(s.apiURL)
	if base == "" {
		return "https://web3.okx.com/api/v6/defi/user/asset"
	}
	base = strings.TrimRight(base, "/")
	base = strings.Replace(base, "https://www.okx.com/", "https://web3.okx.com/", 1)
	replacer := strings.NewReplacer(
		"/api/v6/dex/aggregator", "/api/v6/defi/user/asset",
		"/api/v5/dex/aggregator", "/api/v6/defi/user/asset",
		"/api/v6/dex/market", "/api/v6/defi/user/asset",
		"/api/v5/dex/market", "/api/v6/defi/user/asset",
	)
	next := replacer.Replace(base)
	if next != base {
		return next
	}
	if strings.Contains(base, "/api/v6/defi/user/asset") {
		return base
	}
	return "https://web3.okx.com/api/v6/defi/user/asset"
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

func parseOKXFloat(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseOKXInt64(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseOKXBool(raw string) bool {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}

func okxReadCacheBaseKey(prefix string, parts ...string) string {
	clean := make([]string, 0, len(parts)+1)
	clean = append(clean, strings.TrimSpace(prefix))
	for _, part := range parts {
		clean = append(clean, strings.TrimSpace(part))
	}
	return strings.Join(clean, "|")
}

func cloneMarketTokenBasicInfoResponse(code string, msg string, rows []MarketTokenBasicInfo) *MarketTokenBasicInfoResponse {
	out := make([]MarketTokenBasicInfo, len(rows))
	copy(out, rows)
	return &MarketTokenBasicInfoResponse{Code: code, Msg: msg, Data: out}
}

func cloneMarketTokenAdvancedInfoResponse(resp MarketTokenAdvancedInfoResponse) *MarketTokenAdvancedInfoResponse {
	rows := make([]MarketTokenAdvancedInfo, len(resp.Data))
	for i := range resp.Data {
		rows[i] = resp.Data[i]
		rows[i].TokenTags = append([]string(nil), resp.Data[i].TokenTags...)
	}
	return &MarketTokenAdvancedInfoResponse{Code: resp.Code, Msg: resp.Msg, Data: rows}
}

func cloneMarketCandlesResponse(resp MarketCandlesResponse) *MarketCandlesResponse {
	data := make([][]string, len(resp.Data))
	for i := range resp.Data {
		data[i] = append([]string(nil), resp.Data[i]...)
	}
	rows := make([]MarketCandle, len(resp.Rows))
	copy(rows, resp.Rows)
	return &MarketCandlesResponse{Code: resp.Code, Msg: resp.Msg, Data: data, Rows: rows}
}

func cloneAllTokenBalancesResponse(resp AllTokenBalancesResponse) *AllTokenBalancesResponse {
	out := AllTokenBalancesResponse{Code: resp.Code, Msg: resp.Msg}
	out.Data = make([]struct {
		TokenAssets []TokenBalance `json:"tokenAssets"`
	}, len(resp.Data))
	for i := range resp.Data {
		out.Data[i].TokenAssets = append([]TokenBalance(nil), resp.Data[i].TokenAssets...)
	}
	return &out
}

func cloneDeFiUserAssetResponse(resp DeFiUserAssetResponse) *DeFiUserAssetResponse {
	data := append(json.RawMessage(nil), resp.Data...)
	return &DeFiUserAssetResponse{Code: resp.Code, Msg: resp.Msg, Data: data}
}

func okxBasicInfoCacheKey(baseURL string, req MarketTokenBasicInfoRequest) string {
	chainIndex := strings.TrimSpace(req.ChainIndex)
	tokenAddress := strings.ToLower(strings.TrimSpace(req.TokenContractAddress))
	if chainIndex == "" || tokenAddress == "" {
		return ""
	}
	return okxReadCacheBaseKey("basic-info", strings.TrimRight(baseURL, "/"), chainIndex, tokenAddress)
}

func okxAdvancedInfoCacheKey(baseURL string, req MarketTokenAdvancedInfoRequest) string {
	chainIndex := strings.TrimSpace(req.ChainIndex)
	tokenAddress := strings.ToLower(strings.TrimSpace(req.TokenContractAddress))
	if chainIndex == "" || tokenAddress == "" {
		return ""
	}
	return okxReadCacheBaseKey("advanced-info", strings.TrimRight(baseURL, "/"), chainIndex, tokenAddress)
}

func okxCandlesCacheKey(baseURL string, req MarketCandlesRequest) string {
	return okxReadCacheBaseKey(
		"candles",
		strings.TrimRight(baseURL, "/"),
		req.ChainIndex,
		strings.ToLower(req.TokenContractAddress),
		req.Bar,
		strconv.Itoa(req.Limit),
		req.Before,
		req.After,
	)
}

func okxBalanceCacheKey(chains, address string) string {
	chains = strings.TrimSpace(chains)
	address = strings.ToLower(strings.TrimSpace(address))
	if chains == "" || address == "" {
		return ""
	}
	return okxReadCacheBaseKey("balance/all-token-balances-by-address", chains, address)
}

func okxApproveSpenderCacheKey(baseURL, chainID, tokenAddress string) string {
	chainID = strings.TrimSpace(chainID)
	tokenAddress = strings.ToLower(strings.TrimSpace(tokenAddress))
	if chainID == "" || tokenAddress == "" {
		return ""
	}
	return okxReadCacheBaseKey("approve-transaction", strings.TrimRight(baseURL, "/"), chainID, tokenAddress)
}

func okxDeFiWalletCachePart(items []DeFiWalletAddressRequest) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		chainIndex := strings.TrimSpace(item.ChainIndex)
		walletAddress := normalizeDeFiUserAssetWalletAddress(item.WalletAddress)
		if chainIndex == "" || walletAddress == "" {
			continue
		}
		parts = append(parts, chainIndex+":"+walletAddress)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func okxDeFiPlatformCachePart(items []DeFiPlatformRequest) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		platformID := strings.TrimSpace(item.AnalysisPlatformID)
		if platformID == "" {
			continue
		}
		parts = append(parts, platformID+":"+strings.TrimSpace(item.ChainIndex))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func okxDeFiCacheTTL(resp *DeFiUserAssetResponse) time.Duration {
	if resp == nil {
		return 0
	}
	if okxDeFiRawHasUpdatingAssetStatus(resp.Data) {
		return 0
	}
	return 90 * time.Second
}

func okxDeFiRawHasUpdatingAssetStatus(raw json.RawMessage) bool {
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return okxDeFiValueHasUpdatingAssetStatus(value)
}

func okxDeFiValueHasUpdatingAssetStatus(value interface{}) bool {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, item := range typed {
			if strings.EqualFold(strings.TrimSpace(key), "assetStatus") &&
				strings.TrimSpace(fmt.Sprint(item)) == "2" {
				return true
			}
			if okxDeFiValueHasUpdatingAssetStatus(item) {
				return true
			}
		}
	case []interface{}:
		for _, item := range typed {
			if okxDeFiValueHasUpdatingAssetStatus(item) {
				return true
			}
		}
	}
	return false
}

func normalizeMarketCandlesRows(rawRows [][]string) []MarketCandle {
	out := make([]MarketCandle, 0, len(rawRows))
	for _, row := range rawRows {
		if len(row) < 8 {
			continue
		}
		ts := parseOKXInt64(row[0])
		if ts <= 0 {
			continue
		}
		out = append(out, MarketCandle{
			TimestampMS: ts,
			Open:        parseOKXFloat(row[1]),
			High:        parseOKXFloat(row[2]),
			Low:         parseOKXFloat(row[3]),
			Close:       parseOKXFloat(row[4]),
			Volume:      parseOKXFloat(row[5]),
			VolumeUSD:   parseOKXFloat(row[6]),
			Confirm:     parseOKXBool(row[7]),
		})
	}
	return out
}

func (s *OKXDexService) GetMarketCandles(req MarketCandlesRequest) (*MarketCandlesResponse, error) {
	normalized, err := normalizeMarketCandlesRequest(s, req)
	if err != nil {
		return nil, err
	}
	baseURL := s.marketAPIURL()
	cacheKey := okxCandlesCacheKey(baseURL, normalized)
	now := time.Now()
	if raw, ok := okxCandlesCache.Load(cacheKey); ok {
		if entry, ok := raw.(okxCandlesCacheEntry); ok && entry.expiresAt.After(now) {
			return cloneMarketCandlesResponse(entry.value), nil
		}
		okxCandlesCache.Delete(cacheKey)
	}

	fetchedAny, err, _ := okxReadGroup.Do("candles-fetch|"+cacheKey, func() (any, error) {
		return s.getMarketCandlesUncached(baseURL, normalized)
	})
	if err != nil {
		return nil, err
	}
	fetched, ok := fetchedAny.(*MarketCandlesResponse)
	if !ok || fetched == nil {
		return nil, fmt.Errorf("unexpected OKX candles response")
	}

	ttl := okxCandlesLiveCacheTTL
	if strings.TrimSpace(normalized.Before) != "" || strings.TrimSpace(normalized.After) != "" {
		ttl = okxCandlesHistoryCacheTTL
	}
	okxCandlesCache.Store(cacheKey, okxCandlesCacheEntry{value: *cloneMarketCandlesResponse(*fetched), expiresAt: now.Add(ttl)})
	return cloneMarketCandlesResponse(*fetched), nil
}

func normalizeMarketCandlesRequest(s *OKXDexService, req MarketCandlesRequest) (MarketCandlesRequest, error) {
	normalized := MarketCandlesRequest{
		ChainIndex:           strings.TrimSpace(req.ChainIndex),
		TokenContractAddress: strings.ToLower(strings.TrimSpace(req.TokenContractAddress)),
		Bar:                  strings.TrimSpace(req.Bar),
		Limit:                req.Limit,
		Before:               strings.TrimSpace(req.Before),
		After:                strings.TrimSpace(req.After),
	}
	if normalized.Bar == "" {
		normalized.Bar = "1m"
	}
	if normalized.Limit <= 0 {
		normalized.Limit = 240
	}
	if normalized.Limit > 299 {
		normalized.Limit = 299
	}
	return normalized, nil
}

func (s *OKXDexService) getMarketCandlesUncached(baseURL string, req MarketCandlesRequest) (*MarketCandlesResponse, error) {
	query := url.Values{}
	query.Set(s.chainQueryKey(), strings.TrimSpace(req.ChainIndex))
	query.Set("tokenContractAddress", strings.TrimSpace(req.TokenContractAddress))

	bar := strings.TrimSpace(req.Bar)
	if bar == "" {
		bar = "1m"
	}
	query.Set("bar", bar)

	limit := req.Limit
	if limit <= 0 {
		limit = 240
	}
	if limit > 299 {
		limit = 299
	}
	query.Set("limit", strconv.Itoa(limit))

	if before := strings.TrimSpace(req.Before); before != "" {
		query.Set("before", before)
	}
	if after := strings.TrimSpace(req.After); after != "" {
		query.Set("after", after)
	}

	endpoint := fmt.Sprintf("%s/candles?%s", baseURL, query.Encode())
	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX Market] request URL: %s", endpoint)
	}

	httpReq, err := http.NewRequest(http.MethodGet, endpoint, nil)
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX Market] raw response: %s", string(body))
	}

	var out MarketCandlesResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if out.Code != "0" {
		return nil, &OKXAPIError{Endpoint: "market/candles", Code: out.Code, Msg: out.Msg}
	}
	out.Rows = normalizeMarketCandlesRows(out.Data)
	return &out, nil
}

func (s *OKXDexService) GetMarketTokenBasicInfos(reqs []MarketTokenBasicInfoRequest) (*MarketTokenBasicInfoResponse, error) {
	if len(reqs) == 0 {
		return &MarketTokenBasicInfoResponse{
			Code: "0",
			Msg:  "",
			Data: []MarketTokenBasicInfo{},
		}, nil
	}

	baseURL := s.marketAPIURL()
	now := time.Now()
	uniquePayload := make([]MarketTokenBasicInfoRequest, 0, len(reqs))
	seenPayload := make(map[string]struct{}, len(reqs))
	for _, req := range reqs {
		chainIndex := strings.TrimSpace(req.ChainIndex)
		tokenAddress := strings.ToLower(strings.TrimSpace(req.TokenContractAddress))
		if chainIndex == "" || tokenAddress == "" {
			continue
		}
		key := okxBasicInfoCacheKey(baseURL, MarketTokenBasicInfoRequest{
			ChainIndex:           chainIndex,
			TokenContractAddress: tokenAddress,
		})
		if _, ok := seenPayload[key]; ok {
			continue
		}
		seenPayload[key] = struct{}{}
		uniquePayload = append(uniquePayload, MarketTokenBasicInfoRequest{
			ChainIndex:           chainIndex,
			TokenContractAddress: tokenAddress,
		})
	}
	if len(uniquePayload) == 0 {
		return &MarketTokenBasicInfoResponse{
			Code: "0",
			Msg:  "",
			Data: []MarketTokenBasicInfo{},
		}, nil
	}

	cachedRows := make([]MarketTokenBasicInfo, 0, len(uniquePayload))
	missing := make([]MarketTokenBasicInfoRequest, 0, len(uniquePayload))
	for _, req := range uniquePayload {
		key := okxBasicInfoCacheKey(baseURL, req)
		if raw, ok := okxBasicInfoCache.Load(key); ok {
			if entry, ok := raw.(okxBasicInfoCacheEntry); ok && entry.expiresAt.After(now) {
				if entry.missing {
					continue
				}
				cachedRows = append(cachedRows, entry.value)
				continue
			}
			okxBasicInfoCache.Delete(key)
		}
		missing = append(missing, req)
	}
	if len(missing) == 0 {
		return cloneMarketTokenBasicInfoResponse("0", "", cachedRows), nil
	}

	sort.Slice(missing, func(i, j int) bool {
		left := missing[i].ChainIndex + "|" + missing[i].TokenContractAddress
		right := missing[j].ChainIndex + "|" + missing[j].TokenContractAddress
		return left < right
	})
	groupKeyParts := make([]string, 0, len(missing)+2)
	groupKeyParts = append(groupKeyParts, "basic-info-fetch", strings.TrimRight(baseURL, "/"))
	for _, req := range missing {
		groupKeyParts = append(groupKeyParts, req.ChainIndex+"|"+req.TokenContractAddress)
	}
	groupKey := strings.Join(groupKeyParts, "|")

	fetchedAny, err, _ := okxReadGroup.Do(groupKey, func() (any, error) {
		return s.getMarketTokenBasicInfosUncached(baseURL, missing)
	})
	if err != nil {
		return nil, err
	}
	fetched, ok := fetchedAny.(*MarketTokenBasicInfoResponse)
	if !ok || fetched == nil {
		return nil, fmt.Errorf("unexpected OKX basic-info response")
	}

	found := make(map[string]struct{}, len(fetched.Data))
	expiresAt := now.Add(okxBasicInfoCacheTTL)
	for _, item := range fetched.Data {
		key := okxBasicInfoCacheKey(baseURL, MarketTokenBasicInfoRequest{
			ChainIndex:           item.ChainIndex,
			TokenContractAddress: item.TokenContractAddress,
		})
		if key == "" {
			continue
		}
		found[key] = struct{}{}
		okxBasicInfoCache.Store(key, okxBasicInfoCacheEntry{value: item, expiresAt: expiresAt})
	}
	missingExpiresAt := now.Add(okxBasicInfoMissCacheTTL)
	for _, req := range missing {
		key := okxBasicInfoCacheKey(baseURL, req)
		if key == "" {
			continue
		}
		if _, ok := found[key]; ok {
			continue
		}
		okxBasicInfoCache.Store(key, okxBasicInfoCacheEntry{expiresAt: missingExpiresAt, missing: true})
	}

	rows := append(cachedRows, fetched.Data...)
	return cloneMarketTokenBasicInfoResponse("0", "", rows), nil
}

func (s *OKXDexService) getMarketTokenBasicInfosUncached(baseURL string, reqs []MarketTokenBasicInfoRequest) (*MarketTokenBasicInfoResponse, error) {
	payload := make([]MarketTokenBasicInfoRequest, 0, len(reqs))
	for _, req := range reqs {
		chainIndex := strings.TrimSpace(req.ChainIndex)
		tokenAddress := strings.TrimSpace(req.TokenContractAddress)
		if chainIndex == "" || tokenAddress == "" {
			continue
		}
		payload = append(payload, MarketTokenBasicInfoRequest{
			ChainIndex:           chainIndex,
			TokenContractAddress: strings.ToLower(tokenAddress),
		})
	}
	if len(payload) == 0 {
		return &MarketTokenBasicInfoResponse{
			Code: "0",
			Msg:  "",
			Data: []MarketTokenBasicInfo{},
		}, nil
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/token/basic-info", baseURL)
	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX Market] basic-info request URL: %s", endpoint)
	}

	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	s.addHeaders(httpReq, string(body), timestamp)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX Market] basic-info raw response: %s", string(respBody))
	}

	var out MarketTokenBasicInfoResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if out.Code != "0" {
		return nil, &OKXAPIError{Endpoint: "market/token/basic-info", Code: out.Code, Msg: out.Msg}
	}
	return &out, nil
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

func (s *OKXDexService) GetDeFiUserAssetPlatformList(ctx context.Context, req DeFiUserAssetPlatformListRequest) (*DeFiUserAssetResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := normalizeDeFiUserAssetWalletPayload(req.WalletAddressList)
	if err != nil {
		return nil, err
	}
	cacheKey := okxReadCacheBaseKey(
		"defi-user-asset-platform-list",
		strings.TrimRight(s.defiUserAssetAPIURL(), "/"),
		okxDeFiWalletCachePart(payload),
	)
	return s.doDeFiUserAssetPost(ctx, "platform/list", DeFiUserAssetPlatformListRequest{
		WalletAddressList: payload,
	}, cacheKey)
}

func (s *OKXDexService) GetDeFiUserAssetPlatformDetail(ctx context.Context, req DeFiUserAssetPlatformDetailRequest) (*DeFiUserAssetResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	walletPayload, err := normalizeDeFiUserAssetWalletPayload(req.WalletAddressList)
	if err != nil {
		return nil, err
	}
	platformPayload := make([]DeFiPlatformRequest, 0, len(req.PlatformList))
	for _, item := range req.PlatformList {
		platformID := strings.TrimSpace(item.AnalysisPlatformID)
		if platformID == "" {
			continue
		}
		platformPayload = append(platformPayload, DeFiPlatformRequest{
			AnalysisPlatformID: platformID,
			ChainIndex:         strings.TrimSpace(item.ChainIndex),
		})
	}
	if len(platformPayload) == 0 {
		return nil, fmt.Errorf("analysisPlatformId is required")
	}

	cacheKey := okxReadCacheBaseKey(
		"defi-user-asset-platform-detail",
		strings.TrimRight(s.defiUserAssetAPIURL(), "/"),
		okxDeFiWalletCachePart(walletPayload),
		okxDeFiPlatformCachePart(platformPayload),
	)
	return s.doDeFiUserAssetPost(ctx, "platform/detail", DeFiUserAssetPlatformDetailRequest{
		WalletAddressList: walletPayload,
		PlatformList:      platformPayload,
	}, cacheKey)
}

func normalizeDeFiUserAssetWalletPayload(items []DeFiWalletAddressRequest) ([]DeFiWalletAddressRequest, error) {
	out := make([]DeFiWalletAddressRequest, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		chainIndex := strings.TrimSpace(item.ChainIndex)
		walletAddress := normalizeDeFiUserAssetWalletAddress(item.WalletAddress)
		if chainIndex == "" || walletAddress == "" {
			continue
		}
		key := chainIndex + "|" + walletAddress
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, DeFiWalletAddressRequest{
			ChainIndex:    chainIndex,
			WalletAddress: walletAddress,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("walletAddressList with chainIndex and walletAddress is required")
	}
	return out, nil
}

func normalizeDeFiUserAssetWalletAddress(value string) string {
	walletAddress := strings.TrimSpace(value)
	if len(walletAddress) == 42 && strings.HasPrefix(walletAddress, "0x") {
		return strings.ToLower(walletAddress)
	}
	if len(walletAddress) == 42 && strings.HasPrefix(walletAddress, "0X") {
		return "0x" + strings.ToLower(walletAddress[2:])
	}
	return walletAddress
}

func (s *OKXDexService) doDeFiUserAssetPost(ctx context.Context, path string, payload interface{}, cacheKey string) (*DeFiUserAssetResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	path = strings.Trim(path, "/")
	endpoint := fmt.Sprintf("%s/%s", s.defiUserAssetAPIURL(), path)
	now := time.Now()
	if strings.TrimSpace(cacheKey) != "" {
		if raw, ok := okxDeFiCache.Load(cacheKey); ok {
			if entry, ok := raw.(okxDeFiCacheEntry); ok && entry.expiresAt.After(now) {
				return cloneDeFiUserAssetResponse(entry.value), nil
			}
			okxDeFiCache.Delete(cacheKey)
		}
	}
	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX DeFi] request URL: %s", endpoint)
	}

	doReq := func() (*DeFiUserAssetResponse, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		s.addHeaders(httpReq, string(body), timestamp)

		resp, err := s.client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("OKX defi/user/asset/%s http %d: %s", path, resp.StatusCode, strings.TrimSpace(string(respBody)))
		}

		if config.AppConfig != nil && config.AppConfig.OKXDebug {
			log.Printf("[OKX DeFi] raw response: %s", string(respBody))
		}

		var out DeFiUserAssetResponse
		if err := json.Unmarshal(respBody, &out); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		if out.Code != "0" {
			return nil, &OKXAPIError{Endpoint: "defi/user/asset/" + path, Code: out.Code, Msg: out.Msg}
		}
		return &out, nil
	}

	var out *DeFiUserAssetResponse
	if strings.TrimSpace(cacheKey) != "" {
		fetchedAny, err, _ := okxReadGroup.Do("defi-fetch|"+cacheKey, func() (any, error) {
			return doReq()
		})
		if err != nil {
			return nil, err
		}
		fetched, ok := fetchedAny.(*DeFiUserAssetResponse)
		if !ok || fetched == nil {
			return nil, fmt.Errorf("unexpected OKX DeFi response")
		}
		out = fetched
	} else {
		out, err = doReq()
		if err != nil {
			return nil, err
		}
	}
	ttl := okxDeFiCacheTTL(out)
	if strings.TrimSpace(cacheKey) != "" && ttl > 0 {
		okxDeFiCache.Store(cacheKey, okxDeFiCacheEntry{value: *cloneDeFiUserAssetResponse(*out), expiresAt: now.Add(ttl)})
	}
	return cloneDeFiUserAssetResponse(*out), nil
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

// GetAllTokenBalances 获取钱包所有代币余额
func (s *OKXDexService) GetAllTokenBalances(chains, address string) (*AllTokenBalancesResponse, error) {
	chains = strings.TrimSpace(chains)
	address = strings.ToLower(strings.TrimSpace(address))
	cacheKey := okxBalanceCacheKey(chains, address)
	now := time.Now()
	if cacheKey != "" {
		if raw, ok := okxBalanceCache.Load(cacheKey); ok {
			if entry, ok := raw.(okxBalanceCacheEntry); ok && entry.expiresAt.After(now) {
				return cloneAllTokenBalancesResponse(entry.value), nil
			}
			okxBalanceCache.Delete(cacheKey)
		}
	}

	fetchedAny, err, _ := okxReadGroup.Do("balance-fetch|"+cacheKey, func() (any, error) {
		return s.getAllTokenBalancesUncached(chains, address)
	})
	if err != nil {
		return nil, err
	}
	fetched, ok := fetchedAny.(*AllTokenBalancesResponse)
	if !ok || fetched == nil {
		return nil, fmt.Errorf("unexpected OKX balance response")
	}
	if cacheKey != "" {
		okxBalanceCache.Store(cacheKey, okxBalanceCacheEntry{
			value:     *cloneAllTokenBalancesResponse(*fetched),
			expiresAt: now.Add(okxBalanceCacheTTL),
		})
	}
	return cloneAllTokenBalancesResponse(*fetched), nil
}

func (s *OKXDexService) getAllTokenBalancesUncached(chains, address string) (*AllTokenBalancesResponse, error) {
	query := url.Values{}
	query.Set("chains", strings.TrimSpace(chains))
	query.Set("address", strings.TrimSpace(address))

	endpoint := fmt.Sprintf("https://web3.okx.com/api/v6/dex/balance/all-token-balances-by-address?%s", query.Encode())
	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX Balance] request URL: %s", endpoint)
	}

	httpReq, err := http.NewRequest(http.MethodGet, endpoint, nil)
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX Balance] raw response: %s", string(body))
	}

	var out AllTokenBalancesResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if out.Code != "0" {
		return nil, &OKXAPIError{Endpoint: "balance/all-token-balances-by-address", Code: out.Code, Msg: out.Msg}
	}
	return &out, nil
}
