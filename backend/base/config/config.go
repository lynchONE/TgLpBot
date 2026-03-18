package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"

	"TgLpBot/base/security"

	"github.com/joho/godotenv"
)

type ChainKind string

const (
	ChainKindEVM    ChainKind = "evm"
	ChainKindSolana ChainKind = "solana"
)

type V3DeploymentConfig struct {
	Name                   string
	FactoryAddress         string
	PositionManagerAddress string
}

// ChainConfig defines per-chain runtime configuration.
// For now we only execute EVM chains (bsc/base); the shape is designed to keep
// chain parameters centralized and allow future non-EVM executors (e.g. Solana).
type ChainConfig struct {
	Chain string
	Kind  ChainKind

	RpcURL   string
	RpcWSURL string
	ChainID  int64

	// PrivateZapVersion is a legacy compatibility field retained in config/env.
	// Private Zap invalidation is now admin-triggered by clearing stored bindings,
	// so runtime resolution no longer compares versions.
	PrivateZapVersion int

	StableSymbol   string
	StableAddress  string
	StableDecimals int

	// Optional secondary stables (used for entry-token planning and stable-side detection).
	USDTAddress string
	USDCAddress string
	BUSDAddress string

	WrappedNativeSymbol  string
	WrappedNativeAddress string

	// OKX DEX allowlist (optional, but strongly recommended).
	OKXSwapRouter          string
	OKXTokenApproveAddress string

	// Zap contracts (V3/V4 can be same address).
	ZapV3Address string
	ZapV4Address string

	// Uniswap V4 addresses (optional per chain).
	UniswapV4PoolManagerAddress     string
	UniswapV4StateViewAddress       string
	UniswapV4PositionManagerAddress string

	// V3 deployments (Uniswap/Pancake/Aerodrome Slipstream etc).
	V3Deployments                   []V3DeploymentConfig
	DefaultV3PositionManagerAddress string

	// Explorer URL template: fmt.Sprintf(template, txHash)
	ExplorerTxURLTemplate string
}

type Config struct {
	// Telegram
	TelegramBotToken                 string
	TelegramWebAppURL                string
	TelegramMenuButtonMode           string // commands|default|web_app
	TelegramWebAppAllowEmptyInitData bool
	TelegramWebAppDebugUserID        int64
	TelegramWebAppDebugUsername      string

	// Access Control
	AdminWalletAddress string

	// Uniswap V4
	UniswapV4PoolManagerAddress     string
	UniswapV4StateViewAddress       string
	UniswapV4PositionManagerAddress string
	UniswapV4Debug                  bool

	// BSC Network
	BSCRpcURL   string
	BSCRpcWSURL string
	BSCChainID  int64

	// Database
	MySQLHost     string
	MySQLPort     string
	MySQLUser     string
	MySQLPassword string
	MySQLDatabase string

	// Redis
	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int

	// OKX DEX API
	OKXDexAPIURL              string
	OKXAPIKey                 string
	OKXSecretKey              string
	OKXPassphrase             string
	OKXSwapRouter             string
	OKXTokenApproveAddress    string // OKX DEX 鐨?TokenApprove 鍚堢害鍦板潃
	OKXDebug                  bool
	OKXSwapGasLimitMultiplier float64
	OKXSwapGasLimitMin        uint64
	OKXSwapGasLimitMax        uint64

	// Zap (V3/V4): GasLimit safety buffer (avoid out of gas / reentrancy sentry)
	ZapGasLimitMultiplier float64
	ZapGasLimitMin        uint64
	ZapGasLimitMax        uint64

	// Private per-wallet Zap contracts (deploy + bind).
	PrivateZapEnabled bool
	// Legacy compatibility field; runtime no longer uses version comparison for invalidation.
	PrivateZapVersion int

	// Contracts
	ZapV3Address string
	ZapV4Address string

	// V3 Position Managers (optional defaults)
	PancakeV3PositionManagerAddress string
	UniswapV3PositionManagerAddress string

	// Encryption
	EncryptionKey string

	// Liquidity exit balance sync (RPC lag handling)
	ExitTokenSyncTimeoutSeconds int
	ExitTokenSyncPollMillis     int

	// Workers / Concurrency
	WorkerMaxParallelUsers int // max concurrent per-user jobs (strategy monitor)
	WalletTxMaxParallel    int // max concurrent wallets doing on-chain tx (per-wallet is still serialized)

	// Token Addresses
	USDTAddress      string
	USDCAddress      string
	BUSDAddress      string
	WBNBAddress      string
	PancakeRouterV2  string
	PancakeFactoryV2 string

	// V3 Swap Router (閾句笂 swap 鐢?
	PancakeV3SwapRouter string
	UniswapV3SwapRouter string

	// Mini App / Realtime positions
	V4NFTScanFromBlock   uint64
	RealtimeV3NFTScan    bool
	RealtimeV3NFTScanMax int

	// Pools sync
	PoolsSyncEnabled          bool
	PoolsSyncPoolMBaseURL     string
	PoolsSyncChain            string
	PoolsSyncDexes            string
	PoolsSyncIntervalSeconds  int
	PoolsSyncFetchDelayMillis int
	PoolsRetentionHours       int

	// Smart LP (monitor external contract -> NPM events)
	SmartLPEnabled             bool
	SmartLPDebug               bool
	SmartLPContractAddress     string
	SmartLPScorePerWallet      float64
	SmartLPMinWallets          int
	SmartLPRecentWindowMinutes int // 鑷姩寮€鍗曟墍闇€鐨勬椂闂寸獥鍙ｏ紙鍒嗛挓锛夛紝榛樿10鍒嗛挓
	SmartLPScanIntervalSeconds int
	SmartLPMaxBlocksPerScan    int
	SmartLPRPCTimeoutSeconds   int
	SmartLPScanTimeoutSeconds  int

	// Multi-chain (single instance). Chains are keyed by lower-case chain slug (e.g. "bsc", "base").
	EnabledChains []string
	Chains        map[string]ChainConfig
}

