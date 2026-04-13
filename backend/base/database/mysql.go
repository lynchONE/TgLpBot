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
	if err := DB.AutoMigrate(
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
		&models.UserAssetDailySnapshot{},
		&models.UserLPDailyStat{},
		&models.UserWalletTransferEvent{},
		&models.StrategyTask{},
		&models.TokenMetadata{},
		&models.MonitoredWallet{},
		&models.WatchContract{},
		&models.SmartMoneyScanState{},
		&models.SmartMoneyLPEvent{},
		&models.SmartMoneyLPPosition{},
		&models.SmartMoneyActivePosition{},
		&models.SmartMoneyWalletTransferEvent{},
		&models.SmartMoneyWalletDailySnapshot{},
		&models.SmartMoneyLPDailyStat{},
		&models.SmartMoneyGoldenDogConfig{},
		&models.SmartMoneyWatchWallet{},
		&models.SmartMoneyWatchOpenAlertConfig{},
		&models.SmartMoneyWatchOpenAlertReceipt{},
	); err != nil {
		return err
	}

	if err := migrateSmartMoneyGoldenDogAlertStateTable(); err != nil {
		return err
	}

	// GORM AutoMigrate does not alter existing column types, fix manually.
	DB.Exec("ALTER TABLE sm_lp_events MODIFY COLUMN liquidity_delta DECIMAL(65,0) NOT NULL DEFAULT 0")
	DB.Exec("ALTER TABLE sm_lp_events MODIFY COLUMN token0_amount DECIMAL(65,0) NOT NULL DEFAULT 0")
	DB.Exec("ALTER TABLE sm_lp_events MODIFY COLUMN token1_amount DECIMAL(65,0) NOT NULL DEFAULT 0")
	DB.Exec("ALTER TABLE sm_lp_active_positions MODIFY COLUMN current_liquidity DECIMAL(65,0) NOT NULL DEFAULT 0")
	DB.Exec("ALTER TABLE sm_lp_active_positions MODIFY COLUMN entry_amount0 DECIMAL(65,0) NOT NULL DEFAULT 0")
	DB.Exec("ALTER TABLE sm_lp_active_positions MODIFY COLUMN entry_amount1 DECIMAL(65,0) NOT NULL DEFAULT 0")
	DB.Exec("ALTER TABLE sm_lp_active_positions MODIFY COLUMN net_amount0 DECIMAL(65,0) NOT NULL DEFAULT 0")
	DB.Exec("ALTER TABLE sm_lp_active_positions MODIFY COLUMN net_amount1 DECIMAL(65,0) NOT NULL DEFAULT 0")
	DB.Exec("ALTER TABLE sm_lp_active_positions MODIFY COLUMN fee_amount0 DECIMAL(65,0) NOT NULL DEFAULT 0")
	DB.Exec("ALTER TABLE sm_lp_active_positions MODIFY COLUMN fee_amount1 DECIMAL(65,0) NOT NULL DEFAULT 0")
	DB.Exec("ALTER TABLE user_wallet_transfer_events MODIFY COLUMN amount_raw VARCHAR(78) NOT NULL DEFAULT '0'")
	DB.Exec("ALTER TABLE sm_wallet_transfer_events MODIFY COLUMN amount_raw VARCHAR(78) NOT NULL DEFAULT '0'")

	// Ensure new columns exist (AutoMigrate may skip if table already exists with old schema)
	ensureColumn("sm_wallet_daily_snapshots", "open_lp_usd", "DECIMAL(20,4) NOT NULL DEFAULT 0 AFTER tracked_token_usd")
	ensureColumn("sm_wallet_daily_snapshots", "tracked_token_count", "INT NOT NULL DEFAULT 0 AFTER total_usd")
	ensureColumn("sm_wallet_daily_snapshots", "has_transfer_in", "TINYINT(1) NOT NULL DEFAULT 0 AFTER tracked_token_count")
	ensureColumn("sm_wallet_daily_snapshots", "has_transfer_out", "TINYINT(1) NOT NULL DEFAULT 0 AFTER has_transfer_in")
	ensureColumn("sm_wallet_daily_snapshots", "transfer_in_count", "INT NOT NULL DEFAULT 0 AFTER has_transfer_out")
	ensureColumn("sm_wallet_daily_snapshots", "transfer_out_count", "INT NOT NULL DEFAULT 0 AFTER transfer_in_count")
	ensureColumn("sm_wallet_daily_snapshots", "transfer_in_usd", "DECIMAL(20,4) NOT NULL DEFAULT 0 AFTER transfer_out_count")
	ensureColumn("sm_wallet_daily_snapshots", "transfer_out_usd", "DECIMAL(20,4) NOT NULL DEFAULT 0 AFTER transfer_in_usd")
	ensureColumn("sm_lp_events", "liquidity_delta", "DECIMAL(65,0) NOT NULL DEFAULT 0 AFTER token1_symbol")
	ensureColumn("monitored_wallets", "avatar_url", "VARCHAR(512) NULL AFTER label")
	ensureColumn("trade_records", "open_stable_before", "VARCHAR(78) NOT NULL DEFAULT '0' AFTER open_usdt_spent")
	ensureColumn("trade_records", "open_stable_after", "VARCHAR(78) NOT NULL DEFAULT '0' AFTER open_stable_before")
	ensureColumn("trade_records", "close_stable_before", "VARCHAR(78) NOT NULL DEFAULT '0' AFTER close_usdt_received")
	ensureColumn("trade_records", "close_stable_after", "VARCHAR(78) NOT NULL DEFAULT '0' AFTER close_stable_before")
	normalizeTradeRecordProfitFormula()

	return nil
}

