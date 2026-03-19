package models

import "time"

type MonitoredWallet struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Address        string    `gorm:"size:42;not null;uniqueIndex:uq_sm_address_chain" json:"address"`
	ChainID        int       `gorm:"not null;default:56;uniqueIndex:uq_sm_address_chain" json:"chain_id"`
	Source         string    `gorm:"size:30;not null" json:"source"`
	SourceContract *string   `gorm:"size:42" json:"source_contract"`
	Label          *string   `gorm:"size:100" json:"label"`
	IsActive       bool      `gorm:"not null;default:true" json:"is_active"`
	CreatedAt      time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
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
	IsActive         bool      `gorm:"not null;default:true" json:"is_active"`
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
	WalletAddress   string    `gorm:"size:42;not null;index:idx_sm_evt_wallet_chain" json:"wallet_address"`
	ChainID         int       `gorm:"not null;default:56;index:idx_sm_evt_wallet_chain" json:"chain_id"`
	Protocol        string    `gorm:"size:20;not null" json:"protocol"`
	EventType       string    `gorm:"size:10;not null;index:idx_sm_evt_type" json:"event_type"`
	Token0Address   string    `gorm:"size:42;not null" json:"token0_address"`
	Token1Address   string    `gorm:"size:42;not null" json:"token1_address"`
	Token0Symbol    string    `gorm:"size:20" json:"token0_symbol"`
	Token1Symbol    string    `gorm:"size:20" json:"token1_symbol"`
	Token0Amount    string    `gorm:"type:decimal(65,0);not null;default:0" json:"token0_amount"`
	Token1Amount    string    `gorm:"type:decimal(65,0);not null;default:0" json:"token1_amount"`
	Token0AmountUSD *string   `gorm:"type:decimal(20,4)" json:"token0_amount_usd"`
	Token1AmountUSD *string   `gorm:"type:decimal(20,4)" json:"token1_amount_usd"`
	TotalUSD        *string   `gorm:"type:decimal(20,4)" json:"total_usd"`
	PoolAddress     string    `gorm:"size:66;not null" json:"pool_address"`
	FeeTier         *int      `json:"fee_tier"`
	TickLower       *int      `json:"tick_lower"`
	TickUpper       *int      `json:"tick_upper"`
	NftTokenID      *uint64   `gorm:"index:idx_sm_evt_nft" json:"nft_token_id"`
	TxHash          string    `gorm:"size:66;not null;uniqueIndex:uq_sm_tx_log" json:"tx_hash"`
	BlockNumber     uint64    `gorm:"not null" json:"block_number"`
	LogIndex        int       `gorm:"not null;uniqueIndex:uq_sm_tx_log" json:"log_index"`
	TxTimestamp     time.Time `gorm:"not null;index:idx_sm_evt_ts" json:"tx_timestamp"`
	CreatedAt       time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
}

func (SmartMoneyLPEvent) TableName() string { return "sm_lp_events" }

type SmartMoneyLPPosition struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	WalletAddress string     `gorm:"size:42;not null;index:idx_sm_pos_wallet_status" json:"wallet_address"`
	ChainID       int        `gorm:"not null;default:56" json:"chain_id"`
	Protocol      string     `gorm:"size:20;not null" json:"protocol"`
	NftTokenID    uint64     `gorm:"not null;uniqueIndex:uq_sm_nft_chain" json:"nft_token_id"`
	PoolAddress   string     `gorm:"size:66;not null;index:idx_sm_pos_pool_status" json:"pool_address"`
	Token0Address string     `gorm:"size:42;not null" json:"token0_address"`
	Token1Address string     `gorm:"size:42;not null" json:"token1_address"`
	Token0Symbol  string     `gorm:"size:20" json:"token0_symbol"`
	Token1Symbol  string     `gorm:"size:20" json:"token1_symbol"`
	FeeTier       *int       `json:"fee_tier"`
	TickLower     *int       `json:"tick_lower"`
	TickUpper     *int       `json:"tick_upper"`
	Status        string     `gorm:"size:10;not null;default:open;index:idx_sm_pos_wallet_status;index:idx_sm_pos_pool_status;index:idx_sm_pos_status" json:"status"`
	OpenTxHash    string     `gorm:"size:66;not null" json:"open_tx_hash"`
	CloseTxHash   *string    `gorm:"size:66" json:"close_tx_hash"`
	OpenedAt      time.Time  `gorm:"not null;index:idx_sm_pos_opened" json:"opened_at"`
	ClosedAt      *time.Time `json:"closed_at"`
	UpdatedAt     time.Time  `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

func (SmartMoneyLPPosition) TableName() string { return "sm_lp_positions" }
