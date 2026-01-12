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

// AdminActiveUser 原始结构（仅 Auto 用户）
type AdminActiveUser struct {
	UserID      uint      `json:"user_id" gorm:"column:user_id"`
	TelegramID  int64     `json:"telegram_id" gorm:"column:telegram_id"`
	Username    string    `json:"username" gorm:"column:username"`
	FirstName   string    `json:"first_name" gorm:"column:first_name"`
	LastName    string    `json:"last_name" gorm:"column:last_name"`
	ActiveTasks int       `json:"active_tasks" gorm:"column:active_tasks"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"column:updated_at"`
}

// AdminOnlineUser 在线用户（包含 Auto 和手动任务）
type AdminOnlineUser struct {
	UserID        uint      `json:"user_id" gorm:"column:user_id"`
	TelegramID    int64     `json:"telegram_id" gorm:"column:telegram_id"`
	Username      string    `json:"username" gorm:"column:username"`
	FirstName     string    `json:"first_name" gorm:"column:first_name"`
	LastName      string    `json:"last_name" gorm:"column:last_name"`
	AutoTasks     int       `json:"auto_tasks" gorm:"column:auto_tasks"`
	ManualTasks   int       `json:"manual_tasks" gorm:"column:manual_tasks"`
	TotalTasks    int       `json:"total_tasks" gorm:"column:total_tasks"`
	IsAutoEnabled bool      `json:"is_auto_enabled" gorm:"column:is_auto_enabled"`
	UpdatedAt     time.Time `json:"updated_at" gorm:"column:updated_at"`
}

// AdminActiveTask 活跃任务
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
	IsAuto        bool      `json:"is_auto" gorm:"column:is_auto"`
	Status        string    `json:"status" gorm:"column:status"`
	Paused        bool      `json:"paused" gorm:"column:paused"`
	AmountUSDT    float64   `json:"amount_usdt" gorm:"column:amount_usdt"`
	CreatedAt     time.Time `json:"created_at" gorm:"column:created_at"`
	LastCheckTime time.Time `json:"last_check_time" gorm:"column:last_check_time"`
}

// ListActiveTaskUsers 列出开启 Auto 的用户（原有方法保留兼容）
func (s *AdminRealtimeService) ListActiveTaskUsers(limit int) ([]AdminActiveUser, error) {
	if database.DB == nil {
		return nil, errors.New("数据库未初始化")
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

// ListAllOnlineUsers 列出所有在线用户：
// - 有活跃任务的用户（包括 Auto 和手动）
// - 或开启了 Auto 的用户（即使当前暂无活跃任务）
func (s *AdminRealtimeService) ListAllOnlineUsers(limit int) ([]AdminOnlineUser, error) {
	if database.DB == nil {
		return nil, errors.New("数据库未初始化")
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
			SUM(CASE WHEN st.is_auto = 1 THEN 1 ELSE 0 END) AS auto_tasks,
			SUM(CASE WHEN st.is_auto = 0 THEN 1 ELSE 0 END) AS manual_tasks,
			COUNT(st.id) AS total_tasks,
			COALESCE(cfg.enabled, 0) AS is_auto_enabled,
			COALESCE(MAX(st.updated_at), COALESCE(cfg.last_enabled_at, cfg.updated_at)) AS updated_at`).
		Joins("LEFT JOIN auto_lp_user_configs cfg ON cfg.user_id = u.id AND cfg.deleted_at IS NULL").
		Joins("LEFT JOIN strategy_tasks st ON st.user_id = u.id AND st.status IN ? AND st.deleted_at IS NULL", statuses).
		Where("(st.id IS NOT NULL OR COALESCE(cfg.enabled, 0) = 1)").
		Group("u.id, u.telegram_id, u.username, u.first_name, u.last_name, cfg.enabled, cfg.last_enabled_at, cfg.updated_at").
		Order("updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// ListAllActiveTasks 列出所有活跃任务
func (s *AdminRealtimeService) ListAllActiveTasks(limit int) ([]AdminActiveTask, error) {
	if database.DB == nil {
		return nil, errors.New("数据库未初始化")
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
			st.is_auto AS is_auto,
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
