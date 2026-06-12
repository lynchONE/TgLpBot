package smart_money

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/models"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const MonitoredWalletSourceTokenLiquidityIndexer = "token_liquidity_indexer"
const MonitoredWalletSourcePoolLiquidityRadar = "pool_liquidity_radar"

const (
	rpcPoolLiquidityProviderName    = "rpc"
	rpcPoolLiquidityMaxWindow       = 7 * 24 * time.Hour
	rpcPoolLiquidityMaxWindowHours  = int(rpcPoolLiquidityMaxWindow / time.Hour)
	rpcPoolLiquidityMaxLogs         = 5000
	rpcPoolLiquidityBlockChunk      = uint64(2000)
	rpcPoolLiquidityHeaderCacheSize = 512
)

type TokenLiquidityCandidateQuery struct {
	Chain        string
	ChainID      int
	TokenAddress string
	PoolAddress  string
	PoolID       string
	MinAmountUSD float64
	WindowHours  int
	StartTime    time.Time
	EndTime      time.Time
	Limit        int
	Provider     string
}

type TokenLiquidityCandidate struct {
	WalletAddress    string   `json:"wallet_address"`
	MaxAmountUSD     float64  `json:"max_amount_usd"`
	LastAmountUSD    float64  `json:"last_amount_usd"`
	TxHash           string   `json:"tx_hash"`
	TxTime           string   `json:"tx_time"`
	TokenAddress     string   `json:"token_address"`
	PoolAddress      string   `json:"pool_address"`
	PoolID           string   `json:"pool_id,omitempty"`
	Protocol         string   `json:"protocol,omitempty"`
	Pair             string   `json:"pair"`
	PoolCount        int      `json:"pool_count"`
	AmountSource     string   `json:"amount_source"`
	Provider         string   `json:"provider"`
	AlreadyMonitored bool     `json:"already_monitored"`
	ExcludedReasons  []string `json:"excluded_reasons,omitempty"`
}

type TokenLiquidityCandidateResponse struct {
	Token         TokenLiquidityTokenInfo    `json:"token"`
	Pool          TokenLiquidityPoolInfo     `json:"pool"`
	Filters       TokenLiquidityFilterInfo   `json:"filters"`
	Sources       []TokenLiquiditySourceInfo `json:"sources"`
	Candidates    []TokenLiquidityCandidate  `json:"candidates"`
	ExcludedCount int                        `json:"excluded_count"`
	Warnings      []string                   `json:"warnings"`
}

type TokenLiquidityTokenInfo struct {
	Address string `json:"address"`
	Chain   string `json:"chain"`
}

type TokenLiquidityPoolInfo struct {
	Address string `json:"address,omitempty"`
	PoolID  string `json:"pool_id,omitempty"`
	Chain   string `json:"chain"`
}

type TokenLiquidityFilterInfo struct {
	PoolAddress  string  `json:"pool_address,omitempty"`
	PoolID       string  `json:"pool_id,omitempty"`
	MinAmountUSD float64 `json:"min_amount_usd"`
	WindowHours  int     `json:"window_hours"`
	StartTime    string  `json:"start_time,omitempty"`
	EndTime      string  `json:"end_time,omitempty"`
	Limit        int     `json:"limit"`
}

type TokenLiquiditySourceInfo struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type TokenLiquidityProvider interface {
	FindCandidates(ctx context.Context, query TokenLiquidityCandidateQuery) (*TokenLiquidityCandidateResponse, error)
}

func NewTokenLiquidityProviderFromConfig(cfg *config.Config) (TokenLiquidityProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	return NewRPCTokenLiquidityProvider(), nil
}

func NormalizeTokenLiquidityCandidateQuery(query TokenLiquidityCandidateQuery) (TokenLiquidityCandidateQuery, error) {
	query.Chain = config.NormalizeChain(query.Chain)
	if query.ChainID <= 0 {
		switch query.Chain {
		case "base":
			query.ChainID = 8453
		default:
			query.ChainID = 56
		}
	}
	if query.Chain == "" {
		query.Chain = ChainSlugForID(query.ChainID)
	}
	query.TokenAddress = strings.ToLower(strings.TrimSpace(query.TokenAddress))
	query.PoolAddress = strings.ToLower(strings.TrimSpace(query.PoolAddress))
	query.PoolID = strings.ToLower(strings.TrimSpace(query.PoolID))
	if query.PoolAddress == "" && query.TokenAddress != "" {
		return query, fmt.Errorf("token_address is no longer supported; use pool_address or pool_id")
	}
	if query.PoolAddress == "" && query.PoolID == "" {
		return query, fmt.Errorf("pool_address or pool_id is required")
	}
	if query.PoolAddress != "" && query.PoolID != "" {
		return query, fmt.Errorf("pool_address and pool_id cannot both be set")
	}
	if query.PoolAddress != "" && !isEVMAddress(query.PoolAddress) {
		return query, fmt.Errorf("invalid pool_address")
	}
	if query.PoolID != "" && !isEVMHash(query.PoolID) {
		return query, fmt.Errorf("invalid pool_id")
	}
	if query.MinAmountUSD <= 0 || math.IsNaN(query.MinAmountUSD) || math.IsInf(query.MinAmountUSD, 0) {
		return query, fmt.Errorf("min_amount_usd must be greater than 0")
	}
	if query.StartTime.IsZero() && !query.EndTime.IsZero() {
		return query, fmt.Errorf("start_time is required")
	}
	if query.StartTime.IsZero() && query.WindowHours <= 0 {
		return query, fmt.Errorf("window_hours must be greater than 0")
	}
	if query.WindowHours > rpcPoolLiquidityMaxWindowHours {
		return query, fmt.Errorf("window_hours is too large")
	}
	if !query.StartTime.IsZero() {
		query.StartTime = query.StartTime.UTC()
		if query.EndTime.IsZero() {
			return query, fmt.Errorf("end_time is required")
		}
		query.EndTime = query.EndTime.UTC()
		if !query.EndTime.After(query.StartTime) {
			return query, fmt.Errorf("end_time must be after start_time")
		}
		if query.EndTime.Sub(query.StartTime) > rpcPoolLiquidityMaxWindow {
			return query, fmt.Errorf("time range is too large")
		}
	}
	if query.Limit <= 0 {
		return query, fmt.Errorf("limit must be greater than 0")
	}
	if query.Limit > 100 {
		return query, fmt.Errorf("limit cannot exceed 100")
	}
	query.Provider = strings.ToLower(strings.TrimSpace(query.Provider))
	if query.Provider != "" {
		return query, fmt.Errorf("provider is no longer supported")
	}
	return query, nil
}

