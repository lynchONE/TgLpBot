package assets

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gorm.io/gorm"
)

const (
	recognizedAssetBasis = "原生币 + 稳定币 + 近30天参与LP/普通转账记录代币余额 + 当前open LP估算持仓"
	// Reuse one short-lived Redis entry for all smart-money wallet balance reads.
	smartMoneyWalletLiveCacheTTL        = 30 * time.Minute
	smartMoneyWalletLiveRefreshInterval = 30 * time.Minute
	smartMoneyWalletLiveRefreshTimeout  = 20 * time.Minute
	smartMoneyWalletLiveRefreshWorkers  = 3
	smartMoneyWalletMaxHistoryDays      = 365
	smartMoneyLeaderboardCacheTTL       = 72 * time.Hour
	smartMoneyDefaultPageSize           = 10
	smartMoneyMaxPageSize               = 50
)

var smartMoneyLeaderboardMetrics = []string{"pnl", "yield_rate", "participation"}

type smartMoneyOverviewSection string

const (
	smartMoneyOverviewSectionSummary smartMoneyOverviewSection = "summary"
	smartMoneyOverviewSectionWallets smartMoneyOverviewSection = "wallets"
	smartMoneyOverviewSectionHistory smartMoneyOverviewSection = "history"
	smartMoneyOverviewSectionWindows smartMoneyOverviewSection = "windows"
)

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

type smartMoneyHistoryDayRow struct {
	Day                     string
	NativeUSD               float64
	StableUSD               float64
	TrackedTokenUSD         float64
	OpenLPUSD               float64
	TotalUSD                float64
	HasTransferIn           int
	HasTransferOut          int
	TransferInCount         int
	TransferOutCount        int
	TransferInUSD           float64
	TransferOutUSD          float64
	SnapshotCount           int     `gorm:"column:snapshot_count"`
	DailyStatCount          int     `gorm:"column:daily_stat_count"`
	EstimatedRealizedPnLUSD float64 `gorm:"column:estimated_realized_pnl_usd"`
}

type smartMoneyLeaderboardSnapshotInput struct {
	Wallet             models.MonitoredWallet
	Current            *models.SmartMoneyWalletDailySnapshot
	Previous           *models.SmartMoneyWalletDailySnapshot
	DailyStat          *models.SmartMoneyLPDailyStat
	UseRawAssetDelta   bool
	IgnoreDailyStatPnL bool
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

func readCachedSmartMoneyWalletLiveState(chainID int, address string) (smartMoneyWalletLiveState, bool) {
	var state smartMoneyWalletLiveState
	if database.RedisClient == nil {
		return state, false
	}
	raw, err := database.GetCache(smartMoneyWalletLiveCacheKey(chainID, address))
	if err != nil || strings.TrimSpace(raw) == "" {
		return state, false
	}
	var cached cachedSmartMoneyWalletLiveState
	if err := json.Unmarshal([]byte(raw), &cached); err != nil {
		return state, false
	}
	return cached.liveState(), true
}

func writeCachedSmartMoneyWalletLiveState(chainID int, address string, state smartMoneyWalletLiveState) {
	if database.RedisClient == nil {
		return
	}
	body, err := json.Marshal(newCachedSmartMoneyWalletLiveState(state))
	if err != nil {
		return
	}
	_ = database.SetCache(smartMoneyWalletLiveCacheKey(chainID, address), string(body), smartMoneyWalletLiveCacheTTL)
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

func smartMoneyLeaderboardDailyCacheKey(snapshotDay time.Time, metric string, days int) string {
	return fmt.Sprintf("assets:smart-money:leaderboard:v3:%s:%d:%s", formatDay(snapshotDay), clampLPDays(days), normalizeLeaderboardMetric(metric))
}

func smartMoneyLeaderboardLiveCacheKey(snapshotDay time.Time, metric string, days int) string {
	return fmt.Sprintf("assets:smart-money:leaderboard-live:v1:%s:%d:%s", formatDay(snapshotDay), clampLPDays(days), normalizeLeaderboardMetric(metric))
}

func transferNetUSD(transferInUSD float64, transferOutUSD float64) float64 {
	return round2(transferInUSD - transferOutUSD)
}

func transferTotalCount(transferInCount int, transferOutCount int) int {
	return transferInCount + transferOutCount
}

func adjustedPnL(delta float64, transferInUSD float64, transferOutUSD float64) float64 {
	return round2(delta - (transferInUSD - transferOutUSD))
}

func buildSmartMoneyHistoryPoints(rows []smartMoneyHistoryDayRow) []SmartMoneyHistoryPoint {
	if len(rows) == 0 {
		return nil
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Day < rows[j].Day
	})

	points := make([]SmartMoneyHistoryPoint, 0, len(rows))
	previousTotalUSD := 0.0
	previousDay := ""
	hasPreviousTotal := false
	for _, row := range rows {
		estimatedPnL := 0.0
		if row.SnapshotCount > 0 && row.DailyStatCount == row.SnapshotCount {
			estimatedPnL = round2(row.EstimatedRealizedPnLUSD)
		} else if hasPreviousTotal && isNextSnapshotDay(previousDay, row.Day) {
			estimatedPnL = adjustedPnL(row.TotalUSD-previousTotalUSD, row.TransferInUSD, row.TransferOutUSD)
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
			TransferTotalCount:      transferTotalCount(row.TransferInCount, row.TransferOutCount),
			TransferInUSD:           round2(row.TransferInUSD),
			TransferOutUSD:          round2(row.TransferOutUSD),
			TransferNetUSD:          transferNetUSD(row.TransferInUSD, row.TransferOutUSD),
		})
		previousTotalUSD = row.TotalUSD
		previousDay = row.Day
		hasPreviousTotal = true
	}
	return points
}

func buildSmartMoneyTodayHistoryPoint(day time.Time, assets smartMoneyAssetBreakdown, previousDay string, previousTotalUSD float64, hasPreviousTotal bool, activity smartMoneyTransferActivity) SmartMoneyHistoryPoint {
	pnl := 0.0
	if hasPreviousTotal && isNextSnapshotDay(previousDay, formatDay(day)) {
		pnl = round2(assets.TotalUSD - previousTotalUSD)
	}
	return SmartMoneyHistoryPoint{
		Day:                     formatDay(day),
		NativeUSD:               round2(assets.NativeUSD),
		StableUSD:               round2(assets.StableUSD),
		TrackedTokenUSD:         round2(assets.TrackedTokenUSD),
		OpenLPUSD:               round2(assets.OpenLPUSD),
		TotalUSD:                round2(assets.TotalUSD),
		EstimatedRealizedPnLUSD: pnl,
		HasTransferIn:           activity.HasTransferIn,
		HasTransferOut:          activity.HasTransferOut,
		TransferInCount:         activity.TransferInCount,
		TransferOutCount:        activity.TransferOutCount,
		TransferTotalCount:      transferTotalCount(activity.TransferInCount, activity.TransferOutCount),
		TransferInUSD:           round2(activity.TransferInUSD),
		TransferOutUSD:          round2(activity.TransferOutUSD),
		TransferNetUSD:          transferNetUSD(activity.TransferInUSD, activity.TransferOutUSD),
	}
}

func mergeSmartMoneyHistoryPoint(history []SmartMoneyHistoryPoint, point SmartMoneyHistoryPoint) []SmartMoneyHistoryPoint {
	if strings.TrimSpace(point.Day) == "" {
		return history
	}

	merged := make([]SmartMoneyHistoryPoint, 0, len(history)+1)
	replaced := false
	for _, item := range history {
		if item.Day == point.Day {
			if !replaced {
				merged = append(merged, point)
				replaced = true
			}
			continue
		}
		merged = append(merged, item)
	}
	if !replaced {
		merged = append(merged, point)
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Day < merged[j].Day
	})
	return merged
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

func readCachedSmartMoneyLeaderboard(snapshotDay time.Time, metric string, days int) (*SmartMoneyLeaderboardResponse, bool) {
	return readCachedSmartMoneyLeaderboardByKey(smartMoneyLeaderboardDailyCacheKey(snapshotDay, metric, days))
}

func readCachedSmartMoneyLiveLeaderboard(snapshotDay time.Time, metric string, days int) (*SmartMoneyLeaderboardResponse, bool) {
	return readCachedSmartMoneyLeaderboardByKey(smartMoneyLeaderboardLiveCacheKey(snapshotDay, metric, days))
}

func readCachedSmartMoneyLeaderboardByKey(key string) (*SmartMoneyLeaderboardResponse, bool) {
	if database.RedisClient == nil {
		return nil, false
	}
	raw, err := database.GetCache(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil, false
	}
	var resp SmartMoneyLeaderboardResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, false
	}
	return &resp, true
}

func writeCachedSmartMoneyLeaderboard(snapshotDay time.Time, metric string, days int, resp *SmartMoneyLeaderboardResponse) {
	writeCachedSmartMoneyLeaderboardByKey(smartMoneyLeaderboardDailyCacheKey(snapshotDay, metric, days), resp, smartMoneyLeaderboardCacheTTL)
}

func writeCachedSmartMoneyLiveLeaderboard(snapshotDay time.Time, metric string, days int, resp *SmartMoneyLeaderboardResponse) {
	writeCachedSmartMoneyLeaderboardByKey(smartMoneyLeaderboardLiveCacheKey(snapshotDay, metric, days), resp, smartMoneyWalletLiveCacheTTL)
}

func writeCachedSmartMoneyLeaderboardByKey(key string, resp *SmartMoneyLeaderboardResponse, ttl time.Duration) {
	if database.RedisClient == nil || resp == nil {
		return
	}
	body, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_ = database.SetCache(key, string(body), ttl)
}

