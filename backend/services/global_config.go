package services

import (
	"TgLpBot/database"
	"TgLpBot/models"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type GlobalConfigService struct{}

func NewGlobalConfigService() *GlobalConfigService {
	return &GlobalConfigService{}
}

func (s *GlobalConfigService) GetOrCreate(userID uint) (*models.GlobalConfig, error) {
	var cfg models.GlobalConfig
	err := database.DB.Where("user_id = ?", userID).First(&cfg).Error
	if err == nil {
		return &cfg, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("query global config failed: %w", err)
	}

	cfg = models.GlobalConfig{
		UserID:                    userID,
		RebalanceTimeout:          300,
		StopLossThreshold:         10.0,
		StopLossEnabled:           false,
		StopLossDelaySeconds:      0,
		SlippageTolerance:         0.5,
		AutoReinvest:              false,
		ResidualTolerance:         1.0,
		ExtraNotificationsEnabled: true,
	}
	if err := database.DB.Create(&cfg).Error; err != nil {
		return nil, fmt.Errorf("create global config failed: %w", err)
	}
	return &cfg, nil
}

func (s *GlobalConfigService) Update(userID uint, updates map[string]interface{}) (*models.GlobalConfig, error) {
	cfg, err := s.GetOrCreate(userID)
	if err != nil {
		return nil, err
	}

	if err := database.DB.Model(cfg).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update global config failed: %w", err)
	}

	return s.GetOrCreate(userID)
}