func normalizeLegacyTokenLiquidityCandidateQuery(query TokenLiquidityCandidateQuery) (TokenLiquidityCandidateQuery, error) {
	query.Chain = config.NormalizeChain(query.Chain)
	if query.ChainID <= 0 {
		switch query.Chain {
		case "base":
			query.ChainID = 8453
		default:
			query.ChainID = 56
		}
	}
	if query.Chain == "" {
		query.Chain = ChainSlugForID(query.ChainID)
	}
	query.TokenAddress = strings.ToLower(strings.TrimSpace(query.TokenAddress))
	if !isEVMAddress(query.TokenAddress) {
		return query, fmt.Errorf("invalid token_address")
	}
	if query.MinAmountUSD <= 0 || math.IsNaN(query.MinAmountUSD) || math.IsInf(query.MinAmountUSD, 0) {
		return query, fmt.Errorf("min_amount_usd must be greater than 0")
	}
	if query.StartTime.IsZero() && !query.EndTime.IsZero() {
		return query, fmt.Errorf("start_time is required")
	}
	if query.StartTime.IsZero() && query.WindowHours <= 0 {
		return query, fmt.Errorf("window_hours must be greater than 0")
	}
	if query.WindowHours > 24*30 {
		return query, fmt.Errorf("window_hours is too large")
	}
	if !query.StartTime.IsZero() {
		query.StartTime = query.StartTime.UTC()
		if query.EndTime.IsZero() {
			return query, fmt.Errorf("end_time is required")
		}
		query.EndTime = query.EndTime.UTC()
		if !query.EndTime.After(query.StartTime) {
			return query, fmt.Errorf("end_time must be after start_time")
		}
		if query.EndTime.Sub(query.StartTime) > 30*24*time.Hour {
			return query, fmt.Errorf("time range is too large")
		}
	}
	if query.Limit <= 0 {
		return query, fmt.Errorf("limit must be greater than 0")
	}
	if query.Limit > 100 {
		return query, fmt.Errorf("limit cannot exceed 100")
	}
	query.Provider = strings.ToLower(strings.TrimSpace(query.Provider))
	if query.Provider != "" && query.Provider != "bitquery" {
		return query, fmt.Errorf("unsupported provider: %s", query.Provider)
	}
	return query, nil
}

func ChainSlugForID(chainID int) string {
	switch chainID {
	case 8453:
		return "base"
	default:
		return "bsc"
	}
}

func isEVMAddress(addr string) bool {
	addr = strings.TrimSpace(addr)
	if len(addr) != 42 {
		return false
	}
	if !strings.HasPrefix(addr, "0x") && !strings.HasPrefix(addr, "0X") {
		return false
	}
	for _, c := range addr[2:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func isEVMHash(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 66 {
		return false
	}
	if !strings.HasPrefix(value, "0x") && !strings.HasPrefix(value, "0X") {
		return false
	}
	for _, c := range value[2:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

type RPCTokenLiquidityProvider struct{}

func NewRPCTokenLiquidityProvider() *RPCTokenLiquidityProvider {
	return &RPCTokenLiquidityProvider{}
}

func (p *RPCTokenLiquidityProvider) FindCandidates(ctx context.Context, query TokenLiquidityCandidateQuery) (*TokenLiquidityCandidateResponse, error) {
	query, err := NormalizeTokenLiquidityCandidateQuery(query)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, fmt.Errorf("rpc liquidity provider is nil")
	}

	client, _, err := blockchain.GetEVMClient(query.Chain)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("rpc client not initialized")
	}
	since, till := resolveTokenLiquidityTimeRange(query)
	fromBlock, toBlock, err := resolveRPCBlockRangeByTime(ctx, client, since, till)
	if err != nil {
		return nil, err
	}
	if toBlock < fromBlock {
		return poolLiquidityEmptyResponse(query, since, till, "rpc returned an empty block range"), nil
	}

	watcher := newPoolLiquidityRadarWatcher(query.ChainID)
	if query.PoolID != "" {
		return p.findV4Candidates(ctx, client, watcher, query, since, till, fromBlock, toBlock)
	}
	return p.findV3Candidates(ctx, client, watcher, query, since, till, fromBlock, toBlock)
}

func (p *RPCTokenLiquidityProvider) findV3Candidates(ctx context.Context, client *ethclient.Client, watcher *Watcher, query TokenLiquidityCandidateQuery, since time.Time, till time.Time, fromBlock uint64, toBlock uint64) (*TokenLiquidityCandidateResponse, error) {
	poolAddress := common.HexToAddress(query.PoolAddress)
	deployment, err := resolveV3DeploymentForPool(ctx, client, query.Chain, poolAddress)
	if err != nil {
		return nil, err
	}
	logs, truncated, err := filterPoolLiquidityLogs(ctx, client, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(fromBlock),
		ToBlock:   new(big.Int).SetUint64(toBlock),
		Addresses: []common.Address{deployment.PositionManager},
		Topics: [][]common.Hash{{
			TopicIncreaseLiquidity,
		}},
	})
	if err != nil {
		return nil, err
	}

	headerCache := newRPCHeaderCache(client)
	items := make([]*models.SmartMoneyLPEvent, 0, len(logs))
	excluded := 0
	warnings := []string{}
	for _, vlog := range logs {
		event, err := parsePoolLiquidityCandidateLog(ctx, client, watcher, headerCache, vlog)
		if err != nil {
			excluded++
			continue
		}
		if event.EventType != "add" {
			excluded++
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(event.PoolAddress), query.PoolAddress) {
			excluded++
			continue
		}
		owner, ownerSource, err := resolveV3LiquidityOwner(ctx, client, deployment.PositionManager, event, vlog.BlockNumber)
		if err != nil {
			excluded++
			warnings = appendLimitedWarning(warnings, fmt.Sprintf("excluded tx %s: %v", shortAddr(event.TxHash), err))
			continue
		}
		event.WalletAddress = strings.ToLower(owner.Hex())
		if event.Token0Amount == "0" && event.Token1Amount == "0" && event.Token0Address != "" && event.Token1Address != "" {
			watcher.resolveAmountsFromReceipt(ctx, event)
		}
		ComputeEventAmountUSD(ctx, event)
		if eventTotalUSD(event) < query.MinAmountUSD {
			excluded++
			continue
		}
		if ownerSource == "current_ownerOf" {
			warnings = appendLimitedWarning(warnings, fmt.Sprintf("tx %s owner resolved from current ownerOf because historical ownerOf was unavailable", shortAddr(event.TxHash)))
		}
		items = append(items, event)
	}
	if truncated {
		warnings = append(warnings, fmt.Sprintf("rpc log result reached %d logs; narrow the time range for complete results", rpcPoolLiquidityMaxLogs))
	}
	return aggregateRPCLiquidityCandidates(query, since, till, items, excluded, warnings), nil
}

func (p *RPCTokenLiquidityProvider) findV4Candidates(ctx context.Context, client *ethclient.Client, watcher *Watcher, query TokenLiquidityCandidateQuery, since time.Time, till time.Time, fromBlock uint64, toBlock uint64) (*TokenLiquidityCandidateResponse, error) {
	poolManager, err := resolveV4PoolManagerAddress(query.Chain)
	if err != nil {
		return nil, err
	}
	positionManager, err := resolveV4PositionManagerAddress(query.Chain)
	if err != nil {
		return nil, err
	}
	poolID := common.HexToHash(query.PoolID)
	logs, truncated, err := filterPoolLiquidityLogs(ctx, client, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(fromBlock),
		ToBlock:   new(big.Int).SetUint64(toBlock),
		Addresses: []common.Address{poolManager},
		Topics: [][]common.Hash{
			{TopicModifyLiquidity},
			{poolID},
		},
	})
	if err != nil {
		return nil, err
	}

	headerCache := newRPCHeaderCache(client)
	items := make([]*models.SmartMoneyLPEvent, 0, len(logs))
	excluded := 0
	warnings := []string{}
	for _, vlog := range logs {
		event, err := parsePoolLiquidityCandidateLog(ctx, client, watcher, headerCache, vlog)
		if err != nil {
			excluded++
			continue
		}
		if event.EventType != "add" {
			excluded++
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(event.PoolAddress), query.PoolID) {
			excluded++
			continue
		}
		if event.NftTokenID == nil || *event.NftTokenID == 0 {
			excluded++
			warnings = appendLimitedWarning(warnings, fmt.Sprintf("excluded tx %s: v4 event has no position token id", shortAddr(event.TxHash)))
			continue
		}
		owner, ownerSource, err := resolveV4LiquidityOwner(ctx, client, positionManager, event, vlog.BlockNumber)
		if err != nil {
			excluded++
			warnings = appendLimitedWarning(warnings, fmt.Sprintf("excluded tx %s: %v", shortAddr(event.TxHash), err))
			continue
		}
		event.WalletAddress = strings.ToLower(owner.Hex())
		if event.Token0Amount == "0" && event.Token1Amount == "0" && event.Token0Address != "" && event.Token1Address != "" {
			watcher.resolveAmountsFromReceipt(ctx, event)
		}
		ComputeEventAmountUSD(ctx, event)
		if eventTotalUSD(event) < query.MinAmountUSD {
			excluded++
			continue
		}
		if ownerSource == "current_ownerOf" {
			warnings = appendLimitedWarning(warnings, fmt.Sprintf("tx %s owner resolved from current ownerOf because historical ownerOf was unavailable", shortAddr(event.TxHash)))
		}
		items = append(items, event)
	}
	if truncated {
		warnings = append(warnings, fmt.Sprintf("rpc log result reached %d logs; narrow the time range for complete results", rpcPoolLiquidityMaxLogs))
	}
	return aggregateRPCLiquidityCandidates(query, since, till, items, excluded, warnings), nil
}

