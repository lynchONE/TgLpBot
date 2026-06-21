package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/exchange"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"TgLpBot/service/trade"
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log"
	"math"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// SwapToUSDTError marks an error that happened during the "swap back to USDT" phase,
// after the position was already removed successfully. Callers may choose to recover
// via an additional wallet sweep swap.
type SwapToUSDTError struct {
	Err error
}

func (e *SwapToUSDTError) Error() string {
	if e == nil || e.Err == nil {
		return "swap to USDT failed"
	}
	return e.Err.Error()
}

func (e *SwapToUSDTError) Unwrap() error { return e.Err }

var (
	erc20TransferEventID = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	reTxHash             = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)
	reTxHashAny          = regexp.MustCompile(`0x[0-9a-fA-F]{64}`)
)

const (
	exitSlippageRetryMultiplier = 2.0
	exitSlippageMaxPercent      = 10.0
	exitSwapMinValueUSDT        = 1.0
	exitPercentScale            = int64(1000000)
)

// ValidateExitPercent returns the effective exit percent and whether it is a partial exit.
// A nil percent preserves the legacy full-exit behavior.
func ValidateExitPercent(percent *float64) (float64, bool, error) {
	if percent == nil {
		return 100, false, nil
	}
	v := *percent
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false, fmt.Errorf("exit percent must be a finite number")
	}
	if v <= 0 || v > 100 {
		return 0, false, fmt.Errorf("exit percent must be greater than 0 and less than or equal to 100")
	}
	return v, v < 100, nil
}

func exitLiquidityForPercent(current *big.Int, percent float64) (*big.Int, error) {
	if current == nil || current.Sign() <= 0 {
		return nil, fmt.Errorf("current liquidity must be positive")
	}
	effective, partial, err := ValidateExitPercent(&percent)
	if err != nil {
		return nil, err
	}
	if !partial {
		return cloneBig(current), nil
	}

	scaled := int64(math.Round(effective * float64(exitPercentScale)))
	if scaled <= 0 {
		scaled = 1
	}
	denom := new(big.Int).Mul(big.NewInt(100), big.NewInt(exitPercentScale))
	out := new(big.Int).Mul(cloneBig(current), big.NewInt(scaled))
	out.Div(out, denom)
	if out.Sign() <= 0 {
		out = big.NewInt(1)
	}
	if out.Cmp(current) > 0 {
		out = cloneBig(current)
	}
	return out, nil
}

func persistTaskCurrentLiquidity(task *models.StrategyTask, liquidity *big.Int) {
	if task == nil {
		return
	}
	value := "0"
	if liquidity != nil && liquidity.Sign() > 0 {
		value = liquidity.String()
	}
	task.CurrentLiquidity = value
	if database.DB == nil || task.ID == 0 {
		return
	}
	if err := database.DB.Model(task).Update("current_liquidity", value).Error; err != nil {
		log.Printf("[Liquidity] Warning: update task current_liquidity failed task_id=%d value=%s: %v", task.ID, value, err)
	}
}

func effectiveExitSlippagePercent(task *models.StrategyTask) float64 {
	base := 0.5
	retryCount := 0
	if task != nil {
		if task.SlippageTolerance > 0 {
			base = task.SlippageTolerance
		}
		if task.ExitRetryCount > 0 {
			retryCount = task.ExitRetryCount
		}
	}
	if retryCount <= 0 {
		return base
	}

	expanded := base * math.Pow(exitSlippageRetryMultiplier, float64(retryCount))

	maxPct := exitSlippageMaxPercent
	if base > maxPct {
		maxPct = base
	}
	if expanded > maxPct {
		expanded = maxPct
	}
	if expanded < base {
		expanded = base
	}
	return expanded
}

// exitMinAmounts returns amount0Min/amount1Min for removing `removeLiq` of liquidity at the
// current pool price, allowing `slippagePercent` of downside. It mirrors the Uniswap UI
// approach (quote at the freshly-read price, then apply a slippage floor): a price moved
// between our read and on-chain execution makes decreaseLiquidity revert instead of draining
// the position at a manipulated ratio. Returns (0,0) whenever price/ticks are unavailable, so
// a transient RPC failure never blocks an exit (the exit-retry loop widens slippage on revert).
func exitMinAmounts(sqrtPriceX96 *big.Int, tickLower, tickUpper int, removeLiq *big.Int, slippagePercent float64) (*big.Int, *big.Int) {
	if sqrtPriceX96 == nil || sqrtPriceX96.Sign() <= 0 || removeLiq == nil || removeLiq.Sign() <= 0 || tickLower >= tickUpper {
		return big.NewInt(0), big.NewInt(0)
	}
	sqrtA, err := pool.SqrtRatioAtTick(int32(tickLower))
	if err != nil {
		return big.NewInt(0), big.NewInt(0)
	}
	sqrtB, err := pool.SqrtRatioAtTick(int32(tickUpper))
	if err != nil {
		return big.NewInt(0), big.NewInt(0)
	}
	exp0, exp1 := pool.AmountsForLiquidity(sqrtPriceX96, sqrtA, sqrtB, removeLiq)
	if exp0 == nil {
		exp0 = big.NewInt(0)
	}
	if exp1 == nil {
		exp1 = big.NewInt(0)
	}

	bps := int64(math.Round(slippagePercent * 100))
	if bps < 0 {
		bps = 0
	}
	if bps > 10000 {
		bps = 10000
	}
	keep := big.NewInt(10000 - bps)
	min0 := new(big.Int).Div(new(big.Int).Mul(exp0, keep), big.NewInt(10000))
	min1 := new(big.Int).Div(new(big.Int).Mul(exp1, keep), big.NewInt(10000))
	return min0, min1
}

func (s *LiquidityService) quoteTokenValueInUSDT(exec chainexec.EVMExecutor, tokenAddr common.Address, amount *big.Int, walletAddr common.Address) (float64, bool) {
	if amount == nil || amount.Sign() <= 0 {
		return 0, true
	}
	if exec == nil {
		return 0, false
	}
	cc := exec.Config()
	if !common.IsHexAddress(cc.StableAddress) {
		return 0, false
	}
	if s.okxService == nil {
		s.okxService = exchange.NewOKXDexService()
	}

	usdtAddr := common.HexToAddress(cc.StableAddress)
	chainID := fmt.Sprintf("%d", cc.ChainID)

	resp, err := s.okxService.GetSwapData(exchange.SwapRequest{
		ChainID:           chainID,
		FromTokenAddress:  tokenAddr.Hex(),
		ToTokenAddress:    usdtAddr.Hex(),
		Amount:            amount.String(),
		Slippage:          s.okxSlippageDecimal(1.0), // 1% slippage for quote
		UserWalletAddress: walletAddr.Hex(),
	})
	if err != nil || resp == nil || len(resp.Data) == 0 {
		return 0, false
	}

	toAmountStr := strings.TrimSpace(resp.Data[0].RouterResult.ToTokenAmount)
	if toAmountStr == "" {
		return 0, false
	}
	toAmount, ok := new(big.Int).SetString(toAmountStr, 10)
	if !ok {
		return 0, false
	}

	return toFloat64(toAmount, cc.StableDecimals), true
}

func (s *LiquidityService) shouldSkipExitSwapToUSDT(exec chainexec.EVMExecutor, tokenAddr common.Address, amountIn *big.Int, walletAddr common.Address, label string) (bool, float64) {
	v, ok := s.quoteTokenValueInUSDT(exec, tokenAddr, amountIn, walletAddr)
	if !ok {
		return false, 0
	}
	if v < exitSwapMinValueUSDT {
		if strings.TrimSpace(label) == "" {
			label = tokenAddr.Hex()
		}
		log.Printf("[Liquidity] Exit swap skipped (<%.2f USDT): %s amount=%s estValue=%.6f",
			exitSwapMinValueUSDT, label, amountIn.String(), v)
		return true, v
	}
	return false, v
}

func txHashFromText(value string) (common.Hash, bool) {
	txInfo := strings.TrimSpace(value)
	if txInfo == "" {
		return common.Hash{}, false
	}
	parts := strings.Split(txInfo, "|")
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if reTxHash.MatchString(part) {
			return common.HexToHash(part), true
		}
	}
	if match := reTxHashAny.FindString(txInfo); match != "" {
		return common.HexToHash(match), true
	}
	return common.Hash{}, false
}

func firstTxHash(txHashes []string) (common.Hash, bool) {
	for _, item := range txHashes {
		if h, ok := txHashFromText(item); ok {
			return h, true
		}
	}
	return common.Hash{}, false
}

