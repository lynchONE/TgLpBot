package user

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type SystemConfigService struct{}

func NewSystemConfigService() *SystemConfigService {
	return &SystemConfigService{}
}

func (s *SystemConfigService) GetOrCreate() (*models.SystemConfig, error) {
	var cfg models.SystemConfig
	err := database.DB.First(&cfg).Error
	if err == nil {
		return &cfg, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("query system config failed: %w", err)
	}

	cfg = models.SystemConfig{}
	if err := database.DB.Create(&cfg).Error; err != nil {
		return nil, fmt.Errorf("create system config failed: %w", err)
	}
	return &cfg, nil
}

func (s *SystemConfigService) Update(updates map[string]interface{}) (*models.SystemConfig, error) {
	cfg, err := s.GetOrCreate()
	if err != nil {
		return nil, err
	}

	if err := database.DB.Model(cfg).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update system config failed: %w", err)
	}

	return s.GetOrCreate()
}

func (s *SystemConfigService) GetZapSafetyConfig() (*models.ZapSafetyConfig, error) {
	cfg, err := s.GetOrCreate()
	if err != nil {
		return nil, err
	}

	out := &models.ZapSafetyConfig{
		PriceDeviationMaxPercent: 1.0,
		MinPoolLiquidityUSD:      1000.0,
	}
	if cfg.ZapPriceDeviationMaxPercent > 0 {
		out.PriceDeviationMaxPercent = cfg.ZapPriceDeviationMaxPercent
	}
	if cfg.ZapMinPoolLiquidityUSD > 0 {
		out.MinPoolLiquidityUSD = cfg.ZapMinPoolLiquidityUSD
	}
	return out, nil
}
