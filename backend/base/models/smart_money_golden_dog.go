package models

import (
	"time"

	"gorm.io/gorm"
)

type SmartMoneyGoldenDogConfig struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"not null;index;uniqueIndex:uniq_smgd_user_chain" json:"user_id"`

	Chain string `gorm:"size:10;not null;default:'bsc';uniqueIndex:uniq_smgd_user_chain" json:"chain"`

	Enabled bool `gorm:"default:false" json:"enabled"`

	MinWallets      int `gorm:"not null;default:3" json:"min_wallets"`
	WindowMinutes   int `gorm:"not null;default:10" json:"window_minutes"`
	CooldownMinutes int `gorm:"not null;default:30" json:"cooldown_minutes"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (SmartMoneyGoldenDogConfig) TableName() string {
	return "smart_money_golden_dog_configs"
}

type SmartMoneyGoldenDogAlertState struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"not null;index;uniqueIndex:uniq_smgd_alert_user_chain_pool" json:"user_id"`

	Chain       string `gorm:"size:10;not null;default:'bsc';uniqueIndex:uniq_smgd_alert_user_chain_pool" json:"chain"`
	PoolVersion string `gorm:"size:10;not null;uniqueIndex:uniq_smgd_alert_user_chain_pool" json:"pool_version"`
	PoolID      string `gorm:"size:66;not null;uniqueIndex:uniq_smgd_alert_user_chain_pool" json:"pool_id"`

	LastNotifiedAt *time.Time `gorm:"index" json:"last_notified_at,omitempty"`
	LastWallets    int        `gorm:"not null;default:0" json:"last_wallets"`
	LastPair       string     `gorm:"size:50;default:''" json:"last_pair,omitempty"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (SmartMoneyGoldenDogAlertState) TableName() string {
	return "smart_money_golden_dog_alert_states"
}
