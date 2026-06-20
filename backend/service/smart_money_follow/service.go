package smart_money_follow

import (
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/pool"
	sm "TgLpBot/service/smart_money"
	"TgLpBot/service/strategy"
	"TgLpBot/service/trade"
	"TgLpBot/service/txexec"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/big"
	"sort"
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
const monitoredWalletSourceAutoFollow = "auto_follow"
const maxFollowJobRetryCount = 6

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
	ExecutionWalletID    uint
	ExecutionWalletAddr  string
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
	OK           bool                             `json:"ok"`
	Chain        string                           `json:"chain"`
	Configs      []models.SmartMoneyFollowConfig  `json:"configs"`
	Jobs         []models.SmartMoneyFollowJob     `json:"jobs"`
	Attempts     []models.SmartMoneyFollowAttempt `json:"attempts"`
	TargetEvents []models.SmartMoneyLPEvent       `json:"target_events"`
	JobEvents    []models.SmartMoneyLPEvent       `json:"job_events"`
	Wallets      []ExecutionWalletOption          `json:"wallets"`
}

type DeleteLogsResult struct {
	DeletedJobs     int64 `json:"deleted_jobs"`
	DeletedAttempts int64 `json:"deleted_attempts"`
}

type ExecutionWalletOption struct {
	ID        uint   `json:"id"`
	Address   string `json:"address"`
	Name      string `json:"name,omitempty"`
	IsDefault bool   `json:"is_default"`
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
	if err := backfillFollowTaskRangePercentages(ctx); err != nil {
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
	chain, chainID, err := ResolveChain(chain)
	if err != nil {
		return nil, err
	}
	walletOptions, defaultWallet, err := listExecutionWalletOptions(ctx, userID)
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
	for i := range configs {
		fillConfigExecutionWallet(&configs[i], defaultWallet)
	}
	logCursor, err := followLogCursor(ctx, userID, chain)
	if err != nil {
		return nil, err
	}

	var jobs []models.SmartMoneyFollowJob
	jobsQuery := database.DB.WithContext(ctx).
		Where("user_id = ? AND chain = ?", userID, chain)
	if logCursor != nil {
		jobsQuery = jobsQuery.Where("(created_at > ? OR status IN ?)", logCursor.ClearedAt, []string{
			models.SmartMoneyFollowJobStatusPending,
			models.SmartMoneyFollowJobStatusRunning,
		})
	}
	if err := jobsQuery.
		Order("created_at DESC").
		Limit(30).
		Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("list follow jobs failed: %w", err)
	}
	for i := range jobs {
		fillJobExecutionWallet(&jobs[i], defaultWallet)
	}

	var attempts []models.SmartMoneyFollowAttempt
	attemptsQuery := database.DB.WithContext(ctx).
		Where("user_id = ? AND chain = ?", userID, chain)
	if logCursor != nil {
		attemptsQuery = attemptsQuery.Where("created_at > ?", logCursor.ClearedAt)
	}
	if err := attemptsQuery.
		Order("created_at DESC").
		Limit(50).
		Find(&attempts).Error; err != nil {
		return nil, fmt.Errorf("list follow attempts failed: %w", err)
	}
	for i := range attempts {
		fillAttemptExecutionWallet(&attempts[i], defaultWallet)
	}

	targetEvents, err := listRecentTargetEventsForConfigs(ctx, chainID, configs, logCursor)
	if err != nil {
		return nil, err
	}
	jobEvents, err := listRecentEventsForJobsAndAttempts(ctx, jobs, attempts)
	if err != nil {
		return nil, err
	}

	return &ConfigEnvelope{
		OK:           true,
		Chain:        chain,
		Configs:      configs,
		Jobs:         jobs,
		Attempts:     attempts,
		TargetEvents: targetEvents,
		JobEvents:    jobEvents,
		Wallets:      walletOptions,
	}, nil
}

