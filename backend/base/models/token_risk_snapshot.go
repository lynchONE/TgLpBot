package models

import "time"

// TokenRiskSnapshot stores OKX token risk data keyed by chain + token address.
type TokenRiskSnapshot struct {
	ID uint `gorm:"primaryKey" json:"id"`

	Chain        string `gorm:"size:32;not null;uniqueIndex:uq_token_risk_chain_addr,priority:1;index" json:"chain"`
	ChainIndex   string `gorm:"size:32;not null;default:''" json:"chain_index"`
	TokenAddress string `gorm:"size:128;not null;uniqueIndex:uq_token_risk_chain_addr,priority:2" json:"token_address"`
	TokenSymbol  string `gorm:"size:64;not null;default:''" json:"token_symbol"`
	TokenName    string `gorm:"size:255;not null;default:''" json:"token_name"`

	RiskControlLevel int    `gorm:"not null;default:0;index" json:"risk_control_level"`
	RiskControlLabel string `gorm:"size:32;not null;default:''" json:"risk_control_label"`
	RiskTone         string `gorm:"size:16;not null;default:'unknown';index" json:"risk_tone"`
	TokenTagsJSON    string `gorm:"type:json" json:"token_tags_json"`
	WarningsJSON     string `gorm:"type:json" json:"warnings_json"`

	HasHoneypot     bool `gorm:"not null;default:false;index" json:"has_honeypot"`
	HasLowLiquidity bool `gorm:"not null;default:false;index" json:"has_low_liquidity"`

	Top10HoldPercent         string `gorm:"size:80;not null;default:''" json:"top10_hold_percent"`
	DevHoldingPercent        string `gorm:"size:80;not null;default:''" json:"dev_holding_percent"`
	BundleHoldingPercent     string `gorm:"size:80;not null;default:''" json:"bundle_holding_percent"`
	SuspiciousHoldingPercent string `gorm:"size:80;not null;default:''" json:"suspicious_holding_percent"`
	SniperHoldingPercent     string `gorm:"size:80;not null;default:''" json:"sniper_holding_percent"`
	DevRugPullTokenCount     string `gorm:"size:80;not null;default:''" json:"dev_rug_pull_token_count"`
	DevCreateTokenCount      string `gorm:"size:80;not null;default:''" json:"dev_create_token_count"`
	DevLaunchedTokenCount    string `gorm:"size:80;not null;default:''" json:"dev_launched_token_count"`

	ErrorMessage     string     `gorm:"type:text" json:"error_message"`
	LastErrorMessage string     `gorm:"type:text" json:"last_error_message"`
	LastFailedAt     *time.Time `json:"last_failed_at,omitempty"`
	Source           string     `gorm:"size:64;not null;default:''" json:"source"`
	FetchedAt        time.Time  `gorm:"not null;index" json:"fetched_at"`
	NextRefreshAt    time.Time  `gorm:"not null;index" json:"next_refresh_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (TokenRiskSnapshot) TableName() string { return "token_risk_snapshots" }
