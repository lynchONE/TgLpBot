package web_server

import (
	"TgLpBot/base/models"
	sm "TgLpBot/service/smart_money"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

var (
	smService *sm.Service
	smWSHub   *sm.WSHub
)

func initSmartMoney() {
	smService = sm.NewService()
	smWSHub = sm.NewWSHub()

	smService.SetNotifier(func(event *models.SmartMoneyLPEvent) {
		// Lookup wallet label
		repo := smService.Repo()
		w, _ := repo.GetMonitoredWalletByAddress(context.Background(), event.WalletAddress, event.ChainID)
		var label *string
		if w != nil {
			label = w.Label
		}
		smWSHub.BroadcastLPEvent(event, label)
	})

	smService.Start()
}

func stopSmartMoney() {
	if smService != nil {
		smService.Stop()
	}
}

// --- Route Registration (called from server.go) ---

func (s *Server) registerSmartMoneyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/sm/wallets", s.handleSMWallets)
	mux.HandleFunc("/api/sm/contracts", s.handleSMContracts)
	mux.HandleFunc("/api/sm/pools", s.handleSMPools)
	mux.HandleFunc("/api/sm/positions", s.handleSMPositions)
	mux.HandleFunc("/api/sm/events", s.handleSMEvents)
	mux.HandleFunc("/api/sm/stats", s.handleSMStats)
	mux.HandleFunc("/ws/sm/events", smWSHub.HandleWS)
}

// --- Wallets ---

func (s *Server) handleSMWallets(w http.ResponseWriter, r *http.Request) {
	repo := smService.Repo()
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		if page <= 0 {
			page = 1
		}
		if size <= 0 || size > 100 {
			size = 20
		}
		keyword := r.URL.Query().Get("keyword")
		source := r.URL.Query().Get("source")

		var activeOnly *bool
		if v := r.URL.Query().Get("active"); v != "" {
			b := v == "true" || v == "1"
			activeOnly = &b
		}

		rows, total, err := repo.ListWalletsWithStats(ctx, page, size, keyword, source, activeOnly)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		type walletResp struct {
			sm.WalletStatsRow
			Color string `json:"color"`
		}
		list := make([]walletResp, 0, len(rows))
		for _, row := range rows {
			list = append(list, walletResp{
				WalletStatsRow: row,
				Color:          sm.WalletColor(row.Address),
			})
		}

		jsonOK(w, map[string]interface{}{
			"total": total,
			"page":  page,
			"size":  size,
			"list":  list,
		})

	case http.MethodPost:
		var req struct {
			Address string `json:"address"`
			Label   string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		addr := strings.TrimSpace(req.Address)
		if !isValidAddress(addr) {
			jsonError(w, "invalid address", http.StatusBadRequest)
			return
		}
		label := strings.TrimSpace(req.Label)
		var labelPtr *string
		if label != "" {
			labelPtr = &label
		}
		wallet := &models.MonitoredWallet{
			Address:  strings.ToLower(addr),
			ChainID:  56,
			Source:   "manual",
			Label:    labelPtr,
			IsActive: true,
		}
		if err := repo.CreateMonitoredWallet(ctx, wallet); err != nil {
			if strings.Contains(err.Error(), "Duplicate") {
				jsonError(w, "wallet already exists", http.StatusConflict)
				return
			}
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true})

	case http.MethodPut:
		addr := extractPathParam(r.URL.Path, "/api/sm/wallets/")
		if addr == "" {
			addr = r.URL.Query().Get("address")
		}
		if !isValidAddress(addr) {
			jsonError(w, "invalid address", http.StatusBadRequest)
			return
		}
		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		allowed := map[string]bool{"label": true, "is_active": true}
		filtered := make(map[string]interface{})
		for k, v := range updates {
			if allowed[k] {
				filtered[k] = v
			}
		}
		if len(filtered) == 0 {
			jsonError(w, "no valid fields to update", http.StatusBadRequest)
			return
		}
		if err := repo.UpdateMonitoredWallet(ctx, addr, 56, filtered); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true})

	case http.MethodDelete:
		addr := extractPathParam(r.URL.Path, "/api/sm/wallets/")
		if addr == "" {
			addr = r.URL.Query().Get("address")
		}
		if !isValidAddress(addr) {
			jsonError(w, "invalid address", http.StatusBadRequest)
			return
		}
		if err := repo.SoftDeleteMonitoredWallet(ctx, addr, 56); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Contracts ---

func (s *Server) handleSMContracts(w http.ResponseWriter, r *http.Request) {
	repo := smService.Repo()
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		contracts, err := repo.ListWatchContracts(ctx)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]interface{}{"list": contracts})

	case http.MethodPost:
		var req struct {
			ContractAddress string `json:"contract_address"`
			Protocol        string `json:"protocol"`
			Description     string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if !isValidAddress(req.ContractAddress) {
			jsonError(w, "invalid contract address", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Protocol) == "" {
			jsonError(w, "protocol is required", http.StatusBadRequest)
			return
		}
		var desc *string
		if d := strings.TrimSpace(req.Description); d != "" {
			desc = &d
		}
		c := &models.WatchContract{
			ContractAddress: strings.ToLower(req.ContractAddress),
			ChainID:         56,
			Protocol:        strings.TrimSpace(req.Protocol),
			Description:     desc,
			IsActive:        true,
		}
		if err := repo.CreateWatchContract(ctx, c); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true})

	case http.MethodPut:
		addr := r.URL.Query().Get("address")
		if !isValidAddress(addr) {
			jsonError(w, "invalid address", http.StatusBadRequest)
			return
		}
		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		allowed := map[string]bool{"protocol": true, "description": true, "is_active": true}
		filtered := make(map[string]interface{})
		for k, v := range updates {
			if allowed[k] {
				filtered[k] = v
			}
		}
		if err := repo.UpdateWatchContract(ctx, addr, 56, filtered); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true})

	case http.MethodDelete:
		addr := r.URL.Query().Get("address")
		if !isValidAddress(addr) {
			jsonError(w, "invalid address", http.StatusBadRequest)
			return
		}
		if err := repo.DeleteWatchContract(ctx, addr, 56); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Pools ---

