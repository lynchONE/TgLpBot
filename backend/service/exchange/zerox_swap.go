package exchange

import (
	"TgLpBot/base/config"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ZeroXSwapService struct {
	apiURL       string
	apiKey       string
	apiVersion   string
	feeRecipient string
	feeBps       int
	client       *http.Client
}

type ZeroXQuoteRequest struct {
	ChainID     string
	SellToken   string
	BuyToken    string
	SellAmount  string
	Taker       string
	SlippageBps int
}

type ZeroXFee struct {
	Amount    string `json:"amount"`
	Token     string `json:"token"`
	Type      string `json:"type"`
	Recipient string `json:"recipient"`
	Bps       string `json:"bps"`
}

type ZeroXFees struct {
	ZeroExFee      *ZeroXFee  `json:"zeroExFee"`
	IntegratorFee  *ZeroXFee  `json:"integratorFee"`
	IntegratorFees []ZeroXFee `json:"integratorFees"`
	GasFee         *ZeroXFee  `json:"gasFee"`
}

type ZeroXAllowanceIssue struct {
	Actual  string `json:"actual"`
	Spender string `json:"spender"`
}

type ZeroXIssues struct {
	Allowance            *ZeroXAllowanceIssue `json:"allowance"`
	SimulationIncomplete bool                 `json:"simulationIncomplete"`
}

type ZeroXRouteToken struct {
	Address string `json:"address"`
	Symbol  string `json:"symbol"`
	Name    string `json:"name"`
}

type ZeroXRouteFill struct {
	FromTokenAddress string `json:"from"`
	ToTokenAddress   string `json:"to"`
	Source           string `json:"source"`
	ProportionBps    string `json:"proportionBps"`
}

type ZeroXRoute struct {
	Tokens []ZeroXRouteToken `json:"tokens"`
	Fills  []ZeroXRouteFill  `json:"fills"`
}

type ZeroXTransaction struct {
	To       string `json:"to"`
	Data     string `json:"data"`
	Value    string `json:"value"`
	Gas      string `json:"gas"`
	GasPrice string `json:"gasPrice"`
}

type ZeroXQuoteResponse struct {
	AllowanceTarget    string           `json:"allowanceTarget"`
	BuyAmount          string           `json:"buyAmount"`
	BuyToken           string           `json:"buyToken"`
	MinBuyAmount       string           `json:"minBuyAmount"`
	SellAmount         string           `json:"sellAmount"`
	SellToken          string           `json:"sellToken"`
	TotalNetworkFee    string           `json:"totalNetworkFee"`
	LiquidityAvailable bool             `json:"liquidityAvailable"`
	Fees               ZeroXFees        `json:"fees"`
	Issues             ZeroXIssues      `json:"issues"`
	Route              ZeroXRoute       `json:"route"`
	Transaction        ZeroXTransaction `json:"transaction"`
	Zid                string           `json:"zid"`
}

func NewZeroXSwapService() *ZeroXSwapService {
	cfg := config.AppConfig
	service := &ZeroXSwapService{
		apiURL:     "https://api.0x.org",
		apiVersion: "v2",
		feeBps:     15,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
	if cfg == nil {
		return service
	}
	service.apiURL = strings.TrimRight(strings.TrimSpace(cfg.ZeroXAPIURL), "/")
	if service.apiURL == "" {
		service.apiURL = "https://api.0x.org"
	}
	service.apiKey = strings.TrimSpace(cfg.ZeroXAPIKey)
	service.apiVersion = strings.TrimSpace(cfg.ZeroXAPIVersion)
	if service.apiVersion == "" {
		service.apiVersion = "v2"
	}
	service.feeRecipient = strings.TrimSpace(cfg.ZeroXSwapFeeRecipient)
	if service.feeRecipient == "" {
		service.feeRecipient = strings.TrimSpace(cfg.AdminWalletAddress)
	}
	if cfg.ZeroXSwapFeeBps > 0 {
		service.feeBps = cfg.ZeroXSwapFeeBps
	}
	return service
}

func (s *ZeroXSwapService) GetAllowanceHolderQuote(req ZeroXQuoteRequest) (*ZeroXQuoteResponse, error) {
	if strings.TrimSpace(req.ChainID) == "" {
		return nil, fmt.Errorf("0x chain id is required")
	}
	if strings.TrimSpace(req.SellToken) == "" || strings.TrimSpace(req.BuyToken) == "" {
		return nil, fmt.Errorf("0x sellToken/buyToken is required")
	}
	if strings.TrimSpace(req.SellAmount) == "" {
		return nil, fmt.Errorf("0x sellAmount is required")
	}
	if strings.TrimSpace(req.Taker) == "" {
		return nil, fmt.Errorf("0x taker is required")
	}

	query := url.Values{}
	query.Set("chainId", strings.TrimSpace(req.ChainID))
	query.Set("sellToken", strings.TrimSpace(req.SellToken))
	query.Set("buyToken", strings.TrimSpace(req.BuyToken))
	query.Set("sellAmount", strings.TrimSpace(req.SellAmount))
	query.Set("taker", strings.TrimSpace(req.Taker))
	if req.SlippageBps > 0 {
		query.Set("slippageBps", strconv.Itoa(req.SlippageBps))
	}
	if strings.TrimSpace(s.feeRecipient) != "" && s.feeBps > 0 {
		query.Set("swapFeeRecipient", s.feeRecipient)
		query.Set("swapFeeBps", strconv.Itoa(s.feeBps))
		query.Set("swapFeeToken", strings.TrimSpace(req.BuyToken))
	}

	endpoint := fmt.Sprintf("%s/swap/allowance-holder/quote?%s", s.apiURL, query.Encode())
	httpReq, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create 0x request: %w", err)
	}
	if strings.TrimSpace(s.apiKey) != "" {
		httpReq.Header.Set("0x-api-key", s.apiKey)
	}
	if strings.TrimSpace(s.apiVersion) != "" {
		httpReq.Header.Set("0x-version", s.apiVersion)
	}
	httpReq.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("0x request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read 0x response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("0x quote failed: %s", zeroXErrorMessage(body, resp.StatusCode))
	}

	var out ZeroXQuoteResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("failed to parse 0x response: %w", err)
	}
	if !out.LiquidityAvailable {
		return nil, fmt.Errorf("0x liquidity unavailable")
	}
	return &out, nil
}

func zeroXErrorMessage(body []byte, status int) string {
	type issue struct {
		Field  string `json:"field"`
		Code   int    `json:"code"`
		Reason string `json:"reason"`
	}
	type payload struct {
		Reason           string  `json:"reason"`
		Message          string  `json:"message"`
		ValidationErrors []issue `json:"validationErrors"`
	}
	var parsed payload
	if err := json.Unmarshal(body, &parsed); err == nil {
		if strings.TrimSpace(parsed.Reason) != "" {
			return parsed.Reason
		}
		if strings.TrimSpace(parsed.Message) != "" {
			return parsed.Message
		}
		if len(parsed.ValidationErrors) > 0 && strings.TrimSpace(parsed.ValidationErrors[0].Reason) != "" {
			return parsed.ValidationErrors[0].Reason
		}
	}
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return fmt.Sprintf("HTTP %d", status)
	}
	return msg
}
