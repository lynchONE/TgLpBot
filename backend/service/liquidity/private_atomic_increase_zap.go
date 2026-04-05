package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"gorm.io/gorm"
)

const walletChainContractKindAtomicIncreaseZap = "atomic_increase_zap"

func privateContractCacheKey(chain string, walletID uint, kind string) string {
	return fmt.Sprintf("%s:%s:%d:%s",
		privateContractCachePrefix(kind),
		config.NormalizeChain(chain),
		walletID,
		strings.TrimSpace(kind),
	)
}

func readPrivateContractCache(chain string, walletID uint, kind string) (common.Address, bool) {
	if database.RedisClient == nil {
		return common.Address{}, false
	}
	key := privateContractCacheKey(chain, walletID, kind)
	addrStr, err := database.GetCache(key)
	if err != nil {
		return common.Address{}, false
	}
	addrStr = strings.TrimSpace(addrStr)
	if !common.IsHexAddress(addrStr) {
		_ = database.DeleteCache(key)
		return common.Address{}, false
	}
	return common.HexToAddress(addrStr), true
}

func writePrivateContractCache(chain string, walletID uint, kind string, contractAddr common.Address) {
	if database.RedisClient == nil || contractAddr == (common.Address{}) {
		return
	}
	key := privateContractCacheKey(chain, walletID, kind)
	if err := database.SetCache(key, contractAddr.Hex(), privateZapCacheTTL); err != nil {
		log.Printf("[PrivateZap] warning: redis set failed key=%s err=%v", key, err)
	}
}

func (s *LiquidityService) resolveAtomicIncreaseZapAddress(
	exec chainexec.EVMExecutor,
	wallet *models.Wallet,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	opts TxOptions,
) (common.Address, error) {
	if exec == nil {
		return common.Address{}, fmt.Errorf("executor is nil")
	}
	if config.AppConfig == nil {
		return common.Address{}, fmt.Errorf("config not loaded")
	}
	if !config.AppConfig.PrivateZapEnabled {
		return common.Address{}, fmt.Errorf("atomic add liquidity requires PRIVATE_ZAP_ENABLED")
	}
	return s.ensurePrivateAtomicIncreaseZap(exec, wallet, privateKey, walletAddr, opts)
}

