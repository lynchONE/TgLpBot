package pool_sync

import (
	"TgLpBot/base/models"
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"testing"
)

type memPoolDataSourceStore struct {
	mu      sync.Mutex
	sources []models.PoolDataSource
}

func (s *memPoolDataSourceStore) ListAll(ctx context.Context) ([]models.PoolDataSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.PoolDataSource, len(s.sources))
	copy(out, s.sources)
	sortPoolDataSources(out)
	return out, nil
}

func (s *memPoolDataSourceStore) List(ctx context.Context, chain string, timeframeMinutes int) ([]models.PoolDataSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.PoolDataSource, 0, len(s.sources))
	for _, source := range s.sources {
		if source.Chain == chain && source.TimeframeMinutes == timeframeMinutes {
			out = append(out, source)
		}
	}
	sortPoolDataSources(out)
	return out, nil
}

func (s *memPoolDataSourceStore) GetByID(ctx context.Context, id uint) (*models.PoolDataSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, source := range s.sources {
		if source.ID == id {
			cp := source
			return &cp, nil
		}
	}
	return nil, nil
}

func (s *memPoolDataSourceStore) Create(ctx context.Context, source *models.PoolDataSource) error {
	if source == nil {
		return errors.New("nil source")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var maxID uint
	for _, existing := range s.sources {
		if existing.ID > maxID {
			maxID = existing.ID
		}
	}
	if source.ID == 0 {
		source.ID = maxID + 1
	}
	s.sources = append(s.sources, *source)
	return nil
}

func (s *memPoolDataSourceStore) UpdateByID(ctx context.Context, id uint, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sources {
		if s.sources[i].ID != id {
			continue
		}
		v := reflect.ValueOf(&s.sources[i]).Elem()
		for key, raw := range updates {
			f := v.FieldByName(key)
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
			} else if rv.Type().ConvertibleTo(f.Type()) {
				f.Set(rv.Convert(f.Type()))
			}
		}
		return nil
	}
	return nil
}

func (s *memPoolDataSourceStore) DeleteByID(ctx context.Context, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sources {
		if s.sources[i].ID == id {
			s.sources = append(s.sources[:i], s.sources[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (s *memPoolDataSourceStore) SetCurrent(ctx context.Context, chain string, timeframeMinutes int, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sources {
		if s.sources[i].Chain == chain && s.sources[i].TimeframeMinutes == timeframeMinutes {
			s.sources[i].IsCurrent = s.sources[i].ID == id
		}
	}
	return nil
}

func (s *memPoolDataSourceStore) UnsetCurrent(ctx context.Context, chain string, timeframeMinutes int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sources {
		if s.sources[i].Chain == chain && s.sources[i].TimeframeMinutes == timeframeMinutes {
			s.sources[i].IsCurrent = false
		}
	}
	return nil
}

func sortPoolDataSources(values []models.PoolDataSource) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Chain != values[j].Chain {
			return values[i].Chain < values[j].Chain
		}
		if values[i].TimeframeMinutes != values[j].TimeframeMinutes {
			return values[i].TimeframeMinutes < values[j].TimeframeMinutes
		}
		if values[i].IsCurrent != values[j].IsCurrent {
			return values[i].IsCurrent
		}
		return values[i].ID < values[j].ID
	})
}

func TestPoolDataSourceManager_CandidateSourcesFallsBackToEnv(t *testing.T) {
	mgr := NewPoolDataSourceManager(&memPoolDataSourceStore{})
	candidates := mgr.CandidateSources(context.Background(), "bsc", 5)
	if len(candidates) != 1 {
		t.Fatalf("expected one env candidate, got %d", len(candidates))
	}
	if !candidates[0].IsEnvFallback {
		t.Fatalf("expected env fallback candidate")
	}
	if candidates[0].SourceType != PoolDataSourceTypePoolMTopFees {
		t.Fatalf("expected poolm source type, got %s", candidates[0].SourceType)
	}
}

func TestPoolDataSourceManager_CandidateSourcesPrefersCurrent(t *testing.T) {
	store := &memPoolDataSourceStore{sources: []models.PoolDataSource{
		{ID: 1, Name: "backup", SourceType: PoolDataSourceTypeMarketPools, Chain: "bsc", TimeframeMinutes: 5, BaseURL: "http://backup.example", IsEnabled: true},
		{ID: 2, Name: "current", SourceType: PoolDataSourceTypePoolMTopFees, Chain: "bsc", TimeframeMinutes: 5, BaseURL: "https://poolm.example", IsEnabled: true, IsCurrent: true},
	}}
	mgr := NewPoolDataSourceManager(store)
	candidates := mgr.CandidateSources(context.Background(), "bsc", 5)
	if len(candidates) < 2 {
		t.Fatalf("expected db candidates, got %d", len(candidates))
	}
	if candidates[0].ID == nil || *candidates[0].ID != 2 {
		t.Fatalf("expected current id=2 first, got %+v", candidates[0].ID)
	}
}

func TestPoolDataSourceManager_SwitchRejectsDisabled(t *testing.T) {
	store := &memPoolDataSourceStore{sources: []models.PoolDataSource{
		{ID: 1, Name: "disabled", SourceType: PoolDataSourceTypeMarketPools, Chain: "bsc", TimeframeMinutes: 5, BaseURL: "http://backup.example", IsEnabled: false},
	}}
	mgr := NewPoolDataSourceManager(store)
	if err := mgr.SwitchCurrent(context.Background(), 1); err == nil {
		t.Fatalf("expected disabled source switch to fail")
	}
}
