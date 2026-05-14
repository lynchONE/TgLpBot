package smart_money_follow

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/pool"
	sm "TgLpBot/service/smart_money"
	"TgLpBot/service/strategy"
	"TgLpBot/service/txexec"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxFollowDelaySeconds = 24 * 60 * 60
const defaultFollowTriggerWindowSeconds = 5 * 60
const maxFollowTriggerWindowSeconds = 24 * 60 * 60

var errFollowJobSkipped = errors.New("follow job skipped")
var errFollowJobRetry = errors.New("follow job retry")

type Service struct {
	cancel       context.CancelFunc
	pollInterval time.Duration
}

type SaveConfigInput struct {
	ID                   uint
	Chain                string
	TargetWalletAddress  string
	TargetWallets        []string
	TriggerMode          string
	TriggerMinWallets    int
	TriggerWindowSeconds int
	Enabled              bool
	AmountMode           string
	FixedAmountUSDT      float64
	Ratio                float64
	DelayMode            string
	DelaySeconds         int
	FollowClose          bool
}

type followJobTrigger struct {
	Mode           string
	Wallets        []string
	EventIDs       []uint
	PrimaryEventID uint
}

type ConfigEnvelope struct {
	OK      bool                            `json:"ok"`
	Chain   string                          `json:"chain"`
	Configs []models.SmartMoneyFollowConfig `json:"configs"`
	Jobs    []models.SmartMoneyFollowJob    `json:"jobs"`
}

func NewService() *Service {
	return &Service{pollInterval: 3 * time.Second}
}

func (s *Service) Start() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go s.run(ctx)
}

func (s *Service) Stop() {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancel()
	s.cancel = nil
}

