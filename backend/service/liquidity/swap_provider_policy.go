package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/exchange"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

type SwapRouteInfo struct {
	Provider     string `json:"provider"`
	ProviderName string `json:"provider_name"`
	QuoteID      string `json:"quote_id,omitempty"`
	VendorName   string `json:"vendor_name,omitempty"`
	RouteSummary string `json:"route_summary,omitempty"`
	AmountInRaw  string `json:"amount_in_raw,omitempty"`
	AmountOutRaw string `json:"amount_out_raw,omitempty"`
}

type providerQuote struct {
	info      SwapRouteInfo
	amountOut *big.Int
}

type providerExecutionResult struct {
	TxHash    string
	AmountOut *big.Int
	Info      SwapRouteInfo
}

func normalizeSwapProviderPolicy(policy string) (models.StrategySwapProviderPolicy, error) {
	normalized := models.NormalizeStrategySwapProviderPolicy(policy)
	if normalized == "" {
		return "", fmt.Errorf("unsupported swap provider policy: %s", strings.TrimSpace(policy))
	}
	return normalized, nil
}

func effectiveTaskSwapProviderPolicy(task *models.StrategyTask) models.StrategySwapProviderPolicy {
	return models.ResolveStrategySwapProviderPolicy(task)
}

func swapProviderDisplayName(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "okx":
		return "OKX"
	case "binance":
		return "Binance"
	default:
		return strings.TrimSpace(provider)
	}
}

func parseQuoteAmount(raw string, provider string) (*big.Int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return big.NewInt(0), nil
	}
	if strings.HasPrefix(strings.ToLower(raw), "0x") {
		out, ok := new(big.Int).SetString(raw[2:], 16)
		if !ok || out == nil {
			return nil, fmt.Errorf("invalid %s quote output: %s", provider, raw)
		}
		return out, nil
	}
	out, ok := new(big.Int).SetString(raw, 10)
	if !ok || out == nil {
		return nil, fmt.Errorf("invalid %s quote output: %s", provider, raw)
	}
	return out, nil
}

func binanceTokenAddressParam(token common.Address) string {
	return okxTokenAddressParam(token)
}

func routeSummaryFromLabels(labels []string) string {
	seen := make(map[string]struct{}, len(labels))
	parts := make([]string, 0, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		parts = append(parts, label)
	}
	return strings.Join(parts, " -> ")
}

func okxRouteSummary(raw json.RawMessage) string {
	type token struct {
		TokenSymbol string `json:"tokenSymbol"`
		Symbol      string `json:"symbol"`
	}
	type protocol struct {
		DexName     string `json:"dexName"`
		DexProtocol string `json:"dexProtocol"`
	}
	type subRoute struct {
		FromToken   token      `json:"fromToken"`
		ToToken     token      `json:"toToken"`
		DexProtocol []protocol `json:"dexProtocol"`
	}
	type dexRoute struct {
		SubRouterList []subRoute `json:"subRouterList"`
		DexProtocol   []protocol `json:"dexProtocol"`
	}
	if len(raw) == 0 {
		return ""
	}
	var routes []dexRoute
	if err := json.Unmarshal(raw, &routes); err != nil {
		return ""
	}
	labels := make([]string, 0, len(routes))
	for _, route := range routes {
		if len(route.SubRouterList) > 0 {
			for _, sub := range route.SubRouterList {
				if len(sub.DexProtocol) == 0 {
					continue
				}
				label := strings.TrimSpace(sub.DexProtocol[0].DexName)
				if label == "" {
					label = strings.TrimSpace(sub.DexProtocol[0].DexProtocol)
				}
				labels = append(labels, label)
			}
			continue
		}
		if len(route.DexProtocol) == 0 {
			continue
		}
		label := strings.TrimSpace(route.DexProtocol[0].DexName)
		if label == "" {
			label = strings.TrimSpace(route.DexProtocol[0].DexProtocol)
		}
		labels = append(labels, label)
	}
	return routeSummaryFromLabels(labels)
}

