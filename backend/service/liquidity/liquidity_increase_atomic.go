package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func tokenBalanceOrZero(client chainexec.EVMExecutor, token common.Address, walletAddr common.Address) *big.Int {
	if client == nil || token == (common.Address{}) {
		return big.NewInt(0)
	}
	bal, err := blockchain.GetTokenBalanceWithClient(client.Client(), token, walletAddr)
	if err != nil || bal == nil {
		return big.NewInt(0)
	}
	return bal
}

func assetBalanceOrZero(client chainexec.EVMExecutor, token common.Address, walletAddr common.Address) *big.Int {
	if client == nil {
		return big.NewInt(0)
	}
	if token == (common.Address{}) {
		bal, err := blockchain.GetBalanceWithClient(client.Client(), walletAddr)
		if err != nil || bal == nil {
			return big.NewInt(0)
		}
		return bal
	}
	return tokenBalanceOrZero(client, token, walletAddr)
}

func positiveBalanceDelta(before, after *big.Int) *big.Int {
	if before == nil {
		before = big.NewInt(0)
	}
	if after == nil {
		after = big.NewInt(0)
	}
	if after.Cmp(before) <= 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Sub(after, before)
}

func positiveAssetBalanceDelta(before, after, gasSpent *big.Int, token common.Address) *big.Int {
	if token != (common.Address{}) {
		return positiveBalanceDelta(before, after)
	}
	if before == nil {
		before = big.NewInt(0)
	}
	if after == nil {
		after = big.NewInt(0)
	}
	delta := new(big.Int).Sub(after, before)
	if gasSpent != nil && gasSpent.Sign() > 0 {
		delta.Add(delta, gasSpent)
	}
	if delta.Sign() <= 0 {
		return big.NewInt(0)
	}
	return delta
}

func spentBalanceDelta(before, after *big.Int) *big.Int {
	if before == nil {
		before = big.NewInt(0)
	}
	if after == nil {
		after = big.NewInt(0)
	}
	if before.Cmp(after) <= 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Sub(before, after)
}

func normalizeSwapParamsSimple(params blockchain.SwapParamsSimple) blockchain.SwapParamsSimple {
	if params.AmountIn == nil {
		params.AmountIn = big.NewInt(0)
	}
	if params.MinAmountOut == nil {
		params.MinAmountOut = big.NewInt(0)
	}
	if params.CallData == nil {
		params.CallData = []byte{}
	}
	return params
}

func decodeEventBigInt(value interface{}) (*big.Int, error) {
	switch v := value.(type) {
	case *big.Int:
		if v == nil {
			return big.NewInt(0), nil
		}
		return new(big.Int).Set(v), nil
	case uint8:
		return big.NewInt(int64(v)), nil
	case uint16:
		return big.NewInt(int64(v)), nil
	case uint32:
		return big.NewInt(int64(v)), nil
	case uint64:
		return new(big.Int).SetUint64(v), nil
	default:
		return nil, fmt.Errorf("unexpected event value type: %T", value)
	}
}

func parseAtomicIncreaseV3Event(receipt *types.Receipt, zapAddr common.Address) (*big.Int, *big.Int, *big.Int, *big.Int, error) {
	parsed, err := abi.JSON(strings.NewReader(blockchain.AtomicIncreaseZapABI))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	ev, ok := parsed.Events["ZapIncreaseV3"]
	if !ok {
		return nil, nil, nil, nil, fmt.Errorf("ZapIncreaseV3 event not found in ABI")
	}
	for _, lg := range receipt.Logs {
		if lg == nil || lg.Address != zapAddr || len(lg.Topics) < 4 || lg.Topics[0] != ev.ID {
			continue
		}
		out, err := parsed.Unpack("ZapIncreaseV3", lg.Data)
		if err != nil || len(out) < 3 {
			return nil, nil, nil, nil, fmt.Errorf("unpack ZapIncreaseV3 failed: %w", err)
		}
		tokenID := new(big.Int).SetBytes(lg.Topics[3].Bytes())
		amount0, err := decodeEventBigInt(out[0])
		if err != nil {
			return nil, nil, nil, nil, err
		}
		amount1, err := decodeEventBigInt(out[1])
		if err != nil {
			return nil, nil, nil, nil, err
		}
		liquidity, err := decodeEventBigInt(out[2])
		if err != nil {
			return nil, nil, nil, nil, err
		}
		return tokenID, liquidity, amount0, amount1, nil
	}
	return nil, nil, nil, nil, fmt.Errorf("ZapIncreaseV3 event not found")
}

