package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"bytes"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// IncreaseLiquidityResult holds the result of an increase-liquidity operation.
type IncreaseLiquidityResult struct {
	TxHash               string
	AddedLiquidity       string
	CurrentLiquidity     string
	TickLower            *int
	TickUpper            *int
	ActualStableSpent    float64
	ActualStableSpentWei *big.Int
	Dust0Wei             *big.Int
	Dust1Wei             *big.Int
	Token0               common.Address
	Token1               common.Address
	GasSpentWei          *big.Int
}

// IncreaseLiquidityForTask adds liquidity to an existing V3/V4 position.
// Unlike EnterTaskFromUSDT (which mints a new position), this calls
// NonfungiblePositionManager.increaseLiquidity (V3) or modifyLiquidities with
// INCREASE_LIQUIDITY action (V4) to add to the existing tokenId.
func (s *LiquidityService) IncreaseLiquidityForTask(userID uint, task *models.StrategyTask, addAmountUSDT float64) (*IncreaseLiquidityResult, error) {
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

	usdtAmount, err := convert.FloatToUnits(addAmountUSDT, cc.StableDecimals)
	if err != nil {
		return nil, err
	}

	if !common.IsHexAddress(cc.StableAddress) {
		return nil, fmt.Errorf("stable address not set for chain=%s", exec.Chain())
	}
	usdtAddr := common.HexToAddress(cc.StableAddress)

	// Cap to wallet balance
	usdtBal, _ := blockchain.GetTokenBalanceWithClient(client, usdtAddr, walletAddr)
	if usdtBal == nil {
		usdtBal = big.NewInt(0)
	}
	if usdtBal.Sign() > 0 && usdtBal.Cmp(usdtAmount) < 0 {
		log.Printf("[Liquidity] IncreaseLiquidity: capping to wallet USDT balance=%s (requested=%s)", usdtBal.String(), usdtAmount.String())
		usdtAmount = new(big.Int).Set(usdtBal)
	}
	if usdtAmount.Sign() <= 0 {
		return nil, fmt.Errorf("insufficient USDT balance")
	}

	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	if config.AppConfig.AtomicAddLiquidityEnabled && config.AppConfig.PrivateZapEnabled {
		switch version {
		case "v4":
			return s.increaseV4LiquidityAtomic(exec, wallet, privateKey, walletAddr, usdtAddr, usdtAmount, task, TxOptions{})
		default:
			return s.increaseV3LiquidityAtomic(exec, wallet, privateKey, walletAddr, usdtAddr, usdtAmount, task, TxOptions{})
		}
	}

	// Convert USDT to entry token if needed (same logic as EnterTaskFromUSDT)
	plan, err := s.planEntryToken(task)
	if err != nil {
		return nil, err
	}
	entryToken := usdtAddr
	entryAmount := new(big.Int).Set(usdtAmount)
	if plan.RequiresSwap {
		if !task.AllowEntrySwap {
			// For increase liquidity, always allow the swap
			task.AllowEntrySwap = true
		}
		swapped, swapErr := s.swapExactInViaOKX(exec, privateKey, walletAddr, usdtAddr, plan.EntryToken, usdtAmount, task.SlippageTolerance)
		if swapErr != nil {
			return nil, fmt.Errorf("swap USDT to %s failed: %w", plan.EntrySymbol, swapErr)
		}
		if swapped == nil || swapped.Sign() <= 0 {
			return nil, fmt.Errorf("swap USDT to %s returned 0", plan.EntrySymbol)
		}
		entryToken = plan.EntryToken
		entryAmount = swapped
	}

	var res *IncreaseLiquidityResult
	switch version {
	case "v4":
		res, err = s.increaseV4Liquidity(exec, wallet, privateKey, walletAddr, entryToken, entryAmount, task, TxOptions{})
	default:
		res, err = s.increaseV3Liquidity(exec, wallet, privateKey, walletAddr, entryToken, entryAmount, task, TxOptions{})
	}
	if err != nil {
		return nil, err
	}
	if res != nil && (res.ActualStableSpentWei == nil || res.ActualStableSpentWei.Sign() <= 0) {
		res.ActualStableSpentWei = new(big.Int).Set(usdtAmount)
		res.ActualStableSpent = amountToFloat(usdtAmount, cc.StableDecimals)
	}
	return res, nil
}

