package database

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"fmt"
	"log"
	"strings"

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
		&models.OKXAPIConfig{},
		&models.PoolDataSource{},
		&models.Position{},
		&models.Pool{},
		&models.Transaction{},
		&models.WalletSwapLimitOrder{},
		&models.AuthCode{},
		&models.UserAccess{},
		&models.Announcement{},
		&models.SosoValueNewsItem{},
		&models.SosoValueAPIUsage{},
		&models.TradeRecord{},
		&models.WalletBalanceSnapshot{},
		&models.UserAssetDailySnapshot{},
		&models.UserLPDailyStat{},
		&models.UserLPDailyPnLAdjustment{},
		&models.UserLPProfitBaseline{},
		&models.UserWalletTransferEvent{},
		&models.StrategyTask{},
		&models.TokenMetadata{},
		&models.TokenRiskSnapshot{},
		&models.MonitoredWallet{},
		&models.WatchContract{},
		&models.SmartMoneyScanState{},
		&models.SmartMoneyLPEvent{},
		&models.SmartMoneyLPPosition{},
		&models.SmartMoneyActivePosition{},
		&models.SmartMoneyWalletTransferEvent{},
		&models.SmartMoneyWalletDailySnapshot{},
		&models.SmartMoneyWalletLiveState{},
		&models.SmartMoneyLPDailyStat{},
		&models.SmartMoneyGoldenDogConfig{},
		&models.SmartMoneyWatchWallet{},
		&models.SmartMoneyWatchOpenAlertConfig{},
		&models.SmartMoneyWatchOpenAlertReceipt{},
	); err != nil {
		return err
	}

	if err := migrateSmartMoneyFollowConfigTable(); err != nil {
		return err
	}

	if err := repairSmartMoneyFollowJobRowsBeforeMigrate(); err != nil {
		return err
	}
	if err := repairSmartMoneyFollowTaskRowsBeforeMigrate(); err != nil {
		return err
	}

	if err := DB.AutoMigrate(
		&models.SmartMoneyFollowJob{},
		&models.SmartMoneyFollowTask{},
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
	DB.Exec("ALTER TABLE global_configs ALTER COLUMN rebalance_timeout SET DEFAULT 10")
	DB.Exec("ALTER TABLE strategy_tasks ALTER COLUMN reopen_delay_seconds SET DEFAULT 10")
	DB.Exec("ALTER TABLE global_configs MODIFY COLUMN dca_interval_seconds DECIMAL(10,3) NOT NULL DEFAULT 30")
	DB.Exec("ALTER TABLE strategy_tasks MODIFY COLUMN dca_interval_seconds DECIMAL(10,3) NOT NULL DEFAULT 0")

	// Ensure new columns exist (AutoMigrate may skip if table already exists with old schema)
	ensureColumn("sm_wallet_daily_snapshots", "open_lp_usd", "DECIMAL(20,4) NOT NULL DEFAULT 0 AFTER tracked_token_usd")
	ensureColumn("sm_wallet_daily_snapshots", "tracked_token_count", "INT NOT NULL DEFAULT 0 AFTER total_usd")
	ensureColumn("sm_wallet_daily_snapshots", "has_transfer_in", "TINYINT(1) NOT NULL DEFAULT 0 AFTER tracked_token_count")
	ensureColumn("sm_wallet_daily_snapshots", "has_transfer_out", "TINYINT(1) NOT NULL DEFAULT 0 AFTER has_transfer_in")
	ensureColumn("sm_wallet_daily_snapshots", "transfer_in_count", "INT NOT NULL DEFAULT 0 AFTER has_transfer_out")
	ensureColumn("sm_wallet_daily_snapshots", "transfer_out_count", "INT NOT NULL DEFAULT 0 AFTER transfer_in_count")
	ensureColumn("sm_wallet_daily_snapshots", "transfer_in_usd", "DECIMAL(20,4) NOT NULL DEFAULT 0 AFTER transfer_out_count")
	ensureColumn("sm_wallet_daily_snapshots", "transfer_out_usd", "DECIMAL(20,4) NOT NULL DEFAULT 0 AFTER transfer_in_usd")
	ensureColumn("sm_wallet_live_states", "open_lp_usd", "DECIMAL(20,4) NOT NULL DEFAULT 0 AFTER tracked_token_usd")
	ensureColumn("sm_wallet_live_states", "tracked_token_count", "INT NOT NULL DEFAULT 0 AFTER total_usd")
	ensureColumn("sm_wallet_live_states", "active_pool_count", "INT NOT NULL DEFAULT 0 AFTER tracked_token_count")
	ensureColumn("sm_wallet_live_states", "today_event_count", "INT NOT NULL DEFAULT 0 AFTER active_pool_count")
	ensureColumn("sm_lp_events", "liquidity_delta", "DECIMAL(65,0) NOT NULL DEFAULT 0 AFTER token1_symbol")
	ensureColumn("sm_lp_positions", "metadata_status", "VARCHAR(32) NOT NULL DEFAULT '' AFTER tick_upper")
	ensureColumn("sm_lp_positions", "metadata_error", "TEXT NULL AFTER metadata_status")
	if err := ensureUniqueIndex("sm_lp_positions", "uq_sm_nft_chain_protocol", "`chain_id`, `protocol`, `nft_token_id`"); err != nil {
		return err
	}
	dropIndexIfExists("sm_lp_positions", "uq_sm_nft_chain")
	ensureColumn("smart_money_golden_dog_configs", "wallet_min_total_amount_usd", "DOUBLE NOT NULL DEFAULT 0 AFTER cooldown_minutes")
	ensureColumn("smart_money_golden_dog_configs", "wallet_intensity_mode", "VARCHAR(32) NOT NULL DEFAULT 'fixed' AFTER wallet_intensity")
	ensureColumn("smart_money_golden_dog_configs", "wallet_amount_intensity_tiers", "TEXT NULL AFTER wallet_intensity_mode")
	ensureColumn("monitored_wallets", "avatar_url", "VARCHAR(512) NULL AFTER label")
	ensureColumn("trade_records", "open_stable_before", "VARCHAR(78) NOT NULL DEFAULT '0' AFTER open_usdt_spent")
	ensureColumn("trade_records", "open_stable_after", "VARCHAR(78) NOT NULL DEFAULT '0' AFTER open_stable_before")
	ensureColumn("trade_records", "open_extra_dust", "TEXT NULL AFTER open_dust1")
	ensureColumn("user_accesses", "enabled_modules", "TEXT NULL AFTER mini_app_enabled")
	ensureColumn("auth_codes", "enabled_modules", "TEXT NULL AFTER mini_app_enabled")
	ensureColumn("trade_records", "close_stable_before", "VARCHAR(78) NOT NULL DEFAULT '0' AFTER close_usdt_received")
	ensureColumn("trade_records", "close_stable_after", "VARCHAR(78) NOT NULL DEFAULT '0' AFTER close_stable_before")
	ensureColumn("global_configs", "open_position_target_share_min", "DECIMAL(6,4) NOT NULL DEFAULT 0 AFTER multi_wallet_enabled")
	ensureColumn("global_configs", "open_position_target_share_max", "DECIMAL(6,4) NOT NULL DEFAULT 0 AFTER open_position_target_share_min")
	ensureColumn("global_configs", "open_position_risk_cap_usd", "DECIMAL(20,4) NOT NULL DEFAULT 0 AFTER open_position_target_share_max")
	ensureColumn("global_configs", "open_position_risk_cap_ratio", "DECIMAL(6,4) NOT NULL DEFAULT 0 AFTER open_position_risk_cap_usd")
	ensureColumn("global_configs", "dca_min_split_amount_usdt", "DECIMAL(20,4) NOT NULL DEFAULT 0 AFTER dca_interval_seconds")
	ensureColumn("strategy_tasks", "out_of_range_mode", "VARCHAR(40) NOT NULL DEFAULT 'exit_all' AFTER rebalance_enabled")
	ensureColumn("strategy_tasks", "dca_retry_count", "INT NOT NULL DEFAULT 0 AFTER dca_executed_count")
	ensureColumn("strategy_tasks", "range_activation_pending", "TINYINT(1) NOT NULL DEFAULT 0 AFTER out_of_range_since")
	ensureColumn("system_configs", "open_position_target_share_min", "DECIMAL(6,4) NOT NULL DEFAULT 0 AFTER zap_min_pool_liquidity_usd")
	ensureColumn("system_configs", "open_position_target_share_max", "DECIMAL(6,4) NOT NULL DEFAULT 0 AFTER open_position_target_share_min")
	ensureColumn("system_configs", "open_position_risk_cap_usd", "DECIMAL(20,4) NOT NULL DEFAULT 0 AFTER open_position_target_share_max")
	ensureColumn("system_configs", "open_position_risk_cap_ratio", "DECIMAL(6,4) NOT NULL DEFAULT 0 AFTER open_position_risk_cap_usd")
	ensureSmartMoneyQueryIndexes()
	DB.Exec(`
		UPDATE strategy_tasks
		SET out_of_range_mode = CASE
			WHEN rebalance_enabled = 1 THEN 'rebalance_all'
			ELSE 'exit_all'
		END
		WHERE COALESCE(TRIM(out_of_range_mode), '') = ''
	`)
	DB.Exec(`
		UPDATE strategy_tasks
		SET rebalance_enabled = CASE
			WHEN out_of_range_mode IN ('rebalance_all', 'rebalance_up_exit_down') THEN 1
			ELSE 0
		END
		WHERE COALESCE(TRIM(out_of_range_mode), '') <> ''
	`)
	normalizeTradeRecordProfitFormula()
	if err := backfillAccessModulePermissions(); err != nil {
		return err
	}

	return nil
}

