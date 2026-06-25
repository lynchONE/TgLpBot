package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/service/exchange"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type swapFeeDetail struct {
	Label       string `json:"label"`
	Category    string `json:"category,omitempty"`
	Amount      string `json:"amount,omitempty"`
	AmountFloat string `json:"amount_float,omitempty"`
	Token       string `json:"token,omitempty"`
	TokenSymbol string `json:"token_symbol,omitempty"`
	Rate        string `json:"rate,omitempty"`
	Description string `json:"description,omitempty"`
}

type swapRouteHop struct {
	Index      int    `json:"index"`
	Source     string `json:"source,omitempty"`
	Tool       string `json:"tool,omitempty"`
	FromToken  string `json:"from_token,omitempty"`
	FromSymbol string `json:"from_symbol,omitempty"`
	ToToken    string `json:"to_token,omitempty"`
	ToSymbol   string `json:"to_symbol,omitempty"`
	Share      string `json:"share,omitempty"`
}

type swapProviderQuote struct {
	Provider           string          `json:"provider"`
	ProviderLabel      string          `json:"provider_label"`
	QuoteID            string          `json:"quote_id,omitempty"`
	VendorName         string          `json:"vendor_name,omitempty"`
	ExecutionMode      string          `json:"execution_mode,omitempty"`
	Status             string          `json:"status"`
	Recommended        bool            `json:"recommended,omitempty"`
	Error              string          `json:"error,omitempty"`
	CanExecute         bool            `json:"can_execute"`
	FromAmount         string          `json:"from_amount,omitempty"`
	GrossToAmount      string          `json:"gross_to_amount,omitempty"`
	GrossToAmountFloat string          `json:"gross_to_amount_float,omitempty"`
	NetToAmount        string          `json:"net_to_amount,omitempty"`
	NetToAmountFloat   string          `json:"net_to_amount_float,omitempty"`
	MinToAmount        string          `json:"min_to_amount,omitempty"`
	MinToAmountFloat   string          `json:"min_to_amount_float,omitempty"`
	EstimatedGas       string          `json:"estimated_gas,omitempty"`
	EstimatedGasNative float64         `json:"estimated_gas_native,omitempty"`
	EstimatedGasUSD    float64         `json:"estimated_gas_usd,omitempty"`
	EstimatedGasSymbol string          `json:"estimated_gas_symbol,omitempty"`
	FeeRule            string          `json:"fee_rule,omitempty"`
	FeeSummary         string          `json:"fee_summary,omitempty"`
	PriceImpactPercent string          `json:"price_impact_percent,omitempty"`
	RouteSummary       string          `json:"route_summary,omitempty"`
	Fees               []swapFeeDetail `json:"fees,omitempty"`
	Route              []swapRouteHop  `json:"route,omitempty"`
}

func swapProviderLabel(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "okx":
		return "OKX"
	case "binance":
		return "Binance"
	case "0x":
		return "0x"
	case "li.fi", "lifi":
		return "LI.FI"
	default:
		return strings.TrimSpace(provider)
	}
}

func newUnavailableSwapProviderQuote(provider string, chain string, err error) swapProviderQuote {
	msg := ""
	if err != nil {
		msg = strings.TrimSpace(err.Error())
	}
	return swapProviderQuote{
		Provider:           provider,
		ProviderLabel:      swapProviderLabel(provider),
		Status:             "unavailable",
		Error:              msg,
		CanExecute:         false,
		EstimatedGasSymbol: nativeSymbolForChain(chain),
	}
}

func nativeSymbolForChain(chain string) string {
	if config.AppConfig == nil {
		return "NATIVE"
	}
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok {
		return "NATIVE"
	}
	return nativeSymbolForChainConfig(chain, cc)
}

