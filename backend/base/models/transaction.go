package models

import (
	"time"

	"gorm.io/gorm"
)

// TransactionType represents the type of transaction
type TransactionType string

const (
	TxTypeSwap            TransactionType = "swap"
	TxTypeAddLiquidity    TransactionType = "add_liquidity"
	TxTypeRemoveLiquidity TransactionType = "remove_liquidity"
	TxTypeApprove         TransactionType = "approve"
)

// TransactionStatus represents the status of transaction
type TransactionStatus string

const (
	TxStatusPending   TransactionStatus = "pending"
	TxStatusConfirmed TransactionStatus = "confirmed"
	TxStatusFailed    TransactionStatus = "failed"
)

// Transaction represents a blockchain transaction
type Transaction struct {
	ID       uint              `gorm:"primaryKey" json:"id"`
	UserID   uint              `gorm:"not null;index" json:"user_id"`
	Chain    string            `gorm:"size:10;not null;default:'bsc';index" json:"chain"`
	TaskID   uint              `gorm:"index" json:"task_id"`
	TxHash   string            `gorm:"size:66;uniqueIndex" json:"tx_hash"`
	Type     TransactionType   `gorm:"size:20;not null;index" json:"type"`
	Status   TransactionStatus `gorm:"size:20;not null;index" json:"status"`
	Provider string            `gorm:"size:32;index" json:"provider,omitempty"`

	FromAddress string `gorm:"size:42;not null" json:"from_address"`
	ToAddress   string `gorm:"size:42;not null" json:"to_address"`

	// Transaction details
	TokenInAddress  string `gorm:"size:42" json:"token_in_address"`
	TokenOutAddress string `gorm:"size:42" json:"token_out_address"`
	AmountIn        string `gorm:"type:varchar(78)" json:"amount_in"`
	AmountOut       string `gorm:"type:varchar(78)" json:"amount_out"`

	GasPrice string `gorm:"type:varchar(78)" json:"gas_price"`
	GasUsed  uint64 `gorm:"default:0" json:"gas_used"`

	BlockNumber uint64 `gorm:"default:0;index" json:"block_number"`

	ErrorMessage string `gorm:"type:text" json:"error_message,omitempty"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships (without foreign key constraints in database)
	User User `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:NO ACTION,OnDelete:NO ACTION" json:"user,omitempty"`
}

// TableName specifies the table name for Transaction model
func (Transaction) TableName() string {
	return "transactions"
}
