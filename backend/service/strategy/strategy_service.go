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

	// inflightTasks prevents duplicate processing for the same task. key: taskID, value: task start time.
	inflightTasks   map[uint]time.Time
	inflightTasksMu sync.Mutex
}

type OutOfRangeAction string

const (
	OutOfRangeActionNone      OutOfRangeAction = ""
	OutOfRangeActionRebalance OutOfRangeAction = "rebalance"
	OutOfRangeActionExit      OutOfRangeAction = "exit"
)

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
		ticker:             time.NewTicker(2 * time.Second), // Check every 2 seconds
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
	return CreateTaskRecord(task)
}

// runLoop is the main monitoring loop
func (s *StrategyService) runLoop() {
	log.Println("[Strategy] monitor loop starting")
	for {
		select {
		case <-s.ticker.C:
			s.checkTasks()
		case <-s.stopChan:
			log.Println("[Strategy] monitor loop stopped")
			return
		}
	}
}

// checkTasks iterates through active tasks and checks their status
func (s *StrategyService) checkTasks() {
	var tasks []models.StrategyTask
	activeStatuses := []models.StrategyStatus{
		models.StrategyStatusRunning,
		models.StrategyStatusWaiting,
		models.StrategyStatusStopping,
	}
	// Paused tasks normally skip strategy automation, but queued DCA batches still
	// need monitoring so default-paused opens can finish their configured split entry.
	if err := database.DB.Where(
		"status IN ? AND (paused = ? OR (status = ? AND paused = ? AND dca_enabled = ? AND dca_next_batch_at IS NOT NULL))",
		activeStatuses,
		false,
		models.StrategyStatusRunning,
		true,
		true,
	).Find(&tasks).Error; err != nil {
		log.Printf("[Strategy] load active tasks failed: %v", err)
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
				log.Printf("[Strategy] check user access failed: user_id=%d err=%v", uid, err)
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
		log.Printf("[Strategy] pause user tasks failed: user_id=%d err=%v", userID, res.Error)
		return
	}
	if res.RowsAffected <= 0 {
		return
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "access expired"
	}
	s.notify(userID, fmt.Sprintf("Task monitoring has been paused.\n\nReason: %s", reason))
}

