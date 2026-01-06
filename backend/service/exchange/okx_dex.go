package exchange

import (
	"TgLpBot/base/config"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
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
	ChainID           string `json:"chainId"`
	FromTokenAddress  string `json:"fromTokenAddress"`
	ToTokenAddress    string `json:"toTokenAddress"`
	Amount            string `json:"amount"`
	Slippage          string `json:"slippage"`
	UserWalletAddress string `json:"userWalletAddress"`
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
			FromTokenAmount string `json:"fromTokenAmount"`
			ToTokenAmount   string `json:"toTokenAmount"`
		} `json:"routerResult"`
	} `json:"data"`
}

type OKXAPIError struct {
	Endpoint string
	Code     string
	Msg      string
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

// GetSwapData gets swap transaction data
func (s *OKXDexService) GetSwapData(req SwapRequest) (*SwapResponse, error) {
	url := fmt.Sprintf("%s/swap?%s=%s&fromTokenAddress=%s&toTokenAddress=%s&amount=%s&%s=%s&userWalletAddress=%s",
		s.apiURL, s.chainQueryKey(), req.ChainID, req.FromTokenAddress, req.ToTokenAddress, req.Amount, s.slippageQueryKey(), s.slippageQueryValue(req.Slippage), req.UserWalletAddress)

	if config.AppConfig != nil && config.AppConfig.OKXDebug {
		log.Printf("[OKX API] 请求 URL: %s", url)
	}

	httpReq, err := http.NewRequest("GET", url, nil)
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
