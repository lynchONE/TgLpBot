package liquidity

import (
	"TgLpBot/base/config"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/exchange"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// WalletSwapToUSDTReport is a best-effort report for swapping wallet tokens into USDT.
type WalletSwapToUSDTReport struct {
	WalletAddress string
	CandidateCnt  int
	Swapped       []string // "SYMBOL->USDT|txHash"
	Failed        []string // "SYMBOL->USDT|error"
}

type SwapSingleTokenResult struct {
	Provider      string
	TxHash        string
	AmountOut     *big.Int
	Receipt       *types.Receipt
	RouterAddress common.Address
}

type walletSwapOKXToken struct {
	Address    common.Address
	Symbol     string
	Balance    string
	RawBalance *big.Int
	Decimals   int
	ValueUSDT  float64
}

// SwapWalletOtherTokensToUSDT swaps non-stable ERC20 tokens discovered from OKX wallet balances.
func (s *LiquidityService) SwapWalletOtherTokensToUSDT(userID uint, slippagePercent float64) (*WalletSwapToUSDTReport, error) {
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	return s.SwapWalletOtherTokensToUSDTForChain(userID, "bsc", slippagePercent)
}

func (s *LiquidityService) SwapWalletOtherTokensToUSDTForChain(userID uint, chain string, slippagePercent float64) (*WalletSwapToUSDTReport, error) {
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}

	chain = config.NormalizeChain(chain)
	exec, err := chainexec.GetEVM(chain)
	if err != nil {
		return nil, err
	}
	cc := exec.Config()
	client := exec.Client()
	if client == nil {
		return nil, fmt.Errorf("blockchain client not initialized for chain=%s", exec.Chain())
	}
	if !common.IsHexAddress(cc.StableAddress) {
		return nil, fmt.Errorf("stable address not set for chain=%s", exec.Chain())
	}

	wallet, err := s.walletService.GetDefaultWallet(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}
	privateKeyHex, err := s.walletService.GetPrivateKey(wallet)
	if err != nil {
		return nil, fmt.Errorf("failed to get private key: %w", err)
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	walletAddr := s.walletService.GetWalletAddress(wallet)
	usdtAddr := common.HexToAddress(cc.StableAddress)

	candidates, err := s.collectOKXWalletSwapTokens(exec.Chain(), cc, walletAddr, 0)
	if err != nil {
		return nil, err
	}

	report := &WalletSwapToUSDTReport{
		WalletAddress: walletAddr.Hex(),
		CandidateCnt:  len(candidates),
		Swapped:       make([]string, 0),
		Failed:        make([]string, 0),
	}

	for _, candidate := range candidates {
		if candidate.Address == (common.Address{}) || candidate.RawBalance == nil || candidate.RawBalance.Sign() <= 0 {
			continue
		}

		txHash, serr := s.swapDeltaToUSDTWithHash(exec, privateKey, walletAddr, candidate.Address, usdtAddr, candidate.RawBalance, slippagePercent)
		if serr != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%s->USDT|%v", candidate.Symbol, serr))
			continue
		}
		if strings.TrimSpace(txHash) == "" {
			report.Failed = append(report.Failed, fmt.Sprintf("%s->USDT|empty tx hash", candidate.Symbol))
			continue
		}

		report.Swapped = append(report.Swapped, fmt.Sprintf("%s->USDT|%s", candidate.Symbol, txHash))
	}

	return report, nil
}

func excludedSwapTokens(cc config.ChainConfig) map[common.Address]struct{} {
	excluded := make(map[common.Address]struct{})

	// Stable coins
	if common.IsHexAddress(cc.StableAddress) {
		excluded[common.HexToAddress(cc.StableAddress)] = struct{}{}
	}
	if common.IsHexAddress(cc.USDCAddress) {
		excluded[common.HexToAddress(cc.USDCAddress)] = struct{}{}
	}
	if common.IsHexAddress(cc.BUSDAddress) {
		excluded[common.HexToAddress(cc.BUSDAddress)] = struct{}{}
	}

	// Treat wrapped native token as the gas asset equivalent (avoid swapping user's gas token).
	if common.IsHexAddress(cc.WrappedNativeAddress) {
		excluded[common.HexToAddress(cc.WrappedNativeAddress)] = struct{}{}
	}
	return excluded
}

// WalletTokenInfo 代币信息结构
type WalletTokenInfo struct {
	Address   common.Address
	Symbol    string
	Balance   string
	ValueUSDT float64
}

// ScanWalletTokensForSwap 扫描钱包中符合兑换条件的代币
// 返回价值大于 minValueUSDT 且非 BNB/WBNB/稳定币的代币列表
func (s *LiquidityService) ScanWalletTokensForSwap(userID uint, minValueUSDT float64) ([]WalletTokenInfo, error) {
	return s.ScanWalletTokensForSwapForChain(userID, "bsc", minValueUSDT)
}

