package smart_money

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{}

const smartMoneyNetAmountOrderJoin = `
LEFT JOIN sm_lp_active_positions ap
	ON ap.chain_id = sm_lp_positions.chain_id AND ap.nft_token_id = sm_lp_positions.nft_token_id
LEFT JOIN (
	SELECT
		chain_id,
		nft_token_id,
		SUM(
			CASE
				WHEN event_type = 'add' THEN COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
				WHEN event_type = 'remove' THEN -COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
				ELSE 0
			END
		) AS net_amount_usd
	FROM sm_lp_events
	WHERE event_type IN ('add', 'remove')
	GROUP BY chain_id, nft_token_id
) evt_net
	ON evt_net.chain_id = sm_lp_positions.chain_id
	AND evt_net.nft_token_id = sm_lp_positions.nft_token_id
`

const smartMoneyNetAmountOrderExpr = "COALESCE(ap.net_total_usd, evt_net.net_amount_usd, 0)"
const smartMoneyPositionTable = "sm_lp_positions"

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

func (r *Repository) DeleteMonitoredWallet(ctx context.Context, address string, chainID int) error {
	return database.DB.WithContext(ctx).
		Where("address = ? AND chain_id = ?", strings.ToLower(address), chainID).
		Delete(&models.MonitoredWallet{}).Error
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
		db = db.Where("LOWER(wallet_address) = ?", strings.ToLower(wallet))
	}
	if pool != "" {
		db = db.Where("LOWER(pool_address) = ?", strings.ToLower(pool))
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
	active, err := r.UpsertActivePosition(tx, event)
	if err != nil || active == nil {
		return err
	}

	if event.NftTokenID == nil || *event.NftTokenID == 0 {
		return nil
	}

	var pos models.SmartMoneyLPPosition
	err = tx.Where("nft_token_id = ? AND chain_id = ?", *event.NftTokenID, event.ChainID).First(&pos).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	exists := err == nil
	previousStatus := strings.TrimSpace(pos.Status)
	if !exists {
		pos = models.SmartMoneyLPPosition{
			WalletAddress: strings.ToLower(event.WalletAddress),
			ChainID:       event.ChainID,
			Protocol:      event.Protocol,
			NftTokenID:    *event.NftTokenID,
			PoolAddress:   strings.ToLower(event.PoolAddress),
			Token0Address: strings.ToLower(event.Token0Address),
			Token1Address: strings.ToLower(event.Token1Address),
			Token0Symbol:  strings.TrimSpace(event.Token0Symbol),
			Token1Symbol:  strings.TrimSpace(event.Token1Symbol),
			FeeTier:       cloneIntPtr(event.FeeTier),
			TickLower:     cloneIntPtr(event.TickLower),
			TickUpper:     cloneIntPtr(event.TickUpper),
			Status:        "open",
			OpenTxHash:    strings.TrimSpace(event.TxHash),
			OpenedAt:      active.OpenedAt,
		}
	} else {
		pos.WalletAddress = strings.ToLower(event.WalletAddress)
		pos.ChainID = event.ChainID
		pos.Protocol = strings.TrimSpace(event.Protocol)
		if poolAddress := strings.ToLower(strings.TrimSpace(event.PoolAddress)); poolAddress != "" {
			pos.PoolAddress = poolAddress
		}
		if token0 := strings.ToLower(strings.TrimSpace(event.Token0Address)); token0 != "" {
			pos.Token0Address = token0
		}
		if token1 := strings.ToLower(strings.TrimSpace(event.Token1Address)); token1 != "" {
			pos.Token1Address = token1
		}
		if symbol0 := strings.TrimSpace(event.Token0Symbol); symbol0 != "" {
			pos.Token0Symbol = symbol0
		}
		if symbol1 := strings.TrimSpace(event.Token1Symbol); symbol1 != "" {
			pos.Token1Symbol = symbol1
		}
		if event.FeeTier != nil {
			pos.FeeTier = cloneIntPtr(event.FeeTier)
		}
		if event.TickLower != nil {
			pos.TickLower = cloneIntPtr(event.TickLower)
		}
		if event.TickUpper != nil {
			pos.TickUpper = cloneIntPtr(event.TickUpper)
		}
	}

	if active.IsActive {
		pos.Status = "open"
		pos.CloseTxHash = nil
		pos.ClosedAt = nil
		if !exists || strings.EqualFold(previousStatus, "closed") || strings.TrimSpace(pos.OpenTxHash) == "" || pos.OpenedAt.IsZero() {
			pos.OpenTxHash = strings.TrimSpace(event.TxHash)
			pos.OpenedAt = active.OpenedAt
		}
	} else {
		pos.Status = "closed"
		if txHash := strings.TrimSpace(event.TxHash); txHash != "" {
			pos.CloseTxHash = &txHash
		}
		if active.ClosedAt != nil {
			closedAt := *active.ClosedAt
			pos.ClosedAt = &closedAt
		} else {
			now := event.TxTimestamp
			pos.ClosedAt = &now
		}
		if pos.OpenedAt.IsZero() {
			pos.OpenedAt = active.OpenedAt
		}
		if strings.TrimSpace(pos.OpenTxHash) == "" {
			pos.OpenTxHash = strings.TrimSpace(event.TxHash)
		}
	}

	if exists {
		return tx.Save(&pos).Error
	}
	return tx.Create(&pos).Error
}

