package services

import (
	"TgLpBot/database"
	"TgLpBot/models"
	"errors"
	"time"
)

type AdminRealtimeService struct{}

func NewAdminRealtimeService() *AdminRealtimeService {
	return &AdminRealtimeService{}
}

type AdminActiveUser struct {
	UserID      uint      `json:"user_id" gorm:"column:user_id"`
	TelegramID  int64     `json:"telegram_id" gorm:"column:telegram_id"`
	Username    string    `json:"username" gorm:"column:username"`
	FirstName   string    `json:"first_name" gorm:"column:first_name"`
	LastName    string    `json:"last_name" gorm:"column:last_name"`
	ActiveTasks int       `json:"active_tasks" gorm:"column:active_tasks"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (s *AdminRealtimeService) ListActiveTaskUsers(limit int) ([]AdminActiveUser, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	statuses := []models.StrategyStatus{
		models.StrategyStatusRunning,
		models.StrategyStatusWaiting,
		models.StrategyStatusStopping,
	}

	var rows []AdminActiveUser
	err := database.DB.Table("strategy_tasks st").
		Select(`st.user_id AS user_id,
			COUNT(*) AS active_tasks,
			MAX(st.updated_at) AS updated_at,
			u.telegram_id AS telegram_id,
			u.username AS username,
			u.first_name AS first_name,
			u.last_name AS last_name`).
		Joins("LEFT JOIN users u ON u.id = st.user_id").
		Where("st.status IN ? AND st.deleted_at IS NULL", statuses).
		Group("st.user_id, u.telegram_id, u.username, u.first_name, u.last_name").
		Order("updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
