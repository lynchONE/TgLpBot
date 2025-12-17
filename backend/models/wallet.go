package models

import (
	"time"

	"gorm.io/gorm"
)

// Wallet represents a user's wallet
type Wallet struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	UserID              uint           `gorm:"not null;index" json:"user_id"`
	Address             string         `gorm:"size:42;not null;index" json:"address"`
	EncryptedPrivateKey string         `gorm:"type:text;not null" json:"-"` // Encrypted private key
	Name                string         `gorm:"size:255" json:"name"`
	IsDefault           bool           `gorm:"default:false" json:"is_default"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships (without foreign key constraints in database)
	User User `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:NO ACTION,OnDelete:NO ACTION" json:"user,omitempty"`
}

// TableName specifies the table name for Wallet model
func (Wallet) TableName() string {
	return "wallets"
}
