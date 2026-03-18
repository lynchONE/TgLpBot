package smart_money

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{}

func NewRepository() *Repository {
	return &Repository{}
}

// --- MonitoredWallet ---

func (r *Repository) ListMonitoredWallets(ctx context.Context, page, size int, keyword string, source string, activeOnly *bool) ([]models.MonitoredWallet, int64, error) {
	db := database.DB.WithContext(ctx).Model(&models.MonitoredWallet{})
	if keyword != "" {
		kw := "%" + strings.ToLower(keyword) + "%"
		db = db.Where("address LIKE ? OR label LIKE ?", kw, kw)
	}
	if source != "" {
		db = db.Where("source = ?", source)
	}
	if activeOnly != nil {
		db = db.Where("is_active = ?", *activeOnly)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var wallets []models.MonitoredWallet
	err := db.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&wallets).Error
	return wallets, total, err
}

func (r *Repository) GetMonitoredWalletByAddress(ctx context.Context, address string, chainID int) (*models.MonitoredWallet, error) {
	var w models.MonitoredWallet
	err := database.DB.WithContext(ctx).
		Where("address = ? AND chain_id = ?", strings.ToLower(address), chainID).
		First(&w).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &w, err
}

func (r *Repository) IsMonitoredWallet(ctx context.Context, address string, chainID int) bool {
	var count int64
	database.DB.WithContext(ctx).Model(&models.MonitoredWallet{}).
		Where("address = ? AND chain_id = ? AND is_active = 1", strings.ToLower(address), chainID).
		Count(&count)
	return count > 0
}

func (r *Repository) GetAllActiveWalletAddresses(ctx context.Context, chainID int) (map[string]struct{}, error) {
	var addrs []string
	err := database.DB.WithContext(ctx).Model(&models.MonitoredWallet{}).
		Where("chain_id = ? AND is_active = 1", chainID).
		Pluck("address", &addrs).Error
	if err != nil {
		return nil, err
	}
	m := make(map[string]struct{}, len(addrs))
	for _, a := range addrs {
		m[strings.ToLower(a)] = struct{}{}
	}
	return m, nil
}

func (r *Repository) UpsertMonitoredWallet(ctx context.Context, w *models.MonitoredWallet) error {
	w.Address = strings.ToLower(w.Address)
	return database.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "address"}, {Name: "chain_id"}},
			DoNothing: true,
		}).
		Create(w).Error
}

func (r *Repository) CreateMonitoredWallet(ctx context.Context, w *models.MonitoredWallet) error {
	w.Address = strings.ToLower(w.Address)
	return database.DB.WithContext(ctx).Create(w).Error
}

func (r *Repository) UpdateMonitoredWallet(ctx context.Context, address string, chainID int, updates map[string]interface{}) error {
	return database.DB.WithContext(ctx).Model(&models.MonitoredWallet{}).
		Where("address = ? AND chain_id = ?", strings.ToLower(address), chainID).
		Updates(updates).Error
}

func (r *Repository) SoftDeleteMonitoredWallet(ctx context.Context, address string, chainID int) error {
	return database.DB.WithContext(ctx).Model(&models.MonitoredWallet{}).
		Where("address = ? AND chain_id = ?", strings.ToLower(address), chainID).
		Update("is_active", false).Error
}

// --- WatchContract ---

func (r *Repository) ListWatchContracts(ctx context.Context) ([]models.WatchContract, error) {
	var contracts []models.WatchContract
	err := database.DB.WithContext(ctx).Order("id DESC").Find(&contracts).Error
	return contracts, err
}

func (r *Repository) GetActiveWatchContracts(ctx context.Context) ([]models.WatchContract, error) {
	var contracts []models.WatchContract
	err := database.DB.WithContext(ctx).Where("is_active = 1").Find(&contracts).Error
	return contracts, err
}

func (r *Repository) GetActiveWatchContractsByChain(ctx context.Context, chainID int) ([]models.WatchContract, error) {
	var contracts []models.WatchContract
	err := database.DB.WithContext(ctx).
		Where("is_active = 1 AND chain_id = ?", chainID).
		Find(&contracts).Error
	return contracts, err
}

