package strategy

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/concurrency"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/blacklist"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"TgLpBot/service/txexec"
	"TgLpBot/service/user"
	"bytes"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

const (
	autoModeConsecutiveDownBreakCooldownThreshold = 2
	autoModeConsecutiveUpBreakExpandThreshold     = 2
	autoModeExpandRangeMultiplier                 = 2.0
	autoModeCooldownDurationDefault               = 30 * time.Minute // 默认30分钟，可通过配置覆盖
)

// StrategyService handles background monitoring and strategy execution
type StrategyService struct {
	poolService      *pool.PoolService
	liquidityService *liquidity.LiquidityService
	configService    *user.GlobalConfigService
	stopChan         chan struct{}
	ticker           *time.Ticker
	notifier         func(userID uint, message string) // Callback for notifications
	taskCardNotifier func(userID uint, taskID uint)    // Callback for showing latest task card

	lastLiquidityCheck map[uint]time.Time
	lastLiquidityMu    sync.Mutex

	monitorLimiter *concurrency.KeyedLimiter

	// inflightTasks 用于跟踪正在执行链上交易的任务ID，防止重复提交
	// key: taskID, value: 提交时间
	inflightTasks   map[uint]time.Time
	inflightTasksMu sync.Mutex
}

// NewStrategyService creates a new strategy service
func NewStrategyService() *StrategyService {
	maxUsers := 16
	if config.AppConfig != nil && config.AppConfig.WorkerMaxParallelUsers > 0 {
		maxUsers = config.AppConfig.WorkerMaxParallelUsers
	}
	return &StrategyService{
		poolService:        pool.NewPoolService(),
		liquidityService:   liquidity.NewLiquidityService(),
		configService:      user.NewGlobalConfigService(),
		stopChan:           make(chan struct{}),
		ticker:             time.NewTicker(5 * time.Second), // Check every 5 seconds
		lastLiquidityCheck: make(map[uint]time.Time),
		monitorLimiter:     concurrency.NewKeyedLimiter(maxUsers),
		inflightTasks:      make(map[uint]time.Time),
	}
}

// SetNotifier sets the notification callback
func (s *StrategyService) SetNotifier(fn func(userID uint, message string)) {
	s.notifier = fn
}

func (s *StrategyService) SetTaskCardNotifier(fn func(userID uint, taskID uint)) {
	s.taskCardNotifier = fn
}

// Start starts the strategy monitoring loop
func (s *StrategyService) Start() {
	go s.runLoop()
}

// Stop stops the strategy monitoring loop
func (s *StrategyService) Stop() {
	close(s.stopChan)
	s.ticker.Stop()
}

// CreateTask creates a new strategy task
func (s *StrategyService) CreateTask(task *models.StrategyTask) error {
	return database.DB.Create(task).Error
}

// runLoop is the main monitoring loop
func (s *StrategyService) runLoop() {
	log.Println("[Strategy] 策略监控服务已启动...")
	for {
		select {
		case <-s.ticker.C:
			s.checkTasks()
		case <-s.stopChan:
			log.Println("[Strategy] 策略监控服务已停止")
			return
		}
	}
}

// checkTasks iterates through active tasks and checks their status
func (s *StrategyService) checkTasks() {
	var tasks []models.StrategyTask
	// Find all running or waiting tasks
	if err := database.DB.Where("status IN ? AND paused = ?", []models.StrategyStatus{
		models.StrategyStatusRunning,
		models.StrategyStatusWaiting,
		models.StrategyStatusStopping,
	}, false).Find(&tasks).Error; err != nil {
		log.Printf("[Strategy] 获取任务失败: %v", err)
		return
	}

	// Group tasks by user for isolation: one slow user should not block others.
	byUser := make(map[uint][]*models.StrategyTask)
	for i := range tasks {
		if tasks[i].UserID == 0 {
			continue
		}
		uid := tasks[i].UserID
		byUser[uid] = append(byUser[uid], &tasks[i])
	}

	for uid, userTasks := range byUser {
		userKey := fmt.Sprintf("%d", uid)
		userTasks := userTasks

		if s.monitorLimiter == nil {
			// Fallback: process inline (legacy behavior)
			tickCache := make(map[string]int)
			for i := range userTasks {
				s.processTask(userTasks[i], tickCache)
			}
			continue
		}

		_ = s.monitorLimiter.TryRun(userKey, func() {
			// Cache for current ticks to avoid duplicate RPC calls in the same cycle (per user).
			tickCache := make(map[string]int)
			for i := range userTasks {
				s.processTask(userTasks[i], tickCache)
			}
		})
	}
}