func binanceRouteSummary(routes []exchange.BinanceDexRoute) string {
	labels := make([]string, 0, len(routes))
	for _, route := range routes {
		label := strings.TrimSpace(route.DexProtocol.DexName)
		if label == "" {
			label = strings.TrimSpace(route.DexProtocol.DexProtocol)
		}
		labels = append(labels, label)
	}
	return routeSummaryFromLabels(labels)
}

func (s *LiquidityService) quoteOKXSwapRoute(
	exec chainexec.EVMExecutor,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (*providerQuote, error) {
	if exec == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	if amountIn == nil || amountIn.Sign() <= 0 {
		return &providerQuote{
			info:      SwapRouteInfo{Provider: "okx", ProviderName: "OKX", AmountInRaw: "0", AmountOutRaw: "0"},
			amountOut: big.NewInt(0),
		}, nil
	}
	if tokenIn == tokenOut {
		return &providerQuote{
			info:      SwapRouteInfo{Provider: "okx", ProviderName: "OKX", AmountInRaw: amountIn.String(), AmountOutRaw: amountIn.String()},
			amountOut: cloneBig(amountIn),
		}, nil
	}
	if s.okxService == nil {
		s.okxService = exchange.NewOKXDexService()
	}
	cc := exec.Config()
	swapResp, err := s.okxService.GetSwapData(exchange.SwapRequest{
		ChainID:           fmt.Sprintf("%d", cc.ChainID),
		FromTokenAddress:  okxTokenAddressParam(tokenIn),
		ToTokenAddress:    okxTokenAddressParam(tokenOut),
		Amount:            amountIn.String(),
		Slippage:          s.okxSlippageDecimal(slippagePercent),
		UserWalletAddress: walletAddr.Hex(),
	})
	if err != nil {
		return nil, err
	}
	if swapResp == nil || len(swapResp.Data) == 0 {
		return nil, fmt.Errorf("OKX swap response empty")
	}
	amountOut, err := parseQuoteAmount(swapResp.Data[0].RouterResult.ToTokenAmount, "OKX")
	if err != nil {
		return nil, err
	}
	txObj := swapResp.Data[0].Tx
	if !common.IsHexAddress(strings.TrimSpace(txObj.To)) {
		return nil, fmt.Errorf("OKX tx.to invalid: %s", txObj.To)
	}
	if len(common.FromHex(strings.TrimSpace(txObj.Data))) == 0 {
		return nil, fmt.Errorf("OKX tx.data empty")
	}
	return &providerQuote{
		info: SwapRouteInfo{
			Provider:     "okx",
			ProviderName: "OKX",
			RouteSummary: okxRouteSummary(swapResp.Data[0].RouterResult.DexRouterList),
			AmountInRaw:  amountIn.String(),
			AmountOutRaw: amountOut.String(),
		},
		amountOut: amountOut,
	}, nil
}

func (s *LiquidityService) quoteBestBinanceSwapRoute(
	exec chainexec.EVMExecutor,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
) (*providerQuote, error) {
	if exec == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	if amountIn == nil || amountIn.Sign() <= 0 {
		return &providerQuote{
			info:      SwapRouteInfo{Provider: "binance", ProviderName: "Binance", AmountInRaw: "0", AmountOutRaw: "0"},
			amountOut: big.NewInt(0),
		}, nil
	}
	if tokenIn == tokenOut {
		return &providerQuote{
			info:      SwapRouteInfo{Provider: "binance", ProviderName: "Binance", AmountInRaw: amountIn.String(), AmountOutRaw: amountIn.String()},
			amountOut: cloneBig(amountIn),
		}, nil
	}
	if s.binanceService == nil {
		s.binanceService = exchange.NewBinanceSwapService()
	}
	resp, err := s.binanceService.GetAggregatedQuote(exchange.BinanceQuoteRequest{
		BinanceChainID:    fmt.Sprintf("%d", exec.Config().ChainID),
		Amount:            amountIn.String(),
		FromTokenAddress:  binanceTokenAddressParam(tokenIn),
		ToTokenAddress:    binanceTokenAddressParam(tokenOut),
		UserWalletAddress: walletAddr.Hex(),
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Data) == 0 {
		return nil, fmt.Errorf("Binance Web3 quote response empty")
	}

	var best *providerQuote
	for _, route := range resp.Data {
		if !strings.EqualFold(strings.TrimSpace(route.ExecutionMode), "SWAP") {
			continue
		}
		if strings.TrimSpace(route.QuoteID) == "" {
			continue
		}
		out, err := parseQuoteAmount(route.ToTokenAmount, "Binance")
		if err != nil {
			return nil, err
		}
		q := &providerQuote{
			info: SwapRouteInfo{
				Provider:     "binance",
				ProviderName: "Binance",
				QuoteID:      strings.TrimSpace(route.QuoteID),
				VendorName:   strings.TrimSpace(route.VendorName),
				RouteSummary: binanceRouteSummary(route.DexRouterList),
				AmountInRaw:  amountIn.String(),
				AmountOutRaw: out.String(),
			},
			amountOut: out,
		}
		if best == nil || out.Cmp(best.amountOut) > 0 {
			best = q
		}
	}
	if best == nil {
		return nil, fmt.Errorf("Binance Web3 quote response has no executable SWAP route")
	}
	return best, nil
}

func sortProviderQuotesByAmountOut(quotes []*providerQuote) {
	for i := 1; i < len(quotes); i++ {
		q := quotes[i]
		j := i - 1
		for ; j >= 0; j-- {
			if providerQuoteAmountCmp(quotes[j], q) >= 0 {
				break
			}
			quotes[j+1] = quotes[j]
		}
		quotes[j+1] = q
	}
}

func providerQuoteAmountCmp(a *providerQuote, b *providerQuote) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	if a.amountOut == nil && b.amountOut == nil {
		return 0
	}
	if a.amountOut == nil {
		return -1
	}
	if b.amountOut == nil {
		return 1
	}
	return a.amountOut.Cmp(b.amountOut)
}

func swapProviderFallbackOrder(policy models.StrategySwapProviderPolicy) []models.StrategySwapProviderPolicy {
	switch policy {
	case models.StrategySwapProviderOKX:
		return []models.StrategySwapProviderPolicy{models.StrategySwapProviderOKX, models.StrategySwapProviderBinance}
	case models.StrategySwapProviderBinance:
		return []models.StrategySwapProviderPolicy{models.StrategySwapProviderBinance, models.StrategySwapProviderOKX}
	default:
		return nil
	}
}

func (s *LiquidityService) quoteSwapRoutesByPolicy(
	exec chainexec.EVMExecutor,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
	policy models.StrategySwapProviderPolicy,
) ([]*providerQuote, []error, error) {
	var (
		quotes []*providerQuote
		errs   []error
	)
	addQuote := func(provider models.StrategySwapProviderPolicy) {
		var (
			q   *providerQuote
			err error
		)
		switch provider {
		case models.StrategySwapProviderOKX:
			q, err = s.quoteOKXSwapRoute(exec, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent)
		case models.StrategySwapProviderBinance:
			q, err = s.quoteBestBinanceSwapRoute(exec, walletAddr, tokenIn, tokenOut, amountIn)
		default:
			err = fmt.Errorf("unsupported swap provider policy: %s", provider)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s quote failed: %w", swapProviderDisplayName(string(provider)), err))
			return
		}
		if q != nil {
			quotes = append(quotes, q)
		}
	}

	switch policy {
	case models.StrategySwapProviderBest:
		addQuote(models.StrategySwapProviderOKX)
		addQuote(models.StrategySwapProviderBinance)
		sortProviderQuotesByAmountOut(quotes)
	case models.StrategySwapProviderOKX, models.StrategySwapProviderBinance:
		for _, provider := range swapProviderFallbackOrder(policy) {
			addQuote(provider)
		}
	default:
		return nil, errs, fmt.Errorf("unsupported swap provider policy: %s", policy)
	}
	if len(quotes) == 0 {
		return nil, errs, fmt.Errorf("no executable swap quote: %v", errs)
	}
	return quotes, errs, nil
}

func (s *LiquidityService) quoteSwapByPolicy(
	exec chainexec.EVMExecutor,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
	policy models.StrategySwapProviderPolicy,
) (*providerQuote, error) {
	if policy == models.StrategySwapProviderOKX {
		return s.quoteOKXSwapRoute(exec, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent)
	}
	if policy == models.StrategySwapProviderBinance {
		return s.quoteBestBinanceSwapRoute(exec, walletAddr, tokenIn, tokenOut, amountIn)
	}
	quotes, _, err := s.quoteSwapRoutesByPolicy(exec, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent, policy)
	if err != nil {
		return nil, err
	}
	return quotes[0], nil
}

func (s *LiquidityService) executeProviderQuote(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
	quote *providerQuote,
) (*providerExecutionResult, error) {
	if quote == nil {
		return nil, fmt.Errorf("swap quote is nil")
	}
	var (
		r   *okxSwapExecutionResult
		err error
	)
	switch quote.info.Provider {
	case "okx":
		r, err = s.executeOKXSwapExactIn(exec, privateKey, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent)
	case "binance":
		r, err = s.executeBinanceSwapExactIn(exec, privateKey, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent, quote.info.QuoteID)
	default:
		return nil, fmt.Errorf("unsupported swap provider: %s", quote.info.Provider)
	}
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%s swap result is nil", quote.info.Provider)
	}
	info := quote.info
	amountOut := big.NewInt(0)
	if r.DeltaOut != nil && r.DeltaOut.Sign() > 0 {
		amountOut = cloneBig(r.DeltaOut)
		info.AmountOutRaw = amountOut.String()
	}
	log.Printf("[Liquidity] swap executed provider=%s route=%s quote_id=%s tx=%s amountIn=%s amountOut=%s",
		info.ProviderName, strings.TrimSpace(info.RouteSummary), strings.TrimSpace(info.QuoteID), r.TxHash, info.AmountInRaw, info.AmountOutRaw)
	return &providerExecutionResult{
		TxHash:    r.TxHash,
		AmountOut: amountOut,
		Info:      info,
	}, nil
}

