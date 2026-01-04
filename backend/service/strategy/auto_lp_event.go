package strategy

import 
import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"strings"
)

type AutoLPEventService struct{}

func NewAutoLPEventService() *AutoLPEventService {
	return &AutoLPEventService{}
}

func (s *AutoLPEventService) Record(task *models.StrategyTask, eventType models.AutoLPEventType, reason string) error {
	if task == nil || database.DB == nil {
		return nil
	}
	if !task.IsAuto {
		return nil
	}

	rec := &models.AutoLPEvent{
		UserID:       task.UserID,
		TaskID:       task.ID,
		EventType:    eventType,
		Reason:       strings.TrimSpace(reason),
		PoolVersion:  strings.TrimSpace(task.PoolVersion),
		PoolId:       strings.TrimSpace(task.PoolId),
		Token0Symbol: strings.TrimSpace(task.Token0Symbol),
		Token1Symbol: strings.TrimSpace(task.Token1Symbol),
	}

	return database.DB.Create(rec).Error
}
