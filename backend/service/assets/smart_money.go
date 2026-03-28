package assets

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	recognizedAssetBasis          = "原生币 + 稳定币 + 近30天参与LP的代币余额 + 当前open LP估算持仓"
	smartMoneyWalletLiveCacheTTL  = 30 * time.Minute
	smartMoneyLeaderboardCacheTTL = 72 * time.Hour
	smartMoneyDefaultPageSize     = 10
	smartMoneyMaxPageSize         = 50
)

var smartMoneyLeaderboardMetrics = []string{"pnl", "yield_rate", "participation"}

type smartMoneyWalletLiveState struct {
	assets          smartMoneyAssetBreakdown
	activePoolCount int
	todayEventCount int
	lastActiveAt    *time.Time
	warnings        []string
}

type smartMoneyEventStats struct {
	EstimatedRealizedPnLUSD float64
	MatchedCostUSD          float64
	AddCount                int
	RemoveCount             int
	MatchedRemoveCount      int
	UnmatchedRemoveCount    int
	activePools             map[string]struct{}
}

type smartMoneyTransferActivity struct {
	HasTransferIn    bool
	HasTransferOut   bool
	TransferInCount  int
	TransferOutCount int
	TransferInUSD    float64
	TransferOutUSD   float64
}

type smartMoneyLeaderboardSnapshotInput struct {
	Wallet    models.MonitoredWallet
	Current   *models.SmartMoneyWalletDailySnapshot
	Previous  *models.SmartMoneyWalletDailySnapshot
	DailyStat *models.SmartMoneyLPDailyStat
}

type cachedSmartMoneyWalletLiveState struct {
	Assets          smartMoneyAssetBreakdown `json:"assets"`
	ActivePoolCount int                      `json:"active_pool_count"`
	TodayEventCount int                      `json:"today_event_count"`
	LastActiveAt    *time.Time               `json:"last_active_at,omitempty"`
	Warnings        []string                 `json:"warnings,omitempty"`
}

func newCachedSmartMoneyWalletLiveState(state smartMoneyWalletLiveState) cachedSmartMoneyWalletLiveState {
	return cachedSmartMoneyWalletLiveState{
		Assets:          state.assets,
		ActivePoolCount: state.activePoolCount,
		TodayEventCount: state.todayEventCount,
		LastActiveAt:    state.lastActiveAt,
		Warnings:        state.warnings,
	}
}

func (c cachedSmartMoneyWalletLiveState) liveState() smartMoneyWalletLiveState {
	return smartMoneyWalletLiveState{
		assets:          c.Assets,
		activePoolCount: c.ActivePoolCount,
		todayEventCount: c.TodayEventCount,
		lastActiveAt:    c.LastActiveAt,
		warnings:        c.Warnings,
	}
}

func clampSmartMoneyPage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}

func clampSmartMoneyPageSize(size int) int {
	if size <= 0 {
		return smartMoneyDefaultPageSize
	}
	if size > smartMoneyMaxPageSize {
		return smartMoneyMaxPageSize
	}
	return size
}

func smartMoneyWalletLiveCacheKey(chainID int, address string) string {
	return fmt.Sprintf("assets:smart-money:wallet-live:%d:%s", chainID, normalizeAddress(address))
}

func smartMoneyLeaderboardDailyCacheKey(snapshotDay time.Time, metric string) string {
	return fmt.Sprintf("assets:smart-money:leaderboard:v2:%s:%s", formatDay(snapshotDay), normalizeLeaderboardMetric(metric))
}

func shouldZeroSmartMoneyPnLForTransfer(snapshot *models.SmartMoneyWalletDailySnapshot) bool {
	if snapshot == nil {
		return false
	}
	return snapshot.HasTransferIn || snapshot.HasTransferOut || snapshot.TransferInCount > 0 || snapshot.TransferOutCount > 0
}

func paginateSmartMoneyLeaderboardResponse(resp *SmartMoneyLeaderboardResponse, page int, pageSize int, keyword string) *SmartMoneyLeaderboardResponse {
	if resp == nil {
		return nil
	}

	pageSize = clampSmartMoneyPageSize(pageSize)
	keyword = strings.TrimSpace(keyword)
	filtered := resp.List
	if keyword != "" {
		query := strings.ToLower(keyword)
		filtered = make([]SmartMoneyLeaderboardEntry, 0, len(resp.List))
		for _, entry := range resp.List {
			address := strings.ToLower(strings.TrimSpace(entry.Address))
			label := strings.ToLower(strings.TrimSpace(entry.Label))
			if strings.Contains(address, query) || strings.Contains(label, query) {
				filtered = append(filtered, entry)
			}
		}
	}

	total := len(filtered)
	totalPages := 1
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	if page <= 0 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	cloned := *resp
	cloned.Page = page
	cloned.PageSize = pageSize
	cloned.Total = total
	cloned.TotalPages = totalPages
	cloned.Keyword = keyword
	cloned.List = make([]SmartMoneyLeaderboardEntry, 0, end-start)
	for i := start; i < end; i++ {
		cloned.List = append(cloned.List, filtered[i])
	}
	return &cloned
}

func readCachedSmartMoneyLeaderboard(snapshotDay time.Time, metric string) (*SmartMoneyLeaderboardResponse, bool) {
	if database.RedisClient == nil {
		return nil, false
	}
	raw, err := database.GetCache(smartMoneyLeaderboardDailyCacheKey(snapshotDay, metric))
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil, false
	}
	var resp SmartMoneyLeaderboardResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, false
	}
	return &resp, true
}

func writeCachedSmartMoneyLeaderboard(snapshotDay time.Time, metric string, resp *SmartMoneyLeaderboardResponse) {
	if database.RedisClient == nil || resp == nil {
		return
	}
	body, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_ = database.SetCache(smartMoneyLeaderboardDailyCacheKey(snapshotDay, metric), string(body), smartMoneyLeaderboardCacheTTL)
}

func smartMoneyWalletSummaryFromLive(walletRow models.MonitoredWallet, live smartMoneyWalletLiveState) SmartMoneyWalletSummary {
	summary := SmartMoneyWalletSummary{
		Address:         walletRow.Address,
		ChainID:         walletRow.ChainID,
		Assets:          live.assets,
		ActivePoolCount: live.activePoolCount,
		TodayEventCount: live.todayEventCount,
		LastActiveAt:    live.lastActiveAt,
		RecognizedBasis: recognizedAssetBasis,
	}
	if walletRow.Label != nil {
		summary.Label = strings.TrimSpace(*walletRow.Label)
	}
	if walletRow.AvatarURL != nil {
		summary.AvatarURL = strings.TrimSpace(*walletRow.AvatarURL)
	}
	return summary
}

func smartMoneyWalletSummaryFromSnapshot(walletRow models.MonitoredWallet, snapshot *models.SmartMoneyWalletDailySnapshot, dailyStat *models.SmartMoneyLPDailyStat) SmartMoneyWalletSummary {
	summary := SmartMoneyWalletSummary{
		Address:         walletRow.Address,
		ChainID:         walletRow.ChainID,
		RecognizedBasis: recognizedAssetBasis,
	}
	if walletRow.Label != nil {
		summary.Label = strings.TrimSpace(*walletRow.Label)
	}
	if walletRow.AvatarURL != nil {
		summary.AvatarURL = strings.TrimSpace(*walletRow.AvatarURL)
	}
	if snapshot != nil {
		summary.Assets = smartMoneyAssetBreakdown{
			NativeUSD:         round2(snapshot.NativeUSD),
			StableUSD:         round2(snapshot.StableUSD),
			TrackedTokenUSD:   round2(snapshot.TrackedTokenUSD),
			OpenLPUSD:         round2(snapshot.OpenLPUSD),
			TotalUSD:          round2(snapshot.TotalUSD),
			TrackedTokenCount: snapshot.TrackedTokenCount,
		}
	}
	if dailyStat != nil {
		summary.ActivePoolCount = dailyStat.ActivePoolCount
	}
	return summary
}

func (s *Service) GetSmartMoneyWalletBalance(ctx context.Context, address string, chainID int, forceRefresh bool) (*float64, error) {
	address = normalizeAddress(address)
	if address == "" {
		return nil, fmt.Errorf("invalid wallet address")
	}
	if chainID <= 0 {
		chainID = 56
	}

	live, err := s.loadSmartMoneyWalletLiveStateCached(ctx, models.MonitoredWallet{
		Address: address,
		ChainID: chainID,
	}, forceRefresh)
	if err != nil {
		return nil, err
	}

	totalUSD := round2(live.assets.TotalUSD)
	return &totalUSD, nil
}

