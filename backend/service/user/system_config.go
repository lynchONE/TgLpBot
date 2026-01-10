package user

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// SystemConfigService 管理系统级配置（单例）
type SystemConfigService struct{}

// NewSystemConfigService 创建服务实例
func NewSystemConfigService() *SystemConfigService {
	return &SystemConfigService{}
}

// GetOrCreate 获取系统配置，如不存在则创建默认值
func (s *SystemConfigService) GetOrCreate() (*models.SystemConfig, error) {
	var cfg models.SystemConfig
	err := database.DB.First(&cfg).Error
	if err == nil {
		return &cfg, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("查询系统配置失败: %w", err)
	}

	// 创建默认配置（所有值为0，使用环境变量作为默认值）
	cfg = models.SystemConfig{}
	if err := database.DB.Create(&cfg).Error; err != nil {
		return nil, fmt.Errorf("创建系统配置失败: %w", err)
	}
	return &cfg, nil
}

// Update 更新系统配置
func (s *SystemConfigService) Update(updates map[string]interface{}) (*models.SystemConfig, error) {
	cfg, err := s.GetOrCreate()
	if err != nil {
		return nil, err
	}

	if err := database.DB.Model(cfg).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("更新系统配置失败: %w", err)
	}

	return s.GetOrCreate()
}

// GetHardFilterConfig 获取硬筛配置，优先使用数据库配置，回退到环境变量
func (s *SystemConfigService) GetHardFilterConfig() (*models.HardFilterConfig, error) {
	cfg, err := s.GetOrCreate()
	if err != nil {
		return nil, err
	}

	hf := &models.HardFilterConfig{}

	// TVL阈值：数据库值 > 0 则使用，否则使用环境变量
	if cfg.AutoLPMinPoolValueUSD > 0 {
		hf.MinPoolValueUSD = cfg.AutoLPMinPoolValueUSD
	} else if config.AppConfig != nil {
		hf.MinPoolValueUSD = config.AppConfig.AutoLPMinPoolValueUSD
	}

	// 费率阈值
	if cfg.AutoLPMinFeePercentage > 0 {
		hf.MinFeePercentage = cfg.AutoLPMinFeePercentage
	} else if config.AppConfig != nil {
		hf.MinFeePercentage = config.AppConfig.AutoLPMinFeePercentage
	}

	// 5分钟费用率阈值
	if cfg.AutoLPMinFeeRate5m > 0 {
		hf.MinFeeRate5m = cfg.AutoLPMinFeeRate5m
	} else if config.AppConfig != nil {
		hf.MinFeeRate5m = config.AppConfig.AutoLPMinFeeRate5m
	}

	// 5分钟手续费阈值
	if cfg.AutoLPMinTotalFees5m > 0 {
		hf.MinTotalFees5m = cfg.AutoLPMinTotalFees5m
	} else if config.AppConfig != nil {
		hf.MinTotalFees5m = config.AppConfig.AutoLPMinTotalFees5m
	}

	// 5分钟成交量阈值
	if cfg.AutoLPMinTotalVolume5m > 0 {
		hf.MinTotalVolume5m = cfg.AutoLPMinTotalVolume5m
	} else if config.AppConfig != nil {
		hf.MinTotalVolume5m = config.AppConfig.AutoLPMinTotalVolume5m
	}

	// 5分钟交易笔数阈值
	if cfg.AutoLPMinTx5m > 0 {
		hf.MinTx5m = cfg.AutoLPMinTx5m
	} else if config.AppConfig != nil {
		hf.MinTx5m = config.AppConfig.AutoLPMinTx5m
	}

	return hf, nil
}
