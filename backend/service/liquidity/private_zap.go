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
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

const walletChainContractKindZapSimple = "zap_simple"
const privateZapCacheTTL = time.Hour
const privateZapCachePrefix = "private_zap:binding"

var privateZapMuByKey sync.Map // key=chain|walletID -> *sync.Mutex

func privateZapLock(chain string, walletID uint) *sync.Mutex {
	key := fmt.Sprintf("%s|%d", config.NormalizeChain(chain), walletID)
	v, _ := privateZapMuByKey.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
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
		return s.ensurePrivateZapSimple(exec, wallet, privateKey, walletAddr, opts)
	}

	// Legacy shared Zap contract
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

func requiredPrivateZapVersion(cc config.ChainConfig) int {
	v := cc.PrivateZapVersion
	if v <= 0 {
		v = 1
	}
	return v
}

func privateZapCacheKey(chain string, walletID uint, version int) string {
	v := version
	if v <= 0 {
		v = 1
	}
	return fmt.Sprintf("%s:%s:%d:%s:v%d",
		privateZapCachePrefix,
		config.NormalizeChain(chain),
		walletID,
		walletChainContractKindZapSimple,
		v,
	)
}

func readPrivateZapCache(chain string, walletID uint, version int) (common.Address, bool) {
	if database.RedisClient == nil {
		return common.Address{}, false
	}
	key := privateZapCacheKey(chain, walletID, version)
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

func writePrivateZapCache(chain string, walletID uint, version int, zapAddr common.Address) {
	if database.RedisClient == nil || zapAddr == (common.Address{}) {
		return
	}
	key := privateZapCacheKey(chain, walletID, version)
	if err := database.SetCache(key, zapAddr.Hex(), privateZapCacheTTL); err != nil {
		log.Printf("[PrivateZap] warning: redis set failed key=%s err=%v", key, err)
	}
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
	wantVersion := requiredPrivateZapVersion(cc)
	client := exec.Client()
	chainID := exec.ChainID()
	if client == nil || chainID == nil {
		return common.Address{}, fmt.Errorf("blockchain client not initialized")
	}

	// Fast path: read from redis cache first.
	if cachedAddr, ok := readPrivateZapCache(chain, wallet.ID, wantVersion); ok {
		return cachedAddr, nil
	}

	lock := privateZapLock(chain, wallet.ID)
	lock.Lock()
	defer lock.Unlock()

	// Re-check cache after acquiring lock to reduce duplicate DB reads under concurrency.
	if cachedAddr, ok := readPrivateZapCache(chain, wallet.ID, wantVersion); ok {
		return cachedAddr, nil
	}

	var binding models.WalletChainContract
	findErr := database.DB.Where("wallet_id = ? AND chain = ? AND kind = ?", wallet.ID, chain, walletChainContractKindZapSimple).
		First(&binding).Error
	haveBinding := false
	if findErr == nil {
		haveBinding = true
		addrStr := strings.TrimSpace(binding.ContractAddress)
		if binding.Version == wantVersion && common.IsHexAddress(addrStr) {
			addr := common.HexToAddress(addrStr)
			writePrivateZapCache(chain, wallet.ID, wantVersion, addr)
			return addr, nil
		}
	} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return common.Address{}, findErr
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

	log.Printf("[PrivateZap] deploying zap_simple chain=%s wallet_id=%d wallet=%s version=%d", chain, wallet.ID, walletAddr.Hex(), wantVersion)

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

	zapAddr, deployTx, err := blockchain.DeployZapSimple(deployAuth, client)
	if err != nil {
		return common.Address{}, fmt.Errorf("deploy ZapSimple failed: %w", err)
	}
	deployHash := deployTx.Hash().Hex()
	if _, werr := s.waitMined(client, chainID, deployTx); werr != nil {
		return common.Address{}, fmt.Errorf("deploy ZapSimple tx failed: %w", werr)
	}
	log.Printf("[PrivateZap] deployed zap_simple chain=%s wallet_id=%d address=%s tx=%s", chain, wallet.ID, zapAddr.Hex(), deployHash)

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

	// 4) Persist binding (overwrite old binding on upgrade).
	nowBinding := models.WalletChainContract{
		WalletID:        wallet.ID,
		Chain:           chain,
		Kind:            walletChainContractKindZapSimple,
		ContractAddress: zapAddr.Hex(),
		Version:         wantVersion,
		DeployTxHash:    deployHash,
		ConfigTxHash:    cfgHash,
	}

	// Update existing row if present, otherwise create.
	if haveBinding && binding.ID > 0 {
		if uerr := database.DB.Model(&binding).Updates(map[string]interface{}{
			"contract_address": nowBinding.ContractAddress,
			"version":          nowBinding.Version,
			"deploy_tx_hash":   nowBinding.DeployTxHash,
			"config_tx_hash":   nowBinding.ConfigTxHash,
		}).Error; uerr != nil {
			return common.Address{}, uerr
		}
	} else {
		if cerr := database.DB.Create(&nowBinding).Error; cerr != nil {
			// If another concurrent flow won the race and inserted, read and use that binding.
			var latest models.WalletChainContract
			if rerr := database.DB.Where("wallet_id = ? AND chain = ? AND kind = ?", wallet.ID, chain, walletChainContractKindZapSimple).
				First(&latest).Error; rerr == nil && common.IsHexAddress(latest.ContractAddress) && latest.Version == wantVersion {
				addr := common.HexToAddress(latest.ContractAddress)
				writePrivateZapCache(chain, wallet.ID, wantVersion, addr)
				return addr, nil
			}
			return common.Address{}, cerr
		}
	}

	log.Printf("[PrivateZap] bound zap_simple chain=%s wallet_id=%d address=%s version=%d", chain, wallet.ID, zapAddr.Hex(), wantVersion)
	writePrivateZapCache(chain, wallet.ID, wantVersion, zapAddr)
	return zapAddr, nil
}
