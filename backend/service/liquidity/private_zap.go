package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

const walletChainContractKindZapSimple = "zap_simple"
const walletChainContractStatusDeployed = "deployed"
const walletChainContractStatusReady = "ready"
const privateZapSimpleBindingVersion = 3
const privateAtomicIncreaseZapBindingVersion = 3
const privateZapCacheTTL = time.Hour
const privateZapSimpleCachePrefix = "private_zap:binding:v3"
const privateAtomicIncreaseZapCachePrefix = "private_atomic_increase_zap:binding:v3"

var privateZapMuByKey sync.Map // key=chain|walletID -> *sync.Mutex

func privateZapLock(chain string, walletID uint) *sync.Mutex {
	key := fmt.Sprintf("%s|%d", config.NormalizeChain(chain), walletID)
	v, _ := privateZapMuByKey.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func privateContractBindingVersion(kind string) int {
	switch strings.TrimSpace(kind) {
	case walletChainContractKindAtomicIncreaseZap:
		return privateAtomicIncreaseZapBindingVersion
	default:
		return privateZapSimpleBindingVersion
	}
}

func privateContractCachePrefix(kind string) string {
	switch strings.TrimSpace(kind) {
	case walletChainContractKindAtomicIncreaseZap:
		return privateAtomicIncreaseZapCachePrefix
	default:
		return privateZapSimpleCachePrefix
	}
}

type zapUsage string

const (
	zapUsageV3 zapUsage = "v3"
	zapUsageV4 zapUsage = "v4"
)

func (s *LiquidityService) resolveZapAddress(
	exec chainexec.EVMExecutor,
	wallet *models.Wallet,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	usage zapUsage,
	opts TxOptions,
) (common.Address, error) {
	if exec == nil {
		return common.Address{}, fmt.Errorf("executor is nil")
	}
	if config.AppConfig == nil {
		return common.Address{}, fmt.Errorf("config not loaded")
	}

	cc := exec.Config()
	chain := exec.Chain()

	if config.AppConfig.PrivateZapEnabled {
		log.Printf("[PrivateZap] resolveZapAddress: mode=private chain=%s wallet_id=%d usage=%s", chain, wallet.ID, usage)
		return s.ensurePrivateZapSimple(exec, wallet, privateKey, walletAddr, opts)
	}

	// Legacy shared Zap contract
	log.Printf("[PrivateZap] resolveZapAddress: mode=legacy chain=%s usage=%s (PRIVATE_ZAP_ENABLED=false)", chain, usage)
	switch usage {
	case zapUsageV4:
		if !common.IsHexAddress(cc.ZapV4Address) {
			return common.Address{}, fmt.Errorf("ZAP_V4_ADDRESS not set for chain=%s", chain)
		}
		return common.HexToAddress(cc.ZapV4Address), nil
	default:
		if !common.IsHexAddress(cc.ZapV3Address) {
			return common.Address{}, fmt.Errorf("ZAP_V3_ADDRESS not set for chain=%s", chain)
		}
		return common.HexToAddress(cc.ZapV3Address), nil
	}
}

func (s *LiquidityService) EnsureWalletPrivateZapReady(userID uint, chain string, walletID uint, walletAddress string, usage string) error {
	if s == nil {
		return fmt.Errorf("liquidity service is nil")
	}
	if config.AppConfig == nil {
		return fmt.Errorf("config not loaded")
	}
	chain = config.NormalizeChain(chain)
	exec, err := chainexec.GetEVM(chain)
	if err != nil {
		return err
	}

	wallet, err := s.walletService.ResolveTaskWallet(userID, walletID, walletAddress)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	privateKeyHex, err := s.walletService.GetPrivateKey(wallet)
	if err != nil {
		return fmt.Errorf("failed to get private key: %w", err)
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	zapUse := zapUsageV3
	if strings.EqualFold(strings.TrimSpace(usage), string(zapUsageV4)) {
		zapUse = zapUsageV4
	}
	_, err = s.resolveZapAddress(exec, wallet, privateKey, s.walletService.GetWalletAddress(wallet), zapUse, TxOptions{})
	return err
}

func (s *LiquidityService) ShouldShowWalletPrivateZapProtectionHint(chain string, walletID uint) (bool, error) {
	if !config.AppConfig.PrivateZapEnabled || walletID == 0 {
		return false, nil
	}
	if database.DB == nil {
		return false, fmt.Errorf("database not initialized")
	}
	chain = config.NormalizeChain(chain)
	if cachedAddr, ok := readPrivateZapCache(chain, walletID); ok && cachedAddr != (common.Address{}) {
		return false, nil
	}

	var binding models.WalletChainContract
	err := database.DB.Where("wallet_id = ? AND chain = ? AND kind = ?", walletID, chain, walletChainContractKindZapSimple).
		First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true, nil
		}
		return false, err
	}
	if isPrivateZapBindingUsable(binding) || isPrivateZapBindingConfigPending(binding) {
		return false, nil
	}
	return true, nil
}