// processTask handles the logic for a single task
func (s *StrategyService) processTask(task *models.StrategyTask, tickCache map[string]int) {
	// Update last check time
	database.DB.Model(task).Update("last_check_time", time.Now())

	// If an exit is pending (e.g. previous exit failed), retry it first and skip other logic.
	if s.processExitRetry(task) {
		return
	}
	// If a rebalance re-entry is pending after a successful exit, retry it first.
	if s.processRebalanceRetry(task) {
		return
	}

	// If the on-chain position has no liquidity (e.g. user removed it manually), auto-stop to avoid "stuck running" tasks.
	if s.processNoLiquidityTask(task) {
		return
	}

	if task.Status == models.StrategyStatusRunning {
		s.handleRunningTask(task, tickCache)
	} else if task.Status == models.StrategyStatusWaiting {
		// With new logic, Rebalance is immediate, so Waiting state is mostly for StopLoss with AutoReinvest (old logic)
		// Or if Enter failed during rebalance and manual intervention fixed it?
		// We keep existing waiting logic for compatibility if task entered Waiting state by other means.
		s.handleWaitingTask(task)
	}
}

// handleRunningTask checks if price is within range
func (s *StrategyService) handleRunningTask(task *models.StrategyTask, tickCache map[string]int) {
	var currentTick int
	var err error
	now := time.Now()

	// Try to get from cache first
	cacheKey := fmt.Sprintf("%s_%s", task.PoolVersion, task.PoolId)
	if cached, ok := tickCache[cacheKey]; ok {
		currentTick = cached
	} else {
		currentTick, err = s.getCurrentTick(task)
		if err != nil {
			log.Printf("[Strategy] 任务 #%d 获取当前 tick 失败: %v", task.ID, err)
			return
		}
		tickCache[cacheKey] = currentTick
	}

	log.Printf("[Strategy] 任务 #%d 监控中: Tick %d (范围 %d - %d)", task.ID, currentTick, task.TickLower, task.TickUpper)

	inRange := currentTick >= task.TickLower && currentTick <= task.TickUpper
	alertLines := pricing.FormatRangeAlertLines(task, task.TickLower, task.TickUpper, currentTick)

	if inRange {
		updates := map[string]interface{}{}

		// 如果之前超出范围，现在回到范围内，重置计时并通知用户
		if task.OutOfRangeSince != nil {
			updates["out_of_range_since"] = nil
			s.notify(task.UserID, fmt.Sprintf("✅ 任务 #%d 价格已回到区间范围\n%s\n%s\n%s",
				task.ID, alertLines.Current, alertLines.Lower, alertLines.Upper))
			log.Printf("[Strategy] 任务 #%d 价格回到区间，重置再平衡计时", task.ID)
		}

		// Clear any previous exit retry give-up state once price is back in range.
		if task.ExitGiveUpAt != nil || task.ExitRetryCount != 0 || task.ExitLastError != "" || task.ExitNextRetryAt != nil {
			updates["exit_retry_count"] = 0
			updates["exit_next_retry_at"] = nil
			updates["exit_last_error"] = ""
			updates["exit_give_up_at"] = nil
		}

		if len(updates) > 0 {
			database.DB.Model(task).Updates(updates)
			task.OutOfRangeSince = nil
			task.ExitRetryCount = 0
			task.ExitNextRetryAt = nil
			task.ExitLastError = ""
			task.ExitGiveUpAt = nil
		}

		return
	}

	// Out of Range Logic
	// Use the same "now" for out-of-range duration calculations.
	isFirstTimeOutOfRange := (task.OutOfRangeSince == nil)

	if isFirstTimeOutOfRange {
		database.DB.Model(task).Updates(map[string]interface{}{"out_of_range_since": &now})
		task.OutOfRangeSince = &now
	}

	duration := time.Since(*task.OutOfRangeSince)

	// Determine direction (use stable price when possible).
	_, _, isUp, isDown := pricing.PriceDirectionFromTicks(task, task.TickLower, task.TickUpper, currentTick)

	// 1. Price above range (涨破)
	if isUp {
		// 首次检测到涨破，立即通知用户
		if isFirstTimeOutOfRange {
			if s.extraNotificationsEnabled(task.UserID) {
				s.notify(task.UserID, fmt.Sprintf("⚠️ 任务 #%d 涨破区间上界\n"+
					"%s\n"+
					"%s\n"+
					"如果 %s 内不回到区间，将自动执行再平衡",
					task.ID, alertLines.Current, alertLines.Upper, formatDelayTime(task.ReopenDelaySeconds)))
			}
			log.Printf("[Strategy] 任务 #%d 涨破区间，开始再平衡倒计时 %ds", task.ID, task.ReopenDelaySeconds)
		}

		// Threshold: ReopenDelaySeconds (Rebalance Timeout)
		threshold := time.Duration(task.ReopenDelaySeconds) * time.Second
		if duration >= threshold {
			if task.IsAuto {
				if s.handleAutoModeRangeBreakExit(task, "up", now, currentTick, "📈 涨破区间触发再平衡") {
					return
				}
			}
			s.executeRebalance(task, currentTick, now, "📈 涨破区间触发再平衡")
		}
		return
	}

	// 2. Price below range (跌破)
	if isDown {
		if task.StopLossEnabled {
			// Case A: StopLoss Enabled
			// 首次检测到跌破，立即通知用户
			if isFirstTimeOutOfRange {
				stopLossSec := task.StopLossDelaySeconds
				if s.extraNotificationsEnabled(task.UserID) {
					if stopLossSec == 0 {
						s.notify(task.UserID, fmt.Sprintf("⚠️ 任务 #%d 跌破区间下界\n"+
							"%s\n"+
							"%s\n"+
							"将立即执行止损",
							task.ID, alertLines.Current, alertLines.Lower))
					} else {
						s.notify(task.UserID, fmt.Sprintf("⚠️ 任务 #%d 跌破区间下界\n"+
							"%s\n"+
							"%s\n"+
							"如果 %s 内不回到区间，将自动执行止损",
							task.ID, alertLines.Current, alertLines.Lower, formatDelayTime(stopLossSec)))
					}
				}
				log.Printf("[Strategy] 任务 #%d 跌破区间，开始止损倒计时 %ds", task.ID, task.StopLossDelaySeconds)
			}

			// Threshold: StopLossDelaySeconds
			threshold := time.Duration(task.StopLossDelaySeconds) * time.Second
			if duration >= threshold {
				s.executeStopLoss(task, now, "⚠️ 跌破区间触发止损")
			}
		} else {
			// Case B: StopLoss Disabled -> Treat as Rebalance
			// 首次检测到跌破，立即通知用户
			if isFirstTimeOutOfRange {
				if s.extraNotificationsEnabled(task.UserID) {
					s.notify(task.UserID, fmt.Sprintf("⚠️ 任务 #%d 跌破区间下界\n"+
						"%s\n"+
						"%s\n"+
						"如果 %s 内不回到区间，将自动执行再平衡",
						task.ID, alertLines.Current, alertLines.Lower, formatDelayTime(task.ReopenDelaySeconds)))
				}
				log.Printf("[Strategy] 任务 #%d 跌破区间，开始再平衡倒计时 %ds", task.ID, task.ReopenDelaySeconds)
			}

			// Threshold: ReopenDelaySeconds
			threshold := time.Duration(task.ReopenDelaySeconds) * time.Second
			if duration >= threshold {
				if task.IsAuto {
					if s.handleAutoModeRangeBreakExit(task, "down", now, currentTick, "📉 跌破区间触发再平衡") {
						return
					}
				}
				s.executeRebalance(task, currentTick, now, "📉 跌破区间触发再平衡")
			}
		}
		return
	}
}