func applySmartMoneyLeaderboardWalletMeta(resp *SmartMoneyLeaderboardResponse, wallets []models.MonitoredWallet) {
	if resp == nil || len(resp.List) == 0 || len(wallets) == 0 {
		return
	}

	metaByWallet := make(map[string]models.MonitoredWallet, len(wallets))
	for _, wallet := range wallets {
		metaByWallet[smartMoneyWalletKey(wallet.ChainID, wallet.Address)] = wallet
	}

	for i := range resp.List {
		entry := &resp.List[i]
		wallet, ok := metaByWallet[smartMoneyWalletKey(entry.ChainID, entry.Address)]
		if !ok {
			continue
		}

		entry.Label = ""
		if wallet.Label != nil {
			entry.Label = strings.TrimSpace(*wallet.Label)
		}

		entry.AvatarURL = ""
		if wallet.AvatarURL != nil {
			entry.AvatarURL = strings.TrimSpace(*wallet.AvatarURL)
		}
		entry.Source = strings.TrimSpace(wallet.Source)
		entry.SourceContract = smartMoneySourceContractValue(wallet)
	}
}

func smartMoneySourceContractValue(walletRow models.MonitoredWallet) string {
	if walletRow.SourceContract == nil {
		return ""
	}
	return strings.TrimSpace(*walletRow.SourceContract)
}

func smartMoneyWalletSummaryFromLive(walletRow models.MonitoredWallet, live smartMoneyWalletLiveState) SmartMoneyWalletSummary {
	summary := SmartMoneyWalletSummary{
		Address:         walletRow.Address,
		Source:          strings.TrimSpace(walletRow.Source),
		SourceContract:  smartMoneySourceContractValue(walletRow),
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
		Source:          strings.TrimSpace(walletRow.Source),
		SourceContract:  smartMoneySourceContractValue(walletRow),
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

func smartMoneyWalletLiveStateFromModel(row models.SmartMoneyWalletLiveState) smartMoneyWalletLiveState {
	state := smartMoneyWalletLiveState{
		assets: smartMoneyAssetBreakdown{
			NativeUSD:         round2(row.NativeUSD),
			StableUSD:         round2(row.StableUSD),
			TrackedTokenUSD:   round2(row.TrackedTokenUSD),
			OpenLPUSD:         round2(row.OpenLPUSD),
			TotalUSD:          round2(row.TotalUSD),
			TrackedTokenCount: row.TrackedTokenCount,
		},
		activePoolCount: row.ActivePoolCount,
		todayEventCount: row.TodayEventCount,
		lastActiveAt:    row.LastActiveAt,
	}
	if msg := strings.TrimSpace(row.ErrorMessage); msg != "" {
		state.warnings = append(state.warnings, fmt.Sprintf("上次实时资产刷新失败：%s", msg))
	}
	return state
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
	return s.GetSmartMoneyOverviewSections(ctx, days, page, size, keyword, forceRefresh, nil)
}

func (s *Service) GetSmartMoneyOverviewSections(ctx context.Context, days int, page int, size int, keyword string, forceRefresh bool, sections []string) (*SmartMoneyOverview, error) {
	days = clampLPDays(days)
	page = clampSmartMoneyPage(page)
	size = clampSmartMoneyPageSize(size)
	keyword = strings.TrimSpace(keyword)

	snapshotDay := dayStart(timeutil.Now()).AddDate(0, 0, -1)
	sectionSet := normalizeSmartMoneyOverviewSections(sections)

	resp := &SmartMoneyOverview{
		Wallets:          []SmartMoneyWalletSummary{},
		History:          []SmartMoneyHistoryPoint{},
		WalletPage:       page,
		WalletSize:       size,
		WalletTotalPages: 1,
		SnapshotDay:      formatDay(snapshotDay),
		UpdatedAt:        timeutil.Now(),
		Timezone:         timeutil.LocationName(),
	}

	if forceRefresh {
		if started := s.TriggerSmartMoneyWalletLiveStateRefresh(smartMoneyWalletLiveRefreshTimeout); started {
			resp.Warnings = append(resp.Warnings, "聪明钱实时资产刷新已开始，请稍后再刷新查看最新数据")
		} else {
			resp.Warnings = append(resp.Warnings, "聪明钱实时资产正在刷新中，请稍后再试")
		}
	}

	if sectionSet[smartMoneyOverviewSectionHistory] {
		start := dayStart(timeutil.Now()).AddDate(0, 0, -defaultHistoryDays)
		end := dayStart(timeutil.Now())
		history, err := s.loadSmartMoneyAggregateHistory(ctx, start, end)
		if err != nil {
			return nil, err
		}
		resp.History = history
	}

	if sectionSet[smartMoneyOverviewSectionSummary] {
		summary, today, ok, err := s.loadSmartMoneyOverviewSummaryFromLiveState(ctx, snapshotDay)
		if err != nil {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("实时聪明钱总览暂不可用：%v", err))
		}
		if !ok {
			summary, today, err = s.loadSmartMoneyOverviewSummary(ctx, snapshotDay)
			if err != nil {
				return nil, err
			}
		}
		resp.Summary = summary
		resp.Today = today
	}

	if sectionSet[smartMoneyOverviewSectionWallets] {
		pageWallets, total, err := s.loadPagedSmartMoneyWallets(ctx, page, size, keyword)
		if err != nil {
			return nil, err
		}
		resp.Wallets = make([]SmartMoneyWalletSummary, 0, len(pageWallets))
		resp.WalletTotal = int(total)
		if total > 0 {
			resp.WalletTotalPages = int((total + int64(size) - 1) / int64(size))
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
			live, ok, err := s.loadSmartMoneyWalletLiveStateForOverview(ctx, walletRow)
			if err != nil {
				resp.Warnings = append(resp.Warnings, fmt.Sprintf("钱包 %s 本地实时资产暂不可用：%v", walletRow.Address, err))
			}
			if ok {
				resp.Wallets = append(resp.Wallets, smartMoneyWalletSummaryFromLive(walletRow, live))
				resp.Warnings = append(resp.Warnings, live.warnings...)
				continue
			}
			key := formatDay(snapshotDay) + "|" + smartMoneyWalletKey(walletRow.ChainID, walletRow.Address)
			resp.Wallets = append(resp.Wallets, smartMoneyWalletSummaryFromSnapshot(walletRow, snapshotMap[key], lpStatMap[smartMoneyWalletKey(walletRow.ChainID, walletRow.Address)]))
		}
	}

	if sectionSet[smartMoneyOverviewSectionWindows] {
		window, err := s.loadSmartMoneyWindowStatsFromDaily(ctx, days)
		if err != nil {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("聪明钱每日统计暂不可用：%v", err))
			resp.Windows = []SmartMoneyWindowStats{{Days: days}}
		} else {
			resp.Windows = []SmartMoneyWindowStats{window}
		}
	}
	resp.Warnings = dedupeStrings(resp.Warnings)
	return resp, nil
}

func normalizeSmartMoneyOverviewSections(sections []string) map[smartMoneyOverviewSection]bool {
	out := make(map[smartMoneyOverviewSection]bool, 4)
	for _, section := range sections {
		for _, part := range strings.Split(section, ",") {
			switch strings.ToLower(strings.TrimSpace(part)) {
			case string(smartMoneyOverviewSectionSummary), "asset", "assets":
				out[smartMoneyOverviewSectionSummary] = true
			case string(smartMoneyOverviewSectionWallets), "wallet", "list":
				out[smartMoneyOverviewSectionWallets] = true
			case string(smartMoneyOverviewSectionHistory), "chart":
				out[smartMoneyOverviewSectionHistory] = true
			case string(smartMoneyOverviewSectionWindows), "window", "stats":
				out[smartMoneyOverviewSectionWindows] = true
			case "all":
				out[smartMoneyOverviewSectionSummary] = true
				out[smartMoneyOverviewSectionWallets] = true
				out[smartMoneyOverviewSectionHistory] = true
				out[smartMoneyOverviewSectionWindows] = true
			}
		}
	}
	if len(out) == 0 {
		out[smartMoneyOverviewSectionSummary] = true
		out[smartMoneyOverviewSectionWallets] = true
		out[smartMoneyOverviewSectionHistory] = true
		out[smartMoneyOverviewSectionWindows] = true
	}
	return out
}