func (r *Repository) CreateWatchContract(ctx context.Context, c *models.WatchContract) error {
	c.ContractAddress = strings.ToLower(c.ContractAddress)
	return database.DB.WithContext(ctx).Create(c).Error
}

func (r *Repository) UpdateWatchContract(ctx context.Context, address string, chainID int, updates map[string]interface{}) error {
	return database.DB.WithContext(ctx).Model(&models.WatchContract{}).
		Where("contract_address = ? AND chain_id = ?", strings.ToLower(address), chainID).
		Updates(updates).Error
}

func (r *Repository) DeleteWatchContract(ctx context.Context, address string, chainID int) error {
	return database.DB.WithContext(ctx).
		Where("contract_address = ? AND chain_id = ?", strings.ToLower(address), chainID).
		Delete(&models.WatchContract{}).Error
}

func (r *Repository) UpdateWatchContractLastBlock(ctx context.Context, id uint, blockNum uint64) error {
	return database.DB.WithContext(ctx).Model(&models.WatchContract{}).
		Where("id = ?", id).
		Update("last_scanned_block", blockNum).Error
}

// --- SmartMoneyLPEvent ---

func (r *Repository) InsertLPEvent(tx *gorm.DB, event *models.SmartMoneyLPEvent) error {
	event.WalletAddress = strings.ToLower(event.WalletAddress)
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(event).Error
}

func (r *Repository) ListLPEvents(ctx context.Context, wallet, pool string, page, size int) ([]models.SmartMoneyLPEvent, int64, error) {
	db := database.DB.WithContext(ctx).Model(&models.SmartMoneyLPEvent{})
	if wallet != "" {
		db = db.Where("wallet_address = ?", strings.ToLower(wallet))
	}
	if pool != "" {
		db = db.Where("pool_address = ?", strings.ToLower(pool))
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var events []models.SmartMoneyLPEvent
	err := db.Order("tx_timestamp DESC").Offset((page - 1) * size).Limit(size).Find(&events).Error
	return events, total, err
}

// --- SmartMoneyLPPosition ---

func (r *Repository) UpsertLPPosition(tx *gorm.DB, event *models.SmartMoneyLPEvent) error {
	if event.EventType == "add" {
		if event.NftTokenID == nil {
			return nil
		}
		var count int64
		tx.Model(&models.SmartMoneyLPPosition{}).
			Where("nft_token_id = ? AND chain_id = ?", *event.NftTokenID, event.ChainID).
			Count(&count)
		if count > 0 {
			return nil // already exists, don't duplicate
		}
		pos := models.SmartMoneyLPPosition{
			WalletAddress: strings.ToLower(event.WalletAddress),
			ChainID:       event.ChainID,
			Protocol:      event.Protocol,
			NftTokenID:    *event.NftTokenID,
			PoolAddress:   strings.ToLower(event.PoolAddress),
			Token0Address: strings.ToLower(event.Token0Address),
			Token1Address: strings.ToLower(event.Token1Address),
			Token0Symbol:  event.Token0Symbol,
			Token1Symbol:  event.Token1Symbol,
			FeeTier:       event.FeeTier,
			TickLower:     event.TickLower,
			TickUpper:     event.TickUpper,
			Status:        "open",
			OpenTxHash:    event.TxHash,
			OpenedAt:      event.TxTimestamp,
		}
		return tx.Create(&pos).Error
	}

	// remove event: close the position
	if event.NftTokenID == nil {
		return nil
	}
	now := event.TxTimestamp
	return tx.Model(&models.SmartMoneyLPPosition{}).
		Where("nft_token_id = ? AND chain_id = ?", *event.NftTokenID, event.ChainID).
		Updates(map[string]interface{}{
			"status":        "closed",
			"close_tx_hash": event.TxHash,
			"closed_at":     &now,
		}).Error
}

func (r *Repository) WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return database.DB.WithContext(ctx).Transaction(fn)
}

