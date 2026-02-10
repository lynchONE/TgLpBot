package smart_money_follow

import (
	"context"
	crand "crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"TgLpBot/base/blockchain"
	"TgLpBot/base/clickhouse"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/pool"
	"TgLpBot/service/smart_lp"
	"TgLpBot/service/strategy"
	userSvc "TgLpBot/service/user"

	"github.com/ethereum/go-ethereum/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	jobStatusPending    = "pending"
	jobStatusProcessing = "processing"
	jobStatusDone       = "done"
	jobStatusFailed     = "failed"
	jobStatusCanceled   = "canceled"

	followTaskStatusActive  = "active"
	followTaskStatusClosing = "closing"
	followTaskStatusClosed  = "closed"
)

type SmartMoneyFollowService struct {
	ch      *clickhouse.ClickHouseService
	smartLP *smart_lp.SmartLPService
	poolSvc *pool.PoolService
	liqSvc  *liquidity.LiquidityService
	cfgSvc  *userSvc.GlobalConfigService
	access  *userSvc.AccessService

	stopChan chan struct{}
	ticker   *time.Ticker

	notifier         func(userID uint, message string)
	taskCardNotifier func(userID uint, taskID uint)
}

func NewSmartMoneyFollowService(ch *clickhouse.ClickHouseService) *SmartMoneyFollowService {
	return &SmartMoneyFollowService{
		ch:       ch,
		smartLP:  smart_lp.NewSmartLPService(ch),
		poolSvc:  pool.NewPoolService(),
		liqSvc:   liquidity.NewLiquidityService(),
		cfgSvc:   userSvc.NewGlobalConfigService(),
		access:   userSvc.NewAccessService(),
		stopChan: make(chan struct{}),
		ticker:   time.NewTicker(5 * time.Second),
	}
}

func (s *SmartMoneyFollowService) SetNotifier(fn func(userID uint, message string)) {
	s.notifier = fn
}

func (s *SmartMoneyFollowService) SetTaskCardNotifier(fn func(userID uint, taskID uint)) {
	s.taskCardNotifier = fn
}

func (s *SmartMoneyFollowService) Start() {
	go s.runLoop()
}

func (s *SmartMoneyFollowService) Stop() {
	if s == nil {
		return
	}
	select {
	case <-s.stopChan:
		// already stopped
	default:
		close(s.stopChan)
	}
	if s.ticker != nil {
		s.ticker.Stop()
	}
}

func (s *SmartMoneyFollowService) runLoop() {
	log.Println("[SmartMoneyFollow] service started")
	s.runOnce()
	for {
		select {
		case <-s.stopChan:
			log.Println("[SmartMoneyFollow] service stopped")
			return
		case <-s.ticker.C:
			s.runOnce()
		}
	}
}

func (s *SmartMoneyFollowService) runOnce() {
	if database.DB == nil {
		return
	}
	now := time.Now()
	s.scheduleJobs(now)
	s.processDueJobs(now)
}

func normalizeChain(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "bsc"
	}
	return v
}

func normalizeWallet(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" || !common.IsHexAddress(v) {
		return "", false
	}
	return strings.ToLower(common.HexToAddress(v).Hex()), true
}

func clampDelay(v int) int {
	if v < 0 {
		return 0
	}
	if v > 60 {
		return 60
	}
	return v
}

