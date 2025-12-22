package models

import (
	"time"

	"gorm.io/gorm"
)

// StrategyStatus defines the status of a strategy task
type StrategyStatus string

const (
	StrategyStatusRunning  StrategyStatus = "running"
	StrategyStatusWaiting  StrategyStatus = "waiting" // Waiting to reopen
	StrategyStatusStopping StrategyStatus = "stopping"
	StrategyStatusStopped  StrategyStatus = "stopped"
	StrategyStatusError    StrategyStatus = "error"
)

// StrategyTask represents a monitoring task for a V4 pool position
type StrategyTask struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID uint   `gorm:"not null;index" json:"user_id"`
	PoolId string `gorm:"size:66;not null;index" json:"pool_id"` // V3 pool address (0x...) or V4 PoolId (0x...32 bytes)

	// Pool meta (cached for display)
	PoolVersion  string `gorm:"size:10;default:'v3'" json:"pool_version"`
	Exchange     string `gorm:"size:50" json:"exchange"`
	Token0Symbol string `gorm:"size:20" json:"token0_symbol"`
	Token1Symbol string `gorm:"size:20" json:"token1_symbol"`
	Fee          int    `gorm:"default:0" json:"fee"`
	TickSpacing  int    `gorm:"default:0" json:"tick_spacing"`

	// V4 Pool Key Components (Required because on-chain lookup fails for V4)
	Token0Address string `gorm:"size:42" json:"token0_address"`
	Token1Address string `gorm:"size:42" json:"token1_address"`
	HooksAddress  string `gorm:"size:42;default:'0x0000000000000000000000000000000000000000'" json:"hooks_address"`

	// On-chain position identifiers (optional; required for real remove/swap)
	V3PositionManagerAddress string `gorm:"size:42" json:"v3_position_manager_address"`
	V3TokenID                string `gorm:"type:varchar(78)" json:"v3_token_id"`
	V4TokenID                string `gorm:"type:varchar(78)" json:"v4_token_id"`
	V4Salt                   string `gorm:"size:66" json:"v4_salt"`            // bytes32 hex (0x...)
	V4HookDataHex            string `gorm:"type:text" json:"v4_hook_data_hex"` // optional hookData as hex (0x...)

	// Range Config
	TickLower int `gorm:"not null" json:"tick_lower"`
	TickUpper int `gorm:"not null" json:"tick_upper"`
	// RangePercentage is the +/- percentage used to compute tickLower/tickUpper (for rebalancing around current tick)
	RangePercentage float64 `gorm:"type:decimal(10,4);default:0" json:"range_percentage"`

	// Position Info
	AmountUSDT       float64 `gorm:"type:decimal(20,8)" json:"amount_usdt"`     // Initial/Current USDT amount
	CurrentLiquidity string  `gorm:"type:varchar(78)" json:"current_liquidity"` // Current LP amount (uint128 string)

	// Strategy Config (defaults from GlobalConfig, can be overridden per task)
	ReopenDelaySeconds   int     `gorm:"default:300" json:"reopen_delay_seconds"` // Rebalance cooldown / wait seconds
	SlippageTolerance    float64 `gorm:"type:decimal(5,2);default:0.5" json:"slippage_tolerance"`
	AutoReinvest         bool    `gorm:"default:false" json:"auto_reinvest"`
	ResidualTolerance    float64 `gorm:"type:decimal(5,2);default:1.0" json:"residual_tolerance"`
	StopLossEnabled      bool    `gorm:"default:false" json:"stop_loss_enabled"`
	StopLossDelaySeconds int     `gorm:"default:0" json:"stop_loss_delay_seconds"` // Out-of-range seconds before stop-loss triggers (0 = immediately)

	// State
	Status          StrategyStatus `gorm:"size:20;default:'running'" json:"status"`
	LastExitTime    *time.Time     `json:"last_exit_time"` // When did we exit/remove liquidity?
	LastRebalanceAt *time.Time     `json:"last_rebalance_at"`
	OutOfRangeSince *time.Time     `json:"out_of_range_since"`
	LastCheckTime   time.Time      `json:"last_check_time"`
	ErrorMessage    string         `gorm:"type:text" json:"error_message"`

	// Exit retry state (keep task Status as running when exit fails).
	ExitPendingAction string     `gorm:"size:20;default:''" json:"exit_pending_action"` // manual_stop | stoploss | rebalance
	ExitPendingReason string     `gorm:"type:text" json:"exit_pending_reason"`
	ExitRetryCount    int        `gorm:"default:0" json:"exit_retry_count"` // number of failed attempts
	ExitNextRetryAt   *time.Time `json:"exit_next_retry_at"`
	ExitLastError     string     `gorm:"type:text" json:"exit_last_error"`
	ExitGiveUpAt      *time.Time `json:"exit_give_up_at"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (StrategyTask) TableName() string {
	return "strategy_tasks"
}
