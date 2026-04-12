package models

import (
	"time"

	"gorm.io/gorm"
)

type TradeRecordStatus string

const (
	TradeStatusOpen     TradeRecordStatus = "open"
	TradeStatusClosed   TradeRecordStatus = "closed"
	TradeStatusAborted  TradeRecordStatus = "aborted"
	TradeStatusOrphaned TradeRecordStatus = "orphaned"
)

// TradeRecord represents one full cycle: enter (open) + exit (close).
type TradeRecord struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID uint   `gorm:"not null;index" json:"user_id"`
	TaskID uint   `gorm:"not null;index" json:"task_id"`
	Chain  string `gorm:"size:10;not null;default:'bsc';index" json:"chain"`

	PoolVersion  string `gorm:"size:10" json:"pool_version"`
	PoolId       string `gorm:"size:66;index" json:"pool_id"` // V3 pool address or V4 poolId
	Exchange     string `gorm:"size:50" json:"exchange"`      // e.g. PancakeSwap V3 / Uniswap V3 / Uniswap V4
	Token0Symbol string `gorm:"size:20" json:"token0_symbol"`
	Token1Symbol string `gorm:"size:20" json:"token1_symbol"`

	OpenedAt         time.Time `gorm:"index" json:"opened_at"`
	OpenTxHash       string    `gorm:"size:66" json:"open_tx_hash"`
	OpenUSDTSpent    string    `gorm:"type:varchar(78)" json:"open_usdt_spent"`    // wei (1e18)
	OpenStableBefore string    `gorm:"type:varchar(78)" json:"open_stable_before"` // stable balance before entry, normalized to 1e18
	OpenStableAfter  string    `gorm:"type:varchar(78)" json:"open_stable_after"`  // stable balance after entry, normalized to 1e18
	OpenGasSpentWei  string    `gorm:"type:varchar(78)" json:"open_gas_spent_wei"` // BNB wei (1e18)
	OpenDust0        string    `gorm:"type:varchar(78)" json:"open_dust0"`         // token0 dust wei
	OpenDust1        string    `gorm:"type:varchar(78)" json:"open_dust1"`         // token1 dust wei

	ClosedAt          *time.Time        `gorm:"index" json:"closed_at"`
	CloseTxHash       string            `gorm:"size:66" json:"close_tx_hash"`
	CloseUSDTReceived string            `gorm:"type:varchar(78)" json:"close_usdt_received"`    // wei (1e18)
	CloseStableBefore string            `gorm:"type:varchar(78)" json:"close_stable_before"`    // stable balance before exit, normalized to 1e18
	CloseStableAfter  string            `gorm:"type:varchar(78)" json:"close_stable_after"`     // stable balance after exit, normalized to 1e18
	CloseGasSpentWei  string            `gorm:"type:varchar(78)" json:"close_gas_spent_wei"`    // BNB wei (1e18)
	TotalGasUSDT      string            `gorm:"type:varchar(78)" json:"total_gas_usdt"`         // 开仓+平仓 Gas 的 USDT 价值 (1e18)
	ProfitUSDT        string            `gorm:"type:varchar(78)" json:"profit_usdt"`            // wei (may be negative), 已扣除 Gas
	ProfitPct         float64           `gorm:"type:decimal(10,4);default:0" json:"profit_pct"` // (profit/open)*100
	Status            TradeRecordStatus `gorm:"size:12;index" json:"status"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (TradeRecord) TableName() string {
	return "trade_records"
}
