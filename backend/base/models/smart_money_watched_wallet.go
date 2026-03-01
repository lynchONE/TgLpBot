package models

import "time"

// SmartMoneyWatchedWallet stores user-managed monitored wallet addresses.
type SmartMoneyWatchedWallet struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"uniqueIndex:idx_user_chain_wallet;not null" json:"user_id"`
	Chain         string    `gorm:"size:20;uniqueIndex:idx_user_chain_wallet;not null;default:bsc" json:"chain"`
	WalletAddress string    `gorm:"size:66;uniqueIndex:idx_user_chain_wallet;not null" json:"wallet_address"`
	Label         string    `gorm:"size:100" json:"label"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (SmartMoneyWatchedWallet) TableName() string {
	return "smart_money_watched_wallets"
}
