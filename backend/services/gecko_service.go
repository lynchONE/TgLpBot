package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type GeckoService struct {
	ClickHouse *ClickHouseService
	Client     *http.Client

	feeCache   map[string]float64
	feeCacheMu sync.RWMutex
}

func NewGeckoService(ch *ClickHouseService) *GeckoService {
	return &GeckoService{
		ClickHouse: ch,
		Client:     &http.Client{Timeout: 30 * time.Second},
		feeCache:   make(map[string]float64),
	}
}

// JSON Structs
type GeckoPoolsResponse struct {
	Data []GeckoPoolData `json:"data"`
}

type GeckoPoolData struct {
	ID            string              `json:"id"`
	Type          string              `json:"type"`
	Attributes    GeckoPoolAttributes `json:"attributes"`
	Relationships GeckoRelationships  `json:"relationships"`
}

type GeckoPoolAttributes struct {
	Address               string      `json:"address"`
	Name                  string      `json:"name"`
	PoolFeePercentage     json.Number `json:"pool_fee_percentage"`
	BaseTokenPriceUSD     json.Number `json:"base_token_price_usd"`
	QuoteTokenPriceUSD    json.Number `json:"quote_token_price_usd"`
	BaseTokenPriceNative  json.Number `json:"base_token_price_native_currency"`
	QuoteTokenPriceNative json.Number `json:"quote_token_price_native_currency"`
	BaseTokenPriceQuote   json.Number `json:"base_token_price_quote_token"`
	QuoteTokenPriceBase   json.Number `json:"quote_token_price_base_token"`
	PoolCreatedAt         string      `json:"pool_created_at"`
	FDVUSD                json.Number `json:"fdv_usd"`
	MarketCapUSD          json.Number `json:"market_cap_usd"`
	ReserveInUSD          json.Number `json:"reserve_in_usd"`
	VolumeUSD             VolumeStats `json:"volume_usd"`
	PriceChangePercentage PriceStats  `json:"price_change_percentage"`
	Transactions          TxStats     `json:"transactions"`
}

type VolumeStats struct {
	M5  json.Number `json:"m5"`
	H1  json.Number `json:"h1"`
	H6  json.Number `json:"h6"`
	H24 json.Number `json:"h24"`
}

type PriceStats struct {
	M5  json.Number `json:"m5"`
	H1  json.Number `json:"h1"`
	H6  json.Number `json:"h6"`
	H24 json.Number `json:"h24"`
}

type TxStats struct {
	H24 TxDetail `json:"h24"`
}

type TxDetail struct {
	Buys    int `json:"buys"`
	Sells   int `json:"sells"`
	Buyers  int `json:"buyers"`
	Sellers int `json:"sellers"`
}

type GeckoRelationships struct {
	BaseToken  GeckoTokenRel `json:"base_token"`
	QuoteToken GeckoTokenRel `json:"quote_token"`
	Dex        GeckoDexRel   `json:"dex"`
}

type GeckoTokenRel struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

type GeckoDexRel struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