// formatDelayTime 格式化延迟时间，小于60秒显示秒，否则显示分钟
func formatDelayTime(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%d 秒", seconds)
	}
	return fmt.Sprintf("%d 分钟", seconds/60)
}

func (s *StrategyService) notify(userID uint, message string) {
	if s.notifier != nil {
		s.notifier(userID, message)
	}
}

func (s *StrategyService) extraNotificationsEnabled(userID uint) bool {
	if s.configService == nil {
		return true
	}
	cfg, err := s.configService.GetOrCreate(userID)
	if err != nil {
		log.Printf("[Strategy] get global config failed: %v", err)
		return true
	}
	return cfg.ExtraNotificationsEnabled
}

func (s *StrategyService) notifyTaskCard(userID uint, taskID uint) {
	if s.taskCardNotifier != nil {
		s.taskCardNotifier(userID, taskID)
	}
}

// handleAutoModeRangeBreakExit applies Auto mode streak rules and triggers exit.
// Returns true if an exit action was requested (rebalance or cooldown), and caller should stop further processing.
func (s *StrategyService) handleAutoModeRangeBreakExit(task *models.StrategyTask, direction string, now time.Time, currentTick int, reason string) bool {
	if task == nil || !task.IsAuto {
		return false
	}

	// Don't change behavior during exit/re-entry retries.
	if task.ExitGiveUpAt != nil {
		return false
	}
	if strings.TrimSpace(task.ExitPendingAction) != "" || task.RebalancePending {
		return false
	}

	upStreak := task.RangeBreakUpStreak
	downStreak := task.RangeBreakDownStreak
	nextMult := 1.0

	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "up":
		downStreak = 0
		upStreak++
		if upStreak >= autoModeConsecutiveUpBreakExpandThreshold {
			// 2 consecutive "up" breaks -> widen the next computed range.
			nextMult = autoModeExpandRangeMultiplier
			upStreak = 0
		}
	case "down":
		upStreak = 0
		downStreak++
		if downStreak >= autoModeConsecutiveDownBreakCooldownThreshold {
			// 2 consecutive "down" breaks -> exit to USDT and cooldown this trading pair
			cooldownMinutes := 30
			if config.AppConfig != nil && config.AppConfig.AutoLPGuardCooldownSeconds > 0 {
				cooldownMinutes = config.AppConfig.AutoLPGuardCooldownSeconds / 60
			}
			cooldownReason := fmt.Sprintf("📉 连续 %d 次跌破区间，撤出并冷却 %d 分钟", autoModeConsecutiveDownBreakCooldownThreshold, cooldownMinutes)
			updates := map[string]interface{}{
				"range_break_up_streak":   0,
				"range_break_down_streak": 0,
				"next_range_multiplier":   1.0,
			}
			_ = database.DB.Model(task).Updates(updates).Error
			task.RangeBreakUpStreak = 0
			task.RangeBreakDownStreak = 0
			task.NextRangeMultiplier = 1.0

			log.Printf("[Strategy] 任务 #%d 连续跌破 %d 次，触发冷却退出", task.ID, autoModeConsecutiveDownBreakCooldownThreshold)

			// 同交易对冷却：暂停该用户同交易对的其他任务
			s.cooldownSameTradingPairTasks(task, cooldownMinutes, cooldownReason)

			s.requestExitToUSDT(task, ExitActionCooldown, cooldownReason)
			return true
		}
	default:
		return false
	}

	updates := map[string]interface{}{
		"range_break_up_streak":   upStreak,
		"range_break_down_streak": downStreak,
		"next_range_multiplier":   nextMult,
	}
	_ = database.DB.Model(task).Updates(updates).Error
	task.RangeBreakUpStreak = upStreak
	task.RangeBreakDownStreak = downStreak
	task.NextRangeMultiplier = nextMult

	if nextMult != 1.0 {
		log.Printf("[Strategy] 任务 #%d 连续涨破达到阈值，下次开仓区间扩大 x%.2f", task.ID, nextMult)
	}

	s.executeRebalance(task, currentTick, now, strings.TrimSpace(reason))
	return true
}

