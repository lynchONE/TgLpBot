package models

import (
	"time"

	"gorm.io/gorm"
)

// SystemConfig stores singleton system-level configuration.
type SystemConfig struct {
	ID uint `gorm:"primaryKey" json:"id"`

	ZapPriceDeviationMaxPercent float64 `gorm:"type:decimal(10,4);default:0" json:"zap_price_deviation_max_percent"`
	ZapMinPoolLiquidityUSD      float64 `gorm:"type:decimal(20,4);default:0" json:"zap_min_pool_liquidity_usd"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (SystemConfig) TableName() string {
	return "system_configs"
}

type ZapSafetyConfig struct {
	PriceDeviationMaxPercent float64 `json:"price_deviation_max_percent"`
	MinPoolLiquidityUSD      float64 `json:"min_pool_liquidity_usd"`
}
