package models

import "time"

// UserAssetDailySnapshot stores one daily asset snapshot for a user.
// wallet_id=0 and chain="" represent the aggregated user view.
type UserAssetDailySnapshot struct {
	ID uint `gorm:"primaryKey" json:"id"`

	UserID      uint   `gorm:"not null;index;uniqueIndex:idx_user_asset_day" json:"user_id"`
	WalletID    uint   `gorm:"not null;default:0;uniqueIndex:idx_user_asset_day" json:"wallet_id"`
	Chain       string `gorm:"size:16;not null;default:'';uniqueIndex:idx_user_asset_day" json:"chain"`
	SnapshotDay string `gorm:"size:10;not null;uniqueIndex:idx_user_asset_day" json:"snapshot_day"`

	WalletUSD   float64   `gorm:"type:decimal(20,4);not null;default:0" json:"wallet_usd"`
	PositionUSD float64   `gorm:"type:decimal(20,4);not null;default:0" json:"position_usd"`
	FeeUSD      float64   `gorm:"type:decimal(20,4);not null;default:0" json:"fee_usd"`
	TotalUSD    float64   `gorm:"type:decimal(20,4);not null;default:0" json:"total_usd"`
	CapturedAt  time.Time `gorm:"not null;index" json:"captured_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (UserAssetDailySnapshot) TableName() string {
	return "user_asset_daily_snapshots"
}

// UserLPDailyStat stores one daily realized LP statistic row for a user.
// wallet_id=0 and chain="" represent the aggregated user view.
type UserLPDailyStat struct {
	ID uint `gorm:"primaryKey" json:"id"`

	UserID   uint   `gorm:"not null;index;uniqueIndex:idx_user_lp_day" json:"user_id"`
	WalletID uint   `gorm:"not null;default:0;uniqueIndex:idx_user_lp_day" json:"wallet_id"`
	Chain    string `gorm:"size:16;not null;default:'';uniqueIndex:idx_user_lp_day" json:"chain"`
	StatDay  string `gorm:"size:10;not null;uniqueIndex:idx_user_lp_day" json:"stat_day"`

	RealizedPnLUSD float64   `gorm:"column:realized_pnl_usd;type:decimal(20,4);not null;default:0" json:"realized_pnl_usd"`
	ClosedCount    int       `gorm:"not null;default:0" json:"closed_count"`
	WinCount       int       `gorm:"not null;default:0" json:"win_count"`
	LossCount      int       `gorm:"not null;default:0" json:"loss_count"`
	BreakEvenCount int       `gorm:"not null;default:0" json:"break_even_count"`
	CapturedAt     time.Time `gorm:"not null;index" json:"captured_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (UserLPDailyStat) TableName() string {
	return "user_lp_daily_stats"
}

const (
	SmartMoneyTransferDirectionIn  = "in"
	SmartMoneyTransferDirectionOut = "out"
	SmartMoneyTransferAssetNative  = "native"
	SmartMoneyTransferAssetERC20   = "erc20"
)