func backfillAccessModulePermissions() error {
	if DB == nil {
		return nil
	}
	fullModules, err := models.AccessModuleKeysToJSON(models.DefaultAccessModuleKeys())
	if err != nil {
		return err
	}
	emptyModules, err := models.AccessModuleKeysToJSON([]string{})
	if err != nil {
		return err
	}
	for _, table := range []string{"user_accesses", "auth_codes"} {
		hasColumn, err := tableColumnExists(table, "enabled_modules")
		if err != nil {
			return fmt.Errorf("inspect %s.enabled_modules: %w", table, err)
		}
		if !hasColumn {
			continue
		}
		if err := DB.Exec(fmt.Sprintf(`
			UPDATE %s
			SET enabled_modules = CASE
				WHEN mini_app_enabled = 1 THEN ?
				ELSE ?
			END
			WHERE enabled_modules IS NULL OR TRIM(enabled_modules) = ''
		`, quoteTableName(table)), fullModules, emptyModules).Error; err != nil {
			return fmt.Errorf("backfill %s.enabled_modules: %w", table, err)
		}
	}
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

func ensureColumnExists(table, column, definition string) error {
	if DB == nil {
		return nil
	}
	exists, err := tableColumnExists(table, column)
	if err != nil {
		return fmt.Errorf("inspect column %s.%s: %w", table, column, err)
	}
	if exists {
		return nil
	}
	if err := DB.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", quoteTableName(table), quoteColumnName(column), definition)).Error; err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	log.Printf("[DB] added column %s.%s", table, column)
	return nil
}

func allowNullableLegacyWalletAddress(tableName string) error {
	if DB == nil {
		return nil
	}
	if err := DB.Exec(fmt.Sprintf(
		"ALTER TABLE %s MODIFY COLUMN `wallet_address` VARCHAR(42) NULL",
		quoteTableName(tableName),
	)).Error; err != nil {
		return fmt.Errorf("make legacy %s.wallet_address nullable: %w", tableName, err)
	}
	log.Printf("[DB] made legacy %s.wallet_address nullable", tableName)
	return nil
}

func ensureIndex(table, indexName, columns string) {
	if DB == nil {
		return
	}
	var count int64
	if err := DB.Raw(`
		SELECT COUNT(*)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = ?
		  AND INDEX_NAME = ?
	`, table, indexName).Scan(&count).Error; err != nil {
		log.Printf("[DB] inspect index %s.%s failed: %v", table, indexName, err)
		return
	}
	if count > 0 {
		return
	}
	if err := DB.Exec(fmt.Sprintf("ALTER TABLE `%s` ADD INDEX `%s` (%s)", table, indexName, columns)).Error; err != nil {
		log.Printf("[DB] add index %s.%s failed: %v", table, indexName, err)
		return
	}
	log.Printf("[DB] added index %s.%s", table, indexName)
}

func dropIndexIfExists(table, indexName string) {
	if DB == nil {
		return
	}
	var count int64
	if err := DB.Raw(`
		SELECT COUNT(*)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = ?
		  AND INDEX_NAME = ?
	`, table, indexName).Scan(&count).Error; err != nil {
		log.Printf("[DB] inspect index %s.%s failed: %v", table, indexName, err)
		return
	}
	if count == 0 {
		return
	}
	if err := DB.Exec(fmt.Sprintf("ALTER TABLE %s DROP INDEX `%s`", quoteTableName(table), indexName)).Error; err != nil {
		log.Printf("[DB] drop index %s.%s failed: %v", table, indexName, err)
		return
	}
	log.Printf("[DB] dropped index %s.%s", table, indexName)
}

func ensureSmartMoneyQueryIndexes() {
	ensureIndex("sm_lp_events", "idx_sm_evt_wallet_chain_time", "`wallet_address`, `chain_id`, `tx_timestamp`")
	ensureIndex("sm_lp_events", "idx_sm_evt_wallet_chain_type_time", "`wallet_address`, `chain_id`, `event_type`, `tx_timestamp`")
	ensureIndex("sm_lp_events", "idx_sm_evt_chain_pool_time", "`chain_id`, `pool_address`, `tx_timestamp`")
	ensureIndex("sm_lp_events", "idx_sm_evt_chain_pool_wallet_type_time", "`chain_id`, `pool_address`, `wallet_address`, `event_type`, `tx_timestamp`")
	ensureIndex("sm_lp_events", "idx_sm_evt_chain_type_time", "`chain_id`, `event_type`, `tx_timestamp`")
	ensureIndex("sm_lp_events", "idx_sm_evt_type_chain_protocol_nft", "`event_type`, `chain_id`, `protocol`, `nft_token_id`")
	ensureIndex("sm_lp_events", "idx_sm_evt_chain_protocol_nft_time", "`chain_id`, `protocol`, `nft_token_id`, `tx_timestamp`")

	ensureIndex("sm_lp_positions", "idx_sm_pos_wallet_chain_status_opened", "`wallet_address`, `chain_id`, `status`, `opened_at`")
	ensureIndex("sm_lp_positions", "idx_sm_pos_pool_status_opened", "`pool_address`, `status`, `opened_at`")
	ensureIndex("sm_lp_positions", "idx_sm_pos_status_opened_pool", "`status`, `opened_at`, `pool_address`")
	ensureIndex("sm_lp_positions", "idx_sm_pos_status_closed", "`status`, `closed_at`")
	ensureIndex("sm_lp_positions", "idx_sm_pos_chain_protocol_nft", "`chain_id`, `protocol`, `nft_token_id`")
	ensureIndex("sm_lp_positions", "idx_sm_pos_metadata_status", "`metadata_status`")

	ensureIndex("sm_lp_active_positions", "idx_sm_active_chain_protocol_nft", "`chain_id`, `protocol`, `nft_token_id`")
	ensureIndex("monitored_wallets", "idx_sm_wallet_active_created", "`is_active`, `created_at`")
	ensureIndex("monitored_wallets", "idx_sm_wallet_source_active_created", "`source`, `is_active`, `created_at`")
	ensureIndex("monitored_wallets", "idx_sm_wallet_active_address_chain", "`is_active`, `address`, `chain_id`")
	ensureIndex("watch_contracts", "idx_sm_watch_contract_active", "`is_active`")
	ensureIndex("smart_money_user_watch_wallets", "idx_sm_watch_wallet_chain_user_addr", "`chain`, `user_id`, `wallet_address`")

	ensureIndex("sm_wallet_daily_snapshots", "idx_sm_wallet_snapshot_day_total", "`snapshot_day`, `total_usd`")
	ensureIndex("sm_wallet_daily_snapshots", "idx_sm_wallet_snapshot_day_wallet", "`snapshot_day`, `wallet_address`, `chain_id`")
	ensureIndex("sm_wallet_daily_snapshots", "idx_sm_wallet_snapshot_chain_wallet_day", "`chain_id`, `wallet_address`, `snapshot_day`")

	ensureIndex("sm_lp_daily_stats", "idx_sm_lp_stat_day_wallet", "`stat_day`, `wallet_address`, `chain_id`")
	ensureIndex("sm_lp_daily_stats", "idx_sm_lp_stat_day_pnl", "`stat_day`, `estimated_realized_pnl_usd`")

	ensureIndex("sm_wallet_live_states", "idx_sm_wallet_live_refreshed", "`refreshed_at`")
	ensureIndex("sm_wallet_live_states", "idx_sm_wallet_live_total", "`total_usd`")
	ensureIndex("sm_wallet_live_states", "idx_sm_wallet_live_chain_wallet", "`chain_id`, `wallet_address`")

	ensureIndex("pools", "idx_pools_chain_address", "`chain`, `address`")
	ensureIndex("pools", "idx_pools_chain_updated_at", "`chain`, `updated_at`")
	ensureIndex("pools", "idx_pools_source_chain_updated_at", "`source_requested_chain`, `updated_at`")
}

func migrateSmartMoneyFollowConfigTable() error {
	if DB == nil {
		return nil
	}

	model := &models.SmartMoneyFollowConfig{}
	if err := repairSmartMoneyFollowConfigRowsBeforeMigrate(model.TableName()); err != nil {
		return err
	}
	if err := DB.AutoMigrate(model); err != nil {
		return err
	}
	if err := cleanupSmartMoneyFollowConfigRows(model.TableName()); err != nil {
		return err
	}

	return ensureUniqueIndex(model.TableName(), "uq_sm_follow_user_chain_wallet", "`user_id`, `chain`, `target_wallet_address`")
}

func repairSmartMoneyFollowConfigRowsBeforeMigrate(tableName string) error {
	exists, err := tableExists(tableName)
	if err != nil {
		return fmt.Errorf("inspect %s before migrate: %w", tableName, err)
	}
	if !exists {
		return nil
	}

	if err := ensureColumnExists(tableName, "target_wallet_address", "VARCHAR(42) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "target_wallet_addresses", "JSON NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "execution_wallet_id", "BIGINT UNSIGNED NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "execution_wallet_address", "VARCHAR(42) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "trigger_mode", "VARCHAR(16) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "trigger_min_wallets", "BIGINT NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "trigger_window_seconds", "BIGINT NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "chain_id", "BIGINT NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "amount_mode", "VARCHAR(16) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "fixed_amount_usdt", "DECIMAL(20,8) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "ratio", "DECIMAL(12,8) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "delay_mode", "VARCHAR(20) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "delay_seconds", "BIGINT NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "follow_close", "TINYINT(1) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "cursor_event_id", "BIGINT UNSIGNED NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "last_seen_event_id", "BIGINT UNSIGNED NULL"); err != nil {
		return err
	}

	hasWalletAddress, err := tableColumnExists(tableName, "wallet_address")
	if err != nil {
		return fmt.Errorf("inspect %s.wallet_address before migrate: %w", tableName, err)
	}
	if hasWalletAddress {
		if err := DB.Exec(fmt.Sprintf(`
			UPDATE %s
			SET target_wallet_address = LOWER(TRIM(wallet_address))
			WHERE COALESCE(TRIM(target_wallet_address), '') = ''
			  AND COALESCE(TRIM(wallet_address), '') <> ''
		`, quoteTableName(tableName))).Error; err != nil {
			return fmt.Errorf("backfill %s.target_wallet_address: %w", tableName, err)
		}
		if err := allowNullableLegacyWalletAddress(tableName); err != nil {
			return err
		}
	}
	if err := DB.Exec(fmt.Sprintf(`
		DELETE FROM %s
		WHERE COALESCE(TRIM(target_wallet_address), '') = ''
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("delete empty %s target_wallet_address rows before migrate: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET target_wallet_address = LOWER(TRIM(target_wallet_address))
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("normalize %s.target_wallet_address before migrate: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET target_wallet_addresses = JSON_ARRAY(target_wallet_address)
		WHERE (target_wallet_addresses IS NULL OR JSON_LENGTH(target_wallet_addresses) = 0)
		  AND COALESCE(TRIM(target_wallet_address), '') <> ''
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.target_wallet_addresses: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s AS cfg
		INNER JOIN wallets AS w
			ON w.user_id = cfg.user_id
			AND w.deleted_at IS NULL
			AND (
				(cfg.execution_wallet_id IS NOT NULL AND cfg.execution_wallet_id > 0 AND w.id = cfg.execution_wallet_id)
				OR (
					(cfg.execution_wallet_id IS NULL OR cfg.execution_wallet_id = 0)
					AND COALESCE(TRIM(cfg.execution_wallet_address), '') <> ''
					AND LOWER(w.address) = LOWER(TRIM(cfg.execution_wallet_address))
				)
			)
		SET cfg.execution_wallet_id = w.id,
		    cfg.execution_wallet_address = w.address
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.execution_wallet fields: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET execution_wallet_address = LOWER(TRIM(execution_wallet_address))
		WHERE COALESCE(TRIM(execution_wallet_address), '') <> ''
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("normalize %s.execution_wallet_address: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET trigger_mode = '%s'
		WHERE COALESCE(TRIM(trigger_mode), '') = ''
	`, quoteTableName(tableName), models.SmartMoneyFollowTriggerModeAny)).Error; err != nil {
		return fmt.Errorf("backfill %s.trigger_mode: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET trigger_min_wallets = 1
		WHERE trigger_min_wallets IS NULL OR trigger_min_wallets <= 0
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.trigger_min_wallets: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET trigger_window_seconds = 300
		WHERE trigger_window_seconds IS NULL OR trigger_window_seconds <= 0
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.trigger_window_seconds: %w", tableName, err)
	}

	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET chain_id = CASE LOWER(TRIM(chain))
			WHEN 'base' THEN 8453
			WHEN 'bsc' THEN 56
			WHEN '' THEN 56
			ELSE chain_id
		END
		WHERE chain_id IS NULL OR chain_id <= 0
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.chain_id: %w", tableName, err)
	}

	hasPerTradeAmount, err := tableColumnExists(tableName, "per_trade_amount_usdt")
	if err != nil {
		return fmt.Errorf("inspect %s.per_trade_amount_usdt before migrate: %w", tableName, err)
	}
	if hasPerTradeAmount {
		if err := DB.Exec(fmt.Sprintf(`
			UPDATE %s
			SET fixed_amount_usdt = per_trade_amount_usdt
			WHERE (fixed_amount_usdt IS NULL OR fixed_amount_usdt = 0)
			  AND per_trade_amount_usdt IS NOT NULL
		`, quoteTableName(tableName))).Error; err != nil {
			return fmt.Errorf("backfill %s.fixed_amount_usdt: %w", tableName, err)
		}
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET fixed_amount_usdt = 0
		WHERE fixed_amount_usdt IS NULL
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("finalize %s.fixed_amount_usdt: %w", tableName, err)
	}

	hasDelayMax, err := tableColumnExists(tableName, "delay_max_seconds")
	if err != nil {
		return fmt.Errorf("inspect %s.delay_max_seconds before migrate: %w", tableName, err)
	}
	if hasDelayMax {
		if err := DB.Exec(fmt.Sprintf(`
			UPDATE %s
			SET delay_seconds = delay_max_seconds
			WHERE delay_seconds IS NULL
			  AND delay_max_seconds IS NOT NULL
		`, quoteTableName(tableName))).Error; err != nil {
			return fmt.Errorf("backfill %s.delay_seconds: %w", tableName, err)
		}
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET delay_seconds = 0
		WHERE delay_seconds IS NULL
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("finalize %s.delay_seconds: %w", tableName, err)
	}

	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET amount_mode = '%s'
		WHERE COALESCE(TRIM(amount_mode), '') = ''
	`, quoteTableName(tableName), models.SmartMoneyFollowAmountModeFixed)).Error; err != nil {
		return fmt.Errorf("backfill %s.amount_mode: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET ratio = 1
		WHERE ratio IS NULL OR ratio <= 0
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.ratio: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET delay_mode = CASE
			WHEN COALESCE(delay_seconds, 0) > 0 THEN '%s'
			ELSE '%s'
		END
		WHERE COALESCE(TRIM(delay_mode), '') = ''
	`, quoteTableName(tableName), models.SmartMoneyFollowDelayModeFixed, models.SmartMoneyFollowDelayModeImmediate)).Error; err != nil {
		return fmt.Errorf("backfill %s.delay_mode: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET follow_close = 1
		WHERE follow_close IS NULL
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.follow_close: %w", tableName, err)
	}

	if err := backfillSmartMoneyFollowConfigCursors(tableName); err != nil {
		return err
	}

	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET chain_id = 56
		WHERE chain_id IS NULL OR chain_id <= 0
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("finalize %s.chain_id: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET execution_wallet_id = 0
		WHERE execution_wallet_id IS NULL
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("finalize %s.execution_wallet_id: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET execution_wallet_address = ''
		WHERE execution_wallet_address IS NULL
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("finalize %s.execution_wallet_address: %w", tableName, err)
	}
	return nil
}

func backfillSmartMoneyFollowConfigCursors(tableName string) error {
	eventsTable := (&models.SmartMoneyLPEvent{}).TableName()
	eventsExists, err := tableExists(eventsTable)
	if err != nil {
		return fmt.Errorf("inspect %s before follow config cursor migrate: %w", eventsTable, err)
	}
	if !eventsExists {
		if err := DB.Exec(fmt.Sprintf(`
			UPDATE %s
			SET cursor_event_id = COALESCE(cursor_event_id, 0),
			    last_seen_event_id = COALESCE(last_seen_event_id, 0)
		`, quoteTableName(tableName))).Error; err != nil {
			return fmt.Errorf("backfill %s cursors without events: %w", tableName, err)
		}
		return nil
	}

	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s AS cfg
		LEFT JOIN (
			SELECT wallet_address, chain_id, MAX(id) AS latest_id
			FROM %s
			GROUP BY wallet_address, chain_id
		) AS evt
			ON evt.wallet_address = cfg.target_wallet_address
			AND evt.chain_id = cfg.chain_id
		SET cfg.cursor_event_id = COALESCE(NULLIF(cfg.cursor_event_id, 0), COALESCE(evt.latest_id, 0)),
		    cfg.last_seen_event_id = COALESCE(NULLIF(cfg.last_seen_event_id, 0), COALESCE(evt.latest_id, 0))
		WHERE cfg.cursor_event_id IS NULL
		   OR cfg.cursor_event_id = 0
		   OR cfg.last_seen_event_id IS NULL
		   OR cfg.last_seen_event_id = 0
	`, quoteTableName(tableName), quoteTableName(eventsTable))).Error; err != nil {
		return fmt.Errorf("backfill %s cursors: %w", tableName, err)
	}
	return nil
}

func cleanupSmartMoneyFollowConfigRows(tableName string) error {
	if err := DB.Exec(fmt.Sprintf(`
		DELETE FROM %s
		WHERE COALESCE(TRIM(target_wallet_address), '') = ''
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("delete empty smart money follow wallet rows: %w", err)
	}

	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET target_wallet_address = LOWER(TRIM(target_wallet_address))
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("normalize smart money follow wallet rows: %w", err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET target_wallet_addresses = JSON_ARRAY(target_wallet_address)
		WHERE (target_wallet_addresses IS NULL OR JSON_LENGTH(target_wallet_addresses) = 0)
		  AND COALESCE(TRIM(target_wallet_address), '') <> ''
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill smart money follow wallet array rows: %w", err)
	}

	if err := DB.Exec(fmt.Sprintf(`
		DELETE older
		FROM %s AS older
		INNER JOIN %s AS newer
			ON older.user_id = newer.user_id
			AND older.chain = newer.chain
			AND older.target_wallet_address = newer.target_wallet_address
			AND (
				COALESCE(older.updated_at, '1000-01-01 00:00:00') < COALESCE(newer.updated_at, '1000-01-01 00:00:00')
				OR (
					COALESCE(older.updated_at, '1000-01-01 00:00:00') = COALESCE(newer.updated_at, '1000-01-01 00:00:00')
					AND older.id < newer.id
				)
			)
	`, quoteTableName(tableName), quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("dedupe smart money follow wallet rows: %w", err)
	}

	return nil
}

func repairSmartMoneyFollowJobRowsBeforeMigrate() error {
	if DB == nil {
		return nil
	}

	tableName := (&models.SmartMoneyFollowJob{}).TableName()
	exists, err := tableExists(tableName)
	if err != nil {
		return fmt.Errorf("inspect %s before migrate: %w", tableName, err)
	}
	if !exists {
		return nil
	}

	if err := ensureColumnExists(tableName, "config_id", "BIGINT UNSIGNED NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "chain_id", "BIGINT NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "target_wallet_address", "VARCHAR(42) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "execution_wallet_id", "BIGINT UNSIGNED NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "execution_wallet_address", "VARCHAR(42) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "event_id", "BIGINT UNSIGNED NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "trigger_mode", "VARCHAR(16) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "trigger_wallet_addresses", "JSON NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "trigger_event_ids", "JSON NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "target_position_ref", "VARCHAR(255) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "started_at", "DATETIME(3) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "finished_at", "DATETIME(3) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "amount_usdt", "DECIMAL(20,8) NULL"); err != nil {
		return err
	}

	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET chain_id = CASE LOWER(TRIM(chain))
			WHEN 'base' THEN 8453
			WHEN 'bsc' THEN 56
			WHEN '' THEN 56
			ELSE chain_id
		END
		WHERE chain_id IS NULL OR chain_id <= 0
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.chain_id: %w", tableName, err)
	}

	hasWalletAddress, err := tableColumnExists(tableName, "wallet_address")
	if err != nil {
		return fmt.Errorf("inspect %s.wallet_address before migrate: %w", tableName, err)
	}
	if hasWalletAddress {
		if err := DB.Exec(fmt.Sprintf(`
			UPDATE %s
			SET target_wallet_address = LOWER(TRIM(wallet_address))
			WHERE COALESCE(TRIM(target_wallet_address), '') = ''
			  AND COALESCE(TRIM(wallet_address), '') <> ''
		`, quoteTableName(tableName))).Error; err != nil {
			return fmt.Errorf("backfill %s.target_wallet_address: %w", tableName, err)
		}
		if err := allowNullableLegacyWalletAddress(tableName); err != nil {
			return err
		}
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET target_wallet_address = LOWER(TRIM(target_wallet_address))
		WHERE COALESCE(TRIM(target_wallet_address), '') <> ''
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("normalize %s.target_wallet_address: %w", tableName, err)
	}

	if err := backfillSmartMoneyFollowJobConfigIDs(tableName); err != nil {
		return err
	}
	if err := backfillSmartMoneyFollowJobExecutionWallets(tableName); err != nil {
		return err
	}

	hasEventSeq, err := tableColumnExists(tableName, "event_seq")
	if err != nil {
		return fmt.Errorf("inspect %s.event_seq before migrate: %w", tableName, err)
	}
	if hasEventSeq {
		if err := DB.Exec(fmt.Sprintf(`
			UPDATE %s
			SET event_id = event_seq
			WHERE (event_id IS NULL OR event_id = 0)
			  AND event_seq > 0
		`, quoteTableName(tableName))).Error; err != nil {
			return fmt.Errorf("backfill %s.event_id from event_seq: %w", tableName, err)
		}
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET event_id = 1000000000000000000 + id
		WHERE event_id IS NULL OR event_id = 0
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.event_id sentinel: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET trigger_mode = '%s'
		WHERE COALESCE(TRIM(trigger_mode), '') = ''
	`, quoteTableName(tableName), models.SmartMoneyFollowTriggerModeAny)).Error; err != nil {
		return fmt.Errorf("backfill %s.trigger_mode: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET trigger_wallet_addresses = JSON_ARRAY(target_wallet_address)
		WHERE (trigger_wallet_addresses IS NULL OR JSON_LENGTH(trigger_wallet_addresses) = 0)
		  AND COALESCE(TRIM(target_wallet_address), '') <> ''
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.trigger_wallet_addresses: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET trigger_event_ids = JSON_ARRAY(CAST(event_id AS CHAR))
		WHERE trigger_event_ids IS NULL OR JSON_LENGTH(trigger_event_ids) = 0
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.trigger_event_ids: %w", tableName, err)
	}

	if err := backfillSmartMoneyFollowJobPositionRefs(tableName); err != nil {
		return err
	}

	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET action = CASE LOWER(TRIM(action))
			WHEN 'add' THEN '%s'
			WHEN 'open' THEN '%s'
			WHEN 'remove' THEN '%s'
			WHEN 'close' THEN '%s'
			ELSE '%s'
		END
	`, quoteTableName(tableName),
		models.SmartMoneyFollowJobActionOpen,
		models.SmartMoneyFollowJobActionOpen,
		models.SmartMoneyFollowJobActionClose,
		models.SmartMoneyFollowJobActionClose,
		models.SmartMoneyFollowJobActionOpen,
	)).Error; err != nil {
		return fmt.Errorf("normalize %s.action: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET status = CASE LOWER(TRIM(status))
			WHEN 'done' THEN '%s'
			WHEN 'completed' THEN '%s'
			WHEN 'success' THEN '%s'
			WHEN 'skip' THEN '%s'
			WHEN 'skipped' THEN '%s'
			WHEN 'error' THEN '%s'
			WHEN 'failed' THEN '%s'
			WHEN 'running' THEN '%s'
			WHEN 'pending' THEN '%s'
			ELSE '%s'
		END
	`, quoteTableName(tableName),
		models.SmartMoneyFollowJobStatusSuccess,
		models.SmartMoneyFollowJobStatusSuccess,
		models.SmartMoneyFollowJobStatusSuccess,
		models.SmartMoneyFollowJobStatusSkipped,
		models.SmartMoneyFollowJobStatusSkipped,
		models.SmartMoneyFollowJobStatusFailed,
		models.SmartMoneyFollowJobStatusFailed,
		models.SmartMoneyFollowJobStatusRunning,
		models.SmartMoneyFollowJobStatusPending,
		models.SmartMoneyFollowJobStatusFailed,
	)).Error; err != nil {
		return fmt.Errorf("normalize %s.status: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET amount_usdt = 0
		WHERE amount_usdt IS NULL
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.amount_usdt: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET execution_wallet_id = 0
		WHERE execution_wallet_id IS NULL
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("finalize %s.execution_wallet_id: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET execution_wallet_address = ''
		WHERE execution_wallet_address IS NULL
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("finalize %s.execution_wallet_address: %w", tableName, err)
	}

	hasScheduledAt, err := tableColumnExists(tableName, "scheduled_at")
	if err != nil {
		return fmt.Errorf("inspect %s.scheduled_at before migrate: %w", tableName, err)
	}
	if !hasScheduledAt {
		if err := DB.Exec(fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN `scheduled_at` DATETIME(3) NULL",
			quoteTableName(tableName),
		)).Error; err != nil {
			return fmt.Errorf("add nullable scheduled_at to %s before migrate: %w", tableName, err)
		}
		log.Printf("[DB] added nullable %s.scheduled_at before migration backfill", tableName)
	}

	sourceColumns := make([]string, 0, 5)
	for _, column := range []string{"execute_at", "created_at", "updated_at", "started_at", "finished_at"} {
		exists, err := tableColumnExists(tableName, column)
		if err != nil {
			return fmt.Errorf("inspect %s.%s before migrate: %w", tableName, column, err)
		}
		if exists {
			sourceColumns = append(sourceColumns, column)
		}
	}

	caseLines := make([]string, 0, len(sourceColumns)+1)
	for _, column := range sourceColumns {
		quotedColumn := quoteColumnName(column)
		caseLines = append(caseLines, fmt.Sprintf(
			"WHEN CAST(%s AS CHAR) NOT IN ('', '0000-00-00 00:00:00') THEN %s",
			quotedColumn,
			quotedColumn,
		))
	}
	caseLines = append(caseLines, "ELSE CURRENT_TIMESTAMP")

	tx := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET scheduled_at = CASE
			%s
		END
		WHERE scheduled_at IS NULL
		   OR CAST(scheduled_at AS CHAR) IN ('', '0000-00-00 00:00:00')
	`, quoteTableName(tableName), strings.Join(caseLines, "\n\t\t\t")))
	if tx.Error != nil {
		return fmt.Errorf("repair zero scheduled_at in %s: %w", tableName, tx.Error)
	}
	if tx.RowsAffected > 0 {
		log.Printf("[DB] repaired %d %s rows with invalid scheduled_at", tx.RowsAffected, tableName)
	}

	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET finished_at = CASE
			WHEN CAST(updated_at AS CHAR) NOT IN ('', '0000-00-00 00:00:00') THEN updated_at
			WHEN CAST(scheduled_at AS CHAR) NOT IN ('', '0000-00-00 00:00:00') THEN scheduled_at
			WHEN CAST(created_at AS CHAR) NOT IN ('', '0000-00-00 00:00:00') THEN created_at
			ELSE CURRENT_TIMESTAMP
		END
		WHERE finished_at IS NULL
		  AND status IN ('%s', '%s', '%s')
	`, quoteTableName(tableName),
		models.SmartMoneyFollowJobStatusSuccess,
		models.SmartMoneyFollowJobStatusFailed,
		models.SmartMoneyFollowJobStatusSkipped,
	)).Error; err != nil {
		return fmt.Errorf("backfill terminal %s.finished_at: %w", tableName, err)
	}

	if err := failUnexecutableSmartMoneyFollowJobs(tableName); err != nil {
		return err
	}
	if err := repairSmartMoneyFollowJobUniqueKeys(tableName); err != nil {
		return err
	}

	if err := DB.Exec(fmt.Sprintf(
		"ALTER TABLE %s MODIFY COLUMN `scheduled_at` DATETIME(3) NOT NULL",
		quoteTableName(tableName),
	)).Error; err != nil {
		return fmt.Errorf("make %s.scheduled_at not null: %w", tableName, err)
	}
	return nil
}

func backfillSmartMoneyFollowJobConfigIDs(tableName string) error {
	configTable := (&models.SmartMoneyFollowConfig{}).TableName()
	configExists, err := tableExists(configTable)
	if err != nil {
		return fmt.Errorf("inspect %s before follow job config backfill: %w", configTable, err)
	}
	if !configExists {
		return nil
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s AS job
		INNER JOIN %s AS cfg
			ON cfg.user_id = job.user_id
			AND cfg.chain = job.chain
			AND cfg.target_wallet_address = job.target_wallet_address
		SET job.config_id = cfg.id
		WHERE (job.config_id IS NULL OR job.config_id = 0)
		  AND COALESCE(TRIM(job.target_wallet_address), '') <> ''
	`, quoteTableName(tableName), quoteTableName(configTable))).Error; err != nil {
		return fmt.Errorf("backfill %s.config_id: %w", tableName, err)
	}
	return nil
}