func (s *Service) loadSmartMoneyOverviewSummary(ctx context.Context, snapshotDay time.Time) (smartMoneyAssetBreakdown, SmartMoneyHistoryPoint, error) {
	type row struct {
		NativeUSD         float64 `gorm:"column:native_usd"`
		StableUSD         float64 `gorm:"column:stable_usd"`
		TrackedTokenUSD   float64 `gorm:"column:tracked_token_usd"`
		OpenLPUSD         float64 `gorm:"column:open_lp_usd"`
		TotalUSD          float64 `gorm:"column:total_usd"`
		TrackedTokenCount int     `gorm:"column:tracked_token_count"`
		HasTransferIn     int     `gorm:"column:has_transfer_in"`
		HasTransferOut    int     `gorm:"column:has_transfer_out"`
		TransferInCount   int     `gorm:"column:transfer_in_count"`
		TransferOutCount  int     `gorm:"column:transfer_out_count"`
		TransferInUSD     float64 `gorm:"column:transfer_in_usd"`
		TransferOutUSD    float64 `gorm:"column:transfer_out_usd"`
	}

	openLPSelect := "COALESCE(SUM(s.open_lp_usd), 0) AS open_lp_usd"
	if database.DB != nil && !database.DB.Migrator().HasColumn(&models.SmartMoneyWalletDailySnapshot{}, "OpenLPUSD") {
		openLPSelect = "0 AS open_lp_usd"
	}

	var result row
	err := database.DB.WithContext(ctx).
		Raw(fmt.Sprintf(`
			SELECT
				COALESCE(SUM(s.native_usd), 0) AS native_usd,
				COALESCE(SUM(s.stable_usd), 0) AS stable_usd,
				COALESCE(SUM(s.tracked_token_usd), 0) AS tracked_token_usd,
				%s,
				COALESCE(SUM(s.total_usd), 0) AS total_usd,
				COALESCE(SUM(s.tracked_token_count), 0) AS tracked_token_count,
				MAX(CASE WHEN s.has_transfer_in THEN 1 ELSE 0 END) AS has_transfer_in,
				MAX(CASE WHEN s.has_transfer_out THEN 1 ELSE 0 END) AS has_transfer_out,
				COALESCE(SUM(s.transfer_in_count), 0) AS transfer_in_count,
				COALESCE(SUM(s.transfer_out_count), 0) AS transfer_out_count,
				COALESCE(SUM(s.transfer_in_usd), 0) AS transfer_in_usd,
				COALESCE(SUM(s.transfer_out_usd), 0) AS transfer_out_usd
			FROM sm_wallet_daily_snapshots s
			INNER JOIN monitored_wallets w
				ON w.address = s.wallet_address
				AND w.chain_id = s.chain_id
				AND w.is_active = 1
			WHERE s.snapshot_day = ?
		`, openLPSelect), formatDay(snapshotDay)).
		Scan(&result).Error
	if err != nil {
		return smartMoneyAssetBreakdown{}, SmartMoneyHistoryPoint{}, err
	}

	summary := smartMoneyAssetBreakdown{
		NativeUSD:         round2(result.NativeUSD),
		StableUSD:         round2(result.StableUSD),
		TrackedTokenUSD:   round2(result.TrackedTokenUSD),
		OpenLPUSD:         round2(result.OpenLPUSD),
		TotalUSD:          round2(result.TotalUSD),
		TrackedTokenCount: result.TrackedTokenCount,
	}
	today := SmartMoneyHistoryPoint{
		Day:                     formatDay(snapshotDay),
		NativeUSD:               summary.NativeUSD,
		StableUSD:               summary.StableUSD,
		TrackedTokenUSD:         summary.TrackedTokenUSD,
		OpenLPUSD:               summary.OpenLPUSD,
		TotalUSD:                summary.TotalUSD,
		HasTransferIn:           result.HasTransferIn > 0,
		HasTransferOut:          result.HasTransferOut > 0,
		TransferInCount:         result.TransferInCount,
		TransferOutCount:        result.TransferOutCount,
		TransferTotalCount:      transferTotalCount(result.TransferInCount, result.TransferOutCount),
		TransferInUSD:           round2(result.TransferInUSD),
		TransferOutUSD:          round2(result.TransferOutUSD),
		TransferNetUSD:          transferNetUSD(result.TransferInUSD, result.TransferOutUSD),
		EstimatedRealizedPnLUSD: 0,
	}
	return summary, today, nil
}

func (s *Service) loadSmartMoneyOverviewSummaryFromLiveState(ctx context.Context, snapshotDay time.Time) (smartMoneyAssetBreakdown, SmartMoneyHistoryPoint, bool, error) {
	type row struct {
		NativeUSD         float64 `gorm:"column:native_usd"`
		StableUSD         float64 `gorm:"column:stable_usd"`
		TrackedTokenUSD   float64 `gorm:"column:tracked_token_usd"`
		OpenLPUSD         float64 `gorm:"column:open_lp_usd"`
		TotalUSD          float64 `gorm:"column:total_usd"`
		TrackedTokenCount int     `gorm:"column:tracked_token_count"`
		RowCount          int64   `gorm:"column:row_count"`
	}

	var result row
	err := database.DB.WithContext(ctx).
		Raw(`
			SELECT
				COALESCE(SUM(ls.native_usd), 0) AS native_usd,
				COALESCE(SUM(ls.stable_usd), 0) AS stable_usd,
				COALESCE(SUM(ls.tracked_token_usd), 0) AS tracked_token_usd,
				COALESCE(SUM(ls.open_lp_usd), 0) AS open_lp_usd,
				COALESCE(SUM(ls.total_usd), 0) AS total_usd,
				COALESCE(SUM(ls.tracked_token_count), 0) AS tracked_token_count,
				COUNT(ls.wallet_address) AS row_count
			FROM sm_wallet_live_states ls
			INNER JOIN monitored_wallets w
				ON w.address = ls.wallet_address
				AND w.chain_id = ls.chain_id
				AND w.is_active = 1
		`).
		Scan(&result).Error
	if err != nil {
		return smartMoneyAssetBreakdown{}, SmartMoneyHistoryPoint{}, false, err
	}
	if result.RowCount == 0 {
		return smartMoneyAssetBreakdown{}, SmartMoneyHistoryPoint{}, false, nil
	}

	summary := smartMoneyAssetBreakdown{
		NativeUSD:         round2(result.NativeUSD),
		StableUSD:         round2(result.StableUSD),
		TrackedTokenUSD:   round2(result.TrackedTokenUSD),
		OpenLPUSD:         round2(result.OpenLPUSD),
		TotalUSD:          round2(result.TotalUSD),
		TrackedTokenCount: result.TrackedTokenCount,
	}
	today := SmartMoneyHistoryPoint{
		Day:                formatDay(snapshotDay),
		NativeUSD:          summary.NativeUSD,
		StableUSD:          summary.StableUSD,
		TrackedTokenUSD:    summary.TrackedTokenUSD,
		OpenLPUSD:          summary.OpenLPUSD,
		TotalUSD:           summary.TotalUSD,
		TransferTotalCount: 0,
	}
	return summary, today, true, nil
}

func (s *Service) loadSmartMoneyAggregateHistory(ctx context.Context, start time.Time, end time.Time) ([]SmartMoneyHistoryPoint, error) {
	if !start.Before(end) {
		return nil, nil
	}

	var rows []smartMoneyHistoryDayRow
	openLPSelect := "COALESCE(SUM(s.open_lp_usd), 0) AS open_lp_usd"
	if database.DB != nil && !database.DB.Migrator().HasColumn(&models.SmartMoneyWalletDailySnapshot{}, "OpenLPUSD") {
		openLPSelect = "0 AS open_lp_usd"
	}
	err := database.DB.WithContext(ctx).
		Raw(fmt.Sprintf(`
			SELECT
				s.snapshot_day AS day,
				COALESCE(SUM(s.native_usd), 0) AS native_usd,
				COALESCE(SUM(s.stable_usd), 0) AS stable_usd,
				COALESCE(SUM(s.tracked_token_usd), 0) AS tracked_token_usd,
				%s,
				COALESCE(SUM(s.total_usd), 0) AS total_usd,
				MAX(CASE WHEN s.has_transfer_in THEN 1 ELSE 0 END) AS has_transfer_in,
				MAX(CASE WHEN s.has_transfer_out THEN 1 ELSE 0 END) AS has_transfer_out,
				COALESCE(SUM(s.transfer_in_count), 0) AS transfer_in_count,
				COALESCE(SUM(s.transfer_out_count), 0) AS transfer_out_count,
				COALESCE(SUM(s.transfer_in_usd), 0) AS transfer_in_usd,
				COALESCE(SUM(s.transfer_out_usd), 0) AS transfer_out_usd,
				COUNT(s.wallet_address) AS snapshot_count,
				COUNT(st.stat_day) AS daily_stat_count,
				COALESCE(SUM(st.estimated_realized_pnl_usd), 0) AS estimated_realized_pnl_usd
			FROM sm_wallet_daily_snapshots s
			INNER JOIN monitored_wallets w
				ON w.address = s.wallet_address
				AND w.chain_id = s.chain_id
				AND w.is_active = 1
			LEFT JOIN sm_lp_daily_stats st
				ON st.wallet_address = s.wallet_address
				AND st.chain_id = s.chain_id
				AND st.stat_day = s.snapshot_day
			WHERE s.snapshot_day >= ?
			  AND s.snapshot_day < ?
			GROUP BY s.snapshot_day
			ORDER BY s.snapshot_day ASC
		`, openLPSelect), formatDay(start), formatDay(end)).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return buildSmartMoneyHistoryPoints(rows), nil
}

func (s *Service) loadSmartMoneyWindowStatsFromDaily(ctx context.Context, days int) (SmartMoneyWindowStats, error) {
	days = clampLPDays(days)
	snapshotDay := dayStart(timeutil.Now()).AddDate(0, 0, -1)
	startDay := snapshotDay.AddDate(0, 0, -(days - 1))

	type row struct {
		EstimatedRealizedPnLUSD float64 `gorm:"column:estimated_realized_pnl_usd"`
		MatchedCostUSD          float64 `gorm:"column:matched_cost_usd"`
		AddCount                int     `gorm:"column:add_count"`
		RemoveCount             int     `gorm:"column:remove_count"`
		MatchedRemoveCount      int     `gorm:"column:matched_remove_count"`
		UnmatchedRemoveCount    int     `gorm:"column:unmatched_remove_count"`
		ActivePoolCount         int     `gorm:"column:active_pool_count"`
	}
	var result row
	err := database.DB.WithContext(ctx).
		Raw(`
			SELECT
				COALESCE(SUM(s.estimated_realized_pnl_usd), 0) AS estimated_realized_pnl_usd,
				COALESCE(SUM(s.matched_cost_usd), 0) AS matched_cost_usd,
				COALESCE(SUM(s.add_count), 0) AS add_count,
				COALESCE(SUM(s.remove_count), 0) AS remove_count,
				COALESCE(SUM(s.matched_remove_count), 0) AS matched_remove_count,
				COALESCE(SUM(s.unmatched_remove_count), 0) AS unmatched_remove_count,
				COALESCE(SUM(s.active_pool_count), 0) AS active_pool_count
			FROM sm_lp_daily_stats s
			INNER JOIN monitored_wallets w
				ON w.address = s.wallet_address
				AND w.chain_id = s.chain_id
				AND w.is_active = 1
			WHERE s.stat_day >= ?
			  AND s.stat_day <= ?
		`, formatDay(startDay), formatDay(snapshotDay)).
		Scan(&result).Error
	if err != nil {
		return SmartMoneyWindowStats{}, err
	}

	out := SmartMoneyWindowStats{
		Days:                    days,
		EstimatedRealizedPnLUSD: round2(result.EstimatedRealizedPnLUSD),
		MatchedCostUSD:          round2(result.MatchedCostUSD),
		AddCount:                result.AddCount,
		RemoveCount:             result.RemoveCount,
		MatchedRemoveCount:      result.MatchedRemoveCount,
		UnmatchedRemoveCount:    result.UnmatchedRemoveCount,
		ActivePoolCount:         result.ActivePoolCount,
	}
	if out.MatchedCostUSD > 0 {
		out.YieldRate = round4(out.EstimatedRealizedPnLUSD / out.MatchedCostUSD)
	}
	return out, nil
}

