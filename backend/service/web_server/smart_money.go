package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	sm "TgLpBot/service/smart_money"
	smfollow "TgLpBot/service/smart_money_follow"
	smgd "TgLpBot/service/smart_money_golden_dog"
	smwoa "TgLpBot/service/smart_money_watch_open_alert"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	smService      *sm.Service
	smWSHub        *sm.WSHub
	smGoldenDogSvc *smgd.Service
	smWatchOpenSvc *smwoa.Service
	smFollowSvc    *smfollow.Service
)

func initSmartMoney() {
	smService = sm.NewService()
	smWSHub = sm.NewWSHub()
	smWatchOpenSvc = smwoa.NewService()
	smFollowSvc = smfollow.NewService()

	smService.SetNotifier(func(event *models.SmartMoneyLPEvent) {
		// Lookup wallet label
		repo := smService.Repo()
		w, _ := repo.GetMonitoredWalletByAddress(context.Background(), event.WalletAddress, event.ChainID)
		var label *string
		if w != nil {
			label = w.Label
		}
		smWSHub.BroadcastLPEvent(event, label)
		if smWatchOpenSvc != nil {
			go smWatchOpenSvc.HandleEvent(context.Background(), event, label)
		}
		if smFollowSvc != nil {
			go smFollowSvc.HandleEvent(context.Background(), event)
		}
	})

	smService.Start()
	smGoldenDogSvc = smgd.NewService()
	smGoldenDogSvc.Start()
	smFollowSvc.Start()
}

func stopSmartMoney() {
	if smService != nil {
		smService.Stop()
	}
	if smGoldenDogSvc != nil {
		smGoldenDogSvc.Stop()
	}
	if smFollowSvc != nil {
		smFollowSvc.Stop()
	}
}

// --- Route Registration (called from server.go) ---

func (s *Server) registerSmartMoneyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/sm", s.handleSMCompat)
	mux.HandleFunc("/api/sm_upload", s.handleSMUploadCompat)
	mux.HandleFunc("/api/sm/wallets", s.handleSMWallets)
	mux.HandleFunc("/api/sm/wallet_avatar", s.handleSMWalletAvatar)
	mux.HandleFunc("/api/sm/contracts", s.handleSMContracts)
	mux.HandleFunc("/api/sm/pools", s.handleSMPools)
	mux.HandleFunc("/api/sm/positions", s.handleSMPositions)
	mux.HandleFunc("/api/sm/position_detail", s.handleSMPositionDetail)
	mux.HandleFunc("/api/sm/events", s.handleSMEvents)
	mux.HandleFunc("/api/sm/stats", s.handleSMStats)
	mux.HandleFunc("/api/smart_money_golden_dog_config", s.handleSmartMoneyGoldenDogConfig)
	mux.HandleFunc("/api/smart_money_golden_dog_test", s.handleSmartMoneyGoldenDogTest)
	mux.HandleFunc("/api/smart_money_watch_wallets", s.handleSmartMoneyWatchWallets)
	mux.HandleFunc("/api/smart_money_watch_open_alert_config", s.handleSmartMoneyWatchOpenAlertConfig)
	mux.HandleFunc("/api/smart_money_watch_open_alert_test", s.handleSmartMoneyWatchOpenAlertTest)
	mux.HandleFunc("/api/smart_money_auto_follow", s.handleSmartMoneyAutoFollow)
	mux.HandleFunc("/ws/sm/events", smWSHub.HandleWS)
}

func (s *Server) handleSMCompat(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("endpoint")))
	if endpoint == "avatar_asset" {
		handleSMAvatarAsset(w, r)
		return
	}

	nextPath, ok := smartMoneyCompatEndpointPath(endpoint)
	if !ok {
		jsonError(w, "invalid endpoint", http.StatusBadRequest)
		return
	}
	withSmartMoneyCompatPath(r, nextPath, func() {
		switch endpoint {
		case "wallets":
			s.handleSMWallets(w, r)
		case "contracts":
			s.handleSMContracts(w, r)
		case "pools":
			s.handleSMPools(w, r)
		case "positions":
			s.handleSMPositions(w, r)
		case "position_detail":
			s.handleSMPositionDetail(w, r)
		case "events":
			s.handleSMEvents(w, r)
		case "stats":
			s.handleSMStats(w, r)
		case "auto_follow":
			s.handleSmartMoneyAutoFollow(w, r)
		}
	})
}