func parseSwapBigInt(raw string) (*big.Int, bool) {
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

func formatQuoteAmount(raw string, decimals int) string {
	if v, ok := parseSwapBigInt(raw); ok && v != nil {
		return fmt.Sprintf("%.6f", amountToFloat(v.String(), decimals))
	}
	return ""
}

func minAmountWithSlippage(raw string, slippagePercent float64) string {
	amount, ok := parseSwapBigInt(raw)
	if !ok || amount == nil || amount.Sign() <= 0 {
		return ""
	}
	bps := int(slippagePercent * 100)
	if slippagePercent > 0 && bps == 0 {
		bps = 1
	}
	if bps < 0 {
		bps = 0
	}
	if bps > 10000 {
		bps = 10000
	}
	out := new(big.Int).Mul(amount, big.NewInt(int64(10000-bps)))
	out.Div(out, big.NewInt(10000))
	return out.String()
}

func compareProviderQuote(a, b swapProviderQuote) int {
	if a.Status == "available" && b.Status != "available" {
		return -1
	}
	if a.Status != "available" && b.Status == "available" {
		return 1
	}
	aNet, _ := parseSwapBigInt(a.NetToAmount)
	bNet, _ := parseSwapBigInt(b.NetToAmount)
	if aNet != nil && bNet != nil {
		switch aNet.Cmp(bNet) {
		case 1:
			return -1
		case -1:
			return 1
		}
	}
	if a.EstimatedGasUSD > 0 && b.EstimatedGasUSD > 0 {
		if a.EstimatedGasUSD < b.EstimatedGasUSD {
			return -1
		}
		if a.EstimatedGasUSD > b.EstimatedGasUSD {
			return 1
		}
	}
	return strings.Compare(a.ProviderLabel, b.ProviderLabel)
}

func normalizeProviderQuotes(quotes []swapProviderQuote) ([]swapProviderQuote, *swapProviderQuote) {
	list := append([]swapProviderQuote(nil), quotes...)
	sort.SliceStable(list, func(i, j int) bool {
		return compareProviderQuote(list[i], list[j]) < 0
	})
	var best *swapProviderQuote
	for i := range list {
		list[i].Recommended = false
		if best == nil && list[i].Status == "available" {
			list[i].Recommended = true
			best = &list[i]
		}
	}
	return list, best
}

func buildRouteSummary(hops []swapRouteHop) string {
	seen := make(map[string]struct{}, len(hops))
	parts := make([]string, 0, len(hops))
	for _, hop := range hops {
		label := strings.TrimSpace(hop.Source)
		if label == "" {
			label = strings.TrimSpace(hop.Tool)
		}
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

func walletSwapProviderTokenAddress(token common.Address) string {
	if strings.EqualFold(token.Hex(), nativePseudoTokenAddress) {
		return nativePseudoTokenAddress
	}
	return token.Hex()
}

func buildBinanceProviderQuotes(
	chain string,
	cc config.ChainConfig,
	walletAddr common.Address,
	fromToken common.Address,
	toToken common.Address,
	amount *big.Int,
	slippagePercent float64,
	toDecimals int,
) []swapProviderQuote {
	resp, err := exchange.NewBinanceSwapService().GetAggregatedQuote(exchange.BinanceQuoteRequest{
		BinanceChainID:    fmt.Sprintf("%d", cc.ChainID),
		Amount:            amount.String(),
		FromTokenAddress:  walletSwapProviderTokenAddress(fromToken),
		ToTokenAddress:    walletSwapProviderTokenAddress(toToken),
		UserWalletAddress: walletAddr.Hex(),
	})
	if err != nil {
		return []swapProviderQuote{newUnavailableSwapProviderQuote("binance", chain, err)}
	}
	if resp == nil || len(resp.Data) == 0 {
		return []swapProviderQuote{newUnavailableSwapProviderQuote("binance", chain, fmt.Errorf("empty Binance Web3 quote response"))}
	}

	quotes := make([]swapProviderQuote, 0, len(resp.Data))
	for _, route := range resp.Data {
		quotes = append(quotes, buildBinanceProviderQuote(chain, cc, amount, route, slippagePercent, toDecimals))
	}
	return quotes
}

func buildBinanceProviderQuote(
	chain string,
	cc config.ChainConfig,
	amount *big.Int,
	route exchange.BinanceQuoteRoute,
	slippagePercent float64,
	toDecimals int,
) swapProviderQuote {
	toAmount := strings.TrimSpace(route.ToTokenAmount)
	minAmount := minAmountWithSlippage(toAmount, slippagePercent)
	hops := extractBinanceRouteHops(route.DexRouterList)
	quoteID := strings.TrimSpace(route.QuoteID)
	executionMode := strings.TrimSpace(route.ExecutionMode)
	status := "available"
	canExecute := true
	errText := ""
	if quoteID == "" {
		status = "unavailable"
		canExecute = false
		errText = "Binance route missing quoteId"
	} else if !strings.EqualFold(executionMode, "SWAP") {
		status = "unavailable"
		canExecute = false
		if executionMode == "" {
			errText = "Binance route missing executionMode"
		} else {
			errText = fmt.Sprintf("Binance executionMode %s is not supported", executionMode)
		}
	} else if toAmount == "" {
		status = "unavailable"
		canExecute = false
		errText = "Binance route missing toTokenAmount"
	}

	fees := make([]swapFeeDetail, 0, 1)
	feeSummary := ""
	if tradeFee := strings.TrimSpace(route.TradeFee); tradeFee != "" {
		feeSummary = "tradeFee: " + tradeFee
		fees = append(fees, swapFeeDetail{
			Label:       "tradeFee",
			Category:    "provider",
			Amount:      tradeFee,
			Description: "Binance Web3 route tradeFee",
		})
	}

	return swapProviderQuote{
		Provider:           "binance",
		ProviderLabel:      "Binance",
		QuoteID:            quoteID,
		VendorName:         strings.TrimSpace(route.VendorName),
		ExecutionMode:      executionMode,
		Status:             status,
		Error:              errText,
		CanExecute:         canExecute,
		FromAmount:         amount.String(),
		GrossToAmount:      toAmount,
		GrossToAmountFloat: formatQuoteAmount(toAmount, toDecimals),
		NetToAmount:        toAmount,
		NetToAmountFloat:   formatQuoteAmount(toAmount, toDecimals),
		MinToAmount:        minAmount,
		MinToAmountFloat:   formatQuoteAmount(minAmount, toDecimals),
		EstimatedGas:       strings.TrimSpace(route.EstimateGasFee),
		EstimatedGasSymbol: nativeSymbolForChainConfig(chain, cc),
		FeeSummary:         feeSummary,
		PriceImpactPercent: strings.TrimSpace(route.PriceImpactPercent),
		RouteSummary:       buildRouteSummary(hops),
		Fees:               fees,
		Route:              hops,
	}
}

func buildOKXProviderQuote(
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
) swapProviderQuote {
	okxService := exchange.NewOKXDexService()
	resp, err := okxService.GetSwapData(exchange.SwapRequest{
		ChainID:           fmt.Sprintf("%d", cc.ChainID),
		FromTokenAddress:  okxWalletSwapTokenAddress(fromTokenRaw, fromToken),
		ToTokenAddress:    okxWalletSwapTokenAddress(toTokenRaw, toToken),
		Amount:            amount.String(),
		Slippage:          slippageDecimal,
		UserWalletAddress: walletAddr.Hex(),
	})
	if err != nil {
		return newUnavailableSwapProviderQuote("okx", chain, err)
	}
	if resp == nil || len(resp.Data) == 0 {
		return newUnavailableSwapProviderQuote("okx", chain, fmt.Errorf("empty OKX quote response"))
	}

	txObj := resp.Data[0].Tx
	if !common.IsHexAddress(strings.TrimSpace(txObj.To)) {
		return newUnavailableSwapProviderQuote("okx", chain, fmt.Errorf("OKX tx.to invalid: %s", txObj.To))
	}
	if len(common.FromHex(strings.TrimSpace(txObj.Data))) == 0 {
		return newUnavailableSwapProviderQuote("okx", chain, fmt.Errorf("OKX did not return executable tx.data; this route may not support OKX referrer fee"))
	}

	toAmount := strings.TrimSpace(resp.Data[0].RouterResult.ToTokenAmount)
	estimatedGas := strings.TrimSpace(txObj.Gas)
	estimatedGasPrice := strings.TrimSpace(txObj.GasPrice)
	minAmount := minAmountWithSlippage(toAmount, slippagePercent)
	route := extractOKXRouteHops(resp.Data[0].RouterResult.DexRouterList)
	gasNative, gasUSD := estimateGasCosts(chain, estimatedGas, estimatedGasPrice, client)

	fees := []swapFeeDetail{{
		Label:       "正滑点手续费",
		Category:    "rule",
		Rate:        "10%",
		Description: "仅对正滑点部分收取 10%",
	}}

	return swapProviderQuote{
		Provider:           "okx",
		ProviderLabel:      "OKX",
		Status:             "available",
		CanExecute:         true,
		FromAmount:         amount.String(),
		GrossToAmount:      toAmount,
		GrossToAmountFloat: formatQuoteAmount(toAmount, toDecimals),
		NetToAmount:        toAmount,
		NetToAmountFloat:   formatQuoteAmount(toAmount, toDecimals),
		MinToAmount:        minAmount,
		MinToAmountFloat:   formatQuoteAmount(minAmount, toDecimals),
		EstimatedGas:       estimatedGas,
		EstimatedGasNative: gasNative,
		EstimatedGasUSD:    gasUSD,
		EstimatedGasSymbol: nativeSymbolForChainConfig(chain, cc),
		FeeRule:            "正滑点部分收取 10%",
		FeeSummary:         "正滑点部分 10%",
		RouteSummary:       buildRouteSummary(route),
		Fees:               fees,
		Route:              route,
	}
}

func extractBinanceRouteHops(routes []exchange.BinanceDexRoute) []swapRouteHop {
	hops := make([]swapRouteHop, 0, len(routes))
	for idx, route := range routes {
		source := strings.TrimSpace(route.DexProtocol.DexName)
		if source == "" {
			source = strings.TrimSpace(route.DexProtocol.DexProtocol)
		}
		hops = append(hops, swapRouteHop{
			Index:      idx + 1,
			Source:     source,
			FromToken:  strings.TrimSpace(route.FromToken.TokenContractAddress),
			FromSymbol: strings.TrimSpace(route.FromToken.TokenSymbol),
			ToToken:    strings.TrimSpace(route.ToToken.TokenContractAddress),
			ToSymbol:   strings.TrimSpace(route.ToToken.TokenSymbol),
			Share:      strings.TrimSpace(route.DexProtocol.Percent),
		})
	}
	return hops
}

func extractOKXRouteHops(raw json.RawMessage) []swapRouteHop {
	type okxRouteToken struct {
		TokenContractAddress string `json:"tokenContractAddress"`
		TokenSymbol          string `json:"tokenSymbol"`
		Symbol               string `json:"symbol"`
	}
	type okxRouteProtocol struct {
		DexName     string `json:"dexName"`
		DexProtocol string `json:"dexProtocol"`
		Percent     string `json:"percent"`
	}
	type okxSubRoute struct {
		FromToken   okxRouteToken      `json:"fromToken"`
		ToToken     okxRouteToken      `json:"toToken"`
		DexProtocol []okxRouteProtocol `json:"dexProtocol"`
	}
	type okxDexRoute struct {
		SubRouterList []okxSubRoute      `json:"subRouterList"`
		DexProtocol   []okxRouteProtocol `json:"dexProtocol"`
	}
	if len(raw) == 0 {
		return nil
	}
	var routes []okxDexRoute
	if err := json.Unmarshal(raw, &routes); err != nil {
		return nil
	}
	hops := make([]swapRouteHop, 0, len(routes))
	index := 1
	for _, route := range routes {
		if len(route.SubRouterList) > 0 {
			for _, sub := range route.SubRouterList {
				source := ""
				share := ""
				if len(sub.DexProtocol) > 0 {
					source = strings.TrimSpace(sub.DexProtocol[0].DexName)
					if source == "" {
						source = strings.TrimSpace(sub.DexProtocol[0].DexProtocol)
					}
					share = strings.TrimSpace(sub.DexProtocol[0].Percent)
				}
				fromSymbol := strings.TrimSpace(sub.FromToken.TokenSymbol)
				if fromSymbol == "" {
					fromSymbol = strings.TrimSpace(sub.FromToken.Symbol)
				}
				toSymbol := strings.TrimSpace(sub.ToToken.TokenSymbol)
				if toSymbol == "" {
					toSymbol = strings.TrimSpace(sub.ToToken.Symbol)
				}
				hops = append(hops, swapRouteHop{
					Index:      index,
					Source:     source,
					FromToken:  strings.TrimSpace(sub.FromToken.TokenContractAddress),
					FromSymbol: fromSymbol,
					ToToken:    strings.TrimSpace(sub.ToToken.TokenContractAddress),
					ToSymbol:   toSymbol,
					Share:      share,
				})
				index++
			}
			continue
		}
		if len(route.DexProtocol) > 0 {
			source := strings.TrimSpace(route.DexProtocol[0].DexName)
			if source == "" {
				source = strings.TrimSpace(route.DexProtocol[0].DexProtocol)
			}
			hops = append(hops, swapRouteHop{
				Index:  index,
				Source: source,
				Share:  strings.TrimSpace(route.DexProtocol[0].Percent),
			})
			index++
		}
	}
	return hops
}
