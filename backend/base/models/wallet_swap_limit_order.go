package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	WalletSwapLimitOrderStatusOpen       = "open"
	WalletSwapLimitOrderStatusTriggering = "triggering"
	WalletSwapLimitOrderStatusFilled     = "filled"
	WalletSwapLimitOrderStatusCancelled  = "cancelled"
	WalletSwapLimitOrderStatusFailed     = "failed"

	WalletSwapLimitOrderProviderBest = "best"
)

// WalletSwapLimitOrder stores a user-authorized future wallet swap trigger.
type WalletSwapLimitOrder struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID uint   `gorm:"not null;index:idx_wallet_swap_limit_user_status" json:"user_id"`
	Chain  string `gorm:"size:10;not null;default:'bsc';index" json:"chain"`

	WalletID      uint   `gorm:"not null;index" json:"wallet_id"`
	WalletAddress string `gorm:"size:42;not null;index" json:"wallet_address"`

	FromTokenAddress string `gorm:"size:42;not null;index" json:"from_token_address"`
	ToTokenAddress   string `gorm:"size:42;not null;index" json:"to_token_address"`
	FromAmount       string `gorm:"type:varchar(78);not null" json:"from_amount"`
	TargetToAmount   string `gorm:"type:varchar(78);not null" json:"target_to_amount"`
	TargetPrice      string `gorm:"type:varchar(78)" json:"target_price,omitempty"`

	SlippagePercent    float64 `gorm:"type:decimal(10,4);not null;default:1.0" json:"slippage_percent"`
	ProviderPreference string  `gorm:"size:16;not null;default:'best';index" json:"provider_preference"`

	Status string `gorm:"size:20;not null;default:'open';index:idx_wallet_swap_limit_user_status;index" json:"status"`

	LastCheckedAt        *time.Time `gorm:"index" json:"last_checked_at,omitempty"`
	NextCheckAt          *time.Time `gorm:"index" json:"next_check_at,omitempty"`
	LastQuoteProvider    string     `gorm:"size:16" json:"last_quote_provider,omitempty"`
	LastQuoteToAmount    string     `gorm:"type:varchar(78)" json:"last_quote_to_amount,omitempty"`
	LastQuoteGasUSD      float64    `gorm:"type:decimal(18,8);default:0" json:"last_quote_gas_usd,omitempty"`
	TriggerProvider      string     `gorm:"size:16" json:"trigger_provider,omitempty"`
	TriggerQuoteToAmount string     `gorm:"type:varchar(78)" json:"trigger_quote_to_amount,omitempty"`
	CheckCount           int        `gorm:"not null;default:0" json:"check_count"`

	TxHash         string     `gorm:"size:66;index" json:"tx_hash,omitempty"`
	ActualToAmount string     `gorm:"type:varchar(78)" json:"actual_to_amount,omitempty"`
	TriggeredAt    *time.Time `json:"triggered_at,omitempty"`
	FilledAt       *time.Time `json:"filled_at,omitempty"`
	CancelledAt    *time.Time `json:"cancelled_at,omitempty"`
	FailedAt       *time.Time `json:"failed_at,omitempty"`
	LastError      string     `gorm:"type:text" json:"last_error,omitempty"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (WalletSwapLimitOrder) TableName() string {
	return "wallet_swap_limit_orders"
}
