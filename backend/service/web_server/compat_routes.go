package web_server

import (
	"net/http"
	"strings"
)

// Compat routes let miniapp call merged endpoints directly on backend without Vercel proxy.

func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("endpoint")))
	switch endpoint {
	case "realtime_positions":
		s.handleRealtimePositions(w, r)
	case "me":
		s.handleMe(w, r)
	case "position_profit_poster":
		s.handlePositionProfitPoster(w, r)
	case "assets_overview":
		s.handleAssetOverview(w, r)
	case "assets_history":
		s.handleAssetHistory(w, r)
	case "assets_lp_stats":
		s.handleAssetLPStats(w, r)
	default:
		http.Error(w, "invalid endpoint", http.StatusBadRequest)
	}
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("endpoint")))
	switch endpoint {
	case "config":
		s.handleConfig(w, r)
	case "global_config":
		s.handleGlobalConfig(w, r)
	case "wallets":
		s.handleWallets(w, r)
	default:
		http.Error(w, "invalid endpoint", http.StatusBadRequest)
	}
}

func (s *Server) handleTaskAction(w http.ResponseWriter, r *http.Request) {
	action := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("action")))
	switch action {
	case "pause":
		s.handleTaskPause(w, r)
	case "stop":
		s.handleTaskStop(w, r)
	case "delete":
		s.handleTaskDelete(w, r)
	case "update_range":
		s.handleTaskUpdateRange(w, r)
	case "withdraw_liquidity":
		s.handleTaskWithdrawLiquidity(w, r)
	case "swap_dust":
		s.handleTaskSwapDust(w, r)
	case "trigger_rebalance":
		s.handleTaskTriggerRebalance(w, r)
	case "toggle_rebalance":
		s.handleTaskToggleRebalance(w, r)
	case "add_liquidity":
		s.handleTaskAddLiquidity(w, r)
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
	}
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("endpoint")))
	switch endpoint {
	case "realtime_users":
		s.handleAdminRealtimeUsers(w, r)
	case "realtime_positions":
		s.handleAdminRealtimePositions(w, r)
	case "system_config":
		s.handleAdminSystemConfig(w, r)
	case "online_users":
		s.handleAdminOnlineUsers(w, r)
	case "active_tasks":
		s.handleAdminActiveTasks(w, r)
	case "rpc_pool":
		s.handleAdminRPCPool(w, r)
	case "private_zap":
		s.handleAdminPrivateZap(w, r)
	case "assets_smart_money_overview":
		s.handleAdminSmartMoneyOverview(w, r)
	case "assets_smart_money_wallet":
		s.handleAdminSmartMoneyWallet(w, r)
	case "assets_smart_money_leaderboard":
		s.handleAdminSmartMoneyLeaderboard(w, r)
	default:
		http.Error(w, "invalid endpoint", http.StatusBadRequest)
	}
}

func (s *Server) handleTrading(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("endpoint")))
	switch endpoint {
	case "open_position_preview":
		s.handleOpenPositionPreview(w, r)
	case "open_position":
		s.handleOpenPosition(w, r)
	case "create_pool_preview":
		s.handleCreatePoolPreview(w, r)
	case "create_pool_execute":
		s.handleCreatePoolExecute(w, r)
	default:
		http.Error(w, "invalid endpoint", http.StatusBadRequest)
	}
}
