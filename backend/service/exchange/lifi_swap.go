package exchange

import (
	"TgLpBot/base/config"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const lifiNativeZeroAddress = "0x0000000000000000000000000000000000000000"
const lifiNativePseudoAddress = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

type LIFISwapService struct {
	apiURL     string
	apiKey     string
	integrator string
	feePercent float64
	client     *http.Client
}

type LIFIQuoteRequest struct {
	FromChainID string
	ToChainID   string
	FromToken   string
	ToToken     string
	FromAmount  string
	FromAddress string
	ToAddress   string
	Slippage    float64
}

type LIFIToken struct {
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Decimals int    `json:"decimals"`
	LogoURI  string `json:"logoURI"`
}

type LIFIFeeCost struct {
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Amount    string    `json:"amount"`
	AmountUSD string    `json:"amountUSD"`
	Token     LIFIToken `json:"token"`
}

type LIFIGasCost struct {
	Type      string    `json:"type"`
	Price     string    `json:"price"`
	Estimate  string    `json:"estimate"`
	Limit     string    `json:"limit"`
	Amount    string    `json:"amount"`
	AmountUSD string    `json:"amountUSD"`
	Token     LIFIToken `json:"token"`
}

type LIFIProtocol struct {
	Name             string `json:"name"`
	Part             int    `json:"part"`
	FromTokenAddress string `json:"fromTokenAddress"`
	ToTokenAddress   string `json:"toTokenAddress"`
}

type LIFIEstimateData struct {
	FromToken       LIFIToken      `json:"fromToken"`
	ToToken         LIFIToken      `json:"toToken"`
	FromTokenAmount string         `json:"fromTokenAmount"`
	ToTokenAmount   string         `json:"toTokenAmount"`
	EstimatedGas    uint64         `json:"estimatedGas"`
	Protocols       []LIFIProtocol `json:"protocols"`
}

type LIFIEstimate struct {
	FromAmount      string           `json:"fromAmount"`
	ToAmount        string           `json:"toAmount"`
	ToAmountMin     string           `json:"toAmountMin"`
	ApprovalAddress string           `json:"approvalAddress"`
	FeeCosts        []LIFIFeeCost    `json:"feeCosts"`
	GasCosts        []LIFIGasCost    `json:"gasCosts"`
	Data            LIFIEstimateData `json:"data"`
}

type LIFIAction struct {
	FromChainID int       `json:"fromChainId"`
	ToChainID   int       `json:"toChainId"`
	FromToken   LIFIToken `json:"fromToken"`
	ToToken     LIFIToken `json:"toToken"`
	FromAmount  string    `json:"fromAmount"`
	Slippage    float64   `json:"slippage"`
	FromAddress string    `json:"fromAddress"`
	ToAddress   string    `json:"toAddress"`
}

type LIFIToolDetails struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type LIFITransactionRequest struct {
	From     string `json:"from"`
	To       string `json:"to"`
	ChainID  int    `json:"chainId"`
	Data     string `json:"data"`
	Value    string `json:"value"`
	GasPrice string `json:"gasPrice"`
	GasLimit string `json:"gasLimit"`
}

type LIFIIncludedStep struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Tool        string          `json:"tool"`
	ToolDetails LIFIToolDetails `json:"toolDetails"`
	Action      LIFIAction      `json:"action"`
	Estimate    LIFIEstimate    `json:"estimate"`
}

type LIFIQuoteResponse struct {
	ID                 string                 `json:"id"`
	Type               string                 `json:"type"`
	Tool               string                 `json:"tool"`
	ToolDetails        LIFIToolDetails        `json:"toolDetails"`
	Action             LIFIAction             `json:"action"`
	Estimate           LIFIEstimate           `json:"estimate"`
	Integrator         string                 `json:"integrator"`
	TransactionRequest LIFITransactionRequest `json:"transactionRequest"`
	IncludedSteps      []LIFIIncludedStep     `json:"includedSteps"`
}