func parseAtomicIncreaseV4Event(receipt *types.Receipt, zapAddr common.Address) (*big.Int, *big.Int, *big.Int, *big.Int, error) {
	parsed, err := abi.JSON(strings.NewReader(blockchain.AtomicIncreaseZapABI))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	ev, ok := parsed.Events["ZapIncreaseV4"]
	if !ok {
		return nil, nil, nil, nil, fmt.Errorf("ZapIncreaseV4 event not found in ABI")
	}
	for _, lg := range receipt.Logs {
		if lg == nil || lg.Address != zapAddr || len(lg.Topics) < 4 || lg.Topics[0] != ev.ID {
			continue
		}
		out, err := parsed.Unpack("ZapIncreaseV4", lg.Data)
		if err != nil || len(out) < 3 {
			return nil, nil, nil, nil, fmt.Errorf("unpack ZapIncreaseV4 failed: %w", err)
		}
		tokenID := new(big.Int).SetBytes(lg.Topics[3].Bytes())
		amount0, err := decodeEventBigInt(out[0])
		if err != nil {
			return nil, nil, nil, nil, err
		}
		amount1, err := decodeEventBigInt(out[1])
		if err != nil {
			return nil, nil, nil, nil, err
		}
		liquidity, err := decodeEventBigInt(out[2])
		if err != nil {
			return nil, nil, nil, nil, err
		}
		return tokenID, liquidity, amount0, amount1, nil
	}
	return nil, nil, nil, nil, fmt.Errorf("ZapIncreaseV4 event not found")
}

func (s *LiquidityService) ensureV3NFTApprovalTo(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	positionManager common.Address,
	spender common.Address,
	tokenId *big.Int,
	opts TxOptions,
) error {
	pm, err := blockchain.NewV3PositionManager(positionManager, exec.Client())
	if err != nil {
		return err
	}
	approved, err := pm.GetApproved(nil, tokenId)
	if err == nil && approved == spender {
		return nil
	}
	nonce, err := blockchain.GetNonceWithClient(exec.Client(), walletAddr)
	if err != nil {
		return err
	}
	auth, err := s.buildAuth(exec.Client(), exec.ChainID(), privateKey, nonce, big.NewInt(0), opts)
	if err != nil {
		return err
	}
	tx, err := pm.Approve(auth, spender, tokenId)
	if err != nil {
		return fmt.Errorf("approve V3 NFT failed: %w", err)
	}
	if _, err := s.waitMined(exec.Client(), exec.ChainID(), tx); err != nil {
		return fmt.Errorf("approve V3 NFT tx failed: %w", err)
	}
	return nil
}

func (s *LiquidityService) ensureV4NFTApprovalTo(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	positionManager common.Address,
	spender common.Address,
	tokenId *big.Int,
	opts TxOptions,
) error {
	pm, err := blockchain.NewV4PositionManager(positionManager, exec.Client())
	if err != nil {
		return err
	}
	approved, err := pm.GetApproved(nil, tokenId)
	if err == nil && approved == spender {
		return nil
	}
	nonce, err := blockchain.GetNonceWithClient(exec.Client(), walletAddr)
	if err != nil {
		return err
	}
	auth, err := s.buildAuth(exec.Client(), exec.ChainID(), privateKey, nonce, big.NewInt(0), opts)
	if err != nil {
		return err
	}
	tx, err := pm.Approve(auth, spender, tokenId)
	if err != nil {
		return fmt.Errorf("approve V4 NFT failed: %w", err)
	}
	if _, err := s.waitMined(exec.Client(), exec.ChainID(), tx); err != nil {
		return fmt.Errorf("approve V4 NFT tx failed: %w", err)
	}
	return nil
}

