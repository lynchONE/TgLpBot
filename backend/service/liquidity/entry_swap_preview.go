package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/exchange"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

type EntrySwapPreview struct {
	Required                     bool
	FromTokenAddress             string
	FromTokenSymbol              string
	ToTokenAddress               string
	ToTokenSymbol                string
	AmountIn                     string
	AmountInRaw                  string
	ExpectedAmountOut            string
	ExpectedAmountOutRaw         string
	RecommendedSlippageTolerance float64
	CurrentSlippageTolerance     float64
}

func sanitizeSlippageTolerance(value float64, fallback float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		value = fallback
	}
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	return value
}

func recommendedEntrySwapSlippage(
	taskSlippage float64,
	tokenIn common.Address,
	tokenOut common.Address,
	fromSymbol string,
	toSymbol string,
	cc config.ChainConfig,
) float64 {
	if isStableEntryToken(fromSymbol, tokenIn, cc) && isStableEntryToken(toSymbol, tokenOut, cc) {
		return 0.1
	}
	if taskSlippage > 0 {
		return sanitizeSlippageTolerance(taskSlippage, 0.5)
	}
	return 0.5
}

func resolveEntrySwapSlippage(
	taskSlippage float64,
	override *float64,
	tokenIn common.Address,
	tokenOut common.Address,
	fromSymbol string,
	toSymbol string,
	cc config.ChainConfig,
) float64 {
	recommended := recommendedEntrySwapSlippage(taskSlippage, tokenIn, tokenOut, fromSymbol, toSymbol, cc)
	if override == nil {
		return recommended
	}
	return sanitizeSlippageTolerance(*override, recommended)
}

func formatPreviewAmount(amount *big.Int, decimals int) string {
	if amount == nil || amount.Sign() <= 0 {
		return "0"
	}
	value := amountToFloat(amount, decimals)
	text := strconv.FormatFloat(value, 'f', 8, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" {
		return "0"
	}
	return text
}

func minBigInt(a *big.Int, b *big.Int) *big.Int {
	if a == nil {
		return cloneBig(b)
	}
	if b == nil {
		return cloneBig(a)
	}
	if a.Cmp(b) <= 0 {
		return cloneBig(a)
	}
	return cloneBig(b)
}

