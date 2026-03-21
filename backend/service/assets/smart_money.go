package assets

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

const recognizedAssetBasis = "原生币 + 稳定币 + 近30天参与LP的代币余额 + 当前open LP估算持仓"

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

func (s *Service) GetSmartMoneyOverview(ctx context.Context, days int) (*SmartMoneyOverview, error) {
	days = clampLPDays(days)
	wallets, err := s.loadActiveSmartMoneyWallets(ctx)
	if err != nil {
		return nil, err
	}

	resp := &SmartMoneyOverview{
		Wallets:   make([]SmartMoneyWalletSummary, 0, len(wallets)),
		UpdatedAt: timeutil.Now(),
		Timezone:  timeutil.LocationName(),
	}
	for _, walletRow := range wallets {
		live, err := s.loadSmartMoneyWalletLiveState(ctx, walletRow)
		if err != nil {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("wallet %s unavailable: %v", walletRow.Address, err))
			continue
		}
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
		resp.Wallets = append(resp.Wallets, summary)
		resp.Summary.NativeUSD += live.assets.NativeUSD
		resp.Summary.StableUSD += live.assets.StableUSD
		resp.Summary.TrackedTokenUSD += live.assets.TrackedTokenUSD
		resp.Summary.OpenLPUSD += live.assets.OpenLPUSD
		resp.Summary.TotalUSD += live.assets.TotalUSD
		resp.Summary.TrackedTokenCount += live.assets.TrackedTokenCount
		resp.Warnings = append(resp.Warnings, live.warnings...)
	}
	resp.Summary.NativeUSD = round2(resp.Summary.NativeUSD)
	resp.Summary.StableUSD = round2(resp.Summary.StableUSD)
	resp.Summary.TrackedTokenUSD = round2(resp.Summary.TrackedTokenUSD)
	resp.Summary.OpenLPUSD = round2(resp.Summary.OpenLPUSD)
	resp.Summary.TotalUSD = round2(resp.Summary.TotalUSD)

	start := dayStart(timeutil.Now()).AddDate(0, 0, -defaultHistoryDays)
	end := dayStart(timeutil.Now())
	history, err := s.loadSmartMoneyHistory(ctx, wallets, start, end)
	if err != nil {
		return nil, err
	}
	resp.History = history
	resp.Today = SmartMoneyHistoryPoint{
		Day:             formatDay(timeutil.Now()),
		NativeUSD:       resp.Summary.NativeUSD,
		StableUSD:       resp.Summary.StableUSD,
		TrackedTokenUSD: resp.Summary.TrackedTokenUSD,
		OpenLPUSD:       resp.Summary.OpenLPUSD,
		TotalUSD:        resp.Summary.TotalUSD,
	}

	windowStart := dayStart(timeutil.Now()).AddDate(0, 0, -days)
	statsByWallet, err := s.computeSmartMoneyStats(ctx, wallets, windowStart, timeutil.Now())
	if err != nil {
		return nil, err
	}
	resp.Windows = []SmartMoneyWindowStats{aggregateSmartMoneyWindowStats(days, statsByWallet)}
	resp.Warnings = dedupeStrings(resp.Warnings)
	sort.Slice(resp.Wallets, func(i, j int) bool {
		if resp.Wallets[i].Assets.TotalUSD != resp.Wallets[j].Assets.TotalUSD {
			return resp.Wallets[i].Assets.TotalUSD > resp.Wallets[j].Assets.TotalUSD
		}
		return strings.ToLower(resp.Wallets[i].Address) < strings.ToLower(resp.Wallets[j].Address)
	})
	return resp, nil
}

