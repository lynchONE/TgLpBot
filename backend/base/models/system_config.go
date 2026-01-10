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

	// AutoLP 宽度策略参数（0 表示使用环境变量默认值）
	AutoLPWidthSidewaysPercent    float64 `gorm:"type:decimal(10,4);default:0" json:"autolp_width_sideways_percent"`     // 横盘宽度 (%)
	AutoLPWidthMildUptrendPercent float64 `gorm:"type:decimal(10,4);default:0" json:"autolp_width_mild_uptrend_percent"` // 温和上涨宽度 (%)
	AutoLPWidthRapidPumpPercent   float64 `gorm:"type:decimal(10,4);default:0" json:"autolp_width_rapid_pump_percent"`   // 急涨宽度 (%)

	// AutoLP 退出卫士参数（0 表示使用环境变量默认值）
	AutoLPGuardVolumeDropPercent    float64 `gorm:"type:decimal(10,4);default:0" json:"autolp_guard_volume_drop_percent"`     // 成交量下降阈值
	AutoLPGuardPriceDropPercent     float64 `gorm:"type:decimal(10,4);default:0" json:"autolp_guard_price_drop_percent"`      // 价格跌幅阈值
	AutoLPGuardTxDropPercent        float64 `gorm:"type:decimal(10,4);default:0" json:"autolp_guard_tx_drop_percent"`         // 交易笔数跌幅阈值
	AutoLPGuardLowFeeRate5m         float64 `gorm:"type:decimal(10,4);default:0" json:"autolp_guard_low_fee_rate_5m"`         // 低手续费率阈值
	AutoLPGuardVolumeDropPercentLow float64 `gorm:"type:decimal(10,4);default:0" json:"autolp_guard_volume_drop_percent_low"` // 低费率时成交量下降阈值

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

// WidthGuardConfig 宽度和退出卫士配置结构，用于传递默认值
type WidthGuardConfig struct {
	WidthSidewaysPercent      float64 `json:"width_sideways_percent"`
	WidthMildUptrendPercent   float64 `json:"width_mild_uptrend_percent"`
	WidthRapidPumpPercent     float64 `json:"width_rapid_pump_percent"`
	GuardVolumeDropPercent    float64 `json:"guard_volume_drop_percent"`
	GuardPriceDropPercent     float64 `json:"guard_price_drop_percent"`
	GuardTxDropPercent        float64 `json:"guard_tx_drop_percent"`
	GuardLowFeeRate5m         float64 `json:"guard_low_fee_rate_5m"`
	GuardVolumeDropPercentLow float64 `json:"guard_volume_drop_percent_low"`
}
