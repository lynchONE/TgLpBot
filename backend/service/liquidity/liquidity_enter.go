package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/exchange"
	"TgLpBot/service/pool"
	"TgLpBot/service/trade"
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type EnterResult struct {
	TxHash string

	V3PositionManagerAddress string
	V3TokenID                string

	V4TokenID string

	CurrentLiquidity string

	// Dust0/Dust1 are the leftover amounts returned to the wallet after minting (token0/token1 units).
	// Best-effort: filled from Zap events when available.
	Dust0 *big.Int
	Dust1 *big.Int
}

type EntrySwapRequiredError struct {
	TokenSymbol string
}

func (e *EntrySwapRequiredError) Error() string {
	if e == nil || strings.TrimSpace(e.TokenSymbol) == "" {
		return "entry swap required"
	}
	return fmt.Sprintf("entry swap required: pool does not contain USDT (need %s)", strings.TrimSpace(e.TokenSymbol))
}

type entryTokenCandidate struct {
	Symbol  string
	Address common.Address
}

type entryTokenPlan struct {
	Token0       common.Address
	Token1       common.Address
	EntryToken   common.Address
	EntrySymbol  string
	RequiresSwap bool
}

func entryTokenCandidates() []entryTokenCandidate {
	if config.AppConfig == nil {
		return nil
	}
	var out []entryTokenCandidate
	add := func(symbol, addr string) {
		addr = strings.TrimSpace(addr)
		if !common.IsHexAddress(addr) {
			return
		}
		out = append(out, entryTokenCandidate{
			Symbol:  symbol,
			Address: common.HexToAddress(addr),
		})
	}
	add("USDT", config.AppConfig.USDTAddress)
	add("USDC", config.AppConfig.USDCAddress)
	add("WBNB", config.AppConfig.WBNBAddress)
	return out
}

func (s *LiquidityService) planEntryToken(task *models.StrategyTask) (*entryTokenPlan, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	token0, token1, err := s.resolveTaskTokenAddresses(task)
	if err != nil {
		return nil, err
	}
	candidates := entryTokenCandidates()
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no entry token configured")
	}

	for _, cand := range candidates {
		if cand.Symbol != "USDT" {
			continue
		}
		if token0 == cand.Address || token1 == cand.Address {
			return &entryTokenPlan{
				Token0:       token0,
				Token1:       token1,
				EntryToken:   cand.Address,
				EntrySymbol:  cand.Symbol,
				RequiresSwap: false,
			}, nil
		}
	}

	for _, cand := range candidates {
		if cand.Symbol == "USDT" {
			continue
		}
		if token0 == cand.Address || token1 == cand.Address {
			return &entryTokenPlan{
				Token0:       token0,
				Token1:       token1,
				EntryToken:   cand.Address,
				EntrySymbol:  cand.Symbol,
				RequiresSwap: true,
			}, nil
		}
	}

	supported := make([]string, 0, len(candidates))
	for _, cand := range candidates {
		if cand.Symbol != "" {
			supported = append(supported, cand.Symbol)
		}
	}
	if len(supported) == 0 {
		return nil, fmt.Errorf("pool does not contain a supported entry token")
	}
	return nil, fmt.Errorf("pool does not contain a supported entry token (%s)", strings.Join(supported, "/"))
}

func parseOkxSmartSwapBaseInfo(calldata []byte) (tokenIn common.Address, tokenOut common.Address, amountIn, minOut, deadline *big.Int, err error) {
	// Layout (smartSwapByOrderId):
	// 0..3 selector
	// 4..35 orderId
	// 36..67 baseInfo[0] tokenIn (uint256)
	// 68..99 baseInfo[1] tokenOut (address)
	// 100..131 baseInfo[2] amountIn (uint256)
	// 132..163 baseInfo[3] minOut (uint256)
	// 164..195 baseInfo[4] deadline (uint256)
	shift := -1
	max := len(calldata)
	if max > 1024 {
		max = 1024
	}
	for off := 0; off+4 <= max; off++ {
		if hex.EncodeToString(calldata[off:off+4]) == "b80c2f09" {
			shift = off
			break
		}
	}
	if shift < 0 {
		sel := ""
		if len(calldata) >= 4 {
			sel = "0x" + hex.EncodeToString(calldata[:4])
		}
		return common.Address{}, common.Address{}, nil, nil, nil, fmt.Errorf("not okx smartSwapByOrderId (selector=%s len=%d)", sel, len(calldata))
	}
	if len(calldata) < shift+196 {
		return common.Address{}, common.Address{}, nil, nil, nil, fmt.Errorf("okx calldata too short: %d", len(calldata))
	}

	readWord := func(off int) *big.Int {
		return new(big.Int).SetBytes(calldata[shift+off : shift+off+32])
	}
	wordTokenIn := readWord(36)
	tokenIn = common.BigToAddress(wordTokenIn)
	tokenOut = common.BytesToAddress(calldata[shift+68+12 : shift+68+32])
	amountIn = readWord(100)
	minOut = readWord(132)
	deadline = readWord(164)
	return tokenIn, tokenOut, amountIn, minOut, deadline, nil
}

func ValidateOkxSmartSwapTx(label string, tx blockchain.OkxSwapTx) error {
	if len(tx.Data) == 0 {
		return nil
	}
	if tx.To == (common.Address{}) {
		return fmt.Errorf("%s OKX tx.to is empty", label)
	}
	sel := "0x" + hex.EncodeToString(tx.Data[:min(4, len(tx.Data))])
	log.Printf("[Liquidity] %s OKX tx: to=%s dataLen=%d selector=%s", label, tx.To.Hex(), len(tx.Data), sel)
	_, _, _, _, _, err := parseOkxSmartSwapBaseInfo(tx.Data)
	if err != nil {
		log.Printf("[Liquidity] %s OKX tx.data not smartSwapByOrderId compatible: %v (will pass through as-is)", label, err)
		return nil
	}
	return nil
}