func (s *LiquidityService) executeSwapByPolicy(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
	policy models.StrategySwapProviderPolicy,
) (*providerExecutionResult, error) {
	if amountIn == nil || amountIn.Sign() <= 0 || tokenIn == tokenOut {
		return &providerExecutionResult{
			AmountOut: cloneBig(amountIn),
			Info: SwapRouteInfo{
				Provider:     string(policy),
				ProviderName: swapProviderDisplayName(string(policy)),
				AmountInRaw:  "0",
				AmountOutRaw: "0",
			},
		}, nil
	}
	quotes, quoteErrs, err := s.quoteSwapRoutesByPolicy(exec, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent, policy)
	if err != nil {
		return nil, err
	}
	errs := append([]error{}, quoteErrs...)
	for idx, quote := range quotes {
		result, execErr := s.executeProviderQuote(exec, privateKey, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent, quote)
		if execErr == nil {
			return result, nil
		}
		errs = append(errs, fmt.Errorf("%s execution failed: %w", quote.info.ProviderName, execErr))
		if idx+1 < len(quotes) {
			next := quotes[idx+1]
			log.Printf("[Liquidity] swap provider %s failed, trying fallback %s: %v",
				quote.info.ProviderName, next.info.ProviderName, execErr)
		}
	}
	return nil, fmt.Errorf("all swap providers failed: %w", errors.Join(errs...))
}

