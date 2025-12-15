package services

import (
	"TgLpBot/config"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// QuoteRequest represents a quote request
type QuoteRequest struct {
	ChainID        string `json:"chainId"`
	FromTokenAddress string `json:"fromTokenAddress"`
	ToTokenAddress   string `json:"toTokenAddress"`
	Amount           string `json:"amount"`
	Slippage         string `json:"slippage"`
}

// QuoteResponse represents a quote response
type QuoteResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data []struct {
		RouterResult struct {
			FromToken struct {
				TokenContractAddress string `json:"tokenContractAddress"`
				Decimal              string `json:"decimal"`
				Symbol               string `json:"symbol"`
			} `json:"fromToken"`
			ToToken struct {
				TokenContractAddress string `json:"tokenContractAddress"`
				Decimal              string `json:"decimal"`
				Symbol               string `json:"symbol"`
			} `json:"toToken"`
			FromTokenAmount string `json:"fromTokenAmount"`
			ToTokenAmount   string `json:"toTokenAmount"`
			EstimatedGas    string `json:"estimatedGas"`
			DexRouterList   []struct {
				Router         string `json:"router"`
				RouterPercent  string `json:"routerPercent"`
				SubRouterList  []struct {
					FromToken struct {
						TokenContractAddress string `json:"tokenContractAddress"`
						Symbol               string `json:"symbol"`
					} `json:"fromToken"`
					ToToken struct {
						TokenContractAddress string `json:"tokenContractAddress"`
						Symbol               string `json:"symbol"`
					} `json:"toToken"`
					DexProtocol []struct {
						DexName string `json:"dexName"`
						Percent string `json:"percent"`
					} `json:"dexProtocol"`
				} `json:"subRouterList"`
			} `json:"dexRouterList"`
		} `json:"routerResult"`
		Tx struct {
			From     string `json:"from"`
			To       string `json:"to"`
			Data     string `json:"data"`
			Value    string `json:"value"`
			Gas      string `json:"gas"`
			GasPrice string `json:"gasPrice"`
		} `json:"tx"`
	} `json:"data"`
}

// SwapRequest represents a swap request
type SwapRequest struct {
	ChainID          string `json:"chainId"`
	FromTokenAddress string `json:"fromTokenAddress"`
	ToTokenAddress   string `json:"toTokenAddress"`
	Amount           string `json:"amount"`
	Slippage         string `json:"slippage"`
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

// GetQuote gets a quote for token swap
func (s *OKXDexService) GetQuote(req QuoteRequest) (*QuoteResponse, error) {
	url := fmt.Sprintf("%s/quote?chainId=%s&fromTokenAddress=%s&toTokenAddress=%s&amount=%s&slippage=%s",
		s.apiURL, req.ChainID, req.FromTokenAddress, req.ToTokenAddress, req.Amount, req.Slippage)
	
	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Add headers
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	s.addHeaders(httpReq, "GET", "/api/v5/dex/aggregator/quote", "", timestamp)
	
	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	var quoteResp QuoteResponse
	if err := json.Unmarshal(body, &quoteResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	if quoteResp.Code != "0" {
		return nil, fmt.Errorf("OKX API error: %s", quoteResp.Msg)
	}
	
	return &quoteResp, nil
}

// GetSwapData gets swap transaction data
func (s *OKXDexService) GetSwapData(req SwapRequest) (*SwapResponse, error) {
	url := fmt.Sprintf("%s/swap?chainId=%s&fromTokenAddress=%s&toTokenAddress=%s&amount=%s&slippage=%s&userWalletAddress=%s",
		s.apiURL, req.ChainID, req.FromTokenAddress, req.ToTokenAddress, req.Amount, req.Slippage, req.UserWalletAddress)
	
	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Add headers
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	s.addHeaders(httpReq, "GET", "/api/v5/dex/aggregator/swap", "", timestamp)
	
	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	var swapResp SwapResponse
	if err := json.Unmarshal(body, &swapResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	if swapResp.Code != "0" {
		return nil, fmt.Errorf("OKX API error: %s", swapResp.Msg)
	}
	
	return &swapResp, nil
}

// addHeaders adds authentication headers to the request
func (s *OKXDexService) addHeaders(req *http.Request, method, path, body, timestamp string) {
	message := timestamp + method + path + body
	mac := hmac.New(sha256.New, []byte(s.secretKey))
	mac.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	
	req.Header.Set("OK-ACCESS-KEY", s.apiKey)
	req.Header.Set("OK-ACCESS-SIGN", signature)
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", s.passphrase)
	req.Header.Set("Content-Type", "application/json")
}

