package models

import (
	"time"

	"gorm.io/gorm"
)

// SystemConfig 存储系统级配置（单例）
// 主要用于存储 AutoLP 硬筛阈值，支持管理员动态调整
type SystemConfig struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// AutoLP 硬筛阈值（0 表示使用环境变量默认值）
	AutoLPMinPoolValueUSD  float64 `gorm:"type:decimal(20,4);default:0" json:"autolp_min_pool_value_usd"`  // TVL 阈值 (USD)
	AutoLPMinFeePercentage float64 `gorm:"type:decimal(10,4);default:0" json:"autolp_min_fee_percentage"`  // 费率阈值 (%)
	AutoLPMinFeeRate5m     float64 `gorm:"type:decimal(10,6);default:0" json:"autolp_min_fee_rate_5m"`     // 5分钟费用率阈值 (%)
	AutoLPMinTotalFees5m   float64 `gorm:"type:decimal(20,4);default:0" json:"autolp_min_total_fees_5m"`   // 5分钟手续费阈值 (USD)
	AutoLPMinTotalVolume5m float64 `gorm:"type:decimal(20,4);default:0" json:"autolp_min_total_volume_5m"` // 5分钟成交量阈值 (USD)
	AutoLPMinTx5m          int     `gorm:"default:0" json:"autolp_min_tx_5m"`                              // 5分钟交易笔数阈值

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 指定表名
func (SystemConfig) TableName() string {
	return "system_configs"
}

// HardFilterConfig 硬筛配置结构，用于传递给 AutoLP 服务
type HardFilterConfig struct {
	MinPoolValueUSD  float64 `json:"min_pool_value_usd"`
	MinFeePercentage float64 `json:"min_fee_percentage"`
	MinFeeRate5m     float64 `json:"min_fee_rate_5m"`
	MinTotalFees5m   float64 `json:"min_total_fees_5m"`
	MinTotalVolume5m float64 `json:"min_total_volume_5m"`
	MinTx5m          int     `json:"min_tx_5m"`
}
