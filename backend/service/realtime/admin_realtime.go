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

type AdminOnlineUser struct {
	UserID     uint      `json:"user_id" gorm:"column:user_id"`
	TelegramID int64     `json:"telegram_id" gorm:"column:telegram_id"`
	Username   string    `json:"username" gorm:"column:username"`
	FirstName  string    `json:"first_name" gorm:"column:first_name"`
	LastName   string    `json:"last_name" gorm:"column:last_name"`
	TotalTasks int       `json:"total_tasks" gorm:"column:total_tasks"`
	UpdatedAt  time.Time `json:"updated_at" gorm:"column:updated_at"`
}

type AdminActiveTask struct {
	TaskID        uint      `json:"task_id" gorm:"column:task_id"`
	UserID        uint      `json:"user_id" gorm:"column:user_id"`
	TelegramID    int64     `json:"telegram_id" gorm:"column:telegram_id"`
	Username      string    `json:"username" gorm:"column:username"`
	FirstName     string    `json:"first_name" gorm:"column:first_name"`
	LastName      string    `json:"last_name" gorm:"column:last_name"`
	PoolID        string    `json:"pool_id" gorm:"column:pool_id"`
	PoolVersion   string    `json:"pool_version" gorm:"column:pool_version"`
	Token0Symbol  string    `json:"token0_symbol" gorm:"column:token0_symbol"`
	Token1Symbol  string    `json:"token1_symbol" gorm:"column:token1_symbol"`
	Fee           int       `json:"fee" gorm:"column:fee"`
	Status        string    `json:"status" gorm:"column:status"`
	Paused        bool      `json:"paused" gorm:"column:paused"`
	AmountUSDT    float64   `json:"amount_usdt" gorm:"column:amount_usdt"`
	CreatedAt     time.Time `json:"created_at" gorm:"column:created_at"`
	LastCheckTime time.Time `json:"last_check_time" gorm:"column:last_check_time"`
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
			COUNT(st.id) AS active_tasks,
			COALESCE(MAX(st.updated_at), MAX(st.created_at)) AS updated_at,
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

func (s *AdminRealtimeService) ListAllOnlineUsers(limit int) ([]AdminOnlineUser, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	statuses := []models.StrategyStatus{
		models.StrategyStatusOpening,
		models.StrategyStatusRunning,
		models.StrategyStatusWaiting,
		models.StrategyStatusStopping,
	}

	var rows []AdminOnlineUser
	err := database.DB.Table("users u").
		Select(`u.id AS user_id,
			u.telegram_id AS telegram_id,
			u.username AS username,
			u.first_name AS first_name,
			u.last_name AS last_name,
			COUNT(st.id) AS total_tasks,
			COALESCE(MAX(st.updated_at), MAX(st.created_at)) AS updated_at`).
		Joins("LEFT JOIN strategy_tasks st ON st.user_id = u.id AND st.status IN ? AND st.deleted_at IS NULL", statuses).
		Where("st.id IS NOT NULL").
		Group("u.id, u.telegram_id, u.username, u.first_name, u.last_name").
		Order("updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *AdminRealtimeService) ListAllActiveTasks(limit int) ([]AdminActiveTask, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	statuses := []models.StrategyStatus{
		models.StrategyStatusOpening,
		models.StrategyStatusRunning,
		models.StrategyStatusWaiting,
		models.StrategyStatusStopping,
	}

	var rows []AdminActiveTask
	err := database.DB.Table("strategy_tasks st").
		Select(`st.id AS task_id,
			st.user_id AS user_id,
			u.telegram_id AS telegram_id,
			u.username AS username,
			u.first_name AS first_name,
			u.last_name AS last_name,
			st.pool_id AS pool_id,
			st.pool_version AS pool_version,
			st.token0_symbol AS token0_symbol,
			st.token1_symbol AS token1_symbol,
			st.fee AS fee,
			st.status AS status,
			st.paused AS paused,
			st.amount_usdt AS amount_usdt,
			st.created_at AS created_at,
			st.last_check_time AS last_check_time`).
		Joins("LEFT JOIN users u ON u.id = st.user_id").
		Where("st.status IN ? AND st.deleted_at IS NULL", statuses).
		Order("st.updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