type v3PoolDeployment struct {
	Protocol        string
	Factory         common.Address
	PositionManager common.Address
}

func resolveV3DeploymentForPool(ctx context.Context, client *ethclient.Client, chain string, poolAddress common.Address) (*v3PoolDeployment, error) {
	if poolAddress == (common.Address{}) {
		return nil, fmt.Errorf("pool_address is empty")
	}
	token0, token1, err := blockchain.GetV3PoolTokensWithClient(client, poolAddress)
	if err != nil {
		return nil, err
	}
	fee, err := blockchain.GetV3PoolFeeWithClient(client, poolAddress)
	if err != nil {
		return nil, err
	}
	deployments := configuredV3Deployments(chain)
	if len(deployments) == 0 {
		return nil, fmt.Errorf("v3 deployments are not configured")
	}
	for _, dep := range deployments {
		callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		resolvedPool, err := blockchain.GetV3PoolFromFactoryCtxWithClient(client, callCtx, dep.Factory, token0, token1, uint64(fee))
		cancel()
		if err != nil {
			continue
		}
		if resolvedPool == poolAddress {
			matched := dep
			return &matched, nil
		}
	}
	return nil, fmt.Errorf("pool_address does not match configured v3 deployments")
}

func configuredV3Deployments(chain string) []v3PoolDeployment {
	chain = config.NormalizeChain(chain)
	out := []v3PoolDeployment{}
	seen := map[string]struct{}{}
	add := func(name string, factoryAddr string, managerAddr string) {
		if !common.IsHexAddress(strings.TrimSpace(factoryAddr)) || !common.IsHexAddress(strings.TrimSpace(managerAddr)) {
			return
		}
		protocol := protocolFromDeploymentName(name)
		if protocol == "" {
			return
		}
		key := strings.ToLower(strings.TrimSpace(factoryAddr)) + "|" + strings.ToLower(strings.TrimSpace(managerAddr))
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, v3PoolDeployment{
			Protocol:        protocol,
			Factory:         common.HexToAddress(factoryAddr),
			PositionManager: common.HexToAddress(managerAddr),
		})
	}
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
			for _, dep := range cc.V3Deployments {
				add(dep.Name, dep.FactoryAddress, dep.PositionManagerAddress)
			}
		}
		add("PancakeSwap V3", "0x0BFbcf9fa4f9C56B0F40a671Ad40E0805A091865", config.AppConfig.PancakeV3PositionManagerAddress)
		add("Uniswap V3", "0xdB1d10011AD0Ff90774D0C6Bb92e5C5c8b4461F7", config.AppConfig.UniswapV3PositionManagerAddress)
	}
	return out
}

func protocolFromDeploymentName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(name, "pancake"):
		return "pancake_v3"
	case strings.Contains(name, "uniswap"):
		return "uniswap_v3"
	default:
		return ""
	}
}

func resolveV4PoolManagerAddress(chain string) (common.Address, error) {
	chain = config.NormalizeChain(chain)
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok && common.IsHexAddress(cc.UniswapV4PoolManagerAddress) {
			return common.HexToAddress(cc.UniswapV4PoolManagerAddress), nil
		}
		if common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
			return common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress), nil
		}
	}
	return common.Address{}, fmt.Errorf("uniswap v4 pool manager not configured")
}

func newPoolLiquidityRadarWatcher(chainID int) *Watcher {
	if chainID <= 0 {
		chainID = 56
	}
	var pancakeNPM, uniswapNPM, v4PoolManager string
	chain := smartMoneyChainName(chainID)
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
			for _, dep := range cc.V3Deployments {
				name := strings.ToLower(strings.TrimSpace(dep.Name))
				if strings.Contains(name, "pancake") && strings.TrimSpace(pancakeNPM) == "" {
					pancakeNPM = strings.TrimSpace(dep.PositionManagerAddress)
				}
				if strings.Contains(name, "uniswap") && strings.TrimSpace(uniswapNPM) == "" {
					uniswapNPM = strings.TrimSpace(dep.PositionManagerAddress)
				}
			}
			v4PoolManager = strings.TrimSpace(cc.UniswapV4PoolManagerAddress)
		}
		if strings.TrimSpace(pancakeNPM) == "" {
			pancakeNPM = strings.TrimSpace(config.AppConfig.PancakeV3PositionManagerAddress)
		}
		if strings.TrimSpace(uniswapNPM) == "" {
			uniswapNPM = strings.TrimSpace(config.AppConfig.UniswapV3PositionManagerAddress)
		}
		if strings.TrimSpace(v4PoolManager) == "" {
			v4PoolManager = strings.TrimSpace(config.AppConfig.UniswapV4PoolManagerAddress)
		}
	}
	return NewWatcher(nil, int64(chainID), pancakeNPM, uniswapNPM, v4PoolManager, 2)
}

