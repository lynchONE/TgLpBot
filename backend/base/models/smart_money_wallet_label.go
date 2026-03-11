package models

import "time"

// SmartMoneyWalletLabel stores user-defined labels for both manual and discovered smart money wallets.
type SmartMoneyWalletLabel struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"uniqueIndex:idx_user_chain_wallet_label;not null" json:"user_id"`
	Chain         string    `gorm:"size:20;uniqueIndex:idx_user_chain_wallet_label;not null;default:bsc" json:"chain"`
	WalletAddress string    `gorm:"size:66;uniqueIndex:idx_user_chain_wallet_label;not null" json:"wallet_address"`
	Label         string    `gorm:"size:100" json:"label"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (SmartMoneyWalletLabel) TableName() string {
	return "smart_money_wallet_labels"
}