func (s *Service) GetSmartMoneyOverview(ctx context.Context, days int, page int, size int, keyword string, forceRefresh bool) (*SmartMoneyOverview, error) {
	days = clampLPDays(days)
	page = clampSmartMoneyPage(page)
	size = clampSmartMoneyPageSize(size)
	keyword = strings.TrimSpace(keyword)

	allWallets, err := s.loadActiveSmartMoneyWallets(ctx)
	if err != nil {
		return nil, err
	}
	snapshotDay := dayStart(timeutil.Now()).AddDate(0, 0, -1)
	pageWallets, total, err := s.loadPagedSmartMoneyWallets(ctx, snapshotDay, page, size, keyword)
	if err != nil {
		return nil, err
	}

	resp := &SmartMoneyOverview{
		Wallets:          make([]SmartMoneyWalletSummary, 0, len(pageWallets)),
		WalletPage:       page,
		WalletSize:       size,
		WalletTotal:      int(total),
		WalletTotalPages: 1,
		SnapshotDay:      formatDay(snapshotDay),
		UpdatedAt:        timeutil.Now(),
		Timezone:         timeutil.LocationName(),
	}
	if total > 0 {
		resp.WalletTotalPages = int((total + int64(size) - 1) / int64(size))
	}

	start := dayStart(timeutil.Now()).AddDate(0, 0, -defaultHistoryDays)
	end := dayStart(timeutil.Now())
	history, err := s.loadSmartMoneyHistory(ctx, allWallets, start, end)
	if err != nil {
		return nil, err
	}
	resp.History = history

	summaryHistory, err := s.loadSmartMoneyHistory(ctx, allWallets, snapshotDay, snapshotDay.Add(24*time.Hour))
	if err != nil {
		return nil, err
	}
	if len(summaryHistory) > 0 {
		point := summaryHistory[0]
		resp.Summary = smartMoneyAssetBreakdown{
			NativeUSD:       point.NativeUSD,
			StableUSD:       point.StableUSD,
			TrackedTokenUSD: point.TrackedTokenUSD,
			OpenLPUSD:       point.OpenLPUSD,
			TotalUSD:        point.TotalUSD,
		}
		resp.Today = point
	}

	snapshotMap, err := s.loadSmartMoneySnapshotRows(ctx, pageWallets, snapshotDay)
	if err != nil {
		return nil, err
	}
	lpStatMap, err := s.loadSmartMoneyDailyStatRows(ctx, pageWallets, snapshotDay)
	if err != nil {
		return nil, err
	}

	for _, walletRow := range pageWallets {
		live, err := s.loadSmartMoneyWalletLiveStateCached(ctx, walletRow, forceRefresh)
		if err != nil {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("wallet %s unavailable: %v", walletRow.Address, err))
			key := formatDay(snapshotDay) + "|" + smartMoneyWalletKey(walletRow.ChainID, walletRow.Address)
			resp.Wallets = append(resp.Wallets, smartMoneyWalletSummaryFromSnapshot(walletRow, snapshotMap[key], lpStatMap[smartMoneyWalletKey(walletRow.ChainID, walletRow.Address)]))
			continue
		}
		resp.Wallets = append(resp.Wallets, smartMoneyWalletSummaryFromLive(walletRow, live))
		resp.Warnings = append(resp.Warnings, live.warnings...)
	}

	windowStart := dayStart(timeutil.Now()).AddDate(0, 0, -days)
	statsByWallet, err := s.computeSmartMoneyStats(ctx, allWallets, windowStart, timeutil.Now())
	if err != nil {
		return nil, err
	}
	resp.Windows = []SmartMoneyWindowStats{aggregateSmartMoneyWindowStats(days, statsByWallet)}
	resp.Warnings = dedupeStrings(resp.Warnings)
	return resp, nil
}

func (s *Service) GetSmartMoneyWallet(ctx context.Context, address string, chainID int, days int, forceRefresh bool) (*SmartMoneyWalletResponse, error) {
	address = normalizeAddress(address)
	if address == "" {
		return nil, fmt.Errorf("invalid wallet address")
	}
	if chainID <= 0 {
		chainID = 56
	}
	walletRow, err := s.smRepo.GetMonitoredWalletByAddress(ctx, address, chainID)
	if err != nil {
		return nil, err
	}
	if walletRow == nil || !walletRow.IsActive {
		return nil, fmt.Errorf("wallet not found")
	}

	live, err := s.loadSmartMoneyWalletLiveStateCached(ctx, *walletRow, forceRefresh)
	if err != nil {
		return nil, err
	}
	summary := smartMoneyWalletSummaryFromLive(*walletRow, live)

	start := dayStart(timeutil.Now()).AddDate(0, 0, -defaultHistoryDays)
	end := dayStart(timeutil.Now())
	history, err := s.loadSmartMoneyHistory(ctx, []models.MonitoredWallet{*walletRow}, start, end)
	if err != nil {
		return nil, err
	}

	windowDays := []int{1, 7, 30}
	windows := make([]SmartMoneyWindowStats, 0, len(windowDays))
	for _, window := range windowDays {
		statsByWallet, err := s.computeSmartMoneyStats(ctx, []models.MonitoredWallet{*walletRow}, dayStart(timeutil.Now()).AddDate(0, 0, -window), timeutil.Now())
		if err != nil {
			return nil, err
		}
		windows = append(windows, aggregateSmartMoneyWindowStats(window, statsByWallet))
	}

	todayStatsByWallet, err := s.computeSmartMoneyStats(ctx, []models.MonitoredWallet{*walletRow}, dayStart(timeutil.Now()), timeutil.Now())
	if err != nil {
		return nil, err
	}
	todayStats := todayStatsByWallet[smartMoneyWalletKey(walletRow.ChainID, walletRow.Address)]

	return &SmartMoneyWalletResponse{
		Wallet:  summary,
		History: history,
		Today: SmartMoneyTodayActivity{
			EstimatedRealizedPnLUSD: round2(todayStats.EstimatedRealizedPnLUSD),
			AddCount:                todayStats.AddCount,
			RemoveCount:             todayStats.RemoveCount,
			MatchedRemoveCount:      todayStats.MatchedRemoveCount,
			UnmatchedRemoveCount:    todayStats.UnmatchedRemoveCount,
			ActivePoolCount:         len(todayStats.activePools),
		},
		Windows:   windows,
		UpdatedAt: timeutil.Now(),
		Timezone:  timeutil.LocationName(),
		Warnings:  dedupeStrings(live.warnings),
	}, nil
}

func (s *Service) GetSmartMoneyLeaderboard(ctx context.Context, metric string, days int, page int, pageSize int, keyword string, forceRefresh bool) (*SmartMoneyLeaderboardResponse, error) {
	metric = normalizeLeaderboardMetric(metric)
	snapshotDay := dayStart(timeutil.Now()).AddDate(0, 0, -1)
	comparedDay := snapshotDay.AddDate(0, 0, -1)

	if !forceRefresh {
		if cached, ok := readCachedSmartMoneyLeaderboard(snapshotDay, metric); ok {
			cached.Timezone = timeutil.LocationName()
			return paginateSmartMoneyLeaderboardResponse(cached, page, pageSize, keyword), nil
		}
	}

	wallets, err := s.loadActiveSmartMoneyWallets(ctx)
	if err != nil {
		return nil, err
	}
	inputs, err := s.buildSmartMoneyLeaderboardSnapshotInputs(ctx, wallets, snapshotDay)
	if err != nil {
		return nil, err
	}

	fullResp := buildSmartMoneySnapshotLeaderboard(metric, snapshotDay, comparedDay, 0, inputs)
	fullResp.Timezone = timeutil.LocationName()
	writeCachedSmartMoneyLeaderboard(snapshotDay, metric, fullResp)
	return paginateSmartMoneyLeaderboardResponse(fullResp, page, pageSize, keyword), nil
}

func (s *Service) deleteCachedSmartMoneyLeaderboards(snapshotDay time.Time) {
	if database.RedisClient == nil {
		return
	}
	for _, metric := range smartMoneyLeaderboardMetrics {
		_ = database.DeleteCache(smartMoneyLeaderboardDailyCacheKey(snapshotDay, metric))
	}
}

