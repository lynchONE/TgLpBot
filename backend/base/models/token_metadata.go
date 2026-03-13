package models

import "time"

// TokenMetadata stores low-frequency token presentation metadata such as
// symbol, name and logo URL, keyed by chain + token address.
type TokenMetadata struct {
	ID uint `gorm:"primaryKey" json:"id"`

	Chain        string `gorm:"size:10;not null;uniqueIndex:idx_token_metadata_chain_addr" json:"chain"`
	TokenAddress string `gorm:"size:42;not null;uniqueIndex:idx_token_metadata_chain_addr" json:"token_address"`

	Symbol  string `gorm:"size:64" json:"symbol"`
	Name    string `gorm:"size:255" json:"name"`
	LogoURL string `gorm:"size:1024" json:"logo_url"`

	Source string `gorm:"size:32;not null;default:'okx'" json:"source"`
	Status string `gorm:"size:32;not null;default:'ok';index" json:"status"`

	FetchedAt time.Time `gorm:"not null" json:"fetched_at"`
	ExpiresAt time.Time `gorm:"not null;index" json:"expires_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (TokenMetadata) TableName() string {
	return "token_metadata"
}