func filterPoolLiquidityLogs(ctx context.Context, client *ethclient.Client, base ethereum.FilterQuery) ([]types.Log, bool, error) {
	if client == nil {
		return nil, false, fmt.Errorf("rpc client not initialized")
	}
	if base.FromBlock == nil || base.ToBlock == nil {
		return nil, false, fmt.Errorf("block range is required")
	}
	from := base.FromBlock.Uint64()
	to := base.ToBlock.Uint64()
	if to < from {
		return []types.Log{}, false, nil
	}

	out := []types.Log{}
	truncated := false
	chunk := rpcPoolLiquidityBlockChunk
	for start := from; start <= to; {
		if chunk == 0 {
			return nil, false, fmt.Errorf("rpc log block chunk is zero")
		}
		end := start + chunk - 1
		if end > to {
			end = to
		}
		query := base
		query.FromBlock = new(big.Int).SetUint64(start)
		query.ToBlock = new(big.Int).SetUint64(end)
		logs, err := client.FilterLogs(ctx, query)
		if err != nil {
			if isRPCLogRangeLimitError(err) && chunk > 1 {
				chunk = chunk / 2
				if chunk == 0 {
					chunk = 1
				}
				continue
			}
			return nil, false, fmt.Errorf("filter logs %d-%d: %w", start, end, err)
		}
		out = append(out, logs...)
		if len(out) >= rpcPoolLiquidityMaxLogs {
			out = out[:rpcPoolLiquidityMaxLogs]
			truncated = true
			break
		}
		if end == to {
			break
		}
		start = end + 1
		if chunk < rpcPoolLiquidityBlockChunk {
			chunk = rpcPoolLiquidityBlockChunk
		}
	}
	return out, truncated, nil
}

func isRPCLogRangeLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"block range",
		"range too large",
		"more than",
		"too many",
		"limit exceeded",
		"query returned more than",
		"exceed maximum block range",
		"response size exceeded",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func parsePoolLiquidityCandidateLog(ctx context.Context, client *ethclient.Client, watcher *Watcher, headerCache *rpcHeaderCache, vlog types.Log) (*models.SmartMoneyLPEvent, error) {
	if watcher == nil {
		return nil, fmt.Errorf("watcher not initialized")
	}
	event, err := watcher.parseLog(vlog)
	if err != nil {
		return nil, err
	}
	event.ChainID = int(watcher.chainID)
	event.TxHash = strings.ToLower(vlog.TxHash.Hex())
	event.BlockNumber = vlog.BlockNumber
	event.LogIndex = int(vlog.Index)
	if event.Token0Amount == "" {
		event.Token0Amount = "0"
	}
	if event.Token1Amount == "" {
		event.Token1Amount = "0"
	}
	if err := EnrichLPEvent(ctx, event); err != nil {
		return nil, err
	}
	header, err := headerCache.HeaderByNumber(ctx, client, vlog.BlockNumber)
	if err != nil {
		return nil, err
	}
	event.TxTimestamp = time.Unix(int64(header.Time), 0).UTC()
	return event, nil
}

func resolveV3LiquidityOwner(ctx context.Context, client *ethclient.Client, positionManager common.Address, event *models.SmartMoneyLPEvent, blockNumber uint64) (common.Address, string, error) {
	if event == nil || event.NftTokenID == nil || *event.NftTokenID == 0 {
		return common.Address{}, "", fmt.Errorf("v3 event has no position token id")
	}
	owner, ok, err := resolveERC721TransferOwnerFromReceipt(ctx, client, positionManager, event.TxHash, *event.NftTokenID)
	if err != nil {
		return common.Address{}, "", err
	}
	if ok {
		return owner, "transfer_event", nil
	}
	pm, err := blockchain.NewV3PositionManager(positionManager, client)
	if err != nil {
		return common.Address{}, "", err
	}
	tokenID := new(big.Int).SetUint64(*event.NftTokenID)
	callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	owner, err = pm.OwnerOf(&bind.CallOpts{Context: callCtx, BlockNumber: new(big.Int).SetUint64(blockNumber)}, tokenID)
	cancel()
	if err == nil && owner != (common.Address{}) {
		return owner, "historical_ownerOf", nil
	}
	callCtx, cancel = context.WithTimeout(ctx, 8*time.Second)
	owner, liveErr := pm.OwnerOf(&bind.CallOpts{Context: callCtx}, tokenID)
	cancel()
	if liveErr != nil {
		return common.Address{}, "", fmt.Errorf("resolve v3 ownerOf failed: historical=%v current=%w", err, liveErr)
	}
	if owner == (common.Address{}) {
		return common.Address{}, "", fmt.Errorf("resolve v3 ownerOf returned zero address")
	}
	return owner, "current_ownerOf", nil
}

func resolveV4LiquidityOwner(ctx context.Context, client *ethclient.Client, positionManager common.Address, event *models.SmartMoneyLPEvent, blockNumber uint64) (common.Address, string, error) {
	if event == nil || event.NftTokenID == nil || *event.NftTokenID == 0 {
		return common.Address{}, "", fmt.Errorf("v4 event has no position token id")
	}
	owner, ok, err := resolveERC721TransferOwnerFromReceipt(ctx, client, positionManager, event.TxHash, *event.NftTokenID)
	if err != nil {
		return common.Address{}, "", err
	}
	if ok {
		return owner, "transfer_event", nil
	}
	pm, err := blockchain.NewV4PositionManager(positionManager, client)
	if err != nil {
		return common.Address{}, "", err
	}
	tokenID := new(big.Int).SetUint64(*event.NftTokenID)
	callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	owner, err = pm.OwnerOf(&bind.CallOpts{Context: callCtx, BlockNumber: new(big.Int).SetUint64(blockNumber)}, tokenID)
	cancel()
	if err == nil && owner != (common.Address{}) {
		return owner, "historical_ownerOf", nil
	}
	callCtx, cancel = context.WithTimeout(ctx, 8*time.Second)
	owner, liveErr := pm.OwnerOf(&bind.CallOpts{Context: callCtx}, tokenID)
	cancel()
	if liveErr != nil {
		return common.Address{}, "", fmt.Errorf("resolve v4 ownerOf failed: historical=%v current=%w", err, liveErr)
	}
	if owner == (common.Address{}) {
		return common.Address{}, "", fmt.Errorf("resolve v4 ownerOf returned zero address")
	}
	return owner, "current_ownerOf", nil
}

func resolveERC721TransferOwnerFromReceipt(ctx context.Context, client *ethclient.Client, nftContract common.Address, txHash string, tokenID uint64) (common.Address, bool, error) {
	if client == nil {
		return common.Address{}, false, fmt.Errorf("rpc client not initialized")
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	receipt, err := client.TransactionReceipt(callCtx, common.HexToHash(txHash))
	if err != nil {
		return common.Address{}, false, fmt.Errorf("fetch receipt for owner attribution failed: %w", err)
	}
	if receipt == nil {
		return common.Address{}, false, fmt.Errorf("receipt not found")
	}
	transferTopic := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	tokenTopic := common.BigToHash(new(big.Int).SetUint64(tokenID))
	for _, vlog := range receipt.Logs {
		if vlog == nil || vlog.Address != nftContract || len(vlog.Topics) < 4 {
			continue
		}
		if vlog.Topics[0] != transferTopic || vlog.Topics[3] != tokenTopic {
			continue
		}
		owner := common.BytesToAddress(vlog.Topics[2].Bytes())
		if owner == (common.Address{}) {
			continue
		}
		return owner, true, nil
	}
	return common.Address{}, false, nil
}

type rpcHeaderCache struct {
	mu      sync.Mutex
	client  *ethclient.Client
	headers map[uint64]*types.Header
	order   []uint64
}

func newRPCHeaderCache(client *ethclient.Client) *rpcHeaderCache {
	return &rpcHeaderCache{
		client:  client,
		headers: make(map[uint64]*types.Header),
		order:   make([]uint64, 0, rpcPoolLiquidityHeaderCacheSize),
	}
}

func (c *rpcHeaderCache) HeaderByNumber(ctx context.Context, client *ethclient.Client, blockNumber uint64) (*types.Header, error) {
	if c == nil {
		return nil, fmt.Errorf("header cache not initialized")
	}
	c.mu.Lock()
	if header, ok := c.headers[blockNumber]; ok {
		c.mu.Unlock()
		return header, nil
	}
	c.mu.Unlock()

	if client == nil {
		client = c.client
	}
	if client == nil {
		return nil, fmt.Errorf("rpc client not initialized")
	}
	callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	header, err := client.HeaderByNumber(callCtx, new(big.Int).SetUint64(blockNumber))
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	if _, exists := c.headers[blockNumber]; !exists {
		if len(c.order) >= rpcPoolLiquidityHeaderCacheSize {
			delete(c.headers, c.order[0])
			c.order = c.order[1:]
		}
		c.headers[blockNumber] = header
		c.order = append(c.order, blockNumber)
	}
	c.mu.Unlock()
	return header, nil
}

func resolveRPCBlockRangeByTime(ctx context.Context, client *ethclient.Client, start time.Time, end time.Time) (uint64, uint64, error) {
	if client == nil {
		return 0, 0, fmt.Errorf("rpc client not initialized")
	}
	latest, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("load latest block: %w", err)
	}
	if latest == nil || latest.Number == nil {
		return 0, 0, fmt.Errorf("latest block header is missing")
	}
	latestNumber := latest.Number.Uint64()
	latestTime := time.Unix(int64(latest.Time), 0).UTC()
	if latestTime.Before(start) {
		return latestNumber + 1, latestNumber, nil
	}
	startBlock, err := firstBlockAtOrAfter(ctx, client, latestNumber, start)
	if err != nil {
		return 0, 0, err
	}
	endBlock, err := lastBlockAtOrBefore(ctx, client, latestNumber, end)
	if err != nil {
		return 0, 0, err
	}
	return startBlock, endBlock, nil
}

