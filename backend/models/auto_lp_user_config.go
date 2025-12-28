package models

import (
	"time"

	"gorm.io/gorm"
)

// AutoLPUserConfig stores per-user AutoLP settings.
type AutoLPUserConfig struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"not null;uniqueIndex" json:"user_id"` // One config per user

	Enabled bool `gorm:"default:false" json:"enabled"`

	// TotalAmountUSDT is the total USDT budget for AutoLP.
	// Each new AutoLP position uses: TotalAmountUSDT / MaxActiveTasks
	TotalAmountUSDT float64 `gorm:"type:decimal(20,8);default:0" json:"total_amount_usdt"`

	// Stop conditions (0 = disabled). They are based on cumulative realized PnL (USDT).
	StopLossUSDT   float64 `gorm:"type:decimal(20,8);default:0" json:"stop_loss_usdt"`   // disable AutoLP when profit <= -StopLossUSDT
	TakeProfitUSDT float64 `gorm:"type:decimal(20,8);default:0" json:"take_profit_usdt"` // disable AutoLP when profit >= TakeProfitUSDT
	MaxActiveTasks int     `gorm:"default:1" json:"max_active_tasks"`                    // max concurrent AutoLP tasks

	// LastEnabledAt marks the start time of the current/last AutoLP run.
	LastEnabledAt  *time.Time `json:"last_enabled_at"`
	LastDisabledAt *time.Time `json:"last_disabled_at"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships (without foreign key constraints in database)
	User User `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:NO ACTION,OnDelete:NO ACTION" json:"user,omitempty"`
}

func (AutoLPUserConfig) TableName() string {
	return "auto_lp_user_configs"
}
