package smart_money_golden_dog

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

var freshPoolSelectColumns = []string{
	"address",
	"name",
	"dex_id",
	"protocol_version",
	"token0_symbol",
	"token1_symbol",
	"total_fees",
	"transaction_count",
	"total_volume",
	"current_pool_value",
	"poolm_fee_rate",
	"active_liquidity_usd",
	"active_liquidity_ratio",
	"updated_at",
	"chain",
	"source_requested_chain",
}

func NewRepository() *Repository {
	return &Repository{}
}

func (r *Repository) GetOrCreateConfig(ctx context.Context, userID uint, chain string) (*models.SmartMoneyGoldenDogConfig, error) {
	chain = normalizeChain(chain)

	var cfg models.SmartMoneyGoldenDogConfig
	err := database.DB.WithContext(ctx).
		Where("user_id = ? AND chain = ?", userID, chain).
		First(&cfg).Error
	if err == nil {
		return &cfg, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	cfg = models.SmartMoneyGoldenDogConfig{
		UserID:              userID,
		Chain:               chain,
		Enabled:             false,
		MinWallets:          DefaultMinWallets,
		WindowMinutes:       DefaultWindowMinutes,
		CooldownMinutes:     DefaultCooldownMinutes,
		WalletIntensity:     BarkIntensityRing,
		PoolEnabled:         false,
		PoolCooldownMinutes: DefaultCooldownMinutes,
		PoolIntensity:       BarkIntensityRing,
	}
	if err := database.DB.WithContext(ctx).Create(&cfg).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *Repository) UpdateConfig(ctx context.Context, userID uint, chain string, updates map[string]any) (*models.SmartMoneyGoldenDogConfig, error) {
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

func (r *Repository) ListEnabledConfigs(ctx context.Context) ([]models.SmartMoneyGoldenDogConfig, error) {
	var rows []models.SmartMoneyGoldenDogConfig
	err := database.DB.WithContext(ctx).
		Where("enabled = 1 OR pool_enabled = 1").
		Find(&rows).Error
	return rows, err
}

func (r *Repository) ListRecentAddEvents(ctx context.Context, chainID int, since time.Time) ([]models.SmartMoneyLPEvent, error) {
	var rows []models.SmartMoneyLPEvent
	err := database.DB.WithContext(ctx).
		Select([]string{
			"wallet_address",
			"chain_id",
			"token0_address",
			"token1_address",
			"token0_symbol",
			"token1_symbol",
			"tx_timestamp",
		}).
		Where("chain_id = ? AND event_type = ? AND tx_timestamp >= ?", chainID, "add", since).
		Order("tx_timestamp DESC").
		Find(&rows).Error
	return rows, err
}

func (r *Repository) ListFreshPools(ctx context.Context, chain string, since time.Time) ([]models.Pool, error) {
	chain = normalizeChain(chain)

	var rows []models.Pool
	err := database.DB.WithContext(ctx).
		Model(&models.Pool{}).
		Select(freshPoolSelectColumns).
		Where("(LOWER(chain) = ? OR LOWER(source_requested_chain) = ?) AND updated_at >= ?", chain, chain, since).
		Order("updated_at DESC").
		Find(&rows).Error
	return rows, err
}

func (r *Repository) GetAlertState(ctx context.Context, userID uint, chain string, pairKey string) (*models.SmartMoneyGoldenDogAlertState, error) {
	var state models.SmartMoneyGoldenDogAlertState
	err := database.DB.WithContext(ctx).
		Where("user_id = ? AND chain = ? AND pair_key = ?", userID, normalizeChain(chain), strings.TrimSpace(pairKey)).
		First(&state).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *Repository) UpsertAlertState(ctx context.Context, state *models.SmartMoneyGoldenDogAlertState) error {
	state.Chain = normalizeChain(state.Chain)
	state.PairKey = strings.TrimSpace(state.PairKey)
	state.PairLabel = strings.TrimSpace(state.PairLabel)
	return database.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "user_id"},
				{Name: "chain"},
				{Name: "pair_key"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"pair_label":       state.PairLabel,
				"last_wallets":     state.LastWallets,
				"last_notified_at": state.LastNotifiedAt,
				"updated_at":       time.Now(),
			}),
		}).
		Create(state).Error
}
