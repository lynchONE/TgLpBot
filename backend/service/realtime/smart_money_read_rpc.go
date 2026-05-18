package realtime

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/rpcpool"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	smartMoneyReadRPCTimeout     = 8 * time.Second
	smartMoneyReadRPCMaxAttempts = 3
)

type smartMoneyReadEndpoint struct {
	source     rpcpool.Source
	url        string
	endpointID uint
	client     *ethclient.Client
}

type smartMoneyReadRPCPool struct {
	mu      sync.Mutex
	clients map[string]*ethclient.Client
	next    map[string]int
}

type nonRetryableSmartMoneyReadError struct {
	err error
}

func (e nonRetryableSmartMoneyReadError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e nonRetryableSmartMoneyReadError) Unwrap() error {
	return e.err
}

func nonRetryableSmartMoneyRead(err error) error {
	if err == nil {
		return nil
	}
	return nonRetryableSmartMoneyReadError{err: err}
}

var defaultSmartMoneyReadRPCPool = &smartMoneyReadRPCPool{
	clients: make(map[string]*ethclient.Client),
	next:    make(map[string]int),
}

func (p *smartMoneyReadRPCPool) endpoints(ctx context.Context, chain string) ([]smartMoneyReadEndpoint, error) {
	if p == nil {
		return nil, fmt.Errorf("smart money read rpc pool is nil")
	}
	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}

	items, err := rpcpool.Default().AvailableEndpoints(ctx, chain, rpcpool.TransportHTTP)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("smart money read rpc unavailable: chain=%s", chain)
	}

	out := make([]smartMoneyReadEndpoint, 0, len(items))
	for _, item := range items {
		url := strings.TrimSpace(item.URL)
		if url == "" {
			continue
		}
		client, err := p.client(url)
		if err != nil {
			log.Printf("[SmartMoney ReadRPC] dial failed source=%s endpoint=%d err=%v", item.Source, smartMoneyReadEndpointID(item.Endpoint), err)
			continue
		}
		out = append(out, smartMoneyReadEndpoint{
			source:     item.Source,
			url:        url,
			endpointID: smartMoneyReadEndpointID(item.Endpoint),
			client:     client,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("smart money read rpc clients unavailable: chain=%s", chain)
	}

	p.mu.Lock()
	start := p.next[chain]
	if start >= len(out) {
		start = 0
	}
	if start > 0 {
		rotated := make([]smartMoneyReadEndpoint, 0, len(out))
		rotated = append(rotated, out[start:]...)
		rotated = append(rotated, out[:start]...)
		out = rotated
	}
	p.next[chain] = (start + 1) % len(out)
	p.mu.Unlock()
	return out, nil
}

func (p *smartMoneyReadRPCPool) client(url string) (*ethclient.Client, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, fmt.Errorf("rpc url is empty")
	}

	p.mu.Lock()
	if c := p.clients[url]; c != nil {
		p.mu.Unlock()
		return c, nil
	}
	p.mu.Unlock()

	c, err := ethclient.Dial(url)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if existing := p.clients[url]; existing != nil {
		p.mu.Unlock()
		c.Close()
		return existing, nil
	}
	p.clients[url] = c
	p.mu.Unlock()
	return c, nil
}

func smartMoneyReadEndpointID(ep *models.RpcEndpoint) uint {
	if ep == nil {
		return 0
	}
	return ep.ID
}

func (s *RealtimePositionsService) withSmartMoneyReadClient(ctx context.Context, chain string, fn func(context.Context, *ethclient.Client) error) error {
	if fn == nil {
		return fmt.Errorf("smart money read function is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	endpoints, err := defaultSmartMoneyReadRPCPool.endpoints(ctx, chain)
	if err != nil {
		return err
	}
	if len(endpoints) > smartMoneyReadRPCMaxAttempts {
		endpoints = endpoints[:smartMoneyReadRPCMaxAttempts]
	}

	var lastErr error
	for _, endpoint := range endpoints {
		callCtx, cancel := context.WithTimeout(ctx, smartMoneyReadRPCTimeout)
		err := fn(callCtx, endpoint.client)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		var nonRetryable nonRetryableSmartMoneyReadError
		if errors.As(err, &nonRetryable) {
			return nonRetryable.err
		}
		if rpcpool.IsQuotaExhaustedError(err) && endpoint.endpointID > 0 {
			_ = rpcpool.Default().DisableUntilNextMonth(context.Background(), endpoint.endpointID)
		}
		log.Printf("[SmartMoney ReadRPC] call failed source=%s endpoint=%d err=%v", endpoint.source, endpoint.endpointID, err)
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("smart money read rpc unavailable")
}
