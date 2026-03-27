package assets

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"context"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func activeStrategyStatuses() []models.StrategyStatus {
	return []models.StrategyStatus{
		models.StrategyStatusOpening,
		models.StrategyStatusRunning,
		models.StrategyStatusWaiting,
		models.StrategyStatusStopping,
	}
}

type userLPTradeRow struct {
	ID           uint
	UserID       uint
	PoolID       string
	Token0Symbol string
	Token1Symbol string
	Chain        string
	ProfitUSDT   string
	ClosedAt     *time.Time
}

type userLPBucket struct {
	ProfitWei      *big.Int
	ClosedCount    int
	WinCount       int
	LossCount      int
	BreakEvenCount int
}

type userLPPoolKey struct {
	PoolID       string
	Token0Symbol string
	Token1Symbol string
	Chain        string
}

type userLPPoolBucket struct {
	Key         userLPPoolKey
	ProfitWei   *big.Int
	ClosedCount int
}

func (b *userLPBucket) addProfit(profitWei *big.Int) {
	if b == nil {
		return
	}
	if b.ProfitWei == nil {
		b.ProfitWei = big.NewInt(0)
	}
	if profitWei == nil {
		profitWei = big.NewInt(0)
	}
	b.ProfitWei.Add(b.ProfitWei, profitWei)
	b.ClosedCount++
	switch profitWei.Sign() {
	case 1:
		b.WinCount++
	case -1:
		b.LossCount++
	default:
		b.BreakEvenCount++
	}
}

func (b *userLPBucket) toWindowStats(days int) UserLPWindowStats {
	stats := UserLPWindowStats{
		Days:           days,
		RealizedPnLUSD: profitWeiToUSD(b.ProfitWei),
		ClosedCount:    b.ClosedCount,
		WinCount:       b.WinCount,
		LossCount:      b.LossCount,
		BreakEvenCount: b.BreakEvenCount,
	}
	if stats.ClosedCount > 0 {
		stats.WinRate = round4(float64(stats.WinCount) / float64(stats.ClosedCount))
		stats.AvgPnLUSD = round2(stats.RealizedPnLUSD / float64(stats.ClosedCount))
	}
	return stats
}

func (b *userLPPoolBucket) addProfit(profitWei *big.Int) {
	if b == nil {
		return
	}
	if b.ProfitWei == nil {
		b.ProfitWei = big.NewInt(0)
	}
	if profitWei == nil {
		profitWei = big.NewInt(0)
	}
	b.ProfitWei.Add(b.ProfitWei, profitWei)
	b.ClosedCount++
}

func (b *userLPPoolBucket) toResponse() UserLPPoolPnL {
	return UserLPPoolPnL{
		PoolID:       b.Key.PoolID,
		Token0Symbol: b.Key.Token0Symbol,
		Token1Symbol: b.Key.Token1Symbol,
		Chain:        b.Key.Chain,
		ProfitUSD:    profitWeiToUSD(b.ProfitWei),
		ClosedCount:  b.ClosedCount,
	}
}

func profitWeiToUSD(profitWei *big.Int) float64 {
	if profitWei == nil || profitWei.Sign() == 0 {
		return 0
	}
	return round2(amountToFloat(profitWei.String(), 18))
}

func parseTradeProfitWei(record userLPTradeRow) *big.Int {
	value, err := convert.ParseBigInt(record.ProfitUSDT)
	if err != nil {
		log.Printf("[Assets] invalid trade profit record_id=%d user_id=%d raw=%q err=%v", record.ID, record.UserID, record.ProfitUSDT, err)
		return big.NewInt(0)
	}
	return value
}

