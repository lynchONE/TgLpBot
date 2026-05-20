package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/pricing"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type walletSwapSingleRequest struct {
	InitData        string  `json:"initData"`
	Chain           string  `json:"chain,omitempty"`
	WalletID        uint    `json:"wallet_id,omitempty"`
	FromToken       string  `json:"from_token"`
	ToToken         string  `json:"to_token"`
	Amount          string  `json:"amount"`
	SlippagePercent float64 `json:"slippage_percent,omitempty"`
	Action          string  `json:"action"`
	Provider        string  `json:"provider,omitempty"`
}

type swapQuoteResponse struct {
	OK                 bool                `json:"ok"`
	Chain              string              `json:"chain"`
	FromToken          string              `json:"from_token"`
	ToToken            string              `json:"to_token"`
	FromAmount         string              `json:"from_amount"`
	Provider           string              `json:"provider,omitempty"`
	ProviderLabel      string              `json:"provider_label,omitempty"`
	BestProvider       string              `json:"best_provider,omitempty"`
	BestProviderLabel  string              `json:"best_provider_label,omitempty"`
	ToAmount           string              `json:"to_amount,omitempty"`
	ToAmountFloat      string              `json:"to_amount_float,omitempty"`
	MinToAmount        string              `json:"min_to_amount,omitempty"`
	MinToAmountFloat   string              `json:"min_to_amount_float,omitempty"`
	EstimatedGas       string              `json:"estimated_gas,omitempty"`
	EstimatedGasNative float64             `json:"estimated_gas_native,omitempty"`
	EstimatedGasUSD    float64             `json:"estimated_gas_usd,omitempty"`
	EstimatedGasSymbol string              `json:"estimated_gas_symbol,omitempty"`
	AvailableCount     int                 `json:"available_count"`
	Message            string              `json:"message,omitempty"`
	Quotes             []swapProviderQuote `json:"quotes"`
}

type swapExecuteResponse struct {
	OK            bool   `json:"ok"`
	Chain         string `json:"chain"`
	Provider      string `json:"provider,omitempty"`
	ProviderLabel string `json:"provider_label,omitempty"`
	TxHash        string `json:"tx_hash"`
	TxURL         string `json:"tx_url,omitempty"`
	FromToken     string `json:"from_token"`
	ToToken       string `json:"to_token"`
	ToAmount      string `json:"to_amount,omitempty"`
	ToAmountFloat string `json:"to_amount_float,omitempty"`
	CompletedAt   string `json:"completed_at,omitempty"`
	Message       string `json:"message,omitempty"`
}

func tokenDecimals(client *ethclient.Client, token common.Address) uint8 {
	if strings.EqualFold(token.Hex(), nativePseudoTokenAddress) {
		return 18
	}
	decimals := uint8(18)
	erc20, err := blockchain.NewERC20(token, client)
	if err != nil {
		return decimals
	}
	d, err := erc20.Decimals(nil)
	if err != nil {
		return decimals
	}
	return d
}

func okxWalletSwapTokenAddress(raw string, token common.Address) string {
	if strings.EqualFold(strings.TrimSpace(raw), nativePseudoTokenAddress) {
		return nativePseudoTokenAddress
	}
	return token.Hex()
}

func estimateGasCosts(
	chain string,
	gasLimitRaw string,
	gasPriceRaw string,
	suggester interface {
		SuggestGasPrice(ctx context.Context) (*big.Int, error)
	},
) (float64, float64) {
	gasLimit, ok := parseSwapBigInt(gasLimitRaw)
	if !ok || gasLimit == nil || gasLimit.Sign() <= 0 {
		return 0, 0
	}

	var gasPrice *big.Int
	if parsed, ok := parseSwapBigInt(gasPriceRaw); ok && parsed != nil && parsed.Sign() > 0 {
		gasPrice = parsed
	} else if suggester != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if suggested, err := suggester.SuggestGasPrice(ctx); err == nil && suggested != nil && suggested.Sign() > 0 {
			gasPrice = suggested
		}
	}
	if gasPrice == nil || gasPrice.Sign() <= 0 {
		return 0, 0
	}

	gasWei := new(big.Int).Mul(gasLimit, gasPrice)
	native := amountToFloat(gasWei.String(), 18)
	if native <= 0 {
		return 0, 0
	}
	return native, native * pricing.GetNativePriceUSD(chain)
}

