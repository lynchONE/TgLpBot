package assets

import (
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"testing"
	"time"
)

func TestBuildSmartMoneySnapshotLeaderboard_UsesDailyStatPnLWhenAvailable(t *testing.T) {
	timeutil.Init()

	label := "Alpha"
	avatarURL := "http://minio.example/avatar/smart-money/a1.jpg"
	snapshotDay := time.Date(2026, time.March, 23, 0, 0, 0, 0, timeutil.Location())
	comparedDay := snapshotDay.AddDate(0, 0, -1)

	resp := buildSmartMoneySnapshotLeaderboard("pnl", snapshotDay, comparedDay, 1, 20, []smartMoneyLeaderboardSnapshotInput{
		{
			Wallet: models.MonitoredWallet{
				Address:   "0x00000000000000000000000000000000000000a1",
				ChainID:   56,
				Label:     &label,
				AvatarURL: &avatarURL,
			},
			Current: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD:        135,
				HasTransferIn:   true,
				TransferInCount: 1,
				TransferInUSD:   9.5,
			},
			Previous: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD: 100,
			},
			DailyStat: &models.SmartMoneyLPDailyStat{
				EstimatedRealizedPnLUSD: 25.5,
				MatchedCostUSD:          100,
				AddCount:                1,
				RemoveCount:             2,
				ActivePoolCount:         3,
				UnmatchedRemoveCount:    1,
			},
		},
		{
			Wallet: models.MonitoredWallet{
				Address: "0x00000000000000000000000000000000000000b2",
				ChainID: 56,
			},
			Current: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD:         120,
				HasTransferOut:   true,
				TransferOutCount: 1,
				TransferOutUSD:   18.75,
			},
			Previous: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD: 100,
			},
			DailyStat: &models.SmartMoneyLPDailyStat{
				EstimatedRealizedPnLUSD: 38.75,
				MatchedCostUSD:          100,
				AddCount:                4,
				RemoveCount:             1,
				ActivePoolCount:         2,
			},
		},
		{
			Wallet: models.MonitoredWallet{
				Address: "0x00000000000000000000000000000000000000c3",
				ChainID: 56,
			},
			Current: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD: 80,
			},
		},
	})

	if resp == nil {
		t.Fatal("response is nil")
	}
	if got, want := resp.SnapshotDay, "2026-03-23"; got != want {
		t.Fatalf("snapshot day = %s, want %s", got, want)
	}
	if got, want := resp.ComparedDay, "2026-03-22"; got != want {
		t.Fatalf("compared day = %s, want %s", got, want)
	}
	if got, want := len(resp.List), 2; got != want {
		t.Fatalf("leaderboard size = %d, want %d", got, want)
	}

	first := resp.List[0]
	if got, want := first.Address, "0x00000000000000000000000000000000000000b2"; got != want {
		t.Fatalf("first address = %s, want %s", got, want)
	}
	if got, want := first.Rank, 1; got != want {
		t.Fatalf("first rank = %d, want %d", got, want)
	}
	if got, want := first.EstimatedRealizedPnLUSD, 38.75; got != want {
		t.Fatalf("first pnl = %.2f, want %.2f", got, want)
	}
	if got, want := first.YieldRate, 0.3875; got != want {
		t.Fatalf("first yield = %.4f, want %.4f", got, want)
	}
	if got, want := first.BaselineTotalUSD, 100.0; got != want {
		t.Fatalf("first baseline total = %.2f, want %.2f", got, want)
	}
	if got, want := first.CurrentTotalUSD, 120.0; got != want {
		t.Fatalf("first current total = %.2f, want %.2f", got, want)
	}
	if !first.HasTransferOut || first.TransferOutCount != 1 {
		t.Fatalf("first transfer out flag/count = %v/%d, want true/1", first.HasTransferOut, first.TransferOutCount)
	}
	if got, want := first.TransferNetUSD, -18.75; got != want {
		t.Fatalf("first transfer net usd = %.2f, want %.2f", got, want)
	}

	second := resp.List[1]
	if got, want := second.Address, "0x00000000000000000000000000000000000000a1"; got != want {
		t.Fatalf("second address = %s, want %s", got, want)
	}
	if got, want := second.AvatarURL, avatarURL; got != want {
		t.Fatalf("second avatar url = %s, want %s", got, want)
	}
	if got, want := second.EstimatedRealizedPnLUSD, 25.5; got != want {
		t.Fatalf("second pnl = %.2f, want %.2f", got, want)
	}
	if got, want := second.YieldRate, 0.255; got != want {
		t.Fatalf("second yield = %.4f, want %.4f", got, want)
	}
	if got, want := second.ParticipationCount, 3; got != want {
		t.Fatalf("second participation = %d, want %d", got, want)
	}
	if !second.HasTransferIn || second.TransferInCount != 1 {
		t.Fatalf("second transfer in flag/count = %v/%d, want true/1", second.HasTransferIn, second.TransferInCount)
	}
	if got, want := second.TransferNetUSD, 9.5; got != want {
		t.Fatalf("second transfer net usd = %.2f, want %.2f", got, want)
	}
}

