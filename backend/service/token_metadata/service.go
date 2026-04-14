package token_metadata

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/exchange"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gorm.io/gorm/clause"
)

const (
	cachePrefix    = "token:meta"
	sourceOKX      = "okx"
	statusOK       = "ok"
	statusNotFound = "not_found"
	positiveTTL    = 7 * 24 * time.Hour
	negativeTTL    = 6 * time.Hour
	negativeRetry  = 30 * time.Minute
)

type okxMarketClient interface {
	GetMarketTokenBasicInfos(reqs []exchange.MarketTokenBasicInfoRequest) (*exchange.MarketTokenBasicInfoResponse, error)
}

type Service struct {
	okx okxMarketClient
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
	if config.AppConfig == nil {
		return &Service{}
	}
	return &Service{okx: exchange.NewOKXDexService()}
}

func NewServiceWithClient(okx okxMarketClient) *Service {
	return &Service{okx: okx}
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

	if len(pending) == 0 || s == nil || s.okx == nil {
		return out, nil
	}

	fetched, err := s.fetchFromOKX(chain, mapKeys(pending))
	if err != nil {
		if len(out) > 0 {
			log.Printf("[TokenMetadata] warning: okx refresh failed chain=%s pending=%d cached=%d err=%v", chain, len(pending), len(out), err)
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
			meta.Source = sourceOKX
			meta.Status = statusOK
			meta.FetchedAt = now
			meta.ExpiresAt = now.Add(positiveTTL)
			out[addr] = meta
		} else {
			meta = models.TokenMetadata{
				Chain:        chain,
				TokenAddress: addr,
				Source:       sourceOKX,
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

func (s *Service) fetchFromOKX(chain string, addresses []string) (map[string]models.TokenMetadata, error) {
	if s == nil || s.okx == nil {
		return map[string]models.TokenMetadata{}, nil
	}
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok || cc.ChainID <= 0 {
		return nil, fmt.Errorf("invalid chain config: %s", chain)
	}

	reqs := make([]exchange.MarketTokenBasicInfoRequest, 0, len(addresses))
	chainIndex := fmt.Sprintf("%d", cc.ChainID)
	for _, addr := range normalizeAddresses(addresses) {
		reqs = append(reqs, exchange.MarketTokenBasicInfoRequest{
			ChainIndex:           chainIndex,
			TokenContractAddress: addr,
		})
	}
	if len(reqs) == 0 {
		return map[string]models.TokenMetadata{}, nil
	}

	resp, err := s.okx.GetMarketTokenBasicInfos(reqs)
	if err != nil {
		return nil, err
	}

	out := make(map[string]models.TokenMetadata, len(resp.Data))
	for _, item := range resp.Data {
		addr := NormalizeTokenAddress(item.TokenContractAddress)
		if addr == "" {
			continue
		}
		out[addr] = models.TokenMetadata{
			Chain:        chain,
			TokenAddress: addr,
			Symbol:       strings.TrimSpace(item.TokenSymbol),
			Name:         strings.TrimSpace(item.TokenName),
			LogoURL:      strings.TrimSpace(item.TokenLogoURL),
			Source:       sourceOKX,
			Status:       statusOK,
		}
	}
	return out, nil
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