var AppConfig *Config

func LoadConfig() error {
	log.Println("========================================")
	log.Println("项目启动中..")
	log.Println("========================================")

	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("env加载失败")
	} else {
		log.Println("env加载成功")
	}

	chainID, _ := strconv.ParseInt(getEnv("BSC_CHAIN_ID", "56"), 10, 64)
	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	v4NFTScanFromBlock, _ := strconv.ParseUint(strings.TrimSpace(getEnv("V4_NFT_SCAN_FROM_BLOCK", "0")), 10, 64)
	realtimeV3NFTScanMax, _ := strconv.Atoi(strings.TrimSpace(getEnv("REALTIME_V3_NFT_SCAN_MAX", "20")))
	poolsSyncIntervalSeconds, _ := strconv.Atoi(strings.TrimSpace(getEnv("POOLS_SYNC_INTERVAL_SECONDS", "60")))
	poolsSyncFetchDelayMillis, _ := strconv.Atoi(strings.TrimSpace(getEnv("POOLS_SYNC_FETCH_DELAY_MILLIS", "250")))
	poolsRetentionHours, _ := strconv.Atoi(strings.TrimSpace(getEnv("POOLS_RETENTION_HOURS", "24")))
	smartLPDebug := getEnvBool("SMART_LP_DEBUG", false)
	smartLPScorePerWallet, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("SMART_LP_SCORE_PER_WALLET", "100")), 64)
	smartLPMinWallets, _ := strconv.Atoi(strings.TrimSpace(getEnv("SMART_LP_MIN_WALLETS", "3")))
	smartLPScanInterval, _ := strconv.Atoi(strings.TrimSpace(getEnv("SMART_LP_SCAN_INTERVAL_SECONDS", "60")))
	smartLPRecentWindowMinutes, _ := strconv.Atoi(strings.TrimSpace(getEnv("SMART_LP_RECENT_WINDOW_MINUTES", "10")))
	smartLPMaxBlocksPerScan, _ := strconv.Atoi(strings.TrimSpace(getEnv("SMART_LP_MAX_BLOCKS_PER_SCAN", "200")))
	smartLPRPCTimeoutSeconds, _ := strconv.Atoi(strings.TrimSpace(getEnv("SMART_LP_RPC_TIMEOUT_SECONDS", "30")))
	smartLPScanTimeoutSeconds, _ := strconv.Atoi(strings.TrimSpace(getEnv("SMART_LP_SCAN_TIMEOUT_SECONDS", "600")))
	exitTokenSyncTimeoutSeconds, _ := strconv.Atoi(strings.TrimSpace(getEnv("EXIT_TOKEN_SYNC_TIMEOUT_SECONDS", "30")))
	exitTokenSyncPollMillis, _ := strconv.Atoi(strings.TrimSpace(getEnv("EXIT_TOKEN_SYNC_POLL_MILLIS", "500")))
	okxSwapGasLimitMult, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("OKX_SWAP_GAS_LIMIT_MULTIPLIER", "1.30")), 64)
	okxSwapGasLimitMin, _ := strconv.ParseUint(strings.TrimSpace(getEnv("OKX_SWAP_GAS_LIMIT_MIN", "0")), 10, 64)
	okxSwapGasLimitMax, _ := strconv.ParseUint(strings.TrimSpace(getEnv("OKX_SWAP_GAS_LIMIT_MAX", "0")), 10, 64)
	zapGasLimitMult, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("ZAP_GAS_LIMIT_MULTIPLIER", "1.30")), 64)
	zapGasLimitMin, _ := strconv.ParseUint(strings.TrimSpace(getEnv("ZAP_GAS_LIMIT_MIN", "0")), 10, 64)
	zapGasLimitMax, _ := strconv.ParseUint(strings.TrimSpace(getEnv("ZAP_GAS_LIMIT_MAX", "0")), 10, 64)
	workerMaxParallelUsers, _ := strconv.Atoi(strings.TrimSpace(getEnv("WORKER_MAX_PARALLEL_USERS", "16")))
	walletTxMaxParallel, _ := strconv.Atoi(strings.TrimSpace(getEnv("WALLET_TX_MAX_PARALLEL", "8")))
	webAppDebugUserID, _ := strconv.ParseInt(strings.TrimSpace(getEnv("TELEGRAM_WEBAPP_DEBUG_USER_ID", "0")), 10, 64)
	webAppDebugUsername := strings.TrimSpace(getEnv("TELEGRAM_WEBAPP_DEBUG_USERNAME", "local_debug"))

	if workerMaxParallelUsers <= 0 {
		workerMaxParallelUsers = 1
	}
	if walletTxMaxParallel <= 0 {
		walletTxMaxParallel = 1
	}
	if poolsSyncIntervalSeconds <= 0 {
		poolsSyncIntervalSeconds = 60
	}
	if poolsSyncFetchDelayMillis < 0 {
		poolsSyncFetchDelayMillis = 250
	}
	if poolsRetentionHours <= 0 {
		poolsRetentionHours = 24
	}
	AppConfig = &Config{
		// Telegram
		TelegramBotToken:                 getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramWebAppURL:                normalizeTelegramWebAppURL(getEnv("TELEGRAM_WEBAPP_URL", "")),
		TelegramMenuButtonMode:           normalizeTelegramMenuButtonMode(getEnv("TELEGRAM_MENU_BUTTON_MODE", "commands")),
		TelegramWebAppAllowEmptyInitData: getEnvBool("TELEGRAM_WEBAPP_ALLOW_EMPTY_INITDATA", false),
		TelegramWebAppDebugUserID:        webAppDebugUserID,
		TelegramWebAppDebugUsername:      webAppDebugUsername,

		// Access Control
		AdminWalletAddress: strings.TrimSpace(getEnv("ADMIN_WALLET_ADDRESS", "")),

		// Uniswap V4
		UniswapV4PoolManagerAddress:     getEnv("UNISWAP_V4_POOL_MANAGER_ADDRESS", ""),
		UniswapV4StateViewAddress:       getEnv("UNISWAP_V4_STATE_VIEW_ADDRESS", ""),
		UniswapV4PositionManagerAddress: getEnv("UNISWAP_V4_POSITION_MANAGER_ADDRESS", ""),
		UniswapV4Debug:                  getEnvBool("UNISWAP_V4_DEBUG", false),

		// BSC Network
		BSCRpcURL:   getEnv("BSC_RPC_URL", "https://bsc-dataseed1.binance.org/"),
		BSCRpcWSURL: strings.TrimSpace(getEnv("BSC_RPC_WS_URL", "")),
		BSCChainID:  chainID,

		// Database
		MySQLHost:     getEnv("MYSQL_HOST", "localhost"),
		MySQLPort:     getEnv("MYSQL_PORT", "3306"),
		MySQLUser:     getEnv("MYSQL_USER", "root"),
		MySQLPassword: getEnv("MYSQL_PASSWORD", ""),
		MySQLDatabase: getEnv("MYSQL_DATABASE", "tglpbot"),

		// Redis
		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       redisDB,

		// OKX DEX API
		OKXDexAPIURL:              getEnv("OKX_DEX_API_URL", "https://www.okx.com/api/v6/dex/aggregator"),
		OKXAPIKey:                 getEnv("OKX_API_KEY", ""),
		OKXSecretKey:              getEnv("OKX_SECRET_KEY", ""),
		OKXPassphrase:             getEnv("OKX_PASSPHRASE", ""),
		OKXSwapRouter:             getEnv("OKX_SWAP_ROUTER", ""),
		OKXTokenApproveAddress:    getEnv("OKX_TOKEN_APPROVE_ADDRESS", ""),
		OKXDebug:                  getEnvBool("OKX_DEBUG", false),
		OKXSwapGasLimitMultiplier: okxSwapGasLimitMult,
		OKXSwapGasLimitMin:        okxSwapGasLimitMin,
		OKXSwapGasLimitMax:        okxSwapGasLimitMax,

		// Zap (V3/V4): GasLimit safety buffer
		ZapGasLimitMultiplier: zapGasLimitMult,
		ZapGasLimitMin:        zapGasLimitMin,
		ZapGasLimitMax:        zapGasLimitMax,

		// Private per-wallet Zap contracts
		PrivateZapEnabled: getEnvBool("PRIVATE_ZAP_ENABLED", false),
		PrivateZapVersion: getEnvInt("PRIVATE_ZAP_VERSION", 1),

		// Contracts
		ZapV3Address: getEnv("ZAP_V3_ADDRESS", ""),
		ZapV4Address: getEnv("ZAP_V4_ADDRESS", ""),

		// V3 Position Managers
		PancakeV3PositionManagerAddress: getEnv("PANCAKE_V3_NPM_ADDRESS", ""),
		UniswapV3PositionManagerAddress: getEnv("UNISWAP_V3_NPM_ADDRESS", ""),

		// Encryption
		EncryptionKey: security.NormalizeHexString(getEnv("ENCRYPTION_KEY", "")),

		ExitTokenSyncTimeoutSeconds: exitTokenSyncTimeoutSeconds,
		ExitTokenSyncPollMillis:     exitTokenSyncPollMillis,

		WorkerMaxParallelUsers: workerMaxParallelUsers,
		WalletTxMaxParallel:    walletTxMaxParallel,

		// Token Addresses
		USDTAddress:      getEnv("USDT_ADDRESS", "0x55d398326f99059fF775485246999027B3197955"),
		USDCAddress:      getEnv("USDC_ADDRESS", "0x8ac76a51cc950d9822d68b83fe1ad97b32cd580d"),
		BUSDAddress:      getEnv("BUSD_ADDRESS", "0xe9e7CEA3DedcA5984780Bafc599bD69ADd087D56"),
		WBNBAddress:      getEnv("WBNB_ADDRESS", "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c"),
		PancakeRouterV2:  getEnv("PANCAKE_ROUTER_V2", "0x10ED43C718714eb63d5aA57B78B54704E256024E"),
		PancakeFactoryV2: getEnv("PANCAKE_FACTORY_V2", "0xcA143Ce32Fe78f1f7019d7d551a6402fC5350c73"),

		// V3 Swap Router (PancakeSwap V3 SwapRouter on BSC)
		PancakeV3SwapRouter: getEnv("PANCAKE_V3_SWAP_ROUTER", "0x1b81D678ffb9C0263b24A97847620C99d213eB14"),
		// V3 Swap Router (Uniswap V3 SwapRouter02 on BSC)
		UniswapV3SwapRouter: getEnv("UNISWAP_V3_SWAP_ROUTER", "0xB971eF87ede563556b2ED4b1C0b0019111Dd85d2"),

		// Mini App / Realtime positions
		V4NFTScanFromBlock:         v4NFTScanFromBlock,
		RealtimeV3NFTScan:          getEnvBool("REALTIME_V3_NFT_SCAN", false),
		RealtimeV3NFTScanMax:       realtimeV3NFTScanMax,
		PoolsSyncEnabled:           getEnvBool("POOLS_SYNC_ENABLED", true),
		PoolsSyncPoolMBaseURL:      strings.TrimSpace(getEnv("POOLS_SYNC_POOLM_BASE_URL", "")),
		PoolsSyncChain:             strings.TrimSpace(getEnv("POOLS_SYNC_CHAIN", "bsc")),
		PoolsSyncDexes:             strings.TrimSpace(getEnv("POOLS_SYNC_DEXES", "pcsv3,univ3,univ4")),
		PoolsSyncIntervalSeconds:   poolsSyncIntervalSeconds,
		PoolsSyncFetchDelayMillis:  poolsSyncFetchDelayMillis,
		PoolsRetentionHours:        poolsRetentionHours,
		SmartLPEnabled:             getEnvBool("SMART_LP_ENABLED", false),
		SmartLPDebug:               smartLPDebug,
		SmartLPContractAddress:     strings.TrimSpace(getEnv("SMART_LP_CONTRACT_ADDRESS", "0x17ef7601103792929E01832c0DC3901a55Cf7922 0xd40318d99952680c2aBD7B634710bE8226EcABa4")),
		SmartLPScorePerWallet:      smartLPScorePerWallet,
		SmartLPMinWallets:          smartLPMinWallets,
		SmartLPRecentWindowMinutes: smartLPRecentWindowMinutes,
		SmartLPScanIntervalSeconds: smartLPScanInterval,
		SmartLPMaxBlocksPerScan:    smartLPMaxBlocksPerScan,
		SmartLPRPCTimeoutSeconds:   smartLPRPCTimeoutSeconds,
		SmartLPScanTimeoutSeconds:  smartLPScanTimeoutSeconds,
	}

	// Build per-chain configs (single-instance multi-chain).
	AppConfig.initChainConfigs()

	// Enforce encryption key to avoid storing/decrypting private keys insecurely.
	if _, err := security.DecodeHexKey32(AppConfig.EncryptionKey); err != nil {
		return fmt.Errorf("invalid ENCRYPTION_KEY: %w", err)
	}

	// 鎵撳嵃鍏抽敭閰嶇疆淇℃伅锛堥殣钘忔晱鎰熶俊鎭級
	log.Println("配置如下")
	log.Printf("   - Telegram Bot Token: %s", maskString(AppConfig.TelegramBotToken))
	log.Printf("   - Telegram WebApp URL: %s", AppConfig.TelegramWebAppURL)
	log.Printf("   - Telegram Menu Button Mode: %s", AppConfig.TelegramMenuButtonMode)
	log.Printf("   - Telegram WebApp Allow Empty InitData: %v", AppConfig.TelegramWebAppAllowEmptyInitData)
	log.Printf("   - Telegram WebApp Debug User ID: %d", AppConfig.TelegramWebAppDebugUserID)
	log.Printf("   - Telegram WebApp Debug Username: %s", AppConfig.TelegramWebAppDebugUsername)
	log.Printf("   - Admin Wallet Address: %s", AppConfig.AdminWalletAddress)
	log.Printf("   - Uniswap V4 PoolManager: %s", AppConfig.UniswapV4PoolManagerAddress)
	log.Printf("   - Uniswap V4 StateView: %s", AppConfig.UniswapV4StateViewAddress)
	log.Printf("   - Uniswap V4 PositionManager: %s", AppConfig.UniswapV4PositionManagerAddress)
	log.Printf("   - Uniswap V4 Debug: %v", AppConfig.UniswapV4Debug)
	log.Printf("   - Zap V3: %s", AppConfig.ZapV3Address)
	log.Printf("   - Zap V4: %s", AppConfig.ZapV4Address)
	log.Printf("   - OKX Swap Router: %s", AppConfig.OKXSwapRouter)
	log.Printf("   - OKX TokenApprove: %s", AppConfig.OKXTokenApproveAddress)
	log.Printf("   - OKX Debug: %v", AppConfig.OKXDebug)
	log.Printf("   - OKX Swap GasLimit Multiplier: %.4f", AppConfig.OKXSwapGasLimitMultiplier)
	log.Printf("   - OKX Swap GasLimit Min/Max: %d/%d", AppConfig.OKXSwapGasLimitMin, AppConfig.OKXSwapGasLimitMax)
	log.Printf("   - Zap GasLimit Multiplier: %.4f", AppConfig.ZapGasLimitMultiplier)
	log.Printf("   - Zap GasLimit Min/Max: %d/%d", AppConfig.ZapGasLimitMin, AppConfig.ZapGasLimitMax)
	log.Printf("   - Private Zap Enabled: %v", AppConfig.PrivateZapEnabled)
	log.Printf("   - Private Zap Version (legacy/ignored): %d", AppConfig.PrivateZapVersion)
	log.Printf("   - Pancake V3 NPM: %s", AppConfig.PancakeV3PositionManagerAddress)
	log.Printf("   - Uniswap V3 NPM: %s", AppConfig.UniswapV3PositionManagerAddress)
	log.Printf("   - BSC RPC URL: %s", maskURL(AppConfig.BSCRpcURL))
	log.Printf("   - BSC RPC WS URL: %s", maskURL(AppConfig.BSCRpcWSURL))
	log.Printf("   - BSC Chain ID: %d", AppConfig.BSCChainID)
	if len(AppConfig.EnabledChains) > 0 {
		log.Printf("   - Enabled Chains: %s", strings.Join(AppConfig.EnabledChains, ","))
		for _, ch := range AppConfig.EnabledChains {
			cc, ok := AppConfig.GetChainConfig(ch)
			if !ok {
				continue
			}
			log.Printf("     * %s kind=%s chainId=%d rpc=%s stable=%s(%d) zapV3=%s",
				cc.Chain, cc.Kind, cc.ChainID, maskURL(cc.RpcURL), cc.StableSymbol, cc.StableDecimals, cc.ZapV3Address)
		}
	}
	log.Printf("   - MySQL: %s@%s:%s/%s", AppConfig.MySQLUser, AppConfig.MySQLHost, AppConfig.MySQLPort, AppConfig.MySQLDatabase)
	log.Printf("   - Redis: %s:%s (DB: %d)", AppConfig.RedisHost, AppConfig.RedisPort, AppConfig.RedisDB)
	log.Printf("   - V4 NFT Scan From Block: %d", AppConfig.V4NFTScanFromBlock)
	log.Printf("   - Realtime V3 NFT Scan: %v", AppConfig.RealtimeV3NFTScan)
	log.Printf("   - Realtime V3 NFT Scan Max: %d", AppConfig.RealtimeV3NFTScanMax)
	log.Printf("   - Pools Sync Enabled: %v", AppConfig.PoolsSyncEnabled)
	log.Printf("   - Pools Sync Chain: %s", AppConfig.PoolsSyncChain)
	log.Printf("   - Pools Sync Dexes: %s", AppConfig.PoolsSyncDexes)
	log.Printf("   - Pools Sync Interval: %d seconds", AppConfig.PoolsSyncIntervalSeconds)
	log.Printf("   - Pools Retention: %d hours", AppConfig.PoolsRetentionHours)
	log.Printf("   - SmartLP Enabled: %v", AppConfig.SmartLPEnabled)
	log.Printf("   - SmartLP Debug: %v", AppConfig.SmartLPDebug)
	log.Printf("   - SmartLP Contract Address: %s", AppConfig.SmartLPContractAddress)
	log.Printf("   - SmartLP Scan Interval: %d seconds", AppConfig.SmartLPScanIntervalSeconds)
	log.Printf("   - SmartLP Max Blocks Per Scan: %d", AppConfig.SmartLPMaxBlocksPerScan)
	log.Printf("   - SmartLP RPC Timeout: %d seconds", AppConfig.SmartLPRPCTimeoutSeconds)
	log.Printf("   - SmartLP Scan Timeout: %d seconds", AppConfig.SmartLPScanTimeoutSeconds)
	log.Println("鉁?閰嶇疆鍔犺浇瀹屾垚")
	log.Println("========================================")

	return nil
}

