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
	case "auto_monitor":
		s.handleAutoMonitor(w, r)
	case "autolp_pnl_curve":
		s.handleAutoLPPnLCurve(w, r)
	case "position_profit_poster":
		s.handlePositionProfitPoster(w, r)
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
	case "autolp_config":
		s.handleAutoLPConfig(w, r)
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
	case "autolp_stats":
		s.handleAdminAutoLPStats(w, r)
	case "autolp_disable":
		s.handleAdminAutoLPDisable(w, r)
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
	default:
		http.Error(w, "invalid endpoint", http.StatusBadRequest)
	}
}

func (s *Server) handleTrading(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("endpoint")))
	switch endpoint {
	case "open_position":
		s.handleOpenPosition(w, r)
	case "blacklist":
		handleBlacklist(w, r)
	case "cooldowns":
		handleCooldowns(w, r)
	default:
		http.Error(w, "invalid endpoint", http.StatusBadRequest)
	}
}