func TestApplySmartMoneyLeaderboardWalletMeta_UsesCurrentWalletMetadata(t *testing.T) {
	label := "Updated Alpha"
	avatarURL := "http://minio.example/avatar/smart-money/current.jpg"
	sourceContract := "0x00000000000000000000000000000000000000cc"
	resp := &SmartMoneyLeaderboardResponse{
		List: []SmartMoneyLeaderboardEntry{
			{
				Address:   "0x00000000000000000000000000000000000000a1",
				ChainID:   56,
				Label:     "Old Alpha",
				AvatarURL: "http://minio.example/avatar/smart-money/old.jpg",
				Source:    "manual",
			},
		},
	}

	applySmartMoneyLeaderboardWalletMeta(resp, []models.MonitoredWallet{
		{
			Address:        "0x00000000000000000000000000000000000000a1",
			ChainID:        56,
			Source:         "contract_interaction",
			SourceContract: &sourceContract,
			Label:          &label,
			AvatarURL:      &avatarURL,
		},
	})

	if got, want := resp.List[0].Label, label; got != want {
		t.Fatalf("label = %s, want %s", got, want)
	}
	if got, want := resp.List[0].AvatarURL, avatarURL; got != want {
		t.Fatalf("avatar url = %s, want %s", got, want)
	}
	if got, want := resp.List[0].Source, "contract_interaction"; got != want {
		t.Fatalf("source = %s, want %s", got, want)
	}
	if got, want := resp.List[0].SourceContract, sourceContract; got != want {
		t.Fatalf("source contract = %s, want %s", got, want)
	}
}

func TestSmartMoneyWalletSummaryFromLive_IncludesWalletMetadata(t *testing.T) {
	avatarURL := "http://minio.example/avatar/smart-money/live.jpg"
	sourceContract := "0x00000000000000000000000000000000000000dd"
	label := "Alpha"

	got := smartMoneyWalletSummaryFromLive(models.MonitoredWallet{
		Address:        "0x00000000000000000000000000000000000000a1",
		ChainID:        56,
		Source:         "contract_interaction",
		SourceContract: &sourceContract,
		Label:          &label,
		AvatarURL:      &avatarURL,
	}, smartMoneyWalletLiveState{
		assets: smartMoneyAssetBreakdown{TotalUSD: 123.45},
	}, &models.SmartMoneyWalletDailySnapshot{TotalUSD: 100})

	if got.AvatarURL != avatarURL {
		t.Fatalf("avatar url = %s, want %s", got.AvatarURL, avatarURL)
	}
	if got.Source != "contract_interaction" {
		t.Fatalf("source = %s, want %s", got.Source, "contract_interaction")
	}
	if got.SourceContract != sourceContract {
		t.Fatalf("source contract = %s, want %s", got.SourceContract, sourceContract)
	}
	if got.CurrentTotalUSD == nil || *got.CurrentTotalUSD != 123.45 {
		t.Fatalf("current total = %v, want 123.45", got.CurrentTotalUSD)
	}
	if got.BaselineTotalUSD == nil || *got.BaselineTotalUSD != 100 {
		t.Fatalf("baseline total = %v, want 100", got.BaselineTotalUSD)
	}
}