func (s *Service) run(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		if err := s.ScanAndExecute(ctx); err != nil {
			log.Printf("[SmartMoneyFollow] loop error: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) ScanAndExecute(ctx context.Context) error {
	if err := s.scanNewEvents(ctx); err != nil {
		return err
	}
	if err := s.executeDueJobs(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) HandleEvent(ctx context.Context, event *models.SmartMoneyLPEvent) {
	if s == nil || event == nil || event.ID == 0 || database.DB == nil {
		return
	}
	if err := s.createJobsForEvent(ctx, event, false); err != nil {
		log.Printf("[SmartMoneyFollow] handle event failed: event_id=%d err=%v", event.ID, err)
	}
}

func (s *Service) ListEnvelope(ctx context.Context, userID uint, chain string) (*ConfigEnvelope, error) {
	chain, _, err := ResolveChain(chain)
	if err != nil {
		return nil, err
	}
	var configs []models.SmartMoneyFollowConfig
	if err := database.DB.WithContext(ctx).
		Where("user_id = ? AND chain = ?", userID, chain).
		Order("updated_at DESC").
		Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("list follow configs failed: %w", err)
	}

	var jobs []models.SmartMoneyFollowJob
	if err := database.DB.WithContext(ctx).
		Where("user_id = ? AND chain = ?", userID, chain).
		Order("created_at DESC").
		Limit(30).
		Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("list follow jobs failed: %w", err)
	}

	return &ConfigEnvelope{
		OK:      true,
		Chain:   chain,
		Configs: configs,
		Jobs:    jobs,
	}, nil
}

func (s *Service) SaveConfig(ctx context.Context, userID uint, input SaveConfigInput) (*models.SmartMoneyFollowConfig, error) {
	normalized, err := NormalizeSaveInput(input)
	if err != nil {
		return nil, err
	}
	chain, chainID, err := ResolveChain(normalized.Chain)
	if err != nil {
		return nil, err
	}
	normalized.Chain = chain

	var saved models.SmartMoneyFollowConfig
	err = database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.SmartMoneyFollowConfig
		var existingFound bool
		if normalized.ID != 0 {
			if err := tx.Where("id = ? AND user_id = ?", normalized.ID, userID).First(&existing).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("follow config not found")
				}
				return err
			}
			existingFound = true
		} else if normalized.TriggerMode == models.SmartMoneyFollowTriggerModeAny && len(normalized.TargetWallets) == 1 {
			err := tx.Where("user_id = ? AND chain = ? AND target_wallet_address = ?", userID, chain, normalized.TargetWalletAddress).First(&existing).Error
			if err == nil {
				existingFound = true
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}

		latestID, err := latestEventIDForWallets(ctx, tx, chainID, normalized.TargetWallets)
		if err != nil {
			return err
		}

		if existingFound {
			cursorID := existing.CursorEventID
			triggerChanged, err := followConfigTriggerChanged(&existing, normalized)
			if err != nil {
				return err
			}
			if normalized.Enabled && (!existing.Enabled || triggerChanged) {
				cursorID = latestID
			}
			updates := map[string]any{
				"chain":                   chain,
				"chain_id":                chainID,
				"target_wallet_address":   normalized.TargetWalletAddress,
				"target_wallet_addresses": models.StringArray(normalized.TargetWallets),
				"trigger_mode":            normalized.TriggerMode,
				"trigger_min_wallets":     normalized.TriggerMinWallets,
				"trigger_window_seconds":  normalized.TriggerWindowSeconds,
				"enabled":                 normalized.Enabled,
				"amount_mode":             normalized.AmountMode,
				"fixed_amount_usdt":       normalized.FixedAmountUSDT,
				"ratio":                   normalized.Ratio,
				"delay_mode":              normalized.DelayMode,
				"delay_seconds":           normalized.DelaySeconds,
				"follow_close":            normalized.FollowClose,
				"cursor_event_id":         cursorID,
			}
			if latestID > existing.LastSeenEventID {
				updates["last_seen_event_id"] = latestID
			}
			if err := tx.Model(&existing).Updates(updates).Error; err != nil {
				return err
			}
			return tx.Where("id = ?", existing.ID).First(&saved).Error
		}

		cursorID := uint(0)
		if normalized.Enabled {
			cursorID = latestID
		}
		row := models.SmartMoneyFollowConfig{
			UserID:               userID,
			Chain:                chain,
			ChainID:              chainID,
			TargetWalletAddress:  normalized.TargetWalletAddress,
			TargetWallets:        models.StringArray(normalized.TargetWallets),
			TriggerMode:          normalized.TriggerMode,
			TriggerMinWallets:    normalized.TriggerMinWallets,
			TriggerWindowSeconds: normalized.TriggerWindowSeconds,
			Enabled:              normalized.Enabled,
			AmountMode:           normalized.AmountMode,
			FixedAmountUSDT:      normalized.FixedAmountUSDT,
			Ratio:                normalized.Ratio,
			DelayMode:            normalized.DelayMode,
			DelaySeconds:         normalized.DelaySeconds,
			FollowClose:          normalized.FollowClose,
			CursorEventID:        cursorID,
			LastSeenEventID:      latestID,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		saved = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (s *Service) DeleteConfig(ctx context.Context, userID uint, id uint) error {
	if id == 0 {
		return fmt.Errorf("config id is required")
	}
	result := database.DB.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&models.SmartMoneyFollowConfig{})
	if result.Error != nil {
		return fmt.Errorf("delete follow config failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("follow config not found")
	}
	return nil
}

func NormalizeSaveInput(input SaveConfigInput) (SaveConfigInput, error) {
	chain, _, err := ResolveChain(input.Chain)
	if err != nil {
		return SaveConfigInput{}, err
	}
	wallets, err := normalizeWalletAddresses(input.TargetWallets, input.TargetWalletAddress)
	if err != nil {
		return SaveConfigInput{}, err
	}
	address := wallets[0]

	amountMode := strings.ToLower(strings.TrimSpace(input.AmountMode))
	switch amountMode {
	case models.SmartMoneyFollowAmountModeFixed:
		if input.FixedAmountUSDT <= 0 || math.IsNaN(input.FixedAmountUSDT) || math.IsInf(input.FixedAmountUSDT, 0) {
			return SaveConfigInput{}, fmt.Errorf("fixed amount must be greater than 0")
		}
	case models.SmartMoneyFollowAmountModeRatio:
		if input.Ratio <= 0 || math.IsNaN(input.Ratio) || math.IsInf(input.Ratio, 0) {
			return SaveConfigInput{}, fmt.Errorf("ratio must be greater than 0")
		}
	default:
		return SaveConfigInput{}, fmt.Errorf("invalid amount mode")
	}

	delayMode := strings.ToLower(strings.TrimSpace(input.DelayMode))
	switch delayMode {
	case models.SmartMoneyFollowDelayModeImmediate:
		input.DelaySeconds = 0
	case models.SmartMoneyFollowDelayModeFixed:
		if input.DelaySeconds < 0 || input.DelaySeconds > maxFollowDelaySeconds {
			return SaveConfigInput{}, fmt.Errorf("delay seconds must be between 0 and %d", maxFollowDelaySeconds)
		}
	default:
		return SaveConfigInput{}, fmt.Errorf("invalid delay mode")
	}

	triggerMode := strings.ToLower(strings.TrimSpace(input.TriggerMode))
	if triggerMode == "" {
		triggerMode = models.SmartMoneyFollowTriggerModeAny
	}
	switch triggerMode {
	case models.SmartMoneyFollowTriggerModeAny:
		input.TriggerMinWallets = 1
		if input.TriggerWindowSeconds <= 0 {
			input.TriggerWindowSeconds = defaultFollowTriggerWindowSeconds
		}
	case models.SmartMoneyFollowTriggerModeThreshold:
		if input.TriggerMinWallets < 2 {
			return SaveConfigInput{}, fmt.Errorf("trigger min wallets must be at least 2")
		}
		if input.TriggerMinWallets > len(wallets) {
			return SaveConfigInput{}, fmt.Errorf("trigger min wallets cannot exceed target wallet count")
		}
		if input.TriggerWindowSeconds <= 0 || input.TriggerWindowSeconds > maxFollowTriggerWindowSeconds {
			return SaveConfigInput{}, fmt.Errorf("trigger window seconds must be between 1 and %d", maxFollowTriggerWindowSeconds)
		}
	default:
		return SaveConfigInput{}, fmt.Errorf("invalid trigger mode")
	}

	input.Chain = chain
	input.TargetWalletAddress = address
	input.TargetWallets = wallets
	input.TriggerMode = triggerMode
	input.AmountMode = amountMode
	input.DelayMode = delayMode
	return input, nil
}

func ResolveChain(chain string) (string, int, error) {
	chain = config.NormalizeChain(chain)
	if config.AppConfig == nil {
		return "", 0, fmt.Errorf("config not loaded")
	}
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok || cc.ChainID <= 0 {
		return "", 0, fmt.Errorf("chain config not found: %s", chain)
	}
	return chain, int(cc.ChainID), nil
}

func CalculateFollowAmount(cfg *models.SmartMoneyFollowConfig, event *models.SmartMoneyLPEvent) (float64, error) {
	if cfg == nil {
		return 0, fmt.Errorf("follow config is nil")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.AmountMode)) {
	case models.SmartMoneyFollowAmountModeFixed:
		if cfg.FixedAmountUSDT <= 0 {
			return 0, fmt.Errorf("fixed amount must be greater than 0")
		}
		return cfg.FixedAmountUSDT, nil
	case models.SmartMoneyFollowAmountModeRatio:
		base, err := EventTotalUSD(event)
		if err != nil {
			return 0, err
		}
		if cfg.Ratio <= 0 {
			return 0, fmt.Errorf("ratio must be greater than 0")
		}
		amount := base * cfg.Ratio
		if amount <= 0 {
			return 0, fmt.Errorf("calculated follow amount must be greater than 0")
		}
		return amount, nil
	default:
		return 0, fmt.Errorf("invalid amount mode")
	}
}

func EventTotalUSD(event *models.SmartMoneyLPEvent) (float64, error) {
	if event == nil {
		return 0, fmt.Errorf("event is nil")
	}
	if v, ok := parsePositiveUSD(event.TotalUSD); ok {
		return v, nil
	}
	var total float64
	if v, ok := parsePositiveUSD(event.Token0AmountUSD); ok {
		total += v
	}
	if v, ok := parsePositiveUSD(event.Token1AmountUSD); ok {
		total += v
	}
	if total <= 0 {
		return 0, fmt.Errorf("event USD amount is missing")
	}
	return total, nil
}

func parsePositiveUSD(raw *string) (float64, bool) {
	if raw == nil {
		return 0, false
	}
	text := strings.TrimSpace(*raw)
	if text == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(text, 64)
	if err != nil || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func (s *Service) scanNewEvents(ctx context.Context) error {
	var configs []models.SmartMoneyFollowConfig
	if err := database.DB.WithContext(ctx).
		Where("enabled = ?", true).
		Find(&configs).Error; err != nil {
		return fmt.Errorf("list enabled follow configs failed: %w", err)
	}

	for i := range configs {
		cfg := configs[i]
		wallets, err := configTargetWallets(&cfg)
		if err != nil {
			return fmt.Errorf("invalid follow config wallet set config_id=%d: %w", cfg.ID, err)
		}
		if len(wallets) == 0 {
			log.Printf("[SmartMoneyFollow] skip config with empty wallet set: config_id=%d", cfg.ID)
			continue
		}
		var events []models.SmartMoneyLPEvent
		if err := database.DB.WithContext(ctx).
			Where("wallet_address IN ? AND chain_id = ? AND id > ? AND event_type IN ?", wallets, cfg.ChainID, cfg.CursorEventID, []string{"add", "remove"}).
			Order("id ASC").
			Limit(50).
			Find(&events).Error; err != nil {
			return fmt.Errorf("list follow events failed config_id=%d: %w", cfg.ID, err)
		}
		for j := range events {
			event := events[j]
			if err := s.createJobForConfig(ctx, &cfg, &event); err != nil {
				log.Printf("[SmartMoneyFollow] create job failed config_id=%d event_id=%d err=%v", cfg.ID, event.ID, err)
			}
			if event.ID > cfg.CursorEventID {
				cfg.CursorEventID = event.ID
			}
			if event.ID > cfg.LastSeenEventID {
				cfg.LastSeenEventID = event.ID
			}
		}
		if len(events) > 0 {
			if err := database.DB.WithContext(ctx).Model(&models.SmartMoneyFollowConfig{}).
				Where("id = ?", cfg.ID).
				Updates(map[string]any{
					"cursor_event_id":    cfg.CursorEventID,
					"last_seen_event_id": cfg.LastSeenEventID,
				}).Error; err != nil {
				return fmt.Errorf("update follow cursor failed config_id=%d: %w", cfg.ID, err)
			}
		}
	}
	return nil
}

func (s *Service) createJobsForEvent(ctx context.Context, event *models.SmartMoneyLPEvent, advanceCursor bool) error {
	eventType := strings.ToLower(strings.TrimSpace(event.EventType))
	if eventType != "add" && eventType != "remove" {
		return nil
	}
	address := normalizeWalletAddress(event.WalletAddress)
	if address == "" {
		return fmt.Errorf("event has invalid wallet address")
	}

	var configs []models.SmartMoneyFollowConfig
	if err := database.DB.WithContext(ctx).
		Where("enabled = ? AND chain_id = ? AND cursor_event_id < ?", true, event.ChainID, event.ID).
		Find(&configs).Error; err != nil {
		return fmt.Errorf("list follow configs for event failed: %w", err)
	}
	for i := range configs {
		cfg := configs[i]
		wallets, err := configTargetWallets(&cfg)
		if err != nil {
			return fmt.Errorf("invalid follow config wallet set config_id=%d: %w", cfg.ID, err)
		}
		if !stringSliceContains(wallets, address) {
			continue
		}
		if err := s.createJobForConfig(ctx, &cfg, event); err != nil {
			log.Printf("[SmartMoneyFollow] create event job failed config_id=%d event_id=%d err=%v", cfg.ID, event.ID, err)
		}
		if advanceCursor {
			if err := database.DB.WithContext(ctx).Model(&models.SmartMoneyFollowConfig{}).
				Where("id = ? AND cursor_event_id < ?", cfg.ID, event.ID).
				Updates(map[string]any{
					"cursor_event_id":    event.ID,
					"last_seen_event_id": event.ID,
				}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) createJobForConfig(ctx context.Context, cfg *models.SmartMoneyFollowConfig, event *models.SmartMoneyLPEvent) error {
	if cfg == nil || event == nil {
		return fmt.Errorf("config or event is nil")
	}
	action, ok := followActionForEvent(event.EventType)
	if !ok {
		return nil
	}
	trigger, ok, err := s.resolveJobTrigger(ctx, cfg, event, action)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	status := models.SmartMoneyFollowJobStatusPending
	errorMessage := ""
	amountUSDT := float64(0)
	if action == models.SmartMoneyFollowJobActionOpen {
		amount, err := CalculateFollowAmount(cfg, event)
		if err != nil {
			status = models.SmartMoneyFollowJobStatusFailed
			errorMessage = err.Error()
		} else {
			amountUSDT = amount
		}
	}
	if action == models.SmartMoneyFollowJobActionClose && !cfg.FollowClose {
		status = models.SmartMoneyFollowJobStatusSkipped
		errorMessage = "follow close disabled"
	}
	positionRef := targetPositionRefForFollowJob(cfg, event)
	if strings.TrimSpace(positionRef) == "" {
		status = models.SmartMoneyFollowJobStatusFailed
		errorMessage = "target position ref is missing"
	}
	if action == models.SmartMoneyFollowJobActionOpen && normalizeConfigTriggerMode(cfg.TriggerMode) == models.SmartMoneyFollowTriggerModeThreshold {
		exists, err := thresholdOpenJobExists(ctx, cfg.ID, positionRef)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
	}
	now := time.Now()
	scheduledAt := now
	if cfg.DelayMode == models.SmartMoneyFollowDelayModeFixed {
		scheduledAt = now.Add(time.Duration(cfg.DelaySeconds) * time.Second)
	}
	var finishedAt *time.Time
	if status == models.SmartMoneyFollowJobStatusFailed || status == models.SmartMoneyFollowJobStatusSkipped {
		done := now
		finishedAt = &done
	}
	job := models.SmartMoneyFollowJob{
		ConfigID:            cfg.ID,
		UserID:              cfg.UserID,
		Chain:               cfg.Chain,
		ChainID:             cfg.ChainID,
		TargetWalletAddress: normalizeWalletAddress(event.WalletAddress),
		EventID:             trigger.PrimaryEventID,
		TriggerMode:         trigger.Mode,
		TriggerWallets:      models.StringArray(trigger.Wallets),
		TriggerEventIDs:     uintIDsToStringArray(trigger.EventIDs),
		TargetPositionRef:   positionRef,
		Action:              action,
		Status:              status,
		ScheduledAt:         scheduledAt,
		FinishedAt:          finishedAt,
		AmountUSDT:          amountUSDT,
		ErrorMessage:        errorMessage,
	}
	if err := database.DB.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&job).Error; err != nil {
		return fmt.Errorf("create follow job failed: %w", err)
	}
	return nil
}

func followActionForEvent(eventType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "add":
		return models.SmartMoneyFollowJobActionOpen, true
	case "remove":
		return models.SmartMoneyFollowJobActionClose, true
	default:
		return "", false
	}
}

func targetPositionRefForFollowJob(cfg *models.SmartMoneyFollowConfig, event *models.SmartMoneyLPEvent) string {
	if cfg == nil || event == nil {
		return ""
	}
	if normalizeConfigTriggerMode(cfg.TriggerMode) != models.SmartMoneyFollowTriggerModeThreshold {
		return sm.BuildPositionRefFromEvent(event)
	}
	if event.ChainID <= 0 || strings.TrimSpace(event.Protocol) == "" || strings.TrimSpace(event.PoolAddress) == "" || event.TickLower == nil || event.TickUpper == nil {
		return ""
	}
	return sm.NormalizePositionRef(fmt.Sprintf(
		"%d:%s:threshold:%s:%d:%d",
		event.ChainID,
		strings.ToLower(strings.TrimSpace(event.Protocol)),
		strings.ToLower(strings.TrimSpace(event.PoolAddress)),
		*event.TickLower,
		*event.TickUpper,
	))
}

func thresholdOpenJobExists(ctx context.Context, configID uint, positionRef string) (bool, error) {
	if configID == 0 || strings.TrimSpace(positionRef) == "" {
		return false, nil
	}
	var count int64
	if err := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyFollowJob{}).
		Where("config_id = ? AND action = ? AND target_position_ref = ? AND status IN ?",
			configID,
			models.SmartMoneyFollowJobActionOpen,
			positionRef,
			[]string{
				models.SmartMoneyFollowJobStatusPending,
				models.SmartMoneyFollowJobStatusRunning,
				models.SmartMoneyFollowJobStatusSuccess,
			}).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("check threshold open job exists failed: %w", err)
	}
	if count > 0 {
		return true, nil
	}
	if err := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyFollowTask{}).
		Where("config_id = ? AND target_position_ref = ? AND status = ?", configID, positionRef, models.SmartMoneyFollowTaskStatusOpen).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("check threshold follow task exists failed: %w", err)
	}
	return count > 0, nil
}

