package rpcpool

import (
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
	mu  sync.Mutex
	eps []models.RpcEndpoint
}

func (s *memStore) ListAll(ctx context.Context) ([]models.RpcEndpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.RpcEndpoint, len(s.eps))
	copy(out, s.eps)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Chain != out[j].Chain {
			return out[i].Chain < out[j].Chain
		}
		if out[i].Transport != out[j].Transport {
			return out[i].Transport < out[j].Transport
		}
		if out[i].IsCurrent != out[j].IsCurrent {
			return out[i].IsCurrent
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *memStore) List(ctx context.Context, chain string, transport string) ([]models.RpcEndpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []models.RpcEndpoint
	for _, ep := range s.eps {
		if ep.Chain == chain && ep.Transport == transport {
			out = append(out, ep)
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

func (s *memStore) GetByID(ctx context.Context, id uint) (*models.RpcEndpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ep := range s.eps {
		if ep.ID == id {
			cp := ep
			return &cp, nil
		}
	}
	return nil, nil
}

func (s *memStore) Create(ctx context.Context, ep *models.RpcEndpoint) error {
	if ep == nil {
		return errors.New("nil endpoint")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var maxID uint
	for _, e := range s.eps {
		if e.ID > maxID {
			maxID = e.ID
		}
	}
	if ep.ID == 0 {
		ep.ID = maxID + 1
	}
	s.eps = append(s.eps, *ep)
	return nil
}

func (s *memStore) UpdateByID(ctx context.Context, id uint, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.eps {
		if s.eps[i].ID != id {
			continue
		}

		v := reflect.ValueOf(&s.eps[i]).Elem()
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
				continue
			}
		}
		return nil
	}
	return nil
}

func (s *memStore) SetCurrent(ctx context.Context, chain string, transport string, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.eps {
		if s.eps[i].Chain == chain && s.eps[i].Transport == transport {
			s.eps[i].IsCurrent = s.eps[i].ID == id
		}
	}
	return nil
}

func (s *memStore) DeleteByID(ctx context.Context, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.eps {
		if s.eps[i].ID == id {
			s.eps = append(s.eps[:i], s.eps[i+1:]...)
			return nil
		}
	}
	return errors.New("endpoint not found")
}

func (s *memStore) UnsetCurrent(ctx context.Context, chain string, transport string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.eps {
		if s.eps[i].Chain == chain && s.eps[i].Transport == transport {
			s.eps[i].IsCurrent = false
		}
	}
	return nil
}

type fakeProber struct {
	latency  time.Duration
	errByURL map[string]error
}

func (p *fakeProber) Probe(ctx context.Context, url string, transport string) (time.Duration, error) {
	if p == nil {
		return 0, nil
	}
	if p.errByURL == nil {
		return p.latency, nil
	}
	if err := p.errByURL[url]; err != nil {
		return p.latency, err
	}
	return p.latency, nil
}

func TestManager_EffectiveURL_UsesCurrentWhenAvailable(t *testing.T) {
	store := &memStore{
		eps: []models.RpcEndpoint{
			{ID: 1, Chain: "bsc", Transport: "http", URL: "https://rpc-a.example", IsCurrent: true},
			{ID: 2, Chain: "bsc", Transport: "http", URL: "https://rpc-b.example", IsCurrent: false},
		},
	}
	m := NewManager(store, func(chain string, transport string) string { return "https://env.example" }, &fakeProber{})

	eff, err := m.EffectiveURL(context.Background(), "bsc", "http")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if eff.Source != SourceDB {
		t.Fatalf("expected source=%s, got %s", SourceDB, eff.Source)
	}
	if eff.URL != "https://rpc-a.example" {
		t.Fatalf("expected url=%q, got %q", "https://rpc-a.example", eff.URL)
	}
	if eff.Endpoint == nil || eff.Endpoint.ID != 1 {
		t.Fatalf("expected endpoint id=1, got %+v", eff.Endpoint)
	}
}

func TestManager_EffectiveURL_FailsOverWhenCurrentUnavailable(t *testing.T) {
	disableUntil := time.Now().Add(24 * time.Hour)
	store := &memStore{
		eps: []models.RpcEndpoint{
			{ID: 1, Chain: "bsc", Transport: "http", URL: "https://rpc-a.example", IsCurrent: true, DisabledUntil: &disableUntil},
			{ID: 2, Chain: "bsc", Transport: "http", URL: "https://rpc-b.example", IsCurrent: false},
		},
	}
	m := NewManager(store, func(chain string, transport string) string { return "" }, &fakeProber{})

	eff, err := m.EffectiveURL(context.Background(), "bsc", "http")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if eff.URL != "https://rpc-b.example" {
		t.Fatalf("expected url=%q, got %q", "https://rpc-b.example", eff.URL)
	}

	list, _ := store.List(context.Background(), "bsc", "http")
	var cur uint
	for _, ep := range list {
		if ep.IsCurrent {
			cur = ep.ID
		}
	}
	if cur != 2 {
		t.Fatalf("expected current endpoint id=2 after failover, got %d", cur)
	}
}

func TestManager_CheckAllOnce_DisablesQuotaExhaustedUntilNextMonth(t *testing.T) {
	timeutil.Init()
	loc := timeutil.Location()
	fixedNow := time.Date(2026, 1, 15, 12, 0, 0, 0, loc)

	store := &memStore{
		eps: []models.RpcEndpoint{
			{ID: 1, Chain: "bsc", Transport: "http", URL: "https://rpc-a.example", IsCurrent: true},
		},
	}
	prober := &fakeProber{
		latency: 50 * time.Millisecond,
		errByURL: map[string]error{
			"https://rpc-a.example": errors.New("cu limit exceeded"),
		},
	}
	m := NewManager(store, func(chain string, transport string) string { return "" }, prober).WithNow(func() time.Time { return fixedNow })

	if err := m.CheckAllOnce(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	ep, _ := store.GetByID(context.Background(), 1)
	if ep == nil {
		t.Fatalf("expected endpoint to exist")
	}
	if ep.DisabledReason != ReasonQuotaExhausted {
		t.Fatalf("expected disabled_reason=%q, got %q", ReasonQuotaExhausted, ep.DisabledReason)
	}
	if ep.DisabledUntil == nil {
		t.Fatalf("expected disabled_until to be set")
	}
	if ep.IsCurrent {
		t.Fatalf("expected is_current=false after disable")
	}

	du := ep.DisabledUntil.In(loc)
	if du.Year() != 2026 || du.Month() != time.February || du.Day() != 1 || du.Hour() != 0 || du.Minute() != 0 {
		t.Fatalf("expected disabled_until to be next month start, got %s", du.Format(time.RFC3339))
	}
}