func backfillSmartMoneyFollowJobExecutionWallets(tableName string) error {
	configTable := (&models.SmartMoneyFollowConfig{}).TableName()
	configExists, err := tableExists(configTable)
	if err != nil {
		return fmt.Errorf("inspect %s before follow job execution wallet backfill: %w", configTable, err)
	}
	if !configExists {
		return nil
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s AS job
		INNER JOIN %s AS cfg
			ON cfg.id = job.config_id
		SET job.execution_wallet_id = cfg.execution_wallet_id,
		    job.execution_wallet_address = cfg.execution_wallet_address
		WHERE (job.execution_wallet_id IS NULL OR job.execution_wallet_id = 0)
		  AND cfg.execution_wallet_id > 0
	`, quoteTableName(tableName), quoteTableName(configTable))).Error; err != nil {
		return fmt.Errorf("backfill %s.execution_wallet fields: %w", tableName, err)
	}
	return nil
}

func backfillSmartMoneyFollowJobPositionRefs(tableName string) error {
	hasPoolID, err := tableColumnExists(tableName, "pool_id")
	if err != nil {
		return fmt.Errorf("inspect %s.pool_id before migrate: %w", tableName, err)
	}
	if !hasPoolID {
		if err := DB.Exec(fmt.Sprintf(`
			UPDATE %s
			SET target_position_ref = LOWER(CONCAT('legacy:', id))
			WHERE COALESCE(TRIM(target_position_ref), '') = ''
		`, quoteTableName(tableName))).Error; err != nil {
			return fmt.Errorf("backfill %s.target_position_ref: %w", tableName, err)
		}
		return nil
	}

	protocolExpr := "'legacy'"
	if exists, err := tableColumnExists(tableName, "pool_version"); err != nil {
		return fmt.Errorf("inspect %s.pool_version before migrate: %w", tableName, err)
	} else if exists {
		protocolExpr = "COALESCE(NULLIF(TRIM(`pool_version`), ''), 'legacy')"
	}
	lowerExpr := "''"
	if exists, err := tableColumnExists(tableName, "tick_lower"); err != nil {
		return fmt.Errorf("inspect %s.tick_lower before migrate: %w", tableName, err)
	} else if exists {
		lowerExpr = "COALESCE(CAST(`tick_lower` AS CHAR), '')"
	}
	upperExpr := "''"
	if exists, err := tableColumnExists(tableName, "tick_upper"); err != nil {
		return fmt.Errorf("inspect %s.tick_upper before migrate: %w", tableName, err)
	} else if exists {
		upperExpr = "COALESCE(CAST(`tick_upper` AS CHAR), '')"
	}

	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET target_position_ref = LOWER(CONCAT(
			'legacy:',
			COALESCE(CAST(chain_id AS CHAR), '0'), ':',
			COALESCE(NULLIF(TRIM(target_wallet_address), ''), 'unknown'), ':',
			%s, ':',
			COALESCE(NULLIF(TRIM(pool_id), ''), CAST(id AS CHAR)), ':',
			%s, ':',
			%s
		))
		WHERE COALESCE(TRIM(target_position_ref), '') = ''
	`, quoteTableName(tableName), protocolExpr, lowerExpr, upperExpr)).Error; err != nil {
		return fmt.Errorf("backfill %s.target_position_ref: %w", tableName, err)
	}
	return nil
}