func (s *Service) resolveJobTrigger(ctx context.Context, cfg *models.SmartMoneyFollowConfig, event *models.SmartMoneyLPEvent, action string) (followJobTrigger, bool, error) {
	triggerMode := normalizeConfigTriggerMode(cfg.TriggerMode)
	eventWallet := normalizeWalletAddress(event.WalletAddress)
	if eventWallet == "" {
		return followJobTrigger{}, false, fmt.Errorf("event has invalid wallet address")
	}
	if action != models.SmartMoneyFollowJobActionOpen || triggerMode == models.SmartMoneyFollowTriggerModeAny {
		return followJobTrigger{
			Mode:           triggerMode,
			Wallets:        []string{eventWallet},
			EventIDs:       []uint{event.ID},
			PrimaryEventID: event.ID,
		}, true, nil
	}

	if cfg.TriggerMinWallets <= 1 {
		return followJobTrigger{}, false, fmt.Errorf("threshold trigger min wallets must be greater than 1")
	}
	if event.TickLower == nil || event.TickUpper == nil {
		return followJobTrigger{}, false, nil
	}
	wallets, err := configTargetWallets(cfg)
	if err != nil {
		return followJobTrigger{}, false, err
	}
	if len(wallets) == 0 {
		return followJobTrigger{}, false, fmt.Errorf("follow config has empty wallet set")
	}
	windowSeconds := cfg.TriggerWindowSeconds
	if windowSeconds <= 0 {
		windowSeconds = defaultFollowTriggerWindowSeconds
	}
	windowStart := event.TxTimestamp.Add(-time.Duration(windowSeconds) * time.Second)

	var events []models.SmartMoneyLPEvent
	if err := database.DB.WithContext(ctx).
		Where("wallet_address IN ? AND chain_id = ? AND event_type = ? AND id <= ? AND tx_timestamp >= ? AND protocol = ? AND pool_address = ? AND tick_lower = ? AND tick_upper = ?",
			wallets, event.ChainID, "add", event.ID, windowStart, event.Protocol, event.PoolAddress, *event.TickLower, *event.TickUpper).
		Order("id DESC").
		Find(&events).Error; err != nil {
		return followJobTrigger{}, false, fmt.Errorf("list threshold trigger events failed: %w", err)
	}

	walletSeen := make(map[string]models.SmartMoneyLPEvent, len(wallets))
	for i := range events {
		evt := events[i]
		addr := normalizeWalletAddress(evt.WalletAddress)
		if addr == "" {
			continue
		}
		if _, ok := walletSeen[addr]; !ok {
			walletSeen[addr] = evt
		}
	}
	if len(walletSeen) < cfg.TriggerMinWallets {
		return followJobTrigger{}, false, nil
	}

	triggerWallets := make([]string, 0, len(walletSeen))
	triggerEventIDs := make([]uint, 0, len(walletSeen))
	if evt, ok := walletSeen[eventWallet]; ok {
		triggerWallets = append(triggerWallets, eventWallet)
		triggerEventIDs = append(triggerEventIDs, evt.ID)
	}
	for _, wallet := range wallets {
		if wallet == eventWallet {
			continue
		}
		evt, ok := walletSeen[wallet]
		if !ok {
			continue
		}
		triggerWallets = append(triggerWallets, wallet)
		triggerEventIDs = append(triggerEventIDs, evt.ID)
		if len(triggerWallets) == cfg.TriggerMinWallets {
			break
		}
	}
	if len(triggerWallets) < cfg.TriggerMinWallets {
		return followJobTrigger{}, false, fmt.Errorf("threshold trigger wallet selection mismatch")
	}
	return followJobTrigger{
		Mode:           triggerMode,
		Wallets:        triggerWallets,
		EventIDs:       triggerEventIDs,
		PrimaryEventID: event.ID,
	}, true, nil
}

