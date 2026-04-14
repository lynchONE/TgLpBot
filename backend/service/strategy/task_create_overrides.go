package strategy

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"fmt"
)

func CreateTaskRecord(task *models.StrategyTask) error {
	if task == nil {
		return nil
	}
	return database.DB.Create(task).Error
}

func BuildTaskCreateOverrides(task *models.StrategyTask) map[string]interface{} {
	return task.CreateOverrideUpdates()
}

func ApplyTaskCreateOverrides(task *models.StrategyTask) error {
	if task == nil || task.ID == 0 {
		return nil
	}

	if err := task.ApplyCreateOverrides(database.DB); err != nil {
		return fmt.Errorf("apply task create overrides failed: %w", err)
	}
	return nil
}
