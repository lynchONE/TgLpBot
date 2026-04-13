package strategy

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"fmt"
)

func BuildTaskCreateOverrides(task *models.StrategyTask) map[string]interface{} {
	if task == nil {
		return nil
	}

	updates := make(map[string]interface{})

	// GORM may skip zero-values for fields with DB defaults on create.
	if task.ReopenDelaySeconds == 0 {
		updates["reopen_delay_seconds"] = 0
	}
	if task.SlippageTolerance == 0 {
		updates["slippage_tolerance"] = 0
	}
	if task.ResidualTolerance == 0 {
		updates["residual_tolerance"] = 0
	}
	if task.ZapLossTolerance == 0 {
		updates["zap_loss_tolerance"] = 0
	}

	// Old DBs may still carry a stale default; force the intended value.
	if !task.RebalanceEnabled {
		updates["rebalance_enabled"] = false
	}

	return updates
}

func ApplyTaskCreateOverrides(task *models.StrategyTask) error {
	if task == nil || task.ID == 0 {
		return nil
	}

	updates := BuildTaskCreateOverrides(task)
	if len(updates) == 0 {
		return nil
	}
	if err := database.DB.Model(task).Updates(updates).Error; err != nil {
		return fmt.Errorf("apply task create overrides failed: %w", err)
	}
	return nil
}
