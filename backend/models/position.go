package models

import (
	"time"

	"gorm.io/gorm"
)

// PositionStatus represents the status of a position
type PositionStatus string

const (
	PositionStatusPending PositionStatus = "pending" // 等待中
	PositionStatusActive  PositionStatus = "active"  // 运行中
	PositionStatusStopped PositionStatus = "stopped" // 已停止
	PositionStatusClosed  PositionStatus = "closed"  // 已关闭
	PositionStatusError   PositionStatus = "error"   // 错误
)

// Position represents a liquidity position/task
type Position struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"not null;index" json:"user_id"`

	// Pool information
	PoolAddress   string `gorm:"size:42;not null;index" json:"pool_address"`
	Exchange      string `gorm:"size:50" json:"exchange"` // 交易所名称 (e.g., "PancakeSwap V3")
	Token0Address string `gorm:"size:42;not null" json:"token0_address"`
	Token1Address string `gorm:"size:42;not null" json:"token1_address"`
	Token0Symbol  string `gorm:"size:20" json:"token0_symbol"`
	Token1Symbol  string `gorm:"size:20" json:"token1_symbol"`
	Fee           int    `gorm:"not null" json:"fee"`          // 手续费 (e.g., 500 for 0.05%)
	TickSpacing   int    `gorm:"not null" json:"tick_spacing"` // Tick spacing

	// Position parameters
	TickLower   int    `gorm:"not null" json:"tick_lower"`              // 下限 tick
	TickUpper   int    `gorm:"not null" json:"tick_upper"`              // 上限 tick
	Amount      string `gorm:"type:varchar(78);not null" json:"amount"` // 投入金额 (in wei)
	AmountToken string `gorm:"size:42" json:"amount_token"`             // 投入代币地址 (e.g., USDT)

	// Position state
	Status    PositionStatus `gorm:"size:20;not null;index;default:'pending'" json:"status"`
	TokenID   string         `gorm:"type:varchar(78)" json:"token_id"`  // NFT Token ID (for Uniswap V3 style positions)
	Liquidity string         `gorm:"type:varchar(78)" json:"liquidity"` // Current liquidity

	// Transaction hashes
	OpenTxHash  string `gorm:"size:66" json:"open_tx_hash"`  // 开仓交易哈希
	CloseTxHash string `gorm:"size:66" json:"close_tx_hash"` // 平仓交易哈希

	// Timestamps
	OpenedAt        *time.Time `json:"opened_at"`
	ClosedAt        *time.Time `json:"closed_at"`
	LastRebalanceAt *time.Time `json:"last_rebalance_at"`

	// Error information
	ErrorMessage string `gorm:"type:text" json:"error_message"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships (without foreign key constraints in database)
	User User `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:NO ACTION,OnDelete:NO ACTION" json:"user,omitempty"`
}

// TableName specifies the table name for Position model
func (Position) TableName() string {
	return "positions"
}
