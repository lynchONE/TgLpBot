package rpcpool

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	TransportHTTP = "http"
	TransportWS   = "ws"
)

const (
	ReasonQuotaExhausted = "quota_exhausted"
	ReasonHealthFail     = "health_fail"
	ReasonManual         = "manual"
)

type Source string

const (
	SourceDB  Source = "db"
	SourceEnv Source = "env"
)

type Effective struct {
	Source   Source              `json:"source"`
	URL      string              `json:"url"`
	Endpoint *models.RpcEndpoint `json:"endpoint,omitempty"`
}

type Store interface {
	ListAll(ctx context.Context) ([]models.RpcEndpoint, error)
	List(ctx context.Context, chain string, transport string) ([]models.RpcEndpoint, error)
	GetByID(ctx context.Context, id uint) (*models.RpcEndpoint, error)
	Create(ctx context.Context, ep *models.RpcEndpoint) error
	UpdateByID(ctx context.Context, id uint, updates map[string]interface{}) error
	DeleteByID(ctx context.Context, id uint) error
	SetCurrent(ctx context.Context, chain string, transport string, id uint) error
	UnsetCurrent(ctx context.Context, chain string, transport string) error
}

type EnvProvider func(chain string, transport string) string

type Prober interface {
	Probe(ctx context.Context, url string, transport string) (time.Duration, error)
}

type Manager struct {
	store Store
	env   EnvProvider
	now   func() time.Time

	prober Prober

	failureThreshold   int
	tempDisableFor     time.Duration
	maxLastErrorLength int
}

func NewManager(store Store, env EnvProvider, prober Prober) *Manager {
	return &Manager{
		store:              store,
		env:                env,
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
		defaultManager = NewManager(NewGormStore(), EnvFromConfig, &EthProber{
			DialTimeout: 10 * time.Second,
			CallTimeout: 8 * time.Second,
		})
	})
	return defaultManager
}

func EnvFromConfig(chain string, transport string) string {
	chain = config.NormalizeChain(chain)
	transport = NormalizeTransport(transport)
	if config.AppConfig == nil {
		return ""
	}
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok {
		return ""
	}
	switch transport {
	case TransportHTTP:
		return strings.TrimSpace(cc.RpcURL)
	case TransportWS:
		ws := strings.TrimSpace(cc.RpcWSURL)
		if ws != "" {
			return ws
		}
		// Backward-compatible: allow ws URLs in the HTTP env slot.
		httpURL := strings.TrimSpace(cc.RpcURL)
		if strings.HasPrefix(httpURL, "ws://") || strings.HasPrefix(httpURL, "wss://") {
			return httpURL
		}
		return ""
	default:
		return ""
	}
}

func NormalizeTransport(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "http", "https":
		return TransportHTTP
	case "ws", "wss", "websocket":
		return TransportWS
	default:
		return v
	}
}

func (m *Manager) EffectiveURL(ctx context.Context, chain string, transport string) (Effective, error) {
	if m == nil {
		return Effective{}, fmt.Errorf("rpcpool manager is nil")
	}
	chain = config.NormalizeChain(chain)
	transport = NormalizeTransport(transport)
	if err := validateChainTransport(chain, transport); err != nil {
		return Effective{}, err
	}

	now := m.now()
	envURL := ""
	if m.env != nil {
		envURL = strings.TrimSpace(m.env(chain, transport))
	}

	// If store is missing (DB not ready), degrade to env.
	if m.store == nil {
		return Effective{Source: SourceEnv, URL: envURL}, nil
	}

	list, err := m.store.List(ctx, chain, transport)
	if err != nil {
		return Effective{Source: SourceEnv, URL: envURL}, nil
	}
	if len(list) == 0 {
		return Effective{Source: SourceEnv, URL: envURL}, nil
	}

	var availableCurrent *models.RpcEndpoint
	for i := range list {
		ep := list[i]
		if !ep.IsCurrent {
			continue
		}
		if isAvailable(ep, now) {
			availableCurrent = &ep
			break
		}
	}
	if availableCurrent != nil {
		return Effective{Source: SourceDB, URL: strings.TrimSpace(availableCurrent.URL), Endpoint: availableCurrent}, nil
	}

	var firstAvailable *models.RpcEndpoint
	for i := range list {
		ep := list[i]
		if isAvailable(ep, now) {
			firstAvailable = &ep
			break
		}
	}

	if firstAvailable != nil {
		_ = m.store.SetCurrent(ctx, chain, transport, firstAvailable.ID)
		return Effective{Source: SourceDB, URL: strings.TrimSpace(firstAvailable.URL), Endpoint: firstAvailable}, nil
	}

	// No available DB endpoints: make sure none is marked current to reflect reality.
	_ = m.store.UnsetCurrent(ctx, chain, transport)
	return Effective{Source: SourceEnv, URL: envURL}, nil
}