func NormalizeChain(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "bsc"
	}
	return v
}

// EnabledChainsNormalized returns the server-enabled chain list (normalized, de-duplicated).
// It always returns a non-empty slice (fallback to ["bsc"]).
func EnabledChainsNormalized() []string {
	if AppConfig == nil || len(AppConfig.EnabledChains) == 0 {
		return []string{"bsc"}
	}

	seen := make(map[string]struct{}, len(AppConfig.EnabledChains))
	out := make([]string, 0, len(AppConfig.EnabledChains))
	for _, c := range AppConfig.EnabledChains {
		ch := NormalizeChain(c)
		if ch == "" {
			continue
		}
		if _, ok := seen[ch]; ok {
			continue
		}
		seen[ch] = struct{}{}
		out = append(out, ch)
	}
	if len(out) == 0 {
		return []string{"bsc"}
	}
	return out
}

// PickEnabledChain picks a safe chain from the server-enabled chain list.
// - Prefer the provided chain when enabled.
// - Otherwise prefer "bsc" when enabled.
// - Otherwise fall back to the first enabled chain.
func PickEnabledChain(preferred string) string {
	preferred = NormalizeChain(preferred)
	enabled := EnabledChainsNormalized()

	for _, c := range enabled {
		if NormalizeChain(c) == preferred {
			return preferred
		}
	}
	for _, c := range enabled {
		if NormalizeChain(c) == "bsc" {
			return "bsc"
		}
	}
	if len(enabled) > 0 {
		return NormalizeChain(enabled[0])
	}
	return "bsc"
}