func (s *Service) executeDueJobs(ctx context.Context) error {
	var jobs []models.SmartMoneyFollowJob
	if err := database.DB.WithContext(ctx).
		Where("status = ? AND scheduled_at <= ?", models.SmartMoneyFollowJobStatusPending, time.Now()).
		Order("scheduled_at ASC, id ASC").
		Limit(5).
		Find(&jobs).Error; err != nil {
		return fmt.Errorf("list pending follow jobs failed: %w", err)
	}
	for i := range jobs {
		job := jobs[i]
		if err := s.executeJob(ctx, &job); err != nil {
			log.Printf("[SmartMoneyFollow] execute job failed: job_id=%d err=%v", job.ID, err)
		}
	}
	return nil
}

func (s *Service) executeJob(ctx context.Context, job *models.SmartMoneyFollowJob) error {
	if job == nil {
		return fmt.Errorf("job is nil")
	}
	now := time.Now()
	claimed := database.DB.WithContext(ctx).Model(&models.SmartMoneyFollowJob{}).
		Where("id = ? AND status = ?", job.ID, models.SmartMoneyFollowJobStatusPending).
		Updates(map[string]any{
			"status":     models.SmartMoneyFollowJobStatusRunning,
			"started_at": &now,
		})
	if claimed.Error != nil {
		return claimed.Error
	}
	if claimed.RowsAffected == 0 {
		return nil
	}

	var err error
	var taskID *uint
	switch job.Action {
	case models.SmartMoneyFollowJobActionOpen:
		taskID, err = s.executeOpenJob(ctx, job)
	case models.SmartMoneyFollowJobActionClose:
		taskID, err = s.executeCloseJob(ctx, job)
	default:
		err = fmt.Errorf("invalid follow job action: %s", job.Action)
	}
	if err != nil {
		if errors.Is(err, errFollowJobRetry) {
			return rescheduleJob(ctx, job.ID, 10*time.Second, err.Error())
		}
		if errors.Is(err, errFollowJobSkipped) {
			return markJobFinished(ctx, job.ID, models.SmartMoneyFollowJobStatusSkipped, taskID, err.Error())
		}
		return markJobFinished(ctx, job.ID, models.SmartMoneyFollowJobStatusFailed, taskID, err.Error())
	}
	return markJobFinished(ctx, job.ID, models.SmartMoneyFollowJobStatusSuccess, taskID, "")
}