func privateZapCacheKey(chain string, walletID uint) string {
	return fmt.Sprintf("%s:%s:%d:%s",
		privateZapSimpleCachePrefix,
		config.NormalizeChain(chain),
		walletID,
		walletChainContractKindZapSimple,
	)
}

func privateZapCacheScanPattern(chain string) string {
	return fmt.Sprintf("%s:%s:*", privateZapSimpleCachePrefix, config.NormalizeChain(chain))
}

func readPrivateZapCache(chain string, walletID uint) (common.Address, bool) {
	if database.RedisClient == nil {
		return common.Address{}, false
	}
	key := privateZapCacheKey(chain, walletID)
	addrStr, err := database.GetCache(key)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.Printf("[PrivateZap] warning: redis get failed key=%s err=%v", key, err)
		}
		return common.Address{}, false
	}
	addrStr = strings.TrimSpace(addrStr)
	if !common.IsHexAddress(addrStr) {
		_ = database.DeleteCache(key)
		return common.Address{}, false
	}
	return common.HexToAddress(addrStr), true
}

func writePrivateZapCache(chain string, walletID uint, zapAddr common.Address) {
	if database.RedisClient == nil || zapAddr == (common.Address{}) {
		return
	}
	key := privateZapCacheKey(chain, walletID)
	if err := database.SetCache(key, zapAddr.Hex(), privateZapCacheTTL); err != nil {
		log.Printf("[PrivateZap] warning: redis set failed key=%s err=%v", key, err)
	}
}

func normalizePrivateZapBindingStatus(binding models.WalletChainContract) string {
	status := strings.ToLower(strings.TrimSpace(binding.Status))
	switch status {
	case walletChainContractStatusDeployed, walletChainContractStatusReady:
		return status
	case "":
		if common.IsHexAddress(strings.TrimSpace(binding.ContractAddress)) {
			// Existing rows created before status tracking are treated as ready.
			return walletChainContractStatusReady
		}
	}
	return status
}

func isPrivateContractBindingUsable(binding models.WalletChainContract, requiredVersion int) bool {
	if !common.IsHexAddress(strings.TrimSpace(binding.ContractAddress)) {
		return false
	}
	if binding.Version < requiredVersion {
		return false
	}
	return normalizePrivateZapBindingStatus(binding) == walletChainContractStatusReady
}

func isPrivateContractBindingConfigPending(binding models.WalletChainContract, requiredVersion int) bool {
	if !common.IsHexAddress(strings.TrimSpace(binding.ContractAddress)) {
		return false
	}
	if binding.Version < requiredVersion {
		return false
	}
	return normalizePrivateZapBindingStatus(binding) == walletChainContractStatusDeployed
}

func isPrivateZapBindingUsable(binding models.WalletChainContract) bool {
	return isPrivateContractBindingUsable(binding, privateZapSimpleBindingVersion)
}

func isPrivateZapBindingConfigPending(binding models.WalletChainContract) bool {
	return isPrivateContractBindingConfigPending(binding, privateZapSimpleBindingVersion)
}

func privateZapHasBytecode(client *ethclient.Client, zapAddr common.Address) (bool, error) {
	if client == nil {
		return false, fmt.Errorf("blockchain client not initialized")
	}
	if zapAddr == (common.Address{}) {
		return false, nil
	}
	code, err := client.CodeAt(context.Background(), zapAddr, nil)
	if err != nil {
		return false, err
	}
	return len(code) > 0, nil
}