// handleWaitingTask checks if it's time to reopen (Keep for compatibility)
func (s *StrategyService) handleWaitingTask(task *models.StrategyTask) {
	now := time.Now()

	// Auto-mode cooldown: temporarily ignore this pool, then re-enter after cooldown expires.
	if task.CooldownUntil != nil {
		if now.Before(*task.CooldownUntil) {
			return
		}

		log.Printf("[Strategy] 任务 #%d 冷却结束，准备重新开仓...", task.ID)
		exec := txexec.Default()
		if exec == nil {
			return
		}
		if ok, err := exec.TryRunUser(task.UserID, func(_ string) {
			s.runCooldownReopen(task.ID, task.UserID)
		}); err != nil {
			log.Printf("[Strategy] schedule cooldown reopen failed: task_id=%d user_id=%d err=%v", task.ID, task.UserID, err)
		} else if !ok {
			// Wallet is busy; retry next cycle.
		}
		return
	}

	if task.LastExitTime == nil {
		return
	}
	// ... (Existing logic can remain for tasks that manually went to waiting or old flows)
	// For new flow, Rebalance uses immediate re-entry.
	if !task.AutoReinvest {
		return
	}

	elapsed := now.Sub(*task.LastExitTime)
	remaining := time.Duration(task.ReopenDelaySeconds)*time.Second - elapsed

	if remaining <= 0 {
		log.Printf("[Strategy] 任务 #%d 等待时间结束，准备重新开仓...", task.ID)
		exec := txexec.Default()
		if exec == nil {
			return
		}
		if ok, err := exec.TryRunUser(task.UserID, func(_ string) {
			s.runWaitingReopen(task.ID, task.UserID)
		}); err != nil {
			log.Printf("[Strategy] schedule waiting reopen failed: task_id=%d user_id=%d err=%v", task.ID, task.UserID, err)
		} else if !ok {
			// Wallet is busy; retry next cycle.
		}
	} else {
		// Log occasionally or just debug
		// fmt.Printf("[Strategy] Task #%d waiting... %v remaining\n", task.ID, remaining)
	}
}

