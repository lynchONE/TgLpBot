package assets

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/timeutil"
	"TgLpBot/service/pricing"
	"TgLpBot/service/realtime"
	sm "TgLpBot/service/smart_money"
	"TgLpBot/service/wallet"
	"context"
	"fmt"
	"log"
	"math"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"gorm.io/gorm/clause"
)

const (
	aggregateWalletID  = uint(0)
	defaultHistoryDays = 30
)

type Service struct {
	walletService   *wallet.WalletService
	realtimeService *realtime.RealtimePositionsService
	priceService    *pricing.TokenPriceService
	smRepo          *sm.Repository

	stopOnce sync.Once
	stopCh   chan struct{}
	ticker   *time.Ticker

	decimalsMu    sync.RWMutex
	decimalsCache map[string]int
}

func NewService() *Service {
	return &Service{
		walletService:   wallet.NewWalletService(),
		realtimeService: realtime.NewRealtimePositionsService(),
		priceService:    pricing.NewTokenPriceService(),
		smRepo:          sm.NewRepository(),
		stopCh:          make(chan struct{}),
		ticker:          time.NewTicker(time.Minute),
		decimalsCache:   make(map[string]int),
	}
}

func (s *Service) Start() {
	if s == nil {
		return
	}
	go s.runScheduler()
}

func (s *Service) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.ticker != nil {
			s.ticker.Stop()
		}
		close(s.stopCh)
	})
}

func (s *Service) RunDailyAggregation(day time.Time) error {
	if s == nil {
		return fmt.Errorf("asset service not initialized")
	}
	day = dayStart(day)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := s.captureUserAssetSnapshots(ctx, day); err != nil {
		return fmt.Errorf("capture user asset snapshots: %w", err)
	}
	if err := s.captureUserLPDailyStats(ctx, day); err != nil {
		return fmt.Errorf("capture user lp daily stats: %w", err)
	}
	if err := s.captureSmartMoneyWalletSnapshots(ctx, day); err != nil {
		return fmt.Errorf("capture smart money wallet snapshots: %w", err)
	}
	if err := s.captureSmartMoneyLPDailyStats(ctx, day); err != nil {
		return fmt.Errorf("capture smart money lp daily stats: %w", err)
	}
	return nil
}

func (s *Service) runScheduler() {
	// Wait for blockchain clients to be ready before first aggregation
	time.Sleep(90 * time.Second)
	lastCompleted := ""
	for {
		s.tryAggregatePreviousDay(&lastCompleted)
		select {
		case <-s.stopCh:
			return
		case <-s.ticker.C:
		}
	}
}

func (s *Service) tryAggregatePreviousDay(lastCompleted *string) {
	now := timeutil.Now()
	day := dayStart(now)
	if now.Before(day.Add(5 * time.Minute)) {
		return
	}
	target := day.AddDate(0, 0, -1)
	targetKey := formatDay(target)
	if lastCompleted != nil && *lastCompleted == targetKey {
		return
	}
	if err := s.RunDailyAggregation(target); err != nil {
		log.Printf("[Assets] daily aggregation failed day=%s err=%v", targetKey, err)
		return
	}
	if lastCompleted != nil {
		*lastCompleted = targetKey
	}
	log.Printf("[Assets] daily aggregation completed day=%s", targetKey)
}

type assetSummary struct {
	WalletUSD   float64 `json:"wallet_usd"`
	PositionUSD float64 `json:"position_usd"`
	FeeUSD      float64 `json:"fee_usd"`
	TotalUSD    float64 `json:"total_usd"`
}

type userWalletAsset struct {
	WalletID      uint    `json:"wallet_id"`
	WalletAddress string  `json:"wallet_address"`
	Chain         string  `json:"chain"`
	NativeUSD     float64 `json:"native_usd"`
	StableUSD     float64 `json:"stable_usd"`
	TokenUSD      float64 `json:"token_usd"`
	TotalUSD      float64 `json:"total_usd"`
}

