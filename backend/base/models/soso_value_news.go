package models

import "time"

type SosoValueNewsItem struct {
	ID uint `gorm:"primaryKey" json:"id"`

	Feed       string `gorm:"size:32;not null;uniqueIndex:uq_soso_news_feed_external;index" json:"feed"`
	ExternalID string `gorm:"size:128;not null;uniqueIndex:uq_soso_news_feed_external" json:"external_id"`

	Title           string    `gorm:"size:512;not null;index" json:"title"`
	Content         string    `gorm:"type:text" json:"content"`
	Language        string    `gorm:"size:16;index" json:"language"`
	SourceLink      string    `gorm:"size:1024" json:"source_link"`
	Author          string    `gorm:"size:255;index" json:"author"`
	AuthorAvatarURL string    `gorm:"size:1024" json:"author_avatar_url"`
	NickName        string    `gorm:"size:255" json:"nick_name"`
	Category        int       `gorm:"index" json:"category"`
	FeatureImage    string    `gorm:"size:1024" json:"feature_image"`
	TagsJSON        string    `gorm:"type:text" json:"tags_json"`
	RawJSON         string    `gorm:"type:longtext" json:"raw_json"`
	ReleaseTime     time.Time `gorm:"index" json:"release_time"`
	FetchedAt       time.Time `gorm:"index" json:"fetched_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SosoValueNewsItem) TableName() string {
	return "soso_value_news_items"
}

type SosoValueAPIUsage struct {
	ID uint `gorm:"primaryKey" json:"id"`

	Month         string     `gorm:"size:7;not null;uniqueIndex" json:"month"`
	RequestCount  int        `gorm:"not null;default:0" json:"request_count"`
	LastRequestAt *time.Time `json:"last_request_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SosoValueAPIUsage) TableName() string {
	return "soso_value_api_usages"
}
