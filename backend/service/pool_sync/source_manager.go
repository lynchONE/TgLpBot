package pool_sync

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

const (
	PoolDataSourceTypePoolMTopFees = "poolm_top_fees"
	PoolDataSourceTypeMarketPools  = "market_pools"
)

type PoolDataSourceStore interface {
	ListAll(ctx context.Context) ([]models.PoolDataSource, error)
	List(ctx context.Context, chain string, timeframeMinutes int) ([]models.PoolDataSource, error)
	GetByID(ctx context.Context, id uint) (*models.PoolDataSource, error)
	Create(ctx context.Context, source *models.PoolDataSource) error
	UpdateByID(ctx context.Context, id uint, updates map[string]interface{}) error
	DeleteByID(ctx context.Context, id uint) error
	SetCurrent(ctx context.Context, chain string, timeframeMinutes int, id uint) error
	UnsetCurrent(ctx context.Context, chain string, timeframeMinutes int) error
}

type GormPoolDataSourceStore struct{}

func NewGormPoolDataSourceStore() *GormPoolDataSourceStore { return &GormPoolDataSourceStore{} }

func (s *GormPoolDataSourceStore) db() (*gorm.DB, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	return database.DB, nil
}

func (s *GormPoolDataSourceStore) ListAll(ctx context.Context) ([]models.PoolDataSource, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	var out []models.PoolDataSource
	if err := q.Order("chain asc, timeframe_minutes asc, is_current desc, id asc").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GormPoolDataSourceStore) List(ctx context.Context, chain string, timeframeMinutes int) ([]models.PoolDataSource, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	var out []models.PoolDataSource
	if err := q.Where("chain = ? AND timeframe_minutes = ?", chain, timeframeMinutes).
		Order("is_current desc, id asc").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GormPoolDataSourceStore) GetByID(ctx context.Context, id uint) (*models.PoolDataSource, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	var source models.PoolDataSource
	if err := q.First(&source, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &source, nil
}

func (s *GormPoolDataSourceStore) Create(ctx context.Context, source *models.PoolDataSource) error {
	if source == nil {
		return fmt.Errorf("pool data source is nil")
	}
	db, err := s.db()
	if err != nil {
		return err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	return q.Create(source).Error
}

func (s *GormPoolDataSourceStore) UpdateByID(ctx context.Context, id uint, updates map[string]interface{}) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	return q.Model(&models.PoolDataSource{}).Where("id = ?", id).Updates(updates).Error
}

func (s *GormPoolDataSourceStore) DeleteByID(ctx context.Context, id uint) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	result := q.Delete(&models.PoolDataSource{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("pool data source not found")
	}
	return nil
}

func (s *GormPoolDataSourceStore) SetCurrent(ctx context.Context, chain string, timeframeMinutes int, id uint) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	tx := db
	if ctx != nil {
		tx = tx.WithContext(ctx)
	}
	err = tx.Transaction(func(tx *gorm.DB) error {
		var source models.PoolDataSource
		if err := tx.First(&source, id).Error; err != nil {
			return err
		}
		if source.Chain != chain || source.TimeframeMinutes != timeframeMinutes {
			return fmt.Errorf("pool data source mismatch: id=%d chain=%s timeframe=%d", id, chain, timeframeMinutes)
		}
		if !source.IsEnabled {
			return fmt.Errorf("pool data source is disabled")
		}
		if err := tx.Model(&models.PoolDataSource{}).
			Where("chain = ? AND timeframe_minutes = ?", chain, timeframeMinutes).
			Update("is_current", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.PoolDataSource{}).
			Where("id = ?", id).
			Update("is_current", true).Error; err != nil {
			return err
		}
		return tx.
			Where("chain = ? AND (pool_data_source_id IS NULL OR pool_data_source_id <> ?)", source.Chain, source.ID).
			Delete(&models.Pool{}).Error
	})
	if err != nil {
		return err
	}
	invalidatePoolCatalogCache(ctx, []string{chain})
	return nil
}

func (s *GormPoolDataSourceStore) UnsetCurrent(ctx context.Context, chain string, timeframeMinutes int) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	return q.Model(&models.PoolDataSource{}).
		Where("chain = ? AND timeframe_minutes = ?", chain, timeframeMinutes).
		Update("is_current", false).Error
}

