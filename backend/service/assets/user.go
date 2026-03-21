package assets

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"context"
	"fmt"
	"log"
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
	windows := []int{1, 7, 30}
	startOfToday := dayStart(timeutil.Now())
	out := make([]UserLPWindowStats, 0, len(windows))
	for _, days := range windows {
		stats, err := s.queryUserLPWindow(ctx, userID, startOfToday.AddDate(0, 0, -days), startOfToday, days)
		if err != nil {
			return nil, err
		}
		out = append(out, stats)
	}
	today, err := s.queryUserLPWindow(ctx, userID, startOfToday, timeutil.Now(), 0)
	if err != nil {
		return nil, err
	}

	// Per-pool today breakdown
	type poolRow struct {
		PoolId       string
		Token0Symbol string
		Token1Symbol string
		Chain        string
		ProfitUSD    float64
		ClosedCount  int
	}
	var poolRows []poolRow
	database.DB.WithContext(ctx).
		Raw(`
			SELECT
				pool_id,
				token0_symbol,
				token1_symbol,
				chain,
				COALESCE(SUM(CAST(profit_usdt AS DECIMAL(36, 18)) / 1000000000000000000), 0) AS profit_usd,
				COUNT(*) AS closed_count
			FROM trade_records
			WHERE user_id = ?
			  AND status = ?
			  AND closed_at >= ?
			  AND closed_at < ?
			GROUP BY pool_id, token0_symbol, token1_symbol, chain
			ORDER BY profit_usd DESC
		`, userID, models.TradeStatusClosed, startOfToday, timeutil.Now()).
		Scan(&poolRows)

	todayPools := make([]UserLPPoolPnL, 0, len(poolRows))
	for _, pr := range poolRows {
		todayPools = append(todayPools, UserLPPoolPnL{
			PoolID:       pr.PoolId,
			Token0Symbol: pr.Token0Symbol,
			Token1Symbol: pr.Token1Symbol,
			Chain:        pr.Chain,
			ProfitUSD:    round2(pr.ProfitUSD),
			ClosedCount:  pr.ClosedCount,
		})
	}

	// Daily LP history (last 30 days)
	var dailyRows []models.UserLPDailyStat
	database.DB.WithContext(ctx).
		Where("user_id = ? AND wallet_id = ? AND chain = ? AND stat_day >= ?",
			userID, aggregateWalletID, "", formatDay(startOfToday.AddDate(0, 0, -30))).
		Order("stat_day ASC").
		Find(&dailyRows)

	dailyHistory := make([]UserLPDailyPoint, 0, len(dailyRows))
	for _, dr := range dailyRows {
		dailyHistory = append(dailyHistory, UserLPDailyPoint{
			Day:            dr.StatDay,
			RealizedPnLUSD: round2(dr.RealizedPnLUSD),
			ClosedCount:    dr.ClosedCount,
			WinCount:       dr.WinCount,
			LossCount:      dr.LossCount,
		})
	}

	return &UserLPStatsResponse{
		Windows:      out,
		Today:        today,
		TodayPools:   todayPools,
		DailyHistory: dailyHistory,
		Timezone:     timeutil.LocationName(),
	}, nil
}

func (s *Service) queryUserLPWindow(ctx context.Context, userID uint, start time.Time, end time.Time, days int) (UserLPWindowStats, error) {
	type row struct {
		RealizedPnLUSD float64
		ClosedCount    int
		WinCount       int
		LossCount      int
		BreakEvenCount int
	}
	var result row
	err := database.DB.WithContext(ctx).
		Raw(`
			SELECT
				COALESCE(SUM(CAST(profit_usdt AS DECIMAL(36, 18)) / 1000000000000000000), 0) AS realized_pnl_usd,
				COUNT(*) AS closed_count,
				SUM(CASE WHEN CAST(profit_usdt AS DECIMAL(36, 18)) > 0 THEN 1 ELSE 0 END) AS win_count,
				SUM(CASE WHEN CAST(profit_usdt AS DECIMAL(36, 18)) < 0 THEN 1 ELSE 0 END) AS loss_count,
				SUM(CASE WHEN CAST(profit_usdt AS DECIMAL(36, 18)) = 0 THEN 1 ELSE 0 END) AS break_even_count
			FROM trade_records
			WHERE user_id = ?
			  AND status = ?
			  AND closed_at >= ?
			  AND closed_at < ?
		`, userID, models.TradeStatusClosed, start, end).
		Scan(&result).Error
	if err != nil {
		return UserLPWindowStats{}, err
	}

	stats := UserLPWindowStats{
		Days:           days,
		RealizedPnLUSD: round2(result.RealizedPnLUSD),
		ClosedCount:    result.ClosedCount,
		WinCount:       result.WinCount,
		LossCount:      result.LossCount,
		BreakEvenCount: result.BreakEvenCount,
	}
	if stats.ClosedCount > 0 {
		stats.WinRate = round4(float64(stats.WinCount) / float64(stats.ClosedCount))
		stats.AvgPnLUSD = round2(stats.RealizedPnLUSD / float64(stats.ClosedCount))
	}
	return stats, nil
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

	type row struct {
		UserID         uint
		RealizedPnLUSD float64
		ClosedCount    int
		WinCount       int
		LossCount      int
		BreakEvenCount int
	}
	var rows []row
	if err := database.DB.WithContext(ctx).
		Raw(`
			SELECT
				user_id,
				COALESCE(SUM(CAST(profit_usdt AS DECIMAL(36, 18)) / 1000000000000000000), 0) AS realized_pnl_usd,
				COUNT(*) AS closed_count,
				SUM(CASE WHEN CAST(profit_usdt AS DECIMAL(36, 18)) > 0 THEN 1 ELSE 0 END) AS win_count,
				SUM(CASE WHEN CAST(profit_usdt AS DECIMAL(36, 18)) < 0 THEN 1 ELSE 0 END) AS loss_count,
				SUM(CASE WHEN CAST(profit_usdt AS DECIMAL(36, 18)) = 0 THEN 1 ELSE 0 END) AS break_even_count
			FROM trade_records
			WHERE status = ?
			  AND closed_at >= ?
			  AND closed_at < ?
			GROUP BY user_id
		`, models.TradeStatusClosed, start, end).
		Scan(&rows).Error; err != nil {
		return err
	}

	for _, item := range rows {
		row := &models.UserLPDailyStat{
			UserID:         item.UserID,
			WalletID:       aggregateWalletID,
			Chain:          "",
			StatDay:        dayKey,
			RealizedPnLUSD: round2(item.RealizedPnLUSD),
			ClosedCount:    item.ClosedCount,
			WinCount:       item.WinCount,
			LossCount:      item.LossCount,
			BreakEvenCount: item.BreakEvenCount,
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
