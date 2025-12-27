package services

import (
	"TgLpBot/database"
	"TgLpBot/models"
	"fmt"
	"strings"
)

func applyEnterResultToTask(task *models.StrategyTask, enterRes *EnterResult) error {
	if task == nil || enterRes == nil {
		return fmt.Errorf("task or enterRes is nil")
	}

	updates := map[string]interface{}{
		"current_liquidity": enterRes.CurrentLiquidity,
		"error_message":     "",
		"status":            models.StrategyStatusRunning,
	}

	if strings.TrimSpace(enterRes.V3TokenID) != "" && strings.TrimSpace(enterRes.V3TokenID) != "0" {
		updates["v3_position_manager_address"] = enterRes.V3PositionManagerAddress
		updates["v3_token_id"] = enterRes.V3TokenID
	}
	if strings.TrimSpace(enterRes.V4TokenID) != "" && strings.TrimSpace(enterRes.V4TokenID) != "0" {
		updates["v4_token_id"] = enterRes.V4TokenID
	}

	if err := database.DB.Model(task).Updates(updates).Error; err != nil {
		return err
	}

	task.CurrentLiquidity = enterRes.CurrentLiquidity
	task.ErrorMessage = ""
	task.Status = models.StrategyStatusRunning

	if strings.TrimSpace(enterRes.V3TokenID) != "" && strings.TrimSpace(enterRes.V3TokenID) != "0" {
		task.V3PositionManagerAddress = enterRes.V3PositionManagerAddress
		task.V3TokenID = enterRes.V3TokenID
	}
	if strings.TrimSpace(enterRes.V4TokenID) != "" && strings.TrimSpace(enterRes.V4TokenID) != "0" {
		task.V4TokenID = enterRes.V4TokenID
	}

	return nil
}
