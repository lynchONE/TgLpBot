package web_server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"TgLpBot/base/clickhouse"
	"TgLpBot/base/config"
	"TgLpBot/service/pricing"
	"TgLpBot/service/realtime"
	"TgLpBot/service/token_metadata"
)

type Server struct {
	ClickHouse *clickhouse.ClickHouseService
	Realtime   *realtime.RealtimePositionsService
	TokenPrice *pricing.TokenPriceService
	TokenMeta  *token_metadata.Service
}

func NewServer(ch *clickhouse.ClickHouseService) *Server {
	return &Server{
		ClickHouse: ch,
		Realtime:   realtime.NewRealtimePositionsService(),
		TokenPrice: pricing.NewTokenPriceService(),
		TokenMeta:  token_metadata.NewService(),
	}
}

func (s *Server) Start(port string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", s.handleGetPools)
	mux.HandleFunc("/api/positions", s.handlePositions)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/task_action", s.handleTaskAction)
	mux.HandleFunc("/api/admin", s.handleAdmin)
	mux.HandleFunc("/api/trading", s.handleTrading)
	mux.HandleFunc("/api/hot_pools", s.handleHotPools)
	mux.HandleFunc("/api/search_pools", s.handleSearchPools)
	mux.HandleFunc("/api/token_candles", s.handleTokenCandles)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/global_config", s.handleGlobalConfig)
	mux.HandleFunc("/api/autolp_config", s.handleAutoLPConfig)
	mux.HandleFunc("/api/autolp_pnl_curve", s.handleAutoLPPnLCurve)
	mux.HandleFunc("/api/position_profit_poster", s.handlePositionProfitPoster)
	mux.HandleFunc("/api/me", s.handleMe)
	mux.HandleFunc("/api/me/avatar", s.handleMeAvatar)
	mux.HandleFunc("/api/realtime_positions", s.handleRealtimePositions)
	mux.HandleFunc("/api/smart_money", s.handleSmartMoneyOverview)
	mux.HandleFunc("/api/smart_money_overview", s.handleSmartMoneyOverview)
	mux.HandleFunc("/api/smart_money_pool_adds", s.handleSmartMoneyPoolAdds)
	mux.HandleFunc("/api/smart_money_pool_markers", s.handleSmartMoneyPoolMarkers)
	mux.HandleFunc("/api/smart_money_wallet_positions", s.handleSmartMoneyWalletPositions)
	mux.HandleFunc("/api/smart_money_follow_config", s.handleSmartMoneyFollowConfig)
	mux.HandleFunc("/api/smart_money_follow_configs", s.handleSmartMoneyFollowConfigs)
	mux.HandleFunc("/api/smart_money_golden_dog_config", s.handleSmartMoneyGoldenDogConfig)
	mux.HandleFunc("/api/smart_money_watched_wallets", s.handleSmartMoneyWatchedWallets)
	mux.HandleFunc("/api/smart_money_24h_pool_adds", s.handleSmartMoney24hPoolAdds)
	mux.HandleFunc("/api/auto_monitor", s.handleAutoMonitor)
	mux.HandleFunc("/api/task_pause", s.handleTaskPause)
	mux.HandleFunc("/api/task_stop", s.handleTaskStop)
	mux.HandleFunc("/api/task_delete", s.handleTaskDelete)
	mux.HandleFunc("/api/task_update_range", s.handleTaskUpdateRange)
	mux.HandleFunc("/api/open_position", s.handleOpenPosition)
	mux.HandleFunc("/api/my_trade_markers", s.handleMyTradeMarkers)
	mux.HandleFunc("/api/admin/realtime_users", s.handleAdminRealtimeUsers)
	mux.HandleFunc("/api/admin/realtime_positions", s.handleAdminRealtimePositions)
	mux.HandleFunc("/api/admin/autolp_stats", s.handleAdminAutoLPStats)
	mux.HandleFunc("/api/admin/autolp_disable", s.handleAdminAutoLPDisable)
	mux.HandleFunc("/api/admin/system_config", s.handleAdminSystemConfig)
	mux.HandleFunc("/api/admin/online_users", s.handleAdminOnlineUsers)
	mux.HandleFunc("/api/admin/active_tasks", s.handleAdminActiveTasks)
	mux.HandleFunc("/api/admin/user_access", s.handleAdminUserAccess)
	mux.HandleFunc("/api/admin/rpc_pool", s.handleAdminRPCPool)
	mux.HandleFunc("/api/admin/private_zap", s.handleAdminPrivateZap)
	mux.HandleFunc("/api/blacklist", handleBlacklist)
	mux.HandleFunc("/api/cooldowns", handleCooldowns)
	mux.HandleFunc("/api/web_login", s.handleWebLogin)

	handler := corsMiddleware(mux)

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("🌐 Web API Server listening on %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Web Server Error: %v", err)
		}
	}()
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type PoolResponse struct {
	ID             string    `json:"id" ch:"id"`
	Name           string    `json:"name" ch:"name"`
	Address        string    `json:"address" ch:"address"`
	DexID          string    `json:"dex_id" ch:"dex_id"`
	PriceUSD       float64   `json:"price_usd" ch:"base_token_price_usd"`
	VolumeM5       float64   `json:"volume_m5" ch:"volume_m5"`
	VolumeH1       float64   `json:"volume_h1" ch:"volume_h1"`
	VolumeH6       float64   `json:"volume_h6" ch:"volume_h6"`
	VolumeH24      float64   `json:"volume_h24" ch:"volume_h24"`
	ReserveUSD     float64   `json:"reserve_usd" ch:"reserve_in_usd"`
	PriceChangeM5  float64   `json:"price_change_m5" ch:"price_change_m5"`
	PriceChangeH1  float64   `json:"price_change_h1" ch:"price_change_h1"`
	PriceChangeH6  float64   `json:"price_change_h6" ch:"price_change_h6"`
	PriceChangeH24 float64   `json:"price_change_h24" ch:"price_change_h24"`
	PoolFeePct     float64   `json:"pool_fee_percentage" ch:"pool_fee_percentage"`
	FeeUsdM5       float64   `json:"fee_usd_m5" ch:"fee_usd_m5"`
	FeeUsdH1       float64   `json:"fee_usd_h1" ch:"fee_usd_h1"`
	FeeUsdH6       float64   `json:"fee_usd_h6" ch:"fee_usd_h6"`
	FeeUsdH24      float64   `json:"fee_usd_h24" ch:"fee_usd_h24"`
	FeeAprM5       float64   `json:"fee_apr_m5" ch:"fee_apr_m5"`
	FeeAprH1       float64   `json:"fee_apr_h1" ch:"fee_apr_h1"`
	FeeAprH6       float64   `json:"fee_apr_h6" ch:"fee_apr_h6"`
	FeeAprH24      float64   `json:"fee_apr_h24" ch:"fee_apr_h24"`
	UpdatedAt      time.Time `json:"updated_at" ch:"updated_at"`
}