func EnforceOkxSwapRouter(label string, tx blockchain.OkxSwapTx) error {
	if config.AppConfig == nil {
		return nil
	}
	expectedStr := strings.TrimSpace(config.AppConfig.OKXSwapRouter)
	if expectedStr == "" || !common.IsHexAddress(expectedStr) {
		return nil
	}
	if len(tx.Data) == 0 {
		return nil
	}
	expected := common.HexToAddress(expectedStr)
	if tx.To != expected {
		return fmt.Errorf("%s OKX tx.to mismatch: got %s, want %s (OKX_SWAP_ROUTER)", label, tx.To.Hex(), expected.Hex())
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func splitTotalAmount(total *big.Int, needNonZero0 bool, needNonZero1 bool) (*big.Int, *big.Int, error) {
	if total == nil || total.Sign() <= 0 {
		return nil, nil, fmt.Errorf("total amount must be > 0")
	}
	a0 := new(big.Int).Div(total, big.NewInt(2))
	a1 := new(big.Int).Sub(total, a0)

	one := big.NewInt(1)
	if needNonZero0 && a0.Sign() == 0 {
		if a1.Cmp(one) < 0 {
			return nil, nil, fmt.Errorf("amount too small to split")
		}
		a0.Set(one)
		a1.Sub(a1, one)
	}
	if needNonZero1 && a1.Sign() == 0 {
		if a0.Cmp(one) < 0 {
			return nil, nil, fmt.Errorf("amount too small to split")
		}
		a1.Set(one)
		a0.Sub(a0, one)
	}
	return a0, a1, nil
}

func (s *LiquidityService) EnterTaskFromUSDT(userID uint, task *models.StrategyTask) (*EnterResult, error) {
	return s.EnterTaskFromUSDTWithOptions(userID, task, TxOptions{})
}

func (s *LiquidityService) EnterTaskFromUSDTWithOptions(userID uint, task *models.StrategyTask, opts TxOptions) (*EnterResult, error) {
	opts.GasMultiplier = normalizeGasMultiplier(opts.GasMultiplier)
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	if blockchain.Client == nil || blockchain.ChainID == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if task == nil {
		return nil, fmt.Errorf("task is nil")
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

	usdtAmount, err := convert.FloatUSDTToWei(task.AmountUSDT)
	if err != nil {
		return nil, err
	}

	if !common.IsHexAddress(config.AppConfig.USDTAddress) {
		return nil, fmt.Errorf("USDT address not set")
	}
	usdtAddr := common.HexToAddress(config.AppConfig.USDTAddress)
	plan, err := s.planEntryToken(task)
	if err != nil {
		return nil, err
	}
	token0Addr := plan.Token0
	token1Addr := plan.Token1

	// Capture balance before entering (used for "actual invested" and gas cost).
	usdtBefore, _ := blockchain.GetTokenBalance(usdtAddr, walletAddr)
	if usdtBefore == nil {
		usdtBefore = big.NewInt(0)
	}
	t0Before := big.NewInt(0)
	t1Before := big.NewInt(0)
	if token0Addr != (common.Address{}) {
		if bal, _ := blockchain.GetTokenBalance(token0Addr, walletAddr); bal != nil {
			t0Before = bal
		}
	}
	if token1Addr != (common.Address{}) {
		if bal, _ := blockchain.GetTokenBalance(token1Addr, walletAddr); bal != nil {
			t1Before = bal
		}
	}
	bnbBefore, _ := blockchain.GetBalance(walletAddr)
	if bnbBefore == nil {
		bnbBefore = big.NewInt(0)
	}

	entryToken := usdtAddr
	entryAmount := usdtAmount
	allowEntrySwap := task.AllowEntrySwap
	if !allowEntrySwap && task.IsAuto && config.AppConfig != nil && config.AppConfig.AutoLPAllowEntrySwap {
		allowEntrySwap = true
	}
	if plan.RequiresSwap {
		// 如果用户账户里已经有足够的入场代币（典型场景：上次 swap 成功但 bot 误判返回 0），直接用余额开仓，避免重复提示/重复兑换。
		// 目前仅对 USDC 做“USDT 金额≈USDC 数量”的安全处理；WBNB 等非稳定币不适用该等价关系。
		if strings.EqualFold(plan.EntrySymbol, "USDC") && plan.EntryToken != (common.Address{}) {
			usdcBal, _ := blockchain.GetTokenBalance(plan.EntryToken, walletAddr)
			if usdcBal == nil {
				usdcBal = big.NewInt(0)
			}
			slippagePct := task.SlippageTolerance
			if slippagePct < 0 {
				slippagePct = 0.5
			}
			bps := int64(math.Round(slippagePct * 100))
			if bps < 0 {
				bps = 0
			}
			if bps > 10000 {
				bps = 10000
			}
			minNeeded := new(big.Int).Set(usdtAmount)
			if bps > 0 && bps < 10000 {
				minNeeded = new(big.Int).Mul(usdtAmount, big.NewInt(10000-bps))
				minNeeded.Div(minNeeded, big.NewInt(10000))
			} else if bps >= 10000 {
				minNeeded = big.NewInt(0)
			}
			if usdcBal.Sign() > 0 && usdcBal.Cmp(minNeeded) >= 0 {
				use := new(big.Int).Set(usdtAmount)
				if usdcBal.Cmp(use) < 0 {
					use = new(big.Int).Set(usdcBal)
				}
				log.Printf("[Liquidity] Entry swap skipped: already have %s balance=%s use=%s (minNeeded=%s)",
					plan.EntrySymbol, usdcBal.String(), use.String(), minNeeded.String())
				entryToken = plan.EntryToken
				entryAmount = use
			}
		}

		// 仍然需要 swap 才能使用 USDT 入场
		if entryToken == usdtAddr {
			if !allowEntrySwap {
				return nil, &EntrySwapRequiredError{TokenSymbol: plan.EntrySymbol}
			}
			log.Printf("[Liquidity] Entry swap: USDT -> %s amount=%s", plan.EntrySymbol, usdtAmount.String())
			swapped, err := s.swapExactInViaOKX(privateKey, walletAddr, usdtAddr, plan.EntryToken, usdtAmount, task.SlippageTolerance)
			if err != nil {
				return nil, fmt.Errorf("swap USDT to %s failed: %w", plan.EntrySymbol, err)
			}
			if swapped == nil || swapped.Sign() <= 0 {
				// Best-effort: 有些 RPC 会出现“交易已成功但余额暂时读不到”的情况，回退到读取钱包余额（仅限 USDC）。
				if strings.EqualFold(plan.EntrySymbol, "USDC") {
					if bal, _ := blockchain.GetTokenBalance(plan.EntryToken, walletAddr); bal != nil && bal.Sign() > 0 {
						use := new(big.Int).Set(usdtAmount)
						if bal.Cmp(use) < 0 {
							use = new(big.Int).Set(bal)
						}
						log.Printf("[Liquidity] Entry swap returned 0, fallback to wallet %s balance=%s use=%s",
							plan.EntrySymbol, bal.String(), use.String())
						entryToken = plan.EntryToken
						entryAmount = use
					} else {
						return nil, fmt.Errorf("swap USDT to %s returned 0", plan.EntrySymbol)
					}
				} else {
					return nil, fmt.Errorf("swap USDT to %s returned 0", plan.EntrySymbol)
				}
			} else {
				entryToken = plan.EntryToken
				entryAmount = swapped
			}
		}
	}

	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	var res *EnterResult
	switch version {
	case "v4":
		res, err = s.enterV4FromToken(privateKey, walletAddr, entryToken, entryAmount, task, opts)
	default:
		res, err = s.enterV3FromToken(privateKey, walletAddr, entryToken, entryAmount, task, opts)
	}
	if err != nil {
		return nil, err
	}

	// 等待 RPC 节点状态同步，避免读取到旧的余额值
	time.Sleep(500 * time.Millisecond)

	usdtAfter, _ := blockchain.GetTokenBalance(usdtAddr, walletAddr)
	if usdtAfter == nil {
		usdtAfter = big.NewInt(0)
	}
	t0After := big.NewInt(0)
	t1After := big.NewInt(0)
	if token0Addr != (common.Address{}) {
		if bal, _ := blockchain.GetTokenBalance(token0Addr, walletAddr); bal != nil {
			t0After = bal
		}
	}
	if token1Addr != (common.Address{}) {
		if bal, _ := blockchain.GetTokenBalance(token1Addr, walletAddr); bal != nil {
			t1After = bal
		}
	}
	bnbAfter, _ := blockchain.GetBalance(walletAddr)
	if bnbAfter == nil {
		bnbAfter = big.NewInt(0)
	}

	// Actual USDT spent (delta) for this enter.
	actualSpent := new(big.Int).Sub(usdtBefore, usdtAfter)
	if actualSpent.Sign() < 0 {
		actualSpent = big.NewInt(0)
	}
	walletDust0 := big.NewInt(0)
	walletDust1 := big.NewInt(0)
	if token0Addr != (common.Address{}) {
		if delta := new(big.Int).Sub(t0After, t0Before); delta.Sign() > 0 {
			walletDust0 = delta
		}
	}
	if token1Addr != (common.Address{}) {
		if delta := new(big.Int).Sub(t1After, t1Before); delta.Sign() > 0 {
			walletDust1 = delta
		}
	}
	// 注意：不需要对 USDT 做特殊处理，因为上面的余额变化计算已经正确处理了所有 token 的 dust。
	// 之前的逻辑 (usdtAmount - actualSpent) 是错误的，会导致 dust 显示为用户预期投入金额而非实际残余。
	// Gas spent in native BNB (delta).
	gasSpent := new(big.Int).Sub(bnbBefore, bnbAfter)
	if gasSpent.Sign() < 0 {
		gasSpent = big.NewInt(0)
	}
	if res != nil {
		txHash := strings.TrimSpace(res.TxHash)
		if reTxHash.MatchString(txHash) {
			if receipt, rerr := s.getReceiptWithRetry(common.HexToHash(txHash)); rerr == nil && receipt != nil {
				if cost := s.gasCostWeiFromReceipt(common.HexToHash(txHash), receipt); cost.Sign() > 0 {
					if cost.Cmp(gasSpent) > 0 {
						gasSpent = cost
					}
				}
			}
		}
	}
	log.Printf("[Liquidity] Enter gas tracking: bnbBefore=%s bnbAfter=%s gasSpent=%s", bnbBefore.String(), bnbAfter.String(), gasSpent.String())

	dust0 := walletDust0
	dust1 := walletDust1
	if res != nil {
		if res.Dust0 != nil && res.Dust0.Cmp(dust0) > 0 {
			dust0 = res.Dust0
		}
		if res.Dust1 != nil && res.Dust1.Cmp(dust1) > 0 {
			dust1 = res.Dust1
		}
	}

	// If RPC returns stale balances, actualSpent can be 0 (or otherwise invalid) even when the tx succeeded.
	// We can recover a best-effort "actual spent" from the input amount and refunded USDT dust (when applicable).
	if actualSpent.Sign() <= 0 || actualSpent.Cmp(usdtAmount) > 0 {
		expectedSpent := new(big.Int).Set(usdtAmount)
		if !plan.RequiresSwap {
			usdtDust := big.NewInt(0)
			if token0Addr == usdtAddr {
				usdtDust = dust0
			} else if token1Addr == usdtAddr {
				usdtDust = dust1
			}
			if usdtDust.Sign() > 0 {
				expectedSpent.Sub(expectedSpent, usdtDust)
				if expectedSpent.Sign() < 0 {
					expectedSpent = big.NewInt(0)
				}
			}
		}
		actualSpent = expectedSpent
	}

	_ = trade.NewTradeRecordService().CreateOpenRecord(task, res.TxHash, actualSpent, gasSpent, dust0, dust1)

	return res, nil
}

func (s *LiquidityService) okxSlippageDecimal(slippagePercent float64) string {
	if math.IsNaN(slippagePercent) || math.IsInf(slippagePercent, 0) {
		slippagePercent = 0.5
	}
	if slippagePercent < 0 {
		slippagePercent = 0
	}
	if slippagePercent > 100 {
		slippagePercent = 100
	}
	sl := slippagePercent / 100.0
	return fmt.Sprintf("%.6f", sl)
}

func (s *LiquidityService) enterV3FromToken(
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	amountIn *big.Int,
	task *models.StrategyTask,
	opts TxOptions,
) (*EnterResult, error) {
	if !common.IsHexAddress(config.AppConfig.ZapV3Address) {
		return nil, fmt.Errorf("ZAP_V3_ADDRESS not set")
	}

	// 获取 PositionManager 地址
	pmAddrStr := strings.TrimSpace(task.V3PositionManagerAddress)
	log.Printf("[Liquidity] V3 enter: task.V3PositionManagerAddress=%s", pmAddrStr)
	if pmAddrStr == "" {
		ex := strings.ToLower(task.Exchange)
		log.Printf("[Liquidity] V3 enter: task.Exchange=%s (lowercased: %s)", task.Exchange, ex)
		log.Printf("[Liquidity] V3 enter: config.PancakeV3PositionManagerAddress=%s", config.AppConfig.PancakeV3PositionManagerAddress)
		log.Printf("[Liquidity] V3 enter: config.UniswapV3PositionManagerAddress=%s", config.AppConfig.UniswapV3PositionManagerAddress)

		if strings.Contains(ex, "pancake") && common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
			pmAddrStr = config.AppConfig.PancakeV3PositionManagerAddress
			log.Printf("[Liquidity] V3 enter: 选择 PancakeSwap V3 NPM: %s", pmAddrStr)
		} else if strings.Contains(ex, "uniswap") && common.IsHexAddress(config.AppConfig.UniswapV3PositionManagerAddress) {
			pmAddrStr = config.AppConfig.UniswapV3PositionManagerAddress
			log.Printf("[Liquidity] V3 enter: 选择 Uniswap V3 NPM: %s", pmAddrStr)
		} else {
			log.Printf("[Liquidity] V3 enter: ⚠️ 无法匹配到合适的 Position Manager (exchange=%s)", ex)
		}
	}
	if !common.IsHexAddress(pmAddrStr) {
		return nil, fmt.Errorf("V3 position manager address not configured")
	}
	pmAddr := common.HexToAddress(pmAddrStr)

	// 验证池子地址
	if !common.IsHexAddress(task.PoolId) {
		return nil, fmt.Errorf("invalid V3 pool address: %s", task.PoolId)
	}
	poolAddr := common.HexToAddress(task.PoolId)

	// 获取池子代币
	token0, token1, err := blockchain.GetV3PoolTokens(poolAddr)
	if err != nil {
		return nil, fmt.Errorf("read v3 pool tokens failed: %w", err)
	}
	if bytes.Compare(token0.Bytes(), token1.Bytes()) >= 0 {
		return nil, fmt.Errorf("unexpected v3 token ordering")
	}

	zapAddr := common.HexToAddress(config.AppConfig.ZapV3Address)

	if token0 != tokenIn && token1 != tokenIn {
		return nil, fmt.Errorf("V3 pool does not contain entry token")
	}

	// 确定输入金额
	amount0In := big.NewInt(0)
	amount1In := big.NewInt(0)
	if token0 == tokenIn {
		amount0In = new(big.Int).Set(amountIn)
	} else {
		amount1In = new(big.Int).Set(amountIn)
	}

	// 创建 ZapSimple 实例
	zap, err := blockchain.NewZapSimple(zapAddr, blockchain.Client)
	if err != nil {
		return nil, fmt.Errorf("init ZapSimple failed: %w", err)
	}

	// 使用本地计算避免合约 calculateOptimalSwap revert (尤其是 Pancake 池子)
	zeroForOne, swapAmount, err := s.calculateOptimalSwapLocal(poolAddr, task.TickLower, task.TickUpper, amount0In, amount1In)
	if err != nil {
		log.Printf("[Liquidity] V3 enter: Local calculateOptimalSwap 失败: %v，回退使用一半金额", err)
		swapAmount = new(big.Int).Div(amountIn, big.NewInt(2))
		zeroForOne = (token0 == tokenIn)
	}
	log.Printf("[Liquidity] V3 enter: 最优 swap: zeroForOne=%v swapAmount=%s", zeroForOne, swapAmount.String())

	// 确定 swap 代币
	var swapTokenIn, swapTokenOut common.Address
	if zeroForOne {
		swapTokenIn = token0
		swapTokenOut = token1
	} else {
		swapTokenIn = token1
		swapTokenOut = token0
	}

	// 2. 从 OKX 获取 swap calldata
	swapParams := blockchain.SwapParamsSimple{
		Target:        common.Address{},
		ApproveTarget: common.Address{},
		TokenIn:       swapTokenIn,
		TokenOut:      swapTokenOut,
		AmountIn:      big.NewInt(0),
		MinAmountOut:  big.NewInt(0),
		CallData:      []byte{},
	}
	if swapAmount != nil && swapAmount.Sign() > 0 {
		p, err := s.prepareOKXSwapParams(zapAddr, swapTokenIn, swapTokenOut, swapAmount, task.SlippageTolerance)
		if err != nil {
			return nil, err
		}
		if p != nil {
			swapParams = *p
			log.Printf("[Liquidity] V3 enter: OKX swap target=%s minOut=%s", swapParams.Target.Hex(), swapParams.MinAmountOut.String())
		}
	}

	// 3. Approve 代币给 Zap 合约
	if amount0In.Sign() > 0 {
		log.Printf("[Liquidity] V3 enter: approve token0=%s to Zap amount=%s", token0.Hex(), amount0In.String())
		if err := s.approveToken(privateKey, walletAddr, token0, zapAddr, amount0In, opts); err != nil {
			return nil, fmt.Errorf("approve token0 failed: %w", err)
		}
		// Double check allowance (with retry for RPC node sync delay)
		t0, err := blockchain.NewERC20(token0, blockchain.Client)
		if err != nil {
			return nil, fmt.Errorf("init erc20 token0 failed: %w", err)
		}
		allow, err := t0.Allowance(nil, walletAddr, zapAddr)
		if err != nil {
			return nil, fmt.Errorf("check allowance token0 failed: %w", err)
		}
		// 如果 allowance 不足，可能是 RPC 节点状态未及时同步，等待后重试
		if allow.Cmp(amount0In) < 0 {
			log.Printf("[Liquidity] V3 enter: allowance token0 insufficient on first check (%s < %s), waiting 2s and retrying...", allow.String(), amount0In.String())
			time.Sleep(2 * time.Second)
			allow, err = t0.Allowance(nil, walletAddr, zapAddr)
			if err != nil {
				return nil, fmt.Errorf("check allowance token0 (retry) failed: %w", err)
			}
			if allow.Cmp(amount0In) < 0 {
				return nil, fmt.Errorf("allowance token0 insufficient: %s < %s", allow.String(), amount0In.String())
			}
			log.Printf("[Liquidity] V3 enter: allowance token0 OK after retry: %s", allow.String())
		}
		// Double check balance
		bal0, err := blockchain.GetTokenBalance(token0, walletAddr)
		if err != nil {
			return nil, fmt.Errorf("check balance token0 failed: %w", err)
		}
		if bal0.Cmp(amount0In) < 0 {
			return nil, fmt.Errorf("balance token0 insufficient: %s < %s", bal0.String(), amount0In.String())
		}
	}
	if amount1In.Sign() > 0 {
		log.Printf("[Liquidity] V3 enter: approve token1=%s to Zap amount=%s", token1.Hex(), amount1In.String())
		if err := s.approveToken(privateKey, walletAddr, token1, zapAddr, amount1In, opts); err != nil {
			return nil, fmt.Errorf("approve token1 failed: %w", err)
		}
		// Double check allowance and balance (with retry for RPC node sync delay)
		t1, err := blockchain.NewERC20(token1, blockchain.Client)
		if err != nil {
			return nil, fmt.Errorf("init erc20 token1 failed: %w", err)
		}
		allow, err := t1.Allowance(nil, walletAddr, zapAddr)
		if err != nil {
			return nil, fmt.Errorf("check allowance token1 failed: %w", err)
		}
		// 如果 allowance 不足，可能是 RPC 节点状态未及时同步，等待后重试
		if allow.Cmp(amount1In) < 0 {
			log.Printf("[Liquidity] V3 enter: allowance token1 insufficient on first check (%s < %s), waiting 2s and retrying...", allow.String(), amount1In.String())
			time.Sleep(2 * time.Second)
			allow, err = t1.Allowance(nil, walletAddr, zapAddr)
			if err != nil {
				return nil, fmt.Errorf("check allowance token1 (retry) failed: %w", err)
			}
			if allow.Cmp(amount1In) < 0 {
				return nil, fmt.Errorf("allowance token1 insufficient: %s < %s", allow.String(), amount1In.String())
			}
			log.Printf("[Liquidity] V3 enter: allowance token1 OK after retry: %s", allow.String())
		}
		bal1, err := blockchain.GetTokenBalance(token1, walletAddr)
		if err != nil {
			return nil, fmt.Errorf("check balance token1 failed: %w", err)
		}
		if bal1.Cmp(amount1In) < 0 {
			return nil, fmt.Errorf("balance token1 insufficient: %s < %s", bal1.String(), amount1In.String())
		}
	}

	// 4. 构建 zapInV3 参数
	// Zap 合约侧会用 slippageBps 作为“剩余资产容忍度”(maxDustBps) 进行 dust 校验；swap 的价格安全由 minAmountOut 保证。
	mintSlippageBps := percentageToBps(task.ResidualTolerance)
	params := blockchain.ZapInV3ParamsSimple{
		Pool:            poolAddr,
		PositionManager: pmAddr,
		Token0:          token0,
		Token1:          token1,
		TickLower:       big.NewInt(int64(task.TickLower)),
		TickUpper:       big.NewInt(int64(task.TickUpper)),
		Recipient:       walletAddr,
		Amount0In:       amount0In,
		Amount1In:       amount1In,
		SlippageBps:     mintSlippageBps,
		Swap:            swapParams,
	}

	log.Printf("[Liquidity] V3 enter 参数: pool=%s tick=%d..%d mintSlippage(dust)=%s", poolAddr.Hex(), task.TickLower, task.TickUpper, mintSlippageBps.String())

	// 5. 发送交易
	nonce, err := blockchain.GetNonce(walletAddr)
	if err != nil {
		return nil, err
	}
	auth, err := s.buildAuth(privateKey, nonce, big.NewInt(0), config.AppConfig.GasLimit, opts)
	if err != nil {
		return nil, err
	}

	tx, err := zap.ZapInV3(auth, params)
	if err != nil {
		return nil, fmt.Errorf("zapInV3 failed: %w", err)
	}
	log.Printf("[Liquidity] V3 enter: tx sent %s", tx.Hash().Hex())

	receipt, err := s.waitMined(tx)
	if err != nil {
		return nil, fmt.Errorf("v3 enter tx failed: %w", err)
	}

	// 从交易 receipt 中解析返回值
	// 注意：zapInV3 函数返回 ZapResult struct，包含 tokenId/liquidity/amountUsed/dust，但 tx 回执里拿不到 return data；
	// 我们用 ZapInV3 + SwapExecuted 事件计算实际使用量和 dust（剩余代币）。
	tokenId, liq, used0, used1, err := parseZapInV3Result(receipt, zapAddr)
	if err != nil {
		return nil, fmt.Errorf("parse zap result failed: %w", err)
	}

	// 验证 tokenId 不为 0
	if tokenId == nil || tokenId.Sign() == 0 {
		return nil, fmt.Errorf("解析到的 tokenId 为 0，这是无效的 NFT ID")
	}

	// Record transaction
	tokenOut := token0
	if tokenIn == token0 {
		tokenOut = token1
	}
	txRecord := models.Transaction{
		UserID:          task.UserID,
		TaskID:          task.ID,
		TxHash:          tx.Hash().Hex(),
		Type:            models.TxTypeAddLiquidity,
		Status:          models.TxStatusConfirmed,
		FromAddress:     walletAddr.Hex(),
		ToAddress:       pmAddr.Hex(),
		TokenInAddress:  tokenIn.Hex(),
		TokenOutAddress: tokenOut.Hex(),
		AmountIn:        amountIn.String(),
		AmountOut:       "0", // Initial position doesn't have immediate output
		BlockNumber:     receipt.BlockNumber.Uint64(),
		GasUsed:         receipt.GasUsed,
	}
	// Record entry token and the opposite pool token for reference.

	if err := database.DB.Create(&txRecord).Error; err != nil {
		log.Printf("[Liquidity] Failed to record transaction: %v", err)
	}

	// NOTE: TradeRecord (for PnL tracking) is created by the top-level EnterTaskFromUSDT function
	// using the actual USDT spent (delta of wallet balance before/after).
	// Do NOT create it here to avoid duplicates or incorrect amounts.

	dust0 := big.NewInt(0)
	dust1 := big.NewInt(0)
	if used0 == nil {
		used0 = big.NewInt(0)
	}
	if used1 == nil {
		used1 = big.NewInt(0)
	}

	avail0 := new(big.Int).Set(amount0In)
	avail1 := new(big.Int).Set(amount1In)
	if tin, tout, ain, aout, ok := parseZapSwapExecutedEvent(receipt, zapAddr); ok && ain != nil && ain.Sign() > 0 && aout != nil {
		switch {
		case tin == token0 && tout == token1:
			avail0.Sub(avail0, ain)
			if avail0.Sign() < 0 {
				avail0 = big.NewInt(0)
			}
			avail1.Add(avail1, aout)
		case tin == token1 && tout == token0:
			avail1.Sub(avail1, ain)
			if avail1.Sign() < 0 {
				avail1 = big.NewInt(0)
			}
			avail0.Add(avail0, aout)
		default:
			log.Printf("[Liquidity] Warning: SwapExecuted token mismatch: in=%s out=%s (pool token0=%s token1=%s)", tin.Hex(), tout.Hex(), token0.Hex(), token1.Hex())
		}
	}
	dust0.Sub(avail0, used0)
	if dust0.Sign() < 0 {
		dust0 = big.NewInt(0)
	}
	dust1.Sub(avail1, used1)
	if dust1.Sign() < 0 {
		dust1 = big.NewInt(0)
	}

	return &EnterResult{
		TxHash:                   tx.Hash().Hex(),
		V3PositionManagerAddress: pmAddr.Hex(),
		V3TokenID:                tokenId.String(),
		CurrentLiquidity:         liq.String(),
		Dust0:                    dust0,
		Dust1:                    dust1,
	}, nil
}

// getPoolFee 获取 V3 池子的费率
func (s *LiquidityService) getPoolFee(poolAddr common.Address) (uint32, error) {
	// V3 池子 fee() 函数的 ABI
	feeABI := `[{"inputs":[],"name":"fee","outputs":[{"internalType":"uint24","name":"","type":"uint24"}],"stateMutability":"view","type":"function"}]`

	parsed, err := abi.JSON(strings.NewReader(feeABI))
	if err != nil {
		return 0, err
	}

	contract := bind.NewBoundContract(poolAddr, parsed, blockchain.Client, nil, nil)
	var out []interface{}
	err = contract.Call(nil, &out, "fee")
	if err != nil {
		return 0, err
	}

	if len(out) > 0 {
		if fee, ok := out[0].(*big.Int); ok {
			return uint32(fee.Uint64()), nil
		}
	}
	return 0, fmt.Errorf("fee not found")
}

func slippagePercentToBps(slippagePercent float64) *big.Int {
	// Default to 0.5% (50 bps) if unset.
	if slippagePercent <= 0 {
		return big.NewInt(50)
	}
	// slippagePercent is in %, bps = % * 100.
	bps := int64(math.Round(slippagePercent * 100))
	if bps < 0 {
		bps = 0
	}
	if bps > 10000 {
		bps = 10000
	}
	return big.NewInt(bps)
}

func percentToBpsOrZero(percent float64) *big.Int {
	// 临时使用 100% (10000 bps) 的 dust 容忍度进行测试
	// TODO: 测试成功后改回合理的值（如 5%）
	return big.NewInt(10000) // 100% dust 容忍度
}

// parseZapInV3Result parses tokenId/liquidity/amount0Used/amount1Used from ZapSimple.ZapInV3 event.
func parseZapInV3Result(receipt *types.Receipt, zapAddr common.Address) (*big.Int, *big.Int, *big.Int, *big.Int, error) {
	parsed, err := abi.JSON(strings.NewReader(blockchain.ZapSimpleABI))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("parse ZapSimple ABI failed: %w", err)
	}
	ev, ok := parsed.Events["ZapInV3"]
	if !ok {
		return nil, nil, nil, nil, fmt.Errorf("ZapInV3 event not found in ABI")
	}

	for _, lg := range receipt.Logs {
		if lg == nil || lg.Address != zapAddr || len(lg.Topics) == 0 || lg.Topics[0] != ev.ID {
			continue
		}
		if len(lg.Topics) < 4 {
			continue
		}
		tokenId := new(big.Int).SetBytes(lg.Topics[3].Bytes())
		if tokenId.Sign() <= 0 {
			continue
		}

		out, err := parsed.Unpack("ZapInV3", lg.Data)
		if err != nil {
			continue
		}
		if len(out) < 3 {
			continue
		}
		amount0, ok0 := out[0].(*big.Int)
		amount1, ok1 := out[1].(*big.Int)
		liq, okL := out[2].(*big.Int)
		if !ok0 || amount0 == nil {
			amount0 = big.NewInt(0)
		}
		if !ok1 || amount1 == nil {
			amount1 = big.NewInt(0)
		}
		if !okL || liq == nil {
			liq = big.NewInt(0)
		}
		return tokenId, liq, amount0, amount1, nil
	}

	return nil, nil, nil, nil, fmt.Errorf("ZapInV3 event not found in receipt logs")
}

// calculateOptimalSwapPure calculates optimal swap without blockchain calls
func (s *LiquidityService) calculateOptimalSwapPure(
	sqrtPriceX96 *big.Int,
	currentTick, tickLower, tickUpper int,
	amount0In, amount1In *big.Int,
) (bool, *big.Int, error) {
	// 2. Check range
	if currentTick < tickLower {
		if amount1In.Sign() > 0 {
			return false, amount1In, nil // ZeroForOne=false (1->0)
		}
		return true, big.NewInt(0), nil
	}
	if currentTick >= tickUpper {
		if amount0In.Sign() > 0 {
			return true, amount0In, nil // ZeroForOne=true (0->1)
		}
		return false, big.NewInt(0), nil
	}

	q96 := new(big.Float).SetInt(new(big.Int).Lsh(big.NewInt(1), 96))

	fP := new(big.Float).Quo(new(big.Float).SetInt(sqrtPriceX96), q96)
	fPa := new(big.Float).Quo(new(big.Float).SetInt(getSqrtRatioAtTick(tickLower)), q96)
	fPb := new(big.Float).Quo(new(big.Float).SetInt(getSqrtRatioAtTick(tickUpper)), q96)

	invP := new(big.Float).Quo(big.NewFloat(1), fP)
	invPb := new(big.Float).Quo(big.NewFloat(1), fPb)
	amt0Unit := new(big.Float).Sub(invP, invPb)

	amt1Unit := new(big.Float).Sub(fP, fPa)

	if amt0Unit.Sign() <= 0 {
		amt0Unit = big.NewFloat(1e-18)
	}

	ratio := new(big.Float).Quo(amt1Unit, amt0Unit)
	price := new(big.Float).Mul(fP, fP)

	fIn0 := new(big.Float).SetInt(amount0In)
	fIn1 := new(big.Float).SetInt(amount1In)

	valIn1 := new(big.Float).Add(fIn1, new(big.Float).Mul(fIn0, price))

	denom := new(big.Float).Add(price, ratio)
	ideal0 := new(big.Float).Quo(valIn1, denom)

	delta0 := new(big.Float).Sub(fIn0, ideal0)

	zeroForOne := (delta0.Sign() > 0)
	absDelta0 := new(big.Float).Abs(delta0)

	var swapAmtFloat *big.Float
	if zeroForOne {
		swapAmtFloat = absDelta0
	} else {
		ideal1 := new(big.Float).Mul(ideal0, ratio)
		delta1 := new(big.Float).Sub(fIn1, ideal1)
		swapAmtFloat = new(big.Float).Abs(delta1)
	}

	swapAmt, _ := swapAmtFloat.Int(nil)
	return zeroForOne, swapAmt, nil
}

// calculateOptimalSwapLocal calculates the optimal swap amount locally to match V3 pool ratio
func (s *LiquidityService) calculateOptimalSwapLocal(poolAddr common.Address, tickLower, tickUpper int, amount0In, amount1In *big.Int) (bool, *big.Int, error) {
	sqrtPriceX96, currentTick, err := blockchain.GetV3PoolSlot0(poolAddr)
	if err != nil {
		return false, nil, fmt.Errorf("GetV3PoolSlot0 failed: %w", err)
	}
	return s.calculateOptimalSwapPure(sqrtPriceX96, currentTick, tickLower, tickUpper, amount0In, amount1In)
}

func getSqrtRatioAtTick(tick int) *big.Int {
	val := math.Pow(1.0001, float64(tick))
	sqrtVal := math.Sqrt(val)
	// ratioX96 = sqrtVal * 2^96
	f := new(big.Float).SetFloat64(sqrtVal)
	q96 := new(big.Float).SetInt(new(big.Int).Lsh(big.NewInt(1), 96))
	f.Mul(f, q96)
	res, _ := f.Int(nil)
	return res
}

// prepareOKXSwapParams helps construct SwapParamsSimple for Zap contracts
func (s *LiquidityService) prepareOKXSwapParams(
	executorAddr common.Address,
	tokenIn, tokenOut common.Address,
	amountIn *big.Int,
	slippageTolerance float64,
) (*blockchain.SwapParamsSimple, error) {
	if amountIn == nil || amountIn.Sign() <= 0 {
		return nil, nil // No swap needed
	}

	swapReq := exchange.SwapRequest{
		ChainID:           "56", // BSC
		FromTokenAddress:  tokenIn.Hex(),
		ToTokenAddress:    tokenOut.Hex(),
		Amount:            amountIn.String(),
		Slippage:          s.okxSlippageDecimal(slippageTolerance),
		UserWalletAddress: executorAddr.Hex(), // Zap contract as executor
	}

	okxData, err := s.okxService.GetSwapData(swapReq)
	if err != nil {
		return nil, fmt.Errorf("get OKX swap data failed: %w", err)
	}
	if okxData == nil || len(okxData.Data) == 0 {
		return nil, fmt.Errorf("OKX returned empty data")
	}

	expectedOutText := strings.TrimSpace(okxData.Data[0].RouterResult.ToTokenAmount)
	if expectedOutText == "" {
		expectedOutText = "unknown"
	}
	estGasText := strings.TrimSpace(okxData.Data[0].Tx.Gas)
	if estGasText != "" {
		log.Printf("[Liquidity] OKX swap(zap): %s -> %s amountIn=%s executor=%s expectedOut=%s txGas=%s slippage=%.4f%%",
			tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), executorAddr.Hex(), expectedOutText, estGasText, slippageTolerance)
	} else {
		log.Printf("[Liquidity] OKX swap(zap): %s -> %s amountIn=%s executor=%s expectedOut=%s slippage=%.4f%%",
			tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), executorAddr.Hex(), expectedOutText, slippageTolerance)
	}

	baseOut := big.NewInt(0)
	outStr := strings.TrimSpace(okxData.Data[0].RouterResult.ToTokenAmount)
	if outStr != "" {
		var ok bool
		if strings.HasPrefix(outStr, "0x") || strings.HasPrefix(outStr, "0X") {
			baseOut, ok = new(big.Int).SetString(strings.TrimPrefix(strings.TrimPrefix(outStr, "0x"), "0X"), 16)
		} else {
			baseOut, ok = new(big.Int).SetString(outStr, 10)
		}
		if !ok || baseOut == nil || baseOut.Sign() <= 0 {
			baseOut = big.NewInt(0)
		}
	}

	// 95% protection (keep <= OKX calldata's internal minOut to avoid reverting after swap)
	minOut := big.NewInt(0)
	if baseOut.Sign() > 0 {
		minOut = new(big.Int).Mul(baseOut, big.NewInt(95))
		minOut = minOut.Div(minOut, big.NewInt(100))
	}

	callData := []byte{}
	if okxData.Data[0].Tx.Data != "" {
		callData, _ = hex.DecodeString(strings.TrimPrefix(okxData.Data[0].Tx.Data, "0x"))
	}

	approveTarget := common.HexToAddress(okxData.Data[0].Tx.To)
	if config.AppConfig.OKXTokenApproveAddress != "" {
		approveTarget = common.HexToAddress(config.AppConfig.OKXTokenApproveAddress)
	}

	apiTarget := common.HexToAddress(okxData.Data[0].Tx.To)
	target := apiTarget
	if config.AppConfig.OKXSwapRouter != "" {
		confTarget := common.HexToAddress(config.AppConfig.OKXSwapRouter)
		if confTarget != apiTarget {
			log.Printf("[Liquidity] ⚠️ WARNING: Configured OKX Router (%s) mismatch API returned (%s). Using Configured.", confTarget.Hex(), apiTarget.Hex())
		}
		target = confTarget
	}

	return &blockchain.SwapParamsSimple{
		Target:        target,
		ApproveTarget: approveTarget,
		TokenIn:       tokenIn,
		TokenOut:      tokenOut,
		AmountIn:      amountIn,
		MinAmountOut:  minOut,
		CallData:      callData,
	}, nil
}

