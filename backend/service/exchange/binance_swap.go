package exchange

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"TgLpBot/base/config"
)

const (
	binanceWeb3DefaultAPIURL      = "https://web3.binance.com"
	binanceWeb3DefaultQuotePath   = "/build/api/v1/dex/aggregator/quote"
	binanceWeb3DefaultSwapPath    = "/build/api/v1/dex/aggregator/swap"
	binanceWeb3DefaultRecvWindow  = int64(5000)
	binanceWeb3MaxResponseBodyLen = 4 << 20
)

type BinanceSwapService struct {
	apiURL     string
	apiKey     string
	secretKey  string
	quotePath  string
	swapPath   string
	recvWindow int64
	client     *http.Client
}

type BinanceQuoteRequest struct {
	BinanceChainID    string
	Amount            string
	FromTokenAddress  string
	ToTokenAddress    string
	UserWalletAddress string
}

type BinanceBuildSwapRequest struct {
	BinanceChainID               string
	Amount                       string
	FromTokenAddress             string
	ToTokenAddress               string
	UserWalletAddress            string
	QuoteID                      string
	SlippagePercent              string
	ApproveTransaction           string
	ApproveAmount                string
	GasLimit                     string
	GasLevel                     string
	PriceImpactProtectionPercent string
	AutoSlippage                 string
}

type BinanceQuoteToken struct {
	TokenContractAddress string `json:"tokenContractAddress"`
	TokenSymbol          string `json:"tokenSymbol"`
	TokenUnitPrice       string `json:"tokenUnitPrice"`
	Decimal              string `json:"decimal"`
	IsHoneyPot           bool   `json:"isHoneyPot"`
	TaxRate              string `json:"taxRate"`
}

type BinanceRouteToken struct {
	TokenContractAddress string `json:"tokenContractAddress"`
	TokenSymbol          string `json:"tokenSymbol"`
}

type BinanceDexProtocol struct {
	DexName     string `json:"dexName"`
	DexProtocol string `json:"dexProtocol"`
	Percent     string `json:"percent"`
}

type BinanceDexRoute struct {
	DexProtocol    BinanceDexProtocol `json:"dexProtocol"`
	FromToken      BinanceRouteToken  `json:"fromToken"`
	FromTokenIndex string             `json:"fromTokenIndex"`
	ToToken        BinanceRouteToken  `json:"toToken"`
	ToTokenIndex   string             `json:"toTokenIndex"`
}

type BinanceQuoteRoute struct {
	QuoteID            string            `json:"quoteId"`
	VendorName         string            `json:"vendorName"`
	BinanceChainID     string            `json:"binanceChainId"`
	FromTokenAmount    string            `json:"fromTokenAmount"`
	ToTokenAmount      string            `json:"toTokenAmount"`
	TradeFee           string            `json:"tradeFee"`
	EstimateGasFee     string            `json:"estimateGasFee"`
	PriceImpactPercent string            `json:"priceImpactPercent"`
	Router             string            `json:"router"`
	FromToken          BinanceQuoteToken `json:"fromToken"`
	ToToken            BinanceQuoteToken `json:"toToken"`
	DexRouterList      []BinanceDexRoute `json:"dexRouterList"`
	ExecutionMode      string            `json:"executionMode"`
	ApproveTarget      string            `json:"approveTarget"`
	IsBest             bool              `json:"isBest"`
}

type BinanceQuoteResponse struct {
	Code      int                 `json:"code"`
	Msg       string              `json:"msg"`
	Data      []BinanceQuoteRoute `json:"data"`
	Timestamp int64               `json:"timestamp"`
	Success   bool                `json:"success"`
}

type BinanceSwapTx struct {
	From                 string   `json:"from"`
	To                   string   `json:"to"`
	Data                 string   `json:"data"`
	Value                string   `json:"value"`
	Gas                  string   `json:"gas"`
	GasPrice             string   `json:"gasPrice"`
	MaxPriorityFeePerGas string   `json:"maxPriorityFeePerGas"`
	MinReceiveAmount     string   `json:"minReceiveAmount"`
	SlippagePercent      string   `json:"slippagePercent"`
	SignatureData        []string `json:"signatureData"`
	ComputeUnitPrice     string   `json:"computeUnitPrice"`
	ComputeUnitLimit     string   `json:"computeUnitLimit"`
}