func (m *Manager) AddEndpoint(ctx context.Context, chain string, transport string, name string, url string, setCurrent bool) (*models.RpcEndpoint, error) {
	if m == nil || m.store == nil {
		return nil, fmt.Errorf("rpcpool store not available")
	}
	chain = config.NormalizeChain(chain)
	transport = NormalizeTransport(transport)
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)
	if err := validateChainTransport(chain, transport); err != nil {
		return nil, err
	}
	if err := validateURLForTransport(url, transport); err != nil {
		return nil, err
	}
	name, err := normalizeEndpointName(name, url)
	if err != nil {
		return nil, err
	}

	ep := &models.RpcEndpoint{
		Chain:     chain,
		Transport: transport,
		Name:      name,
		URL:       url,
		IsCurrent: false,
	}
	if err := m.store.Create(ctx, ep); err != nil {
		return nil, err
	}
	if setCurrent {
		if err := m.SwitchCurrent(ctx, ep.ID); err != nil {
			return ep, err
		}
	}
	return ep, nil
}

func (m *Manager) SwitchCurrent(ctx context.Context, endpointID uint) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("rpcpool store not available")
	}
	ep, err := m.store.GetByID(ctx, endpointID)
	if err != nil {
		return err
	}
	if ep == nil {
		return fmt.Errorf("rpc endpoint not found")
	}
	now := m.now()
	if !isAvailable(*ep, now) {
		return fmt.Errorf("rpc endpoint is unavailable")
	}
	return m.store.SetCurrent(ctx, ep.Chain, ep.Transport, endpointID)
}

func (m *Manager) RenameEndpoint(ctx context.Context, endpointID uint, name string) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("rpcpool store not available")
	}
	ep, err := m.store.GetByID(ctx, endpointID)
	if err != nil {
		return err
	}
	if ep == nil {
		return fmt.Errorf("rpc endpoint not found")
	}
	name, err = normalizeEndpointName(name, ep.URL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("rpc name is empty")
	}
	return m.store.UpdateByID(ctx, endpointID, map[string]interface{}{"Name": name})
}

func (m *Manager) DisableEndpoint(ctx context.Context, endpointID uint, until time.Time, reason string) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("rpcpool store not available")
	}
	ep, err := m.store.GetByID(ctx, endpointID)
	if err != nil {
		return err
	}
	if ep == nil {
		return fmt.Errorf("rpc endpoint not found")
	}
	until = until.In(until.Location())
	updates := map[string]interface{}{
		"DisabledUntil":  &until,
		"DisabledReason": strings.TrimSpace(reason),
		"IsCurrent":      false,
	}
	if err := m.store.UpdateByID(ctx, endpointID, updates); err != nil {
		return err
	}
	_, _ = m.EffectiveURL(ctx, ep.Chain, ep.Transport)
	return nil
}

func (m *Manager) DisableUntilNextMonth(ctx context.Context, endpointID uint) error {
	now := m.now()
	until := nextMonthStart(now)
	return m.DisableEndpoint(ctx, endpointID, until, ReasonQuotaExhausted)
}

func (m *Manager) EnableEndpoint(ctx context.Context, endpointID uint) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("rpcpool store not available")
	}
	ep, err := m.store.GetByID(ctx, endpointID)
	if err != nil {
		return err
	}
	if ep == nil {
		return fmt.Errorf("rpc endpoint not found")
	}
	updates := map[string]interface{}{
		"DisabledUntil":       nil,
		"DisabledReason":      "",
		"ConsecutiveFailures": 0,
		"LastError":           "",
		"LastLatencyMs":       int64(0),
		"LastCheckedAt":       nil,
		"LastSuccessAt":       nil,
	}
	if err := m.store.UpdateByID(ctx, endpointID, updates); err != nil {
		return err
	}
	_, _ = m.EffectiveURL(ctx, ep.Chain, ep.Transport)
	return nil
}

func (m *Manager) DeleteEndpoint(ctx context.Context, endpointID uint) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("rpcpool store not available")
	}
	ep, err := m.store.GetByID(ctx, endpointID)
	if err != nil {
		return err
	}
	if ep == nil {
		return fmt.Errorf("rpc endpoint not found")
	}
	wasCurrent := ep.IsCurrent
	chain, transport := ep.Chain, ep.Transport
	if err := m.store.DeleteByID(ctx, endpointID); err != nil {
		return err
	}
	if wasCurrent {
		_, _ = m.EffectiveURL(ctx, chain, transport)
	}
	return nil
}