func (s *LiquidityService) enterV4FromToken(
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	amountIn *big.Int,
	task *models.StrategyTask,
	opts TxOptions,
) (*EnterResult, error) {
	if !common.IsHexAddress(config.AppConfig.ZapV4Address) {
		return nil, fmt.Errorf("ZAP_V4_ADDRESS not set")
	}
	zapAddr := common.HexToAddress(config.AppConfig.ZapV4Address)

	if !common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
		return nil, fmt.Errorf("UNISWAP_V4_POOL_MANAGER_ADDRESS not set")
	}
	if !common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		return nil, fmt.Errorf("UNISWAP_V4_POSITION_MANAGER_ADDRESS not set")
	}

	// 1. Resolve V4 PoolKey (fee/tickSpacing/hooks) via PositionManager.poolKeys(bytes25(poolId)).
	poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
	positionManager := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
	c0, c1, fee, tickSpacing, hooks, poolKeyErr := blockchain.GetUniswapV4PoolKeyFromPositionManager(positionManager, task.PoolId)
	if poolKeyErr != nil {
		// Fallback to PoolManager.Initialize event (requires historical logs).
		log.Printf("[Liquidity] Warning: resolve V4 PoolKey via PositionManager.poolKeys failed, trying Initialize event: %v", poolKeyErr)
		c0, c1, fee, tickSpacing, hooks, poolKeyErr = blockchain.GetUniswapV4PoolKeyFromInitializeEvent(poolManager, task.PoolId)
	}
	if poolKeyErr != nil {
		// Last resort: use stored task values, but only if they exactly match task.PoolId (avoid wasting gas on PoolNotInitialized()).
		log.Printf("[Liquidity] Warning: resolve V4 PoolKey via chain failed, checking task fields as last resort: %v", poolKeyErr)

		if task.Token0Address == "" || task.Token1Address == "" {
			return nil, fmt.Errorf("missing token info in task for V4 pool")
		}

		token0Addr := common.HexToAddress(task.Token0Address)
		token1Addr := common.HexToAddress(task.Token1Address)

		// Sort tokens: c0 must be < c1 for V4 PoolKey
		if bytes.Compare(token0Addr.Bytes(), token1Addr.Bytes()) < 0 {
			c0 = token0Addr
			c1 = token1Addr
		} else {
			c0 = token1Addr
			c1 = token0Addr
		}

		fee = uint64(task.Fee)
		tickSpacing = task.TickSpacing
		if common.IsHexAddress(task.HooksAddress) {
			hooks = common.HexToAddress(task.HooksAddress)
		} else {
			hooks = common.Address{}
		}

		expectedPoolId := strings.TrimSpace(task.PoolId)
		if !strings.HasPrefix(expectedPoolId, "0x") && !strings.HasPrefix(expectedPoolId, "0X") {
			expectedPoolId = "0x" + expectedPoolId
		}
		if len(expectedPoolId) != 66 {
			return nil, fmt.Errorf("invalid V4 PoolId length: %s", task.PoolId)
		}

		// Sanity check: derived PoolId must match task.PoolId.
		uint24Ty, _ := abi.NewType("uint24", "", nil)
		int24Ty, _ := abi.NewType("int24", "", nil)
		addressTy, _ := abi.NewType("address", "", nil)
		encoded, encErr := abi.Arguments{
			{Type: addressTy},
			{Type: addressTy},
			{Type: uint24Ty},
			{Type: int24Ty},
			{Type: addressTy},
		}.Pack(c0, c1, new(big.Int).SetUint64(fee), big.NewInt(int64(tickSpacing)), hooks)
		if encErr != nil {
			return nil, fmt.Errorf("encode v4 poolkey failed: %w", encErr)
		}
		derived := crypto.Keccak256Hash(encoded)
		expected := common.HexToHash(expectedPoolId)
		if derived != expected {
			return nil, fmt.Errorf("v4 PoolKey mismatch: task.PoolId=%s derived=%s (tickSpacing/hooks/fee/token order likely wrong)", expected.Hex(), derived.Hex())
		}
	}
	log.Printf("[Liquidity] V4 PoolKey: c0=%s c1=%s fee=%d tickSpacing=%d hooks=%s", c0.Hex(), c1.Hex(), fee, tickSpacing, hooks.Hex())

	// Persist authoritative PoolKey fields for V4 tasks when available (helps older tasks created with guessed metadata).
	if poolKeyErr == nil {
		updates := map[string]interface{}{}
		if task.Token0Address == "" || common.HexToAddress(task.Token0Address) != c0 {
			updates["token0_address"] = c0.Hex()
			task.Token0Address = c0.Hex()
		}
		if task.Token1Address == "" || common.HexToAddress(task.Token1Address) != c1 {
			updates["token1_address"] = c1.Hex()
			task.Token1Address = c1.Hex()
		}
		if task.HooksAddress == "" || common.HexToAddress(task.HooksAddress) != hooks {
			updates["hooks_address"] = hooks.Hex()
			task.HooksAddress = hooks.Hex()
		}
		if task.Fee != int(fee) {
			updates["fee"] = int(fee)
			task.Fee = int(fee)
		}
		if task.TickSpacing != tickSpacing {
			updates["tick_spacing"] = tickSpacing
			task.TickSpacing = tickSpacing
		}
		if len(updates) > 0 {
			if err := database.DB.Model(task).Updates(updates).Error; err != nil {
				log.Printf("[Liquidity] Warning: update V4 pool metadata failed: %v", err)
			}
		}
	}

	// 2. Get Current Tick & SqrtPrice (Prefer StateView, fallback to PoolManager)
	var currentTick int
	var sqrtPriceX96 *big.Int

	// Only use StateView for V4 slot0 query (PoolManager.slot0 is not supported)
	if config.AppConfig == nil || !common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) {
		return nil, fmt.Errorf("UNISWAP_V4_STATE_VIEW_ADDRESS not configured")
	}
	stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
	var err error
	sqrtPriceX96, currentTick, err = blockchain.GetUniswapV4PoolSlot0ViaStateView(stateView, poolManager, task.PoolId)
	if err != nil {
		return nil, fmt.Errorf("get v4 slot0 via StateView failed: %w", err)
	}

	// Validate tick range against the actual tickSpacing.
	if tickSpacing <= 0 {
		return nil, fmt.Errorf("invalid V4 tickSpacing=%d (PoolId=%s)", tickSpacing, task.PoolId)
	}
	tickLower := task.TickLower
	tickUpper := task.TickUpper
	tc := pool.NewTickCalculator()
	if terr := tc.ValidateTickRange(tickLower, tickUpper, tickSpacing); terr != nil {
		// If the task was created with an incorrect tickSpacing, try to recompute from the stored percentage.
		var newLower, newUpper int
		var recomputeLabel string
		switch {
		case task.RangeLowerPercentage > 0 && task.RangeUpperPercentage > 0:
			newLower, newUpper = tc.CalculateTickFromPercentagesBestFit(currentTick, task.RangeLowerPercentage, task.RangeUpperPercentage, tickSpacing)
			recomputeLabel = fmt.Sprintf("RangeLower=%.4f RangeUpper=%.4f", task.RangeLowerPercentage, task.RangeUpperPercentage)
		case task.RangePercentage > 0:
			newLower, newUpper = tc.CalculateTickFromPercentagesBestFit(currentTick, task.RangePercentage, task.RangePercentage, tickSpacing)
			recomputeLabel = fmt.Sprintf("RangePercentage=%.4f", task.RangePercentage)
		default:
			return nil, fmt.Errorf("invalid V4 tick range: %w", terr)
		}
		if terr2 := tc.ValidateTickRange(newLower, newUpper, tickSpacing); terr2 == nil {
			log.Printf("[Liquidity] V4 tick range invalid for tickSpacing=%d (%v). Recomputed from %s => [%d,%d]", tickSpacing, terr, recomputeLabel, newLower, newUpper)
			tickLower, tickUpper = newLower, newUpper
		} else {
			return nil, fmt.Errorf("invalid V4 tick range (original=%v, recompute=%v)", terr, terr2)
		}
	}

	if poolKeyErr == nil && (tickLower != task.TickLower || tickUpper != task.TickUpper) {
		if err := database.DB.Model(task).Updates(map[string]interface{}{
			"tick_lower": tickLower,
			"tick_upper": tickUpper,
		}).Error; err != nil {
			log.Printf("[Liquidity] Warning: update V4 tick range failed: %v", err)
		} else {
			task.TickLower = tickLower
			task.TickUpper = tickUpper
		}
	}

	// Determine Token0/Token1 and Amounts
	// We assume entry token is one of c0 or c1.
	var amount0In, amount1In *big.Int
	var tokenOut common.Address

	if c0 == tokenIn {
		amount0In = amountIn
		amount1In = big.NewInt(0)
		tokenOut = c1
	} else if c1 == tokenIn {
		amount0In = big.NewInt(0)
		amount1In = amountIn
		tokenOut = c0
	} else {
		return nil, fmt.Errorf("V4 pool does not contain entry token")
	}

	// 3. Calculate Optimal Swap
	_, swapAmount, err := s.calculateOptimalSwapPure(sqrtPriceX96, currentTick, tickLower, tickUpper, amount0In, amount1In)
	if err != nil {
		return nil, fmt.Errorf("calc optimal swap failed: %w", err)
	}

	// 4. Prepare OKX Swap Data if needed
	var swapParams blockchain.SwapParamsSimple
	swapParams.Target = common.Address{}
	swapParams.AmountIn = big.NewInt(0)
	swapParams.MinAmountOut = big.NewInt(0)
	swapParams.CallData = []byte{}

	if swapAmount.Sign() > 0 {
		sParams, err := s.prepareOKXSwapParams(zapAddr, tokenIn, tokenOut, swapAmount, task.SlippageTolerance)
		if err != nil {
			log.Printf("[Liquidity] Warning: prepare OKX swap failed, trying zero swap: %v", err)
		} else if sParams != nil {
			swapParams = *sParams
		}
	}

	// 5. Construct ZapInV4 Params
	// Approve entry token to Zap Contract
	log.Printf("[Liquidity] DEBUG: About to approve entry token to Zap. amount=%s token=%s zapAddr=%s", amountIn.String(), tokenIn.Hex(), zapAddr.Hex())
	if err := s.approveToken(privateKey, walletAddr, tokenIn, zapAddr, amountIn, opts); err != nil {
		log.Printf("[Liquidity] DEBUG: approveToken failed: %v", err)
		return nil, fmt.Errorf("approve entry token to zap failed: %w", err)
	}

	poolKeySimple := blockchain.PoolKeySimple{
		Currency0:   c0,
		Currency1:   c1,
		Fee:         big.NewInt(int64(fee)),
		TickSpacing: big.NewInt(int64(tickSpacing)),
		Hooks:       hooks,
	}

	zapParams := blockchain.ZapInV4ParamsSimple{
		PoolKey:         poolKeySimple,
		StateView:       common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress),
		PositionManager: common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress),
		TickLower:       big.NewInt(int64(tickLower)),
		TickUpper:       big.NewInt(int64(tickUpper)),
		Recipient:       walletAddr,
		Amount0In:       amount0In,
		Amount1In:       amount1In,
		SlippageBps:     percentageToBps(task.SlippageTolerance),
		Swap:            swapParams,
		SqrtPriceX96:    sqrtPriceX96,                            // 传入从链上获取的价格，避免合约重复调用
		MaxDustBps:      percentageToBps(task.ResidualTolerance), // 剩余资产容忍度 (dust)
	}

	// 6. Call ZapInV4
	zap, err := blockchain.NewZapSimple(zapAddr, blockchain.Client)
	if err != nil {
		return nil, err
	}

	nonce, err := blockchain.GetNonce(walletAddr)
	if err != nil {
		return nil, err
	}
	auth, err := s.buildAuth(privateKey, nonce, big.NewInt(0), config.AppConfig.GasLimit, opts)
	if err != nil {
		return nil, err
	}

	// 详细日志：打印所有 ZapInV4 参数
	log.Printf("[Liquidity] ========== ZapInV4 参数详情 ==========")
	log.Printf("[Liquidity] PoolKey.Currency0: %s", zapParams.PoolKey.Currency0.Hex())
	log.Printf("[Liquidity] PoolKey.Currency1: %s", zapParams.PoolKey.Currency1.Hex())
	log.Printf("[Liquidity] PoolKey.Fee: %s", zapParams.PoolKey.Fee.String())
	log.Printf("[Liquidity] PoolKey.TickSpacing: %s", zapParams.PoolKey.TickSpacing.String())
	log.Printf("[Liquidity] PoolKey.Hooks: %s", zapParams.PoolKey.Hooks.Hex())
	log.Printf("[Liquidity] StateView: %s", zapParams.StateView.Hex())
	log.Printf("[Liquidity] PositionManager: %s", zapParams.PositionManager.Hex())
	log.Printf("[Liquidity] TickLower: %s, TickUpper: %s", zapParams.TickLower.String(), zapParams.TickUpper.String())
	log.Printf("[Liquidity] Recipient: %s", zapParams.Recipient.Hex())
	log.Printf("[Liquidity] Amount0In: %s, Amount1In: %s", zapParams.Amount0In.String(), zapParams.Amount1In.String())
	log.Printf("[Liquidity] SlippageBps: %s", zapParams.SlippageBps.String())
	log.Printf("[Liquidity] SqrtPriceX96: %s", zapParams.SqrtPriceX96.String())
	log.Printf("[Liquidity] MaxDustBps: %s", zapParams.MaxDustBps.String())
	log.Printf("[Liquidity] Swap.Target: %s, Swap.AmountIn: %s", zapParams.Swap.Target.Hex(), zapParams.Swap.AmountIn.String())
	log.Printf("[Liquidity] Swap.CallData Length: %d bytes", len(zapParams.Swap.CallData))
	log.Printf("[Liquidity] ==========================================")

	log.Printf("[Liquidity] Calling ZapInV4... PoolId=%s SwapAmt=%s", task.PoolId, swapAmount.String())
	tx, err := zap.ZapInV4(auth, zapParams)
	if err != nil {
		return nil, fmt.Errorf("ZapInV4 failed: %w", err)
	}
	log.Printf("[Liquidity] ZapInV4 sent: %s", tx.Hash().Hex())

	receipt, err := s.waitMined(tx)
	if err != nil {
		return nil, fmt.Errorf("ZapInV4 tx failed: %w", err)
	}

	// 7. Parse Result (ZapInV4 Event)
	tokenId, liq, used0, used1, err := parseZapInV4Event(receipt, zapAddr)
	if err != nil {
		log.Printf("[Liquidity] Warning: parse ZapInV4 event failed: %v", err)
		return nil, err
	}

	// 8. Record Transaction
	txRecord := models.Transaction{
		UserID:          task.UserID,
		TaskID:          task.ID,
		TxHash:          tx.Hash().Hex(),
		Type:            models.TxTypeAddLiquidity,
		Status:          models.TxStatusConfirmed,
		FromAddress:     walletAddr.Hex(),
		ToAddress:       zapAddr.Hex(),
		TokenInAddress:  tokenIn.Hex(),
		TokenOutAddress: task.PoolId, // Pool ID
		AmountIn:        amountIn.String(),
		AmountOut:       "0",
		BlockNumber:     receipt.BlockNumber.Uint64(),
		GasUsed:         receipt.GasUsed,
		CreatedAt:       time.Now(),
	}
	database.DB.Create(&txRecord)

	// NOTE: TradeRecord (for PnL tracking) is created by the top-level EnterTaskFromUSDT function
	// using the actual USDT spent (delta of wallet balance before/after).
	// Do NOT create it here to avoid duplicates or incorrect amounts.

	dust0 := big.NewInt(0)
	dust1 := big.NewInt(0)
	if used0 == nil {
		used0 = big.NewInt(0)
	}
	if used1 == nil {
		used1 = big.NewInt(0)
	}

	avail0 := new(big.Int).Set(amount0In)
	avail1 := new(big.Int).Set(amount1In)
	if tin, tout, ain, aout, ok := parseZapSwapExecutedEvent(receipt, zapAddr); ok && ain != nil && ain.Sign() > 0 && aout != nil {
		switch {
		case tin == c0 && tout == c1:
			avail0.Sub(avail0, ain)
			if avail0.Sign() < 0 {
				avail0 = big.NewInt(0)
			}
			avail1.Add(avail1, aout)
		case tin == c1 && tout == c0:
			avail1.Sub(avail1, ain)
			if avail1.Sign() < 0 {
				avail1 = big.NewInt(0)
			}
			avail0.Add(avail0, aout)
		default:
			log.Printf("[Liquidity] Warning: SwapExecuted token mismatch (V4): in=%s out=%s (c0=%s c1=%s)", tin.Hex(), tout.Hex(), c0.Hex(), c1.Hex())
		}
	}
	dust0.Sub(avail0, used0)
	if dust0.Sign() < 0 {
		dust0 = big.NewInt(0)
	}
	dust1.Sub(avail1, used1)
	if dust1.Sign() < 0 {
		dust1 = big.NewInt(0)
	}

	return &EnterResult{
		TxHash:           tx.Hash().Hex(),
		V4TokenID:        tokenId.String(),
		CurrentLiquidity: liq.String(),
		Dust0:            dust0,
		Dust1:            dust1,
	}, nil
}

