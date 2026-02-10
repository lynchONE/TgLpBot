package models

import (
	"time"

	"gorm.io/gorm"
)

type SmartMoneyFollowConfig struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"not null;index;uniqueIndex:uniq_smf_user_chain_wallet" json:"user_id"`

	Chain         string `gorm:"size:10;not null;default:'bsc';uniqueIndex:uniq_smf_user_chain_wallet" json:"chain"`
	WalletAddress string `gorm:"size:42;not null;uniqueIndex:uniq_smf_user_chain_wallet" json:"wallet_address"`

	Enabled bool `gorm:"default:false" json:"enabled"`

	// Budget settings (USDT)
	MaxTotalAmountUSDT float64 `gorm:"type:decimal(20,8);default:0" json:"max_total_amount_usdt"`
	PerTradeAmountUSDT float64 `gorm:"type:decimal(20,8);default:0" json:"per_trade_amount_usdt"`

	// Random delay range in seconds (0..60).
	DelayMinSeconds int `gorm:"default:0" json:"delay_min_seconds"`
	DelayMaxSeconds int `gorm:"default:60" json:"delay_max_seconds"`

	// Cursor to avoid backfill on enable.
	LastEventSeq uint64 `gorm:"type:bigint unsigned;default:0" json:"last_event_seq"`

	LastEnabledAt  *time.Time `json:"last_enabled_at"`
	LastDisabledAt *time.Time `json:"last_disabled_at"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (SmartMoneyFollowConfig) TableName() string {
	return "smart_money_follow_configs"
}

type SmartMoneyFollowJob struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"not null;index;uniqueIndex:uniq_smf_job_user_chain_event" json:"user_id"`

	Chain         string `gorm:"size:10;not null;index;uniqueIndex:uniq_smf_job_user_chain_event" json:"chain"`
	WalletAddress string `gorm:"size:42;not null;index" json:"wallet_address"`
	EventSeq      uint64 `gorm:"type:bigint unsigned;not null;uniqueIndex:uniq_smf_job_user_chain_event" json:"event_seq"`

	PoolVersion string `gorm:"size:10;not null" json:"pool_version"`
	PoolID      string `gorm:"size:66;not null" json:"pool_id"`
	Action      string `gorm:"size:10;not null" json:"action"` // add | remove

	TickLower int `gorm:"default:0" json:"tick_lower"`
	TickUpper int `gorm:"default:0" json:"tick_upper"`

	ExecuteAt time.Time `gorm:"index" json:"execute_at"`
	Status    string    `gorm:"size:20;default:'pending';index" json:"status"` // pending | processing | done | failed | canceled

	TaskID       uint   `gorm:"default:0" json:"task_id"`
	ErrorMessage string `gorm:"type:text" json:"error_message,omitempty"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (SmartMoneyFollowJob) TableName() string {
	return "smart_money_follow_jobs"
}

type SmartMoneyFollowTask struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"not null;index;uniqueIndex:uniq_smf_task_key" json:"user_id"`

	Chain         string `gorm:"size:10;not null;index;uniqueIndex:uniq_smf_task_key" json:"chain"`
	WalletAddress string `gorm:"size:42;not null;index;uniqueIndex:uniq_smf_task_key" json:"wallet_address"`
	PoolVersion   string `gorm:"size:10;not null;uniqueIndex:uniq_smf_task_key" json:"pool_version"`
	PoolID        string `gorm:"size:66;not null;uniqueIndex:uniq_smf_task_key" json:"pool_id"`

	TaskID uint   `gorm:"not null;index" json:"task_id"`
	Status string `gorm:"size:20;default:'active';index" json:"status"` // active | closing | closed

	LastAddEventSeq    uint64 `gorm:"type:bigint unsigned;default:0" json:"last_add_event_seq"`
	LastRemoveEventSeq uint64 `gorm:"type:bigint unsigned;default:0" json:"last_remove_event_seq"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (SmartMoneyFollowTask) TableName() string {
	return "smart_money_follow_tasks"
}