func failUnexecutableSmartMoneyFollowJobs(tableName string) error {
	eventsTable := (&models.SmartMoneyLPEvent{}).TableName()
	eventsExists, err := tableExists(eventsTable)
	if err != nil {
		return fmt.Errorf("inspect %s before follow job validation: %w", eventsTable, err)
	}
	if !eventsExists {
		return nil
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s AS job
		LEFT JOIN %s AS evt
			ON evt.id = job.event_id
		SET job.status = '%s',
		    job.error_message = CONCAT('legacy follow job cannot be executed after schema migration', CASE
			WHEN COALESCE(TRIM(job.error_message), '') = '' THEN ''
			ELSE CONCAT(': ', job.error_message)
		    END),
		    job.finished_at = COALESCE(job.finished_at, job.updated_at, job.scheduled_at, job.created_at, CURRENT_TIMESTAMP)
		WHERE job.status IN ('%s', '%s')
		  AND evt.id IS NULL
	`, quoteTableName(tableName),
		quoteTableName(eventsTable),
		models.SmartMoneyFollowJobStatusFailed,
		models.SmartMoneyFollowJobStatusPending,
		models.SmartMoneyFollowJobStatusRunning,
	)).Error; err != nil {
		return fmt.Errorf("mark unexecutable %s rows failed: %w", tableName, err)
	}
	return nil
}

func repairSmartMoneyFollowJobUniqueKeys(tableName string) error {
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s AS job
		INNER JOIN %s AS keeper
			ON keeper.config_id = job.config_id
			AND keeper.event_id = job.event_id
			AND keeper.action = job.action
			AND keeper.id < job.id
		SET job.event_id = 1000000000000000000 + job.id
	`, quoteTableName(tableName), quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("repair duplicate %s unique keys: %w", tableName, err)
	}
	return nil
}

