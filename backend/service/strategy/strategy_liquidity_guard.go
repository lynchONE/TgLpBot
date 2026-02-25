package strategy

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

const (
	strategyLiquidityCheckInterval = 30 * time.Second
	strategyLiquidityGracePeriod   = 2 * time.Minute
)

// processNoLiquidityTask auto-stops tasks whose on-chain position has no liquidity.
// Returns true when task processing should stop for this cycle.
func (s *StrategyService) processNoLiquidityTask(task *models.StrategyTask) bool {
	if task == nil {
		return false
	}
	if task.Status != models.StrategyStatusRunning {
		return false
	}
	if task.RebalancePending {
		return false
	}
	if strings.TrimSpace(task.ExitPendingAction) != "" {
		return false
	}

	now := time.Now()
	s.lastLiquidityMu.Lock()
	last, ok := s.lastLiquidityCheck[task.ID]
	if ok && now.Sub(last) < strategyLiquidityCheckInterval {
		s.lastLiquidityMu.Unlock()
		return false
	}
	s.lastLiquidityCheck[task.ID] = now
	s.lastLiquidityMu.Unlock()

	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	tokenID := ""
	if version == "v4" {
		tokenID = strings.TrimSpace(task.V4TokenID)
	} else {
		tokenID = strings.TrimSpace(task.V3TokenID)
	}

	// If we have no tokenId at all for a long time, treat it as a dead/stale task.
	if tokenID == "" || tokenID == "0" {
		if !task.CreatedAt.IsZero() && now.Sub(task.CreatedAt) < strategyLiquidityGracePeriod {
			return false
		}
		return s.autoStopNoLiquidity(task, now, "missing position tokenId")
	}

	liq, err := s.readOnChainTaskLiquidity(task)
	if err != nil {
		log.Printf("[Strategy] 任务 #%d 流动性检查失败: %v", task.ID, err)
		return false
	}
	if liq == nil {
		liq = big.NewInt(0)
	}

	// Keep V4 current_liquidity in sync for UI/miniapp/exit logic.
	if version == "v4" && strings.TrimSpace(task.CurrentLiquidity) != liq.String() {
		_ = database.DB.Model(task).Updates(map[string]interface{}{
			"current_liquidity": liq.String(),
		}).Error
		task.CurrentLiquidity = liq.String()
	}

	if liq.Sign() <= 0 {
		return s.autoStopNoLiquidity(task, now, "position liquidity is 0")
	}
	return false
}

func (s *StrategyService) autoStopNoLiquidity(task *models.StrategyTask, now time.Time, reason string) bool {
	if task == nil {
		return false
	}

	updates := map[string]interface{}{
		"status":                  models.StrategyStatusStopped,
		"last_exit_time":          &now,
		"current_liquidity":       "0",
		"out_of_range_since":      nil,
		"rebalance_pending":       false,
		"rebalance_retry_count":   0,
		"rebalance_next_retry_at": nil,
		"rebalance_last_error":    "",
		"exit_pending_action":     "",
		"exit_pending_reason":     "",
		"exit_gas_multiplier":     1.0,
		"exit_retry_count":        0,
		"exit_next_retry_at":      nil,
		"exit_last_error":         "",
		"exit_give_up_at":         nil,
		"error_message":           "",
	}
	if err := database.DB.Model(task).Updates(updates).Error; err != nil {
		log.Printf("[Strategy] 任务 #%d 自动停止失败: %v", task.ID, err)
		return false
	}

	task.Status = models.StrategyStatusStopped
	task.LastExitTime = &now
	task.CurrentLiquidity = "0"
	task.OutOfRangeSince = nil
	task.RebalancePending = false
	task.RebalanceRetryCount = 0
	task.RebalanceNextRetryAt = nil
	task.RebalanceLastError = ""
	task.ExitPendingAction = ""
	task.ExitPendingReason = ""
	task.ExitGasMultiplier = 1.0
	task.ExitRetryCount = 0
	task.ExitNextRetryAt = nil
	task.ExitLastError = ""
	task.ExitGiveUpAt = nil
	task.ErrorMessage = ""

	msg := fmt.Sprintf("⚠️ 检测到任务 #%d 仓位已无流动性（可能已手动撤出/仓位已关闭），已自动结束任务。", task.ID)
	if strings.TrimSpace(reason) == "missing position tokenId" {
		msg = fmt.Sprintf("⚠️ 任务 #%d 长时间缺少仓位信息（tokenId），已自动结束任务。", task.ID)
	}
	s.notify(task.UserID, msg)
	log.Printf("[Strategy] 任务 #%d 自动结束：%s", task.ID, reason)
	return true
}