func markJobFinished(ctx context.Context, jobID uint, status string, taskID *uint, message string) error {
	done := time.Now()
	updates := map[string]any{
		"status":        status,
		"finished_at":   &done,
		"error_message": message,
	}
	if taskID != nil {
		updates["task_id"] = *taskID
	}
	return database.DB.WithContext(ctx).Model(&models.SmartMoneyFollowJob{}).
		Where("id = ?", jobID).
		Updates(updates).Error
}

func rescheduleJob(ctx context.Context, jobID uint, delay time.Duration, message string) error {
	nextAt := time.Now().Add(delay)
	return database.DB.WithContext(ctx).Model(&models.SmartMoneyFollowJob{}).
		Where("id = ?", jobID).
		Updates(map[string]any{
			"status":        models.SmartMoneyFollowJobStatusPending,
			"scheduled_at":  nextAt,
			"error_message": message,
		}).Error
}

func (s *Service) executeOpenJob(ctx context.Context, job *models.SmartMoneyFollowJob) (*uint, error) {
	cfg, event, err := loadJobConfigAndEvent(ctx, job)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("%w: follow config disabled", errFollowJobSkipped)
	}
	if job.AmountUSDT <= 0 {
		return nil, fmt.Errorf("follow amount must be greater than 0")
	}
	task, err := buildFollowTask(ctx, cfg, event, job.AmountUSDT)
	if err != nil {
		return nil, err
	}
	taskID, err := runOpenTaskSync(ctx, cfg, job, task)
	if err != nil {
		return taskID, err
	}
	return taskID, nil
}

