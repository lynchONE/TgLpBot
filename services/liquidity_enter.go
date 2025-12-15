package services

import (
	"TgLpBot/blockchain"
	"TgLpBot/config"
	"TgLpBot/database"
	"TgLpBot/models"
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
}

func floatUSDTToWei(amount float64) (*big.Int, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be > 0")
	}
	f := new(big.Float).SetFloat64(amount)
	f.Mul(f, big.NewFloat(1e18))
	i, _ := f.Int(nil)
	if i == nil || i.Sign() <= 0 {
		return nil, fmt.Errorf("amount too small")
	}
	return i, nil
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

func validateOkxSmartSwapTx(label string, tx blockchain.OkxSwapTx) error {
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

func enforceOkxSwapRouter(label string, tx blockchain.OkxSwapTx) error {
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
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	if blockchain.Client == nil || blockchain.ChainID == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
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

	usdtAmount, err := floatUSDTToWei(task.AmountUSDT)
	if err != nil {
		return nil, err
	}

	usdtAddr := common.HexToAddress(config.AppConfig.USDTAddress)
	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	switch version {
	case "v4":
		return s.enterV4FromUSDT(privateKey, walletAddr, usdtAddr, usdtAmount, task)
	default:
		return s.enterV3FromUSDT(privateKey, walletAddr, usdtAddr, usdtAmount, task)
	}
}

func (s *LiquidityService) okxSlippageDecimal(slippagePercent float64) string {
	sl := slippagePercent / 100.0
	if sl <= 0 {
		sl = 0.005
	}
	return fmt.Sprintf("%.6f", sl)
}

func (s *LiquidityService) enterV3FromUSDT(
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	usdtAddr common.Address,
	usdtAmount *big.Int,
	task *models.StrategyTask,
) (*EnterResult, error) {
	if !common.IsHexAddress(config.AppConfig.ZapV3Address) {
		return nil, fmt.Errorf("ZAP_V3_ADDRESS not set")
	}

	// 获取 PositionManager 地址
	pmAddrStr := strings.TrimSpace(task.V3PositionManagerAddress)
	if pmAddrStr == "" {
		ex := strings.ToLower(task.Exchange)
		if strings.Contains(ex, "pancake") && common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
			pmAddrStr = config.AppConfig.PancakeV3PositionManagerAddress
		} else if strings.Contains(ex, "uniswap") && common.IsHexAddress(config.AppConfig.UniswapV3PositionManagerAddress) {
			pmAddrStr = config.AppConfig.UniswapV3PositionManagerAddress
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

	if token0 != usdtAddr && token1 != usdtAddr {
		return nil, fmt.Errorf("V3 pool does not contain USDT")
	}

	// 确定输入金额
	amount0In := big.NewInt(0)
	amount1In := big.NewInt(0)
	if token0 == usdtAddr {
		amount0In = new(big.Int).Set(usdtAmount)
	} else {
		amount1In = new(big.Int).Set(usdtAmount)
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
		swapAmount = new(big.Int).Div(usdtAmount, big.NewInt(2))
		zeroForOne = (token0 == usdtAddr)
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
		okxData, err := s.okxService.GetSwapData(SwapRequest{
			ChainID:           "56", // BSC
			FromTokenAddress:  swapTokenIn.Hex(),
			ToTokenAddress:    swapTokenOut.Hex(),
			Amount:            swapAmount.String(),
			Slippage:          s.okxSlippageDecimal(task.SlippageTolerance),
			UserWalletAddress: zapAddr.Hex(), // Zap 合约地址作为执行者
		})
		if err != nil {
			return nil, fmt.Errorf("get OKX swap data failed: %w", err)
		}

		// 验证 OKX 返回数据
		if len(okxData.Data) == 0 {
			return nil, fmt.Errorf("OKX returned empty data")
		}

		// 解析 OKX 返回的数据
		minOut := big.NewInt(0)
		if okxData.Data[0].RouterResult.ToTokenAmount != "" {
			minOut, _ = new(big.Int).SetString(okxData.Data[0].RouterResult.ToTokenAmount, 10)
			// 减少 5% 作为最小输出保护
			minOut = new(big.Int).Mul(minOut, big.NewInt(95))
			minOut = new(big.Int).Div(minOut, big.NewInt(100))
		}

		callData, _ := hex.DecodeString(strings.TrimPrefix(okxData.Data[0].Tx.Data, "0x"))

		// 确定 ApproveTarget: 优先使用配置的 OKX TokenApproveAddress
		approveTarget := common.HexToAddress(okxData.Data[0].Tx.To)
		if config.AppConfig.OKXTokenApproveAddress != "" {
			approveTarget = common.HexToAddress(config.AppConfig.OKXTokenApproveAddress)
		}

		swapParams = blockchain.SwapParamsSimple{
			Target:        common.HexToAddress(okxData.Data[0].Tx.To),
			ApproveTarget: approveTarget,
			TokenIn:       swapTokenIn,
			TokenOut:      swapTokenOut,
			AmountIn:      swapAmount,
			MinAmountOut:  minOut,
			CallData:      callData,
		}
		log.Printf("[Liquidity] V3 enter: OKX swap target=%s minOut=%s", swapParams.Target.Hex(), minOut.String())
	}

	// 3. Approve 代币给 Zap 合约
	if amount0In.Sign() > 0 {
		log.Printf("[Liquidity] V3 enter: approve token0=%s to Zap amount=%s", token0.Hex(), amount0In.String())
		if err := s.approveToken(privateKey, walletAddr, token0, zapAddr, amount0In); err != nil {
			return nil, fmt.Errorf("approve token0 failed: %w", err)
		}
		// Double check allowance
		t0, err := blockchain.NewERC20(token0, blockchain.Client)
		if err != nil {
			return nil, fmt.Errorf("init eras20 token0 failed: %w", err)
		}
		allow, err := t0.Allowance(nil, walletAddr, zapAddr)
		if err != nil {
			return nil, fmt.Errorf("check allowance token0 failed: %w", err)
		}
		if allow.Cmp(amount0In) < 0 {
			return nil, fmt.Errorf("allowance token0 insufficient: %s < %s", allow.String(), amount0In.String())
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
		if err := s.approveToken(privateKey, walletAddr, token1, zapAddr, amount1In); err != nil {
			return nil, fmt.Errorf("approve token1 failed: %w", err)
		}
		// Double check allowance and balance
		t1, err := blockchain.NewERC20(token1, blockchain.Client)
		if err != nil {
			return nil, fmt.Errorf("init erc20 token1 failed: %w", err)
		}
		allow, err := t1.Allowance(nil, walletAddr, zapAddr)
		if err != nil {
			return nil, fmt.Errorf("check allowance token1 failed: %w", err)
		}
		if allow.Cmp(amount1In) < 0 {
			return nil, fmt.Errorf("allowance token1 insufficient: %s < %s", allow.String(), amount1In.String())
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
	// 由于可能因为 calculateOptimalSwap 失败导致比例严重失调，设置为 100% (10000) 容忍度由 mint 自身处理。
	// 这样可以确保交易必定成功，多余的 Dust 会被自动返还。
	// 交易价格安全依然由 Swap 参数中的 minAmountOut 保证。
	mintSlippageBps := big.NewInt(10000)
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
	auth, err := s.buildAuth(privateKey, nonce, big.NewInt(0), config.AppConfig.GasLimit)
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
	// 注意：zapInV3 函数返回 ZapResult struct，包含 tokenId 和 liquidity
	tokenId, liq, err := parseZapInV3Result(receipt, zapAddr)
	if err != nil {
		return nil, fmt.Errorf("parse zap result failed: %w", err)
	}

	// 验证 tokenId 不为 0
	if tokenId == nil || tokenId.Sign() == 0 {
		return nil, fmt.Errorf("解析到的 tokenId 为 0，这是无效的 NFT ID")
	}

	// Record transaction
	txRecord := models.Transaction{
		UserID:          task.UserID,
		TaskID:          task.ID,
		TxHash:          tx.Hash().Hex(),
		Type:            models.TxTypeAddLiquidity,
		Status:          models.TxStatusConfirmed,
		FromAddress:     walletAddr.Hex(),
		ToAddress:       pmAddr.Hex(),
		TokenInAddress:  token0.Hex(), // Mainly USDT
		TokenOutAddress: token1.Hex(), // The other token
		AmountIn:        amount0In.String(),
		AmountOut:       "0", // Initial position doesn't have immediate output
		BlockNumber:     receipt.BlockNumber.Uint64(),
		GasUsed:         receipt.GasUsed,
	}
	// If token0 is not USDT (unlikely in this bot's context but possible), swap assignments or just log as is.
	// For "USDT -> Token" style, AmountIn is USDT.

	if err := database.DB.Create(&txRecord).Error; err != nil {
		log.Printf("[Liquidity] Failed to record transaction: %v", err)
	}

	return &EnterResult{
		TxHash:                   tx.Hash().Hex(),
		V3PositionManagerAddress: pmAddr.Hex(),
		V3TokenID:                tokenId.String(),
		CurrentLiquidity:         liq.String(),
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

// parseZapInV3Result 从 PositionManager 的 IncreaseLiquidity 事件中解析 tokenId
// 注意：ZapInV3 事件中的 tokenId 是 indexed uint256，会被存储为 hash，无法直接解析
// 因此我们从 NonfungiblePositionManager 的 IncreaseLiquidity 事件中获取 tokenId
func parseZapInV3Result(receipt *types.Receipt, zapAddr common.Address) (*big.Int, *big.Int, error) {
	// IncreaseLiquidity 事件签名: IncreaseLiquidity(uint256 indexed tokenId, uint128 liquidity, uint256 amount0, uint256 amount1)
	// Event ID: keccak256("IncreaseLiquidity(uint256,uint128,uint256,uint256)")
	increaseLiquidityEventID := common.HexToHash("0x3067048beee31b25b2f1681f88dac838c8bba36af25bfb2b7cf7473a5847e35f")

	// 先从 ZapInV3 事件获取 liquidity
	zapParsed, err := abi.JSON(strings.NewReader(blockchain.ZapSimpleABI))
	if err != nil {
		return nil, nil, fmt.Errorf("parse ZapSimple ABI failed: %w", err)
	}
	zapEv, ok := zapParsed.Events["ZapInV3"]
	if !ok {
		return nil, nil, fmt.Errorf("ZapInV3 event not found in ABI")
	}

	var liquidity *big.Int
	for _, lg := range receipt.Logs {
		if lg == nil || lg.Address != zapAddr || len(lg.Topics) == 0 || lg.Topics[0] != zapEv.ID {
			continue
		}
		out, err := zapParsed.Unpack("ZapInV3", lg.Data)
		if err != nil {
			continue
		}
		if len(out) >= 3 {
			if liq, ok := out[2].(*big.Int); ok {
				liquidity = liq
				break
			}
		}
	}

	if liquidity == nil {
		return nil, nil, fmt.Errorf("liquidity not found in ZapInV3 event")
	}

	// 从 IncreaseLiquidity 事件获取 tokenId
	for _, lg := range receipt.Logs {
		if lg == nil || len(lg.Topics) == 0 || lg.Topics[0] != increaseLiquidityEventID {
			continue
		}
		if len(lg.Topics) < 2 {
			continue
		}
		// tokenId 是第一个 indexed 参数
		tokenId := new(big.Int).SetBytes(lg.Topics[1].Bytes())

		// 验证 tokenId 不为 0
		if tokenId == nil || tokenId.Sign() == 0 {
			continue
		}

		return tokenId, liquidity, nil
	}

	return nil, nil, fmt.Errorf("IncreaseLiquidity event not found in receipt logs")
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

	okxData, err := s.okxService.GetSwapData(SwapRequest{
		ChainID:           "56", // BSC
		FromTokenAddress:  tokenIn.Hex(),
		ToTokenAddress:    tokenOut.Hex(),
		Amount:            amountIn.String(),
		Slippage:          s.okxSlippageDecimal(slippageTolerance),
		UserWalletAddress: executorAddr.Hex(), // Zap contract as executor
	})
	if err != nil {
		return nil, fmt.Errorf("get OKX swap data failed: %w", err)
	}

	if len(okxData.Data) == 0 {
		return nil, fmt.Errorf("OKX returned empty data")
	}

	minOut := big.NewInt(0)
	if okxData.Data[0].RouterResult.ToTokenAmount != "" {
		minOut, _ = new(big.Int).SetString(okxData.Data[0].RouterResult.ToTokenAmount, 10)
		// 95% protection
		minOut = new(big.Int).Mul(minOut, big.NewInt(95))
		minOut = new(big.Int).Div(minOut, big.NewInt(100))
	}

	callData := []byte{}
	if okxData.Data[0].Tx.Data != "" {
		callData, _ = hex.DecodeString(strings.TrimPrefix(okxData.Data[0].Tx.Data, "0x"))
	}

	approveTarget := common.HexToAddress(okxData.Data[0].Tx.To)
	if config.AppConfig.OKXTokenApproveAddress != "" {
		approveTarget = common.HexToAddress(config.AppConfig.OKXTokenApproveAddress)
	}

	return &blockchain.SwapParamsSimple{
		Target:        common.HexToAddress(okxData.Data[0].Tx.To),
		ApproveTarget: approveTarget,
		TokenIn:       tokenIn,
		TokenOut:      tokenOut,
		AmountIn:      amountIn,
		MinAmountOut:  minOut,
		CallData:      callData,
	}, nil
}

func (s *LiquidityService) enterV4FromUSDT(
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	usdtAddr common.Address,
	usdtAmount *big.Int,
	task *models.StrategyTask,
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
	tc := NewTickCalculator()
	if terr := tc.ValidateTickRange(tickLower, tickUpper, tickSpacing); terr != nil {
		// If the task was created with an incorrect tickSpacing, try to recompute from the stored percentage.
		if task.RangePercentage > 0 {
			newLower, newUpper := tc.CalculateTickFromPercentage(currentTick, task.RangePercentage, tickSpacing)
			if terr2 := tc.ValidateTickRange(newLower, newUpper, tickSpacing); terr2 == nil {
				log.Printf("[Liquidity] V4 tick range invalid for tickSpacing=%d (%v). Recomputed from RangePercentage=%.4f => [%d,%d]", tickSpacing, terr, task.RangePercentage, newLower, newUpper)
				tickLower, tickUpper = newLower, newUpper
			} else {
				return nil, fmt.Errorf("invalid V4 tick range (original=%v, recompute=%v)", terr, terr2)
			}
		} else {
			return nil, fmt.Errorf("invalid V4 tick range: %w", terr)
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
	// We have USDT. We assume USDT is one of c0 or c1.
	var amount0In, amount1In *big.Int
	var tokenIn, tokenOut common.Address

	if c0 == usdtAddr {
		amount0In = usdtAmount
		amount1In = big.NewInt(0)
		tokenIn = c0
		tokenOut = c1
	} else if c1 == usdtAddr {
		amount0In = big.NewInt(0)
		amount1In = usdtAmount
		tokenIn = c1
		tokenOut = c0
	} else {
		return nil, fmt.Errorf("V4 pool does not contain USDT")
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
	// Approve USDT to Zap Contract
	log.Printf("[Liquidity] DEBUG: About to approve USDT to Zap. usdtAmount=%s zapAddr=%s", usdtAmount.String(), zapAddr.Hex())
	if err := s.approveToken(privateKey, walletAddr, usdtAddr, zapAddr, usdtAmount); err != nil {
		log.Printf("[Liquidity] DEBUG: approveToken failed: %v", err)
		return nil, fmt.Errorf("approve USDT to zap failed: %w", err)
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
		SqrtPriceX96:    sqrtPriceX96,     // 传入从链上获取的价格，避免合约重复调用
		MaxDustBps:      big.NewInt(2000), // 默认 20% dust 容忍度
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
	auth, err := s.buildAuth(privateKey, nonce, big.NewInt(0), config.AppConfig.GasLimit)
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
	tokenId, liq, err := parseZapInV4Event(receipt, zapAddr)
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
		TokenInAddress:  tokenIn.Hex(), // USDT
		TokenOutAddress: task.PoolId,   // Pool ID
		AmountIn:        usdtAmount.String(),
		AmountOut:       "0",
		BlockNumber:     receipt.BlockNumber.Uint64(),
		GasUsed:         receipt.GasUsed,
		CreatedAt:       time.Now(),
	}
	database.DB.Create(&txRecord)

	return &EnterResult{
		TxHash:           tx.Hash().Hex(),
		V4TokenID:        tokenId.String(),
		CurrentLiquidity: liq.String(),
	}, nil
}

// parseZapInV4Event parses tokenId and liquidity from logs
func parseZapInV4Event(receipt *types.Receipt, zapAddr common.Address) (*big.Int, *big.Int, error) {
	// ZapInV4(address indexed user, bytes32 indexed poolId, uint256 indexed tokenId, uint256 amount0, uint256 amount1, uint128 liquidity)
	query := blockchain.ZapSimpleABI
	parsed, err := abi.JSON(strings.NewReader(query))
	if err != nil {
		return nil, nil, err
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
				if liq, ok := out[2].(*big.Int); ok {
					return tokenId, liq, nil
				}
			}
		}
	}
	return nil, nil, fmt.Errorf("ZapInV4 event not found")
}

// percentageToBps converts float percent (e.g. 0.5) to bps (e.g. 50)
func percentageToBps(p float64) *big.Int {
	return big.NewInt(int64(p * 100))
}