func (s *Service) GetSmartMoneyWallet(ctx context.Context, address string, chainID int, days int, forceRefresh bool) (*SmartMoneyWalletResponse, error) {
	address = normalizeAddress(address)
	if address == "" {
		return nil, fmt.Errorf("invalid wallet address")
	}
	days = clampSmartMoneyWalletHistoryDays(days)
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

	startOfToday := dayStart(timeutil.Now())
	start := startOfToday.AddDate(0, 0, -days)
	end := startOfToday
	history, err := s.loadSmartMoneyHistory(ctx, []models.MonitoredWallet{*walletRow}, start, end)
	if err != nil {
		return nil, err
	}

	windowDays := []int{1, 7, 30}
	now := timeutil.Now()
	walletKey := smartMoneyWalletKey(walletRow.ChainID, walletRow.Address)

	type statsResult struct {
		windows    []SmartMoneyWindowStats
		todayStats smartMoneyEventStats
	}
	type transferResult struct {
		items map[string]smartMoneyTransferActivity
	}
	statsCh := make(chan statsResult, 1)
	statsErrCh := make(chan error, 1)
	transferCh := make(chan transferResult, 1)
	transferErrCh := make(chan error, 1)

	go func() {
		windows := make([]SmartMoneyWindowStats, 0, len(windowDays))
		for _, window := range windowDays {
			statsByWallet, err := s.computeSmartMoneyStats(ctx, []models.MonitoredWallet{*walletRow}, dayStart(now).AddDate(0, 0, -window), now)
			if err != nil {
				statsErrCh <- err
				return
			}
			windows = append(windows, aggregateSmartMoneyWindowStats(window, statsByWallet))
		}
		todayStatsByWallet, err := s.computeSmartMoneyStats(ctx, []models.MonitoredWallet{*walletRow}, dayStart(now), now)
		if err != nil {
			statsErrCh <- err
			return
		}
		todayStats := todayStatsByWallet[walletKey]
		statsCh <- statsResult{windows: windows, todayStats: todayStats}
	}()

	go func() {
		items, err := s.loadSmartMoneyTransferActivity(ctx, []models.MonitoredWallet{*walletRow}, dayStart(now), now)
		if err != nil {
			transferErrCh <- err
			return
		}
		transferCh <- transferResult{items: items}
	}()

	var stats statsResult
	var todayTransferByWallet map[string]smartMoneyTransferActivity
	for i := 0; i < 2; i++ {
		select {
		case stats = <-statsCh:
		case err := <-statsErrCh:
			return nil, err
		case transfer := <-transferCh:
			todayTransferByWallet = transfer.items
		case err := <-transferErrCh:
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	todayStats := stats.todayStats
	previousTotalUSD := 0.0
	previousDay := ""
	hasPreviousTotal := false
	if len(history) > 0 {
		last := history[len(history)-1]
		previousDay = last.Day
		previousTotalUSD = last.TotalUSD
		hasPreviousTotal = true
	}
	todayPoint := buildSmartMoneyTodayHistoryPoint(now, summary.Assets, previousDay, previousTotalUSD, hasPreviousTotal, todayTransferByWallet[walletKey])
	todayPoint.EstimatedRealizedPnLUSD = round2(todayStats.EstimatedRealizedPnLUSD)
	history = mergeSmartMoneyHistoryPoint(history, todayPoint)

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
		Windows:   stats.windows,
		UpdatedAt: timeutil.Now(),
		Timezone:  timeutil.LocationName(),
		Warnings:  dedupeStrings(live.warnings),
	}, nil
}

func clampSmartMoneyWalletHistoryDays(days int) int {
	if days <= 0 {
		return defaultHistoryDays
	}
	if days > smartMoneyWalletMaxHistoryDays {
		return smartMoneyWalletMaxHistoryDays
	}
	return days
}

func (s *Service) GetSmartMoneyLeaderboard(ctx context.Context, metric string, days int, page int, pageSize int, keyword string, forceRefresh bool) (*SmartMoneyLeaderboardResponse, error) {
	metric = normalizeLeaderboardMetric(metric)
	days = clampLPDays(days)
	snapshotDay := dayStart(timeutil.Now()).AddDate(0, 0, -1)
	if days == 1 {
		if !forceRefresh {
			if cached, ok := readCachedSmartMoneyLiveLeaderboard(snapshotDay, metric, days); ok {
				cached.Timezone = timeutil.LocationName()
				return paginateSmartMoneyLeaderboardResponse(cached, page, pageSize, keyword), nil
			}
		}
		inputs, err := s.buildSmartMoneyLeaderboardLiveInputs(ctx, snapshotDay)
		if err != nil {
			return nil, err
		}
		fullResp := buildSmartMoneySnapshotLeaderboard(metric, timeutil.Now(), snapshotDay, days, 0, inputs)
		fullResp.Timezone = timeutil.LocationName()
		fullResp.SnapshotDay = formatDay(timeutil.Now())
		fullResp.ComparedDay = formatDay(snapshotDay)
		fullResp.StartDay = formatDay(snapshotDay)
		fullResp.EndDay = formatDay(timeutil.Now())
		fullResp.Description = liveLeaderboardDescription(metric)
		writeCachedSmartMoneyLiveLeaderboard(snapshotDay, metric, days, fullResp)
		return paginateSmartMoneyLeaderboardResponse(fullResp, page, pageSize, keyword), nil
	}
	startDay := snapshotDay.AddDate(0, 0, -(days - 1))
	comparedDay := startDay.AddDate(0, 0, -1)

	if !forceRefresh {
		if cached, ok := readCachedSmartMoneyLeaderboard(snapshotDay, metric, days); ok {
			cached.Timezone = timeutil.LocationName()
			return paginateSmartMoneyLeaderboardResponse(cached, page, pageSize, keyword), nil
		}
	}

	inputs, err := s.buildSmartMoneyLeaderboardSnapshotInputs(ctx, snapshotDay, comparedDay, startDay)
	if err != nil {
		return nil, err
	}

	fullResp := buildSmartMoneySnapshotLeaderboard(metric, snapshotDay, comparedDay, days, 0, inputs)
	fullResp.Timezone = timeutil.LocationName()
	writeCachedSmartMoneyLeaderboard(snapshotDay, metric, days, fullResp)
	return paginateSmartMoneyLeaderboardResponse(fullResp, page, pageSize, keyword), nil
}

func (s *Service) deleteCachedSmartMoneyLeaderboards(snapshotDay time.Time) {
	if database.RedisClient == nil {
		return
	}
	for _, metric := range smartMoneyLeaderboardMetrics {
		for _, days := range []int{1, 7, 30} {
			_ = database.DeleteCache(smartMoneyLeaderboardDailyCacheKey(snapshotDay, metric, days))
		}
	}
}

func (s *Service) deleteCachedSmartMoneyLiveLeaderboards(snapshotDay time.Time) {
	if database.RedisClient == nil {
		return
	}
	for _, metric := range smartMoneyLeaderboardMetrics {
		_ = database.DeleteCache(smartMoneyLeaderboardLiveCacheKey(snapshotDay, metric, 1))
	}
}

func (s *Service) buildSmartMoneyLeaderboardSnapshotInputs(ctx context.Context, snapshotDay time.Time, comparedDay time.Time, startDay time.Time) ([]smartMoneyLeaderboardSnapshotInput, error) {
	type row struct {
		WalletAddress  string         `gorm:"column:wallet_address"`
		ChainID        int            `gorm:"column:chain_id"`
		Label          sql.NullString `gorm:"column:label"`
		AvatarURL      sql.NullString `gorm:"column:avatar_url"`
		Source         string         `gorm:"column:source"`
		SourceContract sql.NullString `gorm:"column:source_contract"`

		CurrentTotalUSD         float64        `gorm:"column:current_total_usd"`
		CurrentHasTransferIn    int            `gorm:"column:current_has_transfer_in"`
		CurrentHasTransferOut   int            `gorm:"column:current_has_transfer_out"`
		CurrentTransferInCount  int            `gorm:"column:current_transfer_in_count"`
		CurrentTransferOutCount int            `gorm:"column:current_transfer_out_count"`
		CurrentTransferInUSD    float64        `gorm:"column:current_transfer_in_usd"`
		CurrentTransferOutUSD   float64        `gorm:"column:current_transfer_out_usd"`
		PreviousTotalUSD        float64        `gorm:"column:previous_total_usd"`
		PreviousWalletAddress   sql.NullString `gorm:"column:previous_wallet_address"`

		StatWalletAddress    sql.NullString `gorm:"column:stat_wallet_address"`
		AddCount             int            `gorm:"column:add_count"`
		RemoveCount          int            `gorm:"column:remove_count"`
		ActivePoolCount      int            `gorm:"column:active_pool_count"`
		UnmatchedRemoveCount int            `gorm:"column:unmatched_remove_count"`
		EstimatedPnLUSD      float64        `gorm:"column:estimated_realized_pnl_usd"`
		MatchedCostUSD       float64        `gorm:"column:matched_cost_usd"`
	}

	var rows []row
	if err := database.DB.WithContext(ctx).
		Raw(`
			SELECT
				w.address AS wallet_address,
				w.chain_id AS chain_id,
				w.label AS label,
				w.avatar_url AS avatar_url,
				w.source AS source,
				w.source_contract AS source_contract,
				cur.total_usd AS current_total_usd,
				COALESCE(tx.has_transfer_in, 0) AS current_has_transfer_in,
				COALESCE(tx.has_transfer_out, 0) AS current_has_transfer_out,
				COALESCE(tx.transfer_in_count, 0) AS current_transfer_in_count,
				COALESCE(tx.transfer_out_count, 0) AS current_transfer_out_count,
				COALESCE(tx.transfer_in_usd, 0) AS current_transfer_in_usd,
				COALESCE(tx.transfer_out_usd, 0) AS current_transfer_out_usd,
				prev.total_usd AS previous_total_usd,
				prev.wallet_address AS previous_wallet_address,
				stat.stat_wallet_address AS stat_wallet_address,
				COALESCE(stat.add_count, 0) AS add_count,
				COALESCE(stat.remove_count, 0) AS remove_count,
				COALESCE(stat.active_pool_count, 0) AS active_pool_count,
				COALESCE(stat.unmatched_remove_count, 0) AS unmatched_remove_count,
				COALESCE(stat.estimated_realized_pnl_usd, 0) AS estimated_realized_pnl_usd,
				COALESCE(stat.matched_cost_usd, 0) AS matched_cost_usd
			FROM monitored_wallets w
			INNER JOIN sm_wallet_daily_snapshots cur
				ON cur.wallet_address = w.address
				AND cur.chain_id = w.chain_id
				AND cur.snapshot_day = ?
			LEFT JOIN sm_wallet_daily_snapshots prev
				ON prev.wallet_address = w.address
				AND prev.chain_id = w.chain_id
				AND prev.snapshot_day = ?
			LEFT JOIN (
				SELECT
					wallet_address,
					chain_id,
					MAX(CASE WHEN has_transfer_in THEN 1 ELSE 0 END) AS has_transfer_in,
					MAX(CASE WHEN has_transfer_out THEN 1 ELSE 0 END) AS has_transfer_out,
					COALESCE(SUM(transfer_in_count), 0) AS transfer_in_count,
					COALESCE(SUM(transfer_out_count), 0) AS transfer_out_count,
					COALESCE(SUM(transfer_in_usd), 0) AS transfer_in_usd,
					COALESCE(SUM(transfer_out_usd), 0) AS transfer_out_usd
				FROM sm_wallet_daily_snapshots
				WHERE snapshot_day >= ?
				  AND snapshot_day <= ?
				GROUP BY wallet_address, chain_id
			) tx
				ON tx.wallet_address = w.address
				AND tx.chain_id = w.chain_id
			LEFT JOIN (
				SELECT
					wallet_address,
					chain_id,
					MIN(wallet_address) AS stat_wallet_address,
					COALESCE(SUM(estimated_realized_pnl_usd), 0) AS estimated_realized_pnl_usd,
					COALESCE(SUM(matched_cost_usd), 0) AS matched_cost_usd,
					COALESCE(SUM(add_count), 0) AS add_count,
					COALESCE(SUM(remove_count), 0) AS remove_count,
					COALESCE(MAX(active_pool_count), 0) AS active_pool_count,
					COALESCE(SUM(unmatched_remove_count), 0) AS unmatched_remove_count
				FROM sm_lp_daily_stats
				WHERE stat_day >= ?
				  AND stat_day <= ?
				GROUP BY wallet_address, chain_id
			) stat
				ON stat.wallet_address = w.address
				AND stat.chain_id = w.chain_id
			WHERE w.is_active = 1
		`, formatDay(snapshotDay), formatDay(comparedDay), formatDay(startDay), formatDay(snapshotDay), formatDay(startDay), formatDay(snapshotDay)).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	inputs := make([]smartMoneyLeaderboardSnapshotInput, 0, len(rows))
	for _, item := range rows {
		walletRow := models.MonitoredWallet{
			Address: normalizeAddress(item.WalletAddress),
			ChainID: item.ChainID,
			Source:  strings.TrimSpace(item.Source),
		}
		if walletRow.Address == "" {
			continue
		}
		if item.Label.Valid {
			label := strings.TrimSpace(item.Label.String)
			walletRow.Label = &label
		}
		if item.AvatarURL.Valid {
			avatarURL := strings.TrimSpace(item.AvatarURL.String)
			walletRow.AvatarURL = &avatarURL
		}
		if item.SourceContract.Valid {
			sourceContract := strings.TrimSpace(item.SourceContract.String)
			walletRow.SourceContract = &sourceContract
		}
		current := &models.SmartMoneyWalletDailySnapshot{
			TotalUSD:         item.CurrentTotalUSD,
			HasTransferIn:    item.CurrentHasTransferIn > 0,
			HasTransferOut:   item.CurrentHasTransferOut > 0,
			TransferInCount:  item.CurrentTransferInCount,
			TransferOutCount: item.CurrentTransferOutCount,
			TransferInUSD:    item.CurrentTransferInUSD,
			TransferOutUSD:   item.CurrentTransferOutUSD,
		}
		var previous *models.SmartMoneyWalletDailySnapshot
		if item.PreviousWalletAddress.Valid {
			previous = &models.SmartMoneyWalletDailySnapshot{TotalUSD: item.PreviousTotalUSD}
		}
		var dailyStat *models.SmartMoneyLPDailyStat
		if item.StatWalletAddress.Valid {
			dailyStat = &models.SmartMoneyLPDailyStat{
				EstimatedRealizedPnLUSD: item.EstimatedPnLUSD,
				MatchedCostUSD:          item.MatchedCostUSD,
				AddCount:                item.AddCount,
				RemoveCount:             item.RemoveCount,
				ActivePoolCount:         item.ActivePoolCount,
				UnmatchedRemoveCount:    item.UnmatchedRemoveCount,
			}
		}
		inputs = append(inputs, smartMoneyLeaderboardSnapshotInput{
			Wallet:    walletRow,
			Current:   current,
			Previous:  previous,
			DailyStat: dailyStat,
		})
	}
	return inputs, nil
}

func (s *Service) buildSmartMoneyLeaderboardLiveInputs(ctx context.Context, baseDay time.Time) ([]smartMoneyLeaderboardSnapshotInput, error) {
	type row struct {
		WalletAddress  string         `gorm:"column:wallet_address"`
		ChainID        int            `gorm:"column:chain_id"`
		Label          sql.NullString `gorm:"column:label"`
		AvatarURL      sql.NullString `gorm:"column:avatar_url"`
		Source         string         `gorm:"column:source"`
		SourceContract sql.NullString `gorm:"column:source_contract"`

		CurrentTotalUSD  float64 `gorm:"column:current_total_usd"`
		PreviousTotalUSD float64 `gorm:"column:previous_total_usd"`

		AddCount             int `gorm:"column:add_count"`
		RemoveCount          int `gorm:"column:remove_count"`
		ActivePoolCount      int `gorm:"column:active_pool_count"`
		UnmatchedRemoveCount int `gorm:"column:unmatched_remove_count"`
	}

	now := timeutil.Now()
	todayStart := dayStart(now)
	var rows []row
	if err := database.DB.WithContext(ctx).
		Raw(`
			SELECT
				w.address AS wallet_address,
				w.chain_id AS chain_id,
				w.label AS label,
				w.avatar_url AS avatar_url,
				w.source AS source,
				w.source_contract AS source_contract,
				live.total_usd AS current_total_usd,
				base.total_usd AS previous_total_usd,
				COALESCE(stat.add_count, 0) AS add_count,
				COALESCE(stat.remove_count, 0) AS remove_count,
				COALESCE(live.active_pool_count, 0) AS active_pool_count,
				COALESCE(stat.unmatched_remove_count, 0) AS unmatched_remove_count
			FROM monitored_wallets w
			INNER JOIN sm_wallet_live_states live
				ON live.wallet_address = w.address
				AND live.chain_id = w.chain_id
			INNER JOIN sm_wallet_daily_snapshots base
				ON base.wallet_address = w.address
				AND base.chain_id = w.chain_id
				AND base.snapshot_day = ?
			LEFT JOIN (
				SELECT
					wallet_address,
					chain_id,
					COALESCE(SUM(CASE WHEN event_type = 'add' THEN 1 ELSE 0 END), 0) AS add_count,
					COALESCE(SUM(CASE WHEN event_type = 'remove' THEN 1 ELSE 0 END), 0) AS remove_count,
					0 AS unmatched_remove_count
				FROM sm_lp_events
				WHERE tx_timestamp >= ?
				  AND tx_timestamp < ?
				GROUP BY wallet_address, chain_id
			) stat
				ON stat.wallet_address = w.address
				AND stat.chain_id = w.chain_id
			WHERE w.is_active = 1
		`, formatDay(baseDay), todayStart, now).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	inputs := make([]smartMoneyLeaderboardSnapshotInput, 0, len(rows))
	for _, item := range rows {
		walletRow := models.MonitoredWallet{
			Address: normalizeAddress(item.WalletAddress),
			ChainID: item.ChainID,
			Source:  strings.TrimSpace(item.Source),
		}
		if walletRow.Address == "" {
			continue
		}
		if item.Label.Valid {
			label := strings.TrimSpace(item.Label.String)
			walletRow.Label = &label
		}
		if item.AvatarURL.Valid {
			avatarURL := strings.TrimSpace(item.AvatarURL.String)
			walletRow.AvatarURL = &avatarURL
		}
		if item.SourceContract.Valid {
			sourceContract := strings.TrimSpace(item.SourceContract.String)
			walletRow.SourceContract = &sourceContract
		}
		inputs = append(inputs, smartMoneyLeaderboardSnapshotInput{
			Wallet: walletRow,
			Current: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD: item.CurrentTotalUSD,
			},
			Previous: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD: item.PreviousTotalUSD,
			},
			DailyStat: &models.SmartMoneyLPDailyStat{
				AddCount:             item.AddCount,
				RemoveCount:          item.RemoveCount,
				ActivePoolCount:      item.ActivePoolCount,
				UnmatchedRemoveCount: item.UnmatchedRemoveCount,
			},
			UseRawAssetDelta:   true,
			IgnoreDailyStatPnL: true,
		})
	}
	return inputs, nil
}