func TestSmartMoneyWalletSummaryFromSnapshot_IncludesWalletMetadata(t *testing.T) {
	avatarURL := "http://minio.example/avatar/smart-money/snapshot.jpg"
	sourceContract := "0x00000000000000000000000000000000000000ee"

	got := smartMoneyWalletSummaryFromSnapshot(models.MonitoredWallet{
		Address:        "0x00000000000000000000000000000000000000a1",
		ChainID:        56,
		Source:         "contract_interaction",
		SourceContract: &sourceContract,
		AvatarURL:      &avatarURL,
	}, nil, nil)

	if got.AvatarURL != avatarURL {
		t.Fatalf("avatar url = %s, want %s", got.AvatarURL, avatarURL)
	}
	if got.Source != "contract_interaction" {
		t.Fatalf("source = %s, want %s", got.Source, "contract_interaction")
	}
	if got.SourceContract != sourceContract {
		t.Fatalf("source contract = %s, want %s", got.SourceContract, sourceContract)
	}
}

func TestBuildSmartMoneySnapshotLeaderboard_ParticipationMetricRanksByDailyOps(t *testing.T) {
	timeutil.Init()

	snapshotDay := time.Date(2026, time.March, 23, 0, 0, 0, 0, timeutil.Location())
	comparedDay := snapshotDay.AddDate(0, 0, -1)

	resp := buildSmartMoneySnapshotLeaderboard("participation", snapshotDay, comparedDay, 1, 20, []smartMoneyLeaderboardSnapshotInput{
		{
			Wallet: models.MonitoredWallet{
				Address: "0x00000000000000000000000000000000000000d4",
				ChainID: 56,
			},
			Current:  &models.SmartMoneyWalletDailySnapshot{TotalUSD: 150},
			Previous: &models.SmartMoneyWalletDailySnapshot{TotalUSD: 100},
			DailyStat: &models.SmartMoneyLPDailyStat{
				EstimatedRealizedPnLUSD: 50,
				MatchedCostUSD:          100,
				AddCount:                1,
				RemoveCount:             1,
			},
		},
		{
			Wallet: models.MonitoredWallet{
				Address: "0x00000000000000000000000000000000000000e5",
				ChainID: 56,
			},
			Current:  &models.SmartMoneyWalletDailySnapshot{TotalUSD: 110},
			Previous: &models.SmartMoneyWalletDailySnapshot{TotalUSD: 100},
			DailyStat: &models.SmartMoneyLPDailyStat{
				EstimatedRealizedPnLUSD: 10,
				MatchedCostUSD:          100,
				AddCount:                3,
				RemoveCount:             2,
			},
		},
	})

	if got, want := resp.List[0].Address, "0x00000000000000000000000000000000000000e5"; got != want {
		t.Fatalf("top participation address = %s, want %s", got, want)
	}
	if got, want := resp.List[0].MetricValue, 5.0; got != want {
		t.Fatalf("top participation metric = %.2f, want %.2f", got, want)
	}
	if got, want := resp.List[1].EstimatedRealizedPnLUSD, 50.0; got != want {
		t.Fatalf("second pnl = %.2f, want %.2f", got, want)
	}
}

func TestBuildSmartMoneySnapshotLeaderboard_FallsBackToTransferAdjustedSnapshotDeltaWithoutDailyStat(t *testing.T) {
	timeutil.Init()

	snapshotDay := time.Date(2026, time.March, 23, 0, 0, 0, 0, timeutil.Location())
	comparedDay := snapshotDay.AddDate(0, 0, -1)

	resp := buildSmartMoneySnapshotLeaderboard("pnl", snapshotDay, comparedDay, 1, 20, []smartMoneyLeaderboardSnapshotInput{
		{
			Wallet: models.MonitoredWallet{
				Address: "0x00000000000000000000000000000000000000f6",
				ChainID: 56,
			},
			Current: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD:       155,
				TransferInUSD:  20,
				TransferOutUSD: 10,
			},
			Previous: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD: 100,
			},
		},
	})

	if got, want := len(resp.List), 1; got != want {
		t.Fatalf("leaderboard size = %d, want %d", got, want)
	}
	if got, want := resp.List[0].EstimatedRealizedPnLUSD, 45.0; got != want {
		t.Fatalf("fallback pnl = %.2f, want %.2f", got, want)
	}
	if got, want := resp.List[0].YieldRate, 0.45; got != want {
		t.Fatalf("fallback yield = %.4f, want %.4f", got, want)
	}
	if got, want := resp.List[0].TransferNetUSD, 10.0; got != want {
		t.Fatalf("fallback transfer net usd = %.2f, want %.2f", got, want)
	}
}

