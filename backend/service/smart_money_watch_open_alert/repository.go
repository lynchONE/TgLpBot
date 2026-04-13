package smart_money_watch_open_alert

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{}

func NewRepository() *Repository {
	return &Repository{}
}

func normalizeChain(chain string) string {
	normalized := config.NormalizeChain(chain)
	if normalized == "" {
		return "bsc"
	}
	return normalized
}

func chainIDFor(chain string) int {
	switch normalizeChain(chain) {
	case "base":
		return 8453
	default:
		return 56
	}
}

func normalizeWalletAddress(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != 42 || !strings.HasPrefix(strings.ToLower(value), "0x") {
		return ""
	}
	for _, ch := range value[2:] {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return ""
		}
	}
	return strings.ToLower(value)
}

func normalizeWalletAddresses(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	uniq := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		address := normalizeWalletAddress(value)
		if address == "" {
			continue
		}
		if _, ok := uniq[address]; ok {
			continue
		}
		uniq[address] = struct{}{}
		out = append(out, address)
	}
	sort.Strings(out)
	return out
}

func (r *Repository) ListWatchWallets(ctx context.Context, userID uint, chain string) ([]models.SmartMoneyWatchWallet, error) {
	var rows []models.SmartMoneyWatchWallet
	err := database.DB.WithContext(ctx).
		Where("user_id = ? AND chain = ?", userID, normalizeChain(chain)).
		Order("created_at ASC, wallet_address ASC").
		Find(&rows).Error
	return rows, err
}

func (r *Repository) GetWatchWallet(ctx context.Context, userID uint, chain string, walletAddress string) (*models.SmartMoneyWatchWallet, error) {
	var row models.SmartMoneyWatchWallet
	err := database.DB.WithContext(ctx).
		Where("user_id = ? AND chain = ? AND wallet_address = ?", userID, normalizeChain(chain), normalizeWalletAddress(walletAddress)).
		First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) SetWatchWallet(ctx context.Context, userID uint, chain string, walletAddress string, watched bool) error {
	chain = normalizeChain(chain)
	walletAddress = normalizeWalletAddress(walletAddress)
	if walletAddress == "" {
		return gorm.ErrInvalidData
	}
	if watched {
		return database.DB.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(&models.SmartMoneyWatchWallet{
				UserID:        userID,
				Chain:         chain,
				WalletAddress: walletAddress,
			}).Error
	}

	return database.DB.WithContext(ctx).
		Where("user_id = ? AND chain = ? AND wallet_address = ?", userID, chain, walletAddress).
		Delete(&models.SmartMoneyWatchWallet{}).Error
}

func (r *Repository) ReplaceWatchWallets(ctx context.Context, userID uint, chain string, walletAddresses []string) error {
	chain = normalizeChain(chain)
	walletAddresses = normalizeWalletAddresses(walletAddresses)

	return database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(walletAddresses) == 0 {
			return tx.Where("user_id = ? AND chain = ?", userID, chain).
				Delete(&models.SmartMoneyWatchWallet{}).Error
		}

		if err := tx.Where("user_id = ? AND chain = ? AND wallet_address NOT IN ?", userID, chain, walletAddresses).
			Delete(&models.SmartMoneyWatchWallet{}).Error; err != nil {
			return err
		}

		rows := make([]models.SmartMoneyWatchWallet, 0, len(walletAddresses))
		for _, walletAddress := range walletAddresses {
			rows = append(rows, models.SmartMoneyWatchWallet{
				UserID:        userID,
				Chain:         chain,
				WalletAddress: walletAddress,
			})
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
	})
}

func (r *Repository) GetOrCreateConfig(ctx context.Context, userID uint, chain string) (*models.SmartMoneyWatchOpenAlertConfig, error) {
	chain = normalizeChain(chain)

	var cfg models.SmartMoneyWatchOpenAlertConfig
	err := database.DB.WithContext(ctx).
		Where("user_id = ? AND chain = ?", userID, chain).
		First(&cfg).Error
	if err == nil {
		return &cfg, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	cfg = models.SmartMoneyWatchOpenAlertConfig{
		UserID:       userID,
		Chain:        chain,
		Enabled:      false,
		BarkEnabled:  false,
		SoundEnabled: false,
	}
	if err := database.DB.WithContext(ctx).Create(&cfg).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *Repository) UpdateConfig(ctx context.Context, userID uint, chain string, updates map[string]any) (*models.SmartMoneyWatchOpenAlertConfig, error) {
	cfg, err := r.GetOrCreateConfig(ctx, userID, chain)
	if err != nil {
		return nil, err
	}
	if len(updates) == 0 {
		return cfg, nil
	}
	if err := database.DB.WithContext(ctx).Model(cfg).Updates(updates).Error; err != nil {
		return nil, err
	}
	return r.GetOrCreateConfig(ctx, userID, chain)
}

func (r *Repository) ListWatcherUserIDs(ctx context.Context, chain string, walletAddress string) ([]uint, error) {
	var userIDs []uint
	err := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyWatchWallet{}).
		Distinct("user_id").
		Where("chain = ? AND wallet_address = ?", normalizeChain(chain), normalizeWalletAddress(walletAddress)).
		Order("user_id ASC").
		Pluck("user_id", &userIDs).Error
	return userIDs, err
}

func (r *Repository) ClaimReceipt(ctx context.Context, userID uint, chain string, txHash string, logIndex int) (bool, error) {
	row := models.SmartMoneyWatchOpenAlertReceipt{
		UserID:   userID,
		Chain:    normalizeChain(chain),
		TxHash:   strings.ToLower(strings.TrimSpace(txHash)),
		LogIndex: logIndex,
	}
	result := database.DB.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *Repository) DeleteReceipt(ctx context.Context, userID uint, chain string, txHash string, logIndex int) error {
	return database.DB.WithContext(ctx).
		Where("user_id = ? AND chain = ? AND tx_hash = ? AND log_index = ?",
			userID,
			normalizeChain(chain),
			strings.ToLower(strings.TrimSpace(txHash)),
			logIndex,
		).
		Delete(&models.SmartMoneyWatchOpenAlertReceipt{}).Error
}

func (r *Repository) CleanupReceiptsBefore(ctx context.Context, before time.Time) error {
	return database.DB.WithContext(ctx).
		Where("created_at < ?", before).
		Delete(&models.SmartMoneyWatchOpenAlertReceipt{}).Error
}