func extractTxHashes(txHashes []string) []common.Hash {
	if len(txHashes) == 0 {
		return nil
	}
	seen := make(map[common.Hash]struct{}, len(txHashes))
	out := make([]common.Hash, 0, len(txHashes))
	for _, item := range txHashes {
		h, ok := txHashFromText(item)
		if !ok {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	return out
}

func ReceiptTokenTransferDelta(receipt *types.Receipt, token common.Address, wallet common.Address) *big.Int {
	if receipt == nil || token == (common.Address{}) || wallet == (common.Address{}) {
		return big.NewInt(0)
	}
	in := big.NewInt(0)
	out := big.NewInt(0)
	for _, lg := range receipt.Logs {
		if lg == nil || lg.Address != token || len(lg.Topics) < 3 || lg.Topics[0] != erc20TransferEventID {
			continue
		}
		from := common.BytesToAddress(lg.Topics[1].Bytes())
		to := common.BytesToAddress(lg.Topics[2].Bytes())
		if len(lg.Data) == 0 {
			continue
		}
		val := new(big.Int).SetBytes(lg.Data)
		if to == wallet {
			in.Add(in, val)
		}
		if from == wallet {
			out.Add(out, val)
		}
	}
	delta := new(big.Int).Sub(in, out)
	if delta.Sign() < 0 {
		return big.NewInt(0)
	}
	return delta
}

func receiptGasCostWei(receipt *types.Receipt) *big.Int {
	if receipt == nil || receipt.GasUsed == 0 {
		return big.NewInt(0)
	}
	if receipt.EffectiveGasPrice == nil || receipt.EffectiveGasPrice.Sign() <= 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Mul(receipt.EffectiveGasPrice, new(big.Int).SetUint64(receipt.GasUsed))
}

func (s *LiquidityService) gasCostWeiFromReceipt(client *ethclient.Client, txHash common.Hash, receipt *types.Receipt) *big.Int {
	if receipt == nil {
		return big.NewInt(0)
	}
	if cost := receiptGasCostWei(receipt); cost.Sign() > 0 {
		return cost
	}
	// Fallback for nodes that don't provide EffectiveGasPrice in receipts.
	if client == nil || receipt.GasUsed == 0 {
		return big.NewInt(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	tx, _, err := client.TransactionByHash(ctx, txHash)
	cancel()
	if err != nil || tx == nil {
		return big.NewInt(0)
	}

	// Legacy/AccessList transactions still have a fixed gasPrice.
	if tx.Type() < types.DynamicFeeTxType {
		gp := tx.GasPrice()
		if gp == nil || gp.Sign() <= 0 {
			return big.NewInt(0)
		}
		return new(big.Int).Mul(gp, new(big.Int).SetUint64(receipt.GasUsed))
	}

	// Dynamic fee: effectiveGasPrice = min(maxFeePerGas, baseFee + maxPriorityFeePerGas)
	if receipt.BlockNumber == nil {
		gp := tx.GasPrice()
		if gp == nil || gp.Sign() <= 0 {
			return big.NewInt(0)
		}
		return new(big.Int).Mul(gp, new(big.Int).SetUint64(receipt.GasUsed))
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	header, err := client.HeaderByNumber(ctx2, receipt.BlockNumber)
	cancel2()
	if err != nil || header == nil || header.BaseFee == nil {
		gp := tx.GasPrice()
		if gp == nil || gp.Sign() <= 0 {
			return big.NewInt(0)
		}
		return new(big.Int).Mul(gp, new(big.Int).SetUint64(receipt.GasUsed))
	}

	tipCap := tx.GasTipCap()
	if tipCap == nil {
		tipCap = big.NewInt(0)
	}
	feeCap := tx.GasFeeCap()
	if feeCap == nil || feeCap.Sign() <= 0 {
		feeCap = tx.GasPrice()
	}

	price := new(big.Int).Add(header.BaseFee, tipCap)
	if feeCap != nil && feeCap.Sign() > 0 && feeCap.Cmp(price) < 0 {
		price = feeCap
	}
	if price == nil || price.Sign() <= 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Mul(price, new(big.Int).SetUint64(receipt.GasUsed))
}

func (s *LiquidityService) exitTokenSyncDurations() (time.Duration, time.Duration) {
	timeout := 30 * time.Second
	poll := 500 * time.Millisecond
	if config.AppConfig != nil {
		if config.AppConfig.ExitTokenSyncTimeoutSeconds > 0 {
			timeout = time.Duration(config.AppConfig.ExitTokenSyncTimeoutSeconds) * time.Second
		}
		if config.AppConfig.ExitTokenSyncPollMillis > 0 {
			poll = time.Duration(config.AppConfig.ExitTokenSyncPollMillis) * time.Millisecond
		}
	}
	if poll < 100*time.Millisecond {
		poll = 100 * time.Millisecond
	}
	if timeout < poll {
		timeout = poll
	}
	return timeout, poll
}

func (s *LiquidityService) getReceiptWithRetry(client *ethclient.Client, txHash common.Hash) (*types.Receipt, error) {
	if client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	timeout, poll := s.exitTokenSyncDurations()
	deadline := time.Now().Add(timeout)
	var lastErr error
	readReceipt := func() (*types.Receipt, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		receipt, err := client.TransactionReceipt(ctx, txHash)
		cancel()
		return receipt, err
	}

	if receipt, err := readReceipt(); err == nil && receipt != nil {
		return receipt, nil
	} else {
		lastErr = err
	}

	for _, delay := range s.fastSyncDurations() {
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(delay)
		receipt, err := readReceipt()
		if err == nil && receipt != nil {
			return receipt, nil
		}
		lastErr = err
	}

	for {
		if time.Now().After(deadline) {
			if lastErr != nil {
				return nil, fmt.Errorf("fetch tx receipt timeout %s: %w", txHash.Hex(), lastErr)
			}
			return nil, fmt.Errorf("fetch tx receipt timeout %s", txHash.Hex())
		}
		time.Sleep(poll)
		receipt, err := readReceipt()
		if err == nil && receipt != nil {
			return receipt, nil
		}
		lastErr = err
	}
}

func (s *LiquidityService) waitTokenBalanceAtLeast(client *ethclient.Client, token common.Address, wallet common.Address, min *big.Int, label string) (*big.Int, error) {
	bal, err := blockchain.GetTokenBalanceWithClient(client, token, wallet)
	if bal == nil {
		bal = big.NewInt(0)
	}
	if err != nil {
		return bal, err
	}
	if min == nil || min.Sign() <= 0 || bal.Cmp(min) >= 0 {
		return bal, nil
	}

	timeout, poll := s.exitTokenSyncDurations()
	deadline := time.Now().Add(timeout)
	log.Printf("[Liquidity] 等待 RPC 同步 %s 余额 (%s): have=%s want>=%s timeout=%s poll=%s", label, token.Hex(), bal.String(), min.String(), timeout.String(), poll.String())

	for _, delay := range s.fastSyncDurations() {
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(delay)
		bal, err = blockchain.GetTokenBalanceWithClient(client, token, wallet)
		if bal == nil {
			bal = big.NewInt(0)
		}
		if err == nil && bal.Cmp(min) >= 0 {
			log.Printf("[Liquidity] RPC balance synced for %s (%s): %s", label, token.Hex(), bal.String())
			return bal, nil
		}
	}

	for time.Now().Before(deadline) {
		time.Sleep(poll)
		bal, err = blockchain.GetTokenBalanceWithClient(client, token, wallet)
		if bal == nil {
			bal = big.NewInt(0)
		}
		if err == nil && bal.Cmp(min) >= 0 {
			log.Printf("[Liquidity] RPC 同步完成 %s 余额 (%s): %s", label, token.Hex(), bal.String())
			return bal, nil
		}
	}
	if err != nil {
		return bal, fmt.Errorf("等待 RPC 同步 %s 余额失败 (%s): %w", label, token.Hex(), err)
	}
	return bal, fmt.Errorf("等待 RPC 同步 %s 余额超时 (%s): have=%s want>=%s", label, token.Hex(), bal.String(), min.String())
}

func (s *LiquidityService) ExitTaskToUSDT(userID uint, task *models.StrategyTask, sweepWallet bool) ([]string, error) {
	return s.ExitTaskToUSDTWithOptions(userID, task, sweepWallet, TxOptions{})
}

func (s *LiquidityService) WithdrawTaskLiquidityOnly(userID uint, task *models.StrategyTask) ([]string, error) {
	return s.WithdrawTaskLiquidityOnlyWithOptions(userID, task, TxOptions{})
}

func (s *LiquidityService) WithdrawTaskLiquidityOnlyWithOptions(userID uint, task *models.StrategyTask, opts TxOptions) ([]string, error) {
	opts.GasMultiplier = normalizeGasMultiplier(opts.GasMultiplier)
	_, partialExit, err := ValidateExitPercent(opts.ExitPercent)
	if err != nil {
		return nil, err
	}
	if partialExit {
		return nil, fmt.Errorf("partial exits must use ExitTaskToUSDTWithOptions so recovered tokens are converted to stablecoin")
	}
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	task.Chain = config.NormalizeChain(task.Chain)
	exec, err := chainexec.GetEVM(task.Chain)
	if err != nil {
		return nil, err
	}
	cc := exec.Config()

	wallet, err := s.walletService.ResolveTaskWallet(userID, task.WalletID, task.WalletAddress)
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
	if !common.IsHexAddress(cc.StableAddress) {
		return nil, fmt.Errorf("stable address not set for chain=%s", exec.Chain())
	}
	usdtAddr := common.HexToAddress(cc.StableAddress)

	switch strings.ToLower(strings.TrimSpace(task.PoolVersion)) {
	case "v4":
		return s.exitV4ToUSDT(exec, privateKey, walletAddr, usdtAddr, task, false, opts)
	default:
		return s.exitV3ToUSDT(exec, privateKey, walletAddr, usdtAddr, task, false, opts)
	}
}

func (s *LiquidityService) ExitTaskToUSDTWithOptions(userID uint, task *models.StrategyTask, sweepWallet bool, opts TxOptions) ([]string, error) {
	opts.GasMultiplier = normalizeGasMultiplier(opts.GasMultiplier)
	_, partialExit, err := ValidateExitPercent(opts.ExitPercent)
	if err != nil {
		return nil, err
	}
	if partialExit && sweepWallet {
		// Partial exits must only swap the tokens recovered by this exit.
		// Sweeping the whole wallet would exceed the requested position percentage.
		sweepWallet = false
	}
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	task.Chain = config.NormalizeChain(task.Chain)
	exec, err := chainexec.GetEVM(task.Chain)
	if err != nil {
		return nil, err
	}
	cc := exec.Config()
	client := exec.Client()

	wallet, err := s.walletService.ResolveTaskWallet(userID, task.WalletID, task.WalletAddress)
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
	if !common.IsHexAddress(cc.StableAddress) {
		return nil, fmt.Errorf("stable address not set for chain=%s", exec.Chain())
	}
	usdtAddr := common.HexToAddress(cc.StableAddress)

	// Capture balance before exiting (used for "actual received" and gas cost).
	usdtBefore, _ := blockchain.GetTokenBalanceWithClient(client, usdtAddr, walletAddr)
	if usdtBefore == nil {
		usdtBefore = big.NewInt(0)
	}
	bnbBefore, _ := blockchain.GetBalanceWithClient(client, walletAddr)
	if bnbBefore == nil {
		bnbBefore = big.NewInt(0)
	}

	// SweepWallet mode needs to be resilient to public RPC lag/load-balancing:
	// after liquidity removal is mined, balanceOf() may still return an old state for a short time.
	// We capture pre-exit balances and later enforce "min expected" balances (pre + receipt delta)
	// before swapping, so we don't miss tokens due to stale RPC reads.
	var sweepPreBalances map[common.Address]*big.Int
	if sweepWallet {
		t0, t1, tokErr := s.resolveTaskTokenAddresses(task)
		if tokErr != nil {
			log.Printf("[Liquidity] Warning: resolveTaskTokenAddresses (pre-exit) failed, skip expected-balance check: %v", tokErr)
		} else {
			sweepPreBalances = make(map[common.Address]*big.Int)
			seen := make(map[common.Address]struct{})
			for _, tok := range []common.Address{t0, t1} {
				if tok == (common.Address{}) || tok == usdtAddr {
					continue
				}
				if _, ok := seen[tok]; ok {
					continue
				}
				seen[tok] = struct{}{}
				bal, err := blockchain.GetTokenBalanceWithClient(client, tok, walletAddr)
				if err != nil {
					log.Printf("[Liquidity] Warning: read pre-exit token balance failed (%s): %v", tok.Hex(), err)
					bal = big.NewInt(0)
				}
				if bal == nil {
					bal = big.NewInt(0)
				}
				sweepPreBalances[tok] = cloneBig(bal)
			}
		}
	}

	var txHashes []string
	// When `sweepWallet` is enabled, we prefer sweeping the full wallet balance (清仓)
	// for the pool tokens, instead of only swapping the LP exit deltas.
	swapDeltas := !sweepWallet
	var exitErr error
	skipExit := sweepWallet && task.ExitLiquidityRemoved
	if skipExit {
		log.Printf("[Liquidity] Exit-to-USDT: liquidity already removed (task #%d), skipping exit and retrying swap only", task.ID)
	} else {
		switch strings.ToLower(strings.TrimSpace(task.PoolVersion)) {
		case "v4":
			txHashes, exitErr = s.exitV4ToUSDT(exec, privateKey, walletAddr, usdtAddr, task, swapDeltas, opts)
		default:
			txHashes, exitErr = s.exitV3ToUSDT(exec, privateKey, walletAddr, usdtAddr, task, swapDeltas, opts)
		}
	}

	var sweepErr error
	// Only swap/sweep after a confirmed successful liquidity exit.
	// If liquidity removal failed, keep funds untouched and let the retry strategy continue exiting first.
	if sweepWallet && exitErr == nil {
		// Mark "liquidity removed" before swapping, so a swap failure won't retry removing liquidity again.
		if !task.ExitLiquidityRemoved {
			if database.DB != nil {
				_ = database.DB.Model(task).Updates(map[string]interface{}{"exit_liquidity_removed": true}).Error
			}
			task.ExitLiquidityRemoved = true
		}

		var expectedMinBalances map[common.Address]*big.Int
		if exitErr == nil && len(sweepPreBalances) > 0 {
			if exitHash, ok := firstTxHash(txHashes); ok {
				receipt, rerr := s.getReceiptWithRetry(client, exitHash)
				if rerr != nil {
					log.Printf("[Liquidity] Warning: fetch exit receipt failed, skip expected-balance check: %v", rerr)
				} else if receipt != nil {
					expectedMinBalances = make(map[common.Address]*big.Int)
					for tok, before := range sweepPreBalances {
						delta := ReceiptTokenTransferDelta(receipt, tok, walletAddr)
						if delta.Sign() <= 0 {
							continue
						}
						want := new(big.Int).Add(cloneBig(before), delta)
						if cur, ok := expectedMinBalances[tok]; !ok || want.Cmp(cur) > 0 {
							expectedMinBalances[tok] = want
						}
					}

					if len(expectedMinBalances) == 0 {
						expectedMinBalances = nil
					}
				}
			}
		}

		sweepHashes, err := s.swapWalletTokensToUSDT(exec, privateKey, walletAddr, usdtAddr, task, expectedMinBalances, sweepPreBalances)
		if len(sweepHashes) > 0 {
			txHashes = append(txHashes, sweepHashes...)
		}
		sweepErr = err
		if err != nil && exitErr == nil {
			sweepErr = &SwapToUSDTError{Err: err}
		}
		if sweepErr == nil && task.ExitLiquidityRemoved {
			// Full exit (remove + swap) succeeded; clear the intermediate state.
			if database.DB != nil {
				_ = database.DB.Model(task).Updates(map[string]interface{}{"exit_liquidity_removed": false}).Error
			}
			task.ExitLiquidityRemoved = false
		}
	} else if sweepWallet && exitErr != nil {
		log.Printf("[Liquidity] Exit failed, skip swap-to-USDT sweep: %v", exitErr)
	}

	// 无论是否有错误，都计算实际收到的 USDT 和消耗的 Gas
	// 这样即使部分操作失败，也能正确记录已收到的金额
	usdtAfter, _ := blockchain.GetTokenBalanceWithClient(client, usdtAddr, walletAddr)
	if usdtAfter == nil {
		usdtAfter = big.NewInt(0)
	}
	bnbAfter, _ := blockchain.GetBalanceWithClient(client, walletAddr)
	if bnbAfter == nil {
		bnbAfter = big.NewInt(0)
	}

	// Actual USDT received (delta) for this exit.
	actualReceived := new(big.Int).Sub(usdtAfter, usdtBefore)
	if actualReceived.Sign() < 0 {
		actualReceived = big.NewInt(0)
	}
	// Gas spent in native BNB (delta).
	gasSpent := new(big.Int).Sub(bnbBefore, bnbAfter)
	if gasSpent.Sign() < 0 {
		gasSpent = big.NewInt(0)
	}

	// Prefer receipt-derived deltas when available, to avoid public RPC "stale balanceOf()"
	// causing under-counting (especially for the swap-to-USDT phase).
	receiptUSDT := big.NewInt(0)
	receiptGas := big.NewInt(0)
	for _, h := range extractTxHashes(txHashes) {
		receipt, rerr := s.getReceiptWithRetry(client, h)
		if rerr != nil || receipt == nil {
			continue
		}
		receiptUSDT.Add(receiptUSDT, ReceiptTokenTransferDelta(receipt, usdtAddr, walletAddr))
		receiptGas.Add(receiptGas, s.gasCostWeiFromReceipt(client, h, receipt))
	}
	if receiptUSDT.Cmp(actualReceived) > 0 {
		actualReceived = receiptUSDT
	}
	if receiptGas.Cmp(gasSpent) > 0 {
		gasSpent = receiptGas
	}

	finalStableAfter := cloneBig(usdtAfter)
	if receiptUSDT.Sign() > 0 {
		minStableAfter := new(big.Int).Add(cloneBig(usdtBefore), receiptUSDT)
		if finalStableAfter.Cmp(minStableAfter) < 0 {
			if synced, werr := s.waitTokenBalanceAtLeast(client, usdtAddr, walletAddr, minStableAfter, "stable after exit swaps"); werr == nil && synced != nil {
				finalStableAfter = synced
			} else {
				log.Printf("[Liquidity] Warning: stable balance not synced after exit; using receipt-proven minimum: have=%s want>=%s err=%v",
					finalStableAfter.String(), minStableAfter.String(), werr)
				finalStableAfter = minStableAfter
			}
		}
	}

	// Scale stablecoin units to internal USD(1e18) representation for records/PnL (Base USDT is often 6 decimals).
	actualReceivedWei, cerr := convert.ScaleDecimals(actualReceived, cc.StableDecimals, 18)
	if cerr != nil || actualReceivedWei == nil {
		log.Printf("[Liquidity] Warning: scale stable received failed: %v (chain=%s stableDecimals=%d)", cerr, exec.Chain(), cc.StableDecimals)
		actualReceivedWei = new(big.Int).Set(actualReceived)
	}
	closeStableBeforeWei, berr := convert.ScaleDecimals(usdtBefore, cc.StableDecimals, 18)
	if berr != nil || closeStableBeforeWei == nil {
		closeStableBeforeWei = new(big.Int).Set(usdtBefore)
	}
	closeStableAfterWei, aerr := convert.ScaleDecimals(finalStableAfter, cc.StableDecimals, 18)
	if aerr != nil || closeStableAfterWei == nil {
		closeStableAfterWei = new(big.Int).Set(finalStableAfter)
	}

	mainHash := ""
	if h, ok := firstTxHash(txHashes); ok {
		mainHash = h.Hex()
	}

	finalizeTradeRecord := exitErr == nil && sweepErr == nil && !partialExit
	shouldUpdateRecords := finalizeTradeRecord || actualReceived.Sign() > 0 || gasSpent.Sign() > 0 || mainHash != ""
	if shouldUpdateRecords {
		taskCopy := *task
		mainHashCopy := strings.TrimSpace(mainHash)
		actualReceivedCopy := cloneBig(actualReceivedWei)
		gasSpentCopy := cloneBig(gasSpent)
		closeStableBeforeCopy := cloneBig(closeStableBeforeWei)
		closeStableAfterCopy := cloneBig(closeStableAfterWei)
		nativePriceUSD := 0.0
		if finalizeTradeRecord {
			nativePriceUSD = pricing.GetNativePriceUSD(exec.Chain())
		}
		s.runAsync("exit_trade_record", func() error {
			trSvc := trade.NewTradeRecordService()
			if _, err := trSvc.ApplyExitDeltaWithStableAfter(&taskCopy, mainHashCopy, actualReceivedCopy, gasSpentCopy, closeStableBeforeCopy, closeStableAfterCopy, finalizeTradeRecord, nativePriceUSD); err != nil && finalizeTradeRecord {
				return trSvc.CloseLatestOpenRecordWithStableAfter(&taskCopy, mainHashCopy, actualReceivedCopy, gasSpentCopy, closeStableBeforeCopy, closeStableAfterCopy, nativePriceUSD)
			} else {
				return err
			}
		})
	}

	txHash := strings.TrimSpace(mainHash)
	amountOut := actualReceivedWei.String()
	if txHash != "" && shouldUpdateRecords {
		s.persistTransactionRecordAsync("exit_tx_record", models.Transaction{
			UserID:          task.UserID,
			Chain:           task.Chain,
			TaskID:          task.ID,
			TxHash:          txHash,
			Type:            models.TxTypeRemoveLiquidity,
			Status:          models.TxStatusConfirmed,
			FromAddress:     walletAddr.Hex(),
			TokenOutAddress: usdtAddr.Hex(),
			AmountIn:        "0",
			AmountOut:       amountOut,
			CreatedAt:       time.Now(),
		})
	}

	// 如果有错误，在更新记录后再返回错误
	if exitErr != nil || sweepErr != nil {
		var errs []error
		if exitErr != nil {
			errs = append(errs, exitErr)
		}
		if sweepErr != nil {
			errs = append(errs, sweepErr)
		}
		return txHashes, errors.Join(errs...)
	}

	return txHashes, nil
}

func (s *LiquidityService) swapWalletTokensToUSDT(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	usdtAddr common.Address,
	task *models.StrategyTask,
	expectedMinBalances map[common.Address]*big.Int,
	preBalances map[common.Address]*big.Int,
) ([]string, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}

	if exec == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	client := exec.Client()
	if client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}

	token0, token1, err := s.resolveTaskTokenAddresses(task)
	if err != nil {
		return nil, fmt.Errorf("解析退出代币地址失败: %w", err)
	}

	tokens := []struct {
		addr   common.Address
		symbol string
	}{
		{addr: token0, symbol: strings.TrimSpace(task.Token0Symbol)},
		{addr: token1, symbol: strings.TrimSpace(task.Token1Symbol)},
	}

	type sweepToken struct {
		addr   common.Address
		symbol string
	}

	seen := make(map[common.Address]struct{})
	var targets []sweepToken
	for _, tok := range tokens {
		if tok.addr == (common.Address{}) || tok.addr == usdtAddr {
			continue
		}
		if _, ok := seen[tok.addr]; ok {
			continue
		}
		seen[tok.addr] = struct{}{}
		targets = append(targets, sweepToken{addr: tok.addr, symbol: tok.symbol})
	}

	targetAddrs := make([]common.Address, 0, len(targets))
	for _, tok := range targets {
		targetAddrs = append(targetAddrs, tok.addr)
	}

	var txHashes []string
	var errs []error
	expectedRemaining := make(map[common.Address]*big.Int)
	for _, tok := range targets {
		label := tok.symbol
		if label == "" {
			label = tok.addr.Hex()
		}

		minExpected := (*big.Int)(nil)
		if expectedMinBalances != nil {
			if v, ok := expectedMinBalances[tok.addr]; ok && v != nil && v.Sign() > 0 {
				minExpected = v
			}
		}

		var bal *big.Int
		if minExpected != nil {
			var werr error
			bal, werr = s.waitTokenBalanceAtLeast(client, tok.addr, walletAddr, minExpected, label)
			if werr != nil {
				errs = append(errs, werr)
				continue
			}
		} else {
			var rerr error
			bal, rerr = blockchain.GetTokenBalanceWithClient(client, tok.addr, walletAddr)
			if rerr != nil {
				errs = append(errs, fmt.Errorf("读取 %s 余额失败 (%s): %w", label, tok.addr.Hex(), rerr))
				continue
			}
		}

		if bal == nil || bal.Sign() <= 0 {
			continue
		}

		expectedRemaining[tok.addr] = big.NewInt(0)
		toSwap := cloneBig(bal)

		if skip, _ := s.shouldSkipExitSwapToUSDT(exec, tok.addr, toSwap, walletAddr, label); skip {
			// Skip tiny swaps to avoid spending gas on < 1 USDT outputs.
			expectedRemaining[tok.addr] = cloneBig(bal)
			continue
		}

		swapTxHash, err := s.swapDeltaToUSDTWithHash(exec, privateKey, walletAddr, tok.addr, usdtAddr, toSwap, effectiveExitSlippagePercent(task))
		if err != nil {
			errs = append(errs, fmt.Errorf("清仓兑换 %s→USDT 失败 (%s) amount=%s: %w", label, tok.addr.Hex(), toSwap.String(), err))
			continue
		}
		if swapTxHash == "" {
			errs = append(errs, fmt.Errorf("清仓兑换 %s→USDT 返回空交易哈希 (%s) amount=%s", label, tok.addr.Hex(), toSwap.String()))
			continue
		}

		symbol := tok.symbol
		if symbol == "" {
			symbol = tok.addr.Hex()
		}
		txHashes = append(txHashes, fmt.Sprintf("清仓 %s→USDT|%s", symbol, swapTxHash))
	}

	// 校验：swap 后检查是否仍有余额（仅打印警告，不返回错误以避免重试导致重复兑换）
	for _, tok := range targets {
		bal, err := blockchain.GetTokenBalanceWithClient(client, tok.addr, walletAddr)
		label := tok.symbol
		if label == "" {
			label = tok.addr.Hex()
		}
		if err != nil {
			log.Printf("[Liquidity] Warning: 校验清仓后 %s 余额失败 (%s): %v", label, tok.addr.Hex(), err)
			continue
		}
		keep := expectedRemaining[tok.addr]
		if keep == nil {
			keep = big.NewInt(0)
		}
		if bal != nil && bal.Sign() > 0 && bal.Cmp(keep) > 0 {
			// 仅打印警告日志，不返回错误，避免触发重试机制导致同一代币被多次兑换
			log.Printf("[Liquidity] Warning: 清仓兑换后仍有 %s 余额未兑换 (%s): %s（允许保留 %s；可能是 swap 滑点或小额残余/跳过小额兑换）", label, tok.addr.Hex(), bal.String(), keep.String())
		}
	}

	// 只返回实际的 swap 错误，不因校验残余而返回错误
	if len(errs) > 0 {
		return txHashes, errors.Join(errs...)
	}
	return txHashes, nil
}

func (s *LiquidityService) buildAuth(client *ethclient.Client, chainID *big.Int, privateKey *ecdsa.PrivateKey, nonce uint64, value *big.Int, opts TxOptions) (*bind.TransactOpts, error) {
	if client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if chainID == nil {
		return nil, fmt.Errorf("chainID not set")
	}
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return nil, err
	}
	gasPrice, err := blockchain.GetGasPriceWithMultiplierWithClient(client, normalizeGasMultiplier(opts.GasMultiplier))
	if err != nil {
		return nil, err
	}
	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = value
	auth.GasLimit = 0 // 让节点自动估算 gas
	auth.GasPrice = gasPrice
	return auth, nil
}

func (s *LiquidityService) waitMined(client *ethclient.Client, chainID *big.Int, tx *types.Transaction) (*types.Receipt, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, fmt.Errorf("receipt is nil")
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		reason := tryGetRevertReason(client, chainID, tx, receipt)
		if reason != "" {
			return receipt, fmt.Errorf("tx reverted: %s (%s)", tx.Hash().Hex(), reason)
		}
		return receipt, fmt.Errorf("tx reverted: %s", tx.Hash().Hex())
	}
	return receipt, nil
}

func tryGetRevertReason(client *ethclient.Client, chainID *big.Int, tx *types.Transaction, receipt *types.Receipt) string {
	if client == nil || tx == nil || receipt == nil {
		return ""
	}
	if tx.To() == nil {
		return ""
	}

	var from common.Address
	if chainID != nil {
		signer := types.LatestSignerForChainID(chainID)
		if sender, err := types.Sender(signer, tx); err == nil {
			from = sender
		}
	}

	msg := ethereum.CallMsg{
		From:  from,
		To:    tx.To(),
		Value: tx.Value(),
		Data:  tx.Data(),
	}

	callCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, callErr := client.CallContract(callCtx, msg, receipt.BlockNumber)
	if callErr != nil {
		if reason := unpackRevertReasonFromError(callErr); reason != "" {
			return reason
		}
		// Some public RPCs prune history ("missing trie node"); latest often still returns the reason.
		_, callErr2 := client.CallContract(callCtx, msg, nil)
		if callErr2 != nil {
			if reason := unpackRevertReasonFromError(callErr2); reason != "" {
				return reason
			}
		}
	}

	return ""
}

func unpackRevertReasonFromError(err error) string {
	if err == nil {
		return ""
	}

	// Many RPCs include revert data as a hex string in the error text, e.g.
	// "execution reverted: reason: 0x08c379a0..."
	re := regexp.MustCompile(`0x[0-9a-fA-F]{8,}`)
	matches := re.FindAllString(err.Error(), -1)
	if len(matches) == 0 {
		return ""
	}
	hexData := matches[len(matches)-1]
	data := common.FromHex(hexData)
	if len(data) == 0 {
		return ""
	}
	reason, uerr := abi.UnpackRevert(data)
	if uerr != nil || strings.TrimSpace(reason) == "" {
		// Custom errors (e.g., Uniswap V4) won't decode via UnpackRevert.
		if len(data) >= 4 {
			selector := strings.ToLower(common.Bytes2Hex(data[:4]))
			switch selector {
			case "486aa307":
				return "PoolNotInitialized()"
			}
		}
		return ""
	}
	return reason
}

func (s *LiquidityService) exitV3ToUSDT(exec chainexec.EVMExecutor, privateKey *ecdsa.PrivateKey, walletAddr common.Address, usdtAddr common.Address, task *models.StrategyTask, swapDeltas bool, opts TxOptions) ([]string, error) {
	if exec == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	exitPercent, partialExit, err := ValidateExitPercent(opts.ExitPercent)
	if err != nil {
		return nil, err
	}
	cc := exec.Config()
	client := exec.Client()
	chainID := exec.ChainID()
	if client == nil || chainID == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}

	var txHashes []string
	var swapErr error

	// Capture initial USDT balance for calculating output (only when swapping deltas here).
	usdtBefore := big.NewInt(0)
	if swapDeltas {
		usdtBefore, _ = blockchain.GetTokenBalanceWithClient(client, usdtAddr, walletAddr)
		if usdtBefore == nil {
			usdtBefore = big.NewInt(0)
		}
	}
	pmAddrStr := strings.TrimSpace(task.V3PositionManagerAddress)
	if pmAddrStr == "" && common.IsHexAddress(task.PoolId) {
		poolAddr := common.HexToAddress(task.PoolId)
		if factory, ferr := blockchain.GetV3PoolFactoryWithClient(client, poolAddr); ferr == nil && factory != (common.Address{}) {
			fhex := strings.ToLower(factory.Hex())
			for _, dep := range cc.V3Deployments {
				wantFactory := strings.ToLower(strings.TrimSpace(dep.FactoryAddress))
				if !common.IsHexAddress(wantFactory) {
					continue
				}
				if fhex == strings.ToLower(wantFactory) && common.IsHexAddress(dep.PositionManagerAddress) {
					pmAddrStr = strings.TrimSpace(dep.PositionManagerAddress)
					break
				}
			}
		}
	}
	if pmAddrStr == "" && common.IsHexAddress(cc.DefaultV3PositionManagerAddress) {
		pmAddrStr = strings.TrimSpace(cc.DefaultV3PositionManagerAddress)
	}
	if !common.IsHexAddress(pmAddrStr) {
		for _, dep := range cc.V3Deployments {
			if common.IsHexAddress(dep.PositionManagerAddress) {
				pmAddrStr = strings.TrimSpace(dep.PositionManagerAddress)
				break
			}
		}
	}

	if !common.IsHexAddress(pmAddrStr) {
		return nil, fmt.Errorf("V3 position manager address not configured")
	}
	pmAddr := common.HexToAddress(pmAddrStr)

	// 获取 TokenID
	tokenId, err := convert.ParseBigIntFlexible(task.V3TokenID)
	if err != nil {
		return nil, fmt.Errorf("missing/invalid V3 tokenId: %w", err)
	}
	// 检查 tokenId 是否为 0
	if tokenId == nil || tokenId.Sign() == 0 {
		return nil, fmt.Errorf("V3 tokenId 不能为 0，请检查任务数据")
	}

	// 1. 获取 Position 信息 (确认 Liquidity 和 Token)
	v3pm, err := blockchain.NewV3PositionManager(pmAddr, client)
	if err != nil {
		return nil, fmt.Errorf("init v3 position manager failed: %w", err)
	}
	posInfo, err := v3pm.Positions(nil, tokenId)
	if err != nil {
		return nil, fmt.Errorf("read v3 position failed: %w", err)
	}
	token0 := posInfo.Token0
	token1 := posInfo.Token1
	liq := big.NewInt(0)
	if posInfo.Liquidity != nil {
		liq = cloneBig(posInfo.Liquidity)
	}
	owed0 := big.NewInt(0)
	owed1 := big.NewInt(0)
	if posInfo.TokensOwed0 != nil {
		owed0 = cloneBig(posInfo.TokensOwed0)
	}
	if posInfo.TokensOwed1 != nil {
		owed1 = cloneBig(posInfo.TokensOwed1)
	}
	// Idempotency: if the position is already emptied (no liquidity + no fees owed), skip.
	if liq.Sign() <= 0 && owed0.Sign() <= 0 && owed1.Sign() <= 0 {
		log.Printf("[Liquidity] V3 exit: position already empty, skipping exit tokenId=%s", tokenId.String())
		return txHashes, nil
	}
	if partialExit && liq.Sign() <= 0 {
		return nil, fmt.Errorf("cannot partially exit V3 position with zero liquidity")
	}
	removeLiq := big.NewInt(0)
	if liq.Sign() > 0 {
		removeLiq, err = exitLiquidityForPercent(liq, exitPercent)
		if err != nil {
			return nil, err
		}
	}

	// 撤仓滑点保护：按当前池价估算应得的 token0/token1，给 amount0Min/amount1Min 设一个
	// (1 - 滑点) 下限，防止有人在撤仓前操纵价格把仓位按畸形比例抽走。读取价格失败时回退为 0，
	// 避免因临时 RPC 问题卡住撤仓（撤仓重试会随重试次数放大滑点）。
	amount0Min := big.NewInt(0)
	amount1Min := big.NewInt(0)
	if removeLiq.Sign() > 0 && common.IsHexAddress(task.PoolId) {
		if sqrtPriceX96, _, slot0Err := blockchain.GetV3PoolSlot0WithClient(client, common.HexToAddress(task.PoolId)); slot0Err != nil {
			log.Printf("[Liquidity] V3 exit: read slot0 for min-amount failed, using amount*Min=0: %v", slot0Err)
		} else {
			amount0Min, amount1Min = exitMinAmounts(sqrtPriceX96, posInfo.TickLower, posInfo.TickUpper, removeLiq, effectiveExitSlippagePercent(task))
		}
	}
	deadline := big.NewInt(time.Now().Add(20 * time.Minute).Unix())

	maxUint128 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	collectParams := blockchain.V3CollectParams{
		TokenId:    tokenId,
		Recipient:  walletAddr,
		Amount0Max: maxUint128,
		Amount1Max: maxUint128,
	}

	// Capture pre-exit balances for fallback delta calculation.
	b0Before := big.NewInt(0)
	b1Before := big.NewInt(0)
	if swapDeltas {
		b0Before, _ = blockchain.GetTokenBalanceWithClient(client, token0, walletAddr)
		b1Before, _ = blockchain.GetTokenBalanceWithClient(client, token1, walletAddr)
		if b0Before == nil {
			b0Before = big.NewInt(0)
		}
		if b1Before == nil {
			b1Before = big.NewInt(0)
		}
	}

	var exitReceipt *types.Receipt
	// Prefer a single tx via NPM.multicall(decreaseLiquidity + collect). Fallback to 2 txs when multicall is unavailable.
	if liq.Sign() > 0 {
		decParams := blockchain.V3DecreaseLiquidityParams{
			TokenId:    tokenId,
			Liquidity:  removeLiq, // uint128
			Amount0Min: amount0Min,
			Amount1Min: amount1Min,
			Deadline:   deadline,
		}

		decData, derr := v3pm.Pack("decreaseLiquidity", decParams)
		colData, cerr := v3pm.Pack("collect", collectParams)
		if derr == nil && cerr == nil {
			nonce, err := blockchain.GetNonceWithClient(client, walletAddr)
			if err != nil {
				return nil, err
			}
			auth, err := s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
			if err != nil {
				return nil, err
			}

			tuneZapTxGasLimit("V3 exit NPM multicall", auth, func(o *bind.TransactOpts) (*types.Transaction, error) {
				return v3pm.Multicall(o, [][]byte{decData, colData})
			})

			log.Printf("[Liquidity] V3 exit: Calling NPM.multicall(decreaseLiquidity+collect) tokenId=%s liquidity=%s currentLiquidity=%s exitPercent=%.6f amount0Min=%s amount1Min=%s",
				tokenId.String(), removeLiq.String(), liq.String(), exitPercent, amount0Min.String(), amount1Min.String())

			tx, err := v3pm.Multicall(auth, [][]byte{decData, colData})
			if err == nil && tx != nil {
				log.Printf("[Liquidity] V3 exit: tx sent %s", tx.Hash().Hex())
				txHashes = append(txHashes, "撤出流动性|"+tx.Hash().Hex())

				receipt, err := s.waitMined(client, chainID, tx)
				if err != nil {
					return txHashes, fmt.Errorf("NPM.multicall tx failed: %w", err)
				}
				exitReceipt = receipt
			} else if err != nil {
				log.Printf("[Liquidity] Warning: NPM.multicall failed, falling back to decreaseLiquidity+collect: %v", err)
			}
		} else {
			if derr != nil {
				log.Printf("[Liquidity] Warning: pack decreaseLiquidity calldata failed, falling back: %v", derr)
			}
			if cerr != nil {
				log.Printf("[Liquidity] Warning: pack collect calldata failed, falling back: %v", cerr)
			}
		}

		if exitReceipt == nil {
			nonce, err := blockchain.GetNonceWithClient(client, walletAddr)
			if err != nil {
				return nil, err
			}
			auth, err := s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
			if err != nil {
				return nil, err
			}

			tuneZapTxGasLimit("V3 exit NPM decreaseLiquidity", auth, func(o *bind.TransactOpts) (*types.Transaction, error) {
				return v3pm.DecreaseLiquidity(o, decParams)
			})

			log.Printf("[Liquidity] V3 exit: Calling NPM.decreaseLiquidity tokenId=%s liquidity=%s currentLiquidity=%s exitPercent=%.6f amount0Min=%s amount1Min=%s",
				tokenId.String(), removeLiq.String(), liq.String(), exitPercent, amount0Min.String(), amount1Min.String())

			decTx, err := v3pm.DecreaseLiquidity(auth, decParams)
			if err != nil {
				return nil, fmt.Errorf("decreaseLiquidity call failed: %w", err)
			}
			log.Printf("[Liquidity] V3 exit: decreaseLiquidity tx sent %s", decTx.Hash().Hex())
			txHashes = append(txHashes, "减少流动性|"+decTx.Hash().Hex())
			if _, err := s.waitMined(client, chainID, decTx); err != nil {
				return txHashes, fmt.Errorf("decreaseLiquidity tx failed: %w", err)
			}

			nonce, err = blockchain.GetNonceWithClient(client, walletAddr)
			if err != nil {
				return txHashes, err
			}
			auth, err = s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
			if err != nil {
				return txHashes, err
			}

			tuneZapTxGasLimit("V3 exit NPM collect", auth, func(o *bind.TransactOpts) (*types.Transaction, error) {
				return v3pm.Collect(o, collectParams)
			})

			log.Printf("[Liquidity] V3 exit: Calling NPM.collect tokenId=%s", tokenId.String())
			colTx, err := v3pm.Collect(auth, collectParams)
			if err != nil {
				return txHashes, fmt.Errorf("collect call failed: %w", err)
			}
			log.Printf("[Liquidity] V3 exit: collect tx sent %s", colTx.Hash().Hex())

			// Ensure sweepWallet expected-balance check uses the tx that actually transfers tokens to the wallet.
			txHashes = append([]string{"撤出流动性|" + colTx.Hash().Hex()}, txHashes...)

			receipt, err := s.waitMined(client, chainID, colTx)
			if err != nil {
				return txHashes, fmt.Errorf("collect tx failed: %w", err)
			}
			exitReceipt = receipt
		}
	} else {
		nonce, err := blockchain.GetNonceWithClient(client, walletAddr)
		if err != nil {
			return nil, err
		}
		auth, err := s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
		if err != nil {
			return nil, err
		}

		tuneZapTxGasLimit("V3 exit NPM collect", auth, func(o *bind.TransactOpts) (*types.Transaction, error) {
			return v3pm.Collect(o, collectParams)
		})

		log.Printf("[Liquidity] V3 exit: Calling NPM.collect (fees only) tokenId=%s", tokenId.String())
		tx, err := v3pm.Collect(auth, collectParams)
		if err != nil {
			return nil, fmt.Errorf("collect call failed: %w", err)
		}
		log.Printf("[Liquidity] V3 exit: collect tx sent %s", tx.Hash().Hex())
		txHashes = append(txHashes, "撤出流动性|"+tx.Hash().Hex())

		receipt, err := s.waitMined(client, chainID, tx)
		if err != nil {
			return txHashes, fmt.Errorf("collect tx failed: %w", err)
		}
		exitReceipt = receipt
	}

	if exitReceipt == nil {
		return txHashes, fmt.Errorf("exit receipt is nil")
	}

	remainingLiq := new(big.Int).Sub(cloneBig(liq), cloneBig(removeLiq))
	if remainingLiq.Sign() < 0 {
		remainingLiq = big.NewInt(0)
	}
	if posAfter, readErr := v3pm.Positions(nil, tokenId); readErr == nil && posAfter.Liquidity != nil {
		remainingLiq = cloneBig(posAfter.Liquidity)
	} else if readErr != nil {
		log.Printf("[Liquidity] Warning: read V3 position after exit failed, using calculated remaining liquidity task_id=%d tokenId=%s remaining=%s: %v",
			task.ID, tokenId.String(), remainingLiq.String(), readErr)
	}
	persistTaskCurrentLiquidity(task, remainingLiq)

	// sweepWallet mode does the swap at a higher level via swapWalletTokensToUSDT().
	if !swapDeltas {
		return txHashes, nil
	}

	// 5. 计算获得的代币并 Swap 回 USDT
	hasTokenTransferLog := func(tok common.Address) bool {
		if exitReceipt == nil || tok == (common.Address{}) {
			return false
		}
		for _, lg := range exitReceipt.Logs {
			if lg == nil || lg.Address != tok || len(lg.Topics) == 0 || lg.Topics[0] != erc20TransferEventID {
				continue
			}
			return true
		}
		return false
	}

	d0 := ReceiptTokenTransferDelta(exitReceipt, token0, walletAddr)
	d1 := ReceiptTokenTransferDelta(exitReceipt, token1, walletAddr)

	// Fallback to balance deltas only when Transfer logs are not available for the token.
	if token0 != (common.Address{}) && !hasTokenTransferLog(token0) {
		b0After, _ := blockchain.GetTokenBalanceWithClient(client, token0, walletAddr)
		if b0After == nil {
			b0After = big.NewInt(0)
		}
		d0 = new(big.Int).Sub(b0After, b0Before)
	}
	if token1 != (common.Address{}) && !hasTokenTransferLog(token1) {
		b1After, _ := blockchain.GetTokenBalanceWithClient(client, token1, walletAddr)
		if b1After == nil {
			b1After = big.NewInt(0)
		}
		d1 = new(big.Int).Sub(b1After, b1Before)
	}
	if d0.Sign() < 0 {
		d0 = big.NewInt(0)
	}
	if d1.Sign() < 0 {
		d1 = big.NewInt(0)
	}

	if swapDeltas {
		var swapErrs []error
		if d0.Sign() > 0 && token0 != usdtAddr {
			log.Printf("[Liquidity] V3 exit: Got %s token0, swapping to USDT...", d0.String())
			symbol := "Token0"
			if task.Token0Symbol != "" {
				symbol = task.Token0Symbol
			}
			if skip, _ := s.shouldSkipExitSwapToUSDT(exec, token0, d0, walletAddr, symbol); !skip {
				if swapTxHash, err := s.swapDeltaToUSDTWithHash(exec, privateKey, walletAddr, token0, usdtAddr, d0, effectiveExitSlippagePercent(task)); err != nil {
					swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT failed: %w", symbol, err))
				} else if swapTxHash != "" {
					txHashes = append(txHashes, fmt.Sprintf("兑换 %s→USDT|%s", symbol, swapTxHash))
				} else {
					swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT returned empty tx hash", symbol))
				}
			}
		}
		if d1.Sign() > 0 && token1 != usdtAddr {
			log.Printf("[Liquidity] V3 exit: Got %s token1, swapping to USDT...", d1.String())
			symbol := "Token1"
			if task.Token1Symbol != "" {
				symbol = task.Token1Symbol
			}
			if skip, _ := s.shouldSkipExitSwapToUSDT(exec, token1, d1, walletAddr, symbol); !skip {
				if swapTxHash, err := s.swapDeltaToUSDTWithHash(exec, privateKey, walletAddr, token1, usdtAddr, d1, effectiveExitSlippagePercent(task)); err != nil {
					swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT failed: %w", symbol, err))
				} else if swapTxHash != "" {
					txHashes = append(txHashes, fmt.Sprintf("兑换 %s→USDT|%s", symbol, swapTxHash))
				} else {
					swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT returned empty tx hash", symbol))
				}
			}
		}
		if len(swapErrs) > 0 {
			swapErr = errors.Join(swapErrs...)
		}
	}

	// 6. Record Transaction (Calc total USDT received)
	usdtAfter, _ := blockchain.GetTokenBalanceWithClient(client, usdtAddr, walletAddr)
	if usdtAfter == nil {
		usdtAfter = big.NewInt(0)
	}

	totalUSDTReceived := new(big.Int).Sub(usdtAfter, usdtBefore) // Calculate delta
	if totalUSDTReceived.Sign() < 0 {
		totalUSDTReceived = big.NewInt(0)
	}

	// Use the first hash (exit tx) as the main hash, or a new one?
	// The user wants to see one entry for "Remove Liquidity".
	mainHash := ""
	if len(txHashes) > 0 {
		parts := strings.Split(txHashes[0], "|")
		if len(parts) >= 2 {
			mainHash = parts[1]
		} else {
			mainHash = txHashes[0]
		}
	}

	// 只有当 swapDeltas=true 时才在这里创建 Transaction 记录
	// 当 swapDeltas=false (sweepWallet 模式) 时，由顶层 ExitTaskToUSDTWithOptions 统一创建
	// 这样可以避免创建 AmountOut=0 的记录
	if swapDeltas && mainHash != "" && !partialExit {
		txRecord := models.Transaction{
			UserID:          task.UserID,
			Chain:           task.Chain,
			TaskID:          task.ID,
			TxHash:          mainHash, // Use exit hash (multicall or collect)
			Type:            models.TxTypeRemoveLiquidity,
			Status:          models.TxStatusConfirmed,
			FromAddress:     walletAddr.Hex(),
			ToAddress:       pmAddr.Hex(),
			TokenInAddress:  pmAddr.Hex(), // Representing the pool/position
			TokenOutAddress: usdtAddr.Hex(),
			AmountIn:        "0",
			AmountOut:       totalUSDTReceived.String(),
			CreatedAt:       time.Now(),
		}
		s.persistTransactionRecordAsync("exit_v3_tx_record", txRecord)
	}

	if swapErr != nil {
		return txHashes, &SwapToUSDTError{Err: swapErr}
	}
	return txHashes, nil
}