func (s *Service) buildSmartMoneyLeaderboardSnapshotInputs(ctx context.Context, wallets []models.MonitoredWallet, snapshotDay time.Time) ([]smartMoneyLeaderboardSnapshotInput, error) {
	comparedDay := snapshotDay.AddDate(0, 0, -1)
	snapshotMap, err := s.loadSmartMoneySnapshotRows(ctx, wallets, comparedDay, snapshotDay)
	if err != nil {
		return nil, err
	}
	lpStatMap, err := s.loadSmartMoneyDailyStatRows(ctx, wallets, snapshotDay)
	if err != nil {
		return nil, err
	}

	inputs := make([]smartMoneyLeaderboardSnapshotInput, 0, len(wallets))
	for _, walletRow := range wallets {
		key := smartMoneyWalletKey(walletRow.ChainID, walletRow.Address)
		current := snapshotMap[formatDay(snapshotDay)+"|"+key]
		previous := snapshotMap[formatDay(comparedDay)+"|"+key]
		if current == nil || previous == nil {
			continue
		}
		inputs = append(inputs, smartMoneyLeaderboardSnapshotInput{
			Wallet:    walletRow,
			Current:   current,
			Previous:  previous,
			DailyStat: lpStatMap[key],
		})
	}
	return inputs, nil
}

func (s *Service) refreshSmartMoneyLeaderboardCaches(ctx context.Context, snapshotDay time.Time) error {
	if database.RedisClient == nil {
		return nil
	}
	wallets, err := s.loadActiveSmartMoneyWallets(ctx)
	if err != nil {
		return err
	}
	inputs, err := s.buildSmartMoneyLeaderboardSnapshotInputs(ctx, wallets, snapshotDay)
	if err != nil {
		return err
	}
	comparedDay := snapshotDay.AddDate(0, 0, -1)
	for _, metric := range smartMoneyLeaderboardMetrics {
		resp := buildSmartMoneySnapshotLeaderboard(metric, snapshotDay, comparedDay, 0, inputs)
		resp.Timezone = timeutil.LocationName()
		writeCachedSmartMoneyLeaderboard(snapshotDay, metric, resp)
	}
	s.deleteCachedSmartMoneyLeaderboards(comparedDay)
	return nil
}

func normalizeLeaderboardMetric(metric string) string {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "yield", "yield_rate", "rate":
		return "yield_rate"
	case "count", "participation", "participation_count":
		return "participation"
	default:
		return "pnl"
	}
}

func leaderboardDescription(metric string) string {
	switch metric {
	case "yield_rate":
		return "按窗口内估算已实现收益率排序"
	case "participation":
		return "按窗口内 add/remove 参与次数排序"
	default:
		return "按窗口内估算已实现收益额排序"
	}
}

func smartMoneyWalletKey(chainID int, address string) string {
	return strconv.Itoa(chainID) + "|" + normalizeAddress(address)
}

func buildSmartMoneySnapshotLeaderboard(metric string, snapshotDay time.Time, comparedDay time.Time, limit int, inputs []smartMoneyLeaderboardSnapshotInput) *SmartMoneyLeaderboardResponse {
	list := make([]SmartMoneyLeaderboardEntry, 0, len(inputs))
	for _, input := range inputs {
		if input.Current == nil || input.Previous == nil {
			continue
		}
		estimatedPnL := round2(input.Current.TotalUSD - input.Previous.TotalUSD)
		yieldRate := 0.0
		if input.Previous.TotalUSD > 0 {
			yieldRate = round4(estimatedPnL / input.Previous.TotalUSD)
		}
		entry := SmartMoneyLeaderboardEntry{
			Address:                 input.Wallet.Address,
			ChainID:                 input.Wallet.ChainID,
			EstimatedRealizedPnLUSD: estimatedPnL,
			YieldRate:               yieldRate,
			HasTransferIn:           input.Current.HasTransferIn,
			HasTransferOut:          input.Current.HasTransferOut,
			TransferInCount:         input.Current.TransferInCount,
			TransferOutCount:        input.Current.TransferOutCount,
			TransferInUSD:           round2(input.Current.TransferInUSD),
			TransferOutUSD:          round2(input.Current.TransferOutUSD),
		}
		if input.Wallet.Label != nil {
			entry.Label = strings.TrimSpace(*input.Wallet.Label)
		}
		if input.Wallet.AvatarURL != nil {
			entry.AvatarURL = strings.TrimSpace(*input.Wallet.AvatarURL)
		}
		if input.DailyStat != nil {
			entry.ParticipationCount = input.DailyStat.AddCount + input.DailyStat.RemoveCount
			entry.ActivePoolCount = input.DailyStat.ActivePoolCount
			entry.UnmatchedRemoveCount = input.DailyStat.UnmatchedRemoveCount
		}
		switch metric {
		case "yield_rate":
			entry.MetricValue = entry.YieldRate
		case "participation":
			entry.MetricValue = float64(entry.ParticipationCount)
		default:
			entry.MetricValue = entry.EstimatedRealizedPnLUSD
		}
		list = append(list, entry)
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].MetricValue != list[j].MetricValue {
			return list[i].MetricValue > list[j].MetricValue
		}
		if list[i].EstimatedRealizedPnLUSD != list[j].EstimatedRealizedPnLUSD {
			return list[i].EstimatedRealizedPnLUSD > list[j].EstimatedRealizedPnLUSD
		}
		return strings.ToLower(list[i].Address) < strings.ToLower(list[j].Address)
	})

	resp := &SmartMoneyLeaderboardResponse{
		Days:        1,
		Metric:      metric,
		StartDay:    formatDay(comparedDay),
		EndDay:      formatDay(snapshotDay),
		SnapshotDay: formatDay(snapshotDay),
		ComparedDay: formatDay(comparedDay),
		Description: leaderboardDescription(metric),
		Page:        1,
		PageSize:    len(list),
		Total:       len(list),
		TotalPages:  1,
		List:        make([]SmartMoneyLeaderboardEntry, 0, len(list)),
	}
	if limit <= 0 || limit > len(list) {
		limit = len(list)
	}
	for i := 0; i < len(list) && i < limit; i++ {
		list[i].Rank = i + 1
		resp.List = append(resp.List, list[i])
	}
	return resp
}

func (s *Service) loadActiveSmartMoneyWallets(ctx context.Context) ([]models.MonitoredWallet, error) {
	var wallets []models.MonitoredWallet
	err := database.DB.WithContext(ctx).
		Where("is_active = ?", true).
		Order("chain_id ASC, address ASC").
		Find(&wallets).Error
	return wallets, err
}

