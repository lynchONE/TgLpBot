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
	RouteSummary       string          `json:"route_summary,omitempty"`
	Fees               []swapFeeDetail `json:"fees,omitempty"`
	Route              []swapRouteHop  `json:"route,omitempty"`
}

func swapProviderLabel(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "okx":
		return "OKX"
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

	toAmount := strings.TrimSpace(resp.Data[0].RouterResult.ToTokenAmount)
	estimatedGas := strings.TrimSpace(resp.Data[0].Tx.Gas)
	estimatedGasPrice := strings.TrimSpace(resp.Data[0].Tx.GasPrice)
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

func buildZeroXProviderQuote(
	chain string,
	cc config.ChainConfig,
	client *ethclient.Client,
	walletAddr common.Address,
	fromToken common.Address,
	toToken common.Address,
	amount *big.Int,
	slippagePercent float64,
	toDecimals int,
) swapProviderQuote {
	if config.AppConfig != nil {
		recipient := strings.TrimSpace(config.AppConfig.ZeroXSwapFeeRecipient)
		if recipient == "" {
			recipient = strings.TrimSpace(config.AppConfig.AdminWalletAddress)
		}
		if recipient == "" {
			return newUnavailableSwapProviderQuote("0x", chain, fmt.Errorf("0x fee recipient not configured"))
		}
	}

	quote, err := exchange.NewZeroXSwapService().GetAllowanceHolderQuote(exchange.ZeroXQuoteRequest{
		ChainID:     fmt.Sprintf("%d", cc.ChainID),
		SellToken:   walletSwapProviderTokenAddress(fromToken),
		BuyToken:    walletSwapProviderTokenAddress(toToken),
		SellAmount:  amount.String(),
		Taker:       walletAddr.Hex(),
		SlippageBps: int(slippagePercent * 100),
	})
	if err != nil {
		return newUnavailableSwapProviderQuote("0x", chain, err)
	}
	if quote == nil {
		return newUnavailableSwapProviderQuote("0x", chain, fmt.Errorf("empty 0x quote response"))
	}

	netAmount := strings.TrimSpace(quote.BuyAmount)
	minAmount := strings.TrimSpace(quote.MinBuyAmount)
	if minAmount == "" {
		minAmount = minAmountWithSlippage(netAmount, slippagePercent)
	}
	route := extractZeroXRouteHops(quote.Route)
	gasNative, gasUSD := estimateGasCosts(chain, strings.TrimSpace(quote.Transaction.Gas), strings.TrimSpace(quote.Transaction.GasPrice), client)
	fees, feeSummary := buildZeroXFeeDetails(quote.Fees, toToken, toDecimals)

	return swapProviderQuote{
		Provider:           "0x",
		ProviderLabel:      "0x",
		Status:             "available",
		CanExecute:         true,
		FromAmount:         amount.String(),
		NetToAmount:        netAmount,
		NetToAmountFloat:   formatQuoteAmount(netAmount, toDecimals),
		MinToAmount:        minAmount,
		MinToAmountFloat:   formatQuoteAmount(minAmount, toDecimals),
		EstimatedGas:       strings.TrimSpace(quote.Transaction.Gas),
		EstimatedGasNative: gasNative,
		EstimatedGasUSD:    gasUSD,
		EstimatedGasSymbol: nativeSymbolForChainConfig(chain, cc),
		FeeRule:            "交易额 0.15%",
		FeeSummary:         feeSummary,
		RouteSummary:       buildRouteSummary(route),
		Fees:               fees,
		Route:              route,
	}
}

func buildLIFIProviderQuote(
	chain string,
	cc config.ChainConfig,
	walletAddr common.Address,
	fromToken common.Address,
	toToken common.Address,
	amount *big.Int,
	slippagePercent float64,
	toDecimals int,
) swapProviderQuote {
	quote, err := exchange.NewLIFISwapService().GetQuote(exchange.LIFIQuoteRequest{
		FromChainID: fmt.Sprintf("%d", cc.ChainID),
		ToChainID:   fmt.Sprintf("%d", cc.ChainID),
		FromToken:   exchange.LIFINormalizeTokenAddress(walletSwapProviderTokenAddress(fromToken)),
		ToToken:     exchange.LIFINormalizeTokenAddress(walletSwapProviderTokenAddress(toToken)),
		FromAmount:  amount.String(),
		FromAddress: walletAddr.Hex(),
		ToAddress:   walletAddr.Hex(),
		Slippage:    slippagePercent / 100,
	})
	if err != nil {
		return newUnavailableSwapProviderQuote("li.fi", chain, err)
	}
	if quote == nil {
		return newUnavailableSwapProviderQuote("li.fi", chain, fmt.Errorf("empty LI.FI quote response"))
	}

	netAmount := strings.TrimSpace(quote.Estimate.ToAmount)
	minAmount := strings.TrimSpace(quote.Estimate.ToAmountMin)
	if minAmount == "" {
		minAmount = minAmountWithSlippage(netAmount, slippagePercent)
	}
	fees, feeSummary := buildLIFIFeeDetails(quote.Estimate.FeeCosts)
	gasUnits, gasNative, gasUSD, gasSymbol := buildLIFIGasDetails(chain, cc, quote.Estimate.GasCosts, quote.TransactionRequest.GasLimit)
	route := extractLIFIRouteHops(quote)

	return swapProviderQuote{
		Provider:           "li.fi",
		ProviderLabel:      "LI.FI",
		Status:             "available",
		CanExecute:         true,
		FromAmount:         amount.String(),
		NetToAmount:        netAmount,
		NetToAmountFloat:   formatQuoteAmount(netAmount, toDecimals),
		MinToAmount:        minAmount,
		MinToAmountFloat:   formatQuoteAmount(minAmount, toDecimals),
		EstimatedGas:       gasUnits,
		EstimatedGasNative: gasNative,
		EstimatedGasUSD:    gasUSD,
		EstimatedGasSymbol: gasSymbol,
		FeeRule:            "交易额 0.25%",
		FeeSummary:         feeSummary,
		RouteSummary:       buildRouteSummary(route),
		Fees:               fees,
		Route:              route,
	}
}

func buildZeroXFeeDetails(fees exchange.ZeroXFees, toToken common.Address, toDecimals int) ([]swapFeeDetail, string) {
	items := make([]swapFeeDetail, 0, 2)
	summary := "交易额 0.15%"

	appendFee := func(label string, fee *exchange.ZeroXFee) {
		if fee == nil {
			return
		}
		item := swapFeeDetail{
			Label:       label,
			Category:    "provider",
			Amount:      strings.TrimSpace(fee.Amount),
			Token:       strings.TrimSpace(fee.Token),
			TokenSymbol: strings.TrimSpace(fee.Type),
		}
		if item.TokenSymbol == "" && strings.EqualFold(item.Token, toToken.Hex()) {
			item.TokenSymbol = ""
		}
		if item.Amount != "" && strings.EqualFold(item.Token, toToken.Hex()) {
			item.AmountFloat = formatQuoteAmount(item.Amount, toDecimals)
			if item.AmountFloat != "" {
				summary = fmt.Sprintf("%s (%s)", summary, item.AmountFloat)
			}
		}
		items = append(items, item)
	}

	appendFee("Integrator Fee", fees.IntegratorFee)
	for i := range fees.IntegratorFees {
		item := fees.IntegratorFees[i]
		appendFee("Integrator Fee", &item)
	}
	appendFee("0x Fee", fees.ZeroExFee)
	if len(items) == 0 {
		items = append(items, swapFeeDetail{
			Label:       "Integrator Fee",
			Category:    "provider",
			Rate:        "0.15%",
			Description: "按交易额收取 0.15%",
		})
	}
	return items, summary
}

func buildLIFIFeeDetails(costs []exchange.LIFIFeeCost) ([]swapFeeDetail, string) {
	items := make([]swapFeeDetail, 0, len(costs))
	summary := "交易额 0.25%"
	for _, cost := range costs {
		item := swapFeeDetail{
			Label:       strings.TrimSpace(cost.Name),
			Category:    strings.TrimSpace(cost.Type),
			Amount:      strings.TrimSpace(cost.Amount),
			AmountFloat: formatQuoteAmount(cost.Amount, cost.Token.Decimals),
			Token:       strings.TrimSpace(cost.Token.Address),
			TokenSymbol: strings.TrimSpace(cost.Token.Symbol),
		}
		if item.Label == "" {
			item.Label = "Fee"
		}
		items = append(items, item)
	}
	if len(items) > 0 {
		for _, item := range items {
			if item.AmountFloat != "" && item.TokenSymbol != "" {
				summary = fmt.Sprintf("%s (%s %s)", summary, item.AmountFloat, item.TokenSymbol)
				break
			}
		}
	} else {
		items = append(items, swapFeeDetail{
			Label:       "Integrator Fee",
			Category:    "provider",
			Rate:        "0.25%",
			Description: "按交易额收取 0.25%",
		})
	}
	return items, summary
}

func buildLIFIGasDetails(chain string, cc config.ChainConfig, gasCosts []exchange.LIFIGasCost, fallbackGasLimit string) (string, float64, float64, string) {
	var (
		gasUnits  string
		gasNative float64
		gasUSD    float64
		symbol    = nativeSymbolForChainConfig(chain, cc)
	)
	for _, cost := range gasCosts {
		if strings.TrimSpace(cost.Estimate) != "" && gasUnits == "" {
			gasUnits = strings.TrimSpace(cost.Estimate)
		}
		if strings.TrimSpace(cost.Amount) != "" && cost.Token.Decimals > 0 {
			value := amountToFloat(strings.TrimSpace(cost.Amount), cost.Token.Decimals)
			if value > 0 {
				gasNative += value
			}
		}
		if strings.TrimSpace(cost.AmountUSD) != "" {
			if usd, ok := new(big.Float).SetString(strings.TrimSpace(cost.AmountUSD)); ok {
				value, _ := usd.Float64()
				gasUSD += value
			}
		}
		if strings.TrimSpace(cost.Token.Symbol) != "" {
			symbol = strings.TrimSpace(cost.Token.Symbol)
		}
	}
	if gasUnits == "" {
		gasUnits = strings.TrimSpace(fallbackGasLimit)
	}
	return gasUnits, gasNative, gasUSD, symbol
}

func extractZeroXRouteHops(route exchange.ZeroXRoute) []swapRouteHop {
	tokenSymbols := make(map[string]string, len(route.Tokens))
	for _, token := range route.Tokens {
		addr := strings.ToLower(strings.TrimSpace(token.Address))
		if addr == "" {
			continue
		}
		tokenSymbols[addr] = strings.TrimSpace(token.Symbol)
	}
	hops := make([]swapRouteHop, 0, len(route.Fills))
	for idx, fill := range route.Fills {
		fromAddr := strings.TrimSpace(fill.FromTokenAddress)
		toAddr := strings.TrimSpace(fill.ToTokenAddress)
		hops = append(hops, swapRouteHop{
			Index:      idx + 1,
			Source:     strings.TrimSpace(fill.Source),
			FromToken:  fromAddr,
			FromSymbol: tokenSymbols[strings.ToLower(fromAddr)],
			ToToken:    toAddr,
			ToSymbol:   tokenSymbols[strings.ToLower(toAddr)],
			Share:      strings.TrimSpace(fill.ProportionBps),
		})
	}
	return hops
}

func extractLIFIRouteHops(quote *exchange.LIFIQuoteResponse) []swapRouteHop {
	if quote == nil {
		return nil
	}
	steps := quote.IncludedSteps
	if len(steps) == 0 {
		steps = []exchange.LIFIIncludedStep{{
			Tool:        quote.Tool,
			ToolDetails: quote.ToolDetails,
			Action:      quote.Action,
			Estimate:    quote.Estimate,
		}}
	}
	hops := make([]swapRouteHop, 0, len(steps))
	for idx, step := range steps {
		source := strings.TrimSpace(step.ToolDetails.Name)
		if source == "" {
			source = strings.TrimSpace(step.Tool)
		}
		hop := swapRouteHop{
			Index:      idx + 1,
			Source:     source,
			Tool:       strings.TrimSpace(step.Tool),
			FromToken:  strings.TrimSpace(step.Action.FromToken.Address),
			FromSymbol: strings.TrimSpace(step.Action.FromToken.Symbol),
			ToToken:    strings.TrimSpace(step.Action.ToToken.Address),
			ToSymbol:   strings.TrimSpace(step.Action.ToToken.Symbol),
		}
		hops = append(hops, hop)
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