func buildUserLPStatsFromTrades(trades []userLPTradeRow, now time.Time) UserLPStatsResponse {
	startOfToday := dayStart(now)
	windowDays := []int{1, 7, 30}
	windowStarts := map[int]time.Time{
		1:  startOfToday.AddDate(0, 0, -1),
		7:  startOfToday.AddDate(0, 0, -7),
		30: startOfToday.AddDate(0, 0, -30),
	}
	windowBuckets := map[int]*userLPBucket{
		1:  {},
		7:  {},
		30: {},
	}
	todayBucket := &userLPBucket{}
	todayPools := make(map[userLPPoolKey]*userLPPoolBucket)
	dailyBuckets := make(map[string]*userLPBucket)

	for _, trade := range trades {
		if trade.ClosedAt == nil || trade.ClosedAt.IsZero() {
			continue
		}
		closedAt := trade.ClosedAt.In(timeutil.Location())
		profitWei := parseTradeProfitWei(trade)

		if !closedAt.Before(startOfToday) {
			todayBucket.addProfit(profitWei)
			key := userLPPoolKey{
				PoolID:       trade.PoolID,
				Token0Symbol: trade.Token0Symbol,
				Token1Symbol: trade.Token1Symbol,
				Chain:        trade.Chain,
			}
			poolBucket := todayPools[key]
			if poolBucket == nil {
				poolBucket = &userLPPoolBucket{Key: key}
				todayPools[key] = poolBucket
			}
			poolBucket.addProfit(profitWei)
			continue
		}

		for _, days := range windowDays {
			if !closedAt.Before(windowStarts[days]) {
				windowBuckets[days].addProfit(profitWei)
			}
		}

		if !closedAt.Before(windowStarts[30]) {
			dayKey := formatDay(closedAt)
			dailyBucket := dailyBuckets[dayKey]
			if dailyBucket == nil {
				dailyBucket = &userLPBucket{}
				dailyBuckets[dayKey] = dailyBucket
			}
			dailyBucket.addProfit(profitWei)
		}
	}

	windows := make([]UserLPWindowStats, 0, len(windowDays))
	for _, days := range windowDays {
		windows = append(windows, windowBuckets[days].toWindowStats(days))
	}

	dailyKeys := make([]string, 0, len(dailyBuckets))
	for dayKey := range dailyBuckets {
		dailyKeys = append(dailyKeys, dayKey)
	}
	sort.Strings(dailyKeys)

	dailyHistory := make([]UserLPDailyPoint, 0, len(dailyKeys))
	for _, dayKey := range dailyKeys {
		bucket := dailyBuckets[dayKey]
		dailyHistory = append(dailyHistory, UserLPDailyPoint{
			Day:            dayKey,
			RealizedPnLUSD: profitWeiToUSD(bucket.ProfitWei),
			ClosedCount:    bucket.ClosedCount,
			WinCount:       bucket.WinCount,
			LossCount:      bucket.LossCount,
		})
	}

	pools := make([]UserLPPoolPnL, 0, len(todayPools))
	for _, bucket := range todayPools {
		pools = append(pools, bucket.toResponse())
	}
	sort.Slice(pools, func(i, j int) bool {
		if pools[i].ProfitUSD != pools[j].ProfitUSD {
			return pools[i].ProfitUSD > pools[j].ProfitUSD
		}
		if pools[i].ClosedCount != pools[j].ClosedCount {
			return pools[i].ClosedCount > pools[j].ClosedCount
		}
		if pools[i].PoolID != pools[j].PoolID {
			return pools[i].PoolID < pools[j].PoolID
		}
		if pools[i].Chain != pools[j].Chain {
			return pools[i].Chain < pools[j].Chain
		}
		if pools[i].Token0Symbol != pools[j].Token0Symbol {
			return pools[i].Token0Symbol < pools[j].Token0Symbol
		}
		return pools[i].Token1Symbol < pools[j].Token1Symbol
	})

	return UserLPStatsResponse{
		Windows:      windows,
		Today:        todayBucket.toWindowStats(0),
		TodayPools:   pools,
		DailyHistory: dailyHistory,
		Timezone:     timeutil.LocationName(),
	}
}

func parseSnapshotDay(dayKey string) (time.Time, bool) {
	dayKey = strings.TrimSpace(dayKey)
	if dayKey == "" {
		return time.Time{}, false
	}
	parsed, err := time.ParseInLocation("2006-01-02", dayKey, timeutil.Location())
	if err != nil {
		return time.Time{}, false
	}
	return dayStart(parsed), true
}

func isNextSnapshotDay(prevDayKey string, currDayKey string) bool {
	prev, ok := parseSnapshotDay(prevDayKey)
	if !ok {
		return false
	}
	curr, ok := parseSnapshotDay(currDayKey)
	if !ok {
		return false
	}
	return curr.Equal(prev.Add(24 * time.Hour))
}