func buildAtomicIncreaseV3Params(
	poolAddr common.Address,
	pmAddr common.Address,
	tokenID *big.Int,
	fundingToken common.Address,
	fundingAmount *big.Int,
	entrySwap blockchain.SwapParamsSimple,
	rebalanceSwap blockchain.SwapParamsSimple,
) blockchain.ZapIncreaseV3ParamsSimple {
	return blockchain.ZapIncreaseV3ParamsSimple{
		Pool:            poolAddr,
		PositionManager: pmAddr,
		TokenId:         tokenID,
		Funding: blockchain.FundingParamsSimple{
			Token:  fundingToken,
			Amount: fundingAmount,
		},
		EntrySwap:     normalizeSwapParamsSimple(entrySwap),
		RebalanceSwap: normalizeSwapParamsSimple(rebalanceSwap),
	}
}

func buildAtomicIncreaseV4Params(
	poolKey blockchain.PoolKeySimple,
	stateView common.Address,
	positionManager common.Address,
	tokenID *big.Int,
	tickLower int,
	tickUpper int,
	slippageBps *big.Int,
	fundingToken common.Address,
	fundingAmount *big.Int,
	entrySwap blockchain.SwapParamsSimple,
	rebalanceSwap blockchain.SwapParamsSimple,
	sqrtPriceX96 *big.Int,
) blockchain.ZapIncreaseV4ParamsSimple {
	return blockchain.ZapIncreaseV4ParamsSimple{
		PoolKey:         poolKey,
		StateView:       stateView,
		PositionManager: positionManager,
		TokenId:         tokenID,
		TickLower:       big.NewInt(int64(tickLower)),
		TickUpper:       big.NewInt(int64(tickUpper)),
		SlippageBps:     slippageBps,
		Funding: blockchain.FundingParamsSimple{
			Token:  fundingToken,
			Amount: fundingAmount,
		},
		EntrySwap:     normalizeSwapParamsSimple(entrySwap),
		RebalanceSwap: normalizeSwapParamsSimple(rebalanceSwap),
		SqrtPriceX96:  sqrtPriceX96,
	}
}

