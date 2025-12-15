package models

import (
	"time"

	"gorm.io/gorm"
)

// LPConfig represents liquidity pool configuration
type LPConfig struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	UserID            uint           `gorm:"not null;index" json:"user_id"`
	PoolAddress       string         `gorm:"size:42;not null;index" json:"pool_address"`
	Token0Address     string         `gorm:"size:42;not null" json:"token0_address"`
	Token1Address     string         `gorm:"size:42;not null" json:"token1_address"`
	Token0Symbol      string         `gorm:"size:20" json:"token0_symbol"`
	Token1Symbol      string         `gorm:"size:20" json:"token1_symbol"`
	
	// LP Parameters
	MinToken0Amount   string         `gorm:"type:varchar(78)" json:"min_token0_amount"` // Minimum amount for token0
	MinToken1Amount   string         `gorm:"type:varchar(78)" json:"min_token1_amount"` // Minimum amount for token1
	MaxToken0Amount   string         `gorm:"type:varchar(78)" json:"max_token0_amount"` // Maximum amount for token0
	MaxToken1Amount   string         `gorm:"type:varchar(78)" json:"max_token1_amount"` // Maximum amount for token1
	SlippageTolerance float64        `gorm:"type:decimal(5,2);default:0.5" json:"slippage_tolerance"` // Slippage tolerance in percentage
	
	// Auto-execution settings
	AutoAdd           bool           `gorm:"default:false" json:"auto_add"`
	AutoRemove        bool           `gorm:"default:false" json:"auto_remove"`
	
	IsActive          bool           `gorm:"default:true" json:"is_active"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
	
	// Relationships
	User              User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName specifies the table name for LPConfig model
func (LPConfig) TableName() string {
	return "lp_configs"
}