func parseChainList(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		ch := NormalizeChain(p)
		if ch == "" {
			continue
		}
		if _, ok := seen[ch]; ok {
			continue
		}
		seen[ch] = struct{}{}
		out = append(out, ch)
	}
	return out
}

func getEnvInt(key string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}
	return n
}

func getEnvInt64(key string, defaultValue int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return defaultValue
	}
	return n
}

func getEnvStr(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func pickFirstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func buildV3Deployment(name, factoryAddr, npmAddr string) V3DeploymentConfig {
	return V3DeploymentConfig{
		Name:                   strings.TrimSpace(name),
		FactoryAddress:         strings.TrimSpace(factoryAddr),
		PositionManagerAddress: strings.TrimSpace(npmAddr),
	}
}

func (c *Config) initChainConfigs() {
	if c == nil {
		return
	}

	enabled := parseChainList(getEnv("CHAINS", ""))
	if len(enabled) == 0 {
		enabled = []string{"bsc"}
	}

	chains := make(map[string]ChainConfig, len(enabled))
	for _, chain := range enabled {
		switch chain {
		case "bsc":
			pancakeFactory := getEnv("PANCAKE_V3_FACTORY_ADDRESS", "0x0BFbcf9fa4f9C56B0F40a671Ad40E0805A091865")
			uniswapFactory := getEnv("UNISWAP_V3_FACTORY_ADDRESS", "0xdB1d10011AD0Ff90774D0C6Bb92e5C5c8b4461F7")

			okxRouter := pickFirstNonEmpty(getEnvStr("OKX_SWAP_ROUTER_BSC"), c.OKXSwapRouter)
			okxApprove := pickFirstNonEmpty(getEnvStr("OKX_TOKEN_APPROVE_ADDRESS_BSC"), c.OKXTokenApproveAddress)

			cc := ChainConfig{
				Chain:             "bsc",
				Kind:              ChainKindEVM,
				RpcURL:            strings.TrimSpace(c.BSCRpcURL),
				RpcWSURL:          strings.TrimSpace(c.BSCRpcWSURL),
				ChainID:           c.BSCChainID,
				PrivateZapVersion: getEnvInt("BSC_PRIVATE_ZAP_VERSION", c.PrivateZapVersion),

				StableSymbol:   "USDT",
				StableAddress:  strings.TrimSpace(c.USDTAddress),
				StableDecimals: getEnvInt("BSC_USDT_DECIMALS", 18),
				USDTAddress:    strings.TrimSpace(c.USDTAddress),
				USDCAddress:    strings.TrimSpace(c.USDCAddress),
				BUSDAddress:    strings.TrimSpace(c.BUSDAddress),

				WrappedNativeSymbol:  "WBNB",
				WrappedNativeAddress: strings.TrimSpace(c.WBNBAddress),

				OKXSwapRouter:          okxRouter,
				OKXTokenApproveAddress: okxApprove,

				ZapV3Address: strings.TrimSpace(c.ZapV3Address),
				ZapV4Address: strings.TrimSpace(c.ZapV4Address),

				UniswapV4PoolManagerAddress:     strings.TrimSpace(c.UniswapV4PoolManagerAddress),
				UniswapV4StateViewAddress:       strings.TrimSpace(c.UniswapV4StateViewAddress),
				UniswapV4PositionManagerAddress: strings.TrimSpace(c.UniswapV4PositionManagerAddress),

				ExplorerTxURLTemplate: pickFirstNonEmpty(getEnvStr("BSC_EXPLORER_TX_URL_TEMPLATE"), "https://bscscan.com/tx/%s"),
			}

			cc.V3Deployments = []V3DeploymentConfig{
				buildV3Deployment("PancakeSwap V3", pancakeFactory, strings.TrimSpace(c.PancakeV3PositionManagerAddress)),
				buildV3Deployment("Uniswap V3", uniswapFactory, strings.TrimSpace(c.UniswapV3PositionManagerAddress)),
			}
			cc.DefaultV3PositionManagerAddress = pickFirstNonEmpty(c.PancakeV3PositionManagerAddress, c.UniswapV3PositionManagerAddress)
			chains[chain] = cc

		case "base":
			// Allow per-chain overrides; fall back to global OKX allowlist when not set.
			okxRouter := pickFirstNonEmpty(getEnvStr("OKX_SWAP_ROUTER_BASE"), c.OKXSwapRouter)
			okxApprove := pickFirstNonEmpty(getEnvStr("OKX_TOKEN_APPROVE_ADDRESS_BASE"), c.OKXTokenApproveAddress)

			uniswapFactory := getEnvStr("BASE_UNISWAP_V3_FACTORY_ADDRESS")
			uniswapNPM := getEnvStr("BASE_UNISWAP_V3_NPM_ADDRESS")
			aeroFactory := getEnvStr("BASE_AERODROME_V3_FACTORY_ADDRESS")
			aeroNPM := getEnvStr("BASE_AERODROME_V3_NPM_ADDRESS")

			cc := ChainConfig{
				Chain:             "base",
				Kind:              ChainKindEVM,
				RpcURL:            strings.TrimSpace(getEnvStr("BASE_RPC_URL")),
				RpcWSURL:          strings.TrimSpace(getEnvStr("BASE_RPC_WS_URL")),
				ChainID:           getEnvInt64("BASE_CHAIN_ID", 8453),
				PrivateZapVersion: getEnvInt("BASE_PRIVATE_ZAP_VERSION", c.PrivateZapVersion),

				StableSymbol:   "USDC",
				StableAddress:  strings.TrimSpace(getEnvStr("BASE_USDC_ADDRESS")),
				StableDecimals: getEnvInt("BASE_USDC_DECIMALS", getEnvInt("BASE_USDT_DECIMALS", 6)),
				USDTAddress:    strings.TrimSpace(getEnvStr("BASE_USDT_ADDRESS")),
				USDCAddress:    strings.TrimSpace(getEnvStr("BASE_USDC_ADDRESS")),

				WrappedNativeSymbol:  "WETH",
				WrappedNativeAddress: strings.TrimSpace(getEnvStr("BASE_WETH_ADDRESS")),

				OKXSwapRouter:          okxRouter,
				OKXTokenApproveAddress: okxApprove,

				ZapV3Address: strings.TrimSpace(getEnvStr("BASE_ZAP_V3_ADDRESS")),
				ZapV4Address: strings.TrimSpace(getEnvStr("BASE_ZAP_V4_ADDRESS")),

				UniswapV4PoolManagerAddress:     strings.TrimSpace(getEnvStr("BASE_UNISWAP_V4_POOL_MANAGER_ADDRESS")),
				UniswapV4StateViewAddress:       strings.TrimSpace(getEnvStr("BASE_UNISWAP_V4_STATE_VIEW_ADDRESS")),
				UniswapV4PositionManagerAddress: strings.TrimSpace(getEnvStr("BASE_UNISWAP_V4_POSITION_MANAGER_ADDRESS")),

				ExplorerTxURLTemplate: pickFirstNonEmpty(getEnvStr("BASE_EXPLORER_TX_URL_TEMPLATE"), "https://basescan.org/tx/%s"),
			}

			cc.V3Deployments = []V3DeploymentConfig{
				buildV3Deployment("Uniswap V3", uniswapFactory, uniswapNPM),
				buildV3Deployment("Aerodrome Slipstream", aeroFactory, aeroNPM),
			}
			cc.DefaultV3PositionManagerAddress = pickFirstNonEmpty(uniswapNPM, aeroNPM)
			chains[chain] = cc

		default:
			// Unknown chain key: keep a placeholder config so callers can error with context.
			chains[chain] = ChainConfig{Chain: chain}
		}
	}

	c.EnabledChains = enabled
	c.Chains = chains
}

func (c *Config) GetChainConfig(chain string) (ChainConfig, bool) {
	if c == nil {
		return ChainConfig{}, false
	}
	chain = NormalizeChain(chain)
	if c.Chains == nil {
		return ChainConfig{}, false
	}
	cc, ok := c.Chains[chain]
	return cc, ok
}

// ExplorerTxURL returns a chain-scoped explorer transaction URL for the given txHash.
// It returns empty string when chain config is missing or template is not configured.
func ExplorerTxURL(chain string, txHash string) string {
	txHash = strings.TrimSpace(txHash)
	if txHash == "" {
		return ""
	}
	if AppConfig == nil {
		return ""
	}
	chain = NormalizeChain(chain)
	cc, ok := AppConfig.GetChainConfig(chain)
	if !ok {
		return ""
	}
	tpl := strings.TrimSpace(cc.ExplorerTxURLTemplate)
	if tpl == "" {
		return ""
	}
	return fmt.Sprintf(tpl, txHash)
}

func normalizeTelegramWebAppURL(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	lower := strings.ToLower(v)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return v
	}
	if strings.HasPrefix(lower, "localhost") || strings.HasPrefix(lower, "127.0.0.1") || strings.HasPrefix(lower, "0.0.0.0") {
		return "http://" + v
	}
	return "https://" + v
}

func normalizeTelegramMenuButtonMode(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "commands", "default", "web_app":
		return v
	case "":
		return "commands"
	default:
		log.Printf("鈿狅笍  Unknown TELEGRAM_MENU_BUTTON_MODE=%q; fallback to \"commands\"", v)
		return "commands"
	}
}

// maskString masks sensitive string for logging
func maskString(s string) string {
	if s == "" {
		return "<鏈缃?"
	}
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "..." + s[len(s)-4:]
}

func maskURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "<鏈缃?"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "***"
	}
	return fmt.Sprintf("%s://%s/...", u.Scheme, u.Host)
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvBool(key string, defaultValue bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return defaultValue
	}
}

func (c *Config) GetMySQLDSN() string {
	return c.MySQLUser + ":" + c.MySQLPassword + "@tcp(" + c.MySQLHost + ":" + c.MySQLPort + ")/" + c.MySQLDatabase + "?charset=utf8mb4&parseTime=True&loc=Local"
}

func (c *Config) GetRedisAddr() string {
	return c.RedisHost + ":" + c.RedisPort
}
