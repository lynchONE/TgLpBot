package smart_money

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
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

type WatchActivityQuery struct {
	UserID        uint
	ChainID       int
	WalletAddress string
	Page          int
	Size          int
}

type MonitoredWalletImportResult struct {
	Created         int      `json:"created"`
	Reactivated     int      `json:"reactivated"`
	SkippedExisting int      `json:"skipped_existing"`
	Invalid         []string `json:"invalid"`
}

var smartMoneyNetAmountOrderJoin = smartMoneyActivePositionJoinSQL(smartMoneyPositionTable) + `
LEFT JOIN (
	SELECT
		chain_id,
		protocol,
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
	GROUP BY chain_id, protocol, nft_token_id
) evt_net
	ON evt_net.chain_id = sm_lp_positions.chain_id
	AND evt_net.protocol = sm_lp_positions.protocol
	AND evt_net.nft_token_id = sm_lp_positions.nft_token_id
`

const smartMoneyNetAmountOrderExpr = "COALESCE(ap.net_total_usd, evt_net.net_amount_usd, 0)"
const smartMoneyPositionTable = "sm_lp_positions"
const smartMoneyDisplayRecentWindow = 24 * time.Hour
const smartMoneyPoolRecentOperationWindow = 2 * time.Hour

func smartMoneyPositionRefSQL(tableAlias string) string {
	return fmt.Sprintf(`
LOWER(CONCAT(
	CAST(%[1]s.chain_id AS CHAR), ':',
	TRIM(%[1]s.protocol), ':',
	TRIM(%[1]s.wallet_address), ':',
	CASE
		WHEN %[1]s.nft_token_id > 0 THEN CAST(%[1]s.nft_token_id AS CHAR)
		ELSE CONCAT(
			TRIM(%[1]s.pool_address), ':',
			COALESCE(CAST(%[1]s.tick_lower AS CHAR), ''), ':',
			COALESCE(CAST(%[1]s.tick_upper AS CHAR), '')
		)
	END
)) COLLATE utf8mb4_unicode_ci`, tableAlias)
}

func smartMoneyActivePositionJoinSQL(tableAlias string) string {
	return fmt.Sprintf(`
LEFT JOIN sm_lp_active_positions ap
	ON ap.chain_id = %[1]s.chain_id
	AND ap.protocol = %[1]s.protocol
	AND ap.nft_token_id = %[1]s.nft_token_id
`, tableAlias)
}

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

func (r *Repository) ImportTokenLiquidityWallets(ctx context.Context, chainID int, tokenAddress string, wallets []string, labelPrefix string) (MonitoredWalletImportResult, error) {
	tokenAddress = strings.ToLower(strings.TrimSpace(tokenAddress))
	return r.importMonitoredWalletsFromLiquiditySource(ctx, chainID, tokenAddress, wallets, labelPrefix, MonitoredWalletSourceTokenLiquidityIndexer)
}

func (r *Repository) ImportPoolLiquidityWallets(ctx context.Context, chainID int, poolIdentifier string, wallets []string, labelPrefix string) (MonitoredWalletImportResult, error) {
	poolIdentifier = strings.ToLower(strings.TrimSpace(poolIdentifier))
	return r.importMonitoredWalletsFromLiquiditySource(ctx, chainID, poolIdentifier, wallets, labelPrefix, MonitoredWalletSourcePoolLiquidityRadar)
}

func (r *Repository) importMonitoredWalletsFromLiquiditySource(ctx context.Context, chainID int, sourceContract string, wallets []string, labelPrefix string, source string) (MonitoredWalletImportResult, error) {
	result := MonitoredWalletImportResult{Invalid: []string{}}
	sourceContract = strings.ToLower(strings.TrimSpace(sourceContract))
	source = strings.TrimSpace(source)
	labelPrefix = strings.TrimSpace(labelPrefix)
	if chainID <= 0 {
		chainID = 56
	}
	if source == "" {
		return result, fmt.Errorf("source is required")
	}
	if sourceContract == "" {
		return result, fmt.Errorf("source_contract is required")
	}

	seen := make(map[string]struct{}, len(wallets))
	normalized := make([]string, 0, len(wallets))
	for _, raw := range wallets {
		addr := strings.ToLower(strings.TrimSpace(raw))
		if !isRepositoryEVMAddress(addr) {
			result.Invalid = append(result.Invalid, strings.TrimSpace(raw))
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		normalized = append(normalized, addr)
	}
	if len(result.Invalid) > 0 {
		return result, nil
	}
	if len(normalized) == 0 {
		return result, nil
	}

	err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, addr := range normalized {
			var existing models.MonitoredWallet
			err := tx.Where("address = ? AND chain_id = ?", addr, chainID).First(&existing).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			}
			if err == nil {
				if existing.IsActive {
					result.SkippedExisting++
					continue
				}
				updates := map[string]any{
					"is_active":       true,
					"source":          source,
					"source_contract": sourceContract,
				}
				if labelPrefix != "" {
					label := labelPrefix + " " + addr[len(addr)-4:]
					updates["label"] = label
				}
				if err := tx.Model(&models.MonitoredWallet{}).
					Where("address = ? AND chain_id = ?", addr, chainID).
					Updates(updates).Error; err != nil {
					return err
				}
				result.Reactivated++
				continue
			}

			var labelPtr *string
			if labelPrefix != "" {
				label := labelPrefix + " " + addr[len(addr)-4:]
				labelPtr = &label
			}
			walletSourceContract := sourceContract
			wallet := &models.MonitoredWallet{
				Address:        addr,
				ChainID:        chainID,
				Source:         source,
				SourceContract: &walletSourceContract,
				Label:          labelPtr,
				IsActive:       true,
			}
			if err := tx.Create(wallet).Error; err != nil {
				return err
			}
			result.Created++
		}
		return nil
	})
	return result, err
}

