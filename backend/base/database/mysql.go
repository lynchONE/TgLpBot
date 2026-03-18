package database

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"fmt"
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitMySQL initializes MySQL database connection
func InitMySQL() error {
	log.Println("========================================")
	log.Println("🗄️  开始初始化 MySQL 数据库...")
	log.Println("========================================")

	timeutil.Init()

	dsn := config.AppConfig.GetMySQLDSN()
	log.Printf("📡 连接信息: %s@tcp(%s:%s)/%s",
		config.AppConfig.MySQLUser,
		config.AppConfig.MySQLHost,
		config.AppConfig.MySQLPort,
		config.AppConfig.MySQLDatabase)

	log.Println("🔌 正在连接 MySQL...")
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
		NowFunc:                                  timeutil.Now,
	})

	if err != nil {
		log.Printf("❌ MySQL 连接失败: %v", err)
		log.Println("💡 提示: 请检查 MySQL 服务是否运行，以及 .env 文件中的配置是否正确")
		return fmt.Errorf("failed to connect to MySQL: %w", err)
	}

	log.Println("✅ MySQL 连接成功")

	// Auto migrate models
	log.Println("🔄 开始数据库迁移...")
	if err := autoMigrate(); err != nil {
		log.Printf("❌ 数据库迁移失败: %v", err)
		return fmt.Errorf("failed to auto migrate: %w", err)
	}
	log.Println("✅ 数据库迁移完成")
	log.Println("========================================")

	return nil
}

// autoMigrate runs auto migration for all models
func autoMigrate() error {
	return DB.AutoMigrate(
		&models.User{},
		&models.Wallet{},
		&models.WalletChainContract{},
		&models.LPConfig{},
		&models.GlobalConfig{},
		&models.SystemConfig{},
		&models.RpcEndpoint{},
		&models.Position{},
		&models.Pool{},
		&models.Transaction{},
		&models.AuthCode{},
		&models.UserAccess{},
		&models.Announcement{},
		&models.TradeRecord{},
		&models.WalletBalanceSnapshot{},
		&models.StrategyTask{},
		&models.TokenMetadata{},
		&models.MonitoredWallet{},
		&models.WatchContract{},
		&models.SmartMoneyLPEvent{},
		&models.SmartMoneyLPPosition{},
	)
}

// CloseMySQL closes the MySQL database connection
func CloseMySQL() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