func formatSwapRouteLabel(info SwapRouteInfo) string {
	provider := strings.TrimSpace(info.ProviderName)
	if provider == "" {
		provider = swapProviderDisplayName(info.Provider)
	}
	route := strings.TrimSpace(info.RouteSummary)
	if route == "" {
		route = strings.TrimSpace(info.VendorName)
	}
	if route == "" {
		return provider
	}
	return provider + " " + route
}

func (s *LiquidityService) executeSwapToUSDTByPolicy(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	usdtAddr common.Address,
	amountIn *big.Int,
	slippagePercent float64,
	policy models.StrategySwapProviderPolicy,
) (*providerExecutionResult, error) {
	if exec == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	client := exec.Client()
	if client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if amountIn == nil || amountIn.Sign() <= 0 {
		return &providerExecutionResult{}, nil
	}
	if tokenIn == usdtAddr {
		return &providerExecutionResult{}, nil
	}

	actualBalance, err := getOKXSwapAssetBalance(client, tokenIn, walletAddr)
	if err != nil {
		log.Printf("[Liquidity] Warning: failed to get token balance: %v", err)
	} else {
		if actualBalance == nil {
			actualBalance = big.NewInt(0)
		}
		log.Printf("[Liquidity] Token %s balance: %s, attempting to swap: %s", tokenIn.Hex(), actualBalance.String(), amountIn.String())
		if actualBalance.Cmp(amountIn) < 0 {
			synced, werr := s.waitOKXSwapAssetBalanceAtLeast(client, tokenIn, walletAddr, amountIn, tokenIn.Hex())
			if werr == nil && synced != nil && synced.Cmp(amountIn) >= 0 {
				actualBalance = synced
			} else if synced != nil && synced.Sign() > 0 && synced.Cmp(amountIn) < 0 {
				log.Printf("[Liquidity] Warning: balance insufficient after sync wait, using synced balance %s instead of %s", synced.String(), amountIn.String())
				amountIn = synced
			} else if werr != nil {
				log.Printf("[Liquidity] Warning: balance sync wait failed for %s: %v (proceeding with amount=%s)", tokenIn.Hex(), werr, amountIn.String())
			}
		}
	}
	return s.executeSwapByPolicy(exec, privateKey, walletAddr, tokenIn, usdtAddr, amountIn, slippagePercent, policy)
}

