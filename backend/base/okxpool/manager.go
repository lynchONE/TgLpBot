package okxpool

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type EffectiveConfig struct {
	Source     Source               `json:"source"`
	BaseURL    string               `json:"base_url"`
	APIKey     string               `json:"-"`
	SecretKey  string               `json:"-"`
	Passphrase string               `json:"-"`
	Config     *models.OKXAPIConfig `json:"config,omitempty"`
}

type AvailableConfig struct {
	Source     Source               `json:"source"`
	BaseURL    string               `json:"base_url"`
	APIKey     string               `json:"-"`
	SecretKey  string               `json:"-"`
	Passphrase string               `json:"-"`
	Config     *models.OKXAPIConfig `json:"config,omitempty"`
}

type Input struct {
	Name       string
	BaseURL    string
	APIKey     string
	SecretKey  string
	Passphrase string
	SetCurrent bool
}

type Prober interface {
	Probe(ctx context.Context, cfg EffectiveConfig) (time.Duration, error)
}

type Manager struct {
	store Store
	now   func() time.Time

	prober Prober

	failureThreshold   int
	tempDisableFor     time.Duration
	maxLastErrorLength int
}

func NewManager(store Store, prober Prober) *Manager {
	return &Manager{
		store:              store,
		now:                time.Now,
		prober:             prober,
		failureThreshold:   2,
		tempDisableFor:     10 * time.Minute,
		maxLastErrorLength: 480,
	}
}

var (
	defaultOnce    sync.Once
	defaultManager *Manager
)

func Default() *Manager {
	defaultOnce.Do(func() {
		defaultManager = NewManager(NewGormStore(), &HTTPProber{Timeout: 12 * time.Second})
	})
	return defaultManager
}

func EnvFromConfig() EffectiveConfig {
	if config.AppConfig == nil {
		return EffectiveConfig{Source: SourceEnv}
	}
	return EffectiveConfig{
		Source:     SourceEnv,
		BaseURL:    normalizeBaseURL(config.AppConfig.OKXDexAPIURL),
		APIKey:     strings.TrimSpace(config.AppConfig.OKXAPIKey),
		SecretKey:  strings.TrimSpace(config.AppConfig.OKXSecretKey),
		Passphrase: strings.TrimSpace(config.AppConfig.OKXPassphrase),
	}
}

func (m *Manager) Effective(ctx context.Context) (EffectiveConfig, error) {
	if m == nil {
		return EffectiveConfig{}, fmt.Errorf("okxpool manager is nil")
	}
	env := EnvFromConfig()
	if m.store == nil {
		return env, nil
	}

	list, err := m.store.ListEnabled(ctx)
	if err != nil || len(list) == 0 {
		return env, nil
	}

	now := m.now()
	var firstAvailable *models.OKXAPIConfig
	for i := range list {
		row := list[i]
		if !isAvailable(row, now) {
			continue
		}
		if firstAvailable == nil {
			firstAvailable = &row
		}
		if row.IsCurrent {
			return m.effectiveFromRow(row)
		}
	}

	if firstAvailable != nil {
		_ = m.store.SetCurrent(ctx, firstAvailable.ID)
		firstAvailable.IsCurrent = true
		return m.effectiveFromRow(*firstAvailable)
	}

	_ = m.store.UnsetCurrent(ctx)
	return env, nil
}

func (m *Manager) Available(ctx context.Context) ([]AvailableConfig, error) {
	if m == nil {
		return nil, fmt.Errorf("okxpool manager is nil")
	}
	env := EnvFromConfig()
	if m.store == nil {
		if env.BaseURL == "" {
			return nil, nil
		}
		return []AvailableConfig{{Source: SourceEnv, BaseURL: env.BaseURL, APIKey: env.APIKey, SecretKey: env.SecretKey, Passphrase: env.Passphrase}}, nil
	}

	list, err := m.store.ListEnabled(ctx)
	if err != nil || len(list) == 0 {
		if env.BaseURL == "" {
			return nil, nil
		}
		return []AvailableConfig{{Source: SourceEnv, BaseURL: env.BaseURL, APIKey: env.APIKey, SecretKey: env.SecretKey, Passphrase: env.Passphrase}}, nil
	}

	now := m.now()
	out := make([]AvailableConfig, 0, len(list))
	for i := range list {
		row := list[i]
		if !isAvailable(row, now) {
			continue
		}
		eff, err := m.effectiveFromRow(row)
		if err != nil {
			continue
		}
		out = append(out, AvailableConfig{Source: eff.Source, BaseURL: eff.BaseURL, APIKey: eff.APIKey, SecretKey: eff.SecretKey, Passphrase: eff.Passphrase, Config: eff.Config})
	}
	if len(out) == 0 && env.BaseURL != "" {
		out = append(out, AvailableConfig{Source: SourceEnv, BaseURL: env.BaseURL, APIKey: env.APIKey, SecretKey: env.SecretKey, Passphrase: env.Passphrase})
	}
	return out, nil
}

