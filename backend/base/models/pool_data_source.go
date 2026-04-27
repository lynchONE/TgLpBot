package models

import (
	"time"

	"gorm.io/gorm"
)

// PoolDataSource stores admin-managed upstreams for the hot pool catalog sync.
type PoolDataSource struct {
	ID uint `gorm:"primaryKey" json:"id"`

	Name       string `gorm:"type:varchar(80);not null;default:''" json:"name"`
	SourceType string `gorm:"type:varchar(32);not null;index:idx_pool_ds_chain_tf_current,priority:3" json:"source_type"`
	Chain      string `gorm:"type:varchar(32);not null;default:'bsc';index:idx_pool_ds_chain_tf_current,priority:1" json:"chain"`

	TimeframeMinutes int `gorm:"not null;default:5;index:idx_pool_ds_chain_tf_current,priority:2" json:"timeframe_minutes"`
	Limit            int `gorm:"not null;default:100" json:"limit"`

	BaseURL           string `gorm:"type:varchar(512);not null;default:''" json:"base_url"`
	PathTemplate      string `gorm:"type:varchar(255);not null;default:''" json:"path_template"`
	QueryTemplateJSON string `gorm:"type:json" json:"query_template_json"`
	ProtocolsJSON     string `gorm:"type:json" json:"protocols_json"`
	DexesJSON         string `gorm:"type:json" json:"dexes_json"`

	IsCurrent bool `gorm:"not null;default:false;index:idx_pool_ds_chain_tf_current,priority:4" json:"is_current"`
	IsEnabled bool `gorm:"not null;default:true;index" json:"is_enabled"`

	LastCheckedAt         *time.Time `json:"last_checked_at,omitempty"`
	LastSuccessAt         *time.Time `json:"last_success_at,omitempty"`
	LastLatencyMs         int64      `gorm:"not null;default:0" json:"last_latency_ms"`
	LastError             string     `gorm:"type:varchar(512);not null;default:''" json:"last_error"`
	LastFieldCoverageJSON string     `gorm:"type:json" json:"last_field_coverage_json"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (PoolDataSource) TableName() string { return "pool_data_sources" }