func (r *Repository) UpsertActivePosition(tx *gorm.DB, event *models.SmartMoneyLPEvent) (*models.SmartMoneyActivePosition, error) {
	if event == nil {
		return nil, nil
	}

	positionRef := BuildPositionRefFromEvent(event)
	if positionRef == "" {
		return nil, nil
	}

	var active models.SmartMoneyActivePosition
	err := tx.Where("position_ref = ?", positionRef).First(&active).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	exists := err == nil
	if !exists {
		active = models.SmartMoneyActivePosition{
			PositionRef:      positionRef,
			WalletAddress:    strings.ToLower(event.WalletAddress),
			ChainID:          event.ChainID,
			Protocol:         strings.TrimSpace(event.Protocol),
			PoolAddress:      strings.ToLower(strings.TrimSpace(event.PoolAddress)),
			CurrentLiquidity: "0",
			EntryAmount0:     "0",
			EntryAmount1:     "0",
			NetAmount0:       "0",
			NetAmount1:       "0",
			FeeAmount0:       "0",
			FeeAmount1:       "0",
			OpenedAt:         event.TxTimestamp,
		}
	}

	active.PositionRef = positionRef
	active.WalletAddress = strings.ToLower(event.WalletAddress)
	active.ChainID = event.ChainID
	active.Protocol = strings.TrimSpace(event.Protocol)
	if event.NftTokenID != nil {
		active.NftTokenID = *event.NftTokenID
	}
	if poolAddress := strings.ToLower(strings.TrimSpace(event.PoolAddress)); poolAddress != "" {
		active.PoolAddress = poolAddress
	}
	if token0 := strings.ToLower(strings.TrimSpace(event.Token0Address)); token0 != "" {
		active.Token0Address = token0
	}
	if token1 := strings.ToLower(strings.TrimSpace(event.Token1Address)); token1 != "" {
		active.Token1Address = token1
	}
	if symbol0 := strings.TrimSpace(event.Token0Symbol); symbol0 != "" {
		active.Token0Symbol = symbol0
	}
	if symbol1 := strings.TrimSpace(event.Token1Symbol); symbol1 != "" {
		active.Token1Symbol = symbol1
	}
	if event.FeeTier != nil {
		active.FeeTier = cloneIntPtr(event.FeeTier)
	}
	if event.TickLower != nil {
		active.TickLower = cloneIntPtr(event.TickLower)
	}
	if event.TickUpper != nil {
		active.TickUpper = cloneIntPtr(event.TickUpper)
	}

	r.applyPoolSnapshotToActive(tx, &active)
	applyManagerSnapshotToActive(&active)

	liquidityDelta := parseSignedBigInt(event.LiquidityDelta)
	currentLiquidity := parseSignedBigInt(active.CurrentLiquidity)
	if !exists {
		if liveLiquidity := r.loadCurrentLiquiditySnapshot(event, &active); liveLiquidity != nil {
			currentLiquidity = liveLiquidity
		} else {
			currentLiquidity.Add(currentLiquidity, liquidityDelta)
		}
	} else {
		currentLiquidity.Add(currentLiquidity, liquidityDelta)
	}
	if currentLiquidity.Sign() < 0 {
		currentLiquidity = big.NewInt(0)
	}
	active.CurrentLiquidity = currentLiquidity.String()

	reopening := !exists || !active.IsActive || active.OpenedAt.IsZero()
	switch strings.ToLower(strings.TrimSpace(event.EventType)) {
	case "add":
		ts := event.TxTimestamp
		active.LastAddAt = &ts
		if reopening {
			active.OpenedAt = event.TxTimestamp
			active.EntryAmount0 = normalizeUnsignedBigInt(event.Token0Amount)
			active.EntryAmount1 = normalizeUnsignedBigInt(event.Token1Amount)
			active.EntryTotalUSD = cloneStringPtr(event.TotalUSD)
			active.NetAmount0 = normalizeUnsignedBigInt(event.Token0Amount)
			active.NetAmount1 = normalizeUnsignedBigInt(event.Token1Amount)
			active.NetTotalUSD = cloneStringPtr(event.TotalUSD)
		}
		active.ClosedAt = nil
	case "remove":
		ts := event.TxTimestamp
		active.LastRemoveAt = &ts
	}

	if !reopening || strings.ToLower(strings.TrimSpace(event.EventType)) != "add" {
		active.NetAmount0 = addSignedBigIntStrings(active.NetAmount0, event.Token0Amount, eventAmountSign(event))
		active.NetAmount1 = addSignedBigIntStrings(active.NetAmount1, event.Token1Amount, eventAmountSign(event))
		active.NetTotalUSD = addSignedUSDStrings(active.NetTotalUSD, event.TotalUSD, eventAmountSign(event))
	}

	active.IsActive = currentLiquidity.Sign() > 0
	if !active.IsActive {
		ts := event.TxTimestamp
		active.ClosedAt = &ts
	}

	if exists {
		if err := tx.Save(&active).Error; err != nil {
			return nil, err
		}
		return &active, nil
	}
	if err := tx.Create(&active).Error; err != nil {
		return nil, err
	}
	return &active, nil
}