var poolFeeFromNameRegex = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*%\s*$`)

var defaultDexPoolFeePercentage = map[string]float64{
	// Best-effort defaults when a pool fee can't be resolved from the pool itself.
	// Note: For some V2 forks, part of the swap fee may go to protocol treasury/burn.
	"pancakeswap_v2": 0.25,
	"uniswap_v2":     0.30,
}

func (s *GeckoService) FetchAndStore(network string, page int) error {
	// Usually: https://api.geckoterminal.com/api/v2/networks/{network}/pools?page={page}&page_limit=100
	if network == "" {
		network = "bsc" // Default to BSC
	}

	// GeckoTerminal uses 1-based indexing for pages
	if page < 1 {
		page = 1
	}

	url := fmt.Sprintf("https://api.geckoterminal.com/api/v2/networks/%s/pools?page=%d&page_limit=10", network, page)
	resp, err := s.Client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api error: %s", string(body))
	}

	var result GeckoPoolsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	return s.BatchInsert(result.Data)
}

func (s *GeckoService) resolvePoolFeePercentage(network string, p GeckoPoolData) float64 {
	addr := strings.ToLower(strings.TrimSpace(p.Attributes.Address))
	if addr == "" {
		return 0
	}

	s.feeCacheMu.RLock()
	if v, ok := s.feeCache[addr]; ok {
		s.feeCacheMu.RUnlock()
		return v
	}
	s.feeCacheMu.RUnlock()

	// 1) From Gecko (if present)
	if p.Attributes.PoolFeePercentage != "" {
		if fee, err := p.Attributes.PoolFeePercentage.Float64(); err == nil && fee > 0 {
			s.feeCacheMu.Lock()
			s.feeCache[addr] = fee
			s.feeCacheMu.Unlock()
			return fee
		}
	}

	// 2) Parse from pool name suffix like: "USDT / WBNB 0.01%"
	if m := poolFeeFromNameRegex.FindStringSubmatch(p.Attributes.Name); len(m) == 2 {
		if fee, err := strconv.ParseFloat(m[1], 64); err == nil && fee > 0 {
			s.feeCacheMu.Lock()
			s.feeCache[addr] = fee
			s.feeCacheMu.Unlock()
			return fee
		}
	}

	// 3) Dex default fallback (when the pool type doesn't expose a per-pool fee tier)
	dexID := strings.ToLower(strings.TrimSpace(p.Relationships.Dex.Data.ID))
	if dexID != "" {
		if fee, ok := defaultDexPoolFeePercentage[dexID]; ok && fee > 0 {
			s.feeCacheMu.Lock()
			s.feeCache[addr] = fee
			s.feeCacheMu.Unlock()
			return fee
		}
	}

	// 无法解析手续费，返回 0
	return 0
}

func (s *GeckoService) BatchInsert(pools []GeckoPoolData) error {
	ctx := context.Background()
	batch, err := s.ClickHouse.PrepareBatch(ctx, `INSERT INTO pools (
		id, type, address, name,
		base_token_id, quote_token_id, dex_id,
		base_token_price_usd, quote_token_price_usd,
		base_token_price_native_currency, quote_token_price_native_currency,
		base_token_price_quote_token, quote_token_price_base_token,
		pool_created_at,
		fdv_usd, market_cap_usd, reserve_in_usd,
		price_change_m5, price_change_h1, price_change_h6, price_change_h24,
		volume_m5, volume_h1, volume_h6, volume_h24,
		pool_fee_percentage,
		fee_usd_m5, fee_usd_h1, fee_usd_h6, fee_usd_h24,
		fee_apr_m5, fee_apr_h1, fee_apr_h6, fee_apr_h24,
		transactions_h24_buys, transactions_h24_sells, transactions_h24_buyers, transactions_h24_sellers,
		updated_at
	)`)
	if err != nil {
		return err
	}

	for _, p := range pools {
		attr := p.Attributes

		// Helper to parseFloat
		toFloat := func(n json.Number) float64 {
			f, _ := n.Float64()
			return f
		}

		createdAt, _ := time.Parse(time.RFC3339, attr.PoolCreatedAt)

		cleanName := strings.TrimSpace(poolFeeFromNameRegex.ReplaceAllString(attr.Name, ""))
		if cleanName == "" {
			cleanName = strings.TrimSpace(attr.Name)
		}

		reserveUSD := toFloat(attr.ReserveInUSD)
		feePercent := s.resolvePoolFeePercentage("bsc", p)
		feeRate := feePercent / 100.0

		feeUSDm5 := toFloat(attr.VolumeUSD.M5) * feeRate
		feeUSDh1 := toFloat(attr.VolumeUSD.H1) * feeRate
		feeUSDh6 := toFloat(attr.VolumeUSD.H6) * feeRate
		feeUSDh24 := toFloat(attr.VolumeUSD.H24) * feeRate

		feeAPRm5 := 0.0
		feeAPRh1 := 0.0
		feeAPRh6 := 0.0
		feeAPRh24 := 0.0
		if reserveUSD > 0 {
			feeAPRm5 = (feeUSDm5 / reserveUSD) * (365.0 * 24.0 * 60.0 / 5.0) * 100.0
			feeAPRh1 = (feeUSDh1 / reserveUSD) * (365.0 * 24.0) * 100.0
			feeAPRh6 = (feeUSDh6 / reserveUSD) * (365.0 * 24.0 / 6.0) * 100.0
			feeAPRh24 = (feeUSDh24 / reserveUSD) * 365.0 * 100.0
		}

		if err := batch.Append(
			p.ID,
			p.Type,
			attr.Address,
			cleanName,
			p.Relationships.BaseToken.Data.ID,
			p.Relationships.QuoteToken.Data.ID,
			p.Relationships.Dex.Data.ID,
			toFloat(attr.BaseTokenPriceUSD),
			toFloat(attr.QuoteTokenPriceUSD),
			toFloat(attr.BaseTokenPriceNative),
			toFloat(attr.QuoteTokenPriceNative),
			toFloat(attr.BaseTokenPriceQuote),
			toFloat(attr.QuoteTokenPriceBase),
			createdAt,
			toFloat(attr.FDVUSD),
			toFloat(attr.MarketCapUSD),
			reserveUSD,
			toFloat(attr.PriceChangePercentage.M5),
			toFloat(attr.PriceChangePercentage.H1),
			toFloat(attr.PriceChangePercentage.H6),
			toFloat(attr.PriceChangePercentage.H24),
			toFloat(attr.VolumeUSD.M5),
			toFloat(attr.VolumeUSD.H1),
			toFloat(attr.VolumeUSD.H6),
			toFloat(attr.VolumeUSD.H24),
			feePercent,
			feeUSDm5,
			feeUSDh1,
			feeUSDh6,
			feeUSDh24,
			feeAPRm5,
			feeAPRh1,
			feeAPRh6,
			feeAPRh24,
			uint32(attr.Transactions.H24.Buys),
			uint32(attr.Transactions.H24.Sells),
			uint32(attr.Transactions.H24.Buyers),
			uint32(attr.Transactions.H24.Sellers),
			time.Now(), // updated_at
		); err != nil {
			return err
		}
	}

	return batch.Send()
}

func (s *GeckoService) StartScheduler() {
	log.Println("⏰ Starting GeckoTerminal Scheduler (Interval: 30s, Single-thread: 5 pages)...")

	fetchJob := func() {
		log.Println("🔄 Triggering Fetch Job...")
		for page := 1; page <= 5; page++ {
			if err := s.FetchAndStore("bsc", page); err != nil {
				log.Printf("❌ Fetch Page %d Failed: %v", page, err)
				continue
			}
		}
	}

	go func() {
		// 立即执行一次
		fetchJob()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			fetchJob()
		}
	}()
}