func (s *Service) loadPagedSmartMoneyWallets(ctx context.Context, snapshotDay time.Time, page int, size int, keyword string) ([]models.MonitoredWallet, int64, error) {
	page = clampSmartMoneyPage(page)
	size = clampSmartMoneyPageSize(size)
	keyword = strings.TrimSpace(keyword)

	db := database.DB.WithContext(ctx).
		Model(&models.MonitoredWallet{}).
		Where("monitored_wallets.is_active = ?", true)
	if keyword != "" {
		kw := "%" + strings.ToLower(keyword) + "%"
		db = db.Where("LOWER(monitored_wallets.address) LIKE ? OR LOWER(COALESCE(monitored_wallets.label, '')) LIKE ?", kw, kw)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var wallets []models.MonitoredWallet
	err := db.
		Select("monitored_wallets.*").
		Joins("LEFT JOIN sm_wallet_daily_snapshots smd ON smd.wallet_address = monitored_wallets.address AND smd.chain_id = monitored_wallets.chain_id AND smd.snapshot_day = ?", formatDay(snapshotDay)).
		Order("COALESCE(smd.total_usd, 0) DESC").
		Order("monitored_wallets.chain_id ASC").
		Order("monitored_wallets.address ASC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&wallets).Error
	return wallets, total, err
}

func (s *Service) loadSmartMoneyWalletLiveStateCached(ctx context.Context, walletRow models.MonitoredWallet, forceRefresh bool) (smartMoneyWalletLiveState, error) {
	var state smartMoneyWalletLiveState
	cacheKey := smartMoneyWalletLiveCacheKey(walletRow.ChainID, walletRow.Address)
	if !forceRefresh && database.RedisClient != nil {
		raw, err := database.GetCache(cacheKey)
		if err == nil && strings.TrimSpace(raw) != "" {
			var cached cachedSmartMoneyWalletLiveState
			if err := json.Unmarshal([]byte(raw), &cached); err == nil {
				return cached.liveState(), nil
			}
		}
	}

	state, err := s.loadSmartMoneyWalletLiveStateLive(ctx, walletRow)
	if err != nil {
		return state, err
	}
	if database.RedisClient != nil {
		if body, err := json.Marshal(newCachedSmartMoneyWalletLiveState(state)); err == nil {
			_ = database.SetCache(cacheKey, string(body), smartMoneyWalletLiveCacheTTL)
		}
	}
	return state, nil
}

func (s *Service) loadSmartMoneyWalletLiveStateLive(ctx context.Context, walletRow models.MonitoredWallet) (smartMoneyWalletLiveState, error) {
	var state smartMoneyWalletLiveState
	address := normalizeAddress(walletRow.Address)
	if address == "" {
		return state, fmt.Errorf("invalid wallet address")
	}
	chain := smartMoneyChainFromID(walletRow.ChainID)
	client, cc, err := s.getClientForChain(chain)
	if err != nil {
		return state, err
	}

	walletAddr := common.HexToAddress(address)
	nativePrice := s.nativePriceUSD(chain, cc)
	if nativeBalance, err := blockchain.GetBalanceWithClient(client, walletAddr); err == nil && nativeBalance != nil {
		state.assets.NativeUSD = balanceToUSD(amountToFloat(nativeBalance.String(), 18), nativePrice)
	} else if err != nil {
		state.warnings = append(state.warnings, fmt.Sprintf("native balance unavailable: %v", err))
	}

	stableDescriptors := make([]tokenDescriptor, 0, 3)
	for _, addr := range []string{cc.USDTAddress, cc.USDCAddress, cc.BUSDAddress} {
		if normalized := normalizeAddress(addr); normalized != "" {
			stableDescriptors = append(stableDescriptors, tokenDescriptor{Address: normalized, Stable: true})
		}
	}
	tokenDescriptors, err := s.loadSmartMoneyTrackedTokens(ctx, address, walletRow.ChainID, stableDescriptors)
	if err != nil {
		return state, err
	}
	prices, _ := s.priceService.GetUSDPrices(chain, descriptorAddresses(tokenDescriptors))
	for _, token := range tokenDescriptors {
		addr := normalizeAddress(token.Address)
		if addr == "" {
			continue
		}
		decimals := s.tokenFallbackDecimals(cc, addr)
		decimals = s.getTokenDecimals(chain, client, addr, decimals)
		balance, err := blockchain.GetTokenBalanceWithClient(client, common.HexToAddress(addr), walletAddr)
		if err != nil || balance == nil || balance.Sign() <= 0 {
			continue
		}
		usd := balanceToUSD(amountToFloat(balance.String(), decimals), prices[addr])
		if token.Stable {
			state.assets.StableUSD += usd
		} else {
			state.assets.TrackedTokenUSD += usd
			state.assets.TrackedTokenCount++
		}
	}

	openLPUSD, activePoolCount, err := s.loadSmartMoneyOpenLPState(ctx, address, walletRow.ChainID)
	if err != nil {
		return state, err
	}
	state.assets.OpenLPUSD = round2(openLPUSD)
	state.activePoolCount = activePoolCount
	state.assets.NativeUSD = round2(state.assets.NativeUSD)
	state.assets.StableUSD = round2(state.assets.StableUSD)
	state.assets.TrackedTokenUSD = round2(state.assets.TrackedTokenUSD)
	state.assets.TotalUSD = round2(state.assets.NativeUSD + state.assets.StableUSD + state.assets.TrackedTokenUSD + state.assets.OpenLPUSD)

	todayStart := dayStart(timeutil.Now())
	var todayEventCount int64
	if err := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyLPEvent{}).
		Where("wallet_address = ? AND chain_id = ? AND tx_timestamp >= ? AND tx_timestamp < ?", address, walletRow.ChainID, todayStart, timeutil.Now()).
		Count(&todayEventCount).Error; err == nil {
		state.todayEventCount = int(todayEventCount)
	}
	var lastEvent models.SmartMoneyLPEvent
	if err := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyLPEvent{}).
		Where("wallet_address = ? AND chain_id = ?", address, walletRow.ChainID).
		Order("tx_timestamp DESC").
		First(&lastEvent).Error; err == nil {
		state.lastActiveAt = &lastEvent.TxTimestamp
	}
	return state, nil
}

func smartMoneyChainFromID(chainID int) string {
	switch chainID {
	case 8453:
		return "base"
	default:
		return "bsc"
	}
}

func descriptorAddresses(values []tokenDescriptor) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value.Address == "" {
			continue
		}
		out = append(out, value.Address)
	}
	return out
}

func (s *Service) loadSmartMoneyTrackedTokens(ctx context.Context, address string, chainID int, stable []tokenDescriptor) ([]tokenDescriptor, error) {
	seen := make(map[string]tokenDescriptor)
	for _, item := range stable {
		seen[item.Address] = item
	}

	cutoff := dayStart(timeutil.Now()).AddDate(0, 0, -30)
	type tokenRow struct {
		TokenAddress string
	}
	var rows []tokenRow
	if err := database.DB.WithContext(ctx).
		Raw(`
			SELECT token_address
			FROM (
				SELECT token0_address AS token_address
				FROM sm_lp_events
				WHERE wallet_address = ? AND chain_id = ? AND tx_timestamp >= ?
				UNION
				SELECT token1_address AS token_address
				FROM sm_lp_events
				WHERE wallet_address = ? AND chain_id = ? AND tx_timestamp >= ?
				UNION
				SELECT token0_address AS token_address
				FROM sm_lp_positions
				WHERE wallet_address = ? AND chain_id = ? AND status = 'open'
				UNION
				SELECT token1_address AS token_address
				FROM sm_lp_positions
				WHERE wallet_address = ? AND chain_id = ? AND status = 'open'
			) tokens
		`, address, chainID, cutoff, address, chainID, cutoff, address, chainID, address, chainID).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		addr := normalizeAddress(row.TokenAddress)
		if addr == "" {
			continue
		}
		if existing, ok := seen[addr]; ok {
			seen[addr] = existing
			continue
		}
		seen[addr] = tokenDescriptor{Address: addr}
	}
	out := make([]tokenDescriptor, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Address < out[j].Address
	})
	return out, nil
}

func (s *Service) loadSmartMoneyOpenLPState(ctx context.Context, address string, chainID int) (float64, int, error) {
	type row struct {
		OpenLPUSD       float64
		ActivePoolCount int
	}
	var result row
	err := database.DB.WithContext(ctx).
		Raw(`
			SELECT
				COALESCE(SUM(COALESCE(ap.net_total_usd, evt_net.net_amount_usd, 0)), 0) AS open_lp_usd,
				COUNT(DISTINCT p.pool_address) AS active_pool_count
			FROM sm_lp_positions p
			LEFT JOIN sm_lp_active_positions ap
				ON ap.chain_id = p.chain_id AND ap.nft_token_id = p.nft_token_id
			LEFT JOIN (
				SELECT
					chain_id,
					nft_token_id,
					SUM(
						CASE
							WHEN event_type = 'add' THEN COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
							WHEN event_type = 'remove' THEN -COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
							ELSE 0
						END
					) AS net_amount_usd
				FROM sm_lp_events
				WHERE event_type IN ('add', 'remove')
				GROUP BY chain_id, nft_token_id
			) evt_net
				ON evt_net.chain_id = p.chain_id
				AND evt_net.nft_token_id = p.nft_token_id
			WHERE p.wallet_address = ?
			  AND p.chain_id = ?
			  AND p.status = 'open'
	`, address, chainID).
		Scan(&result).Error
	return round2(result.OpenLPUSD), result.ActivePoolCount, err
}

