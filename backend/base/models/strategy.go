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

	// IsAuto marks tasks created by the AutoLP system (manual tasks are false).
	IsAuto bool `gorm:"default:false;index" json:"is_auto"`

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
	// RangeLowerPercentage/RangeUpperPercentage allow asymmetric ranges (when set > 0).
	RangeLowerPercentage float64 `gorm:"type:decimal(10,4);default:0" json:"range_lower_percentage"`
	RangeUpperPercentage float64 `gorm:"type:decimal(10,4);default:0" json:"range_upper_percentage"`

	// Position Info
	AmountUSDT       float64 `gorm:"type:decimal(20,8)" json:"amount_usdt"`     // Initial/Current USDT amount
	CurrentLiquidity string  `gorm:"type:varchar(78)" json:"current_liquidity"` // Current LP amount (uint128 string)

	// Strategy Config (defaults from GlobalConfig, can be overridden per task)
	ReopenDelaySeconds   int     `gorm:"default:300" json:"reopen_delay_seconds"` // Rebalance cooldown / wait seconds
	SlippageTolerance    float64 `gorm:"type:decimal(5,2);default:0.5" json:"slippage_tolerance"`
	AutoReinvest         bool    `gorm:"default:false" json:"auto_reinvest"`
	ResidualTolerance    float64 `gorm:"type:decimal(5,2);default:1.0" json:"residual_tolerance"`
	AllowEntrySwap       bool    `gorm:"default:false" json:"allow_entry_swap"` // Allow swapping USDT to entry token when pool lacks USDT
	StopLossEnabled      bool    `gorm:"default:false" json:"stop_loss_enabled"`
	StopLossDelaySeconds int     `gorm:"default:0" json:"stop_loss_delay_seconds"` // Out-of-range seconds before stop-loss triggers (0 = immediately)

	// State
	Paused          bool           `gorm:"default:false;index" json:"paused"`
	PausedAt        *time.Time     `json:"paused_at"`
	Status          StrategyStatus `gorm:"size:20;default:'running'" json:"status"`
	LastExitTime    *time.Time     `json:"last_exit_time"` // When did we exit/remove liquidity?
	LastRebalanceAt *time.Time     `json:"last_rebalance_at"`
	OutOfRangeSince *time.Time     `json:"out_of_range_since"`
	LastCheckTime   time.Time      `json:"last_check_time"`
	ErrorMessage    string         `gorm:"type:text" json:"error_message"`

	// Auto-mode guard state (persisted per task/pool)
	GuardOpenVolume5m           float64    `gorm:"type:decimal(20,8);default:0" json:"guard_open_volume_5m"`
	GuardOpenPrice              float64    `gorm:"type:decimal(30,12);default:0" json:"guard_open_price"`
	GuardOpenTxCount5m          int64      `gorm:"default:0" json:"guard_open_tx_count_5m"`
	GuardOpenFeePercentage      float64    `gorm:"type:decimal(10,4);default:0" json:"guard_open_fee_percentage"`
	GuardOpenFeeRate5mPct       float64    `gorm:"type:decimal(10,6);default:0" json:"guard_open_fee_rate_5m_pct"`
	GuardOpenTotalFees5m        float64    `gorm:"type:decimal(20,8);default:0" json:"guard_open_total_fees_5m"`
	GuardOpenTVLUSD             float64    `gorm:"type:decimal(20,8);default:0" json:"guard_open_tvl_usd"`
	GuardOpenMetricsAt          *time.Time `json:"guard_open_metrics_at"`
	GuardVolumeDropArmed        bool       `gorm:"default:false" json:"guard_volume_drop_armed"`
	GuardVolumeDropLastVolume5m float64    `gorm:"type:decimal(20,8);default:0" json:"guard_volume_drop_last_volume_5m"`
	GuardPriceTxDropArmed       bool       `gorm:"default:false" json:"guard_price_tx_drop_armed"`
	RangeBreakUpStreak          int        `gorm:"default:0" json:"range_break_up_streak"`
	RangeBreakDownStreak        int        `gorm:"default:0" json:"range_break_down_streak"`
	NextRangeMultiplier         float64    `gorm:"type:decimal(6,2);default:1.0" json:"next_range_multiplier"`
	CooldownUntil               *time.Time `json:"cooldown_until"`
	CooldownReason              string     `gorm:"type:text" json:"cooldown_reason"`

	// Exit retry state (keep task Status as running when exit fails).
	ExitPendingAction string     `gorm:"size:20;default:''" json:"exit_pending_action"` // manual_stop | stoploss | rebalance | switch | cooldown
	ExitPendingReason string     `gorm:"type:text" json:"exit_pending_reason"`
	ExitGasMultiplier float64    `gorm:"type:decimal(6,2);default:1.0" json:"exit_gas_multiplier"` // Gas multiplier for the next exit attempt (auto strategy may set to 2.0)
	ExitRetryCount    int        `gorm:"default:0" json:"exit_retry_count"`                        // number of failed attempts
	ExitNextRetryAt   *time.Time `json:"exit_next_retry_at"`
	ExitLastError     string     `gorm:"type:text" json:"exit_last_error"`
	ExitGiveUpAt      *time.Time `json:"exit_give_up_at"`

	// Switch target (pool migration) state.
	SwitchTargetPoolVersion  string  `gorm:"size:10;default:''" json:"switch_target_pool_version"`
	SwitchTargetPoolId       string  `gorm:"size:66;default:''" json:"switch_target_pool_id"`
	SwitchTargetTickLowerPct float64 `gorm:"type:decimal(10,4);default:0" json:"switch_target_tick_lower_pct"`
	SwitchTargetTickUpperPct float64 `gorm:"type:decimal(10,4);default:0" json:"switch_target_tick_upper_pct"`
	// ExitLiquidityRemoved marks that the liquidity removal tx already succeeded, and the remaining
	// pending work (if any) should retry swap-to-USDT only.
	ExitLiquidityRemoved bool `gorm:"default:false" json:"exit_liquidity_removed"`

	// Rebalance re-entry retry state (after exit succeeds).
	RebalancePending     bool       `gorm:"default:false" json:"rebalance_pending"`
	RebalanceRetryCount  int        `gorm:"default:0" json:"rebalance_retry_count"`
	RebalanceNextRetryAt *time.Time `json:"rebalance_next_retry_at"`
	RebalanceLastError   string     `gorm:"type:text" json:"rebalance_last_error"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (StrategyTask) TableName() string {
	return "strategy_tasks"
}