func (s *StrategyService) runCooldownReopen(taskID uint, userID uint) {
	if taskID == 0 || userID == 0 {
		return
	}

	var task models.StrategyTask
	if err := database.DB.Where("id = ? AND user_id = ?", taskID, userID).First(&task).Error; err != nil {
		log.Printf("[Strategy] load task for cooldown reopen failed: task_id=%d user_id=%d err=%v", taskID, userID, err)
		return
	}
	if task.Status != models.StrategyStatusWaiting || task.CooldownUntil == nil {
		return
	}

	now := time.Now()
	if now.Before(*task.CooldownUntil) {
		return
	}

	currentTick, err := s.getCurrentTick(&task)
	if err != nil {
		log.Printf("[Strategy] 任务 #%d 冷却结束后获取当前 tick 失败: %v", task.ID, err)
		return
	}

	tickLower, tickUpper, rErr := s.calculateRangeFromPercentage(&task, currentTick)
	if rErr != nil {
		log.Printf("[Strategy] 任务 #%d 冷却结束后计算新 tick 范围失败: %v", task.ID, rErr)
		return
	}

	task.TickLower = tickLower
	task.TickUpper = tickUpper
	_ = database.DB.Model(&task).Updates(map[string]interface{}{
		"tick_lower": tickLower,
		"tick_upper": tickUpper,
	}).Error

	enterRes, err := s.liquidityService.EnterTaskFromUSDT(task.UserID, &task)
	if err != nil {
		log.Printf("[Strategy] 任务 #%d 冷却结束后开仓失败: %v", task.ID, err)
		_ = database.DB.Model(&task).Updates(map[string]interface{}{
			"status":          models.StrategyStatusError,
			"error_message":   fmt.Sprintf("enter failed after cooldown: %v", err),
			"cooldown_until":  nil,
			"cooldown_reason": "",
		}).Error
		return
	}

	updates := map[string]interface{}{
		"status":                      models.StrategyStatusRunning,
		"current_liquidity":           enterRes.CurrentLiquidity,
		"exit_liquidity_removed":      false,
		"v3_position_manager_address": enterRes.V3PositionManagerAddress,
		"v3_token_id":                 enterRes.V3TokenID,
		"v4_token_id":                 enterRes.V4TokenID,
		"tick_lower":                  task.TickLower,
		"tick_upper":                  task.TickUpper,
		"out_of_range_since":          nil,
		"cooldown_until":              nil,
		"cooldown_reason":             "",
		"next_range_multiplier":       1.0,
		"error_message":               "",
	}
	if task.IsAuto {
		updates["GuardOpenVolume5m"] = 0
		updates["GuardOpenPrice"] = 0
		updates["GuardOpenTxCount5m"] = 0
		updates["GuardOpenFeePercentage"] = 0
		updates["GuardOpenFeeRate5mPct"] = 0
		updates["GuardOpenTotalFees5m"] = 0
		updates["GuardOpenTVLUSD"] = 0
		updates["GuardOpenMetricsAt"] = nil
		updates["GuardPeakFeePercentage"] = 0
		updates["GuardPeakFeeRate5mPct"] = 0
		updates["GuardPeakTotalFees5m"] = 0
		updates["GuardPeakVolume5m"] = 0
		updates["GuardPeakTVLUSD"] = 0
		updates["GuardPeakPrice"] = 0
		updates["GuardPeakTxCount5m"] = 0
		updates["GuardVolumeDropArmed"] = false
		updates["GuardVolumeDropLastVolume5m"] = 0
		updates["GuardPriceTxDropArmed"] = false
	}
	if dbErr := database.DB.Model(&task).Updates(updates).Error; dbErr != nil {
		log.Printf("[Strategy] ⚠️ 任务 #%d 保存开仓结果失败 (链上交易已成功): %v", task.ID, dbErr)
		// 兜底：关键字段优先写入，避免任务被误判为未开仓而重复开仓。
		criticalUpdates := map[string]interface{}{
			"status":                      models.StrategyStatusRunning,
			"current_liquidity":           enterRes.CurrentLiquidity,
			"exit_liquidity_removed":      false,
			"v3_position_manager_address": enterRes.V3PositionManagerAddress,
			"v3_token_id":                 enterRes.V3TokenID,
			"v4_token_id":                 enterRes.V4TokenID,
			"tick_lower":                  task.TickLower,
			"tick_upper":                  task.TickUpper,
			"out_of_range_since":          nil,
			"cooldown_until":              nil,
			"cooldown_reason":             "",
			"next_range_multiplier":       1.0,
			"error_message":               "",
		}
		if cErr := database.DB.Model(&task).Updates(criticalUpdates).Error; cErr != nil {
			log.Printf("[Strategy] ⚠️ 任务 #%d 兜底写入关键字段仍失败: %v", task.ID, cErr)
		}
	}

	s.notify(task.UserID, fmt.Sprintf("✅ 冷却结束，已重新开仓（Tick %d）。\n新 Tick 范围: %d - %d\n交易哈希: `%s`", currentTick, tickLower, tickUpper, enterRes.TxHash))
	s.notifyTaskCard(task.UserID, task.ID)
}

