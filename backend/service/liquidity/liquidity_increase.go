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
	TxHash           string
	AddedLiquidity   string
	CurrentLiquidity string
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

	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	switch version {
	case "v4":
		return s.increaseV4Liquidity(exec, wallet, privateKey, walletAddr, entryToken, entryAmount, task, TxOptions{})
	default:
		return s.increaseV3Liquidity(exec, wallet, privateKey, walletAddr, entryToken, entryAmount, task, TxOptions{})
	}
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

	if token0 != tokenIn && token1 != tokenIn {
		return nil, fmt.Errorf("V3 pool does not contain entry token")
	}

	// Determine input amounts
	amount0In := big.NewInt(0)
	amount1In := big.NewInt(0)
	if token0 == tokenIn {
		amount0In = new(big.Int).Set(amountIn)
	} else {
		amount1In = new(big.Int).Set(amountIn)
	}

	// Calculate optimal swap to split into token0/token1
	zeroForOne, swapAmount, err := s.calculateOptimalSwapLocal(client, poolAddr, task.TickLower, task.TickUpper, amount0In, amount1In)
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

	// Call increaseLiquidity on the PositionManager
	pm, err := blockchain.NewV3PositionManager(pmAddr, client)
	if err != nil {
		return nil, fmt.Errorf("init V3 PM failed: %w", err)
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
	_ = receipt

	// Read updated position liquidity
	posInfo, err := pm.Positions(nil, tokenId)
	var currentLiq string
	if err == nil && posInfo != nil && posInfo.Liquidity != nil {
		currentLiq = posInfo.Liquidity.String()
	} else {
		currentLiq = "0"
		log.Printf("[Liquidity] V3 increaseLiquidity: failed to read updated position: %v", err)
	}

	return &IncreaseLiquidityResult{
		TxHash:           tx.Hash().Hex(),
		CurrentLiquidity: currentLiq,
	}, nil
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

	// Resolve token addresses
	c0 := common.HexToAddress(task.Token0Address)
	c1 := common.HexToAddress(task.Token1Address)
	if bytes.Compare(c0.Bytes(), c1.Bytes()) >= 0 {
		return nil, fmt.Errorf("unexpected V4 token ordering")
	}

	if c0 != tokenIn && c1 != tokenIn {
		return nil, fmt.Errorf("V4 pool does not contain entry token")
	}

	// Determine input amounts and optimal swap
	amount0In := big.NewInt(0)
	amount1In := big.NewInt(0)
	var tokenOut common.Address
	if c0 == tokenIn {
		amount0In = new(big.Int).Set(amountIn)
		tokenOut = c1
	} else {
		amount1In = new(big.Int).Set(amountIn)
		tokenOut = c0
	}

	// For V4, use half split as swap (simple approach since we don't have V4 calculateOptimalSwap on-chain)
	swapAmount := new(big.Int).Div(amountIn, big.NewInt(2))
	if swapAmount.Sign() > 0 {
		swapped, swapErr := s.swapExactInViaOKX(exec, privateKey, walletAddr, tokenIn, tokenOut, swapAmount, task.SlippageTolerance)
		if swapErr != nil {
			return nil, fmt.Errorf("V4 optimal swap failed: %w", swapErr)
		}
		time.Sleep(500 * time.Millisecond)
		if swapped != nil && swapped.Sign() > 0 {
			if c0 == tokenIn {
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

	// Approve tokens via Permit2 to PositionManager
	if amount0In.Sign() > 0 {
		if err := s.approveTokenViaPermit2(client, chainID, privateKey, walletAddr, c0, positionManager, amount0In, opts); err != nil {
			return nil, fmt.Errorf("approve token0 via Permit2 failed: %w", err)
		}
	}
	if amount1In.Sign() > 0 {
		if err := s.approveTokenViaPermit2(client, chainID, privateKey, walletAddr, c1, positionManager, amount1In, opts); err != nil {
			return nil, fmt.Errorf("approve token1 via Permit2 failed: %w", err)
		}
	}

	// Build INCREASE_LIQUIDITY + SETTLE_PAIR actions via ABI encoding
	// INCREASE_LIQUIDITY (0x00): (tokenId, liquidityDelta, amount0Max, amount1Max, hookData)
	// Use type(uint128).max for liquidityDelta to add as much as possible given the token amounts.
	maxUint128 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))

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
	increaseLiqParams, err := increaseArgs.Pack(tokenId, maxUint128, amount0In, amount1In, []byte{})
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

	pm, err := blockchain.NewV4PositionManager(positionManager, client)
	if err != nil {
		return nil, fmt.Errorf("init V4 PM failed: %w", err)
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

	log.Printf("[Liquidity] V4 INCREASE_LIQUIDITY: tokenId=%s amount0=%s amount1=%s pm=%s",
		tokenId.String(), amount0In.String(), amount1In.String(), positionManager.Hex())

	tx, err := pm.ModifyLiquidities(auth, unlockData, deadline)
	if err != nil {
		return nil, fmt.Errorf("V4 modifyLiquidities (INCREASE_LIQUIDITY) failed: %w", err)
	}
	log.Printf("[Liquidity] V4 INCREASE_LIQUIDITY tx sent: %s", tx.Hash().Hex())

	receipt, err := s.waitMined(client, chainID, tx)
	if err != nil {
		return nil, fmt.Errorf("V4 INCREASE_LIQUIDITY tx failed: %w", err)
	}
	_ = receipt

	// Read updated position liquidity
	posInfo, err := pm.Positions(nil, tokenId)
	var currentLiq string
	if err == nil && posInfo != nil && posInfo.Liquidity != nil {
		currentLiq = posInfo.Liquidity.String()
	} else {
		currentLiq = "0"
		log.Printf("[Liquidity] V4 INCREASE_LIQUIDITY: failed to read updated position: %v", err)
	}

	return &IncreaseLiquidityResult{
		TxHash:           tx.Hash().Hex(),
		CurrentLiquidity: currentLiq,
	}, nil
}
