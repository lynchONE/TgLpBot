package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"TgLpBot/security"

	"github.com/joho/godotenv"
)

type Config struct {
	// Telegram
	TelegramBotToken       string
	TelegramWebAppURL      string
	TelegramMenuButtonMode string // commands|default|web_app

	// Access Control
	AdminWalletAddress string

	// Uniswap V4
	UniswapV4PoolManagerAddress     string
	UniswapV4StateViewAddress       string
	UniswapV4PositionManagerAddress string
	UniswapV4Debug                  bool

	// BSC Network
	BSCRpcURL  string
	BSCChainID int64

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
	OKXDexAPIURL           string
	OKXAPIKey              string
	OKXSecretKey           string
	OKXPassphrase          string
	OKXSwapRouter          string
	OKXTokenApproveAddress string // OKX DEX 的 TokenApprove 合约地址
	OKXDebug               bool

	// ClickHouse
	ClickHouseAddr     string
	ClickHouseDB       string
	ClickHouseUser     string
	ClickHousePassword string
	ClickHouseDebug    bool
	ClickHouseResetAll bool

	// Contracts
	ZapV3Address string
	ZapV4Address string

	// V3 Position Managers (optional defaults)
	PancakeV3PositionManagerAddress string
	UniswapV3PositionManagerAddress string

	// Encryption
	EncryptionKey string

	// Gas
	MaxGasPrice int64
	GasLimit    uint64

	// Token Addresses
	USDTAddress      string
	USDCAddress      string
	BUSDAddress      string
	WBNBAddress      string
	PancakeRouterV2  string
	PancakeFactoryV2 string

	// V3 Swap Router (链上 swap 用)
	PancakeV3SwapRouter string
	UniswapV3SwapRouter string

	// Mini App / Realtime positions
	V4NFTScanFromBlock uint64

	// Auto LP (PoolM scanner + optional executor)
	AutoLPEnabled                 bool
	AutoLPExecuteEnabled          bool
	AutoLPNotifyTopCandidate      bool
	AutoLPDebug                   bool
	AutoLPPoolMBaseURL            string
	AutoLPChain                   string
	AutoLPProtocols               string
	AutoLPTimeframeShortMinutes   int
	AutoLPTimeframeLongMinutes    int
	AutoLPScanIntervalSeconds     int
	AutoLPFetchDelayMillis        int
	AutoLPUserID                  int
	AutoLPAmountUSDT              float64
	AutoLPBaseWidthPercentage     float64
	AutoLPMaxActiveTasks          int
	AutoLPMinPoolValueUSD         float64
	AutoLPMinFeePercentage        float64
	AutoLPMinTotalFees5m          float64
	AutoLPMinTotalVolume5m        float64
	AutoLPMinTx5m                 int
	AutoLPMinTx60m                int
	AutoLPMinFeeApr5m             float64
	AutoLPMinFeeApr60m            float64
	AutoLPRequireStableSymbol     string
	AutoLPMaxSurgeRatio           float64
	AutoLPMaxCandidates           int
	AutoLPAllowEntrySwap          bool
	AutoLPExitVolumeThreshold     float64
	AutoLPHeatDownScans           int
	AutoLPEmergencyGasMultiplier  float64
	AutoLPWidthSidewaysPercent    float64
	AutoLPWidthMildUptrendPercent float64
	AutoLPWidthRapidPumpPercent   float64
}

var AppConfig *Config

