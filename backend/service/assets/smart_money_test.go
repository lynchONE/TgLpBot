package assets

import (
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"testing"
	"time"
)

func TestBuildSmartMoneySnapshotLeaderboard_UsesBalanceDeltaAndTransferAmounts(t *testing.T) {
	timeutil.Init()

	label := "Alpha"
	snapshotDay := time.Date(2026, time.March, 23, 0, 0, 0, 0, timeutil.Location())
	comparedDay := snapshotDay.AddDate(0, 0, -1)

	resp := buildSmartMoneySnapshotLeaderboard("pnl", snapshotDay, comparedDay, 20, []smartMoneyLeaderboardSnapshotInput{
		{
			Wallet: models.MonitoredWallet{
				Address: "0x00000000000000000000000000000000000000a1",
				ChainID: 56,
				Label:   &label,
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
				AddCount:             1,
				RemoveCount:          2,
				ActivePoolCount:      3,
				UnmatchedRemoveCount: 1,
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
				AddCount:        4,
				RemoveCount:     1,
				ActivePoolCount: 2,
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
	if got, want := first.Address, "0x00000000000000000000000000000000000000a1"; got != want {
		t.Fatalf("first address = %s, want %s", got, want)
	}
	if got, want := first.Rank, 1; got != want {
		t.Fatalf("first rank = %d, want %d", got, want)
	}
	if got, want := first.EstimatedRealizedPnLUSD, 35.0; got != want {
		t.Fatalf("first pnl = %.2f, want %.2f", got, want)
	}
	if got, want := first.YieldRate, 0.35; got != want {
		t.Fatalf("first yield = %.4f, want %.4f", got, want)
	}
	if got, want := first.ParticipationCount, 3; got != want {
		t.Fatalf("first participation = %d, want %d", got, want)
	}
	if !first.HasTransferIn || first.TransferInCount != 1 {
		t.Fatalf("first transfer in flag/count = %v/%d, want true/1", first.HasTransferIn, first.TransferInCount)
	}
	if got, want := first.TransferInUSD, 9.5; got != want {
		t.Fatalf("first transfer in usd = %.2f, want %.2f", got, want)
	}

	second := resp.List[1]
	if got, want := second.Address, "0x00000000000000000000000000000000000000b2"; got != want {
		t.Fatalf("second address = %s, want %s", got, want)
	}
	if got, want := second.EstimatedRealizedPnLUSD, 20.0; got != want {
		t.Fatalf("second pnl = %.2f, want %.2f", got, want)
	}
	if got, want := second.YieldRate, 0.2; got != want {
		t.Fatalf("second yield = %.4f, want %.4f", got, want)
	}
	if !second.HasTransferOut || second.TransferOutCount != 1 {
		t.Fatalf("second transfer out flag/count = %v/%d, want true/1", second.HasTransferOut, second.TransferOutCount)
	}
	if got, want := second.TransferOutUSD, 18.75; got != want {
		t.Fatalf("second transfer out usd = %.2f, want %.2f", got, want)
	}
}

func TestBuildSmartMoneySnapshotLeaderboard_ParticipationMetricRanksByDailyOps(t *testing.T) {
	timeutil.Init()

	snapshotDay := time.Date(2026, time.March, 23, 0, 0, 0, 0, timeutil.Location())
	comparedDay := snapshotDay.AddDate(0, 0, -1)

	resp := buildSmartMoneySnapshotLeaderboard("participation", snapshotDay, comparedDay, 20, []smartMoneyLeaderboardSnapshotInput{
		{
			Wallet: models.MonitoredWallet{
				Address: "0x00000000000000000000000000000000000000d4",
				ChainID: 56,
			},
			Current:  &models.SmartMoneyWalletDailySnapshot{TotalUSD: 150},
			Previous: &models.SmartMoneyWalletDailySnapshot{TotalUSD: 100},
			DailyStat: &models.SmartMoneyLPDailyStat{
				AddCount:    1,
				RemoveCount: 1,
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
				AddCount:    3,
				RemoveCount: 2,
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

func TestBuildSmartMoneySnapshotLeaderboard_FallsBackToSnapshotDeltaWithoutDailyStat(t *testing.T) {
	timeutil.Init()

	snapshotDay := time.Date(2026, time.March, 23, 0, 0, 0, 0, timeutil.Location())
	comparedDay := snapshotDay.AddDate(0, 0, -1)

	resp := buildSmartMoneySnapshotLeaderboard("pnl", snapshotDay, comparedDay, 20, []smartMoneyLeaderboardSnapshotInput{
		{
			Wallet: models.MonitoredWallet{
				Address: "0x00000000000000000000000000000000000000f6",
				ChainID: 56,
			},
			Current: &models.SmartMoneyWalletDailySnapshot{
				TotalUSD: 145,
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
}