func TestBuildSmartMoneySnapshotLeaderboard_UsesRawAssetDeltaForLiveInputs(t *testing.T) {
	timeutil.Init()

	now := time.Date(2026, time.March, 23, 13, 30, 0, 0, timeutil.Location())
	baseDay := dayStart(now).AddDate(0, 0, -1)

	resp := buildSmartMoneySnapshotLeaderboard("pnl", now, baseDay, 1, 20, []smartMoneyLeaderboardSnapshotInput{
		{
			Wallet: models.MonitoredWallet{
				Address: "0x00000000000000000000000000000000000000f7",
				ChainID: 56,
			},
			Current: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD:       155,
				TransferInUSD:  999,
				TransferOutUSD: 20,
			},
			Previous: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD: 100,
			},
			DailyStat: &models.SmartMoneyLPDailyStat{
				EstimatedRealizedPnLUSD: -500,
				MatchedCostUSD:          100,
				AddCount:                2,
				RemoveCount:             1,
				ActivePoolCount:         3,
			},
			UseRawAssetDelta:   true,
			IgnoreDailyStatPnL: true,
		},
	})

	if got, want := len(resp.List), 1; got != want {
		t.Fatalf("leaderboard size = %d, want %d", got, want)
	}
	entry := resp.List[0]
	if got, want := entry.EstimatedRealizedPnLUSD, 55.0; got != want {
		t.Fatalf("live pnl = %.2f, want %.2f", got, want)
	}
	if got, want := entry.YieldRate, 0.55; got != want {
		t.Fatalf("live yield = %.4f, want %.4f", got, want)
	}
	if got, want := entry.ParticipationCount, 3; got != want {
		t.Fatalf("live participation = %d, want %d", got, want)
	}
}

func TestBuildSmartMoneySnapshotLeaderboard_UsesWindowDailyStatPnL(t *testing.T) {
	timeutil.Init()

	snapshotDay := time.Date(2026, time.March, 23, 0, 0, 0, 0, timeutil.Location())
	comparedDay := snapshotDay.AddDate(0, 0, -7)

	resp := buildSmartMoneySnapshotLeaderboard("pnl", snapshotDay, comparedDay, 7, 20, []smartMoneyLeaderboardSnapshotInput{
		{
			Wallet: models.MonitoredWallet{
				Address: "0x00000000000000000000000000000000000000a1",
				ChainID: 56,
			},
			Current: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD:       1200,
				TransferInUSD:  100,
				TransferOutUSD: 20,
			},
			Previous: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD: 1000,
			},
			DailyStat: &models.SmartMoneyLPDailyStat{
				EstimatedRealizedPnLUSD: 42.5,
				MatchedCostUSD:          250,
				AddCount:                2,
				RemoveCount:             1,
				ActivePoolCount:         3,
			},
		},
	})

	if got, want := resp.Days, 7; got != want {
		t.Fatalf("days = %d, want %d", got, want)
	}
	if got, want := resp.StartDay, "2026-03-16"; got != want {
		t.Fatalf("start day = %s, want %s", got, want)
	}
	if got, want := len(resp.List), 1; got != want {
		t.Fatalf("leaderboard size = %d, want %d", got, want)
	}
	entry := resp.List[0]
	if got, want := entry.EstimatedRealizedPnLUSD, 42.5; got != want {
		t.Fatalf("pnl = %.2f, want %.2f", got, want)
	}
	if got, want := entry.YieldRate, 0.17; got != want {
		t.Fatalf("yield = %.4f, want %.4f", got, want)
	}
	if got, want := entry.ParticipationCount, 3; got != want {
		t.Fatalf("participation = %d, want %d", got, want)
	}
}

func TestPaginateSmartMoneyLeaderboardResponse_FiltersAndSlicesFromBackend(t *testing.T) {
	resp := &SmartMoneyLeaderboardResponse{
		Metric: "pnl",
		List: []SmartMoneyLeaderboardEntry{
			{Rank: 1, Address: "0xaaa111", Label: "Alpha"},
			{Rank: 2, Address: "0xbbb222", Label: "Beta"},
			{Rank: 3, Address: "0xccc333", Label: "Alpha Two"},
		},
	}

	got := paginateSmartMoneyLeaderboardResponse(resp, 2, 1, "alpha")
	if got == nil {
		t.Fatal("response is nil")
	}
	if got.Page != 2 {
		t.Fatalf("page = %d, want 2", got.Page)
	}
	if got.PageSize != 1 {
		t.Fatalf("page size = %d, want 1", got.PageSize)
	}
	if got.Total != 2 {
		t.Fatalf("total = %d, want 2", got.Total)
	}
	if got.TotalPages != 2 {
		t.Fatalf("total pages = %d, want 2", got.TotalPages)
	}
	if got.Keyword != "alpha" {
		t.Fatalf("keyword = %q, want alpha", got.Keyword)
	}
	if len(got.List) != 1 {
		t.Fatalf("list size = %d, want 1", len(got.List))
	}
	if got.List[0].Address != "0xccc333" {
		t.Fatalf("page item address = %s, want 0xccc333", got.List[0].Address)
	}
	if got.List[0].Rank != 3 {
		t.Fatalf("page item rank = %d, want 3", got.List[0].Rank)
	}
}