func (s *LiquidityService) increaseV3LiquidityAtomic(
	exec chainexec.EVMExecutor,
	wallet *models.Wallet,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	stableToken common.Address,
	stableAmount *big.Int,
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
	rangeLower, rangeUpper, _ := pickIncreasePositionRange(task.TickLower, task.TickUpper, onchainTickLower, onchainTickUpper)

	if !common.IsHexAddress(task.PoolId) {
		return nil, fmt.Errorf("invalid V3 pool address: %s", task.PoolId)
	}
	poolAddr := common.HexToAddress(task.PoolId)
	token0, token1, err := blockchain.GetV3PoolTokensWithClient(client, poolAddr)
	if err != nil {
		return nil, fmt.Errorf("read v3 pool tokens failed: %w", err)
	}
	if posInfo != nil {
		if posInfo.Token0 != (common.Address{}) && posInfo.Token1 != (common.Address{}) &&
			(posInfo.Token0 != token0 || posInfo.Token1 != token1) {
			return nil, fmt.Errorf("V3 position token mismatch with task pool")
		}
	}

	zapAddr, err := s.resolveAtomicIncreaseZapAddress(exec, wallet, privateKey, walletAddr, opts)
	if err != nil {
		return nil, err
	}
	zap, err := blockchain.NewAtomicIncreaseZap(zapAddr, client)
	if err != nil {
		return nil, err
	}

	entrySwap := blockchain.SwapParamsSimple{}
	rebalanceSwap := blockchain.SwapParamsSimple{}
	amount0Preview := big.NewInt(0)
	amount1Preview := big.NewInt(0)

	plan, err := s.planEntryToken(task)
	if err != nil {
		return nil, err
	}
	entryToken := stableToken
	entryAmount := new(big.Int).Set(stableAmount)
	if plan.RequiresSwap {
		entryToken = plan.EntryToken
		entrySlippage := resolveEntrySwapSlippage(
			task.SlippageTolerance,
			opts.EntrySwapSlippageOverride,
			stableToken,
			entryToken,
			cc.StableSymbol,
			plan.EntrySymbol,
			cc,
		)
		swapParams, out, err := s.prepareOKXSwapParams(cc, zapAddr, stableToken, entryToken, stableAmount, entrySlippage)
		if err != nil {
			return nil, fmt.Errorf("prepare entry swap failed: %w", err)
		}
		if swapParams == nil || out == nil || out.Sign() <= 0 {
			return nil, fmt.Errorf("entry swap output is zero")
		}
		entrySwap = *swapParams
		entryAmount = out
	}

	if entryToken == token0 {
		amount0Preview = cloneBig(entryAmount)
	} else if entryToken == token1 {
		amount1Preview = cloneBig(entryAmount)
	} else {
		return nil, fmt.Errorf("entry token is not in V3 pool")
	}

	zeroForOne, swapAmount, err := s.calculateOptimalSwapLocal(client, poolAddr, rangeLower, rangeUpper, amount0Preview, amount1Preview)
	if err != nil {
		return nil, fmt.Errorf("calculate optimal swap failed: %w", err)
	}
	if swapAmount != nil && swapAmount.Sign() > 0 {
		var swapTokenIn, swapTokenOut common.Address
		if zeroForOne {
			swapTokenIn = token0
			swapTokenOut = token1
		} else {
			swapTokenIn = token1
			swapTokenOut = token0
		}
		swapParams, out, err := s.prepareOKXSwapParams(cc, zapAddr, swapTokenIn, swapTokenOut, swapAmount, task.SlippageTolerance)
		if err != nil {
			return nil, fmt.Errorf("prepare rebalance swap failed: %w", err)
		}
		if swapParams == nil || out == nil || out.Sign() <= 0 {
			return nil, fmt.Errorf("rebalance swap output is zero")
		}
		rebalanceSwap = *swapParams
		if zeroForOne {
			amount0Preview = new(big.Int).Sub(amount0Preview, swapAmount)
			amount1Preview = new(big.Int).Add(amount1Preview, out)
		} else {
			amount1Preview = new(big.Int).Sub(amount1Preview, swapAmount)
			amount0Preview = new(big.Int).Add(amount0Preview, out)
		}
	}

	if err := s.approveToken(client, chainID, privateKey, walletAddr, stableToken, zapAddr, stableAmount, opts); err != nil {
		return nil, fmt.Errorf("approve funding token to atomic zap failed: %w", err)
	}
	if err := s.ensureV3NFTApprovalTo(exec, privateKey, walletAddr, pmAddr, zapAddr, tokenId, opts); err != nil {
		return nil, err
	}

	stableBefore := tokenBalanceOrZero(exec, stableToken, walletAddr)
	token0Before := tokenBalanceOrZero(exec, token0, walletAddr)
	token1Before := tokenBalanceOrZero(exec, token1, walletAddr)

	params := buildAtomicIncreaseV3Params(poolAddr, pmAddr, tokenId, stableToken, stableAmount, entrySwap, rebalanceSwap)
	if _, err := zap.SimulateZapIncreaseV3(&bind.CallOpts{From: walletAddr}, params); err != nil {
		if hint := evmRevertHint(err); hint != "" {
			return nil, fmt.Errorf("simulate atomic V3 increase failed: %s: %w", hint, err)
		}
		return nil, fmt.Errorf("simulate atomic V3 increase failed: %w", err)
	}

	nonce, err := blockchain.GetNonceWithClient(client, walletAddr)
	if err != nil {
		return nil, err
	}
	auth, err := s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
	if err != nil {
		return nil, err
	}
	tuneZapTxGasLimit("V3 increase atomic zap", auth, func(o *bind.TransactOpts) (*types.Transaction, error) {
		return zap.ZapIncreaseV3(o, params)
	})
	tx, err := zap.ZapIncreaseV3(auth, params)
	if err != nil {
		if hint := evmRevertHint(err); hint != "" {
			return nil, fmt.Errorf("atomic V3 increase failed: %s: %w", hint, err)
		}
		return nil, fmt.Errorf("atomic V3 increase failed: %w", err)
	}
	receipt, err := s.waitMined(client, chainID, tx)
	if err != nil {
		return nil, fmt.Errorf("atomic V3 increase tx failed: %w", err)
	}

	addedLiquidity := ""
	if _, liq, _, _, perr := parseAtomicIncreaseV3Event(receipt, zapAddr); perr == nil && liq != nil {
		addedLiquidity = liq.String()
	}

	posInfo, err = pm.Positions(nil, tokenId)
	currentLiq := ""
	if err == nil && posInfo != nil && posInfo.Liquidity != nil {
		currentLiq = posInfo.Liquidity.String()
	}

	stableAfter := tokenBalanceOrZero(exec, stableToken, walletAddr)
	token0After := tokenBalanceOrZero(exec, token0, walletAddr)
	token1After := tokenBalanceOrZero(exec, token1, walletAddr)

	actualStableSpentWei := spentBalanceDelta(stableBefore, stableAfter)
	dust0Wei := positiveBalanceDelta(token0Before, token0After)
	dust1Wei := positiveBalanceDelta(token1Before, token1After)

	return &IncreaseLiquidityResult{
		TxHash:               tx.Hash().Hex(),
		AddedLiquidity:       addedLiquidity,
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

func (s *LiquidityService) increaseV4LiquidityAtomic(
	exec chainexec.EVMExecutor,
	wallet *models.Wallet,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	stableToken common.Address,
	stableAmount *big.Int,
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
	poolManager := common.Address{}
	if common.IsHexAddress(cc.UniswapV4PoolManagerAddress) {
		poolManager = common.HexToAddress(cc.UniswapV4PoolManagerAddress)
	}
	posInfo, err := bestEffortReadV4PositionInfo(exec, positionManager, poolManager, task.PoolId, tokenId)
	if err != nil {
		log.Printf("[Liquidity] Warning: read V4 position failed, fallback to task snapshot: tokenId=%s err=%v", tokenId.String(), err)
		posInfo = nil
	}

	c0 := common.HexToAddress(task.Token0Address)
	c1 := common.HexToAddress(task.Token1Address)
	if posInfo != nil {
		if taskTokenAddressesReady("v4", posInfo.Token0, posInfo.Token1, true, true) {
			c0 = posInfo.Token0
			c1 = posInfo.Token1
		}
	}
	if !taskTokenAddressesReady("v4", c0, c1, true, true) {
		return nil, fmt.Errorf("V4 token addresses missing")
	}
	onchainTickLower, onchainTickUpper := 0, 0
	if posInfo != nil {
		onchainTickLower = posInfo.TickLower
		onchainTickUpper = posInfo.TickUpper
	}
	rangeLower, rangeUpper, _ := pickIncreasePositionRange(task.TickLower, task.TickUpper, onchainTickLower, onchainTickUpper)

	if !common.IsHexAddress(cc.UniswapV4StateViewAddress) || !common.IsHexAddress(cc.UniswapV4PoolManagerAddress) {
		return nil, fmt.Errorf("V4 StateView or PoolManager address not configured")
	}
	stateViewAddr := common.HexToAddress(cc.UniswapV4StateViewAddress)
	sqrtPriceX96, currentTick, slotErr := blockchain.GetUniswapV4PoolSlot0ViaStateView(stateViewAddr, poolManager, task.PoolId)
	if slotErr != nil {
		return nil, fmt.Errorf("read V4 pool sqrtPriceX96 failed: %w", slotErr)
	}

	zapAddr, err := s.resolveAtomicIncreaseZapAddress(exec, wallet, privateKey, walletAddr, opts)
	if err != nil {
		return nil, err
	}
	zap, err := blockchain.NewAtomicIncreaseZap(zapAddr, client)
	if err != nil {
		return nil, err
	}

	entrySwap := blockchain.SwapParamsSimple{}
	rebalanceSwap := blockchain.SwapParamsSimple{}
	amount0Preview := big.NewInt(0)
	amount1Preview := big.NewInt(0)

	plan, err := s.planEntryToken(task)
	if err != nil {
		return nil, err
	}
	entryToken := stableToken
	entryAmount := new(big.Int).Set(stableAmount)
	if plan.RequiresSwap {
		entryToken = plan.EntryToken
		entrySlippage := resolveEntrySwapSlippage(
			task.SlippageTolerance,
			opts.EntrySwapSlippageOverride,
			stableToken,
			entryToken,
			cc.StableSymbol,
			plan.EntrySymbol,
			cc,
		)
		swapParams, out, err := s.prepareOKXSwapParams(cc, zapAddr, stableToken, entryToken, stableAmount, entrySlippage)
		if err != nil {
			return nil, fmt.Errorf("prepare entry swap failed: %w", err)
		}
		if swapParams == nil || out == nil || out.Sign() <= 0 {
			return nil, fmt.Errorf("entry swap output is zero")
		}
		entrySwap = *swapParams
		entryAmount = out
	}

	if v4CurrencyMatchesFundingToken(cc, c0, entryToken) {
		amount0Preview = cloneBig(entryAmount)
	} else if v4CurrencyMatchesFundingToken(cc, c1, entryToken) {
		amount1Preview = cloneBig(entryAmount)
	} else {
		return nil, fmt.Errorf("entry token is not in V4 pool")
	}

	zeroForOne, swapAmount, _ := s.calculateOptimalSwapPure(sqrtPriceX96, currentTick, rangeLower, rangeUpper, amount0Preview, amount1Preview)
	if swapAmount != nil && swapAmount.Sign() > 0 {
		var swapFrom, swapTo common.Address
		if zeroForOne {
			swapFrom, swapTo = c0, c1
		} else {
			swapFrom, swapTo = c1, c0
		}
		swapFromFunding, err := v4CurrencyFundingToken(cc, swapFrom)
		if err != nil {
			return nil, err
		}
		swapToFunding, err := v4CurrencyFundingToken(cc, swapTo)
		if err != nil {
			return nil, err
		}
		swapParams, out, err := s.prepareOKXSwapParams(cc, zapAddr, swapFromFunding, swapToFunding, swapAmount, task.SlippageTolerance)
		if err != nil {
			return nil, fmt.Errorf("prepare rebalance swap failed: %w", err)
		}
		if swapParams == nil || out == nil || out.Sign() <= 0 {
			return nil, fmt.Errorf("rebalance swap output is zero")
		}
		rebalanceSwap = *swapParams
		if zeroForOne {
			amount0Preview = new(big.Int).Sub(amount0Preview, swapAmount)
			amount1Preview = new(big.Int).Add(amount1Preview, out)
		} else {
			amount1Preview = new(big.Int).Sub(amount1Preview, swapAmount)
			amount0Preview = new(big.Int).Add(amount0Preview, out)
		}
	}

	if err := s.approveToken(client, chainID, privateKey, walletAddr, stableToken, zapAddr, stableAmount, opts); err != nil {
		return nil, fmt.Errorf("approve funding token to atomic zap failed: %w", err)
	}
	if err := s.ensureV4NFTApprovalTo(exec, privateKey, walletAddr, positionManager, zapAddr, tokenId, opts); err != nil {
		return nil, err
	}

	stableBefore := tokenBalanceOrZero(exec, stableToken, walletAddr)
	token0Before := assetBalanceOrZero(exec, c0, walletAddr)
	token1Before := assetBalanceOrZero(exec, c1, walletAddr)

	poolKeySimple := blockchain.PoolKeySimple{
		Currency0:   c0,
		Currency1:   c1,
		Fee:         big.NewInt(int64(task.Fee)),
		TickSpacing: big.NewInt(int64(task.TickSpacing)),
		Hooks:       common.HexToAddress(task.HooksAddress),
	}
	params := buildAtomicIncreaseV4Params(
		poolKeySimple,
		stateViewAddr,
		positionManager,
		tokenId,
		rangeLower,
		rangeUpper,
		V4PriceMoveToleranceBps(task.SlippageTolerance),
		stableToken,
		stableAmount,
		entrySwap,
		rebalanceSwap,
		sqrtPriceX96,
	)
	if _, err := zap.SimulateZapIncreaseV4(&bind.CallOpts{From: walletAddr}, params); err != nil {
		if hint := evmRevertHint(err); hint != "" {
			return nil, fmt.Errorf("simulate atomic V4 increase failed: %s: %w", hint, err)
		}
		return nil, fmt.Errorf("simulate atomic V4 increase failed: %w", err)
	}

	nonce, err := blockchain.GetNonceWithClient(client, walletAddr)
	if err != nil {
		return nil, err
	}
	auth, err := s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
	if err != nil {
		return nil, err
	}
	tuneZapTxGasLimit("V4 increase atomic zap", auth, func(o *bind.TransactOpts) (*types.Transaction, error) {
		return zap.ZapIncreaseV4(o, params)
	})
	tx, err := zap.ZapIncreaseV4(auth, params)
	if err != nil {
		if hint := evmRevertHint(err); hint != "" {
			return nil, fmt.Errorf("atomic V4 increase failed: %s: %w", hint, err)
		}
		return nil, fmt.Errorf("atomic V4 increase failed: %w", err)
	}
	receipt, err := s.waitMined(client, chainID, tx)
	if err != nil {
		return nil, fmt.Errorf("atomic V4 increase tx failed: %w", err)
	}

	addedLiquidity := ""
	if _, liq, _, _, perr := parseAtomicIncreaseV4Event(receipt, zapAddr); perr == nil && liq != nil {
		addedLiquidity = liq.String()
	}

	posInfo, err = bestEffortReadV4PositionInfo(exec, positionManager, poolManager, task.PoolId, tokenId)
	currentLiq := ""
	if err == nil && posInfo != nil && posInfo.Liquidity != nil {
		currentLiq = posInfo.Liquidity.String()
	}

	gasSpentWei := s.gasCostWeiFromReceipt(client, tx.Hash(), receipt)
	stableAfter := tokenBalanceOrZero(exec, stableToken, walletAddr)
	token0After := assetBalanceOrZero(exec, c0, walletAddr)
	token1After := assetBalanceOrZero(exec, c1, walletAddr)
	actualStableSpentWei := spentBalanceDelta(stableBefore, stableAfter)
	dust0Wei := positiveAssetBalanceDelta(token0Before, token0After, gasSpentWei, c0)
	dust1Wei := positiveAssetBalanceDelta(token1Before, token1After, gasSpentWei, c1)

	return &IncreaseLiquidityResult{
		TxHash:               tx.Hash().Hex(),
		AddedLiquidity:       addedLiquidity,
		CurrentLiquidity:     currentLiq,
		TickLower:            &rangeLower,
		TickUpper:            &rangeUpper,
		ActualStableSpentWei: actualStableSpentWei,
		ActualStableSpent:    amountToFloat(actualStableSpentWei, cc.StableDecimals),
		Dust0Wei:             dust0Wei,
		Dust1Wei:             dust1Wei,
		Token0:               c0,
		Token1:               c1,
		GasSpentWei:          gasSpentWei,
	}, nil
}