type BinanceRFQInfo struct {
	Taker       string          `json:"taker"`
	Maker       string          `json:"maker"`
	QuoteID     string          `json:"quoteId"`
	DataToSign  json.RawMessage `json:"dataToSign"`
	UserSig     string          `json:"userSig"`
	Permit2Data json.RawMessage `json:"permit2Data"`
}

type BinanceBuildSwapData struct {
	RouterResult  BinanceQuoteRoute `json:"routerResult"`
	Tx            BinanceSwapTx     `json:"tx"`
	ExecutionMode string            `json:"executionMode"`
	RFQ           BinanceRFQInfo    `json:"rfq"`
}

type BinanceBuildSwapResponse struct {
	Code      int                  `json:"code"`
	Msg       string               `json:"msg"`
	Data      BinanceBuildSwapData `json:"data"`
	Timestamp int64                `json:"timestamp"`
	Success   bool                 `json:"success"`
}

type BinanceAPIError struct {
	Endpoint string
	Code     int
	Msg      string
}

func (e *BinanceAPIError) Error() string {
	if e == nil {
		return "Binance Web3 API error"
	}
	msg := strings.TrimSpace(e.Msg)
	if msg == "" {
		msg = "request failed"
	}
	endpoint := strings.TrimSpace(e.Endpoint)
	if endpoint == "" {
		return fmt.Sprintf("Binance Web3 API error: %s (code=%d)", msg, e.Code)
	}
	return fmt.Sprintf("Binance Web3 API error: %s (code=%d endpoint=%s)", msg, e.Code, endpoint)
}