func (r *Repository) DeleteMonitoredWallet(ctx context.Context, address string, chainID int) error {
	_, err := r.DeleteMonitoredWalletsWithHistory(ctx, []WalletRef{{
		Address: strings.ToLower(strings.TrimSpace(address)),
		ChainID: chainID,
	}})
	return err
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

func normalizeWatchActivityQuery(query WatchActivityQuery) WatchActivityQuery {
	query.WalletAddress = strings.ToLower(strings.TrimSpace(query.WalletAddress))
	if query.ChainID <= 0 {
		query.ChainID = 56
	}
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.Size <= 0 || query.Size > 100 {
		query.Size = 20
	}
	return query
}

const watchActivityJoinSQL = `
INNER JOIN smart_money_user_watch_wallets smww
	ON smww.wallet_address = sm_lp_events.wallet_address
	AND smww.chain = ?
	AND smww.user_id = ?
`

func buildWatchActivitySelectSQL(query WatchActivityQuery) (string, []any) {
	query = normalizeWatchActivityQuery(query)
	sql := strings.TrimSpace(`
SELECT sm_lp_events.*
FROM sm_lp_events
` + watchActivityJoinSQL + `
WHERE sm_lp_events.chain_id = ?
  AND sm_lp_events.event_type IN (?, ?)
`)
	args := []any{smartMoneyChainName(query.ChainID), query.UserID, query.ChainID, "add", "remove"}
	if query.WalletAddress != "" {
		sql += "\n  AND sm_lp_events.wallet_address = ?"
		args = append(args, query.WalletAddress)
	}
	sql += "\nORDER BY sm_lp_events.tx_timestamp DESC, sm_lp_events.id DESC\nLIMIT ? OFFSET ?"
	args = append(args, query.Size, (query.Page-1)*query.Size)
	return sql, args
}

func buildWatchActivityCountSQL(query WatchActivityQuery) (string, []any) {
	query = normalizeWatchActivityQuery(query)
	sql := strings.TrimSpace(`
SELECT COUNT(*)
FROM sm_lp_events
` + watchActivityJoinSQL + `
WHERE sm_lp_events.chain_id = ?
  AND sm_lp_events.event_type IN (?, ?)
`)
	args := []any{smartMoneyChainName(query.ChainID), query.UserID, query.ChainID, "add", "remove"}
	if query.WalletAddress != "" {
		sql += "\n  AND sm_lp_events.wallet_address = ?"
		args = append(args, query.WalletAddress)
	}
	return sql, args
}

func (r *Repository) CountWatchLPEvents(ctx context.Context, query WatchActivityQuery) (int64, error) {
	query = normalizeWatchActivityQuery(query)
	if query.UserID == 0 {
		return 0, nil
	}
	var total int64
	sql, args := buildWatchActivityCountSQL(query)
	err := database.DB.WithContext(ctx).Raw(sql, args...).Scan(&total).Error
	return total, err
}

func (r *Repository) ListWatchLPEvents(ctx context.Context, query WatchActivityQuery) ([]models.SmartMoneyLPEvent, int64, error) {
	query = normalizeWatchActivityQuery(query)
	if query.UserID == 0 {
		return nil, 0, nil
	}

	total, err := r.CountWatchLPEvents(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []models.SmartMoneyLPEvent{}, 0, nil
	}

	var events []models.SmartMoneyLPEvent
	sql, args := buildWatchActivitySelectSQL(query)
	err = database.DB.WithContext(ctx).Raw(sql, args...).Scan(&events).Error
	return events, total, err
}

// --- SmartMoneyLPPosition ---

func (r *Repository) UpsertLPPosition(tx *gorm.DB, event *models.SmartMoneyLPEvent, liveLiquidity *big.Int) error {
	active, err := r.UpsertActivePosition(tx, event, liveLiquidity)
	if err != nil || active == nil {
		return err
	}

	if event.NftTokenID == nil || *event.NftTokenID == 0 {
		return nil
	}

	var pos models.SmartMoneyLPPosition
	err = tx.Where("chain_id = ? AND protocol = ? AND nft_token_id = ?", event.ChainID, strings.TrimSpace(event.Protocol), *event.NftTokenID).First(&pos).Error
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

func (r *Repository) UpsertActivePosition(tx *gorm.DB, event *models.SmartMoneyLPEvent, liveLiquidity *big.Int) (*models.SmartMoneyActivePosition, error) {
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
		if liveLiquidity != nil {
			currentLiquidity = new(big.Int).Set(liveLiquidity)
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

func (r *Repository) LiveLiquiditySnapshotForEvent(ctx context.Context, event *models.SmartMoneyLPEvent) (*big.Int, error) {
	if ctx == nil || database.DB == nil || event == nil || event.NftTokenID == nil || *event.NftTokenID == 0 {
		return nil, nil
	}
	positionRef := BuildPositionRefFromEvent(event)
	if positionRef == "" {
		return nil, nil
	}
	var existing models.SmartMoneyActivePosition
	err := database.DB.WithContext(ctx).
		Select("id").
		Where("position_ref = ?", positionRef).
		First(&existing).Error
	if err == nil {
		return nil, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	active := &models.SmartMoneyActivePosition{
		PositionRef:   positionRef,
		WalletAddress: strings.ToLower(event.WalletAddress),
		ChainID:       event.ChainID,
		Protocol:      strings.TrimSpace(event.Protocol),
		NftTokenID:    *event.NftTokenID,
		PoolAddress:   strings.ToLower(strings.TrimSpace(event.PoolAddress)),
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
	applyManagerSnapshotToActive(active)
	liquidity, err := r.loadCurrentLiquiditySnapshot(ctx, event, active)
	if err != nil {
		return nil, nil
	}
	return liquidity, nil
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

func (r *Repository) MarkLPPositionMetadataInvalid(ctx context.Context, id uint, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil
	}
	return database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"metadata_status": "invalid",
			"metadata_error":  reason,
		}).Error
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
		Where("COALESCE(metadata_status, '') <> ?", "invalid").
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
	applyManagerSnapshotToActive(active)
	var liveLiquidity *big.Int
	if active.IsActive {
		liveLiquidity, _ = r.loadCurrentLiquiditySnapshot(ctx, nil, active)
	}
	if err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r.applyPoolSnapshotToActive(tx, active)
		applyManagerSnapshotToActive(active)
		r.hydrateActivePositionFromHistory(tx, active)
		applyLiveLiquiditySnapshot(active, liveLiquidity)
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
		Where("chain_id = ? AND protocol = ? AND nft_token_id = ?", active.ChainID, strings.TrimSpace(active.Protocol), active.NftTokenID).
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

func applyLiveLiquiditySnapshot(active *models.SmartMoneyActivePosition, liveLiquidity *big.Int) {
	if active == nil || liveLiquidity == nil {
		return
	}
	active.CurrentLiquidity = liveLiquidity.String()
	active.IsActive = liveLiquidity.Sign() > 0
	if active.IsActive {
		active.ClosedAt = nil
		return
	}
	if active.ClosedAt == nil {
		closedAt := time.Now()
		active.ClosedAt = &closedAt
	}
}

func (r *Repository) loadCurrentLiquiditySnapshot(ctx context.Context, event *models.SmartMoneyLPEvent, active *models.SmartMoneyActivePosition) (*big.Int, error) {
	if active == nil || active.NftTokenID == 0 {
		return nil, nil
	}
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}

	chainID := active.ChainID
	if chainID == 0 && event != nil {
		chainID = event.ChainID
	}
	chain := smartMoneyChainSlugFromID(chainID)
	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return nil, err
	}

	tokenID := new(big.Int).SetUint64(active.NftTokenID)
	switch strings.ToLower(strings.TrimSpace(active.Protocol)) {
	case "pancake_v3", "uniswap_v3":
		managerAddress := strings.TrimSpace(active.PositionManagerAddress)
		if !common.IsHexAddress(managerAddress) {
			return nil, nil
		}
		pm, err := blockchain.NewV3PositionManager(common.HexToAddress(managerAddress), client)
		if err != nil {
			return nil, err
		}
		info, err := pm.Positions(&bind.CallOpts{Context: ctx}, tokenID)
		if err != nil || info == nil || info.Liquidity == nil {
			return nil, err
		}
		return new(big.Int).Set(info.Liquidity), nil
	case "uniswap_v4":
		managerAddress := strings.TrimSpace(active.PositionManagerAddress)
		if !common.IsHexAddress(managerAddress) {
			return nil, nil
		}
		if !common.IsHexAddress(strings.TrimSpace(active.PoolManagerAddress)) || strings.TrimSpace(active.PoolAddress) == "" {
			return nil, nil
		}
		info, err := blockchain.GetV4PositionInfo(
			common.HexToAddress(managerAddress),
			common.HexToAddress(strings.TrimSpace(active.PoolManagerAddress)),
			strings.TrimSpace(active.PoolAddress),
			tokenID,
		)
		if err != nil || info == nil || info.Liquidity == nil {
			return nil, err
		}
		return new(big.Int).Set(info.Liquidity), nil
	default:
		return nil, nil
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
	Protocol          string  `json:"protocol"`
	NftTokenID        uint64  `json:"nft_token_id"`
	PositionAmountUSD float64 `json:"position_amount_usd"`
	FeeUSD            *string `json:"fee_usd"`
	FeeStatus         string  `json:"fee_status"`
	FeeUpdatedAt      *time.Time
}

func (r *Repository) GetPositionOpenAmountsUSD(ctx context.Context, positions []models.SmartMoneyLPPosition) (map[string]PositionOpenAmountRow, error) {
	out := make(map[string]PositionOpenAmountRow)
	if len(positions) == 0 {
		return out, nil
	}

	chainSeen := make(map[int]struct{}, len(positions))
	protocolSeen := make(map[string]struct{}, len(positions))
	nftSeen := make(map[uint64]struct{}, len(positions))

	chainIDs := make([]int, 0, len(positions))
	protocols := make([]string, 0, len(positions))
	nftIDs := make([]uint64, 0, len(positions))

	for _, pos := range positions {
		if pos.NftTokenID == 0 {
			continue
		}
		if _, ok := chainSeen[pos.ChainID]; !ok {
			chainSeen[pos.ChainID] = struct{}{}
			chainIDs = append(chainIDs, pos.ChainID)
		}
		protocol := strings.TrimSpace(pos.Protocol)
		if protocol == "" {
			continue
		}
		if _, ok := protocolSeen[protocol]; !ok {
			protocolSeen[protocol] = struct{}{}
			protocols = append(protocols, protocol)
		}
		if _, ok := nftSeen[pos.NftTokenID]; !ok {
			nftSeen[pos.NftTokenID] = struct{}{}
			nftIDs = append(nftIDs, pos.NftTokenID)
		}
	}

	if len(chainIDs) == 0 || len(protocols) == 0 || len(nftIDs) == 0 {
		return out, nil
	}

	var rows []PositionOpenAmountRow
	err := database.DB.WithContext(ctx).
		Table("sm_lp_positions p").
		Select(`
			p.chain_id,
			p.protocol,
			p.nft_token_id,
			MAX(COALESCE(ap.net_total_usd, evt_net.net_amount_usd, 0)) AS position_amount_usd,
			MAX(ap.fee_usd) AS fee_usd,
			MAX(ap.fee_status) AS fee_status,
			MAX(ap.fee_updated_at) AS fee_updated_at
		`).
		Joins(smartMoneyActivePositionJoinSQL("p")).
		Joins(`
			LEFT JOIN (
				SELECT
					chain_id,
					protocol,
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
				GROUP BY chain_id, protocol, nft_token_id
			) evt_net
				ON evt_net.chain_id = p.chain_id
				AND evt_net.protocol = p.protocol
				AND evt_net.nft_token_id = p.nft_token_id
		`).
		Where("p.chain_id IN ? AND p.protocol IN ? AND p.nft_token_id IN ?", chainIDs, protocols, nftIDs).
		Group("p.chain_id, p.protocol, p.nft_token_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		out[SmartMoneyPositionAmountKey(row.ChainID, row.Protocol, row.NftTokenID)] = row
	}

	return out, nil
}

func SmartMoneyPositionAmountKey(chainID int, protocol string, nftTokenID uint64) string {
	return strconv.Itoa(chainID) + ":" + strings.TrimSpace(protocol) + ":" + strconv.FormatUint(nftTokenID, 10)
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
`+smartMoneyActivePositionJoinSQL("p")+`
		LEFT JOIN (
				SELECT
					chain_id,
					protocol,
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
			GROUP BY chain_id, protocol, nft_token_id
		) evt_net
			ON evt_net.chain_id = p.chain_id
			AND evt_net.protocol = p.protocol
			AND evt_net.nft_token_id = p.nft_token_id
		WHERE p.status = 'open' AND p.opened_at >= ? AND (ap.id IS NULL OR ap.is_active = 1)
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

func (r *Repository) ListRecentOpenPositionRanges(ctx context.Context, poolAddresses []string, source string) ([]PoolPositionRangeRow, error) {
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
` + smartMoneyActivePositionJoinSQL("p")
	args := []interface{}{}
	if source != "" {
		query += `
		INNER JOIN monitored_wallets mw
			ON mw.address = p.wallet_address
			AND mw.chain_id = p.chain_id
			AND mw.source = ?
		`
		args = append(args, source)
	}
	query += `
		LEFT JOIN (
			SELECT
				chain_id,
				protocol,
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
			GROUP BY chain_id, protocol, nft_token_id
		) evt_net
			ON evt_net.chain_id = p.chain_id
			AND evt_net.protocol = p.protocol
			AND evt_net.nft_token_id = p.nft_token_id
		WHERE p.status = 'open' AND p.opened_at >= ? AND (ap.id IS NULL OR ap.is_active = 1)
	`
	args = append(args, recentCutoff)
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
	FeeDynamic             bool             `json:"fee_dynamic,omitempty"`
	FeePercentage          float64          `json:"fee_percentage,omitempty"`
	Protocol               string           `json:"protocol"`
	ChainID                int              `json:"chain_id"`
	OpenPositionCount      int              `json:"open_position_count"`
	WalletCount            int              `json:"wallet_count"`
	LatestEventAt          time.Time        `json:"latest_event_at"`
	TradingPair            string           `json:"trading_pair"`
	DisplayTokenAddress    string           `json:"display_token_address,omitempty"`
	DisplayTokenSymbol     string           `json:"display_token_symbol,omitempty"`
	DisplayTokenLogoURL    string           `json:"display_token_logo_url,omitempty"`
	MarketCapUSD           float64          `json:"market_cap_usd,omitempty"`
	FDVUSD                 float64          `json:"fdv_usd,omitempty"`
	CurrentTokenFDVUSD     float64          `json:"current_token_fdv_usd,omitempty"`
	MarketCapTokenAddress  string           `json:"market_cap_token_address,omitempty"`
	MarketCapTokenSymbol   string           `json:"market_cap_token_symbol,omitempty"`
	MarketCapProvider      string           `json:"market_cap_provider,omitempty"`
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
	FeeDynamic                bool      `json:"fee_dynamic,omitempty"`
	FeePercentage             float64   `json:"fee_percentage,omitempty"`
	Protocol                  string    `json:"protocol"`
	ChainID                   int       `json:"chain_id"`
	OpenPositionCount         int       `json:"open_position_count"`
	WalletCount               int       `json:"wallet_count"`
	LatestEventAt             time.Time `json:"latest_event_at"`
	TradingPair               string    `json:"trading_pair"`
	DisplayTokenAddress       string    `json:"display_token_address,omitempty"`
	DisplayTokenSymbol        string    `json:"display_token_symbol,omitempty"`
	DisplayTokenLogoURL       string    `json:"display_token_logo_url,omitempty"`
	MarketCapUSD              float64   `json:"market_cap_usd,omitempty"`
	FDVUSD                    float64   `json:"fdv_usd,omitempty"`
	CurrentTokenFDVUSD        float64   `json:"current_token_fdv_usd,omitempty"`
	MarketCapTokenAddress     string    `json:"market_cap_token_address,omitempty"`
	MarketCapTokenSymbol      string    `json:"market_cap_token_symbol,omitempty"`
	MarketCapProvider         string    `json:"market_cap_provider,omitempty"`
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
					FeeDynamic:          IsDynamicFeeTier(pos.Protocol, pos.FeeTier),
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
		if IsDynamicFeeTier(pos.Protocol, pos.FeeTier) {
			agg.row.FeeDynamic = true
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

func (r *Repository) ListPoolsWithPositions(ctx context.Context, source string) ([]PoolAggRow, error) {
	var rows []PoolAggRow
	cutoff := smartMoneyDisplayRecentCutoff()
	query := `
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
` + smartMoneyActivePositionJoinSQL("p")
	args := []interface{}{}
	if source != "" {
		query += `
		INNER JOIN monitored_wallets mw
			ON mw.address = p.wallet_address
			AND mw.chain_id = p.chain_id
			AND mw.source = ?
		`
		args = append(args, source)
	}
	query += `
		LEFT JOIN (
			SELECT
				chain_id,
				protocol,
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
			GROUP BY chain_id, protocol, nft_token_id
		) evt_net
			ON evt_net.chain_id = p.chain_id
			AND evt_net.protocol = p.protocol
			AND evt_net.nft_token_id = p.nft_token_id
		WHERE p.status = 'open' AND p.opened_at >= ? AND (ap.id IS NULL OR ap.is_active = 1)
		GROUP BY p.pool_address, p.token0_symbol, p.token1_symbol, p.token0_address, p.token1_address, p.fee_tier, p.protocol, p.chain_id
		ORDER BY total_position_amount_usd DESC, latest_event_at DESC
	`
	args = append(args, cutoff)
	err := database.DB.WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].FeeDynamic = IsDynamicFeeTier(rows[i].Protocol, rows[i].FeeTier)
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
	FeeDynamic             bool             `json:"fee_dynamic,omitempty"`
	FeePercentage          float64          `json:"fee_percentage,omitempty"`
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
	MarketCapUSD           float64          `json:"market_cap_usd,omitempty"`
	FDVUSD                 float64          `json:"fdv_usd,omitempty"`
	CurrentTokenFDVUSD     float64          `json:"current_token_fdv_usd,omitempty"`
	MarketCapTokenAddress  string           `json:"market_cap_token_address,omitempty"`
	MarketCapTokenSymbol   string           `json:"market_cap_token_symbol,omitempty"`
	MarketCapProvider      string           `json:"market_cap_provider,omitempty"`
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
			SUM(CASE WHEN p.status='open' AND p.opened_at >= ? AND (ap.id IS NULL OR ap.is_active = 1) THEN 1 ELSE 0 END) AS open_position_count,
			COUNT(DISTINCT CASE WHEN p.status='open' AND p.opened_at >= ? AND (ap.id IS NULL OR ap.is_active = 1) THEN p.wallet_address END) AS wallet_count,
			SUM(CASE WHEN p.status='closed' AND p.closed_at >= ? THEN 1 ELSE 0 END) AS closed_today_count,
			COALESCE(SUM(CASE WHEN p.status='open' AND p.opened_at >= ? AND (ap.id IS NULL OR ap.is_active = 1) THEN COALESCE(ap.net_total_usd, evt_net.net_amount_usd, 0) ELSE 0 END), 0) AS total_position_amount_usd
		FROM sm_lp_positions p
`+smartMoneyActivePositionJoinSQL("p")+`
		LEFT JOIN (
			SELECT
				chain_id,
				protocol,
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
			GROUP BY chain_id, protocol, nft_token_id
		) evt_net
			ON evt_net.chain_id = p.chain_id
			AND evt_net.protocol = p.protocol
			AND evt_net.nft_token_id = p.nft_token_id
		WHERE p.pool_address = ?
		GROUP BY p.pool_address
	`, recentCutoff, recentCutoff, today, recentCutoff, poolAddress).Scan(&stats).Error
	if err != nil {
		return nil, err
	}
	stats.FeeDynamic = IsDynamicFeeTier(stats.Protocol, stats.FeeTier)
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
	database.DB.WithContext(ctx).
		Table("sm_lp_positions p").
		Joins(smartMoneyActivePositionJoinSQL("p")).
		Where("p.status = 'open' AND p.opened_at >= ? AND (ap.id IS NULL OR ap.is_active = 1)", recentCutoff).
		Count(&openCount)
	stats.OpenPositionCount = int(openCount)

	var closedToday int64
	database.DB.WithContext(ctx).Model(&models.SmartMoneyLPPosition{}).
		Where("status = 'closed' AND closed_at >= ?", today).Count(&closedToday)
	stats.ClosedTodayCount = int(closedToday)

	var poolCount int64
	database.DB.WithContext(ctx).
		Table("sm_lp_positions p").
		Joins(smartMoneyActivePositionJoinSQL("p")).
		Where("p.status = 'open' AND p.opened_at >= ? AND (ap.id IS NULL OR ap.is_active = 1)", recentCutoff).
		Distinct("p.pool_address").
		Count(&poolCount)
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

type WalletRef struct {
	Address string `json:"address"`
	ChainID int    `json:"chain_id"`
}

type ZombieWalletCandidate struct {
	Address             string     `json:"address"`
	Label               *string    `json:"label"`
	AvatarURL           *string    `json:"avatar_url"`
	Source              string     `json:"source"`
	SourceContract      *string    `json:"source_contract"`
	IsActive            bool       `json:"is_active"`
	ChainID             int        `json:"chain_id"`
	LastActiveAt        *time.Time `json:"last_active_at"`
	WindowAddCount      int        `json:"window_add_count"`
	WindowRemoveCount   int        `json:"window_remove_count"`
	TotalEventCount     int        `json:"total_event_count"`
	PositionCount       int        `json:"position_count"`
	ActivePositionCount int        `json:"active_position_count"`
	SnapshotCount       int        `json:"snapshot_count"`
	TransferEventCount  int        `json:"transfer_event_count"`
	DailyStatCount      int        `json:"daily_stat_count"`
	LiveStateCount      int        `json:"live_state_count"`
	WatchWalletCount    int        `json:"watch_wallet_count"`
	FollowRefCount      int        `json:"follow_ref_count"`
	CreatedAt           time.Time  `json:"created_at"`
}

func (r *Repository) ListWalletsWithStats(ctx context.Context, page, size int, keyword, source string, activeOnly *bool) ([]WalletStatsRow, int64, error) {
	countDB := database.DB.WithContext(ctx).Model(&models.MonitoredWallet{})
	if keyword != "" {
		kw := "%" + strings.TrimSpace(keyword) + "%"
		countDB = countDB.Where("address LIKE ? OR label LIKE ?", kw, kw)
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
			protocol,
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
		Group("chain_id, protocol, nft_token_id")

	walletStatsSubQuery := database.DB.WithContext(ctx).
		Table("sm_lp_positions p").
		Select(`
			p.wallet_address,
			p.chain_id,
			COUNT(*) AS open_position_count,
			COUNT(DISTINCT p.pool_address) AS active_pool_count,
			COALESCE(SUM(COALESCE(ap.net_total_usd, evt_net.net_amount_usd, 0)), 0) AS total_position_amount_usd
		`).
		Joins(smartMoneyActivePositionJoinSQL("p")).
		Joins("LEFT JOIN (?) evt_net ON evt_net.chain_id = p.chain_id AND evt_net.protocol = p.protocol AND evt_net.nft_token_id = p.nft_token_id", eventNetSubQuery).
		Where("p.status = ? AND p.opened_at >= ? AND (ap.id IS NULL OR ap.is_active = 1)", "open", recentCutoff).
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
		kw := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("w.address LIKE ? OR w.label LIKE ?", kw, kw)
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

func (r *Repository) ListZombieWalletCandidates(ctx context.Context, inactiveDays int, chainID int) ([]ZombieWalletCandidate, error) {
	if inactiveDays <= 0 {
		inactiveDays = 30
	}
	if chainID <= 0 {
		chainID = 56
	}
	cutoff := time.Now().AddDate(0, 0, -inactiveDays)
	chain := chainSlugForID(chainID)

	eventSubQuery := database.DB.WithContext(ctx).
		Table("sm_lp_events").
		Select(`
			wallet_address,
			chain_id,
			MAX(tx_timestamp) AS last_active_at,
			SUM(CASE WHEN tx_timestamp >= ? AND event_type = 'add' THEN 1 ELSE 0 END) AS window_add_count,
			SUM(CASE WHEN tx_timestamp >= ? AND event_type = 'remove' THEN 1 ELSE 0 END) AS window_remove_count,
			COUNT(*) AS total_event_count
		`, cutoff, cutoff).
		Where("event_type IN ?", []string{"add", "remove"}).
		Group("wallet_address, chain_id")

	positionSubQuery := database.DB.WithContext(ctx).
		Table("sm_lp_positions").
		Select("wallet_address, chain_id, COUNT(*) AS position_count").
		Group("wallet_address, chain_id")

	activePositionSubQuery := database.DB.WithContext(ctx).
		Table("sm_lp_active_positions").
		Select("wallet_address, chain_id, COUNT(*) AS active_position_count").
		Group("wallet_address, chain_id")

	snapshotSubQuery := database.DB.WithContext(ctx).
		Table("sm_wallet_midnight_snapshots").
		Select("wallet_address, chain_id, COUNT(*) AS snapshot_count").
		Group("wallet_address, chain_id")

	transferSubQuery := database.DB.WithContext(ctx).
		Table("sm_wallet_transfer_events").
		Select("wallet_address, chain_id, COUNT(*) AS transfer_event_count").
		Group("wallet_address, chain_id")

	dailyStatSubQuery := database.DB.WithContext(ctx).
		Table("sm_lp_daily_stats").
		Select("wallet_address, chain_id, COUNT(*) AS daily_stat_count").
		Group("wallet_address, chain_id")

	liveStateSubQuery := database.DB.WithContext(ctx).
		Table("sm_wallet_live_states").
		Select("wallet_address, chain_id, COUNT(*) AS live_state_count").
		Group("wallet_address, chain_id")

	watchWalletSubQuery := database.DB.WithContext(ctx).
		Table("smart_money_user_watch_wallets").
		Select("wallet_address, COUNT(*) AS watch_wallet_count").
		Where("chain = ?", chain).
		Group("wallet_address")

	followRefSubQuery := database.DB.WithContext(ctx).
		Table("smart_money_follow_configs").
		Select("target_wallet_address, COUNT(*) AS follow_ref_count").
		Where("chain_id = ?", chainID).
		Group("target_wallet_address")

	rows := make([]ZombieWalletCandidate, 0)
	err := database.DB.WithContext(ctx).
		Table("monitored_wallets w").
		Select(`
			w.address,
			w.label,
			w.avatar_url,
			w.source,
			w.source_contract,
			w.is_active,
			w.chain_id,
			ev.last_active_at,
			COALESCE(ev.window_add_count, 0) AS window_add_count,
			COALESCE(ev.window_remove_count, 0) AS window_remove_count,
			COALESCE(ev.total_event_count, 0) AS total_event_count,
			COALESCE(pos.position_count, 0) AS position_count,
			COALESCE(ap.active_position_count, 0) AS active_position_count,
			COALESCE(snap.snapshot_count, 0) AS snapshot_count,
			COALESCE(tx.transfer_event_count, 0) AS transfer_event_count,
			COALESCE(stat.daily_stat_count, 0) AS daily_stat_count,
			COALESCE(live.live_state_count, 0) AS live_state_count,
			COALESCE(watch.watch_wallet_count, 0) AS watch_wallet_count,
			COALESCE(follow.follow_ref_count, 0) AS follow_ref_count,
			w.created_at
		`).
		Joins("LEFT JOIN (?) ev ON ev.wallet_address = w.address AND ev.chain_id = w.chain_id", eventSubQuery).
		Joins("LEFT JOIN (?) pos ON pos.wallet_address = w.address AND pos.chain_id = w.chain_id", positionSubQuery).
		Joins("LEFT JOIN (?) ap ON ap.wallet_address = w.address AND ap.chain_id = w.chain_id", activePositionSubQuery).
		Joins("LEFT JOIN (?) snap ON snap.wallet_address = w.address AND snap.chain_id = w.chain_id", snapshotSubQuery).
		Joins("LEFT JOIN (?) tx ON tx.wallet_address = w.address AND tx.chain_id = w.chain_id", transferSubQuery).
		Joins("LEFT JOIN (?) stat ON stat.wallet_address = w.address AND stat.chain_id = w.chain_id", dailyStatSubQuery).
		Joins("LEFT JOIN (?) live ON live.wallet_address = w.address AND live.chain_id = w.chain_id", liveStateSubQuery).
		Joins("LEFT JOIN (?) watch ON watch.wallet_address = w.address", watchWalletSubQuery).
		Joins("LEFT JOIN (?) follow ON follow.target_wallet_address = w.address", followRefSubQuery).
		Where("w.is_active = ? AND w.chain_id = ?", true, chainID).
		Where("ev.last_active_at IS NULL OR ev.last_active_at < ?", cutoff).
		Order("ev.last_active_at ASC").
		Order("w.created_at ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) DeleteMonitoredWalletsWithHistory(ctx context.Context, wallets []WalletRef) (int64, error) {
	wallets = normalizeWalletRefs(wallets)
	if len(wallets) == 0 {
		return 0, nil
	}

	var deleted int64
	err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := cleanupFollowReferencesForWallets(tx, wallets); err != nil {
			return err
		}
		for _, wallet := range wallets {
			removed, err := deleteMonitoredWalletHistory(tx, wallet)
			if err != nil {
				return err
			}
			deleted += removed
		}
		return nil
	})
	return deleted, err
}

func normalizeWalletRefs(wallets []WalletRef) []WalletRef {
	if len(wallets) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(wallets))
	out := make([]WalletRef, 0, len(wallets))
	for _, wallet := range wallets {
		address := strings.ToLower(strings.TrimSpace(wallet.Address))
		if address == "" {
			continue
		}
		chainID := wallet.ChainID
		if chainID <= 0 {
			chainID = 56
		}
		key := strconv.Itoa(chainID) + "|" + address
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, WalletRef{Address: address, ChainID: chainID})
	}
	return out
}

func isRepositoryEVMAddress(addr string) bool {
	addr = strings.TrimSpace(addr)
	if len(addr) != 42 {
		return false
	}
	if !strings.HasPrefix(addr, "0x") && !strings.HasPrefix(addr, "0X") {
		return false
	}
	for _, c := range addr[2:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func deleteMonitoredWalletHistory(tx *gorm.DB, wallet WalletRef) (int64, error) {
	chain := chainSlugForID(wallet.ChainID)
	if err := tx.Where(`
		chain = ?
		AND EXISTS (
			SELECT 1
			FROM sm_lp_events e
			WHERE e.wallet_address = ?
			  AND e.chain_id = ?
			  AND e.tx_hash = smart_money_watch_open_alert_receipts.tx_hash
			  AND e.log_index = smart_money_watch_open_alert_receipts.log_index
		)
	`, chain, wallet.Address, wallet.ChainID).
		Delete(&models.SmartMoneyWatchOpenAlertReceipt{}).Error; err != nil {
		return 0, err
	}

	if err := tx.Where("chain = ? AND wallet_address = ?", chain, wallet.Address).
		Delete(&models.SmartMoneyWatchWallet{}).Error; err != nil {
		return 0, err
	}

	deletes := []struct {
		model any
	}{
		{model: &models.SmartMoneyLPDailyStat{}},
		{model: &models.SmartMoneyWalletLiveState{}},
		{model: &models.SmartMoneyWalletDailySnapshot{}},
		{model: &models.SmartMoneyWalletMidnightSnapshot{}},
		{model: &models.SmartMoneyWalletTransferEvent{}},
		{model: &models.SmartMoneyActivePosition{}},
		{model: &models.SmartMoneyLPPosition{}},
		{model: &models.SmartMoneyLPEvent{}},
	}
	for _, item := range deletes {
		if err := tx.Where("wallet_address = ? AND chain_id = ?", wallet.Address, wallet.ChainID).
			Delete(item.model).Error; err != nil {
			return 0, err
		}
	}

	result := tx.Where("address = ? AND chain_id = ?", wallet.Address, wallet.ChainID).
		Delete(&models.MonitoredWallet{})
	return result.RowsAffected, result.Error
}

func cleanupFollowReferencesForWallets(tx *gorm.DB, wallets []WalletRef) error {
	if len(wallets) == 0 {
		return nil
	}
	byChain := make(map[int]map[string]struct{})
	for _, wallet := range wallets {
		if _, ok := byChain[wallet.ChainID]; !ok {
			byChain[wallet.ChainID] = make(map[string]struct{})
		}
		byChain[wallet.ChainID][wallet.Address] = struct{}{}
	}

	for chainID, addresses := range byChain {
		list := make([]string, 0, len(addresses))
		for address := range addresses {
			list = append(list, address)
		}
		sort.Strings(list)

		if err := tx.Where("chain_id = ? AND target_wallet_address IN ?", chainID, list).
			Delete(&models.SmartMoneyFollowJob{}).Error; err != nil {
			return err
		}
		if err := tx.Where("chain_id = ? AND target_wallet_address IN ?", chainID, list).
			Delete(&models.SmartMoneyFollowTask{}).Error; err != nil {
			return err
		}

		var configs []models.SmartMoneyFollowConfig
		if err := tx.Where("chain_id = ?", chainID).Find(&configs).Error; err != nil {
			return err
		}
		for _, cfg := range configs {
			targets := normalizeFollowConfigWallets(cfg.TargetWalletAddress, []string(cfg.TargetWallets))
			if len(targets) == 0 {
				continue
			}
			filtered := make([]string, 0, len(targets))
			changed := false
			for _, target := range targets {
				if _, remove := addresses[target]; remove {
					changed = true
					continue
				}
				filtered = append(filtered, target)
			}
			if !changed {
				continue
			}
			if len(filtered) == 0 {
				if err := tx.Where("config_id = ?", cfg.ID).Delete(&models.SmartMoneyFollowJob{}).Error; err != nil {
					return err
				}
				if err := tx.Where("config_id = ?", cfg.ID).Delete(&models.SmartMoneyFollowTask{}).Error; err != nil {
					return err
				}
				if err := tx.Delete(&models.SmartMoneyFollowConfig{}, cfg.ID).Error; err != nil {
					return err
				}
				continue
			}
			if err := tx.Model(&models.SmartMoneyFollowConfig{}).
				Where("id = ?", cfg.ID).
				Updates(map[string]any{
					"target_wallet_address":   filtered[0],
					"target_wallet_addresses": models.StringArray(filtered),
				}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeFollowConfigWallets(primary string, extra []string) []string {
	seen := make(map[string]struct{}, len(extra)+1)
	out := make([]string, 0, len(extra)+1)
	for _, value := range append([]string{primary}, extra...) {
		address := strings.ToLower(strings.TrimSpace(value))
		if address == "" {
			continue
		}
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		out = append(out, address)
	}
	return out
}

func chainSlugForID(chainID int) string {
	switch chainID {
	case 8453:
		return "base"
	default:
		return "bsc"
	}
}