func (s *LiquidityService) ensurePrivateAtomicIncreaseZap(
	exec chainexec.EVMExecutor,
	wallet *models.Wallet,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	opts TxOptions,
) (common.Address, error) {
	if exec == nil {
		return common.Address{}, fmt.Errorf("executor is nil")
	}
	if wallet == nil {
		return common.Address{}, fmt.Errorf("wallet is nil")
	}
	if wallet.ID == 0 {
		return common.Address{}, fmt.Errorf("wallet id is 0")
	}
	if privateKey == nil {
		return common.Address{}, fmt.Errorf("privateKey is nil")
	}
	if database.DB == nil {
		return common.Address{}, fmt.Errorf("database not initialized")
	}
	if config.AppConfig == nil {
		return common.Address{}, fmt.Errorf("config not loaded")
	}

	cc := exec.Config()
	chain := config.NormalizeChain(exec.Chain())
	client := exec.Client()
	chainID := exec.ChainID()
	if client == nil || chainID == nil {
		return common.Address{}, fmt.Errorf("blockchain client not initialized")
	}

	if cachedAddr, ok := readPrivateContractCache(chain, wallet.ID, walletChainContractKindAtomicIncreaseZap); ok {
		return cachedAddr, nil
	}

	lock := privateZapLock(chain, wallet.ID)
	lock.Lock()
	defer lock.Unlock()

	if cachedAddr, ok := readPrivateContractCache(chain, wallet.ID, walletChainContractKindAtomicIncreaseZap); ok {
		return cachedAddr, nil
	}

	var binding models.WalletChainContract
	findErr := database.DB.Where("wallet_id = ? AND chain = ? AND kind = ?", wallet.ID, chain, walletChainContractKindAtomicIncreaseZap).
		First(&binding).Error
	haveBinding := false
	zapAddr := common.Address{}
	deployHash := ""
	if findErr == nil {
		haveBinding = true
		addrStr := strings.TrimSpace(binding.ContractAddress)
		if isPrivateContractBindingUsable(binding, privateAtomicIncreaseZapBindingVersion) {
			addr := common.HexToAddress(addrStr)
			if deployed, derr := privateZapHasBytecode(client, addr); derr != nil {
				log.Printf("[PrivateZap] warning: verify atomic binding failed chain=%s wallet_id=%d address=%s err=%v", chain, wallet.ID, addrStr, derr)
				writePrivateContractCache(chain, wallet.ID, walletChainContractKindAtomicIncreaseZap, addr)
				return addr, nil
			} else if deployed {
				writePrivateContractCache(chain, wallet.ID, walletChainContractKindAtomicIncreaseZap, addr)
				return addr, nil
			}
		} else if isPrivateContractBindingConfigPending(binding, privateAtomicIncreaseZapBindingVersion) {
			addr := common.HexToAddress(addrStr)
			if deployed, derr := privateZapHasBytecode(client, addr); derr != nil {
				log.Printf("[PrivateZap] warning: verify pending atomic binding failed chain=%s wallet_id=%d address=%s err=%v", chain, wallet.ID, addrStr, derr)
				zapAddr = addr
				deployHash = strings.TrimSpace(binding.DeployTxHash)
			} else if deployed {
				zapAddr = addr
				deployHash = strings.TrimSpace(binding.DeployTxHash)
			}
		}
	} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return common.Address{}, findErr
	}

	persistBinding := func(contractAddress, status, deployTxHash, configTxHash string) error {
		updates := map[string]interface{}{
			"status":           strings.TrimSpace(status),
			"contract_address": strings.TrimSpace(contractAddress),
			"version":          privateAtomicIncreaseZapBindingVersion,
			"deploy_tx_hash":   strings.TrimSpace(deployTxHash),
			"config_tx_hash":   strings.TrimSpace(configTxHash),
		}

		if haveBinding && binding.ID > 0 {
			if err := database.DB.Model(&binding).Updates(updates).Error; err != nil {
				return err
			}
			binding.Status = updates["status"].(string)
			binding.ContractAddress = updates["contract_address"].(string)
			binding.Version = updates["version"].(int)
			binding.DeployTxHash = updates["deploy_tx_hash"].(string)
			binding.ConfigTxHash = updates["config_tx_hash"].(string)
			return nil
		}

		nowBinding := models.WalletChainContract{
			WalletID:        wallet.ID,
			Chain:           chain,
			Kind:            walletChainContractKindAtomicIncreaseZap,
			Status:          strings.TrimSpace(status),
			ContractAddress: strings.TrimSpace(contractAddress),
			Version:         privateAtomicIncreaseZapBindingVersion,
			DeployTxHash:    strings.TrimSpace(deployTxHash),
			ConfigTxHash:    strings.TrimSpace(configTxHash),
		}
		if err := database.DB.Create(&nowBinding).Error; err != nil {
			var latest models.WalletChainContract
			if rerr := database.DB.Where("wallet_id = ? AND chain = ? AND kind = ?", wallet.ID, chain, walletChainContractKindAtomicIncreaseZap).
				First(&latest).Error; rerr == nil {
				binding = latest
				haveBinding = true
				if uerr := database.DB.Model(&binding).Updates(updates).Error; uerr == nil {
					binding.Status = updates["status"].(string)
					binding.ContractAddress = updates["contract_address"].(string)
					binding.Version = updates["version"].(int)
					binding.DeployTxHash = updates["deploy_tx_hash"].(string)
					binding.ConfigTxHash = updates["config_tx_hash"].(string)
					return nil
				}
			}
			return err
		}
		binding = nowBinding
		haveBinding = true
		return nil
	}

	if !common.IsHexAddress(cc.OKXSwapRouter) {
		return common.Address{}, fmt.Errorf("OKX_SWAP_ROUTER not set for chain=%s", chain)
	}
	if !common.IsHexAddress(cc.OKXTokenApproveAddress) {
		return common.Address{}, fmt.Errorf("OKX_TOKEN_APPROVE_ADDRESS not set for chain=%s", chain)
	}

	v3Primary := strings.TrimSpace(cc.DefaultV3PositionManagerAddress)
	if !common.IsHexAddress(v3Primary) {
		for _, dep := range cc.V3Deployments {
			if common.IsHexAddress(dep.PositionManagerAddress) {
				v3Primary = strings.TrimSpace(dep.PositionManagerAddress)
				break
			}
		}
	}
	if !common.IsHexAddress(v3Primary) {
		return common.Address{}, fmt.Errorf("V3 position manager not configured for chain=%s", chain)
	}

	okxRouter := common.HexToAddress(cc.OKXSwapRouter)
	okxApprove := common.HexToAddress(cc.OKXTokenApproveAddress)
	v3pm := common.HexToAddress(v3Primary)
	v4pm := common.Address{}
	if common.IsHexAddress(cc.UniswapV4PositionManagerAddress) {
		v4pm = common.HexToAddress(cc.UniswapV4PositionManagerAddress)
	}

	if zapAddr == (common.Address{}) {
		nonce, err := blockchain.GetNonceWithClient(client, walletAddr)
		if err != nil {
			return common.Address{}, err
		}
		deployAuth, err := s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
		if err != nil {
			return common.Address{}, err
		}
		tuneZapTxGasLimit("PrivateZap deploy AtomicIncreaseZap", deployAuth, func(o *bind.TransactOpts) (*types.Transaction, error) {
			_, tx, derr := blockchain.DeployAtomicIncreaseZap(o, client)
			return tx, derr
		})

		var deployTx *types.Transaction
		zapAddr, deployTx, err = blockchain.DeployAtomicIncreaseZap(deployAuth, client)
		if err != nil {
			return common.Address{}, fmt.Errorf("deploy AtomicIncreaseZap failed: %w", err)
		}
		deployHash = deployTx.Hash().Hex()
		if _, werr := s.waitMined(client, chainID, deployTx); werr != nil {
			return common.Address{}, fmt.Errorf("deploy AtomicIncreaseZap tx failed: %w", werr)
		}
		if err := persistBinding(zapAddr.Hex(), walletChainContractStatusDeployed, deployHash, ""); err != nil {
			return common.Address{}, fmt.Errorf("persist deployed atomic binding failed: %w", err)
		}
	}

	nonce2, err := blockchain.GetNonceWithClient(client, walletAddr)
	if err != nil {
		return common.Address{}, err
	}
	cfgAuth, err := s.buildAuth(client, chainID, privateKey, nonce2, big.NewInt(0), opts)
	if err != nil {
		return common.Address{}, err
	}
	tuneZapTxGasLimit("PrivateZap setTrustedAddresses (atomic)", cfgAuth, func(o *bind.TransactOpts) (*types.Transaction, error) {
		return blockchain.AtomicIncreaseZapSetTrustedAddresses(o, client, zapAddr, okxRouter, okxApprove, v3pm, v4pm)
	})
	cfgTx, err := blockchain.AtomicIncreaseZapSetTrustedAddresses(cfgAuth, client, zapAddr, okxRouter, okxApprove, v3pm, v4pm)
	if err != nil {
		return common.Address{}, fmt.Errorf("setTrustedAddresses failed: %w", err)
	}
	cfgHash := cfgTx.Hash().Hex()
	if _, werr := s.waitMined(client, chainID, cfgTx); werr != nil {
		return common.Address{}, fmt.Errorf("setTrustedAddresses tx failed: %w", werr)
	}

	var extras []common.Address
	seen := map[common.Address]struct{}{v3pm: {}}
	for _, dep := range cc.V3Deployments {
		pmStr := strings.TrimSpace(dep.PositionManagerAddress)
		if !common.IsHexAddress(pmStr) {
			continue
		}
		pm := common.HexToAddress(pmStr)
		if pm == (common.Address{}) {
			continue
		}
		if _, ok := seen[pm]; ok {
			continue
		}
		seen[pm] = struct{}{}
		extras = append(extras, pm)
	}
	if len(extras) > 0 {
		nonce3, nerr := blockchain.GetNonceWithClient(client, walletAddr)
		if nerr != nil {
			return common.Address{}, nerr
		}
		extraAuth, aerr := s.buildAuth(client, chainID, privateKey, nonce3, big.NewInt(0), opts)
		if aerr != nil {
			return common.Address{}, aerr
		}
		tuneZapTxGasLimit("PrivateZap setTrustedV3PositionManagers (atomic)", extraAuth, func(o *bind.TransactOpts) (*types.Transaction, error) {
			return blockchain.AtomicIncreaseZapSetTrustedV3PositionManagers(o, client, zapAddr, extras, true)
		})
		if tx, terr := blockchain.AtomicIncreaseZapSetTrustedV3PositionManagers(extraAuth, client, zapAddr, extras, true); terr != nil {
			return common.Address{}, fmt.Errorf("setTrustedV3PositionManagers failed: %w", terr)
		} else if tx != nil {
			if _, werr := s.waitMined(client, chainID, tx); werr != nil {
				return common.Address{}, fmt.Errorf("setTrustedV3PositionManagers tx failed: %w", werr)
			}
		}
	}

	if err := persistBinding(zapAddr.Hex(), walletChainContractStatusReady, deployHash, cfgHash); err != nil {
		return common.Address{}, err
	}
	writePrivateContractCache(chain, wallet.ID, walletChainContractKindAtomicIncreaseZap, zapAddr)
	return zapAddr, nil
}
