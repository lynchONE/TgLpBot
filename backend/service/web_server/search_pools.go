package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"TgLpBot/service/pool"

	"github.com/ethereum/go-ethereum/common"
)

type searchPoolsEnvelope struct {
	Data             []HotPoolResponse `json:"data"`
	Query            string            `json:"query"`
	Chain            string            `json:"chain"`
	TimeframeMinutes int               `json:"timeframe_minutes"`
	Limit            int               `json:"limit"`
}

func (s *Server) handleSearchPools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	q := strings.TrimSpace(query.Get("q"))
	if q == "" {
		q = strings.TrimSpace(query.Get("keyword"))
	}
	if q == "" {
		http.Error(w, "missing q", http.StatusBadRequest)
		return
	}
	if len(q) > 96 {
		q = q[:96]
	}

	chain := strings.ToLower(strings.TrimSpace(query.Get("chain")))
	if chain == "" {
		chain = "bsc"
	}

	limit := 10
	if v := strings.TrimSpace(query.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 10 {
		limit = 10
	}

	timeframeMinutes := 5

	normalizedPool := strings.ToLower(strings.TrimSpace(q))
	if normalizedPool != "" && (strings.HasPrefix(normalizedPool, "0x") || strings.HasPrefix(normalizedPool, "0X")) {
		normalizedPool = normalizedPool[2:]
	}
	poolIdOrAddress := isV4PoolId(q) || common.IsHexAddress(normalizeHexPrefixed(q))

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1) 优先从 ClickHouse 的快照表查询（用于代币搜索与已收录的池子ID搜索）
	if s.ClickHouse != nil && s.ClickHouse.Conn != nil {
		var rows []HotPoolResponse

		selectCols := `
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
		`

		var qSql string
		var args []any
		if poolIdOrAddress && normalizedPool != "" {
			qSql = fmt.Sprintf(`
				SELECT %s
				FROM poolm_top_fees_realtime
				WHERE chain = ?
				  AND timeframe_minutes = ?
				  AND lower(pool_address) = ?
				ORDER BY current_pool_value DESC
				LIMIT 1
			`, selectCols)
			args = []any{chain, uint16(timeframeMinutes), "0x" + normalizedPool}
		} else {
			needle := strings.ToLower(strings.TrimSpace(q))
			if len(needle) < 2 {
				http.Error(w, "q too short", http.StatusBadRequest)
				return
			}

			qSql = fmt.Sprintf(`
				SELECT %s
				FROM poolm_top_fees_realtime
				WHERE chain = ?
				  AND timeframe_minutes = ?
				  AND (
					position(lower(trading_pair), ?) > 0
					OR position(lower(pool_address), ?) > 0
					OR position(lower(token0_address), ?) > 0
					OR position(lower(token1_address), ?) > 0
				  )
				ORDER BY current_pool_value DESC
				LIMIT %d
			`, selectCols, limit)
			args = []any{chain, uint16(timeframeMinutes), needle, needle, needle, needle}
		}

		if err := s.ClickHouse.Conn.Select(ctx, &rows, qSql, args...); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// ClickHouse 命中则直接返回
		if len(rows) > 0 {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(searchPoolsEnvelope{
				Data:             rows,
				Query:            q,
				Chain:            chain,
				TimeframeMinutes: timeframeMinutes,
				Limit:            limit,
			})
			return
		}
	}

	// 2) ClickHouse 未命中：仅当是池子ID搜索时尝试链上回退
	if !poolIdOrAddress {
		if s.ClickHouse == nil || s.ClickHouse.Conn == nil {
			http.Error(w, "ClickHouse not configured", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searchPoolsEnvelope{
			Data:             []HotPoolResponse{},
			Query:            q,
			Chain:            chain,
			TimeframeMinutes: timeframeMinutes,
			Limit:            limit,
		})
		return
	}

	// pool id / address 走链上读取（用于未被快照收录的池子）
	poolService := pool.NewPoolService()
	poolAddress := normalizeHexPrefixed(q)
	var (
		poolInfo *pool.PoolInfo
		infoErr  error
		version  string
	)
	if isV4PoolId(poolAddress) {
		version = "v4"
		poolInfo, infoErr = poolService.GetV4PoolInfo(poolAddress)
	} else {
		version = "v3"
		if !common.IsHexAddress(poolAddress) {
			http.Error(w, "invalid pool address", http.StatusBadRequest)
			return
		}
		poolInfo, infoErr = poolService.GetPoolInfo(poolAddress)
	}
	if infoErr != nil || poolInfo == nil {
		http.Error(w, "pool not found", http.StatusNotFound)
		return
	}

	feePct := float64(poolInfo.Fee) / 10000.0
	tradingPair := strings.TrimSpace(poolInfo.Token0Symbol) + "/" + strings.TrimSpace(poolInfo.Token1Symbol)
	fallback := HotPoolResponse{
		ProtocolVersion:  version,
		PoolAddress:      poolAddress,
		Dex:              "",
		FactoryName:      poolInfo.Exchange,
		TradingPair:      tradingPair,
		FeePercentage:    feePct,
		TransactionCount: 0,
		TotalFees:        0,
		TotalVolume:      0,
		CurrentPoolValue: 0,
		FeeRate:          0,
		PriceDisplay:     "",
		UpdatedAt:        time.Now(),
		LastSwapAt:       time.Time{},
		Token0Address:    poolInfo.Token0,
		Token1Address:    poolInfo.Token1,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(searchPoolsEnvelope{
		Data:             []HotPoolResponse{fallback},
		Query:            q,
		Chain:            chain,
		TimeframeMinutes: timeframeMinutes,
		Limit:            limit,
	})
}
