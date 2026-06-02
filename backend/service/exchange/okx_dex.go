package exchange

import (
	"TgLpBot/base/config"
	"TgLpBot/base/okxpool"
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
	usePool    bool
	pool       *okxpool.Manager
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
	svc := &OKXDexService{
		client:  &http.Client{Timeout: 30 * time.Second},
		usePool: true,
		pool:    okxpool.Default(),
	}
	if config.AppConfig == nil {
		return svc
	}
	svc.apiURL = config.AppConfig.OKXDexAPIURL
	svc.apiKey = config.AppConfig.OKXAPIKey
	svc.secretKey = config.AppConfig.OKXSecretKey
	svc.passphrase = config.AppConfig.OKXPassphrase
	return svc
}

func (s *OKXDexService) httpClient() *http.Client {
	if s != nil && s.client != nil {
		return s.client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (s *OKXDexService) manager() *okxpool.Manager {
	if s != nil && s.pool != nil {
		return s.pool
	}
	return okxpool.Default()
}

func (s *OKXDexService) staticConfig() okxpool.EffectiveConfig {
	return okxpool.EffectiveConfig{
		Source:     okxpool.SourceEnv,
		BaseURL:    strings.TrimRight(strings.TrimSpace(s.apiURL), "/"),
		APIKey:     strings.TrimSpace(s.apiKey),
		SecretKey:  strings.TrimSpace(s.secretKey),
		Passphrase: strings.TrimSpace(s.passphrase),
	}
}

func (s *OKXDexService) effectiveConfig(ctx context.Context) (okxpool.EffectiveConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil {
		return okxpool.EffectiveConfig{}, fmt.Errorf("OKX service is nil")
	}
	if !s.usePool {
		return s.staticConfig(), nil
	}
	mgr := s.manager()
	if mgr == nil {
		return s.staticConfig(), nil
	}
	eff, err := mgr.Effective(ctx)
	if err != nil {
		return okxpool.EffectiveConfig{}, err
	}
	if strings.TrimSpace(eff.BaseURL) == "" {
		return okxpool.EffectiveConfig{}, fmt.Errorf("OKX DEX API URL not configured")
	}
	return eff, nil
}

func (s *OKXDexService) recordOKXSuccess(ctx context.Context, eff okxpool.EffectiveConfig, latency time.Duration) {
	if s == nil || !s.usePool {
		return
	}
	if mgr := s.manager(); mgr != nil {
		mgr.RecordSuccess(ctx, eff, latency)
	}
}

func (s *OKXDexService) recordOKXFailure(ctx context.Context, eff okxpool.EffectiveConfig, latency time.Duration, err error) {
	if s == nil || !s.usePool || err == nil {
		return
	}
	if mgr := s.manager(); mgr != nil {
		mgr.RecordFailure(ctx, eff, latency, err)
	}
}

func (s *OKXDexService) shouldRetryWithNextOKXConfig(eff okxpool.EffectiveConfig, err error, attempt int) bool {
	if s == nil || !s.usePool || attempt >= 2 || err == nil {
		return false
	}
	return eff.Source == okxpool.SourceDB && eff.Config != nil
}

func (s *OKXDexService) isV6ForBase(baseURL string) bool {
	return strings.Contains(baseURL, "/api/v6/") || strings.Contains(baseURL, "/api/v6")
}

func (s *OKXDexService) chainQueryKeyForBase(baseURL string) string {
	if s.isV6ForBase(baseURL) {
		return "chainIndex"
	}
	return "chainId"
}

func (s *OKXDexService) slippageQueryKeyForBase(baseURL string) string {
	if s.isV6ForBase(baseURL) {
		return "slippagePercent"
	}
	return "slippage"
}

func (s *OKXDexService) slippageQueryValueForBase(baseURL string, slippage string) string {
	slippage = strings.TrimSpace(slippage)
	if slippage == "" {
		return ""
	}
	if !s.isV6ForBase(baseURL) {
		return slippage
	}

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

func (s *OKXDexService) isV6() bool {
	return s.isV6ForBase(s.apiURL)
}

func (s *OKXDexService) chainQueryKey() string {
	return s.chainQueryKeyForBase(s.apiURL)
}

func (s *OKXDexService) slippageQueryKey() string {
	return s.slippageQueryKeyForBase(s.apiURL)
}

func (s *OKXDexService) slippageQueryValue(slippage string) string {
	return s.slippageQueryValueForBase(s.apiURL, slippage)
}

// NewStaticOKXDexService creates an OKX DEX service bound to a fixed config.
func NewStaticOKXDexService(apiURL, apiKey, secretKey, passphrase string, client *http.Client) *OKXDexService {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &OKXDexService{
		apiURL:     apiURL,
		apiKey:     apiKey,
		secretKey:  secretKey,
		passphrase: passphrase,
		client:     client,
	}
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
	return s.marketAPIURLForBase(s.apiURL)
}

func (s *OKXDexService) marketAPIURLForBase(baseURL string) string {
	base := strings.TrimSpace(baseURL)
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
	return s.swapEndpointForBase(s.apiURL, req)
}

func (s *OKXDexService) swapEndpointForBase(baseURL string, req SwapRequest) (string, error) {
	query := url.Values{}
	query.Set(s.chainQueryKeyForBase(baseURL), strings.TrimSpace(req.ChainID))
	query.Set("fromTokenAddress", strings.TrimSpace(req.FromTokenAddress))
	query.Set("toTokenAddress", strings.TrimSpace(req.ToTokenAddress))
	query.Set("amount", strings.TrimSpace(req.Amount))
	query.Set(s.slippageQueryKeyForBase(baseURL), s.slippageQueryValueForBase(baseURL, req.Slippage))
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

	return fmt.Sprintf("%s/swap?%s", strings.TrimRight(strings.TrimSpace(baseURL), "/"), query.Encode()), nil
}

// GetSwapData gets swap transaction data
func (s *OKXDexService) GetSwapData(req SwapRequest) (*SwapResponse, error) {
	ctx := context.Background()
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		eff, err := s.effectiveConfig(ctx)
		if err != nil {
			return nil, err
		}
		resp, err := s.getSwapDataOnce(ctx, eff, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !s.shouldRetryWithNextOKXConfig(eff, err, attempt) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (s *OKXDexService) getSwapDataOnce(ctx context.Context, eff okxpool.EffectiveConfig, req SwapRequest) (*SwapResponse, error) {
	endpoint, err := s.swapEndpointForBase(eff.BaseURL, req)
	if err != nil {
		return nil, err
	}

	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX API] 请求 URL: %s", endpoint)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	s.addHeadersWithConfig(httpReq, "", timestamp, eff)

	start := time.Now()
	resp, err := s.httpClient().Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		wrapped := fmt.Errorf("failed to send request: %w", err)
		s.recordOKXFailure(ctx, eff, latency, wrapped)
		return nil, wrapped
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		wrapped := fmt.Errorf("failed to read response: %w", err)
		s.recordOKXFailure(ctx, eff, latency, wrapped)
		return nil, wrapped
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		wrapped := fmt.Errorf("OKX swap http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		s.recordOKXFailure(ctx, eff, latency, wrapped)
		return nil, wrapped
	}

	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX API] 响应原始数据: %s", string(body))
	}

	var swapResp SwapResponse
	if err := json.Unmarshal(body, &swapResp); err != nil {
		wrapped := fmt.Errorf("failed to parse response: %w", err)
		s.recordOKXFailure(ctx, eff, latency, wrapped)
		return nil, wrapped
	}

	if swapResp.Code != "0" {
		err := &OKXAPIError{Endpoint: "swap", Code: swapResp.Code, Msg: swapResp.Msg}
		s.recordOKXFailure(ctx, eff, latency, err)
		return nil, err
	}

	s.recordOKXSuccess(ctx, eff, latency)

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
	eff, err := s.effectiveConfig(ctx)
	if err != nil {
		return nil, err
	}
	baseURL := s.marketAPIURLForBase(eff.BaseURL)
	cacheKey := okxAdvancedInfoCacheKey(baseURL, normalized)
	now := time.Now()
	if raw, ok := okxAdvancedInfoCache.Load(cacheKey); ok {
		if entry, ok := raw.(okxAdvancedInfoCacheEntry); ok && entry.expiresAt.After(now) {
			return cloneMarketTokenAdvancedInfoResponse(entry.value), nil
		}
		okxAdvancedInfoCache.Delete(cacheKey)
	}

	fetchedAny, err, _ := okxReadGroup.Do("advanced-info-fetch|"+cacheKey, func() (any, error) {
		var lastErr error
		for attempt := 1; attempt <= 2; attempt++ {
			nextEff := eff
			if attempt > 1 {
				var cfgErr error
				nextEff, cfgErr = s.effectiveConfig(ctx)
				if cfgErr != nil {
					return nil, cfgErr
				}
			}
			nextBaseURL := s.marketAPIURLForBase(nextEff.BaseURL)
			resp, err := s.getMarketTokenAdvancedInfoUncached(ctx, nextEff, nextBaseURL, normalized)
			if err == nil {
				return resp, nil
			}
			lastErr = err
			if !s.shouldRetryWithNextOKXConfig(nextEff, err, attempt) {
				return nil, err
			}
		}
		return nil, lastErr
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

func (s *OKXDexService) getMarketTokenAdvancedInfoUncached(ctx context.Context, eff okxpool.EffectiveConfig, baseURL string, req MarketTokenAdvancedInfoRequest) (*MarketTokenAdvancedInfoResponse, error) {
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
	s.addHeadersWithConfig(httpReq, "", timestamp, eff)

	start := time.Now()
	resp, err := s.httpClient().Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		wrapped := fmt.Errorf("failed to send request: %w", err)
		s.recordOKXFailure(ctx, eff, latency, wrapped)
		return nil, wrapped
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		wrapped := fmt.Errorf("failed to read response: %w", err)
		s.recordOKXFailure(ctx, eff, latency, wrapped)
		return nil, wrapped
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		wrapped := fmt.Errorf("OKX market/token/advanced-info http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		s.recordOKXFailure(ctx, eff, latency, wrapped)
		return nil, wrapped
	}

	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX Market] advanced-info raw response: %s", string(respBody))
	}

	var out MarketTokenAdvancedInfoResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		wrapped := fmt.Errorf("failed to parse response: %w", err)
		s.recordOKXFailure(ctx, eff, latency, wrapped)
		return nil, wrapped
	}
	if out.Code != "0" {
		err := &OKXAPIError{Endpoint: "market/token/advanced-info", Code: out.Code, Msg: out.Msg}
		s.recordOKXFailure(ctx, eff, latency, err)
		return nil, err
	}
	s.recordOKXSuccess(ctx, eff, latency)
	return &out, nil
}

// addHeaders adds authentication headers to the request
func (s *OKXDexService) addHeaders(req *http.Request, body, timestamp string) {
	s.addHeadersWithConfig(req, body, timestamp, s.staticConfig())
}

func (s *OKXDexService) addHeadersWithConfig(req *http.Request, body, timestamp string, eff okxpool.EffectiveConfig) {
	message := timestamp + req.Method + req.URL.RequestURI() + body
	mac := hmac.New(sha256.New, []byte(eff.SecretKey))
	mac.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req.Header.Set("OK-ACCESS-KEY", eff.APIKey)
	req.Header.Set("OK-ACCESS-SIGN", signature)
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", eff.Passphrase)
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
	eff, err := s.effectiveConfig(context.Background())
	if err != nil {
		return "", err
	}
	cacheKey := okxApproveSpenderCacheKey(eff.BaseURL, chainID, tokenAddress)
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
		var lastErr error
		for attempt := 1; attempt <= 2; attempt++ {
			nextEff := eff
			if attempt > 1 {
				var cfgErr error
				nextEff, cfgErr = s.effectiveConfig(context.Background())
				if cfgErr != nil {
					return "", cfgErr
				}
			}
			spender, err := s.getApproveSpenderUncached(nextEff, chainID, tokenAddress)
			if err == nil {
				return spender, nil
			}
			lastErr = err
			if !s.shouldRetryWithNextOKXConfig(nextEff, err, attempt) {
				return "", err
			}
		}
		return "", lastErr
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

func (s *OKXDexService) getApproveSpenderUncached(eff okxpool.EffectiveConfig, chainID string, tokenAddress string) (string, error) {
	url := fmt.Sprintf("%s/approve-transaction?%s=%s&tokenContractAddress=%s&approveAmount=1",
		eff.BaseURL, s.chainQueryKeyForBase(eff.BaseURL), chainID, tokenAddress)

	ctx := context.Background()
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	s.addHeadersWithConfig(httpReq, "", timestamp, eff)

	start := time.Now()
	resp, err := s.httpClient().Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		wrapped := fmt.Errorf("failed to send request: %w", err)
		s.recordOKXFailure(ctx, eff, latency, wrapped)
		return "", wrapped
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		wrapped := fmt.Errorf("failed to read response: %w", err)
		s.recordOKXFailure(ctx, eff, latency, wrapped)
		return "", wrapped
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		wrapped := fmt.Errorf("OKX approve-transaction http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		s.recordOKXFailure(ctx, eff, latency, wrapped)
		return "", wrapped
	}

	var approveResp ApproveTransactionResponse
	if err := json.Unmarshal(body, &approveResp); err != nil {
		wrapped := fmt.Errorf("failed to parse response: %w", err)
		s.recordOKXFailure(ctx, eff, latency, wrapped)
		return "", wrapped
	}

	if approveResp.Code != "0" {
		err := &OKXAPIError{Endpoint: "approve-transaction", Code: approveResp.Code, Msg: approveResp.Msg}
		s.recordOKXFailure(ctx, eff, latency, err)
		return "", err
	}

	if len(approveResp.Data) == 0 || approveResp.Data[0].DexContractAddress == "" {
		err := fmt.Errorf("OKX API returned empty approve address")
		s.recordOKXFailure(ctx, eff, latency, err)
		return "", err
	}

	s.recordOKXSuccess(ctx, eff, latency)
	return approveResp.Data[0].DexContractAddress, nil
}