func (s *Service) refreshSmartMoneyLeaderboardCaches(ctx context.Context, snapshotDay time.Time) error {
	if database.RedisClient == nil {
		return nil
	}
	for _, days := range []int{1, 7, 30} {
		startDay := snapshotDay.AddDate(0, 0, -(days - 1))
		comparedDay := startDay.AddDate(0, 0, -1)
		inputs, err := s.buildSmartMoneyLeaderboardSnapshotInputs(ctx, snapshotDay, comparedDay, startDay)
		if err != nil {
			return err
		}
		for _, metric := range smartMoneyLeaderboardMetrics {
			resp := buildSmartMoneySnapshotLeaderboard(metric, snapshotDay, comparedDay, days, 0, inputs)
			resp.Timezone = timeutil.LocationName()
			writeCachedSmartMoneyLeaderboard(snapshotDay, metric, days, resp)
		}
	}
	s.deleteCachedSmartMoneyLeaderboards(snapshotDay.AddDate(0, 0, -1))
	return nil
}

func (s *Service) refreshSmartMoneyLiveLeaderboardCaches(ctx context.Context, baseDay time.Time) error {
	if database.RedisClient == nil {
		return nil
	}
	inputs, err := s.buildSmartMoneyLeaderboardLiveInputs(ctx, baseDay)
	if err != nil {
		return err
	}
	now := timeutil.Now()
	for _, metric := range smartMoneyLeaderboardMetrics {
		resp := buildSmartMoneySnapshotLeaderboard(metric, now, baseDay, 1, 0, inputs)
		resp.Timezone = timeutil.LocationName()
		resp.SnapshotDay = formatDay(now)
		resp.ComparedDay = formatDay(baseDay)
		resp.StartDay = formatDay(baseDay)
		resp.EndDay = formatDay(now)
		resp.Description = liveLeaderboardDescription(metric)
		writeCachedSmartMoneyLiveLeaderboard(baseDay, metric, 1, resp)
	}
	return nil
}

