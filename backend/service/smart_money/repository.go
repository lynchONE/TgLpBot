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

// --- Scan State ---

func (r *Repository) UpsertLPScanState(ctx context.Context, chainID int, blockNum uint64) error {
	state := &models.SmartMoneyScanState{
		ChainID:          chainID,
		LastScannedBlock: blockNum,
	}
	return database.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "chain_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{"last_scanned_block": blockNum}),
		}).
		Create(state).Error
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

func (r *Repository) GetWatchContractByAddress(ctx context.Context, address string, chainID int) (*models.WatchContract, error) {
	var contract models.WatchContract
	err := database.DB.WithContext(ctx).
		Where("contract_address = ? AND chain_id = ?", strings.ToLower(address), chainID).
		First(&contract).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &contract, err
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
	recentCutoff := time.Now().Add(-2 * time.Hour)
	if status != "" && status != "all" {
		db = db.Where("status = ?", status)
		if status == "open" {
			db = db.Where("opened_at >= ?", recentCutoff)
		}
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

func (r *Repository) ListAllPositions(ctx context.Context, status, wallet, pool, protocol string, orderBy string) ([]models.SmartMoneyLPPosition, error) {
	db := database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{})
	recentCutoff := time.Now().Add(-2 * time.Hour)
	if status != "" && status != "all" {
		db = db.Where("status = ?", status)
		if status == "open" {
			db = db.Where("opened_at >= ?", recentCutoff)
		}
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

	switch orderBy {
	case "opened_at_asc":
		db = db.Order("opened_at ASC")
	default:
		db = db.Order("opened_at DESC")
	}

	var positions []models.SmartMoneyLPPosition
	err := db.Find(&positions).Error
	return positions, err
}

func (r *Repository) UpdateLPPositionMetadata(ctx context.Context, id uint, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	return database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func (r *Repository) ListPositionsNeedingMetadataRepair(ctx context.Context, poolIdentifiers []string) ([]models.SmartMoneyLPPosition, error) {
	db := database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{})

	conditions := []string{
		"COALESCE(token0_symbol, '') = ''",
		"COALESCE(token1_symbol, '') = ''",
		"fee_tier IS NULL",
		"tick_lower IS NULL",
		"tick_upper IS NULL",
	}
	args := make([]interface{}, 0, 1)
	if len(poolIdentifiers) > 0 {
		conditions = append(conditions, "LOWER(pool_address) IN ?")
		args = append(args, poolIdentifiers)
	}

	var positions []models.SmartMoneyLPPosition
	err := db.
		Where(strings.Join(conditions, " OR "), args...).
		Order("opened_at DESC").
		Find(&positions).Error
	return positions, err
}

func (r *Repository) GetPositionByID(ctx context.Context, id uint) (*models.SmartMoneyLPPosition, error) {
	var p models.SmartMoneyLPPosition
	err := database.DB.WithContext(ctx).First(&p, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &p, err
}

type PositionOpenAmountRow struct {
	ChainID           int     `json:"chain_id"`
	NftTokenID        uint64  `json:"nft_token_id"`
	PositionAmountUSD float64 `json:"position_amount_usd"`
}

func (r *Repository) GetPositionOpenAmountsUSD(ctx context.Context, positions []models.SmartMoneyLPPosition) (map[int]map[uint64]float64, error) {
	out := make(map[int]map[uint64]float64)
	if len(positions) == 0 {
		return out, nil
	}

	chainSeen := make(map[int]struct{}, len(positions))
	nftSeen := make(map[uint64]struct{}, len(positions))
	txSeen := make(map[string]struct{}, len(positions))

	chainIDs := make([]int, 0, len(positions))
	nftIDs := make([]uint64, 0, len(positions))
	txHashes := make([]string, 0, len(positions))

	for _, pos := range positions {
		txHash := strings.ToLower(strings.TrimSpace(pos.OpenTxHash))
		if pos.NftTokenID == 0 || txHash == "" {
			continue
		}
		if _, ok := chainSeen[pos.ChainID]; !ok {
			chainSeen[pos.ChainID] = struct{}{}
			chainIDs = append(chainIDs, pos.ChainID)
		}
		if _, ok := nftSeen[pos.NftTokenID]; !ok {
			nftSeen[pos.NftTokenID] = struct{}{}
			nftIDs = append(nftIDs, pos.NftTokenID)
		}
		if _, ok := txSeen[txHash]; !ok {
			txSeen[txHash] = struct{}{}
			txHashes = append(txHashes, txHash)
		}
	}

	if len(chainIDs) == 0 || len(nftIDs) == 0 || len(txHashes) == 0 {
		return out, nil
	}

	var rows []PositionOpenAmountRow
	err := database.DB.WithContext(ctx).
		Table("sm_lp_events").
		Select(`
			chain_id,
			nft_token_id,
			MAX(COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)) AS position_amount_usd
		`).
		Where("event_type = ? AND chain_id IN ? AND nft_token_id IN ? AND LOWER(tx_hash) IN ?", "add", chainIDs, nftIDs, txHashes).
		Group("chain_id, nft_token_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		if _, ok := out[row.ChainID]; !ok {
			out[row.ChainID] = make(map[uint64]float64)
		}
		out[row.ChainID][row.NftTokenID] = row.PositionAmountUSD
	}

	return out, nil
}

// --- Pool-level total amounts ---

type PoolAmountRow struct {
	PoolAddress    string  `json:"pool_address"`
	TotalAmountUSD float64 `json:"total_amount_usd"`
}

func (r *Repository) GetPoolTotalAmountsUSD(ctx context.Context) (map[string]float64, error) {
	recentCutoff := time.Now().Add(-2 * time.Hour)
	var rows []PoolAmountRow
	err := database.DB.WithContext(ctx).Raw(`
		SELECT
			p.pool_address,
			COALESCE(SUM(e_agg.position_amount_usd), 0) AS total_amount_usd
		FROM sm_lp_positions p
		LEFT JOIN (
			SELECT chain_id, nft_token_id,
				MAX(COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)) AS position_amount_usd
			FROM sm_lp_events
			WHERE event_type = 'add'
			GROUP BY chain_id, nft_token_id
		) e_agg ON e_agg.chain_id = p.chain_id AND e_agg.nft_token_id = p.nft_token_id
		WHERE p.status = 'open' AND p.opened_at >= ?
		GROUP BY p.pool_address
	`, recentCutoff).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(rows))
	for _, row := range rows {
		out[strings.ToLower(row.PoolAddress)] = row.TotalAmountUSD
	}
	return out, nil
}

// --- Aggregate queries ---

type PoolAggRow struct {
	PoolAddress            string    `json:"pool_address"`
	Token0Symbol           string    `json:"token0_symbol"`
	Token1Symbol           string    `json:"token1_symbol"`
	Token0Address          string    `json:"token0_address"`
	Token1Address          string    `json:"token1_address"`
	FeeTier                *int      `json:"fee_tier"`
	Protocol               string    `json:"protocol"`
	ChainID                int       `json:"chain_id"`
	OpenPositionCount      int       `json:"open_position_count"`
	WalletCount            int       `json:"wallet_count"`
	LatestEventAt          time.Time `json:"latest_event_at"`
	TradingPair            string    `json:"trading_pair"`
	DisplayTokenAddress    string    `json:"display_token_address,omitempty"`
	DisplayTokenSymbol     string    `json:"display_token_symbol,omitempty"`
	DisplayTokenLogoURL    string    `json:"display_token_logo_url,omitempty"`
	TotalPositionAmountUSD float64   `json:"total_position_amount_usd"`
}

func (r *Repository) ListPoolsWithPositions(ctx context.Context) ([]PoolAggRow, error) {
	var rows []PoolAggRow
	cutoff := time.Now().Add(-2 * time.Hour)
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
			SUM(CASE WHEN p.status='open' AND p.opened_at >= ? THEN 1 ELSE 0 END) AS open_position_count,
			COUNT(DISTINCT CASE WHEN p.status='open' AND p.opened_at >= ? THEN p.wallet_address END) AS wallet_count,
			MAX(CASE WHEN p.status='open' AND p.opened_at >= ? THEN p.opened_at END) AS latest_event_at
		FROM sm_lp_positions p
		GROUP BY p.pool_address, p.token0_symbol, p.token1_symbol, p.token0_address, p.token1_address, p.fee_tier, p.protocol, p.chain_id
		HAVING open_position_count > 0
		ORDER BY latest_event_at DESC
	`, cutoff, cutoff, cutoff).Scan(&rows).Error
	return rows, err
}

type PoolStats struct {
	PoolAddress            string  `json:"pool_address"`
	Token0Symbol           string  `json:"token0_symbol"`
	Token1Symbol           string  `json:"token1_symbol"`
	Token0Address          string  `json:"token0_address"`
	Token1Address          string  `json:"token1_address"`
	FeeTier                *int    `json:"fee_tier"`
	Protocol               string  `json:"protocol"`
	ChainID                int     `json:"chain_id"`
	OpenPositionCount      int     `json:"open_position_count"`
	WalletCount            int     `json:"wallet_count"`
	ClosedTodayCount       int     `json:"closed_today_count"`
	TotalPositionAmountUSD float64 `json:"total_position_amount_usd"`
	TradingPair            string  `json:"trading_pair"`
	CurrentPrice           string  `json:"current_price"`
	PriceChange24h         float64 `json:"price_change_24h"`
	DisplayTokenAddress    string  `json:"display_token_address,omitempty"`
	DisplayTokenSymbol     string  `json:"display_token_symbol,omitempty"`
	DisplayTokenLogoURL    string  `json:"display_token_logo_url,omitempty"`
}

func (r *Repository) GetPoolStats(ctx context.Context, poolAddress string) (*PoolStats, error) {
	poolAddress = strings.ToLower(poolAddress)
	var stats PoolStats
	today := time.Now().Truncate(24 * time.Hour)
	recentCutoff := time.Now().Add(-2 * time.Hour)
	err := database.DB.WithContext(ctx).Raw(`
		SELECT
			pool_address,
			MAX(token0_symbol) AS token0_symbol,
			MAX(token1_symbol) AS token1_symbol,
			MAX(token0_address) AS token0_address,
			MAX(token1_address) AS token1_address,
			MAX(fee_tier) AS fee_tier,
			MAX(protocol) AS protocol,
			MAX(chain_id) AS chain_id,
			SUM(CASE WHEN status='open' AND opened_at >= ? THEN 1 ELSE 0 END) AS open_position_count,
			COUNT(DISTINCT CASE WHEN status='open' AND opened_at >= ? THEN wallet_address END) AS wallet_count,
			SUM(CASE WHEN status='closed' AND closed_at >= ? THEN 1 ELSE 0 END) AS closed_today_count,
			COALESCE(SUM(CASE WHEN status='open' AND opened_at >= ? THEN COALESCE(e_agg.position_amount_usd, 0) ELSE 0 END), 0) AS total_position_amount_usd
		FROM sm_lp_positions p
		LEFT JOIN (
			SELECT chain_id, nft_token_id,
				MAX(COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)) AS position_amount_usd
			FROM sm_lp_events
			WHERE event_type = 'add'
			GROUP BY chain_id, nft_token_id
		) e_agg ON e_agg.chain_id = p.chain_id AND e_agg.nft_token_id = p.nft_token_id
		WHERE p.pool_address = ?
		GROUP BY pool_address
	`, recentCutoff, recentCutoff, today, recentCutoff, poolAddress).Scan(&stats).Error
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
	recentCutoff := time.Now().Add(-2 * time.Hour)

	database.DB.WithContext(ctx).Model(&models.MonitoredWallet{}).
		Where("is_active = 1").Count(new(int64))

	var walletCount int64
	database.DB.WithContext(ctx).Model(&models.MonitoredWallet{}).
		Where("is_active = 1").Count(&walletCount)
	stats.MonitoredWalletCount = int(walletCount)

	var openCount int64
	database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{}).
		Where("status = 'open' AND opened_at >= ?", recentCutoff).Count(&openCount)
	stats.OpenPositionCount = int(openCount)

	var closedToday int64
	database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{}).
		Where("status = 'closed' AND closed_at >= ?", today).Count(&closedToday)
	stats.ClosedTodayCount = int(closedToday)

	var poolCount int64
	database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{}).
		Where("status = 'open' AND opened_at >= ?", recentCutoff).
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
	recentCutoff := time.Now().Add(-2 * time.Hour)

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
			Where("wallet_address = ? AND status = 'open' AND opened_at >= ?", addr, recentCutoff).Count(&openCount)
		row.OpenPositionCount = int(openCount)

		var poolCount int64
		database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{}).
			Where("wallet_address = ? AND status = 'open' AND opened_at >= ?", addr, recentCutoff).
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
