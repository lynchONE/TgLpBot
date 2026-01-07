package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/exchange"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"TgLpBot/service/trade"
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log"
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
)

func firstTxHash(txHashes []string) (common.Hash, bool) {
	if len(txHashes) == 0 {
		return common.Hash{}, false
	}
	txInfo := strings.TrimSpace(txHashes[0])
	if txInfo == "" {
		return common.Hash{}, false
	}
	parts := strings.Split(txInfo, "|")
	if len(parts) >= 2 {
		txInfo = strings.TrimSpace(parts[1])
	}
	if !reTxHash.MatchString(txInfo) {
		return common.Hash{}, false
	}
	return common.HexToHash(txInfo), true
}

func extractTxHashes(txHashes []string) []common.Hash {
	if len(txHashes) == 0 {
		return nil
	}
	seen := make(map[common.Hash]struct{}, len(txHashes))
	out := make([]common.Hash, 0, len(txHashes))
	for _, item := range txHashes {
		s := strings.TrimSpace(item)
		if s == "" {
			continue
		}
		parts := strings.Split(s, "|")
		if len(parts) >= 2 {
			s = strings.TrimSpace(parts[len(parts)-1])
		}
		if !reTxHash.MatchString(s) {
			continue
		}
		h := common.HexToHash(s)
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

func (s *LiquidityService) gasCostWeiFromReceipt(txHash common.Hash, receipt *types.Receipt) *big.Int {
	if receipt == nil {
		return big.NewInt(0)
	}
	if cost := receiptGasCostWei(receipt); cost.Sign() > 0 {
		return cost
	}
	// Fallback for nodes that don't provide EffectiveGasPrice in receipts.
	if blockchain.Client == nil || receipt.GasUsed == 0 {
		return big.NewInt(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	tx, _, err := blockchain.Client.TransactionByHash(ctx, txHash)
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
	header, err := blockchain.Client.HeaderByNumber(ctx2, receipt.BlockNumber)
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

func (s *LiquidityService) getReceiptWithRetry(txHash common.Hash) (*types.Receipt, error) {
	if blockchain.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	timeout, poll := s.exitTokenSyncDurations()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		receipt, err := blockchain.Client.TransactionReceipt(ctx, txHash)
		cancel()
		if err == nil && receipt != nil {
			return receipt, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			if lastErr != nil {
				return nil, fmt.Errorf("fetch tx receipt timeout %s: %w", txHash.Hex(), lastErr)
			}
			return nil, fmt.Errorf("fetch tx receipt timeout %s", txHash.Hex())
		}
		time.Sleep(poll)
	}
}

func (s *LiquidityService) waitTokenBalanceAtLeast(token common.Address, wallet common.Address, min *big.Int, label string) (*big.Int, error) {
	bal, err := blockchain.GetTokenBalance(token, wallet)
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

	for time.Now().Before(deadline) {
		time.Sleep(poll)
		bal, err = blockchain.GetTokenBalance(token, wallet)
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

func (s *LiquidityService) ExitTaskToUSDTWithOptions(userID uint, task *models.StrategyTask, sweepWallet bool, opts TxOptions) ([]string, error) {
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
	usdtAddr := common.HexToAddress(config.AppConfig.USDTAddress)

	// Capture balance before exiting (used for "actual received" and gas cost).
	usdtBefore, _ := blockchain.GetTokenBalance(usdtAddr, walletAddr)
	if usdtBefore == nil {
		usdtBefore = big.NewInt(0)
	}
	bnbBefore, _ := blockchain.GetBalance(walletAddr)
	if bnbBefore == nil {
		bnbBefore = big.NewInt(0)
	}

	// SweepWallet mode needs to be resilient to public RPC lag/load-balancing:
	// after liquidity removal is mined, balanceOf() may still return an old state for a short time.
	// We capture pre-exit balances and later enforce "min expected" balances (pre + receipt delta)
	// before swapping, so we don't miss tokens due to stale RPC reads.
	var sweepToken0 common.Address
	var sweepToken1 common.Address
	var sweepPreBalances map[common.Address]*big.Int
	if sweepWallet {
		t0, t1, tokErr := s.resolveTaskTokenAddresses(task)
		if tokErr != nil {
			log.Printf("[Liquidity] Warning: resolveTaskTokenAddresses (pre-exit) failed, skip expected-balance check: %v", tokErr)
		} else {
			sweepToken0, sweepToken1 = t0, t1
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
				bal, err := blockchain.GetTokenBalance(tok, walletAddr)
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
			txHashes, exitErr = s.exitV4ToUSDT(privateKey, walletAddr, usdtAddr, task, swapDeltas, opts)
		default:
			txHashes, exitErr = s.exitV3ToUSDT(privateKey, walletAddr, usdtAddr, task, swapDeltas, opts)
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
				receipt, rerr := s.getReceiptWithRetry(exitHash)
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

					// V3 fallback: use ZapOutV3 event deltas when Transfer logs are not available.
					if strings.ToLower(strings.TrimSpace(task.PoolVersion)) != "v4" {
						if common.IsHexAddress(config.AppConfig.ZapV3Address) {
							tokenId, _ := convert.ParseBigIntFlexible(task.V3TokenID)
							if tokenId != nil && tokenId.Sign() > 0 {
								zapAddr := common.HexToAddress(config.AppConfig.ZapV3Address)
								if a0, a1, ok := parseZapOutV3Result(receipt, zapAddr, walletAddr, tokenId); ok {
									if sweepToken0 != (common.Address{}) && sweepToken0 != usdtAddr {
										if _, exists := expectedMinBalances[sweepToken0]; !exists && a0.Sign() > 0 {
											before := sweepPreBalances[sweepToken0]
											expectedMinBalances[sweepToken0] = new(big.Int).Add(cloneBig(before), cloneBig(a0))
										}
									}
									if sweepToken1 != (common.Address{}) && sweepToken1 != usdtAddr {
										if _, exists := expectedMinBalances[sweepToken1]; !exists && a1.Sign() > 0 {
											before := sweepPreBalances[sweepToken1]
											expectedMinBalances[sweepToken1] = new(big.Int).Add(cloneBig(before), cloneBig(a1))
										}
									}
								}
							}
						}
					}

					if len(expectedMinBalances) == 0 {
						expectedMinBalances = nil
					}
				}
			}
		}

		sweepHashes, err := s.swapWalletTokensToUSDT(privateKey, walletAddr, usdtAddr, task, expectedMinBalances, sweepPreBalances)
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
	usdtAfter, _ := blockchain.GetTokenBalance(usdtAddr, walletAddr)
	if usdtAfter == nil {
		usdtAfter = big.NewInt(0)
	}
	bnbAfter, _ := blockchain.GetBalance(walletAddr)
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
		receipt, rerr := s.getReceiptWithRetry(h)
		if rerr != nil || receipt == nil {
			continue
		}
		receiptUSDT.Add(receiptUSDT, ReceiptTokenTransferDelta(receipt, usdtAddr, walletAddr))
		receiptGas.Add(receiptGas, s.gasCostWeiFromReceipt(h, receipt))
	}
	if receiptUSDT.Cmp(actualReceived) > 0 {
		actualReceived = receiptUSDT
	}
	if receiptGas.Cmp(gasSpent) > 0 {
		gasSpent = receiptGas
	}

	mainHash := ""
	if len(txHashes) > 0 {
		parts := strings.Split(txHashes[0], "|")
		if len(parts) >= 2 {
			mainHash = strings.TrimSpace(parts[1])
		} else {
			mainHash = strings.TrimSpace(txHashes[0])
		}
	}

	finalizeTradeRecord := exitErr == nil && sweepErr == nil
	shouldUpdateRecords := finalizeTradeRecord || actualReceived.Sign() > 0 || gasSpent.Sign() > 0 || mainHash != ""

	var tradeRec *models.TradeRecord
	if shouldUpdateRecords {
		trSvc := trade.NewTradeRecordService()
		bnbPriceUSDT := 0.0
		if finalizeTradeRecord {
			// Only needed when finalizing the record (Profit/TotalGasUSDT computation).
			bnbPriceUSDT = pricing.GetBNBPriceUSDT()
		}
		if rec, err := trSvc.ApplyExitDelta(task, mainHash, actualReceived, gasSpent, finalizeTradeRecord, bnbPriceUSDT); err == nil {
			tradeRec = rec
		} else if finalizeTradeRecord {
			// Legacy fallback: create a closed record so we don't lose the exit summary.
			_ = trSvc.CloseLatestOpenRecord(task, mainHash, actualReceived, gasSpent, bnbPriceUSDT)
		}
	}

	// Update/Create Transaction record (best-effort). Use cumulative amount when available.
	txHash := strings.TrimSpace(mainHash)
	amountOut := actualReceived.String()
	if tradeRec != nil {
		if strings.TrimSpace(tradeRec.CloseTxHash) != "" {
			txHash = strings.TrimSpace(tradeRec.CloseTxHash)
		}
		if strings.TrimSpace(tradeRec.CloseUSDTReceived) != "" {
			amountOut = strings.TrimSpace(tradeRec.CloseUSDTReceived)
		}
	}
	if txHash != "" && shouldUpdateRecords {
		var existing models.Transaction
		if err := database.DB.Where("tx_hash = ?", txHash).First(&existing).Error; err == nil {
			if err := database.DB.Model(&existing).Updates(map[string]interface{}{"amount_out": amountOut}).Error; err != nil {
				log.Printf("[Liquidity] Warning: update exit transaction amount_out failed: %v", err)
			}
		} else {
			usdtAddr := common.HexToAddress(config.AppConfig.USDTAddress)
			txRecord := models.Transaction{
				UserID:          task.UserID,
				TaskID:          task.ID,
				TxHash:          txHash,
				Type:            models.TxTypeRemoveLiquidity,
				Status:          models.TxStatusConfirmed,
				FromAddress:     walletAddr.Hex(),
				TokenOutAddress: usdtAddr.Hex(),
				AmountIn:        "0",
				AmountOut:       amountOut,
				CreatedAt:       time.Now(),
			}
			if err := database.DB.Create(&txRecord).Error; err != nil {
				log.Printf("[Liquidity] Warning: create exit transaction record failed: %v", err)
			}
		}
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

func parseZapOutV3Result(receipt *types.Receipt, zapAddr common.Address, user common.Address, tokenId *big.Int) (*big.Int, *big.Int, bool) {
	if receipt == nil {
		return nil, nil, false
	}
	parsed, err := abi.JSON(strings.NewReader(blockchain.ZapSimpleABI))
	if err != nil {
		return nil, nil, false
	}
	ev, ok := parsed.Events["ZapOutV3"]
	if !ok {
		return nil, nil, false
	}

	for _, lg := range receipt.Logs {
		if lg == nil || lg.Address != zapAddr || len(lg.Topics) == 0 || lg.Topics[0] != ev.ID {
			continue
		}
		if len(lg.Topics) < 3 {
			continue
		}
		if user != (common.Address{}) {
			// topic[1] is indexed user (address) padded to 32 bytes.
			if common.BytesToAddress(lg.Topics[1].Bytes()) != user {
				continue
			}
		}
		if tokenId != nil && tokenId.Sign() > 0 {
			if new(big.Int).SetBytes(lg.Topics[2].Bytes()).Cmp(tokenId) != 0 {
				continue
			}
		}
		out, err := parsed.Unpack("ZapOutV3", lg.Data)
		if err != nil || len(out) < 2 {
			continue
		}
		amount0, _ := out[0].(*big.Int)
		amount1, _ := out[1].(*big.Int)
		if amount0 == nil {
			amount0 = big.NewInt(0)
		}
		if amount1 == nil {
			amount1 = big.NewInt(0)
		}
		return amount0, amount1, true
	}

	return nil, nil, false
}

type otherTaskDustRow struct {
	Token0Address string `gorm:"column:token0_address"`
	Token1Address string `gorm:"column:token1_address"`
	OpenDust0     string `gorm:"column:open_dust0"`
	OpenDust1     string `gorm:"column:open_dust1"`
}

// reservedDustForOtherOpenTasks returns the sum of recorded dust amounts (OpenDust0/OpenDust1)
// for other open tasks (same user, TradeStatusOpen) keyed by token address.
// It is best-effort: query failures are returned to callers so they can decide whether to ignore.
func (s *LiquidityService) reservedDustForOtherOpenTasks(userID uint, excludeTaskID uint, tokens []common.Address) (map[common.Address]*big.Int, error) {
	reserved := make(map[common.Address]*big.Int)
	if userID == 0 || len(tokens) == 0 {
		return reserved, nil
	}

	tokenSet := make(map[common.Address]struct{}, len(tokens))
	for _, t := range tokens {
		if t == (common.Address{}) {
			continue
		}
		tokenSet[t] = struct{}{}
	}
	if len(tokenSet) == 0 {
		return reserved, nil
	}

	var rows []otherTaskDustRow
	err := database.DB.
		Table("trade_records tr").
		Select("st.token0_address AS token0_address, st.token1_address AS token1_address, tr.open_dust0 AS open_dust0, tr.open_dust1 AS open_dust1").
		Joins("JOIN strategy_tasks st ON st.id = tr.task_id").
		Where("tr.user_id = ? AND tr.status = ? AND tr.task_id <> ?", userID, models.TradeStatusOpen, excludeTaskID).
		Scan(&rows).Error
	if err != nil {
		return reserved, err
	}

	add := func(addrStr string, dustStr string) {
		if !common.IsHexAddress(addrStr) {
			return
		}
		addr := common.HexToAddress(strings.TrimSpace(addrStr))
		if _, ok := tokenSet[addr]; !ok {
			return
		}
		dust, perr := convert.ParseBigInt(dustStr)
		if perr != nil || dust == nil || dust.Sign() <= 0 {
			return
		}
		if cur := reserved[addr]; cur != nil {
			cur.Add(cur, dust)
			return
		}
		reserved[addr] = cloneBig(dust)
	}

	for _, r := range rows {
		add(r.Token0Address, r.OpenDust0)
		add(r.Token1Address, r.OpenDust1)
	}

	return reserved, nil
}

func (s *LiquidityService) swapWalletTokensToUSDT(
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
	reservedOtherDust, rerr := s.reservedDustForOtherOpenTasks(task.UserID, task.ID, targetAddrs)
	if rerr != nil {
		log.Printf("[Liquidity] Warning: 读取其他任务残余余额失败，可能导致收益归属不准确: %v", rerr)
		reservedOtherDust = nil
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
			bal, werr = s.waitTokenBalanceAtLeast(tok.addr, walletAddr, minExpected, label)
			if werr != nil {
				errs = append(errs, werr)
				continue
			}
		} else {
			var rerr error
			bal, rerr = blockchain.GetTokenBalance(tok.addr, walletAddr)
			if rerr != nil {
				errs = append(errs, fmt.Errorf("读取 %s 余额失败 (%s): %w", label, tok.addr.Hex(), rerr))
				continue
			}
		}

		if bal == nil || bal.Sign() <= 0 {
			continue
		}

		// Keep other open tasks' recorded dust for this token in the wallet, so one task's sweep
		// won't accidentally "consume" other tasks' residuals and mis-attribute the PnL.
		keep := big.NewInt(0)
		if reservedOtherDust != nil {
			if v := reservedOtherDust[tok.addr]; v != nil && v.Sign() > 0 {
				keep = cloneBig(v)
			}
		}
		if preBalances != nil {
			if pre := preBalances[tok.addr]; pre != nil && pre.Sign() > 0 && keep.Cmp(pre) > 0 {
				// Only reserve what existed before this exit; exit deltas always belong to this task.
				keep = cloneBig(pre)
			}
		}
		expectedRemaining[tok.addr] = cloneBig(keep)

		toSwap := new(big.Int).Sub(cloneBig(bal), keep)
		if toSwap.Sign() <= 0 {
			continue
		}

		swapTxHash, err := s.swapDeltaToUSDTWithHash(privateKey, walletAddr, tok.addr, usdtAddr, toSwap, task.SlippageTolerance)
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
		bal, err := blockchain.GetTokenBalance(tok.addr, walletAddr)
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
			log.Printf("[Liquidity] Warning: 清仓兑换后仍有 %s 余额未兑换 (%s): %s（预留 %s；可能是 swap 滑点或小额残余）", label, tok.addr.Hex(), bal.String(), keep.String())
		}
	}

	// 只返回实际的 swap 错误，不因校验残余而返回错误
	if len(errs) > 0 {
		return txHashes, errors.Join(errs...)
	}
	return txHashes, nil
}

func (s *LiquidityService) buildAuth(privateKey *ecdsa.PrivateKey, nonce uint64, value *big.Int, opts TxOptions) (*bind.TransactOpts, error) {
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, blockchain.ChainID)
	if err != nil {
		return nil, err
	}
	gasPrice, err := blockchain.GetGasPriceWithMultiplier(normalizeGasMultiplier(opts.GasMultiplier))
	if err != nil {
		return nil, err
	}
	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = value
	auth.GasLimit = 0 // 让节点自动估算 gas
	auth.GasPrice = gasPrice
	return auth, nil
}

func (s *LiquidityService) waitMined(tx *types.Transaction) (*types.Receipt, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	receipt, err := bind.WaitMined(ctx, blockchain.Client, tx)
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, fmt.Errorf("receipt is nil")
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		reason := tryGetRevertReason(tx, receipt)
		if reason != "" {
			return receipt, fmt.Errorf("tx reverted: %s (%s)", tx.Hash().Hex(), reason)
		}
		return receipt, fmt.Errorf("tx reverted: %s", tx.Hash().Hex())
	}
	return receipt, nil
}

func tryGetRevertReason(tx *types.Transaction, receipt *types.Receipt) string {
	if blockchain.Client == nil || tx == nil || receipt == nil {
		return ""
	}
	if tx.To() == nil {
		return ""
	}

	var from common.Address
	if blockchain.ChainID != nil {
		signer := types.LatestSignerForChainID(blockchain.ChainID)
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

	_, callErr := blockchain.Client.CallContract(callCtx, msg, receipt.BlockNumber)
	if callErr != nil {
		if reason := unpackRevertReasonFromError(callErr); reason != "" {
			return reason
		}
		// Some public RPCs prune history ("missing trie node"); latest often still returns the reason.
		_, callErr2 := blockchain.Client.CallContract(callCtx, msg, nil)
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

func (s *LiquidityService) exitV3ToUSDT(privateKey *ecdsa.PrivateKey, walletAddr common.Address, usdtAddr common.Address, task *models.StrategyTask, swapDeltas bool, opts TxOptions) ([]string, error) {
	var txHashes []string
	var swapErr error

	// Capture initial USDT balance for calculating output
	usdtBefore, _ := blockchain.GetTokenBalance(usdtAddr, walletAddr)
	if usdtBefore == nil {
		usdtBefore = big.NewInt(0)
	}

	pmAddrStr := strings.TrimSpace(config.AppConfig.UniswapV3PositionManagerAddress)
	if strings.Contains(strings.ToLower(task.Exchange), "pancake") {
		pmAddrStr = config.AppConfig.PancakeV3PositionManagerAddress
	} else if task.V3PositionManagerAddress != "" {
		pmAddrStr = task.V3PositionManagerAddress
	}

	if !common.IsHexAddress(pmAddrStr) {
		return nil, fmt.Errorf("V3 position manager address not configured")
	}
	pmAddr := common.HexToAddress(pmAddrStr)
	zapAddr := common.HexToAddress(config.AppConfig.ZapV3Address)

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
	v3pm, err := blockchain.NewV3PositionManager(pmAddr, blockchain.Client)
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
		liq = posInfo.Liquidity
	}
	owed0 := big.NewInt(0)
	owed1 := big.NewInt(0)
	if posInfo.TokensOwed0 != nil {
		owed0 = posInfo.TokensOwed0
	}
	if posInfo.TokensOwed1 != nil {
		owed1 = posInfo.TokensOwed1
	}
	// Idempotency: if the position is already emptied (no liquidity + no fees owed), skip ZapOut.
	if liq.Sign() <= 0 && owed0.Sign() <= 0 && owed1.Sign() <= 0 {
		log.Printf("[Liquidity] V3 exit: position already empty, skipping ZapOutV3 tokenId=%s", tokenId.String())
		return txHashes, nil
	}

	// Compute V3 exit min amounts using current pool price (slippage protection for DECREASE_LIQUIDITY).
	poolAddrStr := strings.TrimSpace(task.PoolId)
	if !common.IsHexAddress(poolAddrStr) {
		return nil, fmt.Errorf("V3 pool address invalid: %s", poolAddrStr)
	}
	poolAddr := common.HexToAddress(poolAddrStr)

	sqrtPriceX96, _, err := blockchain.GetV3PoolSlot0(poolAddr)
	if err != nil {
		return nil, fmt.Errorf("read V3 pool slot0 failed: %w", err)
	}
	sqrtA, err := pool.SqrtRatioAtTick(int32(posInfo.TickLower))
	if err != nil {
		return nil, fmt.Errorf("compute sqrtA failed: %w", err)
	}
	sqrtB, err := pool.SqrtRatioAtTick(int32(posInfo.TickUpper))
	if err != nil {
		return nil, fmt.Errorf("compute sqrtB failed: %w", err)
	}
	expected0, expected1 := pool.AmountsForLiquidity(sqrtPriceX96, sqrtA, sqrtB, posInfo.Liquidity)

	slippageBps := int64(task.SlippageTolerance * 100)
	if slippageBps < 0 {
		slippageBps = 0
	}
	if slippageBps > 10000 {
		slippageBps = 10000
	}
	factor := big.NewInt(10000 - slippageBps)
	denom := big.NewInt(10000)
	amount0Min := new(big.Int).Div(new(big.Int).Mul(expected0, factor), denom)
	amount1Min := new(big.Int).Div(new(big.Int).Mul(expected1, factor), denom)

	// 4. Approve NFT 给 Zap 合约（优化：使用 setApprovalForAll）
	// 检查是否已经 setApprovalForAll
	isApprovedForAll, err := s.isNFTApprovedForAll(pmAddr, walletAddr, zapAddr)
	if err != nil {
		log.Printf("[Liquidity] Warning: failed to check isApprovedForAll: %v", err)
	}

	if !isApprovedForAll {
		// 首次使用，设置 setApprovalForAll（一次性授权所有 NFT）
		log.Printf("[Liquidity] V3 exit: Setting ApprovalForAll for Zap contract (one-time setup)")
		if err := s.setNFTApprovalForAll(privateKey, walletAddr, pmAddr, zapAddr, true, opts); err != nil {
			// 如果 setApprovalForAll 失败，降级到单个 approve
			log.Printf("[Liquidity] Warning: setApprovalForAll failed, falling back to single approve: %v", err)
			if err := s.approveNFT(privateKey, walletAddr, pmAddr, zapAddr, tokenId, opts); err != nil {
				return nil, fmt.Errorf("approve NFT failed: %w", err)
			}
		}
	} else {
		log.Printf("[Liquidity] V3 exit: Already approved for all NFTs, skipping approve")
	}

	// 3. 记录余额（用于计算获得的代币）
	b0Before, _ := blockchain.GetTokenBalance(token0, walletAddr)
	b1Before, _ := blockchain.GetTokenBalance(token1, walletAddr)
	if b0Before == nil {
		b0Before = big.NewInt(0)
	}
	if b1Before == nil {
		b1Before = big.NewInt(0)
	}

	// 4. 调用 ZapOutV3
	nonce, err := blockchain.GetNonce(walletAddr)
	if err != nil {
		return nil, err
	}
	auth, err := s.buildAuth(privateKey, nonce, big.NewInt(0), opts)
	if err != nil {
		return nil, err
	}

	zap, err := blockchain.NewZapSimple(zapAddr, blockchain.Client)
	if err != nil {
		return nil, fmt.Errorf("init zap contract failed: %w", err)
	}

	tuneZapTxGasLimit("V3 exit zapOutV3", auth, func(o *bind.TransactOpts) (*types.Transaction, error) {
		return zap.ZapOutV3(o, pmAddr, tokenId, walletAddr, amount0Min, amount1Min)
	})

	log.Printf("[Liquidity] V3 exit: Calling ZapOutV3 tokenId=%s amount0Min=%s amount1Min=%s", tokenId.String(), amount0Min.String(), amount1Min.String())
	tx, err := zap.ZapOutV3(auth, pmAddr, tokenId, walletAddr, amount0Min, amount1Min)
	if err != nil {
		return nil, fmt.Errorf("ZapOutV3 call failed: %w", err)
	}
	log.Printf("[Liquidity] V3 exit: tx sent %s", tx.Hash().Hex())
	txHashes = append(txHashes, "撤出流动性|"+tx.Hash().Hex())

	receipt, err := s.waitMined(tx)
	if err != nil {
		return txHashes, fmt.Errorf("ZapOutV3 tx failed: %w", err)
	}

	// 5. 计算获得的代币并 Swap 回 USDT
	// Prefer the ZapOutV3 event amounts (exact deltas), fall back to wallet balance deltas.
	d0 := big.NewInt(0)
	d1 := big.NewInt(0)
	if a0, a1, ok := parseZapOutV3Result(receipt, zapAddr, walletAddr, tokenId); ok {
		d0 = cloneBig(a0)
		d1 = cloneBig(a1)
	} else {
		b0After, _ := blockchain.GetTokenBalance(token0, walletAddr)
		b1After, _ := blockchain.GetTokenBalance(token1, walletAddr)
		if b0After == nil {
			b0After = big.NewInt(0)
		}
		if b1After == nil {
			b1After = big.NewInt(0)
		}
		d0 = new(big.Int).Sub(b0After, b0Before)
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
			if swapTxHash, err := s.swapDeltaToUSDTWithHash(privateKey, walletAddr, token0, usdtAddr, d0, task.SlippageTolerance); err != nil {
				symbol := "Token0"
				if task.Token0Symbol != "" {
					symbol = task.Token0Symbol
				}
				swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT failed: %w", symbol, err))
			} else if swapTxHash != "" {
				symbol := "Token0"
				if task.Token0Symbol != "" {
					symbol = task.Token0Symbol
				}
				txHashes = append(txHashes, fmt.Sprintf("兑换 %s→USDT|%s", symbol, swapTxHash))
			} else {
				symbol := "Token0"
				if task.Token0Symbol != "" {
					symbol = task.Token0Symbol
				}
				swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT returned empty tx hash", symbol))
			}
		}
		if d1.Sign() > 0 && token1 != usdtAddr {
			log.Printf("[Liquidity] V3 exit: Got %s token1, swapping to USDT...", d1.String())
			if swapTxHash, err := s.swapDeltaToUSDTWithHash(privateKey, walletAddr, token1, usdtAddr, d1, task.SlippageTolerance); err != nil {
				symbol := "Token1"
				if task.Token1Symbol != "" {
					symbol = task.Token1Symbol
				}
				swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT failed: %w", symbol, err))
			} else if swapTxHash != "" {
				symbol := "Token1"
				if task.Token1Symbol != "" {
					symbol = task.Token1Symbol
				}
				txHashes = append(txHashes, fmt.Sprintf("兑换 %s→USDT|%s", symbol, swapTxHash))
			} else {
				symbol := "Token1"
				if task.Token1Symbol != "" {
					symbol = task.Token1Symbol
				}
				swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT returned empty tx hash", symbol))
			}
		}
		if len(swapErrs) > 0 {
			swapErr = errors.Join(swapErrs...)
		}
	}

	// 6. Record Transaction (Calc total USDT received)
	usdtAfter, _ := blockchain.GetTokenBalance(usdtAddr, walletAddr)
	if usdtAfter == nil {
		usdtAfter = big.NewInt(0)
	}

	totalUSDTReceived := new(big.Int).Sub(usdtAfter, usdtBefore) // Calculate delta
	if totalUSDTReceived.Sign() < 0 {
		totalUSDTReceived = big.NewInt(0)
	}

	// Use the first hash (ZapOut) as the main hash, or a new one?
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
	if swapDeltas && mainHash != "" {
		txRecord := models.Transaction{
			UserID:          task.UserID,
			TaskID:          task.ID,
			TxHash:          mainHash, // Use ZapOut hash
			Type:            models.TxTypeRemoveLiquidity,
			Status:          models.TxStatusConfirmed,
			FromAddress:     walletAddr.Hex(),
			ToAddress:       zapAddr.Hex(),
			TokenInAddress:  pmAddr.Hex(), // Representing the pool/position
			TokenOutAddress: usdtAddr.Hex(),
			AmountIn:        "0",
			AmountOut:       totalUSDTReceived.String(),
			CreatedAt:       time.Now(),
		}
		if err := database.DB.Create(&txRecord).Error; err != nil {
			log.Printf("[Liquidity] Warning: failed to record exit transaction: %v", err)
		}
	}

	if swapErr != nil {
		return txHashes, &SwapToUSDTError{Err: swapErr}
	}
	return txHashes, nil
}

// approveNFT checks approval and approves if needed (ERC721)
func (s *LiquidityService) approveNFT(privateKey *ecdsa.PrivateKey, walletAddr, tokenAddr, spender common.Address, tokenId *big.Int, opts TxOptions) error {
	// 简单的 ERC721 ABI 用于 getApproved 和 approve
	const erc721ABI = `[{"constant":true,"inputs":[{"name":"tokenId","type":"uint256"}],"name":"getApproved","outputs":[{"name":"","type":"address"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":false,"inputs":[{"name":"to","type":"address"},{"name":"tokenId","type":"uint256"}],"name":"approve","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"}]`

	parsed, err := abi.JSON(strings.NewReader(erc721ABI))
	if err != nil {
		return err
	}

	// Check approved
	var out []interface{}
	// Pack call
	data, err := parsed.Pack("getApproved", tokenId)
	if err != nil {
		return err
	}
	msg := ethereum.CallMsg{To: &tokenAddr, Data: data}
	res, err := blockchain.Client.CallContract(context.Background(), msg, nil)
	if err == nil {
		if out, err = parsed.Unpack("getApproved", res); err == nil && len(out) > 0 {
			if approvedAddr, ok := out[0].(common.Address); ok && approvedAddr == spender {
				return nil // Already approved
			}
		}
	}

	// Approve
	nonce, err := blockchain.GetNonce(walletAddr)
	if err != nil {
		return err
	}
	auth, err := s.buildAuth(privateKey, nonce, big.NewInt(0), opts)
	if err != nil {
		return err
	}

	contract := bind.NewBoundContract(tokenAddr, parsed, blockchain.Client, blockchain.Client, blockchain.Client)
	tx, err := contract.Transact(auth, "approve", spender, tokenId)
	if err != nil {
		return err
	}
	_, err = s.waitMined(tx)
	return err
}

func (s *LiquidityService) exitV4ToUSDT(privateKey *ecdsa.PrivateKey, walletAddr common.Address, usdtAddr common.Address, task *models.StrategyTask, swapDeltas bool, opts TxOptions) ([]string, error) {
	var txHashes []string
	var swapErr error

	if !common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
		return nil, fmt.Errorf("UNISWAP_V4_POOL_MANAGER_ADDRESS not set")
	}
	if !common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		return nil, fmt.Errorf("UNISWAP_V4_POSITION_MANAGER_ADDRESS not set")
	}

	// Capture initial USDT balance
	usdtBefore, _ := blockchain.GetTokenBalance(usdtAddr, walletAddr)
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

	b0Before, _ := blockchain.GetTokenBalance(c0, walletAddr)
	b1Before, _ := blockchain.GetTokenBalance(c1, walletAddr)

	v4pmAddr := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
	v4pm, err := blockchain.NewV4PositionManager(v4pmAddr, blockchain.Client)
	if err != nil {
		return nil, fmt.Errorf("init v4 position manager failed: %w", err)
	}

	pos, posErr := v4pm.Positions(nil, tokenId)
	liq := big.NewInt(0)
	owed0 := big.NewInt(0)
	owed1 := big.NewInt(0)
	if posErr == nil && pos != nil {
		if pos.Liquidity != nil {
			liq = pos.Liquidity
		}
		if pos.TokensOwed0 != nil {
			owed0 = pos.TokensOwed0
		}
		if pos.TokensOwed1 != nil {
			owed1 = pos.TokensOwed1
		}
	}

	// If on-chain says the position is already empty, skip remove to make exit retry-safe.
	if posErr == nil && liq.Sign() <= 0 && owed0.Sign() <= 0 && owed1.Sign() <= 0 {
		log.Printf("[Liquidity] V4 exit: position already empty, skipping remove tokenId=%s", tokenId.String())
		return txHashes, nil
	}

	// If we couldn't query on-chain position, fall back to task.current_liquidity.
	if posErr != nil {
		liq, err = convert.ParseBigIntFlexible(task.CurrentLiquidity)
		if err != nil || liq == nil || liq.Sign() <= 0 {
			return nil, fmt.Errorf("missing/invalid current_liquidity for V4 remove and failed to query on-chain position: %w", posErr)
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
	decreaseParams, err := decreaseArgs.Pack(tokenId, liq, big.NewInt(0), big.NewInt(0), []byte{})
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

	log.Printf("[Liquidity] Removing V4 liquidity via PositionManager=%s tokenId=%s poolId=%s liq=%s", v4pmAddr.Hex(), tokenId.String(), task.PoolId, liq.String())
	nonce, err := blockchain.GetNonce(walletAddr)
	if err != nil {
		return nil, err
	}
	auth, err := s.buildAuth(privateKey, nonce, big.NewInt(0), opts)
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

	if _, err := s.waitMined(tx); err != nil {
		return txHashes, fmt.Errorf("v4 remove tx failed: %w", err)
	}

	b0After, _ := blockchain.GetTokenBalance(c0, walletAddr)
	b1After, _ := blockchain.GetTokenBalance(c1, walletAddr)
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
		if s0, sErr := blockchain.GetTokenSymbol(c0); sErr == nil && strings.TrimSpace(s0) != "" {
			sym0 = strings.TrimSpace(s0)
		} else {
			sym0 = c0.Hex()
		}
	}
	sym1 := strings.TrimSpace(task.Token1Symbol)
	if sym1 == "" {
		if s1, sErr := blockchain.GetTokenSymbol(c1); sErr == nil && strings.TrimSpace(s1) != "" {
			sym1 = strings.TrimSpace(s1)
		} else {
			sym1 = c1.Hex()
		}
	}

	if swapDeltas {
		var swapErrs []error
		if d0.Sign() > 0 && c0 != usdtAddr {
			if hash, err := s.swapDeltaToUSDTWithHash(privateKey, walletAddr, c0, usdtAddr, d0, task.SlippageTolerance); err != nil {
				swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT failed: %w", sym0, err))
			} else if hash != "" {
				txHashes = append(txHashes, fmt.Sprintf("交换 %s→USDT|%s", sym0, hash))
			} else {
				swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT returned empty tx hash", sym0))
			}
		}
		if d1.Sign() > 0 && c1 != usdtAddr {
			if hash, err := s.swapDeltaToUSDTWithHash(privateKey, walletAddr, c1, usdtAddr, d1, task.SlippageTolerance); err != nil {
				swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT failed: %w", sym1, err))
			} else if hash != "" {
				txHashes = append(txHashes, fmt.Sprintf("交换 %s→USDT|%s", sym1, hash))
			} else {
				swapErrs = append(swapErrs, fmt.Errorf("swap %s→USDT returned empty tx hash", sym1))
			}
		}
		if len(swapErrs) > 0 {
			swapErr = errors.Join(swapErrs...)
		}
	}

	// Record Transaction
	usdtAfter, _ := blockchain.GetTokenBalance(usdtAddr, walletAddr)
	if usdtAfter == nil {
		usdtAfter = big.NewInt(0)
	}
	totalUSDTReceived := new(big.Int).Sub(usdtAfter, usdtBefore)
	if totalUSDTReceived.Sign() < 0 {
		totalUSDTReceived = big.NewInt(0)
	}

	mainHash := removeTxHash

	txRecord := models.Transaction{
		UserID:          task.UserID,
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
	if err := database.DB.Create(&txRecord).Error; err != nil {
		log.Printf("[Liquidity] Warning: failed to record V4 exit transaction: %v", err)
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
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	usdtAddr common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) error {
	if amountIn == nil || amountIn.Sign() <= 0 {
		return nil
	}
	if tokenIn == usdtAddr {
		return nil
	}

	// 检查实际余额
	actualBalance, err := blockchain.GetTokenBalance(tokenIn, walletAddr)
	if err != nil {
		log.Printf("[Liquidity] Warning: failed to get token balance: %v", err)
	} else {
		if actualBalance == nil {
			actualBalance = big.NewInt(0)
		}
		log.Printf("[Liquidity] Token %s balance: %s, attempting to swap: %s", tokenIn.Hex(), actualBalance.String(), amountIn.String())
		// 如果实际余额小于要 swap 的数量，先等待 RPC 同步再决定是否截断。
		if actualBalance.Cmp(amountIn) < 0 {
			synced, werr := s.waitTokenBalanceAtLeast(tokenIn, walletAddr, amountIn, tokenIn.Hex())
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

	_, err = s.swapExactInViaOKX(privateKey, walletAddr, tokenIn, usdtAddr, amountIn, slippagePercent)
	return err
}

// swapDeltaToUSDTWithHash 与 swapDeltaToUSDT 类似，但返回交易哈希
func (s *LiquidityService) swapDeltaToUSDTWithHash(
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	usdtAddr common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (string, error) {
	if amountIn == nil || amountIn.Sign() <= 0 {
		return "", nil
	}
	if tokenIn == usdtAddr {
		return "", nil
	}

	// 检查实际余额
	actualBalance, err := blockchain.GetTokenBalance(tokenIn, walletAddr)
	if err != nil {
		log.Printf("[Liquidity] Warning: failed to get token balance: %v", err)
	} else {
		if actualBalance == nil {
			actualBalance = big.NewInt(0)
		}
		log.Printf("[Liquidity] Token %s balance: %s, attempting to swap: %s", tokenIn.Hex(), actualBalance.String(), amountIn.String())
		// 如果实际余额小于要 swap 的数量，先等待 RPC 同步再决定是否截断。
		if actualBalance.Cmp(amountIn) < 0 {
			synced, werr := s.waitTokenBalanceAtLeast(tokenIn, walletAddr, amountIn, tokenIn.Hex())
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

	txHash, err := s.swapExactInViaOKXWithHash(privateKey, walletAddr, tokenIn, usdtAddr, amountIn, slippagePercent)
	return txHash, err
}

// swapExactInViaOKXWithHash 与 swapExactInViaOKX 类似，但返回交易哈希
func (s *LiquidityService) swapExactInViaOKXWithHash(
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (string, error) {
	if config.AppConfig == nil {
		return "", fmt.Errorf("config not loaded")
	}
	if blockchain.Client == nil || blockchain.ChainID == nil {
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
		ChainID:           fmt.Sprintf("%d", config.AppConfig.BSCChainID),
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
		return "", fmt.Errorf("OKX tx.data empty")
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
	if err := EnforceOkxSwapRouter("swap", swapTx); err != nil {
		return "", err
	}

	// 获取 OKX TokenApprove 合约地址
	chainID := fmt.Sprintf("%d", config.AppConfig.BSCChainID)
	approveSpender, err := s.okxService.GetApproveSpender(chainID, tokenIn.Hex())
	if err != nil {
		log.Printf("[Liquidity] Warning: failed to get OKX approve spender, using router as fallback: %v", err)
		approveSpender = to.Hex()
	}
	approveAddr := common.HexToAddress(approveSpender)

	log.Printf("[Liquidity] OKX swap: %s -> %s amount=%s router=%s approveTarget=%s",
		tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), to.Hex(), approveAddr.Hex())

	// Approve TokenApprove 合约
	if err := s.approveToken(privateKey, walletAddr, tokenIn, approveAddr, amountIn, TxOptions{}); err != nil {
		return "", fmt.Errorf("approve TokenApprove contract failed: %w", err)
	}

	nonce, err := blockchain.GetNonce(walletAddr)
	if err != nil {
		return "", err
	}
	gasPrice, err := blockchain.GetGasPrice()
	if err != nil {
		return "", err
	}

	gasLimit, err := okxSwapGasLimit(walletAddr, to, value, data, okxGasLimit)
	if err != nil {
		return "", err
	}
	log.Printf("[Liquidity] OKX swap gasLimit: okx=%d final=%d", okxGasLimit, gasLimit)

	rawTx := types.NewTransaction(nonce, to, value, gasLimit, gasPrice, data)
	signed, err := types.SignTx(rawTx, types.NewEIP155Signer(blockchain.ChainID), privateKey)
	if err != nil {
		return "", err
	}
	if _, err := blockchain.SendTransaction(signed); err != nil {
		return "", err
	}

	txHash := signed.Hash().Hex()
	if _, err := s.waitMined(signed); err != nil {
		return txHash, err
	}

	return txHash, nil
}

// isNFTApprovedForAll 检查是否已经 setApprovalForAll
func (s *LiquidityService) isNFTApprovedForAll(nftContract, owner, operator common.Address) (bool, error) {
	const erc721ABI = `[{"constant":true,"inputs":[{"name":"owner","type":"address"},{"name":"operator","type":"address"}],"name":"isApprovedForAll","outputs":[{"name":"","type":"bool"}],"payable":false,"stateMutability":"view","type":"function"}]`

	parsed, err := abi.JSON(strings.NewReader(erc721ABI))
	if err != nil {
		return false, err
	}

	data, err := parsed.Pack("isApprovedForAll", owner, operator)
	if err != nil {
		return false, err
	}

	msg := ethereum.CallMsg{To: &nftContract, Data: data}
	res, err := blockchain.Client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return false, err
	}

	var out []interface{}
	out, err = parsed.Unpack("isApprovedForAll", res)
	if err != nil {
		return false, err
	}

	if len(out) > 0 {
		if approved, ok := out[0].(bool); ok {
			return approved, nil
		}
	}

	return false, nil
}

// setNFTApprovalForAll 设置 setApprovalForAll
func (s *LiquidityService) setNFTApprovalForAll(
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	nftContract common.Address,
	operator common.Address,
	approved bool,
	opts TxOptions,
) error {
	const erc721ABI = `[{"constant":false,"inputs":[{"name":"operator","type":"address"},{"name":"approved","type":"bool"}],"name":"setApprovalForAll","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"}]`

	parsed, err := abi.JSON(strings.NewReader(erc721ABI))
	if err != nil {
		return err
	}

	nonce, err := blockchain.GetNonce(walletAddr)
	if err != nil {
		return err
	}

	auth, err := s.buildAuth(privateKey, nonce, big.NewInt(0), opts)
	if err != nil {
		return err
	}

	contract := bind.NewBoundContract(nftContract, parsed, blockchain.Client, blockchain.Client, blockchain.Client)
	tx, err := contract.Transact(auth, "setApprovalForAll", operator, approved)
	if err != nil {
		return err
	}

	_, err = s.waitMined(tx)
	return err
}
