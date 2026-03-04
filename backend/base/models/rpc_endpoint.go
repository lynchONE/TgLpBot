package models

import (
	"time"

	"gorm.io/gorm"
)

// RpcEndpoint stores admin-managed RPC endpoints for a given chain and transport.
// Transport values: "http" | "ws".
// Chain values: "bsc" | "base" (extendable).
type RpcEndpoint struct {
	ID uint `gorm:"primaryKey" json:"id"`

	Chain     string `gorm:"type:varchar(16);not null;index:idx_rpc_chain_transport_current,priority:1;index:idx_rpc_chain_transport_url,unique,priority:1" json:"chain"`
	Transport string `gorm:"type:varchar(8);not null;index:idx_rpc_chain_transport_current,priority:2;index:idx_rpc_chain_transport_url,unique,priority:2" json:"transport"`
	Name      string `gorm:"type:varchar(64);not null;default:''" json:"name"`
	URL       string `gorm:"type:varchar(512);not null;index:idx_rpc_chain_transport_url,unique,priority:3" json:"url"`

	IsCurrent bool `gorm:"not null;default:false;index:idx_rpc_chain_transport_current,priority:3" json:"is_current"`

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

func (RpcEndpoint) TableName() string { return "rpc_endpoints" }