func (s *LiquidityService) quoteOKXSwapExactIn(
	exec chainexec.EVMExecutor,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (*big.Int, error) {
	if exec == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	if amountIn == nil || amountIn.Sign() <= 0 {
		return big.NewInt(0), nil
	}
	if tokenIn == tokenOut {
		return cloneBig(amountIn), nil
	}
	if s.okxService == nil {
		s.okxService = exchange.NewOKXDexService()
	}

	cc := exec.Config()
	swapReq := exchange.SwapRequest{
		ChainID:           fmt.Sprintf("%d", cc.ChainID),
		FromTokenAddress:  tokenIn.Hex(),
		ToTokenAddress:    tokenOut.Hex(),
		Amount:            amountIn.String(),
		Slippage:          s.okxSlippageDecimal(slippagePercent),
		UserWalletAddress: walletAddr.Hex(),
	}
	swapResp, err := s.okxService.GetSwapData(swapReq)
	if err != nil {
		return nil, err
	}
	if swapResp == nil || len(swapResp.Data) == 0 {
		return nil, fmt.Errorf("OKX swap response empty")
	}

	outStr := strings.TrimSpace(swapResp.Data[0].RouterResult.ToTokenAmount)
	if outStr == "" {
		return big.NewInt(0), nil
	}
	if strings.HasPrefix(outStr, "0x") || strings.HasPrefix(outStr, "0X") {
		out, ok := new(big.Int).SetString(strings.TrimPrefix(strings.TrimPrefix(outStr, "0x"), "0X"), 16)
		if !ok || out == nil {
			return nil, fmt.Errorf("invalid OKX quote output")
		}
		return out, nil
	}
	out, ok := new(big.Int).SetString(outStr, 10)
	if !ok || out == nil {
		return nil, fmt.Errorf("invalid OKX quote output")
	}
	return out, nil
}

func (s *LiquidityService) PreviewEntrySwap(
	task *models.StrategyTask,
	wallet *models.Wallet,
	taskSlippage float64,
	override *float64,
) (*EntrySwapPreview, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	if wallet == nil {
		return nil, fmt.Errorf("wallet is nil")
	}
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}

	task.Chain = config.NormalizeChain(task.Chain)
	exec, err := chainexec.GetEVM(task.Chain)
	if err != nil {
		return nil, err
	}
	client := exec.Client()
	if client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	cc := exec.Config()
	if !common.IsHexAddress(cc.StableAddress) {
		return nil, fmt.Errorf("stable address not set for chain=%s", exec.Chain())
	}

	plan, err := s.planEntryToken(task)
	if err != nil {
		return nil, err
	}
	if !plan.RequiresSwap {
		return &EntrySwapPreview{Required: false}, nil
	}

	walletAddr := s.walletService.GetWalletAddress(wallet)
	requestedAmount, err := convert.FloatToUnits(task.AmountUSDT, cc.StableDecimals)
	if err != nil {
		return nil, err
	}
	if requestedAmount == nil || requestedAmount.Sign() <= 0 {
		return &EntrySwapPreview{Required: false}, nil
	}

	if plan.EntryToken != (common.Address{}) {
		entryBalance, _ := blockchain.GetTokenBalanceWithClient(client, plan.EntryToken, walletAddr)
		if entryBalance != nil && entryBalance.Sign() > 0 {
			budgetCap, capErr := tokenBudgetUnits(client, exec.Chain(), plan.EntryToken, plan.EntrySymbol, cc, task.AmountUSDT)
			if capErr == nil && budgetCap != nil && budgetCap.Sign() > 0 {
				threshold := new(big.Int).Mul(budgetCap, big.NewInt(95))
				threshold.Div(threshold, big.NewInt(100))
				if entryBalance.Cmp(threshold) >= 0 {
					return &EntrySwapPreview{Required: false}, nil
				}
			}
		}
	}

	stableAddr := common.HexToAddress(cc.StableAddress)
	effectiveSlippage := resolveEntrySwapSlippage(
		taskSlippage,
		override,
		stableAddr,
		plan.EntryToken,
		cc.StableSymbol,
		plan.EntrySymbol,
		cc,
	)
	recommendedSlippage := recommendedEntrySwapSlippage(
		taskSlippage,
		stableAddr,
		plan.EntryToken,
		cc.StableSymbol,
		plan.EntrySymbol,
		cc,
	)

	expectedOut, err := s.quoteOKXSwapExactIn(exec, walletAddr, stableAddr, plan.EntryToken, requestedAmount, effectiveSlippage)
	if err != nil {
		return nil, err
	}

	entryDecimals := tokenDecimalsWithFallback(client, plan.EntryToken, cc.StableDecimals)
	return &EntrySwapPreview{
		Required:                     true,
		FromTokenAddress:             stableAddr.Hex(),
		FromTokenSymbol:              strings.ToUpper(strings.TrimSpace(cc.StableSymbol)),
		ToTokenAddress:               plan.EntryToken.Hex(),
		ToTokenSymbol:                strings.ToUpper(strings.TrimSpace(plan.EntrySymbol)),
		AmountIn:                     formatPreviewAmount(requestedAmount, cc.StableDecimals),
		AmountInRaw:                  requestedAmount.String(),
		ExpectedAmountOut:            formatPreviewAmount(expectedOut, entryDecimals),
		ExpectedAmountOutRaw:         expectedOut.String(),
		RecommendedSlippageTolerance: recommendedSlippage,
		CurrentSlippageTolerance:     effectiveSlippage,
	}, nil
}
