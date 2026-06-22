package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

const (
	SmartMoneyFollowAmountModeFixed = "fixed"
	SmartMoneyFollowAmountModeRatio = "ratio"

	SmartMoneyFollowDelayModeImmediate = "immediate"
	SmartMoneyFollowDelayModeFixed     = "fixed_delay"

	SmartMoneyFollowTriggerModeAny       = "any"
	SmartMoneyFollowTriggerModeThreshold = "threshold"

	SmartMoneyFollowExecutionWalletModeFixed      = "fixed"
	SmartMoneyFollowExecutionWalletModeRoundRobin = "round_robin"
	SmartMoneyFollowExecutionWalletModeRandom     = "random"

	SmartMoneyFollowStopReasonTakeProfit = "take_profit"
	SmartMoneyFollowStopReasonStopLoss   = "stop_loss"

	SmartMoneyFollowJobActionOpen         = "open"
	SmartMoneyFollowJobActionAddLiquidity = "add_liquidity"
	SmartMoneyFollowJobActionClose        = "close"

	SmartMoneyFollowJobStatusPending = "pending"
	SmartMoneyFollowJobStatusRunning = "running"
	SmartMoneyFollowJobStatusSuccess = "success"
	SmartMoneyFollowJobStatusFailed  = "failed"
	SmartMoneyFollowJobStatusSkipped = "skipped"

	SmartMoneyFollowAttemptStatusMatched = "matched"
	SmartMoneyFollowAttemptStatusCreated = "created"
	SmartMoneyFollowAttemptStatusFailed  = "failed"
	SmartMoneyFollowAttemptStatusSkipped = "skipped"

	SmartMoneyFollowTaskStatusOpen   = "open"
	SmartMoneyFollowTaskStatusClosed = "closed"
)

