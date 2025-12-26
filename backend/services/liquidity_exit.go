package services

import (
	"TgLpBot/blockchain"
	"TgLpBot/config"
	"TgLpBot/database"
	"TgLpBot/models"
	"context"
	"crypto/ecdsa"
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

func parseBigIntFlexible(s string) (*big.Int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty number")
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, ok := new(big.Int).SetString(s[2:], 16)
		if !ok {
			return nil, fmt.Errorf("invalid hex number")
		}
		return v, nil
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("invalid decimal number")
	}
	return v, nil
}

func (s *LiquidityService) ExitTaskToUSDT(userID uint, task *models.StrategyTask, sweepWallet bool) ([]string, error) {
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

	var txHashes []string
	swapDeltas := !sweepWallet
	switch strings.ToLower(strings.TrimSpace(task.PoolVersion)) {
	case "v4":
		txHashes, err = s.exitV4ToUSDT(privateKey, walletAddr, usdtAddr, task, swapDeltas)
	default:
		txHashes, err = s.exitV3ToUSDT(privateKey, walletAddr, usdtAddr, task, swapDeltas)
	}
	if err != nil {
		return nil, err
	}

	if sweepWallet {
		if sweepHashes, sweepErr := s.swapWalletTokensToUSDT(privateKey, walletAddr, usdtAddr, task); sweepErr != nil {
			log.Printf("[Liquidity] Warning: sweep wallet tokens failed: %v", sweepErr)
		} else if len(sweepHashes) > 0 {
			txHashes = append(txHashes, sweepHashes...)
		}
	}

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

	mainHash := ""
	if len(txHashes) > 0 {
		parts := strings.Split(txHashes[0], "|")
		if len(parts) >= 2 {
			mainHash = strings.TrimSpace(parts[1])
		} else {
			mainHash = strings.TrimSpace(txHashes[0])
		}
	}

	// 获取 BNB 价格用于计算 Gas 的 USDT 价值
	bnbPriceUSDT := NewPnLService().GetBNBPriceUSDT()
	_ = NewTradeRecordService().CloseLatestOpenRecord(task, mainHash, actualReceived, gasSpent, bnbPriceUSDT)
	if mainHash != "" {
		if err := database.DB.Model(&models.Transaction{}).Where("tx_hash = ? AND task_id = ?", mainHash, task.ID).Updates(map[string]interface{}{
			"amount_out": actualReceived.String(),
		}).Error; err != nil {
			log.Printf("[Liquidity] Warning: update exit transaction amount_out failed: %v", err)
		}
	}

	return txHashes, nil
}

func (s *LiquidityService) swapWalletTokensToUSDT(
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	usdtAddr common.Address,
	task *models.StrategyTask,
) ([]string, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}

	token0 := common.Address{}
	token1 := common.Address{}
	if common.IsHexAddress(task.Token0Address) {
		token0 = common.HexToAddress(task.Token0Address)
	}
	if common.IsHexAddress(task.Token1Address) {
		token1 = common.HexToAddress(task.Token1Address)
	}
	if (token0 == common.Address{} || token1 == common.Address{}) && common.IsHexAddress(task.PoolId) {
		if c0, c1, err := blockchain.GetV3PoolTokens(common.HexToAddress(task.PoolId)); err == nil {
			if token0 == (common.Address{}) {
				token0 = c0
			}
			if token1 == (common.Address{}) {
				token1 = c1
			}
		}
	}

	tokens := []struct {
		addr   common.Address
		symbol string
	}{
		{addr: token0, symbol: strings.TrimSpace(task.Token0Symbol)},
		{addr: token1, symbol: strings.TrimSpace(task.Token1Symbol)},
	}

	seen := make(map[common.Address]struct{})
	var txHashes []string
	for _, tok := range tokens {
		if tok.addr == (common.Address{}) || tok.addr == usdtAddr {
			continue
		}
		if _, ok := seen[tok.addr]; ok {
			continue
		}
		seen[tok.addr] = struct{}{}

		bal, err := blockchain.GetTokenBalance(tok.addr, walletAddr)
		if err != nil {
			log.Printf("[Liquidity] Warning: failed to get token balance for sweep: %v", err)
			continue
		}
		if bal == nil || bal.Sign() <= 0 {
			continue
		}

		swapTxHash, err := s.swapDeltaToUSDTWithHash(privateKey, walletAddr, tok.addr, usdtAddr, bal, task.SlippageTolerance)
		if err != nil {
			log.Printf("[Liquidity] Warning: sweep swap failed for %s: %v", tok.addr.Hex(), err)
			continue
		}
		if swapTxHash != "" {
			symbol := tok.symbol
			if symbol == "" {
				symbol = tok.addr.Hex()
			}
			txHashes = append(txHashes, fmt.Sprintf("清仓 %s→USDT|%s", symbol, swapTxHash))
		}
	}

	return txHashes, nil
}

