package web_server

import (
	"encoding/json"
	"time"
)

// HotPoolResponse is retained as the search-pool response model.
// The pool catalog and search results reuse the same response shape.
type HotPoolResponse struct {
	Chain                   string          `json:"chain,omitempty"`
	ProtocolVersion         string          `json:"protocol_version"`
	PoolAddress             string          `json:"pool_address"`
	Dex                     string          `json:"dex"`
	FactoryName             string          `json:"factory_name"`
	FactoryAddress          string          `json:"factory_address,omitempty"`
	TradingPair             string          `json:"trading_pair"`
	FeePercentage           float64         `json:"fee_percentage"`
	FeeRate                 float64         `json:"fee_rate"`
	FeeTier                 int             `json:"fee_tier,omitempty"`
	TransactionCount        uint32          `json:"transaction_count"`
	TotalFees               float64         `json:"total_fees"`
	TotalVolume             float64         `json:"total_volume"`
	CurrentPoolValue        float64         `json:"current_pool_value"`
	PriceDisplay            string          `json:"price_display"`
	UpdatedAt               time.Time       `json:"updated_at"`
	LastSwapAt              *time.Time      `json:"last_swap_at,omitempty"`
	Token0Address           string          `json:"token0_address"`
	Token1Address           string          `json:"token1_address"`
	Token0Symbol            string          `json:"token0_symbol,omitempty"`
	Token1Symbol            string          `json:"token1_symbol,omitempty"`
	Token0Name              string          `json:"token0_name,omitempty"`
	Token1Name              string          `json:"token1_name,omitempty"`
	Token0Decimals          int             `json:"token0_decimals,omitempty"`
	Token1Decimals          int             `json:"token1_decimals,omitempty"`
	StableCoinSymbol        string          `json:"stable_coin_symbol,omitempty"`
	HookAddress             string          `json:"hook_address,omitempty"`
	CurrentToken0Balance    float64         `json:"current_token0_balance,omitempty"`
	CurrentToken1Balance    float64         `json:"current_token1_balance,omitempty"`
	CurrentTokenPrice       float64         `json:"current_token_price,omitempty"`
	PricedTokenAddress      string          `json:"priced_token_address,omitempty"`
	CurrentTokenTotalSupply float64         `json:"current_token_total_supply,omitempty"`
	CurrentTokenFDVUSD      float64         `json:"current_token_fdv_usd,omitempty"`
	TokenSupplyUpdatedAt    *time.Time      `json:"token_supply_updated_at,omitempty"`
	TickSpacing             *int            `json:"tick_spacing,omitempty"`
	CurrentTick             int             `json:"current_tick,omitempty"`
	CurrentSqrtPriceX96     string          `json:"current_sqrt_price_x96,omitempty"`
	CurrentLiquidity        string          `json:"current_liquidity,omitempty"`
	StableCoinPosition      string          `json:"stable_coin_position,omitempty"`
	MetricTrends            json.RawMessage `json:"metricTrends,omitempty"`
	UniqueWallets           uint32          `json:"unique_wallets,omitempty"`
	TopWalletVolPct         float64         `json:"top_wallet_vol_pct,omitempty"`
	ActiveTickCount         int             `json:"activeTickCount,omitempty"`
	ActiveLiquidityUSD      float64         `json:"activeLiquidityUSD,omitempty"`
	ActiveLiquidityRatio    float64         `json:"activeLiquidityRatio,omitempty"`
	LiquidityTicks          json.RawMessage `json:"liquidityTicks,omitempty"`
	LiquidityCurrentTick    int             `json:"liquidityCurrentTick,omitempty"`
	LiquidityTickSpacing    int             `json:"liquidityTickSpacing,omitempty"`
	Badges                  json.RawMessage `json:"badges,omitempty"`
	DisplayTokenAddress     string          `json:"display_token_address,omitempty"`
	DisplayTokenSymbol      string          `json:"display_token_symbol,omitempty"`
	DisplayTokenName        string          `json:"display_token_name,omitempty"`
	DisplayTokenLogoURL     string          `json:"display_token_logo_url,omitempty"`
	TokenRisk               *TokenRiskInfo  `json:"token_risk,omitempty"`
	TotalFees24h            float64         `json:"total_fees_24h,omitempty"`
	TotalVolume24h          float64         `json:"total_volume_24h,omitempty"`
	TransactionCount24h     uint32          `json:"transaction_count_24h,omitempty"`
}
