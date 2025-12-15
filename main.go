package main

import (
	"TgLpBot/blockchain"
	"TgLpBot/bot"
	"TgLpBot/config"
	"TgLpBot/database"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Println("Starting TgLpBot...")
	
	// Load configuration
	if err := config.LoadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Println("Configuration loaded")
	
	// Initialize MySQL
	if err := database.InitMySQL(); err != nil {
		log.Fatalf("Failed to initialize MySQL: %v", err)
	}
	log.Println("MySQL initialized")
	defer database.CloseMySQL()
	
	// Initialize Redis
	if err := database.InitRedis(); err != nil {
		log.Fatalf("Failed to initialize Redis: %v", err)
	}
	log.Println("Redis initialized")
	defer database.CloseRedis()
	
	// Initialize Blockchain client
	if err := blockchain.InitBlockchain(); err != nil {
		log.Fatalf("Failed to initialize blockchain: %v", err)
	}
	log.Println("Blockchain client initialized")
	defer blockchain.CloseBlockchain()
	
	// Create and start bot
	telegramBot, err := bot.NewBot()
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}
	log.Println("Bot created successfully")
	
	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		
		log.Println("Shutting down gracefully...")
		telegramBot.Stop()
		database.CloseMySQL()
		database.CloseRedis()
		blockchain.CloseBlockchain()
		os.Exit(0)
	}()
	
	// Start bot
	log.Println("Bot is running...")
	telegramBot.Start()
}