func (m *Manager) ListAll(ctx context.Context) ([]models.OKXAPIConfig, error) {
	if m == nil || m.store == nil {
		return nil, fmt.Errorf("okxpool store not available")
	}
	return m.store.ListAll(ctx)
}

func (m *Manager) AddConfig(ctx context.Context, input Input) (*models.OKXAPIConfig, error) {
	if m == nil || m.store == nil {
		return nil, fmt.Errorf("okxpool store not available")
	}
	row, err := normalizeInput(input, nil)
	if err != nil {
		return nil, err
	}
	if err := m.store.Create(ctx, row); err != nil {
		return nil, err
	}
	if input.SetCurrent {
		if err := m.SwitchCurrent(ctx, row.ID); err != nil {
			return row, err
		}
		row.IsCurrent = true
	}
	return row, nil
}

func (m *Manager) UpdateConfig(ctx context.Context, id uint, input Input) (*models.OKXAPIConfig, error) {
	if m == nil || m.store == nil {
		return nil, fmt.Errorf("okxpool store not available")
	}
	existing, err := m.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("okx api config not found")
	}
	row, err := normalizeInput(input, existing)
	if err != nil {
		return nil, err
	}
	updates := map[string]interface{}{
		"Name":                row.Name,
		"BaseURL":             row.BaseURL,
		"APIKey":              row.APIKey,
		"SecretKeyEncrypted":  row.SecretKeyEncrypted,
		"PassphraseEncrypted": row.PassphraseEncrypted,
	}
	if err := m.store.UpdateByID(ctx, id, updates); err != nil {
		return nil, err
	}
	if input.SetCurrent {
		if err := m.SwitchCurrent(ctx, id); err != nil {
			return nil, err
		}
	}
	return m.store.GetByID(ctx, id)
}

func (m *Manager) RenameConfig(ctx context.Context, id uint, name string) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("okxpool store not available")
	}
	row, err := m.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("okx api config not found")
	}
	name, err = normalizeName(name, row.BaseURL)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("okx config name is empty")
	}
	return m.store.UpdateByID(ctx, id, map[string]interface{}{"Name": name})
}

func (m *Manager) SwitchCurrent(ctx context.Context, id uint) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("okxpool store not available")
	}
	row, err := m.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("okx api config not found")
	}
	if !row.IsEnabled {
		return fmt.Errorf("okx api config is disabled")
	}
	if !isAvailable(*row, m.now()) {
		return fmt.Errorf("okx api config is unavailable")
	}
	return m.store.SetCurrent(ctx, id)
}

func (m *Manager) DisableConfig(ctx context.Context, id uint, until time.Time, reason string) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("okxpool store not available")
	}
	row, err := m.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("okx api config not found")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = ReasonManual
	}
	updates := map[string]interface{}{
		"DisabledUntil":  &until,
		"DisabledReason": reason,
		"IsCurrent":      false,
	}
	if err := m.store.UpdateByID(ctx, id, updates); err != nil {
		return err
	}
	_, _ = m.Effective(ctx)
	return nil
}

func (m *Manager) DisableUntilNextMonth(ctx context.Context, id uint) error {
	return m.DisableConfig(ctx, id, nextMonthStart(m.now()), ReasonQuotaExhausted)
}

func (m *Manager) EnableConfig(ctx context.Context, id uint) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("okxpool store not available")
	}
	row, err := m.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("okx api config not found")
	}
	if err := m.store.UpdateByID(ctx, id, map[string]interface{}{
		"IsEnabled":           true,
		"DisabledUntil":       nil,
		"DisabledReason":      "",
		"ConsecutiveFailures": 0,
		"LastError":           "",
		"LastLatencyMs":       int64(0),
		"LastCheckedAt":       nil,
		"LastSuccessAt":       nil,
	}); err != nil {
		return err
	}
	_, _ = m.Effective(ctx)
	return nil
}

