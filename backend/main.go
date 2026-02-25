package main

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/clickhouse"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/timeutil"
	"TgLpBot/service/bot"
	"TgLpBot/service/wallet"
	"TgLpBot/service/web_server"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// 设置日志输出到标准输出，不使用缓冲
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	timeutil.Init()

	// 立即输出第一条日志
	fmt.Println("========================================")
	fmt.Println("🚀 TgLpBot 启动中...")
	fmt.Println("========================================")
	log.Println("程序已进入 main 函数")

	// Load configuration
	log.Println("开始加载配置...")
	if err := config.LoadConfig(); err != nil {
		log.Fatalf("❌ 配置加载失败: %v", err)
	}

	// Initialize MySQL
	log.Println("开始初始化 MySQL...")
	if err := database.InitMySQL(); err != nil {
		log.Fatalf("❌ MySQL 初始化失败: %v", err)
	}
	defer database.CloseMySQL()

	// Security: migrate any legacy plaintext wallet private keys to encrypted storage.
	ws := wallet.NewWalletService()
	if migrated, err := ws.MigratePlaintextPrivateKeys(); err != nil {
		log.Fatalf("❌ 私钥加密迁移失败: %v", err)
	} else if migrated > 0 {
		log.Printf("✅ 已加密迁移 %d 个钱包私钥", migrated)
	}

	// Initialize Redis
	log.Println("========================================")
	log.Println("📮 开始初始化 Redis...")
	log.Println("========================================")
	if err := database.InitRedis(); err != nil {
		log.Fatalf("❌ Redis 初始化失败: %v", err)
	}
	log.Println("✅ Redis 初始化成功")
	log.Println("========================================")
	defer database.CloseRedis()

	// Initialize ClickHouse
	log.Println("========================================")
	log.Println("📊 初始化 ClickHouse...")
	log.Println("========================================")
	chService, err := clickhouse.NewClickHouseService(
		config.AppConfig.ClickHouseAddr,
		config.AppConfig.ClickHouseDB,
		config.AppConfig.ClickHouseUser,
		config.AppConfig.ClickHousePassword,
		config.AppConfig.ClickHouseProtocol,
		config.AppConfig.ClickHouseDebug,
	)
	if err != nil {
		log.Printf("⚠️ ClickHouse 连接失败: %v", err)
		log.Println("💡 /api/pools 等依赖 ClickHouse 的接口将不可用")
	} else {
		log.Println("✅ ClickHouse 连接成功")
		chService.StartDailyRetentionCleanup()
	}

	// Start Web Server (always on; some endpoints may be disabled if ClickHouse is unavailable)
	webServer := web_server.NewServer(chService)
	webServer.Start("8080")

	// Initialize Blockchain client (synchronous to ensure it's ready before bot starts)
	log.Println("========================================")
	log.Println("⛓️  开始初始化区块链客户端...")
	log.Println("========================================")
	if err := blockchain.InitBlockchains(); err != nil {
		log.Printf("⚠️  警告: 区块链初始化失败: %v", err)
		log.Println("💡 机器人将继续运行，但池子查询可能会失败")
	} else {
		log.Println("✅ 区块链客户端初始化成功")
	}
	log.Println("========================================")
	defer blockchain.CloseBlockchains()

	// Create and start bot
	log.Println("========================================")
	log.Println("🤖 开始创建 Telegram Bot...")
	log.Println("========================================")
	telegramBot, err := bot.NewBot(chService)
	if err != nil {
		log.Fatalf("❌ Bot 创建失败: %v", err)
	}
	log.Println("✅ Bot 创建成功")
	log.Println("========================================")

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("========================================")
		log.Println("🛑 收到停止信号，正在优雅关闭...")
		log.Println("========================================")
		telegramBot.Stop()
		database.CloseMySQL()
		database.CloseRedis()
		blockchain.CloseBlockchains()
		log.Println("✅ 服务已停止")
		os.Exit(0)
	}()

	// Start bot
	log.Println("========================================")
	log.Println("✅ 所有服务初始化完成！")
	log.Println("🎉 Bot 正在运行，等待消息...")
	log.Println("========================================")
	telegramBot.Start()
}