type smartMoneyLiveRefreshResult struct {
	refreshed int
	skipped   int
	failed    int
}

func (s *Service) RefreshSmartMoneyWalletLiveStates(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("asset service not initialized")
	}
	result, err := s.refreshSmartMoneyWalletLiveStates(ctx)
	if err != nil {
		return fmt.Errorf("smart money live-state refresh incomplete refreshed=%d skipped=%d failed=%d: %w", result.refreshed, result.skipped, result.failed, err)
	}
	if result.failed > 0 {
		return fmt.Errorf("smart money live-state refresh had %d failed wallets", result.failed)
	}
	return nil
}

func (s *Service) TriggerSmartMoneyWalletLiveStateRefresh(timeout time.Duration) bool {
	if s == nil {
		return false
	}
	if timeout <= 0 {
		timeout = smartMoneyWalletLiveRefreshTimeout
	}
	if !s.smartMoneyLiveRefreshMu.TryLock() {
		return false
	}
	go func() {
		defer s.smartMoneyLiveRefreshMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		result, err := s.refreshSmartMoneyWalletLiveStates(ctx)
		if err != nil {
			log.Printf("[Assets] smart money live-state refresh incomplete refreshed=%d skipped=%d failed=%d err=%v", result.refreshed, result.skipped, result.failed, err)
			return
		}
		snapshotDay := dayStart(timeutil.Now()).AddDate(0, 0, -1)
		s.deleteCachedSmartMoneyLiveLeaderboards(snapshotDay)
		if cacheErr := s.refreshSmartMoneyLiveLeaderboardCaches(ctx, snapshotDay); cacheErr != nil {
			log.Printf("[Assets] smart money live leaderboard cache refresh failed day=%s err=%v", formatDay(snapshotDay), cacheErr)
		}
		if result.refreshed > 0 || result.failed > 0 {
			log.Printf("[Assets] smart money live-state refresh completed refreshed=%d skipped=%d failed=%d", result.refreshed, result.skipped, result.failed)
		}
	}()
	return true
}

func (s *Service) tryRefreshSmartMoneyLiveStates(lastCompleted *time.Time) {
	if s == nil {
		return
	}
	now := timeutil.Now()
	if lastCompleted != nil && !lastCompleted.IsZero() && now.Sub(*lastCompleted) < smartMoneyWalletLiveRefreshInterval {
		return
	}
	if !s.smartMoneyLiveRefreshMu.TryLock() {
		return
	}
	defer s.smartMoneyLiveRefreshMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), smartMoneyWalletLiveRefreshTimeout)
	defer cancel()
	result, err := s.refreshSmartMoneyWalletLiveStates(ctx)
	if err != nil {
		log.Printf("[Assets] smart money live-state refresh incomplete refreshed=%d skipped=%d failed=%d err=%v", result.refreshed, result.skipped, result.failed, err)
		return
	}
	if lastCompleted != nil {
		*lastCompleted = timeutil.Now()
	}
	snapshotDay := dayStart(timeutil.Now()).AddDate(0, 0, -1)
	s.deleteCachedSmartMoneyLiveLeaderboards(snapshotDay)
	if cacheErr := s.refreshSmartMoneyLiveLeaderboardCaches(ctx, snapshotDay); cacheErr != nil {
		log.Printf("[Assets] smart money live leaderboard cache refresh failed day=%s err=%v", formatDay(snapshotDay), cacheErr)
	}
	if result.refreshed > 0 || result.failed > 0 {
		log.Printf("[Assets] smart money live-state refresh completed refreshed=%d skipped=%d failed=%d", result.refreshed, result.skipped, result.failed)
	}
}

func (s *Service) refreshSmartMoneyWalletLiveStates(ctx context.Context) (smartMoneyLiveRefreshResult, error) {
	var result smartMoneyLiveRefreshResult
	wallets, err := s.loadActiveSmartMoneyWallets(ctx)
	if err != nil {
		return result, err
	}
	if len(wallets) == 0 {
		return result, nil
	}

	workers := smartMoneyWalletLiveRefreshWorkers
	if workers <= 0 {
		workers = 1
	}
	jobs := make(chan models.MonitoredWallet)
	var wg sync.WaitGroup
	var mu sync.Mutex
	addResult := func(refreshed, skipped, failed int) {
		mu.Lock()
		result.refreshed += refreshed
		result.skipped += skipped
		result.failed += failed
		mu.Unlock()
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for walletRow := range jobs {
				if ctx.Err() != nil {
					addResult(0, 0, 1)
					continue
				}
				fresh, err := s.hasFreshSmartMoneyWalletLiveState(ctx, walletRow, smartMoneyWalletLiveCacheTTL)
				if err == nil && fresh {
					addResult(0, 1, 0)
					continue
				}
				state, err := s.loadSmartMoneyWalletLiveStateLive(ctx, walletRow)
				if err != nil {
					s.recordSmartMoneyWalletLiveStateError(ctx, walletRow, err)
					addResult(0, 0, 1)
					continue
				}
				if err := s.persistSmartMoneyWalletLiveState(ctx, walletRow, state, timeutil.Now()); err != nil {
					log.Printf("[Assets] persist smart money live state failed wallet=%s chain=%d err=%v", walletRow.Address, walletRow.ChainID, err)
					addResult(0, 0, 1)
					continue
				}
				writeCachedSmartMoneyWalletLiveState(walletRow.ChainID, walletRow.Address, state)
				addResult(1, 0, 0)
			}
		}()
	}

sendLoop:
	for _, walletRow := range wallets {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobs <- walletRow:
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	return result, nil
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
		return "按剔除净转账后的资产变化收益率排序"
	case "participation":
		return "按窗口内 add/remove 参与次数排序"
	default:
		return "按剔除净转账后的资产变化收益额排序"
	}
}

func liveLeaderboardDescription(metric string) string {
	switch metric {
	case "yield_rate":
		return "按半小时实时资产相对今日0点基准的收益率排序"
	case "participation":
		return "按今日 add/remove 参与次数排序，并展示实时资产相对今日0点基准的变化"
	default:
		return "按半小时实时资产相对今日0点基准的变化排序"
	}
}

func smartMoneyWalletKey(chainID int, address string) string {
	return strconv.Itoa(chainID) + "|" + normalizeAddress(address)
}