func cryptoRandIntn(n int) int {
	if n <= 1 {
		return 0
	}
x:
	r, err := crand.Int(crand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	if r.Sign() < 0 {
		goto x
	}
	return int(r.Int64())
}

func randomDelaySeconds(min int, max int) int {
	min = clampDelay(min)
	max = clampDelay(max)
	if max < min {
		max, min = min, max
	}
	if max == min {
		return min
	}
	return min + cryptoRandIntn(max-min+1)
}

func (s *SmartMoneyFollowService) scheduleJobs(now time.Time) {
	var cfgs []models.SmartMoneyFollowConfig
	if err := database.DB.Where("enabled = ?", true).Find(&cfgs).Error; err != nil {
		log.Printf("[SmartMoneyFollow] load configs failed: %v", err)
		return
	}
	if len(cfgs) == 0 {
		return
	}

	for _, cfg := range cfgs {
		s.scheduleJobsForConfig(cfg, now)
	}
}

func (s *SmartMoneyFollowService) maxWalletEventSeq(ctx context.Context, chain string, wallet string) (uint64, error) {
	if s == nil || s.ch == nil || s.ch.Conn == nil {
		return 0, fmt.Errorf("clickhouse not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	chain = normalizeChain(chain)
	wallet, ok := normalizeWallet(wallet)
	if !ok {
		return 0, fmt.Errorf("invalid wallet")
	}

	var maxSeq uint64
	q := `
		SELECT max(event_seq)
		FROM smart_lp_events
		WHERE lowerUTF8(chain) = ?
			AND wallet_address = ?
	`
	if err := s.ch.Conn.QueryRow(ctx, q, chain, wallet).Scan(&maxSeq); err != nil {
		return 0, err
	}
	return maxSeq, nil
}

func (s *SmartMoneyFollowService) scheduleJobsForConfig(cfg models.SmartMoneyFollowConfig, now time.Time) {
	chain := normalizeChain(cfg.Chain)
	wallet, ok := normalizeWallet(cfg.WalletAddress)
	if !ok || cfg.UserID == 0 {
		return
	}

	if s.smartLP == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var events []smart_lp.SmartLPEvent
	var err error
	if cfg.LastEventSeq == 0 {
		// No-backfill bootstrapping: only pick up events after enable time.
		if cfg.LastEnabledAt != nil && !cfg.LastEnabledAt.IsZero() {
			events, err = s.smartLP.GetWalletEventsSinceTime(ctx, chain, wallet, *cfg.LastEnabledAt, 120)
		} else {
			// Fallback for legacy rows: initialize cursor to "now" by advancing to max(event_seq).
			maxSeq, maxErr := s.maxWalletEventSeq(ctx, chain, wallet)
			if maxErr == nil && maxSeq > 0 {
				_ = database.DB.Model(&models.SmartMoneyFollowConfig{}).
					Where("id = ? AND last_event_seq = 0", cfg.ID).
					Update("last_event_seq", maxSeq).Error
			}
			return
		}
	} else {
		events, err = s.smartLP.GetWalletEventsSince(ctx, chain, wallet, cfg.LastEventSeq, 120)
	}
	if err != nil {
		return
	}
	if len(events) == 0 {
		return
	}

	maxSeq := cfg.LastEventSeq
	for _, ev := range events {
		if ev.EventSeq > maxSeq {
			maxSeq = ev.EventSeq
		}

		pv := strings.ToLower(strings.TrimSpace(ev.PoolVersion))
		pid := strings.ToLower(strings.TrimSpace(ev.PoolID))
		act := strings.ToLower(strings.TrimSpace(ev.Action))
		if pv == "" || pid == "" {
			continue
		}
		if act != "add" && act != "remove" {
			continue
		}

		delay := randomDelaySeconds(cfg.DelayMinSeconds, cfg.DelayMaxSeconds)
		execAt := now.Add(time.Duration(delay) * time.Second)
		if !ev.Ts.IsZero() {
			execAt = ev.Ts.Add(time.Duration(delay) * time.Second)
			if execAt.Before(now) {
				execAt = now
			}
		}

		job := models.SmartMoneyFollowJob{
			UserID:        cfg.UserID,
			Chain:         chain,
			WalletAddress: wallet,
			EventSeq:      ev.EventSeq,
			PoolVersion:   pv,
			PoolID:        pid,
			Action:        act,
			TickLower:     ev.TickLower,
			TickUpper:     ev.TickUpper,
			ExecuteAt:     execAt,
			Status:        jobStatusPending,
		}
		// Create only; never overwrite existing job statuses.
		_ = database.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&job).Error
	}

	_ = database.DB.Model(&models.SmartMoneyFollowConfig{}).
		Where("id = ? AND last_event_seq < ?", cfg.ID, maxSeq).
		Update("last_event_seq", maxSeq).Error
}

func (s *SmartMoneyFollowService) processDueJobs(now time.Time) {
	var jobs []models.SmartMoneyFollowJob
	if err := database.DB.
		Where("status = ? AND execute_at <= ?", jobStatusPending, now).
		Order("execute_at ASC").
		Limit(25).
		Find(&jobs).Error; err != nil {
		log.Printf("[SmartMoneyFollow] load jobs failed: %v", err)
		return
	}
	for i := range jobs {
		job := jobs[i]
		s.processJob(job)
	}
}

func (s *SmartMoneyFollowService) claimJob(jobID uint) bool {
	if jobID == 0 {
		return false
	}
	res := database.DB.Model(&models.SmartMoneyFollowJob{}).
		Where("id = ? AND status = ?", jobID, jobStatusPending).
		Update("status", jobStatusProcessing)
	return res.Error == nil && res.RowsAffected > 0
}

func (s *SmartMoneyFollowService) finishJob(jobID uint, status string, taskID uint, errMsg string) {
	if jobID == 0 {
		return
	}
	updates := map[string]interface{}{
		"status":        status,
		"task_id":       taskID,
		"error_message": strings.TrimSpace(errMsg),
	}
	_ = database.DB.Model(&models.SmartMoneyFollowJob{}).Where("id = ?", jobID).Updates(updates).Error
}

func (s *SmartMoneyFollowService) loadConfig(userID uint, chain string, wallet string) (*models.SmartMoneyFollowConfig, error) {
	var cfg models.SmartMoneyFollowConfig
	if err := database.DB.Where("user_id = ? AND chain = ? AND wallet_address = ?", userID, normalizeChain(chain), strings.ToLower(strings.TrimSpace(wallet))).First(&cfg).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *SmartMoneyFollowService) processJob(job models.SmartMoneyFollowJob) {
	if job.ID == 0 {
		return
	}
	if !s.claimJob(job.ID) {
		return
	}

	chain := normalizeChain(job.Chain)
	wallet, ok := normalizeWallet(job.WalletAddress)
	if !ok {
		s.finishJob(job.ID, jobStatusFailed, 0, "invalid wallet_address")
		return
	}

	cfg, err := s.loadConfig(job.UserID, chain, wallet)
	if err != nil {
		s.finishJob(job.ID, jobStatusCanceled, 0, "follow config not found")
		return
	}
	if cfg == nil || !cfg.Enabled {
		s.finishJob(job.ID, jobStatusCanceled, 0, "follow disabled")
		return
	}

	if s.access != nil {
		check, err := s.access.CheckUserAccess(job.UserID, time.Now())
		if err != nil {
			s.finishJob(job.ID, jobStatusFailed, 0, "access check failed")
			return
		}
		if !check.Allowed {
			s.finishJob(job.ID, jobStatusCanceled, 0, strings.TrimSpace(check.Reason))
			return
		}
		if !check.IsAdmin {
			if check.Access == nil || !check.Access.SmartMoneyEnabled {
				s.finishJob(job.ID, jobStatusCanceled, 0, "Smart Money permission required")
				return
			}
		}
	}

	act := strings.ToLower(strings.TrimSpace(job.Action))
	switch act {
	case "add":
		taskID, msg, err := s.executeAdd(job, cfg)
		if err != nil {
			s.finishJob(job.ID, jobStatusFailed, taskID, err.Error())
			return
		}
		s.finishJob(job.ID, jobStatusDone, taskID, msg)
		if taskID != 0 && s.taskCardNotifier != nil {
			go s.taskCardNotifier(job.UserID, taskID)
		}
	case "remove":
		msg, err := s.executeRemove(job, cfg)
		if err != nil {
			s.finishJob(job.ID, jobStatusFailed, 0, err.Error())
			return
		}
		s.finishJob(job.ID, jobStatusDone, 0, msg)
	default:
		s.finishJob(job.ID, jobStatusFailed, 0, "unsupported action")
	}
}

func (s *SmartMoneyFollowService) notify(userID uint, message string) {
	if s == nil || s.notifier == nil {
		return
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		return
	}
	go s.notifier(userID, msg)
}

func (s *SmartMoneyFollowService) followUsedAmountUSDT(userID uint, chain string, wallet string) (float64, error) {
	type row struct {
		Total float64 `gorm:"column:total"`
	}
	var out row
	err := database.DB.Table("smart_money_follow_tasks AS ft").
		Joins("JOIN strategy_tasks st ON st.id = ft.task_id").
		Where("ft.user_id = ? AND ft.chain = ? AND ft.wallet_address = ?", userID, chain, wallet).
		Where("ft.status IN ?", []string{followTaskStatusActive, followTaskStatusClosing}).
		Where("st.status <> ?", models.StrategyStatusStopped).
		Select("COALESCE(SUM(st.amount_usdt), 0) AS total").
		Scan(&out).Error
	if err != nil {
		return 0, err
	}
	if out.Total < 0 {
		return 0, nil
	}
	return out.Total, nil
}

func (s *SmartMoneyFollowService) loadFollowTask(userID uint, chain string, wallet string, poolVersion string, poolID string) (*models.SmartMoneyFollowTask, error) {
	var ft models.SmartMoneyFollowTask
	err := database.DB.Where(
		"user_id = ? AND chain = ? AND wallet_address = ? AND pool_version = ? AND pool_id = ?",
		userID, chain, wallet, poolVersion, poolID,
	).First(&ft).Error
	if err != nil {
		return nil, err
	}
	return &ft, nil
}

func (s *SmartMoneyFollowService) ensureFollowTaskClosedIfStopped(ft *models.SmartMoneyFollowTask) {
	if ft == nil || ft.ID == 0 || ft.TaskID == 0 {
		return
	}
	var task models.StrategyTask
	err := database.DB.Where("id = ? AND user_id = ?", ft.TaskID, ft.UserID).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = database.DB.Model(&models.SmartMoneyFollowTask{}).Where("id = ?", ft.ID).Update("status", followTaskStatusClosed).Error
		}
		return
	}
	if task.Status == models.StrategyStatusStopped {
		_ = database.DB.Model(&models.SmartMoneyFollowTask{}).Where("id = ?", ft.ID).Update("status", followTaskStatusClosed).Error
	}
}