func (s *LiquidityService) prepareProviderSwapParams(
	cc config.ChainConfig,
	executorAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippageTolerance float64,
	policy models.StrategySwapProviderPolicy,
) (*blockchain.SwapParamsSimple, *big.Int, SwapRouteInfo, error) {
	switch policy {
	case models.StrategySwapProviderOKX:
		return s.prepareOKXSwapParamsWithInfo(cc, executorAddr, tokenIn, tokenOut, amountIn, slippageTolerance)
	case models.StrategySwapProviderBinance:
		return s.prepareBinanceSwapParams(cc, executorAddr, tokenIn, tokenOut, amountIn, slippageTolerance)
	case models.StrategySwapProviderBest:
		okxParams, okxOut, okxInfo, okxErr := s.prepareOKXSwapParamsWithInfo(cc, executorAddr, tokenIn, tokenOut, amountIn, slippageTolerance)
		binanceParams, binanceOut, binanceInfo, binanceErr := s.prepareBinanceSwapParams(cc, executorAddr, tokenIn, tokenOut, amountIn, slippageTolerance)
		if okxErr != nil && binanceErr != nil {
			return nil, nil, SwapRouteInfo{}, fmt.Errorf("no executable zap swap quote: OKX=%v; Binance=%v", okxErr, binanceErr)
		}
		if okxErr == nil && (binanceErr != nil || binanceOut == nil || (okxOut != nil && okxOut.Cmp(binanceOut) >= 0)) {
			return okxParams, okxOut, okxInfo, nil
		}
		if binanceErr == nil {
			return binanceParams, binanceOut, binanceInfo, nil
		}
		return nil, nil, SwapRouteInfo{}, okxErr
	default:
		return nil, nil, SwapRouteInfo{}, fmt.Errorf("unsupported swap provider policy: %s", policy)
	}
}

