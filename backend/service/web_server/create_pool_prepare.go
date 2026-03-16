package web_server

import (
	"fmt"
	"math/big"
	"strings"

	"TgLpBot/base/blockchain"
	"TgLpBot/service/liquidity"
	poolSvc "TgLpBot/service/pool"

	"github.com/ethereum/go-ethereum/common"
)

func (s *Server) prepareCreatePoolPlan(req *createPoolRequest, requireAmounts bool) (*createPoolPlan, *createPoolPreviewResponse, error) {
	ctx, err := s.resolveCreatePoolContext(req)
	if err != nil {
		return nil, nil, err
	}
	if ctx.client == nil || ctx.chainID == nil || ctx.chainID.Sign() <= 0 {
		return nil, nil, fmt.Errorf("blockchain client not initialized")
	}

	protocol := normalizeCreatePoolProtocol(req.Protocol)
	if protocol == "" {
		return nil, nil, fmt.Errorf("unsupported protocol")
	}
	mode := normalizeCreatePoolMode(req.Mode)
	if mode == "" {
		return nil, nil, fmt.Errorf("unsupported mode")
	}
	rangeMode := normalizeCreatePoolRangeMode(req.RangeMode)
	if rangeMode == "" {
		return nil, nil, fmt.Errorf("unsupported range mode")
	}
	amountMode := normalizeCreatePoolAmountMode(req.AmountMode)
	if amountMode == "" {
		return nil, nil, fmt.Errorf("unsupported amount mode")
	}
	slippagePct := 0.5
	if req.Slippage != nil {
		if *req.Slippage < 0 || *req.Slippage > 100 {
			return nil, nil, fmt.Errorf("invalid slippage_tolerance")
		}
		slippagePct = *req.Slippage
	}

	tokenA, err := s.loadCreatePoolTokenMeta(ctx, req.TokenAAddress)
	if err != nil {
		return nil, nil, err
	}
	tokenB, err := s.loadCreatePoolTokenMeta(ctx, req.TokenBAddress)
	if err != nil {
		return nil, nil, err
	}
	if tokenA.Address == tokenB.Address {
		return nil, nil, fmt.Errorf("token_a_address and token_b_address must be different")
	}

	token0 := tokenA
	token1 := tokenB
	tokenAIsToken0 := true
	if bytesCompare(tokenA.Address.Bytes(), tokenB.Address.Bytes()) > 0 {
		token0 = tokenB
		token1 = tokenA
		tokenAIsToken0 = false
	}

	plan := &createPoolPlan{
		ctx:            ctx,
		protocol:       protocol,
		mode:           mode,
		rangeMode:      rangeMode,
		amountMode:     amountMode,
		slippagePct:    slippagePct,
		tokenA:         tokenA,
		tokenB:         tokenB,
		token0:         token0,
		token1:         token1,
		tokenAIsToken0: tokenAIsToken0,
		feeTier:        req.FeeTier,
		amountAInput:   strings.TrimSpace(req.AmountA),
		amountBInput:   strings.TrimSpace(req.AmountB),
		hooks:          common.Address{},
	}

	if plan.feeTier == 0 {
		return nil, nil, fmt.Errorf("missing fee_tier")
	}
	if !createPoolSupportsFeeTier(plan.protocol, plan.feeTier) {
		return nil, nil, fmt.Errorf("fee tier %d is not supported by selected protocol", plan.feeTier)
	}
	if req.TickSpacing < 0 {
		return nil, nil, fmt.Errorf("tick_spacing must be greater than 0")
	}

	switch plan.protocol {
	case createPoolProtocolUniV3, createPoolProtocolPcsV3:
		deploymentName := "uniswap v3"
		if plan.protocol == createPoolProtocolPcsV3 {
			deploymentName = "pancakeswap v3"
		}
		for _, dep := range ctx.cc.V3Deployments {
			if strings.EqualFold(strings.TrimSpace(dep.Name), deploymentName) {
				if common.IsHexAddress(dep.FactoryAddress) {
					plan.factory = common.HexToAddress(dep.FactoryAddress)
				}
				if common.IsHexAddress(dep.PositionManagerAddress) {
					plan.positionManager = common.HexToAddress(dep.PositionManagerAddress)
				}
				break
			}
		}
		if plan.factory == (common.Address{}) || plan.positionManager == (common.Address{}) {
			return nil, nil, fmt.Errorf("target v3 deployment is not configured")
		}
		tickSpacing, err := blockchain.GetV3FeeAmountTickSpacingWithClient(ctx.client, plan.factory, plan.feeTier)
		if err != nil || tickSpacing <= 0 {
			return nil, nil, fmt.Errorf("fee tier %d is not supported by target protocol", plan.feeTier)
		}
		if req.TickSpacing > 0 && req.TickSpacing != tickSpacing {
			return nil, nil, fmt.Errorf("tick_spacing %d does not match the selected v3 fee tier", req.TickSpacing)
		}
		plan.tickSpacing = tickSpacing
		poolAddr, err := blockchain.GetV3PoolFromFactoryWithClient(ctx.client, plan.factory, plan.token0.Address, plan.token1.Address, plan.feeTier)
		if err == nil && poolAddr != (common.Address{}) {
			plan.poolExists = true
			plan.existingPoolAddress = poolAddr
		}

	case createPoolProtocolUniV4:
		if !common.IsHexAddress(ctx.cc.UniswapV4PositionManagerAddress) {
			return nil, nil, fmt.Errorf("UNISWAP_V4_POSITION_MANAGER_ADDRESS not configured")
		}
		if !common.IsHexAddress(ctx.cc.UniswapV4PoolManagerAddress) {
			return nil, nil, fmt.Errorf("UNISWAP_V4_POOL_MANAGER_ADDRESS not configured")
		}
		if !common.IsHexAddress(ctx.cc.UniswapV4StateViewAddress) {
			return nil, nil, fmt.Errorf("UNISWAP_V4_STATE_VIEW_ADDRESS not configured")
		}
		plan.tickSpacing = req.TickSpacing
		if plan.tickSpacing <= 0 {
			if tickSpacing, err := poolSvc.StandardTickSpacingFromFee(plan.feeTier); err == nil && tickSpacing > 0 {
				plan.tickSpacing = tickSpacing
				plan.warnings = append(plan.warnings, fmt.Sprintf("未显式提供 tick_spacing，已按常见费率档位默认使用 %d", tickSpacing))
			}
		}
		if plan.tickSpacing <= 0 {
			return nil, nil, fmt.Errorf("tick_spacing is required for uniswap v4 custom fee tiers")
		}
		plan.positionManager = common.HexToAddress(ctx.cc.UniswapV4PositionManagerAddress)
		plan.predictedPoolID, err = blockchain.ComputeUniswapV4PoolID(plan.token0.Address, plan.token1.Address, plan.feeTier, plan.tickSpacing, plan.hooks)
		if err != nil {
			return nil, nil, fmt.Errorf("compute predicted pool id failed: %w", err)
		}
		stateView := common.HexToAddress(ctx.cc.UniswapV4StateViewAddress)
		poolManager := common.HexToAddress(ctx.cc.UniswapV4PoolManagerAddress)
		if sqrt, _, err := blockchain.GetUniswapV4PoolSlot0ViaStateView(stateView, poolManager, plan.predictedPoolID.Hex()); err == nil && sqrt != nil && sqrt.Sign() > 0 {
			plan.poolExists = true
		} else if err != nil && !containsCreatePoolNotInitialized(err) {
			plan.warnings = append(plan.warnings, fmt.Sprintf("V4 池子存在性暂未确认，执行前会再次校验: %v", err))
		}

	default:
		return nil, nil, fmt.Errorf("unsupported protocol")
	}

	initialPrice, initialPriceText, priceSource, priceWarnings, err := s.resolveCreatePoolPrice(plan, req.InitialPrice)
	if err != nil {
		return nil, nil, err
	}
	plan.warnings = append(plan.warnings, priceWarnings...)
	plan.initialPriceAB = initialPrice
	plan.initialPriceABText = initialPriceText
	plan.initialPriceSource = priceSource

	if plan.initialPriceAB != nil {
		canonicalPrice, err := poolSvc.HumanPriceFromBaseQuote(plan.initialPriceAB, plan.tokenAIsToken0)
		if err != nil {
			return nil, nil, err
		}
		plan.canonicalPriceToken1Per0 = canonicalPrice
		plan.canonicalPriceText = formatCreatePoolDecimal(canonicalPrice, 12)
		plan.sqrtPriceX96, err = poolSvc.SqrtPriceX96FromHumanPrice(canonicalPrice, plan.token0.Decimals, plan.token1.Decimals)
		if err != nil {
			return nil, nil, fmt.Errorf("compute sqrtPriceX96 failed: %w", err)
		}
		canonicalFloat, _ := canonicalPrice.Float64()
		plan.currentTick, err = poolSvc.TickFromHumanPrice(canonicalFloat, plan.token0.Decimals, plan.token1.Decimals)
		if err != nil {
			return nil, nil, fmt.Errorf("compute current tick failed: %w", err)
		}
	}

	switch plan.rangeMode {
	case createPoolRangeModeFull:
		plan.tickLower, plan.tickUpper, err = poolSvc.FullRangeTicks(plan.tickSpacing)
		if err != nil {
			return nil, nil, err
		}
	case createPoolRangeModeCustom:
		if strings.TrimSpace(req.MinPrice) == "" || strings.TrimSpace(req.MaxPrice) == "" {
			return nil, nil, fmt.Errorf("custom_range requires min_price and max_price")
		}
		minCanonical, _, err := createPoolCanonicalPriceFromAB(req.MinPrice, plan.tokenAIsToken0)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid min_price: %w", err)
		}
		maxCanonical, _, err := createPoolCanonicalPriceFromAB(req.MaxPrice, plan.tokenAIsToken0)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid max_price: %w", err)
		}
		if minCanonical.Cmp(maxCanonical) == 0 {
			return nil, nil, fmt.Errorf("min_price and max_price must be different")
		}
		lowCanonical := minCanonical
		highCanonical := maxCanonical
		if lowCanonical.Cmp(highCanonical) > 0 {
			lowCanonical, highCanonical = highCanonical, lowCanonical
		}
		lowFloat, _ := lowCanonical.Float64()
		highFloat, _ := highCanonical.Float64()
		minTick, err := poolSvc.TickFromHumanPrice(lowFloat, plan.token0.Decimals, plan.token1.Decimals)
		if err != nil {
			return nil, nil, fmt.Errorf("compute min tick failed: %w", err)
		}
		maxTick, err := poolSvc.TickFromHumanPrice(highFloat, plan.token0.Decimals, plan.token1.Decimals)
		if err != nil {
			return nil, nil, fmt.Errorf("compute max tick failed: %w", err)
		}
		plan.tickLower, plan.tickUpper, err = createPoolAlignRangeTicks(minTick, maxTick, plan.tickSpacing)
		if err != nil {
			return nil, nil, err
		}
		if plan.tokenAIsToken0 {
			plan.minPriceABText = formatCreatePoolDecimal(new(big.Float).Set(lowCanonical), 12)
			plan.maxPriceABText = formatCreatePoolDecimal(new(big.Float).Set(highCanonical), 12)
		} else {
			minAB, err := createPoolPriceABFromCanonical(highCanonical, false)
			if err != nil {
				return nil, nil, fmt.Errorf("format min_price failed: %w", err)
			}
			maxAB, err := createPoolPriceABFromCanonical(lowCanonical, false)
			if err != nil {
				return nil, nil, fmt.Errorf("format max_price failed: %w", err)
			}
			plan.minPriceABText = minAB
			plan.maxPriceABText = maxAB
		}
	default:
		return nil, nil, fmt.Errorf("unsupported range mode")
	}

	if plan.amountAInput != "" {
		plan.amountAUnits, err = poolSvc.DecimalToUnits(plan.amountAInput, plan.tokenA.Decimals)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid amount_a: %w", err)
		}
	}
	if plan.amountBInput != "" {
		plan.amountBUnits, err = poolSvc.DecimalToUnits(plan.amountBInput, plan.tokenB.Decimals)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid amount_b: %w", err)
		}
	}
	if plan.amountAUnits != nil && plan.tokenA.WalletBalance != nil && plan.tokenA.WalletBalance.Cmp(plan.amountAUnits) < 0 {
		plan.warnings = append(plan.warnings, fmt.Sprintf("Token A 余额不足: 当前 %s %s", plan.tokenA.WalletBalanceStr, plan.tokenA.Symbol))
	}
	if plan.amountBUnits != nil && plan.tokenB.WalletBalance != nil && plan.tokenB.WalletBalance.Cmp(plan.amountBUnits) < 0 {
		plan.warnings = append(plan.warnings, fmt.Sprintf("Token B 余额不足: 当前 %s %s", plan.tokenB.WalletBalanceStr, plan.tokenB.Symbol))
	}

	if plan.mode == createPoolModeCreateAndSeed {
		hasA := plan.amountAUnits != nil && plan.amountAUnits.Sign() > 0
		hasB := plan.amountBUnits != nil && plan.amountBUnits.Sign() > 0
		if hasA || hasB {
			if err := createPoolValidateSingleInput(plan); err != nil {
				plan.warnings = append(plan.warnings, err.Error())
			} else if plan.amountMode == createPoolAmountModeDual {
				if plan.tokenAIsToken0 {
					plan.amount0Desired = new(big.Int).Set(plan.amountAUnits)
					plan.amount1Desired = new(big.Int).Set(plan.amountBUnits)
				} else {
					plan.amount0Desired = new(big.Int).Set(plan.amountBUnits)
					plan.amount1Desired = new(big.Int).Set(plan.amountAUnits)
				}
			} else {
				quoteWarnings, err := createPoolQuoteSingleSided(plan)
				if err != nil {
					return nil, nil, err
				}
				plan.warnings = append(plan.warnings, quoteWarnings...)
				if plan.singleInputAmount != nil && plan.singleInputAmount.Sign() > 0 && plan.sqrtPriceX96 != nil {
					liqSvc := liquidity.NewLiquidityService()
					amount0In := big.NewInt(0)
					amount1In := big.NewInt(0)
					if plan.singleInputToken == plan.token0.Address {
						amount0In = new(big.Int).Set(plan.singleInputAmount)
					} else {
						amount1In = new(big.Int).Set(plan.singleInputAmount)
					}
					zeroForOne, swapAmount, swapErr := liqSvc.CalculateOptimalSwapForRange(plan.sqrtPriceX96, plan.currentTick, plan.tickLower, plan.tickUpper, amount0In, amount1In)
					if swapErr != nil {
						plan.warnings = append(plan.warnings, fmt.Sprintf("单币自动配比估算失败，执行时会再次计算: %v", swapErr))
					} else if swapAmount != nil {
						direction, _, _ := createPoolQuoteDirection(plan, zeroForOne)
						plan.estimatedSwapDirection = direction
						plan.estimatedSwapAmount = new(big.Int).Set(swapAmount)
						if direction == "a_to_b" {
							plan.estimatedSwapAmountText = createPoolHumanAmount(swapAmount, plan.tokenA.Decimals)
							plan.estimatedSwapTokenIn = plan.tokenA.Address
							plan.estimatedSwapTokenOut = plan.tokenB.Address
						} else if direction == "b_to_a" {
							plan.estimatedSwapAmountText = createPoolHumanAmount(swapAmount, plan.tokenB.Decimals)
							plan.estimatedSwapTokenIn = plan.tokenB.Address
							plan.estimatedSwapTokenOut = plan.tokenA.Address
						}
					}
				}
			}
		} else {
			if plan.amountMode == createPoolAmountModeSingle {
				plan.warnings = append(plan.warnings, "single_auto_swap 需要输入 Token A 或 Token B 其中一个数量")
			} else {
				plan.warnings = append(plan.warnings, "dual_exact 需要同时输入 Token A 和 Token B 数量")
			}
		}
	}

	if plan.mode == createPoolModeCreateAndSeed && plan.sqrtPriceX96 != nil {
		sqrtLower, err := poolSvc.SqrtRatioAtTick(int32(plan.tickLower))
		if err != nil {
			return nil, nil, err
		}
		sqrtUpper, err := poolSvc.SqrtRatioAtTick(int32(plan.tickUpper))
		if err != nil {
			return nil, nil, err
		}
		switch plan.amountMode {
		case createPoolAmountModeDual:
			if plan.amount0Desired != nil && plan.amount1Desired != nil {
				plan.estimatedLiquidity = poolSvc.LiquidityForAmounts(plan.sqrtPriceX96, sqrtLower, sqrtUpper, plan.amount0Desired, plan.amount1Desired)
			}
		case createPoolAmountModeSingle:
			var quote0, quote1 *big.Int
			if plan.tokenAIsToken0 {
				quote0 = cloneBigInt(plan.mirrorAmountAUnits)
				quote1 = cloneBigInt(plan.mirrorAmountBUnits)
			} else {
				quote0 = cloneBigInt(plan.mirrorAmountBUnits)
				quote1 = cloneBigInt(plan.mirrorAmountAUnits)
			}
			if quote0 != nil && quote1 != nil && (quote0.Sign() > 0 || quote1.Sign() > 0) {
				plan.estimatedLiquidity = poolSvc.LiquidityForAmounts(plan.sqrtPriceX96, sqrtLower, sqrtUpper, quote0, quote1)
			}
		}
	}

	if requireAmounts && plan.initialPriceAB == nil {
		return nil, nil, fmt.Errorf("missing initial_price and failed to derive a suggested price")
	}
	if requireAmounts && plan.mode == createPoolModeCreateAndSeed {
		switch plan.amountMode {
		case createPoolAmountModeDual:
			if plan.amount0Desired == nil || plan.amount1Desired == nil {
				return nil, nil, fmt.Errorf("dual_exact requires both amount_a and amount_b")
			}
		case createPoolAmountModeSingle:
			if err := createPoolValidateSingleInput(plan); err != nil {
				return nil, nil, err
			}
			if plan.singleInputToken == (common.Address{}) || plan.singleInputAmount == nil || plan.singleInputAmount.Sign() <= 0 {
				return nil, nil, fmt.Errorf("single_auto_swap requires exactly one non-zero input amount")
			}
		}
	}
	if requireAmounts && plan.poolExists {
		if plan.protocol == createPoolProtocolUniV4 {
			return nil, nil, fmt.Errorf("target v4 pool already exists")
		}
		return nil, nil, fmt.Errorf("target v3 pool already exists at %s", plan.existingPoolAddress.Hex())
	}

	readyToExecute := plan.initialPriceAB != nil && !plan.poolExists
	if plan.mode == createPoolModeCreateAndSeed {
		if plan.amountMode == createPoolAmountModeDual {
			readyToExecute = readyToExecute && plan.amount0Desired != nil && plan.amount1Desired != nil
		} else {
			readyToExecute = readyToExecute && plan.singleInputToken != (common.Address{}) && plan.singleInputAmount != nil && plan.singleInputAmount.Sign() > 0
		}
	}

	resp := &createPoolPreviewResponse{
		OK:                       true,
		ReadyToExecute:           readyToExecute,
		Chain:                    plan.ctx.chain,
		Protocol:                 plan.protocol,
		Mode:                     plan.mode,
		RangeMode:                plan.rangeMode,
		AmountMode:               plan.amountMode,
		WalletAddress:            strings.TrimSpace(plan.ctx.wallet.Address),
		TokenA:                   createPoolTokenResponseFromMeta(plan.tokenA),
		TokenB:                   createPoolTokenResponseFromMeta(plan.tokenB),
		Token0:                   createPoolTokenResponseFromMeta(plan.token0),
		Token1:                   createPoolTokenResponseFromMeta(plan.token1),
		FeeTier:                  plan.feeTier,
		TickSpacing:              plan.tickSpacing,
		InitialPrice:             plan.initialPriceABText,
		InitialPriceSource:       plan.initialPriceSource,
		SuggestedInitialPrice:    plan.initialPriceABText,
		CanonicalPriceToken1Per0: plan.canonicalPriceText,
		MinPrice:                 plan.minPriceABText,
		MaxPrice:                 plan.maxPriceABText,
		SqrtPriceX96:             amountString(plan.sqrtPriceX96),
		TickLower:                plan.tickLower,
		TickUpper:                plan.tickUpper,
		AmountA:                  amountString(plan.amountAUnits),
		AmountB:                  amountString(plan.amountBUnits),
		Amount0Desired:           amountString(plan.amount0Desired),
		Amount1Desired:           amountString(plan.amount1Desired),
		MirrorAmountA:            plan.mirrorAmountA,
		MirrorAmountB:            plan.mirrorAmountB,
		MirrorAmountSource:       plan.mirrorAmountSource,
		SingleSidedInput:         plan.singleSidedInput,
		EstimatedSwapDirection:   plan.estimatedSwapDirection,
		EstimatedSwapAmount:      plan.estimatedSwapAmountText,
		EstimatedSwapAmountRaw:   amountString(plan.estimatedSwapAmount),
		EstimatedSwapTokenIn:     plan.estimatedSwapTokenIn.Hex(),
		EstimatedSwapTokenOut:    plan.estimatedSwapTokenOut.Hex(),
		EstimatedLiquidity:       amountString(plan.estimatedLiquidity),
		PoolExists:               plan.poolExists,
		Warnings:                 plan.warnings,
	}
	if plan.existingPoolAddress != (common.Address{}) {
		resp.ExistingPoolAddress = plan.existingPoolAddress.Hex()
	}
	if plan.predictedPoolID != (common.Hash{}) {
		resp.PredictedPoolID = plan.predictedPoolID.Hex()
		if plan.poolExists {
			resp.ExistingPoolID = plan.predictedPoolID.Hex()
		}
	}
	return plan, resp, nil
}