func TestBuildSmartMoneyHistoryPoints_FallsBackToTransferAdjustedBalanceDelta(t *testing.T) {
	rows := []smartMoneyHistoryDayRow{
		{Day: "2026-03-25", TotalUSD: 100, NativeUSD: 10},
		{Day: "2026-03-26", TotalUSD: 140, NativeUSD: 11},
		{Day: "2026-03-27", TotalUSD: 90, NativeUSD: 9, HasTransferOut: 1, TransferOutCount: 1, TransferOutUSD: 50},
	}

	points := buildSmartMoneyHistoryPoints(rows)
	if got, want := len(points), 3; got != want {
		t.Fatalf("history points = %d, want %d", got, want)
	}
	if got, want := points[0].EstimatedRealizedPnLUSD, 0.0; got != want {
		t.Fatalf("first day pnl = %.2f, want %.2f", got, want)
	}
	if got, want := points[1].EstimatedRealizedPnLUSD, 40.0; got != want {
		t.Fatalf("second day pnl = %.2f, want %.2f", got, want)
	}
	if got, want := points[2].EstimatedRealizedPnLUSD, 0.0; got != want {
		t.Fatalf("third day pnl = %.2f, want %.2f", got, want)
	}
	if got, want := points[2].TransferNetUSD, -50.0; got != want {
		t.Fatalf("transfer net usd = %.2f, want %.2f", got, want)
	}
}

func TestBuildSmartMoneyHistoryPoints_UsesDailyStatPnLWhenAvailable(t *testing.T) {
	rows := []smartMoneyHistoryDayRow{
		{
			Day:                     "2026-03-25",
			TotalUSD:                100,
			SnapshotCount:           1,
			DailyStatCount:          1,
			EstimatedRealizedPnLUSD: 7.25,
		},
		{
			Day:                     "2026-03-26",
			TotalUSD:                140,
			TransferInUSD:           100,
			SnapshotCount:           1,
			DailyStatCount:          1,
			EstimatedRealizedPnLUSD: -3.5,
		},
	}

	points := buildSmartMoneyHistoryPoints(rows)
	if got, want := len(points), 2; got != want {
		t.Fatalf("history points = %d, want %d", got, want)
	}
	if got, want := points[0].EstimatedRealizedPnLUSD, 7.25; got != want {
		t.Fatalf("first day pnl = %.2f, want %.2f", got, want)
	}
	if got, want := points[1].EstimatedRealizedPnLUSD, -3.5; got != want {
		t.Fatalf("second day pnl = %.2f, want %.2f", got, want)
	}
}

func TestBuildSmartMoneyHistoryPoints_FallsBackWhenDailyStatsArePartial(t *testing.T) {
	rows := []smartMoneyHistoryDayRow{
		{Day: "2026-03-25", TotalUSD: 100, SnapshotCount: 2, DailyStatCount: 2, EstimatedRealizedPnLUSD: 4},
		{Day: "2026-03-26", TotalUSD: 150, SnapshotCount: 2, DailyStatCount: 1, EstimatedRealizedPnLUSD: 999, TransferInUSD: 10},
	}

	points := buildSmartMoneyHistoryPoints(rows)
	if got, want := len(points), 2; got != want {
		t.Fatalf("history points = %d, want %d", got, want)
	}
	if got, want := points[1].EstimatedRealizedPnLUSD, 40.0; got != want {
		t.Fatalf("partial stat pnl = %.2f, want fallback %.2f", got, want)
	}
}