type UserAssetOverview struct {
	Summary   assetSummary      `json:"summary"`
	Wallets   []userWalletAsset `json:"wallets"`
	UpdatedAt time.Time         `json:"updated_at"`
	Timezone  string            `json:"timezone"`
	Warnings  []string          `json:"warnings,omitempty"`
}

type UserAssetHistoryPoint struct {
	Day         string  `json:"day"`
	WalletUSD   float64 `json:"wallet_usd"`
	PositionUSD float64 `json:"position_usd"`
	FeeUSD      float64 `json:"fee_usd"`
	TotalUSD    float64 `json:"total_usd"`
}

type UserAssetHistory struct {
	Days        int                     `json:"days"`
	History     []UserAssetHistoryPoint `json:"history"`
	Today       UserAssetHistoryPoint   `json:"today"`
	MissingDays []string                `json:"missing_days,omitempty"`
	UpdatedAt   time.Time               `json:"updated_at"`
	Timezone    string                  `json:"timezone"`
	Warnings    []string                `json:"warnings,omitempty"`
}

type UserLPWindowStats struct {
	Days           int     `json:"days"`
	RealizedPnLUSD float64 `json:"realized_pnl_usd"`
	ClosedCount    int     `json:"closed_count"`
	WinCount       int     `json:"win_count"`
	LossCount      int     `json:"loss_count"`
	BreakEvenCount int     `json:"break_even_count"`
	WinRate        float64 `json:"win_rate"`
	AvgPnLUSD      float64 `json:"avg_pnl_usd"`
}

type UserLPPoolPnL struct {
	PoolID       string  `json:"pool_id"`
	Token0Symbol string  `json:"token0_symbol"`
	Token1Symbol string  `json:"token1_symbol"`
	Chain        string  `json:"chain"`
	ProfitUSD    float64 `json:"profit_usd"`
	ClosedCount  int     `json:"closed_count"`
}

type UserLPDailyPoint struct {
	Day            string  `json:"day"`
	RealizedPnLUSD float64 `json:"realized_pnl_usd"`
	ClosedCount    int     `json:"closed_count"`
	WinCount       int     `json:"win_count"`
	LossCount      int     `json:"loss_count"`
}

type UserLPStatsResponse struct {
	Windows      []UserLPWindowStats `json:"windows"`
	Today        UserLPWindowStats   `json:"today"`
	TodayPools   []UserLPPoolPnL     `json:"today_pools,omitempty"`
	DailyHistory []UserLPDailyPoint  `json:"daily_history,omitempty"`
	Timezone     string              `json:"timezone"`
}

type smartMoneyAssetBreakdown struct {
	NativeUSD         float64 `json:"native_usd"`
	StableUSD         float64 `json:"stable_usd"`
	TrackedTokenUSD   float64 `json:"tracked_token_usd"`
	OpenLPUSD         float64 `json:"open_lp_usd"`
	TotalUSD          float64 `json:"total_usd"`
	TrackedTokenCount int     `json:"tracked_token_count"`
}

type SmartMoneyHistoryPoint struct {
	Day              string  `json:"day"`
	NativeUSD        float64 `json:"native_usd"`
	StableUSD        float64 `json:"stable_usd"`
	TrackedTokenUSD  float64 `json:"tracked_token_usd"`
	OpenLPUSD        float64 `json:"open_lp_usd"`
	TotalUSD         float64 `json:"total_usd"`
	HasTransferIn    bool    `json:"has_transfer_in,omitempty"`
	HasTransferOut   bool    `json:"has_transfer_out,omitempty"`
	TransferInCount  int     `json:"transfer_in_count,omitempty"`
	TransferOutCount int     `json:"transfer_out_count,omitempty"`
}

type SmartMoneyWindowStats struct {
	Days                    int     `json:"days"`
	EstimatedRealizedPnLUSD float64 `json:"estimated_realized_pnl_usd"`
	MatchedCostUSD          float64 `json:"matched_cost_usd"`
	YieldRate               float64 `json:"yield_rate"`
	AddCount                int     `json:"add_count"`
	RemoveCount             int     `json:"remove_count"`
	MatchedRemoveCount      int     `json:"matched_remove_count"`
	UnmatchedRemoveCount    int     `json:"unmatched_remove_count"`
	ActivePoolCount         int     `json:"active_pool_count"`
}

