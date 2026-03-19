package models

import "time"

// Pool stores the current pool catalog used by /api/pools.
// Legacy compatibility columns are retained while the latest PoolM top-fees/5
// payload is persisted in dedicated fields and JSON blobs.
type Pool struct {
	ID      string `gorm:"column:id;type:varchar(128);primaryKey" json:"id"`
	Type    string `gorm:"column:type;type:varchar(32);not null;default:''" json:"type"`
	Address string `gorm:"column:address;type:varchar(128);not null;index:idx_pools_address" json:"address"`

	// Legacy compatibility fields still used by existing code paths.
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

	// PoolM top-fees/5 source metadata.
	Chain                       string     `gorm:"column:chain;type:varchar(32);not null;default:'';index:idx_pools_chain" json:"chain"`
	ProtocolVersion             string     `gorm:"column:protocol_version;type:varchar(16);not null;default:''" json:"protocol_version"`
	FactoryName                 string     `gorm:"column:factory_name;type:varchar(64);not null;default:''" json:"factory_name"`
	FactoryAddress              string     `gorm:"column:factory_address;type:varchar(128);not null;default:''" json:"factory_address"`
	Token0Symbol                string     `gorm:"column:token0_symbol;type:varchar(64);not null;default:''" json:"token0_symbol"`
	Token1Symbol                string     `gorm:"column:token1_symbol;type:varchar(64);not null;default:''" json:"token1_symbol"`
	Token0Name                  string     `gorm:"column:token0_name;type:varchar(255);not null;default:''" json:"token0_name"`
	Token1Name                  string     `gorm:"column:token1_name;type:varchar(255);not null;default:''" json:"token1_name"`
	Token0Decimals              int        `gorm:"column:token0_decimals;type:int;not null;default:0" json:"token0_decimals"`
	Token1Decimals              int        `gorm:"column:token1_decimals;type:int;not null;default:0" json:"token1_decimals"`
	StableCoinSymbol            string     `gorm:"column:stable_coin_symbol;type:varchar(64);not null;default:''" json:"stable_coin_symbol"`
	PoolMFeeRate                int        `gorm:"column:poolm_fee_rate;type:int;not null;default:0" json:"poolm_fee_rate"`
	HookAddress                 string     `gorm:"column:hook_address;type:varchar(128);not null;default:''" json:"hook_address"`
	TransactionCount            uint32     `gorm:"column:transaction_count;type:int unsigned;not null;default:0" json:"transaction_count"`
	TotalFees                   float64    `gorm:"column:total_fees;type:double;not null;default:0;index:idx_pools_total_fees"`
	TotalVolume                 float64    `gorm:"column:total_volume;type:double;not null;default:0;index:idx_pools_total_volume"`
	CurrentPoolValue            float64    `gorm:"column:current_pool_value;type:double;not null;default:0;index:idx_pools_current_pool_value"`
	CurrentToken0Balance        float64    `gorm:"column:current_token0_balance;type:double;not null;default:0" json:"current_token0_balance"`
	CurrentToken1Balance        float64    `gorm:"column:current_token1_balance;type:double;not null;default:0" json:"current_token1_balance"`
	CurrentTokenPrice           float64    `gorm:"column:current_token_price;type:double;not null;default:0" json:"current_token_price"`
	PricedTokenAddress          string     `gorm:"column:priced_token_address;type:varchar(128);not null;default:''" json:"priced_token_address"`
	CurrentTokenTotalSupply     float64    `gorm:"column:current_token_total_supply;type:double;not null;default:0" json:"current_token_total_supply"`
	CurrentTokenFDVUSD          float64    `gorm:"column:current_token_fdv_usd;type:double;not null;default:0" json:"current_token_fdv_usd"`
	TokenSupplyUpdatedAt        *time.Time `gorm:"column:token_supply_updated_at;type:datetime(3)" json:"token_supply_updated_at,omitempty"`
	PriceDisplay                string     `gorm:"column:price_display;type:varchar(255);not null;default:''" json:"price_display"`
	LastSwapAt                  *time.Time `gorm:"column:last_swap_at;type:datetime(3)" json:"last_swap_at,omitempty"`
	TickSpacing                 *int       `gorm:"column:tick_spacing;type:int" json:"tick_spacing,omitempty"`
	CurrentTick                 int        `gorm:"column:current_tick;type:int;not null;default:0" json:"current_tick"`
	CurrentSqrtPriceX96         string     `gorm:"column:current_sqrt_price_x96;type:varchar(128);not null;default:''" json:"current_sqrt_price_x96"`
	CurrentLiquidity            string     `gorm:"column:current_liquidity;type:varchar(128);not null;default:''" json:"current_liquidity"`
	StableCoinPosition          string     `gorm:"column:stable_coin_position;type:varchar(16);not null;default:''" json:"stable_coin_position"`
	UniqueWallets               uint32     `gorm:"column:unique_wallets;type:int unsigned;not null;default:0" json:"unique_wallets"`
	TopWalletVolPct             float64    `gorm:"column:top_wallet_vol_pct;type:double;not null;default:0" json:"top_wallet_vol_pct"`
	ActiveTickCount             int        `gorm:"column:active_tick_count;type:int;not null;default:0" json:"active_tick_count"`
	ActiveLiquidityUSD          float64    `gorm:"column:active_liquidity_usd;type:double;not null;default:0" json:"active_liquidity_usd"`
	ActiveLiquidityRatio        float64    `gorm:"column:active_liquidity_ratio;type:double;not null;default:0" json:"active_liquidity_ratio"`
	LiquidityCurrentTick        int        `gorm:"column:liquidity_current_tick;type:int;not null;default:0" json:"liquidity_current_tick"`
	LiquidityTickSpacing        int        `gorm:"column:liquidity_tick_spacing;type:int;not null;default:0" json:"liquidity_tick_spacing"`
	SourceTimeframe             string     `gorm:"column:source_timeframe;type:varchar(64);not null;default:''" json:"source_timeframe"`
	SourceRequestedLimit        int        `gorm:"column:source_requested_limit;type:int;not null;default:0" json:"source_requested_limit"`
	SourceRequestedChain        string     `gorm:"column:source_requested_chain;type:varchar(32);not null;default:''" json:"source_requested_chain"`
	SourceTotalPools            int        `gorm:"column:source_total_pools;type:int;not null;default:0" json:"source_total_pools"`
	SourceRequestedProtocolJSON string     `gorm:"column:source_requested_protocol_json;type:json" json:"source_requested_protocol_json"`
	SourceRequestedDexJSON      string     `gorm:"column:source_requested_dex_json;type:json" json:"source_requested_dex_json"`
	MetricTrendsIndexJSON       string     `gorm:"column:metric_trends_index_json;type:json" json:"metric_trends_index_json"`
	LiquidityTicksIndexJSON     string     `gorm:"column:liquidity_ticks_index_json;type:json" json:"liquidity_ticks_index_json"`
	MetricTrendsJSON            string     `gorm:"column:metric_trends_json;type:json" json:"metric_trends_json"`
	LiquidityTicksJSON          string     `gorm:"column:liquidity_ticks_json;type:json" json:"liquidity_ticks_json"`
	BadgesJSON                  string     `gorm:"column:badges_json;type:json" json:"badges_json"`
	SourcePayloadJSON           string     `gorm:"column:source_payload_json;type:json" json:"source_payload_json"`

	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime(3);not null;autoUpdateTime:milli;index:idx_pools_updated_at" json:"updated_at"`
}

func (Pool) TableName() string {
	return "pools"
}