func TestBuildSmartMoneyHistoryPoints_RequiresConsecutiveSnapshots(t *testing.T) {
	rows := []smartMoneyHistoryDayRow{
		{Day: "2026-03-25", TotalUSD: 100},
		{Day: "2026-03-27", TotalUSD: 140},
	}

	points := buildSmartMoneyHistoryPoints(rows)
	if got, want := len(points), 2; got != want {
		t.Fatalf("history points = %d, want %d", got, want)
	}
	if got, want := points[1].EstimatedRealizedPnLUSD, 0.0; got != want {
		t.Fatalf("gap day pnl = %.2f, want %.2f", got, want)
	}
}

func TestBuildSmartMoneyTodayHistoryPoint_UsesPreviousDaySnapshotDelta(t *testing.T) {
	timeutil.Init()
	now := time.Date(2026, time.March, 28, 19, 37, 0, 0, timeutil.Location())

	point := buildSmartMoneyTodayHistoryPoint(now, smartMoneyAssetBreakdown{
		NativeUSD:       112.12,
		StableUSD:       2800.44,
		TrackedTokenUSD: 75.5,
		OpenLPUSD:       0,
		TotalUSD:        2988.06,
	}, "2026-03-27", 2864.61, true, smartMoneyTransferActivity{
		HasTransferIn:   true,
		TransferInCount: 1,
		TransferInUSD:   800,
	})

	if got, want := point.Day, "2026-03-28"; got != want {
		t.Fatalf("today day = %s, want %s", got, want)
	}
	if got, want := point.EstimatedRealizedPnLUSD, 123.45; got != want {
		t.Fatalf("today pnl = %.2f, want %.2f", got, want)
	}
	if got, want := point.TransferNetUSD, 800.0; got != want {
		t.Fatalf("today transfer net usd = %.2f, want %.2f", got, want)
	}
}

func TestBuildSmartMoneyTodayHistoryPoint_RequiresYesterdaySnapshot(t *testing.T) {
	timeutil.Init()
	now := time.Date(2026, time.March, 28, 19, 37, 0, 0, timeutil.Location())

	point := buildSmartMoneyTodayHistoryPoint(now, smartMoneyAssetBreakdown{
		TotalUSD: 2988.06,
	}, "2026-03-26", 2864.61, true, smartMoneyTransferActivity{})

	if got, want := point.EstimatedRealizedPnLUSD, 0.0; got != want {
		t.Fatalf("today pnl with stale snapshot = %.2f, want %.2f", got, want)
	}
}

func TestSmartMoneyWalletLiveCacheTTL_IsThirtyMinutes(t *testing.T) {
	if got, want := smartMoneyWalletLiveCacheTTL, 30*time.Minute; got != want {
		t.Fatalf("smart money wallet live cache ttl = %s, want %s", got, want)
	}
}

func TestNormalizeSmartMoneyOverviewSections_DefaultsToAll(t *testing.T) {
	got := normalizeSmartMoneyOverviewSections(nil)

	for _, section := range []smartMoneyOverviewSection{
		smartMoneyOverviewSectionSummary,
		smartMoneyOverviewSectionWallets,
		smartMoneyOverviewSectionHistory,
		smartMoneyOverviewSectionWindows,
	} {
		if !got[section] {
			t.Fatalf("section %s disabled, want enabled", section)
		}
	}
}

func TestNormalizeSmartMoneyOverviewSections_ParsesRequestedSections(t *testing.T) {
	got := normalizeSmartMoneyOverviewSections([]string{"summary,wallets"})

	if !got[smartMoneyOverviewSectionSummary] {
		t.Fatal("summary section disabled, want enabled")
	}
	if !got[smartMoneyOverviewSectionWallets] {
		t.Fatal("wallets section disabled, want enabled")
	}
	if got[smartMoneyOverviewSectionHistory] {
		t.Fatal("history section enabled, want disabled")
	}
	if got[smartMoneyOverviewSectionWindows] {
		t.Fatal("windows section enabled, want disabled")
	}
}

func TestClampSmartMoneyWalletHistoryDays_AllowsCalendarMonths(t *testing.T) {
	if got, want := clampSmartMoneyWalletHistoryDays(120), 120; got != want {
		t.Fatalf("history days = %d, want %d", got, want)
	}
	if got, want := clampSmartMoneyWalletHistoryDays(999), smartMoneyWalletMaxHistoryDays; got != want {
		t.Fatalf("history days cap = %d, want %d", got, want)
	}
}
