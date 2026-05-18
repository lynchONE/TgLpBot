package models

import "time"

type MonitoredWallet struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Address        string    `gorm:"size:42;not null;uniqueIndex:uq_sm_address_chain" json:"address"`
	ChainID        int       `gorm:"not null;default:56;uniqueIndex:uq_sm_address_chain" json:"chain_id"`
	Source         string    `gorm:"size:30;not null;index:idx_sm_wallet_source_active_created,priority:1" json:"source"`
	SourceContract *string   `gorm:"size:42" json:"source_contract"`
	Label          *string   `gorm:"size:100" json:"label"`
	AvatarURL      *string   `gorm:"size:512" json:"avatar_url"`
	IsActive       bool      `gorm:"not null;default:true;index:idx_sm_wallet_active_created,priority:1;index:idx_sm_wallet_source_active_created,priority:2" json:"is_active"`
	CreatedAt      time.Time `gorm:"not null;autoCreateTime;index:idx_sm_wallet_active_created,priority:2;index:idx_sm_wallet_source_active_created,priority:3" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (MonitoredWallet) TableName() string { return "monitored_wallets" }

type WatchContract struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ContractAddress  string    `gorm:"size:42;not null;uniqueIndex:uq_sm_contract_chain" json:"contract_address"`
	ChainID          int       `gorm:"not null;default:56;uniqueIndex:uq_sm_contract_chain" json:"chain_id"`
	Protocol         string    `gorm:"size:50;not null" json:"protocol"`
	Description      *string   `gorm:"type:text" json:"description"`
	LastScannedBlock uint64    `gorm:"not null;default:0" json:"last_scanned_block"`
	IsActive         bool      `gorm:"not null;default:true;index:idx_sm_watch_contract_active" json:"is_active"`
	CreatedAt        time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
}

func (WatchContract) TableName() string { return "watch_contracts" }

type SmartMoneyScanState struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ChainID          int       `gorm:"not null;default:56;uniqueIndex:uq_sm_scan_chain" json:"chain_id"`
	LastScannedBlock uint64    `gorm:"not null;default:0" json:"last_scanned_block"`
	UpdatedAt        time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (SmartMoneyScanState) TableName() string { return "sm_scan_states" }

type SmartMoneyLPEvent struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	WalletAddress   string    `gorm:"size:42;not null;index:idx_sm_evt_wallet_chain;index:idx_sm_evt_wallet_chain_time,priority:1;index:idx_sm_evt_wallet_chain_type_time,priority:1" json:"wallet_address"`
	ChainID         int       `gorm:"not null;default:56;index:idx_sm_evt_wallet_chain;index:idx_sm_evt_wallet_chain_time,priority:2;index:idx_sm_evt_wallet_chain_type_time,priority:2;index:idx_sm_evt_chain_pool_time,priority:1;index:idx_sm_evt_chain_type_time,priority:1;index:idx_sm_evt_type_chain_protocol_nft,priority:2;index:idx_sm_evt_chain_protocol_nft_time,priority:1" json:"chain_id"`
	Protocol        string    `gorm:"size:20;not null;index:idx_sm_evt_type_chain_protocol_nft,priority:3;index:idx_sm_evt_chain_protocol_nft_time,priority:2" json:"protocol"`
	EventType       string    `gorm:"size:10;not null;index:idx_sm_evt_type;index:idx_sm_evt_wallet_chain_type_time,priority:3;index:idx_sm_evt_chain_type_time,priority:2;index:idx_sm_evt_type_chain_protocol_nft,priority:1" json:"event_type"`
	Token0Address   string    `gorm:"size:42;not null" json:"token0_address"`
	Token1Address   string    `gorm:"size:42;not null" json:"token1_address"`
	Token0Symbol    string    `gorm:"size:20" json:"token0_symbol"`
	Token1Symbol    string    `gorm:"size:20" json:"token1_symbol"`
	LiquidityDelta  string    `gorm:"type:decimal(65,0);not null;default:0" json:"liquidity_delta"`
	Token0Amount    string    `gorm:"type:decimal(65,0);not null;default:0" json:"token0_amount"`
	Token1Amount    string    `gorm:"type:decimal(65,0);not null;default:0" json:"token1_amount"`
	Token0AmountUSD *string   `gorm:"type:decimal(20,4)" json:"token0_amount_usd"`
	Token1AmountUSD *string   `gorm:"type:decimal(20,4)" json:"token1_amount_usd"`
	TotalUSD        *string   `gorm:"type:decimal(20,4)" json:"total_usd"`
	PoolAddress     string    `gorm:"size:66;not null;index:idx_sm_evt_chain_pool_time,priority:2" json:"pool_address"`
	FeeTier         *int      `json:"fee_tier"`
	TickLower       *int      `json:"tick_lower"`
	TickUpper       *int      `json:"tick_upper"`
	NftTokenID      *uint64   `gorm:"index:idx_sm_evt_nft;index:idx_sm_evt_type_chain_protocol_nft,priority:4;index:idx_sm_evt_chain_protocol_nft_time,priority:3" json:"nft_token_id"`
	TxHash          string    `gorm:"size:66;not null;uniqueIndex:uq_sm_tx_log" json:"tx_hash"`
	BlockNumber     uint64    `gorm:"not null" json:"block_number"`
	LogIndex        int       `gorm:"not null;uniqueIndex:uq_sm_tx_log" json:"log_index"`
	TxTimestamp     time.Time `gorm:"not null;index:idx_sm_evt_ts;index:idx_sm_evt_wallet_chain_time,priority:3;index:idx_sm_evt_wallet_chain_type_time,priority:4;index:idx_sm_evt_chain_pool_time,priority:3;index:idx_sm_evt_chain_type_time,priority:3;index:idx_sm_evt_chain_protocol_nft_time,priority:4" json:"tx_timestamp"`
	CreatedAt       time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
}

func (SmartMoneyLPEvent) TableName() string { return "sm_lp_events" }

type SmartMoneyLPPosition struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	WalletAddress  string     `gorm:"size:42;not null;index:idx_sm_pos_wallet_status;index:idx_sm_pos_wallet_chain_status_opened,priority:1" json:"wallet_address"`
	ChainID        int        `gorm:"not null;default:56;uniqueIndex:uq_sm_nft_chain_protocol,priority:1;index:idx_sm_pos_wallet_chain_status_opened,priority:2;index:idx_sm_pos_chain_protocol_nft,priority:1" json:"chain_id"`
	Protocol       string     `gorm:"size:20;not null;uniqueIndex:uq_sm_nft_chain_protocol,priority:2;index:idx_sm_pos_chain_protocol_nft,priority:2" json:"protocol"`
	NftTokenID     uint64     `gorm:"not null;uniqueIndex:uq_sm_nft_chain_protocol,priority:3;index:idx_sm_pos_chain_protocol_nft,priority:3" json:"nft_token_id"`
	PoolAddress    string     `gorm:"size:66;not null;index:idx_sm_pos_pool_status;index:idx_sm_pos_pool_status_opened,priority:1;index:idx_sm_pos_status_opened_pool,priority:3" json:"pool_address"`
	Token0Address  string     `gorm:"size:42;not null" json:"token0_address"`
	Token1Address  string     `gorm:"size:42;not null" json:"token1_address"`
	Token0Symbol   string     `gorm:"size:20" json:"token0_symbol"`
	Token1Symbol   string     `gorm:"size:20" json:"token1_symbol"`
	FeeTier        *int       `json:"fee_tier"`
	TickLower      *int       `json:"tick_lower"`
	TickUpper      *int       `json:"tick_upper"`
	MetadataStatus string     `gorm:"size:32;not null;default:'';index:idx_sm_pos_metadata_status" json:"metadata_status"`
	MetadataError  string     `gorm:"type:text" json:"metadata_error"`
	Status         string     `gorm:"size:10;not null;default:open;index:idx_sm_pos_wallet_status;index:idx_sm_pos_pool_status;index:idx_sm_pos_status;index:idx_sm_pos_wallet_chain_status_opened,priority:3;index:idx_sm_pos_pool_status_opened,priority:2;index:idx_sm_pos_status_opened_pool,priority:1;index:idx_sm_pos_status_closed,priority:1" json:"status"`
	OpenTxHash     string     `gorm:"size:66;not null" json:"open_tx_hash"`
	CloseTxHash    *string    `gorm:"size:66" json:"close_tx_hash"`
	OpenedAt       time.Time  `gorm:"not null;index:idx_sm_pos_opened;index:idx_sm_pos_wallet_chain_status_opened,priority:4;index:idx_sm_pos_pool_status_opened,priority:3;index:idx_sm_pos_status_opened_pool,priority:2" json:"opened_at"`
	ClosedAt       *time.Time `gorm:"index:idx_sm_pos_status_closed,priority:2" json:"closed_at"`
	UpdatedAt      time.Time  `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (SmartMoneyLPPosition) TableName() string { return "sm_lp_positions" }

