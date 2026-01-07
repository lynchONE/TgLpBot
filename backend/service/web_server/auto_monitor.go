package web_server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/strategy"
	userSvc "TgLpBot/service/user"
)

type autoMonitorRequest struct {
	InitData string `json:"initData"`
}

type autoMonitorConfig struct {
	VolumeDropPct       float64 `json:"volume_drop_pct"`
	VolumeDropPctLow    float64 `json:"volume_drop_pct_low"`
	PriceTxDropPct      float64 `json:"price_tx_drop_pct"`
	NoExitMinFeeRate5m  float64 `json:"no_exit_min_fee_rate_5m"`
	LowFeeRate5m        float64 `json:"low_fee_rate_5m"`
	EffectiveDefaultVol float64 `json:"effective_default_vol_drop_pct"`
}

type autoMonitorMetrics struct {
	At           *time.Time `json:"at,omitempty"`
	FeePct       float64    `json:"fee_pct"`
	FeeRate5mPct float64    `json:"fee_rate_5m_pct"`
	Fees5m       float64    `json:"fees_5m"`
	Volume5m     float64    `json:"volume_5m"`
	TVL          float64    `json:"tvl"`
	Price        float64    `json:"price"`
	Tx5m         uint64     `json:"tx_5m"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	LastSwapAt   *time.Time `json:"last_swap_at,omitempty"`
	Ok           bool       `json:"ok"`
}

type autoMonitorGuardVolume struct {
	Enabled          bool    `json:"enabled"`
	Blocked          bool    `json:"blocked"`
	BlockedReason    string  `json:"blocked_reason,omitempty"`
	Skip             bool    `json:"skip"`
	SkipReason       string  `json:"skip_reason,omitempty"`
	DropPct          float64 `json:"drop_pct"`
	Threshold        float64 `json:"threshold"`
	Hit              bool    `json:"hit"`
	FirstMark        bool    `json:"first_mark"`
	Armed            bool    `json:"armed"`
	LastVolume5m     float64 `json:"last_volume_5m"`
	ShouldExitNow    bool    `json:"should_exit_now"`
	OpenVolume5m     float64 `json:"open_volume_5m"`
	CurrentVolume5m  float64 `json:"current_volume_5m"`
	CurrentFeeRate5m float64 `json:"current_fee_rate_5m_pct"`
}

type autoMonitorGuardPriceTx struct {
	Enabled       bool    `json:"enabled"`
	Blocked       bool    `json:"blocked"`
	BlockedReason string  `json:"blocked_reason,omitempty"`
	DropPct       float64 `json:"drop_pct"`
	PriceHit      bool    `json:"price_hit"`
	TxHit         bool    `json:"tx_hit"`
	Hit           bool    `json:"hit"`
	FirstMark     bool    `json:"first_mark"`
	Armed         bool    `json:"armed"`
	ShouldExitNow bool    `json:"should_exit_now"`
	OpenPrice     float64 `json:"open_price"`
	CurrentPrice  float64 `json:"current_price"`
	OpenTx5m      uint64  `json:"open_tx_5m"`
	CurrentTx5m   uint64  `json:"current_tx_5m"`
}

type autoMonitorTask struct {
	TaskID      uint   `json:"task_id"`
	PoolVersion string `json:"pool_version"`
	PoolID      string `json:"pool_id"`
	Exchange    string `json:"exchange"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Paused      bool   `json:"paused"`

	ExitPendingAction string     `json:"exit_pending_action,omitempty"`
	ExitPendingReason string     `json:"exit_pending_reason,omitempty"`
	ExitRetryCount    int        `json:"exit_retry_count,omitempty"`
	ExitNextRetryAt   *time.Time `json:"exit_next_retry_at,omitempty"`
	ExitLastError     string     `json:"exit_last_error,omitempty"`
	ExitGiveUpAt      *time.Time `json:"exit_give_up_at,omitempty"`

	Open    autoMonitorMetrics `json:"open"`
	Current autoMonitorMetrics `json:"current"`

	GuardVolume  autoMonitorGuardVolume  `json:"guard_volume"`
	GuardPriceTx autoMonitorGuardPriceTx `json:"guard_price_tx"`
}

type autoMonitorResponse struct {
	UpdatedAt time.Time         `json:"updated_at"`
	Chain     string            `json:"chain"`
	Config    autoMonitorConfig `json:"config"`
	Tasks     []autoMonitorTask `json:"tasks"`
}

