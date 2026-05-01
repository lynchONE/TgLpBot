package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	tradepkg "TgLpBot/service/trade"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// SwapTaskDustToUSDT swaps recorded dust tokens back to USDT (best effort).
func (s *LiquidityService) SwapTaskDustToUSDT(userID uint, task *models.StrategyTask) ([]string, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	task.Chain = config.NormalizeChain(task.Chain)
	exec, err := chainexec.GetEVM(task.Chain)
	if err != nil {
		return nil, err
	}
	cc := exec.Config()
	client := exec.Client()
	if client == nil {
		return nil, fmt.Errorf("blockchain client not initialized for chain=%s", exec.Chain())
	}
	if !common.IsHexAddress(cc.StableAddress) {
		return nil, fmt.Errorf("stable address not set for chain=%s", exec.Chain())
	}

	rec, err := s.tradeRecordService.GetLatestOpenRecord(task.UserID, task.ID)
	if err != nil {
		return nil, fmt.Errorf("no open trade record")
	}

	dust0, _ := convert.ParseBigInt(rec.OpenDust0)
	dust1, _ := convert.ParseBigInt(rec.OpenDust1)
	if dust0 == nil {
		dust0 = big.NewInt(0)
	}
	if dust1 == nil {
		dust1 = big.NewInt(0)
	}
	extraDust := tradepkg.ParseOpenDustAssets(rec.OpenExtraDust)
	if dust0.Sign() <= 0 && dust1.Sign() <= 0 && len(extraDust) == 0 {
		return nil, fmt.Errorf("no dust to swap")
	}

	token0Addr, token1Addr, err := s.resolveTaskTokenAddresses(task)
	if err != nil {
		return nil, err
	}

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
	usdtAddr := common.HexToAddress(cc.StableAddress)

	txHashes := make([]string, 0, 2)

	openSpentWei, _ := convert.ParseBigInt(rec.OpenUSDTSpent)
	if openSpentWei == nil {
		openSpentWei = big.NewInt(0)
	}
	openStableBeforeWei, _ := convert.ParseBigInt(rec.OpenStableBefore)
	if openStableBeforeWei == nil {
		openStableBeforeWei = big.NewInt(0)
	}
	openGasWei, _ := convert.ParseBigInt(rec.OpenGasSpentWei)
	if openGasWei == nil {
		openGasWei = big.NewInt(0)
	}

	if dust0.Sign() > 0 {
		if token0Addr != usdtAddr {
			nativeBefore, _ := blockchain.GetBalanceWithClient(client, walletAddr)
			if nativeBefore == nil {
				nativeBefore = big.NewInt(0)
			}
			usdtBefore, _ := blockchain.GetTokenBalanceWithClient(client, usdtAddr, walletAddr)
			if usdtBefore == nil {
				usdtBefore = big.NewInt(0)
			}
			txHash, err := s.swapDeltaToUSDTWithHash(exec, privateKey, walletAddr, token0Addr, usdtAddr, dust0, task.SlippageTolerance)
			if err != nil {
				return txHashes, err
			}
			if txHash != "" {
				nativeAfter, _ := blockchain.GetBalanceWithClient(client, walletAddr)
				if nativeAfter == nil {
					nativeAfter = big.NewInt(0)
				}
				gasSpentWei := new(big.Int).Sub(nativeBefore, nativeAfter)
				if gasSpentWei.Sign() < 0 {
					gasSpentWei = big.NewInt(0)
				}
				if gasSpentWei.Sign() > 0 {
					openGasWei.Add(openGasWei, gasSpentWei)
				}

				usdtAfter, _ := blockchain.GetTokenBalanceWithClient(client, usdtAddr, walletAddr)
				if usdtAfter == nil {
					usdtAfter = big.NewInt(0)
				}
				usdtDeltaRaw := new(big.Int).Sub(usdtAfter, usdtBefore)
				if usdtDeltaRaw.Sign() < 0 {
					usdtDeltaRaw = big.NewInt(0)
				}
				if receipt, rerr := s.getReceiptWithRetry(client, common.HexToHash(txHash)); rerr == nil && receipt != nil {
					if d := ReceiptTokenTransferDelta(receipt, usdtAddr, walletAddr); d != nil && d.Sign() > 0 {
						usdtDeltaRaw = d
					}
				}
				usdtDeltaWei, derr := convert.ScaleDecimals(usdtDeltaRaw, cc.StableDecimals, 18)
				if derr != nil || usdtDeltaWei == nil {
					usdtDeltaWei = new(big.Int).Set(usdtDeltaRaw)
				}
				if usdtDeltaWei.Sign() > 0 && openSpentWei.Sign() > 0 {
					openSpentWei.Sub(openSpentWei, usdtDeltaWei)
					if openSpentWei.Sign() < 0 {
						openSpentWei = big.NewInt(0)
					}
				}
				openStableAfterWei := new(big.Int).Sub(new(big.Int).Set(openStableBeforeWei), openSpentWei)
				if openStableAfterWei.Sign() < 0 {
					openStableAfterWei = big.NewInt(0)
				}
				_ = database.DB.Model(&models.TradeRecord{}).Where("id = ?", rec.ID).Updates(map[string]interface{}{
					"open_dust0":         "0",
					"open_usdt_spent":    openSpentWei.String(),
					"open_stable_after":  openStableAfterWei.String(),
					"open_gas_spent_wei": openGasWei.String(),
				}).Error
				rec.OpenDust0 = "0"
				rec.OpenUSDTSpent = openSpentWei.String()
				rec.OpenStableAfter = openStableAfterWei.String()
				rec.OpenGasSpentWei = openGasWei.String()
				sym := strings.TrimSpace(task.Token0Symbol)
				if sym == "" {
					sym = token0Addr.Hex()
				}
				txHashes = append(txHashes, fmt.Sprintf("Dust %s->USDT|%s", sym, txHash))
			}
		}
	}

	if dust1.Sign() > 0 {
		if token1Addr != usdtAddr {
			nativeBefore, _ := blockchain.GetBalanceWithClient(client, walletAddr)
			if nativeBefore == nil {
				nativeBefore = big.NewInt(0)
			}
			usdtBefore, _ := blockchain.GetTokenBalanceWithClient(client, usdtAddr, walletAddr)
			if usdtBefore == nil {
				usdtBefore = big.NewInt(0)
			}
			txHash, err := s.swapDeltaToUSDTWithHash(exec, privateKey, walletAddr, token1Addr, usdtAddr, dust1, task.SlippageTolerance)
			if err != nil {
				return txHashes, err
			}
			if txHash != "" {
				nativeAfter, _ := blockchain.GetBalanceWithClient(client, walletAddr)
				if nativeAfter == nil {
					nativeAfter = big.NewInt(0)
				}
				gasSpentWei := new(big.Int).Sub(nativeBefore, nativeAfter)
				if gasSpentWei.Sign() < 0 {
					gasSpentWei = big.NewInt(0)
				}
				if gasSpentWei.Sign() > 0 {
					openGasWei.Add(openGasWei, gasSpentWei)
				}

				usdtAfter, _ := blockchain.GetTokenBalanceWithClient(client, usdtAddr, walletAddr)
				if usdtAfter == nil {
					usdtAfter = big.NewInt(0)
				}
				usdtDeltaRaw := new(big.Int).Sub(usdtAfter, usdtBefore)
				if usdtDeltaRaw.Sign() < 0 {
					usdtDeltaRaw = big.NewInt(0)
				}
				if receipt, rerr := s.getReceiptWithRetry(client, common.HexToHash(txHash)); rerr == nil && receipt != nil {
					if d := ReceiptTokenTransferDelta(receipt, usdtAddr, walletAddr); d != nil && d.Sign() > 0 {
						usdtDeltaRaw = d
					}
				}
				usdtDeltaWei, derr := convert.ScaleDecimals(usdtDeltaRaw, cc.StableDecimals, 18)
				if derr != nil || usdtDeltaWei == nil {
					usdtDeltaWei = new(big.Int).Set(usdtDeltaRaw)
				}
				if usdtDeltaWei.Sign() > 0 && openSpentWei.Sign() > 0 {
					openSpentWei.Sub(openSpentWei, usdtDeltaWei)
					if openSpentWei.Sign() < 0 {
						openSpentWei = big.NewInt(0)
					}
				}
				openStableAfterWei := new(big.Int).Sub(new(big.Int).Set(openStableBeforeWei), openSpentWei)
				if openStableAfterWei.Sign() < 0 {
					openStableAfterWei = big.NewInt(0)
				}
				_ = database.DB.Model(&models.TradeRecord{}).Where("id = ?", rec.ID).Updates(map[string]interface{}{
					"open_dust1":         "0",
					"open_usdt_spent":    openSpentWei.String(),
					"open_stable_after":  openStableAfterWei.String(),
					"open_gas_spent_wei": openGasWei.String(),
				}).Error
				rec.OpenDust1 = "0"
				rec.OpenUSDTSpent = openSpentWei.String()
				rec.OpenStableAfter = openStableAfterWei.String()
				rec.OpenGasSpentWei = openGasWei.String()
				sym := strings.TrimSpace(task.Token1Symbol)
				if sym == "" {
					sym = token1Addr.Hex()
				}
				txHashes = append(txHashes, fmt.Sprintf("Dust %s->USDT|%s", sym, txHash))
			}
		}
	}

	if len(extraDust) > 0 {
		remainingExtra := append([]models.TradeRecordDustAsset(nil), extraDust...)
		persistExtraDust := func() {
			openStableAfterWei := new(big.Int).Sub(new(big.Int).Set(openStableBeforeWei), openSpentWei)
			if openStableAfterWei.Sign() < 0 {
				openStableAfterWei = big.NewInt(0)
			}
			_ = database.DB.Model(&models.TradeRecord{}).Where("id = ?", rec.ID).Updates(map[string]interface{}{
				"open_extra_dust":    tradepkg.EncodeOpenDustAssets(remainingExtra),
				"open_usdt_spent":    openSpentWei.String(),
				"open_stable_after":  openStableAfterWei.String(),
				"open_gas_spent_wei": openGasWei.String(),
			}).Error
		}

		for idx := 0; idx < len(remainingExtra); {
			asset := remainingExtra[idx]
			amount, _ := convert.ParseBigInt(asset.Amount)
			if amount == nil || amount.Sign() <= 0 {
				remainingExtra = append(remainingExtra[:idx], remainingExtra[idx+1:]...)
				persistExtraDust()
				continue
			}
			if !common.IsHexAddress(asset.Address) {
				idx++
				continue
			}
			tokenAddr := common.HexToAddress(asset.Address)
			sym := strings.TrimSpace(asset.Symbol)
			if sym == "" {
				sym = tokenAddr.Hex()
			}
			if tokenAddr == usdtAddr {
				idx++
				continue
			}

			nativeBefore, _ := blockchain.GetBalanceWithClient(client, walletAddr)
			if nativeBefore == nil {
				nativeBefore = big.NewInt(0)
			}
			usdtBefore, _ := blockchain.GetTokenBalanceWithClient(client, usdtAddr, walletAddr)
			if usdtBefore == nil {
				usdtBefore = big.NewInt(0)
			}
			txHash, err := s.swapDeltaToUSDTWithHash(exec, privateKey, walletAddr, tokenAddr, usdtAddr, amount, task.SlippageTolerance)
			if err != nil {
				return txHashes, err
			}
			if txHash == "" {
				idx++
				continue
			}

			nativeAfter, _ := blockchain.GetBalanceWithClient(client, walletAddr)
			if nativeAfter == nil {
				nativeAfter = big.NewInt(0)
			}
			gasSpentWei := new(big.Int).Sub(nativeBefore, nativeAfter)
			if gasSpentWei.Sign() < 0 {
				gasSpentWei = big.NewInt(0)
			}
			if gasSpentWei.Sign() > 0 {
				openGasWei.Add(openGasWei, gasSpentWei)
			}

			usdtAfter, _ := blockchain.GetTokenBalanceWithClient(client, usdtAddr, walletAddr)
			if usdtAfter == nil {
				usdtAfter = big.NewInt(0)
			}
			usdtDeltaRaw := new(big.Int).Sub(usdtAfter, usdtBefore)
			if usdtDeltaRaw.Sign() < 0 {
				usdtDeltaRaw = big.NewInt(0)
			}
			if receipt, rerr := s.getReceiptWithRetry(client, common.HexToHash(txHash)); rerr == nil && receipt != nil {
				if d := ReceiptTokenTransferDelta(receipt, usdtAddr, walletAddr); d != nil && d.Sign() > 0 {
					usdtDeltaRaw = d
				}
			}
			usdtDeltaWei, derr := convert.ScaleDecimals(usdtDeltaRaw, cc.StableDecimals, 18)
			if derr != nil || usdtDeltaWei == nil {
				usdtDeltaWei = new(big.Int).Set(usdtDeltaRaw)
			}
			if usdtDeltaWei.Sign() > 0 && openSpentWei.Sign() > 0 {
				openSpentWei.Sub(openSpentWei, usdtDeltaWei)
				if openSpentWei.Sign() < 0 {
					openSpentWei = big.NewInt(0)
				}
			}
			txHashes = append(txHashes, fmt.Sprintf("Dust %s->USDT|%s", sym, txHash))
			remainingExtra = append(remainingExtra[:idx], remainingExtra[idx+1:]...)
			persistExtraDust()
		}
	}

	return txHashes, nil
}