func buildSmartMoneySnapshotLeaderboard(metric string, snapshotDay time.Time, comparedDay time.Time, days int, limit int, inputs []smartMoneyLeaderboardSnapshotInput) *SmartMoneyLeaderboardResponse {
	days = clampLPDays(days)
	list := make([]SmartMoneyLeaderboardEntry, 0, len(inputs))
	for _, input := range inputs {
		if input.Current == nil || (input.Previous == nil && input.DailyStat == nil) {
			continue
		}
		estimatedPnL := 0.0
		yieldRate := 0.0
		if input.Previous != nil {
			if input.UseRawAssetDelta {
				estimatedPnL = round2(input.Current.TotalUSD - input.Previous.TotalUSD)
			} else {
				estimatedPnL = adjustedPnL(input.Current.TotalUSD-input.Previous.TotalUSD, input.Current.TransferInUSD, input.Current.TransferOutUSD)
			}
			if input.Previous.TotalUSD > 0 {
				yieldRate = round4(estimatedPnL / input.Previous.TotalUSD)
			}
		}
		if input.DailyStat != nil && !input.IgnoreDailyStatPnL {
			estimatedPnL = round2(input.DailyStat.EstimatedRealizedPnLUSD)
			if input.DailyStat.MatchedCostUSD > 0 {
				yieldRate = round4(input.DailyStat.EstimatedRealizedPnLUSD / input.DailyStat.MatchedCostUSD)
			} else {
				yieldRate = 0
			}
		}
		entry := SmartMoneyLeaderboardEntry{
			Address:                 input.Wallet.Address,
			Source:                  strings.TrimSpace(input.Wallet.Source),
			SourceContract:          smartMoneySourceContractValue(input.Wallet),
			ChainID:                 input.Wallet.ChainID,
			EstimatedRealizedPnLUSD: estimatedPnL,
			YieldRate:               yieldRate,
			HasTransferIn:           input.Current.HasTransferIn,
			HasTransferOut:          input.Current.HasTransferOut,
			TransferInCount:         input.Current.TransferInCount,
			TransferOutCount:        input.Current.TransferOutCount,
			TransferTotalCount:      transferTotalCount(input.Current.TransferInCount, input.Current.TransferOutCount),
			TransferInUSD:           round2(input.Current.TransferInUSD),
			TransferOutUSD:          round2(input.Current.TransferOutUSD),
			TransferNetUSD:          transferNetUSD(input.Current.TransferInUSD, input.Current.TransferOutUSD),
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
		Days:        days,
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

func (s *Service) loadPagedSmartMoneyWallets(ctx context.Context, page int, size int, keyword string) ([]models.MonitoredWallet, int64, error) {
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
		Joins("LEFT JOIN sm_wallet_live_states sml ON sml.wallet_address = monitored_wallets.address AND sml.chain_id = monitored_wallets.chain_id").
		Order("sml.total_usd IS NULL ASC").
		Order("sml.total_usd DESC").
		Order("monitored_wallets.chain_id ASC").
		Order("monitored_wallets.address ASC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&wallets).Error
	return wallets, total, err
}

func (s *Service) loadSmartMoneyWalletLiveStateCached(ctx context.Context, walletRow models.MonitoredWallet, forceRefresh bool) (smartMoneyWalletLiveState, error) {
	var state smartMoneyWalletLiveState
	if !forceRefresh {
		if cached, ok := readCachedSmartMoneyWalletLiveState(walletRow.ChainID, walletRow.Address); ok {
			return cached, nil
		}
		if stored, refreshedAt, ok, err := s.loadStoredSmartMoneyWalletLiveState(ctx, walletRow); err != nil {
			log.Printf("[Assets] read smart money live state failed wallet=%s chain=%d err=%v", walletRow.Address, walletRow.ChainID, err)
		} else if ok && !refreshedAt.Before(timeutil.Now().Add(-smartMoneyWalletLiveCacheTTL)) {
			writeCachedSmartMoneyWalletLiveState(walletRow.ChainID, walletRow.Address, stored)
			return stored, nil
		}
	}

	state, err := s.loadSmartMoneyWalletLiveStateLive(ctx, walletRow)
	if err != nil {
		return state, err
	}
	if err := s.persistSmartMoneyWalletLiveState(ctx, walletRow, state, timeutil.Now()); err != nil {
		log.Printf("[Assets] persist smart money live state failed wallet=%s chain=%d err=%v", walletRow.Address, walletRow.ChainID, err)
	}
	writeCachedSmartMoneyWalletLiveState(walletRow.ChainID, walletRow.Address, state)
	return state, nil
}

func (s *Service) loadSmartMoneyWalletLiveStateForOverview(ctx context.Context, walletRow models.MonitoredWallet) (smartMoneyWalletLiveState, bool, error) {
	var state smartMoneyWalletLiveState
	if cached, ok := readCachedSmartMoneyWalletLiveState(walletRow.ChainID, walletRow.Address); ok {
		return cached, true, nil
	}
	stored, _, ok, err := s.loadStoredSmartMoneyWalletLiveState(ctx, walletRow)
	if err != nil {
		return state, false, err
	}
	return stored, ok, nil
}

func (s *Service) loadStoredSmartMoneyWalletLiveState(ctx context.Context, walletRow models.MonitoredWallet) (smartMoneyWalletLiveState, time.Time, bool, error) {
	var state smartMoneyWalletLiveState
	if database.DB == nil {
		return state, time.Time{}, false, fmt.Errorf("database not initialized")
	}
	address := normalizeAddress(walletRow.Address)
	if address == "" {
		return state, time.Time{}, false, nil
	}

	var row models.SmartMoneyWalletLiveState
	err := database.DB.WithContext(ctx).
		Where("wallet_address = ? AND chain_id = ?", address, walletRow.ChainID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return state, time.Time{}, false, nil
		}
		return state, time.Time{}, false, err
	}
	return smartMoneyWalletLiveStateFromModel(row), row.RefreshedAt, true, nil
}

func (s *Service) hasFreshSmartMoneyWalletLiveState(ctx context.Context, walletRow models.MonitoredWallet, maxAge time.Duration) (bool, error) {
	if database.DB == nil {
		return false, fmt.Errorf("database not initialized")
	}
	address := normalizeAddress(walletRow.Address)
	if address == "" {
		return true, nil
	}
	var count int64
	err := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyWalletLiveState{}).
		Where("wallet_address = ? AND chain_id = ? AND refreshed_at >= ?", address, walletRow.ChainID, timeutil.Now().Add(-maxAge)).
		Count(&count).Error
	return count > 0, err
}

func (s *Service) persistSmartMoneyWalletLiveState(ctx context.Context, walletRow models.MonitoredWallet, state smartMoneyWalletLiveState, refreshedAt time.Time) error {
	address := normalizeAddress(walletRow.Address)
	if address == "" {
		return nil
	}
	row := &models.SmartMoneyWalletLiveState{
		WalletAddress:     address,
		ChainID:           walletRow.ChainID,
		NativeUSD:         round2(state.assets.NativeUSD),
		StableUSD:         round2(state.assets.StableUSD),
		TrackedTokenUSD:   round2(state.assets.TrackedTokenUSD),
		OpenLPUSD:         round2(state.assets.OpenLPUSD),
		TotalUSD:          round2(state.assets.TotalUSD),
		TrackedTokenCount: state.assets.TrackedTokenCount,
		ActivePoolCount:   state.activePoolCount,
		TodayEventCount:   state.todayEventCount,
		LastActiveAt:      state.lastActiveAt,
		RefreshedAt:       refreshedAt,
		ErrorMessage:      "",
	}
	return upsertByColumns(ctx, row,
		[]string{"wallet_address", "chain_id"},
		map[string]interface{}{
			"native_usd":          row.NativeUSD,
			"stable_usd":          row.StableUSD,
			"tracked_token_usd":   row.TrackedTokenUSD,
			"open_lp_usd":         row.OpenLPUSD,
			"total_usd":           row.TotalUSD,
			"tracked_token_count": row.TrackedTokenCount,
			"active_pool_count":   row.ActivePoolCount,
			"today_event_count":   row.TodayEventCount,
			"last_active_at":      row.LastActiveAt,
			"refreshed_at":        row.RefreshedAt,
			"error_message":       "",
			"updated_at":          refreshedAt,
		})
}

func (s *Service) recordSmartMoneyWalletLiveStateError(ctx context.Context, walletRow models.MonitoredWallet, refreshErr error) {
	if database.DB == nil || refreshErr == nil {
		return
	}
	address := normalizeAddress(walletRow.Address)
	if address == "" {
		return
	}
	msg := strings.TrimSpace(refreshErr.Error())
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	if msg == "" {
		return
	}
	if err := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyWalletLiveState{}).
		Where("wallet_address = ? AND chain_id = ?", address, walletRow.ChainID).
		Updates(map[string]interface{}{
			"error_message": msg,
			"updated_at":    timeutil.Now(),
		}).Error; err != nil {
		log.Printf("[Assets] record smart money live state error failed wallet=%s chain=%d err=%v", walletRow.Address, walletRow.ChainID, err)
	}
}

func (s *Service) loadSmartMoneyWalletLiveStateLive(ctx context.Context, walletRow models.MonitoredWallet) (smartMoneyWalletLiveState, error) {
	var state smartMoneyWalletLiveState
	address := normalizeAddress(walletRow.Address)
	if address == "" {
		return state, fmt.Errorf("invalid wallet address")
	}
	chain := smartMoneyChainFromID(walletRow.ChainID)
	cc, endpoints, err := s.smartMoneyAssetReadEndpoints(ctx, chain)
	if err != nil {
		return state, err
	}
	if len(endpoints) == 0 {
		return state, fmt.Errorf("smart money asset rpc clients unavailable: chain=%s", chain)
	}

	walletAddr := common.HexToAddress(address)
	nativePrice := s.nativePriceUSD(chain, cc)
	if nativeBalance, err := readSmartMoneyNativeBalanceFromPool(ctx, endpoints, walletAddr); err == nil && nativeBalance != nil {
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
	tokenBalances := s.loadSmartMoneyTokenBalances(ctx, chain, cc, endpoints, walletAddr, tokenDescriptors, prices)
	state.assets.StableUSD = tokenBalances.stableUSD
	state.assets.TrackedTokenUSD = tokenBalances.trackedTokenUSD
	state.assets.TrackedTokenCount = tokenBalances.trackedTokenCount
	state.warnings = append(state.warnings, tokenBalances.warnings...)

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

type smartMoneyTokenBalanceTotals struct {
	stableUSD         float64
	trackedTokenUSD   float64
	trackedTokenCount int
	warnings          []string
}

func (s *Service) loadSmartMoneyTokenBalances(ctx context.Context, chain string, cc config.ChainConfig, endpoints []smartMoneyAssetReadEndpoint, walletAddr common.Address, tokens []tokenDescriptor, prices map[string]float64) smartMoneyTokenBalanceTotals {
	var totals smartMoneyTokenBalanceTotals
	if len(tokens) == 0 {
		return totals
	}

	workers := smartMoneyWalletTokenReadWorkers
	if workers <= 0 {
		workers = 1
	}
	if workers > len(tokens) {
		workers = len(tokens)
	}

	type tokenJob struct {
		index int
		token tokenDescriptor
	}
	type tokenResult struct {
		token    tokenDescriptor
		usd      float64
		hasValue bool
		warning  string
	}

	jobs := make(chan tokenJob)
	results := make(chan tokenResult, len(tokens))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				addr := normalizeAddress(job.token.Address)
				if addr == "" {
					continue
				}
				tokenAddr := common.HexToAddress(addr)
				decimals := s.getSmartMoneyTokenDecimals(ctx, chain, cc, endpoints, job.index+workerID, addr)
				balance, err := readSmartMoneyTokenBalanceFromPool(ctx, endpoints, job.index+workerID, tokenAddr, walletAddr)
				if err != nil {
					results <- tokenResult{token: job.token, warning: fmt.Sprintf("token balance unavailable %s: %v", addr, err)}
					continue
				}
				if balance == nil || balance.Sign() <= 0 {
					results <- tokenResult{token: job.token}
					continue
				}
				results <- tokenResult{
					token:    job.token,
					usd:      balanceToUSD(amountToFloat(balance.String(), decimals), prices[addr]),
					hasValue: true,
				}
			}
		}(i)
	}

	go func() {
		defer close(jobs)
		for i, token := range tokens {
			select {
			case <-ctx.Done():
				return
			case jobs <- tokenJob{index: i, token: token}:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		if result.warning != "" {
			totals.warnings = append(totals.warnings, result.warning)
			continue
		}
		if !result.hasValue {
			continue
		}
		if result.token.Stable {
			totals.stableUSD += result.usd
		} else {
			totals.trackedTokenUSD += result.usd
			totals.trackedTokenCount++
		}
	}
	totals.stableUSD = round2(totals.stableUSD)
	totals.trackedTokenUSD = round2(totals.trackedTokenUSD)
	totals.warnings = dedupeStrings(totals.warnings)
	if ctx.Err() != nil {
		totals.warnings = append(totals.warnings, fmt.Sprintf("token balance scan incomplete: %v", ctx.Err()))
	}
	return totals
}

func (s *Service) getSmartMoneyTokenDecimals(ctx context.Context, chain string, cc config.ChainConfig, endpoints []smartMoneyAssetReadEndpoint, endpointStart int, tokenAddress string) int {
	addr := normalizeAddress(tokenAddress)
	if addr == "" {
		return 18
	}
	key := config.NormalizeChain(chain) + "|" + addr

	s.decimalsMu.RLock()
	if v, ok := s.decimalsCache[key]; ok && v > 0 {
		s.decimalsMu.RUnlock()
		return v
	}
	s.decimalsMu.RUnlock()

	decimals := s.tokenFallbackDecimals(cc, addr)
	if decimals <= 0 {
		decimals = 18
	}
	if len(endpoints) > 0 {
		if v, err := readSmartMoneyTokenDecimalsFromPool(ctx, endpoints, endpointStart, common.HexToAddress(addr)); err == nil && v > 0 {
			decimals = int(v)
		}
	}

	s.decimalsMu.Lock()
	s.decimalsCache[key] = decimals
	s.decimalsMu.Unlock()
	return decimals
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
				UNION
				SELECT token_address
				FROM sm_wallet_transfer_events
				WHERE wallet_address = ? AND chain_id = ? AND tx_timestamp >= ? AND asset_type = 'token'
			) tokens
		`, address, chainID, cutoff, address, chainID, cutoff, address, chainID, address, chainID, address, chainID, cutoff).
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
				ON ap.chain_id = p.chain_id AND ap.protocol = p.protocol AND ap.nft_token_id = p.nft_token_id
			LEFT JOIN (
				SELECT
					chain_id,
					protocol,
					nft_token_id,
					SUM(
						CASE
							WHEN event_type = 'add' THEN COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
							WHEN event_type = 'remove' THEN -COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)
							ELSE 0
						END
					) AS net_amount_usd
				FROM sm_lp_events
				WHERE wallet_address = ?
				  AND chain_id = ?
				  AND event_type IN ('add', 'remove')
				GROUP BY chain_id, protocol, nft_token_id
			) evt_net
				ON evt_net.chain_id = p.chain_id
				AND evt_net.protocol = p.protocol
				AND evt_net.nft_token_id = p.nft_token_id
			WHERE p.wallet_address = ?
			  AND p.chain_id = ?
			  AND p.status = 'open'
	`, address, chainID, address, chainID).
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

	var rows []smartMoneyHistoryDayRow
	openLPSelect := "COALESCE(SUM(s.open_lp_usd), 0) AS open_lp_usd"
	if database.DB != nil && !database.DB.Migrator().HasColumn(&models.SmartMoneyWalletDailySnapshot{}, "OpenLPUSD") {
		openLPSelect = "0 AS open_lp_usd"
	}
	err := database.DB.WithContext(ctx).
		Raw(fmt.Sprintf(`
			SELECT
				s.snapshot_day AS day,
				COALESCE(SUM(s.native_usd), 0) AS native_usd,
				COALESCE(SUM(s.stable_usd), 0) AS stable_usd,
				COALESCE(SUM(s.tracked_token_usd), 0) AS tracked_token_usd,
				%s,
				COALESCE(SUM(s.total_usd), 0) AS total_usd,
				MAX(CASE WHEN s.has_transfer_in THEN 1 ELSE 0 END) AS has_transfer_in,
				MAX(CASE WHEN s.has_transfer_out THEN 1 ELSE 0 END) AS has_transfer_out,
				COALESCE(SUM(s.transfer_in_count), 0) AS transfer_in_count,
				COALESCE(SUM(s.transfer_out_count), 0) AS transfer_out_count,
				COALESCE(SUM(s.transfer_in_usd), 0) AS transfer_in_usd,
				COALESCE(SUM(s.transfer_out_usd), 0) AS transfer_out_usd,
				COUNT(s.wallet_address) AS snapshot_count,
				COUNT(st.stat_day) AS daily_stat_count,
				COALESCE(SUM(st.estimated_realized_pnl_usd), 0) AS estimated_realized_pnl_usd
			FROM sm_wallet_daily_snapshots s
			LEFT JOIN sm_lp_daily_stats st
				ON st.wallet_address = s.wallet_address
				AND st.chain_id = s.chain_id
				AND st.stat_day = s.snapshot_day
			WHERE s.chain_id IN ?
			  AND s.wallet_address IN ?
			  AND s.snapshot_day >= ?
			  AND s.snapshot_day < ?
			GROUP BY s.snapshot_day
			ORDER BY s.snapshot_day ASC
		`, openLPSelect), chainIDs, addresses, formatDay(start), formatDay(end)).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return buildSmartMoneyHistoryPoints(rows), nil
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

func (s *Service) loadSmartMoneyTransferActivity(ctx context.Context, wallets []models.MonitoredWallet, start time.Time, end time.Time) (map[string]smartMoneyTransferActivity, error) {
	out := make(map[string]smartMoneyTransferActivity, len(wallets))
	if len(wallets) == 0 || !start.Before(end) {
		return out, nil
	}
	for _, wallet := range wallets {
		out[smartMoneyWalletKey(wallet.ChainID, wallet.Address)] = smartMoneyTransferActivity{}
	}

	rows, err := s.smRepo.AggregateWalletTransferActivity(ctx, wallets, start, end)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		key := smartMoneyWalletKey(row.ChainID, row.WalletAddress)
		activity := out[key]
		activity.HasTransferIn = row.HasTransferIn > 0
		activity.HasTransferOut = row.HasTransferOut > 0
		activity.TransferInCount = row.TransferInCount
		activity.TransferOutCount = row.TransferOutCount
		activity.TransferInUSD = round2(row.TransferInUSD)
		activity.TransferOutUSD = round2(row.TransferOutUSD)
		out[key] = activity
	}
	return out, nil
}

