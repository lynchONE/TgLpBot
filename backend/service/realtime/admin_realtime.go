package realtime

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
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
	err := database.DB.Table("auto_lp_user_configs cfg").
		Select(`cfg.user_id AS user_id,
			COUNT(st.id) AS active_tasks,
			COALESCE(cfg.last_enabled_at, cfg.updated_at) AS updated_at,
			u.telegram_id AS telegram_id,
			u.username AS username,
			u.first_name AS first_name,
			u.last_name AS last_name`).
		Joins("LEFT JOIN users u ON u.id = cfg.user_id").
		Joins("LEFT JOIN strategy_tasks st ON st.user_id = cfg.user_id AND st.is_auto = 1 AND st.status IN ? AND st.deleted_at IS NULL", statuses).
		Where("cfg.enabled = 1 AND cfg.deleted_at IS NULL").
		Group("cfg.user_id, u.telegram_id, u.username, u.first_name, u.last_name, cfg.last_enabled_at, cfg.updated_at").
		Order("updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