func LoadConfig() error {
	log.Println("========================================")
	log.Println("📋 开始加载配置...")
	log.Println("========================================")

	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  警告: .env 文件未找到，使用环境变量")
	} else {
		log.Println("✅ .env 文件加载成功")
	}

	chainID, _ := strconv.ParseInt(getEnv("BSC_CHAIN_ID", "56"), 10, 64)
	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	maxGasPrice, _ := strconv.ParseInt(getEnv("MAX_GAS_PRICE", "5000000000"), 10, 64)
	gasLimit, _ := strconv.ParseUint(getEnv("GAS_LIMIT", "500000"), 10, 64)
	v4NFTScanFromBlock, _ := strconv.ParseUint(strings.TrimSpace(getEnv("V4_NFT_SCAN_FROM_BLOCK", "0")), 10, 64)
	autoLPShortTF, _ := strconv.Atoi(strings.TrimSpace(getEnv("AUTO_LP_TIMEFRAME_SHORT_MINUTES", "5")))
	autoLPLongTF, _ := strconv.Atoi(strings.TrimSpace(getEnv("AUTO_LP_TIMEFRAME_LONG_MINUTES", "60")))
	autoLPScanInterval, _ := strconv.Atoi(strings.TrimSpace(getEnv("AUTO_LP_SCAN_INTERVAL_SECONDS", "60")))
	autoLPFetchDelayMillis, _ := strconv.Atoi(strings.TrimSpace(getEnv("AUTO_LP_FETCH_DELAY_MILLIS", "250")))
	autoLPUserID, _ := strconv.Atoi(strings.TrimSpace(getEnv("AUTO_LP_USER_ID", "0")))
	autoLPAmountUSDT, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_AMOUNT_USDT", "0")), 64)
	autoLPBaseWidthPct, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_BASE_WIDTH_PERCENT", "5")), 64)
	autoLPMaxActiveTasks, _ := strconv.Atoi(strings.TrimSpace(getEnv("AUTO_LP_MAX_ACTIVE_TASKS", "1")))
	autoLPMinPoolValueUSD, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_MIN_POOL_VALUE_USD", "50000")), 64)
	autoLPMinFeePct, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_MIN_FEE_PERCENTAGE", "0.2")), 64)
	autoLPMinTotalFees5m, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_MIN_TOTAL_FEES_5M", "100")), 64)
	autoLPMinTotalVolume5m, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_MIN_TOTAL_VOLUME_5M", "5000")), 64)
	autoLPMinTx5m, _ := strconv.Atoi(strings.TrimSpace(getEnv("AUTO_LP_MIN_TX_5M", "0")))
	autoLPMinTx60m, _ := strconv.Atoi(strings.TrimSpace(getEnv("AUTO_LP_MIN_TX_60M", "0")))
	autoLPMinFeeApr5m, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_MIN_FEE_APR_5M", "0")), 64)
	autoLPMinFeeApr60m, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_MIN_FEE_APR_60M", "0")), 64)
	autoLPMaxSurgeRatio, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_MAX_SURGE_RATIO", "0")), 64)
	autoLPMaxCandidates, _ := strconv.Atoi(strings.TrimSpace(getEnv("AUTO_LP_MAX_CANDIDATES", "20")))
	autoLPExitVolThreshold, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_AUTO_EXIT_VOLUME_THRESHOLD", "0.5")), 64)
	autoLPHeatDownScans, _ := strconv.Atoi(strings.TrimSpace(getEnv("AUTO_LP_HEAT_DOWN_SCANS", "6")))
	autoLPEmergencyGasMult, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_EMERGENCY_GAS_MULTIPLIER", "2.0")), 64)
	autoLPDebug := getEnvBool("AUTO_LP_DEBUG", false)
	autoLPWidthSideways, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_WIDTH_SIDEWAYS_PERCENT", "2.0")), 64)
	autoLPWidthMildUp, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_WIDTH_MILD_UPTREND_PERCENT", "5.0")), 64)
	autoLPWidthRapidPump, _ := strconv.ParseFloat(strings.TrimSpace(getEnv("AUTO_LP_WIDTH_RAPID_PUMP_PERCENT", "15.0")), 64)

	AppConfig = &Config{
		// Telegram
		TelegramBotToken:       getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramWebAppURL:      normalizeTelegramWebAppURL(getEnv("TELEGRAM_WEBAPP_URL", "")),
		TelegramMenuButtonMode: normalizeTelegramMenuButtonMode(getEnv("TELEGRAM_MENU_BUTTON_MODE", "commands")),

		// Access Control
		AdminWalletAddress: strings.TrimSpace(getEnv("ADMIN_WALLET_ADDRESS", "")),

		// Uniswap V4
		UniswapV4PoolManagerAddress:     getEnv("UNISWAP_V4_POOL_MANAGER_ADDRESS", ""),
		UniswapV4StateViewAddress:       getEnv("UNISWAP_V4_STATE_VIEW_ADDRESS", ""),
		UniswapV4PositionManagerAddress: getEnv("UNISWAP_V4_POSITION_MANAGER_ADDRESS", ""),
		UniswapV4Debug:                  getEnvBool("UNISWAP_V4_DEBUG", false),

		// BSC Network
		BSCRpcURL:  getEnv("BSC_RPC_URL", "https://bsc-dataseed1.binance.org/"),
		BSCChainID: chainID,

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
		OKXDexAPIURL:           getEnv("OKX_DEX_API_URL", "https://www.okx.com/api/v5/dex/aggregator"),
		OKXAPIKey:              getEnv("OKX_API_KEY", ""),
		OKXSecretKey:           getEnv("OKX_SECRET_KEY", ""),
		OKXPassphrase:          getEnv("OKX_PASSPHRASE", ""),
		OKXSwapRouter:          getEnv("OKX_SWAP_ROUTER", ""),
		OKXTokenApproveAddress: getEnv("OKX_TOKEN_APPROVE_ADDRESS", ""),
		OKXDebug:               getEnvBool("OKX_DEBUG", false),

		// ClickHouse
		ClickHouseAddr:     getEnv("CLICKHOUSE_ADDR", "localhost:9000"),
		ClickHouseDB:       getEnv("CLICKHOUSE_DB", "default"),
		ClickHouseUser:     getEnv("CLICKHOUSE_USER", "default"),
		ClickHousePassword: getEnv("CLICKHOUSE_PASSWORD", ""),
		ClickHouseDebug:    getEnvBool("CLICKHOUSE_DEBUG", false),
		ClickHouseResetAll: getEnvBool("CLICKHOUSE_RESET_ALL", false),

		// Contracts
		ZapV3Address: getEnv("ZAP_V3_ADDRESS", ""),
		ZapV4Address: getEnv("ZAP_V4_ADDRESS", ""),

		// V3 Position Managers
		PancakeV3PositionManagerAddress: getEnv("PANCAKE_V3_NPM_ADDRESS", ""),
		UniswapV3PositionManagerAddress: getEnv("UNISWAP_V3_NPM_ADDRESS", ""),

		// Encryption
		EncryptionKey: security.NormalizeHexString(getEnv("ENCRYPTION_KEY", "")),

		// Gas
		MaxGasPrice: maxGasPrice,
		GasLimit:    gasLimit,

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
		V4NFTScanFromBlock: v4NFTScanFromBlock,

		// Auto LP (PoolM scanner + optional executor)
		AutoLPEnabled:                 getEnvBool("AUTO_LP_ENABLED", false),
		AutoLPExecuteEnabled:          getEnvBool("AUTO_LP_EXECUTE_ENABLED", false),
		AutoLPNotifyTopCandidate:      getEnvBool("AUTO_LP_NOTIFY_TOP_CANDIDATE", false),
		AutoLPDebug:                   autoLPDebug,
		AutoLPPoolMBaseURL:            strings.TrimSpace(getEnv("AUTO_LP_POOLM_BASE_URL", "")),
		AutoLPChain:                   strings.TrimSpace(getEnv("AUTO_LP_CHAIN", "bsc")),
		AutoLPProtocols:               strings.TrimSpace(getEnv("AUTO_LP_PROTOCOLS", "v3,v4")),
		AutoLPTimeframeShortMinutes:   autoLPShortTF,
		AutoLPTimeframeLongMinutes:    autoLPLongTF,
		AutoLPScanIntervalSeconds:     autoLPScanInterval,
		AutoLPFetchDelayMillis:        autoLPFetchDelayMillis,
		AutoLPUserID:                  autoLPUserID,
		AutoLPAmountUSDT:              autoLPAmountUSDT,
		AutoLPBaseWidthPercentage:     autoLPBaseWidthPct,
		AutoLPMaxActiveTasks:          autoLPMaxActiveTasks,
		AutoLPMinPoolValueUSD:         autoLPMinPoolValueUSD,
		AutoLPMinFeePercentage:        autoLPMinFeePct,
		AutoLPMinTotalFees5m:          autoLPMinTotalFees5m,
		AutoLPMinTotalVolume5m:        autoLPMinTotalVolume5m,
		AutoLPMinTx5m:                 autoLPMinTx5m,
		AutoLPMinTx60m:                autoLPMinTx60m,
		AutoLPMinFeeApr5m:             autoLPMinFeeApr5m,
		AutoLPMinFeeApr60m:            autoLPMinFeeApr60m,
		AutoLPRequireStableSymbol:     strings.TrimSpace(getEnv("AUTO_LP_REQUIRE_STABLE_SYMBOL", "USDT")),
		AutoLPMaxSurgeRatio:           autoLPMaxSurgeRatio,
		AutoLPMaxCandidates:           autoLPMaxCandidates,
		AutoLPAllowEntrySwap:          getEnvBool("AUTO_LP_ALLOW_ENTRY_SWAP", false),
		AutoLPExitVolumeThreshold:     autoLPExitVolThreshold,
		AutoLPHeatDownScans:           autoLPHeatDownScans,
		AutoLPEmergencyGasMultiplier:  autoLPEmergencyGasMult,
		AutoLPWidthSidewaysPercent:    autoLPWidthSideways,
		AutoLPWidthMildUptrendPercent: autoLPWidthMildUp,
		AutoLPWidthRapidPumpPercent:   autoLPWidthRapidPump,
	}

	// Enforce encryption key to avoid storing/decrypting private keys insecurely.
	if _, err := security.DecodeHexKey32(AppConfig.EncryptionKey); err != nil {
		return fmt.Errorf("invalid ENCRYPTION_KEY: %w", err)
	}

	// 打印关键配置信息（隐藏敏感信息）
	log.Println("📝 配置信息:")
	log.Printf("   - Telegram Bot Token: %s", maskString(AppConfig.TelegramBotToken))
	log.Printf("   - Telegram WebApp URL: %s", AppConfig.TelegramWebAppURL)
	log.Printf("   - Telegram Menu Button Mode: %s", AppConfig.TelegramMenuButtonMode)
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
	log.Printf("   - Pancake V3 NPM: %s", AppConfig.PancakeV3PositionManagerAddress)
	log.Printf("   - Uniswap V3 NPM: %s", AppConfig.UniswapV3PositionManagerAddress)
	log.Printf("   - BSC RPC URL: %s", AppConfig.BSCRpcURL)
	log.Printf("   - BSC Chain ID: %d", AppConfig.BSCChainID)
	log.Printf("   - MySQL: %s@%s:%s/%s", AppConfig.MySQLUser, AppConfig.MySQLHost, AppConfig.MySQLPort, AppConfig.MySQLDatabase)
	log.Printf("   - Redis: %s:%s (DB: %d)", AppConfig.RedisHost, AppConfig.RedisPort, AppConfig.RedisDB)
	log.Printf("   - V4 NFT Scan From Block: %d", AppConfig.V4NFTScanFromBlock)
	log.Printf("   - AutoLP Enabled: %v", AppConfig.AutoLPEnabled)
	log.Printf("   - AutoLP Execute Enabled: %v", AppConfig.AutoLPExecuteEnabled)
	log.Printf("   - AutoLP Debug: %v", AppConfig.AutoLPDebug)
	log.Printf("   - AutoLP UserID: %d", AppConfig.AutoLPUserID)
	log.Printf("   - AutoLP Chain: %s", AppConfig.AutoLPChain)
	log.Printf("   - AutoLP Protocols: %s", AppConfig.AutoLPProtocols)
	log.Printf("   - AutoLP Timeframes: %d/%d minutes", AppConfig.AutoLPTimeframeShortMinutes, AppConfig.AutoLPTimeframeLongMinutes)
	log.Printf("   - AutoLP Scan Interval: %d seconds", AppConfig.AutoLPScanIntervalSeconds)
	log.Printf("   - AutoLP Fetch Delay: %d ms", AppConfig.AutoLPFetchDelayMillis)
	log.Printf("   - AutoLP Amount USDT: %.2f", AppConfig.AutoLPAmountUSDT)
	log.Printf("   - AutoLP Base Width Percentage: %.4f", AppConfig.AutoLPBaseWidthPercentage)
	log.Printf("   - AutoLP Width Sideways Percentage: %.4f", AppConfig.AutoLPWidthSidewaysPercent)
	log.Printf("   - AutoLP Width MildUptrend Percentage: %.4f", AppConfig.AutoLPWidthMildUptrendPercent)
	log.Printf("   - AutoLP Width RapidPump Percentage: %.4f", AppConfig.AutoLPWidthRapidPumpPercent)
	log.Printf("   - AutoLP Max Active Tasks: %d", AppConfig.AutoLPMaxActiveTasks)
	log.Printf("   - AutoLP Require Stable (已不用于筛选): %s", AppConfig.AutoLPRequireStableSymbol)
	log.Println("✅ 配置加载完成")
	log.Println("========================================")

	return nil
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
		log.Printf("⚠️  Unknown TELEGRAM_MENU_BUTTON_MODE=%q; fallback to \"commands\"", v)
		return "commands"
	}
}

// maskString masks sensitive string for logging
func maskString(s string) string {
	if s == "" {
		return "<未设置>"
	}
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "..." + s[len(s)-4:]
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
