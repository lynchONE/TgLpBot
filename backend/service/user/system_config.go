package user

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type SystemConfigService struct{}

const (
	defaultOpenPositionTargetShareMin = 0.20
	defaultOpenPositionTargetShareMax = 0.65
	defaultOpenPositionRiskCapUSD     = 500.0
	defaultOpenPositionRiskCapRatio   = 0.20
)

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

	priceDeviationDefault := 1.0
	minLiquidityDefault := 1000.0
	if config.AppConfig != nil {
		if config.AppConfig.ZapPriceDeviationMaxPercent > 0 {
			priceDeviationDefault = config.AppConfig.ZapPriceDeviationMaxPercent
		}
		if config.AppConfig.ZapMinPoolLiquidityUSD > 0 {
			minLiquidityDefault = config.AppConfig.ZapMinPoolLiquidityUSD
		}
	}

	out := &models.ZapSafetyConfig{
		PriceDeviationMaxPercent: priceDeviationDefault,
		MinPoolLiquidityUSD:      minLiquidityDefault,
	}
	if cfg.ZapPriceDeviationMaxPercent > 0 {
		out.PriceDeviationMaxPercent = cfg.ZapPriceDeviationMaxPercent
	}
	if cfg.ZapMinPoolLiquidityUSD > 0 {
		out.MinPoolLiquidityUSD = cfg.ZapMinPoolLiquidityUSD
	}
	return out, nil
}

func (s *SystemConfigService) GetOpenPositionSizingConfig() (*models.OpenPositionSizingConfig, error) {
	cfg, err := s.GetOrCreate()
	if err != nil {
		return nil, err
	}

	out := s.DefaultOpenPositionSizingConfig()
	if cfg.OpenPositionTargetShareMin > 0 {
		out.TargetShareMin = cfg.OpenPositionTargetShareMin
	}
	if cfg.OpenPositionTargetShareMax > 0 {
		out.TargetShareMax = cfg.OpenPositionTargetShareMax
	}
	if cfg.OpenPositionRiskCapUSD > 0 {
		out.RiskCapUSD = cfg.OpenPositionRiskCapUSD
	}
	if cfg.OpenPositionRiskCapRatio > 0 {
		out.RiskCapRatio = cfg.OpenPositionRiskCapRatio
	}
	return normalizeOpenPositionSizingConfig(out), nil
}

func (s *SystemConfigService) DefaultOpenPositionSizingConfig() *models.OpenPositionSizingConfig {
	return normalizeOpenPositionSizingConfig(&models.OpenPositionSizingConfig{
		TargetShareMin: defaultOpenPositionTargetShareMin,
		TargetShareMax: defaultOpenPositionTargetShareMax,
		RiskCapUSD:     defaultOpenPositionRiskCapUSD,
		RiskCapRatio:   defaultOpenPositionRiskCapRatio,
	})
}

func normalizeOpenPositionSizingConfig(cfg *models.OpenPositionSizingConfig) *models.OpenPositionSizingConfig {
	if cfg == nil {
		cfg = &models.OpenPositionSizingConfig{}
	}
	out := *cfg

	if out.TargetShareMin <= 0 {
		out.TargetShareMin = defaultOpenPositionTargetShareMin
	}
	if out.TargetShareMax <= 0 {
		out.TargetShareMax = defaultOpenPositionTargetShareMax
	}
	if out.TargetShareMin > out.TargetShareMax {
		out.TargetShareMin, out.TargetShareMax = out.TargetShareMax, out.TargetShareMin
	}
	if out.TargetShareMin <= 0 {
		out.TargetShareMin = defaultOpenPositionTargetShareMin
	}
	if out.TargetShareMax <= 0 {
		out.TargetShareMax = defaultOpenPositionTargetShareMax
	}
	if out.RiskCapUSD <= 0 {
		out.RiskCapUSD = defaultOpenPositionRiskCapUSD
	}
	if out.RiskCapRatio <= 0 {
		out.RiskCapRatio = defaultOpenPositionRiskCapRatio
	}

	return &out
}