func firstBlockAtOrAfter(ctx context.Context, client *ethclient.Client, latest uint64, target time.Time) (uint64, error) {
	var lo uint64
	hi := latest
	for lo < hi {
		mid := lo + (hi-lo)/2
		header, err := client.HeaderByNumber(ctx, new(big.Int).SetUint64(mid))
		if err != nil {
			return 0, fmt.Errorf("load block %d: %w", mid, err)
		}
		if time.Unix(int64(header.Time), 0).Before(target) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, nil
}

func lastBlockAtOrBefore(ctx context.Context, client *ethclient.Client, latest uint64, target time.Time) (uint64, error) {
	var lo uint64
	hi := latest
	for lo < hi {
		mid := lo + (hi-lo+1)/2
		header, err := client.HeaderByNumber(ctx, new(big.Int).SetUint64(mid))
		if err != nil {
			return 0, fmt.Errorf("load block %d: %w", mid, err)
		}
		if time.Unix(int64(header.Time), 0).After(target) {
			hi = mid - 1
		} else {
			lo = mid
		}
	}
	return lo, nil
}

func aggregateRPCLiquidityCandidates(query TokenLiquidityCandidateQuery, since time.Time, till time.Time, events []*models.SmartMoneyLPEvent, excluded int, warnings []string) *TokenLiquidityCandidateResponse {
	type agg struct {
		candidate TokenLiquidityCandidate
		pools     map[string]struct{}
		lastTime  time.Time
	}
	byWallet := make(map[string]*agg)
	for _, event := range events {
		if event == nil {
			excluded++
			continue
		}
		wallet := strings.ToLower(strings.TrimSpace(event.WalletAddress))
		if !isEVMAddress(wallet) {
			excluded++
			continue
		}
		amountUSD := eventTotalUSD(event)
		if amountUSD < query.MinAmountUSD {
			excluded++
			continue
		}
		walletAgg := byWallet[wallet]
		if walletAgg == nil {
			walletAgg = &agg{
				candidate: TokenLiquidityCandidate{
					WalletAddress: wallet,
					TokenAddress:  firstNonEmptyString(event.Token0Address, event.Token1Address),
					PoolAddress:   strings.ToLower(strings.TrimSpace(event.PoolAddress)),
					PoolID:        poolIDForCandidate(event),
					Pair:          buildBitqueryPair(event.Token0Symbol, event.Token1Symbol),
					Protocol:      strings.TrimSpace(event.Protocol),
					Provider:      rpcPoolLiquidityProviderName,
					AmountSource:  "rpc_lp_event_amount_usd",
				},
				pools: make(map[string]struct{}),
			}
			byWallet[wallet] = walletAgg
		}
		if amountUSD > walletAgg.candidate.MaxAmountUSD {
			walletAgg.candidate.MaxAmountUSD = amountUSD
		}
		if event.TxTimestamp.IsZero() || !event.TxTimestamp.Before(walletAgg.lastTime) {
			walletAgg.lastTime = event.TxTimestamp
			walletAgg.candidate.LastAmountUSD = amountUSD
			walletAgg.candidate.TxHash = strings.ToLower(strings.TrimSpace(event.TxHash))
			walletAgg.candidate.TxTime = event.TxTimestamp.UTC().Format(time.RFC3339)
			walletAgg.candidate.PoolAddress = strings.ToLower(strings.TrimSpace(event.PoolAddress))
			walletAgg.candidate.PoolID = poolIDForCandidate(event)
			walletAgg.candidate.Pair = buildBitqueryPair(event.Token0Symbol, event.Token1Symbol)
			walletAgg.candidate.Protocol = strings.TrimSpace(event.Protocol)
		}
		poolKey := strings.ToLower(strings.TrimSpace(event.PoolAddress))
		if poolKey != "" {
			walletAgg.pools[poolKey] = struct{}{}
		}
	}

	candidates := make([]TokenLiquidityCandidate, 0, len(byWallet))
	for _, item := range byWallet {
		item.candidate.PoolCount = len(item.pools)
		candidates = append(candidates, item.candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].MaxAmountUSD != candidates[j].MaxAmountUSD {
			return candidates[i].MaxAmountUSD > candidates[j].MaxAmountUSD
		}
		return strings.Compare(candidates[i].TxTime, candidates[j].TxTime) > 0
	})
	if len(candidates) > query.Limit {
		candidates = candidates[:query.Limit]
	}
	if len(events) == 0 {
		warnings = append(warnings, "rpc returned no add-liquidity events for this pool and window")
	}

	return &TokenLiquidityCandidateResponse{
		Token: TokenLiquidityTokenInfo{
			Address: "",
			Chain:   query.Chain,
		},
		Pool: TokenLiquidityPoolInfo{
			Address: query.PoolAddress,
			PoolID:  query.PoolID,
			Chain:   query.Chain,
		},
		Filters: TokenLiquidityFilterInfo{
			PoolAddress:  query.PoolAddress,
			PoolID:       query.PoolID,
			MinAmountUSD: query.MinAmountUSD,
			WindowHours:  query.WindowHours,
			StartTime:    since.Format(time.RFC3339),
			EndTime:      till.Format(time.RFC3339),
			Limit:        query.Limit,
		},
		Sources: []TokenLiquiditySourceInfo{
			{Name: "rpc", Role: "pool_liquidity_event_reader"},
		},
		Candidates:    candidates,
		ExcludedCount: excluded,
		Warnings:      warnings,
	}
}