func (s *Server) handleSMUploadCompat(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("endpoint")))
	if endpoint != "wallet_avatar" {
		jsonError(w, "invalid endpoint", http.StatusBadRequest)
		return
	}
	withSmartMoneyCompatPath(r, "/api/sm/wallet_avatar", func() {
		s.handleSMWalletAvatar(w, r)
	})
}

func smartMoneyCompatEndpointPath(endpoint string) (string, bool) {
	switch endpoint {
	case "wallets", "contracts", "pools", "positions", "position_detail", "events", "stats":
		return "/api/sm/" + endpoint, true
	case "auto_follow":
		return "/api/smart_money_auto_follow", true
	default:
		return "", false
	}
}

func withSmartMoneyCompatPath(r *http.Request, path string, fn func()) {
	oldURL := *r.URL
	oldRequestURI := r.RequestURI
	query := oldURL.Query()
	query.Del("endpoint")
	r.URL.Path = path
	r.URL.RawQuery = query.Encode()
	r.RequestURI = path
	if r.URL.RawQuery != "" {
		r.RequestURI += "?" + r.URL.RawQuery
	}
	defer func() {
		r.URL = &oldURL
		r.RequestURI = oldRequestURI
	}()
	fn()
}

func handleSMAvatarAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
	parsed, ok := parseAllowedSmartMoneyAvatarURL(targetURL)
	if !ok {
		http.Error(w, "invalid avatar url", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, r.Method, parsed.String(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	req.Header.Set("Accept", "image/*,*/*;q=0.8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for _, name := range []string{"Content-Type", "Content-Length", "ETag", "Last-Modified"} {
		if value := resp.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		w.Header().Set("Cache-Control", "public, max-age=300")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodHead || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified {
		return
	}
	_, _ = io.Copy(w, resp.Body)
}

func parseAllowedSmartMoneyAvatarURL(raw string) (*url.URL, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, false
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, false
	}
	if !strings.HasPrefix(parsed.EscapedPath(), "/avatar/") && !strings.HasPrefix(parsed.Path, "/avatar/") {
		return nil, false
	}

	allowedHosts := make(map[string]struct{})
	if config.AppConfig != nil {
		addSmartMoneyAvatarAllowedHost(allowedHosts, config.AppConfig.MinIOPublicBaseURL)
		addSmartMoneyAvatarAllowedHost(allowedHosts, config.AppConfig.MinIOEndpoint)
	}
	host := strings.ToLower(parsed.Hostname())
	if _, ok := allowedHosts[host]; !ok {
		return nil, false
	}
	return parsed, true
}

func addSmartMoneyAvatarAllowedHost(allowedHosts map[string]struct{}, raw string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return
	}
	if !strings.HasPrefix(strings.ToLower(value), "http://") && !strings.HasPrefix(strings.ToLower(value), "https://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return
	}
	allowedHosts[strings.ToLower(parsed.Hostname())] = struct{}{}
}

// --- Wallets ---

func (s *Server) handleSMWallets(w http.ResponseWriter, r *http.Request) {
	repo := smService.Repo()
	ctx := r.Context()
	chainID := resolveSmartMoneyRequestChainID(r)

	switch r.Method {
	case http.MethodGet:
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		if page <= 0 {
			page = 1
		}
		if size <= 0 || size > 100 {
			size = 10
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
			ChainID:  chainID,
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
		allowed := map[string]bool{"label": true, "is_active": true, "avatar_url": true}
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
		if rawLabel, ok := filtered["label"]; ok {
			switch v := rawLabel.(type) {
			case nil:
				filtered["label"] = nil
			case string:
				label := strings.TrimSpace(v)
				if label == "" {
					filtered["label"] = nil
				} else {
					filtered["label"] = label
				}
			default:
				label := strings.TrimSpace(fmt.Sprint(v))
				if label == "" {
					filtered["label"] = nil
				} else {
					filtered["label"] = label
				}
			}
		}
		if rawAvatarURL, ok := filtered["avatar_url"]; ok {
			switch v := rawAvatarURL.(type) {
			case nil:
				filtered["avatar_url"] = nil
			case string:
				avatarURL := strings.TrimSpace(v)
				if avatarURL == "" {
					filtered["avatar_url"] = nil
				} else {
					filtered["avatar_url"] = avatarURL
				}
			default:
				avatarURL := strings.TrimSpace(fmt.Sprint(v))
				if avatarURL == "" {
					filtered["avatar_url"] = nil
				} else {
					filtered["avatar_url"] = avatarURL
				}
			}
		}

		existing, err := repo.GetMonitoredWalletByAddress(ctx, addr, chainID)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if existing == nil {
			labelValue, hasLabel := filtered["label"]
			_, hasActive := filtered["is_active"]
			_, hasAvatar := filtered["avatar_url"]
			if !hasLabel || hasActive || hasAvatar {
				jsonError(w, "wallet not found", http.StatusNotFound)
				return
			}
			var labelPtr *string
			if label, ok := labelValue.(string); ok && strings.TrimSpace(label) != "" {
				trimmed := strings.TrimSpace(label)
				labelPtr = &trimmed
			}
			wallet := &models.MonitoredWallet{
				Address:  strings.ToLower(addr),
				ChainID:  chainID,
				Source:   "manual",
				Label:    labelPtr,
				IsActive: false,
			}
			if err := repo.CreateMonitoredWallet(ctx, wallet); err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			jsonOK(w, map[string]interface{}{"ok": true})
			return
		}

		if err := repo.UpdateMonitoredWallet(ctx, addr, chainID, filtered); err != nil {
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
		if err := repo.DeleteMonitoredWallet(ctx, addr, chainID); err != nil {
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
		protocol := normalizeSmartMoneyProtocol(req.Protocol)
		if protocol == "" {
			protocol = "watch_contract"
		}
		addr := strings.ToLower(strings.TrimSpace(req.ContractAddress))
		existing, err := repo.GetWatchContractByAddress(ctx, addr, 56)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if existing != nil {
			jsonError(w, "contract already exists", http.StatusConflict)
			return
		}
		var desc *string
		if d := strings.TrimSpace(req.Description); d != "" {
			desc = &d
		}
		c := &models.WatchContract{
			ContractAddress: addr,
			ChainID:         56,
			Protocol:        protocol,
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
		allowed := map[string]bool{"description": true, "is_active": true}
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
		if rawDescription, ok := filtered["description"]; ok {
			if rawDescription == nil {
				filtered["description"] = nil
			} else {
				desc := strings.TrimSpace(fmt.Sprintf("%v", rawDescription))
				if desc == "" {
					filtered["description"] = nil
				} else {
					filtered["description"] = desc
				}
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
	repairSmartMoneyPositions(ctx, repo)

	poolAddr := r.URL.Query().Get("pool")
	if poolAddr != "" {
		// Single pool stats
		stats, err := repo.GetPoolStats(ctx, poolAddr)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		stats.TradingPair = buildSmartMoneyTradingPair(stats.Token0Symbol, stats.Token1Symbol)
		stats.DisplayTokenAddress, stats.DisplayTokenSymbol = smartMoneyPickDisplayToken(
			stats.Token0Address,
			stats.Token1Address,
			stats.Token0Symbol,
			stats.Token1Symbol,
		)
		stats.CurrentPrice, stats.PriceChange24h = loadSmartMoneyPoolMarketSnapshot(ctx, poolAddr)
		applySmartMoneyDisplayToken(
			smartMoneyChainSlug(stats.ChainID),
			&stats.DisplayTokenAddress,
			&stats.DisplayTokenSymbol,
			&stats.DisplayTokenLogoURL,
			s.loadSmartMoneyTokenMetadataByChain(ctx, map[string][]string{
				smartMoneyChainSlug(stats.ChainID): []string{stats.DisplayTokenAddress},
			}),
		)
		if err := attachSmartMoneyRangeGroupsToPoolStats(ctx, repo, stats); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, stats)
		return
	}

	// Pool list
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 10
	}
	keyword := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("keyword")))
	protocol := strings.TrimSpace(r.URL.Query().Get("protocol"))

	pools, err := repo.ListPoolsWithPositions(ctx)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for i := range pools {
		pools[i].TradingPair = buildSmartMoneyTradingPair(pools[i].Token0Symbol, pools[i].Token1Symbol)
		pools[i].DisplayTokenAddress, pools[i].DisplayTokenSymbol = smartMoneyPickDisplayToken(
			pools[i].Token0Address,
			pools[i].Token1Address,
			pools[i].Token0Symbol,
			pools[i].Token1Symbol,
		)
	}

	filtered := make([]sm.PoolAggRow, 0, len(pools))
	for _, pool := range pools {
		if protocol != "" && protocol != "all" && pool.Protocol != protocol {
			continue
		}
		if keyword != "" {
			pairText := strings.ToLower(strings.TrimSpace(pool.TradingPair))
			poolAddrText := strings.ToLower(strings.TrimSpace(pool.PoolAddress))
			if !strings.Contains(pairText, keyword) && !strings.Contains(poolAddrText, keyword) {
				continue
			}
		}
		filtered = append(filtered, pool)
	}

	total := len(filtered)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	pagedPools := filtered[start:end]

	addressesByChain := make(map[string][]string)
	for i := range pagedPools {
		if pagedPools[i].DisplayTokenAddress != "" {
			chain := smartMoneyChainSlug(pagedPools[i].ChainID)
			addressesByChain[chain] = append(addressesByChain[chain], pagedPools[i].DisplayTokenAddress)
		}
	}
	metaByChain := s.loadSmartMoneyTokenMetadataByChain(ctx, addressesByChain)
	for i := range pagedPools {
		applySmartMoneyDisplayToken(
			smartMoneyChainSlug(pagedPools[i].ChainID),
			&pagedPools[i].DisplayTokenAddress,
			&pagedPools[i].DisplayTokenSymbol,
			&pagedPools[i].DisplayTokenLogoURL,
			metaByChain,
		)
	}
	if err := attachSmartMoneyRangeGroupsToPoolList(ctx, repo, pagedPools); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{
		"total": total,
		"list":  pagedPools,
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
	repairSmartMoneyPositions(ctx, repo)

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
	poolsByAddress, err := smartMoneyLoadPoolsByAddress(ctx, positions)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Enrich with wallet color, label, price_lower, price_upper
	type posResp struct {
		models.SmartMoneyLPPosition
		PositionRef         string  `json:"position_ref"`
		WalletLabel         *string `json:"wallet_label"`
		WalletAvatarURL     *string `json:"wallet_avatar_url"`
		WalletColor         string  `json:"wallet_color"`
		PriceLower          string  `json:"price_lower"`
		PriceUpper          string  `json:"price_upper"`
		RangePercent        float64 `json:"range_percent"`
		PositionAmountUSD   float64 `json:"position_amount_usd"`
		BscscanURL          string  `json:"bscscan_url"`
		TradingPair         string  `json:"trading_pair"`
		DisplayTokenAddress string  `json:"display_token_address,omitempty"`
		DisplayTokenSymbol  string  `json:"display_token_symbol,omitempty"`
		DisplayTokenLogoURL string  `json:"display_token_logo_url,omitempty"`
	}

	list := make([]posResp, 0, len(positions))
	addressesByChain := make(map[string][]string)
	amountsByChain, err := repo.GetPositionOpenAmountsUSD(ctx, positions)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Cache wallet labels
	walletCache := make(map[string]*models.MonitoredWallet)
	for _, p := range positions {
		resp := posResp{
			SmartMoneyLPPosition: p,
			PositionRef:          sm.BuildPositionRefFromPosition(&p),
			WalletColor:          sm.WalletColor(p.WalletAddress),
			BscscanURL:           "https://bscscan.com/tx/" + p.OpenTxHash,
		}
		resp.DisplayTokenAddress, resp.DisplayTokenSymbol = smartMoneyPickDisplayToken(
			p.Token0Address,
			p.Token1Address,
			p.Token0Symbol,
			p.Token1Symbol,
		)

		// Wallet label
		if w, ok := walletCache[p.WalletAddress]; ok {
			if w != nil {
				resp.WalletLabel = w.Label
				resp.WalletAvatarURL = w.AvatarURL
			}
		} else {
			w, _ := repo.GetMonitoredWalletByAddress(ctx, p.WalletAddress, p.ChainID)
			walletCache[p.WalletAddress] = w
			if w != nil {
				resp.WalletLabel = w.Label
				resp.WalletAvatarURL = w.AvatarURL
			}
		}

		poolMeta := poolsByAddress[strings.ToLower(strings.TrimSpace(p.PoolAddress))]
		resp.PriceLower, resp.PriceUpper = smartMoneyFormatPositionPriceBounds(
			p.TickLower,
			p.TickUpper,
			poolMeta.Token0Decimals,
			poolMeta.Token1Decimals,
			smartMoneyDisplayTokenUsesToken1(
				resp.DisplayTokenAddress,
				resp.DisplayTokenSymbol,
				p.Token0Address,
				p.Token1Address,
				p.Token0Symbol,
				p.Token1Symbol,
			),
		)
		resp.RangePercent = smartMoneyRangePercentFromTicks(p.TickLower, p.TickUpper)
		if byNFT, ok := amountsByChain[p.ChainID]; ok {
			resp.PositionAmountUSD = byNFT[p.NftTokenID]
		}
		resp.TradingPair = buildSmartMoneyTradingPair(p.Token0Symbol, p.Token1Symbol)
		if resp.DisplayTokenAddress != "" {
			chain := smartMoneyChainSlug(p.ChainID)
			addressesByChain[chain] = append(addressesByChain[chain], resp.DisplayTokenAddress)
		}

		list = append(list, resp)
	}
	metaByChain := s.loadSmartMoneyTokenMetadataByChain(ctx, addressesByChain)
	for i := range list {
		applySmartMoneyDisplayToken(
			smartMoneyChainSlug(list[i].ChainID),
			&list[i].DisplayTokenAddress,
			&list[i].DisplayTokenSymbol,
			&list[i].DisplayTokenLogoURL,
			metaByChain,
		)
	}

	jsonOK(w, map[string]interface{}{
		"total": total,
		"page":  page,
		"size":  size,
		"list":  list,
	})
}

func (s *Server) handleSMPositionDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repo := smService.Repo()
	ctx := r.Context()
	repairSmartMoneyPositions(ctx, repo)

	q := r.URL.Query()
	positionRef := sm.NormalizePositionRef(q.Get("position_ref"))
	rawPositionID := strings.TrimSpace(q.Get("position_id"))

	active, err := repo.GetActivePositionByRef(ctx, positionRef)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if active == nil && rawPositionID != "" {
		positionID, parseErr := strconv.ParseUint(rawPositionID, 10, 64)
		if parseErr != nil || positionID == 0 {
			jsonError(w, "invalid position_id", http.StatusBadRequest)
			return
		}
		pos, err := repo.GetPositionByID(ctx, uint(positionID))
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if pos == nil {
			jsonError(w, "position not found", http.StatusNotFound)
			return
		}
		active, err = repo.EnsureActivePositionFromPosition(ctx, pos)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if active == nil {
		jsonError(w, "position detail not found", http.StatusNotFound)
		return
	}

	detail, err := s.Realtime.GetSmartMoneyPositionDetail(active)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, detail)
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
		s.attachSmartMoneyWalletBalances(ctx, rows, true)
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

func repairSmartMoneyPositions(ctx context.Context, repo *sm.Repository) {
	if err := sm.RepairPositions(ctx, repo); err != nil {
		log.Printf("[SmartMoney API] repair position metadata failed: %v", err)
	}
}

func normalizeSmartMoneyProtocol(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pcsv3", "pancakev3", "pancakeswap_v3", "pancake_v3":
		return "pancake_v3"
	case "univ3", "uniswapv3", "uniswap_v3":
		return "uniswap_v3"
	case "univ4", "uniswapv4", "uniswap_v4":
		return "uniswap_v4"
	default:
		return ""
	}
}

func buildSmartMoneyTradingPair(token0Symbol string, token1Symbol string) string {
	left := strings.TrimSpace(token0Symbol)
	right := strings.TrimSpace(token1Symbol)
	switch {
	case left != "" && right != "":
		return left + "/" + right
	case left != "":
		return left
	case right != "":
		return right
	default:
		return ""
	}
}

func loadSmartMoneyPoolMarketSnapshot(ctx context.Context, poolAddress string) (string, float64) {
	poolAddress = strings.ToLower(strings.TrimSpace(poolAddress))
	if poolAddress == "" || database.DB == nil {
		return "", 0
	}

	var row models.Pool
	if err := database.DB.WithContext(ctx).
		Model(&models.Pool{}).
		Where("address = ?", poolAddress).
		First(&row).Error; err != nil {
		return "", 0
	}

	priceDisplay := strings.TrimSpace(row.PriceDisplay)
	if priceDisplay == "" {
		priceDisplay = formatPoolCatalogPrice(firstPositiveFloat(row.CurrentTokenPrice, row.BaseTokenPriceUSD))
	}

	priceChange := row.PriceChangeH24
	if priceChange == 0 {
		priceChange = metricTrendPriceChange(rawJSONFromString(row.MetricTrendsJSON, "[]"))
	}

	return priceDisplay, priceChange
}

var smartMoneyStableSymbols = map[string]struct{}{
	"usdc":  {},
	"usdt":  {},
	"busd":  {},
	"dai":   {},
	"frax":  {},
	"usdd":  {},
	"fdusd": {},
	"wbnb":  {},
	"weth":  {},
	"wsol":  {},
	"bnb":   {},
	"eth":   {},
	"sol":   {},
}

func resolveSmartMoneyRequestChainID(r *http.Request) int {
	if r == nil {
		return 56
	}
	query := r.URL.Query()
	if raw := strings.TrimSpace(query.Get("chain_id")); raw != "" {
		if chainID, err := strconv.Atoi(raw); err == nil && chainID > 0 {
			return chainID
		}
	}
	switch config.NormalizeChain(query.Get("chain")) {
	case "base":
		return 8453
	default:
		return 56
	}
}

func smartMoneyChainSlug(chainID int) string {
	switch chainID {
	case 8453:
		return "base"
	default:
		return "bsc"
	}
}

func smartMoneyNormalizeTokenAddress(value string) string {
	value = strings.TrimSpace(value)
	if !isValidAddress(value) {
		return ""
	}
	return strings.ToLower(value)
}

func smartMoneyIsStableLikeSymbol(symbol string) bool {
	_, ok := smartMoneyStableSymbols[strings.ToLower(strings.TrimSpace(symbol))]
	return ok
}

func smartMoneyPickDisplayToken(token0Address string, token1Address string, token0Symbol string, token1Symbol string) (string, string) {
	token0Address = smartMoneyNormalizeTokenAddress(token0Address)
	token1Address = smartMoneyNormalizeTokenAddress(token1Address)
	token0Symbol = strings.TrimSpace(token0Symbol)
	token1Symbol = strings.TrimSpace(token1Symbol)

	token0Stable := smartMoneyIsStableLikeSymbol(token0Symbol)
	token1Stable := smartMoneyIsStableLikeSymbol(token1Symbol)

	switch {
	case token0Stable && !token1Stable:
		return firstSmartMoneyDisplayToken(token1Address, token1Symbol, token0Address, token0Symbol)
	case token1Stable && !token0Stable:
		return firstSmartMoneyDisplayToken(token0Address, token0Symbol, token1Address, token1Symbol)
	default:
		return firstSmartMoneyDisplayToken(token0Address, token0Symbol, token1Address, token1Symbol)
	}
}

func smartMoneyLoadPoolsByAddress(ctx context.Context, positions []models.SmartMoneyLPPosition) (map[string]models.Pool, error) {
	out := make(map[string]models.Pool)
	if len(positions) == 0 {
		return out, nil
	}

	seen := make(map[string]struct{}, len(positions))
	addresses := make([]string, 0, len(positions))
	for _, pos := range positions {
		addr := strings.ToLower(strings.TrimSpace(pos.PoolAddress))
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		addresses = append(addresses, addr)
	}
	if len(addresses) == 0 {
		return out, nil
	}

	var pools []models.Pool
	if err := database.DB.WithContext(ctx).
		Model(&models.Pool{}).
		Where("address IN ?", addresses).
		Find(&pools).Error; err != nil {
		return nil, err
	}

	for _, pool := range pools {
		addr := strings.ToLower(strings.TrimSpace(pool.Address))
		if addr == "" {
			continue
		}
		out[addr] = pool
	}
	return out, nil
}

func smartMoneyTokenDecimalsOrDefault(decimals int) int {
	if decimals > 0 {
		return decimals
	}
	return 18
}

func smartMoneyFormatPositionPriceBounds(tickLower *int, tickUpper *int, token0Decimals int, token1Decimals int, invert bool) (string, string) {
	if tickLower == nil && tickUpper == nil {
		return "", ""
	}

	dec0 := smartMoneyTokenDecimalsOrDefault(token0Decimals)
	dec1 := smartMoneyTokenDecimalsOrDefault(token1Decimals)

	formatTick := func(tick *int) string {
		if tick == nil {
			return ""
		}
		price := sm.TickToPrice(*tick, dec0, dec1)
		if invert {
			if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
				return ""
			}
			price = 1 / price
		}
		if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			return ""
		}
		return fmt.Sprintf("%.6g", price)
	}

	if invert && tickLower != nil && tickUpper != nil {
		lower := sm.TickToPrice(*tickLower, dec0, dec1)
		upper := sm.TickToPrice(*tickUpper, dec0, dec1)
		if lower <= 0 || upper <= 0 || math.IsNaN(lower) || math.IsNaN(upper) || math.IsInf(lower, 0) || math.IsInf(upper, 0) {
			return "", ""
		}
		return fmt.Sprintf("%.6g", 1/upper), fmt.Sprintf("%.6g", 1/lower)
	}

	return formatTick(tickLower), formatTick(tickUpper)
}

func smartMoneyDisplayTokenUsesToken1(displayTokenAddress string, displayTokenSymbol string, token0Address string, token1Address string, token0Symbol string, token1Symbol string) bool {
	displayAddr := smartMoneyNormalizeTokenAddress(displayTokenAddress)
	token0Addr := smartMoneyNormalizeTokenAddress(token0Address)
	token1Addr := smartMoneyNormalizeTokenAddress(token1Address)
	if displayAddr != "" {
		return displayAddr == token1Addr && displayAddr != token0Addr
	}

	displayTokenSymbol = strings.TrimSpace(displayTokenSymbol)
	token0Symbol = strings.TrimSpace(token0Symbol)
	token1Symbol = strings.TrimSpace(token1Symbol)
	return displayTokenSymbol != "" &&
		token1Symbol != "" &&
		strings.EqualFold(displayTokenSymbol, token1Symbol) &&
		!strings.EqualFold(token0Symbol, token1Symbol)
}

func firstSmartMoneyDisplayToken(primaryAddress string, primarySymbol string, fallbackAddress string, fallbackSymbol string) (string, string) {
	if primaryAddress != "" || primarySymbol != "" {
		return primaryAddress, primarySymbol
	}
	return fallbackAddress, fallbackSymbol
}

func (s *Server) loadSmartMoneyTokenMetadataByChain(ctx context.Context, addressesByChain map[string][]string) map[string]map[string]models.TokenMetadata {
	out := make(map[string]map[string]models.TokenMetadata, len(addressesByChain))
	if s == nil || s.TokenMeta == nil {
		return out
	}

	for chain, addresses := range addressesByChain {
		normalized := make([]string, 0, len(addresses))
		seen := make(map[string]struct{}, len(addresses))
		for _, raw := range addresses {
			addr := smartMoneyNormalizeTokenAddress(raw)
			if addr == "" {
				continue
			}
			if _, ok := seen[addr]; ok {
				continue
			}
			seen[addr] = struct{}{}
			normalized = append(normalized, addr)
		}
		if len(normalized) == 0 {
			continue
		}

		meta, err := s.TokenMeta.GetBatch(ctx, chain, normalized)
		if err != nil {
			log.Printf("[SmartMoney API] load token metadata failed chain=%s err=%v", chain, err)
		}
		if len(meta) == 0 {
			continue
		}
		out[chain] = meta
	}

	return out
}

func applySmartMoneyDisplayToken(chain string, displayAddress *string, displaySymbol *string, displayLogoURL *string, metaByChain map[string]map[string]models.TokenMetadata) {
	if displayAddress == nil || displaySymbol == nil || displayLogoURL == nil {
		return
	}

	addr := smartMoneyNormalizeTokenAddress(*displayAddress)
	if addr == "" {
		return
	}

	meta := metaByChain[chain][addr]
	*displayAddress = addr
	if strings.TrimSpace(*displaySymbol) == "" {
		*displaySymbol = strings.TrimSpace(meta.Symbol)
	}
	if strings.TrimSpace(meta.LogoURL) != "" {
		*displayLogoURL = strings.TrimSpace(meta.LogoURL)
	}
}

func smartMoneyRangePercentFromTicks(tickLower *int, tickUpper *int) float64 {
	if tickLower == nil || tickUpper == nil {
		return 0
	}

	lower := *tickLower
	upper := *tickUpper
	if upper <= lower {
		return 0
	}

	lowerPrice := math.Pow(1.0001, float64(lower))
	upperPrice := math.Pow(1.0001, float64(upper))
	if lowerPrice <= 0 || upperPrice <= 0 || math.IsNaN(lowerPrice) || math.IsNaN(upperPrice) || math.IsInf(lowerPrice, 0) || math.IsInf(upperPrice, 0) {
		return 0
	}

	// Use half-width around the range midpoint so the displayed value matches a true "±range".
	pct := ((upperPrice - lowerPrice) / (upperPrice + lowerPrice)) * 100.0
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		pct = 100
	}
	return math.Round(pct*10) / 10
}

func buildSmartMoneyPoolRangeGroups(rows []sm.PoolPositionRangeRow) map[string][]sm.PoolRangeGroup {
	byPool := make(map[string]map[string]*sm.PoolRangeGroup)

	for _, row := range rows {
		poolAddress := strings.ToLower(strings.TrimSpace(row.PoolAddress))
		if poolAddress == "" {
			continue
		}
		rangePercent := smartMoneyRangePercentFromTicks(row.TickLower, row.TickUpper)
		if rangePercent <= 0 {
			continue
		}
		rangePercent = math.Round(rangePercent*10) / 10
		rangeKey := strconv.FormatFloat(rangePercent, 'f', 1, 64)

		if _, ok := byPool[poolAddress]; !ok {
			byPool[poolAddress] = make(map[string]*sm.PoolRangeGroup)
		}
		group, ok := byPool[poolAddress][rangeKey]
		if !ok {
			group = &sm.PoolRangeGroup{RangePercent: rangePercent}
			byPool[poolAddress][rangeKey] = group
		}
		group.PositionCount += row.PositionCount
		group.TotalAmountUSD += row.TotalAmountUSD
	}

	out := make(map[string][]sm.PoolRangeGroup, len(byPool))
	for poolAddress, groups := range byPool {
		list := make([]sm.PoolRangeGroup, 0, len(groups))
		for _, group := range groups {
			if group == nil || group.PositionCount <= 0 {
				continue
			}
			list = append(list, *group)
		}
		sort.Slice(list, func(i, j int) bool {
			if math.Abs(list[i].TotalAmountUSD-list[j].TotalAmountUSD) > 0.0001 {
				return list[i].TotalAmountUSD > list[j].TotalAmountUSD
			}
			if list[i].PositionCount != list[j].PositionCount {
				return list[i].PositionCount > list[j].PositionCount
			}
			return list[i].RangePercent < list[j].RangePercent
		})
		out[poolAddress] = list
	}

	return out
}

func attachSmartMoneyRangeGroupsToPoolList(ctx context.Context, repo *sm.Repository, pools []sm.PoolAggRow) error {
	if repo == nil || len(pools) == 0 {
		return nil
	}

	poolAddresses := make([]string, 0, len(pools))
	for _, pool := range pools {
		addr := strings.ToLower(strings.TrimSpace(pool.PoolAddress))
		if addr == "" {
			continue
		}
		poolAddresses = append(poolAddresses, addr)
	}

	rangeRows, err := repo.ListRecentOpenPositionRanges(ctx, poolAddresses)
	if err != nil {
		return err
	}
	rangeGroups := buildSmartMoneyPoolRangeGroups(rangeRows)
	for i := range pools {
		pools[i].RangeGroups = rangeGroups[strings.ToLower(strings.TrimSpace(pools[i].PoolAddress))]
	}
	return nil
}

func attachSmartMoneyRangeGroupsToPoolStats(ctx context.Context, repo *sm.Repository, stats *sm.PoolStats) error {
	if repo == nil || stats == nil {
		return nil
	}

	addr := strings.ToLower(strings.TrimSpace(stats.PoolAddress))
	if addr == "" {
		return nil
	}

	rangeRows, err := repo.ListRecentOpenPositionRanges(ctx, []string{addr})
	if err != nil {
		return err
	}
	stats.RangeGroups = buildSmartMoneyPoolRangeGroups(rangeRows)[addr]
	return nil
}

func (s *Server) attachSmartMoneyWalletBalances(ctx context.Context, rows []sm.WalletStatsRow, forceRefresh bool) {
	if s == nil || s.Assets == nil || len(rows) == 0 {
		return
	}

	for i := range rows {
		balance, err := s.Assets.GetSmartMoneyWalletBalance(ctx, rows[i].Address, rows[i].ChainID, forceRefresh)
		if err != nil {
			log.Printf("[SmartMoney API] enrich wallet balance failed wallet=%s chain=%d err=%v", rows[i].Address, rows[i].ChainID, err)
			continue
		}
		rows[i].WalletBalanceUSD = balance
	}
}
