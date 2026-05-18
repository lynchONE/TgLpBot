package assets

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/rpcpool"
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	smartMoneyWalletReadRPCTimeout      = 8 * time.Second
	smartMoneyWalletReadRPCMaxEndpoints = 4
	smartMoneyWalletTokenReadWorkers    = 6
)

type smartMoneyAssetReadEndpoint struct {
	source     rpcpool.Source
	url        string
	endpointID uint
	client     *ethclient.Client
}

type smartMoneyAssetReadRPCPool struct {
	mu      sync.Mutex
	clients map[string]*ethclient.Client
	next    map[string]int
}

var defaultSmartMoneyAssetReadRPCPool = &smartMoneyAssetReadRPCPool{
	clients: make(map[string]*ethclient.Client),
	next:    make(map[string]int),
}

func (p *smartMoneyAssetReadRPCPool) endpoints(ctx context.Context, chain string) ([]smartMoneyAssetReadEndpoint, error) {
	if p == nil {
		return nil, fmt.Errorf("smart money asset rpc pool is nil")
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
		return nil, fmt.Errorf("smart money asset rpc unavailable: chain=%s", chain)
	}

	out := make([]smartMoneyAssetReadEndpoint, 0, len(items))
	for _, item := range items {
		url := strings.TrimSpace(item.URL)
		if url == "" {
			continue
		}
		client, err := p.client(url)
		if err != nil {
			log.Printf("[Assets] smart money asset rpc dial failed source=%s endpoint=%d err=%v", item.Source, smartMoneyAssetReadEndpointID(item.Endpoint), err)
			continue
		}
		out = append(out, smartMoneyAssetReadEndpoint{
			source:     item.Source,
			url:        url,
			endpointID: smartMoneyAssetReadEndpointID(item.Endpoint),
			client:     client,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("smart money asset rpc clients unavailable: chain=%s", chain)
	}

	p.mu.Lock()
	start := p.next[chain]
	if start >= len(out) {
		start = 0
	}
	if start > 0 {
		rotated := make([]smartMoneyAssetReadEndpoint, 0, len(out))
		rotated = append(rotated, out[start:]...)
		rotated = append(rotated, out[:start]...)
		out = rotated
	}
	p.next[chain] = (start + 1) % len(out)
	p.mu.Unlock()

	if len(out) > smartMoneyWalletReadRPCMaxEndpoints {
		out = out[:smartMoneyWalletReadRPCMaxEndpoints]
	}
	return out, nil
}

func (p *smartMoneyAssetReadRPCPool) client(url string) (*ethclient.Client, error) {
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

func smartMoneyAssetReadEndpointID(ep *models.RpcEndpoint) uint {
	if ep == nil {
		return 0
	}
	return ep.ID
}

func (s *Service) smartMoneyAssetReadEndpoints(ctx context.Context, chain string) (config.ChainConfig, []smartMoneyAssetReadEndpoint, error) {
	if config.AppConfig == nil {
		return config.ChainConfig{}, nil, fmt.Errorf("config not loaded")
	}
	chain = config.NormalizeChain(chain)
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok {
		return config.ChainConfig{}, nil, fmt.Errorf("chain config not found: %s", chain)
	}

	endpoints, err := defaultSmartMoneyAssetReadRPCPool.endpoints(ctx, chain)
	if err == nil && len(endpoints) > 0 {
		return cc, endpoints, nil
	}

	client, _, clientErr := blockchain.GetEVMClient(chain)
	if clientErr != nil {
		if err != nil {
			return cc, nil, err
		}
		return cc, nil, clientErr
	}
	return cc, []smartMoneyAssetReadEndpoint{{
		source: rpcpool.SourceEnv,
		client: client,
	}}, nil
}

func smartMoneyAssetEndpointAt(endpoints []smartMoneyAssetReadEndpoint, index int) smartMoneyAssetReadEndpoint {
	return endpoints[index%len(endpoints)]
}

func readSmartMoneyNativeBalanceFromPool(ctx context.Context, endpoints []smartMoneyAssetReadEndpoint, wallet common.Address) (*big.Int, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("smart money asset rpc clients unavailable")
	}
	var lastErr error
	for i := range endpoints {
		balance, err := readSmartMoneyNativeBalance(ctx, smartMoneyAssetEndpointAt(endpoints, i), wallet)
		if err == nil {
			return balance, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func readSmartMoneyTokenBalanceFromPool(ctx context.Context, endpoints []smartMoneyAssetReadEndpoint, start int, tokenAddress common.Address, wallet common.Address) (*big.Int, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("smart money asset rpc clients unavailable")
	}
	var lastErr error
	for i := range endpoints {
		balance, err := readSmartMoneyTokenBalance(ctx, smartMoneyAssetEndpointAt(endpoints, start+i), tokenAddress, wallet)
		if err == nil {
			return balance, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func readSmartMoneyTokenDecimalsFromPool(ctx context.Context, endpoints []smartMoneyAssetReadEndpoint, start int, tokenAddress common.Address) (uint8, error) {
	if len(endpoints) == 0 {
		return 0, fmt.Errorf("smart money asset rpc clients unavailable")
	}
	var lastErr error
	for i := range endpoints {
		decimals, err := readSmartMoneyTokenDecimals(ctx, smartMoneyAssetEndpointAt(endpoints, start+i), tokenAddress)
		if err == nil {
			return decimals, nil
		}
		lastErr = err
	}
	return 0, lastErr
}

func readSmartMoneyNativeBalance(ctx context.Context, endpoint smartMoneyAssetReadEndpoint, wallet common.Address) (*big.Int, error) {
	if endpoint.client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	callCtx, cancel := context.WithTimeout(ctx, smartMoneyWalletReadRPCTimeout)
	defer cancel()
	balance, err := endpoint.client.BalanceAt(callCtx, wallet, nil)
	if err != nil {
		recordSmartMoneyAssetReadRPCError(endpoint, err)
		return nil, err
	}
	return balance, nil
}

func readSmartMoneyTokenBalance(ctx context.Context, endpoint smartMoneyAssetReadEndpoint, tokenAddress common.Address, wallet common.Address) (*big.Int, error) {
	if endpoint.client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	token, err := blockchain.NewERC20(tokenAddress, endpoint.client)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, smartMoneyWalletReadRPCTimeout)
	defer cancel()
	balance, err := token.BalanceOf(&bind.CallOpts{Context: callCtx}, wallet)
	if err != nil {
		recordSmartMoneyAssetReadRPCError(endpoint, err)
		return nil, err
	}
	return balance, nil
}

func readSmartMoneyTokenDecimals(ctx context.Context, endpoint smartMoneyAssetReadEndpoint, tokenAddress common.Address) (uint8, error) {
	if endpoint.client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}
	token, err := blockchain.NewERC20(tokenAddress, endpoint.client)
	if err != nil {
		return 0, err
	}
	callCtx, cancel := context.WithTimeout(ctx, smartMoneyWalletReadRPCTimeout)
	defer cancel()
	decimals, err := token.Decimals(&bind.CallOpts{Context: callCtx})
	if err != nil {
		recordSmartMoneyAssetReadRPCError(endpoint, err)
		return 0, err
	}
	return decimals, nil
}

func recordSmartMoneyAssetReadRPCError(endpoint smartMoneyAssetReadEndpoint, err error) {
	if err == nil || endpoint.endpointID == 0 {
		return
	}
	if rpcpool.IsQuotaExhaustedError(err) {
		_ = rpcpool.Default().DisableUntilNextMonth(context.Background(), endpoint.endpointID)
	}
}
