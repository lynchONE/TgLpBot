package token_metadata

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gorm.io/gorm/clause"
)

const (
	cachePrefix    = "token:meta"
	sourceRPC      = "rpc"
	sourceGecko    = "geckoterminal"
	sourceDex      = "dexscreener"
	sourceTrust    = "trustwallet"
	statusOK       = "ok"
	statusNotFound = "not_found"
	positiveTTL    = 7 * 24 * time.Hour
	negativeTTL    = 6 * time.Hour
	negativeRetry  = 30 * time.Minute
)

type chainMetadataProvider interface {
	GetBatch(ctx context.Context, chain string, addresses []string) (map[string]models.TokenMetadata, error)
}

type displayMetadataProvider interface {
	GetBatch(ctx context.Context, chain string, addresses []string) (map[string]models.TokenMetadata, error)
}

type Service struct {
	chainProvider   chainMetadataProvider
	displayProvider displayMetadataProvider
}

var readCacheBatchFn = readCacheBatch

type cacheEntry struct {
	Chain        string    `json:"chain"`
	TokenAddress string    `json:"token_address"`
	Symbol       string    `json:"symbol,omitempty"`
	Name         string    `json:"name,omitempty"`
	LogoURL      string    `json:"logo_url,omitempty"`
	Source       string    `json:"source,omitempty"`
	Status       string    `json:"status"`
	FetchedAt    time.Time `json:"fetched_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func NewService() *Service {
	displayClient := &http.Client{Timeout: 12 * time.Second}
	return &Service{
		chainProvider: evmChainMetadataProvider{},
		displayProvider: chainedDisplayMetadataProvider{providers: []displayMetadataProvider{
			geckoDisplayMetadataProvider{client: displayClient},
			dexScreenerDisplayMetadataProvider{client: displayClient},
			trustWalletDisplayMetadataProvider{client: displayClient},
		}},
	}
}

func NewServiceWithProviders(chainProvider chainMetadataProvider, displayProvider displayMetadataProvider) *Service {
	return &Service{chainProvider: chainProvider, displayProvider: displayProvider}
}

func NormalizeTokenAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !common.IsHexAddress(raw) {
		return ""
	}
	return strings.ToLower(common.HexToAddress(raw).Hex())
}

func normalizeAddresses(addresses []string) []string {
	seen := make(map[string]struct{}, len(addresses))
	out := make([]string, 0, len(addresses))
	for _, raw := range addresses {
		addr := NormalizeTokenAddress(raw)
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	sort.Strings(out)
	return out
}

func cacheKey(chain string, tokenAddress string) string {
	return fmt.Sprintf("%s:%s:%s", cachePrefix, config.NormalizeChain(chain), NormalizeTokenAddress(tokenAddress))
}

func modelFromCache(entry cacheEntry) models.TokenMetadata {
	return models.TokenMetadata{
		Chain:        config.NormalizeChain(entry.Chain),
		TokenAddress: NormalizeTokenAddress(entry.TokenAddress),
		Symbol:       strings.TrimSpace(entry.Symbol),
		Name:         strings.TrimSpace(entry.Name),
		LogoURL:      strings.TrimSpace(entry.LogoURL),
		Source:       strings.TrimSpace(entry.Source),
		Status:       strings.TrimSpace(entry.Status),
		FetchedAt:    entry.FetchedAt,
		ExpiresAt:    entry.ExpiresAt,
	}
}

func cacheFromModel(meta models.TokenMetadata) cacheEntry {
	return cacheEntry{
		Chain:        config.NormalizeChain(meta.Chain),
		TokenAddress: NormalizeTokenAddress(meta.TokenAddress),
		Symbol:       strings.TrimSpace(meta.Symbol),
		Name:         strings.TrimSpace(meta.Name),
		LogoURL:      strings.TrimSpace(meta.LogoURL),
		Source:       strings.TrimSpace(meta.Source),
		Status:       strings.TrimSpace(meta.Status),
		FetchedAt:    meta.FetchedAt,
		ExpiresAt:    meta.ExpiresAt,
	}
}

func shouldRefreshMetadata(meta models.TokenMetadata) bool {
	status := strings.ToLower(strings.TrimSpace(meta.Status))
	switch status {
	case statusOK:
		return strings.TrimSpace(meta.LogoURL) == ""
	case statusNotFound:
		if meta.FetchedAt.IsZero() {
			return true
		}
		return time.Since(meta.FetchedAt) >= negativeRetry
	default:
		return true
	}
}

func (s *Service) GetBatch(ctx context.Context, chain string, addresses []string) (map[string]models.TokenMetadata, error) {
	chain = config.NormalizeChain(chain)
	list := normalizeAddresses(addresses)
	out := make(map[string]models.TokenMetadata, len(list))
	if len(list) == 0 {
		return out, nil
	}

	pending := make(map[string]struct{}, len(list))
	for _, addr := range list {
		pending[addr] = struct{}{}
	}

	if cached, err := readCacheBatchFn(chain, list); err != nil {
		log.Printf("[TokenMetadata] warning: redis batch read failed chain=%s err=%v", chain, err)
	} else {
		for addr, meta := range cached {
			if strings.EqualFold(meta.Status, statusOK) {
				out[addr] = meta
			}
			if !shouldRefreshMetadata(meta) {
				delete(pending, addr)
			}
		}
	}

	if len(pending) > 0 && database.DB != nil {
		dbRows, err := loadDBBatch(ctx, chain, mapKeys(pending))
		if err != nil {
			log.Printf("[TokenMetadata] warning: db batch load failed chain=%s err=%v", chain, err)
		} else {
			now := time.Now()
			for _, row := range dbRows {
				addr := NormalizeTokenAddress(row.TokenAddress)
				if addr == "" {
					continue
				}
				if strings.EqualFold(strings.TrimSpace(row.Status), statusOK) {
					out[addr] = row
				}
				if row.ExpiresAt.After(now) && !shouldRefreshMetadata(row) {
					delete(pending, addr)
					writeCache(chain, row)
					continue
				}
				// Keep stale positive data as a fallback while attempting a refresh.
				if row.Status == "" {
					row.Status = statusOK
				}
			}
		}
	}

	if len(pending) == 0 || s == nil || s.chainProvider == nil {
		return out, nil
	}

	fetched, err := s.fetchFreshMetadata(ctx, chain, mapKeys(pending))
	if err != nil {
		if len(out) > 0 {
			log.Printf("[TokenMetadata] warning: metadata refresh failed chain=%s pending=%d cached=%d err=%v", chain, len(pending), len(out), err)
			return out, nil
		}
		return out, err
	}

	now := time.Now()
	rowsToSave := make([]models.TokenMetadata, 0, len(pending))
	for _, addr := range mapKeys(pending) {
		meta, ok := fetched[addr]
		if ok {
			meta.Chain = chain
			meta.TokenAddress = addr
			if strings.TrimSpace(meta.Source) == "" {
				meta.Source = sourceRPC
			}
			meta.Status = statusOK
			meta.FetchedAt = now
			meta.ExpiresAt = now.Add(positiveTTL)
			out[addr] = meta
		} else {
			meta = models.TokenMetadata{
				Chain:        chain,
				TokenAddress: addr,
				Source:       sourceRPC,
				Status:       statusNotFound,
				FetchedAt:    now,
				ExpiresAt:    now.Add(negativeTTL),
			}
		}
		rowsToSave = append(rowsToSave, meta)
		writeCache(chain, meta)
	}

	if len(rowsToSave) > 0 {
		if err := upsertDBBatch(ctx, rowsToSave); err != nil {
			log.Printf("[TokenMetadata] warning: db batch upsert failed chain=%s err=%v", chain, err)
		}
	}

	return out, nil
}

func (s *Service) fetchFreshMetadata(ctx context.Context, chain string, addresses []string) (map[string]models.TokenMetadata, error) {
	if s == nil || s.chainProvider == nil {
		return map[string]models.TokenMetadata{}, nil
	}
	addresses = normalizeAddresses(addresses)
	if len(addresses) == 0 {
		return map[string]models.TokenMetadata{}, nil
	}
	out, err := s.chainProvider.GetBatch(ctx, chain, addresses)
	if err != nil {
		return nil, err
	}
	if s.displayProvider == nil {
		return out, nil
	}
	display, err := s.displayProvider.GetBatch(ctx, chain, addresses)
	if err != nil {
		log.Printf("[TokenMetadata] warning: display metadata refresh failed chain=%s err=%v", chain, err)
		return out, nil
	}
	for addr, extra := range display {
		meta, ok := out[addr]
		if !ok {
			continue
		}
		if strings.TrimSpace(meta.Symbol) == "" {
			meta.Symbol = strings.TrimSpace(extra.Symbol)
		}
		if strings.TrimSpace(meta.Name) == "" {
			meta.Name = strings.TrimSpace(extra.Name)
		}
		if strings.TrimSpace(extra.LogoURL) != "" {
			meta.LogoURL = strings.TrimSpace(extra.LogoURL)
			meta.Source = mergeMetadataSource(meta.Source, extra.Source)
		}
		out[addr] = meta
	}
	return out, nil
}

func mergeMetadataSource(primary string, display string) string {
	primary = strings.TrimSpace(primary)
	display = strings.TrimSpace(display)
	if primary == "" {
		primary = sourceRPC
	}
	if display == "" || strings.Contains(primary, display) {
		return primary
	}
	return primary + "+" + display
}

type evmChainMetadataProvider struct{}

func (evmChainMetadataProvider) GetBatch(ctx context.Context, chain string, addresses []string) (map[string]models.TokenMetadata, error) {
	chain = config.NormalizeChain(chain)
	exec, err := chainexec.GetEVM(chain)
	if err != nil {
		return nil, err
	}
	client := exec.Client()
	if client == nil {
		return nil, fmt.Errorf("rpc client unavailable for chain %s", chain)
	}

	out := make(map[string]models.TokenMetadata, len(addresses))
	var firstErr error
	for _, addr := range normalizeAddresses(addresses) {
		token := common.HexToAddress(addr)
		erc20, err := blockchain.NewERC20(token, client)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		symbol, symErr := erc20.Symbol(nil)
		name, nameErr := erc20.Name(nil)
		_, decErr := erc20.Decimals(nil)
		if symErr != nil || nameErr != nil || decErr != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("read token metadata failed token=%s symbol=%v name=%v decimals=%v", addr, symErr, nameErr, decErr)
			}
			continue
		}

		out[addr] = models.TokenMetadata{
			Chain:        chain,
			TokenAddress: addr,
			Symbol:       strings.TrimSpace(symbol),
			Name:         strings.TrimSpace(name),
			Source:       sourceRPC,
			Status:       statusOK,
		}
	}
	if len(out) == 0 && firstErr != nil {
		return out, firstErr
	}
	return out, nil
}

type geckoDisplayMetadataProvider struct {
	client *http.Client
}

type chainedDisplayMetadataProvider struct {
	providers []displayMetadataProvider
}

func (p chainedDisplayMetadataProvider) GetBatch(ctx context.Context, chain string, addresses []string) (map[string]models.TokenMetadata, error) {
	addresses = normalizeAddresses(addresses)
	out := make(map[string]models.TokenMetadata, len(addresses))
	if len(addresses) == 0 {
		return out, nil
	}

	missing := make(map[string]struct{}, len(addresses))
	for _, addr := range addresses {
		missing[addr] = struct{}{}
	}
	var firstErr error
	for _, provider := range p.providers {
		if provider == nil || len(missing) == 0 {
			continue
		}
		fetched, err := provider.GetBatch(ctx, chain, mapKeys(missing))
		if err != nil && firstErr == nil {
			firstErr = err
		}
		for addr, meta := range fetched {
			addr = NormalizeTokenAddress(addr)
			if addr == "" || strings.TrimSpace(meta.LogoURL) == "" {
				continue
			}
			out[addr] = meta
			delete(missing, addr)
		}
	}
	if len(out) == 0 && firstErr != nil {
		return out, firstErr
	}
	return out, nil
}

type geckoTokenMetadataResponse struct {
	Data struct {
		Attributes struct {
			Address  string `json:"address"`
			Name     string `json:"name"`
			Symbol   string `json:"symbol"`
			ImageURL string `json:"image_url"`
		} `json:"attributes"`
	} `json:"data"`
}

func (p geckoDisplayMetadataProvider) GetBatch(ctx context.Context, chain string, addresses []string) (map[string]models.TokenMetadata, error) {
	addresses = normalizeAddresses(addresses)
	out := make(map[string]models.TokenMetadata, len(addresses))
	if len(addresses) == 0 {
		return out, nil
	}
	client := p.client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	network := geckoNetwork(chain)
	var firstErr error
	for _, addr := range addresses {
		meta, err := p.fetchOne(ctx, client, network, chain, addr)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if meta.TokenAddress != "" {
			out[meta.TokenAddress] = meta
		}
	}
	if len(out) == 0 && firstErr != nil {
		return out, firstErr
	}
	return out, nil
}

func (p geckoDisplayMetadataProvider) fetchOne(ctx context.Context, client *http.Client, network string, chain string, addr string) (models.TokenMetadata, error) {
	endpoint := fmt.Sprintf(
		"https://api.geckoterminal.com/api/v2/networks/%s/tokens/%s",
		url.PathEscape(network),
		url.PathEscape(addr),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return models.TokenMetadata{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return models.TokenMetadata{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return models.TokenMetadata{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return models.TokenMetadata{}, fmt.Errorf("geckoterminal token metadata http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed geckoTokenMetadataResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return models.TokenMetadata{}, err
	}
	gotAddr := NormalizeTokenAddress(parsed.Data.Attributes.Address)
	if gotAddr == "" {
		gotAddr = NormalizeTokenAddress(addr)
	}
	return models.TokenMetadata{
		Chain:        config.NormalizeChain(chain),
		TokenAddress: gotAddr,
		Symbol:       strings.TrimSpace(parsed.Data.Attributes.Symbol),
		Name:         strings.TrimSpace(parsed.Data.Attributes.Name),
		LogoURL:      strings.TrimSpace(parsed.Data.Attributes.ImageURL),
		Source:       sourceGecko,
		Status:       statusOK,
	}, nil
}

type dexScreenerDisplayMetadataProvider struct {
	client *http.Client
}

type dexScreenerTokenResponse struct {
	Pairs []dexScreenerTokenPair `json:"pairs"`
}

type dexScreenerTokenPair struct {
	BaseToken struct {
		Address string `json:"address"`
		Name    string `json:"name"`
		Symbol  string `json:"symbol"`
	} `json:"baseToken"`
	QuoteToken struct {
		Address string `json:"address"`
		Name    string `json:"name"`
		Symbol  string `json:"symbol"`
	} `json:"quoteToken"`
	Info struct {
		ImageURL string `json:"imageUrl"`
	} `json:"info"`
	Liquidity struct {
		USD float64 `json:"usd"`
	} `json:"liquidity"`
}

const dexScreenerBatchSize = 30

func (p dexScreenerDisplayMetadataProvider) GetBatch(ctx context.Context, chain string, addresses []string) (map[string]models.TokenMetadata, error) {
	addresses = normalizeAddresses(addresses)
	out := make(map[string]models.TokenMetadata, len(addresses))
	if len(addresses) == 0 {
		return out, nil
	}
	chainID := dexScreenerChainID(chain)
	if chainID == "" {
		return out, fmt.Errorf("unsupported dexscreener chain: %s", chain)
	}
	client := p.client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}

	var firstErr error
	for start := 0; start < len(addresses); start += dexScreenerBatchSize {
		end := start + dexScreenerBatchSize
		if end > len(addresses) {
			end = len(addresses)
		}
		part, err := p.fetchBatch(ctx, client, chainID, config.NormalizeChain(chain), addresses[start:end])
		if err != nil && firstErr == nil {
			firstErr = err
		}
		for addr, meta := range part {
			out[addr] = meta
		}
	}
	if len(out) == 0 && firstErr != nil {
		return out, firstErr
	}
	return out, nil
}

func (p dexScreenerDisplayMetadataProvider) fetchBatch(ctx context.Context, client *http.Client, chainID string, chain string, addresses []string) (map[string]models.TokenMetadata, error) {
	addresses = normalizeAddresses(addresses)
	out := make(map[string]models.TokenMetadata, len(addresses))
	if len(addresses) == 0 {
		return out, nil
	}
	lookup := make(map[string]struct{}, len(addresses))
	for _, addr := range addresses {
		lookup[addr] = struct{}{}
	}
	endpoint := fmt.Sprintf(
		"https://api.dexscreener.com/tokens/v1/%s/%s",
		url.PathEscape(chainID),
		url.PathEscape(strings.Join(addresses, ",")),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("dexscreener token metadata http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var pairs []dexScreenerTokenPair
	if err := json.Unmarshal(body, &pairs); err != nil {
		var envelope dexScreenerTokenResponse
		if envErr := json.Unmarshal(body, &envelope); envErr != nil {
			return nil, err
		}
		pairs = envelope.Pairs
	}

	bestLiquidity := make(map[string]float64, len(addresses))
	for _, pair := range pairs {
		imageURL := strings.TrimSpace(pair.Info.ImageURL)
		if imageURL == "" {
			continue
		}
		addr := NormalizeTokenAddress(pair.BaseToken.Address)
		if addr == "" {
			continue
		}
		if _, ok := lookup[addr]; !ok {
			continue
		}
		if existing, ok := bestLiquidity[addr]; ok && existing > pair.Liquidity.USD {
			continue
		}
		bestLiquidity[addr] = pair.Liquidity.USD
		out[addr] = models.TokenMetadata{
			Chain:        chain,
			TokenAddress: addr,
			Symbol:       strings.TrimSpace(pair.BaseToken.Symbol),
			Name:         strings.TrimSpace(pair.BaseToken.Name),
			LogoURL:      imageURL,
			Source:       sourceDex,
			Status:       statusOK,
		}
	}
	return out, nil
}

func dexScreenerChainID(chain string) string {
	switch strings.ToLower(strings.TrimSpace(chain)) {
	case "", "bsc", "bnb":
		return "bsc"
	case "base":
		return "base"
	case "eth", "ethereum":
		return "ethereum"
	default:
		return strings.ToLower(strings.TrimSpace(chain))
	}
}

type trustWalletDisplayMetadataProvider struct {
	client *http.Client
}

func (p trustWalletDisplayMetadataProvider) GetBatch(ctx context.Context, chain string, addresses []string) (map[string]models.TokenMetadata, error) {
	addresses = normalizeAddresses(addresses)
	out := make(map[string]models.TokenMetadata, len(addresses))
	if len(addresses) == 0 {
		return out, nil
	}
	assetChain := trustWalletAssetChain(chain)
	if assetChain == "" {
		return out, fmt.Errorf("unsupported trustwallet chain: %s", chain)
	}
	client := p.client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}

	var firstErr error
	for _, addr := range addresses {
		meta, err := p.fetchOne(ctx, client, assetChain, config.NormalizeChain(chain), addr)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out[addr] = meta
	}
	if len(out) == 0 && firstErr != nil {
		return out, firstErr
	}
	return out, nil
}

func (p trustWalletDisplayMetadataProvider) fetchOne(ctx context.Context, client *http.Client, assetChain string, chain string, addr string) (models.TokenMetadata, error) {
	checksummed := common.HexToAddress(addr).Hex()
	logoURL := fmt.Sprintf(
		"https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/%s/assets/%s/logo.png",
		url.PathEscape(assetChain),
		url.PathEscape(checksummed),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, logoURL, nil)
	if err != nil {
		return models.TokenMetadata{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return models.TokenMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return models.TokenMetadata{}, fmt.Errorf("trustwallet token logo http %d", resp.StatusCode)
	}
	return models.TokenMetadata{
		Chain:        chain,
		TokenAddress: NormalizeTokenAddress(addr),
		LogoURL:      logoURL,
		Source:       sourceTrust,
		Status:       statusOK,
	}, nil
}

func trustWalletAssetChain(chain string) string {
	switch strings.ToLower(strings.TrimSpace(chain)) {
	case "", "bsc", "bnb":
		return "smartchain"
	case "base":
		return "base"
	case "eth", "ethereum":
		return "ethereum"
	default:
		return ""
	}
}

func geckoNetwork(chain string) string {
	switch strings.ToLower(strings.TrimSpace(chain)) {
	case "", "bsc", "bnb":
		return "bsc"
	case "base":
		return "base"
	case "eth", "ethereum":
		return "eth"
	default:
		return strings.ToLower(strings.TrimSpace(chain))
	}
}

func readCacheBatch(chain string, addresses []string) (map[string]models.TokenMetadata, error) {
	out := make(map[string]models.TokenMetadata, len(addresses))
	if database.RedisClient == nil {
		return out, nil
	}

	keys := make([]string, 0, len(addresses))
	for _, addr := range normalizeAddresses(addresses) {
		keys = append(keys, cacheKey(chain, addr))
	}
	if len(keys) == 0 {
		return out, nil
	}

	values, err := database.RedisClient.MGet(context.Background(), keys...).Result()
	if err != nil {
		return nil, err
	}

	for idx, raw := range values {
		if raw == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprintf("%v", raw))
		if text == "" {
			continue
		}
		var entry cacheEntry
		if err := json.Unmarshal([]byte(text), &entry); err != nil {
			log.Printf("[TokenMetadata] warning: redis unmarshal failed key=%s err=%v", keys[idx], err)
			_ = database.DeleteCache(keys[idx])
			continue
		}
		meta := modelFromCache(entry)
		if meta.TokenAddress == "" {
			_ = database.DeleteCache(keys[idx])
			continue
		}
		out[meta.TokenAddress] = meta
	}
	return out, nil
}

func writeCache(chain string, meta models.TokenMetadata) {
	if database.RedisClient == nil {
		return
	}
	ttl := time.Until(meta.ExpiresAt)
	if ttl <= 0 {
		return
	}
	entry := cacheFromModel(meta)
	b, err := json.Marshal(entry)
	if err != nil {
		log.Printf("[TokenMetadata] warning: redis marshal failed chain=%s token=%s err=%v", chain, meta.TokenAddress, err)
		return
	}
	if err := database.SetCache(cacheKey(chain, meta.TokenAddress), string(b), ttl); err != nil {
		log.Printf("[TokenMetadata] warning: redis set failed chain=%s token=%s err=%v", chain, meta.TokenAddress, err)
	}
}

func loadDBBatch(ctx context.Context, chain string, addresses []string) ([]models.TokenMetadata, error) {
	if database.DB == nil {
		return nil, nil
	}
	var rows []models.TokenMetadata
	err := database.DB.WithContext(ctx).
		Where("chain = ? AND token_address IN ?", config.NormalizeChain(chain), normalizeAddresses(addresses)).
		Find(&rows).Error
	return rows, err
}

func upsertDBBatch(ctx context.Context, rows []models.TokenMetadata) error {
	if database.DB == nil || len(rows) == 0 {
		return nil
	}
	return database.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "chain"},
				{Name: "token_address"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"symbol",
				"name",
				"logo_url",
				"source",
				"status",
				"fetched_at",
				"expires_at",
			}),
		}).
		Create(&rows).Error
}

func mapKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