func recordWalletSwapTransaction(
	userID uint,
	chain string,
	provider string,
	walletAddr common.Address,
	fromTokenAddress string,
	toTokenAddress string,
	amountIn *big.Int,
	swapResult *liquidity.SwapSingleTokenResult,
) {
	if database.DB == nil || swapResult == nil || strings.TrimSpace(swapResult.TxHash) == "" {
		return
	}

	var existing models.Transaction
	if err := database.DB.Where("tx_hash = ?", strings.TrimSpace(swapResult.TxHash)).First(&existing).Error; err == nil {
		return
	}

	amountInStr := "0"
	if amountIn != nil {
		amountInStr = amountIn.String()
	}
	amountOutStr := "0"
	if swapResult.AmountOut != nil {
		amountOutStr = swapResult.AmountOut.String()
	}
	blockNumber := uint64(0)
	gasUsed := uint64(0)
	if swapResult.Receipt != nil {
		gasUsed = swapResult.Receipt.GasUsed
		if swapResult.Receipt.BlockNumber != nil {
			blockNumber = swapResult.Receipt.BlockNumber.Uint64()
		}
	}

	txRecord := models.Transaction{
		UserID:          userID,
		Chain:           config.NormalizeChain(chain),
		Provider:        strings.TrimSpace(provider),
		TxHash:          strings.TrimSpace(swapResult.TxHash),
		Type:            models.TxTypeSwap,
		Status:          models.TxStatusConfirmed,
		FromAddress:     walletAddr.Hex(),
		ToAddress:       swapResult.RouterAddress.Hex(),
		TokenInAddress:  strings.TrimSpace(fromTokenAddress),
		TokenOutAddress: strings.TrimSpace(toTokenAddress),
		AmountIn:        amountInStr,
		AmountOut:       amountOutStr,
		GasUsed:         gasUsed,
		BlockNumber:     blockNumber,
		CreatedAt:       time.Now(),
	}
	if err := database.DB.Create(&txRecord).Error; err != nil {
		fmt.Printf("[WalletSwap] record transaction failed: %v\n", err)
	}
}

func aggregateSwapProviderQuotes(
	chain string,
	cc config.ChainConfig,
	client *ethclient.Client,
	walletAddr common.Address,
	fromTokenRaw string,
	toTokenRaw string,
	fromToken common.Address,
	toToken common.Address,
	amount *big.Int,
	slippageDecimal string,
	slippagePercent float64,
	toDecimals int,
) []swapProviderQuote {
	type result struct {
		quote swapProviderQuote
	}

	outCh := make(chan result, 3)
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		outCh <- result{quote: buildOKXProviderQuote(chain, cc, client, walletAddr, fromTokenRaw, toTokenRaw, fromToken, toToken, amount, slippageDecimal, slippagePercent, toDecimals)}
	}()
	go func() {
		defer wg.Done()
		outCh <- result{quote: buildZeroXProviderQuote(chain, cc, client, walletAddr, fromToken, toToken, amount, slippagePercent, toDecimals)}
	}()
	go func() {
		defer wg.Done()
		outCh <- result{quote: buildLIFIProviderQuote(chain, cc, walletAddr, fromToken, toToken, amount, slippagePercent, toDecimals)}
	}()

	go func() {
		wg.Wait()
		close(outCh)
	}()

	quotes := make([]swapProviderQuote, 0, 3)
	for item := range outCh {
		quotes = append(quotes, item.quote)
	}
	return quotes
}

func applyBestQuote(resp *swapQuoteResponse, best *swapProviderQuote) {
	if resp == nil || best == nil {
		return
	}
	resp.Provider = best.Provider
	resp.ProviderLabel = best.ProviderLabel
	resp.BestProvider = best.Provider
	resp.BestProviderLabel = best.ProviderLabel
	resp.ToAmount = best.NetToAmount
	resp.ToAmountFloat = best.NetToAmountFloat
	resp.MinToAmount = best.MinToAmount
	resp.MinToAmountFloat = best.MinToAmountFloat
	resp.EstimatedGas = best.EstimatedGas
	resp.EstimatedGasNative = best.EstimatedGasNative
	resp.EstimatedGasUSD = best.EstimatedGasUSD
	resp.EstimatedGasSymbol = best.EstimatedGasSymbol
}

