package auto_lp

import 
import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/strategy"
	"errors"
	"fmt"
	"strings"
	"time"
)

type AdminAutoLPService struct{}

func NewAdminAutoLPService() *AdminAutoLPService {
	return &AdminAutoLPService{}
}

type AdminAutoLPDisableResult struct {
	UserID           uint `json:"user_id"`
	ConfigWasEnabled bool `json:"config_was_enabled"`
	ConfigUpdated    bool `json:"config_updated"`

	TasksFound     int `json:"tasks_found"`
	ExitRequested  int `json:"exit_requested"`
	TasksStopped   int `json:"tasks_stopped"`
	TasksUnchanged int `json:"tasks_unchanged"`
}

func (s *AdminAutoLPService) GetUserStats(userID uint) (*models.AutoLPUserConfig, *AutoLPStats, error) {
	if userID == 0 {
		return nil, nil, fmt.Errorf("invalid userID")
	}

	cfgService := NewAutoLPUserConfigService()
	cfg, err := cfgService.GetOrCreate(userID)
	if err != nil {
		return nil, nil, err
	}

	statsService := NewAutoLPStatsService()
	stats, err := statsService.GetUserStats(userID, cfg)
	if err != nil {
		return cfg, nil, err
	}

	return cfg, stats, nil
}

func (s *AdminAutoLPService) DisableUserAutoLP(userID uint, reason string, gasMultiplier float64) (*AdminAutoLPDisableResult, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	if userID == 0 {
		return nil, fmt.Errorf("invalid userID")
	}
	if gasMultiplier <= 0 {
		gasMultiplier = 1.0
	}

	cfgService := NewAutoLPUserConfigService()
	cfg, err := cfgService.GetOrCreate(userID)
	if err != nil {
		return nil, err
	}

	out := &AdminAutoLPDisableResult{
		UserID:           userID,
		ConfigWasEnabled: cfg.Enabled,
	}

	if cfg.Enabled {
		now := time.Now()
		if _, err := cfgService.Update(userID, map[string]interface{}{
			"enabled":          false,
			"last_disabled_at": now,
		}); err != nil {
			return nil, err
		}
		out.ConfigUpdated = true
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "🛑 管理员已关闭 AutoLP"
	}

	var tasks []models.StrategyTask
	if err := database.DB.Where("user_id = ? AND is_auto = ? AND status IN ?", userID, true, []models.StrategyStatus{
		models.StrategyStatusRunning,
		models.StrategyStatusWaiting,
	}).Find(&tasks).Error; err != nil {
		return nil, err
	}
	out.TasksFound = len(tasks)

	now := time.Now()
	var firstErr error

	for i := range tasks {
		task := &tasks[i]

		if hasTaskPositionForExit(task) {
			pending := strings.TrimSpace(task.ExitPendingAction)
			if pending != "" && pending != strategy.ExitActionRebalance {
				out.TasksUnchanged++
				continue
			}

			updates := map[string]interface{}{
				"exit_pending_action":     strategy.ExitActionManualStop,
				"exit_pending_reason":     reason,
				"exit_gas_multiplier":     gasMultiplier,
				"exit_retry_count":        0,
				"exit_next_retry_at":      nil,
				"exit_last_error":         "",
				"exit_give_up_at":         nil,
				"rebalance_pending":       false,
				"rebalance_retry_count":   0,
				"rebalance_next_retry_at": nil,
				"rebalance_last_error":    "",
				"error_message":           "",
				"out_of_range_since":      nil,
				"last_check_time":         now,
			}
			if err := database.DB.Model(task).Updates(updates).Error; err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			out.ExitRequested++
			continue
		}

		updates := map[string]interface{}{
			"status":                  models.StrategyStatusStopped,
			"out_of_range_since":      nil,
			"error_message":           "",
			"exit_pending_action":     "",
			"exit_pending_reason":     "",
			"exit_retry_count":        0,
			"exit_next_retry_at":      nil,
			"exit_last_error":         "",
			"exit_give_up_at":         nil,
			"rebalance_pending":       false,
			"rebalance_retry_count":   0,
			"rebalance_next_retry_at": nil,
			"rebalance_last_error":    "",
		}
		if err := database.DB.Model(task).Updates(updates).Error; err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out.TasksStopped++
	}

	return out, firstErr
}