func NewLIFISwapService() *LIFISwapService {
	cfg := config.AppConfig
	service := &LIFISwapService{
		apiURL:     "https://li.quest",
		integrator: "tg-lp-bot",
		feePercent: 0.0025,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
	if cfg == nil {
		return service
	}
	service.apiURL = strings.TrimRight(strings.TrimSpace(cfg.LIFIAPIURL), "/")
	if service.apiURL == "" {
		service.apiURL = "https://li.quest"
	}
	service.apiKey = strings.TrimSpace(cfg.LIFIAPIKey)
	if strings.TrimSpace(cfg.LIFIIntegrator) != "" {
		service.integrator = strings.TrimSpace(cfg.LIFIIntegrator)
	}
	if cfg.LIFIFeePercent > 0 {
		service.feePercent = cfg.LIFIFeePercent
	}
	return service
}

func (s *LIFISwapService) GetQuote(req LIFIQuoteRequest) (*LIFIQuoteResponse, error) {
	if strings.TrimSpace(req.FromChainID) == "" || strings.TrimSpace(req.ToChainID) == "" {
		return nil, fmt.Errorf("LI.FI chain id is required")
	}
	if strings.TrimSpace(req.FromToken) == "" || strings.TrimSpace(req.ToToken) == "" {
		return nil, fmt.Errorf("LI.FI fromToken/toToken is required")
	}
	if strings.TrimSpace(req.FromAmount) == "" {
		return nil, fmt.Errorf("LI.FI fromAmount is required")
	}
	if strings.TrimSpace(req.FromAddress) == "" || strings.TrimSpace(req.ToAddress) == "" {
		return nil, fmt.Errorf("LI.FI from/to address is required")
	}

	query := url.Values{}
	query.Set("fromChain", strings.TrimSpace(req.FromChainID))
	query.Set("toChain", strings.TrimSpace(req.ToChainID))
	query.Set("fromToken", strings.TrimSpace(req.FromToken))
	query.Set("toToken", strings.TrimSpace(req.ToToken))
	query.Set("fromAmount", strings.TrimSpace(req.FromAmount))
	query.Set("fromAddress", strings.TrimSpace(req.FromAddress))
	query.Set("toAddress", strings.TrimSpace(req.ToAddress))
	query.Set("allowBridges", "none")
	query.Set("order", "CHEAPEST")
	if req.Slippage > 0 {
		query.Set("slippage", strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", req.Slippage), "0"), "."))
	}
	if strings.TrimSpace(s.integrator) != "" {
		query.Set("integrator", s.integrator)
	}
	if s.feePercent > 0 {
		query.Set("fee", strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", s.feePercent), "0"), "."))
	}

	endpoint := fmt.Sprintf("%s/v1/quote?%s", s.apiURL, query.Encode())
	httpReq, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create LI.FI request: %w", err)
	}
	if strings.TrimSpace(s.apiKey) != "" {
		httpReq.Header.Set("x-lifi-api-key", s.apiKey)
	}
	httpReq.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("LI.FI request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read LI.FI response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("LI.FI quote failed: %s", lifiErrorMessage(body, resp.StatusCode))
	}

	var out LIFIQuoteResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("failed to parse LI.FI response: %w", err)
	}
	return &out, nil
}

func lifiErrorMessage(body []byte, status int) string {
	type payload struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	var parsed payload
	if err := json.Unmarshal(body, &parsed); err == nil {
		if strings.TrimSpace(parsed.Message) != "" {
			return parsed.Message
		}
		if strings.TrimSpace(parsed.Error) != "" {
			return parsed.Error
		}
	}
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return fmt.Sprintf("HTTP %d", status)
	}
	return msg
}

func LIFINormalizeTokenAddress(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), lifiNativePseudoAddress) {
		return lifiNativeZeroAddress
	}
	return strings.TrimSpace(raw)
}
