package database

import (
	"TgLpBot/config"
	"TgLpBot/models"
	"fmt"
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitMySQL initializes MySQL database connection
func InitMySQL() error {
	dsn := config.AppConfig.GetMySQLDSN()
	
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	
	if err != nil {
		return fmt.Errorf("failed to connect to MySQL: %w", err)
	}
	
	log.Println("MySQL connected successfully")
	
	// Auto migrate models
	if err := autoMigrate(); err != nil {
		return fmt.Errorf("failed to auto migrate: %w", err)
	}
	
	return nil
}

// autoMigrate runs auto migration for all models
func autoMigrate() error {
	return DB.AutoMigrate(
		&models.User{},
		&models.Wallet{},
		&models.LPConfig{},
		&models.Transaction{},
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