func pickIncreasePositionRange(taskTickLower, taskTickUpper int, onchainTickLower, onchainTickUpper int) (int, int, bool) {
	if onchainTickLower < onchainTickUpper {
		return onchainTickLower, onchainTickUpper, onchainTickLower != taskTickLower || onchainTickUpper != taskTickUpper
	}
	return taskTickLower, taskTickUpper, false
}

func bestEffortReadV4PositionInfo(
	client chainexec.EVMExecutor,
	positionManager common.Address,
	poolManager common.Address,
	poolID string,
	tokenId *big.Int,
) (*blockchain.V4PositionInfo, error) {
	if config.NormalizeChain(client.Chain()) == "bsc" && blockchain.Client != nil {
		return blockchain.GetV4PositionInfo(positionManager, poolManager, poolID, tokenId)
	}
	return nil, fmt.Errorf("v4 position read requires supported chain client")
}

func (s *LiquidityService) increaseV3Liquidity(
	exec chainexec.EVMExecutor,
	wallet *models.Wallet,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	amountIn *big.Int,
	task *models.StrategyTask,
	opts TxOptions,
) (*IncreaseLiquidityResult, error) {
	client := exec.Client()
	chainID := exec.ChainID()
	cc := exec.Config()

	tokenIdStr := strings.TrimSpace(task.V3TokenID)
	if tokenIdStr == "" || tokenIdStr == "0" {
		return nil, fmt.Errorf("task has no V3 tokenId, cannot increase liquidity")
	}
	tokenId, ok := new(big.Int).SetString(tokenIdStr, 10)
	if !ok || tokenId.Sign() <= 0 {
		return nil, fmt.Errorf("invalid V3 tokenId: %s", tokenIdStr)
	}

	pmAddrStr := strings.TrimSpace(task.V3PositionManagerAddress)
	if !common.IsHexAddress(pmAddrStr) {
		return nil, fmt.Errorf("task has no V3 position manager address")
	}
	pmAddr := common.HexToAddress(pmAddrStr)
	pm, err := blockchain.NewV3PositionManager(pmAddr, client)
	if err != nil {
		return nil, fmt.Errorf("init V3 PM failed: %w", err)
	}

	posInfo, err := pm.Positions(nil, tokenId)
	if err != nil {
		return nil, fmt.Errorf("read V3 position failed: %w", err)
	}
	onchainTickLower, onchainTickUpper := 0, 0
	if posInfo != nil {
		onchainTickLower = posInfo.TickLower
		onchainTickUpper = posInfo.TickUpper
	}
	rangeLower, rangeUpper, rangeSynced := pickIncreasePositionRange(task.TickLower, task.TickUpper, onchainTickLower, onchainTickUpper)
	if rangeSynced {
		log.Printf("[Liquidity] V3 increase: use on-chain position range tick=%d/%d instead of task tick=%d/%d (tokenId=%s)",
			rangeLower, rangeUpper, task.TickLower, task.TickUpper, tokenId.String())
	}

	if !common.IsHexAddress(task.PoolId) {
		return nil, fmt.Errorf("invalid V3 pool address: %s", task.PoolId)
	}
	poolAddr := common.HexToAddress(task.PoolId)

	token0, token1, err := blockchain.GetV3PoolTokensWithClient(client, poolAddr)
	if err != nil {
		return nil, fmt.Errorf("read v3 pool tokens failed: %w", err)
	}
	if bytes.Compare(token0.Bytes(), token1.Bytes()) >= 0 {
		return nil, fmt.Errorf("unexpected v3 token ordering")
	}
	if posInfo != nil {
		if posInfo.Token0 != (common.Address{}) && posInfo.Token1 != (common.Address{}) &&
			(posInfo.Token0 != token0 || posInfo.Token1 != token1) {
			return nil, fmt.Errorf("V3 position token mismatch with task pool: position=%s/%s pool=%s/%s",
				posInfo.Token0.Hex(), posInfo.Token1.Hex(), token0.Hex(), token1.Hex())
		}
	}

	if token0 != tokenIn && token1 != tokenIn {
		return nil, fmt.Errorf("V3 pool does not contain entry token")
	}

	// Capture balances before any operation so we can measure actual stable spent and dust returned.
	stableAddr := common.Address{}
	if common.IsHexAddress(cc.StableAddress) {
		stableAddr = common.HexToAddress(cc.StableAddress)
	}
	stableBefore := tokenBalanceOrZero(exec, stableAddr, walletAddr)
	token0Before := tokenBalanceOrZero(exec, token0, walletAddr)
	token1Before := tokenBalanceOrZero(exec, token1, walletAddr)

	// Determine input amounts
	amount0In := big.NewInt(0)
	amount1In := big.NewInt(0)
	if token0 == tokenIn {
		amount0In = new(big.Int).Set(amountIn)
	} else {
		amount1In = new(big.Int).Set(amountIn)
	}

	// Calculate optimal swap to split into token0/token1
	zeroForOne, swapAmount, err := s.calculateOptimalSwapLocal(client, poolAddr, rangeLower, rangeUpper, amount0In, amount1In)
	if err != nil {
		log.Printf("[Liquidity] V3 increase: calculateOptimalSwapLocal failed: %v, fallback to half", err)
		swapAmount = new(big.Int).Div(amountIn, big.NewInt(2))
		zeroForOne = (token0 == tokenIn)
	}

	// Execute the internal swap via OKX to get both tokens
	if swapAmount != nil && swapAmount.Sign() > 0 {
		var swapTokenIn, swapTokenOut common.Address
		if zeroForOne {
			swapTokenIn = token0
			swapTokenOut = token1
		} else {
			swapTokenIn = token1
			swapTokenOut = token0
		}

		swapped, swapErr := s.swapExactInViaOKX(exec, privateKey, walletAddr, swapTokenIn, swapTokenOut, swapAmount, task.SlippageTolerance)
		if swapErr != nil {
			return nil, fmt.Errorf("optimal swap failed: %w", swapErr)
		}

		// After swap, recalculate actual amounts from wallet balances
		time.Sleep(500 * time.Millisecond)
		if swapped != nil && swapped.Sign() > 0 {
			if zeroForOne {
				amount0In.Sub(amount0In, swapAmount)
				amount1In.Add(amount1In, swapped)
			} else {
				amount1In.Sub(amount1In, swapAmount)
				amount0In.Add(amount0In, swapped)
			}
		}
	}

	if amount0In.Sign() <= 0 && amount1In.Sign() <= 0 {
		return nil, fmt.Errorf("both token amounts are zero after swap")
	}

	// Approve tokens to PM (not Zap — we're calling PM directly)
	if amount0In.Sign() > 0 {
		if err := s.approveToken(client, chainID, privateKey, walletAddr, token0, pmAddr, amount0In, opts); err != nil {
			return nil, fmt.Errorf("approve token0 to PM failed: %w", err)
		}
	}
	if amount1In.Sign() > 0 {
		if err := s.approveToken(client, chainID, privateKey, walletAddr, token1, pmAddr, amount1In, opts); err != nil {
			return nil, fmt.Errorf("approve token1 to PM failed: %w", err)
		}
	}

	deadline := big.NewInt(time.Now().Add(5 * time.Minute).Unix())
	params := blockchain.V3IncreaseLiquidityParams{
		TokenId:        tokenId,
		Amount0Desired: amount0In,
		Amount1Desired: amount1In,
		Amount0Min:     big.NewInt(0),
		Amount1Min:     big.NewInt(0),
		Deadline:       deadline,
	}

	nonce, err := blockchain.GetNonceWithClient(client, walletAddr)
	if err != nil {
		return nil, err
	}
	auth, err := s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
	if err != nil {
		return nil, err
	}

	log.Printf("[Liquidity] V3 increaseLiquidity: tokenId=%s amount0=%s amount1=%s pm=%s",
		tokenId.String(), amount0In.String(), amount1In.String(), pmAddr.Hex())

	tx, err := pm.IncreaseLiquidity(auth, params)
	if err != nil {
		return nil, fmt.Errorf("increaseLiquidity failed: %w", err)
	}
	log.Printf("[Liquidity] V3 increaseLiquidity tx sent: %s", tx.Hash().Hex())

	receipt, err := s.waitMined(client, chainID, tx)
	if err != nil {
		return nil, fmt.Errorf("increaseLiquidity tx failed: %w", err)
	}

	// Read updated position liquidity
	posInfo, err = pm.Positions(nil, tokenId)
	var currentLiq string
	if err == nil && posInfo != nil && posInfo.Liquidity != nil {
		currentLiq = posInfo.Liquidity.String()
	} else {
		currentLiq = "0"
		log.Printf("[Liquidity] V3 increaseLiquidity: failed to read updated position: %v", err)
	}

	// Capture balances after all operations to compute actual stable spent and dust returned.
	stableAfter := tokenBalanceOrZero(exec, stableAddr, walletAddr)
	token0After := tokenBalanceOrZero(exec, token0, walletAddr)
	token1After := tokenBalanceOrZero(exec, token1, walletAddr)

	actualStableSpentWei := spentBalanceDelta(stableBefore, stableAfter)
	dust0Wei := positiveBalanceDelta(token0Before, token0After)
	dust1Wei := positiveBalanceDelta(token1Before, token1After)
	log.Printf("[Liquidity] V3 increaseLiquidity balance delta: tokenId=%s stableSpent=%s dust0=%s dust1=%s",
		tokenId.String(), actualStableSpentWei.String(), dust0Wei.String(), dust1Wei.String())

	return &IncreaseLiquidityResult{
		TxHash:               tx.Hash().Hex(),
		CurrentLiquidity:     currentLiq,
		TickLower:            &rangeLower,
		TickUpper:            &rangeUpper,
		ActualStableSpentWei: actualStableSpentWei,
		ActualStableSpent:    amountToFloat(actualStableSpentWei, cc.StableDecimals),
		Dust0Wei:             dust0Wei,
		Dust1Wei:             dust1Wei,
		Token0:               token0,
		Token1:               token1,
		GasSpentWei:          s.gasCostWeiFromReceipt(client, tx.Hash(), receipt),
	}, nil
}

