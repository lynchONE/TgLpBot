package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/exchange"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
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
	if blockchain.Client == nil || blockchain.ChainID == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if database.DB == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	if !common.IsHexAddress(config.AppConfig.USDTAddress) {
		return nil, fmt.Errorf("USDT address not set")
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
	usdtAddr := common.HexToAddress(config.AppConfig.USDTAddress)

	excluded := excludedSwapTokens()
	excluded[usdtAddr] = struct{}{}

	candidates, err := s.collectUserTaskTokens(userID)
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

		bal, berr := blockchain.GetTokenBalance(tokenAddr, walletAddr)
		if berr != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%s->USDT|get balance failed: %v", tokenLabel(tokenAddr, symGuess), berr))
			continue
		}
		if bal == nil || bal.Sign() <= 0 {
			continue
		}

		txHash, serr := s.swapDeltaToUSDTWithHash(privateKey, walletAddr, tokenAddr, usdtAddr, bal, slippagePercent)
		if serr != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%s->USDT|%v", tokenLabel(tokenAddr, symGuess), serr))
			continue
		}
		if strings.TrimSpace(txHash) == "" {
			report.Failed = append(report.Failed, fmt.Sprintf("%s->USDT|empty tx hash", tokenLabel(tokenAddr, symGuess)))
			continue
		}

		report.Swapped = append(report.Swapped, fmt.Sprintf("%s->USDT|%s", tokenLabel(tokenAddr, symGuess), txHash))
	}

	return report, nil
}

func excludedSwapTokens() map[common.Address]struct{} {
	excluded := make(map[common.Address]struct{})
	if config.AppConfig == nil {
		return excluded
	}
	// Stable coins
	if common.IsHexAddress(config.AppConfig.USDCAddress) {
		excluded[common.HexToAddress(config.AppConfig.USDCAddress)] = struct{}{}
	}
	if common.IsHexAddress(config.AppConfig.BUSDAddress) {
		excluded[common.HexToAddress(config.AppConfig.BUSDAddress)] = struct{}{}
	}
	// Treat WBNB as BNB for safety (avoid swapping user's gas asset equivalent).
	if common.IsHexAddress(config.AppConfig.WBNBAddress) {
		excluded[common.HexToAddress(config.AppConfig.WBNBAddress)] = struct{}{}
	}
	return excluded
}

func tokenLabel(tokenAddr common.Address, symGuess string) string {
	sym := strings.TrimSpace(symGuess)
	if sym == "" && blockchain.Client != nil {
		if erc20, err := blockchain.NewERC20(tokenAddr, blockchain.Client); err == nil {
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

func (s *LiquidityService) collectUserTaskTokens(userID uint) (map[common.Address]string, error) {
	var tasks []models.StrategyTask
	if err := database.DB.Select(
		"id",
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
	).Where("user_id = ?", userID).Find(&tasks).Error; err != nil {
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
	if config.AppConfig == nil {
		return nil, fmt.Errorf("配置未加载")
	}
	if blockchain.Client == nil || blockchain.ChainID == nil {
		return nil, fmt.Errorf("区块链客户端未初始化")
	}
	if database.DB == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}

	wallet, err := s.walletService.GetDefaultWallet(userID)
	if err != nil {
		return nil, fmt.Errorf("获取钱包失败: %w", err)
	}

	walletAddr := s.walletService.GetWalletAddress(wallet)
	usdtAddr := common.HexToAddress(config.AppConfig.USDTAddress)

	excluded := excludedSwapTokens()
	excluded[usdtAddr] = struct{}{}

	candidates, err := s.collectUserTaskTokens(userID)
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

		bal, berr := blockchain.GetTokenBalance(tokenAddr, walletAddr)
		if berr != nil {
			continue
		}
		if bal == nil || bal.Sign() <= 0 {
			continue
		}

		// 获取代币符号
		symbol := tokenLabel(tokenAddr, symGuess)

		// 获取代币小数位数
		decimals := 18
		if blockchain.Client != nil {
			if erc20, err := blockchain.NewERC20(tokenAddr, blockchain.Client); err == nil {
				if d, err := erc20.Decimals(nil); err == nil {
					decimals = int(d)
				}
			}
		}

		// 计算人类可读余额
		balFloat := toFloat64(bal, decimals)
		balanceStr := fmt.Sprintf("%.4f", balFloat)

		// 获取代币的 USDT 价值 (使用 OKX DEX 报价)
		valueUSDT := s.getTokenValueInUSDT(tokenAddr, bal, walletAddr)

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
func (s *LiquidityService) getTokenValueInUSDT(tokenAddr common.Address, amount *big.Int, walletAddr common.Address) float64 {
	if amount == nil || amount.Sign() <= 0 {
		return 0
	}
	if s.okxService == nil {
		return 0
	}

	usdtAddr := common.HexToAddress(config.AppConfig.USDTAddress)
	chainID := blockchain.ChainID.String()

	// 使用 OKX DEX 获取报价
	resp, err := s.okxService.GetSwapData(exchange.SwapRequest{
		ChainID:           chainID,
		FromTokenAddress:  tokenAddr.Hex(),
		ToTokenAddress:    usdtAddr.Hex(),
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
	return toFloat64(toAmount, 18)
}
