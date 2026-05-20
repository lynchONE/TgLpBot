package models

import (
	"time"

	"gorm.io/gorm"
)

// AuthCode represents an authorization code that can be redeemed by users.
type AuthCode struct {
	ID uint `gorm:"primaryKey" json:"id"`

	Code string `gorm:"size:64;uniqueIndex;not null" json:"code"`

	CreatedByUserID uint   `gorm:"not null;index" json:"created_by_user_id"`
	Note            string `gorm:"size:255" json:"note"`

	ActiveFrom *time.Time `gorm:"index" json:"active_from"`
	ActiveTo   *time.Time `gorm:"index" json:"active_to"`

	MaxRedemptions int `gorm:"default:1" json:"max_redemptions"`
	RedeemedCount  int `gorm:"default:0" json:"redeemed_count"`

	MaxWallets     int    `gorm:"default:1" json:"max_wallets"`
	MaxActiveTasks int    `gorm:"default:1" json:"max_active_tasks"`
	MiniAppEnabled bool   `gorm:"default:false" json:"mini_app_enabled"`
	EnabledModules string `gorm:"type:text" json:"-"`

	DisabledAt *time.Time     `gorm:"index" json:"disabled_at"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (AuthCode) TableName() string {
	return "auth_codes"
}