func (s *LiquidityService) ScanWalletTokensForSwapForChain(userID uint, chain string, minValueUSDT float64) ([]WalletTokenInfo, error) {
	return s.ScanWalletTokensForSwapForChainWithWallet(userID, 0, chain, minValueUSDT)
}

// ScanWalletTokensForSwapForChainWithWallet scans OKX wallet balances for swappable tokens.
func (s *LiquidityService) ScanWalletTokensForSwapForChainWithWallet(userID uint, walletID uint, chain string, minValueUSDT float64) ([]WalletTokenInfo, error) {
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}

	exec, err := chainexec.GetEVM(chain)
	if err != nil {
		return nil, err
	}
	cc := exec.Config()
	if !common.IsHexAddress(cc.StableAddress) {
		return nil, fmt.Errorf("stable address not set for chain=%s", exec.Chain())
	}

	wallet, err := s.walletService.ResolveTaskWallet(userID, walletID, "")
	if err != nil {
		return nil, fmt.Errorf("get wallet failed: %w", err)
	}

	tokens, err := s.collectOKXWalletSwapTokens(exec.Chain(), cc, s.walletService.GetWalletAddress(wallet), minValueUSDT)
	if err != nil {
		return nil, err
	}

	out := make([]WalletTokenInfo, 0, len(tokens))
	for _, token := range tokens {
		out = append(out, WalletTokenInfo{
			Address:   token.Address,
			Symbol:    token.Symbol,
			Balance:   token.Balance,
			ValueUSDT: token.ValueUSDT,
		})
	}
	return out, nil
}

func (s *LiquidityService) collectOKXWalletSwapTokens(chain string, cc config.ChainConfig, walletAddr common.Address, minValueUSDT float64) ([]walletSwapOKXToken, error) {
	if s == nil || s.okxService == nil {
		return nil, fmt.Errorf("OKX service unavailable")
	}
	if cc.ChainID <= 0 {
		return nil, fmt.Errorf("invalid chain id")
	}

	resp, err := s.okxService.GetAllTokenBalancesByAddress(nil, exchange.BalanceAllTokenBalancesRequest{
		Address: walletAddr.Hex(),
		Chains:  []string{strconv.FormatInt(cc.ChainID, 10)},
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Data) == 0 {
		return nil, fmt.Errorf("empty OKX balance response")
	}

	excluded := excludedSwapTokens(cc)
	out := make([]walletSwapOKXToken, 0)
	seen := make(map[common.Address]struct{})
	for _, data := range resp.Data {
		for _, asset := range data.TokenAssets {
			tokenAddr, ok := normalizeOKXWalletSwapTokenAddress(asset)
			if !ok || tokenAddr == (common.Address{}) {
				continue
			}
			if _, ok := excluded[tokenAddr]; ok {
				continue
			}
			if _, ok := seen[tokenAddr]; ok {
				continue
			}
			rawBalance, ok := parseOKXWalletSwapRawBalance(asset.RawBalance)
			if !ok || rawBalance.Sign() <= 0 {
				continue
			}

			decimals := okxWalletSwapTokenDecimals(asset, rawBalance)
			valueUSDT := okxWalletSwapTokenValueUSDT(rawBalance, decimals, asset.TokenPrice, tokenAddr, cc)
			if valueUSDT < minValueUSDT {
				continue
			}

			symbol := firstOKXWalletSwapString(asset.Symbol, asset.TokenSymbol)
			if symbol == "" {
				symbol = shortOKXWalletSwapTokenLabel(tokenAddr)
			}
			balance := strings.TrimSpace(asset.Balance)
			if balance == "" {
				balance = formatOKXWalletSwapRawAmount(rawBalance, decimals)
			}

			seen[tokenAddr] = struct{}{}
			out = append(out, walletSwapOKXToken{
				Address:    tokenAddr,
				Symbol:     symbol,
				Balance:    balance,
				RawBalance: rawBalance,
				Decimals:   decimals,
				ValueUSDT:  valueUSDT,
			})
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ValueUSDT == out[j].ValueUSDT {
			return out[i].Symbol < out[j].Symbol
		}
		return out[i].ValueUSDT > out[j].ValueUSDT
	})
	return out, nil
}

func normalizeOKXWalletSwapTokenAddress(asset exchange.BalanceTokenAsset) (common.Address, bool) {
	raw := firstOKXWalletSwapString(asset.TokenContractAddress, asset.TokenAddress)
	if raw == "" || strings.EqualFold(raw, "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee") {
		return common.Address{}, false
	}
	if !common.IsHexAddress(raw) {
		return common.Address{}, false
	}
	return common.HexToAddress(raw), true
}

func parseOKXWalletSwapRawBalance(raw string) (*big.Int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	amount, ok := new(big.Int).SetString(raw, 10)
	if !ok || amount == nil {
		return nil, false
	}
	return amount, true
}

func okxWalletSwapTokenDecimals(asset exchange.BalanceTokenAsset, rawBalance *big.Int) int {
	for _, raw := range []string{asset.TokenDecimal, asset.Decimals} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		decimals, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		if decimals >= 0 && decimals <= 36 {
			return decimals
		}
	}
	return inferOKXWalletSwapDecimals(rawBalance, asset.Balance)
}