func ensureColumn(table, column, definition string) {
	if DB == nil {
		return
	}
	if DB.Migrator().HasColumn(&struct{}{}, column) {
		return
	}
	var count int64
	DB.Raw("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?", table, column).Scan(&count)
	if count == 0 {
		DB.Exec(fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s", table, column, definition))
		log.Printf("[DB] added column %s.%s", table, column)
	}
}

func normalizeTradeRecordProfitFormula() {
	if DB == nil {
		return
	}

	openExpr := "COALESCE(CAST(NULLIF(TRIM(open_usdt_spent), '') AS DECIMAL(65,0)), 0)"
	closeExpr := "COALESCE(CAST(NULLIF(TRIM(close_usdt_received), '') AS DECIMAL(65,0)), 0)"
	gasExpr := "COALESCE(CAST(NULLIF(TRIM(total_gas_usdt), '') AS DECIMAL(65,0)), 0)"
	profitExpr := fmt.Sprintf("((%s) - (%s) - (%s))", closeExpr, openExpr, gasExpr)
	profitPctExpr := fmt.Sprintf("CASE WHEN (%s) > 0 THEN ROUND(((%s) / (%s)) * 100, 4) ELSE 0 END", openExpr, profitExpr, openExpr)
	query := fmt.Sprintf(
		"UPDATE trade_records SET profit_usdt = CAST(%s AS CHAR), profit_pct = %s WHERE status = 'closed' AND (profit_usdt <> CAST(%s AS CHAR) OR ABS(COALESCE(profit_pct, 0) - (%s)) > 0.00005)",
		profitExpr,
		profitPctExpr,
		profitExpr,
		profitPctExpr,
	)

	tx := DB.Exec(query)
	if tx.Error != nil {
		log.Printf("[DB] normalize trade record profit formula failed: %v", tx.Error)
		return
	}
	if tx.RowsAffected > 0 {
		log.Printf("[DB] normalized %d trade_records to direct realized profit formula", tx.RowsAffected)
	}
}

func migrateSmartMoneyGoldenDogAlertStateTable() error {
	if DB == nil {
		return nil
	}

	model := &models.SmartMoneyGoldenDogAlertState{}
	tableName := model.TableName()
	migrator := DB.Migrator()

	if migrator.HasTable(tableName) {
		legacy, err := hasLegacySmartMoneyGoldenDogAlertStateSchema(tableName)
		if err != nil {
			return err
		}
		if legacy {
			log.Printf("[DB] recreate legacy table: %s", tableName)
			if err := migrator.DropTable(tableName); err != nil {
				return fmt.Errorf("drop legacy %s: %w", tableName, err)
			}
		} else {
			if err := cleanupSmartMoneyGoldenDogAlertStateRows(tableName); err != nil {
				return err
			}
		}
	}

	if err := DB.AutoMigrate(model); err != nil {
		return err
	}

	return cleanupSmartMoneyGoldenDogAlertStateRows(tableName)
}

func hasLegacySmartMoneyGoldenDogAlertStateSchema(tableName string) (bool, error) {
	hasPairKey, err := tableColumnExists(tableName, "pair_key")
	if err != nil {
		return false, fmt.Errorf("inspect %s.pair_key: %w", tableName, err)
	}
	if !hasPairKey {
		return true, nil
	}

	legacyColumns := []string{"pool_version", "pool_id", "last_pair", "deleted_at"}
	for _, column := range legacyColumns {
		exists, err := tableColumnExists(tableName, column)
		if err != nil {
			return false, fmt.Errorf("inspect %s.%s: %w", tableName, column, err)
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}

func tableColumnExists(tableName, columnName string) (bool, error) {
	var count int64
	err := DB.Raw(`
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = ?
		  AND column_name = ?
	`, tableName, columnName).Scan(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func cleanupSmartMoneyGoldenDogAlertStateRows(tableName string) error {
	hasPairKey, err := tableColumnExists(tableName, "pair_key")
	if err != nil {
		return fmt.Errorf("inspect %s.pair_key before cleanup: %w", tableName, err)
	}
	if !hasPairKey {
		return nil
	}

	if err := DB.Exec(`
		DELETE FROM smart_money_golden_dog_alert_states
		WHERE COALESCE(TRIM(pair_key), '') = ''
	`).Error; err != nil {
		return fmt.Errorf("delete empty pair_key rows: %w", err)
	}

	if err := DB.Exec(`
		DELETE older
		FROM smart_money_golden_dog_alert_states AS older
		INNER JOIN smart_money_golden_dog_alert_states AS newer
			ON older.user_id = newer.user_id
			AND older.chain = newer.chain
			AND older.pair_key = newer.pair_key
			AND (
				older.updated_at < newer.updated_at
				OR (older.updated_at = newer.updated_at AND older.id < newer.id)
			)
	`).Error; err != nil {
		return fmt.Errorf("dedupe pair_key rows: %w", err)
	}

	return nil
}

// CloseMySQL closes the MySQL database connection
func CloseMySQL() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
