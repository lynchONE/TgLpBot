package models

import (
	"time"

	"gorm.io/gorm"
)

// GlobalConfig represents global configuration for all tasks
type GlobalConfig struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"not null;uniqueIndex" json:"user_id"` // One config per user

	// Bark notifications (optional; per-user)
	BarkEnabled      bool   `gorm:"not null;default:false" json:"bark_enabled"`
	BarkKeyEncrypted string `gorm:"type:text" json:"-"`
	BarkServer       string `gorm:"size:255;default:''" json:"bark_server"`
	BarkGroup        string `gorm:"size:100;default:''" json:"bark_group"`

	// Rebalance settings
	RebalanceTimeout int `gorm:"default:10" json:"rebalance_timeout"` // Rebalance timeout in seconds (-1 = immediate)

	// Stop loss settings
	StopLossThreshold    float64 `gorm:"type:decimal(10,4);default:10.0" json:"stop_loss_threshold"` // Stop loss threshold (range width percentage)
	StopLossEnabled      bool    `gorm:"default:false" json:"stop_loss_enabled"`
	StopLossDelaySeconds int     `gorm:"default:0" json:"stop_loss_delay_seconds"` // Out-of-range seconds before stop-loss triggers (0 = immediately)

	// Slippage settings
	SlippageTolerance float64 `gorm:"type:decimal(5,2);default:0.5" json:"slippage_tolerance"` // Slippage tolerance in percentage

	// Reinvest
	AutoReinvest bool `gorm:"default:false" json:"auto_reinvest"`

	// Residual tolerance when adding liquidity (percentage, e.g. 1.0 = 1%)
	ResidualTolerance float64 `gorm:"type:decimal(5,2);default:1.0" json:"residual_tolerance"`

	// Zap swap loss tolerance (percentage, e.g. 0.5 = 0.5%; 0 = disabled)
	// When > 0, the Zap contract reverts if cross-pool swap causes value loss exceeding this threshold
	ZapLossTolerance float64 `gorm:"type:decimal(5,2);default:0.5" json:"zap_loss_tolerance"`

	// Notifications
	ExtraNotificationsEnabled bool `gorm:"not null;default:true" json:"extra_notifications_enabled"`

	// Token filters
	FilterChineseTokens bool `gorm:"not null;default:false" json:"filter_chinese_tokens"`

	// Chain selection mode (per-user UX)
	MultiChainEnabled bool   `gorm:"not null;default:true" json:"multi_chain_enabled"`
	DefaultChain      string `gorm:"size:16;not null;default:'bsc'" json:"default_chain"`

	// Wallet selection mode (per-user UX)
	MultiWalletEnabled bool `gorm:"not null;default:false" json:"multi_wallet_enabled"`

	// Open-position sizing overrides (0 = inherit system default)
	OpenPositionTargetShareMin float64 `gorm:"type:decimal(6,4);default:0" json:"open_position_target_share_min"`
	OpenPositionTargetShareMax float64 `gorm:"type:decimal(6,4);default:0" json:"open_position_target_share_max"`
	OpenPositionRiskCapUSD     float64 `gorm:"type:decimal(20,4);default:0" json:"open_position_risk_cap_usd"`
	OpenPositionRiskCapRatio   float64 `gorm:"type:decimal(6,4);default:0" json:"open_position_risk_cap_ratio"`

	// DCA (dollar-cost average) batching defaults — split a single open into N batches to
	// avoid taking the full position at a momentary price spike. See strategy/dca.go.
	DCAEnabled            bool    `gorm:"default:false" json:"dca_enabled"`
	DCAPercentagesJSON    string  `gorm:"type:varchar(128);default:'[50,50]'" json:"dca_percentages_json"`
	DCAIntervalSeconds    float64 `gorm:"type:decimal(10,3);default:30" json:"dca_interval_seconds"`
	DCAMinSplitAmountUSDT float64 `gorm:"type:decimal(20,4);default:0" json:"dca_min_split_amount_usdt"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships (without foreign key constraints in database)
	User User `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:NO ACTION,OnDelete:NO ACTION" json:"user,omitempty"`
}

// TableName specifies the table name for GlobalConfig model
func (GlobalConfig) TableName() string {
	return "global_configs"
}

// LPConfig represents liquidity pool configuration (kept for backward compatibility)
type LPConfig struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	UserID        uint   `gorm:"not null;index" json:"user_id"`
	PoolAddress   string `gorm:"size:42;not null;index" json:"pool_address"`
	Token0Address string `gorm:"size:42;not null" json:"token0_address"`
	Token1Address string `gorm:"size:42;not null" json:"token1_address"`
	Token0Symbol  string `gorm:"size:20" json:"token0_symbol"`
	Token1Symbol  string `gorm:"size:20" json:"token1_symbol"`

	// LP Parameters
	MinToken0Amount   string  `gorm:"type:varchar(78)" json:"min_token0_amount"`               // Minimum amount for token0
	MinToken1Amount   string  `gorm:"type:varchar(78)" json:"min_token1_amount"`               // Minimum amount for token1
	MaxToken0Amount   string  `gorm:"type:varchar(78)" json:"max_token0_amount"`               // Maximum amount for token0
	MaxToken1Amount   string  `gorm:"type:varchar(78)" json:"max_token1_amount"`               // Maximum amount for token1
	SlippageTolerance float64 `gorm:"type:decimal(5,2);default:0.5" json:"slippage_tolerance"` // Slippage tolerance in percentage

	// Auto-execution settings
	AutoAdd    bool `gorm:"default:false" json:"auto_add"`
	AutoRemove bool `gorm:"default:false" json:"auto_remove"`

	IsActive  bool           `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships (without foreign key constraints in database)
	User User `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:NO ACTION,OnDelete:NO ACTION" json:"user,omitempty"`
}

// TableName specifies the table name for LPConfig model
func (LPConfig) TableName() string {
	return "lp_configs"
}