func (s *LiquidityService) resolveTaskTokenAddresses(task *models.StrategyTask) (common.Address, common.Address, error) {
	if task == nil {
		return common.Address{}, common.Address{}, fmt.Errorf("task is nil")
	}

	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	token0Addr := common.Address{}
	token1Addr := common.Address{}
	token0Set := false
	token1Set := false
	if common.IsHexAddress(task.Token0Address) {
		token0Addr = common.HexToAddress(task.Token0Address)
		token0Set = true
	}
	if common.IsHexAddress(task.Token1Address) {
		token1Addr = common.HexToAddress(task.Token1Address)
		token1Set = true
	}
	if taskTokenAddressesReady(version, token0Addr, token1Addr, token0Set, token1Set) {
		return token0Addr, token1Addr, nil
	}

	chain := config.NormalizeChain(task.Chain)
	exec, err := chainexec.GetEVM(chain)
	if err != nil {
		return common.Address{}, common.Address{}, err
	}
	cc := exec.Config()
	client := exec.Client()
	if client == nil {
		return common.Address{}, common.Address{}, fmt.Errorf("blockchain client not initialized for chain=%s", exec.Chain())
	}

	switch version {
	case "v4":
		pmAddrStr := strings.TrimSpace(cc.UniswapV4PositionManagerAddress)
		if !common.IsHexAddress(pmAddrStr) && config.AppConfig != nil {
			pmAddrStr = strings.TrimSpace(config.AppConfig.UniswapV4PositionManagerAddress)
		}
		if common.IsHexAddress(pmAddrStr) {
			tokenId, _ := convert.ParseBigInt(task.V4TokenID)
			if tokenId.Sign() > 0 {
				v4pmAddr := common.HexToAddress(pmAddrStr)
				poolMgrStr := strings.TrimSpace(cc.UniswapV4PoolManagerAddress)
				if !common.IsHexAddress(poolMgrStr) && config.AppConfig != nil {
					poolMgrStr = strings.TrimSpace(config.AppConfig.UniswapV4PoolManagerAddress)
				}
				if common.IsHexAddress(poolMgrStr) {
					if pos, err := blockchain.GetV4PositionInfo(v4pmAddr, common.HexToAddress(poolMgrStr), task.PoolId, tokenId); err == nil && pos != nil {
						if !token0Set {
							token0Addr = pos.Token0
							token0Set = true
						}
						if !token1Set {
							token1Addr = pos.Token1
							token1Set = true
						}
					}
				}
			}
		}
	default:
		if common.IsHexAddress(task.PoolId) {
			poolAddr := common.HexToAddress(task.PoolId)
			if c0, c1, err := blockchain.GetV3PoolTokensWithClient(client, poolAddr); err == nil {
				if !token0Set {
					token0Addr = c0
					token0Set = true
				}
				if !token1Set {
					token1Addr = c1
					token1Set = true
				}
			}
		}

		if !taskTokenAddressesReady(version, token0Addr, token1Addr, token0Set, token1Set) {
			tokenId, _ := convert.ParseBigInt(task.V3TokenID)
			if tokenId.Sign() > 0 {
				pmAddrStr := strings.TrimSpace(task.V3PositionManagerAddress)
				if pmAddrStr == "" && common.IsHexAddress(cc.DefaultV3PositionManagerAddress) {
					pmAddrStr = strings.TrimSpace(cc.DefaultV3PositionManagerAddress)
				}
				if common.IsHexAddress(pmAddrStr) {
					pmAddr := common.HexToAddress(pmAddrStr)
					if v3pm, err := blockchain.NewV3PositionManager(pmAddr, client); err == nil {
						if pos, err := v3pm.Positions(nil, tokenId); err == nil && pos != nil {
							if !token0Set {
								token0Addr = pos.Token0
								token0Set = true
							}
							if !token1Set {
								token1Addr = pos.Token1
								token1Set = true
							}
						}
					}
				}
			}
		}
	}

	if !taskTokenAddressesReady(version, token0Addr, token1Addr, token0Set, token1Set) {
		return common.Address{}, common.Address{}, fmt.Errorf("token address missing (pool_version=%s pool_id=%s)", version, strings.TrimSpace(task.PoolId))
	}
	return token0Addr, token1Addr, nil
}

func taskTokenAddressesReady(version string, token0Addr, token1Addr common.Address, token0Set, token1Set bool) bool {
	if !token0Set || !token1Set {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(version), "v4") {
		if token0Addr == token1Addr {
			return false
		}
		return token0Addr != (common.Address{}) || token1Addr != (common.Address{})
	}
	return token0Addr != (common.Address{}) && token1Addr != (common.Address{})
}