func (r *Repository) WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return database.DB.WithContext(ctx).Transaction(fn)
}

func (r *Repository) ListPositions(ctx context.Context, status, wallet, pool, protocol string, page, size int, orderBy string) ([]models.SmartMoneyLPPosition, int64, error) {
	db := database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{})
	recentCutoff := time.Now().Add(-2 * time.Hour)
	if status != "" && status != "all" {
		db = db.Where(smartMoneyPositionTable+".status = ?", status)
		if status == "open" {
			db = db.Where(smartMoneyPositionTable+".opened_at >= ?", recentCutoff)
		}
	}
	if wallet != "" {
		db = db.Where("LOWER("+smartMoneyPositionTable+".wallet_address) = ?", strings.ToLower(wallet))
	}
	if pool != "" {
		db = db.Where("LOWER("+smartMoneyPositionTable+".pool_address) = ?", strings.ToLower(pool))
	}
	if protocol != "" {
		db = db.Where(smartMoneyPositionTable+".protocol = ?", protocol)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	switch orderBy {
	case "opened_at_asc":
		db = db.Order(smartMoneyPositionTable + ".opened_at ASC")
	case "position_amount_asc", "net_amount_asc":
		db = db.Select(smartMoneyPositionTable + ".*").
			Joins(smartMoneyNetAmountOrderJoin).
			Order(smartMoneyNetAmountOrderExpr + " ASC").
			Order(smartMoneyPositionTable + ".opened_at DESC")
	case "position_amount_desc", "net_amount_desc":
		db = db.Select(smartMoneyPositionTable + ".*").
			Joins(smartMoneyNetAmountOrderJoin).
			Order(smartMoneyNetAmountOrderExpr + " DESC").
			Order(smartMoneyPositionTable + ".opened_at DESC")
	default:
		db = db.Order(smartMoneyPositionTable + ".opened_at DESC")
	}

	var positions []models.SmartMoneyLPPosition
	err := db.Offset((page - 1) * size).Limit(size).Find(&positions).Error
	return positions, total, err
}

