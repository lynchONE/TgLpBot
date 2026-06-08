package smart_money

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"TgLpBot/base/config"
)

const MonitoredWalletSourceTokenLiquidityIndexer = "token_liquidity_indexer"

type TokenLiquidityCandidateQuery struct {
	Chain        string
	ChainID      int
	TokenAddress string
	MinAmountUSD float64
	WindowHours  int
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
	Pair             string   `json:"pair"`
	PoolCount        int      `json:"pool_count"`
	AmountSource     string   `json:"amount_source"`
	Provider         string   `json:"provider"`
	AlreadyMonitored bool     `json:"already_monitored"`
	ExcludedReasons  []string `json:"excluded_reasons,omitempty"`
}

type TokenLiquidityCandidateResponse struct {
	Token         TokenLiquidityTokenInfo    `json:"token"`
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

type TokenLiquidityFilterInfo struct {
	MinAmountUSD float64 `json:"min_amount_usd"`
	WindowHours  int     `json:"window_hours"`
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
	provider := strings.ToLower(strings.TrimSpace(cfg.SmartMoneyLiquidityIndexProvider))
	if provider == "" {
		return nil, fmt.Errorf("SMART_MONEY_LIQUIDITY_INDEX_PROVIDER is not configured")
	}
	if provider != "bitquery" {
		return nil, fmt.Errorf("unsupported SMART_MONEY_LIQUIDITY_INDEX_PROVIDER: %s", provider)
	}
	if strings.TrimSpace(cfg.BitqueryAPIKey) == "" {
		return nil, fmt.Errorf("BITQUERY_API_KEY is not configured")
	}
	if strings.TrimSpace(cfg.BitqueryAPIURL) == "" {
		return nil, fmt.Errorf("BITQUERY_API_URL is not configured")
	}
	return NewBitqueryTokenLiquidityProvider(cfg.BitqueryAPIURL, cfg.BitqueryAPIKey), nil
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
	if !isEVMAddress(query.TokenAddress) {
		return query, fmt.Errorf("invalid token_address")
	}
	if query.MinAmountUSD <= 0 || math.IsNaN(query.MinAmountUSD) || math.IsInf(query.MinAmountUSD, 0) {
		return query, fmt.Errorf("min_amount_usd must be greater than 0")
	}
	if query.WindowHours <= 0 {
		return query, fmt.Errorf("window_hours must be greater than 0")
	}
	if query.WindowHours > 24*30 {
		return query, fmt.Errorf("window_hours is too large")
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
	query, err := NormalizeTokenLiquidityCandidateQuery(query)
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
	since := time.Now().UTC().Add(-time.Duration(query.WindowHours) * time.Hour)

	events, err := p.queryLiquidityEvents(ctx, query, since, limit)
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

const bitqueryLiquidityEventsQuery = `
query TokenLiquidityEvents($token: String!, $since: DateTime!, $limit: Int!) {
  EVM(network: bsc) {
    currencyA: DEXPoolEvents(
      limit: { count: $limit }
      orderBy: { descending: Block_Time }
      where: {
        Block: { Time: { since: $since } }
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
        Block: { Time: { since: $since } }
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

func (p *BitqueryTokenLiquidityProvider) queryLiquidityEvents(ctx context.Context, query TokenLiquidityCandidateQuery, since time.Time, limit int) ([]bitqueryDEXPoolEvent, error) {
	var out bitqueryGraphQLResponse
	err := p.postGraphQL(ctx, bitqueryLiquidityEventsQuery, map[string]any{
		"token": strings.ToLower(query.TokenAddress),
		"since": since.Format(time.RFC3339),
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