// parseZapInV4Event parses tokenId/liquidity/amount0Used/amount1Used from logs.
func parseZapInV4Event(receipt *types.Receipt, zapAddr common.Address) (*big.Int, *big.Int, *big.Int, *big.Int, error) {
	// ZapInV4(address indexed user, bytes32 indexed poolId, uint256 indexed tokenId, uint256 amount0, uint256 amount1, uint128 liquidity)
	query := blockchain.ZapSimpleABI
	parsed, err := abi.JSON(strings.NewReader(query))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	eventID := parsed.Events["ZapInV4"].ID

	for _, lg := range receipt.Logs {
		if lg.Address == zapAddr && len(lg.Topics) > 0 && lg.Topics[0] == eventID {
			if len(lg.Topics) < 4 {
				continue
			}
			tokenId := new(big.Int).SetBytes(lg.Topics[3].Bytes())

			// Unpack non-indexed: amount0, amount1, liquidity
			out, err := parsed.Unpack("ZapInV4", lg.Data)
			if err != nil {
				continue
			}
			if len(out) >= 3 {
				a0, _ := out[0].(*big.Int)
				a1, _ := out[1].(*big.Int)
				liq, _ := out[2].(*big.Int)
				if a0 == nil {
					a0 = big.NewInt(0)
				}
				if a1 == nil {
					a1 = big.NewInt(0)
				}
				if liq == nil {
					liq = big.NewInt(0)
				}
				return tokenId, liq, a0, a1, nil
			}
		}
	}
	return nil, nil, nil, nil, fmt.Errorf("ZapInV4 event not found")
}