func (s *LiquidityService) ensurePrivateZapSimple(
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

	// Fast path: read from redis cache first.
	if cachedAddr, ok := readPrivateZapCache(chain, wallet.ID); ok {
		return cachedAddr, nil
	}

	lock := privateZapLock(chain, wallet.ID)
	lock.Lock()
	defer lock.Unlock()

	// Re-check cache after acquiring lock to reduce duplicate DB reads under concurrency.
	if cachedAddr, ok := readPrivateZapCache(chain, wallet.ID); ok {
		return cachedAddr, nil
	}

	var binding models.WalletChainContract
	findErr := database.DB.Where("wallet_id = ? AND chain = ? AND kind = ?", wallet.ID, chain, walletChainContractKindZapSimple).
		First(&binding).Error
	haveBinding := false
	zapAddr := common.Address{}
	deployHash := ""
	if findErr == nil {
		haveBinding = true
		addrStr := strings.TrimSpace(binding.ContractAddress)
		if isPrivateZapBindingUsable(binding) {
			addr := common.HexToAddress(addrStr)
			if deployed, derr := privateZapHasBytecode(client, addr); derr != nil {
				log.Printf("[PrivateZap] warning: verify existing binding failed chain=%s wallet_id=%d address=%s err=%v", chain, wallet.ID, addrStr, derr)
				writePrivateZapCache(chain, wallet.ID, addr)
				return addr, nil
			} else if deployed {
				log.Printf("[PrivateZap] using existing binding chain=%s wallet_id=%d address=%s", chain, wallet.ID, addrStr)
				writePrivateZapCache(chain, wallet.ID, addr)
				return addr, nil
			} else {
				log.Printf("[PrivateZap] binding ready but bytecode missing: chain=%s wallet_id=%d address=%s -> will redeploy", chain, wallet.ID, addrStr)
			}
		} else if isPrivateZapBindingConfigPending(binding) {
			addr := common.HexToAddress(addrStr)
			if deployed, derr := privateZapHasBytecode(client, addr); derr != nil {
				log.Printf("[PrivateZap] warning: verify pending binding failed chain=%s wallet_id=%d address=%s err=%v", chain, wallet.ID, addrStr, derr)
				zapAddr = addr
				deployHash = strings.TrimSpace(binding.DeployTxHash)
			} else if deployed {
				zapAddr = addr
				deployHash = strings.TrimSpace(binding.DeployTxHash)
				log.Printf("[PrivateZap] resuming config for deployed binding chain=%s wallet_id=%d address=%s", chain, wallet.ID, addrStr)
			} else {
				log.Printf("[PrivateZap] pending binding bytecode missing: chain=%s wallet_id=%d address=%s -> will redeploy", chain, wallet.ID, addrStr)
			}
		}
		if zapAddr == (common.Address{}) {
			log.Printf("[PrivateZap] binding missing or invalid: chain=%s wallet_id=%d stored_address=%q status=%q -> will redeploy", chain, wallet.ID, addrStr, strings.TrimSpace(binding.Status))
		}
	} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return common.Address{}, findErr
	}

	persistBinding := func(contractAddress, status, deployTxHash, configTxHash string) error {
		updates := map[string]interface{}{
			"status":           strings.TrimSpace(status),
			"contract_address": strings.TrimSpace(contractAddress),
			"version":          privateZapSimpleBindingVersion,
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
			Kind:            walletChainContractKindZapSimple,
			Status:          strings.TrimSpace(status),
			ContractAddress: strings.TrimSpace(contractAddress),
			Version:         privateZapSimpleBindingVersion,
			DeployTxHash:    strings.TrimSpace(deployTxHash),
			ConfigTxHash:    strings.TrimSpace(configTxHash),
		}
		if err := database.DB.Create(&nowBinding).Error; err != nil {
			var latest models.WalletChainContract
			if rerr := database.DB.Where("wallet_id = ? AND chain = ? AND kind = ?", wallet.ID, chain, walletChainContractKindZapSimple).
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

	// Validate required chain-scoped trusted addresses before deploying.
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
	wrappedNative := common.Address{}
	if common.IsHexAddress(cc.WrappedNativeAddress) {
		wrappedNative = common.HexToAddress(cc.WrappedNativeAddress)
	}

	if zapAddr == (common.Address{}) {
		log.Printf("[PrivateZap] deploying zap_simple chain=%s wallet_id=%d wallet=%s", chain, wallet.ID, walletAddr.Hex())

		// 1) Deploy contract (nonce N)
		nonce, err := blockchain.GetNonceWithClient(client, walletAddr)
		if err != nil {
			return common.Address{}, err
		}
		deployAuth, err := s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
		if err != nil {
			return common.Address{}, err
		}
		tuneZapTxGasLimit("PrivateZap deploy ZapSimple", deployAuth, func(o *bind.TransactOpts) (*types.Transaction, error) {
			_, tx, derr := blockchain.DeployZapSimple(o, client)
			return tx, derr
		})

		var deployTx *types.Transaction
		zapAddr, deployTx, err = blockchain.DeployZapSimple(deployAuth, client)
		if err != nil {
			return common.Address{}, fmt.Errorf("deploy ZapSimple failed: %w", err)
		}
		deployHash = deployTx.Hash().Hex()
		if _, werr := s.waitMined(client, chainID, deployTx); werr != nil {
			return common.Address{}, fmt.Errorf("deploy ZapSimple tx failed: %w", werr)
		}
		log.Printf("[PrivateZap] deployed zap_simple chain=%s wallet_id=%d address=%s tx=%s", chain, wallet.ID, zapAddr.Hex(), deployHash)
		if err := persistBinding(zapAddr.Hex(), walletChainContractStatusDeployed, deployHash, ""); err != nil {
			return common.Address{}, fmt.Errorf("persist deployed private zap binding failed: %w", err)
		}
	} else {
		log.Printf("[PrivateZap] configuring existing deployed zap_simple chain=%s wallet_id=%d address=%s", chain, wallet.ID, zapAddr.Hex())
	}

	// 2) Configure trusted addresses (nonce N+1 ...)
	nonce2, err := blockchain.GetNonceWithClient(client, walletAddr)
	if err != nil {
		return common.Address{}, err
	}
	cfgAuth, err := s.buildAuth(client, chainID, privateKey, nonce2, big.NewInt(0), opts)
	if err != nil {
		return common.Address{}, err
	}
	tuneZapTxGasLimit("PrivateZap setTrustedAddresses", cfgAuth, func(o *bind.TransactOpts) (*types.Transaction, error) {
		return blockchain.ZapSimpleSetTrustedAddresses(o, client, zapAddr, okxRouter, okxApprove, v3pm, v4pm)
	})
	cfgTx, err := blockchain.ZapSimpleSetTrustedAddresses(cfgAuth, client, zapAddr, okxRouter, okxApprove, v3pm, v4pm)
	if err != nil {
		return common.Address{}, fmt.Errorf("setTrustedAddresses failed: %w", err)
	}
	cfgHash := cfgTx.Hash().Hex()
	if _, werr := s.waitMined(client, chainID, cfgTx); werr != nil {
		return common.Address{}, fmt.Errorf("setTrustedAddresses tx failed: %w", werr)
	}

	if wrappedNative != (common.Address{}) {
		nonceWrapped, nerr := blockchain.GetNonceWithClient(client, walletAddr)
		if nerr != nil {
			return common.Address{}, nerr
		}
		wrappedAuth, aerr := s.buildAuth(client, chainID, privateKey, nonceWrapped, big.NewInt(0), opts)
		if aerr != nil {
			return common.Address{}, aerr
		}
		tuneZapTxGasLimit("PrivateZap setWrappedNative", wrappedAuth, func(o *bind.TransactOpts) (*types.Transaction, error) {
			return blockchain.ZapSimpleSetWrappedNative(o, client, zapAddr, wrappedNative)
		})
		wrappedTx, werr := blockchain.ZapSimpleSetWrappedNative(wrappedAuth, client, zapAddr, wrappedNative)
		if werr != nil {
			return common.Address{}, fmt.Errorf("setWrappedNative failed: %w", werr)
		}
		cfgHash = wrappedTx.Hash().Hex()
		if _, werr := s.waitMined(client, chainID, wrappedTx); werr != nil {
			return common.Address{}, fmt.Errorf("setWrappedNative tx failed: %w", werr)
		}
	}

	// 3) Allowlist additional V3 position managers (optional).
	var extras []common.Address
	seen := make(map[common.Address]struct{})
	seen[v3pm] = struct{}{}
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
		tuneZapTxGasLimit("PrivateZap setTrustedV3PositionManagers", extraAuth, func(o *bind.TransactOpts) (*types.Transaction, error) {
			return blockchain.ZapSimpleSetTrustedV3PositionManagers(o, client, zapAddr, extras, true)
		})
		if tx, terr := blockchain.ZapSimpleSetTrustedV3PositionManagers(extraAuth, client, zapAddr, extras, true); terr != nil {
			return common.Address{}, fmt.Errorf("setTrustedV3PositionManagers failed: %w", terr)
		} else if tx != nil {
			if _, werr := s.waitMined(client, chainID, tx); werr != nil {
				return common.Address{}, fmt.Errorf("setTrustedV3PositionManagers tx failed: %w", werr)
			}
		}
	}

	if err := persistBinding(zapAddr.Hex(), walletChainContractStatusReady, deployHash, cfgHash); err != nil {
		var latest models.WalletChainContract
		if rerr := database.DB.Where("wallet_id = ? AND chain = ? AND kind = ?", wallet.ID, chain, walletChainContractKindZapSimple).
			First(&latest).Error; rerr == nil && isPrivateZapBindingUsable(latest) {
			addr := common.HexToAddress(latest.ContractAddress)
			writePrivateZapCache(chain, wallet.ID, addr)
			return addr, nil
		}
		return common.Address{}, err
	}

	log.Printf("[PrivateZap] bound zap_simple chain=%s wallet_id=%d address=%s", chain, wallet.ID, zapAddr.Hex())
	writePrivateZapCache(chain, wallet.ID, zapAddr)
	return zapAddr, nil
}
