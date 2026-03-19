package smart_money_golden_dog

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/notify"
	"TgLpBot/base/security"
	userSvc "TgLpBot/service/user"
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

const (
	DefaultMinWallets      = 3
	DefaultWindowMinutes   = 10
	DefaultCooldownMinutes = 30
	defaultScanInterval    = 20 * time.Second
)

type BarkStatus struct {
	Enabled    bool
	Configured bool
	Ready      bool
	Config     notify.BarkConfig
}

type Service struct {
	repo     *Repository
	cancel   context.CancelFunc
	interval time.Duration
}

type pairBucket struct {
	Key        string
	Label      string
	WalletSeen map[string]time.Time
}

type pairSignal struct {
	Key         string
	Label       string
	WalletCount int
	LatestAt    time.Time
}

func NewService() *Service {
	return &Service{
		repo:     NewRepository(),
		interval: defaultScanInterval,
	}
}

func (s *Service) Start() {
	if config.AppConfig == nil || !config.AppConfig.SmartMoneyEnabled {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go s.loop(ctx)
}

func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Service) loop(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	if err := s.runOnce(ctx); err != nil {
		log.Printf("[GoldenDog] initial scan failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.runOnce(ctx); err != nil {
				log.Printf("[GoldenDog] scan failed: %v", err)
			}
		}
	}
}

func (s *Service) runOnce(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return nil
	}

	configs, err := s.repo.ListEnabledConfigs(ctx)
	if err != nil {
		return fmt.Errorf("list enabled configs: %w", err)
	}
	if len(configs) == 0 {
		return nil
	}

	type chainWork struct {
		maxWindow int
		configs   []models.SmartMoneyGoldenDogConfig
		buckets   map[string]*pairBucket
	}
	workByChain := make(map[string]*chainWork)

	for _, cfg := range configs {
		chain := normalizeChain(cfg.Chain)
		work := workByChain[chain]
		if work == nil {
			work = &chainWork{}
			workByChain[chain] = work
		}
		if cfg.WindowMinutes > work.maxWindow {
			work.maxWindow = cfg.WindowMinutes
		}
		work.configs = append(work.configs, cfg)
	}

	now := time.Now()
	for chain, work := range workByChain {
		chainID := chainIDFor(chain)
		if chainID <= 0 {
			continue
		}
		window := clampWindowMinutes(work.maxWindow)
		events, err := s.repo.ListRecentAddEvents(ctx, chainID, now.Add(-time.Duration(window)*time.Minute))
		if err != nil {
			return fmt.Errorf("list recent add events chain=%s: %w", chain, err)
		}
		work.buckets = buildPairBuckets(events)
	}

	barkCache := make(map[uint]BarkStatus)
	for chain, work := range workByChain {
		for _, cfg := range work.configs {
			signals := pairSignalsForConfig(work.buckets, now, cfg)
			if len(signals) == 0 {
				continue
			}

			barkStatus, ok := barkCache[cfg.UserID]
			if !ok {
				barkStatus, err = ResolveUserBarkStatus(ctx, cfg.UserID)
				if err != nil {
					log.Printf("[GoldenDog] resolve bark status failed user=%d: %v", cfg.UserID, err)
					continue
				}
				barkCache[cfg.UserID] = barkStatus
			}
			if !barkStatus.Ready {
				continue
			}

			for _, signal := range signals {
				state, err := s.repo.GetAlertState(ctx, cfg.UserID, chain, signal.Key)
				if err != nil {
					return fmt.Errorf("load alert state user=%d chain=%s pair=%s: %w", cfg.UserID, chain, signal.Key, err)
				}
				if cooldownActive(state, now, clampCooldownMinutes(cfg.CooldownMinutes)) {
					continue
				}

				title, body := buildBarkMessage(signal, cfg)
				if err := notify.SendBarkWithConfig(title, body, barkStatus.Config); err != nil {
					log.Printf("[GoldenDog] bark notify failed user=%d pair=%s: %v", cfg.UserID, signal.Key, err)
					continue
				}

				err = s.repo.UpsertAlertState(ctx, &models.SmartMoneyGoldenDogAlertState{
					UserID:         cfg.UserID,
					Chain:          chain,
					PairKey:        signal.Key,
					PairLabel:      signal.Label,
					LastWallets:    signal.WalletCount,
					LastNotifiedAt: now,
				})
				if err != nil {
					return fmt.Errorf("save alert state user=%d chain=%s pair=%s: %w", cfg.UserID, chain, signal.Key, err)
				}
			}
		}
	}

	return nil
}

func ResolveUserBarkStatus(ctx context.Context, userID uint) (BarkStatus, error) {
	var status BarkStatus
	if config.AppConfig == nil {
		return status, fmt.Errorf("config not loaded")
	}

	cfg, err := userSvc.NewGlobalConfigService().GetOrCreate(userID)
	if err != nil {
		return status, err
	}

	status.Enabled = cfg.BarkEnabled
	status.Configured = strings.TrimSpace(cfg.BarkKeyEncrypted) != ""
	if !status.Enabled || !status.Configured {
		return status, nil
	}

	keyBytes, err := security.DecodeHexKey32(config.AppConfig.EncryptionKey)
	if err != nil {
		return status, err
	}
	plain, err := security.DecryptAESGCMHex(keyBytes, cfg.BarkKeyEncrypted)
	if err != nil {
		return status, err
	}

	status.Ready = true
	status.Config = notify.BarkConfig{
		Server: cfg.BarkServer,
		Key:    strings.TrimSpace(string(plain)),
		Group:  cfg.BarkGroup,
	}
	return status, nil
}

