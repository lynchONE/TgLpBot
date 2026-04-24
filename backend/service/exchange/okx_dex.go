package exchange

import (
	"TgLpBot/base/config"
	"bytes"
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
	"strconv"
	"strings"
	"time"
)

// OKXDexService handles OKX DEX API interactions
type OKXDexService struct {
	apiURL     string
	apiKey     string
	secretKey  string
	passphrase string
	client     *http.Client
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

type MarketTokenBasicInfo struct {
	ChainIndex           string `json:"chainIndex"`
	TokenContractAddress string `json:"tokenContractAddress"`
	TokenSymbol          string `json:"tokenSymbol"`
	TokenName            string `json:"tokenName"`
	TokenLogoURL         string `json:"tokenLogoUrl"`
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

	endpoint := fmt.Sprintf("%s/candles?%s", s.marketAPIURL(), query.Encode())
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

	endpoint := fmt.Sprintf("%s/token/basic-info", s.marketAPIURL())
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
