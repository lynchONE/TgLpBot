package models

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// StrategyStatus defines the status of a strategy task
type StrategyStatus string

const (
	StrategyStatusOpening  StrategyStatus = "opening"
	StrategyStatusRunning  StrategyStatus = "running"
	StrategyStatusWaiting  StrategyStatus = "waiting" // Waiting to reopen
	StrategyStatusStopping StrategyStatus = "stopping"
	StrategyStatusStopped  StrategyStatus = "stopped"
	StrategyStatusError    StrategyStatus = "error"
)

type StrategyOutOfRangeMode string

const (
	StrategyOutOfRangeModeRebalanceAll        StrategyOutOfRangeMode = "rebalance_all"
	StrategyOutOfRangeModeExitAll             StrategyOutOfRangeMode = "exit_all"
	StrategyOutOfRangeModeRebalanceUpExitDown StrategyOutOfRangeMode = "rebalance_up_exit_down"
	StrategyTaskModePause                     string                 = "pause"
)

// StrategyTask represents a monitoring task for a V4 pool position
type StrategyTask struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID uint   `gorm:"not null;index" json:"user_id"`
	Chain  string `gorm:"size:10;not null;default:'bsc';index" json:"chain"`
	PoolId string `gorm:"size:66;not null;index" json:"pool_id"` // V3 pool address (0x...) or V4 PoolId (0x...32 bytes)

	// Wallet binding: tasks MUST execute on the wallet used at entry time.
	WalletID      uint   `gorm:"not null;default:0;index" json:"wallet_id"`
	WalletAddress string `gorm:"size:42;not null;default:'';index" json:"wallet_address"`

	// IsFollow marks tasks created by Smart Money wallet follow (copy-trading).
	// Follow tasks keep the tick range consistent with the target wallet; they should not auto-rebalance.
	IsFollow bool `gorm:"default:false;index" json:"is_follow"`

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
	ReopenDelaySeconds   int     `gorm:"default:10" json:"reopen_delay_seconds"` // Rebalance cooldown / wait seconds (-1 = immediate)
	SlippageTolerance    float64 `gorm:"type:decimal(5,2);default:0.5" json:"slippage_tolerance"`
	AutoReinvest         bool    `gorm:"default:false" json:"auto_reinvest"`
	ResidualTolerance    float64 `gorm:"type:decimal(5,2);default:1.0" json:"residual_tolerance"`
	ZapLossTolerance     float64 `gorm:"type:decimal(5,2);default:0.5" json:"zap_loss_tolerance"` // Swap loss tolerance (0 = disabled)
	AllowEntrySwap       bool    `gorm:"default:false" json:"allow_entry_swap"`                   // Allow swapping USDT to entry token when pool lacks USDT
	StopLossEnabled      bool    `gorm:"default:false" json:"stop_loss_enabled"`
	StopLossDelaySeconds int     `gorm:"default:0" json:"stop_loss_delay_seconds"` // Out-of-range seconds before stop-loss triggers (0 = immediately)
	RebalanceEnabled     bool    `gorm:"default:false" json:"rebalance_enabled"`   // When false, out-of-range positions exit to USDT and stop after the same delay
	OutOfRangeMode       string  `gorm:"size:40;not null;default:'exit_all'" json:"out_of_range_mode"`

	// State
	Paused                 bool           `gorm:"default:false;index" json:"paused"`
	PausedAt               *time.Time     `json:"paused_at"`
	Status                 StrategyStatus `gorm:"size:20;default:'running'" json:"status"`
	LastExitTime           *time.Time     `json:"last_exit_time"` // When did we exit/remove liquidity?
	LastRebalanceAt        *time.Time     `json:"last_rebalance_at"`
	OutOfRangeSince        *time.Time     `json:"out_of_range_since"`
	RangeActivationPending bool           `gorm:"default:false" json:"range_activation_pending"` // Single-sided positions wait until first in-range before auto handling starts
	LastCheckTime          time.Time      `json:"last_check_time"`
	ErrorMessage           string         `gorm:"type:text" json:"error_message"`

	// Exit retry state (keep task Status as running when exit fails).
	ExitPendingAction string     `gorm:"size:20;default:''" json:"exit_pending_action"` // manual_stop | stoploss | rebalance | switch
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

	// DCA (time-based batching) state — first batch is the mint, later batches call
	// IncreaseLiquidity on this same position. See strategy/dca.go and strategy/strategy_dca.go.
	DCAEnabled         bool       `gorm:"default:false" json:"dca_enabled"`
	DCATotalAmountUSDT float64    `gorm:"type:decimal(20,8);default:0" json:"dca_total_amount_usdt"`
	DCAPercentagesJSON string     `gorm:"type:varchar(128);default:''" json:"dca_percentages_json"`
	DCAIntervalSeconds float64    `gorm:"type:decimal(10,3);default:0" json:"dca_interval_seconds"`
	DCAExecutedCount   int        `gorm:"default:0" json:"dca_executed_count"`
	DCARetryCount      int        `gorm:"default:0" json:"dca_retry_count"`
	DCANextBatchAt     *time.Time `json:"dca_next_batch_at"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (StrategyTask) TableName() string {
	return "strategy_tasks"
}

func NormalizeStrategyOutOfRangeMode(value string) StrategyOutOfRangeMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(StrategyOutOfRangeModeRebalanceAll):
		return StrategyOutOfRangeModeRebalanceAll
	case string(StrategyOutOfRangeModeExitAll):
		return StrategyOutOfRangeModeExitAll
	case string(StrategyOutOfRangeModeRebalanceUpExitDown):
		return StrategyOutOfRangeModeRebalanceUpExitDown
	default:
		return ""
	}
}

func NormalizeStrategyTaskMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(StrategyOutOfRangeModeRebalanceAll):
		return string(StrategyOutOfRangeModeRebalanceAll)
	case string(StrategyOutOfRangeModeExitAll):
		return string(StrategyOutOfRangeModeExitAll)
	case string(StrategyOutOfRangeModeRebalanceUpExitDown):
		return string(StrategyOutOfRangeModeRebalanceUpExitDown)
	case StrategyTaskModePause:
		return StrategyTaskModePause
	default:
		return ""
	}
}

func RebalanceEnabledForOutOfRangeMode(mode StrategyOutOfRangeMode) bool {
	switch mode {
	case StrategyOutOfRangeModeRebalanceAll, StrategyOutOfRangeModeRebalanceUpExitDown:
		return true
	default:
		return false
	}
}

func ResolveStrategyOutOfRangeMode(task *StrategyTask) StrategyOutOfRangeMode {
	if task == nil {
		return ""
	}
	if mode := NormalizeStrategyOutOfRangeMode(task.OutOfRangeMode); mode != "" {
		return mode
	}
	if task.RebalanceEnabled {
		return StrategyOutOfRangeModeRebalanceAll
	}
	return StrategyOutOfRangeModeExitAll
}

func EffectiveStrategyTaskMode(task *StrategyTask) string {
	if task == nil {
		return ""
	}
	if task.Paused {
		return StrategyTaskModePause
	}
	return string(ResolveStrategyOutOfRangeMode(task))
}

func (t *StrategyTask) SyncOutOfRangeModeFields() {
	if t == nil {
		return
	}
	mode := ResolveStrategyOutOfRangeMode(t)
	if mode == "" {
		mode = StrategyOutOfRangeModeExitAll
	}
	t.OutOfRangeMode = string(mode)
	t.RebalanceEnabled = RebalanceEnabledForOutOfRangeMode(mode)
}

// CreateOverrideUpdates returns the zero/false values that must be persisted
// explicitly after create, otherwise MySQL defaults may overwrite them.
func (t *StrategyTask) CreateOverrideUpdates() map[string]interface{} {
	if t == nil {
		return nil
	}

	updates := make(map[string]interface{})

	if t.ReopenDelaySeconds == 0 {
		updates["reopen_delay_seconds"] = 0
	}
	if t.SlippageTolerance == 0 {
		updates["slippage_tolerance"] = 0
	}
	if t.ResidualTolerance == 0 {
		updates["residual_tolerance"] = 0
	}
	if t.ZapLossTolerance == 0 {
		updates["zap_loss_tolerance"] = 0
	}
	if !t.RebalanceEnabled {
		updates["rebalance_enabled"] = false
	}
	if t.Paused {
		updates["paused"] = true
		if t.PausedAt != nil {
			updates["paused_at"] = t.PausedAt
		}
	}

	if !t.DCAEnabled {
		updates["dca_enabled"] = false
	}
	if t.DCAIntervalSeconds == 0 {
		updates["dca_interval_seconds"] = 0
	}
	if t.DCAExecutedCount == 0 {
		updates["dca_executed_count"] = 0
	}

	return updates
}

// ApplyCreateOverrides persists zero/false values that GORM may skip on insert
// when the column has a database default.
func (t *StrategyTask) ApplyCreateOverrides(tx *gorm.DB) error {
	if t == nil || t.ID == 0 || tx == nil {
		return nil
	}

	updates := t.CreateOverrideUpdates()
	if len(updates) == 0 {
		return nil
	}

	return tx.Model(t).UpdateColumns(updates).Error
}

func (t *StrategyTask) AfterCreate(tx *gorm.DB) error {
	return t.ApplyCreateOverrides(tx)
}

func (t *StrategyTask) BeforeSave(tx *gorm.DB) error {
	t.SyncOutOfRangeModeFields()
	return nil
}