func inferOKXWalletSwapDecimals(rawBalance *big.Int, balanceText string) int {
	if rawBalance == nil || rawBalance.Sign() <= 0 {
		return 18
	}
	balanceText = strings.TrimSpace(balanceText)
	if balanceText == "" || strings.ContainsAny(balanceText, "eE") {
		return 18
	}
	human, ok := new(big.Rat).SetString(balanceText)
	if !ok || human == nil || human.Sign() <= 0 {
		return 18
	}
	scale := new(big.Rat).Quo(new(big.Rat).SetInt(rawBalance), human)
	for decimals := 0; decimals <= 36; decimals++ {
		pow := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
		if scale.Cmp(new(big.Rat).SetInt(pow)) == 0 {
			return decimals
		}
	}
	return 18
}

func okxWalletSwapTokenValueUSDT(rawBalance *big.Int, decimals int, tokenPrice string, tokenAddr common.Address, cc config.ChainConfig) float64 {
	if rawBalance == nil || rawBalance.Sign() <= 0 {
		return 0
	}
	if walletSwapIsStableToken(tokenAddr, cc) {
		return toFloat64(rawBalance, decimals)
	}
	tokenPrice = strings.TrimSpace(tokenPrice)
	if tokenPrice == "" {
		return 0
	}
	price, err := strconv.ParseFloat(tokenPrice, 64)
	if err != nil || price <= 0 {
		return 0
	}
	return toFloat64(rawBalance, decimals) * price
}

func walletSwapIsStableToken(tokenAddr common.Address, cc config.ChainConfig) bool {
	for _, raw := range []string{cc.StableAddress, cc.USDTAddress, cc.USDCAddress, cc.BUSDAddress} {
		if common.IsHexAddress(raw) && tokenAddr == common.HexToAddress(raw) {
			return true
		}
	}
	return false
}

func firstOKXWalletSwapString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func shortOKXWalletSwapTokenLabel(tokenAddr common.Address) string {
	addr := tokenAddr.Hex()
	if len(addr) < 10 {
		return "TOKEN"
	}
	return strings.ToUpper(addr[:6] + "..." + addr[len(addr)-4:])
}

func formatOKXWalletSwapRawAmount(amount *big.Int, decimals int) string {
	if amount == nil || amount.Sign() == 0 {
		return "0"
	}
	if decimals <= 0 {
		return amount.String()
	}
	raw := amount.String()
	if len(raw) <= decimals {
		frac := strings.Repeat("0", decimals-len(raw)) + raw
		frac = strings.TrimRight(frac, "0")
		if frac == "" {
			return "0"
		}
		return "0." + frac
	}
	intPart := raw[:len(raw)-decimals]
	fracPart := strings.TrimRight(raw[len(raw)-decimals:], "0")
	if fracPart == "" {
		return intPart
	}
	return intPart + "." + fracPart
}

func toFloat64(val *big.Int, decimals int) float64 {
	if val == nil {
		return 0
	}
	f := new(big.Float).SetInt(val)
	divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	f.Quo(f, divisor)
	result, _ := f.Float64()
	return result
}

// SwapSingleToken 执行单个代币兑换（任意代币对），返回交易哈希
func (s *LiquidityService) SwapSingleToken(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (string, error) {
	r, err := s.executeOKXSwapExactIn(exec, privateKey, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent)
	if r == nil {
		return "", err
	}
	return r.TxHash, err
}

func (s *LiquidityService) SwapSingleTokenDetailed(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (*SwapSingleTokenResult, error) {
	r, err := s.executeOKXSwapExactIn(exec, privateKey, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent)
	if r == nil {
		return nil, err
	}

	amountOut := big.NewInt(0)
	if r.DeltaOut != nil {
		amountOut = new(big.Int).Set(r.DeltaOut)
	}

	return &SwapSingleTokenResult{
		Provider:      "okx",
		TxHash:        r.TxHash,
		AmountOut:     amountOut,
		Receipt:       r.Receipt,
		RouterAddress: r.To,
	}, err
}