func (s *Service) GetSmartMoneyWallet(ctx context.Context, address string, chainID int, days int) (*SmartMoneyWalletResponse, error) {
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

	live, err := s.loadSmartMoneyWalletLiveState(ctx, *walletRow)
	if err != nil {
		return nil, err
	}
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

func (s *Service) GetSmartMoneyLeaderboard(ctx context.Context, metric string, days int, limit int) (*SmartMoneyLeaderboardResponse, error) {
	wallets, err := s.loadActiveSmartMoneyWallets(ctx)
	if err != nil {
		return nil, err
	}
	days = clampLPDays(days)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	metric = normalizeLeaderboardMetric(metric)

	start := dayStart(timeutil.Now()).AddDate(0, 0, -days)
	end := timeutil.Now()
	statsByWallet, err := s.computeSmartMoneyStats(ctx, wallets, start, end)
	if err != nil {
		return nil, err
	}

	type candidate struct {
		SmartMoneyLeaderboardEntry
	}
	list := make([]candidate, 0, len(wallets))
	for _, walletRow := range wallets {
		key := smartMoneyWalletKey(walletRow.ChainID, walletRow.Address)
		stats := statsByWallet[key]
		entry := SmartMoneyLeaderboardEntry{
			Address:                 walletRow.Address,
			ChainID:                 walletRow.ChainID,
			EstimatedRealizedPnLUSD: round2(stats.EstimatedRealizedPnLUSD),
			ParticipationCount:      stats.AddCount + stats.RemoveCount,
			ActivePoolCount:         len(stats.activePools),
			UnmatchedRemoveCount:    stats.UnmatchedRemoveCount,
		}
		if walletRow.Label != nil {
			entry.Label = strings.TrimSpace(*walletRow.Label)
		}
		if stats.MatchedCostUSD > 0 {
			entry.YieldRate = round4(stats.EstimatedRealizedPnLUSD / stats.MatchedCostUSD)
		}
		switch metric {
		case "yield_rate":
			entry.MetricValue = entry.YieldRate
		case "participation":
			entry.MetricValue = float64(entry.ParticipationCount)
		default:
			entry.MetricValue = entry.EstimatedRealizedPnLUSD
		}
		list = append(list, candidate{SmartMoneyLeaderboardEntry: entry})
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
		StartDay:    formatDay(start),
		EndDay:      formatDay(end.Add(-time.Second)),
		Timezone:    timeutil.LocationName(),
		Description: leaderboardDescription(metric),
		List:        make([]SmartMoneyLeaderboardEntry, 0, limit),
	}
	for i := 0; i < len(list) && i < limit; i++ {
		list[i].Rank = i + 1
		resp.List = append(resp.List, list[i].SmartMoneyLeaderboardEntry)
	}
	return resp, nil
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

func (s *Service) loadActiveSmartMoneyWallets(ctx context.Context) ([]models.MonitoredWallet, error) {
	var wallets []models.MonitoredWallet
	err := database.DB.WithContext(ctx).
		Where("is_active = ?", true).
		Order("chain_id ASC, address ASC").
		Find(&wallets).Error
	return wallets, err
}

func (s *Service) loadSmartMoneyWalletLiveState(ctx context.Context, walletRow models.MonitoredWallet) (smartMoneyWalletLiveState, error) {
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
				COALESCE(SUM(COALESCE(e_agg.position_amount_usd, 0)), 0) AS open_lp_usd,
				COUNT(DISTINCT p.pool_address) AS active_pool_count
			FROM sm_lp_positions p
			LEFT JOIN (
				SELECT
					chain_id,
					nft_token_id,
					MAX(COALESCE(total_usd, COALESCE(token0_amount_usd, 0) + COALESCE(token1_amount_usd, 0), 0)) AS position_amount_usd
				FROM sm_lp_events
				WHERE event_type = 'add'
				GROUP BY chain_id, nft_token_id
			) e_agg ON e_agg.chain_id = p.chain_id AND e_agg.nft_token_id = p.nft_token_id
			WHERE p.wallet_address = ?
			  AND p.chain_id = ?
			  AND p.status = 'open'
		`, address, chainID).
		Scan(&result).Error
	return round2(result.OpenLPUSD), result.ActivePoolCount, err
}

func (s *Service) loadSmartMoneyHistory(ctx context.Context, wallets []models.MonitoredWallet, start time.Time, end time.Time) ([]SmartMoneyHistoryPoint, error) {
	if len(wallets) == 0 {
		return nil, nil
	}
	walletKeys := make([]string, 0, len(wallets))
	chainIDs := make([]int, 0, len(wallets))
	addresses := make([]string, 0, len(wallets))
	chainSeen := make(map[int]struct{})
	addrSeen := make(map[string]struct{})
	for _, wallet := range wallets {
		walletKeys = append(walletKeys, smartMoneyWalletKey(wallet.ChainID, wallet.Address))
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
		Day             string
		NativeUSD       float64
		StableUSD       float64
		TrackedTokenUSD float64
		OpenLPUSD       float64
		TotalUSD        float64
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
				COALESCE(SUM(total_usd), 0) AS total_usd
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
	for _, row := range rows {
		points = append(points, SmartMoneyHistoryPoint{
			Day:             row.Day,
			NativeUSD:       round2(row.NativeUSD),
			StableUSD:       round2(row.StableUSD),
			TrackedTokenUSD: round2(row.TrackedTokenUSD),
			OpenLPUSD:       round2(row.OpenLPUSD),
			TotalUSD:        round2(row.TotalUSD),
		})
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

func (s *Service) captureSmartMoneyWalletSnapshots(ctx context.Context, day time.Time) error {
	wallets, err := s.loadActiveSmartMoneyWallets(ctx)
	if err != nil {
		return err
	}
	dayKey := formatDay(day)
	if err := database.DB.WithContext(ctx).
		Where("snapshot_day = ?", dayKey).
		Delete(&models.SmartMoneyWalletDailySnapshot{}).Error; err != nil {
		return err
	}
	for _, walletRow := range wallets {
		live, err := s.loadSmartMoneyWalletLiveState(ctx, walletRow)
		if err != nil {
			log.Printf("[Assets] skip smart money snapshot wallet=%s chain=%d err=%v", walletRow.Address, walletRow.ChainID, err)
			continue
		}
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