func (s *LiquidityService) exitV4ToUSDT(exec chainexec.EVMExecutor, privateKey *ecdsa.PrivateKey, walletAddr common.Address, usdtAddr common.Address, task *models.StrategyTask, swapDeltas bool, opts TxOptions) ([]string, error) {
	var txHashes []string
	var swapErr error

	if exec == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	exitPercent, partialExit, err := ValidateExitPercent(opts.ExitPercent)
	if err != nil {
		return nil, err
	}
	cc := exec.Config()
	client := exec.Client()
	chainID := exec.ChainID()
	if client == nil || chainID == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}

	if !common.IsHexAddress(cc.UniswapV4PoolManagerAddress) {
		return nil, fmt.Errorf("UNISWAP_V4_POOL_MANAGER_ADDRESS not set for chain=%s", exec.Chain())
	}
	if !common.IsHexAddress(cc.UniswapV4PositionManagerAddress) {
		return nil, fmt.Errorf("UNISWAP_V4_POSITION_MANAGER_ADDRESS not set for chain=%s", exec.Chain())
	}

	// Capture initial USDT balance
	usdtBefore, _ := blockchain.GetTokenBalanceWithClient(client, usdtAddr, walletAddr)
	if usdtBefore == nil {
		usdtBefore = big.NewInt(0)
	}

	tokenId, err := convert.ParseBigIntFlexible(task.V4TokenID)
	if err != nil {
		return nil, fmt.Errorf("missing/invalid V4 tokenId: %w", err)
	}
	// 检查 tokenId 是否为 0
	if tokenId == nil || tokenId.Sign() == 0 {
		return nil, fmt.Errorf("V4 tokenId 不能为 0，请检查任务数据")
	}

	// Resolve V4 PoolKey from Task
	// poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
	if task.Token0Address == "" || task.Token1Address == "" {
		return nil, fmt.Errorf("missing token info in task for V4 pool exit")
	}
	c0 := common.HexToAddress(task.Token0Address)
	c1 := common.HexToAddress(task.Token1Address)

	if c0.Hex() == "0x0000000000000000000000000000000000000000" || c1.Hex() == "0x0000000000000000000000000000000000000000" {
		return nil, fmt.Errorf("native currency not supported for swap flow (use WBNB)")
	}

	b0Before, _ := blockchain.GetTokenBalanceWithClient(client, c0, walletAddr)
	b1Before, _ := blockchain.GetTokenBalanceWithClient(client, c1, walletAddr)

	v4pmAddr := common.HexToAddress(cc.UniswapV4PositionManagerAddress)
	v4pm, err := blockchain.NewV4PositionManager(v4pmAddr, client)
	if err != nil {
		return nil, fmt.Errorf("init v4 position manager failed: %w", err)
	}

	pos, posErr := blockchain.GetV4PositionInfo(v4pmAddr, common.HexToAddress(cc.UniswapV4PoolManagerAddress), task.PoolId, tokenId)
	liq := big.NewInt(0)
	if posErr == nil && pos != nil {
		if pos.Liquidity != nil {
			liq = cloneBig(pos.Liquidity)
		}
	}

	// If we couldn't query on-chain position, fall back to task.current_liquidity.
	if posErr != nil {
		if partialExit {
			return nil, fmt.Errorf("read V4 on-chain liquidity failed for partial exit: %w", posErr)
		}
		liq, err = convert.ParseBigIntFlexible(task.CurrentLiquidity)
		if err != nil || liq == nil || liq.Sign() <= 0 {
			return nil, fmt.Errorf("missing/invalid current_liquidity for V4 remove and failed to query on-chain position: %w", posErr)
		}
		liq = cloneBig(liq)
	}
	if liq.Sign() <= 0 {
		return nil, fmt.Errorf("V4 position has no liquidity")
	}
	removeLiq, err := exitLiquidityForPercent(liq, exitPercent)
	if err != nil {
		return nil, err
	}

	// 撤仓滑点保护：按当前池价估算应得数量给 amount0Min/amount1Min 设下限（同 V3 逻辑）。
	// 读取失败时回退为 0，避免卡住撤仓。
	amount0Min := big.NewInt(0)
	amount1Min := big.NewInt(0)
	if removeLiq.Sign() > 0 && common.IsHexAddress(cc.UniswapV4StateViewAddress) && common.IsHexAddress(cc.UniswapV4PoolManagerAddress) {
		exitTickLower, exitTickUpper := task.TickLower, task.TickUpper
		if pos != nil && pos.TickLower < pos.TickUpper {
			exitTickLower, exitTickUpper = pos.TickLower, pos.TickUpper
		}
		stateViewAddr := common.HexToAddress(cc.UniswapV4StateViewAddress)
		poolManagerAddr := common.HexToAddress(cc.UniswapV4PoolManagerAddress)
		if sqrtPriceX96, _, slot0Err := blockchain.GetUniswapV4PoolSlot0ViaStateView(stateViewAddr, poolManagerAddr, task.PoolId); slot0Err != nil {
			log.Printf("[Liquidity] V4 exit: read slot0 for min-amount failed, using amount*Min=0: %v", slot0Err)
		} else {
			amount0Min, amount1Min = exitMinAmounts(sqrtPriceX96, exitTickLower, exitTickUpper, removeLiq, effectiveExitSlippagePercent(task))
		}
	}

	// Build `unlockData` = abi.encode(actions, params) to call PositionManager.modifyLiquidities.
	// actions = [DECREASE_LIQUIDITY (0x01), TAKE_PAIR (0x11)].
	actions := []byte{0x01, 0x11}

	uint256Ty, _ := abi.NewType("uint256", "", nil)
	uint128Ty, _ := abi.NewType("uint128", "", nil)
	addressTy, _ := abi.NewType("address", "", nil)
	bytesTy, _ := abi.NewType("bytes", "", nil)
	bytesArrTy, _ := abi.NewType("bytes[]", "", nil)

	decreaseArgs := abi.Arguments{
		{Type: uint256Ty}, // tokenId
		{Type: uint256Ty}, // liquidity
		{Type: uint128Ty}, // amount0Min
		{Type: uint128Ty}, // amount1Min
		{Type: bytesTy},   // hookData
	}
	decreaseParams, err := decreaseArgs.Pack(tokenId, removeLiq, amount0Min, amount1Min, []byte{})
	if err != nil {
		return nil, fmt.Errorf("encode v4 decrease params failed: %w", err)
	}

	takePairArgs := abi.Arguments{
		{Type: addressTy}, // currency0
		{Type: addressTy}, // currency1
		{Type: addressTy}, // recipient
	}
	takePairParams, err := takePairArgs.Pack(c0, c1, walletAddr)
	if err != nil {
		return nil, fmt.Errorf("encode v4 takePair params failed: %w", err)
	}

	unlockArgs := abi.Arguments{
		{Type: bytesTy},    // actions
		{Type: bytesArrTy}, // params[]
	}
	unlockData, err := unlockArgs.Pack(actions, [][]byte{decreaseParams, takePairParams})
	if err != nil {
		return nil, fmt.Errorf("encode v4 unlockData failed: %w", err)
	}

	log.Printf("[Liquidity] Removing V4 liquidity via PositionManager=%s tokenId=%s poolId=%s liq=%s currentLiquidity=%s exitPercent=%.6f amount0Min=%s amount1Min=%s",
		v4pmAddr.Hex(), tokenId.String(), task.PoolId, removeLiq.String(), liq.String(), exitPercent, amount0Min.String(), amount1Min.String())
	nonce, err := blockchain.GetNonce(walletAddr)
	if err != nil {
		return nil, err
	}
	auth, err := s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
	if err != nil {
		return nil, err
	}
	deadline := big.NewInt(time.Now().Add(20 * time.Minute).Unix())
	tx, err := v4pm.ModifyLiquidities(auth, unlockData, deadline)
	if err != nil {
		return nil, fmt.Errorf("v4 modifyLiquidities(remove) failed: %w", err)
	}

	removeTxHash := tx.Hash().Hex()
	txHashes = append(txHashes, fmt.Sprintf("V4撤仓|%s", removeTxHash))

	if _, err := s.waitMined(client, chainID, tx); err != nil {
		return txHashes, fmt.Errorf("v4 remove tx failed: %w", err)
	}

	remainingLiq := new(big.Int).Sub(cloneBig(liq), cloneBig(removeLiq))
	if remainingLiq.Sign() < 0 {
		remainingLiq = big.NewInt(0)
	}
	if posAfter, readErr := blockchain.GetV4PositionInfo(v4pmAddr, common.HexToAddress(cc.UniswapV4PoolManagerAddress), task.PoolId, tokenId); readErr == nil && posAfter != nil && posAfter.Liquidity != nil {
		remainingLiq = cloneBig(posAfter.Liquidity)
	} else if readErr != nil {
		log.Printf("[Liquidity] Warning: read V4 position after exit failed, using calculated remaining liquidity task_id=%d tokenId=%s remaining=%s: %v",
			task.ID, tokenId.String(), remainingLiq.String(), readErr)
	}
	persistTaskCurrentLiquidity(task, remainingLiq)

	b0After, _ := blockchain.GetTokenBalanceWithClient(client, c0, walletAddr)
	b1After, _ := blockchain.GetTokenBalanceWithClient(client, c1, walletAddr)
	d0 := new(big.Int).Sub(b0After, b0Before)
	d1 := new(big.Int).Sub(b1After, b1Before)
	if d0.Sign() < 0 {
		d0 = big.NewInt(0)
	}
	if d1.Sign() < 0 {
		d1 = big.NewInt(0)
	}

	log.Printf("[Liquidity] V4 exit: recovered %s Token0, %s Token1", d0.String(), d1.String())

	// Prefer cached symbols from task; fall back to on-chain ERC20 symbol; finally fall back to address.
	sym0 := strings.TrimSpace(task.Token0Symbol)
	if sym0 == "" {
		if s0, sErr := blockchain.GetTokenSymbolWithClient(client, c0); sErr == nil && strings.TrimSpace(s0) != "" {
			sym0 = strings.TrimSpace(s0)
		} else {
			sym0 = c0.Hex()
		}
	}
	sym1 := strings.TrimSpace(task.Token1Symbol)
	if sym1 == "" {
		if s1, sErr := blockchain.GetTokenSymbolWithClient(client, c1); sErr == nil && strings.TrimSpace(s1) != "" {
			sym1 = strings.TrimSpace(s1)
		} else {
			sym1 = c1.Hex()
		}
	}

	if swapDeltas {
		var swapErrs []error
		if d0.Sign() > 0 && c0 != usdtAddr {
			if skip, _ := s.shouldSkipExitSwapToUSDT(exec, c0, d0, walletAddr, sym0); skip {
				goto swap1
			}
			if hash, err := s.swapDeltaToUSDTWithHash(exec, privateKey, walletAddr, c0, usdtAddr, d0, effectiveExitSlippagePercent(task)); err != nil {
				swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT failed: %w", sym0, err))
			} else if hash != "" {
				txHashes = append(txHashes, fmt.Sprintf("交换 %s→USDT|%s", sym0, hash))
			} else {
				swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT returned empty tx hash", sym0))
			}
		}
	swap1:
		if d1.Sign() > 0 && c1 != usdtAddr {
			if skip, _ := s.shouldSkipExitSwapToUSDT(exec, c1, d1, walletAddr, sym1); skip {
				goto afterSwap
			}
			if hash, err := s.swapDeltaToUSDTWithHash(exec, privateKey, walletAddr, c1, usdtAddr, d1, effectiveExitSlippagePercent(task)); err != nil {
				swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT failed: %w", sym1, err))
			} else if hash != "" {
				txHashes = append(txHashes, fmt.Sprintf("交换 %s→USDT|%s", sym1, hash))
			} else {
				swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT returned empty tx hash", sym1))
			}
		}
	afterSwap:
		if len(swapErrs) > 0 {
			swapErr = errors.Join(swapErrs...)
		}
	}

	if !swapDeltas {
		return txHashes, nil
	}

	// Record Transaction
	usdtAfter, _ := blockchain.GetTokenBalanceWithClient(client, usdtAddr, walletAddr)
	if usdtAfter == nil {
		usdtAfter = big.NewInt(0)
	}
	totalUSDTReceived := new(big.Int).Sub(usdtAfter, usdtBefore)
	if totalUSDTReceived.Sign() < 0 {
		totalUSDTReceived = big.NewInt(0)
	}

	mainHash := removeTxHash

	if !partialExit {
		txRecord := models.Transaction{
			UserID:          task.UserID,
			Chain:           task.Chain,
			TaskID:          task.ID,
			TxHash:          mainHash,
			Type:            models.TxTypeRemoveLiquidity,
			Status:          models.TxStatusConfirmed,
			FromAddress:     walletAddr.Hex(),
			ToAddress:       v4pmAddr.Hex(),
			TokenInAddress:  tFromBytes32(task.PoolId).Hex(),
			TokenOutAddress: usdtAddr.Hex(),
			AmountIn:        "0",
			AmountOut:       totalUSDTReceived.String(),
			CreatedAt:       time.Now(),
		}
		s.persistTransactionRecordAsync("exit_v4_tx_record", txRecord)
	}

	if swapErr != nil {
		return txHashes, &SwapToUSDTError{Err: swapErr}
	}
	return txHashes, nil
}