func repairSmartMoneyFollowTaskRowsBeforeMigrate() error {
	if DB == nil {
		return nil
	}

	tableName := (&models.SmartMoneyFollowTask{}).TableName()
	exists, err := tableExists(tableName)
	if err != nil {
		return fmt.Errorf("inspect %s before migrate: %w", tableName, err)
	}
	if !exists {
		return nil
	}

	if err := ensureColumnExists(tableName, "config_id", "BIGINT UNSIGNED NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "chain_id", "BIGINT NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "target_wallet_address", "VARCHAR(42) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "execution_wallet_id", "BIGINT UNSIGNED NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "execution_wallet_address", "VARCHAR(42) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "target_position_ref", "VARCHAR(255) NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "open_event_id", "BIGINT UNSIGNED NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "open_job_id", "BIGINT UNSIGNED NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "close_event_id", "BIGINT UNSIGNED NULL"); err != nil {
		return err
	}
	if err := ensureColumnExists(tableName, "close_job_id", "BIGINT UNSIGNED NULL"); err != nil {
		return err
	}

	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET chain_id = CASE LOWER(TRIM(chain))
			WHEN 'base' THEN 8453
			WHEN 'bsc' THEN 56
			WHEN '' THEN 56
			ELSE chain_id
		END
		WHERE chain_id IS NULL OR chain_id <= 0
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.chain_id: %w", tableName, err)
	}

	hasWalletAddress, err := tableColumnExists(tableName, "wallet_address")
	if err != nil {
		return fmt.Errorf("inspect %s.wallet_address before migrate: %w", tableName, err)
	}
	if hasWalletAddress {
		if err := DB.Exec(fmt.Sprintf(`
			UPDATE %s
			SET target_wallet_address = LOWER(TRIM(wallet_address))
			WHERE COALESCE(TRIM(target_wallet_address), '') = ''
			  AND COALESCE(TRIM(wallet_address), '') <> ''
		`, quoteTableName(tableName))).Error; err != nil {
			return fmt.Errorf("backfill %s.target_wallet_address: %w", tableName, err)
		}
		if err := allowNullableLegacyWalletAddress(tableName); err != nil {
			return err
		}
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET target_wallet_address = LOWER(TRIM(target_wallet_address))
		WHERE COALESCE(TRIM(target_wallet_address), '') <> ''
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("normalize %s.target_wallet_address: %w", tableName, err)
	}

	if err := backfillSmartMoneyFollowTaskConfigIDs(tableName); err != nil {
		return err
	}
	if err := backfillSmartMoneyFollowTaskExecutionWallets(tableName); err != nil {
		return err
	}
	if err := backfillSmartMoneyFollowTaskPositionRefs(tableName); err != nil {
		return err
	}
	if err := backfillSmartMoneyFollowTaskEvents(tableName); err != nil {
		return err
	}
	if err := backfillSmartMoneyFollowTaskJobs(tableName); err != nil {
		return err
	}
	if err := normalizeSmartMoneyFollowTaskStatuses(tableName); err != nil {
		return err
	}
	if err := repairSmartMoneyFollowTaskUniqueKeys(tableName); err != nil {
		return err
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET execution_wallet_id = 0
		WHERE execution_wallet_id IS NULL
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("finalize %s.execution_wallet_id: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET execution_wallet_address = ''
		WHERE execution_wallet_address IS NULL
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("finalize %s.execution_wallet_address: %w", tableName, err)
	}
	return nil
}

