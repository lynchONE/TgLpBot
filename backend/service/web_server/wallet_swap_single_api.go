package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/exchange"
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
}

type swapQuoteResponse struct {
	OK                 bool    `json:"ok"`
	Chain              string  `json:"chain"`
	FromToken          string  `json:"from_token"`
	ToToken            string  `json:"to_token"`
	FromAmount         string  `json:"from_amount"`
	ToAmount           string  `json:"to_amount"`
	ToAmountFloat      string  `json:"to_amount_float"`
	EstimatedGas       string  `json:"estimated_gas,omitempty"`
	EstimatedGasNative float64 `json:"estimated_gas_native,omitempty"`
	EstimatedGasUSD    float64 `json:"estimated_gas_usd,omitempty"`
	EstimatedGasSymbol string  `json:"estimated_gas_symbol,omitempty"`
}

type swapExecuteResponse struct {
	OK        bool   `json:"ok"`
	Chain     string `json:"chain"`
	TxHash    string `json:"tx_hash"`
	FromToken string `json:"from_token"`
	ToToken   string `json:"to_token"`
	Message   string `json:"message,omitempty"`
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
	gasLimit, ok := new(big.Int).SetString(strings.TrimSpace(gasLimitRaw), 10)
	if !ok || gasLimit.Sign() <= 0 {
		return 0, 0
	}

	var gasPrice *big.Int
	if parsed, ok := new(big.Int).SetString(strings.TrimSpace(gasPriceRaw), 10); ok && parsed.Sign() > 0 {
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
	if status, msg := requireMiniAppPermission(check); status != 0 {
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
	if disallowNativeDirectSwap := false; disallowNativeDirectSwap && (strings.EqualFold(fromTokenStr, nativePseudoTokenAddress) || strings.EqualFold(toTokenStr, nativePseudoTokenAddress)) {
		http.Error(w, "原生币暂不支持直接兑换，请先换成 WBNB/WETH 后再操作", http.StatusBadRequest)
		return
	}
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

	okxService := exchange.NewOKXDexService()
	chainIDStr := fmt.Sprintf("%d", cc.ChainID)
	action := strings.TrimSpace(strings.ToLower(req.Action))
	okxFromTokenAddress := okxWalletSwapTokenAddress(fromTokenStr, fromToken)
	okxToTokenAddress := okxWalletSwapTokenAddress(toTokenStr, toToken)

	switch action {
	case "quote":
		swapReq := exchange.SwapRequest{
			ChainID:           chainIDStr,
			FromTokenAddress:  okxFromTokenAddress,
			ToTokenAddress:    okxToTokenAddress,
			Amount:            amount.String(),
			Slippage:          slippageDecimal,
			UserWalletAddress: walletAddr.Hex(),
		}
		resp, err := okxService.GetSwapData(swapReq)
		if err != nil {
			http.Error(w, "get quote failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if resp == nil || len(resp.Data) == 0 {
			http.Error(w, "empty quote response", http.StatusInternalServerError)
			return
		}

		toAmount := strings.TrimSpace(resp.Data[0].RouterResult.ToTokenAmount)
		estGas := strings.TrimSpace(resp.Data[0].Tx.Gas)
		estGasPrice := strings.TrimSpace(resp.Data[0].Tx.GasPrice)

		toAmountFloat := ""
		if toBI, parseOK := new(big.Int).SetString(toAmount, 10); parseOK {
			toAmountFloat = fmt.Sprintf("%.6f", amountToFloat(toBI.String(), int(toDecimals)))
		}

		gasNative, gasUSD := estimateGasCosts(chain, estGas, estGasPrice, client)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(swapQuoteResponse{
			OK:                 true,
			Chain:              chain,
			FromToken:          fromToken.Hex(),
			ToToken:            toToken.Hex(),
			FromAmount:         amount.String(),
			ToAmount:           toAmount,
			ToAmountFloat:      toAmountFloat,
			EstimatedGas:       estGas,
			EstimatedGasNative: gasNative,
			EstimatedGasUSD:    gasUSD,
			EstimatedGasSymbol: nativeSymbolForChainConfig(chain, cc),
		})

	case "swap":
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
		txHash, err := lpService.SwapSingleToken(exec, privateKey, walletAddr, fromToken, toToken, amount, slippage)
		if err != nil {
			http.Error(w, "swap failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(swapExecuteResponse{
			OK:        true,
			Chain:     chain,
			TxHash:    txHash,
			FromToken: fromToken.Hex(),
			ToToken:   toToken.Hex(),
			Message:   "兑换交易已提交",
		})

	default:
		http.Error(w, "invalid action, use quote or swap", http.StatusBadRequest)
	}
}