func (s *Service) loadSmartMoneySnapshotRows(ctx context.Context, wallets []models.MonitoredWallet, days ...time.Time) (map[string]*models.SmartMoneyWalletDailySnapshot, error) {
	rowsByKey := make(map[string]*models.SmartMoneyWalletDailySnapshot)
	if len(wallets) == 0 || len(days) == 0 {
		return rowsByKey, nil
	}

	chainIDs := make([]int, 0, len(wallets))
	addresses := make([]string, 0, len(wallets))
	chainSeen := make(map[int]struct{}, len(wallets))
	addrSeen := make(map[string]struct{}, len(wallets))
	for _, wallet := range wallets {
		if _, ok := chainSeen[wallet.ChainID]; !ok {
			chainSeen[wallet.ChainID] = struct{}{}
			chainIDs = append(chainIDs, wallet.ChainID)
		}
		addr := normalizeAddress(wallet.Address)
		if addr == "" {
			continue
		}
		if _, ok := addrSeen[addr]; ok {
			continue
		}
		addrSeen[addr] = struct{}{}
		addresses = append(addresses, addr)
	}

	dayKeys := make([]string, 0, len(days))
	daySeen := make(map[string]struct{}, len(days))
	for _, day := range days {
		dayKey := formatDay(day)
		if _, ok := daySeen[dayKey]; ok {
			continue
		}
		daySeen[dayKey] = struct{}{}
		dayKeys = append(dayKeys, dayKey)
	}

	var rows []models.SmartMoneyWalletDailySnapshot
	if err := database.DB.WithContext(ctx).
		Where("chain_id IN ? AND wallet_address IN ? AND snapshot_day IN ?", chainIDs, addresses, dayKeys).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	for i := range rows {
		row := rows[i]
		key := row.SnapshotDay + "|" + smartMoneyWalletKey(row.ChainID, row.WalletAddress)
		rowCopy := row
		rowsByKey[key] = &rowCopy
	}
	return rowsByKey, nil
}

func (s *Service) loadSmartMoneyDailyStatRows(ctx context.Context, wallets []models.MonitoredWallet, day time.Time) (map[string]*models.SmartMoneyLPDailyStat, error) {
	rowsByKey := make(map[string]*models.SmartMoneyLPDailyStat)
	if len(wallets) == 0 {
		return rowsByKey, nil
	}

	chainIDs := make([]int, 0, len(wallets))
	addresses := make([]string, 0, len(wallets))
	chainSeen := make(map[int]struct{}, len(wallets))
	addrSeen := make(map[string]struct{}, len(wallets))
	for _, wallet := range wallets {
		if _, ok := chainSeen[wallet.ChainID]; !ok {
			chainSeen[wallet.ChainID] = struct{}{}
			chainIDs = append(chainIDs, wallet.ChainID)
		}
		addr := normalizeAddress(wallet.Address)
		if addr == "" {
			continue
		}
		if _, ok := addrSeen[addr]; ok {
			continue
		}
		addrSeen[addr] = struct{}{}
		addresses = append(addresses, addr)
	}

	var rows []models.SmartMoneyLPDailyStat
	if err := database.DB.WithContext(ctx).
		Where("chain_id IN ? AND wallet_address IN ? AND stat_day = ?", chainIDs, addresses, formatDay(day)).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	for i := range rows {
		row := rows[i]
		key := smartMoneyWalletKey(row.ChainID, row.WalletAddress)
		rowCopy := row
		rowsByKey[key] = &rowCopy
	}
	return rowsByKey, nil
}

func (s *Service) loadSmartMoneyDailyStatSeries(ctx context.Context, wallets []models.MonitoredWallet, start time.Time, end time.Time) (map[string]float64, error) {
	out := make(map[string]float64)
	if len(wallets) == 0 {
		return out, nil
	}

	chainIDs := make([]int, 0, len(wallets))
	addresses := make([]string, 0, len(wallets))
	chainSeen := make(map[int]struct{}, len(wallets))
	addrSeen := make(map[string]struct{}, len(wallets))
	for _, wallet := range wallets {
		if _, ok := chainSeen[wallet.ChainID]; !ok {
			chainSeen[wallet.ChainID] = struct{}{}
			chainIDs = append(chainIDs, wallet.ChainID)
		}
		addr := normalizeAddress(wallet.Address)
		if addr == "" {
			continue
		}
		if _, ok := addrSeen[addr]; ok {
			continue
		}
		addrSeen[addr] = struct{}{}
		addresses = append(addresses, addr)
	}

	type row struct {
		Day                     string
		EstimatedRealizedPnLUSD float64
	}
	var rows []row
	if err := database.DB.WithContext(ctx).
		Raw(`
			SELECT
				stat_day AS day,
				COALESCE(SUM(estimated_realized_pnl_usd), 0) AS estimated_realized_pnl_usd
			FROM sm_lp_daily_stats
			WHERE chain_id IN ?
			  AND wallet_address IN ?
			  AND stat_day >= ?
			  AND stat_day < ?
			GROUP BY stat_day
			ORDER BY stat_day ASC
		`, chainIDs, addresses, formatDay(start), formatDay(end)).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.Day] = round2(row.EstimatedRealizedPnLUSD)
	}
	return out, nil
}

func (s *Service) loadSmartMoneyHistory(ctx context.Context, wallets []models.MonitoredWallet, start time.Time, end time.Time) ([]SmartMoneyHistoryPoint, error) {
	if len(wallets) == 0 {
		return nil, nil
	}
	chainIDs := make([]int, 0, len(wallets))
	addresses := make([]string, 0, len(wallets))
	chainSeen := make(map[int]struct{})
	addrSeen := make(map[string]struct{})
	for _, wallet := range wallets {
		if _, ok := chainSeen[wallet.ChainID]; !ok {
			chainSeen[wallet.ChainID] = struct{}{}
			chainIDs = append(chainIDs, wallet.ChainID)
		}
		addr := normalizeAddress(wallet.Address)
		if _, ok := addrSeen[addr]; !ok {
			addrSeen[addr] = struct{}{}
			addresses = append(addresses, addr)
		}
	}

	type row struct {
		Day              string
		NativeUSD        float64
		StableUSD        float64
		TrackedTokenUSD  float64
		OpenLPUSD        float64
		TotalUSD         float64
		HasTransferIn    int
		HasTransferOut   int
		TransferInCount  int
		TransferOutCount int
		TransferInUSD    float64
		TransferOutUSD   float64
	}
	var rows []row
	openLPSelect := "COALESCE(SUM(open_lp_usd), 0) AS open_lp_usd"
	if database.DB != nil && !database.DB.Migrator().HasColumn(&models.SmartMoneyWalletDailySnapshot{}, "OpenLPUSD") {
		openLPSelect = "0 AS open_lp_usd"
	}
	err := database.DB.WithContext(ctx).
		Raw(fmt.Sprintf(`
			SELECT
				snapshot_day AS day,
				COALESCE(SUM(native_usd), 0) AS native_usd,
				COALESCE(SUM(stable_usd), 0) AS stable_usd,
				COALESCE(SUM(tracked_token_usd), 0) AS tracked_token_usd,
				%s,
				COALESCE(SUM(total_usd), 0) AS total_usd,
				MAX(CASE WHEN has_transfer_in THEN 1 ELSE 0 END) AS has_transfer_in,
				MAX(CASE WHEN has_transfer_out THEN 1 ELSE 0 END) AS has_transfer_out,
				COALESCE(SUM(transfer_in_count), 0) AS transfer_in_count,
				COALESCE(SUM(transfer_out_count), 0) AS transfer_out_count,
				COALESCE(SUM(transfer_in_usd), 0) AS transfer_in_usd,
				COALESCE(SUM(transfer_out_usd), 0) AS transfer_out_usd
			FROM sm_wallet_daily_snapshots
			WHERE chain_id IN ?
			  AND wallet_address IN ?
			  AND snapshot_day >= ?
			  AND snapshot_day < ?
			GROUP BY snapshot_day
			ORDER BY snapshot_day ASC
		`, openLPSelect), chainIDs, addresses, formatDay(start), formatDay(end)).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	points := make([]SmartMoneyHistoryPoint, 0, len(rows))
	previousTotalUSD := 0.0
	hasPreviousTotal := false
	for _, row := range rows {
		estimatedPnL := 0.0
		if hasPreviousTotal {
			estimatedPnL = round2(row.TotalUSD - previousTotalUSD)
		}
		points = append(points, SmartMoneyHistoryPoint{
			Day:                     row.Day,
			NativeUSD:               round2(row.NativeUSD),
			StableUSD:               round2(row.StableUSD),
			TrackedTokenUSD:         round2(row.TrackedTokenUSD),
			OpenLPUSD:               round2(row.OpenLPUSD),
			TotalUSD:                round2(row.TotalUSD),
			EstimatedRealizedPnLUSD: estimatedPnL,
			HasTransferIn:           row.HasTransferIn > 0,
			HasTransferOut:          row.HasTransferOut > 0,
			TransferInCount:         row.TransferInCount,
			TransferOutCount:        row.TransferOutCount,
			TransferInUSD:           round2(row.TransferInUSD),
			TransferOutUSD:          round2(row.TransferOutUSD),
		})
		previousTotalUSD = row.TotalUSD
		hasPreviousTotal = true
	}
	return points, nil
}