func (s *Service) DeleteLogs(ctx context.Context, userID uint, chain string) (*DeleteLogsResult, error) {
	chain, chainID, err := ResolveChain(chain)
	if err != nil {
		return nil, err
	}
	result := &DeleteLogsResult{}
	err = database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		clearedAt := time.Now()
		clearedEventID, err := maxLPEventIDForChain(tx, chainID)
		if err != nil {
			return err
		}
		cursor := models.SmartMoneyFollowLogCursor{
			UserID:         userID,
			Chain:          chain,
			ClearedAt:      clearedAt,
			ClearedEventID: clearedEventID,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "chain"}},
			DoUpdates: clause.Assignments(map[string]any{
				"cleared_at":       clearedAt,
				"cleared_event_id": clearedEventID,
				"updated_at":       clearedAt,
			}),
		}).Create(&cursor).Error; err != nil {
			return fmt.Errorf("save follow log cursor failed: %w", err)
		}

		attempts := tx.
			Where("user_id = ? AND chain = ? AND created_at <= ?", userID, chain, clearedAt).
			Delete(&models.SmartMoneyFollowAttempt{})
		if attempts.Error != nil {
			return fmt.Errorf("delete follow attempts failed: %w", attempts.Error)
		}
		result.DeletedAttempts = attempts.RowsAffected

		jobs := tx.
			Where("user_id = ? AND chain = ? AND status IN ?", userID, chain, []string{
				models.SmartMoneyFollowJobStatusSuccess,
				models.SmartMoneyFollowJobStatusFailed,
				models.SmartMoneyFollowJobStatusSkipped,
			}).
			Where("created_at <= ?", clearedAt).
			Delete(&models.SmartMoneyFollowJob{})
		if jobs.Error != nil {
			return fmt.Errorf("delete follow jobs failed: %w", jobs.Error)
		}
		result.DeletedJobs = jobs.RowsAffected
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func maxLPEventIDForChain(tx *gorm.DB, chainID int) (uint, error) {
	var event models.SmartMoneyLPEvent
	err := tx.Model(&models.SmartMoneyLPEvent{}).
		Select("id").
		Where("chain_id = ?", chainID).
		Order("id DESC").
		Take(&event).Error
	if err == nil {
		return event.ID, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return 0, fmt.Errorf("load max LP event id failed: %w", err)
}

func followLogCursor(ctx context.Context, userID uint, chain string) (*models.SmartMoneyFollowLogCursor, error) {
	var cursor models.SmartMoneyFollowLogCursor
	err := database.DB.WithContext(ctx).
		Where("user_id = ? AND chain = ?", userID, chain).
		First(&cursor).Error
	if err == nil {
		return &cursor, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("load follow log cursor failed: %w", err)
}

func listRecentTargetEventsForConfigs(ctx context.Context, chainID int, configs []models.SmartMoneyFollowConfig, cursor *models.SmartMoneyFollowLogCursor) ([]models.SmartMoneyLPEvent, error) {
	walletSet := make(map[string]struct{})
	for i := range configs {
		cfg := configs[i]
		wallets, err := configTargetWallets(&cfg)
		if err != nil {
			log.Printf("[SmartMoneyFollow] skip invalid config when listing target events: config_id=%d err=%v", cfg.ID, err)
			continue
		}
		for _, wallet := range wallets {
			walletSet[wallet] = struct{}{}
		}
	}
	if len(walletSet) == 0 {
		return nil, nil
	}

	wallets := make([]string, 0, len(walletSet))
	for wallet := range walletSet {
		wallets = append(wallets, wallet)
	}
	sort.Strings(wallets)

	var events []models.SmartMoneyLPEvent
	query := database.DB.WithContext(ctx).
		Where("wallet_address IN ? AND chain_id = ? AND event_type IN ?", wallets, chainID, []string{"add", "remove"})
	if cursor != nil {
		if cursor.ClearedEventID > 0 {
			query = query.Where("id > ?", cursor.ClearedEventID)
		} else {
			query = query.Where("tx_timestamp > ?", cursor.ClearedAt)
		}
	}
	if err := query.
		Order("id DESC").
		Limit(50).
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("list follow target events failed: %w", err)
	}
	return events, nil
}

func listRecentEventsForJobsAndAttempts(ctx context.Context, jobs []models.SmartMoneyFollowJob, attempts []models.SmartMoneyFollowAttempt) ([]models.SmartMoneyLPEvent, error) {
	eventIDs := followJobAndAttemptEventIDs(jobs, attempts)
	if len(eventIDs) == 0 {
		return nil, nil
	}
	var events []models.SmartMoneyLPEvent
	if err := database.DB.WithContext(ctx).
		Where("id IN ?", eventIDs).
		Order("id DESC").
		Limit(50).
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("list follow job events failed: %w", err)
	}
	return events, nil
}