func normalizeChain(chain string) string {
	normalized := config.NormalizeChain(chain)
	if normalized == "" {
		return "bsc"
	}
	return normalized
}

func chainIDFor(chain string) int {
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok && cc.ChainID > 0 {
			return int(cc.ChainID)
		}
	}
	switch normalizeChain(chain) {
	case "base":
		return 8453
	default:
		return 56
	}
}

func clampMinWallets(value int) int {
	if value < 1 {
		return 1
	}
	if value > 100 {
		return 100
	}
	return value
}

func clampWindowMinutes(value int) int {
	if value < 1 {
		return DefaultWindowMinutes
	}
	if value > 1440 {
		return 1440
	}
	return value
}

func clampCooldownMinutes(value int) int {
	if value < 0 {
		return DefaultCooldownMinutes
	}
	if value > 10080 {
		return 10080
	}
	return value
}

func cooldownActive(state *models.SmartMoneyGoldenDogAlertState, now time.Time, cooldownMinutes int) bool {
	if state == nil || cooldownMinutes <= 0 {
		return false
	}
	return now.Sub(state.LastNotifiedAt) < time.Duration(cooldownMinutes)*time.Minute
}

func buildPairBuckets(events []models.SmartMoneyLPEvent) map[string]*pairBucket {
	out := make(map[string]*pairBucket)
	for _, event := range events {
		pairKey, pairLabel := canonicalPair(event.Token0Address, event.Token1Address, event.Token0Symbol, event.Token1Symbol)
		if pairKey == "" {
			continue
		}
		wallet := strings.ToLower(strings.TrimSpace(event.WalletAddress))
		if wallet == "" {
			continue
		}

		bucket := out[pairKey]
		if bucket == nil {
			bucket = &pairBucket{
				Key:        pairKey,
				Label:      pairLabel,
				WalletSeen: make(map[string]time.Time),
			}
			out[pairKey] = bucket
		}
		if bucket.Label == "" && pairLabel != "" {
			bucket.Label = pairLabel
		}
		if seenAt, ok := bucket.WalletSeen[wallet]; !ok || event.TxTimestamp.After(seenAt) {
			bucket.WalletSeen[wallet] = event.TxTimestamp
		}
	}
	return out
}

func pairSignalsForConfig(buckets map[string]*pairBucket, now time.Time, cfg models.SmartMoneyGoldenDogConfig) []*pairSignal {
	if len(buckets) == 0 {
		return nil
	}

	cutoff := now.Add(-time.Duration(clampWindowMinutes(cfg.WindowMinutes)) * time.Minute)
	minWallets := clampMinWallets(cfg.MinWallets)

	signals := make([]*pairSignal, 0, len(buckets))
	for _, bucket := range buckets {
		count := 0
		latestAt := time.Time{}
		for _, seenAt := range bucket.WalletSeen {
			if seenAt.Before(cutoff) {
				continue
			}
			count++
			if seenAt.After(latestAt) {
				latestAt = seenAt
			}
		}
		if count < minWallets {
			continue
		}
		signals = append(signals, &pairSignal{
			Key:         bucket.Key,
			Label:       bucket.Label,
			WalletCount: count,
			LatestAt:    latestAt,
		})
	}
	sort.Slice(signals, func(i, j int) bool {
		if signals[i].WalletCount != signals[j].WalletCount {
			return signals[i].WalletCount > signals[j].WalletCount
		}
		return signals[i].LatestAt.After(signals[j].LatestAt)
	})
	return signals
}

func canonicalPair(token0Address string, token1Address string, token0Symbol string, token1Symbol string) (string, string) {
	leftAddr := strings.ToLower(strings.TrimSpace(token0Address))
	rightAddr := strings.ToLower(strings.TrimSpace(token1Address))
	leftSymbol := strings.ToUpper(strings.TrimSpace(token0Symbol))
	rightSymbol := strings.ToUpper(strings.TrimSpace(token1Symbol))
	if leftAddr == "" || rightAddr == "" {
		return "", ""
	}
	if leftAddr > rightAddr {
		leftAddr, rightAddr = rightAddr, leftAddr
		leftSymbol, rightSymbol = rightSymbol, leftSymbol
	}
	labelLeft := firstNonEmpty(leftSymbol, shortenAddress(leftAddr))
	labelRight := firstNonEmpty(rightSymbol, shortenAddress(rightAddr))
	return leftAddr + "|" + rightAddr, labelLeft + "/" + labelRight
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func shortenAddress(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 10 {
		return value
	}
	return value[:6] + "..." + value[len(value)-4:]
}

func buildBarkMessage(signal *pairSignal, cfg models.SmartMoneyGoldenDogConfig) (string, string) {
	label := firstNonEmpty(signal.Label, "未知交易对")
	title := "金狗通知"
	body := fmt.Sprintf("%s 在 %d 分钟内出现 %d 个聪明钱钱包加 LP，建议立即关注", label, clampWindowMinutes(cfg.WindowMinutes), signal.WalletCount)
	return title, body
}

func SortedWallets(bucket *pairBucket) []string {
	if bucket == nil || len(bucket.WalletSeen) == 0 {
		return nil
	}
	out := make([]string, 0, len(bucket.WalletSeen))
	for wallet := range bucket.WalletSeen {
		out = append(out, wallet)
	}
	sort.Strings(out)
	return out
}
