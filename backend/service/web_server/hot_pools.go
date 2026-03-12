package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
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
	// 24小时数据
	TotalFees24h        float64 `json:"total_fees_24h,omitempty"`
	TotalVolume24h      float64 `json:"total_volume_24h,omitempty"`
	TransactionCount24h uint32  `json:"transaction_count_24h,omitempty"`
}

type hotPoolsEnvelope struct {
	Data             []HotPoolResponse `json:"data"`
	UpdatedAt        *time.Time        `json:"updated_at,omitempty"`
	TimeframeMinutes int               `json:"timeframe_minutes"`
	Chain            string            `json:"chain"`
	Sort             string            `json:"sort"`
	Dex              string            `json:"dex,omitempty"`
}

func normalizeHotPoolTokenAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !common.IsHexAddress(raw) {
		return ""
	}
	return strings.ToLower(common.HexToAddress(raw).Hex())
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

	initData := initDataFromQuery(r)
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		http.Error(w, msg, status)
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
	tokenAddress := normalizeHotPoolTokenAddress(query.Get("token_address"))
	if strings.TrimSpace(query.Get("token_address")) != "" && tokenAddress == "" {
		http.Error(w, "invalid token_address", http.StatusBadRequest)
		return
	}

	// 解析 include_pools 参数（逗号分隔的池子地址列表）
	var includePools []string
	if v := strings.TrimSpace(query.Get("include_pools")); v != "" {
		parts := strings.Split(v, ",")
		for _, p := range parts {
			addr := strings.TrimSpace(p)
			if addr != "" {
				includePools = append(includePools, strings.ToLower(addr))
			}
		}
	}

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
	if tokenAddress != "" {
		where += " AND (lower(token0_address) = ? OR lower(token1_address) = ?)"
		args = append(args, tokenAddress, tokenAddress)
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

	// 获取24小时数据
	if len(rows) > 0 {
		poolAddresses := make([]string, len(rows))
		for i, row := range rows {
			poolAddresses[i] = row.PoolAddress
		}

		// 查询24小时聚合数据
		q24h := `
			SELECT
				pool_address,
				sum(total_fees) AS total_fees_24h,
				sum(total_volume) AS total_volume_24h,
				sum(transaction_count) AS transaction_count_24h
			FROM poolm_top_fees_raw
			WHERE chain = ?
			  AND pool_address IN (?)
			  AND ts >= now() - INTERVAL 24 HOUR
			GROUP BY pool_address
		`

		type stats24h struct {
			PoolAddress         string  `ch:"pool_address"`
			TotalFees24h        float64 `ch:"total_fees_24h"`
			TotalVolume24h      float64 `ch:"total_volume_24h"`
			TransactionCount24h uint32  `ch:"transaction_count_24h"`
		}

		var stats24hRows []stats24h
		if err := s.ClickHouse.Conn.Select(ctx, &stats24hRows, q24h, chain, poolAddresses); err == nil {
			// 构建map用于快速查找
			stats24hMap := make(map[string]stats24h)
			for _, s := range stats24hRows {
				stats24hMap[s.PoolAddress] = s
			}

			// 合并24小时数据到结果
			for i := range rows {
				if s, ok := stats24hMap[rows[i].PoolAddress]; ok {
					rows[i].TotalFees24h = s.TotalFees24h
					rows[i].TotalVolume24h = s.TotalVolume24h
					rows[i].TransactionCount24h = s.TransactionCount24h
				}
			}
		}
	}

	// 处理 include_pools：查询并合并不在热门列表中的指定池子
	if len(includePools) > 0 {
		// 构建已有池子地址的 set（小写）
		existingPools := make(map[string]bool)
		for _, row := range rows {
			existingPools[strings.ToLower(row.PoolAddress)] = true
		}

		// 筛选出不在热门列表中的池子
		var missingPools []string
		for _, addr := range includePools {
			if !existingPools[addr] {
				missingPools = append(missingPools, addr)
			}
		}

		// 查询这些池子的数据
		if len(missingPools) > 0 {
			qExtra := `
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
				WHERE chain = ?
				  AND timeframe_minutes = ?
				  AND lower(pool_address) IN (?)
			`
			extraArgs := []any{chain, uint16(timeframeMinutes), missingPools}
			if tokenAddress != "" {
				qExtra += `
				  AND (lower(token0_address) = ? OR lower(token1_address) = ?)
				`
				extraArgs = append(extraArgs, tokenAddress, tokenAddress)
			}

			var extraRows []HotPoolResponse
			if err := s.ClickHouse.Conn.Select(ctx, &extraRows, qExtra, extraArgs...); err == nil {
				// 获取这些额外池子的24小时数据
				if len(extraRows) > 0 {
					extraAddrs := make([]string, len(extraRows))
					for i, row := range extraRows {
						extraAddrs[i] = row.PoolAddress
					}

					type stats24h struct {
						PoolAddress         string  `ch:"pool_address"`
						TotalFees24h        float64 `ch:"total_fees_24h"`
						TotalVolume24h      float64 `ch:"total_volume_24h"`
						TransactionCount24h uint32  `ch:"transaction_count_24h"`
					}

					q24hExtra := `
						SELECT
							pool_address,
							sum(total_fees) AS total_fees_24h,
							sum(total_volume) AS total_volume_24h,
							sum(transaction_count) AS transaction_count_24h
						FROM poolm_top_fees_raw
						WHERE chain = ?
						  AND pool_address IN (?)
						  AND ts >= now() - INTERVAL 24 HOUR
						GROUP BY pool_address
					`

					var extraStats []stats24h
					if err := s.ClickHouse.Conn.Select(ctx, &extraStats, q24hExtra, chain, extraAddrs); err == nil {
						stats24hMap := make(map[string]stats24h)
						for _, s := range extraStats {
							stats24hMap[s.PoolAddress] = s
						}
						for i := range extraRows {
							if s, ok := stats24hMap[extraRows[i].PoolAddress]; ok {
								extraRows[i].TotalFees24h = s.TotalFees24h
								extraRows[i].TotalVolume24h = s.TotalVolume24h
								extraRows[i].TransactionCount24h = s.TransactionCount24h
							}
						}
					}

					// 将额外池子追加到结果
					rows = append(rows, extraRows...)
				}
			}
		}
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