func buildV4IncreaseUnlockData(
	tokenId *big.Int,
	liquidityDelta *big.Int,
	amount0Max *big.Int,
	amount1Max *big.Int,
	c0 common.Address,
	c1 common.Address,
) ([]byte, error) {
	actions := []byte{0x00, 0x0d} // INCREASE_LIQUIDITY=0x00, SETTLE_PAIR=0x0d

	uint256Ty, _ := abi.NewType("uint256", "", nil)
	uint128Ty, _ := abi.NewType("uint128", "", nil)
	addressTy, _ := abi.NewType("address", "", nil)
	bytesTy, _ := abi.NewType("bytes", "", nil)
	bytesArrTy, _ := abi.NewType("bytes[]", "", nil)

	increaseArgs := abi.Arguments{
		{Type: uint256Ty}, // tokenId
		{Type: uint256Ty}, // liquidityDelta
		{Type: uint128Ty}, // amount0Max
		{Type: uint128Ty}, // amount1Max
		{Type: bytesTy},   // hookData
	}
	increaseLiqParams, err := increaseArgs.Pack(tokenId, liquidityDelta, amount0Max, amount1Max, []byte{})
	if err != nil {
		return nil, fmt.Errorf("encode V4 INCREASE_LIQUIDITY params failed: %w", err)
	}

	settlePairArgs := abi.Arguments{
		{Type: addressTy}, // currency0
		{Type: addressTy}, // currency1
	}
	settlePairParams, err := settlePairArgs.Pack(c0, c1)
	if err != nil {
		return nil, fmt.Errorf("encode V4 SETTLE_PAIR params failed: %w", err)
	}

	unlockArgs := abi.Arguments{
		{Type: bytesTy},    // actions
		{Type: bytesArrTy}, // params[]
	}
	unlockData, err := unlockArgs.Pack(actions, [][]byte{increaseLiqParams, settlePairParams})
	if err != nil {
		return nil, fmt.Errorf("encode V4 unlockData failed: %w", err)
	}
	return unlockData, nil
}