func dayKeyInRange(dayKey string, start time.Time, end time.Time) bool {
	day, ok := parseSnapshotDay(dayKey)
	if !ok {
		return false
	}
	return !day.Before(dayStart(start)) && day.Before(dayStart(end))
}

func buildUserSnapshotPnLByDay(rows []models.UserAssetDailySnapshot) map[string]float64 {
	out := make(map[string]float64)
	if len(rows) < 2 {
		return out
	}
	for i := 1; i < len(rows); i++ {
		prev := rows[i-1]
		curr := rows[i]
		if !isNextSnapshotDay(prev.SnapshotDay, curr.SnapshotDay) {
			continue
		}
		out[curr.SnapshotDay] = round2(curr.TotalUSD - prev.TotalUSD)
	}
	return out
}

func mergeUserDailyHistoryPnL(history []UserLPDailyPoint, snapshotPnLByDay map[string]float64, start time.Time, end time.Time) []UserLPDailyPoint {
	merged := make(map[string]UserLPDailyPoint, len(history)+len(snapshotPnLByDay))
	for _, item := range history {
		if !dayKeyInRange(item.Day, start, end) {
			continue
		}
		merged[item.Day] = item
	}
	for dayKey, pnl := range snapshotPnLByDay {
		if !dayKeyInRange(dayKey, start, end) {
			continue
		}
		item := merged[dayKey]
		item.Day = dayKey
		item.RealizedPnLUSD = round2(pnl)
		merged[dayKey] = item
	}

	keys := make([]string, 0, len(merged))
	for dayKey := range merged {
		keys = append(keys, dayKey)
	}
	sort.Strings(keys)

	out := make([]UserLPDailyPoint, 0, len(keys))
	for _, dayKey := range keys {
		item := merged[dayKey]
		item.RealizedPnLUSD = round2(item.RealizedPnLUSD)
		out = append(out, item)
	}
	return out
}

func sumUserDailyHistoryPnL(history []UserLPDailyPoint, start time.Time, end time.Time) float64 {
	total := 0.0
	for _, item := range history {
		if !dayKeyInRange(item.Day, start, end) {
			continue
		}
		total += item.RealizedPnLUSD
	}
	return round2(total)
}

func snapshotTodayPnL(rows []models.UserAssetDailySnapshot, liveTotalUSD float64, now time.Time) (float64, bool) {
	yesterdayKey := formatDay(dayStart(now).AddDate(0, 0, -1))
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].SnapshotDay != yesterdayKey {
			continue
		}
		return round2(liveTotalUSD - rows[i].TotalUSD), true
	}
	return 0, false
}

func setUserLPWindowPnL(stats *UserLPWindowStats, pnl float64) {
	if stats == nil {
		return
	}
	stats.RealizedPnLUSD = round2(pnl)
	if stats.ClosedCount > 0 {
		stats.AvgPnLUSD = round2(stats.RealizedPnLUSD / float64(stats.ClosedCount))
	} else {
		stats.AvgPnLUSD = 0
	}
}

func applyUserSnapshotPnL(base UserLPStatsResponse, snapshotRows []models.UserAssetDailySnapshot, liveTotalUSD *float64, now time.Time) UserLPStatsResponse {
	startOfToday := dayStart(now)
	historyStart := startOfToday.AddDate(0, 0, -30)
	snapshotPnLByDay := buildUserSnapshotPnLByDay(snapshotRows)

	base.DailyHistory = mergeUserDailyHistoryPnL(base.DailyHistory, snapshotPnLByDay, historyStart, startOfToday)

	if liveTotalUSD != nil {
		if pnl, ok := snapshotTodayPnL(snapshotRows, *liveTotalUSD, now); ok {
			setUserLPWindowPnL(&base.Today, pnl)
		}
	}

	for i := range base.Windows {
		days := base.Windows[i].Days
		windowStart := startOfToday.AddDate(0, 0, -days)
		setUserLPWindowPnL(&base.Windows[i], sumUserDailyHistoryPnL(base.DailyHistory, windowStart, startOfToday))
	}

	return base
}