func backfillSmartMoneyFollowTaskConfigIDs(tableName string) error {
	configTable := (&models.SmartMoneyFollowConfig{}).TableName()
	configExists, err := tableExists(configTable)
	if err != nil {
		return fmt.Errorf("inspect %s before follow task config backfill: %w", configTable, err)
	}
	if !configExists {
		return nil
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s AS task
		INNER JOIN %s AS cfg
			ON cfg.user_id = task.user_id
			AND cfg.chain = task.chain
			AND cfg.target_wallet_address = task.target_wallet_address
		SET task.config_id = cfg.id
		WHERE (task.config_id IS NULL OR task.config_id = 0)
		  AND COALESCE(TRIM(task.target_wallet_address), '') <> ''
	`, quoteTableName(tableName), quoteTableName(configTable))).Error; err != nil {
		return fmt.Errorf("backfill %s.config_id: %w", tableName, err)
	}
	return nil
}

func backfillSmartMoneyFollowTaskExecutionWallets(tableName string) error {
	configTable := (&models.SmartMoneyFollowConfig{}).TableName()
	configExists, err := tableExists(configTable)
	if err != nil {
		return fmt.Errorf("inspect %s before follow task execution wallet backfill: %w", configTable, err)
	}
	if !configExists {
		return nil
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s AS task
		INNER JOIN %s AS cfg
			ON cfg.id = task.config_id
		SET task.execution_wallet_id = cfg.execution_wallet_id,
		    task.execution_wallet_address = cfg.execution_wallet_address
		WHERE (task.execution_wallet_id IS NULL OR task.execution_wallet_id = 0)
		  AND cfg.execution_wallet_id > 0
	`, quoteTableName(tableName), quoteTableName(configTable))).Error; err != nil {
		return fmt.Errorf("backfill %s.execution_wallet fields: %w", tableName, err)
	}
	return nil
}

