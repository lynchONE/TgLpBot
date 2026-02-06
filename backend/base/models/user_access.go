package models

import (
	"time"

	"gorm.io/gorm"
)

// UserAccess represents a user's access grant (time range + quotas).
type UserAccess struct {
	ID uint `gorm:"primaryKey" json:"id"`

	UserID uint `gorm:"uniqueIndex;not null" json:"user_id"`
	User   User `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:NO ACTION,OnDelete:NO ACTION" json:"user,omitempty"`

	GrantedByUserID uint  `gorm:"index" json:"granted_by_user_id"`
	GrantedByCodeID *uint `gorm:"index" json:"granted_by_code_id"`

	ActiveFrom *time.Time `gorm:"index" json:"active_from"`
	ActiveTo   *time.Time `gorm:"index" json:"active_to"`

	MaxWallets     int `gorm:"default:1" json:"max_wallets"`
	MaxActiveTasks int `gorm:"default:1" json:"max_active_tasks"`

	AutoModeEnabled   bool `gorm:"default:false" json:"auto_mode_enabled"`   // 是否有 Auto 模式权限
	MiniAppEnabled    bool `gorm:"default:false" json:"mini_app_enabled"`    // 是否有 Mini App 权限
	SmartMoneyEnabled bool `gorm:"default:false" json:"smart_money_enabled"` // 是否有 Smart Money(聪明钱) 权限

	RevokedAt       *time.Time `gorm:"index" json:"revoked_at"`
	RevokedByUserID *uint      `gorm:"index" json:"revoked_by_user_id"`

	Note      string         `gorm:"size:255" json:"note"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (UserAccess) TableName() string {
	return "user_accesses"
}
