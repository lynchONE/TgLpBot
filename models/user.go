package models

import (
	"time"

	"gorm.io/gorm"
)

// User represents a Telegram user
type User struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	TelegramID     int64          `gorm:"uniqueIndex;not null" json:"telegram_id"`
	Username       string         `gorm:"size:255" json:"username"`
	FirstName      string         `gorm:"size:255" json:"first_name"`
	LastName       string         `gorm:"size:255" json:"last_name"`
	LanguageCode   string         `gorm:"size:10" json:"language_code"`
	IsActive       bool           `gorm:"default:true" json:"is_active"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	
	// Relationships
	Wallets        []Wallet       `gorm:"foreignKey:UserID" json:"wallets,omitempty"`
	LPConfigs      []LPConfig     `gorm:"foreignKey:UserID" json:"lp_configs,omitempty"`
	Transactions   []Transaction  `gorm:"foreignKey:UserID" json:"transactions,omitempty"`
}

// TableName specifies the table name for User model
func (User) TableName() string {
	return "users"
}

