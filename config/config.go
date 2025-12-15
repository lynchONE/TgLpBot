package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	// Telegram
	TelegramBotToken string

	// BSC Network
	BSCRpcURL   string
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
	OKXDexAPIURL  string
	OKXAPIKey     string
	OKXSecretKey  string
	OKXPassphrase string

	// Contracts
	ZapContractAddress string

	// Encryption
	EncryptionKey string

	// Gas
	MaxGasPrice int64
	GasLimit    uint64

	// Token Addresses
	USDTAddress          string
	BUSDAddress          string
	WBNBAddress          string
	PancakeRouterV2      string
	PancakeFactoryV2     string
}

var AppConfig *Config

func LoadConfig() error {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	chainID, _ := strconv.ParseInt(getEnv("BSC_CHAIN_ID", "56"), 10, 64)
	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	maxGasPrice, _ := strconv.ParseInt(getEnv("MAX_GAS_PRICE", "5000000000"), 10, 64)
	gasLimit, _ := strconv.ParseUint(getEnv("GAS_LIMIT", "500000"), 10, 64)

	AppConfig = &Config{
		// Telegram
		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),

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
		OKXDexAPIURL:  getEnv("OKX_DEX_API_URL", "https://www.okx.com/api/v5/dex/aggregator"),
		OKXAPIKey:     getEnv("OKX_API_KEY", ""),
		OKXSecretKey:  getEnv("OKX_SECRET_KEY", ""),
		OKXPassphrase: getEnv("OKX_PASSPHRASE", ""),

		// Contracts
		ZapContractAddress: getEnv("ZAP_CONTRACT_ADDRESS", ""),

		// Encryption
		EncryptionKey: getEnv("ENCRYPTION_KEY", ""),

		// Gas
		MaxGasPrice: maxGasPrice,
		GasLimit:    gasLimit,

		// Token Addresses
		USDTAddress:      getEnv("USDT_ADDRESS", "0x55d398326f99059fF775485246999027B3197955"),
		BUSDAddress:      getEnv("BUSD_ADDRESS", "0xe9e7CEA3DedcA5984780Bafc599bD69ADd087D56"),
		WBNBAddress:      getEnv("WBNB_ADDRESS", "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c"),
		PancakeRouterV2:  getEnv("PANCAKE_ROUTER_V2", "0x10ED43C718714eb63d5aA57B78B54704E256024E"),
		PancakeFactoryV2: getEnv("PANCAKE_FACTORY_V2", "0xcA143Ce32Fe78f1f7019d7d551a6402fC5350c73"),
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func (c *Config) GetMySQLDSN() string {
	return c.MySQLUser + ":" + c.MySQLPassword + "@tcp(" + c.MySQLHost + ":" + c.MySQLPort + ")/" + c.MySQLDatabase + "?charset=utf8mb4&parseTime=True&loc=Local"
}

func (c *Config) GetRedisAddr() string {
	return c.RedisHost + ":" + c.RedisPort
}