func NewBinanceSwapService() *BinanceSwapService {
	svc := &BinanceSwapService{
		apiURL:     binanceWeb3DefaultAPIURL,
		quotePath:  binanceWeb3DefaultQuotePath,
		swapPath:   binanceWeb3DefaultSwapPath,
		recvWindow: binanceWeb3DefaultRecvWindow,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
	if config.AppConfig == nil {
		return svc
	}
	svc.apiURL = strings.TrimRight(strings.TrimSpace(config.AppConfig.BinanceWeb3APIURL), "/")
	svc.apiKey = strings.TrimSpace(config.AppConfig.BinanceWeb3APIKey)
	svc.secretKey = strings.TrimSpace(config.AppConfig.BinanceWeb3SecretKey)
	svc.quotePath = strings.TrimSpace(config.AppConfig.BinanceWeb3QuotePath)
	svc.swapPath = strings.TrimSpace(config.AppConfig.BinanceWeb3BuildTxPath)
	svc.recvWindow = config.AppConfig.BinanceWeb3RecvWindow
	if svc.apiURL == "" {
		svc.apiURL = binanceWeb3DefaultAPIURL
	}
	if svc.quotePath == "" {
		svc.quotePath = binanceWeb3DefaultQuotePath
	}
	if svc.swapPath == "" {
		svc.swapPath = binanceWeb3DefaultSwapPath
	}
	if svc.recvWindow <= 0 {
		svc.recvWindow = binanceWeb3DefaultRecvWindow
	}
	return svc
}

func NewStaticBinanceSwapService(apiURL, apiKey, secretKey, quotePath, swapPath string, recvWindow int64, client *http.Client) *BinanceSwapService {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if strings.TrimSpace(apiURL) == "" {
		apiURL = binanceWeb3DefaultAPIURL
	}
	if strings.TrimSpace(quotePath) == "" {
		quotePath = binanceWeb3DefaultQuotePath
	}
	if strings.TrimSpace(swapPath) == "" {
		swapPath = binanceWeb3DefaultSwapPath
	}
	if recvWindow <= 0 {
		recvWindow = binanceWeb3DefaultRecvWindow
	}
	return &BinanceSwapService{
		apiURL:     strings.TrimRight(strings.TrimSpace(apiURL), "/"),
		apiKey:     strings.TrimSpace(apiKey),
		secretKey:  strings.TrimSpace(secretKey),
		quotePath:  strings.TrimSpace(quotePath),
		swapPath:   strings.TrimSpace(swapPath),
		recvWindow: recvWindow,
		client:     client,
	}
}

func (s *BinanceSwapService) GetAggregatedQuote(req BinanceQuoteRequest) (*BinanceQuoteResponse, error) {
	return s.GetAggregatedQuoteWithContext(context.Background(), req)
}

func (s *BinanceSwapService) GetAggregatedQuoteWithContext(ctx context.Context, req BinanceQuoteRequest) (*BinanceQuoteResponse, error) {
	if err := validateBinanceQuoteRequest(req); err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("binanceChainId", strings.TrimSpace(req.BinanceChainID))
	query.Set("amount", strings.TrimSpace(req.Amount))
	query.Set("fromTokenAddress", strings.TrimSpace(req.FromTokenAddress))
	query.Set("toTokenAddress", strings.TrimSpace(req.ToTokenAddress))
	if wallet := strings.TrimSpace(req.UserWalletAddress); wallet != "" {
		query.Set("userWalletAddress", wallet)
	}

	var out BinanceQuoteResponse
	if err := s.doSignedGET(ctx, s.quotePath, query, &out); err != nil {
		return nil, err
	}
	if out.Code != 0 || !out.Success {
		return nil, &BinanceAPIError{Endpoint: "aggregator/quote", Code: out.Code, Msg: out.Msg}
	}
	return &out, nil
}

func (s *BinanceSwapService) BuildSwapTransaction(req BinanceBuildSwapRequest) (*BinanceBuildSwapResponse, error) {
	return s.BuildSwapTransactionWithContext(context.Background(), req)
}

func (s *BinanceSwapService) BuildSwapTransactionWithContext(ctx context.Context, req BinanceBuildSwapRequest) (*BinanceBuildSwapResponse, error) {
	if err := validateBinanceBuildSwapRequest(req); err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("binanceChainId", strings.TrimSpace(req.BinanceChainID))
	query.Set("amount", strings.TrimSpace(req.Amount))
	query.Set("fromTokenAddress", strings.TrimSpace(req.FromTokenAddress))
	query.Set("toTokenAddress", strings.TrimSpace(req.ToTokenAddress))
	query.Set("userWalletAddress", strings.TrimSpace(req.UserWalletAddress))
	query.Set("quoteId", strings.TrimSpace(req.QuoteID))
	binanceSetOptionalQuery(query, "slippagePercent", req.SlippagePercent)
	binanceSetOptionalQuery(query, "approveTransaction", req.ApproveTransaction)
	binanceSetOptionalQuery(query, "approveAmount", req.ApproveAmount)
	binanceSetOptionalQuery(query, "gasLimit", req.GasLimit)
	binanceSetOptionalQuery(query, "gasLevel", req.GasLevel)
	binanceSetOptionalQuery(query, "priceImpactProtectionPercent", req.PriceImpactProtectionPercent)
	binanceSetOptionalQuery(query, "autoSlippage", req.AutoSlippage)

	var out BinanceBuildSwapResponse
	if err := s.doSignedGET(ctx, s.swapPath, query, &out); err != nil {
		return nil, err
	}
	if out.Code != 0 || !out.Success {
		return nil, &BinanceAPIError{Endpoint: "aggregator/swap", Code: out.Code, Msg: out.Msg}
	}
	return &out, nil
}

func validateBinanceQuoteRequest(req BinanceQuoteRequest) error {
	if strings.TrimSpace(req.BinanceChainID) == "" {
		return fmt.Errorf("Binance Web3 binanceChainId is required")
	}
	if strings.TrimSpace(req.Amount) == "" {
		return fmt.Errorf("Binance Web3 amount is required")
	}
	if strings.TrimSpace(req.FromTokenAddress) == "" || strings.TrimSpace(req.ToTokenAddress) == "" {
		return fmt.Errorf("Binance Web3 fromTokenAddress/toTokenAddress is required")
	}
	return nil
}

func validateBinanceBuildSwapRequest(req BinanceBuildSwapRequest) error {
	if err := validateBinanceQuoteRequest(BinanceQuoteRequest{
		BinanceChainID:   req.BinanceChainID,
		Amount:           req.Amount,
		FromTokenAddress: req.FromTokenAddress,
		ToTokenAddress:   req.ToTokenAddress,
	}); err != nil {
		return err
	}
	if strings.TrimSpace(req.UserWalletAddress) == "" {
		return fmt.Errorf("Binance Web3 userWalletAddress is required")
	}
	if strings.TrimSpace(req.QuoteID) == "" {
		return fmt.Errorf("Binance Web3 quoteId is required")
	}
	autoSlippage := strings.EqualFold(strings.TrimSpace(req.AutoSlippage), "true")
	if strings.TrimSpace(req.SlippagePercent) == "" && !autoSlippage {
		return fmt.Errorf("Binance Web3 slippagePercent is required unless autoSlippage=true")
	}
	return nil
}

func binanceSetOptionalQuery(query url.Values, key, value string) {
	if strings.TrimSpace(value) != "" {
		query.Set(key, strings.TrimSpace(value))
	}
}

func (s *BinanceSwapService) doSignedGET(ctx context.Context, path string, query url.Values, out any) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}
	if err := s.ensureCredentials(); err != nil {
		return err
	}
	endpoint, requestPath, err := s.endpoint(path, query)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create Binance Web3 request: %w", err)
	}
	s.addSignedHeaders(httpReq, http.MethodGet, requestPath, "")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := s.httpClient().Do(httpReq)
	if err != nil {
		return fmt.Errorf("Binance Web3 request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, binanceWeb3MaxResponseBodyLen))
	if err != nil {
		return fmt.Errorf("failed to read Binance Web3 response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Binance Web3 http %d: %s", resp.StatusCode, binanceHTTPErrorMessage(body, resp.StatusCode))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("failed to parse Binance Web3 response: %w", err)
	}
	return nil
}

func (s *BinanceSwapService) endpoint(path string, query url.Values) (string, string, error) {
	baseRaw := strings.TrimSpace(s.apiURL)
	if baseRaw == "" {
		baseRaw = binanceWeb3DefaultAPIURL
	}
	base, err := url.Parse(strings.TrimRight(baseRaw, "/"))
	if err != nil {
		return "", "", fmt.Errorf("invalid Binance Web3 API URL: %w", err)
	}
	if base.Scheme == "" || base.Host == "" {
		return "", "", fmt.Errorf("invalid Binance Web3 API URL: missing scheme or host")
	}
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return "", "", fmt.Errorf("Binance Web3 request path is required")
	}
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}

	basePath := strings.TrimRight(base.Path, "/")
	fullPath := cleanPath
	if basePath != "" && basePath != "/" && !strings.HasPrefix(cleanPath, basePath+"/") && cleanPath != basePath {
		fullPath = basePath + cleanPath
	}

	rawQuery := query.Encode()
	base.Path = fullPath
	base.RawPath = ""
	base.RawQuery = rawQuery
	base.Fragment = ""

	requestPath := base.EscapedPath()
	if rawQuery != "" {
		requestPath += "?" + rawQuery
	}
	return base.String(), requestPath, nil
}