func (s *LiquidityService) prepareBinanceSwapParams(
	cc config.ChainConfig,
	executorAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippageTolerance float64,
) (*blockchain.SwapParamsSimple, *big.Int, SwapRouteInfo, error) {
	if amountIn == nil || amountIn.Sign() <= 0 {
		return nil, nil, SwapRouteInfo{}, nil
	}
	if s.binanceService == nil {
		s.binanceService = exchange.NewBinanceSwapService()
	}

	quote, err := s.quoteBestBinanceSwapRouteForChain(cc.ChainID, executorAddr, tokenIn, tokenOut, amountIn)
	if err != nil {
		return nil, nil, SwapRouteInfo{}, err
	}
	if quote == nil {
		return nil, nil, SwapRouteInfo{}, fmt.Errorf("Binance Web3 quote response empty")
	}

	resp, err := s.binanceService.BuildSwapTransaction(exchange.BinanceBuildSwapRequest{
		BinanceChainID:     fmt.Sprintf("%d", cc.ChainID),
		Amount:             amountIn.String(),
		FromTokenAddress:   binanceTokenAddressParam(tokenIn),
		ToTokenAddress:     binanceTokenAddressParam(tokenOut),
		UserWalletAddress:  executorAddr.Hex(),
		QuoteID:            quote.info.QuoteID,
		SlippagePercent:    slippagePercentParam(slippageTolerance),
		ApproveTransaction: "true",
		ApproveAmount:      amountIn.String(),
	})
	if err != nil {
		return nil, nil, SwapRouteInfo{}, err
	}
	if resp == nil {
		return nil, nil, SwapRouteInfo{}, fmt.Errorf("Binance Web3 swap response empty")
	}
	if mode := strings.TrimSpace(resp.Data.ExecutionMode); mode != "" && !strings.EqualFold(mode, "SWAP") {
		return nil, nil, SwapRouteInfo{}, fmt.Errorf("Binance Web3 executionMode %s is not supported", mode)
	}
	tx := resp.Data.Tx
	if strings.TrimSpace(tx.From) != "" && !strings.EqualFold(strings.TrimSpace(tx.From), executorAddr.Hex()) {
		return nil, nil, SwapRouteInfo{}, fmt.Errorf("Binance Web3 tx.from mismatch: %s", tx.From)
	}
	if !common.IsHexAddress(strings.TrimSpace(tx.To)) {
		return nil, nil, SwapRouteInfo{}, fmt.Errorf("Binance Web3 tx.to invalid: %s", tx.To)
	}
	txTo := common.HexToAddress(strings.TrimSpace(tx.To))
	txData := common.FromHex(strings.TrimSpace(tx.Data))
	if len(txData) == 0 {
		return nil, nil, SwapRouteInfo{}, fmt.Errorf("Binance Web3 tx.data is empty")
	}
	txValue, ok := parseProviderBigInt(tx.Value)
	if !ok {
		return nil, nil, SwapRouteInfo{}, fmt.Errorf("Binance Web3 tx.value invalid: %s", tx.Value)
	}
	if txValue.Sign() != 0 {
		return nil, nil, SwapRouteInfo{}, fmt.Errorf("Binance Web3 swap(zap) requires native value=%s; Zap contract cannot execute this route", txValue.String())
	}

	spender, ok := binanceApproveSpenderFromSignatureData(tx.SignatureData)
	if !ok {
		if target := strings.TrimSpace(resp.Data.RouterResult.ApproveTarget); common.IsHexAddress(target) {
			spender = common.HexToAddress(target)
			ok = true
		}
	}
	if !ok {
		return nil, nil, SwapRouteInfo{}, fmt.Errorf("Binance Web3 approve spender missing")
	}

	baseOut := quote.amountOut
	if baseOut == nil {
		baseOut = big.NewInt(0)
	}
	if raw := strings.TrimSpace(resp.Data.RouterResult.ToTokenAmount); raw != "" {
		if parsed, perr := parseQuoteAmount(raw, "Binance"); perr == nil {
			baseOut = parsed
		} else {
			return nil, nil, SwapRouteInfo{}, perr
		}
	}
	keepBps := zapSwapMinOutKeepBps(slippageTolerance)
	minOut := big.NewInt(0)
	if baseOut.Sign() > 0 {
		minOut = new(big.Int).Div(new(big.Int).Mul(baseOut, big.NewInt(keepBps)), big.NewInt(10000))
	}

	info := quote.info
	info.AmountOutRaw = baseOut.String()
	if routeSummary := binanceRouteSummary(resp.Data.RouterResult.DexRouterList); strings.TrimSpace(routeSummary) != "" {
		info.RouteSummary = routeSummary
	}
	if vendor := strings.TrimSpace(resp.Data.RouterResult.VendorName); vendor != "" {
		info.VendorName = vendor
	}

	log.Printf("[Liquidity] Binance swap(zap): %s -> %s amountIn=%s executor=%s expectedOut=%s route=%s slippage=%.4f%%",
		tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), executorAddr.Hex(), baseOut.String(), strings.TrimSpace(info.RouteSummary), slippageTolerance)

	return &blockchain.SwapParamsSimple{
		Target:        txTo,
		ApproveTarget: spender,
		TokenIn:       tokenIn,
		TokenOut:      tokenOut,
		AmountIn:      amountIn,
		MinAmountOut:  minOut,
		CallData:      txData,
	}, baseOut, info, nil
}

