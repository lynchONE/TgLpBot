package smart_money

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/rpcpool"
	"context"
	"fmt"
	"log"
	"math"

	"gorm.io/gorm"
)

type Service struct {
	repo    *Repository
	watcher *Watcher
	cancel  context.CancelFunc
}

type MonitorStatus struct {
	MonitorEnabled bool `json:"monitor_enabled"`
	WatcherEnabled bool `json:"watcher_enabled"`
	CrawlerEnabled bool `json:"crawler_enabled"`
}

func NewService() *Service {
	cfg := config.AppConfig
	if cfg == nil || !cfg.SmartMoneyEnabled {
		return &Service{repo: NewRepository()}
	}

	repo := NewRepository()
	ctx := context.Background()

	var watcher *Watcher
	if hasSmartMoneyRPC(ctx, rpcpool.TransportHTTP) {
		watcher = NewWatcher(
			repo,
			cfg.BSCChainID,
			cfg.PancakeV3PositionManagerAddress,
			cfg.UniswapV3PositionManagerAddress,
			cfg.UniswapV4PoolManagerAddress,
			cfg.SmartMoneyPollInterval,
		)
	}

	return &Service{
		repo:    repo,
		watcher: watcher,
	}
}

func (s *Service) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	if s.watcher != nil {
		go s.watcher.Start(ctx)
		log.Println("[SmartMoney] Unified watcher started")
	}
}

func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.watcher != nil {
		s.watcher.Stop()
	}
}

func (s *Service) SetNotifier(fn func(*models.SmartMoneyLPEvent)) {
	if s.watcher != nil {
		s.watcher.SetNotifier(fn)
	}
}

func (s *Service) Repo() *Repository {
	return s.repo
}

func (s *Service) Status() MonitorStatus {
	if s == nil {
		return MonitorStatus{}
	}

	status := MonitorStatus{}
	hasHTTP := hasSmartMoneyRPC(context.Background(), rpcpool.TransportHTTP)
	watcherActive := s.watcher != nil && hasHTTP
	if watcherActive && s.watcher.hasLPContracts() {
		status.WatcherEnabled = true
	}
	if s.watcher != nil && hasHTTP {
		// Keep crawler_enabled for API compatibility. It now means the
		// unified watcher can monitor watched-contract interactions.
		status.CrawlerEnabled = true
	}
	status.MonitorEnabled = status.WatcherEnabled || status.CrawlerEnabled
	return status
}

func (s *Service) WriteEventInTx(ctx context.Context, event *models.SmartMoneyLPEvent) error {
	return s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.InsertLPEvent(tx, event); err != nil {
			return err
		}
		return s.repo.UpsertLPPosition(tx, event)
	})
}

func TickToPrice(tick int, token0Decimals, token1Decimals int) float64 {
	raw := math.Pow(1.0001, float64(tick))
	return raw * math.Pow(10, float64(token0Decimals-token1Decimals))
}

func FormatFeeTier(feeTier *int) string {
	if feeTier == nil {
		return ""
	}
	f := *feeTier
	switch f {
	case 100:
		return "0.01%"
	case 500:
		return "0.05%"
	case 2500:
		return "0.25%"
	case 3000:
		return "0.3%"
	case 10000:
		return "1%"
	default:
		pct := float64(f) / 10000.0
		return fmt.Sprintf("%.4g%%", pct)
	}
}