// processTask handles the logic for a single task
func (s *StrategyService) processTask(task *models.StrategyTask, tickCache map[string]int) {
	// Update last check time
	database.DB.Model(task).Update("last_check_time", time.Now())

	if task.Paused {
		if shouldMonitorPausedDCA(task) {
			s.handleRunningTask(task, tickCache)
		}
		return
	}

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
			log.Printf("[Strategy] task #%d load current tick failed: %v", task.ID, err)
			return
		}
		tickCache[cacheKey] = currentTick
	}

	log.Printf("[Strategy] task #%d current tick=%d range=%d-%d", task.ID, currentTick, task.TickLower, task.TickUpper)

	inRange := currentTick >= task.TickLower && currentTick <= task.TickUpper

	// DCA batching: advance or cancel remaining batches based on current inRange state.
	s.processDCABatch(task, inRange)
	if task.Paused {
		return
	}

	alertLines := pricing.FormatRangeAlertLines(task, task.TickLower, task.TickUpper, currentTick)

	if inRange {
		updates := map[string]interface{}{}

		if ShouldDelayOutOfRangeHandling(task) {
			updates["range_activation_pending"] = false
			if s.extraNotificationsEnabled(task.UserID) {
				s.notify(task.UserID, fmt.Sprintf("Task #%d is now in range. Automatic handling has been enabled.", task.ID))
			}
			log.Printf("[Strategy] task #%d first entered range, auto handling enabled", task.ID)
		}

		if task.OutOfRangeSince != nil {
			updates["out_of_range_since"] = nil
			s.notify(task.UserID, fmt.Sprintf("Task #%d returned to range.\n%s\n%s\n%s", task.ID, alertLines.Current, alertLines.Lower, alertLines.Upper))
			log.Printf("[Strategy] task #%d returned to range", task.ID)
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
			task.RangeActivationPending = false
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
	_, _, isUp, isDown := pricing.PriceDirectionFromTicks(task, task.TickLower, task.TickUpper, currentTick)
	if !isUp && !isDown {
		return
	}

	if task.IsFollow {
		if ShouldExitFollowDownside(task, isDown) {
			s.executeOutOfRangeStop(task, now, "跟单仓位下破区间：保底撤出并停止任务")
		}
		return
	}

	if ShouldDelayOutOfRangeHandling(task) {
		return
	}

	isFirstTimeOutOfRange := (task.OutOfRangeSince == nil)

	if isFirstTimeOutOfRange {
		database.DB.Model(task).Updates(map[string]interface{}{"out_of_range_since": &now})
		task.OutOfRangeSince = &now
	}

	duration := time.Since(*task.OutOfRangeSince)

	action := ResolveOutOfRangeAction(task, isUp, isDown)
	if action == OutOfRangeActionNone {
		return
	}

	alertBoundary := alertLines.Upper
	directionText := "breakout above range"
	if isDown {
		alertBoundary = alertLines.Lower
		directionText = "breakdown below range"
	}

	actionText := "再平衡"
	if action == OutOfRangeActionExit {
		actionText = "exit liquidity and stop task"
	}

	if isFirstTimeOutOfRange {
		if s.extraNotificationsEnabled(task.UserID) {
			s.notify(task.UserID, fmt.Sprintf("Task #%d %s\n%s\n%s\nAction will run after %s: %s",
				task.ID,
				directionText,
				alertLines.Current,
				alertBoundary,
				FormatDelayTime(task.ReopenDelaySeconds),
				actionText,
			))
		}
		log.Printf("[Strategy] task #%d %s, countdown=%ds action=%s", task.ID, directionText, task.ReopenDelaySeconds, actionText)
	}

	threshold := time.Duration(task.ReopenDelaySeconds) * time.Second
	if duration < threshold {
		return
	}

	if action == OutOfRangeActionExit {
		s.executeOutOfRangeStop(task, now, fmt.Sprintf("%s out of range: exit liquidity and stop task", directionText))
		return
	}

	s.executeRebalance(task, currentTick, now, fmt.Sprintf("%s出区间：执行再平衡", directionText))
	return

}

func ShouldDelayOutOfRangeHandling(task *models.StrategyTask) bool {
	return task != nil && task.RangeActivationPending
}

func ShouldExitFollowDownside(task *models.StrategyTask, isDown bool) bool {
	return task != nil && task.IsFollow && isDown
}

func shouldMonitorPausedDCA(task *models.StrategyTask) bool {
	return task != nil &&
		task.Paused &&
		task.Status == models.StrategyStatusRunning &&
		task.DCAEnabled &&
		task.DCANextBatchAt != nil
}

func ShouldStopOutOfRange(task *models.StrategyTask) bool {
	return task != nil && models.ResolveStrategyOutOfRangeMode(task) == models.StrategyOutOfRangeModeExitAll
}

func ResolveOutOfRangeAction(task *models.StrategyTask, isUp, isDown bool) OutOfRangeAction {
	if task == nil || (!isUp && !isDown) {
		return OutOfRangeActionNone
	}

	switch models.ResolveStrategyOutOfRangeMode(task) {
	case models.StrategyOutOfRangeModeExitAll:
		return OutOfRangeActionExit
	case models.StrategyOutOfRangeModeRebalanceUpExitDown:
		if isDown {
			return OutOfRangeActionExit
		}
		if isUp {
			return OutOfRangeActionRebalance
		}
	case models.StrategyOutOfRangeModeRebalanceAll:
		return OutOfRangeActionRebalance
	}

	return OutOfRangeActionNone
}

func formatDelayTime(seconds int) string {
	if seconds <= 0 {
		return "immediately"
	}
	if seconds < 60 {
		return fmt.Sprintf("%d sec", seconds)
	}
	return fmt.Sprintf("%d min", seconds/60)
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
		log.Printf("[Strategy] task #%d waiting period elapsed, scheduling reopen", task.ID)
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
		log.Printf("[Strategy] task #%d load current tick failed: %v", task.ID, err)
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
		log.Printf("[Strategy] task #%d waiting reopen enter failed: %v", task.ID, err)
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
		log.Printf("[Strategy] task #%d save waiting reopen state failed: %v", task.ID, dbErr)
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
			log.Printf("[Strategy] task #%d save critical waiting reopen state failed: %v", task.ID, cErr)
		}
	}

	log.Printf("[Strategy] task #%d waiting reopen completed", task.ID)
}

// Mock functions for V4 until V4 contract is ready
func (s *StrategyService) mockV4Remove(task *models.StrategyTask) error {
	time.Sleep(1 * time.Second)
	log.Printf("[Strategy] [MOCK V4] remove liquidity for pool=%s", task.PoolId)
	return nil
}

func (s *StrategyService) mockV4Add(task *models.StrategyTask) error {
	time.Sleep(1 * time.Second)
	log.Printf("[Strategy] [MOCK V4] add liquidity from USDT for pool=%s", task.PoolId)
	return nil
}

func (s *StrategyService) mockV3Add(task *models.StrategyTask) error {
	time.Sleep(1 * time.Second)
	log.Printf("[Strategy] [MOCK V3] add liquidity from USDT for pool=%s", task.PoolId)
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
		info, err = s.poolService.GetPoolInfoForVersionCached(chain, "v4", poolID)
	default:
		info, err = s.poolService.GetPoolInfoForVersionCached(chain, "v3", poolID)
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
	return tickLower, tickUpper, nil
}

func (s *StrategyService) executeStopLoss(task *models.StrategyTask, now time.Time, reason string) {
	if task.ExitGiveUpAt != nil {
		return
	}

	log.Printf("[Strategy] task #%d %s, execute stop loss", task.ID, reason)
	s.requestExitToUSDT(task, ExitActionStopLoss, reason)
}

func (s *StrategyService) executeOutOfRangeStop(task *models.StrategyTask, now time.Time, reason string) {
	if task == nil || task.ExitGiveUpAt != nil {
		return
	}

	log.Printf("[Strategy] task #%d %s, execute exit and stop", task.ID, reason)
	s.requestExitToUSDT(task, ExitActionOutOfRangeStop, reason)
}

func (s *StrategyService) executeRebalance(task *models.StrategyTask, currentTick int, now time.Time, reason string) {
	if task.ExitGiveUpAt != nil {
		return
	}

	log.Printf("[Strategy] task #%d %s, execute rebalance", task.ID, reason)
	s.requestExitToUSDT(task, ExitActionRebalance, reason)
}