func (s *LiquidityService) quoteBestBinanceSwapRouteForChain(
	chainID int64,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
) (*providerQuote, error) {
	if amountIn == nil || amountIn.Sign() <= 0 {
		return &providerQuote{
			info:      SwapRouteInfo{Provider: "binance", ProviderName: "Binance", AmountInRaw: "0", AmountOutRaw: "0"},
			amountOut: big.NewInt(0),
		}, nil
	}
	if s.binanceService == nil {
		s.binanceService = exchange.NewBinanceSwapService()
	}
	resp, err := s.binanceService.GetAggregatedQuote(exchange.BinanceQuoteRequest{
		BinanceChainID:    fmt.Sprintf("%d", chainID),
		Amount:            amountIn.String(),
		FromTokenAddress:  binanceTokenAddressParam(tokenIn),
		ToTokenAddress:    binanceTokenAddressParam(tokenOut),
		UserWalletAddress: walletAddr.Hex(),
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Data) == 0 {
		return nil, fmt.Errorf("Binance Web3 quote response empty")
	}
	var best *providerQuote
	for _, route := range resp.Data {
		if !strings.EqualFold(strings.TrimSpace(route.ExecutionMode), "SWAP") || strings.TrimSpace(route.QuoteID) == "" {
			continue
		}
		out, err := parseQuoteAmount(route.ToTokenAmount, "Binance")
		if err != nil {
			return nil, err
		}
		q := &providerQuote{
			info: SwapRouteInfo{
				Provider:     "binance",
				ProviderName: "Binance",
				QuoteID:      strings.TrimSpace(route.QuoteID),
				VendorName:   strings.TrimSpace(route.VendorName),
				RouteSummary: binanceRouteSummary(route.DexRouterList),
				AmountInRaw:  amountIn.String(),
				AmountOutRaw: out.String(),
			},
			amountOut: out,
		}
		if best == nil || out.Cmp(best.amountOut) > 0 {
			best = q
		}
	}
	if best == nil {
		return nil, fmt.Errorf("Binance Web3 quote response has no executable SWAP route")
	}
	return best, nil
}