type PoolDataSourceInput struct {
	Name             string
	NameSet          bool
	SourceType       string
	Chain            string
	TimeframeMinutes int
	Limit            int
	BaseURL          string
	PathTemplate     string
	PathTemplateSet  bool
	QueryTemplate    map[string]string
	Protocols        []string
	Dexes            []string
	SetCurrent       bool
}

type PoolDataSourceConfig struct {
	ID               *uint
	Name             string
	SourceType       string
	Chain            string
	TimeframeMinutes int
	Limit            int
	BaseURL          string
	PathTemplate     string
	QueryTemplate    map[string]string
	Protocols        []string
	Dexes            []string
	IsEnabled        bool
	IsCurrent        bool
	IsEnvFallback    bool
}

type PoolDataSourceManager struct {
	store PoolDataSourceStore
	now   func() time.Time
}

func NewPoolDataSourceManager(store PoolDataSourceStore) *PoolDataSourceManager {
	return &PoolDataSourceManager{store: store, now: time.Now}
}

var (
	defaultPoolDataSourceOnce    sync.Once
	defaultPoolDataSourceManager *PoolDataSourceManager
)

func DefaultPoolDataSourceManager() *PoolDataSourceManager {
	defaultPoolDataSourceOnce.Do(func() {
		defaultPoolDataSourceManager = NewPoolDataSourceManager(NewGormPoolDataSourceStore())
	})
	return defaultPoolDataSourceManager
}

func (m *PoolDataSourceManager) ListAll(ctx context.Context) ([]models.PoolDataSource, error) {
	if m == nil || m.store == nil {
		return nil, fmt.Errorf("pool data source store not available")
	}
	return m.store.ListAll(ctx)
}

func (m *PoolDataSourceManager) GetSource(ctx context.Context, id uint) (*models.PoolDataSource, error) {
	if m == nil || m.store == nil {
		return nil, fmt.Errorf("pool data source store not available")
	}
	return m.store.GetByID(ctx, id)
}

func (m *PoolDataSourceManager) CandidateSources(ctx context.Context, chain string, timeframeMinutes int) []PoolDataSourceConfig {
	chain = normalizePoolDataSourceChain(chain)
	if timeframeMinutes <= 0 {
		timeframeMinutes = 5
	}

	env := envPoolDataSource(chain, timeframeMinutes)
	if m == nil || m.store == nil {
		return []PoolDataSourceConfig{env}
	}

	list, err := m.store.List(ctx, chain, timeframeMinutes)
	if err != nil || len(list) == 0 {
		return []PoolDataSourceConfig{env}
	}

	out := make([]PoolDataSourceConfig, 0, len(list)+1)
	for i := range list {
		source := list[i]
		if !source.IsEnabled || !source.IsCurrent {
			continue
		}
		out = append(out, configFromPoolDataSource(source))
	}
	for i := range list {
		source := list[i]
		if !source.IsEnabled || source.IsCurrent {
			continue
		}
		out = append(out, configFromPoolDataSource(source))
	}

	if len(out) == 0 {
		_ = m.store.UnsetCurrent(ctx, chain, timeframeMinutes)
		return []PoolDataSourceConfig{env}
	}
	if !hasCurrentPoolDataSource(out) && out[0].ID != nil {
		_ = m.store.SetCurrent(ctx, chain, timeframeMinutes, *out[0].ID)
		out[0].IsCurrent = true
	}

	if !containsEquivalentPoolDataSource(out, env) {
		out = append(out, env)
	}
	return out
}

func (m *PoolDataSourceManager) AddSource(ctx context.Context, input PoolDataSourceInput) (*models.PoolDataSource, error) {
	if m == nil || m.store == nil {
		return nil, fmt.Errorf("pool data source store not available")
	}
	source, err := normalizePoolDataSourceInput(input, nil)
	if err != nil {
		return nil, err
	}
	if err := m.store.Create(ctx, source); err != nil {
		return nil, err
	}
	if input.SetCurrent {
		if err := m.SwitchCurrent(ctx, source.ID); err != nil {
			return source, err
		}
		source.IsCurrent = true
	}
	return source, nil
}

