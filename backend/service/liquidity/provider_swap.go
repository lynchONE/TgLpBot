package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/exchange"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

type providerSwapExecutionRequest struct {
	Provider          string
	Exec              chainexec.EVMExecutor
	PrivateKey        *ecdsa.PrivateKey
	WalletAddr        common.Address
	TokenIn           common.Address
	TokenOut          common.Address
	AmountIn          *big.Int
	ApproveSpender    common.Address
	TxTo              common.Address
	TxData            []byte
	TxValue           *big.Int
	SuggestedGasLimit uint64
	SuggestedGasPrice *big.Int
	ExpectedOut       string
}

func parseProviderBigInt(raw string) (*big.Int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return big.NewInt(0), true
	}
	if strings.HasPrefix(strings.ToLower(raw), "0x") {
		v, ok := new(big.Int).SetString(raw[2:], 16)
		return v, ok
	}
	if v, ok := new(big.Int).SetString(raw, 10); ok {
		return v, true
	}
	v, ok := new(big.Int).SetString(raw, 16)
	return v, ok
}

func parseProviderUint64(raw string) uint64 {
	v, ok := parseProviderBigInt(raw)
	if !ok || v == nil || !v.IsUint64() {
		return 0
	}
	return v.Uint64()
}

func clampSlippageBps(slippagePercent float64) int {
	if slippagePercent <= 0 {
		return 100
	}
	bps := int(math.Round(slippagePercent * 100))
	if bps < 1 {
		return 1
	}
	if bps > 10000 {
		return 10000
	}
	return bps
}

func (s *LiquidityService) executeProviderSwapTx(req providerSwapExecutionRequest) (*okxSwapExecutionResult, error) {
	if req.Exec == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	client := req.Exec.Client()
	chainID := req.Exec.ChainID()
	if client == nil || chainID == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if req.AmountIn == nil || req.AmountIn.Sign() <= 0 {
		return &okxSwapExecutionResult{DeltaOut: big.NewInt(0)}, nil
	}
	if req.TokenIn == req.TokenOut {
		return &okxSwapExecutionResult{DeltaOut: new(big.Int).Set(req.AmountIn), To: req.TxTo}, nil
	}
	if req.TxValue == nil {
		req.TxValue = big.NewInt(0)
	}
	if req.TxTo == (common.Address{}) {
		return nil, fmt.Errorf("%s tx.to is empty", req.Provider)
	}
	if len(req.TxData) == 0 {
		return nil, fmt.Errorf("%s tx.data is empty", req.Provider)
	}

	tokenInIsNative := isOKXNativeToken(req.TokenIn)
	tokenOutIsNative := isOKXNativeToken(req.TokenOut)

	outBefore, _ := getOKXSwapAssetBalance(client, req.TokenOut, req.WalletAddr)
	if outBefore == nil {
		outBefore = big.NewInt(0)
	}

	if tokenInIsNative {
		if req.TxValue.Cmp(req.AmountIn) != 0 {
			return nil, fmt.Errorf("%s native swap tx.value=%s does not match amountIn=%s", req.Provider, req.TxValue.String(), req.AmountIn.String())
		}
	} else {
		if req.TxValue.Sign() != 0 {
			return nil, fmt.Errorf("%s ERC20 swap returned unexpected native value=%s", req.Provider, req.TxValue.String())
		}
		if req.ApproveSpender == (common.Address{}) {
			return nil, fmt.Errorf("%s approve spender is empty", req.Provider)
		}
		if err := s.approveToken(client, chainID, req.PrivateKey, req.WalletAddr, req.TokenIn, req.ApproveSpender, req.AmountIn, TxOptions{}); err != nil {
			return nil, fmt.Errorf("%s approve spender failed: %w", req.Provider, err)
		}
	}

	gasPrice := req.SuggestedGasPrice
	if gasPrice == nil || gasPrice.Sign() <= 0 {
		var err error
		gasPrice, err = blockchain.GetGasPriceWithClient(client)
		if err != nil {
			return nil, err
		}
	}

	gasLimit, err := okxSwapGasLimit(client, req.WalletAddr, req.TxTo, req.TxValue, req.TxData, req.SuggestedGasLimit)
	if err != nil {
		return nil, err
	}
	log.Printf("[Liquidity] %s swap gasLimit: suggested=%d final=%d", strings.ToUpper(req.Provider), req.SuggestedGasLimit, gasLimit)

	signed, err := blockchain.SendRawTransactionWithRetry(blockchain.SendRawTxParams{
		Client:     client,
		ChainID:    chainID,
		PrivateKey: req.PrivateKey,
		From:       req.WalletAddr,
		To:         req.TxTo,
		Value:      req.TxValue,
		Data:       req.TxData,
		GasLimit:   gasLimit,
		GasPrice:   gasPrice,
	})
	if err != nil {
		return nil, err
	}

	txHash := signed.Hash().Hex()
	receipt, err := s.waitMined(client, chainID, signed)
	if err != nil {
		return nil, err
	}

	if d := ReceiptTokenTransferDelta(receipt, req.TokenOut, req.WalletAddr); d != nil && d.Sign() > 0 {
		return &okxSwapExecutionResult{TxHash: txHash, Receipt: receipt, DeltaOut: d, To: req.TxTo}, nil
	}

	outAfter, _ := getOKXSwapAssetBalance(client, req.TokenOut, req.WalletAddr)
	if outAfter == nil {
		outAfter = big.NewInt(0)
	}

	var delta *big.Int
	if tokenOutIsNative {
		gasCostWei := s.gasCostWeiFromReceipt(client, signed.Hash(), receipt)
		delta = nativeBalanceDelta(outBefore, outAfter, gasCostWei)
		if delta.Sign() <= 0 {
			minWanted := new(big.Int).Sub(new(big.Int).Set(outBefore), gasCostWei)
			if minWanted.Sign() < 0 {
				minWanted = big.NewInt(0)
			}
			minWanted.Add(minWanted, big.NewInt(1))
			if synced, werr := s.waitOKXSwapAssetBalanceAtLeast(client, req.TokenOut, req.WalletAddr, minWanted, req.Provider+" swap native out"); werr == nil && synced != nil {
				outAfter = synced
				delta = nativeBalanceDelta(outBefore, outAfter, gasCostWei)
			}
		}
	} else {
		delta = new(big.Int).Sub(outAfter, outBefore)
		if delta.Sign() <= 0 {
			minWanted := new(big.Int).Add(outBefore, big.NewInt(1))
			if synced, werr := s.waitOKXSwapAssetBalanceAtLeast(client, req.TokenOut, req.WalletAddr, minWanted, req.Provider+" swap out"); werr == nil && synced != nil {
				outAfter = synced
				delta = new(big.Int).Sub(outAfter, outBefore)
			}
		}
	}
	if delta.Sign() < 0 {
		delta = big.NewInt(0)
	}
	if delta.Sign() == 0 {
		log.Printf("[Liquidity] Warning: %s swap mined but tokenOut delta is 0 (tx=%s tokenOut=%s expected=%s outBefore=%s outAfter=%s)",
			strings.ToUpper(req.Provider), txHash, req.TokenOut.Hex(), req.ExpectedOut, outBefore.String(), outAfter.String())
	}
	return &okxSwapExecutionResult{TxHash: txHash, Receipt: receipt, DeltaOut: delta, To: req.TxTo}, nil
}