func (m *Manager) DisableEnabledFlag(ctx context.Context, id uint) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("okxpool store not available")
	}
	row, err := m.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("okx api config not found")
	}
	if err := m.store.UpdateByID(ctx, id, map[string]interface{}{
		"IsEnabled":      false,
		"IsCurrent":      false,
		"DisabledUntil":  nil,
		"DisabledReason": ReasonManual,
	}); err != nil {
		return err
	}
	if row.IsCurrent {
		_, _ = m.Effective(ctx)
	}
	return nil
}

func (m *Manager) DeleteConfig(ctx context.Context, id uint) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("okxpool store not available")
	}
	row, err := m.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("okx api config not found")
	}
	wasCurrent := row.IsCurrent
	if err := m.store.DeleteByID(ctx, id); err != nil {
		return err
	}
	if wasCurrent {
		_, _ = m.Effective(ctx)
	}
	return nil
}

func (m *Manager) RecordSuccess(ctx context.Context, eff EffectiveConfig, latency time.Duration) {
	if m == nil || m.store == nil || eff.Config == nil || eff.Config.ID == 0 {
		return
	}
	now := m.now()
	_ = m.store.UpdateByID(ctx, eff.Config.ID, map[string]interface{}{
		"LastCheckedAt":       &now,
		"LastSuccessAt":       &now,
		"LastLatencyMs":       latency.Milliseconds(),
		"LastError":           "",
		"ConsecutiveFailures": 0,
		"DisabledUntil":       nil,
		"DisabledReason":      "",
	})
}

func (m *Manager) RecordFailure(ctx context.Context, eff EffectiveConfig, latency time.Duration, err error) {
	if m == nil || m.store == nil || eff.Config == nil || eff.Config.ID == 0 {
		return
	}
	id := eff.Config.ID
	now := m.now()
	failures := eff.Config.ConsecutiveFailures + 1
	reason := ReasonHealthFail
	var disabledUntil *time.Time
	unsetCurrent := false

	if IsQuotaExhaustedError(err) {
		reason = ReasonQuotaExhausted
		until := nextMonthStart(now)
		disabledUntil = &until
		unsetCurrent = true
	} else if IsAuthError(err) {
		reason = ReasonAuthFail
		until := now.Add(m.tempDisableFor)
		disabledUntil = &until
		unsetCurrent = true
	} else if IsRateLimitedError(err) {
		reason = ReasonRateLimited
		until := now.Add(m.tempDisableFor)
		disabledUntil = &until
		unsetCurrent = true
	} else if failures >= m.failureThreshold {
		reason = ReasonHealthFail
		until := now.Add(m.tempDisableFor)
		disabledUntil = &until
		unsetCurrent = true
	}

	updates := map[string]interface{}{
		"LastCheckedAt":       &now,
		"LastLatencyMs":       latency.Milliseconds(),
		"LastError":           truncateString(err.Error(), m.maxLastErrorLength),
		"ConsecutiveFailures": failures,
	}
	if disabledUntil != nil {
		updates["DisabledUntil"] = disabledUntil
		updates["DisabledReason"] = reason
	}
	if unsetCurrent {
		updates["IsCurrent"] = false
	}
	_ = m.store.UpdateByID(ctx, id, updates)
	if unsetCurrent {
		_, _ = m.Effective(ctx)
	}
}

func (m *Manager) CheckOne(ctx context.Context, id uint) error {
	if m == nil || m.store == nil || m.prober == nil {
		return fmt.Errorf("okxpool not ready")
	}
	row, err := m.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("okx api config not found")
	}
	eff, err := m.effectiveFromRow(*row)
	if err != nil {
		return err
	}
	start := time.Now()
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	latency, err := m.prober.Probe(probeCtx, eff)
	cancel()
	if latency <= 0 {
		latency = time.Since(start)
	}
	if err != nil {
		m.RecordFailure(ctx, eff, latency, err)
		return err
	}
	m.RecordSuccess(ctx, eff, latency)
	return nil
}