var poolFeeFromNameRegex = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*%\s*$`)

func (s *Server) handleGetPools(w http.ResponseWriter, r *http.Request) {
	if endpoint := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("endpoint"))); endpoint != "" {
		switch endpoint {
		case "hot_pools":
			s.handleHotPools(w, r)
			return
		case "search_pools":
			s.handleSearchPools(w, r)
			return
		case "token_candles":
			s.handleTokenCandles(w, r)
			return
		default:
			http.Error(w, "invalid endpoint", http.StatusBadRequest)
			return
		}
	}

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

	if cached, ok := readRedisRawCache("pools:default:v1"); ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(cached)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var pools []PoolResponse

	// Simple top 50 query
	query := `
		SELECT 
			id, name, address, dex_id,
			base_token_price_usd, 
			volume_m5, volume_h1, volume_h6, volume_h24,
			reserve_in_usd,
			price_change_m5, price_change_h1, price_change_h6, price_change_h24,
			pool_fee_percentage,
			fee_usd_m5, fee_usd_h1, fee_usd_h6, fee_usd_h24,
			fee_apr_m5, fee_apr_h1, fee_apr_h6, fee_apr_h24,
			updated_at
		FROM pools FINAL
		ORDER BY volume_h24 DESC
		LIMIT 50
	`

	if err := s.ClickHouse.Conn.Select(ctx, &pools, query); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for i := range pools {
		cleanName := strings.TrimSpace(poolFeeFromNameRegex.ReplaceAllString(pools[i].Name, ""))
		if cleanName != "" {
			pools[i].Name = cleanName
		}
	}

	resp := map[string]interface{}{
		"data": pools,
	}
	b, err := marshalJSONPayload(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeRedisRawCache("pools:default:v1", b, hotPoolsCacheTTL)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
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

	cfg := map[string]string{
		"zap_v3": config.AppConfig.ZapV3Address,
		"zap_v4": config.AppConfig.ZapV4Address,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}