func backfillSmartMoneyFollowTaskPositionRefs(tableName string) error {
	hasPoolID, err := tableColumnExists(tableName, "pool_id")
	if err != nil {
		return fmt.Errorf("inspect %s.pool_id before migrate: %w", tableName, err)
	}
	if !hasPoolID {
		if err := DB.Exec(fmt.Sprintf(`
			UPDATE %s
			SET target_position_ref = LOWER(CONCAT('legacy:', id))
			WHERE COALESCE(TRIM(target_position_ref), '') = ''
		`, quoteTableName(tableName))).Error; err != nil {
			return fmt.Errorf("backfill %s.target_position_ref: %w", tableName, err)
		}
		return nil
	}

	protocolExpr := "'legacy'"
	if exists, err := tableColumnExists(tableName, "pool_version"); err != nil {
		return fmt.Errorf("inspect %s.pool_version before migrate: %w", tableName, err)
	} else if exists {
		protocolExpr = "COALESCE(NULLIF(TRIM(`pool_version`), ''), 'legacy')"
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET target_position_ref = LOWER(CONCAT(
			'legacy:',
			COALESCE(CAST(chain_id AS CHAR), '0'), ':',
			COALESCE(NULLIF(TRIM(target_wallet_address), ''), 'unknown'), ':',
			%s, ':',
			COALESCE(NULLIF(TRIM(pool_id), ''), CAST(id AS CHAR))
		))
		WHERE COALESCE(TRIM(target_position_ref), '') = ''
	`, quoteTableName(tableName), protocolExpr)).Error; err != nil {
		return fmt.Errorf("backfill %s.target_position_ref: %w", tableName, err)
	}
	return nil
}

func backfillSmartMoneyFollowTaskEvents(tableName string) error {
	hasLastAddEventSeq, err := tableColumnExists(tableName, "last_add_event_seq")
	if err != nil {
		return fmt.Errorf("inspect %s.last_add_event_seq before migrate: %w", tableName, err)
	}
	if hasLastAddEventSeq {
		if err := DB.Exec(fmt.Sprintf(`
			UPDATE %s
			SET open_event_id = last_add_event_seq
			WHERE (open_event_id IS NULL OR open_event_id = 0)
			  AND last_add_event_seq > 0
		`, quoteTableName(tableName))).Error; err != nil {
			return fmt.Errorf("backfill %s.open_event_id: %w", tableName, err)
		}
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET open_event_id = 1000000000000000000 + id
		WHERE open_event_id IS NULL OR open_event_id = 0
	`, quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("backfill %s.open_event_id sentinel: %w", tableName, err)
	}

	hasLastRemoveEventSeq, err := tableColumnExists(tableName, "last_remove_event_seq")
	if err != nil {
		return fmt.Errorf("inspect %s.last_remove_event_seq before migrate: %w", tableName, err)
	}
	if hasLastRemoveEventSeq {
		if err := DB.Exec(fmt.Sprintf(`
			UPDATE %s
			SET close_event_id = last_remove_event_seq
			WHERE (close_event_id IS NULL OR close_event_id = 0)
			  AND last_remove_event_seq > 0
		`, quoteTableName(tableName))).Error; err != nil {
			return fmt.Errorf("backfill %s.close_event_id: %w", tableName, err)
		}
	}
	return nil
}