func (s *BinanceSwapService) addSignedHeaders(req *http.Request, method, requestPath, body string) {
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	prehash := timestamp + method + requestPath + body
	mac := hmac.New(sha256.New, []byte(s.secretKey))
	_, _ = mac.Write([]byte(prehash))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req.Header.Set("X-OC-APIKEY", s.apiKey)
	req.Header.Set("X-OC-TIMESTAMP", timestamp)
	req.Header.Set("X-OC-SIGN", signature)
	if s.recvWindow > 0 {
		req.Header.Set("X-OC-RECV-WINDOW", strconv.FormatInt(s.recvWindow, 10))
	}
}

func (s *BinanceSwapService) ensureCredentials() error {
	if s == nil {
		return fmt.Errorf("Binance Web3 service is nil")
	}
	if strings.TrimSpace(s.apiKey) == "" {
		return fmt.Errorf("Binance Web3 API key is required")
	}
	if strings.TrimSpace(s.secretKey) == "" {
		return fmt.Errorf("Binance Web3 secret key is required")
	}
	return nil
}

func (s *BinanceSwapService) httpClient() *http.Client {
	if s != nil && s.client != nil {
		return s.client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func binanceHTTPErrorMessage(body []byte, status int) string {
	var parsed struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if strings.TrimSpace(parsed.Msg) != "" {
			return fmt.Sprintf("%s (code=%d)", strings.TrimSpace(parsed.Msg), parsed.Code)
		}
		if strings.TrimSpace(parsed.Message) != "" {
			return fmt.Sprintf("%s (code=%d)", strings.TrimSpace(parsed.Message), parsed.Code)
		}
	}
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return fmt.Sprintf("HTTP %d", status)
	}
	return msg
}
