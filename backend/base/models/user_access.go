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

	MaxWallets     int  `gorm:"default:1" json:"max_wallets"`
	MaxActiveTasks int  `gorm:"default:1" json:"max_active_tasks"`
	MiniAppEnabled bool `gorm:"default:false" json:"mini_app_enabled"`

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