func (s *Server) handleAutoMonitor(w http.ResponseWriter, r *http.Request) {
	initData := ""
	switch r.Method {
	case http.MethodGet:
		initData = strings.TrimSpace(r.URL.Query().Get("initData"))
		if initData == "" {
			initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
		}
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var req autoMonitorRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if config.AppConfig == nil {
		http.Error(w, "config not loaded", http.StatusInternalServerError)
		return
	}
	if database.DB == nil {
		http.Error(w, "database not initialized", http.StatusInternalServerError)
		return
	}

	parsed, err := ParseTelegramWebAppInitData(initData, config.AppConfig.TelegramBotToken)
	if err != nil {
		if errors.Is(err, ErrMissingInitData) {
			http.Error(w, "missing initData", http.StatusBadRequest)
		} else {
			http.Error(w, "invalid initData", http.StatusUnauthorized)
		}
		return
	}

	userService := userSvc.NewUserService()
	user, err := userService.GetOrCreateUser(
		parsed.User.ID,
		parsed.User.Username,
		parsed.User.FirstName,
		parsed.User.LastName,
		parsed.User.LanguageCode,
	)
	if err != nil {
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return
	}

	chain := strings.ToLower(strings.TrimSpace(config.AppConfig.AutoLPChain))
	if chain == "" {
		chain = "bsc"
	}

	volumeDropPct := config.AppConfig.AutoLPGuardVolumeDropPercent
	if volumeDropPct > 1 && volumeDropPct <= 100 {
		volumeDropPct = volumeDropPct / 100
	}
	if volumeDropPct <= 0 || volumeDropPct >= 1 {
		volumeDropPct = 0.30
	}

	volumeDropPctLow := config.AppConfig.AutoLPGuardVolumeDropPercentLow
	if volumeDropPctLow > 1 && volumeDropPctLow <= 100 {
		volumeDropPctLow = volumeDropPctLow / 100
	}
	if volumeDropPctLow <= 0 || volumeDropPctLow >= 1 {
		volumeDropPctLow = 0
	}

	noExitMinFeeRate5m := config.AppConfig.AutoLPGuardNoExitMinFeeRate5m
	if noExitMinFeeRate5m < 0 {
		noExitMinFeeRate5m = 0
	}
	lowFeeRate5m := config.AppConfig.AutoLPGuardLowFeeRate5m
	if lowFeeRate5m < 0 {
		lowFeeRate5m = 0
	}

	priceTxDropPct := config.AppConfig.AutoLPGuardPriceTxDropPercent
	if priceTxDropPct > 1 && priceTxDropPct <= 100 {
		priceTxDropPct = priceTxDropPct / 100
	}
	if priceTxDropPct <= 0 || priceTxDropPct >= 1 {
		priceTxDropPct = 0.10
	}

	var tasks []models.StrategyTask
	if err := database.DB.
		Where("user_id = ? AND is_auto = ? AND status IN ?", user.ID, true, []models.StrategyStatus{
			models.StrategyStatusRunning,
			models.StrategyStatusWaiting,
		}).
		Order("updated_at DESC").
		Find(&tasks).Error; err != nil {
		http.Error(w, "query tasks failed", http.StatusInternalServerError)
		return
	}

	type poolMRow struct {
		ProtocolVersion   string    `ch:"protocol_version"`
		PoolAddress       string    `ch:"pool_address"`
		FeePercentage     float64   `ch:"fee_percentage"`
		TransactionCount  uint32    `ch:"transaction_count"`
		TotalFees         float64   `ch:"total_fees"`
		TotalVolume       float64   `ch:"total_volume"`
		CurrentPoolValue  float64   `ch:"current_pool_value"`
		CurrentTokenPrice float64   `ch:"current_token_price"`
		UpdatedAt         time.Time `ch:"updated_at"`
		LastSwapAt        time.Time `ch:"last_swap_at"`
	}

	metricsMap := map[string]poolMRow{}
	if s.ClickHouse != nil && s.ClickHouse.Conn != nil && len(tasks) > 0 {
		seenPools := make(map[string]struct{}, len(tasks))
		poolIDs := make([]string, 0, len(tasks))
		for i := range tasks {
			pool := strings.ToLower(strings.TrimSpace(tasks[i].PoolId))
			if pool == "" {
				continue
			}
			if _, ok := seenPools[pool]; ok {
				continue
			}
			seenPools[pool] = struct{}{}
			poolIDs = append(poolIDs, pool)
		}

		if len(poolIDs) > 0 {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()

			q := `
				SELECT
					protocol_version,
					pool_address,
					argMax(fee_percentage, ts) AS fee_percentage,
					argMax(transaction_count, ts) AS transaction_count,
					argMax(total_fees, ts) AS total_fees,
					argMax(total_volume, ts) AS total_volume,
					argMax(current_pool_value, ts) AS current_pool_value,
					argMax(current_token_price, ts) AS current_token_price,
					argMax(last_swap_at, ts) AS last_swap_at,
					max(ts) AS updated_at
				FROM poolm_top_fees_raw
				WHERE chain = ?
				  AND timeframe_minutes = 5
				  AND pool_address IN (?)
				GROUP BY protocol_version, pool_address
			`

			var rows []poolMRow
			if err := s.ClickHouse.Conn.Select(ctx, &rows, q, chain, poolIDs); err == nil {
				for _, row := range rows {
					proto := strings.ToLower(strings.TrimSpace(row.ProtocolVersion))
					addr := strings.ToLower(strings.TrimSpace(row.PoolAddress))
					if proto == "" || addr == "" {
						continue
					}
					metricsMap[proto+":"+addr] = row
				}
			}
		}
	}

	toTitle := func(t models.StrategyTask) string {
		s0 := strings.TrimSpace(t.Token0Symbol)
		s1 := strings.TrimSpace(t.Token1Symbol)
		if s0 == "" && s1 == "" {
			return ""
		}
		if s0 != "" && s1 != "" {
			return s0 + "/" + s1
		}
		if s0 != "" {
			return s0
		}
		return s1
	}

	out := make([]autoMonitorTask, 0, len(tasks))
	for i := range tasks {
		task := tasks[i]
		proto := strings.ToLower(strings.TrimSpace(task.PoolVersion))
		pool := strings.ToLower(strings.TrimSpace(task.PoolId))
		key := proto + ":" + pool

		openTx := uint64(0)
		if task.GuardOpenTxCount5m > 0 {
			openTx = uint64(task.GuardOpenTxCount5m)
		}

		open := autoMonitorMetrics{
			At:           task.GuardOpenMetricsAt,
			FeePct:       task.GuardOpenFeePercentage,
			FeeRate5mPct: task.GuardOpenFeeRate5mPct,
			Fees5m:       task.GuardOpenTotalFees5m,
			Volume5m:     task.GuardOpenVolume5m,
			TVL:          task.GuardOpenTVLUSD,
			Price:        task.GuardOpenPrice,
			Tx5m:         openTx,
			Ok:           task.GuardOpenVolume5m > 0,
		}

		current := autoMonitorMetrics{Ok: false}
		curRow, okCur := metricsMap[key]
		if okCur {
			feeRatePct := 0.0
			if curRow.CurrentPoolValue > 0 {
				feeRatePct = (curRow.TotalFees / curRow.CurrentPoolValue) * 100
			}
			updatedAt := curRow.UpdatedAt
			lastSwapAt := curRow.LastSwapAt
			current = autoMonitorMetrics{
				FeePct:       curRow.FeePercentage,
				FeeRate5mPct: feeRatePct,
				Fees5m:       curRow.TotalFees,
				Volume5m:     curRow.TotalVolume,
				TVL:          curRow.CurrentPoolValue,
				Price:        curRow.CurrentTokenPrice,
				Tx5m:         uint64(curRow.TransactionCount),
				UpdatedAt:    &updatedAt,
				LastSwapAt:   &lastSwapAt,
				Ok:           true,
			}
		}

		guardBlocked := false
		guardBlockedReason := ""
		if task.Paused {
			guardBlocked = true
			guardBlockedReason = "task paused"
		} else if task.ExitGiveUpAt != nil {
			guardBlocked = true
			guardBlockedReason = "exit give up"
		} else if pending := strings.TrimSpace(task.ExitPendingAction); pending != "" && pending != strategy.ExitActionRebalance {
			guardBlocked = true
			guardBlockedReason = "exit pending: " + pending
		}

		// Volume guard status.
		volGuard := autoMonitorGuardVolume{
			Enabled:         current.Ok && open.Volume5m > 0 && current.Volume5m > 0,
			Blocked:         guardBlocked,
			BlockedReason:   guardBlockedReason,
			DropPct:         volumeDropPct,
			OpenVolume5m:    open.Volume5m,
			CurrentVolume5m: current.Volume5m,
			Armed:           task.GuardVolumeDropArmed,
			LastVolume5m:    task.GuardVolumeDropLastVolume5m,
		}

		currentFeeRate := current.FeeRate5mPct
		volGuard.CurrentFeeRate5m = currentFeeRate
		effectiveDropPct := volumeDropPct
		skipVolumeExit := false
		if noExitMinFeeRate5m > 0 || lowFeeRate5m > 0 {
			if noExitMinFeeRate5m > 0 && currentFeeRate > noExitMinFeeRate5m {
				skipVolumeExit = true
			} else if lowFeeRate5m > 0 && currentFeeRate < lowFeeRate5m && volumeDropPctLow > 0 {
				effectiveDropPct = volumeDropPctLow
			}
		}
		volGuard.DropPct = effectiveDropPct
		volGuard.Skip = skipVolumeExit
		if skipVolumeExit {
			volGuard.SkipReason = "fee rate too high"
		}

		if volGuard.Enabled && !volGuard.Blocked && !volGuard.Skip && effectiveDropPct > 0 && effectiveDropPct < 1 {
			volGuard.Threshold = open.Volume5m * (1.0 - effectiveDropPct)
			volGuard.Hit = current.Volume5m <= volGuard.Threshold
			volGuard.FirstMark = volGuard.Hit && !task.GuardVolumeDropArmed
			if volGuard.Hit && task.GuardVolumeDropArmed && task.GuardVolumeDropLastVolume5m > 0 && current.Volume5m < task.GuardVolumeDropLastVolume5m {
				volGuard.ShouldExitNow = true
			}
		}

		// Price+Tx guard status.
		priceTx := autoMonitorGuardPriceTx{
			Enabled:       current.Ok && open.Price > 0 && openTx > 0 && current.Price > 0 && current.Tx5m > 0,
			Blocked:       guardBlocked,
			BlockedReason: guardBlockedReason,
			DropPct:       priceTxDropPct,
			OpenPrice:     open.Price,
			CurrentPrice:  current.Price,
			OpenTx5m:      openTx,
			CurrentTx5m:   current.Tx5m,
			Armed:         task.GuardPriceTxDropArmed,
		}
		if priceTx.Enabled && !priceTx.Blocked && priceTxDropPct > 0 && priceTxDropPct < 1 {
			priceTx.PriceHit = current.Price <= open.Price*(1.0-priceTxDropPct)
			priceTx.TxHit = float64(current.Tx5m) <= float64(openTx)*(1.0-priceTxDropPct)
			priceTx.Hit = priceTx.PriceHit && priceTx.TxHit
			priceTx.FirstMark = priceTx.Hit && !task.GuardPriceTxDropArmed
			priceTx.ShouldExitNow = priceTx.Hit && task.GuardPriceTxDropArmed
		}

		out = append(out, autoMonitorTask{
			TaskID:      task.ID,
			PoolVersion: task.PoolVersion,
			PoolID:      task.PoolId,
			Exchange:    task.Exchange,
			Title:       toTitle(task),
			Status:      string(task.Status),
			Paused:      task.Paused,

			ExitPendingAction: strings.TrimSpace(task.ExitPendingAction),
			ExitPendingReason: strings.TrimSpace(task.ExitPendingReason),
			ExitRetryCount:    task.ExitRetryCount,
			ExitNextRetryAt:   task.ExitNextRetryAt,
			ExitLastError:     strings.TrimSpace(task.ExitLastError),
			ExitGiveUpAt:      task.ExitGiveUpAt,

			Open:         open,
			Current:      current,
			GuardVolume:  volGuard,
			GuardPriceTx: priceTx,
		})
	}

	resp := autoMonitorResponse{
		UpdatedAt: time.Now(),
		Chain:     chain,
		Config: autoMonitorConfig{
			VolumeDropPct:       volumeDropPct,
			VolumeDropPctLow:    volumeDropPctLow,
			PriceTxDropPct:      priceTxDropPct,
			NoExitMinFeeRate5m:  noExitMinFeeRate5m,
			LowFeeRate5m:        lowFeeRate5m,
			EffectiveDefaultVol: volumeDropPct,
		},
		Tasks: out,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
