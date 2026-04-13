package models

import "time"

type SmartMoneyWatchWallet struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"not null;uniqueIndex:uq_sm_watch_wallet_user_chain_addr" json:"user_id"`
	Chain         string    `gorm:"size:16;not null;default:'bsc';uniqueIndex:uq_sm_watch_wallet_user_chain_addr" json:"chain"`
	WalletAddress string    `gorm:"size:42;not null;uniqueIndex:uq_sm_watch_wallet_user_chain_addr" json:"wallet_address"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (SmartMoneyWatchWallet) TableName() string {
	return "smart_money_user_watch_wallets"
}

type SmartMoneyWatchOpenAlertConfig struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"not null;uniqueIndex:uq_sm_watch_open_alert_user_chain" json:"user_id"`
	Chain        string    `gorm:"size:16;not null;default:'bsc';uniqueIndex:uq_sm_watch_open_alert_user_chain" json:"chain"`
	Enabled      bool      `gorm:"not null;default:false" json:"enabled"`
	BarkEnabled  bool      `gorm:"not null;default:false" json:"bark_enabled"`
	SoundEnabled bool      `gorm:"not null;default:false" json:"sound_enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (SmartMoneyWatchOpenAlertConfig) TableName() string {
	return "smart_money_watch_open_alert_configs"
}

type SmartMoneyWatchOpenAlertReceipt struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"not null;uniqueIndex:uq_sm_watch_open_alert_receipt" json:"user_id"`
	Chain     string    `gorm:"size:16;not null;default:'bsc';uniqueIndex:uq_sm_watch_open_alert_receipt" json:"chain"`
	TxHash    string    `gorm:"size:66;not null;uniqueIndex:uq_sm_watch_open_alert_receipt" json:"tx_hash"`
	LogIndex  int       `gorm:"not null;uniqueIndex:uq_sm_watch_open_alert_receipt" json:"log_index"`
	CreatedAt time.Time `json:"created_at"`
}

func (SmartMoneyWatchOpenAlertReceipt) TableName() string {
	return "smart_money_watch_open_alert_receipts"
}