// UserWalletTransferEvent stores one persisted normal transfer event for a user wallet.
type UserWalletTransferEvent struct {
	ID uint `gorm:"primaryKey" json:"id"`

	UserID        uint      `gorm:"not null;index:idx_user_wallet_transfer_user_time,priority:1" json:"user_id"`
	WalletID      uint      `gorm:"not null;index:idx_user_wallet_transfer_wallet_time,priority:1;uniqueIndex:uq_user_wallet_transfer_event,priority:1" json:"wallet_id"`
	WalletAddress string    `gorm:"size:42;not null;index:idx_user_wallet_transfer_wallet_time,priority:2" json:"wallet_address"`
	Chain         string    `gorm:"size:16;not null;default:'';index:idx_user_wallet_transfer_wallet_time,priority:3;index:idx_user_wallet_transfer_user_time,priority:2;uniqueIndex:uq_user_wallet_transfer_event,priority:2" json:"chain"`
	Direction     string    `gorm:"size:8;not null;uniqueIndex:uq_user_wallet_transfer_event,priority:5" json:"direction"`
	AssetType     string    `gorm:"size:16;not null" json:"asset_type"`
	TokenAddress  string    `gorm:"size:42;not null;default:''" json:"token_address"`
	TokenSymbol   string    `gorm:"size:32" json:"token_symbol"`
	TokenDecimals int       `gorm:"not null;default:0" json:"token_decimals"`
	AmountRaw     string    `gorm:"type:varchar(78);not null;default:'0'" json:"amount_raw"`
	AmountDecimal float64   `gorm:"type:decimal(36,18);not null;default:0" json:"amount_decimal"`
	AmountUSD     float64   `gorm:"type:decimal(20,4);not null;default:0" json:"amount_usd"`
	TxHash        string    `gorm:"size:66;not null;uniqueIndex:uq_user_wallet_transfer_event,priority:3" json:"tx_hash"`
	BlockNumber   uint64    `gorm:"not null" json:"block_number"`
	LogIndex      int       `gorm:"not null;default:-1;uniqueIndex:uq_user_wallet_transfer_event,priority:4" json:"log_index"`
	TxTimestamp   time.Time `gorm:"not null;index:idx_user_wallet_transfer_wallet_time,priority:4;index:idx_user_wallet_transfer_user_time,priority:3" json:"tx_timestamp"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (UserWalletTransferEvent) TableName() string {
	return "user_wallet_transfer_events"
}

// SmartMoneyWalletTransferEvent stores one persisted normal transfer event for a monitored smart money wallet.
type SmartMoneyWalletTransferEvent struct {
	ID uint `gorm:"primaryKey" json:"id"`

	WalletAddress string    `gorm:"size:42;not null;index:idx_sm_wallet_transfer_wallet_time,priority:1;uniqueIndex:uq_sm_wallet_transfer_event,priority:1" json:"wallet_address"`
	ChainID       int       `gorm:"not null;default:56;index:idx_sm_wallet_transfer_wallet_time,priority:2;uniqueIndex:uq_sm_wallet_transfer_event,priority:2" json:"chain_id"`
	Direction     string    `gorm:"size:8;not null;uniqueIndex:uq_sm_wallet_transfer_event,priority:5" json:"direction"`
	AssetType     string    `gorm:"size:16;not null" json:"asset_type"`
	TokenAddress  string    `gorm:"size:42;not null;default:''" json:"token_address"`
	TokenSymbol   string    `gorm:"size:32" json:"token_symbol"`
	TokenDecimals int       `gorm:"not null;default:0" json:"token_decimals"`
	AmountRaw     string    `gorm:"type:varchar(78);not null;default:'0'" json:"amount_raw"`
	AmountDecimal float64   `gorm:"type:decimal(36,18);not null;default:0" json:"amount_decimal"`
	AmountUSD     float64   `gorm:"type:decimal(20,4);not null;default:0" json:"amount_usd"`
	TxHash        string    `gorm:"size:66;not null;uniqueIndex:uq_sm_wallet_transfer_event,priority:3" json:"tx_hash"`
	BlockNumber   uint64    `gorm:"not null" json:"block_number"`
	LogIndex      int       `gorm:"not null;default:-1;uniqueIndex:uq_sm_wallet_transfer_event,priority:4" json:"log_index"`
	TxTimestamp   time.Time `gorm:"not null;index:idx_sm_wallet_transfer_wallet_time,priority:3" json:"tx_timestamp"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (SmartMoneyWalletTransferEvent) TableName() string {
	return "sm_wallet_transfer_events"
}

// SmartMoneyWalletDailySnapshot stores one daily recognized-asset snapshot for a smart money wallet.
type SmartMoneyWalletDailySnapshot struct {
	ID uint `gorm:"primaryKey" json:"id"`

	WalletAddress string `gorm:"size:42;not null;uniqueIndex:idx_sm_wallet_asset_day" json:"wallet_address"`
	ChainID       int    `gorm:"not null;default:56;uniqueIndex:idx_sm_wallet_asset_day" json:"chain_id"`
	SnapshotDay   string `gorm:"size:10;not null;uniqueIndex:idx_sm_wallet_asset_day" json:"snapshot_day"`

	NativeUSD         float64   `gorm:"type:decimal(20,4);not null;default:0" json:"native_usd"`
	StableUSD         float64   `gorm:"type:decimal(20,4);not null;default:0" json:"stable_usd"`
	TrackedTokenUSD   float64   `gorm:"type:decimal(20,4);not null;default:0" json:"tracked_token_usd"`
	OpenLPUSD         float64   `gorm:"type:decimal(20,4);not null;default:0" json:"open_lp_usd"`
	TotalUSD          float64   `gorm:"type:decimal(20,4);not null;default:0" json:"total_usd"`
	TrackedTokenCount int       `gorm:"not null;default:0" json:"tracked_token_count"`
	HasTransferIn     bool      `gorm:"not null;default:false" json:"has_transfer_in"`
	HasTransferOut    bool      `gorm:"not null;default:false" json:"has_transfer_out"`
	TransferInCount   int       `gorm:"not null;default:0" json:"transfer_in_count"`
	TransferOutCount  int       `gorm:"not null;default:0" json:"transfer_out_count"`
	TransferInUSD     float64   `gorm:"type:decimal(20,4);not null;default:0" json:"transfer_in_usd"`
	TransferOutUSD    float64   `gorm:"type:decimal(20,4);not null;default:0" json:"transfer_out_usd"`
	CapturedAt        time.Time `gorm:"not null;index" json:"captured_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SmartMoneyWalletDailySnapshot) TableName() string {
	return "sm_wallet_daily_snapshots"
}

// SmartMoneyLPDailyStat stores one daily smart money LP statistic row for a wallet.
type SmartMoneyLPDailyStat struct {
	ID uint `gorm:"primaryKey" json:"id"`

	WalletAddress string `gorm:"size:42;not null;uniqueIndex:idx_sm_lp_day" json:"wallet_address"`
	ChainID       int    `gorm:"not null;default:56;uniqueIndex:idx_sm_lp_day" json:"chain_id"`
	StatDay       string `gorm:"size:10;not null;uniqueIndex:idx_sm_lp_day" json:"stat_day"`

	EstimatedRealizedPnLUSD float64   `gorm:"column:estimated_realized_pnl_usd;type:decimal(20,4);not null;default:0" json:"estimated_realized_pnl_usd"`
	MatchedCostUSD          float64   `gorm:"column:matched_cost_usd;type:decimal(20,4);not null;default:0" json:"matched_cost_usd"`
	MatchedRemoveCount      int       `gorm:"not null;default:0" json:"matched_remove_count"`
	UnmatchedRemoveCount    int       `gorm:"not null;default:0" json:"unmatched_remove_count"`
	AddCount                int       `gorm:"not null;default:0" json:"add_count"`
	RemoveCount             int       `gorm:"not null;default:0" json:"remove_count"`
	ActivePoolCount         int       `gorm:"not null;default:0" json:"active_pool_count"`
	CapturedAt              time.Time `gorm:"not null;index" json:"captured_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SmartMoneyLPDailyStat) TableName() string {
	return "sm_lp_daily_stats"
}