func (s *Service) loadUserLPTrades(ctx context.Context, userID uint, start, end time.Time) ([]userLPTradeRow, error) {
	query := database.DB.WithContext(ctx).
		Model(&models.TradeRecord{}).
		Select("id, user_id, pool_id, token0_symbol, token1_symbol, chain, profit_usdt, closed_at").
		Where("status = ? AND closed_at >= ? AND closed_at < ?", models.TradeStatusClosed, start, end)
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}

	var rows []userLPTradeRow
	if err := query.Order("closed_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) loadUserAssetSnapshotRows(ctx context.Context, userID uint, start, end time.Time) ([]models.UserAssetDailySnapshot, error) {
	var rows []models.UserAssetDailySnapshot
	err := database.DB.WithContext(ctx).
		Where("user_id = ? AND wallet_id = ? AND chain = ? AND snapshot_day >= ? AND snapshot_day < ?",
			userID, aggregateWalletID, "", formatDay(start), formatDay(end)).
		Order("snapshot_day ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) GetUserOverview(ctx context.Context, userID uint) (*UserAssetOverview, error) {
	if userID == 0 {
		return nil, fmt.Errorf("invalid user id")
	}
	wallets, err := s.walletService.GetUserWallets(userID)
	if err != nil {
		return nil, err
	}
	if len(wallets) == 0 {
		return nil, fmt.Errorf("wallet not found")
	}

	trackedTokens, err := s.loadUserTrackedTokens(ctx, userID)
	if err != nil {
		return nil, err
	}

	records := make([]userWalletAsset, 0, len(wallets)*len(trackedTokens))
	warnings := make([]string, 0, 4)
	seenChains := make(map[string]struct{})
	for _, walletRow := range wallets {
		addr := normalizeAddress(walletRow.Address)
		if addr == "" {
			continue
		}

		chains := make([]string, 0, len(trackedTokens))
		for chain := range trackedTokens {
			chains = append(chains, chain)
		}
		if len(chains) == 0 {
			chains = append(chains, config.PickEnabledChain("bsc"))
		}
		sort.Strings(chains)

		for _, chain := range chains {
			seenChains[chain] = struct{}{}
			record, warn := s.buildUserWalletAsset(ctx, walletRow, chain, trackedTokens[chain])
			if warn != "" {
				warnings = append(warnings, warn)
			}
			if record.TotalUSD <= 0 && record.NativeUSD <= 0 && record.StableUSD <= 0 && record.TokenUSD <= 0 {
				continue
			}
			records = append(records, record)
		}
	}

	summary := assetSummary{}
	for _, item := range records {
		summary.WalletUSD += item.TotalUSD
	}

	updatedAt := timeutil.Now()
	if realtimeResp, err := s.realtimeService.GetForUser(userID); err == nil && realtimeResp != nil {
		for _, position := range realtimeResp.Positions {
			summary.PositionUSD += round2(position.Totals.PositionUSD)
			summary.FeeUSD += round2(position.Totals.FeeUSD)
		}
		if !realtimeResp.UpdatedAt.IsZero() {
			updatedAt = realtimeResp.UpdatedAt
		}
		if len(realtimeResp.Warnings) > 0 {
			warnings = append(warnings, realtimeResp.Warnings...)
		}
	} else if err != nil {
		warnings = append(warnings, fmt.Sprintf("realtime positions unavailable: %v", err))
	}

	summary.WalletUSD = round2(summary.WalletUSD)
	summary.PositionUSD = round2(summary.PositionUSD)
	summary.FeeUSD = round2(summary.FeeUSD)
	summary.TotalUSD = round2(summary.WalletUSD + summary.PositionUSD + summary.FeeUSD)

	sort.Slice(records, func(i, j int) bool {
		if records[i].Chain != records[j].Chain {
			return records[i].Chain < records[j].Chain
		}
		return strings.ToLower(records[i].WalletAddress) < strings.ToLower(records[j].WalletAddress)
	})

	return &UserAssetOverview{
		Summary:   summary,
		Wallets:   records,
		UpdatedAt: updatedAt,
		Timezone:  timeutil.LocationName(),
		Warnings:  dedupeStrings(warnings),
	}, nil
}

func (s *Service) buildUserWalletAsset(ctx context.Context, walletRow models.Wallet, chain string, tracked []tokenDescriptor) (userWalletAsset, string) {
	record := userWalletAsset{
		WalletID:      walletRow.ID,
		WalletAddress: normalizeAddress(walletRow.Address),
		Chain:         config.NormalizeChain(chain),
	}
	if record.WalletAddress == "" {
		return record, "invalid wallet address"
	}

	client, cc, err := s.getClientForChain(record.Chain)
	if err != nil {
		return record, fmt.Sprintf("chain %s unavailable: %v", record.Chain, err)
	}

	walletAddr := common.HexToAddress(record.WalletAddress)
	nativePrice := s.nativePriceUSD(record.Chain, cc)
	if nativeBalance, err := blockchain.GetBalanceWithClient(client, walletAddr); err == nil && nativeBalance != nil {
		record.NativeUSD = balanceToUSD(amountToFloat(nativeBalance.String(), 18), nativePrice)
	}

	tokenAddresses := make([]string, 0, len(tracked))
	for _, token := range tracked {
		if token.Address == "" {
			continue
		}
		tokenAddresses = append(tokenAddresses, token.Address)
	}
	prices, _ := s.priceService.GetUSDPrices(record.Chain, tokenAddresses)

	for _, token := range tracked {
		addr := normalizeAddress(token.Address)
		if addr == "" {
			continue
		}
		decimals := s.tokenFallbackDecimals(cc, addr)
		decimals = s.getTokenDecimals(record.Chain, client, addr, decimals)
		balance, err := blockchain.GetTokenBalanceWithClient(client, common.HexToAddress(addr), walletAddr)
		if err != nil || balance == nil || balance.Sign() <= 0 {
			continue
		}
		price := prices[addr]
		usd := balanceToUSD(amountToFloat(balance.String(), decimals), price)
		if token.Stable {
			record.StableUSD += usd
		} else {
			record.TokenUSD += usd
		}
	}

	record.NativeUSD = round2(record.NativeUSD)
	record.StableUSD = round2(record.StableUSD)
	record.TokenUSD = round2(record.TokenUSD)
	record.TotalUSD = round2(record.NativeUSD + record.StableUSD + record.TokenUSD)
	return record, ""
}

func (s *Service) loadUserTrackedTokens(ctx context.Context, userID uint) (map[string][]tokenDescriptor, error) {
	out := make(map[string][]tokenDescriptor)
	if config.AppConfig == nil {
		return out, fmt.Errorf("config not loaded")
	}

	for _, chain := range config.EnabledChainsNormalized() {
		cc, ok := config.AppConfig.GetChainConfig(chain)
		if !ok {
			continue
		}
		tokenMap := map[string]tokenDescriptor{}
		for _, addr := range []string{cc.USDTAddress, cc.USDCAddress, cc.BUSDAddress} {
			if normalized := normalizeAddress(addr); normalized != "" {
				tokenMap[normalized] = tokenDescriptor{Address: normalized, Stable: true}
			}
		}
		out[chain] = append(out[chain], mapValues(tokenMap)...)
	}

	type tokenRow struct {
		Chain         string
		Token0Address string
		Token1Address string
	}
	var rows []tokenRow
	err := database.DB.WithContext(ctx).
		Model(&models.StrategyTask{}).
		Select("chain, token0_address, token1_address").
		Where("user_id = ? AND status IN ?", userID, activeStrategyStatuses()).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		chain := config.NormalizeChain(row.Chain)
		if chain == "" {
			chain = "bsc"
		}
		descriptors := make(map[string]tokenDescriptor)
		for _, existing := range out[chain] {
			descriptors[existing.Address] = existing
		}
		for _, addr := range []string{row.Token0Address, row.Token1Address} {
			normalized := normalizeAddress(addr)
			if normalized == "" {
				continue
			}
			existing := descriptors[normalized]
			existing.Address = normalized
			descriptors[normalized] = existing
		}
		out[chain] = mapValues(descriptors)
	}

	for chain := range out {
		sort.Slice(out[chain], func(i, j int) bool {
			return out[chain][i].Address < out[chain][j].Address
		})
	}
	return out, nil
}