func (s *SmartMoneyFollowService) executeAdd(job models.SmartMoneyFollowJob, cfg *models.SmartMoneyFollowConfig) (uint, string, error) {
	chain := normalizeChain(job.Chain)
	wallet, _ := normalizeWallet(job.WalletAddress)
	poolVersion := strings.ToLower(strings.TrimSpace(job.PoolVersion))
	poolID := strings.ToLower(strings.TrimSpace(job.PoolID))

	if cfg == nil {
		return 0, "", fmt.Errorf("config is nil")
	}
	perTrade := cfg.PerTradeAmountUSDT
	if perTrade <= 0 {
		return 0, "", fmt.Errorf("per_trade_amount_usdt not set")
	}

	// If already active for this pool, ignore the add.
	if ft, err := s.loadFollowTask(job.UserID, chain, wallet, poolVersion, poolID); err == nil && ft != nil {
		s.ensureFollowTaskClosedIfStopped(ft)
		if ft.Status == followTaskStatusActive || ft.Status == followTaskStatusClosing {
			return 0, "already active", nil
		}
	}

	if cfg.MaxTotalAmountUSDT > 0 {
		used, err := s.followUsedAmountUSDT(job.UserID, chain, wallet)
		if err != nil {
			return 0, "", fmt.Errorf("failed to compute used amount")
		}
		if used+perTrade > cfg.MaxTotalAmountUSDT {
			return 0, "", fmt.Errorf("budget exceeded: used=%.4f + per=%.4f > max=%.4f", used, perTrade, cfg.MaxTotalAmountUSDT)
		}
	}

	if s.poolSvc == nil || s.liqSvc == nil || s.cfgSvc == nil {
		return 0, "", fmt.Errorf("service not initialized")
	}

	var info *pool.PoolInfo
	var err error
	switch poolVersion {
	case "v4":
		info, err = s.poolSvc.GetV4PoolInfo(poolID)
	default:
		info, err = s.poolSvc.GetPoolInfo(poolID)
	}
	if err != nil || info == nil {
		return 0, "", fmt.Errorf("failed to load pool info: %v", err)
	}

	userCfg, err := s.cfgSvc.GetOrCreate(job.UserID)
	if err != nil {
		return 0, "", fmt.Errorf("failed to load user config")
	}

	currentTick := 0
	switch poolVersion {
	case "v4":
		if config.AppConfig == nil || !common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
			return 0, "", fmt.Errorf("UNISWAP_V4_POOL_MANAGER_ADDRESS not set")
		}
		if config.AppConfig == nil || !common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) {
			return 0, "", fmt.Errorf("UNISWAP_V4_STATE_VIEW_ADDRESS not set")
		}
		poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
		stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
		currentTick, err = blockchain.GetUniswapV4PoolCurrentTickViaStateView(stateView, poolManager, poolID)
	default:
		if !common.IsHexAddress(poolID) {
			return 0, "", fmt.Errorf("invalid pool id")
		}
		currentTick, err = blockchain.GetV3PoolCurrentTick(common.HexToAddress(poolID))
	}
	if err != nil {
		return 0, "", fmt.Errorf("failed to read current tick")
	}

	tc := pool.NewTickCalculator()
	tickLower := job.TickLower
	tickUpper := job.TickUpper
	if tickLower > tickUpper {
		tickLower, tickUpper = tickUpper, tickLower
	}
	if err := tc.ValidateTickRange(tickLower, tickUpper, info.TickSpacing); err != nil {
		return 0, "", fmt.Errorf("invalid tick range from target wallet: tick_lower=%d tick_upper=%d tick_spacing=%d err=%v", tickLower, tickUpper, info.TickSpacing, err)
	}

	effLowerPct, effUpperPct := tc.CalculatePercentagesFromTicks(currentTick, tickLower, tickUpper)
	rangePct := (effLowerPct + effUpperPct) / 2.0

	hooksAddr := strings.TrimSpace(info.HooksAddress)
	if hooksAddr == "" || !common.IsHexAddress(hooksAddr) {
		hooksAddr = "0x0000000000000000000000000000000000000000"
	}

	allowEntrySwap := false
	if config.AppConfig != nil && config.AppConfig.AutoLPAllowEntrySwap {
		allowEntrySwap = true
	}

	task := &models.StrategyTask{
		UserID:               job.UserID,
		PoolId:               poolID,
		PoolVersion:          poolVersion,
		Exchange:             strings.TrimSpace(info.Exchange),
		Token0Symbol:         strings.TrimSpace(info.Token0Symbol),
		Token1Symbol:         strings.TrimSpace(info.Token1Symbol),
		Token0Address:        strings.TrimSpace(info.Token0),
		Token1Address:        strings.TrimSpace(info.Token1),
		HooksAddress:         hooksAddr,
		Fee:                  info.Fee,
		TickSpacing:          info.TickSpacing,
		TickLower:            tickLower,
		TickUpper:            tickUpper,
		RangePercentage:      rangePct,
		RangeLowerPercentage: effLowerPct,
		RangeUpperPercentage: effUpperPct,
		AmountUSDT:           perTrade,
		CurrentLiquidity:     "0",
		IsFollow:             true,
		ReopenDelaySeconds:   userCfg.RebalanceTimeout,
		SlippageTolerance:    userCfg.SlippageTolerance,
		AutoReinvest:         userCfg.AutoReinvest,
		ResidualTolerance:    userCfg.ResidualTolerance,
		AllowEntrySwap:       allowEntrySwap,
		StopLossEnabled:      userCfg.StopLossEnabled,
		StopLossDelaySeconds: userCfg.StopLossDelaySeconds,
		Status:               models.StrategyStatusRunning,
		LastCheckTime:        time.Now(),
	}

	if err := database.DB.Create(task).Error; err != nil {
		return 0, "", fmt.Errorf("failed to create task")
	}

	enterRes, err := s.liqSvc.EnterTaskFromUSDT(job.UserID, task)
	if err != nil {
		_ = database.DB.Model(task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": err.Error(),
		}).Error
		var swapErr *liquidity.EntrySwapRequiredError
		if errors.As(err, &swapErr) {
			return task.ID, "", fmt.Errorf("entry swap required")
		}
		return task.ID, "", fmt.Errorf("enter failed: %v", err)
	}

	updates := map[string]interface{}{
		"current_liquidity":      enterRes.CurrentLiquidity,
		"exit_liquidity_removed": false,
		"error_message":          "",
		"status":                 models.StrategyStatusRunning,
	}

	if v3TokenId := strings.TrimSpace(enterRes.V3TokenID); v3TokenId != "" && v3TokenId != "0" {
		updates["v3_position_manager_address"] = enterRes.V3PositionManagerAddress
		updates["v3_token_id"] = enterRes.V3TokenID
	}
	if v4TokenId := strings.TrimSpace(enterRes.V4TokenID); v4TokenId != "" && v4TokenId != "0" {
		updates["v4_token_id"] = enterRes.V4TokenID
	}

	if err := database.DB.Model(task).Updates(updates).Error; err != nil {
		return task.ID, "", fmt.Errorf("failed to update task")
	}

	ftUpdates := map[string]interface{}{
		"user_id":            job.UserID,
		"chain":              chain,
		"wallet_address":     wallet,
		"pool_version":       poolVersion,
		"pool_id":            poolID,
		"task_id":            task.ID,
		"status":             followTaskStatusActive,
		"last_add_event_seq": job.EventSeq,
	}
	_ = database.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "chain"},
			{Name: "wallet_address"},
			{Name: "pool_version"},
			{Name: "pool_id"},
		},
		DoUpdates: clause.Assignments(ftUpdates),
	}).Create(&models.SmartMoneyFollowTask{
		UserID:             job.UserID,
		Chain:              chain,
		WalletAddress:      wallet,
		PoolVersion:        poolVersion,
		PoolID:             poolID,
		TaskID:             task.ID,
		Status:             followTaskStatusActive,
		LastAddEventSeq:    job.EventSeq,
		LastRemoveEventSeq: 0,
	}).Error

	s.notify(job.UserID, fmt.Sprintf("🔁 跟单开仓: %s %s %s amount=%.2f", shortHex(wallet), poolVersion, shortPool(poolID), perTrade))
	return task.ID, "opened", nil
}