func (r *Repository) ListPositions(ctx context.Context, status, wallet, pool, protocol string, page, size int, orderBy string) ([]models.SmartMoneyLPPosition, int64, error) {
	db := database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{})
	if status != "" && status != "all" {
		db = db.Where("status = ?", status)
	}
	if wallet != "" {
		db = db.Where("wallet_address = ?", strings.ToLower(wallet))
	}
	if pool != "" {
		db = db.Where("pool_address = ?", strings.ToLower(pool))
	}
	if protocol != "" {
		db = db.Where("protocol = ?", protocol)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	switch orderBy {
	case "opened_at_asc":
		db = db.Order("opened_at ASC")
	default:
		db = db.Order("opened_at DESC")
	}

	var positions []models.SmartMoneyLPPosition
	err := db.Offset((page - 1) * size).Limit(size).Find(&positions).Error
	return positions, total, err
}

func (r *Repository) GetPositionByID(ctx context.Context, id uint) (*models.SmartMoneyLPPosition, error) {
	var p models.SmartMoneyLPPosition
	err := database.DB.WithContext(ctx).First(&p, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &p, err
}

// --- Aggregate queries ---

type PoolAggRow struct {
	PoolAddress       string    `json:"pool_address"`
	Token0Symbol      string    `json:"token0_symbol"`
	Token1Symbol      string    `json:"token1_symbol"`
	Token0Address     string    `json:"token0_address"`
	Token1Address     string    `json:"token1_address"`
	FeeTier           *int      `json:"fee_tier"`
	Protocol          string    `json:"protocol"`
	ChainID           int       `json:"chain_id"`
	OpenPositionCount int       `json:"open_position_count"`
	WalletCount       int       `json:"wallet_count"`
	LatestEventAt     time.Time `json:"latest_event_at"`
}

func (r *Repository) ListPoolsWithPositions(ctx context.Context) ([]PoolAggRow, error) {
	var rows []PoolAggRow
	err := database.DB.WithContext(ctx).Raw(`
		SELECT
			p.pool_address,
			p.token0_symbol,
			p.token1_symbol,
			p.token0_address,
			p.token1_address,
			p.fee_tier,
			p.protocol,
			p.chain_id,
			SUM(CASE WHEN p.status='open' THEN 1 ELSE 0 END) AS open_position_count,
			COUNT(DISTINCT CASE WHEN p.status='open' THEN p.wallet_address END) AS wallet_count,
			MAX(p.opened_at) AS latest_event_at
		FROM sm_lp_positions p
		GROUP BY p.pool_address, p.token0_symbol, p.token1_symbol, p.token0_address, p.token1_address, p.fee_tier, p.protocol, p.chain_id
		HAVING open_position_count > 0
		ORDER BY latest_event_at DESC
	`).Scan(&rows).Error
	return rows, err
}

type PoolStats struct {
	PoolAddress       string `json:"pool_address"`
	Token0Symbol      string `json:"token0_symbol"`
	Token1Symbol      string `json:"token1_symbol"`
	FeeTier           *int   `json:"fee_tier"`
	Protocol          string `json:"protocol"`
	OpenPositionCount int    `json:"open_position_count"`
	WalletCount       int    `json:"wallet_count"`
	ClosedTodayCount  int    `json:"closed_today_count"`
}

func (r *Repository) GetPoolStats(ctx context.Context, poolAddress string) (*PoolStats, error) {
	poolAddress = strings.ToLower(poolAddress)
	var stats PoolStats
	today := time.Now().Truncate(24 * time.Hour)
	err := database.DB.WithContext(ctx).Raw(`
		SELECT
			pool_address,
			MAX(token0_symbol) AS token0_symbol,
			MAX(token1_symbol) AS token1_symbol,
			MAX(fee_tier) AS fee_tier,
			MAX(protocol) AS protocol,
			SUM(CASE WHEN status='open' THEN 1 ELSE 0 END) AS open_position_count,
			COUNT(DISTINCT CASE WHEN status='open' THEN wallet_address END) AS wallet_count,
			SUM(CASE WHEN status='closed' AND closed_at >= ? THEN 1 ELSE 0 END) AS closed_today_count
		FROM sm_lp_positions
		WHERE pool_address = ?
		GROUP BY pool_address
	`, today, poolAddress).Scan(&stats).Error
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

type GlobalStats struct {
	ActivePoolCount      int  `json:"active_pool_count"`
	ActiveContractCount  int  `json:"active_contract_count"`
	MonitoredWalletCount int  `json:"monitored_wallet_count"`
	OpenPositionCount    int  `json:"open_position_count"`
	ClosedTodayCount     int  `json:"closed_today_count"`
	MonitorEnabled       bool `json:"monitor_enabled"`
	WatcherEnabled       bool `json:"watcher_enabled"`
	CrawlerEnabled       bool `json:"crawler_enabled"`
}

func (r *Repository) GetGlobalStats(ctx context.Context) (*GlobalStats, error) {
	var stats GlobalStats
	today := time.Now().Truncate(24 * time.Hour)

	database.DB.WithContext(ctx).Model(&models.MonitoredWallet{}).
		Where("is_active = 1").Count(new(int64))

	var walletCount int64
	database.DB.WithContext(ctx).Model(&models.MonitoredWallet{}).
		Where("is_active = 1").Count(&walletCount)
	stats.MonitoredWalletCount = int(walletCount)

	var openCount int64
	database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{}).
		Where("status = 'open'").Count(&openCount)
	stats.OpenPositionCount = int(openCount)

	var closedToday int64
	database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{}).
		Where("status = 'closed' AND closed_at >= ?", today).Count(&closedToday)
	stats.ClosedTodayCount = int(closedToday)

	var poolCount int64
	database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{}).
		Where("status = 'open'").
		Distinct("pool_address").Count(&poolCount)
	stats.ActivePoolCount = int(poolCount)

	var contractCount int64
	database.DB.WithContext(ctx).Model(&models.WatchContract{}).
		Where("is_active = 1").Count(&contractCount)
	stats.ActiveContractCount = int(contractCount)

	return &stats, nil
}

