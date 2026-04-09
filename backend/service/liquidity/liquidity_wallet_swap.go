package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/exchange"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// WalletSwapToUSDTReport is a best-effort report for swapping wallet tokens into USDT.
type WalletSwapToUSDTReport struct {
	WalletAddress string
	CandidateCnt  int
	Swapped       []string // "SYMBOL->USDT|txHash"
	Failed        []string // "SYMBOL->USDT|error"
}

// SwapWalletOtherTokensToUSDT swaps all known non-stable ERC20 tokens (excluding WBNB) in the default wallet to USDT.
// Tokens are discovered from the user's task history (StrategyTask.token0/token1).
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
	if database.DB == nil {
		return nil, fmt.Errorf("db not initialized")
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

	excluded := excludedSwapTokens(cc)
	excluded[usdtAddr] = struct{}{}

	candidates, err := s.collectUserTaskTokensForChain(userID, exec.Chain())
	if err != nil {
		return nil, err
	}

	report := &WalletSwapToUSDTReport{
		WalletAddress: walletAddr.Hex(),
		CandidateCnt:  len(candidates),
		Swapped:       make([]string, 0),
		Failed:        make([]string, 0),
	}

	for tokenAddr, symGuess := range candidates {
		if tokenAddr == (common.Address{}) {
			continue
		}
		if _, ok := excluded[tokenAddr]; ok {
			continue
		}

		bal, berr := blockchain.GetTokenBalanceWithClient(client, tokenAddr, walletAddr)
		if berr != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%s->USDT|get balance failed: %v", tokenLabel(client, tokenAddr, symGuess), berr))
			continue
		}
		if bal == nil || bal.Sign() <= 0 {
			continue
		}

		txHash, serr := s.swapDeltaToUSDTWithHash(exec, privateKey, walletAddr, tokenAddr, usdtAddr, bal, slippagePercent)
		if serr != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%s->USDT|%v", tokenLabel(client, tokenAddr, symGuess), serr))
			continue
		}
		if strings.TrimSpace(txHash) == "" {
			report.Failed = append(report.Failed, fmt.Sprintf("%s->USDT|empty tx hash", tokenLabel(client, tokenAddr, symGuess)))
			continue
		}

		report.Swapped = append(report.Swapped, fmt.Sprintf("%s->USDT|%s", tokenLabel(client, tokenAddr, symGuess), txHash))
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

func tokenLabel(client *ethclient.Client, tokenAddr common.Address, symGuess string) string {
	sym := strings.TrimSpace(symGuess)
	if sym == "" && client != nil {
		if erc20, err := blockchain.NewERC20(tokenAddr, client); err == nil {
			if s2, err := erc20.Symbol(nil); err == nil {
				sym = strings.TrimSpace(s2)
			}
		}
	}
	if sym == "" {
		sym = tokenAddr.Hex()
	}
	return sym
}

func (s *LiquidityService) collectUserTaskTokensForChain(userID uint, chain string) (map[common.Address]string, error) {
	var tasks []models.StrategyTask
	if err := database.DB.Select(
		"id",
		"chain",
		"pool_version",
		"exchange",
		"pool_id",
		"token0_address",
		"token1_address",
		"token0_symbol",
		"token1_symbol",
		"v3_position_manager_address",
		"v3_token_id",
		"v4_token_id",
	).Where("user_id = ? AND chain = ?", userID, config.NormalizeChain(chain)).Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("query tasks failed: %w", err)
	}

	out := make(map[common.Address]string)
	for i := range tasks {
		task := &tasks[i]

		token0 := common.Address{}
		token1 := common.Address{}
		if common.IsHexAddress(task.Token0Address) {
			token0 = common.HexToAddress(task.Token0Address)
		}
		if common.IsHexAddress(task.Token1Address) {
			token1 = common.HexToAddress(task.Token1Address)
		}
		if token0 == (common.Address{}) || token1 == (common.Address{}) {
			if c0, c1, err := s.resolveTaskTokenAddresses(task); err == nil {
				if token0 == (common.Address{}) {
					token0 = c0
				}
				if token1 == (common.Address{}) {
					token1 = c1
				}
			} else {
				log.Printf("[Liquidity] Warning: resolve task tokens failed (task=%d): %v", task.ID, err)
			}
		}

		if token0 != (common.Address{}) {
			out[token0] = strings.TrimSpace(task.Token0Symbol)
		}
		if token1 != (common.Address{}) {
			out[token1] = strings.TrimSpace(task.Token1Symbol)
		}
	}

	return out, nil
}

// WalletTokenInfo 代币信息结构
type WalletTokenInfo struct {
	Address   common.Address
	Symbol    string
	Balance   string  // 人类可读的余额
	ValueUSDT float64 // USDT 价值
}