func runOpenTaskSync(ctx context.Context, cfg *models.SmartMoneyFollowConfig, job *models.SmartMoneyFollowJob, task *models.StrategyTask) (*uint, error) {
	if cfg == nil || job == nil || task == nil {
		return nil, fmt.Errorf("config, job or task is nil")
	}
	var taskID uint
	err := runTaskSync(cfg.UserID, task, func() error {
		if err := strategy.CreateTaskRecord(task); err != nil {
			return fmt.Errorf("create strategy task failed: %w", err)
		}
		taskID = task.ID

		enterRes, err := liquidity.NewLiquidityService().EnterTaskFromUSDT(cfg.UserID, task)
		if err != nil {
			_ = database.DB.WithContext(ctx).Model(task).Updates(map[string]any{
				"status":        models.StrategyStatusError,
				"error_message": err.Error(),
			}).Error
			return fmt.Errorf("enter follow position failed: %w", err)
		}
		if err := applyEnterResult(ctx, task, enterRes); err != nil {
			return fmt.Errorf("save enter result failed: %w", err)
		}

		mapping := models.SmartMoneyFollowTask{
			ConfigID:            cfg.ID,
			UserID:              cfg.UserID,
			Chain:               cfg.Chain,
			ChainID:             cfg.ChainID,
			TargetWalletAddress: cfg.TargetWalletAddress,
			TargetPositionRef:   job.TargetPositionRef,
			OpenEventID:         job.EventID,
			OpenJobID:           job.ID,
			TaskID:              taskID,
			Status:              models.SmartMoneyFollowTaskStatusOpen,
		}
		if err := database.DB.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(&mapping).Error; err != nil {
			return fmt.Errorf("create follow task mapping failed: %w", err)
		}
		return nil
	})
	if taskID > 0 {
		return &taskID, err
	}
	return nil, err
}

func (s *Service) executeCloseJob(ctx context.Context, job *models.SmartMoneyFollowJob) (*uint, error) {
	cfg, _, err := loadJobConfigAndEvent(ctx, job)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("%w: follow config disabled", errFollowJobSkipped)
	}
	if !cfg.FollowClose {
		return nil, fmt.Errorf("%w: follow close disabled", errFollowJobSkipped)
	}

	var mapping models.SmartMoneyFollowTask
	if err := database.DB.WithContext(ctx).
		Where("config_id = ? AND target_position_ref = ? AND status = ?", cfg.ID, job.TargetPositionRef, models.SmartMoneyFollowTaskStatusOpen).
		Order("id DESC").
		First(&mapping).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: open follow task not found", errFollowJobSkipped)
		}
		return nil, err
	}
	taskID := mapping.TaskID

	task, err := strategy.NewStrategyTaskService().GetByID(cfg.UserID, taskID)
	if err != nil {
		return &taskID, err
	}
	if !task.IsFollow {
		return &taskID, fmt.Errorf("task is not a follow task")
	}
	if task.Status == models.StrategyStatusStopped {
		return &taskID, closeFollowMapping(ctx, &mapping, job)
	}

	runErr := runTaskSync(cfg.UserID, task, func() error {
		_, err := liquidity.NewLiquidityService().ExitTaskToUSDTWithOptions(cfg.UserID, task, false, liquidity.TxOptions{})
		return err
	})
	if runErr != nil {
		return &taskID, runErr
	}

	if err := database.DB.WithContext(ctx).Model(task).Updates(map[string]any{
		"status":            models.StrategyStatusStopped,
		"current_liquidity": "0",
		"error_message":     "",
	}).Error; err != nil {
		return &taskID, err
	}
	if err := closeFollowMapping(ctx, &mapping, job); err != nil {
		return &taskID, err
	}
	return &taskID, nil
}