func (s *LiquidityService) buildAuth(privateKey *ecdsa.PrivateKey, nonce uint64, value *big.Int, gasLimit uint64) (*bind.TransactOpts, error) {
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, blockchain.ChainID)
	if err != nil {
		return nil, err
	}
	gasPrice, err := blockchain.GetGasPrice()
	if err != nil {
		return nil, err
	}
	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = value
	auth.GasLimit = gasLimit
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

func (s *LiquidityService) exitV3ToUSDT(privateKey *ecdsa.PrivateKey, walletAddr common.Address, usdtAddr common.Address, task *models.StrategyTask, swapDeltas bool) ([]string, error) {
	var txHashes []string

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
	tokenId, err := parseBigIntFlexible(task.V3TokenID)
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
	sqrtA, err := SqrtRatioAtTick(int32(posInfo.TickLower))
	if err != nil {
		return nil, fmt.Errorf("compute sqrtA failed: %w", err)
	}
	sqrtB, err := SqrtRatioAtTick(int32(posInfo.TickUpper))
	if err != nil {
		return nil, fmt.Errorf("compute sqrtB failed: %w", err)
	}
	expected0, expected1 := AmountsForLiquidity(sqrtPriceX96, sqrtA, sqrtB, posInfo.Liquidity)

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
		if err := s.setNFTApprovalForAll(privateKey, walletAddr, pmAddr, zapAddr, true); err != nil {
			// 如果 setApprovalForAll 失败，降级到单个 approve
			log.Printf("[Liquidity] Warning: setApprovalForAll failed, falling back to single approve: %v", err)
			if err := s.approveNFT(privateKey, walletAddr, pmAddr, zapAddr, tokenId); err != nil {
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
	auth, err := s.buildAuth(privateKey, nonce, big.NewInt(0), config.AppConfig.GasLimit)
	if err != nil {
		return nil, err
	}

	zap, err := blockchain.NewZapSimple(zapAddr, blockchain.Client)
	if err != nil {
		return nil, fmt.Errorf("init zap contract failed: %w", err)
	}

	log.Printf("[Liquidity] V3 exit: Calling ZapOutV3 tokenId=%s amount0Min=%s amount1Min=%s", tokenId.String(), amount0Min.String(), amount1Min.String())
	tx, err := zap.ZapOutV3(auth, pmAddr, tokenId, walletAddr, amount0Min, amount1Min)
	if err != nil {
		return nil, fmt.Errorf("ZapOutV3 call failed: %w", err)
	}
	log.Printf("[Liquidity] V3 exit: tx sent %s", tx.Hash().Hex())
	txHashes = append(txHashes, "撤出流动性|"+tx.Hash().Hex())

	if _, err := s.waitMined(tx); err != nil {
		return txHashes, fmt.Errorf("ZapOutV3 tx failed: %w", err)
	}

	// 5. 计算获得的代币并 Swap 回 USDT
	b0After, _ := blockchain.GetTokenBalance(token0, walletAddr)
	b1After, _ := blockchain.GetTokenBalance(token1, walletAddr)

	d0 := new(big.Int).Sub(b0After, b0Before)
	d1 := new(big.Int).Sub(b1After, b1Before)

	if swapDeltas {
		if d0.Sign() > 0 {
			log.Printf("[Liquidity] V3 exit: Got %s token0, swapping to USDT...", d0.String())
			if swapTxHash, err := s.swapDeltaToUSDTWithHash(privateKey, walletAddr, token0, usdtAddr, d0, task.SlippageTolerance); err != nil {
				log.Printf("[Liquidity] Warning: swap token0->USDT failed: %v", err)
			} else if swapTxHash != "" {
				symbol := "Token0"
				if task.Token0Symbol != "" {
					symbol = task.Token0Symbol
				}
				txHashes = append(txHashes, fmt.Sprintf("兑换 %s→USDT|%s", symbol, swapTxHash))
			}
		}
		if d1.Sign() > 0 {
			log.Printf("[Liquidity] V3 exit: Got %s token1, swapping to USDT...", d1.String())
			if swapTxHash, err := s.swapDeltaToUSDTWithHash(privateKey, walletAddr, token1, usdtAddr, d1, task.SlippageTolerance); err != nil {
				log.Printf("[Liquidity] Warning: swap token1->USDT failed: %v", err)
			} else if swapTxHash != "" {
				symbol := "Token1"
				if task.Token1Symbol != "" {
					symbol = task.Token1Symbol
				}
				txHashes = append(txHashes, fmt.Sprintf("兑换 %s→USDT|%s", symbol, swapTxHash))
			}
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
	// Try creating. If hash conflict (unlikely if unique), log error
	if mainHash != "" {
		if err := database.DB.Create(&txRecord).Error; err != nil {
			log.Printf("[Liquidity] Warning: failed to record exit transaction: %v", err)
		}
	}

	return txHashes, nil
}

// approveNFT checks approval and approves if needed (ERC721)
func (s *LiquidityService) approveNFT(privateKey *ecdsa.PrivateKey, walletAddr, tokenAddr, spender common.Address, tokenId *big.Int) error {
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
	auth, err := s.buildAuth(privateKey, nonce, big.NewInt(0), config.AppConfig.GasLimit)
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

func (s *LiquidityService) exitV4ToUSDT(privateKey *ecdsa.PrivateKey, walletAddr common.Address, usdtAddr common.Address, task *models.StrategyTask, swapDeltas bool) ([]string, error) {
	var txHashes []string

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

	tokenId, err := parseBigIntFlexible(task.V4TokenID)
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

	liq, err := parseBigIntFlexible(task.CurrentLiquidity)
	if err != nil || liq == nil || liq.Sign() <= 0 {
		if pos, pErr := v4pm.Positions(nil, tokenId); pErr == nil && pos != nil && pos.Liquidity != nil && pos.Liquidity.Sign() > 0 {
			log.Printf("[Liquidity] V4 exit: using on-chain liquidity %s (task value invalid)", pos.Liquidity.String())
			liq = pos.Liquidity
		}
	}
	if liq == nil || liq.Sign() <= 0 {
		return nil, fmt.Errorf("missing/invalid current_liquidity for V4 remove (need >0)")
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
	auth, err := s.buildAuth(privateKey, nonce, big.NewInt(0), config.AppConfig.GasLimit)
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
		return nil, fmt.Errorf("v4 remove tx failed: %w", err)
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
		if d0.Sign() > 0 {
			if hash, err := s.swapDeltaToUSDTWithHash(privateKey, walletAddr, c0, usdtAddr, d0, task.SlippageTolerance); err != nil {
				log.Printf("[Liquidity] Warning: swap currency0->USDT failed: %v", err)
			} else if hash != "" {
				txHashes = append(txHashes, fmt.Sprintf("交换 %s→USDT|%s", sym0, hash))
			}
		}
		if d1.Sign() > 0 {
			if hash, err := s.swapDeltaToUSDTWithHash(privateKey, walletAddr, c1, usdtAddr, d1, task.SlippageTolerance); err != nil {
				log.Printf("[Liquidity] Warning: swap currency1->USDT failed: %v", err)
			} else if hash != "" {
				txHashes = append(txHashes, fmt.Sprintf("交换 %s→USDT|%s", sym1, hash))
			}
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
		log.Printf("[Liquidity] Token %s balance: %s, attempting to swap: %s", tokenIn.Hex(), actualBalance.String(), amountIn.String())
		// 如果实际余额小于要 swap 的数量，使用实际余额
		if actualBalance.Cmp(amountIn) < 0 {
			log.Printf("[Liquidity] Warning: balance insufficient, using actual balance %s instead of %s", actualBalance.String(), amountIn.String())
			amountIn = actualBalance
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
		log.Printf("[Liquidity] Token %s balance: %s, attempting to swap: %s", tokenIn.Hex(), actualBalance.String(), amountIn.String())
		// 如果实际余额小于要 swap 的数量，使用实际余额
		if actualBalance.Cmp(amountIn) < 0 {
			log.Printf("[Liquidity] Warning: balance insufficient, using actual balance %s instead of %s", actualBalance.String(), amountIn.String())
			amountIn = actualBalance
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
		s.okxService = NewOKXDexService()
	}

	swapResp, err := s.okxService.GetSwapData(SwapRequest{
		ChainID:           fmt.Sprintf("%d", config.AppConfig.BSCChainID),
		FromTokenAddress:  tokenIn.Hex(),
		ToTokenAddress:    tokenOut.Hex(),
		Amount:            amountIn.String(),
		Slippage:          s.okxSlippageDecimal(slippagePercent),
		UserWalletAddress: walletAddr.Hex(),
	})
	if err != nil {
		return "", err
	}
	if len(swapResp.Data) == 0 {
		return "", fmt.Errorf("OKX swap response empty")
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

	gasLimit := config.AppConfig.GasLimit
	if strings.TrimSpace(txObj.Gas) != "" {
		if g, ok := new(big.Int).SetString(strings.TrimSpace(txObj.Gas), 10); ok && g.IsUint64() {
			gasLimit = g.Uint64()
		}
	}

	swapTx := blockchain.OkxSwapTx{To: to, Value: value, Data: data}
	_ = validateOkxSmartSwapTx("swap", swapTx)
	if err := enforceOkxSwapRouter("swap", swapTx); err != nil {
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
	if err := s.approveToken(privateKey, walletAddr, tokenIn, approveAddr, amountIn); err != nil {
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

	auth, err := s.buildAuth(privateKey, nonce, big.NewInt(0), config.AppConfig.GasLimit)
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
