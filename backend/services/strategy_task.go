package services

import (
	"TgLpBot/database"
	"TgLpBot/models"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type StrategyTaskService struct{}

func NewStrategyTaskService() *StrategyTaskService {
	return &StrategyTaskService{}
}

func (s *StrategyTaskService) GetByID(userID uint, taskID uint) (*models.StrategyTask, error) {
	var task models.StrategyTask
	if err := database.DB.Where("id = ? AND user_id = ?", taskID, userID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("task not found")
		}
		return nil, fmt.Errorf("query task failed: %w", err)
	}
	return &task, nil
}

func (s *StrategyTaskService) ListActive(userID uint, limit int) ([]models.StrategyTask, error) {
	if limit <= 0 {
		limit = 10
	}
	var tasks []models.StrategyTask
	if err := database.DB.Where("user_id = ? AND status IN ?", userID, []models.StrategyStatus{
		models.StrategyStatusRunning,
		models.StrategyStatusWaiting,
		models.StrategyStatusStopping,
	}).Order("updated_at DESC").Limit(limit).Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("list tasks failed: %w", err)
	}
	return tasks, nil
}

func (s *StrategyTaskService) Update(userID uint, taskID uint, updates map[string]interface{}) error {
	task, err := s.GetByID(userID, taskID)
	if err != nil {
		return err
	}
	if err := database.DB.Model(task).Updates(updates).Error; err != nil {
		return fmt.Errorf("update task failed: %w", err)
	}
	return nil
}

func (s *StrategyTaskService) Delete(userID uint, taskID uint) error {
	task, err := s.GetByID(userID, taskID)
	if err != nil {
		return err
	}
	if err := database.DB.Delete(task).Error; err != nil {
		return fmt.Errorf("delete task failed: %w", err)
	}
	return nil
}