func runTaskSync(userID uint, task *models.StrategyTask, fn func() error) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}
	resultCh := make(chan error, 1)
	ok, err := txexec.Default().TryRunTask(userID, task.WalletID, task.WalletAddress, func(_ string) {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- fmt.Errorf("task execution panic: %v", r)
			}
		}()
		resultCh <- fn()
	})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: wallet is busy", errFollowJobRetry)
	}
	select {
	case err := <-resultCh:
		return err
	case <-time.After(10 * time.Minute):
		return fmt.Errorf("task execution timeout")
	}
}

func closeFollowMapping(ctx context.Context, mapping *models.SmartMoneyFollowTask, job *models.SmartMoneyFollowJob) error {
	if mapping == nil || job == nil {
		return fmt.Errorf("mapping or job is nil")
	}
	closeEventID := job.EventID
	closeJobID := job.ID
	return database.DB.WithContext(ctx).Model(mapping).Updates(map[string]any{
		"status":         models.SmartMoneyFollowTaskStatusClosed,
		"close_event_id": &closeEventID,
		"close_job_id":   &closeJobID,
	}).Error
}

func loadJobConfigAndEvent(ctx context.Context, job *models.SmartMoneyFollowJob) (*models.SmartMoneyFollowConfig, *models.SmartMoneyLPEvent, error) {
	var cfg models.SmartMoneyFollowConfig
	if err := database.DB.WithContext(ctx).
		Where("id = ? AND user_id = ?", job.ConfigID, job.UserID).
		First(&cfg).Error; err != nil {
		return nil, nil, fmt.Errorf("load follow config failed: %w", err)
	}
	var event models.SmartMoneyLPEvent
	if err := database.DB.WithContext(ctx).
		Where("id = ?", job.EventID).
		First(&event).Error; err != nil {
		return nil, nil, fmt.Errorf("load follow event failed: %w", err)
	}
	return &cfg, &event, nil
}

func buildFollowTask(_ context.Context, cfg *models.SmartMoneyFollowConfig, event *models.SmartMoneyLPEvent, amountUSDT float64) (*models.StrategyTask, error) {
	if cfg == nil || event == nil {
		return nil, fmt.Errorf("config or event is nil")
	}
	if event.TickLower == nil || event.TickUpper == nil || *event.TickLower >= *event.TickUpper {
		return nil, fmt.Errorf("event tick range is invalid")
	}
	poolID := strings.TrimSpace(event.PoolAddress)
	if poolID == "" {
		return nil, fmt.Errorf("event pool address is missing")
	}
	poolVersion := poolVersionFromProtocol(event.Protocol)
	if poolVersion == "" {
		return nil, fmt.Errorf("unsupported protocol: %s", event.Protocol)
	}

	walletRow, err := wallet.NewWalletService().GetDefaultWallet(cfg.UserID)
	if err != nil {
		return nil, err
	}
	globalCfg, err := userSvc.NewGlobalConfigService().GetOrCreate(cfg.UserID)
	if err != nil {
		return nil, err
	}

	poolInfo, err := pool.NewPoolService().GetPoolInfoForVersionCached(cfg.Chain, poolVersion, poolID)
	if err != nil {
		return nil, fmt.Errorf("load pool info failed: %w", err)
	}
	if poolInfo == nil {
		return nil, fmt.Errorf("pool info is nil")
	}
	token0 := strings.TrimSpace(poolInfo.Token0)
	token1 := strings.TrimSpace(poolInfo.Token1)
	if token0 == "" {
		token0 = strings.TrimSpace(event.Token0Address)
	}
	if token1 == "" {
		token1 = strings.TrimSpace(event.Token1Address)
	}
	if !common.IsHexAddress(token0) || !common.IsHexAddress(token1) {
		return nil, fmt.Errorf("pool token address is invalid")
	}
	token0Symbol := strings.TrimSpace(poolInfo.Token0Symbol)
	if token0Symbol == "" {
		token0Symbol = strings.TrimSpace(event.Token0Symbol)
	}
	token1Symbol := strings.TrimSpace(poolInfo.Token1Symbol)
	if token1Symbol == "" {
		token1Symbol = strings.TrimSpace(event.Token1Symbol)
	}
	if token0Symbol == "" || token1Symbol == "" {
		return nil, fmt.Errorf("pool token symbol is missing")
	}
	tickSpacing := poolInfo.TickSpacing
	if tickSpacing <= 0 {
		return nil, fmt.Errorf("pool tick spacing is invalid")
	}
	hooksAddr := normalizeHookAddress(poolInfo.HooksAddress)

	return &models.StrategyTask{
		UserID:                 cfg.UserID,
		Chain:                  cfg.Chain,
		PoolId:                 poolID,
		PoolVersion:            poolVersion,
		Exchange:               strings.TrimSpace(poolInfo.Exchange),
		WalletID:               walletRow.ID,
		WalletAddress:          walletRow.Address,
		IsFollow:               true,
		Token0Symbol:           token0Symbol,
		Token1Symbol:           token1Symbol,
		Token0Address:          token0,
		Token1Address:          token1,
		HooksAddress:           hooksAddr,
		Fee:                    poolInfo.Fee,
		TickSpacing:            tickSpacing,
		TickLower:              *event.TickLower,
		TickUpper:              *event.TickUpper,
		AmountUSDT:             amountUSDT,
		CurrentLiquidity:       "0",
		ReopenDelaySeconds:     strategy.NormalizeRebalanceTimeout(globalCfg.RebalanceTimeout),
		SlippageTolerance:      globalCfg.SlippageTolerance,
		AutoReinvest:           globalCfg.AutoReinvest,
		AllowEntrySwap:         true,
		RebalanceEnabled:       false,
		OutOfRangeMode:         string(models.StrategyOutOfRangeModeExitAll),
		RangeActivationPending: false,
		Status:                 models.StrategyStatusRunning,
		LastCheckTime:          time.Now(),
	}, nil
}