func (s *Server) handleWalletSwapSingle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req walletSwapSingleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if status, msg := requireModulePermission(check, models.AccessModuleSwap); status != 0 {
		http.Error(w, msg, status)
		return
	}

	cfgService := userSvc.NewGlobalConfigService()
	cfg, err := cfgService.GetOrCreate(user.ID)
	if err != nil {
		http.Error(w, "failed to load config", http.StatusInternalServerError)
		return
	}

	chain := strings.TrimSpace(req.Chain)
	if chain == "" {
		if cfg != nil && !cfg.MultiChainEnabled {
			chain = config.PickEnabledChain(cfg.DefaultChain)
		} else {
			chain = config.PickEnabledChain("bsc")
		}
	} else {
		chain = config.NormalizeChain(chain)
	}

	fromTokenStr := strings.TrimSpace(req.FromToken)
	toTokenStr := strings.TrimSpace(req.ToToken)
	if !common.IsHexAddress(fromTokenStr) || !common.IsHexAddress(toTokenStr) {
		http.Error(w, "invalid token address", http.StatusBadRequest)
		return
	}
	fromToken := common.HexToAddress(fromTokenStr)
	toToken := common.HexToAddress(toTokenStr)

	amountStr := strings.TrimSpace(req.Amount)
	if amountStr == "" {
		http.Error(w, "missing amount", http.StatusBadRequest)
		return
	}

	exec, err := chainexec.GetEVM(chain)
	if err != nil {
		http.Error(w, "chain init failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	cc := exec.Config()
	client := exec.Client()
	if client == nil {
		http.Error(w, "rpc client unavailable", http.StatusInternalServerError)
		return
	}

	fromDecimals := tokenDecimals(client, fromToken)
	toDecimals := tokenDecimals(client, toToken)

	amountFloat, ok := new(big.Float).SetString(amountStr)
	if !ok {
		http.Error(w, "invalid amount", http.StatusBadRequest)
		return
	}
	multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromDecimals)), nil))
	amountFloat.Mul(amountFloat, multiplier)
	amount := new(big.Int)
	amountFloat.Int(amount)
	if amount.Sign() <= 0 {
		http.Error(w, "amount must be greater than 0", http.StatusBadRequest)
		return
	}

	walletService := wallet.NewWalletService()
	var wlt *models.Wallet
	if req.WalletID > 0 {
		wlt, err = walletService.GetWalletByID(user.ID, req.WalletID)
		if err != nil {
			http.Error(w, "load wallet failed: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		wlt, err = walletService.GetDefaultWallet(user.ID)
		if err != nil {
			http.Error(w, "load default wallet failed: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	walletAddr := common.HexToAddress(wlt.Address)

	slippage := req.SlippagePercent
	if slippage <= 0 {
		if cfg != nil && cfg.SlippageTolerance > 0 {
			slippage = cfg.SlippageTolerance
		} else {
			slippage = 1.0
		}
	}
	slippageDecimal := fmt.Sprintf("%.4f", slippage/100)

	action := strings.TrimSpace(strings.ToLower(req.Action))
	switch action {
	case "quote":
		quotes := aggregateSwapProviderQuotes(
			chain,
			cc,
			client,
			walletAddr,
			fromTokenStr,
			toTokenStr,
			fromToken,
			toToken,
			amount,
			slippageDecimal,
			slippage,
			int(toDecimals),
		)
		quotes, best := normalizeProviderQuotes(quotes)
		availableCount := 0
		for _, quote := range quotes {
			if quote.Status == "available" {
				availableCount++
			}
		}

		resp := swapQuoteResponse{
			OK:             true,
			Chain:          chain,
			FromToken:      fromTokenStr,
			ToToken:        toTokenStr,
			FromAmount:     amount.String(),
			AvailableCount: availableCount,
			Quotes:         quotes,
		}
		if best != nil {
			applyBestQuote(&resp, best)
		} else {
			resp.Message = "暂无可用报价，请稍后重试或切换代币"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)

	case "swap":
		provider := strings.ToLower(strings.TrimSpace(req.Provider))
		if provider == "lifi" {
			provider = "li.fi"
		}
		if provider == "" {
			quotes := aggregateSwapProviderQuotes(
				chain,
				cc,
				client,
				walletAddr,
				fromTokenStr,
				toTokenStr,
				fromToken,
				toToken,
				amount,
				slippageDecimal,
				slippage,
				int(toDecimals),
			)
			_, best := normalizeProviderQuotes(quotes)
			if best == nil || best.Status != "available" || strings.TrimSpace(best.Provider) == "" {
				http.Error(w, "swap failed: no executable swap provider available", http.StatusBadRequest)
				return
			}
			provider = strings.ToLower(strings.TrimSpace(best.Provider))
			if provider == "lifi" {
				provider = "li.fi"
			}
		}

		pkHex, err := walletService.GetPrivateKey(wlt)
		if err != nil {
			http.Error(w, "load private key failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		privateKey, err := crypto.HexToECDSA(pkHex)
		if err != nil {
			http.Error(w, "invalid private key", http.StatusInternalServerError)
			return
		}

		lpService := liquidity.NewLiquidityService()
		swapResult, err := lpService.SwapSingleTokenDetailedByProvider(provider, exec, privateKey, walletAddr, fromToken, toToken, amount, slippage)
		if err != nil {
			http.Error(w, "swap failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		recordWalletSwapTransaction(user.ID, chain, provider, walletAddr, fromTokenStr, toTokenStr, amount, swapResult)

		txHash := ""
		toAmount := ""
		toAmountFloat := ""
		if swapResult != nil {
			txHash = strings.TrimSpace(swapResult.TxHash)
			if swapResult.AmountOut != nil && swapResult.AmountOut.Sign() > 0 {
				toAmount = swapResult.AmountOut.String()
				toAmountFloat = fmt.Sprintf("%.6f", amountToFloat(toAmount, int(toDecimals)))
			}
			if strings.TrimSpace(swapResult.Provider) != "" {
				provider = strings.TrimSpace(swapResult.Provider)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(swapExecuteResponse{
			OK:            true,
			Chain:         chain,
			Provider:      provider,
			ProviderLabel: swapProviderLabel(provider),
			TxHash:        txHash,
			TxURL:         explorerTxURLHelper(chain, txHash),
			FromToken:     fromTokenStr,
			ToToken:       toTokenStr,
			ToAmount:      toAmount,
			ToAmountFloat: toAmountFloat,
			CompletedAt:   time.Now().Format("2006-01-02 15:04:05"),
			Message:       fmt.Sprintf("已通过 %s 提交兑换", swapProviderLabel(provider)),
		})

	default:
		http.Error(w, "invalid action, use quote or swap", http.StatusBadRequest)
	}
}