func (r *Repository) ListAllPositions(ctx context.Context, status, wallet, pool, protocol string, orderBy string) ([]models.SmartMoneyLPPosition, error) {
	db := database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{})
	recentCutoff := time.Now().Add(-2 * time.Hour)
	if status != "" && status != "all" {
		db = db.Where(smartMoneyPositionTable+".status = ?", status)
		if status == "open" {
			db = db.Where(smartMoneyPositionTable+".opened_at >= ?", recentCutoff)
		}
	}
	if wallet != "" {
		db = db.Where(smartMoneyPositionTable+".wallet_address = ?", strings.ToLower(wallet))
	}
	if pool != "" {
		db = db.Where(smartMoneyPositionTable+".pool_address = ?", strings.ToLower(pool))
	}
	if protocol != "" {
		db = db.Where(smartMoneyPositionTable+".protocol = ?", protocol)
	}

	switch orderBy {
	case "opened_at_asc":
		db = db.Order(smartMoneyPositionTable + ".opened_at ASC")
	case "position_amount_asc", "net_amount_asc":
		db = db.Select(smartMoneyPositionTable + ".*").
			Joins(smartMoneyNetAmountOrderJoin).
			Order(smartMoneyNetAmountOrderExpr + " ASC").
			Order(smartMoneyPositionTable + ".opened_at DESC")
	case "position_amount_desc", "net_amount_desc":
		db = db.Select(smartMoneyPositionTable + ".*").
			Joins(smartMoneyNetAmountOrderJoin).
			Order(smartMoneyNetAmountOrderExpr + " DESC").
			Order(smartMoneyPositionTable + ".opened_at DESC")
	default:
		db = db.Order(smartMoneyPositionTable + ".opened_at DESC")
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

func (r *Repository) GetActivePositionByRef(ctx context.Context, positionRef string) (*models.SmartMoneyActivePosition, error) {
	positionRef = NormalizePositionRef(positionRef)
	if positionRef == "" {
		return nil, nil
	}
	var pos models.SmartMoneyActivePosition
	err := database.DB.WithContext(ctx).
		Where("position_ref = ?", positionRef).
		First(&pos).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &pos, err
}

func (r *Repository) EnsureActivePositionFromPosition(ctx context.Context, pos *models.SmartMoneyLPPosition) (*models.SmartMoneyActivePosition, error) {
	if pos == nil {
		return nil, nil
	}

	positionRef := BuildPositionRefFromPosition(pos)
	if positionRef == "" {
		return nil, nil
	}

	active, err := r.GetActivePositionByRef(ctx, positionRef)
	if err != nil {
		return nil, err
	}
	if active != nil {
		return active, nil
	}

	active = &models.SmartMoneyActivePosition{
		PositionRef:      positionRef,
		WalletAddress:    strings.ToLower(pos.WalletAddress),
		ChainID:          pos.ChainID,
		Protocol:         strings.TrimSpace(pos.Protocol),
		NftTokenID:       pos.NftTokenID,
		PoolAddress:      strings.ToLower(strings.TrimSpace(pos.PoolAddress)),
		Token0Address:    strings.ToLower(strings.TrimSpace(pos.Token0Address)),
		Token1Address:    strings.ToLower(strings.TrimSpace(pos.Token1Address)),
		Token0Symbol:     strings.TrimSpace(pos.Token0Symbol),
		Token1Symbol:     strings.TrimSpace(pos.Token1Symbol),
		FeeTier:          cloneIntPtr(pos.FeeTier),
		TickLower:        cloneIntPtr(pos.TickLower),
		TickUpper:        cloneIntPtr(pos.TickUpper),
		CurrentLiquidity: "0",
		EntryAmount0:     "0",
		EntryAmount1:     "0",
		NetAmount0:       "0",
		NetAmount1:       "0",
		FeeAmount0:       "0",
		FeeAmount1:       "0",
		IsActive:         strings.EqualFold(strings.TrimSpace(pos.Status), "open"),
		OpenedAt:         pos.OpenedAt,
		ClosedAt:         pos.ClosedAt,
	}
	if active.OpenedAt.IsZero() {
		active.OpenedAt = time.Now()
	}
	if err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r.applyPoolSnapshotToActive(tx, active)
		applyManagerSnapshotToActive(active)
		r.hydrateActivePositionFromHistory(tx, active)
		if active.IsActive {
			if liveLiquidity := r.loadCurrentLiquiditySnapshot(nil, active); liveLiquidity != nil {
				active.CurrentLiquidity = liveLiquidity.String()
			}
		}
		return tx.Create(active).Error
	}); err != nil {
		return nil, err
	}
	active, err = r.GetActivePositionByRef(ctx, positionRef)
	if err != nil {
		return nil, err
	}
	return active, nil
}

func (r *Repository) applyPoolSnapshotToActive(tx *gorm.DB, active *models.SmartMoneyActivePosition) {
	if tx == nil || active == nil {
		return
	}
	addr := strings.ToLower(strings.TrimSpace(active.PoolAddress))
	if addr == "" {
		return
	}

	var poolRow models.Pool
	if err := tx.Model(&models.Pool{}).
		Where("LOWER(address) = ?", addr).
		First(&poolRow).Error; err != nil {
		return
	}

	if active.Token0Symbol == "" {
		active.Token0Symbol = strings.TrimSpace(poolRow.Token0Symbol)
	}
	if active.Token1Symbol == "" {
		active.Token1Symbol = strings.TrimSpace(poolRow.Token1Symbol)
	}
	if active.Token0Decimals <= 0 && poolRow.Token0Decimals > 0 {
		active.Token0Decimals = poolRow.Token0Decimals
	}
	if active.Token1Decimals <= 0 && poolRow.Token1Decimals > 0 {
		active.Token1Decimals = poolRow.Token1Decimals
	}
	if active.TickSpacing <= 0 {
		if poolRow.TickSpacing != nil && *poolRow.TickSpacing > 0 {
			active.TickSpacing = *poolRow.TickSpacing
		} else if active.FeeTier != nil {
			active.TickSpacing = tickSpacingFromFeeTier(*active.FeeTier)
		}
	}
}