func (s *LiquidityService) executeZeroXSwapExactIn(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (*okxSwapExecutionResult, error) {
	if exec == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	if amountIn == nil || amountIn.Sign() <= 0 {
		return &okxSwapExecutionResult{DeltaOut: big.NewInt(0)}, nil
	}
	if tokenIn == tokenOut {
		return &okxSwapExecutionResult{DeltaOut: new(big.Int).Set(amountIn)}, nil
	}

	quote, err := exchange.NewZeroXSwapService().GetAllowanceHolderQuote(exchange.ZeroXQuoteRequest{
		ChainID:     fmt.Sprintf("%d", exec.Config().ChainID),
		SellToken:   okxTokenAddressParam(tokenIn),
		BuyToken:    okxTokenAddressParam(tokenOut),
		SellAmount:  amountIn.String(),
		Taker:       walletAddr.Hex(),
		SlippageBps: clampSlippageBps(slippagePercent),
	})
	if err != nil {
		return nil, err
	}
	if quote == nil {
		return nil, fmt.Errorf("0x quote response empty")
	}
	if !common.IsHexAddress(strings.TrimSpace(quote.Transaction.To)) {
		return nil, fmt.Errorf("0x tx.to invalid: %s", quote.Transaction.To)
	}
	txTo := common.HexToAddress(strings.TrimSpace(quote.Transaction.To))
	txData := common.FromHex(strings.TrimSpace(quote.Transaction.Data))
	txValue, ok := parseProviderBigInt(quote.Transaction.Value)
	if !ok {
		return nil, fmt.Errorf("0x tx.value invalid: %s", quote.Transaction.Value)
	}
	suggestedGasPrice, _ := parseProviderBigInt(quote.Transaction.GasPrice)
	var approveSpender common.Address
	if !isOKXNativeToken(tokenIn) {
		spender := strings.TrimSpace(quote.AllowanceTarget)
		if quote.Issues.Allowance != nil && strings.TrimSpace(quote.Issues.Allowance.Spender) != "" {
			spender = strings.TrimSpace(quote.Issues.Allowance.Spender)
		}
		if !common.IsHexAddress(spender) {
			return nil, fmt.Errorf("0x approve spender invalid: %s", spender)
		}
		approveSpender = common.HexToAddress(spender)
	}
	return s.executeProviderSwapTx(providerSwapExecutionRequest{
		Provider:          "0x",
		Exec:              exec,
		PrivateKey:        privateKey,
		WalletAddr:        walletAddr,
		TokenIn:           tokenIn,
		TokenOut:          tokenOut,
		AmountIn:          amountIn,
		ApproveSpender:    approveSpender,
		TxTo:              txTo,
		TxData:            txData,
		TxValue:           txValue,
		SuggestedGasLimit: parseProviderUint64(quote.Transaction.Gas),
		SuggestedGasPrice: suggestedGasPrice,
		ExpectedOut:       strings.TrimSpace(quote.BuyAmount),
	})
}

func (s *LiquidityService) executeLIFISwapExactIn(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (*okxSwapExecutionResult, error) {
	if exec == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	if amountIn == nil || amountIn.Sign() <= 0 {
		return &okxSwapExecutionResult{DeltaOut: big.NewInt(0)}, nil
	}
	if tokenIn == tokenOut {
		return &okxSwapExecutionResult{DeltaOut: new(big.Int).Set(amountIn)}, nil
	}

	quote, err := exchange.NewLIFISwapService().GetQuote(exchange.LIFIQuoteRequest{
		FromChainID: fmt.Sprintf("%d", exec.Config().ChainID),
		ToChainID:   fmt.Sprintf("%d", exec.Config().ChainID),
		FromToken:   exchange.LIFINormalizeTokenAddress(okxTokenAddressParam(tokenIn)),
		ToToken:     exchange.LIFINormalizeTokenAddress(okxTokenAddressParam(tokenOut)),
		FromAmount:  amountIn.String(),
		FromAddress: walletAddr.Hex(),
		ToAddress:   walletAddr.Hex(),
		Slippage:    slippagePercent / 100,
	})
	if err != nil {
		return nil, err
	}
	if quote == nil {
		return nil, fmt.Errorf("LI.FI quote response empty")
	}
	if !common.IsHexAddress(strings.TrimSpace(quote.TransactionRequest.To)) {
		return nil, fmt.Errorf("LI.FI tx.to invalid: %s", quote.TransactionRequest.To)
	}
	txTo := common.HexToAddress(strings.TrimSpace(quote.TransactionRequest.To))
	txData := common.FromHex(strings.TrimSpace(quote.TransactionRequest.Data))
	txValue, ok := parseProviderBigInt(quote.TransactionRequest.Value)
	if !ok {
		return nil, fmt.Errorf("LI.FI tx.value invalid: %s", quote.TransactionRequest.Value)
	}
	suggestedGasPrice, _ := parseProviderBigInt(quote.TransactionRequest.GasPrice)
	var approveSpender common.Address
	if !isOKXNativeToken(tokenIn) {
		spender := strings.TrimSpace(quote.Estimate.ApprovalAddress)
		if !common.IsHexAddress(spender) {
			return nil, fmt.Errorf("LI.FI approvalAddress invalid: %s", spender)
		}
		approveSpender = common.HexToAddress(spender)
	}
	return s.executeProviderSwapTx(providerSwapExecutionRequest{
		Provider:          "li.fi",
		Exec:              exec,
		PrivateKey:        privateKey,
		WalletAddr:        walletAddr,
		TokenIn:           tokenIn,
		TokenOut:          tokenOut,
		AmountIn:          amountIn,
		ApproveSpender:    approveSpender,
		TxTo:              txTo,
		TxData:            txData,
		TxValue:           txValue,
		SuggestedGasLimit: parseProviderUint64(quote.TransactionRequest.GasLimit),
		SuggestedGasPrice: suggestedGasPrice,
		ExpectedOut:       strings.TrimSpace(quote.Estimate.ToAmount),
	})
}

func (s *LiquidityService) SwapSingleTokenDetailedByProvider(
	provider string,
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (*SwapSingleTokenResult, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))

	var (
		r   *okxSwapExecutionResult
		err error
	)

	switch provider {
	case "", "okx":
		provider = "okx"
		r, err = s.executeOKXSwapExactIn(exec, privateKey, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent)
	case "0x":
		r, err = s.executeZeroXSwapExactIn(exec, privateKey, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent)
	case "li.fi", "lifi":
		provider = "li.fi"
		r, err = s.executeLIFISwapExactIn(exec, privateKey, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent)
	default:
		return nil, fmt.Errorf("unsupported swap provider: %s", provider)
	}
	if r == nil {
		return nil, err
	}

	amountOut := big.NewInt(0)
	if r.DeltaOut != nil {
		amountOut = new(big.Int).Set(r.DeltaOut)
	}

	return &SwapSingleTokenResult{
		Provider:      provider,
		TxHash:        r.TxHash,
		AmountOut:     amountOut,
		Receipt:       r.Receipt,
		RouterAddress: r.To,
	}, err
}