func followJobAndAttemptEventIDs(jobs []models.SmartMoneyFollowJob, attempts []models.SmartMoneyFollowAttempt) []uint {
	seen := make(map[uint]struct{})
	eventIDs := make([]uint, 0, len(jobs)+len(attempts))
	appendID := func(id uint) {
		if id == 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		eventIDs = append(eventIDs, id)
	}
	for i := range jobs {
		job := jobs[i]
		appendID(job.EventID)
		for _, raw := range job.TriggerEventIDs {
			id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
			if err != nil || id == 0 {
				continue
			}
			appendID(uint(id))
		}
	}
	for i := range attempts {
		appendID(attempts[i].EventID)
	}
	sort.Slice(eventIDs, func(i, j int) bool { return eventIDs[i] > eventIDs[j] })
	if len(eventIDs) > 50 {
		return eventIDs[:50]
	}
	return eventIDs
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
		executionWallet, err := resolveExecutionWalletForSave(ctx, tx, userID, normalized.ExecutionWalletID, normalized.ExecutionWalletAddr)
		if err != nil {
			return err
		}
		normalized.ExecutionWalletID = executionWallet.ID
		normalized.ExecutionWalletAddr = normalizeWalletAddress(executionWallet.Address)

		if normalized.Enabled {
			if err := ensureFollowTargetWalletsMonitored(ctx, tx, chainID, normalized.TargetWallets); err != nil {
				return err
			}
		}

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
				"chain":                    chain,
				"chain_id":                 chainID,
				"target_wallet_address":    normalized.TargetWalletAddress,
				"target_wallet_addresses":  models.StringArray(normalized.TargetWallets),
				"execution_wallet_id":      normalized.ExecutionWalletID,
				"execution_wallet_address": normalized.ExecutionWalletAddr,
				"trigger_mode":             normalized.TriggerMode,
				"trigger_min_wallets":      normalized.TriggerMinWallets,
				"trigger_window_seconds":   normalized.TriggerWindowSeconds,
				"enabled":                  normalized.Enabled,
				"amount_mode":              normalized.AmountMode,
				"fixed_amount_usdt":        normalized.FixedAmountUSDT,
				"ratio":                    normalized.Ratio,
				"delay_mode":               normalized.DelayMode,
				"delay_seconds":            normalized.DelaySeconds,
				"follow_close":             normalized.FollowClose,
				"cursor_event_id":          cursorID,
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
			ExecutionWalletID:    normalized.ExecutionWalletID,
			ExecutionWalletAddr:  normalized.ExecutionWalletAddr,
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

func ensureFollowTargetWalletsMonitored(ctx context.Context, tx *gorm.DB, chainID int, wallets []string) error {
	if tx == nil {
		return fmt.Errorf("db transaction is nil")
	}
	if chainID <= 0 {
		return fmt.Errorf("invalid chain_id")
	}
	if len(wallets) == 0 {
		return fmt.Errorf("target wallet set is empty")
	}
	for _, raw := range wallets {
		addr := normalizeWalletAddress(raw)
		if addr == "" {
			return fmt.Errorf("invalid target wallet address")
		}
		var existing models.MonitoredWallet
		err := tx.WithContext(ctx).
			Where("address = ? AND chain_id = ?", addr, chainID).
			First(&existing).Error
		if err == nil {
			if existing.IsActive {
				continue
			}
			if err := tx.Model(&models.MonitoredWallet{}).
				Where("id = ?", existing.ID).
				Update("is_active", true).Error; err != nil {
				return fmt.Errorf("reactivate follow target wallet failed: %w", err)
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load follow target wallet failed: %w", err)
		}
		wallet := &models.MonitoredWallet{
			Address:  addr,
			ChainID:  chainID,
			Source:   monitoredWalletSourceAutoFollow,
			IsActive: true,
		}
		if err := tx.Create(wallet).Error; err != nil {
			return fmt.Errorf("create follow target wallet failed: %w", err)
		}
	}
	return nil
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
	if strings.TrimSpace(input.ExecutionWalletAddr) != "" {
		executionAddr := normalizeWalletAddress(input.ExecutionWalletAddr)
		if executionAddr == "" {
			return SaveConfigInput{}, fmt.Errorf("invalid execution wallet address")
		}
		input.ExecutionWalletAddr = executionAddr
	}

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
			log.Printf("[SmartMoneyFollow] skip invalid follow config wallet set: config_id=%d err=%v", cfg.ID, err)
			continue
		}
		if len(wallets) == 0 {
			log.Printf("[SmartMoneyFollow] skip config with empty wallet set: config_id=%d", cfg.ID)
			continue
		}
		if err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return ensureFollowTargetWalletsMonitored(ctx, tx, cfg.ChainID, wallets)
		}); err != nil {
			log.Printf("[SmartMoneyFollow] ensure follow target wallets monitored failed: config_id=%d err=%v", cfg.ID, err)
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
				break
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
	matchedConfigs := 0
	for i := range configs {
		cfg := configs[i]
		wallets, err := configTargetWallets(&cfg)
		if err != nil {
			log.Printf("[SmartMoneyFollow] skip invalid follow config wallet set for event: config_id=%d event_id=%d err=%v", cfg.ID, event.ID, err)
			continue
		}
		if !stringSliceContains(wallets, address) {
			continue
		}
		matchedConfigs++
		if err := s.createJobForConfig(ctx, &cfg, event); err != nil {
			log.Printf("[SmartMoneyFollow] create event job failed config_id=%d event_id=%d err=%v", cfg.ID, event.ID, err)
			continue
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
	if matchedConfigs > 0 {
		log.Printf("[SmartMoneyFollow] event processed: event_id=%d wallet=%s chain_id=%d candidate_configs=%d matched_configs=%d advance_cursor=%t",
			event.ID, address, event.ChainID, len(configs), matchedConfigs, advanceCursor)
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
	positionRef := targetPositionRefForFollowJob(cfg, event)
	if action == models.SmartMoneyFollowJobActionOpen && strings.TrimSpace(positionRef) != "" {
		resolvedAction, err := s.resolveOpenEventJobAction(ctx, cfg, event, positionRef)
		if err != nil {
			return err
		}
		action = resolvedAction
	}
	trigger, ok, err := s.resolveJobTrigger(ctx, cfg, event, action)
	if err != nil {
		return err
	}
	if !ok {
		log.Printf("[SmartMoneyFollow] job trigger not ready: config_id=%d event_id=%d trigger_mode=%s action=%s",
			cfg.ID, event.ID, normalizeConfigTriggerMode(cfg.TriggerMode), action)
		recordFollowAttempt(ctx, cfg, event, action, models.SmartMoneyFollowAttemptStatusSkipped, "trigger not ready", nil)
		return nil
	}
	status := models.SmartMoneyFollowJobStatusPending
	errorMessage := ""
	amountUSDT := float64(0)
	if isFollowAmountAction(action) {
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
	if strings.TrimSpace(positionRef) == "" {
		status = models.SmartMoneyFollowJobStatusFailed
		errorMessage = "target position ref is missing"
	}
	if action == models.SmartMoneyFollowJobActionOpen && normalizeConfigTriggerMode(cfg.TriggerMode) == models.SmartMoneyFollowTriggerModeThreshold {
		exists, err := thresholdOpenJobExists(ctx, cfg.ID, positionRef, trigger.PrimaryEventID)
		if err != nil {
			return err
		}
		if exists {
			log.Printf("[SmartMoneyFollow] skip duplicate threshold open job: config_id=%d event_id=%d position_ref=%s",
				cfg.ID, event.ID, positionRef)
			recordFollowAttempt(ctx, cfg, event, action, models.SmartMoneyFollowAttemptStatusSkipped, "duplicate threshold open job", nil)
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
		ExecutionWalletID:   cfg.ExecutionWalletID,
		ExecutionWalletAddr: cfg.ExecutionWalletAddr,
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
	recordFollowAttempt(ctx, cfg, event, action, models.SmartMoneyFollowAttemptStatusMatched, "matched follow config", nil)
	result := database.DB.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&job)
	if result.Error != nil {
		recordFollowAttempt(ctx, cfg, event, action, models.SmartMoneyFollowAttemptStatusFailed, result.Error.Error(), nil)
		return fmt.Errorf("create follow job failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		log.Printf("[SmartMoneyFollow] follow job already exists: config_id=%d event_id=%d action=%s", cfg.ID, trigger.PrimaryEventID, action)
		existing, err := findExistingFollowJob(ctx, cfg.ID, trigger.PrimaryEventID, action)
		if err != nil {
			recordFollowAttempt(ctx, cfg, event, action, models.SmartMoneyFollowAttemptStatusFailed, err.Error(), nil)
			return err
		}
		if existing != nil {
			recordFollowAttempt(ctx, cfg, event, action, models.SmartMoneyFollowAttemptStatusCreated, "", &existing.ID)
			return nil
		}
		recordFollowAttempt(ctx, cfg, event, action, models.SmartMoneyFollowAttemptStatusSkipped, "follow job already exists but row not found", nil)
		return nil
	}
	recordFollowAttempt(ctx, cfg, event, action, models.SmartMoneyFollowAttemptStatusCreated, errorMessage, &job.ID)
	log.Printf("[SmartMoneyFollow] follow job created: job_id=%d config_id=%d event_id=%d action=%s status=%s amount=%.8f error=%s",
		job.ID, cfg.ID, trigger.PrimaryEventID, action, status, amountUSDT, errorMessage)
	return nil
}

func recordFollowAttempt(ctx context.Context, cfg *models.SmartMoneyFollowConfig, event *models.SmartMoneyLPEvent, action string, status string, message string, jobID *uint) {
	if cfg == nil || event == nil || database.DB == nil {
		return
	}
	if cfg.ID == 0 || event.ID == 0 || strings.TrimSpace(action) == "" {
		return
	}
	attempt := models.SmartMoneyFollowAttempt{
		ConfigID:            cfg.ID,
		UserID:              cfg.UserID,
		Chain:               cfg.Chain,
		ChainID:             cfg.ChainID,
		TargetWalletAddress: normalizeWalletAddress(event.WalletAddress),
		ExecutionWalletID:   cfg.ExecutionWalletID,
		ExecutionWalletAddr: cfg.ExecutionWalletAddr,
		EventID:             event.ID,
		Action:              action,
		Status:              status,
		Message:             strings.TrimSpace(message),
		JobID:               jobID,
	}
	now := time.Now()
	updates := map[string]any{
		"user_id":                  attempt.UserID,
		"chain":                    attempt.Chain,
		"chain_id":                 attempt.ChainID,
		"target_wallet_address":    attempt.TargetWalletAddress,
		"execution_wallet_id":      attempt.ExecutionWalletID,
		"execution_wallet_address": attempt.ExecutionWalletAddr,
		"status":                   attempt.Status,
		"message":                  attempt.Message,
		"job_id":                   attempt.JobID,
		"updated_at":               now,
	}
	if err := database.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "config_id"}, {Name: "event_id"}, {Name: "action"}},
			DoUpdates: clause.Assignments(updates),
		}).
		Create(&attempt).Error; err != nil {
		log.Printf("[SmartMoneyFollow] record follow attempt failed: config_id=%d event_id=%d action=%s err=%v", cfg.ID, event.ID, action, err)
	}
}

func findExistingFollowJob(ctx context.Context, configID uint, eventID uint, action string) (*models.SmartMoneyFollowJob, error) {
	var job models.SmartMoneyFollowJob
	err := database.DB.WithContext(ctx).
		Where("config_id = ? AND event_id = ? AND action = ?", configID, eventID, action).
		First(&job).Error
	if err == nil {
		return &job, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("find existing follow job failed: %w", err)
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

func isFollowAmountAction(action string) bool {
	switch strings.TrimSpace(action) {
	case models.SmartMoneyFollowJobActionOpen, models.SmartMoneyFollowJobActionAddLiquidity:
		return true
	default:
		return false
	}
}

func (s *Service) resolveOpenEventJobAction(ctx context.Context, cfg *models.SmartMoneyFollowConfig, event *models.SmartMoneyLPEvent, positionRef string) (string, error) {
	if cfg == nil || event == nil {
		return "", fmt.Errorf("config or event is nil")
	}
	if strings.TrimSpace(positionRef) == "" {
		return models.SmartMoneyFollowJobActionOpen, nil
	}
	existing, err := findExistingFollowJob(ctx, cfg.ID, event.ID, models.SmartMoneyFollowJobActionOpen)
	if err != nil {
		return "", err
	}
	if existing != nil {
		return followOpenEventAction(true, false, false), nil
	}

	var openTasks int64
	if err := database.DB.WithContext(ctx).Model(&models.SmartMoneyFollowTask{}).
		Where("config_id = ? AND target_position_ref = ? AND status = ?", cfg.ID, positionRef, models.SmartMoneyFollowTaskStatusOpen).
		Count(&openTasks).Error; err != nil {
		return "", fmt.Errorf("check open follow task failed: %w", err)
	}
	var openingJobs int64
	if err := database.DB.WithContext(ctx).Model(&models.SmartMoneyFollowJob{}).
		Where("config_id = ? AND target_position_ref = ? AND action = ? AND status IN ?",
			cfg.ID,
			positionRef,
			models.SmartMoneyFollowJobActionOpen,
			[]string{
				models.SmartMoneyFollowJobStatusPending,
				models.SmartMoneyFollowJobStatusRunning,
			}).
		Count(&openingJobs).Error; err != nil {
		return "", fmt.Errorf("check pending open follow job failed: %w", err)
	}
	return followOpenEventAction(false, openTasks > 0, openingJobs > 0), nil
}

func followOpenEventAction(existingSameEventOpen bool, hasOpenTask bool, hasOpeningJob bool) string {
	if existingSameEventOpen {
		return models.SmartMoneyFollowJobActionOpen
	}
	if hasOpenTask || hasOpeningJob {
		return models.SmartMoneyFollowJobActionAddLiquidity
	}
	return models.SmartMoneyFollowJobActionOpen
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

func thresholdOpenJobExists(ctx context.Context, configID uint, positionRef string, eventID uint) (bool, error) {
	if configID == 0 || strings.TrimSpace(positionRef) == "" {
		return false, nil
	}
	var count int64
	query := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyFollowJob{}).
		Where("config_id = ? AND action = ? AND target_position_ref = ? AND status IN ?",
			configID,
			models.SmartMoneyFollowJobActionOpen,
			positionRef,
			[]string{
				models.SmartMoneyFollowJobStatusPending,
				models.SmartMoneyFollowJobStatusRunning,
				models.SmartMoneyFollowJobStatusSuccess,
			})
	if eventID > 0 {
		query = query.Where("event_id <> ?", eventID)
	}
	if err := query.Count(&count).Error; err != nil {
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
	case models.SmartMoneyFollowJobActionAddLiquidity:
		taskID, err = s.executeAddLiquidityJob(ctx, job)
	case models.SmartMoneyFollowJobActionClose:
		taskID, err = s.executeCloseJob(ctx, job)
	default:
		err = fmt.Errorf("invalid follow job action: %s", job.Action)
	}
	if err != nil {
		if errors.Is(err, errFollowJobRetry) {
			return rescheduleJob(ctx, job.ID, 10*time.Second, err.Error())
		}
		if isRetryableFollowSlippageError(err) && job.RetryCount < maxFollowJobRetryCount {
			return rescheduleJobRetry(ctx, job.ID, job.RetryCount+1, followRetryDelay(job.RetryCount+1), err.Error())
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

func rescheduleJobRetry(ctx context.Context, jobID uint, retryCount int, delay time.Duration, message string) error {
	nextAt := time.Now().Add(delay)
	return database.DB.WithContext(ctx).Model(&models.SmartMoneyFollowJob{}).
		Where("id = ?", jobID).
		Updates(map[string]any{
			"status":        models.SmartMoneyFollowJobStatusPending,
			"scheduled_at":  nextAt,
			"retry_count":   retryCount,
			"error_message": fmt.Sprintf("retry %d/%d: %s", retryCount, maxFollowJobRetryCount, message),
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
	mapping, err := findOpenFollowTaskMapping(ctx, cfg.ID, job.TargetPositionRef)
	if err != nil {
		return nil, err
	}
	if mapping != nil {
		taskID := mapping.TaskID
		return &taskID, nil
	}

	task, err := buildFollowTask(ctx, cfg, event, job.AmountUSDT)
	if err != nil {
		return nil, err
	}
	taskID, err := runOpenTaskSync(ctx, cfg, job, task, followJobTxOptions(job, task.SlippageTolerance))
	if err != nil {
		return taskID, err
	}
	return taskID, nil
}

func runOpenTaskSync(ctx context.Context, cfg *models.SmartMoneyFollowConfig, job *models.SmartMoneyFollowJob, task *models.StrategyTask, opts liquidity.TxOptions) (*uint, error) {
	if cfg == nil || job == nil || task == nil {
		return nil, fmt.Errorf("config, job or task is nil")
	}
	var taskID uint
	err := runTaskSync(cfg.UserID, task, func() error {
		if job.TaskID != nil && *job.TaskID > 0 {
			taskID = *job.TaskID
			existing, err := strategy.NewStrategyTaskService().GetByID(cfg.UserID, taskID)
			if err != nil {
				return err
			}
			if !existing.IsFollow {
				return fmt.Errorf("task is not a follow task")
			}
			task = existing
		} else {
			if err := strategy.CreateTaskRecord(task); err != nil {
				return fmt.Errorf("create strategy task failed: %w", err)
			}
			taskID = task.ID
			if err := database.DB.WithContext(ctx).Model(&models.SmartMoneyFollowJob{}).
				Where("id = ? AND task_id IS NULL", job.ID).
				Update("task_id", taskID).Error; err != nil {
				return fmt.Errorf("bind follow job task failed: %w", err)
			}
		}

		enterRes, err := liquidity.NewLiquidityService().EnterTaskFromUSDTWithOptions(cfg.UserID, task, opts)
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
			ExecutionWalletID:   task.WalletID,
			ExecutionWalletAddr: normalizeWalletAddress(task.WalletAddress),
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

func (s *Service) executeAddLiquidityJob(ctx context.Context, job *models.SmartMoneyFollowJob) (*uint, error) {
	cfg, _, err := loadJobConfigAndEvent(ctx, job)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("%w: follow config disabled", errFollowJobSkipped)
	}
	if job.AmountUSDT <= 0 {
		return nil, fmt.Errorf("follow add liquidity amount must be greater than 0")
	}

	mapping, err := findOpenFollowTaskMapping(ctx, cfg.ID, job.TargetPositionRef)
	if err != nil {
		return nil, err
	}
	if mapping == nil {
		opening, err := openFollowJobInProgress(ctx, cfg.ID, job.TargetPositionRef)
		if err != nil {
			return nil, err
		}
		if opening {
			return nil, fmt.Errorf("%w: open follow task is not ready", errFollowJobRetry)
		}
		return nil, fmt.Errorf("%w: open follow task not found", errFollowJobSkipped)
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
		return &taskID, fmt.Errorf("%w: follow task already stopped", errFollowJobSkipped)
	}
	if !taskHasPositionToken(task) {
		return &taskID, fmt.Errorf("%w: follow task has no on-chain position yet", errFollowJobRetry)
	}

	runErr := runTaskSync(cfg.UserID, task, func() error {
		res, err := liquidity.NewLiquidityService().IncreaseLiquidityForTaskWithOptions(cfg.UserID, task, job.AmountUSDT, followJobTxOptions(job, task.SlippageTolerance))
		if err != nil {
			return err
		}
		return applyIncreaseLiquidityResult(ctx, task, job.AmountUSDT, res)
	})
	if runErr != nil {
		return &taskID, runErr
	}
	return &taskID, nil
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

func findOpenFollowTaskMapping(ctx context.Context, configID uint, positionRef string) (*models.SmartMoneyFollowTask, error) {
	if configID == 0 || strings.TrimSpace(positionRef) == "" {
		return nil, nil
	}
	var mapping models.SmartMoneyFollowTask
	if err := database.DB.WithContext(ctx).
		Where("config_id = ? AND target_position_ref = ? AND status = ?", configID, positionRef, models.SmartMoneyFollowTaskStatusOpen).
		Order("id DESC").
		First(&mapping).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &mapping, nil
}

func openFollowJobInProgress(ctx context.Context, configID uint, positionRef string) (bool, error) {
	if configID == 0 || strings.TrimSpace(positionRef) == "" {
		return false, nil
	}
	var count int64
	if err := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyFollowJob{}).
		Where("config_id = ? AND target_position_ref = ? AND action = ? AND status IN ?",
			configID,
			positionRef,
			models.SmartMoneyFollowJobActionOpen,
			[]string{
				models.SmartMoneyFollowJobStatusPending,
				models.SmartMoneyFollowJobStatusRunning,
			}).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func taskHasPositionToken(task *models.StrategyTask) bool {
	if task == nil {
		return false
	}
	if strings.TrimSpace(task.V3TokenID) != "" && strings.TrimSpace(task.V3TokenID) != "0" {
		return true
	}
	return strings.TrimSpace(task.V4TokenID) != "" && strings.TrimSpace(task.V4TokenID) != "0"
}

func followJobTxOptions(job *models.SmartMoneyFollowJob, baseSlippage float64) liquidity.TxOptions {
	opts := liquidity.TxOptions{}
	if job == nil || job.RetryCount <= 0 {
		return opts
	}
	slippage := followRetrySlippagePercent(baseSlippage, job.RetryCount)
	opts.SlippageToleranceOverride = &slippage
	opts.EntrySwapSlippageOverride = &slippage
	opts.GasMultiplier = followRetryGasMultiplier(job.RetryCount)
	return opts
}

func followRetryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return 500 * time.Millisecond
	}
	delays := []time.Duration{
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		3 * time.Second,
		5 * time.Second,
		10 * time.Second,
	}
	idx := attempt - 1
	if idx >= len(delays) {
		return delays[len(delays)-1]
	}
	return delays[idx]
}

func followRetrySlippagePercent(base float64, attempt int) float64 {
	if base <= 0 {
		base = 0.5
	}
	if attempt <= 0 {
		return base
	}
	widened := base * math.Pow(2, float64(attempt))
	if widened > 10 {
		return 10
	}
	return widened
}

func followRetryGasMultiplier(attempt int) float64 {
	if attempt <= 0 {
		return 1
	}
	multiplier := 1.0 + 0.25*float64(attempt)
	if multiplier > 3 {
		return 3
	}
	return multiplier
}

func isRetryableFollowSlippageError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	if text == "" {
		return false
	}
	markers := []string{
		"slippage",
		"price move",
		"price moved",
		"too little received",
		"insufficient_output_amount",
		"minimum amount",
		"maximum amount exceeded",
		"maximumamountexceeded",
		"0x31e30ad0",
	}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
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

func buildFollowTask(ctx context.Context, cfg *models.SmartMoneyFollowConfig, event *models.SmartMoneyLPEvent, amountUSDT float64) (*models.StrategyTask, error) {
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

	walletRow, err := resolveExecutionWalletForRun(ctx, cfg)
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

	task := &models.StrategyTask{
		UserID:                 cfg.UserID,
		Chain:                  cfg.Chain,
		PoolId:                 poolID,
		PoolVersion:            poolVersion,
		Exchange:               strings.TrimSpace(poolInfo.Exchange),
		WalletID:               walletRow.ID,
		WalletAddress:          normalizeWalletAddress(walletRow.Address),
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
	}
	if err := fillFollowTaskRangePercentages(task); err != nil {
		return nil, err
	}
	return task, nil
}

func fillFollowTaskRangePercentages(task *models.StrategyTask) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}
	rangePct, err := followRangePercentFromTicks(task.TickLower, task.TickUpper)
	if err != nil {
		return err
	}
	task.RangeLowerPercentage = rangePct
	task.RangeUpperPercentage = rangePct
	task.RangePercentage = rangePct
	return nil
}

func followRangePercentFromTicks(tickLower int, tickUpper int) (float64, error) {
	if tickUpper <= tickLower {
		return 0, fmt.Errorf("invalid tick range")
	}
	lowerPrice := math.Pow(1.0001, float64(tickLower))
	upperPrice := math.Pow(1.0001, float64(tickUpper))
	if lowerPrice <= 0 || upperPrice <= 0 || math.IsNaN(lowerPrice) || math.IsNaN(upperPrice) || math.IsInf(lowerPrice, 0) || math.IsInf(upperPrice, 0) {
		return 0, fmt.Errorf("invalid tick price range")
	}
	rangePct := ((upperPrice - lowerPrice) / (upperPrice + lowerPrice)) * 100.0
	if rangePct <= 0 {
		return 0, fmt.Errorf("invalid range width")
	}
	return rangePct, nil
}

func backfillFollowTaskRangePercentages(ctx context.Context) error {
	var tasks []models.StrategyTask
	if err := database.DB.WithContext(ctx).
		Where("is_follow = ? AND tick_lower < tick_upper", true).
		Where("range_percentage <= 0 OR range_lower_percentage <= 0 OR range_upper_percentage <= 0").
		Order("id DESC").
		Limit(100).
		Find(&tasks).Error; err != nil {
		return fmt.Errorf("list follow tasks missing range width failed: %w", err)
	}
	for i := range tasks {
		task := tasks[i]
		if err := fillFollowTaskRangePercentages(&task); err != nil {
			return fmt.Errorf("calculate follow task range width failed: task_id=%d err=%w", task.ID, err)
		}
		if err := database.DB.WithContext(ctx).Model(&models.StrategyTask{}).
			Where("id = ?", task.ID).
			Updates(map[string]any{
				"range_percentage":       task.RangePercentage,
				"range_lower_percentage": task.RangeLowerPercentage,
				"range_upper_percentage": task.RangeUpperPercentage,
			}).Error; err != nil {
			return fmt.Errorf("backfill follow task range width failed: task_id=%d err=%w", task.ID, err)
		}
	}
	return nil
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

func applyIncreaseLiquidityResult(ctx context.Context, task *models.StrategyTask, requestedAmountUSDT float64, res *liquidity.IncreaseLiquidityResult) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}
	spent := requestedAmountUSDT
	if res != nil && res.ActualStableSpent > 0 {
		spent = res.ActualStableSpent
	}

	updates := map[string]any{
		"amount_usdt":     task.AmountUSDT + spent,
		"error_message":   "",
		"status":          models.StrategyStatusRunning,
		"last_check_time": time.Now(),
	}
	if res != nil && strings.TrimSpace(res.CurrentLiquidity) != "" {
		updates["current_liquidity"] = res.CurrentLiquidity
	}
	if res != nil && res.TickLower != nil && res.TickUpper != nil && *res.TickLower < *res.TickUpper {
		updates["tick_lower"] = *res.TickLower
		updates["tick_upper"] = *res.TickUpper
	}
	if err := database.DB.WithContext(ctx).Model(task).Updates(updates).Error; err != nil {
		return fmt.Errorf("update follow add liquidity task failed: %w", err)
	}

	var deltaWei *big.Int
	if res != nil && res.ActualStableSpentWei != nil && res.ActualStableSpentWei.Sign() > 0 {
		deltaWei = res.ActualStableSpentWei
	} else if conv, convErr := convert.FloatUSDTToWei(spent); convErr == nil && conv != nil && conv.Sign() > 0 {
		deltaWei = conv
	}

	extraDust := []models.TradeRecordDustAsset(nil)
	var gasSpent *big.Int
	var dust0 *big.Int
	var dust1 *big.Int
	if res != nil {
		extraDust = res.ExtraDust
		gasSpent = res.GasSpentWei
		dust0 = res.Dust0Wei
		dust1 = res.Dust1Wei
	}
	if tradeErr := trade.NewTradeRecordService().ApplyAddLiquidityDelta(task, deltaWei, gasSpent, dust0, dust1, extraDust...); tradeErr != nil {
		log.Printf("[SmartMoneyFollow] add liquidity trade record update failed: task_id=%d err=%v", task.ID, tradeErr)
	}
	return nil
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

func resolveExecutionWalletForSave(ctx context.Context, tx *gorm.DB, userID uint, walletID uint, walletAddress string) (*models.Wallet, error) {
	if userID == 0 {
		return nil, fmt.Errorf("invalid user_id")
	}
	if walletID == 0 && strings.TrimSpace(walletAddress) == "" {
		return nil, fmt.Errorf("execution wallet is required")
	}
	db := tx.WithContext(ctx).Where("user_id = ?", userID)
	if walletID != 0 {
		db = db.Where("id = ?", walletID)
	} else {
		addr := normalizeWalletAddress(walletAddress)
		if addr == "" {
			return nil, fmt.Errorf("invalid execution wallet address")
		}
		db = db.Where("LOWER(address) = ?", addr)
	}
	var row models.Wallet
	if err := db.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("execution wallet not found")
		}
		return nil, fmt.Errorf("load execution wallet failed: %w", err)
	}
	return &row, nil
}

func resolveExecutionWalletForRun(ctx context.Context, cfg *models.SmartMoneyFollowConfig) (*models.Wallet, error) {
	if cfg == nil {
		return nil, fmt.Errorf("follow config is nil")
	}
	ws := wallet.NewWalletService()
	if cfg.ExecutionWalletID != 0 {
		walletRow, err := ws.GetWalletByID(cfg.UserID, cfg.ExecutionWalletID)
		if err != nil {
			return nil, fmt.Errorf("load execution wallet failed: %w", err)
		}
		return walletRow, nil
	}
	if strings.TrimSpace(cfg.ExecutionWalletAddr) != "" {
		walletRow, err := ws.GetWalletByAddress(cfg.UserID, cfg.ExecutionWalletAddr)
		if err != nil {
			return nil, fmt.Errorf("load execution wallet failed: %w", err)
		}
		return walletRow, nil
	}
	var row models.Wallet
	err := database.DB.WithContext(ctx).
		Where("user_id = ? AND is_default = ?", cfg.UserID, true).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = database.DB.WithContext(ctx).
				Where("user_id = ?", cfg.UserID).
				Order("id ASC").
				First(&row).Error
			if err != nil {
				return nil, fmt.Errorf("no execution wallet found: %w", err)
			}
			return &row, nil
		}
		return nil, fmt.Errorf("load default execution wallet failed: %w", err)
	}
	return &row, nil
}

func listExecutionWalletOptions(ctx context.Context, userID uint) ([]ExecutionWalletOption, *models.Wallet, error) {
	var rows []models.Wallet
	if err := database.DB.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("is_default DESC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("list execution wallets failed: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil, fmt.Errorf("no execution wallet found")
	}
	adminAddr := ""
	if config.AppConfig != nil {
		adminAddr = normalizeWalletAddress(config.AppConfig.AdminWalletAddress)
	}
	options := make([]ExecutionWalletOption, 0, len(rows))
	var defaultWallet *models.Wallet
	for i := range rows {
		row := rows[i]
		if adminAddr != "" && normalizeWalletAddress(row.Address) == adminAddr {
			continue
		}
		if row.IsDefault {
			defaultWallet = &rows[i]
		}
		options = append(options, ExecutionWalletOption{
			ID:        row.ID,
			Address:   normalizeWalletAddress(row.Address),
			Name:      strings.TrimSpace(row.Name),
			IsDefault: row.IsDefault,
		})
	}
	if len(options) == 0 {
		return nil, nil, fmt.Errorf("no execution wallet found")
	}
	if defaultWallet == nil {
		for i := range rows {
			if normalizeWalletAddress(rows[i].Address) == options[0].Address {
				defaultWallet = &rows[i]
				break
			}
		}
	}
	return options, defaultWallet, nil
}

func fillConfigExecutionWallet(cfg *models.SmartMoneyFollowConfig, defaultWallet *models.Wallet) {
	if cfg == nil {
		return
	}
	if cfg.ExecutionWalletID != 0 && strings.TrimSpace(cfg.ExecutionWalletAddr) != "" {
		cfg.ExecutionWalletAddr = normalizeWalletAddress(cfg.ExecutionWalletAddr)
		return
	}
	if defaultWallet == nil {
		return
	}
	if cfg.ExecutionWalletID == 0 {
		cfg.ExecutionWalletID = defaultWallet.ID
	}
	if strings.TrimSpace(cfg.ExecutionWalletAddr) == "" {
		cfg.ExecutionWalletAddr = normalizeWalletAddress(defaultWallet.Address)
	}
}

func fillJobExecutionWallet(job *models.SmartMoneyFollowJob, defaultWallet *models.Wallet) {
	if job == nil {
		return
	}
	if job.ExecutionWalletID != 0 && strings.TrimSpace(job.ExecutionWalletAddr) != "" {
		job.ExecutionWalletAddr = normalizeWalletAddress(job.ExecutionWalletAddr)
		return
	}
	if defaultWallet == nil {
		return
	}
	if job.ExecutionWalletID == 0 {
		job.ExecutionWalletID = defaultWallet.ID
	}
	if strings.TrimSpace(job.ExecutionWalletAddr) == "" {
		job.ExecutionWalletAddr = normalizeWalletAddress(defaultWallet.Address)
	}
}

func fillAttemptExecutionWallet(attempt *models.SmartMoneyFollowAttempt, defaultWallet *models.Wallet) {
	if attempt == nil {
		return
	}
	if attempt.ExecutionWalletID != 0 && strings.TrimSpace(attempt.ExecutionWalletAddr) != "" {
		attempt.ExecutionWalletAddr = normalizeWalletAddress(attempt.ExecutionWalletAddr)
		return
	}
	if defaultWallet == nil {
		return
	}
	if attempt.ExecutionWalletID == 0 {
		attempt.ExecutionWalletID = defaultWallet.ID
	}
	if strings.TrimSpace(attempt.ExecutionWalletAddr) == "" {
		attempt.ExecutionWalletAddr = normalizeWalletAddress(defaultWallet.Address)
	}
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
