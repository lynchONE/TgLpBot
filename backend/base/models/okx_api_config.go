package models

import (
	"time"

	"gorm.io/gorm"
)

// OKXAPIConfig stores admin-managed OKX DEX API credentials.
type OKXAPIConfig struct {
	ID uint `gorm:"primaryKey" json:"id"`

	Name    string `gorm:"type:varchar(80);not null;default:''" json:"name"`
	BaseURL string `gorm:"type:varchar(512);not null;index:idx_okx_base_key,unique,priority:1" json:"base_url"`
	APIKey  string `gorm:"type:varchar(255);not null;index:idx_okx_base_key,unique,priority:2" json:"-"`

	SecretKeyEncrypted  string `gorm:"type:text;not null" json:"-"`
	PassphraseEncrypted string `gorm:"type:text;not null" json:"-"`

	IsCurrent bool `gorm:"not null;default:false;index" json:"is_current"`
	IsEnabled bool `gorm:"not null;default:true;index" json:"is_enabled"`

	DisabledUntil  *time.Time `gorm:"index" json:"disabled_until,omitempty"`
	DisabledReason string     `gorm:"type:varchar(32);not null;default:''" json:"disabled_reason"`

	ConsecutiveFailures int `gorm:"not null;default:0" json:"consecutive_failures"`

	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastLatencyMs int64      `gorm:"not null;default:0" json:"last_latency_ms"`
	LastError     string     `gorm:"type:varchar(512);not null;default:''" json:"last_error"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (OKXAPIConfig) TableName() string { return "okx_api_configs" }