func (s *LiquidityService) simulateV4ModifyLiquidities(
	pm *blockchain.V4PositionManager,
	client chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	nonce uint64,
	unlockData []byte,
	deadline *big.Int,
	opts TxOptions,
) error {
	if pm == nil {
		return fmt.Errorf("V4 position manager is nil")
	}
	auth, err := s.buildAuth(client.Client(), client.ChainID(), privateKey, nonce, big.NewInt(0), opts)
	if err != nil {
		return err
	}
	auth.NoSend = true
	_, err = pm.ModifyLiquidities(auth, unlockData, deadline)
	return err
}

func (s *LiquidityService) fitV4IncreaseLiquidityDelta(
	pm *blockchain.V4PositionManager,
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	nonce uint64,
	deadline *big.Int,
	tokenId *big.Int,
	amount0Max *big.Int,
	amount1Max *big.Int,
	c0 common.Address,
	c1 common.Address,
	initialLiquidityDelta *big.Int,
	opts TxOptions,
) (*big.Int, []byte, error) {
	if initialLiquidityDelta == nil || initialLiquidityDelta.Sign() <= 0 {
		return nil, nil, fmt.Errorf("initial liquidityDelta invalid")
	}

	one := big.NewInt(1)
	lo := big.NewInt(1)
	hi := new(big.Int).Set(initialLiquidityDelta)
	var best *big.Int
	var bestUnlockData []byte

	for lo.Cmp(hi) <= 0 {
		mid := new(big.Int).Add(lo, hi)
		mid.Rsh(mid, 1)
		if mid.Sign() <= 0 {
			break
		}

		unlockData, err := buildV4IncreaseUnlockData(tokenId, mid, amount0Max, amount1Max, c0, c1)
		if err != nil {
			return nil, nil, err
		}
		simErr := s.simulateV4ModifyLiquidities(pm, exec, privateKey, nonce, unlockData, deadline, opts)
		if simErr == nil {
			best = new(big.Int).Set(mid)
			bestUnlockData = unlockData
			lo = new(big.Int).Add(mid, one)
			continue
		}
		if strings.Contains(simErr.Error(), maximumAmountExceededSelector) {
			hi = new(big.Int).Sub(mid, one)
			continue
		}
		if hint := evmRevertHint(simErr); hint != "" {
			return nil, nil, fmt.Errorf("simulate V4 INCREASE_LIQUIDITY failed at liquidityDelta=%s: %s: %w", mid.String(), hint, simErr)
		}
		return nil, nil, fmt.Errorf("simulate V4 INCREASE_LIQUIDITY failed at liquidityDelta=%s: %w", mid.String(), simErr)
	}

	if best == nil || len(bestUnlockData) == 0 {
		return nil, nil, fmt.Errorf("no V4 liquidityDelta fits amount0Max=%s amount1Max=%s", amount0Max.String(), amount1Max.String())
	}

	return best, bestUnlockData, nil
}

