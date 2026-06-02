package okxpool

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/timeutil"
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

type memStore struct {
	mu   sync.Mutex
	rows []models.OKXAPIConfig
}

func (s *memStore) ListAll(ctx context.Context) ([]models.OKXAPIConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.OKXAPIConfig, len(s.rows))
	copy(out, s.rows)
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsCurrent != out[j].IsCurrent {
			return out[i].IsCurrent
		}
		if out[i].IsEnabled != out[j].IsEnabled {
			return out[i].IsEnabled
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *memStore) ListEnabled(ctx context.Context) ([]models.OKXAPIConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []models.OKXAPIConfig
	for _, row := range s.rows {
		if row.IsEnabled {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsCurrent != out[j].IsCurrent {
			return out[i].IsCurrent
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *memStore) GetByID(ctx context.Context, id uint) (*models.OKXAPIConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.rows {
		if row.ID == id {
			cp := row
			return &cp, nil
		}
	}
	return nil, nil
}

func (s *memStore) Create(ctx context.Context, row *models.OKXAPIConfig) error {
	if row == nil {
		return errors.New("nil okx config")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var maxID uint
	for _, existing := range s.rows {
		if existing.ID > maxID {
			maxID = existing.ID
		}
	}
	if row.ID == 0 {
		row.ID = maxID + 1
	}
	s.rows = append(s.rows, *row)
	return nil
}

func (s *memStore) UpdateByID(ctx context.Context, id uint, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.rows {
		if s.rows[i].ID != id {
			continue
		}
		v := reflect.ValueOf(&s.rows[i]).Elem()
		for k, raw := range updates {
			f := v.FieldByName(k)
			if !f.IsValid() || !f.CanSet() {
				continue
			}
			if raw == nil {
				f.Set(reflect.Zero(f.Type()))
				continue
			}
			rv := reflect.ValueOf(raw)
			if rv.Type().AssignableTo(f.Type()) {
				f.Set(rv)
				continue
			}
			if rv.Type().ConvertibleTo(f.Type()) {
				f.Set(rv.Convert(f.Type()))
			}
		}
		return nil
	}
	return nil
}

func (s *memStore) DeleteByID(ctx context.Context, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.rows {
		if s.rows[i].ID == id {
			s.rows = append(s.rows[:i], s.rows[i+1:]...)
			return nil
		}
	}
	return errors.New("okx api config not found")
}

func (s *memStore) SetCurrent(ctx context.Context, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.rows {
		s.rows[i].IsCurrent = s.rows[i].ID == id
	}
	return nil
}

func (s *memStore) UnsetCurrent(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.rows {
		s.rows[i].IsCurrent = false
	}
	return nil
}

type fakeProber struct {
	latency time.Duration
	errByID map[uint]error
}

func (p *fakeProber) Probe(ctx context.Context, cfg EffectiveConfig) (time.Duration, error) {
	if p == nil {
		return 0, nil
	}
	if cfg.Config != nil && p.errByID != nil {
		if err := p.errByID[cfg.Config.ID]; err != nil {
			return p.latency, err
		}
	}
	return p.latency, nil
}

func withTestConfig(t *testing.T) {
	t.Helper()
	old := config.AppConfig
	config.AppConfig = &config.Config{
		EncryptionKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		OKXDexAPIURL:  "https://env.example/api/v6/dex/aggregator",
		OKXAPIKey:     "env-key",
		OKXSecretKey:  "env-secret",
		OKXPassphrase: "env-pass",
	}
	t.Cleanup(func() { config.AppConfig = old })
}

func addConfigForTest(t *testing.T, m *Manager, input Input) *models.OKXAPIConfig {
	t.Helper()
	row, err := m.AddConfig(context.Background(), input)
	if err != nil {
		t.Fatalf("AddConfig failed: %v", err)
	}
	return row
}

func TestManagerEffectiveUsesCurrentDBConfig(t *testing.T) {
	withTestConfig(t)
	store := &memStore{}
	m := NewManager(store, &fakeProber{})
	row := addConfigForTest(t, m, Input{
		Name:       "primary",
		BaseURL:    "https://okx-a.example/api/v6/dex/aggregator",
		APIKey:     "key-a",
		SecretKey:  "secret-a",
		Passphrase: "pass-a",
		SetCurrent: true,
	})

	eff, err := m.Effective(context.Background())
	if err != nil {
		t.Fatalf("Effective failed: %v", err)
	}
	if eff.Source != SourceDB {
		t.Fatalf("expected source=%s, got %s", SourceDB, eff.Source)
	}
	if eff.Config == nil || eff.Config.ID != row.ID {
		t.Fatalf("expected config id=%d, got %+v", row.ID, eff.Config)
	}
	if eff.APIKey != "key-a" || eff.SecretKey != "secret-a" || eff.Passphrase != "pass-a" {
		t.Fatalf("unexpected effective credentials: %+v", eff)
	}
}

func TestManagerEffectiveFailsOverWhenCurrentUnavailable(t *testing.T) {
	withTestConfig(t)
	store := &memStore{}
	m := NewManager(store, &fakeProber{})
	first := addConfigForTest(t, m, Input{
		BaseURL:    "https://okx-a.example/api/v6/dex/aggregator",
		APIKey:     "key-a",
		SecretKey:  "secret-a",
		Passphrase: "pass-a",
		SetCurrent: true,
	})
	second := addConfigForTest(t, m, Input{
		BaseURL:    "https://okx-b.example/api/v6/dex/aggregator",
		APIKey:     "key-b",
		SecretKey:  "secret-b",
		Passphrase: "pass-b",
	})
	until := time.Now().Add(time.Hour)
	if err := store.UpdateByID(context.Background(), first.ID, map[string]interface{}{"DisabledUntil": &until}); err != nil {
		t.Fatalf("UpdateByID failed: %v", err)
	}

	eff, err := m.Effective(context.Background())
	if err != nil {
		t.Fatalf("Effective failed: %v", err)
	}
	if eff.Config == nil || eff.Config.ID != second.ID {
		t.Fatalf("expected failover to config id=%d, got %+v", second.ID, eff.Config)
	}
	row, _ := store.GetByID(context.Background(), second.ID)
	if row == nil || !row.IsCurrent {
		t.Fatalf("expected second config to become current")
	}
}

func TestManagerEffectiveFallsBackToEnvWhenNoDBAvailable(t *testing.T) {
	withTestConfig(t)
	until := time.Now().Add(time.Hour)
	store := &memStore{}
	m := NewManager(store, &fakeProber{})
	addConfigForTest(t, m, Input{
		BaseURL:    "https://okx-a.example/api/v6/dex/aggregator",
		APIKey:     "key-a",
		SecretKey:  "secret-a",
		Passphrase: "pass-a",
		SetCurrent: true,
	})
	if err := store.UpdateByID(context.Background(), 1, map[string]interface{}{"DisabledUntil": &until}); err != nil {
		t.Fatalf("UpdateByID failed: %v", err)
	}

	eff, err := m.Effective(context.Background())
	if err != nil {
		t.Fatalf("Effective failed: %v", err)
	}
	if eff.Source != SourceEnv || eff.BaseURL != "https://env.example/api/v6/dex/aggregator" {
		t.Fatalf("expected env fallback, got %+v", eff)
	}
}

func TestManagerRecordFailureDisablesQuotaUntilNextMonth(t *testing.T) {
	withTestConfig(t)
	timeutil.Init()
	loc := timeutil.Location()
	fixedNow := time.Date(2026, time.January, 15, 12, 0, 0, 0, loc)
	store := &memStore{}
	m := NewManager(store, &fakeProber{}).WithNow(func() time.Time { return fixedNow })
	row := addConfigForTest(t, m, Input{
		BaseURL:    "https://okx-a.example/api/v6/dex/aggregator",
		APIKey:     "key-a",
		SecretKey:  "secret-a",
		Passphrase: "pass-a",
		SetCurrent: true,
	})

	eff, err := m.Effective(context.Background())
	if err != nil {
		t.Fatalf("Effective failed: %v", err)
	}
	m.RecordFailure(context.Background(), eff, 50*time.Millisecond, errors.New("monthly quota exceeded"))

	got, _ := store.GetByID(context.Background(), row.ID)
	if got == nil {
		t.Fatalf("expected config to exist")
	}
	if got.DisabledReason != ReasonQuotaExhausted {
		t.Fatalf("expected disabled_reason=%q, got %q", ReasonQuotaExhausted, got.DisabledReason)
	}
	if got.DisabledUntil == nil {
		t.Fatalf("expected disabled_until")
	}
	du := got.DisabledUntil.In(loc)
	if du.Year() != 2026 || du.Month() != time.February || du.Day() != 1 || du.Hour() != 0 {
		t.Fatalf("expected next month start, got %s", du.Format(time.RFC3339))
	}
	if got.IsCurrent {
		t.Fatalf("expected current flag cleared")
	}
}