func (m *Manager) CheckAllOnce(ctx context.Context) error {
	if m == nil || m.store == nil || m.prober == nil {
		return nil
	}
	rows, err := m.store.ListEnabled(ctx)
	if err != nil {
		return err
	}
	now := m.now()
	for _, row := range rows {
		if row.DisabledUntil != nil && now.Before(*row.DisabledUntil) {
			continue
		}
		eff, err := m.effectiveFromRow(row)
		if err != nil {
			continue
		}
		start := time.Now()
		probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		latency, probeErr := m.prober.Probe(probeCtx, eff)
		cancel()
		if latency <= 0 {
			latency = time.Since(start)
		}
		if probeErr != nil {
			m.RecordFailure(ctx, eff, latency, probeErr)
			continue
		}
		m.RecordSuccess(ctx, eff, latency)
	}
	return nil
}

func (m *Manager) WithNow(fn func() time.Time) *Manager {
	if m == nil || fn == nil {
		return m
	}
	m.now = fn
	return m
}

func (m *Manager) effectiveFromRow(row models.OKXAPIConfig) (EffectiveConfig, error) {
	if strings.TrimSpace(row.BaseURL) == "" {
		return EffectiveConfig{}, fmt.Errorf("okx api config base_url is empty")
	}
	if strings.TrimSpace(row.APIKey) == "" {
		return EffectiveConfig{}, fmt.Errorf("okx api config api_key is empty")
	}
	secret, err := decryptSecret(row.SecretKeyEncrypted)
	if err != nil {
		return EffectiveConfig{}, fmt.Errorf("decrypt okx secret failed: %w", err)
	}
	pass, err := decryptSecret(row.PassphraseEncrypted)
	if err != nil {
		return EffectiveConfig{}, fmt.Errorf("decrypt okx passphrase failed: %w", err)
	}
	if strings.TrimSpace(secret) == "" {
		return EffectiveConfig{}, fmt.Errorf("okx api config secret is empty")
	}
	if strings.TrimSpace(pass) == "" {
		return EffectiveConfig{}, fmt.Errorf("okx api config passphrase is empty")
	}
	cp := row
	return EffectiveConfig{
		Source:     SourceDB,
		BaseURL:    normalizeBaseURL(row.BaseURL),
		APIKey:     strings.TrimSpace(row.APIKey),
		SecretKey:  secret,
		Passphrase: pass,
		Config:     &cp,
	}, nil
}

func normalizeInput(input Input, existing *models.OKXAPIConfig) (*models.OKXAPIConfig, error) {
	row := &models.OKXAPIConfig{}
	if existing != nil {
		*row = *existing
	}

	if strings.TrimSpace(input.BaseURL) != "" || existing == nil {
		row.BaseURL = normalizeBaseURL(input.BaseURL)
	}
	if err := validateBaseURL(row.BaseURL); err != nil {
		return nil, err
	}

	name, err := normalizeName(input.Name, row.BaseURL)
	if err != nil {
		return nil, err
	}
	if name != "" || existing == nil {
		row.Name = name
	}

	if strings.TrimSpace(input.APIKey) != "" || existing == nil {
		row.APIKey = strings.TrimSpace(input.APIKey)
	}
	if row.APIKey == "" {
		return nil, fmt.Errorf("okx api_key is required")
	}
	if len(row.APIKey) > 255 {
		return nil, fmt.Errorf("okx api_key too long (max 255 chars)")
	}

	if strings.TrimSpace(input.SecretKey) != "" || existing == nil {
		if strings.TrimSpace(input.SecretKey) == "" {
			return nil, fmt.Errorf("okx secret is required")
		}
		enc, err := encryptSecret(input.SecretKey)
		if err != nil {
			return nil, err
		}
		row.SecretKeyEncrypted = enc
	}
	if row.SecretKeyEncrypted == "" {
		return nil, fmt.Errorf("okx secret is required")
	}

	if strings.TrimSpace(input.Passphrase) != "" || existing == nil {
		if strings.TrimSpace(input.Passphrase) == "" {
			return nil, fmt.Errorf("okx passphrase is required")
		}
		enc, err := encryptSecret(input.Passphrase)
		if err != nil {
			return nil, err
		}
		row.PassphraseEncrypted = enc
	}
	if row.PassphraseEncrypted == "" {
		return nil, fmt.Errorf("okx passphrase is required")
	}

	if existing == nil {
		row.IsEnabled = true
	}
	return row, nil
}

func isAvailable(row models.OKXAPIConfig, now time.Time) bool {
	if !row.IsEnabled {
		return false
	}
	if row.DisabledUntil != nil && now.Before(*row.DisabledUntil) {
		return false
	}
	return strings.TrimSpace(row.BaseURL) != "" && strings.TrimSpace(row.APIKey) != ""
}