func (s *StrategyService) runWaitingReopen(taskID uint, userID uint) {
	if taskID == 0 || userID == 0 {
		return
	}

	var task models.StrategyTask
	if err := database.DB.Where("id = ? AND user_id = ?", taskID, userID).First(&task).Error; err != nil {
		log.Printf("[Strategy] load task for waiting reopen failed: task_id=%d user_id=%d err=%v", taskID, userID, err)
		return
	}
	if task.Status != models.StrategyStatusWaiting || !task.AutoReinvest || task.LastExitTime == nil {
		return
	}

	now := time.Now()
	elapsed := now.Sub(*task.LastExitTime)
	remaining := time.Duration(task.ReopenDelaySeconds)*time.Second - elapsed
	if remaining > 0 {
		return
	}

	currentTick, err := s.getCurrentTick(&task)
	if err != nil {
		log.Printf("[Strategy] 任务 #%d 获取当前 tick 失败: %v", task.ID, err)
		return
	}
	// Update range around current tick (best-effort)
	tickLower, tickUpper, rErr := s.calculateRangeFromPercentage(&task, currentTick)
	if rErr == nil {
		task.TickLower = tickLower
		task.TickUpper = tickUpper
	}

	enterRes, err := s.liquidityService.EnterTaskFromUSDT(task.UserID, &task)
	if err != nil {
		log.Printf("[Strategy] 任务 #%d 开仓失败: %v", task.ID, err)
		_ = database.DB.Model(&task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": fmt.Sprintf("enter failed: %v", err),
		}).Error
		return
	}

	updates := map[string]interface{}{
		"status":                      models.StrategyStatusRunning,
		"current_liquidity":           enterRes.CurrentLiquidity,
		"exit_liquidity_removed":      false,
		"v3_position_manager_address": enterRes.V3PositionManagerAddress,
		"v3_token_id":                 enterRes.V3TokenID,
		"v4_token_id":                 enterRes.V4TokenID,
		"tick_lower":                  task.TickLower,
		"tick_upper":                  task.TickUpper,
		"out_of_range_since":          nil,
		"error_message":               "",
	}
	if task.IsAuto {
		updates["GuardOpenVolume5m"] = 0
		updates["GuardOpenPrice"] = 0
		updates["GuardOpenTxCount5m"] = 0
		updates["GuardOpenFeePercentage"] = 0
		updates["GuardOpenFeeRate5mPct"] = 0
		updates["GuardOpenTotalFees5m"] = 0
		updates["GuardOpenTVLUSD"] = 0
		updates["GuardOpenMetricsAt"] = nil
		updates["GuardPeakFeePercentage"] = 0
		updates["GuardPeakFeeRate5mPct"] = 0
		updates["GuardPeakTotalFees5m"] = 0
		updates["GuardPeakVolume5m"] = 0
		updates["GuardPeakTVLUSD"] = 0
		updates["GuardPeakPrice"] = 0
		updates["GuardPeakTxCount5m"] = 0
		updates["GuardVolumeDropArmed"] = false
		updates["GuardVolumeDropLastVolume5m"] = 0
		updates["GuardPriceTxDropArmed"] = false
	}
	if dbErr := database.DB.Model(&task).Updates(updates).Error; dbErr != nil {
		log.Printf("[Strategy] ⚠️ 任务 #%d 保存开仓结果失败 (链上交易已成功): %v", task.ID, dbErr)
		criticalUpdates := map[string]interface{}{
			"status":                      models.StrategyStatusRunning,
			"current_liquidity":           enterRes.CurrentLiquidity,
			"exit_liquidity_removed":      false,
			"v3_position_manager_address": enterRes.V3PositionManagerAddress,
			"v3_token_id":                 enterRes.V3TokenID,
			"v4_token_id":                 enterRes.V4TokenID,
			"tick_lower":                  task.TickLower,
			"tick_upper":                  task.TickUpper,
			"out_of_range_since":          nil,
			"next_range_multiplier":       1.0,
			"error_message":               "",
		}
		if cErr := database.DB.Model(&task).Updates(criticalUpdates).Error; cErr != nil {
			log.Printf("[Strategy] ⚠️ 任务 #%d 兜底写入关键字段仍失败: %v", task.ID, cErr)
		}
	}

	log.Printf("[Strategy] 任务 #%d 已重新开仓! 继续监控.", task.ID)
}

// Mock functions for V4 until V4 contract is ready
func (s *StrategyService) mockV4Remove(task *models.StrategyTask) error {
	time.Sleep(1 * time.Second)
	log.Printf("[Strategy] [MOCK V4] 已从 V4 池 %s 移除流动性，转换为 USDT", task.PoolId)
	return nil
}

func (s *StrategyService) mockV4Add(task *models.StrategyTask) error {
	time.Sleep(1 * time.Second)
	log.Printf("[Strategy] [MOCK V4] 已将 USDT 添加到 V4 池 %s", task.PoolId)
	return nil
}

func (s *StrategyService) mockV3Add(task *models.StrategyTask) error {
	time.Sleep(1 * time.Second)
	log.Printf("[Strategy] [MOCK V3] 已将 USDT 添加到 V3 池 %s", task.PoolId)
	return nil
}

func (s *StrategyService) mockAdd(task *models.StrategyTask) error {
	switch strings.ToLower(strings.TrimSpace(task.PoolVersion)) {
	case "v4":
		return s.mockV4Add(task)
	default:
		return s.mockV3Add(task)
	}
}

