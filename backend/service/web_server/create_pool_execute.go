package web_server

import (
	"fmt"
	"math/big"
	"time"

	"TgLpBot/base/blockchain"
	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/wallet"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func (s *Server) executeCreatePoolPlan(plan *createPoolPlan) (*createPoolExecuteResponse, error) {
	if plan == nil || plan.ctx == nil {
		return nil, fmt.Errorf("create pool plan is nil")
	}

	walletSvc := wallet.NewWalletService()
	privateKeyHex, err := walletSvc.GetPrivateKey(plan.ctx.wallet)
	if err != nil {
		return nil, fmt.Errorf("failed to load wallet private key: %w", err)
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid wallet private key: %w", err)
	}

	liqSvc := liquidity.NewLiquidityService()
	walletAddr := common.HexToAddress(plan.ctx.wallet.Address)
	var txHashes []string

	switch plan.protocol {
	case createPoolProtocolUniV3, createPoolProtocolPcsV3:
		v3pm, err := blockchain.NewV3PositionManager(plan.positionManager, plan.ctx.client)
		if err != nil {
			return nil, fmt.Errorf("init v3 position manager failed: %w", err)
		}

		feeBig := new(big.Int).SetUint64(plan.feeTier)
		initData, err := v3pm.Pack("createAndInitializePoolIfNecessary", plan.token0.Address, plan.token1.Address, feeBig, plan.sqrtPriceX96)
		if err != nil {
			return nil, fmt.Errorf("pack v3 init calldata failed: %w", err)
		}

		nonce, err := blockchain.GetNonceWithClient(plan.ctx.client, walletAddr)
		if err != nil {
			return nil, err
		}
		auth, err := liqSvc.BuildAuthForTx(plan.ctx.client, plan.ctx.chainID, privateKey, nonce, big.NewInt(0), liquidity.TxOptions{})
		if err != nil {
			return nil, err
		}

		var tx *types.Transaction
		if plan.mode == createPoolModeCreateOnly {
			tx, err = v3pm.CreateAndInitializePoolIfNecessary(auth, plan.token0.Address, plan.token1.Address, feeBig, plan.sqrtPriceX96)
			if err != nil {
				return nil, fmt.Errorf("create v3 pool failed: %w", err)
			}
		} else if plan.amountMode == createPoolAmountModeDual {
			if err := liqSvc.ApproveTokenForSpender(plan.ctx.client, plan.ctx.chainID, privateKey, walletAddr, plan.token0.Address, plan.positionManager, plan.amount0Desired, liquidity.TxOptions{}); err != nil {
				return nil, fmt.Errorf("approve token0 failed: %w", err)
			}
			if err := liqSvc.ApproveTokenForSpender(plan.ctx.client, plan.ctx.chainID, privateKey, walletAddr, plan.token1.Address, plan.positionManager, plan.amount1Desired, liquidity.TxOptions{}); err != nil {
				return nil, fmt.Errorf("approve token1 failed: %w", err)
			}

			mintParams := blockchain.V3MintParams{
				Token0:         plan.token0.Address,
				Token1:         plan.token1.Address,
				Fee:            feeBig,
				TickLower:      big.NewInt(int64(plan.tickLower)),
				TickUpper:      big.NewInt(int64(plan.tickUpper)),
				Amount0Desired: plan.amount0Desired,
				Amount1Desired: plan.amount1Desired,
				Amount0Min:     big.NewInt(0),
				Amount1Min:     big.NewInt(0),
				Recipient:      walletAddr,
				Deadline:       big.NewInt(time.Now().Add(20 * time.Minute).Unix()),
			}
			mintData, err := v3pm.Pack("mint", mintParams)
			if err != nil {
				return nil, fmt.Errorf("pack v3 mint calldata failed: %w", err)
			}
			tx, err = v3pm.Multicall(auth, [][]byte{initData, mintData})
			if err != nil {
				return nil, fmt.Errorf("create+mint v3 pool failed: %w", err)
			}
		} else {
			tx, err = v3pm.CreateAndInitializePoolIfNecessary(auth, plan.token0.Address, plan.token1.Address, feeBig, plan.sqrtPriceX96)
			if err != nil {
				return nil, fmt.Errorf("create v3 pool failed: %w", err)
			}
		}

		txHashes = append(txHashes, tx.Hash().Hex())
		receipt, err := liqSvc.WaitMinedTx(plan.ctx.client, plan.ctx.chainID, tx)
		if err != nil {
			return nil, fmt.Errorf("v3 create pool transaction failed: %w", err)
		}

		poolAddr, err := blockchain.GetV3PoolFromFactoryWithClient(plan.ctx.client, plan.factory, plan.token0.Address, plan.token1.Address, plan.feeTier)
		if err != nil {
			return nil, fmt.Errorf("resolve created v3 pool failed: %w", err)
		}

		resp := &createPoolExecuteResponse{
			OK:            true,
			Status:        "ok",
			Chain:         plan.ctx.chain,
			Protocol:      plan.protocol,
			Mode:          plan.mode,
			WalletAddress: plan.ctx.wallet.Address,
			PoolAddress:   poolAddr.Hex(),
			TxHash:        tx.Hash().Hex(),
			TxHashes:      txHashes,
			ExplorerURLs:  explorerURLsForTxs(plan.ctx.cc, txHashes),
			Warnings:      plan.warnings,
		}
		if plan.mode == createPoolModeCreateOnly {
			return resp, nil
		}

		if plan.amountMode == createPoolAmountModeDual {
			if tokenID := parseCreatePoolMintedTokenID(receipt, plan.positionManager, walletAddr); tokenID != nil {
				resp.TokenID = tokenID.String()
				if pos, err := v3pm.Positions(&bind.CallOpts{}, tokenID); err == nil && pos != nil && pos.Liquidity != nil {
					resp.Liquidity = pos.Liquidity.String()
				}
			}
			return resp, nil
		}

		task := createPoolStrategyTask(plan, poolAddr.Hex())
		entry, err := liqSvc.EnterV3FromToken(plan.ctx.exec, plan.ctx.wallet, privateKey, walletAddr, plan.singleInputToken, plan.singleInputAmount, task, liquidity.TxOptions{})
		if err != nil {
			return nil, fmt.Errorf("single-token v3 seed failed: %w", err)
		}
		if entry != nil && entry.TxHash != "" {
			txHashes = append(txHashes, entry.TxHash)
			resp.TxHash = entry.TxHash
			resp.TxHashes = txHashes
			resp.ExplorerURLs = explorerURLsForTxs(plan.ctx.cc, txHashes)
			resp.TokenID = entry.V3TokenID
			resp.Liquidity = entry.CurrentLiquidity
		}
		return resp, nil

	case createPoolProtocolUniV4:
		v4pm, err := blockchain.NewV4PositionManager(plan.positionManager, plan.ctx.client)
		if err != nil {
			return nil, fmt.Errorf("init v4 position manager failed: %w", err)
		}

		nonce, err := blockchain.GetNonceWithClient(plan.ctx.client, walletAddr)
		if err != nil {
			return nil, err
		}
		auth, err := liqSvc.BuildAuthForTx(plan.ctx.client, plan.ctx.chainID, privateKey, nonce, big.NewInt(0), liquidity.TxOptions{})
		if err != nil {
			return nil, err
		}
		key := blockchain.V4PoolKey{
			Currency0:   plan.token0.Address,
			Currency1:   plan.token1.Address,
			Fee:         new(big.Int).SetUint64(plan.feeTier),
			TickSpacing: big.NewInt(int64(plan.tickSpacing)),
			Hooks:       plan.hooks,
		}
		initTx, err := v4pm.InitializePool(auth, key, plan.sqrtPriceX96)
		if err != nil {
			return nil, fmt.Errorf("initialize v4 pool failed: %w", err)
		}
		txHashes = append(txHashes, initTx.Hash().Hex())
		if _, err := liqSvc.WaitMinedTx(plan.ctx.client, plan.ctx.chainID, initTx); err != nil {
			return nil, fmt.Errorf("v4 initialize pool transaction failed: %w", err)
		}

		resp := &createPoolExecuteResponse{
			OK:            true,
			Status:        "ok",
			Chain:         plan.ctx.chain,
			Protocol:      plan.protocol,
			Mode:          plan.mode,
			WalletAddress: plan.ctx.wallet.Address,
			PoolID:        plan.predictedPoolID.Hex(),
			TxHash:        initTx.Hash().Hex(),
			TxHashes:      txHashes,
			Warnings:      plan.warnings,
		}
		if plan.mode == createPoolModeCreateOnly {
			resp.ExplorerURLs = explorerURLsForTxs(plan.ctx.cc, txHashes)
			return resp, nil
		}

		if plan.amountMode == createPoolAmountModeSingle {
			task := createPoolStrategyTask(plan, plan.predictedPoolID.Hex())
			entry, err := liqSvc.EnterV4FromToken(plan.ctx.exec, plan.ctx.wallet, privateKey, walletAddr, plan.singleInputToken, plan.singleInputAmount, task, liquidity.TxOptions{})
			if err != nil {
				return nil, fmt.Errorf("single-token v4 seed failed: %w", err)
			}
			if entry != nil && entry.TxHash != "" {
				txHashes = append(txHashes, entry.TxHash)
				resp.TxHash = entry.TxHash
				resp.TxHashes = txHashes
				resp.ExplorerURLs = explorerURLsForTxs(plan.ctx.cc, txHashes)
				resp.TokenID = entry.V4TokenID
				resp.Liquidity = entry.CurrentLiquidity
			}
			return resp, nil
		}

		if !common.IsHexAddress(plan.ctx.cc.ZapV4Address) {
			return nil, fmt.Errorf("ZAP_V4_ADDRESS not configured")
		}
		zapAddr := common.HexToAddress(plan.ctx.cc.ZapV4Address)
		if err := liqSvc.ApproveTokenForSpender(plan.ctx.client, plan.ctx.chainID, privateKey, walletAddr, plan.token0.Address, zapAddr, plan.amount0Desired, liquidity.TxOptions{}); err != nil {
			return nil, fmt.Errorf("approve token0 to zap failed: %w", err)
		}
		if err := liqSvc.ApproveTokenForSpender(plan.ctx.client, plan.ctx.chainID, privateKey, walletAddr, plan.token1.Address, zapAddr, plan.amount1Desired, liquidity.TxOptions{}); err != nil {
			return nil, fmt.Errorf("approve token1 to zap failed: %w", err)
		}

		zap, err := blockchain.NewZapSimple(zapAddr, plan.ctx.client)
		if err != nil {
			return nil, fmt.Errorf("init zap v4 failed: %w", err)
		}
		nonce, err = blockchain.GetNonceWithClient(plan.ctx.client, walletAddr)
		if err != nil {
			return nil, err
		}
		auth, err = liqSvc.BuildAuthForTx(plan.ctx.client, plan.ctx.chainID, privateKey, nonce, big.NewInt(0), liquidity.TxOptions{})
		if err != nil {
			return nil, err
		}

		zapParams := blockchain.ZapInV4ParamsSimple{
			PoolKey: blockchain.PoolKeySimple{
				Currency0:   plan.token0.Address,
				Currency1:   plan.token1.Address,
				Fee:         new(big.Int).SetUint64(plan.feeTier),
				TickSpacing: big.NewInt(int64(plan.tickSpacing)),
				Hooks:       plan.hooks,
			},
			StateView:       common.HexToAddress(plan.ctx.cc.UniswapV4StateViewAddress),
			PositionManager: plan.positionManager,
			TickLower:       big.NewInt(int64(plan.tickLower)),
			TickUpper:       big.NewInt(int64(plan.tickUpper)),
			Recipient:       walletAddr,
			Amount0In:       plan.amount0Desired,
			Amount1In:       plan.amount1Desired,
			SlippageBps:     big.NewInt(int64(plan.slippagePct * 100)),
			Swap: blockchain.SwapParamsSimple{
				Target:        common.Address{},
				ApproveTarget: common.Address{},
				TokenIn:       common.Address{},
				TokenOut:      common.Address{},
				AmountIn:      big.NewInt(0),
				MinAmountOut:  big.NewInt(0),
				CallData:      []byte{},
			},
			SqrtPriceX96: plan.sqrtPriceX96,
		}
		seedTx, err := zap.ZapInV4(auth, zapParams)
		if err != nil {
			return nil, fmt.Errorf("seed v4 pool failed: %w", err)
		}
		txHashes = append(txHashes, seedTx.Hash().Hex())
		seedReceipt, err := liqSvc.WaitMinedTx(plan.ctx.client, plan.ctx.chainID, seedTx)
		if err != nil {
			return nil, fmt.Errorf("v4 seed pool transaction failed: %w", err)
		}

		resp.TxHash = seedTx.Hash().Hex()
		resp.TxHashes = txHashes
		resp.ExplorerURLs = explorerURLsForTxs(plan.ctx.cc, txHashes)
		if tokenID, liq, ok := parseCreatePoolZapInV4(seedReceipt, zapAddr); ok {
			if tokenID != nil {
				resp.TokenID = tokenID.String()
			}
			if liq != nil {
				resp.Liquidity = liq.String()
			}
		} else if tokenID := parseCreatePoolMintedTokenID(seedReceipt, plan.positionManager, walletAddr); tokenID != nil {
			resp.TokenID = tokenID.String()
			if pos, err := v4pm.Positions(&bind.CallOpts{}, tokenID); err == nil && pos != nil && pos.Liquidity != nil {
				resp.Liquidity = pos.Liquidity.String()
			}
		}
		return resp, nil

	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}

func createPoolStrategyTask(plan *createPoolPlan, poolRef string) *models.StrategyTask {
	task := &models.StrategyTask{
		Chain:             plan.ctx.chain,
		PoolId:            poolRef,
		WalletID:          plan.ctx.wallet.ID,
		WalletAddress:     plan.ctx.wallet.Address,
		Fee:               int(plan.feeTier),
		TickSpacing:       plan.tickSpacing,
		Token0Address:     plan.token0.Address.Hex(),
		Token1Address:     plan.token1.Address.Hex(),
		HooksAddress:      plan.hooks.Hex(),
		TickLower:         plan.tickLower,
		TickUpper:         plan.tickUpper,
		SlippageTolerance: plan.slippagePct,
	}
	switch plan.protocol {
	case createPoolProtocolUniV3:
		task.PoolVersion = "v3"
		task.Exchange = "Uniswap V3"
		task.V3PositionManagerAddress = plan.positionManager.Hex()
	case createPoolProtocolPcsV3:
		task.PoolVersion = "v3"
		task.Exchange = "PancakeSwap V3"
		task.V3PositionManagerAddress = plan.positionManager.Hex()
	case createPoolProtocolUniV4:
		task.PoolVersion = "v4"
		task.Exchange = "Uniswap V4"
	}
	return task
}
