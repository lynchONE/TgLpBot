package models

import (
	"time"

	"gorm.io/gorm"
)

// WalletBalanceSnapshot stores daily wallet balance snapshots for charting.
type WalletBalanceSnapshot struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	UserID        uint   `gorm:"not null;index;uniqueIndex:idx_wallet_day" json:"user_id"`
	WalletAddress string `gorm:"size:42;not null;index;uniqueIndex:idx_wallet_day" json:"wallet_address"`

	// Day is in format YYYY-MM-DD (server-local day).
	Day string `gorm:"size:10;not null;index;uniqueIndex:idx_wallet_day" json:"day"`

	BNBBalanceWei  string `gorm:"type:varchar(78)" json:"bnb_balance_wei"`
	USDTBalanceWei string `gorm:"type:varchar(78)" json:"usdt_balance_wei"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (WalletBalanceSnapshot) TableName() string {
	return "wallet_balance_snapshots"
}