type SmartMoneyWalletSummary struct {
	Address         string                   `json:"address"`
	Label           string                   `json:"label,omitempty"`
	ChainID         int                      `json:"chain_id"`
	Assets          smartMoneyAssetBreakdown `json:"assets"`
	ActivePoolCount int                      `json:"active_pool_count"`
	TodayEventCount int                      `json:"today_event_count"`
	LastActiveAt    *time.Time               `json:"last_active_at,omitempty"`
	RecognizedBasis string                   `json:"recognized_basis"`
}

type SmartMoneyOverview struct {
	Summary   smartMoneyAssetBreakdown  `json:"summary"`
	Wallets   []SmartMoneyWalletSummary `json:"wallets"`
	History   []SmartMoneyHistoryPoint  `json:"history"`
	Today     SmartMoneyHistoryPoint    `json:"today"`
	Windows   []SmartMoneyWindowStats   `json:"windows"`
	UpdatedAt time.Time                 `json:"updated_at"`
	Timezone  string                    `json:"timezone"`
	Warnings  []string                  `json:"warnings,omitempty"`
}

type SmartMoneyTodayActivity struct {
	EstimatedRealizedPnLUSD float64 `json:"estimated_realized_pnl_usd"`
	AddCount                int     `json:"add_count"`
	RemoveCount             int     `json:"remove_count"`
	MatchedRemoveCount      int     `json:"matched_remove_count"`
	UnmatchedRemoveCount    int     `json:"unmatched_remove_count"`
	ActivePoolCount         int     `json:"active_pool_count"`
}

type SmartMoneyWalletResponse struct {
	Wallet    SmartMoneyWalletSummary  `json:"wallet"`
	History   []SmartMoneyHistoryPoint `json:"history"`
	Today     SmartMoneyTodayActivity  `json:"today"`
	Windows   []SmartMoneyWindowStats  `json:"windows"`
	UpdatedAt time.Time                `json:"updated_at"`
	Timezone  string                   `json:"timezone"`
	Warnings  []string                 `json:"warnings,omitempty"`
}

type SmartMoneyLeaderboardEntry struct {
	Rank                    int     `json:"rank"`
	Address                 string  `json:"address"`
	Label                   string  `json:"label,omitempty"`
	ChainID                 int     `json:"chain_id"`
	MetricValue             float64 `json:"metric_value"`
	EstimatedRealizedPnLUSD float64 `json:"estimated_realized_pnl_usd"`
	YieldRate               float64 `json:"yield_rate"`
	ParticipationCount      int     `json:"participation_count"`
	ActivePoolCount         int     `json:"active_pool_count"`
	UnmatchedRemoveCount    int     `json:"unmatched_remove_count"`
	HasTransferIn           bool    `json:"has_transfer_in,omitempty"`
	HasTransferOut          bool    `json:"has_transfer_out,omitempty"`
	TransferInCount         int     `json:"transfer_in_count,omitempty"`
	TransferOutCount        int     `json:"transfer_out_count,omitempty"`
}

type SmartMoneyLeaderboardResponse struct {
	Days        int                          `json:"days"`
	Metric      string                       `json:"metric"`
	StartDay    string                       `json:"start_day"`
	EndDay      string                       `json:"end_day"`
	SnapshotDay string                       `json:"snapshot_day,omitempty"`
	ComparedDay string                       `json:"compared_day,omitempty"`
	Timezone    string                       `json:"timezone"`
	Description string                       `json:"description"`
	List        []SmartMoneyLeaderboardEntry `json:"list"`
}

type tokenDescriptor struct {
	Address string
	Stable  bool
}

func formatDay(t time.Time) string {
	return t.In(timeutil.Location()).Format("2006-01-02")
}

func dayStart(t time.Time) time.Time {
	tt := t.In(timeutil.Location())
	return time.Date(tt.Year(), tt.Month(), tt.Day(), 0, 0, 0, 0, timeutil.Location())
}

