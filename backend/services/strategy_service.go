package services

import (
	"TgLpBot/blockchain"
	"TgLpBot/config"
	"TgLpBot/database"
	"TgLpBot/models"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// StrategyService handles background monitoring and strategy execution
type StrategyService struct {
	poolService      *PoolService
	liquidityService *LiquidityService
	stopChan         chan struct{}
	ticker           *time.Ticker
	notifier         func(userID uint, message string) // Callback for notifications
}

// NewStrategyService creates a new strategy service
func NewStrategyService() *StrategyService {
	return &StrategyService{
		poolService:      NewPoolService(),
		liquidityService: NewLiquidityService(),
		stopChan:         make(chan struct{}),
		ticker:           time.NewTicker(5 * time.Second), // Check every 5 seconds
	}
}

// SetNotifier sets the notification callback
func (s *StrategyService) SetNotifier(fn func(userID uint, message string)) {
	s.notifier = fn
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
	if err := database.DB.Where("status IN ?", []models.StrategyStatus{models.StrategyStatusRunning, models.StrategyStatusWaiting}).Find(&tasks).Error; err != nil {
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

	if inRange {
		// 如果之前超出范围，现在回到范围内，重置计时并通知用户
		if task.OutOfRangeSince != nil {
			database.DB.Model(task).Updates(map[string]interface{}{"out_of_range_since": nil})
			s.notify(task.UserID, fmt.Sprintf("✅ 任务 #%d 价格已回到区间范围\nTick: %d (范围: %d - %d)",
				task.ID, currentTick, task.TickLower, task.TickUpper))
			log.Printf("[Strategy] 任务 #%d 价格回到区间，重置再平衡计时", task.ID)
		}
		return
	}

	// Out of Range Logic
	now := time.Now()
	isFirstTimeOutOfRange := (task.OutOfRangeSince == nil)

	if isFirstTimeOutOfRange {
		database.DB.Model(task).Updates(map[string]interface{}{"out_of_range_since": &now})
		task.OutOfRangeSince = &now
	}

	duration := time.Since(*task.OutOfRangeSince)

	// Determine direction
	isUp := currentTick > task.TickUpper
	isDown := currentTick < task.TickLower

	// 1. Price > TickUpper (涨破)
	if isUp {
		// 首次检测到涨破，立即通知用户
		if isFirstTimeOutOfRange {
			s.notify(task.UserID, fmt.Sprintf("⚠️ 任务 #%d 涨破区间上界\n"+
				"当前 Tick: %d\n"+
				"区间上界: %d\n"+
				"如果 %s 内不回到区间，将自动执行再平衡",
				task.ID, currentTick, task.TickUpper, formatDelayTime(task.ReopenDelaySeconds)))
			log.Printf("[Strategy] 任务 #%d 涨破区间，开始再平衡倒计时 %ds", task.ID, task.ReopenDelaySeconds)
		}

		// Threshold: ReopenDelaySeconds (Rebalance Timeout)
		threshold := time.Duration(task.ReopenDelaySeconds) * time.Second
		if duration >= threshold {
			s.executeRebalance(task, currentTick, now, "📈 涨破区间触发再平衡")
		}
		return
	}

	// 2. Price < TickLower (跌破)
	if isDown {
		if task.StopLossEnabled {
			// Case A: StopLoss Enabled
			// 首次检测到跌破，立即通知用户
			if isFirstTimeOutOfRange {
				stopLossSec := task.StopLossDelaySeconds
				if stopLossSec == 0 {
					s.notify(task.UserID, fmt.Sprintf("⚠️ 任务 #%d 跌破区间下界\n"+
						"当前 Tick: %d\n"+
						"区间下界: %d\n"+
						"将立即执行止损",
						task.ID, currentTick, task.TickLower))
				} else {
					s.notify(task.UserID, fmt.Sprintf("⚠️ 任务 #%d 跌破区间下界\n"+
						"当前 Tick: %d\n"+
						"区间下界: %d\n"+
						"如果 %s 内不回到区间，将自动执行止损",
						task.ID, currentTick, task.TickLower, formatDelayTime(stopLossSec)))
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
				s.notify(task.UserID, fmt.Sprintf("⚠️ 任务 #%d 跌破区间下界\n"+
					"当前 Tick: %d\n"+
					"区间下界: %d\n"+
					"如果 %s 内不回到区间，将自动执行再平衡",
					task.ID, currentTick, task.TickLower, formatDelayTime(task.ReopenDelaySeconds)))
				log.Printf("[Strategy] 任务 #%d 跌破区间，开始再平衡倒计时 %ds", task.ID, task.ReopenDelaySeconds)
			}

			// Threshold: ReopenDelaySeconds
			threshold := time.Duration(task.ReopenDelaySeconds) * time.Second
			if duration >= threshold {
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

// handleWaitingTask checks if it's time to reopen (Keep for compatibility)
func (s *StrategyService) handleWaitingTask(task *models.StrategyTask) {
	if task.LastExitTime == nil {
		return
	}
	// ... (Existing logic can remain for tasks that manually went to waiting or old flows)
	// For new flow, Rebalance uses immediate re-entry.
	if !task.AutoReinvest {
		return
	}

	elapsed := time.Since(*task.LastExitTime)
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
			"v3_position_manager_address": enterRes.V3PositionManagerAddress,
			"v3_token_id":                 enterRes.V3TokenID,
			"v4_token_id":                 enterRes.V4TokenID,
			"tick_lower":                  task.TickLower,
			"tick_upper":                  task.TickUpper,
			"out_of_range_since":          nil,
			"error_message":               "",
		}
		database.DB.Model(task).Updates(updates)

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
	if task.TickSpacing <= 0 {
		return 0, 0, fmt.Errorf("tick spacing not set")
	}
	if task.RangePercentage <= 0 || task.RangePercentage >= 100 {
		return 0, 0, fmt.Errorf("range percentage not set")
	}
	tc := NewTickCalculator()
	tickLower, tickUpper := tc.CalculateTickFromPercentage(currentTick, task.RangePercentage, task.TickSpacing)
	if err := tc.ValidateTickRange(tickLower, tickUpper, task.TickSpacing); err != nil {
		return 0, 0, err
	}
	return tickLower, tickUpper, nil
}

func (s *StrategyService) executeStopLoss(task *models.StrategyTask, now time.Time, reason string) {
	log.Printf("[Strategy] 任务 #%d %s，执行退出", task.ID, reason)
	s.notify(task.UserID, fmt.Sprintf("%s，正在撤出...", reason))

	if _, err := s.liquidityService.ExitTaskToUSDT(task.UserID, task, true); err != nil {
		log.Printf("[Strategy] 任务 #%d 止损退出失败: %v", task.ID, err)
		s.notify(task.UserID, fmt.Sprintf("❌ 止损撤出失败: %v", err))
		database.DB.Model(task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": fmt.Sprintf("stoploss exit failed: %v", err),
		})
		return
	}

	updates := map[string]interface{}{
		"status":             models.StrategyStatusStopped,
		"last_exit_time":     &now,
		"current_liquidity":  "0",
		"out_of_range_since": nil,
		"error_message":      "",
	}
	database.DB.Model(task).Updates(updates)
	s.notify(task.UserID, fmt.Sprintf("✅ %s 完成，任务已停止。", reason))
}

func (s *StrategyService) executeRebalance(task *models.StrategyTask, currentTick int, now time.Time, reason string) {
	log.Printf("[Strategy] 任务 #%d %s，执行再平衡", task.ID, reason)
	s.notify(task.UserID, fmt.Sprintf("%s，正在执行再平衡...", reason))

	// 1. Exit
	if _, err := s.liquidityService.ExitTaskToUSDT(task.UserID, task, true); err != nil {
		log.Printf("[Strategy] 任务 #%d 再平衡退出失败: %v", task.ID, err)
		s.notify(task.UserID, fmt.Sprintf("❌ 再平衡撤出失败: %v", err))
		database.DB.Model(task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": fmt.Sprintf("rebalance exit failed: %v", err),
		})
		return
	}

	// 2. Calculate New Range
	tickLower, tickUpper, err := s.calculateRangeFromPercentage(task, currentTick)
	if err != nil {
		log.Printf("[Strategy] 任务 #%d 计算新 tick 范围失败: %v", task.ID, err)
		s.notify(task.UserID, fmt.Sprintf("❌ 再平衡计算范围失败: %v", err))
		database.DB.Model(task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": fmt.Sprintf("rebalance range calc failed: %v", err),
		})
		return
	}

	// 3. Update Task Params
	task.TickLower = tickLower
	task.TickUpper = tickUpper
	// Must clear these to avoid confusion in Enter
	task.V3TokenID = ""
	task.V4TokenID = ""
	task.CurrentLiquidity = "0"

	// 4. Enter Immediately
	s.notify(task.UserID, "🔄 再平衡撤出已完成，正在按新价格重新开仓...")
	enterRes, err := s.liquidityService.EnterTaskFromUSDT(task.UserID, task)
	if err != nil {
		log.Printf("[Strategy] 任务 #%d 再平衡开仓失败: %v", task.ID, err)
		s.notify(task.UserID, fmt.Sprintf("❌ 再平衡开仓失败: %v。任务已暂停。", err))
		database.DB.Model(task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": fmt.Sprintf("rebalance enter failed: %v", err),
			"tick_lower":    tickLower, // Save the calculated ticks anyway
			"tick_upper":    tickUpper,
		})
		return
	}

	// 5. Update Task Success
	updates := map[string]interface{}{
		"status":                      models.StrategyStatusRunning,
		"tick_lower":                  tickLower,
		"tick_upper":                  tickUpper,
		"last_rebalance_at":           &now,
		"last_exit_time":              &now, // technically re-entered, but marks the event
		"current_liquidity":           enterRes.CurrentLiquidity,
		"v3_position_manager_address": enterRes.V3PositionManagerAddress,
		"v3_token_id":                 enterRes.V3TokenID,
		"v4_token_id":                 enterRes.V4TokenID,
		"out_of_range_since":          nil,
		"error_message":               "",
	}
	database.DB.Model(task).Updates(updates)
	s.notify(task.UserID, fmt.Sprintf("✅ 再平衡完成！\n新 Tick 范围: %d - %d\n交易哈希: `%s`", tickLower, tickUpper, enterRes.TxHash))
}