func (r *Repository) hydrateActivePositionFromHistory(tx *gorm.DB, active *models.SmartMoneyActivePosition) {
	if tx == nil || active == nil || active.NftTokenID == 0 {
		return
	}

	var events []models.SmartMoneyLPEvent
	if err := tx.Model(&models.SmartMoneyLPEvent{}).
		Where("chain_id = ? AND nft_token_id = ?", active.ChainID, active.NftTokenID).
		Order("tx_timestamp ASC").
		Order("id ASC").
		Find(&events).Error; err != nil || len(events) == 0 {
		return
	}

	currentLiquidity := big.NewInt(0)
	seenFirstAdd := false
	active.NetAmount0 = "0"
	active.NetAmount1 = "0"
	active.FeeAmount0 = normalizeUnsignedBigInt(active.FeeAmount0)
	active.FeeAmount1 = normalizeUnsignedBigInt(active.FeeAmount1)
	active.NetTotalUSD = nil

	for _, evt := range events {
		if wallet := strings.ToLower(strings.TrimSpace(evt.WalletAddress)); wallet != "" {
			active.WalletAddress = wallet
		}
		if protocol := strings.TrimSpace(evt.Protocol); protocol != "" {
			active.Protocol = protocol
		}
		if poolAddress := strings.ToLower(strings.TrimSpace(evt.PoolAddress)); poolAddress != "" {
			active.PoolAddress = poolAddress
		}
		if token0 := strings.ToLower(strings.TrimSpace(evt.Token0Address)); token0 != "" {
			active.Token0Address = token0
		}
		if token1 := strings.ToLower(strings.TrimSpace(evt.Token1Address)); token1 != "" {
			active.Token1Address = token1
		}
		if symbol0 := strings.TrimSpace(evt.Token0Symbol); symbol0 != "" {
			active.Token0Symbol = symbol0
		}
		if symbol1 := strings.TrimSpace(evt.Token1Symbol); symbol1 != "" {
			active.Token1Symbol = symbol1
		}
		if evt.FeeTier != nil {
			active.FeeTier = cloneIntPtr(evt.FeeTier)
		}
		if evt.TickLower != nil {
			active.TickLower = cloneIntPtr(evt.TickLower)
		}
		if evt.TickUpper != nil {
			active.TickUpper = cloneIntPtr(evt.TickUpper)
		}

		eventType := strings.ToLower(strings.TrimSpace(evt.EventType))
		if eventType == "add" {
			ts := evt.TxTimestamp
			active.LastAddAt = &ts
			if !seenFirstAdd {
				active.EntryAmount0 = normalizeUnsignedBigInt(evt.Token0Amount)
				active.EntryAmount1 = normalizeUnsignedBigInt(evt.Token1Amount)
				active.EntryTotalUSD = cloneStringPtr(evt.TotalUSD)
				active.OpenedAt = evt.TxTimestamp
				seenFirstAdd = true
			}
		}
		if eventType == "remove" {
			ts := evt.TxTimestamp
			active.LastRemoveAt = &ts
		}

		active.NetAmount0 = addSignedBigIntStrings(active.NetAmount0, evt.Token0Amount, eventAmountSign(&evt))
		active.NetAmount1 = addSignedBigIntStrings(active.NetAmount1, evt.Token1Amount, eventAmountSign(&evt))
		active.NetTotalUSD = addSignedUSDStrings(active.NetTotalUSD, evt.TotalUSD, eventAmountSign(&evt))
		currentLiquidity.Add(currentLiquidity, parseSignedBigInt(evt.LiquidityDelta))
	}

	if currentLiquidity.Sign() > 0 {
		active.CurrentLiquidity = currentLiquidity.String()
	}
}

func applyManagerSnapshotToActive(active *models.SmartMoneyActivePosition) {
	if active == nil || config.AppConfig == nil {
		return
	}

	chain := smartMoneyChainSlugFromID(active.ChainID)
	switch strings.ToLower(strings.TrimSpace(active.Protocol)) {
	case "pancake_v3", "uniswap_v3":
		if strings.TrimSpace(active.PositionManagerAddress) == "" {
			if addr := resolveV3ManagerAddressForProtocol(chain, active.Protocol); addr != "" {
				active.PositionManagerAddress = strings.ToLower(addr)
			}
		}
	case "uniswap_v4":
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
			if strings.TrimSpace(active.PoolManagerAddress) == "" && strings.TrimSpace(cc.UniswapV4PoolManagerAddress) != "" {
				active.PoolManagerAddress = strings.ToLower(strings.TrimSpace(cc.UniswapV4PoolManagerAddress))
			}
			if strings.TrimSpace(active.StateViewAddress) == "" && strings.TrimSpace(cc.UniswapV4StateViewAddress) != "" {
				active.StateViewAddress = strings.ToLower(strings.TrimSpace(cc.UniswapV4StateViewAddress))
			}
			if strings.TrimSpace(active.PositionManagerAddress) == "" && strings.TrimSpace(cc.UniswapV4PositionManagerAddress) != "" {
				active.PositionManagerAddress = strings.ToLower(strings.TrimSpace(cc.UniswapV4PositionManagerAddress))
			}
			return
		}
		if strings.TrimSpace(active.PoolManagerAddress) == "" && strings.TrimSpace(config.AppConfig.UniswapV4PoolManagerAddress) != "" {
			active.PoolManagerAddress = strings.ToLower(strings.TrimSpace(config.AppConfig.UniswapV4PoolManagerAddress))
		}
		if strings.TrimSpace(active.StateViewAddress) == "" && strings.TrimSpace(config.AppConfig.UniswapV4StateViewAddress) != "" {
			active.StateViewAddress = strings.ToLower(strings.TrimSpace(config.AppConfig.UniswapV4StateViewAddress))
		}
		if strings.TrimSpace(active.PositionManagerAddress) == "" && strings.TrimSpace(config.AppConfig.UniswapV4PositionManagerAddress) != "" {
			active.PositionManagerAddress = strings.ToLower(strings.TrimSpace(config.AppConfig.UniswapV4PositionManagerAddress))
		}
	}
}

