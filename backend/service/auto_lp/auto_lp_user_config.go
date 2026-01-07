package auto_lp

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type AutoLPUserConfigService struct{}

func NewAutoLPUserConfigService() *AutoLPUserConfigService {
	return &AutoLPUserConfigService{}
}

func (s *AutoLPUserConfigService) GetOrCreate(userID uint) (*models.AutoLPUserConfig, error) {
	var cfg models.AutoLPUserConfig
	err := database.DB.Where("user_id = ?", userID).First(&cfg).Error
	if err == nil {
		return &cfg, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("query autolp config failed: %w", err)
	}

	cfg = models.AutoLPUserConfig{
		UserID:                  userID,
		Enabled:                 false,
		TotalAmountUSDT:         0,
		StopLossUSDT:            0,
		TakeProfitUSDT:          0,
		MaxActiveTasks:          1,
		SwitchMinImprovementPct: 0,
		SwitchCooldownSeconds:   300,
	}
	if err := database.DB.Create(&cfg).Error; err != nil {
		return nil, fmt.Errorf("create autolp config failed: %w", err)
	}
	return &cfg, nil
}

func (s *AutoLPUserConfigService) Update(userID uint, updates map[string]interface{}) (*models.AutoLPUserConfig, error) {
	cfg, err := s.GetOrCreate(userID)
	if err != nil {
		return nil, err
	}
	if err := database.DB.Model(cfg).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update autolp config failed: %w", err)
	}
	return s.GetOrCreate(userID)
}

func (s *AutoLPUserConfigService) ListEnabled() ([]models.AutoLPUserConfig, error) {
	var cfgs []models.AutoLPUserConfig
	if err := database.DB.Where("enabled = ?", true).Find(&cfgs).Error; err != nil {
		return nil, fmt.Errorf("list enabled autolp configs failed: %w", err)
	}
	return cfgs, nil
}