func mapValues[T any](values map[string]T) []T {
	out := make([]T, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func (s *Service) nativePriceUSD(chain string, cc config.ChainConfig) float64 {
	addr := normalizeAddress(cc.WrappedNativeAddress)
	if addr == "" {
		return 0
	}
	prices, _ := s.priceService.GetUSDPrices(chain, []string{addr})
	return round4(prices[addr])
}

func (s *Service) tokenFallbackDecimals(cc config.ChainConfig, tokenAddress string) int {
	addr := normalizeAddress(tokenAddress)
	switch addr {
	case normalizeAddress(cc.StableAddress):
		if cc.StableDecimals > 0 {
			return cc.StableDecimals
		}
	case normalizeAddress(cc.USDCAddress):
		if cc.Chain == "base" {
			return 6
		}
	case normalizeAddress(cc.USDTAddress), normalizeAddress(cc.BUSDAddress), normalizeAddress(cc.WrappedNativeAddress):
		return 18
	}
	return 18
}

func (s *Service) GetUserHistory(ctx context.Context, userID uint, days int) (*UserAssetHistory, error) {
	days = clampHistoryDays(days)
	start := dayStart(timeutil.Now()).AddDate(0, 0, -days)
	end := dayStart(timeutil.Now())

	var rows []models.UserAssetDailySnapshot
	if err := database.DB.WithContext(ctx).
		Where("user_id = ? AND wallet_id = ? AND chain = ? AND snapshot_day >= ? AND snapshot_day < ?",
			userID, aggregateWalletID, "", formatDay(start), formatDay(end)).
		Order("snapshot_day ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	points := make([]UserAssetHistoryPoint, 0, len(rows))
	foundDays := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		point := UserAssetHistoryPoint{
			Day:         row.SnapshotDay,
			WalletUSD:   round2(row.WalletUSD),
			PositionUSD: round2(row.PositionUSD),
			FeeUSD:      round2(row.FeeUSD),
			TotalUSD:    round2(row.TotalUSD),
		}
		points = append(points, point)
		foundDays[row.SnapshotDay] = struct{}{}
	}

	missing := make([]string, 0)
	for cursor := start; cursor.Before(end); cursor = cursor.Add(24 * time.Hour) {
		day := formatDay(cursor)
		if _, ok := foundDays[day]; !ok {
			missing = append(missing, day)
		}
	}

	overview, err := s.GetUserOverview(ctx, userID)
	if err != nil {
		return nil, err
	}
	today := UserAssetHistoryPoint{
		Day:         formatDay(timeutil.Now()),
		WalletUSD:   overview.Summary.WalletUSD,
		PositionUSD: overview.Summary.PositionUSD,
		FeeUSD:      overview.Summary.FeeUSD,
		TotalUSD:    overview.Summary.TotalUSD,
	}

	return &UserAssetHistory{
		Days:        days,
		History:     points,
		Today:       today,
		MissingDays: missing,
		UpdatedAt:   overview.UpdatedAt,
		Timezone:    timeutil.LocationName(),
		Warnings:    overview.Warnings,
	}, nil
}

func (s *Service) GetUserLPStats(ctx context.Context, userID uint) (*UserLPStatsResponse, error) {
	now := timeutil.Now()
	start := dayStart(now).AddDate(0, 0, -30)
	trades, err := s.loadUserLPTrades(ctx, userID, start, now)
	if err != nil {
		return nil, err
	}
	resp := buildUserLPStatsFromTrades(trades, now)

	snapshotRows, err := s.loadUserAssetSnapshotRows(ctx, userID, dayStart(now).AddDate(0, 0, -31), dayStart(now))
	if err != nil {
		return nil, err
	}

	var liveTotalUSD *float64
	overview, err := s.GetUserOverview(ctx, userID)
	if err != nil {
		log.Printf("[Assets] user overview unavailable for live snapshot pnl user=%d err=%v", userID, err)
	} else if overview != nil {
		total := overview.Summary.TotalUSD
		liveTotalUSD = &total
	}

	resp = applyUserSnapshotPnL(resp, snapshotRows, liveTotalUSD, now)
	return &resp, nil
}

func (s *Service) captureUserAssetSnapshots(ctx context.Context, day time.Time) error {
	if database.DB == nil {
		return fmt.Errorf("database not initialized")
	}
	var userIDs []uint
	if err := database.DB.WithContext(ctx).
		Model(&models.Wallet{}).
		Distinct("user_id").
		Order("user_id ASC").
		Pluck("user_id", &userIDs).Error; err != nil {
		return err
	}

	dayKey := formatDay(day)
	if err := database.DB.WithContext(ctx).
		Where("snapshot_day = ? AND wallet_id = ? AND chain = ?", dayKey, aggregateWalletID, "").
		Delete(&models.UserAssetDailySnapshot{}).Error; err != nil {
		return err
	}

	for _, userID := range userIDs {
		overview, err := s.GetUserOverview(ctx, userID)
		if err != nil {
			log.Printf("[Assets] skip user snapshot user=%d err=%v", userID, err)
			continue
		}
		row := &models.UserAssetDailySnapshot{
			UserID:      userID,
			WalletID:    aggregateWalletID,
			Chain:       "",
			SnapshotDay: dayKey,
			WalletUSD:   overview.Summary.WalletUSD,
			PositionUSD: overview.Summary.PositionUSD,
			FeeUSD:      overview.Summary.FeeUSD,
			TotalUSD:    overview.Summary.TotalUSD,
			CapturedAt:  timeutil.Now(),
		}
		if err := upsertByColumns(ctx, row,
			[]string{"user_id", "wallet_id", "chain", "snapshot_day"},
			map[string]interface{}{
				"wallet_usd":   row.WalletUSD,
				"position_usd": row.PositionUSD,
				"fee_usd":      row.FeeUSD,
				"total_usd":    row.TotalUSD,
				"captured_at":  row.CapturedAt,
			}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) captureUserLPDailyStats(ctx context.Context, day time.Time) error {
	if database.DB == nil {
		return fmt.Errorf("database not initialized")
	}
	start := dayStart(day)
	end := dayEnd(day)
	dayKey := formatDay(day)

	if err := database.DB.WithContext(ctx).
		Where("stat_day = ? AND wallet_id = ? AND chain = ?", dayKey, aggregateWalletID, "").
		Delete(&models.UserLPDailyStat{}).Error; err != nil {
		return err
	}

	trades, err := s.loadUserLPTrades(ctx, 0, start, end)
	if err != nil {
		return err
	}

	statsByUser := make(map[uint]*userLPBucket)
	for _, trade := range trades {
		if trade.UserID == 0 {
			continue
		}
		bucket := statsByUser[trade.UserID]
		if bucket == nil {
			bucket = &userLPBucket{}
			statsByUser[trade.UserID] = bucket
		}
		bucket.addProfit(parseTradeProfitWei(trade))
	}

	userIDs := make([]uint, 0, len(statsByUser))
	for userID := range statsByUser {
		userIDs = append(userIDs, userID)
	}
	sort.Slice(userIDs, func(i, j int) bool {
		return userIDs[i] < userIDs[j]
	})

	for _, userID := range userIDs {
		stats := statsByUser[userID].toWindowStats(0)
		row := &models.UserLPDailyStat{
			UserID:         userID,
			WalletID:       aggregateWalletID,
			Chain:          "",
			StatDay:        dayKey,
			RealizedPnLUSD: stats.RealizedPnLUSD,
			ClosedCount:    stats.ClosedCount,
			WinCount:       stats.WinCount,
			LossCount:      stats.LossCount,
			BreakEvenCount: stats.BreakEvenCount,
			CapturedAt:     timeutil.Now(),
		}
		if err := upsertByColumns(ctx, row,
			[]string{"user_id", "wallet_id", "chain", "stat_day"},
			map[string]interface{}{
				"realized_pnl_usd": row.RealizedPnLUSD,
				"closed_count":     row.ClosedCount,
				"win_count":        row.WinCount,
				"loss_count":       row.LossCount,
				"break_even_count": row.BreakEvenCount,
				"captured_at":      row.CapturedAt,
			}); err != nil {
			return err
		}
	}
	return nil
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
