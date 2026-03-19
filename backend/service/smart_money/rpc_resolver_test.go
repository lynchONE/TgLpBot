package smart_money

import (
	"TgLpBot/base/models"
	"TgLpBot/base/rpcpool"
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"
)

type smartMoneyMemStore struct {
	mu  sync.Mutex
	eps []models.RpcEndpoint
}

func (s *smartMoneyMemStore) ListAll(ctx context.Context) ([]models.RpcEndpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.RpcEndpoint, len(s.eps))
	copy(out, s.eps)
	return out, nil
}

func (s *smartMoneyMemStore) List(ctx context.Context, chain string, transport string) ([]models.RpcEndpoint, error) {
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

func (s *smartMoneyMemStore) GetByID(ctx context.Context, id uint) (*models.RpcEndpoint, error) {
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

func (s *smartMoneyMemStore) Create(ctx context.Context, ep *models.RpcEndpoint) error {
	if ep == nil {
		return errors.New("nil endpoint")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eps = append(s.eps, *ep)
	return nil
}

func (s *smartMoneyMemStore) UpdateByID(ctx context.Context, id uint, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.eps {
		if s.eps[i].ID != id {
			continue
		}
		for key, raw := range updates {
			switch key {
			case "DisabledUntil":
				if raw == nil {
					s.eps[i].DisabledUntil = nil
				} else if v, ok := raw.(*time.Time); ok {
					s.eps[i].DisabledUntil = v
				}
			case "DisabledReason":
				if v, ok := raw.(string); ok {
					s.eps[i].DisabledReason = v
				}
			case "IsCurrent":
				if v, ok := raw.(bool); ok {
					s.eps[i].IsCurrent = v
				}
			}
		}
		return nil
	}
	return nil
}

func (s *smartMoneyMemStore) DeleteByID(ctx context.Context, id uint) error {
	return nil
}

func (s *smartMoneyMemStore) SetCurrent(ctx context.Context, chain string, transport string, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.eps {
		if s.eps[i].Chain == chain && s.eps[i].Transport == transport {
			s.eps[i].IsCurrent = s.eps[i].ID == id
		}
	}
	return nil
}

func (s *smartMoneyMemStore) UnsetCurrent(ctx context.Context, chain string, transport string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.eps {
		if s.eps[i].Chain == chain && s.eps[i].Transport == transport {
			s.eps[i].IsCurrent = false
		}
	}
	return nil
}

func TestHandleSmartMoneyRPCEndpointError_IgnoresTemporary429(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{
			name: "per-second throttle",
			err:  errors.New(`429 Too Many Requests: {"error":{"code":-32003,"message":"cu limit exceeded; request too fast per second"},"id":14,"jsonrpc":"2.0"}`),
		},
		{
			name: "credit plan throttle",
			err:  errors.New(`429 Too Many Requests: {"error":{"code":-32005,"message":"user credit plan check; credit request service not available; msg: credit consumption has reached the maximum of the package"},"id":1,"jsonrpc":"2.0"}`),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &smartMoneyMemStore{
				eps: []models.RpcEndpoint{
					{ID: 1, Chain: "bsc", Transport: "http", URL: "https://rpc-a.example", IsCurrent: true},
				},
			}
			mgr := rpcpool.NewManager(store, func(chain string, transport string) string { return "" }, nil)
			eff := rpcpool.Effective{
				Source: rpcpool.SourceDB,
				URL:    "https://rpc-a.example",
				Endpoint: &models.RpcEndpoint{
					ID:        1,
					Chain:     "bsc",
					Transport: "http",
					URL:       "https://rpc-a.example",
					IsCurrent: true,
				},
			}

			handleSmartMoneyRPCEndpointErrorWithManager(mgr, eff, tc.err)

			ep, err := store.GetByID(context.Background(), 1)
			if err != nil {
				t.Fatalf("get endpoint: %v", err)
			}
			if ep == nil {
				t.Fatal("expected endpoint to exist")
			}
			if ep.DisabledUntil != nil {
				t.Fatalf("expected temporary 429 to keep endpoint enabled, got disabled_until=%v", ep.DisabledUntil)
			}
			if ep.DisabledReason != "" {
				t.Fatalf("expected disabled_reason to stay empty, got %q", ep.DisabledReason)
			}
			if !ep.IsCurrent {
				t.Fatal("expected endpoint to remain current")
			}
		})
	}
}

func TestHandleSmartMoneyRPCEndpointError_DisablesNonThrottleError(t *testing.T) {
	t.Parallel()

	store := &smartMoneyMemStore{
		eps: []models.RpcEndpoint{
			{ID: 1, Chain: "bsc", Transport: "http", URL: "https://rpc-a.example", IsCurrent: true},
		},
	}
	mgr := rpcpool.NewManager(store, func(chain string, transport string) string { return "" }, nil)
	eff := rpcpool.Effective{
		Source: rpcpool.SourceDB,
		URL:    "https://rpc-a.example",
		Endpoint: &models.RpcEndpoint{
			ID:        1,
			Chain:     "bsc",
			Transport: "http",
			URL:       "https://rpc-a.example",
			IsCurrent: true,
		},
	}

	handleSmartMoneyRPCEndpointErrorWithManager(mgr, eff, errors.New("dial tcp: i/o timeout"))

	ep, err := store.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("get endpoint: %v", err)
	}
	if ep == nil {
		t.Fatal("expected endpoint to exist")
	}
	if ep.DisabledUntil == nil {
		t.Fatal("expected non-throttle error to disable endpoint")
	}
	if ep.DisabledReason != rpcpool.ReasonHealthFail {
		t.Fatalf("expected disabled_reason=%q, got %q", rpcpool.ReasonHealthFail, ep.DisabledReason)
	}
	if ep.IsCurrent {
		t.Fatal("expected disabled endpoint to stop being current")
	}
}