func (s *LiquidityService) increaseV4Liquidity(
	exec chainexec.EVMExecutor,
	wallet *models.Wallet,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	amountIn *big.Int,
	task *models.StrategyTask,
	opts TxOptions,
) (*IncreaseLiquidityResult, error) {
	cc := exec.Config()
	client := exec.Client()
	chainID := exec.ChainID()

	tokenIdStr := strings.TrimSpace(task.V4TokenID)
	if tokenIdStr == "" || tokenIdStr == "0" {
		return nil, fmt.Errorf("task has no V4 tokenId, cannot increase liquidity")
	}
	tokenId, ok := new(big.Int).SetString(tokenIdStr, 10)
	if !ok || tokenId.Sign() <= 0 {
		return nil, fmt.Errorf("invalid V4 tokenId: %s", tokenIdStr)
	}

	if !common.IsHexAddress(cc.UniswapV4PositionManagerAddress) {
		return nil, fmt.Errorf("V4 PositionManager address not configured")
	}
	positionManager := common.HexToAddress(cc.UniswapV4PositionManagerAddress)
	pm, err := blockchain.NewV4PositionManager(positionManager, client)
	if err != nil {
		return nil, fmt.Errorf("init V4 PM failed: %w", err)
	}
	poolManager := common.Address{}
	if common.IsHexAddress(cc.UniswapV4PoolManagerAddress) {
		poolManager = common.HexToAddress(cc.UniswapV4PoolManagerAddress)
	}
	posInfo, err := bestEffortReadV4PositionInfo(exec, positionManager, poolManager, task.PoolId, tokenId)
	if err != nil {
		log.Printf("[Liquidity] Warning: read V4 position failed, fallback to task snapshot: tokenId=%s err=%v", tokenId.String(), err)
		posInfo = nil
	}

	// Resolve token addresses
	c0 := common.HexToAddress(task.Token0Address)
	c1 := common.HexToAddress(task.Token1Address)
	if posInfo != nil {
		if posInfo.Token0 != (common.Address{}) {
			c0 = posInfo.Token0
		}
		if posInfo.Token1 != (common.Address{}) {
			c1 = posInfo.Token1
		}
	}
	if bytes.Compare(c0.Bytes(), c1.Bytes()) >= 0 {
		return nil, fmt.Errorf("unexpected V4 token ordering")
	}
	onchainTickLower, onchainTickUpper := 0, 0
	if posInfo != nil {
		onchainTickLower = posInfo.TickLower
		onchainTickUpper = posInfo.TickUpper
	}
	rangeLower, rangeUpper, rangeSynced := pickIncreasePositionRange(task.TickLower, task.TickUpper, onchainTickLower, onchainTickUpper)
	if rangeSynced {
		log.Printf("[Liquidity] V4 increase: use on-chain position range tick=%d/%d instead of task tick=%d/%d (tokenId=%s)",
			rangeLower, rangeUpper, task.TickLower, task.TickUpper, tokenId.String())
	}

	if c0 != tokenIn && c1 != tokenIn {
		return nil, fmt.Errorf("V4 pool does not contain entry token")
	}

	// Capture balances before any operation so we can measure actual stable spent and dust returned.
	stableAddr := common.Address{}
	if common.IsHexAddress(cc.StableAddress) {
		stableAddr = common.HexToAddress(cc.StableAddress)
	}
	stableBefore := tokenBalanceOrZero(exec, stableAddr, walletAddr)
	c0Before := tokenBalanceOrZero(exec, c0, walletAddr)
	c1Before := tokenBalanceOrZero(exec, c1, walletAddr)

	// Determine input amounts
	amount0In := big.NewInt(0)
	amount1In := big.NewInt(0)
	if c0 == tokenIn {
		amount0In = new(big.Int).Set(amountIn)
	} else {
		amount1In = new(big.Int).Set(amountIn)
	}

	// Read pool sqrtPriceX96 via V4 StateView to compute optimal swap ratio
	if !common.IsHexAddress(cc.UniswapV4StateViewAddress) || !common.IsHexAddress(cc.UniswapV4PoolManagerAddress) {
		return nil, fmt.Errorf("V4 StateView or PoolManager address not configured")
	}
	stateViewAddr := common.HexToAddress(cc.UniswapV4StateViewAddress)
	poolManagerAddr := poolManager
	sqrtPriceX96, currentTick, slotErr := blockchain.GetUniswapV4PoolSlot0ViaStateView(stateViewAddr, poolManagerAddr, task.PoolId)
	if slotErr != nil {
		return nil, fmt.Errorf("read V4 pool sqrtPriceX96 failed: %w", slotErr)
	}

	// Use optimal swap calculation based on actual pool price
	zeroForOne, swapAmount, _ := s.calculateOptimalSwapPure(sqrtPriceX96, currentTick, rangeLower, rangeUpper, amount0In, amount1In)
	if swapAmount != nil && swapAmount.Sign() > 0 {
		var swapFrom, swapTo common.Address
		if zeroForOne {
			swapFrom, swapTo = c0, c1
		} else {
			swapFrom, swapTo = c1, c0
		}
		swapped, swapErr := s.swapExactInViaOKX(exec, privateKey, walletAddr, swapFrom, swapTo, swapAmount, task.SlippageTolerance)
		if swapErr != nil {
			return nil, fmt.Errorf("V4 optimal swap failed: %w", swapErr)
		}
		time.Sleep(500 * time.Millisecond)
		if swapped != nil && swapped.Sign() > 0 {
			if zeroForOne {
				amount0In.Sub(amount0In, swapAmount)
				amount1In.Add(amount1In, swapped)
			} else {
				amount1In.Sub(amount1In, swapAmount)
				amount0In.Add(amount0In, swapped)
			}
		}
	}

	// Re-read pool price after swap for accurate liquidity calculation
	sqrtPriceX96After, _, slotErrAfter := blockchain.GetUniswapV4PoolSlot0ViaStateView(stateViewAddr, poolManagerAddr, task.PoolId)
	if slotErrAfter == nil && sqrtPriceX96After != nil && sqrtPriceX96After.Sign() > 0 {
		sqrtPriceX96 = sqrtPriceX96After
	}

	if amount0In.Sign() <= 0 && amount1In.Sign() <= 0 {
		return nil, fmt.Errorf("both token amounts are zero after swap")
	}

	amount0Max := new(big.Int).Set(amount0In)
	amount1Max := new(big.Int).Set(amount1In)
	if bal0, balErr := blockchain.GetTokenBalanceWithClient(client, c0, walletAddr); balErr == nil && bal0 != nil && bal0.Cmp(amount0Max) > 0 {
		amount0Max = bal0
	}
	if bal1, balErr := blockchain.GetTokenBalanceWithClient(client, c1, walletAddr); balErr == nil && bal1 != nil && bal1.Cmp(amount1Max) > 0 {
		amount1Max = bal1
	}
	log.Printf("[Liquidity] V4 increase spend caps: tokenId=%s amount0In=%s amount1In=%s amount0Max=%s amount1Max=%s",
		tokenId.String(), amount0In.String(), amount1In.String(), amount0Max.String(), amount1Max.String())

	// Approve tokens via Permit2 to PositionManager
	if amount0Max.Sign() > 0 {
		if err := s.approveTokenViaPermit2(client, chainID, privateKey, walletAddr, c0, positionManager, amount0Max, opts); err != nil {
			return nil, fmt.Errorf("approve token0 via Permit2 failed: %w", err)
		}
	}
	if amount1Max.Sign() > 0 {
		if err := s.approveTokenViaPermit2(client, chainID, privateKey, walletAddr, c1, positionManager, amount1Max, opts); err != nil {
			return nil, fmt.Errorf("approve token1 via Permit2 failed: %w", err)
		}
	}

	// Compute actual liquidityDelta from token amounts and pool price
	liquidityDelta, liqErr := estimateV4LiquidityForAmounts(sqrtPriceX96, rangeLower, rangeUpper, amount0In, amount1In)
	if liqErr != nil || liquidityDelta == nil || liquidityDelta.Sign() <= 0 {
		return nil, fmt.Errorf("V4 compute liquidityDelta failed: %w", liqErr)
	}
	log.Printf("[Liquidity] V4 INCREASE_LIQUIDITY computed: liquidityDelta=%s sqrtPriceX96=%s tick=%d/%d",
		liquidityDelta.String(), sqrtPriceX96.String(), rangeLower, rangeUpper)

	unlockData, err := buildV4IncreaseUnlockData(tokenId, liquidityDelta, amount0Max, amount1Max, c0, c1)
	if err != nil {
		return nil, err
	}

	deadline := big.NewInt(time.Now().Add(5 * time.Minute).Unix())
	nonce, err := blockchain.GetNonceWithClient(client, walletAddr)
	if err != nil {
		return nil, err
	}
	auth, err := s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
	if err != nil {
		return nil, err
	}

	selectedLiquidityDelta := new(big.Int).Set(liquidityDelta)
	log.Printf("[Liquidity] V4 INCREASE_LIQUIDITY: tokenId=%s amount0=%s amount1=%s amount0Max=%s amount1Max=%s liquidityDelta=%s pm=%s",
		tokenId.String(), amount0In.String(), amount1In.String(), amount0Max.String(), amount1Max.String(), selectedLiquidityDelta.String(), positionManager.Hex())

	tx, err := pm.ModifyLiquidities(auth, unlockData, deadline)
	if err != nil {
		if strings.Contains(err.Error(), maximumAmountExceededSelector) {
			adjustedLiquidityDelta, adjustedUnlockData, fitErr := s.fitV4IncreaseLiquidityDelta(
				pm,
				exec,
				privateKey,
				nonce,
				deadline,
				tokenId,
				amount0Max,
				amount1Max,
				c0,
				c1,
				liquidityDelta,
				opts,
			)
			if fitErr != nil {
				if hint := evmRevertHint(err); hint != "" {
					return nil, fmt.Errorf("V4 modifyLiquidities (INCREASE_LIQUIDITY) failed: %s; fit retry failed: %v; original: %w", hint, fitErr, err)
				}
				return nil, fmt.Errorf("V4 modifyLiquidities (INCREASE_LIQUIDITY) failed: fit retry failed: %v; original: %w", fitErr, err)
			}
			selectedLiquidityDelta = adjustedLiquidityDelta
			unlockData = adjustedUnlockData
			log.Printf("[Liquidity] V4 INCREASE_LIQUIDITY retry: shrink liquidityDelta from %s to %s (tokenId=%s)",
				liquidityDelta.String(), selectedLiquidityDelta.String(), tokenId.String())

			auth, err = s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
			if err != nil {
				return nil, err
			}
			tx, err = pm.ModifyLiquidities(auth, unlockData, deadline)
		}
		if err != nil {
			if hint := evmRevertHint(err); hint != "" {
				return nil, fmt.Errorf("V4 modifyLiquidities (INCREASE_LIQUIDITY) failed: %s: %w", hint, err)
			}
			return nil, fmt.Errorf("V4 modifyLiquidities (INCREASE_LIQUIDITY) failed: %w", err)
		}
	}
	log.Printf("[Liquidity] V4 INCREASE_LIQUIDITY tx sent: %s", tx.Hash().Hex())

	receipt, err := s.waitMined(client, chainID, tx)
	if err != nil {
		return nil, fmt.Errorf("V4 INCREASE_LIQUIDITY tx failed: %w", err)
	}

	// Read updated position liquidity
	posInfo, err = bestEffortReadV4PositionInfo(exec, positionManager, poolManager, task.PoolId, tokenId)
	var currentLiq string
	if err == nil && posInfo != nil && posInfo.Liquidity != nil {
		currentLiq = posInfo.Liquidity.String()
	} else {
		currentLiq = ""
		log.Printf("[Liquidity] V4 INCREASE_LIQUIDITY: failed to read updated position: %v", err)
	}

	// Capture balances after all operations to compute actual stable spent and dust returned.
	stableAfter := tokenBalanceOrZero(exec, stableAddr, walletAddr)
	c0After := tokenBalanceOrZero(exec, c0, walletAddr)
	c1After := tokenBalanceOrZero(exec, c1, walletAddr)

	actualStableSpentWei := spentBalanceDelta(stableBefore, stableAfter)
	dust0Wei := positiveBalanceDelta(c0Before, c0After)
	dust1Wei := positiveBalanceDelta(c1Before, c1After)
	log.Printf("[Liquidity] V4 INCREASE_LIQUIDITY balance delta: tokenId=%s stableSpent=%s dust0=%s dust1=%s",
		tokenId.String(), actualStableSpentWei.String(), dust0Wei.String(), dust1Wei.String())

	return &IncreaseLiquidityResult{
		TxHash:               tx.Hash().Hex(),
		CurrentLiquidity:     currentLiq,
		TickLower:            &rangeLower,
		TickUpper:            &rangeUpper,
		ActualStableSpentWei: actualStableSpentWei,
		ActualStableSpent:    amountToFloat(actualStableSpentWei, cc.StableDecimals),
		Dust0Wei:             dust0Wei,
		Dust1Wei:             dust1Wei,
		Token0:               c0,
		Token1:               c1,
		GasSpentWei:          s.gasCostWeiFromReceipt(client, tx.Hash(), receipt),
	}, nil
}