func (m *PoolDataSourceManager) UpdateSource(ctx context.Context, id uint, input PoolDataSourceInput) (*models.PoolDataSource, error) {
	if m == nil || m.store == nil {
		return nil, fmt.Errorf("pool data source store not available")
	}
	existing, err := m.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("pool data source not found")
	}
	source, err := normalizePoolDataSourceInput(input, existing)
	if err != nil {
		return nil, err
	}
	updates := map[string]interface{}{
		"Name":              source.Name,
		"SourceType":        source.SourceType,
		"Chain":             source.Chain,
		"TimeframeMinutes":  source.TimeframeMinutes,
		"Limit":             source.Limit,
		"BaseURL":           source.BaseURL,
		"PathTemplate":      source.PathTemplate,
		"QueryTemplateJSON": source.QueryTemplateJSON,
		"ProtocolsJSON":     source.ProtocolsJSON,
		"DexesJSON":         source.DexesJSON,
	}
	if err := m.store.UpdateByID(ctx, id, updates); err != nil {
		return nil, err
	}
	if input.SetCurrent {
		if err := m.SwitchCurrent(ctx, id); err != nil {
			return nil, err
		}
	} else if source.IsCurrent {
		if err := m.store.SetCurrent(ctx, source.Chain, source.TimeframeMinutes, id); err != nil {
			return nil, err
		}
	}
	return m.store.GetByID(ctx, id)
}

func (m *PoolDataSourceManager) SwitchCurrent(ctx context.Context, id uint) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("pool data source store not available")
	}
	source, err := m.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if source == nil {
		return fmt.Errorf("pool data source not found")
	}
	if !source.IsEnabled {
		return fmt.Errorf("pool data source is disabled")
	}
	return m.store.SetCurrent(ctx, source.Chain, source.TimeframeMinutes, source.ID)
}

func (m *PoolDataSourceManager) EnableSource(ctx context.Context, id uint) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("pool data source store not available")
	}
	return m.store.UpdateByID(ctx, id, map[string]interface{}{
		"IsEnabled": true,
		"LastError": "",
	})
}

func (m *PoolDataSourceManager) DisableSource(ctx context.Context, id uint) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("pool data source store not available")
	}
	source, err := m.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if source == nil {
		return fmt.Errorf("pool data source not found")
	}
	if err := m.store.UpdateByID(ctx, id, map[string]interface{}{
		"IsEnabled": false,
		"IsCurrent": false,
	}); err != nil {
		return err
	}
	if source.IsCurrent {
		_ = m.store.UnsetCurrent(ctx, source.Chain, source.TimeframeMinutes)
	}
	return nil
}

func (m *PoolDataSourceManager) DeleteSource(ctx context.Context, id uint) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("pool data source store not available")
	}
	return m.store.DeleteByID(ctx, id)
}

func (m *PoolDataSourceManager) CheckSource(ctx context.Context, id uint) (PoolDataSourceFieldCoverage, error) {
	var coverage PoolDataSourceFieldCoverage
	if m == nil || m.store == nil {
		return coverage, fmt.Errorf("pool data source store not available")
	}
	source, err := m.store.GetByID(ctx, id)
	if err != nil {
		return coverage, err
	}
	if source == nil {
		return coverage, fmt.Errorf("pool data source not found")
	}
	cfg := configFromPoolDataSource(*source)
	start := time.Now()
	snapshot, err := fetchSnapshotFromPoolDataSource(ctx, cfg, cfg.Chain, poolSyncConfiguredDexes())
	latency := time.Since(start)
	if err != nil {
		m.RecordFailure(ctx, cfg, latency, err)
		return coverage, err
	}
	coverage = poolDataSourceFieldCoverage(snapshot)
	m.RecordSuccess(ctx, cfg, latency, coverage)
	return coverage, nil
}

func (m *PoolDataSourceManager) RecordSuccess(ctx context.Context, source PoolDataSourceConfig, latency time.Duration, coverage any) {
	if m == nil || m.store == nil || source.ID == nil {
		return
	}
	now := m.now()
	coverageJSON := "{}"
	if b, err := json.Marshal(coverage); err == nil && len(b) > 0 {
		coverageJSON = string(b)
	}
	_ = m.store.UpdateByID(ctx, *source.ID, map[string]interface{}{
		"LastCheckedAt":         &now,
		"LastSuccessAt":         &now,
		"LastLatencyMs":         latency.Milliseconds(),
		"LastError":             "",
		"LastFieldCoverageJSON": coverageJSON,
	})
}

