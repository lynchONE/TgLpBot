package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type HotPoolResponse struct {
	ProtocolVersion  string    `json:"protocol_version" ch:"protocol_version"`
	PoolAddress      string    `json:"pool_address" ch:"pool_address"`
	Dex              string    `json:"dex" ch:"dex"`
	FactoryName      string    `json:"factory_name" ch:"factory_name"`
	TradingPair      string    `json:"trading_pair" ch:"trading_pair"`
	FeePercentage    float64   `json:"fee_percentage" ch:"fee_percentage"`
	TransactionCount uint32    `json:"transaction_count" ch:"transaction_count"`
	TotalFees        float64   `json:"total_fees" ch:"total_fees"`
	TotalVolume      float64   `json:"total_volume" ch:"total_volume"`
	CurrentPoolValue float64   `json:"current_pool_value" ch:"current_pool_value"`
	FeeRate          float64   `json:"fee_rate" ch:"fee_rate"`
	PriceDisplay     string    `json:"price_display" ch:"price_display"`
	UpdatedAt        time.Time `json:"updated_at" ch:"updated_at"`
	LastSwapAt       time.Time `json:"last_swap_at" ch:"last_swap_at"`
	Token0Address    string    `json:"token0_address" ch:"token0_address"`
	Token1Address    string    `json:"token1_address" ch:"token1_address"`
}

type hotPoolsEnvelope struct {
	Data             []HotPoolResponse `json:"data"`
	UpdatedAt        *time.Time        `json:"updated_at,omitempty"`
	TimeframeMinutes int               `json:"timeframe_minutes"`
	Chain            string            `json:"chain"`
	Sort             string            `json:"sort"`
	Dex              string            `json:"dex,omitempty"`
}

func (s *Server) handleHotPools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.ClickHouse == nil || s.ClickHouse.Conn == nil {
		http.Error(w, "ClickHouse not configured", http.StatusServiceUnavailable)
		return
	}

	query := r.URL.Query()

	sort := strings.ToLower(strings.TrimSpace(query.Get("sort")))
	switch sort {
	case "", "fees", "fee", "fee_usd":
		sort = "fees"
	case "fee_rate", "rate", "fees_rate":
		sort = "fee_rate"
	case "volume", "vol":
		sort = "volume"
	default:
		http.Error(w, "invalid sort", http.StatusBadRequest)
		return
	}

	chain := strings.ToLower(strings.TrimSpace(query.Get("chain")))
	if chain == "" {
		chain = "bsc"
	}

	timeframeMinutes := 5
	if v := strings.TrimSpace(query.Get("timeframe_minutes")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeframeMinutes = n
		}
	}
	switch timeframeMinutes {
	case 5, 15, 60, 360:
	default:
		http.Error(w, "invalid timeframe_minutes", http.StatusBadRequest)
		return
	}

	limit := 50
	if v := strings.TrimSpace(query.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}

	dex := strings.ToLower(strings.TrimSpace(query.Get("dex")))

	orderBy := "total_fees"
	switch sort {
	case "fees":
		orderBy = "total_fees"
	case "fee_rate":
		orderBy = "fee_rate"
	case "volume":
		orderBy = "total_volume"
	}

	where := `
		WHERE chain = ?
		  AND timeframe_minutes = ?
	`
	args := []any{chain, uint16(timeframeMinutes)}
	if dex != "" {
		where += " AND lower(dex) = ?"
		args = append(args, dex)
	}

	q := fmt.Sprintf(`
		SELECT
			protocol_version,
			pool_address,
			dex,
			factory_name,
			trading_pair,
			fee_percentage,
			transaction_count,
			total_fees,
			total_volume,
			current_pool_value,
			if(current_pool_value > 0, total_fees / current_pool_value * 100, 0) AS fee_rate,
			price_display,
			ts AS updated_at,
			last_swap_at,
			token0_address,
			token1_address
		FROM poolm_top_fees_realtime
		%s
		ORDER BY %s DESC
		LIMIT %d
	`, where, orderBy, limit)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var rows []HotPoolResponse
	if err := s.ClickHouse.Conn.Select(ctx, &rows, q, args...); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var newest *time.Time
	for _, row := range rows {
		if row.UpdatedAt.IsZero() {
			continue
		}
		if newest == nil || row.UpdatedAt.After(*newest) {
			t := row.UpdatedAt
			newest = &t
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(hotPoolsEnvelope{
		Data:             rows,
		UpdatedAt:        newest,
		TimeframeMinutes: timeframeMinutes,
		Chain:            chain,
		Sort:             sort,
		Dex:              dex,
	})
}