func (s *StrategyService) refreshTaskPoolMeta(task *models.StrategyTask) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}
	if s == nil || s.poolService == nil {
		return fmt.Errorf("pool service not initialized")
	}
	poolID := strings.TrimSpace(task.PoolId)
	if poolID == "" {
		return fmt.Errorf("pool id is empty")
	}

	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	var info *pool.PoolInfo
	var err error
	switch version {
	case "v4":
		info, err = s.poolService.GetV4PoolInfo(poolID)
	default:
		info, err = s.poolService.GetPoolInfo(poolID)
	}
	if err != nil {
		return err
	}
	if info == nil || info.TickSpacing <= 0 {
		return fmt.Errorf("invalid pool info")
	}

	token0Addr := strings.TrimSpace(info.Token0)
	token1Addr := strings.TrimSpace(info.Token1)
	token0Symbol := strings.TrimSpace(info.Token0Symbol)
	token1Symbol := strings.TrimSpace(info.Token1Symbol)
	if version == "v4" && common.IsHexAddress(token0Addr) && common.IsHexAddress(token1Addr) {
		a0 := common.HexToAddress(token0Addr)
		a1 := common.HexToAddress(token1Addr)
		if bytes.Compare(a0.Bytes(), a1.Bytes()) > 0 {
			token0Addr, token1Addr = token1Addr, token0Addr
			token0Symbol, token1Symbol = token1Symbol, token0Symbol
		}
	}

	hooksAddr := strings.TrimSpace(info.HooksAddress)
	if !common.IsHexAddress(hooksAddr) {
		hooksAddr = "0x0000000000000000000000000000000000000000"
	}

	updates := map[string]interface{}{
		"exchange":       strings.TrimSpace(info.Exchange),
		"token0_symbol":  token0Symbol,
		"token1_symbol":  token1Symbol,
		"token0_address": token0Addr,
		"token1_address": token1Addr,
		"hooks_address":  hooksAddr,
		"fee":            info.Fee,
		"tick_spacing":   info.TickSpacing,
	}
	if err := database.DB.Model(task).Updates(updates).Error; err != nil {
		return err
	}

	task.Exchange = strings.TrimSpace(info.Exchange)
	task.Token0Symbol = token0Symbol
	task.Token1Symbol = token1Symbol
	task.Token0Address = token0Addr
	task.Token1Address = token1Addr
	task.HooksAddress = hooksAddr
	task.Fee = info.Fee
	task.TickSpacing = info.TickSpacing
	return nil
}

func (s *StrategyService) getCurrentTick(task *models.StrategyTask) (int, error) {
	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	switch version {
	case "v4":
		if config.AppConfig == nil {
			return 0, fmt.Errorf("config not loaded")
		}
		if !common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
			return 0, fmt.Errorf("UNISWAP_V4_POOL_MANAGER_ADDRESS not set")
		}
		poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
		// Use StateView directly (PoolManager.slot0 is not supported on this deployment)
		if !common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) {
			return 0, fmt.Errorf("UNISWAP_V4_STATE_VIEW_ADDRESS not configured for V4 tick query")
		}
		stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
		currentTick, err := blockchain.GetUniswapV4PoolCurrentTickViaStateView(stateView, poolManager, task.PoolId)
		return currentTick, err
	default:
		if !common.IsHexAddress(task.PoolId) {
			return 0, fmt.Errorf("invalid V3 pool address: %s", task.PoolId)
		}
		return blockchain.GetV3PoolCurrentTick(common.HexToAddress(task.PoolId))
	}
}

func (s *StrategyService) calculateRangeFromPercentage(task *models.StrategyTask, currentTick int) (int, int, error) {
	return s.calculateRangeFromPercentageWithMultiplier(task, currentTick, 1.0)
}

func (s *StrategyService) calculateRangeFromPercentageWithMultiplier(task *models.StrategyTask, currentTick int, multiplier float64) (int, int, error) {
	if task.TickSpacing <= 0 {
		return 0, 0, fmt.Errorf("tick spacing not set")
	}
	tc := pool.NewTickCalculator()

	lowerPct := task.RangePercentage
	upperPct := task.RangePercentage

	if task.RangeLowerPercentage > 0 && task.RangeUpperPercentage > 0 {
		lowerPct = task.RangeLowerPercentage
		upperPct = task.RangeUpperPercentage
	} else if task.TickLower != 0 && task.TickUpper != 0 {
		centerTick := (task.TickLower + task.TickUpper) / 2
		effLower, effUpper := tc.CalculatePercentagesFromTicks(centerTick, task.TickLower, task.TickUpper)
		if effLower > 0 && effUpper > 0 {
			lowerPct = effLower
			upperPct = effUpper
		}
	}

	if multiplier <= 0 {
		multiplier = 1.0
	}
	if multiplier != 1.0 {
		lowerPct = lowerPct * multiplier
		upperPct = upperPct * multiplier
	}

	// Hard cap at <100 to satisfy tick calculation constraints.
	const maxPct = 99.0
	if lowerPct > maxPct {
		lowerPct = maxPct
	}
	if upperPct > maxPct {
		upperPct = maxPct
	}

	if lowerPct <= 0 || upperPct <= 0 || lowerPct >= 100 || upperPct >= 100 {
		return 0, 0, fmt.Errorf("range percentage not set")
	}

	// Use best-fit rounding to minimize distortion caused by tickSpacing quantization.
	tickLower, tickUpper := tc.CalculateTickFromPercentagesBestFit(currentTick, lowerPct, upperPct, task.TickSpacing)
	if err := tc.ValidateTickRange(tickLower, tickUpper, task.TickSpacing); err != nil {
		return 0, 0, err
	}
	return tickLower, tickUpper, nil
}

func (s *StrategyService) executeStopLoss(task *models.StrategyTask, now time.Time, reason string) {
	if task.ExitGiveUpAt != nil {
		return
	}

	log.Printf("[Strategy] 任务 #%d %s，执行退出", task.ID, reason)
	s.requestExitToUSDT(task, ExitActionStopLoss, reason)
}