func (s *StrategyService) readOnChainTaskLiquidity(task *models.StrategyTask) (*big.Int, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}

	chain := config.NormalizeChain(task.Chain)
	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	switch version {
	case "v4":
		// V4 reads are still single-chain; restrict to bsc until v4 blockchain helpers are refactored.
		if chain != "bsc" {
			return nil, fmt.Errorf("v4 not supported on chain=%s", chain)
		}
		if blockchain.Client == nil {
			return nil, fmt.Errorf("blockchain client not initialized")
		}

		tokenId, err := convert.ParseBigIntFlexible(task.V4TokenID)
		if err != nil || tokenId == nil || tokenId.Sign() <= 0 {
			return nil, fmt.Errorf("invalid V4 tokenId")
		}
		cc, ok := config.AppConfig.GetChainConfig(chain)
		if !ok {
			return nil, fmt.Errorf("chain config not found: %s", chain)
		}
		pmAddrStr := strings.TrimSpace(cc.UniswapV4PositionManagerAddress)
		poolMgrStr := strings.TrimSpace(cc.UniswapV4PoolManagerAddress)
		if !common.IsHexAddress(pmAddrStr) {
			pmAddrStr = strings.TrimSpace(config.AppConfig.UniswapV4PositionManagerAddress)
		}
		if !common.IsHexAddress(poolMgrStr) {
			poolMgrStr = strings.TrimSpace(config.AppConfig.UniswapV4PoolManagerAddress)
		}
		if !common.IsHexAddress(pmAddrStr) {
			return nil, fmt.Errorf("UNISWAP_V4_POSITION_MANAGER_ADDRESS not configured")
		}
		v4pmAddr := common.HexToAddress(pmAddrStr)
		poolMgr := common.Address{}
		if common.IsHexAddress(poolMgrStr) {
			poolMgr = common.HexToAddress(poolMgrStr)
		}
		pos, err := blockchain.GetV4PositionInfo(v4pmAddr, poolMgr, task.PoolId, tokenId)
		if err != nil {
			return nil, err
		}
		if pos == nil || pos.Liquidity == nil {
			return big.NewInt(0), nil
		}
		return cloneBig(pos.Liquidity), nil
	default:
		tokenId, err := convert.ParseBigIntFlexible(task.V3TokenID)
		if err != nil || tokenId == nil || tokenId.Sign() <= 0 {
			return nil, fmt.Errorf("invalid V3 tokenId")
		}

		client, _, err := blockchain.GetEVMClient(chain)
		if err != nil {
			return nil, err
		}

		pmAddrStr := strings.TrimSpace(task.V3PositionManagerAddress)
		if !common.IsHexAddress(pmAddrStr) {
			if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
				if common.IsHexAddress(cc.DefaultV3PositionManagerAddress) {
					pmAddrStr = strings.TrimSpace(cc.DefaultV3PositionManagerAddress)
				} else {
					for _, dep := range cc.V3Deployments {
						if common.IsHexAddress(dep.PositionManagerAddress) {
							pmAddrStr = strings.TrimSpace(dep.PositionManagerAddress)
							break
						}
					}
				}
			}
		}
		if !common.IsHexAddress(pmAddrStr) {
			ex := strings.ToLower(strings.TrimSpace(task.Exchange))
			if strings.Contains(ex, "pancake") && common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
				pmAddrStr = strings.TrimSpace(config.AppConfig.PancakeV3PositionManagerAddress)
			} else {
				pmAddrStr = strings.TrimSpace(config.AppConfig.UniswapV3PositionManagerAddress)
			}
		}
		if !common.IsHexAddress(pmAddrStr) {
			return nil, fmt.Errorf("V3 position manager address not configured")
		}

		v3pm, err := blockchain.NewV3PositionManager(common.HexToAddress(pmAddrStr), client)
		if err != nil {
			return nil, fmt.Errorf("init v3 position manager failed: %w", err)
		}
		pos, err := v3pm.Positions(nil, tokenId)
		if err != nil {
			return nil, fmt.Errorf("read v3 position failed: %w", err)
		}
		if pos == nil || pos.Liquidity == nil {
			return big.NewInt(0), nil
		}
		return cloneBig(pos.Liquidity), nil
	}
}