type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal([]string(a))
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (a *StringArray) Scan(value any) error {
	if value == nil {
		*a = nil
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("scan StringArray from %T", value)
	}
	if len(raw) == 0 {
		*a = nil
		return nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	*a = out
	return nil
}

type UintArray []uint

func (a UintArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal([]uint(a))
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (a *UintArray) Scan(value any) error {
	if value == nil {
		*a = nil
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("scan UintArray from %T", value)
	}
	if len(raw) == 0 {
		*a = nil
		return nil
	}
	var out []uint
	if err := json.Unmarshal(raw, &out); err == nil {
		*a = out
		return nil
	}
	var rawStrings []string
	if err := json.Unmarshal(raw, &rawStrings); err != nil {
		return err
	}
	out = make([]uint, 0, len(rawStrings))
	for _, item := range rawStrings {
		id64, err := strconv.ParseUint(item, 10, 64)
		if err != nil {
			return err
		}
		out = append(out, uint(id64))
	}
	*a = out
	return nil
}

type SmartMoneyFollowConfig struct {
	ID                        uint        `gorm:"primaryKey" json:"id"`
	UserID                    uint        `gorm:"not null;index" json:"user_id"`
	Chain                     string      `gorm:"size:16;not null;default:'bsc';index" json:"chain"`
	ChainID                   int         `gorm:"not null;default:56;index" json:"chain_id"`
	TaskName                  string      `gorm:"column:task_name;size:100;not null;default:''" json:"task_name"`
	TargetWalletAddress       string      `gorm:"size:42;not null;index" json:"target_wallet_address"`
	TargetWallets             StringArray `gorm:"column:target_wallet_addresses;type:json" json:"target_wallet_addresses"`
	ExecutionWalletID         uint        `gorm:"not null;default:0;index" json:"execution_wallet_id"`
	ExecutionWalletAddr       string      `gorm:"column:execution_wallet_address;size:42;not null;default:'';index" json:"execution_wallet_address"`
	ExecutionWalletIDs        UintArray   `gorm:"column:execution_wallet_ids;type:json" json:"execution_wallet_ids"`
	ExecutionWalletMode       string      `gorm:"size:20;not null;default:'fixed'" json:"execution_wallet_mode"`
	ExecutionWalletCursor     int         `gorm:"not null;default:0" json:"execution_wallet_cursor"`
	TriggerMode               string      `gorm:"size:16;not null;default:'any'" json:"trigger_mode"`
	TriggerMinWallets         int         `gorm:"not null;default:1" json:"trigger_min_wallets"`
	TriggerWindowSeconds      int         `gorm:"not null;default:300" json:"trigger_window_seconds"`
	Enabled                   bool        `gorm:"not null;default:false;index" json:"enabled"`
	AmountMode                string      `gorm:"size:16;not null;default:'fixed'" json:"amount_mode"`
	FixedAmountUSDT           float64     `gorm:"type:decimal(20,8);not null;default:0" json:"fixed_amount_usdt"`
	Ratio                     float64     `gorm:"type:decimal(12,8);not null;default:1" json:"ratio"`
	DelayMode                 string      `gorm:"size:20;not null;default:'immediate'" json:"delay_mode"`
	DelaySeconds              int         `gorm:"not null;default:0" json:"delay_seconds"`
	FollowClose               bool        `gorm:"not null;default:false" json:"follow_close"`
	RangeShiftGrids           int         `gorm:"not null;default:0" json:"range_shift_grids"`
	NotifyEnabled             bool        `gorm:"not null;default:false" json:"notify_enabled"`
	NotifyIntensity           string      `gorm:"size:32;not null;default:'ring'" json:"notify_intensity"`
	TakeProfitUSDT            float64     `gorm:"column:take_profit_usdt;type:decimal(20,8);not null;default:0" json:"take_profit_usdt"`
	StopLossUSDT              float64     `gorm:"column:stop_loss_usdt;type:decimal(20,8);not null;default:0" json:"stop_loss_usdt"`
	PnLBaselineUSDT           float64     `gorm:"column:pnl_baseline_usdt;type:decimal(20,8);not null;default:0" json:"pnl_baseline_usdt"`
	PnLBaselineRealizedUSDT   float64     `gorm:"column:pnl_baseline_realized_usdt;type:decimal(20,8);not null;default:0" json:"pnl_baseline_realized_usdt"`
	PnLBaselineUnrealizedUSDT float64     `gorm:"column:pnl_baseline_unrealized_usdt;type:decimal(20,8);not null;default:0" json:"pnl_baseline_unrealized_usdt"`
	PnLBaselineAt             *time.Time  `gorm:"column:pnl_baseline_at" json:"pnl_baseline_at,omitempty"`
	StopTriggeredAt           *time.Time  `json:"stop_triggered_at,omitempty"`
	StopTriggeredReason       string      `gorm:"size:32;not null;default:''" json:"stop_triggered_reason"`
	StopTriggeredPnLUSDT      float64     `gorm:"column:stop_triggered_pnl_usdt;type:decimal(20,8);not null;default:0" json:"stop_triggered_pnl_usdt"`
	CursorEventID             uint        `gorm:"not null;default:0" json:"cursor_event_id"`
	LastSeenEventID           uint        `gorm:"not null;default:0" json:"last_seen_event_id"`
	CreatedAt                 time.Time   `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt                 time.Time   `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (SmartMoneyFollowConfig) TableName() string { return "smart_money_follow_configs" }

type SmartMoneyFollowJob struct {
	ID                  uint        `gorm:"primaryKey" json:"id"`
	ConfigID            uint        `gorm:"not null;uniqueIndex:uq_sm_follow_job_config_event_action,priority:1;index" json:"config_id"`
	UserID              uint        `gorm:"not null;index" json:"user_id"`
	Chain               string      `gorm:"size:16;not null;default:'bsc';index" json:"chain"`
	ChainID             int         `gorm:"not null;default:56;index" json:"chain_id"`
	TargetWalletAddress string      `gorm:"size:42;not null;index" json:"target_wallet_address"`
	ExecutionWalletID   uint        `gorm:"not null;default:0;index" json:"execution_wallet_id"`
	ExecutionWalletAddr string      `gorm:"column:execution_wallet_address;size:42;not null;default:'';index" json:"execution_wallet_address"`
	EventID             uint        `gorm:"not null;uniqueIndex:uq_sm_follow_job_config_event_action,priority:2;index" json:"event_id"`
	TriggerMode         string      `gorm:"size:16;not null;default:'any'" json:"trigger_mode"`
	TriggerWallets      StringArray `gorm:"column:trigger_wallet_addresses;type:json" json:"trigger_wallet_addresses"`
	TriggerEventIDs     StringArray `gorm:"column:trigger_event_ids;type:json" json:"trigger_event_ids"`
	TargetPositionRef   string      `gorm:"size:255;not null;default:'';index" json:"target_position_ref"`
	Action              string      `gorm:"size:16;not null;uniqueIndex:uq_sm_follow_job_config_event_action,priority:3;index" json:"action"`
	Status              string      `gorm:"size:16;not null;default:'pending';index" json:"status"`
	ScheduledAt         time.Time   `gorm:"not null;index" json:"scheduled_at"`
	StartedAt           *time.Time  `json:"started_at"`
	FinishedAt          *time.Time  `json:"finished_at"`
	AmountUSDT          float64     `gorm:"type:decimal(20,8);not null;default:0" json:"amount_usdt"`
	TaskID              *uint       `gorm:"index" json:"task_id,omitempty"`
	RetryCount          int         `gorm:"not null;default:0" json:"retry_count"`
	ErrorMessage        string      `gorm:"type:text" json:"error_message"`
	CreatedAt           time.Time   `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time   `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (SmartMoneyFollowJob) TableName() string { return "smart_money_follow_jobs" }

type SmartMoneyFollowAttempt struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	ConfigID            uint      `gorm:"not null;uniqueIndex:uq_sm_follow_attempt_config_event_action,priority:1;index" json:"config_id"`
	UserID              uint      `gorm:"not null;index" json:"user_id"`
	Chain               string    `gorm:"size:16;not null;default:'bsc';index" json:"chain"`
	ChainID             int       `gorm:"not null;default:56;index" json:"chain_id"`
	TargetWalletAddress string    `gorm:"size:42;not null;index" json:"target_wallet_address"`
	ExecutionWalletID   uint      `gorm:"not null;default:0;index" json:"execution_wallet_id"`
	ExecutionWalletAddr string    `gorm:"column:execution_wallet_address;size:42;not null;default:'';index" json:"execution_wallet_address"`
	EventID             uint      `gorm:"not null;uniqueIndex:uq_sm_follow_attempt_config_event_action,priority:2;index" json:"event_id"`
	Action              string    `gorm:"size:16;not null;uniqueIndex:uq_sm_follow_attempt_config_event_action,priority:3;index" json:"action"`
	Status              string    `gorm:"size:16;not null;default:'matched';index" json:"status"`
	Message             string    `gorm:"type:text" json:"message"`
	JobID               *uint     `gorm:"index" json:"job_id,omitempty"`
	CreatedAt           time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (SmartMoneyFollowAttempt) TableName() string { return "smart_money_follow_attempts" }

type SmartMoneyFollowLogCursor struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"not null;uniqueIndex:uq_sm_follow_log_cursor_user_chain,priority:1;index" json:"user_id"`
	Chain          string    `gorm:"size:16;not null;default:'bsc';uniqueIndex:uq_sm_follow_log_cursor_user_chain,priority:2;index" json:"chain"`
	ClearedAt      time.Time `gorm:"not null;index" json:"cleared_at"`
	ClearedEventID uint      `gorm:"not null;default:0;index" json:"cleared_event_id"`
	CreatedAt      time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (SmartMoneyFollowLogCursor) TableName() string { return "smart_money_follow_log_cursors" }

type SmartMoneyFollowTask struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	ConfigID            uint      `gorm:"not null;index:idx_sm_follow_task_config_ref,priority:1" json:"config_id"`
	UserID              uint      `gorm:"not null;index" json:"user_id"`
	Chain               string    `gorm:"size:16;not null;default:'bsc';index" json:"chain"`
	ChainID             int       `gorm:"not null;default:56;index" json:"chain_id"`
	TargetWalletAddress string    `gorm:"size:42;not null;index" json:"target_wallet_address"`
	ExecutionWalletID   uint      `gorm:"not null;default:0;index" json:"execution_wallet_id"`
	ExecutionWalletAddr string    `gorm:"column:execution_wallet_address;size:42;not null;default:'';index" json:"execution_wallet_address"`
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