func (s *StrategyService) executeRebalance(task *models.StrategyTask, currentTick int, now time.Time, reason string) {
	if task.ExitGiveUpAt != nil {
		return
	}

	log.Printf("[Strategy] 任务 #%d %s，执行再平衡", task.ID, reason)
	s.requestExitToUSDT(task, ExitActionRebalance, reason)
}

func isStableCoin(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "USDT", "USDC", "DAI", "FDUSD", "TUSD", "BUSD":
		return true
	}
	return false
}

// cooldownSameTradingPairTasks 将同交易对的其他活跃任务也设置为冷却状态
// 交易对匹配逻辑：Token0Symbol和Token1Symbol相同（忽略顺序）
func (s *StrategyService) cooldownSameTradingPairTasks(task *models.StrategyTask, cooldownMinutes int, reason string) {
	if task == nil || task.UserID == 0 {
		return
	}

	// 获取交易对符号
	sym0 := strings.ToUpper(strings.TrimSpace(task.Token0Symbol))
	sym1 := strings.ToUpper(strings.TrimSpace(task.Token1Symbol))
	if sym0 == "" || sym1 == "" {
		return
	}

	// 计算冷却结束时间
	cooldownDuration := time.Duration(cooldownMinutes) * time.Minute
	cooldownUntil := time.Now().Add(cooldownDuration)

	// 识别冷却目标（非稳定币代币 或 交易对）
	// 用户要求：包含稳定币以外的那个代币的池子都要被禁止开仓
	// 因此，我们尝试提取非稳定币作为冷却 Key
	cooldownKey := fmt.Sprintf("%s/%s", sym0, sym1)
	if isStableCoin(sym0) && !isStableCoin(sym1) {
		cooldownKey = sym1
	} else if !isStableCoin(sym0) && isStableCoin(sym1) {
		cooldownKey = sym0
	}

	// 写入 Redis 冷却
	cooldownSvc := blacklist.NewCooldownService()
	if err := cooldownSvc.Add(task.UserID, cooldownKey, reason, cooldownDuration); err != nil {
		log.Printf("[Strategy] 写入 Redis 冷却失败: user_id=%d key=%s err=%v", task.UserID, cooldownKey, err)
	} else {
		log.Printf("[Strategy] 目标 %s 已写入 Redis 冷却 %d 分钟", cooldownKey, cooldownMinutes)
	}

	// 查找该用户同交易对的其他活跃任务
	var otherTasks []models.StrategyTask
	if err := database.DB.Where(
		"user_id = ? AND id != ? AND is_auto = ? AND status IN ?",
		task.UserID, task.ID, true, []models.StrategyStatus{
			models.StrategyStatusRunning,
			models.StrategyStatusWaiting,
		},
	).Find(&otherTasks).Error; err != nil {
		log.Printf("[Strategy] 查询同交易对任务失败: %v", err)
		return
	}

	cooldownCount := 0
	for _, otherTask := range otherTasks {
		otherSym0 := strings.ToUpper(strings.TrimSpace(otherTask.Token0Symbol))
		otherSym1 := strings.ToUpper(strings.TrimSpace(otherTask.Token1Symbol))

		// 检查是否同交易对（忽略顺序）
		sameSymbols := (sym0 == otherSym0 && sym1 == otherSym1) || (sym0 == otherSym1 && sym1 == otherSym0)
		if !sameSymbols {
			continue
		}

		// 设置冷却
		updates := map[string]interface{}{
			"status":                  models.StrategyStatusWaiting,
			"cooldown_until":          &cooldownUntil,
			"cooldown_reason":         fmt.Sprintf("📉 同交易对 %s/%s 触发冷却", sym0, sym1),
			"range_break_up_streak":   0,
			"range_break_down_streak": 0,
			"next_range_multiplier":   1.0,
			"out_of_range_since":      nil,
			"rebalance_pending":       false,
			"rebalance_retry_count":   0,
			"rebalance_next_retry_at": nil,
			"rebalance_last_error":    "",
			"exit_pending_action":     "",
			"exit_pending_reason":     "",
			"exit_retry_count":        0,
			"exit_next_retry_at":      nil,
			"exit_last_error":         "",
			"exit_give_up_at":         nil,
		}
		if err := database.DB.Model(&otherTask).Updates(updates).Error; err != nil {
			log.Printf("[Strategy] 设置任务 #%d 冷却失败: %v", otherTask.ID, err)
			continue
		}

		cooldownCount++
		log.Printf("[Strategy] 任务 #%d 同交易对 %s/%s 触发冷却 %d 分钟", otherTask.ID, sym0, sym1, cooldownMinutes)
	}

	if cooldownCount > 0 {
		s.notify(task.UserID, fmt.Sprintf("⚠️ 交易对 %s/%s 连续跌破，同交易对共 %d 个任务进入冷却 %d 分钟", sym0, sym1, cooldownCount+1, cooldownMinutes))
	}
}