func poolLiquidityEmptyResponse(query TokenLiquidityCandidateQuery, since time.Time, till time.Time, warning string) *TokenLiquidityCandidateResponse {
	warnings := []string{}
	if strings.TrimSpace(warning) != "" {
		warnings = append(warnings, strings.TrimSpace(warning))
	}
	return aggregateRPCLiquidityCandidates(query, since, till, []*models.SmartMoneyLPEvent{}, 0, warnings)
}

func eventTotalUSD(event *models.SmartMoneyLPEvent) float64 {
	if event == nil || event.TotalUSD == nil {
		return 0
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(*event.TotalUSD), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value < 0 {
		return -value
	}
	return value
}

func poolIDForCandidate(event *models.SmartMoneyLPEvent) string {
	if event == nil {
		return ""
	}
	pool := strings.ToLower(strings.TrimSpace(event.PoolAddress))
	if isEVMHash(pool) {
		return pool
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func appendLimitedWarning(warnings []string, warning string) []string {
	warning = strings.TrimSpace(warning)
	if warning == "" {
		return warnings
	}
	if len(warnings) >= 5 {
		return warnings
	}
	return append(warnings, warning)
}

type BitqueryTokenLiquidityProvider struct {
	apiURL     string
	apiKey     string
	httpClient *http.Client
}

func NewBitqueryTokenLiquidityProvider(apiURL string, apiKey string) *BitqueryTokenLiquidityProvider {
	return &BitqueryTokenLiquidityProvider{
		apiURL: strings.TrimRight(strings.TrimSpace(apiURL), "/"),
		apiKey: strings.TrimSpace(apiKey),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *BitqueryTokenLiquidityProvider) FindCandidates(ctx context.Context, query TokenLiquidityCandidateQuery) (*TokenLiquidityCandidateResponse, error) {
	query, err := normalizeLegacyTokenLiquidityCandidateQuery(query)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, fmt.Errorf("bitquery provider is nil")
	}
	if strings.TrimSpace(p.apiURL) == "" {
		return nil, fmt.Errorf("BITQUERY_API_URL is not configured")
	}
	if strings.TrimSpace(p.apiKey) == "" {
		return nil, fmt.Errorf("BITQUERY_API_KEY is not configured")
	}
	if query.Chain != "bsc" {
		return nil, fmt.Errorf("bitquery liquidity wallet discovery currently supports bsc only")
	}

	limit := query.Limit * 8
	if limit < 50 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	since, till := resolveTokenLiquidityTimeRange(query)

	events, err := p.queryLiquidityEvents(ctx, query, since, till, limit)
	if err != nil {
		return nil, err
	}
	txHashes := uniqueLiquidityTxHashes(events)
	lpEvents, err := p.queryLiquidityABIEvents(ctx, txHashes)
	if err != nil {
		return nil, err
	}
	events = filterLiquidityEventsByABIEvents(events, lpEvents)
	txHashes = uniqueLiquidityTxHashes(events)
	balances, err := p.queryTransactionBalances(ctx, txHashes, query.TokenAddress)
	if err != nil {
		return nil, err
	}

	resp := aggregateBitqueryCandidates(query, events, balances)
	return resp, nil
}

func resolveTokenLiquidityTimeRange(query TokenLiquidityCandidateQuery) (time.Time, time.Time) {
	if !query.StartTime.IsZero() {
		return query.StartTime.UTC(), query.EndTime.UTC()
	}
	till := time.Now().UTC()
	return till.Add(-time.Duration(query.WindowHours) * time.Hour), till
}

const bitqueryLiquidityEventsQuery = `
query TokenLiquidityEvents($token: String!, $since: DateTime!, $till: DateTime!, $limit: Int!) {
  EVM(network: bsc) {
    currencyA: DEXPoolEvents(
      limit: { count: $limit }
      orderBy: { descending: Block_Time }
      where: {
        Block: { Time: { since: $since, till: $till } }
        PoolEvent: { Pool: { CurrencyA: { SmartContract: { is: $token } } } }
      }
    ) {
      Block { Time Number }
      Transaction { Hash }
      PoolEvent {
        Dex { ProtocolName SmartContract }
        Pool {
          SmartContract
          PoolId
          CurrencyA { Symbol SmartContract }
          CurrencyB { Symbol SmartContract }
        }
        Liquidity { AmountCurrencyA AmountCurrencyB }
      }
    }
    currencyB: DEXPoolEvents(
      limit: { count: $limit }
      orderBy: { descending: Block_Time }
      where: {
        Block: { Time: { since: $since, till: $till } }
        PoolEvent: { Pool: { CurrencyB: { SmartContract: { is: $token } } } }
      }
    ) {
      Block { Time Number }
      Transaction { Hash }
      PoolEvent {
        Dex { ProtocolName SmartContract }
        Pool {
          SmartContract
          PoolId
          CurrencyA { Symbol SmartContract }
          CurrencyB { Symbol SmartContract }
        }
        Liquidity { AmountCurrencyA AmountCurrencyB }
      }
    }
  }
}`

const bitqueryLiquidityABIEventsQuery = `
query TokenLiquidityABIEvents($hashes: [String!], $signatures: [String!]) {
  EVM(network: bsc) {
    Events(
      limit: { count: 1000 }
      where: {
        Transaction: { Hash: { in: $hashes } }
        Log: { Signature: { Name: { in: $signatures } } }
        TransactionStatus: { Success: true }
      }
    ) {
      Block { Time Number }
      Transaction { Hash }
      Log {
        SmartContract
        Signature { Name Parsed Signature }
      }
      Arguments {
        Name
        Type
        Value {
          ... on EVM_ABI_Address_Value_Arg { address }
          ... on EVM_ABI_BigInt_Value_Arg { bigInteger }
          ... on EVM_ABI_Integer_Value_Arg { integer }
          ... on EVM_ABI_Bytes_Value_Arg { hex }
          ... on EVM_ABI_String_Value_Arg { string }
          ... on EVM_ABI_Boolean_Value_Arg { bool }
        }
      }
    }
  }
}`

const bitqueryTransactionBalancesQuery = `
query TokenLiquidityTransactionBalances($hashes: [String!], $token: String!) {
  EVM(network: bsc) {
    TransactionBalances(
      limit: { count: 1000 }
      where: {
        Transaction: { Hash: { in: $hashes } }
        TokenBalance: { Currency: { SmartContract: { is: $token } } }
      }
    ) {
      Block { Time Number }
      Transaction { Hash }
      TokenBalance {
        Address
        PreBalance
        PostBalance
        PostBalanceInUSD
        BalanceChangeReasonCode
        Currency { Symbol SmartContract }
      }
    }
  }
}`

type bitqueryGraphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type bitqueryGraphQLResponse struct {
	Data   bitqueryData       `json:"data"`
	Errors []bitqueryGraphErr `json:"errors"`
}

type bitqueryGraphErr struct {
	Message string `json:"message"`
}

type bitqueryData struct {
	EVM bitqueryEVM `json:"EVM"`
}

type bitqueryEVM struct {
	DEXPoolEvents       []bitqueryDEXPoolEvent       `json:"DEXPoolEvents"`
	CurrencyAEvents     []bitqueryDEXPoolEvent       `json:"currencyA"`
	CurrencyBEvents     []bitqueryDEXPoolEvent       `json:"currencyB"`
	TransactionBalances []bitqueryTransactionBalance `json:"TransactionBalances"`
	Events              []bitqueryEvent              `json:"Events"`
}

type bitqueryDEXPoolEvent struct {
	Block       bitqueryBlock     `json:"Block"`
	Transaction bitqueryTx        `json:"Transaction"`
	PoolEvent   bitqueryPoolEvent `json:"PoolEvent"`
}

type bitqueryBlock struct {
	Time   string `json:"Time"`
	Number uint64 `json:"Number"`
}

type bitqueryTx struct {
	Hash string `json:"Hash"`
}

type bitqueryPoolEvent struct {
	Dex       bitqueryDex       `json:"Dex"`
	Pool      bitqueryPool      `json:"Pool"`
	Liquidity bitqueryLiquidity `json:"Liquidity"`
}

type bitqueryDex struct {
	ProtocolName  string `json:"ProtocolName"`
	SmartContract string `json:"SmartContract"`
}

type bitqueryPool struct {
	SmartContract string           `json:"SmartContract"`
	PoolId        string           `json:"PoolId"`
	CurrencyA     bitqueryCurrency `json:"CurrencyA"`
	CurrencyB     bitqueryCurrency `json:"CurrencyB"`
}

type bitqueryCurrency struct {
	Symbol        string `json:"Symbol"`
	SmartContract string `json:"SmartContract"`
}

type bitqueryLiquidity struct {
	AmountCurrencyA string `json:"AmountCurrencyA"`
	AmountCurrencyB string `json:"AmountCurrencyB"`
}

type bitqueryTransactionBalance struct {
	Block        bitqueryBlock        `json:"Block"`
	Transaction  bitqueryTx           `json:"Transaction"`
	TokenBalance bitqueryTokenBalance `json:"TokenBalance"`
}

type bitqueryTokenBalance struct {
	Address                 string           `json:"Address"`
	PreBalance              string           `json:"PreBalance"`
	PostBalance             string           `json:"PostBalance"`
	PostBalanceInUSD        json.RawMessage  `json:"PostBalanceInUSD"`
	BalanceChangeReasonCode string           `json:"BalanceChangeReasonCode"`
	Currency                bitqueryCurrency `json:"Currency"`
}

type bitqueryEvent struct {
	Block       bitqueryBlock      `json:"Block"`
	Transaction bitqueryTx         `json:"Transaction"`
	Log         bitqueryLog        `json:"Log"`
	Arguments   []bitqueryArgument `json:"Arguments"`
}

type bitqueryLog struct {
	SmartContract string            `json:"SmartContract"`
	Signature     bitquerySignature `json:"Signature"`
}

type bitquerySignature struct {
	Name      string `json:"Name"`
	Parsed    bool   `json:"Parsed"`
	Signature string `json:"Signature"`
}

type bitqueryArgument struct {
	Name  string           `json:"Name"`
	Type  string           `json:"Type"`
	Value bitqueryArgValue `json:"Value"`
}

type bitqueryArgValue struct {
	Address    string `json:"address"`
	BigInteger string `json:"bigInteger"`
	Integer    int64  `json:"integer"`
	Hex        string `json:"hex"`
	String     string `json:"string"`
	Bool       bool   `json:"bool"`
}

func (p *BitqueryTokenLiquidityProvider) queryLiquidityEvents(ctx context.Context, query TokenLiquidityCandidateQuery, since time.Time, till time.Time, limit int) ([]bitqueryDEXPoolEvent, error) {
	var out bitqueryGraphQLResponse
	err := p.postGraphQL(ctx, bitqueryLiquidityEventsQuery, map[string]any{
		"token": strings.ToLower(query.TokenAddress),
		"since": since.Format(time.RFC3339),
		"till":  till.Format(time.RFC3339),
		"limit": limit,
	}, &out)
	if err != nil {
		return nil, err
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("bitquery liquidity events error: %s", joinBitqueryErrors(out.Errors))
	}
	return dedupeBitqueryLiquidityEvents(append(out.Data.EVM.CurrencyAEvents, out.Data.EVM.CurrencyBEvents...)), nil
}

func (p *BitqueryTokenLiquidityProvider) queryLiquidityABIEvents(ctx context.Context, txHashes []string) ([]bitqueryEvent, error) {
	if len(txHashes) == 0 {
		return []bitqueryEvent{}, nil
	}
	var out bitqueryGraphQLResponse
	err := p.postGraphQL(ctx, bitqueryLiquidityABIEventsQuery, map[string]any{
		"hashes":     txHashes,
		"signatures": []string{"Mint", "IncreaseLiquidity", "AddLiquidity"},
	}, &out)
	if err != nil {
		return nil, err
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("bitquery liquidity abi events error: %s", joinBitqueryErrors(out.Errors))
	}
	return out.Data.EVM.Events, nil
}

func (p *BitqueryTokenLiquidityProvider) queryTransactionBalances(ctx context.Context, txHashes []string, tokenAddress string) ([]bitqueryTransactionBalance, error) {
	if len(txHashes) == 0 {
		return []bitqueryTransactionBalance{}, nil
	}
	var out bitqueryGraphQLResponse
	err := p.postGraphQL(ctx, bitqueryTransactionBalancesQuery, map[string]any{
		"hashes": txHashes,
		"token":  strings.ToLower(strings.TrimSpace(tokenAddress)),
	}, &out)
	if err != nil {
		return nil, err
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("bitquery transaction balances error: %s", joinBitqueryErrors(out.Errors))
	}
	return out.Data.EVM.TransactionBalances, nil
}

func (p *BitqueryTokenLiquidityProvider) postGraphQL(ctx context.Context, query string, variables map[string]any, out any) error {
	payload, err := json.Marshal(bitqueryGraphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return fmt.Errorf("encode bitquery request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitquery http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode bitquery response: %w", err)
	}
	return nil
}

func joinBitqueryErrors(errors []bitqueryGraphErr) string {
	parts := make([]string, 0, len(errors))
	for _, err := range errors {
		msg := strings.TrimSpace(err.Message)
		if msg != "" {
			parts = append(parts, msg)
		}
	}
	return strings.Join(parts, "; ")
}

func uniqueLiquidityTxHashes(events []bitqueryDEXPoolEvent) []string {
	seen := make(map[string]struct{}, len(events))
	out := make([]string, 0, len(events))
	for _, event := range events {
		hash := strings.ToLower(strings.TrimSpace(event.Transaction.Hash))
		if hash == "" {
			continue
		}
		if _, ok := seen[hash]; ok {
			continue
		}
		seen[hash] = struct{}{}
		out = append(out, hash)
	}
	return out
}

func dedupeBitqueryLiquidityEvents(events []bitqueryDEXPoolEvent) []bitqueryDEXPoolEvent {
	seen := make(map[string]struct{}, len(events))
	out := make([]bitqueryDEXPoolEvent, 0, len(events))
	for _, event := range events {
		txHash := strings.ToLower(strings.TrimSpace(event.Transaction.Hash))
		pool := strings.ToLower(strings.TrimSpace(event.PoolEvent.Pool.SmartContract))
		if txHash == "" {
			continue
		}
		key := txHash + "|" + pool
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, event)
	}
	return out
}

func filterLiquidityEventsByABIEvents(events []bitqueryDEXPoolEvent, abiEvents []bitqueryEvent) []bitqueryDEXPoolEvent {
	if len(events) == 0 || len(abiEvents) == 0 {
		return []bitqueryDEXPoolEvent{}
	}
	validTx := make(map[string]struct{}, len(abiEvents))
	for _, event := range abiEvents {
		name := strings.ToLower(strings.TrimSpace(event.Log.Signature.Name))
		switch name {
		case "mint", "increaseliquidity", "addliquidity":
			txHash := strings.ToLower(strings.TrimSpace(event.Transaction.Hash))
			if txHash != "" {
				validTx[txHash] = struct{}{}
			}
		}
	}
	out := make([]bitqueryDEXPoolEvent, 0, len(events))
	for _, event := range events {
		txHash := strings.ToLower(strings.TrimSpace(event.Transaction.Hash))
		if _, ok := validTx[txHash]; ok {
			out = append(out, event)
		}
	}
	return out
}

type bitqueryTxBalanceCandidate struct {
	WalletAddress string
	AmountUSD     float64
	Reason        string
}

func aggregateBitqueryCandidates(query TokenLiquidityCandidateQuery, events []bitqueryDEXPoolEvent, balances []bitqueryTransactionBalance) *TokenLiquidityCandidateResponse {
	querySince, queryTill := resolveTokenLiquidityTimeRange(query)
	balancesByTx := make(map[string][]bitqueryTxBalanceCandidate)
	excluded := 0
	for _, balance := range balances {
		txHash := strings.ToLower(strings.TrimSpace(balance.Transaction.Hash))
		wallet := strings.ToLower(strings.TrimSpace(balance.TokenBalance.Address))
		if txHash == "" || !isEVMAddress(wallet) {
			excluded++
			continue
		}
		amountUSD, ok := bitqueryBalanceDeltaUSD(balance.TokenBalance)
		if !ok || amountUSD <= 0 {
			excluded++
			continue
		}
		balancesByTx[txHash] = append(balancesByTx[txHash], bitqueryTxBalanceCandidate{
			WalletAddress: wallet,
			AmountUSD:     amountUSD,
			Reason:        strings.TrimSpace(balance.TokenBalance.BalanceChangeReasonCode),
		})
	}

	type agg struct {
		candidate TokenLiquidityCandidate
		pools     map[string]struct{}
		lastTime  time.Time
	}
	byWallet := make(map[string]*agg)
	warnings := []string{}
	for _, event := range events {
		txHash := strings.ToLower(strings.TrimSpace(event.Transaction.Hash))
		poolAddress := strings.ToLower(strings.TrimSpace(event.PoolEvent.Pool.SmartContract))
		if txHash == "" {
			excluded++
			continue
		}
		eventTime, _ := time.Parse(time.RFC3339, strings.TrimSpace(event.Block.Time))
		pair := buildBitqueryPair(event.PoolEvent.Pool.CurrencyA.Symbol, event.PoolEvent.Pool.CurrencyB.Symbol)
		txBalances := filterBitqueryWalletBalanceCandidates(balancesByTx[txHash], poolAddress)
		if len(txBalances) == 0 {
			excluded++
			continue
		}
		for _, balance := range txBalances {
			if balance.AmountUSD < query.MinAmountUSD {
				excluded++
				continue
			}
			walletAgg := byWallet[balance.WalletAddress]
			if walletAgg == nil {
				walletAgg = &agg{
					candidate: TokenLiquidityCandidate{
						WalletAddress: balance.WalletAddress,
						TokenAddress:  query.TokenAddress,
						Provider:      "bitquery",
						AmountSource:  "bitquery_transaction_balance_delta_usd",
					},
					pools: make(map[string]struct{}),
				}
				byWallet[balance.WalletAddress] = walletAgg
			}
			if balance.AmountUSD > walletAgg.candidate.MaxAmountUSD {
				walletAgg.candidate.MaxAmountUSD = balance.AmountUSD
			}
			if eventTime.IsZero() || !eventTime.Before(walletAgg.lastTime) {
				walletAgg.lastTime = eventTime
				walletAgg.candidate.LastAmountUSD = balance.AmountUSD
				walletAgg.candidate.TxHash = txHash
				walletAgg.candidate.TxTime = strings.TrimSpace(event.Block.Time)
				walletAgg.candidate.PoolAddress = poolAddress
				walletAgg.candidate.Pair = pair
			}
			if poolAddress != "" {
				walletAgg.pools[poolAddress] = struct{}{}
			}
		}
	}

	candidates := make([]TokenLiquidityCandidate, 0, len(byWallet))
	for _, item := range byWallet {
		item.candidate.PoolCount = len(item.pools)
		candidates = append(candidates, item.candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].MaxAmountUSD != candidates[j].MaxAmountUSD {
			return candidates[i].MaxAmountUSD > candidates[j].MaxAmountUSD
		}
		return strings.Compare(candidates[i].TxTime, candidates[j].TxTime) > 0
	})
	if len(candidates) > query.Limit {
		candidates = candidates[:query.Limit]
	}
	if len(events) == 0 {
		warnings = append(warnings, "bitquery returned no liquidity events for this token and window")
	}

	return &TokenLiquidityCandidateResponse{
		Token: TokenLiquidityTokenInfo{
			Address: query.TokenAddress,
			Chain:   query.Chain,
		},
		Filters: TokenLiquidityFilterInfo{
			MinAmountUSD: query.MinAmountUSD,
			WindowHours:  query.WindowHours,
			StartTime:    querySince.Format(time.RFC3339),
			EndTime:      queryTill.Format(time.RFC3339),
			Limit:        query.Limit,
		},
		Sources: []TokenLiquiditySourceInfo{
			{Name: "bitquery", Role: "primary_liquidity_indexer"},
		},
		Candidates:    candidates,
		ExcludedCount: excluded,
		Warnings:      warnings,
	}
}

func parseBitqueryUSD(raw json.RawMessage) (float64, bool) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return 0, false
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number, true
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		str = strings.TrimSpace(str)
		if str == "" {
			return 0, false
		}
		value, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return 0, false
		}
		return value, true
	}
	return 0, false
}

