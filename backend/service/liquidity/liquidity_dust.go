package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
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
	if blockchain.Client == nil || blockchain.ChainID == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if !common.IsHexAddress(config.AppConfig.USDTAddress) {
		return nil, fmt.Errorf("USDT address not set")
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
	if dust0.Sign() <= 0 && dust1.Sign() <= 0 {
		return nil, fmt.Errorf("no dust to swap")
	}

	token0Addr, token1Addr, err := s.resolveTaskTokenAddresses(task)
	if err != nil {
		return nil, err
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

	txHashes := make([]string, 0, 2)

	openSpentWei, _ := convert.ParseBigInt(rec.OpenUSDTSpent)
	if openSpentWei == nil {
		openSpentWei = big.NewInt(0)
	}
	openGasWei, _ := convert.ParseBigInt(rec.OpenGasSpentWei)
	if openGasWei == nil {
		openGasWei = big.NewInt(0)
	}

	if dust0.Sign() > 0 {
		if token0Addr != usdtAddr {
			bnbBefore, _ := blockchain.GetBalance(walletAddr)
			if bnbBefore == nil {
				bnbBefore = big.NewInt(0)
			}
			usdtBefore, _ := blockchain.GetTokenBalance(usdtAddr, walletAddr)
			if usdtBefore == nil {
				usdtBefore = big.NewInt(0)
			}
			txHash, err := s.swapDeltaToUSDTWithHash(privateKey, walletAddr, token0Addr, usdtAddr, dust0, task.SlippageTolerance)
			if err != nil {
				return txHashes, err
			}
			if txHash != "" {
				bnbAfter, _ := blockchain.GetBalance(walletAddr)
				if bnbAfter == nil {
					bnbAfter = big.NewInt(0)
				}
				gasSpentWei := new(big.Int).Sub(bnbBefore, bnbAfter)
				if gasSpentWei.Sign() < 0 {
					gasSpentWei = big.NewInt(0)
				}
				if gasSpentWei.Sign() > 0 {
					openGasWei.Add(openGasWei, gasSpentWei)
				}

				usdtAfter, _ := blockchain.GetTokenBalance(usdtAddr, walletAddr)
				if usdtAfter == nil {
					usdtAfter = big.NewInt(0)
				}
				usdtDelta := new(big.Int).Sub(usdtAfter, usdtBefore)
				if usdtDelta.Sign() < 0 {
					usdtDelta = big.NewInt(0)
				}
				if receipt, rerr := s.getReceiptWithRetry(common.HexToHash(txHash)); rerr == nil && receipt != nil {
					if d := ReceiptTokenTransferDelta(receipt, usdtAddr, walletAddr); d != nil && d.Sign() > 0 {
						usdtDelta = d
					}
				}
				if usdtDelta.Sign() > 0 && openSpentWei.Sign() > 0 {
					openSpentWei.Sub(openSpentWei, usdtDelta)
					if openSpentWei.Sign() < 0 {
						openSpentWei = big.NewInt(0)
					}
				}
				_ = database.DB.Model(&models.TradeRecord{}).Where("id = ?", rec.ID).Updates(map[string]interface{}{
					"open_dust0":         "0",
					"open_usdt_spent":    openSpentWei.String(),
					"open_gas_spent_wei": openGasWei.String(),
				}).Error
				rec.OpenDust0 = "0"
				rec.OpenUSDTSpent = openSpentWei.String()
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
			bnbBefore, _ := blockchain.GetBalance(walletAddr)
			if bnbBefore == nil {
				bnbBefore = big.NewInt(0)
			}
			usdtBefore, _ := blockchain.GetTokenBalance(usdtAddr, walletAddr)
			if usdtBefore == nil {
				usdtBefore = big.NewInt(0)
			}
			txHash, err := s.swapDeltaToUSDTWithHash(privateKey, walletAddr, token1Addr, usdtAddr, dust1, task.SlippageTolerance)
			if err != nil {
				return txHashes, err
			}
			if txHash != "" {
				bnbAfter, _ := blockchain.GetBalance(walletAddr)
				if bnbAfter == nil {
					bnbAfter = big.NewInt(0)
				}
				gasSpentWei := new(big.Int).Sub(bnbBefore, bnbAfter)
				if gasSpentWei.Sign() < 0 {
					gasSpentWei = big.NewInt(0)
				}
				if gasSpentWei.Sign() > 0 {
					openGasWei.Add(openGasWei, gasSpentWei)
				}

				usdtAfter, _ := blockchain.GetTokenBalance(usdtAddr, walletAddr)
				if usdtAfter == nil {
					usdtAfter = big.NewInt(0)
				}
				usdtDelta := new(big.Int).Sub(usdtAfter, usdtBefore)
				if usdtDelta.Sign() < 0 {
					usdtDelta = big.NewInt(0)
				}
				if receipt, rerr := s.getReceiptWithRetry(common.HexToHash(txHash)); rerr == nil && receipt != nil {
					if d := ReceiptTokenTransferDelta(receipt, usdtAddr, walletAddr); d != nil && d.Sign() > 0 {
						usdtDelta = d
					}
				}
				if usdtDelta.Sign() > 0 && openSpentWei.Sign() > 0 {
					openSpentWei.Sub(openSpentWei, usdtDelta)
					if openSpentWei.Sign() < 0 {
						openSpentWei = big.NewInt(0)
					}
				}
				_ = database.DB.Model(&models.TradeRecord{}).Where("id = ?", rec.ID).Updates(map[string]interface{}{
					"open_dust1":         "0",
					"open_usdt_spent":    openSpentWei.String(),
					"open_gas_spent_wei": openGasWei.String(),
				}).Error
				rec.OpenDust1 = "0"
				rec.OpenUSDTSpent = openSpentWei.String()
				rec.OpenGasSpentWei = openGasWei.String()
				sym := strings.TrimSpace(task.Token1Symbol)
				if sym == "" {
					sym = token1Addr.Hex()
				}
				txHashes = append(txHashes, fmt.Sprintf("Dust %s->USDT|%s", sym, txHash))
			}
		}
	}

	return txHashes, nil
}

func (s *LiquidityService) resolveTaskTokenAddresses(task *models.StrategyTask) (common.Address, common.Address, error) {
	token0Addr := common.Address{}
	token1Addr := common.Address{}
	resolvedFromChain := false
	if common.IsHexAddress(task.Token0Address) {
		token0Addr = common.HexToAddress(task.Token0Address)
	}
	if common.IsHexAddress(task.Token1Address) {
		token1Addr = common.HexToAddress(task.Token1Address)
	}
	if token0Addr != (common.Address{}) && token1Addr != (common.Address{}) {
		return token0Addr, token1Addr, nil
	}

	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	switch version {
	case "v4":
		// Try PositionManager.positions(tokenId)
		if common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
			tokenId, _ := convert.ParseBigInt(task.V4TokenID)
			if tokenId.Sign() > 0 {
				v4pmAddr := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
				if v4pm, err := blockchain.NewV4PositionManager(v4pmAddr, blockchain.Client); err == nil {
					if pos, err := v4pm.Positions(nil, tokenId); err == nil && pos != nil {
						resolvedFromChain = true
						if token0Addr == (common.Address{}) {
							token0Addr = pos.Token0
						}
						if token1Addr == (common.Address{}) {
							token1Addr = pos.Token1
						}
					}
				}
			}
		}

		// Try PoolKey from PositionManager / Initialize event
		if (token0Addr == common.Address{} || token1Addr == common.Address{}) && strings.TrimSpace(task.PoolId) != "" {
			if common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
				pmAddr := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
				if c0, c1, _, _, _, err := blockchain.GetUniswapV4PoolKeyFromPositionManager(pmAddr, task.PoolId); err == nil {
					resolvedFromChain = true
					if token0Addr == (common.Address{}) {
						token0Addr = c0
					}
					if token1Addr == (common.Address{}) {
						token1Addr = c1
					}
				} else if common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
					poolMgr := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
					if c0, c1, _, _, _, err := blockchain.GetUniswapV4PoolKeyFromInitializeEvent(poolMgr, task.PoolId); err == nil {
						resolvedFromChain = true
						if token0Addr == (common.Address{}) {
							token0Addr = c0
						}
						if token1Addr == (common.Address{}) {
							token1Addr = c1
						}
					}
				}
			}
		}
	default:
		if common.IsHexAddress(task.PoolId) {
			poolAddr := common.HexToAddress(task.PoolId)
			if c0, c1, err := blockchain.GetV3PoolTokens(poolAddr); err == nil {
				resolvedFromChain = true
				if token0Addr == (common.Address{}) {
					token0Addr = c0
				}
				if token1Addr == (common.Address{}) {
					token1Addr = c1
				}
			}
		}

		if token0Addr == (common.Address{}) || token1Addr == (common.Address{}) {
			tokenId, _ := convert.ParseBigInt(task.V3TokenID)
			if tokenId.Sign() > 0 {
				pmAddrStr := strings.TrimSpace(task.V3PositionManagerAddress)
				if pmAddrStr == "" {
					ex := strings.ToLower(strings.TrimSpace(task.Exchange))
					if strings.Contains(ex, "pancake") && common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
						pmAddrStr = config.AppConfig.PancakeV3PositionManagerAddress
					} else if common.IsHexAddress(config.AppConfig.UniswapV3PositionManagerAddress) {
						pmAddrStr = config.AppConfig.UniswapV3PositionManagerAddress
					}
				}
				if common.IsHexAddress(pmAddrStr) {
					pmAddr := common.HexToAddress(pmAddrStr)
					if v3pm, err := blockchain.NewV3PositionManager(pmAddr, blockchain.Client); err == nil {
						if pos, err := v3pm.Positions(nil, tokenId); err == nil && pos != nil {
							resolvedFromChain = true
							if token0Addr == (common.Address{}) {
								token0Addr = pos.Token0
							}
							if token1Addr == (common.Address{}) {
								token1Addr = pos.Token1
							}
						}
					}
				}
			}
		}
	}

	if token0Addr == (common.Address{}) || token1Addr == (common.Address{}) {
		isExplicitZero := func(addr string) bool {
			addr = strings.TrimSpace(addr)
			return common.IsHexAddress(addr) && common.HexToAddress(addr) == (common.Address{})
		}
		if version == "v4" && (resolvedFromChain || isExplicitZero(task.Token0Address) || isExplicitZero(task.Token1Address)) {
			return common.Address{}, common.Address{}, fmt.Errorf("native currency not supported (use WBNB)")
		}
		return common.Address{}, common.Address{}, fmt.Errorf("token address missing (pool_version=%s pool_id=%s)", version, strings.TrimSpace(task.PoolId))
	}
	return token0Addr, token1Addr, nil
}
