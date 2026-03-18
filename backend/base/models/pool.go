package models

import "time"

// Pool stores the current pool catalog used by /api/pools.
// The column set mirrors the legacy pools analytics schema so the retained
// business can switch to MySQL without changing the response shape.
type Pool struct {
	ID                            string     `gorm:"column:id;type:varchar(128);primaryKey" json:"id"`
	Type                          string     `gorm:"column:type;type:varchar(32);not null;default:''" json:"type"`
	Address                       string     `gorm:"column:address;type:varchar(128);not null;index:idx_pools_address" json:"address"`
	Name                          string     `gorm:"column:name;type:varchar(255);not null;default:''" json:"name"`
	BaseTokenID                   string     `gorm:"column:base_token_id;type:varchar(128);not null;default:''" json:"base_token_id"`
	QuoteTokenID                  string     `gorm:"column:quote_token_id;type:varchar(128);not null;default:''" json:"quote_token_id"`
	DexID                         string     `gorm:"column:dex_id;type:varchar(64);not null;default:'';index:idx_pools_dex" json:"dex_id"`
	BaseTokenPriceUSD             float64    `gorm:"column:base_token_price_usd;type:double;not null;default:0" json:"base_token_price_usd"`
	QuoteTokenPriceUSD            float64    `gorm:"column:quote_token_price_usd;type:double;not null;default:0" json:"quote_token_price_usd"`
	BaseTokenPriceNativeCurrency  float64    `gorm:"column:base_token_price_native_currency;type:double;not null;default:0" json:"base_token_price_native_currency"`
	QuoteTokenPriceNativeCurrency float64    `gorm:"column:quote_token_price_native_currency;type:double;not null;default:0" json:"quote_token_price_native_currency"`
	BaseTokenPriceQuoteToken      float64    `gorm:"column:base_token_price_quote_token;type:double;not null;default:0" json:"base_token_price_quote_token"`
	QuoteTokenPriceBaseToken      float64    `gorm:"column:quote_token_price_base_token;type:double;not null;default:0" json:"quote_token_price_base_token"`
	PoolCreatedAt                 *time.Time `gorm:"column:pool_created_at;type:datetime(3)" json:"pool_created_at,omitempty"`
	FDVUSD                        float64    `gorm:"column:fdv_usd;type:double;not null;default:0" json:"fdv_usd"`
	MarketCapUSD                  float64    `gorm:"column:market_cap_usd;type:double;not null;default:0" json:"market_cap_usd"`
	ReserveInUSD                  float64    `gorm:"column:reserve_in_usd;type:double;not null;default:0" json:"reserve_in_usd"`
	PriceChangeM5                 float64    `gorm:"column:price_change_m5;type:double;not null;default:0" json:"price_change_m5"`
	PriceChangeH1                 float64    `gorm:"column:price_change_h1;type:double;not null;default:0" json:"price_change_h1"`
	PriceChangeH6                 float64    `gorm:"column:price_change_h6;type:double;not null;default:0" json:"price_change_h6"`
	PriceChangeH24                float64    `gorm:"column:price_change_h24;type:double;not null;default:0" json:"price_change_h24"`
	VolumeM5                      float64    `gorm:"column:volume_m5;type:double;not null;default:0" json:"volume_m5"`
	VolumeH1                      float64    `gorm:"column:volume_h1;type:double;not null;default:0" json:"volume_h1"`
	VolumeH6                      float64    `gorm:"column:volume_h6;type:double;not null;default:0" json:"volume_h6"`
	VolumeH24                     float64    `gorm:"column:volume_h24;type:double;not null;default:0;index:idx_pools_volume_h24" json:"volume_h24"`
	PoolFeePercentage             float64    `gorm:"column:pool_fee_percentage;type:double;not null;default:0" json:"pool_fee_percentage"`
	FeeUSDM5                      float64    `gorm:"column:fee_usd_m5;type:double;not null;default:0" json:"fee_usd_m5"`
	FeeUSDH1                      float64    `gorm:"column:fee_usd_h1;type:double;not null;default:0" json:"fee_usd_h1"`
	FeeUSDH6                      float64    `gorm:"column:fee_usd_h6;type:double;not null;default:0" json:"fee_usd_h6"`
	FeeUSDH24                     float64    `gorm:"column:fee_usd_h24;type:double;not null;default:0" json:"fee_usd_h24"`
	FeeAPRM5                      float64    `gorm:"column:fee_apr_m5;type:double;not null;default:0" json:"fee_apr_m5"`
	FeeAPRH1                      float64    `gorm:"column:fee_apr_h1;type:double;not null;default:0" json:"fee_apr_h1"`
	FeeAPRH6                      float64    `gorm:"column:fee_apr_h6;type:double;not null;default:0" json:"fee_apr_h6"`
	FeeAPRH24                     float64    `gorm:"column:fee_apr_h24;type:double;not null;default:0" json:"fee_apr_h24"`
	TransactionsH24Buys           uint32     `gorm:"column:transactions_h24_buys;type:int unsigned;not null;default:0" json:"transactions_h24_buys"`
	TransactionsH24Sells          uint32     `gorm:"column:transactions_h24_sells;type:int unsigned;not null;default:0" json:"transactions_h24_sells"`
	TransactionsH24Buyers         uint32     `gorm:"column:transactions_h24_buyers;type:int unsigned;not null;default:0" json:"transactions_h24_buyers"`
	TransactionsH24Sellers        uint32     `gorm:"column:transactions_h24_sellers;type:int unsigned;not null;default:0" json:"transactions_h24_sellers"`
	UpdatedAt                     time.Time  `gorm:"column:updated_at;type:datetime(3);not null;autoUpdateTime:milli;index:idx_pools_updated_at" json:"updated_at"`
}

func (Pool) TableName() string {
	return "pools"
}
