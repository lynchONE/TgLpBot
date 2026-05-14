package models

import "time"

const (
	SmartMoneyFollowAmountModeFixed = "fixed"
	SmartMoneyFollowAmountModeRatio = "ratio"

	SmartMoneyFollowDelayModeImmediate = "immediate"
	SmartMoneyFollowDelayModeFixed     = "fixed_delay"

	SmartMoneyFollowJobActionOpen  = "open"
	SmartMoneyFollowJobActionClose = "close"

	SmartMoneyFollowJobStatusPending = "pending"
	SmartMoneyFollowJobStatusRunning = "running"
	SmartMoneyFollowJobStatusSuccess = "success"
	SmartMoneyFollowJobStatusFailed  = "failed"
	SmartMoneyFollowJobStatusSkipped = "skipped"

	SmartMoneyFollowTaskStatusOpen   = "open"
	SmartMoneyFollowTaskStatusClosed = "closed"
)

type SmartMoneyFollowConfig struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	UserID              uint      `gorm:"not null;index" json:"user_id"`
	Chain               string    `gorm:"size:16;not null;default:'bsc';index" json:"chain"`
	ChainID             int       `gorm:"not null;default:56;index" json:"chain_id"`
	TargetWalletAddress string    `gorm:"size:42;not null;index" json:"target_wallet_address"`
	Enabled             bool      `gorm:"not null;default:false;index" json:"enabled"`
	AmountMode          string    `gorm:"size:16;not null;default:'fixed'" json:"amount_mode"`
	FixedAmountUSDT     float64   `gorm:"type:decimal(20,8);not null;default:0" json:"fixed_amount_usdt"`
	Ratio               float64   `gorm:"type:decimal(12,8);not null;default:1" json:"ratio"`
	DelayMode           string    `gorm:"size:20;not null;default:'immediate'" json:"delay_mode"`
	DelaySeconds        int       `gorm:"not null;default:0" json:"delay_seconds"`
	FollowClose         bool      `gorm:"not null;default:false" json:"follow_close"`
	CursorEventID       uint      `gorm:"not null;default:0" json:"cursor_event_id"`
	LastSeenEventID     uint      `gorm:"not null;default:0" json:"last_seen_event_id"`
	CreatedAt           time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (SmartMoneyFollowConfig) TableName() string { return "smart_money_follow_configs" }

type SmartMoneyFollowJob struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	ConfigID            uint       `gorm:"not null;uniqueIndex:uq_sm_follow_job_config_event_action,priority:1;index" json:"config_id"`
	UserID              uint       `gorm:"not null;index" json:"user_id"`
	Chain               string     `gorm:"size:16;not null;default:'bsc';index" json:"chain"`
	ChainID             int        `gorm:"not null;default:56;index" json:"chain_id"`
	TargetWalletAddress string     `gorm:"size:42;not null;index" json:"target_wallet_address"`
	EventID             uint       `gorm:"not null;uniqueIndex:uq_sm_follow_job_config_event_action,priority:2;index" json:"event_id"`
	TargetPositionRef   string     `gorm:"size:255;not null;default:'';index" json:"target_position_ref"`
	Action              string     `gorm:"size:16;not null;uniqueIndex:uq_sm_follow_job_config_event_action,priority:3;index" json:"action"`
	Status              string     `gorm:"size:16;not null;default:'pending';index" json:"status"`
	ScheduledAt         time.Time  `gorm:"not null;index" json:"scheduled_at"`
	StartedAt           *time.Time `json:"started_at"`
	FinishedAt          *time.Time `json:"finished_at"`
	AmountUSDT          float64    `gorm:"type:decimal(20,8);not null;default:0" json:"amount_usdt"`
	TaskID              *uint      `gorm:"index" json:"task_id,omitempty"`
	ErrorMessage        string     `gorm:"type:text" json:"error_message"`
	CreatedAt           time.Time  `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (SmartMoneyFollowJob) TableName() string { return "smart_money_follow_jobs" }

type SmartMoneyFollowTask struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	ConfigID            uint      `gorm:"not null;index:idx_sm_follow_task_config_ref,priority:1" json:"config_id"`
	UserID              uint      `gorm:"not null;index" json:"user_id"`
	Chain               string    `gorm:"size:16;not null;default:'bsc';index" json:"chain"`
	ChainID             int       `gorm:"not null;default:56;index" json:"chain_id"`
	TargetWalletAddress string    `gorm:"size:42;not null;index" json:"target_wallet_address"`
	TargetPositionRef   string    `gorm:"size:255;not null;index:idx_sm_follow_task_config_ref,priority:2" json:"target_position_ref"`
	OpenEventID         uint      `gorm:"not null;uniqueIndex:uq_sm_follow_task_open_event" json:"open_event_id"`
	OpenJobID           uint      `gorm:"not null;index" json:"open_job_id"`
	TaskID              uint      `gorm:"not null;uniqueIndex;index" json:"task_id"`
	Status              string    `gorm:"size:16;not null;default:'open';index" json:"status"`
	CloseEventID        *uint     `json:"close_event_id,omitempty"`
	CloseJobID          *uint     `json:"close_job_id,omitempty"`
	CreatedAt           time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (SmartMoneyFollowTask) TableName() string { return "smart_money_follow_tasks" }
