package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/exchange"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
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

func slippagePercentParam(slippagePercent float64) string {
	if slippagePercent <= 0 {
		slippagePercent = 1
	}
	out := fmt.Sprintf("%.4f", slippagePercent)
	out = strings.TrimRight(out, "0")
	out = strings.TrimRight(out, ".")
	if out == "" {
		return "1"
	}
	return out
}

func binanceApproveSpenderFromSignatureData(values []string) (common.Address, bool) {
	for _, raw := range values {
		item := strings.TrimSpace(raw)
		if common.IsHexAddress(item) {
			return common.HexToAddress(item), true
		}
		var parsed struct {
			ApproveContract string `json:"approveContract"`
			Spender         string `json:"spender"`
			To              string `json:"to"`
		}
		if err := json.Unmarshal([]byte(item), &parsed); err != nil {
			continue
		}
		for _, candidate := range []string{parsed.ApproveContract, parsed.Spender, parsed.To} {
			if common.IsHexAddress(strings.TrimSpace(candidate)) {
				return common.HexToAddress(strings.TrimSpace(candidate)), true
			}
		}
	}
	return common.Address{}, false
}

func (s *LiquidityService) executeBinanceSwapExactIn(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
	quoteID string,
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
	if s.binanceService == nil {
		s.binanceService = exchange.NewBinanceSwapService()
	}
	quoteID = strings.TrimSpace(quoteID)
	if quoteID == "" {
		quoteResp, err := s.binanceService.GetAggregatedQuote(exchange.BinanceQuoteRequest{
			BinanceChainID:    fmt.Sprintf("%d", exec.Config().ChainID),
			Amount:            amountIn.String(),
			FromTokenAddress:  okxTokenAddressParam(tokenIn),
			ToTokenAddress:    okxTokenAddressParam(tokenOut),
			UserWalletAddress: walletAddr.Hex(),
		})
		if err != nil {
			return nil, err
		}
		for _, route := range quoteResp.Data {
			if strings.EqualFold(strings.TrimSpace(route.ExecutionMode), "SWAP") && strings.TrimSpace(route.QuoteID) != "" {
				quoteID = strings.TrimSpace(route.QuoteID)
				break
			}
		}
		if quoteID == "" {
			return nil, fmt.Errorf("Binance Web3 quote response has no executable SWAP route")
		}
	}

	resp, err := s.binanceService.BuildSwapTransaction(exchange.BinanceBuildSwapRequest{
		BinanceChainID:     fmt.Sprintf("%d", exec.Config().ChainID),
		Amount:             amountIn.String(),
		FromTokenAddress:   okxTokenAddressParam(tokenIn),
		ToTokenAddress:     okxTokenAddressParam(tokenOut),
		UserWalletAddress:  walletAddr.Hex(),
		QuoteID:            quoteID,
		SlippagePercent:    slippagePercentParam(slippagePercent),
		ApproveTransaction: "true",
		ApproveAmount:      amountIn.String(),
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("Binance Web3 swap response empty")
	}
	mode := strings.TrimSpace(resp.Data.ExecutionMode)
	if mode != "" && !strings.EqualFold(mode, "SWAP") {
		return nil, fmt.Errorf("Binance Web3 executionMode %s is not supported", mode)
	}
	tx := resp.Data.Tx
	if strings.TrimSpace(tx.From) != "" && !strings.EqualFold(strings.TrimSpace(tx.From), walletAddr.Hex()) {
		return nil, fmt.Errorf("Binance Web3 tx.from mismatch: %s", tx.From)
	}
	if !common.IsHexAddress(strings.TrimSpace(tx.To)) {
		return nil, fmt.Errorf("Binance Web3 tx.to invalid: %s", tx.To)
	}
	txTo := common.HexToAddress(strings.TrimSpace(tx.To))
	txData := common.FromHex(strings.TrimSpace(tx.Data))
	if len(txData) == 0 {
		return nil, fmt.Errorf("Binance Web3 tx.data is empty")
	}
	txValue, ok := parseProviderBigInt(tx.Value)
	if !ok {
		return nil, fmt.Errorf("Binance Web3 tx.value invalid: %s", tx.Value)
	}
	suggestedGasPrice, _ := parseProviderBigInt(tx.GasPrice)

	var approveSpender common.Address
	if !isOKXNativeToken(tokenIn) {
		spender, ok := binanceApproveSpenderFromSignatureData(tx.SignatureData)
		if !ok {
			return nil, fmt.Errorf("Binance Web3 approve spender missing from signatureData")
		}
		approveSpender = spender
	}
	return s.executeProviderSwapTx(providerSwapExecutionRequest{
		Provider:          "binance",
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
		SuggestedGasLimit: parseProviderUint64(tx.Gas),
		SuggestedGasPrice: suggestedGasPrice,
		ExpectedOut:       strings.TrimSpace(resp.Data.RouterResult.ToTokenAmount),
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
	return s.SwapSingleTokenDetailedByProviderQuote(provider, "", exec, privateKey, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent)
}

func (s *LiquidityService) SwapSingleTokenDetailedByProviderQuote(
	provider string,
	quoteID string,
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
	case "binance":
		r, err = s.executeBinanceSwapExactIn(exec, privateKey, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent, quoteID)
	case "0x":
		return nil, fmt.Errorf("swap provider 0x is no longer supported")
	case "li.fi", "lifi":
		return nil, fmt.Errorf("swap provider li.fi is no longer supported")
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