func (r *Repository) loadCurrentLiquiditySnapshot(event *models.SmartMoneyLPEvent, active *models.SmartMoneyActivePosition) *big.Int {
	if active == nil || active.NftTokenID == 0 {
		return nil
	}

	chainID := active.ChainID
	if chainID == 0 && event != nil {
		chainID = event.ChainID
	}
	chain := smartMoneyChainSlugFromID(chainID)
	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return nil
	}

	tokenID := new(big.Int).SetUint64(active.NftTokenID)
	switch strings.ToLower(strings.TrimSpace(active.Protocol)) {
	case "pancake_v3", "uniswap_v3":
		managerAddress := strings.TrimSpace(active.PositionManagerAddress)
		if !common.IsHexAddress(managerAddress) {
			return nil
		}
		pm, err := blockchain.NewV3PositionManager(common.HexToAddress(managerAddress), client)
		if err != nil {
			return nil
		}
		info, err := pm.Positions(nil, tokenID)
		if err != nil || info == nil || info.Liquidity == nil {
			return nil
		}
		return new(big.Int).Set(info.Liquidity)
	case "uniswap_v4":
		managerAddress := strings.TrimSpace(active.PositionManagerAddress)
		if !common.IsHexAddress(managerAddress) {
			return nil
		}
		pm, err := blockchain.NewV4PositionManager(common.HexToAddress(managerAddress), client)
		if err != nil {
			return nil
		}
		info, err := pm.Positions(nil, tokenID)
		if err == nil && info != nil && info.Liquidity != nil {
			return new(big.Int).Set(info.Liquidity)
		}
		if !common.IsHexAddress(strings.TrimSpace(active.PoolManagerAddress)) || strings.TrimSpace(active.PoolAddress) == "" {
			return nil
		}
		info, err = blockchain.GetV4PositionInfo(
			common.HexToAddress(managerAddress),
			common.HexToAddress(strings.TrimSpace(active.PoolManagerAddress)),
			strings.TrimSpace(active.PoolAddress),
			tokenID,
		)
		if err != nil || info == nil || info.Liquidity == nil {
			return nil
		}
		return new(big.Int).Set(info.Liquidity)
	default:
		return nil
	}
}

func resolveV3ManagerAddressForProtocol(chain string, protocol string) string {
	if config.AppConfig == nil {
		return ""
	}
	if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
		for _, dep := range cc.V3Deployments {
			name := strings.ToLower(strings.TrimSpace(dep.Name))
			switch strings.ToLower(strings.TrimSpace(protocol)) {
			case "pancake_v3":
				if strings.Contains(name, "pancake") && strings.TrimSpace(dep.PositionManagerAddress) != "" {
					return strings.TrimSpace(dep.PositionManagerAddress)
				}
			case "uniswap_v3":
				if strings.Contains(name, "uniswap") && strings.TrimSpace(dep.PositionManagerAddress) != "" {
					return strings.TrimSpace(dep.PositionManagerAddress)
				}
			}
		}
	}
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "pancake_v3":
		return strings.TrimSpace(config.AppConfig.PancakeV3PositionManagerAddress)
	case "uniswap_v3":
		return strings.TrimSpace(config.AppConfig.UniswapV3PositionManagerAddress)
	default:
		return ""
	}
}

func smartMoneyChainSlugFromID(chainID int) string {
	switch chainID {
	case 8453:
		return "base"
	default:
		return "bsc"
	}
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	out := trimmed
	return &out
}

func parseSignedBigInt(raw string) *big.Int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return big.NewInt(0)
	}
	if v, ok := new(big.Int).SetString(raw, 10); ok && v != nil {
		return v
	}
	return big.NewInt(0)
}

func normalizeUnsignedBigInt(raw string) string {
	v := parseSignedBigInt(raw)
	if v.Sign() < 0 {
		return "0"
	}
	return v.String()
}

func addSignedBigIntStrings(current string, delta string, sign int) string {
	base := parseSignedBigInt(current)
	change := parseSignedBigInt(delta)
	if sign < 0 {
		change.Neg(change)
	}
	base.Add(base, change)
	return base.String()
}

