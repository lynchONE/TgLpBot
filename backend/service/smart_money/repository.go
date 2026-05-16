package smart_money

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{}

type WalletTransferActivityRow struct {
	WalletAddress    string
	ChainID          int
	HasTransferIn    int
	HasTransferOut   int
	TransferInCount  int
	TransferOutCount int
	TransferInUSD    float64
	TransferOutUSD   float64
}

type UserWalletRef struct {
	UserID        uint
	WalletID      uint
	WalletAddress string
}

type UserTransferActivityDayRow struct {
	Day              string
	HasTransferIn    int
	HasTransferOut   int
	TransferInCount  int
	TransferOutCount int
	TransferInUSD    float64
	TransferOutUSD   float64
}

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
const smartMoneyDisplayRecentWindow = 24 * time.Hour
const smartMoneyPoolRecentOperationWindow = 2 * time.Hour

func smartMoneyDisplayRecentCutoff() time.Time {
	return time.Now().Add(-smartMoneyDisplayRecentWindow)
}

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

func (r *Repository) GetAllUserWalletRefs(ctx context.Context) (map[string][]UserWalletRef, error) {
	type row struct {
		UserID   uint
		WalletID uint
		Address  string
	}

	var rows []row
	if err := database.DB.WithContext(ctx).
		Model(&models.Wallet{}).
		Select("user_id, id AS wallet_id, address").
		Order("user_id ASC, id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make(map[string][]UserWalletRef, len(rows))
	for _, row := range rows {
		addr := strings.ToLower(strings.TrimSpace(row.Address))
		if addr == "" || row.UserID == 0 || row.WalletID == 0 {
			continue
		}
		out[addr] = append(out[addr], UserWalletRef{
			UserID:        row.UserID,
			WalletID:      row.WalletID,
			WalletAddress: addr,
		})
	}
	return out, nil
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

func (r *Repository) GetLPScanState(ctx context.Context, chainID int) (*models.SmartMoneyScanState, error) {
	var state models.SmartMoneyScanState
	err := database.DB.WithContext(ctx).
		Where("chain_id = ?", chainID).
		First(&state).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
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

// --- SmartMoneyWalletTransferEvent ---

func (r *Repository) InsertWalletTransferEvents(tx *gorm.DB, events []*models.SmartMoneyWalletTransferEvent) (int64, error) {
	if tx == nil || len(events) == 0 {
		return 0, nil
	}

	items := make([]*models.SmartMoneyWalletTransferEvent, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		event.WalletAddress = strings.ToLower(strings.TrimSpace(event.WalletAddress))
		event.Direction = strings.ToLower(strings.TrimSpace(event.Direction))
		event.AssetType = strings.ToLower(strings.TrimSpace(event.AssetType))
		event.TokenAddress = strings.ToLower(strings.TrimSpace(event.TokenAddress))
		event.TokenSymbol = strings.TrimSpace(event.TokenSymbol)
		event.TxHash = strings.ToLower(strings.TrimSpace(event.TxHash))
		items = append(items, event)
	}
	if len(items) == 0 {
		return 0, nil
	}

	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(items)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func (r *Repository) AggregateWalletTransferActivity(ctx context.Context, wallets []models.MonitoredWallet, start time.Time, end time.Time) ([]WalletTransferActivityRow, error) {
	if len(wallets) == 0 || !start.Before(end) {
		return nil, nil
	}

	addresses := make([]string, 0, len(wallets))
	chainIDs := make([]int, 0, len(wallets))
	addrSeen := make(map[string]struct{}, len(wallets))
	chainSeen := make(map[int]struct{}, len(wallets))
	for _, wallet := range wallets {
		addr := strings.ToLower(strings.TrimSpace(wallet.Address))
		if addr != "" {
			if _, ok := addrSeen[addr]; !ok {
				addrSeen[addr] = struct{}{}
				addresses = append(addresses, addr)
			}
		}
		if _, ok := chainSeen[wallet.ChainID]; !ok {
			chainSeen[wallet.ChainID] = struct{}{}
			chainIDs = append(chainIDs, wallet.ChainID)
		}
	}
	if len(addresses) == 0 || len(chainIDs) == 0 {
		return nil, nil
	}

	var rows []WalletTransferActivityRow
	err := database.DB.WithContext(ctx).
		Raw(`
			SELECT
				wallet_address,
				chain_id,
				MAX(CASE WHEN direction = 'in' THEN 1 ELSE 0 END) AS has_transfer_in,
				MAX(CASE WHEN direction = 'out' THEN 1 ELSE 0 END) AS has_transfer_out,
				COALESCE(SUM(CASE WHEN direction = 'in' THEN 1 ELSE 0 END), 0) AS transfer_in_count,
				COALESCE(SUM(CASE WHEN direction = 'out' THEN 1 ELSE 0 END), 0) AS transfer_out_count,
				COALESCE(SUM(CASE WHEN direction = 'in' THEN amount_usd ELSE 0 END), 0) AS transfer_in_usd,
				COALESCE(SUM(CASE WHEN direction = 'out' THEN amount_usd ELSE 0 END), 0) AS transfer_out_usd
			FROM sm_wallet_transfer_events
			WHERE wallet_address IN ?
			  AND chain_id IN ?
			  AND tx_timestamp >= ?
			  AND tx_timestamp < ?
			GROUP BY wallet_address, chain_id
		`, addresses, chainIDs, start, end).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// --- UserWalletTransferEvent ---

func (r *Repository) InsertUserWalletTransferEvents(tx *gorm.DB, events []*models.UserWalletTransferEvent) (int64, error) {
	if tx == nil || len(events) == 0 {
		return 0, nil
	}

	items := make([]*models.UserWalletTransferEvent, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		event.WalletAddress = strings.ToLower(strings.TrimSpace(event.WalletAddress))
		event.Chain = strings.ToLower(strings.TrimSpace(event.Chain))
		event.Direction = strings.ToLower(strings.TrimSpace(event.Direction))
		event.AssetType = strings.ToLower(strings.TrimSpace(event.AssetType))
		event.TokenAddress = strings.ToLower(strings.TrimSpace(event.TokenAddress))
		event.TokenSymbol = strings.TrimSpace(event.TokenSymbol)
		event.TxHash = strings.ToLower(strings.TrimSpace(event.TxHash))
		items = append(items, event)
	}
	if len(items) == 0 {
		return 0, nil
	}

	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(items)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func (r *Repository) AggregateUserTransferActivityByDay(ctx context.Context, userID uint, start time.Time, end time.Time) ([]UserTransferActivityDayRow, error) {
	if userID == 0 || !start.Before(end) {
		return nil, nil
	}

	var rows []UserTransferActivityDayRow
	err := database.DB.WithContext(ctx).
		Raw(`
			SELECT
				DATE_FORMAT(tx_timestamp, '%Y-%m-%d') AS day,
				MAX(CASE WHEN direction = 'in' THEN 1 ELSE 0 END) AS has_transfer_in,
				MAX(CASE WHEN direction = 'out' THEN 1 ELSE 0 END) AS has_transfer_out,
				COALESCE(SUM(CASE WHEN direction = 'in' THEN 1 ELSE 0 END), 0) AS transfer_in_count,
				COALESCE(SUM(CASE WHEN direction = 'out' THEN 1 ELSE 0 END), 0) AS transfer_out_count,
				COALESCE(SUM(CASE WHEN direction = 'in' THEN amount_usd ELSE 0 END), 0) AS transfer_in_usd,
				COALESCE(SUM(CASE WHEN direction = 'out' THEN amount_usd ELSE 0 END), 0) AS transfer_out_usd
			FROM user_wallet_transfer_events
			WHERE user_id = ?
			  AND tx_timestamp >= ?
			  AND tx_timestamp < ?
			GROUP BY DATE_FORMAT(tx_timestamp, '%Y-%m-%d')
			ORDER BY day ASC
		`, userID, start, end).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// --- SmartMoneyLPEvent ---

func (r *Repository) InsertLPEvent(tx *gorm.DB, event *models.SmartMoneyLPEvent) (bool, error) {
	if tx == nil || event == nil {
		return false, nil
	}
	event.WalletAddress = strings.ToLower(strings.TrimSpace(event.WalletAddress))
	event.PoolAddress = strings.ToLower(strings.TrimSpace(event.PoolAddress))
	event.Token0Address = strings.ToLower(strings.TrimSpace(event.Token0Address))
	event.Token1Address = strings.ToLower(strings.TrimSpace(event.Token1Address))
	event.TxHash = strings.ToLower(strings.TrimSpace(event.TxHash))
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(event)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *Repository) ListLPEvents(ctx context.Context, wallet, pool string, page, size int) ([]models.SmartMoneyLPEvent, int64, error) {
	db := database.DB.WithContext(ctx).Model(&models.SmartMoneyLPEvent{})
	if wallet != "" {
		db = db.Where("wallet_address = ?", strings.ToLower(strings.TrimSpace(wallet)))
	}
	if pool != "" {
		db = db.Where("pool_address = ?", strings.ToLower(strings.TrimSpace(pool)))
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
	recentCutoff := smartMoneyDisplayRecentCutoff()
	if status != "" && status != "all" {
		db = db.Where(smartMoneyPositionTable+".status = ?", status)
		if status == "open" {
			db = db.Where(smartMoneyPositionTable+".opened_at >= ?", recentCutoff)
		}
	}
	if wallet != "" {
		db = db.Where(smartMoneyPositionTable+".wallet_address = ?", strings.ToLower(strings.TrimSpace(wallet)))
	}
	if pool != "" {
		db = db.Where(smartMoneyPositionTable+".pool_address = ?", strings.ToLower(strings.TrimSpace(pool)))
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
	recentCutoff := smartMoneyDisplayRecentCutoff()
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
		conditions = append(conditions, "pool_address IN ?")
		args = append(args, poolIdentifiers)
	}

	var positions []models.SmartMoneyLPPosition
	err := db.
		Where(strings.Join(conditions, " OR "), args...).
		Order("opened_at DESC").
		Find(&positions).Error
	return positions, err
}

func (r *Repository) ListRecentOpenPositionsForStateRepair(ctx context.Context, since time.Time, limit int) ([]models.SmartMoneyLPPosition, error) {
	if limit <= 0 {
		limit = 50
	}
	var positions []models.SmartMoneyLPPosition
	err := database.DB.WithContext(ctx).
		Where("status = ? AND opened_at >= ?", "open", since).
		Order("updated_at DESC").
		Limit(limit).
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
				active.IsActive = liveLiquidity.Sign() > 0
				if active.IsActive {
					active.ClosedAt = nil
				} else if active.ClosedAt == nil {
					closedAt := time.Now()
					active.ClosedAt = &closedAt
				}
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
		Where("address = ?", addr).
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
		if !common.IsHexAddress(strings.TrimSpace(active.PoolManagerAddress)) || strings.TrimSpace(active.PoolAddress) == "" {
			return nil
		}
		info, err := blockchain.GetV4PositionInfo(
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
	recentCutoff := smartMoneyDisplayRecentCutoff()
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
	recentCutoff := smartMoneyDisplayRecentCutoff()
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
		query += ` AND p.pool_address IN ?`
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

type PoolFeeHeatmapOptions struct {
	WindowSeconds int
	Sort          string
	Now           time.Time
}

type PoolFeeHeatmapRow struct {
	PoolAddress               string    `json:"pool_address"`
	Token0Symbol              string    `json:"token0_symbol"`
	Token1Symbol              string    `json:"token1_symbol"`
	Token0Address             string    `json:"token0_address"`
	Token1Address             string    `json:"token1_address"`
	FeeTier                   *int      `json:"fee_tier"`
	Protocol                  string    `json:"protocol"`
	ChainID                   int       `json:"chain_id"`
	OpenPositionCount         int       `json:"open_position_count"`
	WalletCount               int       `json:"wallet_count"`
	LatestEventAt             time.Time `json:"latest_event_at"`
	TradingPair               string    `json:"trading_pair"`
	DisplayTokenAddress       string    `json:"display_token_address,omitempty"`
	DisplayTokenSymbol        string    `json:"display_token_symbol,omitempty"`
	DisplayTokenLogoURL       string    `json:"display_token_logo_url,omitempty"`
	TotalPositionAmountUSD    float64   `json:"total_position_amount_usd"`
	FeeUSD                    float64   `json:"fee_usd"`
	ProjectedFeeUSD           float64   `json:"projected_fee_usd"`
	FeeRatePer1KUSDWindow     float64   `json:"fee_rate_per_1k_usd_window"`
	FeeRatePer1KUSDPerMinute  float64   `json:"fee_rate_per_1k_usd_per_min"`
	AveragePositionAgeSeconds float64   `json:"average_position_age_seconds"`
	FeePositionCount          int       `json:"fee_position_count"`
	RatePositionCount         int       `json:"rate_position_count"`
	MissingFeeCount           int       `json:"missing_fee_count"`
	MissingAmountCount        int       `json:"missing_amount_count"`
	MissingAgeCount           int       `json:"missing_age_count"`
	SampleStatus              string    `json:"sample_status"`
}

type poolFeeHeatmapAgg struct {
	row             PoolFeeHeatmapRow
	wallets         map[string]struct{}
	positionRefs    map[string]struct{}
	feeUSDPerSec    float64
	rateAmountUSD   float64
	validAgeSeconds float64
}

func (r *Repository) ListPoolFeeHeatmap(ctx context.Context, opts PoolFeeHeatmapOptions) ([]PoolFeeHeatmapRow, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	positions, err := r.ListActivePositionsForFeeHeatmap(ctx)
	if err != nil {
		return nil, err
	}

	return BuildPoolFeeHeatmapRows(positions, PoolFeeHeatmapOptions{
		WindowSeconds: opts.WindowSeconds,
		Sort:          opts.Sort,
		Now:           now,
	}), nil
}

func (r *Repository) ListActivePositionsForFeeHeatmap(ctx context.Context) ([]models.SmartMoneyActivePosition, error) {
	var positions []models.SmartMoneyActivePosition
	err := database.DB.WithContext(ctx).
		Where("is_active = ? AND opened_at >= ?", true, smartMoneyDisplayRecentCutoff()).
		Order("opened_at ASC, id ASC").
		Find(&positions).Error
	if err != nil {
		return nil, err
	}
	return positions, nil
}

func (r *Repository) UpdateActivePositionFeeSnapshot(ctx context.Context, id uint, feeUSD *float64, status string, now time.Time) error {
	updates := map[string]interface{}{
		"fee_status":     strings.TrimSpace(status),
		"fee_updated_at": &now,
	}
	if feeUSD != nil {
		updates["fee_usd"] = strconv.FormatFloat(*feeUSD, 'f', 4, 64)
	}
	return database.DB.WithContext(ctx).
		Model(&models.SmartMoneyActivePosition{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func BuildPoolFeeHeatmapRows(positions []models.SmartMoneyActivePosition, opts PoolFeeHeatmapOptions) []PoolFeeHeatmapRow {
	return buildPoolFeeHeatmapRows(positions, opts)
}

func buildPoolFeeHeatmapRows(positions []models.SmartMoneyActivePosition, opts PoolFeeHeatmapOptions) []PoolFeeHeatmapRow {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	windowSeconds := opts.WindowSeconds
	if windowSeconds <= 0 {
		windowSeconds = 60
	}

	byPool := make(map[string]*poolFeeHeatmapAgg)
	for i := range positions {
		pos := positions[i]
		poolAddress := strings.ToLower(strings.TrimSpace(pos.PoolAddress))
		if poolAddress == "" {
			continue
		}

		agg := byPool[poolAddress]
		if agg == nil {
			agg = &poolFeeHeatmapAgg{
				row: PoolFeeHeatmapRow{
					PoolAddress:         poolAddress,
					Token0Symbol:        strings.TrimSpace(pos.Token0Symbol),
					Token1Symbol:        strings.TrimSpace(pos.Token1Symbol),
					Token0Address:       strings.TrimSpace(pos.Token0Address),
					Token1Address:       strings.TrimSpace(pos.Token1Address),
					FeeTier:             pos.FeeTier,
					Protocol:            strings.TrimSpace(pos.Protocol),
					ChainID:             pos.ChainID,
					LatestEventAt:       pos.OpenedAt,
					DisplayTokenAddress: "",
					DisplayTokenSymbol:  "",
				},
				wallets:      make(map[string]struct{}),
				positionRefs: make(map[string]struct{}),
			}
			byPool[poolAddress] = agg
		}

		wallet := strings.ToLower(strings.TrimSpace(pos.WalletAddress))
		if wallet != "" {
			agg.wallets[wallet] = struct{}{}
		}
		positionRef := strings.TrimSpace(pos.PositionRef)
		if positionRef == "" {
			positionRef = strconv.FormatUint(pos.NftTokenID, 10)
		}
		if positionRef != "" && positionRef != "0" {
			agg.positionRefs[positionRef] = struct{}{}
		}

		if eventAt := latestSmartMoneyPositionEventAt(pos); eventAt.After(agg.row.LatestEventAt) {
			agg.row.LatestEventAt = eventAt
		}

		feeUSD, feeOK := parseSmartMoneyPositiveOrZero(pos.FeeUSD)
		if feeOK && !smartMoneyHeatmapFeeSnapshotUsable(pos.FeeStatus) {
			feeOK = false
		}
		if !feeOK {
			agg.row.MissingFeeCount++
		} else {
			agg.row.FeeUSD += feeUSD
			agg.row.FeePositionCount++
		}

		amountUSD, amountOK := smartMoneyPositionAmountUSD(pos)
		if !amountOK {
			agg.row.MissingAmountCount++
		} else {
			agg.row.TotalPositionAmountUSD += amountUSD
		}

		ageSeconds, ageOK := smartMoneyPositionAgeSeconds(pos.OpenedAt, now)
		if !ageOK {
			agg.row.MissingAgeCount++
		}

		if feeOK && amountOK && ageOK {
			agg.feeUSDPerSec += feeUSD / ageSeconds
			agg.rateAmountUSD += amountUSD
			agg.validAgeSeconds += ageSeconds
			agg.row.RatePositionCount++
		}
	}

	rows := make([]PoolFeeHeatmapRow, 0, len(byPool))
	for _, agg := range byPool {
		row := agg.row
		row.WalletCount = len(agg.wallets)
		row.OpenPositionCount = len(agg.positionRefs)
		if row.OpenPositionCount == 0 {
			row.OpenPositionCount = row.FeePositionCount + row.MissingFeeCount
		}
		if agg.rateAmountUSD > 0 && row.RatePositionCount > 0 {
			window := float64(windowSeconds)
			row.ProjectedFeeUSD = agg.feeUSDPerSec * window
			row.FeeRatePer1KUSDWindow = row.ProjectedFeeUSD / agg.rateAmountUSD * 1000
			row.FeeRatePer1KUSDPerMinute = agg.feeUSDPerSec * 60 / agg.rateAmountUSD * 1000
			row.AveragePositionAgeSeconds = agg.validAgeSeconds / float64(row.RatePositionCount)
		}
		row.SampleStatus = smartMoneyHeatmapSampleStatus(row)
		rows = append(rows, row)
	}

	sortPoolFeeHeatmapRows(rows, opts.Sort)
	return rows
}

func latestSmartMoneyPositionEventAt(pos models.SmartMoneyActivePosition) time.Time {
	latest := pos.OpenedAt
	if pos.LastAddAt != nil && pos.LastAddAt.After(latest) {
		latest = *pos.LastAddAt
	}
	if pos.LastRemoveAt != nil && pos.LastRemoveAt.After(latest) {
		latest = *pos.LastRemoveAt
	}
	return latest
}

func smartMoneyHeatmapSampleStatus(row PoolFeeHeatmapRow) string {
	if row.RatePositionCount <= 0 {
		return "insufficient"
	}
	if row.MissingFeeCount > 0 || row.MissingAmountCount > 0 || row.MissingAgeCount > 0 {
		return "partial"
	}
	return "ok"
}

func smartMoneyHeatmapFeeSnapshotUsable(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "ok":
		return true
	default:
		return false
	}
}

func parseSmartMoneyPositiveOrZero(value *string) (float64, bool) {
	if value == nil {
		return 0, false
	}
	raw := strings.TrimSpace(*value)
	if raw == "" {
		return 0, false
	}
	num, err := strconv.ParseFloat(raw, 64)
	if err != nil || num < 0 {
		return 0, false
	}
	return num, true
}

func smartMoneyPositionAmountUSD(pos models.SmartMoneyActivePosition) (float64, bool) {
	if value, ok := parseSmartMoneyPositiveAmount(pos.NetTotalUSD); ok {
		return value, true
	}
	return parseSmartMoneyPositiveAmount(pos.EntryTotalUSD)
}

func parseSmartMoneyPositiveAmount(value *string) (float64, bool) {
	if value == nil {
		return 0, false
	}
	raw := strings.TrimSpace(*value)
	if raw == "" {
		return 0, false
	}
	num, err := strconv.ParseFloat(raw, 64)
	if err != nil || num <= 0 {
		return 0, false
	}
	return num, true
}

func smartMoneyPositionAgeSeconds(openedAt time.Time, now time.Time) (float64, bool) {
	if openedAt.IsZero() || now.IsZero() || openedAt.After(now) {
		return 0, false
	}
	seconds := now.Sub(openedAt).Seconds()
	if seconds <= 0 {
		return 1, true
	}
	return seconds, true
}

func sortPoolFeeHeatmapRows(rows []PoolFeeHeatmapRow, sortKey string) {
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]
		if sortKey == "fee" {
			if left.FeeUSD != right.FeeUSD {
				return left.FeeUSD > right.FeeUSD
			}
			if left.FeeRatePer1KUSDWindow != right.FeeRatePer1KUSDWindow {
				return left.FeeRatePer1KUSDWindow > right.FeeRatePer1KUSDWindow
			}
		} else {
			if left.FeeRatePer1KUSDWindow != right.FeeRatePer1KUSDWindow {
				return left.FeeRatePer1KUSDWindow > right.FeeRatePer1KUSDWindow
			}
			if left.ProjectedFeeUSD != right.ProjectedFeeUSD {
				return left.ProjectedFeeUSD > right.ProjectedFeeUSD
			}
			if left.FeeUSD != right.FeeUSD {
				return left.FeeUSD > right.FeeUSD
			}
		}
		if left.TotalPositionAmountUSD != right.TotalPositionAmountUSD {
			return left.TotalPositionAmountUSD > right.TotalPositionAmountUSD
		}
		if !left.LatestEventAt.Equal(right.LatestEventAt) {
			return left.LatestEventAt.After(right.LatestEventAt)
		}
		return left.PoolAddress < right.PoolAddress
	})
}

func (r *Repository) ListPoolsWithPositions(ctx context.Context) ([]PoolAggRow, error) {
	var rows []PoolAggRow
	cutoff := smartMoneyDisplayRecentCutoff()
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
			COUNT(*) AS open_position_count,
			COUNT(DISTINCT p.wallet_address) AS wallet_count,
			MAX(GREATEST(
				p.opened_at,
				COALESCE(ap.last_add_at, p.opened_at),
				COALESCE(ap.last_remove_at, p.opened_at),
				COALESCE(evt_net.latest_event_at, p.opened_at)
			)) AS latest_event_at,
			COALESCE(SUM(COALESCE(ap.net_total_usd, evt_net.net_amount_usd, 0)), 0) AS total_position_amount_usd
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
				) AS net_amount_usd,
				MAX(tx_timestamp) AS latest_event_at
			FROM sm_lp_events
			WHERE event_type IN ('add', 'remove')
			GROUP BY chain_id, nft_token_id
		) evt_net
			ON evt_net.chain_id = p.chain_id
			AND evt_net.nft_token_id = p.nft_token_id
		WHERE p.status = 'open' AND p.opened_at >= ?
		GROUP BY p.pool_address, p.token0_symbol, p.token1_symbol, p.token0_address, p.token1_address, p.fee_tier, p.protocol, p.chain_id
		ORDER BY total_position_amount_usd DESC, latest_event_at DESC
	`, cutoff).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	sortPoolAggRows(rows, time.Now())
	return rows, nil
}

func sortPoolAggRows(rows []PoolAggRow, now time.Time) {
	recentCutoff := now.Add(-smartMoneyPoolRecentOperationWindow)
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]
		leftRecent := !left.LatestEventAt.IsZero() && !left.LatestEventAt.Before(recentCutoff)
		rightRecent := !right.LatestEventAt.IsZero() && !right.LatestEventAt.Before(recentCutoff)

		if leftRecent != rightRecent {
			return leftRecent
		}
		if leftRecent {
			if !left.LatestEventAt.Equal(right.LatestEventAt) {
				return left.LatestEventAt.After(right.LatestEventAt)
			}
			if left.TotalPositionAmountUSD != right.TotalPositionAmountUSD {
				return left.TotalPositionAmountUSD > right.TotalPositionAmountUSD
			}
			return left.PoolAddress < right.PoolAddress
		}
		if left.TotalPositionAmountUSD != right.TotalPositionAmountUSD {
			return left.TotalPositionAmountUSD > right.TotalPositionAmountUSD
		}
		if !left.LatestEventAt.Equal(right.LatestEventAt) {
			return left.LatestEventAt.After(right.LatestEventAt)
		}
		return left.PoolAddress < right.PoolAddress
	})
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
	recentCutoff := smartMoneyDisplayRecentCutoff()
	err := database.DB.WithContext(ctx).Raw(`
		SELECT
			p.pool_address,
			MAX(p.token0_symbol) AS token0_symbol,
			MAX(p.token1_symbol) AS token1_symbol,
			MAX(p.token0_address) AS token0_address,
			MAX(p.token1_address) AS token1_address,
			MAX(p.fee_tier) AS fee_tier,
			MAX(p.protocol) AS protocol,
			MAX(p.chain_id) AS chain_id,
			SUM(CASE WHEN p.status='open' AND p.opened_at >= ? THEN 1 ELSE 0 END) AS open_position_count,
			COUNT(DISTINCT CASE WHEN p.status='open' AND p.opened_at >= ? THEN p.wallet_address END) AS wallet_count,
			SUM(CASE WHEN p.status='closed' AND p.closed_at >= ? THEN 1 ELSE 0 END) AS closed_today_count,
			COALESCE(SUM(CASE WHEN p.status='open' AND p.opened_at >= ? THEN COALESCE(ap.net_total_usd, evt_net.net_amount_usd, 0) ELSE 0 END), 0) AS total_position_amount_usd
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
		WHERE p.pool_address = ?
		GROUP BY p.pool_address
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
	recentCutoff := smartMoneyDisplayRecentCutoff()

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
	Address                string     `json:"address"`
	Label                  *string    `json:"label"`
	AvatarURL              *string    `json:"avatar_url"`
	WalletBalanceUSD       *float64   `json:"wallet_balance_usd,omitempty"`
	Source                 string     `json:"source"`
	SourceContract         *string    `json:"source_contract"`
	IsActive               bool       `json:"is_active"`
	ChainID                int        `json:"chain_id"`
	OpenPositionCount      int        `json:"open_position_count"`
	ActivePoolCount        int        `json:"active_pool_count"`
	TotalAddCount          int        `json:"total_add_count"`
	TotalRemoveCount       int        `json:"total_remove_count"`
	LastActiveAt           *time.Time `json:"last_active_at"`
	CreatedAt              time.Time  `json:"created_at"`
	TotalPositionAmountUSD float64    `json:"total_position_amount_usd"`
}

func (r *Repository) ListWalletsWithStats(ctx context.Context, page, size int, keyword, source string, activeOnly *bool) ([]WalletStatsRow, int64, error) {
	countDB := database.DB.WithContext(ctx).Model(&models.MonitoredWallet{})
	if keyword != "" {
		kw := "%" + strings.ToLower(strings.TrimSpace(keyword)) + "%"
		countDB = countDB.Where("LOWER(address) LIKE ? OR LOWER(COALESCE(label, '')) LIKE ?", kw, kw)
	}
	if source != "" {
		countDB = countDB.Where("source = ?", source)
	}
	if activeOnly != nil {
		countDB = countDB.Where("is_active = ?", *activeOnly)
	}

	var total int64
	if err := countDB.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	recentCutoff := smartMoneyDisplayRecentCutoff()
	eventNetSubQuery := database.DB.WithContext(ctx).
		Table("sm_lp_events").
		Select(`
			chain_id,
			nft_token_id,
			SUM(
				CASE
					WHEN event_type = 'add' THEN COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
					WHEN event_type = 'remove' THEN -COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
					ELSE 0
				END
			) AS net_amount_usd
		`).
		Where("event_type IN ?", []string{"add", "remove"}).
		Group("chain_id, nft_token_id")

	walletStatsSubQuery := database.DB.WithContext(ctx).
		Table("sm_lp_positions p").
		Select(`
			p.wallet_address,
			p.chain_id,
			COUNT(*) AS open_position_count,
			COUNT(DISTINCT p.pool_address) AS active_pool_count,
			COALESCE(SUM(COALESCE(ap.net_total_usd, evt_net.net_amount_usd, 0)), 0) AS total_position_amount_usd
		`).
		Joins("LEFT JOIN sm_lp_active_positions ap ON ap.chain_id = p.chain_id AND ap.nft_token_id = p.nft_token_id").
		Joins("LEFT JOIN (?) evt_net ON evt_net.chain_id = p.chain_id AND evt_net.nft_token_id = p.nft_token_id", eventNetSubQuery).
		Where("p.status = ? AND p.opened_at >= ?", "open", recentCutoff).
		Group("p.wallet_address, p.chain_id")

	walletEventSubQuery := database.DB.WithContext(ctx).
		Table("sm_lp_events e").
		Select(`
			e.wallet_address,
			e.chain_id,
			SUM(CASE WHEN e.event_type = 'add' THEN 1 ELSE 0 END) AS total_add_count,
			SUM(CASE WHEN e.event_type = 'remove' THEN 1 ELSE 0 END) AS total_remove_count,
			MAX(e.tx_timestamp) AS last_active_at
		`).
		Group("e.wallet_address, e.chain_id")

	query := database.DB.WithContext(ctx).
		Table("monitored_wallets w").
		Select(`
			w.address,
			w.label,
			w.avatar_url,
			w.source,
			w.source_contract,
			w.is_active,
			w.chain_id,
			COALESCE(ws.open_position_count, 0) AS open_position_count,
			COALESCE(ws.active_pool_count, 0) AS active_pool_count,
			COALESCE(we.total_add_count, 0) AS total_add_count,
			COALESCE(we.total_remove_count, 0) AS total_remove_count,
			we.last_active_at AS last_active_at,
			w.created_at,
			COALESCE(ws.total_position_amount_usd, 0) AS total_position_amount_usd
		`).
		Joins("LEFT JOIN (?) ws ON ws.wallet_address = w.address AND ws.chain_id = w.chain_id", walletStatsSubQuery).
		Joins("LEFT JOIN (?) we ON we.wallet_address = w.address AND we.chain_id = w.chain_id", walletEventSubQuery)

	if keyword != "" {
		kw := "%" + strings.ToLower(strings.TrimSpace(keyword)) + "%"
		query = query.Where("LOWER(w.address) LIKE ? OR LOWER(COALESCE(w.label, '')) LIKE ?", kw, kw)
	}
	if source != "" {
		query = query.Where("w.source = ?", source)
	}
	if activeOnly != nil {
		query = query.Where("w.is_active = ?", *activeOnly)
	}

	rows := make([]WalletStatsRow, 0, size)
	err := query.
		Order("COALESCE(ws.total_position_amount_usd, 0) DESC").
		Order("COALESCE(ws.open_position_count, 0) DESC").
		Order("we.last_active_at DESC").
		Order("w.created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