type WalletStatsRow struct {
	Address           string     `json:"address"`
	Label             *string    `json:"label"`
	Source            string     `json:"source"`
	SourceContract    *string    `json:"source_contract"`
	IsActive          bool       `json:"is_active"`
	ChainID           int        `json:"chain_id"`
	OpenPositionCount int        `json:"open_position_count"`
	ActivePoolCount   int        `json:"active_pool_count"`
	TotalAddCount     int        `json:"total_add_count"`
	TotalRemoveCount  int        `json:"total_remove_count"`
	LastActiveAt      *time.Time `json:"last_active_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

func (r *Repository) ListWalletsWithStats(ctx context.Context, page, size int, keyword, source string, activeOnly *bool) ([]WalletStatsRow, int64, error) {
	wallets, total, err := r.ListMonitoredWallets(ctx, page, size, keyword, source, activeOnly)
	if err != nil {
		return nil, 0, err
	}

	rows := make([]WalletStatsRow, 0, len(wallets))
	for _, w := range wallets {
		row := WalletStatsRow{
			Address:        w.Address,
			Label:          w.Label,
			Source:         w.Source,
			SourceContract: w.SourceContract,
			IsActive:       w.IsActive,
			ChainID:        w.ChainID,
			CreatedAt:      w.CreatedAt,
		}

		addr := strings.ToLower(w.Address)

		var openCount int64
		database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{}).
			Where("wallet_address = ? AND status = 'open'", addr).Count(&openCount)
		row.OpenPositionCount = int(openCount)

		var poolCount int64
		database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{}).
			Where("wallet_address = ? AND status = 'open'", addr).
			Distinct("pool_address").Count(&poolCount)
		row.ActivePoolCount = int(poolCount)

		var addCount int64
		database.DB.WithContext(ctx).Model(&models.SmartMoneyLPEvent{}).
			Where("wallet_address = ? AND event_type = 'add'", addr).Count(&addCount)
		row.TotalAddCount = int(addCount)

		var removeCount int64
		database.DB.WithContext(ctx).Model(&models.SmartMoneyLPEvent{}).
			Where("wallet_address = ? AND event_type = 'remove'", addr).Count(&removeCount)
		row.TotalRemoveCount = int(removeCount)

		var lastEvent models.SmartMoneyLPEvent
		if err := database.DB.WithContext(ctx).Model(&models.SmartMoneyLPEvent{}).
			Where("wallet_address = ?", addr).
			Order("tx_timestamp DESC").
			First(&lastEvent).Error; err == nil {
			row.LastActiveAt = &lastEvent.TxTimestamp
		}

		rows = append(rows, row)
	}
	return rows, total, nil
}