func (s *Service) computeSmartMoneyStats(ctx context.Context, wallets []models.MonitoredWallet, windowStart time.Time, windowEnd time.Time) (map[string]smartMoneyEventStats, error) {
	out := make(map[string]smartMoneyEventStats, len(wallets))
	if len(wallets) == 0 {
		return out, nil
	}

	addresses := make([]string, 0, len(wallets))
	chainIDs := make([]int, 0, len(wallets))
	addrSeen := make(map[string]struct{})
	chainSeen := make(map[int]struct{})
	validWallets := make(map[string]struct{}, len(wallets))
	for _, wallet := range wallets {
		key := smartMoneyWalletKey(wallet.ChainID, wallet.Address)
		validWallets[key] = struct{}{}
		out[key] = smartMoneyEventStats{activePools: make(map[string]struct{})}
		addr := normalizeAddress(wallet.Address)
		if _, ok := addrSeen[addr]; !ok {
			addrSeen[addr] = struct{}{}
			addresses = append(addresses, addr)
		}
		if _, ok := chainSeen[wallet.ChainID]; !ok {
			chainSeen[wallet.ChainID] = struct{}{}
			chainIDs = append(chainIDs, wallet.ChainID)
		}
	}

	var events []models.SmartMoneyLPEvent
	if err := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyLPEvent{}).
		Where("wallet_address IN ? AND chain_id IN ? AND tx_timestamp < ?", addresses, chainIDs, windowEnd).
		Where("event_type IN ?", []string{"add", "remove"}).
		Order("wallet_address ASC").
		Order("chain_id ASC").
		Order("tx_timestamp ASC").
		Order("block_number ASC").
		Order("log_index ASC").
		Find(&events).Error; err != nil {
		return nil, err
	}

	type eventState struct {
		openUSD   float64
		ambiguous bool
	}
	stateByWallet := make(map[string]map[string]eventState, len(wallets))

	for _, event := range events {
		walletKey := smartMoneyWalletKey(event.ChainID, event.WalletAddress)
		if _, ok := validWallets[walletKey]; !ok {
			continue
		}
		stats := out[walletKey]
		if stats.activePools == nil {
			stats.activePools = make(map[string]struct{})
		}
		if _, ok := stateByWallet[walletKey]; !ok {
			stateByWallet[walletKey] = make(map[string]eventState)
		}

		positionKey := smartMoneyPositionKey(event)
		eventUSD := smartMoneyEventUSD(event)
		inWindow := !event.TxTimestamp.Before(windowStart) && event.TxTimestamp.Before(windowEnd)
		if inWindow {
			pool := strings.ToLower(strings.TrimSpace(event.PoolAddress))
			if pool != "" {
				stats.activePools[pool] = struct{}{}
			}
		}

		switch strings.ToLower(strings.TrimSpace(event.EventType)) {
		case "add":
			if inWindow {
				stats.AddCount++
			}
			if positionKey != "" {
				state := stateByWallet[walletKey][positionKey]
				if state.openUSD > 0 || state.ambiguous {
					state.ambiguous = true
				} else {
					state.openUSD = eventUSD
				}
				stateByWallet[walletKey][positionKey] = state
			}
		case "remove":
			if inWindow {
				stats.RemoveCount++
			}
			matched := false
			if positionKey != "" {
				state := stateByWallet[walletKey][positionKey]
				if state.openUSD > 0 && !state.ambiguous && eventUSD > 0 {
					if inWindow {
						stats.MatchedRemoveCount++
						stats.EstimatedRealizedPnLUSD += eventUSD - state.openUSD
						stats.MatchedCostUSD += state.openUSD
					}
					matched = true
				}
				delete(stateByWallet[walletKey], positionKey)
			}
			if inWindow && !matched {
				stats.UnmatchedRemoveCount++
			}
		}
		out[walletKey] = stats
	}

	for key, stats := range out {
		stats.EstimatedRealizedPnLUSD = round2(stats.EstimatedRealizedPnLUSD)
		stats.MatchedCostUSD = round2(stats.MatchedCostUSD)
		out[key] = stats
	}
	return out, nil
}

func aggregateSmartMoneyWindowStats(days int, statsByWallet map[string]smartMoneyEventStats) SmartMoneyWindowStats {
	out := SmartMoneyWindowStats{Days: days}
	activePools := make(map[string]struct{})
	for _, stats := range statsByWallet {
		out.EstimatedRealizedPnLUSD += stats.EstimatedRealizedPnLUSD
		out.MatchedCostUSD += stats.MatchedCostUSD
		out.AddCount += stats.AddCount
		out.RemoveCount += stats.RemoveCount
		out.MatchedRemoveCount += stats.MatchedRemoveCount
		out.UnmatchedRemoveCount += stats.UnmatchedRemoveCount
		for pool := range stats.activePools {
			activePools[pool] = struct{}{}
		}
	}
	out.EstimatedRealizedPnLUSD = round2(out.EstimatedRealizedPnLUSD)
	out.MatchedCostUSD = round2(out.MatchedCostUSD)
	if out.MatchedCostUSD > 0 {
		out.YieldRate = round4(out.EstimatedRealizedPnLUSD / out.MatchedCostUSD)
	}
	out.ActivePoolCount = len(activePools)
	return out
}

func smartMoneyPositionKey(event models.SmartMoneyLPEvent) string {
	walletAddress := normalizeAddress(event.WalletAddress)
	poolAddress := normalizeAddress(event.PoolAddress)
	if walletAddress == "" || poolAddress == "" {
		return ""
	}
	if event.NftTokenID != nil && *event.NftTokenID > 0 {
		return walletAddress + "|" + poolAddress + "|nft|" + strconv.FormatUint(*event.NftTokenID, 10)
	}
	if event.TickLower != nil && event.TickUpper != nil {
		return walletAddress + "|" + poolAddress + "|range|" + strconv.Itoa(*event.TickLower) + "|" + strconv.Itoa(*event.TickUpper)
	}
	return ""
}

func smartMoneyEventUSD(event models.SmartMoneyLPEvent) float64 {
	if value := amountToFloat(pointerString(event.TotalUSD), 0); value > 0 {
		return round2(value)
	}
	return round2(amountToFloat(pointerString(event.Token0AmountUSD), 0) + amountToFloat(pointerString(event.Token1AmountUSD), 0))
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Service) detectSmartMoneyWalletTransferActivity(ctx context.Context, wallets []models.MonitoredWallet, day time.Time) (map[string]smartMoneyTransferActivity, error) {
	out := make(map[string]smartMoneyTransferActivity, len(wallets))
	if len(wallets) == 0 {
		return out, nil
	}

	byChain := make(map[int][]models.MonitoredWallet)
	for _, wallet := range wallets {
		key := smartMoneyWalletKey(wallet.ChainID, wallet.Address)
		out[key] = smartMoneyTransferActivity{}
		byChain[wallet.ChainID] = append(byChain[wallet.ChainID], wallet)
	}

	var firstErr error
	start := dayStart(day)
	end := dayEnd(day)
	for chainID, chainWallets := range byChain {
		if err := s.detectSmartMoneyWalletTransferActivityForChain(ctx, chainID, chainWallets, start, end, out); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			log.Printf("[Assets] smart money transfer detection failed chain=%d day=%s err=%v", chainID, formatDay(day), err)
		}
	}
	return out, firstErr
}