func backfillSmartMoneyFollowTaskJobs(tableName string) error {
	jobsTable := (&models.SmartMoneyFollowJob{}).TableName()
	jobsExists, err := tableExists(jobsTable)
	if err != nil {
		return fmt.Errorf("inspect %s before follow task job backfill: %w", jobsTable, err)
	}
	if !jobsExists {
		return nil
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s AS task
		LEFT JOIN %s AS job
			ON job.config_id = task.config_id
			AND job.event_id = task.open_event_id
			AND job.action = '%s'
		SET task.open_job_id = CASE
			WHEN task.open_job_id IS NULL OR task.open_job_id = 0 THEN COALESCE(job.id, 0)
			ELSE task.open_job_id
		END
		WHERE task.open_job_id IS NULL OR task.open_job_id = 0
	`, quoteTableName(tableName),
		quoteTableName(jobsTable),
		models.SmartMoneyFollowJobActionOpen,
	)).Error; err != nil {
		return fmt.Errorf("backfill %s.open_job_id: %w", tableName, err)
	}
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s AS task
		LEFT JOIN %s AS job
			ON job.config_id = task.config_id
			AND job.event_id = task.close_event_id
			AND job.action = '%s'
		SET task.close_job_id = CASE
			WHEN task.close_event_id IS NULL OR task.close_event_id = 0 THEN NULL
			WHEN task.close_job_id IS NULL OR task.close_job_id = 0 THEN COALESCE(job.id, 0)
			ELSE task.close_job_id
		END
		WHERE task.close_event_id IS NULL
		   OR task.close_event_id = 0
		   OR task.close_job_id IS NULL
		   OR task.close_job_id = 0
	`, quoteTableName(tableName),
		quoteTableName(jobsTable),
		models.SmartMoneyFollowJobActionClose,
	)).Error; err != nil {
		return fmt.Errorf("backfill %s.close_job_id: %w", tableName, err)
	}
	return nil
}

func normalizeSmartMoneyFollowTaskStatuses(tableName string) error {
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s
		SET status = CASE LOWER(TRIM(status))
			WHEN 'active' THEN '%s'
			WHEN 'open' THEN '%s'
			WHEN 'closing' THEN '%s'
			WHEN 'closed' THEN '%s'
			ELSE '%s'
		END
	`, quoteTableName(tableName),
		models.SmartMoneyFollowTaskStatusOpen,
		models.SmartMoneyFollowTaskStatusOpen,
		models.SmartMoneyFollowTaskStatusOpen,
		models.SmartMoneyFollowTaskStatusClosed,
		models.SmartMoneyFollowTaskStatusClosed,
	)).Error; err != nil {
		return fmt.Errorf("normalize %s.status: %w", tableName, err)
	}
	return nil
}

func repairSmartMoneyFollowTaskUniqueKeys(tableName string) error {
	if err := DB.Exec(fmt.Sprintf(`
		UPDATE %s AS task
		INNER JOIN %s AS keeper
			ON keeper.open_event_id = task.open_event_id
			AND keeper.id < task.id
		SET task.open_event_id = 1000000000000000000 + task.id
	`, quoteTableName(tableName), quoteTableName(tableName))).Error; err != nil {
		return fmt.Errorf("repair duplicate %s open_event_id keys: %w", tableName, err)
	}
	return nil
}

func ensureUniqueIndex(table, indexName, columns string) error {
	if DB == nil {
		return nil
	}
	var existing int64
	if err := DB.Raw(`
		SELECT COUNT(*)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = ?
		  AND INDEX_NAME = ?
	`, table, indexName).Scan(&existing).Error; err != nil {
		return fmt.Errorf("inspect index %s.%s: %w", table, indexName, err)
	}
	var count int64
	if err := DB.Raw(`
		SELECT COUNT(*)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = ?
		  AND INDEX_NAME = ?
		  AND NON_UNIQUE = 0
	`, table, indexName).Scan(&count).Error; err != nil {
		return fmt.Errorf("inspect unique index %s.%s: %w", table, indexName, err)
	}
	if count > 0 {
		return nil
	}
	if existing > 0 {
		if err := DB.Exec(fmt.Sprintf("ALTER TABLE %s DROP INDEX `%s`", quoteTableName(table), indexName)).Error; err != nil {
			return fmt.Errorf("drop non-unique index %s.%s: %w", table, indexName, err)
		}
	}
	if err := DB.Exec(fmt.Sprintf("ALTER TABLE %s ADD UNIQUE INDEX `%s` (%s)", quoteTableName(table), indexName, columns)).Error; err != nil {
		return fmt.Errorf("add unique index %s.%s: %w", table, indexName, err)
	}
	log.Printf("[DB] added unique index %s.%s", table, indexName)
	return nil
}

func quoteTableName(tableName string) string {
	return "`" + strings.ReplaceAll(tableName, "`", "``") + "`"
}

func quoteColumnName(columnName string) string {
	return "`" + strings.ReplaceAll(columnName, "`", "``") + "`"
}

func tableExists(tableName string) (bool, error) {
	var count int64
	err := DB.Raw(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name = ?
	`, tableName).Scan(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
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