// CheckOne probes a single endpoint and returns updated status.
func (m *Manager) CheckOne(ctx context.Context, endpointID uint) error {
	if m == nil || m.store == nil || m.prober == nil {
		return fmt.Errorf("rpcpool not ready")
	}
	ep, err := m.store.GetByID(ctx, endpointID)
	if err != nil {
		return err
	}
	if ep == nil {
		return fmt.Errorf("rpc endpoint not found")
	}

	now := m.now()
	chain := config.NormalizeChain(ep.Chain)
	transport := NormalizeTransport(ep.Transport)

	start := time.Now()
	probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	latency, probeErr := m.prober.Probe(probeCtx, strings.TrimSpace(ep.URL), transport)
	cancel()
	if latency <= 0 {
		latency = time.Since(start)
	}

	updates := map[string]interface{}{
		"LastCheckedAt": &now,
		"LastLatencyMs": latency.Milliseconds(),
	}

	if probeErr == nil {
		updates["LastSuccessAt"] = &now
		updates["ConsecutiveFailures"] = 0
		updates["LastError"] = ""
		if ep.DisabledReason == ReasonHealthFail {
			updates["DisabledUntil"] = nil
			updates["DisabledReason"] = ""
		}
	} else {
		errStr := truncateString(strings.TrimSpace(probeErr.Error()), m.maxLastErrorLength)
		updates["LastError"] = errStr
		updates["ConsecutiveFailures"] = ep.ConsecutiveFailures + 1
	}

	if err := m.store.UpdateByID(ctx, ep.ID, updates); err != nil {
		return err
	}

	if probeErr != nil && ep.IsCurrent && ep.ConsecutiveFailures+1 >= m.failureThreshold {
		_, _ = m.EffectiveURL(ctx, chain, transport)
	}
	return probeErr
}

// CheckAllOnce probes all available endpoints and updates status fields.
// It auto-disables endpoints on quota exhaustion, and temporarily disables
// endpoints after repeated failures to enable failover.
func (m *Manager) CheckAllOnce(ctx context.Context) error {
	if m == nil || m.store == nil || m.prober == nil {
		return nil
	}
	now := m.now()

	list, err := m.store.ListAll(ctx)
	if err != nil {
		return err
	}

	type key struct {
		chain     string
		transport string
	}
	needsEnsure := make(map[key]struct{})

	for _, ep := range list {
		chain := config.NormalizeChain(ep.Chain)
		transport := NormalizeTransport(ep.Transport)
		if validateChainTransport(chain, transport) != nil {
			continue
		}

		// Skip currently-disabled endpoints.
		if ep.DisabledUntil != nil && now.Before(*ep.DisabledUntil) {
			continue
		}

		start := time.Now()
		probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		latency, probeErr := m.prober.Probe(probeCtx, strings.TrimSpace(ep.URL), transport)
		cancel()
		if latency <= 0 {
			latency = time.Since(start)
		}

		updates := map[string]interface{}{
			"LastCheckedAt": &now,
			"LastLatencyMs": latency.Milliseconds(),
		}

		if probeErr == nil {
			updates["LastSuccessAt"] = &now
			updates["ConsecutiveFailures"] = 0
			updates["LastError"] = ""
			// Clear temporary disable flags after a success.
			if ep.DisabledReason == ReasonHealthFail {
				updates["DisabledUntil"] = nil
				updates["DisabledReason"] = ""
			}
		} else {
			errStr := truncateString(strings.TrimSpace(probeErr.Error()), m.maxLastErrorLength)
			updates["LastError"] = errStr
			updates["ConsecutiveFailures"] = ep.ConsecutiveFailures + 1

			if IsQuotaExhaustedError(probeErr) {
				until := nextMonthStart(now)
				updates["DisabledUntil"] = &until
				updates["DisabledReason"] = ReasonQuotaExhausted
				updates["IsCurrent"] = false
				needsEnsure[key{chain: chain, transport: transport}] = struct{}{}
			} else if ep.ConsecutiveFailures+1 >= m.failureThreshold {
				until := now.Add(m.tempDisableFor)
				updates["DisabledUntil"] = &until
				updates["DisabledReason"] = ReasonHealthFail
				if ep.IsCurrent {
					updates["IsCurrent"] = false
					needsEnsure[key{chain: chain, transport: transport}] = struct{}{}
				}
			}
		}

		if err := m.store.UpdateByID(ctx, ep.ID, updates); err != nil {
			// Non-fatal; continue probing others.
			continue
		}
	}

	for k := range needsEnsure {
		_, _ = m.EffectiveURL(ctx, k.chain, k.transport)
	}
	return nil
}

func (m *Manager) WithNow(fn func() time.Time) *Manager {
	if m == nil {
		return m
	}
	if fn == nil {
		return m
	}
	m.now = fn
	return m
}

func validateChainTransport(chain string, transport string) error {
	chain = config.NormalizeChain(chain)
	switch chain {
	case "bsc", "base":
	default:
		return fmt.Errorf("unsupported chain=%s", chain)
	}
	switch transport {
	case TransportHTTP, TransportWS:
	default:
		return fmt.Errorf("unsupported transport=%s", transport)
	}
	return nil
}

func isAvailable(ep models.RpcEndpoint, now time.Time) bool {
	if ep.DisabledUntil != nil && now.Before(*ep.DisabledUntil) {
		return false
	}
	if ep.DisabledReason == ReasonHealthFail && ep.DisabledUntil != nil && !now.Before(*ep.DisabledUntil) {
		// A health-failed endpoint should not become eligible again just because
		// the cooldown window elapsed. It must pass a later health check first.
		if ep.LastSuccessAt == nil || !ep.LastSuccessAt.After(*ep.DisabledUntil) {
			return false
		}
	}
	return strings.TrimSpace(ep.URL) != ""
}

var ErrNotReady = errors.New("rpcpool not ready")
