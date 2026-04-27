package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/service/assets"
	"TgLpBot/service/pricing"
	"TgLpBot/service/realtime"
	"TgLpBot/service/token_metadata"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Server struct {
	Realtime   *realtime.RealtimePositionsService
	TokenPrice *pricing.TokenPriceService
	TokenMeta  *token_metadata.Service
	Assets     *assets.Service
}

func NewServer() *Server {
	return &Server{
		Realtime:   realtime.NewRealtimePositionsService(),
		TokenPrice: pricing.NewTokenPriceService(),
		TokenMeta:  token_metadata.NewService(),
		Assets:     assets.NewService(),
	}
}

func (s *Server) Start(port string) {
	initSmartMoney()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", s.handleGetPools)
	mux.HandleFunc("/api/positions", s.handlePositions)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/task_action", s.handleTaskAction)
	mux.HandleFunc("/api/admin", s.handleAdmin)
	mux.HandleFunc("/api/trading", s.handleTrading)
	mux.HandleFunc("/api/search_pools", s.handleSearchPools)
	mux.HandleFunc("/api/token_candles", s.handleTokenCandles)
	mux.HandleFunc("/api/smart_money_pool_markers", s.handleSmartMoneyPoolMarkers)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/global_config", s.handleGlobalConfig)
	mux.HandleFunc("/api/position_profit_poster", s.handlePositionProfitPoster)
	mux.HandleFunc("/api/me", s.handleMe)
	mux.HandleFunc("/api/me/avatar", s.handleMeAvatar)
	mux.HandleFunc("/api/realtime_positions", s.handleRealtimePositions)
	mux.HandleFunc("/api/assets/overview", s.handleAssetOverview)
	mux.HandleFunc("/api/assets/history", s.handleAssetHistory)
	mux.HandleFunc("/api/assets/lp_stats", s.handleAssetLPStats)
	mux.HandleFunc("/api/assets_overview", s.handleAssetOverview)
	mux.HandleFunc("/api/assets_history", s.handleAssetHistory)
	mux.HandleFunc("/api/assets_lp_stats", s.handleAssetLPStats)
	mux.HandleFunc("/api/admin/assets/smart_money_overview", s.handleAdminSmartMoneyOverview)
	mux.HandleFunc("/api/admin/assets/smart_money_wallet", s.handleAdminSmartMoneyWallet)
	mux.HandleFunc("/api/admin/assets/smart_money_leaderboard", s.handleAdminSmartMoneyLeaderboard)
	mux.HandleFunc("/api/admin/assets_smart_money_overview", s.handleAdminSmartMoneyOverview)
	mux.HandleFunc("/api/admin/assets_smart_money_wallet", s.handleAdminSmartMoneyWallet)
	mux.HandleFunc("/api/admin/assets_smart_money_leaderboard", s.handleAdminSmartMoneyLeaderboard)
	mux.HandleFunc("/api/task_pause", s.handleTaskPause)
	mux.HandleFunc("/api/task_stop", s.handleTaskStop)
	mux.HandleFunc("/api/task_delete", s.handleTaskDelete)
	mux.HandleFunc("/api/task_update_range", s.handleTaskUpdateRange)
	mux.HandleFunc("/api/task_withdraw_liquidity", s.handleTaskWithdrawLiquidity)
	mux.HandleFunc("/api/task_swap_dust", s.handleTaskSwapDust)
	mux.HandleFunc("/api/task_trigger_rebalance", s.handleTaskTriggerRebalance)
	mux.HandleFunc("/api/task_toggle_rebalance", s.handleTaskToggleRebalance)
	mux.HandleFunc("/api/task_update_mode", s.handleTaskUpdateMode)
	mux.HandleFunc("/api/task_add_liquidity", s.handleTaskAddLiquidity)
	mux.HandleFunc("/api/open_position_prepare", s.handleOpenPositionPrepare)
	mux.HandleFunc("/api/open_position_preview", s.handleOpenPositionPreview)
	mux.HandleFunc("/api/open_position", s.handleOpenPosition)
	mux.HandleFunc("/api/pool_liquidity_distribution", s.handleLiquidityDistribution)
	mux.HandleFunc("/api/create_pool_preview", s.handleCreatePoolPreview)
	mux.HandleFunc("/api/create_pool_execute", s.handleCreatePoolExecute)
	mux.HandleFunc("/api/my_trade_markers", s.handleMyTradeMarkers)
	mux.HandleFunc("/api/trade_history", s.handleTradeHistory)
	mux.HandleFunc("/api/wallet_swap_preview", s.handleWalletSwapPreview)
	mux.HandleFunc("/api/wallet_swap_execute", s.handleWalletSwapExecute)
	mux.HandleFunc("/api/wallet_swap_token_metadata", s.handleWalletSwapTokenMetadata)
	mux.HandleFunc("/api/wallet_swap_history", s.handleWalletSwapHistory)
	mux.HandleFunc("/api/admin/realtime_users", s.handleAdminRealtimeUsers)
	mux.HandleFunc("/api/admin/realtime_positions", s.handleAdminRealtimePositions)
	mux.HandleFunc("/api/admin/system_config", s.handleAdminSystemConfig)
	mux.HandleFunc("/api/admin/online_users", s.handleAdminOnlineUsers)
	mux.HandleFunc("/api/admin/active_tasks", s.handleAdminActiveTasks)
	mux.HandleFunc("/api/admin/user_access", s.handleAdminUserAccess)
	mux.HandleFunc("/api/admin/rpc_pool", s.handleAdminRPCPool)
	mux.HandleFunc("/api/admin/pool_data_sources", s.handleAdminPoolDataSources)
	mux.HandleFunc("/api/admin/private_zap", s.handleAdminPrivateZap)
	mux.HandleFunc("/api/web_login", s.handleWebLogin)

	// Smart Money routes
	s.registerSmartMoneyRoutes(mux)

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
		log.Printf("Web API Server listening on %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Web Server Error: %v", err)
		}
	}()
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type PoolResponse struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Address        string    `json:"address"`
	DexID          string    `json:"dex_id"`
	PriceUSD       float64   `json:"price_usd"`
	VolumeM5       float64   `json:"volume_m5"`
	VolumeH1       float64   `json:"volume_h1"`
	VolumeH6       float64   `json:"volume_h6"`
	VolumeH24      float64   `json:"volume_h24"`
	ReserveUSD     float64   `json:"reserve_usd"`
	PriceChangeM5  float64   `json:"price_change_m5"`
	PriceChangeH1  float64   `json:"price_change_h1"`
	PriceChangeH6  float64   `json:"price_change_h6"`
	PriceChangeH24 float64   `json:"price_change_h24"`
	PoolFeePct     float64   `json:"pool_fee_percentage"`
	FeeUsdM5       float64   `json:"fee_usd_m5"`
	FeeUsdH1       float64   `json:"fee_usd_h1"`
	FeeUsdH6       float64   `json:"fee_usd_h6"`
	FeeUsdH24      float64   `json:"fee_usd_h24"`
	FeeAprM5       float64   `json:"fee_apr_m5"`
	FeeAprH1       float64   `json:"fee_apr_h1"`
	FeeAprH6       float64   `json:"fee_apr_h6"`
	FeeAprH24      float64   `json:"fee_apr_h24"`
	UpdatedAt      time.Time `json:"updated_at"`
}

var poolFeeFromNameRegex = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*%\s*$`)

func (s *Server) handleGetPools(w http.ResponseWriter, r *http.Request) {
	if endpoint := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("endpoint"))); endpoint != "" {
		switch endpoint {
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

	opts := parsePoolCatalogOptions(r)
	cacheKey := buildPoolCatalogCacheKey(opts)
	if cacheKey != "" {
		if cached, ok := readRedisRawCache(cacheKey); ok {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cached)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	poolRows, err := loadPoolCatalogRows(ctx, opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := buildPoolCatalogResponse(poolRows, opts)
	s.enrichHotPoolDisplayTokens(ctx, opts.Chain, resp.Data)
	b, err := marshalJSONPayload(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if cacheKey != "" {
		writeRedisRawCache(cacheKey, b, poolsCacheTTL)
	}
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
	_ = json.NewEncoder(w).Encode(cfg)
}
