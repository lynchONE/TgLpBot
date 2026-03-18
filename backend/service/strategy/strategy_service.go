package strategy

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/concurrency"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
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

// StrategyService handles background monitoring and strategy execution
type StrategyService struct {
	poolService      *pool.PoolService
	liquidityService *liquidity.LiquidityService
	configService    *user.GlobalConfigService
	accessService    *user.AccessService
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
		accessService:      user.NewAccessService(),
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

		if s.accessService != nil {
			check, err := s.accessService.CheckUserAccess(uid, time.Now())
			if err != nil {
				log.Printf("[Strategy] 检查用户授权失败: user_id=%d err=%v", uid, err)
				continue
			}
			if !check.Allowed {
				s.pauseUserTasks(uid, check.Reason)
				continue
			}
		}

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

func (s *StrategyService) pauseUserTasks(userID uint, reason string) {
	if database.DB == nil {
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"paused":             true,
		"paused_at":          &now,
		"out_of_range_since": nil,
	}

	res := database.DB.Model(&models.StrategyTask{}).
		Where("user_id = ? AND paused = ?", userID, false).
		Updates(updates)
	if res.Error != nil {
		log.Printf("[Strategy] 暂停用户任务失败: user_id=%d err=%v", userID, res.Error)
		return
	}
	if res.RowsAffected <= 0 {
		return
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "未授权"
	}
	s.notify(userID, fmt.Sprintf("⚠️ 授权状态变更：%s\n\n已自动暂停所有任务。", reason))
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
	cacheKey := fmt.Sprintf("%s_%s_%s", config.NormalizeChain(task.Chain), task.PoolVersion, task.PoolId)
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

	// Follow tasks are copy-trading positions: keep the tick range consistent with the target wallet.
	// Do not auto-rebalance or stop-loss based on out-of-range.
	if task.IsFollow {
		return
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

// handleWaitingTask checks if it's time to reopen (Keep for compatibility)
func (s *StrategyService) handleWaitingTask(task *models.StrategyTask) {
	now := time.Now()

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
		if ok, err := exec.TryRunTask(task.UserID, task.WalletID, task.WalletAddress, func(_ string) {
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

	chain := config.NormalizeChain(task.Chain)
	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	var info *pool.PoolInfo
	var err error
	switch version {
	case "v4":
		if chain != "bsc" {
			return fmt.Errorf("v4 not supported on chain=%s", chain)
		}
		info, err = s.poolService.GetV4PoolInfo(poolID)
	default:
		info, err = s.poolService.GetPoolInfoForChain(chain, poolID)
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
	chain := config.NormalizeChain(task.Chain)
	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	switch version {
	case "v4":
		if config.AppConfig == nil {
			return 0, fmt.Errorf("config not loaded")
		}
		if chain != "bsc" {
			return 0, fmt.Errorf("v4 not supported on chain=%s", chain)
		}

		cc, ok := config.AppConfig.GetChainConfig(chain)
		if !ok {
			return 0, fmt.Errorf("chain config not found: %s", chain)
		}

		poolManagerStr := strings.TrimSpace(cc.UniswapV4PoolManagerAddress)
		stateViewStr := strings.TrimSpace(cc.UniswapV4StateViewAddress)
		if !common.IsHexAddress(poolManagerStr) {
			poolManagerStr = strings.TrimSpace(config.AppConfig.UniswapV4PoolManagerAddress)
		}
		if !common.IsHexAddress(stateViewStr) {
			stateViewStr = strings.TrimSpace(config.AppConfig.UniswapV4StateViewAddress)
		}
		if !common.IsHexAddress(poolManagerStr) {
			return 0, fmt.Errorf("UNISWAP_V4_POOL_MANAGER_ADDRESS not set")
		}
		if !common.IsHexAddress(stateViewStr) {
			return 0, fmt.Errorf("UNISWAP_V4_STATE_VIEW_ADDRESS not configured for V4 tick query")
		}

		poolManager := common.HexToAddress(poolManagerStr)
		stateView := common.HexToAddress(stateViewStr)
		currentTick, err := blockchain.GetUniswapV4PoolCurrentTickViaStateView(stateView, poolManager, task.PoolId)
		return currentTick, err
	default:
		if !common.IsHexAddress(task.PoolId) {
			return 0, fmt.Errorf("invalid V3 pool address: %s", task.PoolId)
		}
		client, _, err := blockchain.GetEVMClient(chain)
		if err != nil {
			return 0, err
		}
		return blockchain.GetV3PoolCurrentTickWithClient(client, common.HexToAddress(task.PoolId))
	}
}

func (s *StrategyService) calculateRangeFromPercentage(task *models.StrategyTask, currentTick int) (int, int, error) {
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