func shortHex(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 12 {
		return v
	}
	return v[:6] + "..." + v[len(v)-4:]
}

func shortPool(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 18 {
		return v
	}
	return v[:10] + "..." + v[len(v)-6:]
}

func (s *SmartMoneyFollowService) executeRemove(job models.SmartMoneyFollowJob, cfg *models.SmartMoneyFollowConfig) (string, error) {
	chain := normalizeChain(job.Chain)
	wallet, _ := normalizeWallet(job.WalletAddress)
	poolVersion := strings.ToLower(strings.TrimSpace(job.PoolVersion))
	poolID := strings.ToLower(strings.TrimSpace(job.PoolID))

	ft, err := s.loadFollowTask(job.UserID, chain, wallet, poolVersion, poolID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "no follow task", nil
		}
		return "", fmt.Errorf("load follow task failed")
	}
	if ft == nil || ft.TaskID == 0 {
		return "no follow task", nil
	}

	var task models.StrategyTask
	err = database.DB.Where("id = ? AND user_id = ?", ft.TaskID, job.UserID).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = database.DB.Model(&models.SmartMoneyFollowTask{}).Where("id = ?", ft.ID).Update("status", followTaskStatusClosed).Error
			return "task not found", nil
		}
		return "", fmt.Errorf("load task failed")
	}
	if task.Status == models.StrategyStatusStopped {
		_ = database.DB.Model(&models.SmartMoneyFollowTask{}).Where("id = ?", ft.ID).Update("status", followTaskStatusClosed).Error
		return "already stopped", nil
	}

	if strings.TrimSpace(task.ExitPendingAction) != "" {
		_ = database.DB.Model(&models.SmartMoneyFollowTask{}).Where("id = ?", ft.ID).Updates(map[string]interface{}{
			"status":                followTaskStatusClosing,
			"last_remove_event_seq": job.EventSeq,
		}).Error
		return "exit already pending", nil
	}

	updates := map[string]interface{}{
		"exit_pending_action":     strategy.ExitActionManualStop,
		"exit_pending_reason":     "🔁 跟单撤出",
		"exit_retry_count":        0,
		"exit_next_retry_at":      nil,
		"exit_last_error":         "",
		"exit_give_up_at":         nil,
		"rebalance_pending":       false,
		"rebalance_retry_count":   0,
		"rebalance_next_retry_at": nil,
		"rebalance_last_error":    "",
		"error_message":           "",
	}
	if err := database.DB.Model(&models.StrategyTask{}).
		Where("id = ? AND user_id = ?", task.ID, job.UserID).
		Updates(updates).Error; err != nil {
		return "", fmt.Errorf("failed to request exit")
	}

	_ = database.DB.Model(&models.SmartMoneyFollowTask{}).Where("id = ?", ft.ID).Updates(map[string]interface{}{
		"status":                followTaskStatusClosing,
		"last_remove_event_seq": job.EventSeq,
	}).Error

	s.notify(job.UserID, fmt.Sprintf("🔁 跟单撤出: %s %s %s", shortHex(wallet), poolVersion, shortPool(poolID)))
	return "exit requested", nil
}