func (s *Server) handleSMPools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repo := smService.Repo()
	ctx := r.Context()

	poolAddr := r.URL.Query().Get("pool")
	if poolAddr != "" {
		// Single pool stats
		stats, err := repo.GetPoolStats(ctx, poolAddr)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, stats)
		return
	}

	// Pool list
	pools, err := repo.ListPoolsWithPositions(ctx)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{
		"total": len(pools),
		"list":  pools,
	})
}

// --- Positions ---

func (s *Server) handleSMPositions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repo := smService.Repo()
	ctx := r.Context()

	q := r.URL.Query()
	status := q.Get("status")
	if status == "" {
		status = "open"
	}
	wallet := q.Get("wallet")
	pool := q.Get("pool")
	protocol := q.Get("protocol")
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	orderBy := q.Get("order_by")
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}

	positions, total, err := repo.ListPositions(ctx, status, wallet, pool, protocol, page, size, orderBy)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Enrich with wallet color, label, price_lower, price_upper
	type posResp struct {
		models.SmartMoneyLPPosition
		WalletLabel *string `json:"wallet_label"`
		WalletColor string  `json:"wallet_color"`
		PriceLower  string  `json:"price_lower"`
		PriceUpper  string  `json:"price_upper"`
		BscscanURL  string  `json:"bscscan_url"`
	}

	list := make([]posResp, 0, len(positions))
	// Cache wallet labels
	walletCache := make(map[string]*models.MonitoredWallet)
	for _, p := range positions {
		resp := posResp{
			SmartMoneyLPPosition: p,
			WalletColor:          sm.WalletColor(p.WalletAddress),
			BscscanURL:           "https://bscscan.com/tx/" + p.OpenTxHash,
		}

		// Wallet label
		if w, ok := walletCache[p.WalletAddress]; ok {
			if w != nil {
				resp.WalletLabel = w.Label
			}
		} else {
			w, _ := repo.GetMonitoredWalletByAddress(ctx, p.WalletAddress, p.ChainID)
			walletCache[p.WalletAddress] = w
			if w != nil {
				resp.WalletLabel = w.Label
			}
		}

		// Tick to price (assuming 18 decimals for both tokens as default)
		if p.TickLower != nil {
			resp.PriceLower = fmt.Sprintf("%.6g", sm.TickToPrice(*p.TickLower, 18, 18))
		}
		if p.TickUpper != nil {
			resp.PriceUpper = fmt.Sprintf("%.6g", sm.TickToPrice(*p.TickUpper, 18, 18))
		}

		list = append(list, resp)
	}

	jsonOK(w, map[string]interface{}{
		"total": total,
		"page":  page,
		"size":  size,
		"list":  list,
	})
}

// --- Events ---

func (s *Server) handleSMEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repo := smService.Repo()
	ctx := r.Context()

	wallet := r.URL.Query().Get("wallet")
	pool := r.URL.Query().Get("pool")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}

	events, total, err := repo.ListLPEvents(ctx, wallet, pool, page, size)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]interface{}{
		"total": total,
		"page":  page,
		"size":  size,
		"list":  events,
	})
}

// --- Stats ---

func (s *Server) handleSMStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repo := smService.Repo()
	ctx := r.Context()

	// Single wallet stats
	addr := r.URL.Query().Get("address")
	if addr != "" && isValidAddress(addr) {
		wal, err := repo.GetMonitoredWalletByAddress(ctx, addr, 56)
		if err != nil || wal == nil {
			jsonError(w, "wallet not found", http.StatusNotFound)
			return
		}
		rows, _, err := repo.ListWalletsWithStats(ctx, 1, 1, addr, "", nil)
		if err != nil || len(rows) == 0 {
			jsonError(w, "stats not found", http.StatusNotFound)
			return
		}
		jsonOK(w, rows[0])
		return
	}

	// Global stats
	stats, err := repo.GetGlobalStats(ctx)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	status := smService.Status()
	stats.MonitorEnabled = status.MonitorEnabled
	stats.WatcherEnabled = status.WatcherEnabled
	stats.CrawlerEnabled = status.CrawlerEnabled
	jsonOK(w, stats)
}

// --- Helpers ---

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 0,
		"data": data,
	})
}

func jsonError(w http.ResponseWriter, msg string, statusCode int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    statusCode,
		"message": msg,
	})
}

func isValidAddress(addr string) bool {
	addr = strings.TrimSpace(addr)
	if len(addr) != 42 {
		return false
	}
	if !strings.HasPrefix(addr, "0x") && !strings.HasPrefix(addr, "0X") {
		return false
	}
	for _, c := range addr[2:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func extractPathParam(path, prefix string) string {
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return ""
}