func (m *PoolDataSourceManager) RecordFailure(ctx context.Context, source PoolDataSourceConfig, latency time.Duration, err error) {
	if m == nil || m.store == nil || source.ID == nil {
		return
	}
	now := m.now()
	_ = m.store.UpdateByID(ctx, *source.ID, map[string]interface{}{
		"LastCheckedAt": &now,
		"LastLatencyMs": latency.Milliseconds(),
		"LastError":     truncatePoolDataSourceError(err, 512),
	})
}

func configFromPoolDataSource(source models.PoolDataSource) PoolDataSourceConfig {
	id := source.ID
	return PoolDataSourceConfig{
		ID:               &id,
		Name:             strings.TrimSpace(source.Name),
		SourceType:       NormalizePoolDataSourceType(source.SourceType),
		Chain:            normalizePoolDataSourceChain(source.Chain),
		TimeframeMinutes: positiveOrDefault(source.TimeframeMinutes, 5),
		Limit:            positiveOrDefault(source.Limit, 100),
		BaseURL:          strings.TrimSpace(source.BaseURL),
		PathTemplate:     strings.TrimSpace(source.PathTemplate),
		QueryTemplate:    parseStringMapJSON(source.QueryTemplateJSON),
		Protocols:        parseStringSliceJSON(source.ProtocolsJSON),
		Dexes:            parseStringSliceJSON(source.DexesJSON),
		IsEnabled:        source.IsEnabled,
		IsCurrent:        source.IsCurrent,
	}
}

func envPoolDataSource(chain string, timeframeMinutes int) PoolDataSourceConfig {
	baseURL := ""
	if config.AppConfig != nil {
		baseURL = strings.TrimSpace(config.AppConfig.PoolsSyncPoolMBaseURL)
	}
	if baseURL == "" {
		baseURL = defaultPoolMBaseURL
	}
	return PoolDataSourceConfig{
		Name:             "PoolM (.env)",
		SourceType:       PoolDataSourceTypePoolMTopFees,
		Chain:            normalizePoolDataSourceChain(chain),
		TimeframeMinutes: positiveOrDefault(timeframeMinutes, 5),
		Limit:            100,
		BaseURL:          strings.TrimRight(baseURL, "/"),
		IsEnabled:        true,
		IsCurrent:        false,
		IsEnvFallback:    true,
	}
}

func normalizePoolDataSourceInput(input PoolDataSourceInput, existing *models.PoolDataSource) (*models.PoolDataSource, error) {
	source := &models.PoolDataSource{}
	if existing != nil {
		*source = *existing
	}

	if input.NameSet || strings.TrimSpace(input.Name) != "" || existing == nil {
		source.Name = strings.TrimSpace(input.Name)
	}
	if strings.TrimSpace(input.SourceType) != "" || existing == nil {
		source.SourceType = NormalizePoolDataSourceType(input.SourceType)
	}
	if strings.TrimSpace(input.Chain) != "" || existing == nil {
		source.Chain = normalizePoolDataSourceChain(input.Chain)
	}
	if input.TimeframeMinutes > 0 || existing == nil {
		source.TimeframeMinutes = positiveOrDefault(input.TimeframeMinutes, 5)
	}
	if input.Limit > 0 || existing == nil {
		source.Limit = positiveOrDefault(input.Limit, 100)
	}
	if strings.TrimSpace(input.BaseURL) != "" || existing == nil {
		source.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	}
	if input.PathTemplateSet || input.PathTemplate != "" || existing == nil {
		source.PathTemplate = strings.TrimSpace(input.PathTemplate)
	}
	if input.QueryTemplate != nil || existing == nil {
		source.QueryTemplateJSON = marshalStringMapJSON(input.QueryTemplate)
	}
	if input.Protocols != nil || existing == nil {
		source.ProtocolsJSON = marshalStringSliceJSON(input.Protocols)
	}
	if input.Dexes != nil || existing == nil {
		source.DexesJSON = marshalStringSliceJSON(input.Dexes)
	}

	if source.Name == "" {
		source.Name = derivePoolDataSourceName(source.BaseURL)
	}
	if source.Name == "" {
		source.Name = source.SourceType
	}
	if len(source.Name) > 80 {
		return nil, fmt.Errorf("pool data source name too long (max 80 chars)")
	}
	if err := validatePoolDataSourceType(source.SourceType); err != nil {
		return nil, err
	}
	if source.Chain == "" {
		source.Chain = "bsc"
	}
	if source.TimeframeMinutes <= 0 {
		source.TimeframeMinutes = 5
	}
	if source.Limit <= 0 {
		source.Limit = 100
	}
	if err := validatePoolDataSourceURL(source.BaseURL); err != nil {
		return nil, err
	}
	source.IsEnabled = true
	if existing != nil {
		source.IsEnabled = existing.IsEnabled
		source.IsCurrent = existing.IsCurrent
	}
	if source.QueryTemplateJSON == "" {
		source.QueryTemplateJSON = "{}"
	}
	if source.ProtocolsJSON == "" {
		source.ProtocolsJSON = "[]"
	}
	if source.DexesJSON == "" {
		source.DexesJSON = "[]"
	}
	if source.LastFieldCoverageJSON == "" {
		source.LastFieldCoverageJSON = "{}"
	}
	return source, nil
}