func filterBitqueryWalletBalanceCandidates(items []bitqueryTxBalanceCandidate, poolAddress string) []bitqueryTxBalanceCandidate {
	poolAddress = strings.ToLower(strings.TrimSpace(poolAddress))
	out := make([]bitqueryTxBalanceCandidate, 0, len(items))
	for _, item := range items {
		if item.WalletAddress == "" {
			continue
		}
		if poolAddress != "" && item.WalletAddress == poolAddress {
			continue
		}
		out = append(out, item)
	}
	return out
}

func bitqueryBalanceDeltaUSD(balance bitqueryTokenBalance) (float64, bool) {
	pre, err := strconv.ParseFloat(strings.TrimSpace(balance.PreBalance), 64)
	if err != nil {
		return 0, false
	}
	post, err := strconv.ParseFloat(strings.TrimSpace(balance.PostBalance), 64)
	if err != nil {
		return 0, false
	}
	if pre <= post {
		return 0, false
	}
	postUSD, ok := parseBitqueryUSD(balance.PostBalanceInUSD)
	if !ok || postUSD <= 0 || post <= 0 {
		return 0, false
	}
	unitUSD := postUSD / post
	if unitUSD <= 0 || math.IsNaN(unitUSD) || math.IsInf(unitUSD, 0) {
		return 0, false
	}
	return (pre - post) * unitUSD, true
}

func buildBitqueryPair(left string, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left != "" && right != "" {
		return left + "/" + right
	}
	if left != "" {
		return left
	}
	return right
}