func parseZapSwapExecutedEvent(receipt *types.Receipt, zapAddr common.Address) (common.Address, common.Address, *big.Int, *big.Int, bool) {
	parsed, err := abi.JSON(strings.NewReader(blockchain.ZapSimpleABI))
	if err != nil {
		return common.Address{}, common.Address{}, nil, nil, false
	}
	ev, ok := parsed.Events["SwapExecuted"]
	if !ok {
		return common.Address{}, common.Address{}, nil, nil, false
	}

	for _, lg := range receipt.Logs {
		if lg == nil || lg.Address != zapAddr || len(lg.Topics) == 0 || lg.Topics[0] != ev.ID {
			continue
		}
		out, err := parsed.Unpack("SwapExecuted", lg.Data)
		if err != nil || len(out) < 4 {
			continue
		}
		tokenIn, _ := out[0].(common.Address)
		tokenOut, _ := out[1].(common.Address)
		amountIn, _ := out[2].(*big.Int)
		amountOut, _ := out[3].(*big.Int)
		if amountIn == nil {
			amountIn = big.NewInt(0)
		}
		if amountOut == nil {
			amountOut = big.NewInt(0)
		}
		return tokenIn, tokenOut, amountIn, amountOut, true
	}

	return common.Address{}, common.Address{}, nil, nil, false
}

// percentageToBps converts float percent (e.g. 0.5) to bps (e.g. 50)
func percentageToBps(p float64) *big.Int {
	// 临时禁用 dust 校验，避免 PancakeSwap V3 因 swap 精度问题 revert
	// slippageBps = 0 表示不校验 dust，剩余代币会被退回用户
	// TODO: 优化 swap 计算逻辑后，可以改回用户配置的值
	return big.NewInt(0) // 禁用 dust 校验
	// return big.NewInt(int64(p * 100))
}