func (s *Service) detectSmartMoneyWalletTransferActivityForChain(ctx context.Context, chainID int, wallets []models.MonitoredWallet, start time.Time, end time.Time, out map[string]smartMoneyTransferActivity) error {
	if len(wallets) == 0 {
		return nil
	}

	chain := smartMoneyChainFromID(chainID)
	client, cc, err := s.getClientForChain(chain)
	if err != nil {
		return err
	}
	fromBlock, toBlock, ok, err := s.findBlockRangeForTimeWindow(ctx, client, start, end)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	baseTokens := []string{cc.StableAddress, cc.USDTAddress, cc.USDCAddress, cc.BUSDAddress, cc.WrappedNativeAddress}
	tokenAddresses, err := s.loadSmartMoneyTransferTokenAddresses(ctx, wallets, chainID, baseTokens, start)
	if err != nil {
		return err
	}
	if len(tokenAddresses) == 0 {
		return nil
	}
	stableAddr := normalizeAddress(cc.StableAddress)
	usdtAddr := normalizeAddress(cc.USDTAddress)
	usdcAddr := normalizeAddress(cc.USDCAddress)
	busdAddr := normalizeAddress(cc.BUSDAddress)
	wrappedNativeAddr := normalizeAddress(cc.WrappedNativeAddress)
	tokenAddressStrings := make([]string, 0, len(tokenAddresses))
	tokenMeta := make(map[string]struct {
		decimals int
		priceUSD float64
	}, len(tokenAddresses))
	for _, addr := range tokenAddresses {
		normalized := normalizeAddress(addr.Hex())
		if normalized == "" {
			continue
		}
		tokenAddressStrings = append(tokenAddressStrings, normalized)
	}
	prices, _ := s.priceService.GetUSDPrices(chain, tokenAddressStrings)
	nativePrice := s.nativePriceUSD(chain, cc)
	for _, tokenAddress := range tokenAddressStrings {
		priceUSD := round4(prices[tokenAddress])
		switch tokenAddress {
		case stableAddr, usdtAddr, usdcAddr, busdAddr:
			if priceUSD <= 0 {
				priceUSD = 1
			}
		case wrappedNativeAddr:
			if priceUSD <= 0 {
				priceUSD = nativePrice
			}
		}
		decimals := s.tokenFallbackDecimals(cc, tokenAddress)
		decimals = s.getTokenDecimals(chain, client, tokenAddress, decimals)
		tokenMeta[tokenAddress] = struct {
			decimals int
			priceUSD float64
		}{
			decimals: decimals,
			priceUSD: priceUSD,
		}
	}

	excludedTxHashes, err := s.loadSmartMoneyLPEventTxHashes(ctx, wallets, chainID, start, end)
	if err != nil {
		return err
	}

	walletTopics := make([]common.Hash, 0, len(wallets))
	for _, wallet := range wallets {
		addr := normalizeAddress(wallet.Address)
		if addr == "" {
			continue
		}
		walletTopics = append(walletTopics, common.BytesToHash(common.HexToAddress(addr).Bytes()))
	}
	if len(walletTopics) == 0 {
		return nil
	}

	transferSig := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	inTxs := make(map[string]map[string]struct{}, len(wallets))
	outTxs := make(map[string]map[string]struct{}, len(wallets))

	const walletChunkSize = 32
	const tokenChunkSize = 64

	for walletStart := 0; walletStart < len(walletTopics); walletStart += walletChunkSize {
		walletEnd := walletStart + walletChunkSize
		if walletEnd > len(walletTopics) {
			walletEnd = len(walletTopics)
		}
		walletChunk := walletTopics[walletStart:walletEnd]

		for tokenStart := 0; tokenStart < len(tokenAddresses); tokenStart += tokenChunkSize {
			tokenEnd := tokenStart + tokenChunkSize
			if tokenEnd > len(tokenAddresses) {
				tokenEnd = len(tokenAddresses)
			}
			tokenChunk := tokenAddresses[tokenStart:tokenEnd]

			outLogs, err := client.FilterLogs(ctx, ethereum.FilterQuery{
				FromBlock: new(big.Int).SetUint64(fromBlock),
				ToBlock:   new(big.Int).SetUint64(toBlock),
				Addresses: tokenChunk,
				Topics:    [][]common.Hash{{transferSig}, walletChunk},
			})
			if err != nil {
				return err
			}
			for _, lg := range outLogs {
				if len(lg.Topics) < 3 {
					continue
				}
				txHash := strings.ToLower(lg.TxHash.Hex())
				if _, excluded := excludedTxHashes[txHash]; excluded {
					continue
				}
				addr := normalizeAddress(common.BytesToAddress(lg.Topics[1].Bytes()).Hex())
				key := smartMoneyWalletKey(chainID, addr)
				if _, ok := out[key]; !ok {
					continue
				}
				if outTxs[key] == nil {
					outTxs[key] = make(map[string]struct{})
				}
				outTxs[key][txHash] = struct{}{}
				tokenAddress := normalizeAddress(lg.Address.Hex())
				meta, ok := tokenMeta[tokenAddress]
				if ok && len(lg.Data) > 0 && meta.priceUSD > 0 {
					amount := amountToFloat(new(big.Int).SetBytes(lg.Data).String(), meta.decimals)
					if amount > 0 {
						activity := out[key]
						activity.TransferOutUSD += amount * meta.priceUSD
						out[key] = activity
					}
				}
			}

			inLogs, err := client.FilterLogs(ctx, ethereum.FilterQuery{
				FromBlock: new(big.Int).SetUint64(fromBlock),
				ToBlock:   new(big.Int).SetUint64(toBlock),
				Addresses: tokenChunk,
				Topics:    [][]common.Hash{{transferSig}, nil, walletChunk},
			})
			if err != nil {
				return err
			}
			for _, lg := range inLogs {
				if len(lg.Topics) < 3 {
					continue
				}
				txHash := strings.ToLower(lg.TxHash.Hex())
				if _, excluded := excludedTxHashes[txHash]; excluded {
					continue
				}
				addr := normalizeAddress(common.BytesToAddress(lg.Topics[2].Bytes()).Hex())
				key := smartMoneyWalletKey(chainID, addr)
				if _, ok := out[key]; !ok {
					continue
				}
				if inTxs[key] == nil {
					inTxs[key] = make(map[string]struct{})
				}
				inTxs[key][txHash] = struct{}{}
				tokenAddress := normalizeAddress(lg.Address.Hex())
				meta, ok := tokenMeta[tokenAddress]
				if ok && len(lg.Data) > 0 && meta.priceUSD > 0 {
					amount := amountToFloat(new(big.Int).SetBytes(lg.Data).String(), meta.decimals)
					if amount > 0 {
						activity := out[key]
						activity.TransferInUSD += amount * meta.priceUSD
						out[key] = activity
					}
				}
			}
		}
	}

	for key, txs := range inTxs {
		activity := out[key]
		activity.HasTransferIn = len(txs) > 0
		activity.TransferInCount = len(txs)
		activity.TransferInUSD = round2(activity.TransferInUSD)
		out[key] = activity
	}
	for key, txs := range outTxs {
		activity := out[key]
		activity.HasTransferOut = len(txs) > 0
		activity.TransferOutCount = len(txs)
		activity.TransferOutUSD = round2(activity.TransferOutUSD)
		out[key] = activity
	}
	return nil
}

func (s *Service) loadSmartMoneyTransferTokenAddresses(ctx context.Context, wallets []models.MonitoredWallet, chainID int, baseTokens []string, start time.Time) ([]common.Address, error) {
	seen := make(map[string]common.Address)
	for _, raw := range baseTokens {
		addr := normalizeAddress(raw)
		if addr == "" {
			continue
		}
		seen[addr] = common.HexToAddress(addr)
	}

	addresses := make([]string, 0, len(wallets))
	addrSeen := make(map[string]struct{}, len(wallets))
	for _, wallet := range wallets {
		addr := normalizeAddress(wallet.Address)
		if addr == "" {
			continue
		}
		if _, ok := addrSeen[addr]; ok {
			continue
		}
		addrSeen[addr] = struct{}{}
		addresses = append(addresses, addr)
	}
	if len(addresses) == 0 {
		return nil, nil
	}

	cutoff := dayStart(start).AddDate(0, 0, -30)
	type tokenRow struct {
		TokenAddress string
	}
	var rows []tokenRow
	if err := database.DB.WithContext(ctx).
		Raw(`
			SELECT token_address
			FROM (
				SELECT token0_address AS token_address
				FROM sm_lp_events
				WHERE wallet_address IN ? AND chain_id = ? AND tx_timestamp >= ?
				UNION
				SELECT token1_address AS token_address
				FROM sm_lp_events
				WHERE wallet_address IN ? AND chain_id = ? AND tx_timestamp >= ?
				UNION
				SELECT token0_address AS token_address
				FROM sm_lp_positions
				WHERE wallet_address IN ? AND chain_id = ? AND status = 'open'
				UNION
				SELECT token1_address AS token_address
				FROM sm_lp_positions
				WHERE wallet_address IN ? AND chain_id = ? AND status = 'open'
			) tokens
		`, addresses, chainID, cutoff, addresses, chainID, cutoff, addresses, chainID, addresses, chainID).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		addr := normalizeAddress(row.TokenAddress)
		if addr == "" {
			continue
		}
		seen[addr] = common.HexToAddress(addr)
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]common.Address, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out, nil
}