type SmartMoneyActivePosition struct {
	ID                     uint       `gorm:"primaryKey" json:"id"`
	PositionRef            string     `gorm:"size:255;not null;uniqueIndex:uq_sm_active_position_ref" json:"position_ref"`
	WalletAddress          string     `gorm:"size:42;not null;index:idx_sm_active_wallet_status" json:"wallet_address"`
	ChainID                int        `gorm:"not null;default:56;index:idx_sm_active_wallet_status;index:idx_sm_active_pool_status;index:idx_sm_active_chain_protocol_nft,priority:1" json:"chain_id"`
	Protocol               string     `gorm:"size:20;not null;index:idx_sm_active_protocol;index:idx_sm_active_chain_protocol_nft,priority:2" json:"protocol"`
	NftTokenID             uint64     `gorm:"not null;default:0;index:idx_sm_active_nft;index:idx_sm_active_chain_protocol_nft,priority:3" json:"nft_token_id"`
	PoolAddress            string     `gorm:"size:66;not null;index:idx_sm_active_pool_status" json:"pool_address"`
	PositionManagerAddress string     `gorm:"size:66" json:"position_manager_address"`
	PoolManagerAddress     string     `gorm:"size:66" json:"pool_manager_address"`
	StateViewAddress       string     `gorm:"size:66" json:"state_view_address"`
	Token0Address          string     `gorm:"size:42;not null" json:"token0_address"`
	Token1Address          string     `gorm:"size:42;not null" json:"token1_address"`
	Token0Symbol           string     `gorm:"size:20" json:"token0_symbol"`
	Token1Symbol           string     `gorm:"size:20" json:"token1_symbol"`
	Token0Decimals         int        `gorm:"not null;default:0" json:"token0_decimals"`
	Token1Decimals         int        `gorm:"not null;default:0" json:"token1_decimals"`
	FeeTier                *int       `json:"fee_tier"`
	TickLower              *int       `json:"tick_lower"`
	TickUpper              *int       `json:"tick_upper"`
	TickSpacing            int        `gorm:"not null;default:0" json:"tick_spacing"`
	CurrentLiquidity       string     `gorm:"type:decimal(65,0);not null;default:0" json:"current_liquidity"`
	EntryAmount0           string     `gorm:"type:decimal(65,0);not null;default:0" json:"entry_amount0"`
	EntryAmount1           string     `gorm:"type:decimal(65,0);not null;default:0" json:"entry_amount1"`
	EntryTotalUSD          *string    `gorm:"type:decimal(20,4)" json:"entry_total_usd"`
	NetAmount0             string     `gorm:"type:decimal(65,0);not null;default:0" json:"net_amount0"`
	NetAmount1             string     `gorm:"type:decimal(65,0);not null;default:0" json:"net_amount1"`
	NetTotalUSD            *string    `gorm:"type:decimal(20,4)" json:"net_total_usd"`
	FeeAmount0             string     `gorm:"type:decimal(65,0);not null;default:0" json:"fee_amount0"`
	FeeAmount1             string     `gorm:"type:decimal(65,0);not null;default:0" json:"fee_amount1"`
	FeeUSD                 *string    `gorm:"type:decimal(20,4)" json:"fee_usd"`
	FeeStatus              string     `gorm:"size:20;not null;default:''" json:"fee_status"`
	FeeUpdatedAt           *time.Time `json:"fee_updated_at"`
	IsActive               bool       `gorm:"not null;default:true;index:idx_sm_active_wallet_status;index:idx_sm_active_pool_status;index:idx_sm_active_status" json:"is_active"`
	OpenedAt               time.Time  `gorm:"not null;index:idx_sm_active_opened" json:"opened_at"`
	LastAddAt              *time.Time `json:"last_add_at"`
	LastRemoveAt           *time.Time `json:"last_remove_at"`
	ClosedAt               *time.Time `json:"closed_at"`
	UpdatedAt              time.Time  `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (SmartMoneyActivePosition) TableName() string { return "sm_lp_active_positions" }
