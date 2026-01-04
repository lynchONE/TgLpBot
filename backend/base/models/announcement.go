package models

import (
	"time"

	"gorm.io/gorm"
)

// Announcement represents an admin announcement to be broadcast to users.
type Announcement struct {
	ID uint `gorm:"primaryKey" json:"id"`

	CreatedByUserID uint   `gorm:"not null;index" json:"created_by_user_id"`
	Title           string `gorm:"size:255" json:"title"`
	Content         string `gorm:"type:text;not null" json:"content"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Announcement) TableName() string {
	return "announcements"
}