func addSignedUSDStrings(current *string, delta *string, sign int) *string {
	base := parseUSDString(current)
	change := parseUSDString(delta)
	if sign < 0 {
		change = -change
	}
	return formatUSDString(base + change)
}

func parseUSDString(value *string) float64 {
	if value == nil {
		return 0
	}
	raw := strings.TrimSpace(*value)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return v
}

func formatUSDString(value float64) *string {
	if value == 0 {
		return nil
	}
	out := strconv.FormatFloat(value, 'f', 4, 64)
	return &out
}

func eventAmountSign(event *models.SmartMoneyLPEvent) int {
	if event == nil {
		return 1
	}
	if strings.EqualFold(strings.TrimSpace(event.EventType), "remove") {
		return -1
	}
	return 1
}

func tickSpacingFromFeeTier(feeTier int) int {
	switch feeTier {
	case 100:
		return 1
	case 500:
		return 10
	case 2500:
		return 50
	case 3000:
		return 60
	case 10000:
		return 200
	case 20000:
		return 2000
	default:
		return 0
	}
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

	chainIDs := make([]int, 0, len(positions))
	nftIDs := make([]uint64, 0, len(positions))

	for _, pos := range positions {
		if pos.NftTokenID == 0 {
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
	}

	if len(chainIDs) == 0 || len(nftIDs) == 0 {
		return out, nil
	}

	var rows []PositionOpenAmountRow
	err := database.DB.WithContext(ctx).
		Table("sm_lp_positions p").
		Select(`
			p.chain_id,
			p.nft_token_id,
			MAX(COALESCE(ap.net_total_usd, evt_net.net_amount_usd, 0)) AS position_amount_usd
		`).
		Joins(`
			LEFT JOIN sm_lp_active_positions ap
				ON ap.chain_id = p.chain_id AND ap.nft_token_id = p.nft_token_id
		`).
		Joins(`
			LEFT JOIN (
				SELECT
					chain_id,
					nft_token_id,
					SUM(
						CASE
							WHEN event_type = 'add' THEN COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
							WHEN event_type = 'remove' THEN -COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
							ELSE 0
						END
					) AS net_amount_usd
				FROM sm_lp_events
				WHERE event_type IN ('add', 'remove')
				GROUP BY chain_id, nft_token_id
			) evt_net
				ON evt_net.chain_id = p.chain_id
				AND evt_net.nft_token_id = p.nft_token_id
		`).
		Where("p.chain_id IN ? AND p.nft_token_id IN ?", chainIDs, nftIDs).
		Group("p.chain_id, p.nft_token_id").
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

type PoolPositionRangeRow struct {
	PoolAddress    string  `json:"pool_address"`
	TickLower      *int    `json:"tick_lower"`
	TickUpper      *int    `json:"tick_upper"`
	PositionCount  int     `json:"position_count"`
	TotalAmountUSD float64 `json:"total_amount_usd"`
}

type PoolRangeGroup struct {
	RangePercent   float64 `json:"range_percent"`
	PositionCount  int     `json:"position_count"`
	TotalAmountUSD float64 `json:"total_amount_usd"`
}

func (r *Repository) GetPoolTotalAmountsUSD(ctx context.Context) (map[string]float64, error) {
	recentCutoff := time.Now().Add(-2 * time.Hour)
	var rows []PoolAmountRow
	err := database.DB.WithContext(ctx).Raw(`
		SELECT
			p.pool_address,
			COALESCE(SUM(COALESCE(ap.net_total_usd, evt_net.net_amount_usd, 0)), 0) AS total_amount_usd
		FROM sm_lp_positions p
		LEFT JOIN sm_lp_active_positions ap
			ON ap.chain_id = p.chain_id AND ap.nft_token_id = p.nft_token_id
		LEFT JOIN (
			SELECT
				chain_id,
				nft_token_id,
				SUM(
					CASE
						WHEN event_type = 'add' THEN COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
						WHEN event_type = 'remove' THEN -COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
						ELSE 0
					END
				) AS net_amount_usd
			FROM sm_lp_events
			WHERE event_type IN ('add', 'remove')
			GROUP BY chain_id, nft_token_id
		) evt_net
			ON evt_net.chain_id = p.chain_id
			AND evt_net.nft_token_id = p.nft_token_id
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

func (r *Repository) ListRecentOpenPositionRanges(ctx context.Context, poolAddresses []string) ([]PoolPositionRangeRow, error) {
	recentCutoff := time.Now().Add(-2 * time.Hour)
	normalizedPools := make([]string, 0, len(poolAddresses))
	for _, raw := range poolAddresses {
		addr := strings.ToLower(strings.TrimSpace(raw))
		if addr == "" {
			continue
		}
		normalizedPools = append(normalizedPools, addr)
	}

	query := `
		SELECT
			p.pool_address,
			p.tick_lower,
			p.tick_upper,
			COUNT(*) AS position_count,
			COALESCE(SUM(COALESCE(ap.net_total_usd, evt_net.net_amount_usd, 0)), 0) AS total_amount_usd
		FROM sm_lp_positions p
		LEFT JOIN sm_lp_active_positions ap
			ON ap.chain_id = p.chain_id AND ap.nft_token_id = p.nft_token_id
		LEFT JOIN (
			SELECT
				chain_id,
				nft_token_id,
				SUM(
					CASE
						WHEN event_type = 'add' THEN COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
						WHEN event_type = 'remove' THEN -COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
						ELSE 0
					END
				) AS net_amount_usd
			FROM sm_lp_events
			WHERE event_type IN ('add', 'remove')
			GROUP BY chain_id, nft_token_id
		) evt_net
			ON evt_net.chain_id = p.chain_id
			AND evt_net.nft_token_id = p.nft_token_id
		WHERE p.status = 'open' AND p.opened_at >= ?
	`
	args := []interface{}{recentCutoff}
	if len(normalizedPools) > 0 {
		query += ` AND LOWER(p.pool_address) IN ?`
		args = append(args, normalizedPools)
	}
	query += `
		GROUP BY p.pool_address, p.tick_lower, p.tick_upper
		ORDER BY p.pool_address ASC, total_amount_usd DESC, position_count DESC
	`

	var rows []PoolPositionRangeRow
	err := database.DB.WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// --- Aggregate queries ---

type PoolAggRow struct {
	PoolAddress            string           `json:"pool_address"`
	Token0Symbol           string           `json:"token0_symbol"`
	Token1Symbol           string           `json:"token1_symbol"`
	Token0Address          string           `json:"token0_address"`
	Token1Address          string           `json:"token1_address"`
	FeeTier                *int             `json:"fee_tier"`
	Protocol               string           `json:"protocol"`
	ChainID                int              `json:"chain_id"`
	OpenPositionCount      int              `json:"open_position_count"`
	WalletCount            int              `json:"wallet_count"`
	LatestEventAt          time.Time        `json:"latest_event_at"`
	TradingPair            string           `json:"trading_pair"`
	DisplayTokenAddress    string           `json:"display_token_address,omitempty"`
	DisplayTokenSymbol     string           `json:"display_token_symbol,omitempty"`
	DisplayTokenLogoURL    string           `json:"display_token_logo_url,omitempty"`
	TotalPositionAmountUSD float64          `json:"total_position_amount_usd"`
	RangeGroups            []PoolRangeGroup `gorm:"-" json:"range_groups,omitempty"`
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
	PoolAddress            string           `json:"pool_address"`
	Token0Symbol           string           `json:"token0_symbol"`
	Token1Symbol           string           `json:"token1_symbol"`
	Token0Address          string           `json:"token0_address"`
	Token1Address          string           `json:"token1_address"`
	FeeTier                *int             `json:"fee_tier"`
	Protocol               string           `json:"protocol"`
	ChainID                int              `json:"chain_id"`
	OpenPositionCount      int              `json:"open_position_count"`
	WalletCount            int              `json:"wallet_count"`
	ClosedTodayCount       int              `json:"closed_today_count"`
	TotalPositionAmountUSD float64          `json:"total_position_amount_usd"`
	TradingPair            string           `json:"trading_pair"`
	CurrentPrice           string           `json:"current_price"`
	PriceChange24h         float64          `json:"price_change_24h"`
	DisplayTokenAddress    string           `json:"display_token_address,omitempty"`
	DisplayTokenSymbol     string           `json:"display_token_symbol,omitempty"`
	DisplayTokenLogoURL    string           `json:"display_token_logo_url,omitempty"`
	RangeGroups            []PoolRangeGroup `gorm:"-" json:"range_groups,omitempty"`
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
			COALESCE(SUM(CASE WHEN status='open' AND opened_at >= ? THEN COALESCE(ap.net_total_usd, evt_net.net_amount_usd, 0) ELSE 0 END), 0) AS total_position_amount_usd
		FROM sm_lp_positions p
		LEFT JOIN sm_lp_active_positions ap
			ON ap.chain_id = p.chain_id AND ap.nft_token_id = p.nft_token_id
		LEFT JOIN (
			SELECT
				chain_id,
				nft_token_id,
				SUM(
					CASE
						WHEN event_type = 'add' THEN COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
						WHEN event_type = 'remove' THEN -COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
						ELSE 0
					END
				) AS net_amount_usd
			FROM sm_lp_events
			WHERE event_type IN ('add', 'remove')
			GROUP BY chain_id, nft_token_id
		) evt_net
			ON evt_net.chain_id = p.chain_id
			AND evt_net.nft_token_id = p.nft_token_id
		WHERE LOWER(p.pool_address) = ?
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