func (s *Service) captureSmartMoneyWalletSnapshots(ctx context.Context, day time.Time) error {
	wallets, err := s.loadActiveSmartMoneyWallets(ctx)
	if err != nil {
		return err
	}
	dayKey := formatDay(day)
	transferActivity, err := s.loadSmartMoneyTransferActivity(ctx, wallets, dayStart(day), dayEnd(day))
	if err != nil {
		return err
	}
	if err := database.DB.WithContext(ctx).
		Where("snapshot_day = ?", dayKey).
		Delete(&models.SmartMoneyWalletDailySnapshot{}).Error; err != nil {
		return err
	}
	for _, walletRow := range wallets {
		// Daily snapshots intentionally bypass the short-lived Redis cache used by
		// interactive smart-money wallet balance reads.
		live, err := s.loadSmartMoneyWalletLiveStateLive(ctx, walletRow)
		if err != nil {
			log.Printf("[Assets] skip smart money snapshot wallet=%s chain=%d err=%v", walletRow.Address, walletRow.ChainID, err)
			continue
		}
		if err := s.persistSmartMoneyWalletLiveState(ctx, walletRow, live, timeutil.Now()); err != nil {
			log.Printf("[Assets] persist smart money live state from snapshot failed wallet=%s chain=%d err=%v", walletRow.Address, walletRow.ChainID, err)
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
