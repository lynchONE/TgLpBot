package strategy

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"TgLpBot/service/user"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

const (
	autoModeConsecutiveDownBreakCooldownThreshold = 2
	autoModeConsecutiveUpBreakExpandThreshold     = 3
	autoModeExpandRangeMultiplier                 = 2.0
	autoModeCooldownDuration                      = 1 * time.Hour
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
}

// NewStrategyService creates a new strategy service
func NewStrategyService() *StrategyService {
	return &StrategyService{
		poolService:        pool.NewPoolService(),
		liquidityService:   liquidity.NewLiquidityService(),
		configService:      user.NewGlobalConfigService(),
		stopChan:           make(chan struct{}),
		ticker:             time.NewTicker(5 * time.Second), // Check every 5 seconds
		lastLiquidityCheck: make(map[uint]time.Time),
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
	if err := database.DB.Where("status IN ? AND paused = ?", []models.StrategyStatus{models.StrategyStatusRunning, models.StrategyStatusWaiting}, false).Find(&tasks).Error; err != nil {
		log.Printf("[Strategy] 获取任务失败: %v", err)
		return
	}

	// Cache for current ticks to avoid duplicate RPC calls in the same cycle
	tickCache := make(map[string]int)

	for i := range tasks {
		s.processTask(&tasks[i], tickCache)
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
		if upStreak >= autoModeConsecutiveUpBreakExpandThreshold {
			// 3 consecutive "up" breaks -> next (4th) open widens the computed range.
			nextMult = autoModeExpandRangeMultiplier
			upStreak = 0
		} else {
			upStreak++
		}
	case "down":
		upStreak = 0
		downStreak++
		if downStreak >= autoModeConsecutiveDownBreakCooldownThreshold {
			// 2 consecutive "down" breaks -> exit to USDT and ignore this pool for 1 hour.
			cooldownReason := fmt.Sprintf("📉 连续 %d 次跌破区间，撤出并冷却 1 小时", autoModeConsecutiveDownBreakCooldownThreshold)
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

		currentTick, err := s.getCurrentTick(task)
		if err != nil {
			log.Printf("[Strategy] 任务 #%d 冷却结束后获取当前 tick 失败: %v", task.ID, err)
			return
		}

		tickLower, tickUpper, rErr := s.calculateRangeFromPercentage(task, currentTick)
		if rErr != nil {
			log.Printf("[Strategy] 任务 #%d 冷却结束后计算新 tick 范围失败: %v", task.ID, rErr)
			return
		}

		task.TickLower = tickLower
		task.TickUpper = tickUpper
		_ = database.DB.Model(task).Updates(map[string]interface{}{
			"tick_lower": tickLower,
			"tick_upper": tickUpper,
		}).Error

		enterRes, err := s.liquidityService.EnterTaskFromUSDT(task.UserID, task)
		if err != nil {
			log.Printf("[Strategy] 任务 #%d 冷却结束后开仓失败: %v", task.ID, err)
			database.DB.Model(task).Updates(map[string]interface{}{
				"status":          models.StrategyStatusError,
				"error_message":   fmt.Sprintf("enter failed after cooldown: %v", err),
				"cooldown_until":  nil,
				"cooldown_reason": "",
			})
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
		database.DB.Model(task).Updates(updates)
		task.ExitLiquidityRemoved = false
		task.CooldownUntil = nil
		task.CooldownReason = ""
		task.NextRangeMultiplier = 1.0

		s.notify(task.UserID, fmt.Sprintf("✅ 冷却结束，已重新开仓（Tick %d）。\n新 Tick 范围: %d - %d\n交易哈希: `%s`", currentTick, tickLower, tickUpper, enterRes.TxHash))
		s.notifyTaskCard(task.UserID, task.ID)
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

		currentTick, err := s.getCurrentTick(task)
		if err != nil {
			log.Printf("[Strategy] 任务 #%d 获取当前 tick 失败: %v", task.ID, err)
			return
		}
		// Update range around current tick (best-effort)
		tickLower, tickUpper, rErr := s.calculateRangeFromPercentage(task, currentTick)
		if rErr == nil {
			task.TickLower = tickLower
			task.TickUpper = tickUpper
		}

		enterRes, err := s.liquidityService.EnterTaskFromUSDT(task.UserID, task)
		if err != nil {
			log.Printf("[Strategy] 任务 #%d 开仓失败: %v", task.ID, err)
			database.DB.Model(task).Updates(map[string]interface{}{
				"status":        models.StrategyStatusError,
				"error_message": fmt.Sprintf("enter failed: %v", err),
			})
			return
		}

		// Update task state
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
		database.DB.Model(task).Updates(updates)
		task.ExitLiquidityRemoved = false

		log.Printf("[Strategy] 任务 #%d 已重新开仓! 继续监控.", task.ID)
	} else {
		// Log occasionally or just debug
		// fmt.Printf("[Strategy] Task #%d waiting... %v remaining\n", task.ID, remaining)
	}
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
