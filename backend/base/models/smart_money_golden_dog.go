package models

import (
	"time"

	"gorm.io/gorm"
)

type SmartMoneyGoldenDogConfig struct {
	ID                          uint           `gorm:"primaryKey" json:"id"`
	UserID                      uint           `gorm:"not null;uniqueIndex:uq_sm_golden_dog_user_chain" json:"user_id"`
	Chain                       string         `gorm:"size:16;not null;default:'bsc';uniqueIndex:uq_sm_golden_dog_user_chain" json:"chain"`
	Enabled                     bool           `gorm:"not null;default:false" json:"enabled"`
	MinWallets                  int            `gorm:"not null;default:3" json:"min_wallets"`
	WindowMinutes               int            `gorm:"not null;default:10" json:"window_minutes"`
	CooldownMinutes             int            `gorm:"not null;default:30" json:"cooldown_minutes"`
	WalletIntensity             string         `gorm:"size:32;not null;default:'ring'" json:"wallet_intensity"`
	PoolEnabled                 bool           `gorm:"not null;default:false" json:"pool_enabled"`
	PoolCooldownMinutes         int            `gorm:"not null;default:30" json:"pool_cooldown_minutes"`
	PoolMinTotalFees            float64        `gorm:"type:double;not null;default:0" json:"pool_min_total_fees"`
	PoolMinTransactionCount     int            `gorm:"not null;default:0" json:"pool_min_transaction_count"`
	PoolMinTVL                  float64        `gorm:"type:double;not null;default:0" json:"pool_min_tvl"`
	PoolMinVolume               float64        `gorm:"type:double;not null;default:0" json:"pool_min_volume"`
	PoolMinFeeRate              float64        `gorm:"type:double;not null;default:0" json:"pool_min_fee_rate"`
	PoolMinActiveLiquidityRatio float64        `gorm:"type:double;not null;default:0" json:"pool_min_active_liquidity_ratio"`
	PoolIntensity               string         `gorm:"size:32;not null;default:'ring'" json:"pool_intensity"`
	CreatedAt                   time.Time      `json:"created_at"`
	UpdatedAt                   time.Time      `json:"updated_at"`
	DeletedAt                   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (SmartMoneyGoldenDogConfig) TableName() string {
	return "smart_money_golden_dog_configs"
}

type SmartMoneyGoldenDogAlertState struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"not null;uniqueIndex:uq_sm_golden_dog_alert_user_pair" json:"user_id"`
	Chain          string    `gorm:"size:16;not null;default:'bsc';uniqueIndex:uq_sm_golden_dog_alert_user_pair" json:"chain"`
	PairKey        string    `gorm:"size:255;not null;uniqueIndex:uq_sm_golden_dog_alert_user_pair" json:"pair_key"`
	PairLabel      string    `gorm:"size:128;not null;default:''" json:"pair_label"`
	LastWallets    int       `gorm:"not null;default:0" json:"last_wallets"`
	LastNotifiedAt time.Time `gorm:"not null" json:"last_notified_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (SmartMoneyGoldenDogAlertState) TableName() string {
	return "smart_money_golden_dog_alert_states"
}