// tFromBytes32 helper for poolId to address like (not real address but for record)
func tFromBytes32(h string) common.Address {
	return common.HexToAddress(h)
}

func (s *LiquidityService) swapDeltaToUSDT(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	usdtAddr common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) error {
	if exec == nil {
		return fmt.Errorf("executor is nil")
	}
	client := exec.Client()
	if client == nil {
		return fmt.Errorf("blockchain client not initialized")
	}

	if amountIn == nil || amountIn.Sign() <= 0 {
		return nil
	}
	if tokenIn == usdtAddr {
		return nil
	}

	// 检查实际余额
	actualBalance, err := getOKXSwapAssetBalance(client, tokenIn, walletAddr)
	if err != nil {
		log.Printf("[Liquidity] Warning: failed to get token balance: %v", err)
	} else {
		if actualBalance == nil {
			actualBalance = big.NewInt(0)
		}
		log.Printf("[Liquidity] Token %s balance: %s, attempting to swap: %s", tokenIn.Hex(), actualBalance.String(), amountIn.String())
		// 如果实际余额小于要 swap 的数量，先等待 RPC 同步再决定是否截断。
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

	_, err = s.swapExactInViaOKX(exec, privateKey, walletAddr, tokenIn, usdtAddr, amountIn, slippagePercent)
	return err
}

// swapDeltaToUSDTWithHash 与 swapDeltaToUSDT 类似，但返回交易哈希
func (s *LiquidityService) swapDeltaToUSDTWithHash(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	usdtAddr common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (string, error) {
	if exec == nil {
		return "", fmt.Errorf("executor is nil")
	}
	client := exec.Client()
	if client == nil {
		return "", fmt.Errorf("blockchain client not initialized")
	}

	if amountIn == nil || amountIn.Sign() <= 0 {
		return "", nil
	}
	if tokenIn == usdtAddr {
		return "", nil
	}

	// 检查实际余额
	actualBalance, err := getOKXSwapAssetBalance(client, tokenIn, walletAddr)
	if err != nil {
		log.Printf("[Liquidity] Warning: failed to get token balance: %v", err)
	} else {
		if actualBalance == nil {
			actualBalance = big.NewInt(0)
		}
		log.Printf("[Liquidity] Token %s balance: %s, attempting to swap: %s", tokenIn.Hex(), actualBalance.String(), amountIn.String())
		// 如果实际余额小于要 swap 的数量，先等待 RPC 同步再决定是否截断。
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

	txHash, err := s.swapExactInViaOKXWithHash(exec, privateKey, walletAddr, tokenIn, usdtAddr, amountIn, slippagePercent)
	return txHash, err
}

// swapExactInViaOKXWithHash 与 swapExactInViaOKX 类似，但返回交易哈希
func (s *LiquidityService) swapExactInViaOKXWithHash(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (string, error) {
	if exec != nil {
		r, err := s.executeOKXSwapExactIn(exec, privateKey, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent)
		if r == nil {
			return "", err
		}
		return r.TxHash, err
	}
	if exec == nil {
		return "", fmt.Errorf("executor is nil")
	}
	cc := exec.Config()
	client := exec.Client()
	chainID := exec.ChainID()
	if client == nil || chainID == nil {
		return "", fmt.Errorf("blockchain client not initialized")
	}
	if amountIn == nil || amountIn.Sign() <= 0 {
		return "", nil
	}
	if tokenIn == tokenOut {
		return "", nil
	}

	if s.okxService == nil {
		s.okxService = exchange.NewOKXDexService()
	}

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
		return "", err
	}
	if swapResp == nil || len(swapResp.Data) == 0 {
		return "", fmt.Errorf("OKX swap response empty")
	}

	expectedOut := strings.TrimSpace(swapResp.Data[0].RouterResult.ToTokenAmount)
	if expectedOut == "" {
		expectedOut = "unknown"
	}
	estGas := strings.TrimSpace(swapResp.Data[0].Tx.Gas)
	if estGas != "" {
		log.Printf("[Liquidity] OKX swap preview: %s -> %s amountIn=%s expectedOut=%s txGas=%s slippage=%.4f%%",
			tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), expectedOut, estGas, slippagePercent)
	} else {
		log.Printf("[Liquidity] OKX swap preview: %s -> %s amountIn=%s expectedOut=%s slippage=%.4f%%",
			tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), expectedOut, slippagePercent)
	}

	txObj := swapResp.Data[0].Tx
	if !common.IsHexAddress(txObj.To) {
		return "", fmt.Errorf("OKX tx.to invalid: %s", txObj.To)
	}
	to := common.HexToAddress(txObj.To)
	data := common.FromHex(txObj.Data)
	if len(data) == 0 {
		return "", fmt.Errorf("OKX tx.data empty; OKX did not return executable calldata, the selected route may not support referrer fee")
	}

	value := new(big.Int)
	if strings.TrimSpace(txObj.Value) != "" {
		if v, ok := new(big.Int).SetString(strings.TrimSpace(txObj.Value), 10); ok {
			value = v
		} else if v, ok := new(big.Int).SetString(strings.TrimPrefix(strings.TrimSpace(txObj.Value), "0x"), 16); ok {
			value = v
		}
	}
	if value.Sign() != 0 {
		return "", fmt.Errorf("OKX swap requires native value; not supported")
	}

	var okxGasLimit uint64
	if strings.TrimSpace(txObj.Gas) != "" {
		if g, ok := new(big.Int).SetString(strings.TrimSpace(txObj.Gas), 10); ok && g.IsUint64() {
			okxGasLimit = g.Uint64()
		}
	}

	swapTx := blockchain.OkxSwapTx{To: to, Value: value, Data: data}
	_ = ValidateOkxSmartSwapTx("swap", swapTx)
	if err := EnforceOkxSwapRouter("swap", cc.OKXSwapRouter, swapTx); err != nil {
		return "", err
	}

	// 获取 OKX TokenApprove 合约地址
	chainIDText := fmt.Sprintf("%d", cc.ChainID)
	approveSpender, err := s.okxService.GetApproveSpender(chainIDText, tokenIn.Hex())
	if err != nil {
		log.Printf("[Liquidity] Warning: failed to get OKX approve spender, using router as fallback: %v", err)
		approveSpender = to.Hex()
	}
	approveAddr := common.HexToAddress(approveSpender)

	log.Printf("[Liquidity] OKX swap: %s -> %s amount=%s router=%s approveTarget=%s",
		tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), to.Hex(), approveAddr.Hex())

	// Approve spender (ERC20 or Permit2).
	if approveAddr == blockchain.Permit2Address {
		if err := s.approveTokenViaPermit2(client, chainID, privateKey, walletAddr, tokenIn, to, amountIn, TxOptions{}); err != nil {
			return "", fmt.Errorf("approve via Permit2 failed: %w", err)
		}
	} else {
		if err := s.approveToken(client, chainID, privateKey, walletAddr, tokenIn, approveAddr, amountIn, TxOptions{}); err != nil {
			return "", fmt.Errorf("approve spender failed: %w", err)
		}
	}

	gasPrice, err := blockchain.GetGasPriceWithClient(client)
	if err != nil {
		return "", err
	}

	gasLimit, err := okxSwapGasLimit(client, walletAddr, to, value, data, okxGasLimit)
	if err != nil {
		return "", err
	}
	log.Printf("[Liquidity] OKX swap gasLimit: okx=%d final=%d", okxGasLimit, gasLimit)

	signed, err := blockchain.SendRawTransactionWithRetry(blockchain.SendRawTxParams{
		Client:     client,
		ChainID:    chainID,
		PrivateKey: privateKey,
		From:       walletAddr,
		To:         to,
		Value:      value,
		Data:       data,
		GasLimit:   gasLimit,
		GasPrice:   gasPrice,
	})
	if err != nil {
		return "", err
	}

	txHash := signed.Hash().Hex()
	if _, err := s.waitMined(client, chainID, signed); err != nil {
		return txHash, err
	}

	return txHash, nil
}
