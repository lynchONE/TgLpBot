package web_server

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/exchange"
	"TgLpBot/service/liquidity"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// --- Single Token Swap (任意代币兑换) ---

type walletSwapSingleRequest struct {
	InitData        string  `json:"initData"`
	Chain           string  `json:"chain,omitempty"`
	WalletID        uint    `json:"wallet_id,omitempty"`
	FromToken       string  `json:"from_token"`
	ToToken         string  `json:"to_token"`
	Amount          string  `json:"amount"` // raw amount (wei)
	SlippagePercent float64 `json:"slippage_percent,omitempty"`
	Action          string  `json:"action"` // "quote" or "swap"
}

type swapQuoteResponse struct {
	OK            bool   `json:"ok"`
	Chain         string `json:"chain"`
	FromToken     string `json:"from_token"`
	ToToken       string `json:"to_token"`
	FromAmount    string `json:"from_amount"`
	ToAmount      string `json:"to_amount"`
	ToAmountFloat string `json:"to_amount_float"`
	EstimatedGas  string `json:"estimated_gas,omitempty"`
}

type swapExecuteResponse struct {
	OK        bool   `json:"ok"`
	Chain     string `json:"chain"`
	TxHash    string `json:"tx_hash"`
	FromToken string `json:"from_token"`
	ToToken   string `json:"to_token"`
	Message   string `json:"message,omitempty"`
}

func (s *Server) handleWalletSwapSingle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req walletSwapSingleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "无效的 JSON 请求体", http.StatusBadRequest)
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
		http.Error(w, "加载配置失败", http.StatusInternalServerError)
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
		http.Error(w, "无效的代币地址", http.StatusBadRequest)
		return
	}
	fromToken := common.HexToAddress(fromTokenStr)
	toToken := common.HexToAddress(toTokenStr)

	amountStr := strings.TrimSpace(req.Amount)
	if amountStr == "" {
		http.Error(w, "缺少兑换数量", http.StatusBadRequest)
		return
	}

	exec, err := chainexec.GetEVM(chain)
	if err != nil {
		http.Error(w, "链初始化失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	cc := exec.Config()
	client := exec.Client()

	if client == nil {
		http.Error(w, "链节点初始化失败", http.StatusInternalServerError)
		return
	}

	// Fetch token decimals
	decimals := uint8(18)
	if erc20, err := blockchain.NewERC20(fromToken, client); err == nil {
		if d, err := erc20.Decimals(nil); err == nil {
			decimals = d
		}
	}

	// Parse human readable amount to wei
	amountFloat, ok := new(big.Float).SetString(amountStr)
	if !ok {
		http.Error(w, "无效的兑换数量", http.StatusBadRequest)
		return
	}
	multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	amountFloat.Mul(amountFloat, multiplier)
	amount := new(big.Int)
	amountFloat.Int(amount)

	if amount.Sign() <= 0 {
		http.Error(w, "兑换数量必须大于0", http.StatusBadRequest)
		return
	}

	// Get wallet
	walletService := wallet.NewWalletService()
	var wlt *models.Wallet
	if req.WalletID > 0 {
		wlt, err = walletService.GetWalletByID(user.ID, req.WalletID)
		if err != nil {
			http.Error(w, "获取钱包失败: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		wlt, err = walletService.GetDefaultWallet(user.ID)
		if err != nil {
			http.Error(w, "获取默认钱包失败: "+err.Error(), http.StatusBadRequest)
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

	switch action {
	case "quote":
		swapReq := exchange.SwapRequest{
			ChainID:           chainIDStr,
			FromTokenAddress:  fromToken.Hex(),
			ToTokenAddress:    toToken.Hex(),
			Amount:            amount.String(),
			Slippage:          slippageDecimal,
			UserWalletAddress: walletAddr.Hex(),
		}
		resp, err := okxService.GetSwapData(swapReq)
		if err != nil {
			http.Error(w, "获取报价失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if resp == nil || len(resp.Data) == 0 {
			http.Error(w, "无法获取兑换报价", http.StatusInternalServerError)
			return
		}

		toAmount := strings.TrimSpace(resp.Data[0].RouterResult.ToTokenAmount)
		estGas := strings.TrimSpace(resp.Data[0].Tx.Gas)

		// Calculate float display
		toAmountFloat := ""
		if toBI, parseOK := new(big.Int).SetString(toAmount, 10); parseOK {
			toAmountFloat = fmt.Sprintf("%.6f", amountToFloat(toBI.String(), 18))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(swapQuoteResponse{
			OK:            true,
			Chain:         chain,
			FromToken:     fromToken.Hex(),
			ToToken:       toToken.Hex(),
			FromAmount:    amount.String(),
			ToAmount:      toAmount,
			ToAmountFloat: toAmountFloat,
			EstimatedGas:  estGas,
		})

	case "swap":
		pkHex, err := walletService.GetPrivateKey(wlt)
		if err != nil {
			http.Error(w, "获取私钥失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		privateKey, err := crypto.HexToECDSA(pkHex)
		if err != nil {
			http.Error(w, "解析私钥失败", http.StatusInternalServerError)
			return
		}

		lpService := liquidity.NewLiquidityService()
		txHash, err := lpService.SwapSingleToken(exec, privateKey, walletAddr, fromToken, toToken, amount, slippage)
		if err != nil {
			http.Error(w, "兑换失败: "+err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "无效的操作，请使用 quote 或 swap", http.StatusBadRequest)
	}
}