func NormalizePoolDataSourceType(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "", "poolm", "poolm_top_fees", "top_fees":
		return PoolDataSourceTypePoolMTopFees
	case "market", "market_pools", "market/pools":
		return PoolDataSourceTypeMarketPools
	default:
		return v
	}
}

func validatePoolDataSourceType(sourceType string) error {
	switch NormalizePoolDataSourceType(sourceType) {
	case PoolDataSourceTypePoolMTopFees, PoolDataSourceTypeMarketPools:
		return nil
	default:
		return fmt.Errorf("unsupported pool data source type=%s", sourceType)
	}
}

func validatePoolDataSourceURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("pool data source base_url is empty")
	}
	if len(raw) > 512 {
		return fmt.Errorf("pool data source base_url too long (max 512 chars)")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid pool data source base_url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("pool data source base_url scheme must be http/https")
	}
	if u.Host == "" {
		return fmt.Errorf("invalid pool data source base_url")
	}
	return nil
}

func normalizePoolDataSourceChain(chain string) string {
	chain = config.NormalizeChain(chain)
	if chain == "" {
		return "bsc"
	}
	return chain
}

func derivePoolDataSourceName(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || strings.TrimSpace(u.Host) == "" {
		return ""
	}
	host := strings.TrimSpace(u.Host)
	if len(host) > 80 {
		host = host[:80]
	}
	return host
}

func parseStringSliceJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return normalizeStringList(out)
}

func parseStringMapJSON(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	for k, v := range out {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" {
			delete(out, k)
			continue
		}
		if key != k {
			delete(out, k)
		}
		out[key] = val
	}
	return out
}

func marshalStringSliceJSON(values []string) string {
	out := normalizeStringList(values)
	if out == nil {
		out = []string{}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func marshalStringMapJSON(values map[string]string) string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(v)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func hasCurrentPoolDataSource(values []PoolDataSourceConfig) bool {
	for _, value := range values {
		if value.IsCurrent {
			return true
		}
	}
	return false
}

func containsEquivalentPoolDataSource(values []PoolDataSourceConfig, target PoolDataSourceConfig) bool {
	targetType := NormalizePoolDataSourceType(target.SourceType)
	targetURL := strings.TrimRight(strings.TrimSpace(target.BaseURL), "/")
	for _, value := range values {
		if NormalizePoolDataSourceType(value.SourceType) == targetType &&
			strings.TrimRight(strings.TrimSpace(value.BaseURL), "/") == targetURL {
			return true
		}
	}
	return false
}

func positiveOrDefault(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func truncatePoolDataSourceError(err error, max int) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if len(msg) <= max || max <= 0 {
		return msg
	}
	if max <= 3 {
		return msg[:max]
	}
	return msg[:max-3] + "..."
}