func applyEnterResult(ctx context.Context, task *models.StrategyTask, enterRes *liquidity.EnterResult) error {
	if task == nil || enterRes == nil {
		return fmt.Errorf("task or enter result is nil")
	}
	updates := map[string]any{
		"current_liquidity":      enterRes.CurrentLiquidity,
		"exit_liquidity_removed": false,
		"error_message":          "",
		"status":                 models.StrategyStatusRunning,
	}
	if v3TokenID := strings.TrimSpace(enterRes.V3TokenID); v3TokenID != "" && v3TokenID != "0" {
		updates["v3_position_manager_address"] = enterRes.V3PositionManagerAddress
		updates["v3_token_id"] = enterRes.V3TokenID
	}
	if v4TokenID := strings.TrimSpace(enterRes.V4TokenID); v4TokenID != "" && v4TokenID != "0" {
		updates["v4_token_id"] = enterRes.V4TokenID
	}
	return database.DB.WithContext(ctx).Model(task).Updates(updates).Error
}

func poolVersionFromProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if strings.Contains(protocol, "v4") {
		return "v4"
	}
	if strings.Contains(protocol, "v3") {
		return "v3"
	}
	return ""
}

func normalizeHookAddress(value string) string {
	value = strings.TrimSpace(value)
	if common.IsHexAddress(value) {
		return common.HexToAddress(value).Hex()
	}
	return "0x0000000000000000000000000000000000000000"
}

func latestEventIDForWallet(ctx context.Context, tx *gorm.DB, chainID int, address string) (uint, error) {
	var row models.SmartMoneyLPEvent
	err := tx.WithContext(ctx).
		Where("wallet_address = ? AND chain_id = ?", address, chainID).
		Order("id DESC").
		First(&row).Error
	if err == nil {
		return row.ID, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return 0, err
}

func latestEventIDForWallets(ctx context.Context, tx *gorm.DB, chainID int, wallets []string) (uint, error) {
	if len(wallets) == 0 {
		return 0, fmt.Errorf("target wallet set is empty")
	}
	var row models.SmartMoneyLPEvent
	err := tx.WithContext(ctx).
		Where("wallet_address IN ? AND chain_id = ?", wallets, chainID).
		Order("id DESC").
		First(&row).Error
	if err == nil {
		return row.ID, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return 0, err
}

func normalizeWalletAddress(value string) string {
	value = strings.TrimSpace(value)
	if !common.IsHexAddress(value) {
		return ""
	}
	return strings.ToLower(common.HexToAddress(value).Hex())
}

func normalizeWalletAddresses(values []string, legacy string) ([]string, error) {
	seen := make(map[string]struct{}, len(values)+1)
	wallets := make([]string, 0, len(values)+1)
	appendWallet := func(value string) error {
		addr := normalizeWalletAddress(value)
		if addr == "" {
			if strings.TrimSpace(value) == "" {
				return nil
			}
			return fmt.Errorf("invalid target wallet address")
		}
		if _, ok := seen[addr]; ok {
			return nil
		}
		seen[addr] = struct{}{}
		wallets = append(wallets, addr)
		return nil
	}
	for _, value := range values {
		if err := appendWallet(value); err != nil {
			return nil, err
		}
	}
	if err := appendWallet(legacy); err != nil {
		return nil, err
	}
	if len(wallets) == 0 {
		return nil, fmt.Errorf("target wallet address is required")
	}
	return wallets, nil
}

func configTargetWallets(cfg *models.SmartMoneyFollowConfig) ([]string, error) {
	if cfg == nil {
		return nil, fmt.Errorf("follow config is nil")
	}
	wallets, err := normalizeWalletAddresses([]string(cfg.TargetWallets), cfg.TargetWalletAddress)
	if err != nil {
		return nil, err
	}
	return wallets, nil
}

func normalizeConfigTriggerMode(value string) string {
	mode := strings.ToLower(strings.TrimSpace(value))
	if mode == models.SmartMoneyFollowTriggerModeThreshold {
		return mode
	}
	return models.SmartMoneyFollowTriggerModeAny
}

func followConfigTriggerChanged(existing *models.SmartMoneyFollowConfig, next SaveConfigInput) (bool, error) {
	if existing == nil {
		return true, nil
	}
	if normalizeConfigTriggerMode(existing.TriggerMode) != next.TriggerMode {
		return true, nil
	}
	if existing.TriggerMinWallets != next.TriggerMinWallets || existing.TriggerWindowSeconds != next.TriggerWindowSeconds {
		return true, nil
	}
	wallets, err := configTargetWallets(existing)
	if err != nil {
		return false, err
	}
	return !stringSlicesEqual(wallets, next.TargetWallets), nil
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringSliceContains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func uintIDsToStringArray(ids []uint) models.StringArray {
	out := make(models.StringArray, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		out = append(out, strconv.FormatUint(uint64(id), 10))
	}
	return out
}