// ScanWalletTokensForSwap 扫描钱包中符合兑换条件的代币
// 返回价值大于 minValueUSDT 且非 BNB/WBNB/稳定币的代币列表
func (s *LiquidityService) ScanWalletTokensForSwap(userID uint, minValueUSDT float64) ([]WalletTokenInfo, error) {
	return s.ScanWalletTokensForSwapForChain(userID, "bsc", minValueUSDT)
}

func (s *LiquidityService) ScanWalletTokensForSwapForChain(userID uint, chain string, minValueUSDT float64) ([]WalletTokenInfo, error) {
	return s.ScanWalletTokensForSwapForChainWithWallet(userID, 0, chain, minValueUSDT)
}

func (s *LiquidityService) ScanWalletTokensForSwapForChainWithWallet(userID uint, walletID uint, chain string, minValueUSDT float64) ([]WalletTokenInfo, error) {
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	if database.DB == nil {
		return nil, fmt.Errorf("db not initialized")
	}

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

	wallet, err := s.walletService.ResolveTaskWallet(userID, walletID, "")
	if err != nil {
		return nil, fmt.Errorf("获取钱包失败: %w", err)
	}

	walletAddr := s.walletService.GetWalletAddress(wallet)
	usdtAddr := common.HexToAddress(cc.StableAddress)

	excluded := excludedSwapTokens(cc)
	excluded[usdtAddr] = struct{}{}

	candidates, err := s.collectUserTaskTokensForChain(userID, exec.Chain())
	if err != nil {
		return nil, err
	}

	var result []WalletTokenInfo

	for tokenAddr, symGuess := range candidates {
		if tokenAddr == (common.Address{}) {
			continue
		}
		if _, ok := excluded[tokenAddr]; ok {
			continue
		}

		bal, berr := blockchain.GetTokenBalanceWithClient(client, tokenAddr, walletAddr)
		if berr != nil {
			continue
		}
		if bal == nil || bal.Sign() <= 0 {
			continue
		}

		// 获取代币符号
		symbol := tokenLabel(client, tokenAddr, symGuess)

		// 获取代币小数位数
		decimals := 18
		if client != nil {
			if erc20, err := blockchain.NewERC20(tokenAddr, client); err == nil {
				if d, err := erc20.Decimals(nil); err == nil {
					decimals = int(d)
				}
			}
		}

		// 计算人类可读余额
		balFloat := toFloat64(bal, decimals)
		balanceStr := fmt.Sprintf("%.4f", balFloat)

		// 获取代币的 USDT 价值 (使用 OKX DEX 报价)
		valueUSDT := s.getTokenValueInStable(exec, tokenAddr, bal, walletAddr)

		// 只返回价值大于阈值的代币
		if valueUSDT >= minValueUSDT {
			result = append(result, WalletTokenInfo{
				Address:   tokenAddr,
				Symbol:    symbol,
				Balance:   balanceStr,
				ValueUSDT: valueUSDT,
			})
		}
	}

	return result, nil
}

// toFloat64 将 big.Int 转换为 float64
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

// getTokenValueInUSDT 获取代币的 USDT 价值
func (s *LiquidityService) getTokenValueInStable(exec chainexec.EVMExecutor, tokenAddr common.Address, amount *big.Int, walletAddr common.Address) float64 {
	if amount == nil || amount.Sign() <= 0 {
		return 0
	}
	if exec == nil {
		return 0
	}
	if s.okxService == nil {
		return 0
	}

	cc := exec.Config()
	if !common.IsHexAddress(cc.StableAddress) {
		return 0
	}
	stableAddr := common.HexToAddress(cc.StableAddress)
	chainID := fmt.Sprintf("%d", cc.ChainID)

	// 使用 OKX DEX 获取报价
	resp, err := s.okxService.GetSwapData(exchange.SwapRequest{
		ChainID:           chainID,
		FromTokenAddress:  tokenAddr.Hex(),
		ToTokenAddress:    stableAddr.Hex(),
		Amount:            amount.String(),
		Slippage:          "0.01", // 1% slippage for quote
		UserWalletAddress: walletAddr.Hex(),
	})
	if err != nil {
		log.Printf("[Liquidity] getTokenValueInUSDT quote failed for %s: %v", tokenAddr.Hex(), err)
		return 0
	}

	if resp == nil || len(resp.Data) == 0 {
		return 0
	}

	// 解析返回的 USDT 数量
	toAmountStr := strings.TrimSpace(resp.Data[0].RouterResult.ToTokenAmount)
	if toAmountStr == "" {
		return 0
	}
	toAmount, ok := new(big.Int).SetString(toAmountStr, 10)
	if !ok || toAmount.Sign() <= 0 {
		return 0
	}

	// USDT 精度是 18
	return toFloat64(toAmount, cc.StableDecimals)
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
