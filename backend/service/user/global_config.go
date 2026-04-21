package user

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
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
		MultiChainEnabled:         true,
		DefaultChain:              "bsc",
		MultiWalletEnabled:        false,
		RebalanceTimeout:          10,
		StopLossThreshold:         10.0,
		StopLossEnabled:           false,
		StopLossDelaySeconds:      0,
		SlippageTolerance:         0.5,
		AutoReinvest:              false,
		ExtraNotificationsEnabled: true,
		FilterChineseTokens:       false,
		BarkEnabled:               false,
		BarkKeyEncrypted:          "",
		BarkServer:                "",
		BarkGroup:                 "",
		DCAMinSplitAmountUSDT:     0,
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

	oldRebalanceTimeout := cfg.RebalanceTimeout
	newRebalanceTimeout, syncTaskRebalanceTimeout := rebalanceTimeoutUpdate(updates)

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(cfg).Updates(updates).Error; err != nil {
			return fmt.Errorf("update global config failed: %w", err)
		}

		if syncTaskRebalanceTimeout && newRebalanceTimeout != oldRebalanceTimeout {
			if err := tx.Model(&models.StrategyTask{}).
				Where("user_id = ? AND status <> ? AND reopen_delay_seconds = ?", userID, models.StrategyStatusStopped, oldRebalanceTimeout).
				Update("reopen_delay_seconds", newRebalanceTimeout).Error; err != nil {
				return fmt.Errorf("sync task rebalance timeout failed: %w", err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return s.GetOrCreate(userID)
}

func rebalanceTimeoutUpdate(updates map[string]interface{}) (int, bool) {
	if len(updates) == 0 {
		return 0, false
	}
	raw, ok := updates["rebalance_timeout"]
	if !ok {
		return 0, false
	}

	switch v := raw.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

func (s *GlobalConfigService) ResolveOpenPositionSizingConfig(userID uint) (*models.OpenPositionSizingConfig, error) {
	sysCfg, err := NewSystemConfigService().GetOpenPositionSizingConfig()
	if err != nil {
		return nil, err
	}

	cfg, err := s.GetOrCreate(userID)
	if err != nil {
		return nil, err
	}

	out := *sysCfg
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
	return normalizeOpenPositionSizingConfig(&out), nil
}
