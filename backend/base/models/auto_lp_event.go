package models

import "time"

// AutoLPEventType represents a tracked AutoLP execution event.
type AutoLPEventType string

const (
	AutoLPEventOpen      AutoLPEventType = "open"
	AutoLPEventRebalance AutoLPEventType = "rebalance"
	AutoLPEventGuardExit AutoLPEventType = "guard_exit"
)

// AutoLPEvent stores aggregated execution events for AutoLP tasks.
type AutoLPEvent struct {
	ID        uint            `gorm:"primaryKey" json:"id"`
	UserID    uint            `gorm:"not null;index" json:"user_id"`
	TaskID    uint            `gorm:"index" json:"task_id"`
	EventType AutoLPEventType `gorm:"size:20;index" json:"event_type"`
	Reason    string          `gorm:"type:text" json:"reason,omitempty"`

	PoolVersion  string `gorm:"size:10" json:"pool_version"`
	PoolId       string `gorm:"size:66;index" json:"pool_id"`
	Token0Symbol string `gorm:"size:20" json:"token0_symbol"`
	Token1Symbol string `gorm:"size:20" json:"token1_symbol"`

	CreatedAt time.Time `json:"created_at"`
}

func (AutoLPEvent) TableName() string {
	return "auto_lp_events"
}
