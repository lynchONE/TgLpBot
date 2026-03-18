package web_server

import "time"

// HotPoolResponse is retained as the search-pool response model.
// The hot-pools business is removed, but search results still reuse this shape.
type HotPoolResponse struct {
	ProtocolVersion     string    `json:"protocol_version"`
	PoolAddress         string    `json:"pool_address"`
	Dex                 string    `json:"dex"`
	FactoryName         string    `json:"factory_name"`
	TradingPair         string    `json:"trading_pair"`
	FeePercentage       float64   `json:"fee_percentage"`
	TransactionCount    uint32    `json:"transaction_count"`
	TotalFees           float64   `json:"total_fees"`
	TotalVolume         float64   `json:"total_volume"`
	CurrentPoolValue    float64   `json:"current_pool_value"`
	FeeRate             float64   `json:"fee_rate"`
	PriceDisplay        string    `json:"price_display"`
	UpdatedAt           time.Time `json:"updated_at"`
	LastSwapAt          time.Time `json:"last_swap_at"`
	Token0Address       string    `json:"token0_address"`
	Token1Address       string    `json:"token1_address"`
	DisplayTokenAddress string    `json:"display_token_address,omitempty"`
	DisplayTokenSymbol  string    `json:"display_token_symbol,omitempty"`
	DisplayTokenName    string    `json:"display_token_name,omitempty"`
	DisplayTokenLogoURL string    `json:"display_token_logo_url,omitempty"`
	TotalFees24h        float64   `json:"total_fees_24h,omitempty"`
	TotalVolume24h      float64   `json:"total_volume_24h,omitempty"`
	TransactionCount24h uint32    `json:"transaction_count_24h,omitempty"`
}
