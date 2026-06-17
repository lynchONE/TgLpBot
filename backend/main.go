package main

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/okxpool"
	"TgLpBot/base/rpcpool"
	"TgLpBot/base/timeutil"
	"TgLpBot/service/bot"
	"TgLpBot/service/pool_sync"
	"TgLpBot/service/wallet"
	"TgLpBot/service/web_server"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	timeutil.Init()

	fmt.Println("========================================")
	fmt.Println("TgLpBot 开始...")
	fmt.Println("========================================")

	if err := config.LoadConfig(); err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	if err := database.InitMySQL(); err != nil {
		log.Fatalf("init mysql failed: %v", err)
	}
	defer database.CloseMySQL()

	rpcpool.StartDefaultHealthChecker(time.Hour)
	defer rpcpool.StopDefaultHealthChecker()
	okxpool.StartDefaultHealthChecker(30 * time.Minute)
	defer okxpool.StopDefaultHealthChecker()

	ws := wallet.NewWalletService()
	if migrated, err := ws.MigratePlaintextPrivateKeys(); err != nil {
		log.Fatalf("migrate plaintext private keys failed: %v", err)
	} else if migrated > 0 {
		log.Printf("migrated %d plaintext private keys", migrated)
	}

	if err := database.InitRedis(); err != nil {
		log.Fatalf("init redis failed: %v", err)
	}
	defer database.CloseRedis()

	if err := blockchain.InitBlockchains(); err != nil {
		log.Printf("warning: init blockchains failed: %v", err)
	} else {
		log.Println("blockchain clients initialized")
	}
	defer blockchain.CloseBlockchains()

	blockchain.StartRPCPoolRefresher(30 * time.Second)
	defer blockchain.StopRPCPoolRefresher()

	poolSyncService := pool_sync.NewService()
	poolSyncService.Start()
	defer poolSyncService.Stop()

	webServer := web_server.NewServer()
	webServer.Start("8080")
	if webServer.Assets != nil {
		defer webServer.Assets.Stop()
	}
	if webServer.SwapLimitOrders != nil {
		defer webServer.SwapLimitOrders.Stop()
	}

	telegramBot, err := bot.NewBot()
	if err != nil {
		log.Fatalf("create bot failed: %v", err)
	}

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("shutting down...")
		poolSyncService.Stop()
		if webServer.Assets != nil {
			webServer.Assets.Stop()
		}
		if webServer.SwapLimitOrders != nil {
			webServer.SwapLimitOrders.Stop()
		}
		telegramBot.Stop()
		rpcpool.StopDefaultHealthChecker()
		okxpool.StopDefaultHealthChecker()
		database.CloseMySQL()
		database.CloseRedis()
		blockchain.StopRPCPoolRefresher()
		blockchain.CloseBlockchains()
		os.Exit(0)
	}()

	log.Println("all services initialized")
	telegramBot.Start()
}