func dayEnd(t time.Time) time.Time {
	return dayStart(t).Add(24 * time.Hour)
}

func clampHistoryDays(days int) int {
	switch days {
	case 1, 7, 30, 90:
		return days
	default:
		return defaultHistoryDays
	}
}

func clampLPDays(days int) int {
	switch days {
	case 1, 7, 30:
		return days
	default:
		return 7
	}
}

func round2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

func round4(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*10000) / 10000
}

func normalizeAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if !common.IsHexAddress(raw) {
		return ""
	}
	return strings.ToLower(common.HexToAddress(raw).Hex())
}

func (s *Service) getClientForChain(chain string) (*ethclient.Client, config.ChainConfig, error) {
	if config.AppConfig == nil {
		return nil, config.ChainConfig{}, fmt.Errorf("config not loaded")
	}
	chain = config.NormalizeChain(chain)
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok {
		return nil, config.ChainConfig{}, fmt.Errorf("chain config not found: %s", chain)
	}
	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return nil, cc, err
	}
	return client, cc, nil
}

func (s *Service) getTokenDecimals(chain string, client *ethclient.Client, tokenAddress string, fallback int) int {
	addr := normalizeAddress(tokenAddress)
	if addr == "" {
		if fallback > 0 {
			return fallback
		}
		return 18
	}
	key := config.NormalizeChain(chain) + "|" + addr

	s.decimalsMu.RLock()
	if v, ok := s.decimalsCache[key]; ok && v > 0 {
		s.decimalsMu.RUnlock()
		return v
	}
	s.decimalsMu.RUnlock()

	decimals := fallback
	if decimals <= 0 {
		decimals = 18
	}
	if client != nil {
		if v, err := blockchain.GetTokenDecimalsWithClient(client, common.HexToAddress(addr)); err == nil && v > 0 {
			decimals = int(v)
		}
	}

	s.decimalsMu.Lock()
	s.decimalsCache[key] = decimals
	s.decimalsMu.Unlock()
	return decimals
}

func balanceToUSD(rawAmount float64, price float64) float64 {
	if rawAmount <= 0 || price < 0 {
		return 0
	}
	return round2(rawAmount * price)
}

func weiToFloat(balance string) float64 {
	if strings.TrimSpace(balance) == "" {
		return 0
	}
	var value float64
	if _, err := fmt.Sscan(balance, &value); err != nil {
		return 0
	}
	return value
}

func amountToFloat(raw string, decimals int) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	sign := 1.0
	if strings.HasPrefix(raw, "-") {
		sign = -1
		raw = strings.TrimPrefix(raw, "-")
	}
	if raw == "" {
		return 0
	}
	n := new(bigFloat)
	if !n.SetString(raw) {
		return 0
	}
	if decimals > 0 {
		n.QuoPow10(decimals)
	}
	return sign * n.Float64()
}

type bigFloat struct {
	f *big.Float
}

func (b *bigFloat) ensure() {
	if b.f == nil {
		b.f = new(big.Float).SetPrec(256)
	}
}

func (b *bigFloat) SetString(raw string) bool {
	b.ensure()
	_, ok := b.f.SetString(raw)
	return ok
}

func (b *bigFloat) QuoPow10(decimals int) {
	b.ensure()
	if decimals <= 0 {
		return
	}
	den := new(big.Float).SetPrec(256).SetFloat64(math.Pow10(decimals))
	b.f.Quo(b.f, den)
}

func (b *bigFloat) Float64() float64 {
	b.ensure()
	value, _ := b.f.Float64()
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func upsertByColumns(ctx context.Context, model interface{}, columns []string, values map[string]interface{}) error {
	if database.DB == nil {
		return fmt.Errorf("database not initialized")
	}
	conflictColumns := make([]clause.Column, 0, len(columns))
	for _, name := range columns {
		conflictColumns = append(conflictColumns, clause.Column{Name: name})
	}
	return database.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   conflictColumns,
			DoUpdates: clause.Assignments(values),
		}).
		Create(model).Error
}
