package models

import (
	"time"

	"gorm.io/gorm"
)

// WalletChainContract stores a per-wallet, per-chain contract binding (e.g. private ZapSimple).
// One wallet+chain+kind has at most one active binding row; upgrades overwrite the row (version bump).
type WalletChainContract struct {
	ID uint `gorm:"primaryKey" json:"id"`

	WalletID uint   `gorm:"not null;index;uniqueIndex:uniq_wallet_chain_kind,priority:1" json:"wallet_id"`
	Chain    string `gorm:"size:10;not null;default:'bsc';index;uniqueIndex:uniq_wallet_chain_kind,priority:2" json:"chain"`
	Kind     string `gorm:"size:32;not null;default:'zap_simple';uniqueIndex:uniq_wallet_chain_kind,priority:3" json:"kind"`

	Status string `gorm:"size:16;not null;default:'ready';index" json:"status"`

	ContractAddress string `gorm:"size:42;not null" json:"contract_address"`
	Version         int    `gorm:"not null;default:1" json:"version"`

	DeployTxHash string `gorm:"size:66;default:''" json:"deploy_tx_hash"`
	ConfigTxHash string `gorm:"size:66;default:''" json:"config_tx_hash"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (WalletChainContract) TableName() string {
	return "wallet_chain_contracts"
}