func (s *Service) loadSmartMoneyLPEventTxHashes(ctx context.Context, wallets []models.MonitoredWallet, chainID int, start time.Time, end time.Time) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	if len(wallets) == 0 {
		return out, nil
	}

	addresses := make([]string, 0, len(wallets))
	seen := make(map[string]struct{}, len(wallets))
	for _, wallet := range wallets {
		addr := normalizeAddress(wallet.Address)
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		addresses = append(addresses, addr)
	}
	if len(addresses) == 0 {
		return out, nil
	}

	type txRow struct {
		TxHash string
	}
	var rows []txRow
	if err := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyLPEvent{}).
		Select("DISTINCT tx_hash").
		Where("chain_id = ? AND wallet_address IN ? AND tx_timestamp >= ? AND tx_timestamp < ?", chainID, addresses, start, end).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		txHash := strings.ToLower(strings.TrimSpace(row.TxHash))
		if txHash == "" {
			continue
		}
		out[txHash] = struct{}{}
	}
	return out, nil
}

func (s *Service) findBlockRangeForTimeWindow(ctx context.Context, client *ethclient.Client, start time.Time, end time.Time) (uint64, uint64, bool, error) {
	if client == nil || !start.Before(end) {
		return 0, 0, false, nil
	}

	latest, err := client.BlockNumber(ctx)
	if err != nil {
		return 0, 0, false, err
	}
	fromBlock, err := s.findFirstBlockAtOrAfter(ctx, client, latest, start)
	if err != nil {
		return 0, 0, false, err
	}
	if fromBlock > latest {
		return 0, 0, false, nil
	}

	toExclusive, err := s.findFirstBlockAtOrAfter(ctx, client, latest, end)
	if err != nil {
		return 0, 0, false, err
	}
	if toExclusive <= fromBlock {
		return 0, 0, false, nil
	}
	if toExclusive > latest {
		return fromBlock, latest, true, nil
	}
	return fromBlock, toExclusive - 1, true, nil
}

func (s *Service) findFirstBlockAtOrAfter(ctx context.Context, client *ethclient.Client, latest uint64, target time.Time) (uint64, error) {
	latestHeader, err := client.HeaderByNumber(ctx, new(big.Int).SetUint64(latest))
	if err != nil {
		return 0, err
	}
	if latestHeader == nil {
		return 0, fmt.Errorf("latest header unavailable")
	}
	if time.Unix(int64(latestHeader.Time), 0).Before(target) {
		return latest + 1, nil
	}

	low := uint64(0)
	high := latest
	for low < high {
		mid := low + (high-low)/2
		header, err := client.HeaderByNumber(ctx, new(big.Int).SetUint64(mid))
		if err != nil {
			return 0, err
		}
		if header == nil {
			return 0, fmt.Errorf("header unavailable for block %d", mid)
		}
		if time.Unix(int64(header.Time), 0).Before(target) {
			low = mid + 1
		} else {
			high = mid
		}
	}
	return low, nil
}

func (s *Service) captureSmartMoneyWalletSnapshots(ctx context.Context, day time.Time) error {
	wallets, err := s.loadActiveSmartMoneyWallets(ctx)
	if err != nil {
		return err
	}
	dayKey := formatDay(day)
	transferActivity, transferErr := s.detectSmartMoneyWalletTransferActivity(ctx, wallets, day)
	if transferErr != nil {
		log.Printf("[Assets] smart money transfer detection incomplete day=%s err=%v", dayKey, transferErr)
	}
	if err := database.DB.WithContext(ctx).
		Where("snapshot_day = ?", dayKey).
		Delete(&models.SmartMoneyWalletDailySnapshot{}).Error; err != nil {
		return err
	}
	for _, walletRow := range wallets {
		live, err := s.loadSmartMoneyWalletLiveStateLive(ctx, walletRow)
		if err != nil {
			log.Printf("[Assets] skip smart money snapshot wallet=%s chain=%d err=%v", walletRow.Address, walletRow.ChainID, err)
			continue
		}
		activity := transferActivity[smartMoneyWalletKey(walletRow.ChainID, walletRow.Address)]
		row := &models.SmartMoneyWalletDailySnapshot{
			WalletAddress:     normalizeAddress(walletRow.Address),
			ChainID:           walletRow.ChainID,
			SnapshotDay:       dayKey,
			NativeUSD:         live.assets.NativeUSD,
			StableUSD:         live.assets.StableUSD,
			TrackedTokenUSD:   live.assets.TrackedTokenUSD,
			OpenLPUSD:         live.assets.OpenLPUSD,
			TotalUSD:          live.assets.TotalUSD,
			TrackedTokenCount: live.assets.TrackedTokenCount,
			HasTransferIn:     activity.HasTransferIn,
			HasTransferOut:    activity.HasTransferOut,
			TransferInCount:   activity.TransferInCount,
			TransferOutCount:  activity.TransferOutCount,
			TransferInUSD:     round2(activity.TransferInUSD),
			TransferOutUSD:    round2(activity.TransferOutUSD),
			CapturedAt:        timeutil.Now(),
		}
		if err := upsertByColumns(ctx, row,
			[]string{"wallet_address", "chain_id", "snapshot_day"},
			map[string]interface{}{
				"native_usd":          row.NativeUSD,
				"stable_usd":          row.StableUSD,
				"tracked_token_usd":   row.TrackedTokenUSD,
				"open_lp_usd":         row.OpenLPUSD,
				"total_usd":           row.TotalUSD,
				"tracked_token_count": row.TrackedTokenCount,
				"has_transfer_in":     row.HasTransferIn,
				"has_transfer_out":    row.HasTransferOut,
				"transfer_in_count":   row.TransferInCount,
				"transfer_out_count":  row.TransferOutCount,
				"transfer_in_usd":     row.TransferInUSD,
				"transfer_out_usd":    row.TransferOutUSD,
				"captured_at":         row.CapturedAt,
			}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) captureSmartMoneyLPDailyStats(ctx context.Context, day time.Time) error {
	wallets, err := s.loadActiveSmartMoneyWallets(ctx)
	if err != nil {
		return err
	}
	dayKey := formatDay(day)
	if err := database.DB.WithContext(ctx).
		Where("stat_day = ?", dayKey).
		Delete(&models.SmartMoneyLPDailyStat{}).Error; err != nil {
		return err
	}

	statsByWallet, err := s.computeSmartMoneyStats(ctx, wallets, dayStart(day), dayEnd(day))
	if err != nil {
		return err
	}
	for _, walletRow := range wallets {
		key := smartMoneyWalletKey(walletRow.ChainID, walletRow.Address)
		stats := statsByWallet[key]
		row := &models.SmartMoneyLPDailyStat{
			WalletAddress:           normalizeAddress(walletRow.Address),
			ChainID:                 walletRow.ChainID,
			StatDay:                 dayKey,
			EstimatedRealizedPnLUSD: round2(stats.EstimatedRealizedPnLUSD),
			MatchedCostUSD:          round2(stats.MatchedCostUSD),
			MatchedRemoveCount:      stats.MatchedRemoveCount,
			UnmatchedRemoveCount:    stats.UnmatchedRemoveCount,
			AddCount:                stats.AddCount,
			RemoveCount:             stats.RemoveCount,
			ActivePoolCount:         len(stats.activePools),
			CapturedAt:              timeutil.Now(),
		}
		if err := upsertByColumns(ctx, row,
			[]string{"wallet_address", "chain_id", "stat_day"},
			map[string]interface{}{
				"estimated_realized_pnl_usd": row.EstimatedRealizedPnLUSD,
				"matched_cost_usd":           row.MatchedCostUSD,
				"matched_remove_count":       row.MatchedRemoveCount,
				"unmatched_remove_count":     row.UnmatchedRemoveCount,
				"add_count":                  row.AddCount,
				"remove_count":               row.RemoveCount,
				"active_pool_count":          row.ActivePoolCount,
				"captured_at":                row.CapturedAt,
			}); err != nil {
			return err
		}
	}
	return nil
}
