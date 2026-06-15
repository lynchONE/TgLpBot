package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/pricing"
	sm "TgLpBot/service/smart_money"
	smfollow "TgLpBot/service/smart_money_follow"
	smgd "TgLpBot/service/smart_money_golden_dog"
	smwoa "TgLpBot/service/smart_money_watch_open_alert"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	smService      *sm.Service
	smWSHub        *sm.WSHub
	smGoldenDogSvc *smgd.Service
	smWatchOpenSvc *smwoa.Service
	smFollowSvc    *smfollow.Service

	smPositionRepairRunning int32
)

const (
	smartMoneyPoolLiquidityScanTimeout     = 50 * time.Second
	smartMoneyPoolLiquidityAnnotationLimit = 3 * time.Second
)

func initSmartMoney() {
	smService = sm.NewService()
	smWSHub = sm.NewWSHub()
	smWatchOpenSvc = smwoa.NewService()
	smFollowSvc = smfollow.NewService()

	smService.SetNotifier(func(event *models.SmartMoneyLPEvent) {
		repo := smService.Repo()
		w, err := repo.GetMonitoredWalletByAddress(context.Background(), event.WalletAddress, event.ChainID)
		if err != nil {
			log.Printf("[SmartMoney WS] load wallet metadata failed wallet=%s chain=%d err=%v", event.WalletAddress, event.ChainID, err)
		}
		var label *string
		source := ""
		sourceContract := ""
		if w != nil {
			label = w.Label
			source = smartMoneyWalletSourceValue(w)
			sourceContract = smartMoneyWalletSourceContractValue(w)
		}
		smWSHub.BroadcastLPEvent(event, label, source, sourceContract)
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
	mux.HandleFunc("/api/sm/wallet_zombies", s.handleSMWalletZombies)
	mux.HandleFunc("/api/sm/pool_liquidity_wallet_candidates", s.handleSMPoolLiquidityWalletCandidates)
	mux.HandleFunc("/api/sm/pool_liquidity_wallet_candidates_stream", s.handleSMPoolLiquidityWalletCandidatesStream)
	mux.HandleFunc("/api/sm/pool_liquidity_wallet_import", s.handleSMPoolLiquidityWalletImport)
	mux.HandleFunc("/api/sm/token_liquidity_wallet_candidates", s.handleSMTokenLiquidityWalletCandidates)
	mux.HandleFunc("/api/sm/token_liquidity_wallet_import", s.handleSMTokenLiquidityWalletImport)
	mux.HandleFunc("/api/sm/wallet_avatar", s.handleSMWalletAvatar)
	mux.HandleFunc("/api/sm/contracts", s.handleSMContracts)
	mux.HandleFunc("/api/sm/pools", s.handleSMPools)
	mux.HandleFunc("/api/sm/pool_fee_heatmap", s.handleSMPoolFeeHeatmap)
	mux.HandleFunc("/api/sm/positions", s.handleSMPositions)
	mux.HandleFunc("/api/sm/position_detail", s.handleSMPositionDetail)
	mux.HandleFunc("/api/sm/events", s.handleSMEvents)
	mux.HandleFunc("/api/sm/stats", s.handleSMStats)
	mux.HandleFunc("/api/smart_money_golden_dog_config", s.handleSmartMoneyGoldenDogConfig)
	mux.HandleFunc("/api/smart_money_golden_dog_test", s.handleSmartMoneyGoldenDogTest)
	mux.HandleFunc("/api/smart_money_watch_wallets", s.handleSmartMoneyWatchWallets)
	mux.HandleFunc("/api/smart_money_watch_activity", s.handleSmartMoneyWatchActivity)
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
		case "wallet_zombies":
			s.handleSMWalletZombies(w, r)
		case "pool_liquidity_wallet_candidates":
			s.handleSMPoolLiquidityWalletCandidates(w, r)
		case "pool_liquidity_wallet_candidates_stream":
			s.handleSMPoolLiquidityWalletCandidatesStream(w, r)
		case "pool_liquidity_wallet_import":
			s.handleSMPoolLiquidityWalletImport(w, r)
		case "token_liquidity_wallet_candidates":
			s.handleSMTokenLiquidityWalletCandidates(w, r)
		case "token_liquidity_wallet_import":
			s.handleSMTokenLiquidityWalletImport(w, r)
		case "contracts":
			s.handleSMContracts(w, r)
		case "pools":
			s.handleSMPools(w, r)
		case "pool_fee_heatmap":
			s.handleSMPoolFeeHeatmap(w, r)
		case "positions":
			s.handleSMPositions(w, r)
		case "position_detail":
			s.handleSMPositionDetail(w, r)
		case "events":
			s.handleSMEvents(w, r)
		case "stats":
			s.handleSMStats(w, r)
		case "watch_activity":
			s.handleSmartMoneyWatchActivity(w, r)
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
	case "wallets", "wallet_zombies", "pool_liquidity_wallet_candidates", "pool_liquidity_wallet_candidates_stream", "pool_liquidity_wallet_import", "token_liquidity_wallet_candidates", "token_liquidity_wallet_import", "contracts", "pools", "pool_fee_heatmap", "positions", "position_detail", "events", "stats":
		return "/api/sm/" + endpoint, true
	case "watch_activity":
		return "/api/smart_money_watch_activity", true
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
		wallets := []sm.WalletRef{{Address: strings.ToLower(strings.TrimSpace(addr)), ChainID: chainID}}
		if _, err := repo.DeleteMonitoredWalletsWithHistory(ctx, wallets); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if s.Assets != nil {
			s.Assets.InvalidateSmartMoneyWalletCaches(wallets)
		}
		jsonOK(w, map[string]interface{}{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSMWalletZombies(w http.ResponseWriter, r *http.Request) {
	repo := smService.Repo()
	ctx := r.Context()
	chainID := resolveSmartMoneyRequestChainID(r)

	switch r.Method {
	case http.MethodGet:
		days, _ := strconv.Atoi(r.URL.Query().Get("days"))
		if days <= 0 {
			days = 30
		}
		if days > 365 {
			jsonError(w, "days too large", http.StatusBadRequest)
			return
		}
		rows, err := repo.ListZombieWalletCandidates(ctx, days, chainID)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]interface{}{
			"days":     days,
			"chain_id": chainID,
			"total":    len(rows),
			"list":     rows,
		})

	case http.MethodDelete:
		var req struct {
			Wallets []struct {
				Address string `json:"address"`
				ChainID int    `json:"chain_id"`
			} `json:"wallets"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		wallets := make([]sm.WalletRef, 0, len(req.Wallets))
		for _, item := range req.Wallets {
			if !isValidAddress(item.Address) {
				jsonError(w, "invalid wallet address", http.StatusBadRequest)
				return
			}
			itemChainID := item.ChainID
			if itemChainID <= 0 {
				itemChainID = chainID
			}
			wallets = append(wallets, sm.WalletRef{
				Address: strings.ToLower(strings.TrimSpace(item.Address)),
				ChainID: itemChainID,
			})
		}
		if len(wallets) == 0 {
			jsonError(w, "wallets is required", http.StatusBadRequest)
			return
		}
		deleted, err := repo.DeleteMonitoredWalletsWithHistory(ctx, wallets)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if s.Assets != nil {
			s.Assets.InvalidateSmartMoneyWalletCaches(wallets)
		}
		jsonOK(w, map[string]interface{}{
			"ok":      true,
			"deleted": deleted,
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSMTokenLiquidityWalletCandidates(w http.ResponseWriter, r *http.Request) {
	s.handleSMPoolLiquidityWalletCandidates(w, r)
}

func parseSMPoolLiquidityCandidateQuery(r *http.Request) (sm.TokenLiquidityCandidateQuery, int, error) {
	if r == nil {
		return sm.TokenLiquidityCandidateQuery{}, 0, fmt.Errorf("request is nil")
	}
	q := r.URL.Query()
	chainID := resolveSmartMoneyRequestChainID(r)
	chain := smartMoneyChainSlug(chainID)
	poolAddress := smartMoneyNormalizePoolIdentifier(q.Get("pool_address"))
	poolID := smartMoneyNormalizePoolID(q.Get("pool_id"))
	if poolAddress == "" && poolID == "" && strings.TrimSpace(q.Get("token_address")) != "" {
		return sm.TokenLiquidityCandidateQuery{}, chainID, fmt.Errorf("token_address is no longer supported; use pool_address or pool_id")
	}
	if poolAddress == "" && poolID == "" {
		return sm.TokenLiquidityCandidateQuery{}, chainID, fmt.Errorf("pool_address or pool_id is required")
	}
	if poolAddress != "" && poolID != "" {
		return sm.TokenLiquidityCandidateQuery{}, chainID, fmt.Errorf("pool_address and pool_id cannot both be set")
	}
	minAmountUSD, err := strconv.ParseFloat(strings.TrimSpace(q.Get("min_amount_usd")), 64)
	if err != nil || minAmountUSD <= 0 {
		return sm.TokenLiquidityCandidateQuery{}, chainID, fmt.Errorf("min_amount_usd must be greater than 0")
	}
	startTime, endTime, windowHours, err := parseSMTokenLiquidityTimeRange(q)
	if err != nil {
		return sm.TokenLiquidityCandidateQuery{}, chainID, err
	}
	limit, err := strconv.Atoi(strings.TrimSpace(q.Get("limit")))
	if err != nil || limit <= 0 {
		return sm.TokenLiquidityCandidateQuery{}, chainID, fmt.Errorf("limit must be greater than 0")
	}
	providerName := strings.TrimSpace(q.Get("provider"))
	if providerName != "" {
		return sm.TokenLiquidityCandidateQuery{}, chainID, fmt.Errorf("provider is no longer supported")
	}
	return sm.TokenLiquidityCandidateQuery{
		Chain:        chain,
		ChainID:      chainID,
		PoolAddress:  poolAddress,
		PoolID:       poolID,
		MinAmountUSD: minAmountUSD,
		WindowHours:  windowHours,
		StartTime:    startTime,
		EndTime:      endTime,
		Limit:        limit,
		Provider:     providerName,
	}, chainID, nil
}

func (s *Server) handleSMPoolLiquidityWalletCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if smService == nil {
		jsonError(w, "smart money service not initialized", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	query, chainID, err := parseSMPoolLiquidityCandidateQuery(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	provider, err := sm.NewTokenLiquidityProviderFromConfig(config.AppConfig)
	if err != nil {
		jsonError(w, smartMoneyPoolLiquidityConfigErrorMessage(err), http.StatusBadRequest)
		return
	}
	scanCtx, cancel := context.WithTimeout(ctx, smartMoneyPoolLiquidityScanTimeout)
	defer cancel()
	resp, err := provider.FindCandidates(scanCtx, query)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(scanCtx.Err(), context.DeadlineExceeded) {
			jsonError(w, "扫描超时，请缩小时间范围后重试", http.StatusGatewayTimeout)
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(scanCtx.Err(), context.Canceled) {
			jsonError(w, "扫描请求已取消，请重试", http.StatusRequestTimeout)
			return
		}
		jsonError(w, smartMoneyPoolLiquidityScanErrorMessage(err, http.StatusBadGateway), http.StatusBadGateway)
		return
	}

	repo := smService.Repo()
	if repo != nil && resp != nil {
		annotationCtx, annotationCancel := context.WithTimeout(ctx, smartMoneyPoolLiquidityAnnotationLimit)
		defer annotationCancel()
		activeWallets, err := repo.GetAllActiveWalletAddresses(annotationCtx, chainID)
		if err != nil {
			resp.Partial = true
			resp.Warnings = append(resp.Warnings, "已返回候选钱包，但未能标记是否已在监控列表中")
		} else {
			for i := range resp.Candidates {
				addr := strings.ToLower(strings.TrimSpace(resp.Candidates[i].WalletAddress))
				_, resp.Candidates[i].AlreadyMonitored = activeWallets[addr]
			}
		}
	}
	jsonOK(w, resp)
}

func (s *Server) handleSMPoolLiquidityWalletCandidatesStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if smService == nil {
		jsonError(w, "smart money service not initialized", http.StatusInternalServerError)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	query, chainID, err := parseSMPoolLiquidityCandidateQuery(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	provider, err := sm.NewTokenLiquidityProviderFromConfig(config.AppConfig)
	if err != nil {
		jsonError(w, smartMoneyPoolLiquidityConfigErrorMessage(err), http.StatusBadRequest)
		return
	}
	streamProvider, ok := provider.(sm.TokenLiquidityStreamProvider)
	if !ok {
		jsonError(w, "liquidity provider does not support streaming", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	writeEvent := func(event string, payload interface{}) error {
		return writeSmartMoneySSEEvent(w, flusher, event, payload)
	}
	if err := writeEvent("stage", sm.TokenLiquidityStreamStage{
		Stage:   "accepted",
		Message: "已接收扫描请求",
	}); err != nil {
		return
	}

	activeWallets := map[string]struct{}{}
	if repo := smService.Repo(); repo != nil {
		annotationCtx, annotationCancel := context.WithTimeout(ctx, smartMoneyPoolLiquidityAnnotationLimit)
		wallets, err := repo.GetAllActiveWalletAddresses(annotationCtx, chainID)
		annotationCancel()
		if err != nil {
			_ = writeEvent("warning", sm.TokenLiquidityStreamWarning{
				Message: "已开始扫描，但未能标记是否已在监控列表中",
				Code:    "annotation_failed",
			})
		} else {
			activeWallets = wallets
		}
	}

	scanCtx, cancel := context.WithTimeout(ctx, smartMoneyPoolLiquidityScanTimeout)
	defer cancel()
	resp, err := streamProvider.StreamCandidates(scanCtx, query, sm.TokenLiquidityCandidateStreamCallbacks{
		Stage: func(event sm.TokenLiquidityStreamStage) error {
			return writeEvent("stage", event)
		},
		Candidate: func(event sm.TokenLiquidityStreamCandidate) error {
			addr := strings.ToLower(strings.TrimSpace(event.Candidate.WalletAddress))
			_, event.Candidate.AlreadyMonitored = activeWallets[addr]
			return writeEvent("candidate", event)
		},
		Warning: func(event sm.TokenLiquidityStreamWarning) error {
			return writeEvent("warning", event)
		},
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(scanCtx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			_ = writeEvent("error", map[string]interface{}{
				"message":     "扫描请求已取消",
				"recoverable": true,
			})
			return
		}
		message := smartMoneyPoolLiquidityScanErrorMessage(err, http.StatusBadGateway)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(scanCtx.Err(), context.DeadlineExceeded) {
			message = "扫描超时，请缩小时间范围后重试"
		}
		_ = writeEvent("error", map[string]interface{}{
			"message":     message,
			"recoverable": true,
		})
		return
	}
	if resp == nil {
		resp = &sm.TokenLiquidityCandidateResponse{}
	}
	for i := range resp.Candidates {
		addr := strings.ToLower(strings.TrimSpace(resp.Candidates[i].WalletAddress))
		_, resp.Candidates[i].AlreadyMonitored = activeWallets[addr]
	}
	if err := writeEvent("summary", map[string]interface{}{
		"candidate_count": len(resp.Candidates),
		"excluded_count":  resp.ExcludedCount,
		"partial":         resp.Partial,
		"warnings":        resp.Warnings,
		"response":        resp,
	}); err != nil {
		return
	}
	_ = writeEvent("done", map[string]interface{}{
		"partial": resp.Partial,
	})
}

func writeSmartMoneySSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, payload interface{}) error {
	if w == nil || flusher == nil {
		return fmt.Errorf("sse writer is not initialized")
	}
	event = strings.TrimSpace(event)
	if event == "" {
		return fmt.Errorf("sse event name is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, body); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func parseSMTokenLiquidityTimeRange(q url.Values) (time.Time, time.Time, int, error) {
	startText := strings.TrimSpace(q.Get("start_time"))
	endText := strings.TrimSpace(q.Get("end_time"))
	if startText != "" || endText != "" {
		if startText == "" || endText == "" {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("start_time and end_time are both required")
		}
		startTime, err := time.Parse(time.RFC3339, startText)
		if err != nil {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid start_time")
		}
		endTime, err := time.Parse(time.RFC3339, endText)
		if err != nil {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid end_time")
		}
		startTime = startTime.UTC()
		endTime = endTime.UTC()
		if !endTime.After(startTime) {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("end_time must be after start_time")
		}
		if endTime.Sub(startTime) > 7*24*time.Hour {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("time range is too large")
		}
		return startTime, endTime, int(math.Ceil(endTime.Sub(startTime).Hours())), nil
	}

	windowHours, err := strconv.Atoi(strings.TrimSpace(q.Get("window_hours")))
	if err != nil || windowHours <= 0 {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("window_hours must be greater than 0")
	}
	if windowHours > 7*24 {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("window_hours is too large")
	}
	return time.Time{}, time.Time{}, windowHours, nil
}

func smartMoneyPoolLiquidityConfigErrorMessage(err error) string {
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return "smart money radar scan failed"
	}
	return msg
}

func smartMoneyPoolLiquidityScanErrorMessage(err error, statusCode int) string {
	msg := ""
	if err != nil {
		msg = strings.TrimSpace(err.Error())
	}
	if msg == "" {
		return "聪明钱雷达扫描失败，请稍后重试"
	}

	normalized := strings.ToLower(strings.Join(strings.Fields(msg), " "))
	if statusCode == http.StatusGatewayTimeout ||
		strings.Contains(normalized, "gateway time-out") ||
		strings.Contains(normalized, "gateway timeout") ||
		strings.Contains(normalized, "context deadline exceeded") ||
		strings.Contains(normalized, "timeout") ||
		strings.Contains(normalized, "504") {
		return "扫描服务超时（504 Gateway Timeout），请缩小时间范围后重试。"
	}
	if strings.Contains(normalized, "bad gateway") ||
		strings.Contains(normalized, "502") {
		return "扫描服务暂时不可用（502 Bad Gateway），请稍后重试。"
	}
	if strings.Contains(normalized, "cloudflare") ||
		strings.Contains(normalized, "cdn-cgi") ||
		strings.Contains(normalized, "cf-error") ||
		strings.Contains(normalized, "<!doctype") ||
		strings.Contains(normalized, "<html") {
		if statusCode > 0 {
			return fmt.Sprintf("请求失败（HTTP %d），服务返回了非 JSON 页面。", statusCode)
		}
		return "请求失败，服务返回了非 JSON 页面。"
	}
	if len(msg) > 240 {
		return msg[:240] + "..."
	}
	return msg
}

func (s *Server) handleSMTokenLiquidityWalletImport(w http.ResponseWriter, r *http.Request) {
	s.handleSMPoolLiquidityWalletImport(w, r)
}

func (s *Server) handleSMPoolLiquidityWalletImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if smService == nil {
		jsonError(w, "smart money service not initialized", http.StatusInternalServerError)
		return
	}
	var req struct {
		Chain       string   `json:"chain"`
		PoolAddress string   `json:"pool_address"`
		PoolID      string   `json:"pool_id"`
		Wallets     []string `json:"wallets"`
		LabelPrefix string   `json:"label_prefix"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	chainID := resolveSmartMoneyRequestChainID(r)
	if strings.TrimSpace(req.Chain) != "" {
		switch config.NormalizeChain(req.Chain) {
		case "base":
			chainID = 8453
		default:
			chainID = 56
		}
	}
	poolAddress := smartMoneyNormalizePoolIdentifier(req.PoolAddress)
	poolID := smartMoneyNormalizePoolID(req.PoolID)
	if poolAddress == "" && poolID == "" {
		jsonError(w, "pool_address or pool_id is required", http.StatusBadRequest)
		return
	}
	if poolAddress != "" && poolID != "" {
		jsonError(w, "pool_address and pool_id cannot both be set", http.StatusBadRequest)
		return
	}
	sourceContract := poolAddress
	if poolID != "" {
		sourceContract = poolID
	}
	if len(req.Wallets) == 0 {
		jsonError(w, "wallets is required", http.StatusBadRequest)
		return
	}
	if len(req.Wallets) > 100 {
		jsonError(w, "wallets cannot exceed 100", http.StatusBadRequest)
		return
	}

	repo := smService.Repo()
	result, err := repo.ImportPoolLiquidityWallets(r.Context(), chainID, sourceContract, req.Wallets, req.LabelPrefix)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(result.Invalid) > 0 {
		jsonOK(w, map[string]any{
			"created":          result.Created,
			"reactivated":      result.Reactivated,
			"skipped_existing": result.SkippedExisting,
			"invalid":          result.Invalid,
		})
		return
	}
	jsonOK(w, result)
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
	reqStarted := time.Now()
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
		s.enrichSmartMoneyPoolStatsMarketData(ctx, stats)
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
	source, ok := normalizeSmartMoneyWalletSourceScope(r.URL.Query().Get("source"))
	if !ok {
		jsonError(w, "invalid source", http.StatusBadRequest)
		return
	}
	var minSmartMoneyUSD float64
	hasMinSmartMoneyUSD := false
	if raw := strings.TrimSpace(r.URL.Query().Get("min_smart_money_usd")); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			jsonError(w, "invalid min_smart_money_usd", http.StatusBadRequest)
			return
		}
		minSmartMoneyUSD = value
		hasMinSmartMoneyUSD = true
	}
	var maxFeeRate float64
	hasMaxFeeRate := false
	if raw := strings.TrimSpace(r.URL.Query().Get("max_fee_rate")); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			jsonError(w, "invalid max_fee_rate", http.StatusBadRequest)
			return
		}
		maxFeeRate = value
		hasMaxFeeRate = true
	}
	var minFDVUSD float64
	hasMinFDVUSD := false
	minFDVParam := "min_fdv_usd"
	rawMinFDV := strings.TrimSpace(r.URL.Query().Get(minFDVParam))
	if rawMinFDV == "" {
		minFDVParam = "min_market_cap_usd"
		rawMinFDV = strings.TrimSpace(r.URL.Query().Get(minFDVParam))
	}
	if rawMinFDV != "" {
		value, err := strconv.ParseFloat(rawMinFDV, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			jsonError(w, "invalid "+minFDVParam, http.StatusBadRequest)
			return
		}
		minFDVUSD = value
		hasMinFDVUSD = true
	}

	sqlStarted := time.Now()
	pools, err := repo.ListPoolsWithPositions(ctx, source)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sqlElapsed := time.Since(sqlStarted)
	for i := range pools {
		pools[i].TradingPair = buildSmartMoneyTradingPair(pools[i].Token0Symbol, pools[i].Token1Symbol)
		pools[i].DisplayTokenAddress, pools[i].DisplayTokenSymbol = smartMoneyPickDisplayToken(
			pools[i].Token0Address,
			pools[i].Token1Address,
			pools[i].Token0Symbol,
			pools[i].Token1Symbol,
		)
	}
	s.enrichSmartMoneyPoolMarketData(ctx, pools)

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
		if hasMinSmartMoneyUSD && pool.TotalPositionAmountUSD < minSmartMoneyUSD {
			continue
		}
		if hasMaxFeeRate {
			if pool.FeeTier == nil {
				continue
			}
			feePercent := float64(*pool.FeeTier) / 10000
			if feePercent > maxFeeRate {
				continue
			}
		}
		if hasMinFDVUSD && smartMoneyFDVUSD(pool.FDVUSD, pool.CurrentTokenFDVUSD) < minFDVUSD {
			continue
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
	metaStarted := time.Now()
	metaByChain := s.loadSmartMoneyTokenMetadataByChain(ctx, addressesByChain)
	metaElapsed := time.Since(metaStarted)
	for i := range pagedPools {
		applySmartMoneyDisplayToken(
			smartMoneyChainSlug(pagedPools[i].ChainID),
			&pagedPools[i].DisplayTokenAddress,
			&pagedPools[i].DisplayTokenSymbol,
			&pagedPools[i].DisplayTokenLogoURL,
			metaByChain,
		)
	}
	rangeStarted := time.Now()
	if err := attachSmartMoneyRangeGroupsToPoolList(ctx, repo, pagedPools, source); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rangeElapsed := time.Since(rangeStarted)
	logSmartMoneySlowStage("pools", reqStarted, "sql", sqlElapsed, "metadata", metaElapsed, "ranges", rangeElapsed, "total", total, "page_size", len(pagedPools))
	jsonOK(w, map[string]interface{}{
		"total": total,
		"list":  pagedPools,
	})
}

func (s *Server) handleSMPoolFeeHeatmap(w http.ResponseWriter, r *http.Request) {
	reqStarted := time.Now()
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repo := smService.Repo()
	ctx := r.Context()
	repairSmartMoneyPositions(ctx, repo)

	windowKey, windowSeconds, ok := parseSmartMoneyHeatmapWindow(r.URL.Query().Get("window"))
	if !ok {
		jsonError(w, "invalid window", http.StatusBadRequest)
		return
	}
	sortKey := parseSmartMoneyHeatmapSort(r.URL.Query().Get("sort"))

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	keyword := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("keyword")))
	protocol := strings.TrimSpace(r.URL.Query().Get("protocol"))
	protocolFilter := strings.ToLower(protocol)

	sqlStarted := time.Now()
	positions, err := repo.ListActivePositionsForFeeHeatmap(ctx)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sqlElapsed := time.Since(sqlStarted)
	filteredPositions := filterSmartMoneyHeatmapPositions(positions, keyword, protocolFilter)
	refreshStarted := time.Now()
	s.triggerSmartMoneyHeatmapFeeRefresh(repo, filteredPositions)
	refreshElapsed := time.Since(refreshStarted)

	buildStarted := time.Now()
	rows := sm.BuildPoolFeeHeatmapRows(filteredPositions, sm.PoolFeeHeatmapOptions{
		WindowSeconds: windowSeconds,
		Sort:          sortKey,
		Now:           time.Now(),
	})
	buildElapsed := time.Since(buildStarted)
	for i := range rows {
		rows[i].TradingPair = buildSmartMoneyTradingPair(rows[i].Token0Symbol, rows[i].Token1Symbol)
		rows[i].DisplayTokenAddress, rows[i].DisplayTokenSymbol = smartMoneyPickDisplayToken(
			rows[i].Token0Address,
			rows[i].Token1Address,
			rows[i].Token0Symbol,
			rows[i].Token1Symbol,
		)
	}

	total := len(rows)
	start := (page - 1) * size
	if start > len(rows) {
		start = len(rows)
	}
	end := start + size
	if end > len(rows) {
		end = len(rows)
	}
	pagedRows := rows[start:end]
	s.enrichSmartMoneyHeatmapMarketData(ctx, pagedRows)

	addressesByChain := make(map[string][]string)
	for i := range pagedRows {
		if pagedRows[i].DisplayTokenAddress != "" {
			chain := smartMoneyChainSlug(pagedRows[i].ChainID)
			addressesByChain[chain] = append(addressesByChain[chain], pagedRows[i].DisplayTokenAddress)
		}
	}
	metaStarted := time.Now()
	metaByChain := s.loadSmartMoneyTokenMetadataByChain(ctx, addressesByChain)
	metaElapsed := time.Since(metaStarted)
	for i := range pagedRows {
		applySmartMoneyDisplayToken(
			smartMoneyChainSlug(pagedRows[i].ChainID),
			&pagedRows[i].DisplayTokenAddress,
			&pagedRows[i].DisplayTokenSymbol,
			&pagedRows[i].DisplayTokenLogoURL,
			metaByChain,
		)
	}

	jsonOK(w, map[string]interface{}{
		"window":         windowKey,
		"window_seconds": windowSeconds,
		"sort":           sortKey,
		"page":           page,
		"size":           size,
		"total":          total,
		"list":           pagedRows,
		"updated_at":     time.Now(),
	})
	logSmartMoneySlowStage("pool_fee_heatmap", reqStarted, "sql", sqlElapsed, "refresh_trigger", refreshElapsed, "build", buildElapsed, "metadata", metaElapsed, "positions", len(filteredPositions), "rows", total)
}

func filterSmartMoneyHeatmapPositions(positions []models.SmartMoneyActivePosition, keyword string, protocol string) []models.SmartMoneyActivePosition {
	hasProtocol := protocol != "" && protocol != "all"
	hasKeyword := keyword != ""
	if !hasProtocol && !hasKeyword {
		return positions
	}

	filtered := make([]models.SmartMoneyActivePosition, 0, len(positions))
	for _, pos := range positions {
		if hasProtocol && strings.ToLower(strings.TrimSpace(pos.Protocol)) != protocol {
			continue
		}
		if hasKeyword && !smartMoneyHeatmapPositionMatchesKeyword(pos, keyword) {
			continue
		}
		filtered = append(filtered, pos)
	}
	return filtered
}

func smartMoneyHeatmapPositionMatchesKeyword(pos models.SmartMoneyActivePosition, keyword string) bool {
	pairText := strings.ToLower(buildSmartMoneyTradingPair(pos.Token0Symbol, pos.Token1Symbol))
	poolAddrText := strings.ToLower(strings.TrimSpace(pos.PoolAddress))
	token0Text := strings.ToLower(strings.TrimSpace(pos.Token0Address))
	token1Text := strings.ToLower(strings.TrimSpace(pos.Token1Address))
	return strings.Contains(pairText, keyword) ||
		strings.Contains(poolAddrText, keyword) ||
		strings.Contains(token0Text, keyword) ||
		strings.Contains(token1Text, keyword)
}

const (
	smartMoneyHeatmapFeeRefreshInterval = 15 * time.Second
	smartMoneyHeatmapFeeRefreshWorkers  = 6
)

var smartMoneyHeatmapRefresh sync.Map

func logSmartMoneySlowStage(endpoint string, started time.Time, fields ...interface{}) {
	elapsed := time.Since(started)
	if elapsed < 2*time.Second {
		return
	}
	parts := make([]string, 0, len(fields)/2+2)
	parts = append(parts, "endpoint="+endpoint, "elapsed="+elapsed.String())
	for i := 0; i+1 < len(fields); i += 2 {
		key := strings.TrimSpace(fmt.Sprint(fields[i]))
		if key == "" {
			continue
		}
		parts = append(parts, key+"="+fmt.Sprint(fields[i+1]))
	}
	log.Printf("[SmartMoney API] slow %s", strings.Join(parts, " "))
}

func (s *Server) refreshSmartMoneyHeatmapFees(ctx context.Context, repo *sm.Repository, positions []models.SmartMoneyActivePosition) {
	if s == nil || s.Realtime == nil || repo == nil || len(positions) == 0 {
		return
	}

	now := time.Now()
	jobs := make(chan int)
	var wg sync.WaitGroup

	for worker := 0; worker < smartMoneyHeatmapFeeRefreshWorkers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				detail, err := s.Realtime.GetSmartMoneyPositionDetail(&positions[idx])
				if err != nil {
					continue
				}
				feeUSD := detail.Totals.FeeUSD
				if math.IsNaN(feeUSD) || math.IsInf(feeUSD, 0) || feeUSD < 0 {
					continue
				}
				if smartMoneyHeatmapFeeWarningsBlockSnapshot(detail.Warnings) {
					positions[idx].FeeStatus = "unavailable"
					positions[idx].FeeUpdatedAt = &now
					if updateErr := repo.UpdateActivePositionFeeSnapshot(ctx, positions[idx].ID, nil, "unavailable", now); updateErr != nil {
						log.Printf("smart money heatmap fee status update failed: id=%d err=%v", positions[idx].ID, updateErr)
					}
					continue
				}

				feeText := strconv.FormatFloat(feeUSD, 'f', 4, 64)
				positions[idx].FeeUSD = &feeText
				positions[idx].FeeStatus = "ok"
				positions[idx].FeeUpdatedAt = &now
				if updateErr := repo.UpdateActivePositionFeeSnapshot(ctx, positions[idx].ID, &feeUSD, "ok", now); updateErr != nil {
					log.Printf("smart money heatmap fee snapshot update failed: id=%d err=%v", positions[idx].ID, updateErr)
				}
			}
		}()
	}

	for i := range positions {
		if !smartMoneyHeatmapShouldRefreshFee(positions[i], now) {
			continue
		}
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case jobs <- i:
		}
	}

	close(jobs)
	wg.Wait()
}

func (s *Server) triggerSmartMoneyHeatmapFeeRefresh(repo *sm.Repository, positions []models.SmartMoneyActivePosition) {
	if s == nil || s.Realtime == nil || repo == nil || len(positions) == 0 {
		return
	}

	now := time.Now()
	jobs := make([]models.SmartMoneyActivePosition, 0, len(positions))
	for i := range positions {
		if !smartMoneyHeatmapShouldRefreshFee(positions[i], now) {
			continue
		}
		key := smartMoneyHeatmapRefreshKey(positions[i])
		if key == "" {
			continue
		}
		if _, loaded := smartMoneyHeatmapRefresh.LoadOrStore(key, struct{}{}); loaded {
			continue
		}
		jobs = append(jobs, positions[i])
	}
	if len(jobs) == 0 {
		return
	}

	go func() {
		defer func() {
			for i := range jobs {
				if key := smartMoneyHeatmapRefreshKey(jobs[i]); key != "" {
					smartMoneyHeatmapRefresh.Delete(key)
				}
			}
		}()

		refreshCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		s.refreshSmartMoneyHeatmapFees(refreshCtx, repo, jobs)
	}()
}

func smartMoneyHeatmapRefreshKey(pos models.SmartMoneyActivePosition) string {
	if pos.ID > 0 {
		return strconv.FormatUint(uint64(pos.ID), 10)
	}
	return strings.TrimSpace(pos.PositionRef)
}

func smartMoneyHeatmapShouldRefreshFee(pos models.SmartMoneyActivePosition, now time.Time) bool {
	if !pos.IsActive || pos.ID == 0 {
		return false
	}
	if pos.FeeUpdatedAt != nil && now.Sub(*pos.FeeUpdatedAt) < smartMoneyHeatmapFeeRefreshInterval {
		return false
	}
	return true
}

func smartMoneyHeatmapFeeWarningsBlockSnapshot(warnings []string) bool {
	for _, warning := range warnings {
		text := strings.ToLower(strings.TrimSpace(warning))
		if text == "" {
			continue
		}
		if strings.Contains(text, "fee") ||
			strings.Contains(text, "position manager") ||
			strings.Contains(text, "runtime metadata") ||
			strings.Contains(text, "read v3 position") ||
			strings.Contains(text, "read v4 position") {
			return true
		}
	}
	return false
}

func parseSmartMoneyHeatmapWindow(raw string) (string, int, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "1m":
		return "1m", 60, true
	case "30s":
		return "30s", 30, true
	case "5m":
		return "5m", 300, true
	case "1h":
		return "1h", 3600, true
	default:
		return "", 0, false
	}
}

func parseSmartMoneyHeatmapSort(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "fee":
		return "fee"
	default:
		return "rate"
	}
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
		PositionRef          string  `json:"position_ref"`
		WalletLabel          *string `json:"wallet_label"`
		WalletAvatarURL      *string `json:"wallet_avatar_url"`
		WalletSource         string  `json:"wallet_source,omitempty"`
		WalletSourceContract string  `json:"wallet_source_contract,omitempty"`
		WalletColor          string  `json:"wallet_color"`
		PriceLower           string  `json:"price_lower"`
		PriceUpper           string  `json:"price_upper"`
		RangePercent         float64 `json:"range_percent"`
		PositionAmountUSD    float64 `json:"position_amount_usd"`
		BscscanURL           string  `json:"bscscan_url"`
		TradingPair          string  `json:"trading_pair"`
		DisplayTokenAddress  string  `json:"display_token_address,omitempty"`
		DisplayTokenSymbol   string  `json:"display_token_symbol,omitempty"`
		DisplayTokenLogoURL  string  `json:"display_token_logo_url,omitempty"`
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
		walletCacheKey := strconv.Itoa(p.ChainID) + "|" + strings.ToLower(strings.TrimSpace(p.WalletAddress))
		if cachedWallet, ok := walletCache[walletCacheKey]; ok {
			if cachedWallet != nil {
				resp.WalletLabel = cachedWallet.Label
				resp.WalletAvatarURL = cachedWallet.AvatarURL
				resp.WalletSource = smartMoneyWalletSourceValue(cachedWallet)
				resp.WalletSourceContract = smartMoneyWalletSourceContractValue(cachedWallet)
			}
		} else {
			walletRow, err := repo.GetMonitoredWalletByAddress(ctx, p.WalletAddress, p.ChainID)
			if err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			walletCache[walletCacheKey] = walletRow
			if walletRow != nil {
				resp.WalletLabel = walletRow.Label
				resp.WalletAvatarURL = walletRow.AvatarURL
				resp.WalletSource = smartMoneyWalletSourceValue(walletRow)
				resp.WalletSourceContract = smartMoneyWalletSourceContractValue(walletRow)
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
		resp.PositionAmountUSD = amountsByChain[sm.SmartMoneyPositionAmountKey(p.ChainID, p.Protocol, p.NftTokenID)]
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
	reqStarted := time.Now()
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

	lookupStarted := time.Now()
	active, err := repo.GetActivePositionByRef(ctx, positionRef)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lookupElapsed := time.Since(lookupStarted)

	if active == nil && rawPositionID != "" {
		fallbackStarted := time.Now()
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
		lookupElapsed += time.Since(fallbackStarted)
	}

	if active == nil {
		jsonError(w, "position detail not found", http.StatusNotFound)
		return
	}

	rpcStarted := time.Now()
	detail, err := s.Realtime.GetSmartMoneyPositionDetail(active)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	walletRow, walletErr := repo.GetMonitoredWalletByAddress(ctx, active.WalletAddress, active.ChainID)
	if walletErr != nil {
		jsonError(w, walletErr.Error(), http.StatusInternalServerError)
		return
	}
	if walletRow != nil {
		detail.WalletLabel = walletRow.Label
		detail.WalletAvatarURL = walletRow.AvatarURL
		detail.WalletSource = smartMoneyWalletSourceValue(walletRow)
		detail.WalletSourceContract = smartMoneyWalletSourceContractValue(walletRow)
	}
	jsonOK(w, detail)
	logSmartMoneySlowStage("position_detail", reqStarted, "lookup", lookupElapsed, "rpc_detail", time.Since(rpcStarted), "protocol", active.Protocol, "nft", active.NftTokenID)
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

	type eventResp struct {
		models.SmartMoneyLPEvent
		WalletLabel          *string `json:"wallet_label,omitempty"`
		WalletAvatarURL      *string `json:"wallet_avatar_url,omitempty"`
		WalletSource         string  `json:"wallet_source,omitempty"`
		WalletSourceContract string  `json:"wallet_source_contract,omitempty"`
	}
	list := make([]eventResp, 0, len(events))
	walletCache := make(map[string]*models.MonitoredWallet)
	for _, event := range events {
		resp := eventResp{SmartMoneyLPEvent: event}
		walletCacheKey := strconv.Itoa(event.ChainID) + "|" + strings.ToLower(strings.TrimSpace(event.WalletAddress))
		walletRow, ok := walletCache[walletCacheKey]
		if !ok {
			var walletErr error
			walletRow, walletErr = repo.GetMonitoredWalletByAddress(ctx, event.WalletAddress, event.ChainID)
			if walletErr != nil {
				jsonError(w, walletErr.Error(), http.StatusInternalServerError)
				return
			}
			walletCache[walletCacheKey] = walletRow
		}
		if walletRow != nil {
			resp.WalletLabel = walletRow.Label
			resp.WalletAvatarURL = walletRow.AvatarURL
			resp.WalletSource = smartMoneyWalletSourceValue(walletRow)
			resp.WalletSourceContract = smartMoneyWalletSourceContractValue(walletRow)
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

func smartMoneyWalletSourceValue(wallet *models.MonitoredWallet) string {
	if wallet == nil {
		return ""
	}
	return strings.TrimSpace(wallet.Source)
}

func smartMoneyWalletSourceContractValue(wallet *models.MonitoredWallet) string {
	if wallet == nil || wallet.SourceContract == nil {
		return ""
	}
	return strings.TrimSpace(*wallet.SourceContract)
}

func normalizeSmartMoneyWalletSourceScope(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return "", true
	case "manual":
		return "manual", true
	case "contract", "contract_interaction":
		return "contract_interaction", true
	case "token_liquidity", "token_liquidity_indexer":
		return sm.MonitoredWalletSourceTokenLiquidityIndexer, true
	case "pool_liquidity", "pool_liquidity_radar":
		return sm.MonitoredWalletSourcePoolLiquidityRadar, true
	default:
		return "", false
	}
}

func repairSmartMoneyPositions(ctx context.Context, repo *sm.Repository) {
	if repo == nil {
		return
	}
	if !atomic.CompareAndSwapInt32(&smPositionRepairRunning, 0, 1) {
		return
	}
	go func() {
		defer atomic.StoreInt32(&smPositionRepairRunning, 0)
		repairCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := sm.RepairPositions(repairCtx, repo); err != nil {
			log.Printf("[SmartMoney API] repair position metadata failed: %v", err)
		}
	}()
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

func smartMoneyNormalizePoolIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if !isValidAddress(value) {
		return ""
	}
	return strings.ToLower(value)
}

func smartMoneyNormalizePoolID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != 66 {
		return ""
	}
	if !strings.HasPrefix(value, "0x") && !strings.HasPrefix(value, "0X") {
		return ""
	}
	for _, c := range value[2:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return ""
		}
	}
	return strings.ToLower(value)
}

func smartMoneyIsStableLikeSymbol(symbol string) bool {
	_, ok := smartMoneyStableSymbols[strings.ToLower(strings.TrimSpace(symbol))]
	return ok
}

func smartMoneyQuoteLikeToken(chain string, address string, symbol string) bool {
	if pricing.IsStableAddress(chain, address) || pricing.IsWrappedNativeAddress(chain, address) {
		return true
	}
	return pricing.IsQuoteLikeSymbol(symbol)
}

func smartMoneyPickMarketCapToken(chain string, token0Address string, token1Address string, token0Symbol string, token1Symbol string, displayTokenAddress string) (string, string) {
	token0Address = smartMoneyNormalizeTokenAddress(token0Address)
	token1Address = smartMoneyNormalizeTokenAddress(token1Address)
	token0Symbol = strings.TrimSpace(token0Symbol)
	token1Symbol = strings.TrimSpace(token1Symbol)

	token0Quote := smartMoneyQuoteLikeToken(chain, token0Address, token0Symbol)
	token1Quote := smartMoneyQuoteLikeToken(chain, token1Address, token1Symbol)
	switch {
	case token0Quote && !token1Quote:
		return token1Address, token1Symbol
	case token1Quote && !token0Quote:
		return token0Address, token0Symbol
	case !token0Quote && !token1Quote:
		displayTokenAddress = smartMoneyNormalizeTokenAddress(displayTokenAddress)
		if displayTokenAddress != "" && displayTokenAddress == token1Address {
			return token1Address, token1Symbol
		}
		return token0Address, token0Symbol
	default:
		return "", ""
	}
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

func (s *Server) enrichSmartMoneyPoolMarketData(ctx context.Context, pools []sm.PoolAggRow) {
	if s == nil || s.TokenPrice == nil || len(pools) == 0 {
		return
	}
	addressesByChain := make(map[string][]string)
	seenByChain := make(map[string]map[string]struct{})
	for i := range pools {
		chain := smartMoneyChainSlug(pools[i].ChainID)
		addr, symbol := smartMoneyPickMarketCapToken(
			chain,
			pools[i].Token0Address,
			pools[i].Token1Address,
			pools[i].Token0Symbol,
			pools[i].Token1Symbol,
			pools[i].DisplayTokenAddress,
		)
		pools[i].MarketCapTokenAddress = addr
		pools[i].MarketCapTokenSymbol = symbol
		if addr == "" {
			continue
		}
		seen := seenByChain[chain]
		if seen == nil {
			seen = make(map[string]struct{})
			seenByChain[chain] = seen
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		addressesByChain[chain] = append(addressesByChain[chain], addr)
	}

	marketDataByChain := s.loadTokenMarketDataByChain(ctx, addressesByChain)
	for i := range pools {
		applySmartMoneyMarketDataToFields(
			smartMoneyChainSlug(pools[i].ChainID),
			pools[i].MarketCapTokenAddress,
			&pools[i].MarketCapUSD,
			&pools[i].FDVUSD,
			&pools[i].CurrentTokenFDVUSD,
			&pools[i].MarketCapProvider,
			marketDataByChain,
		)
	}
}

func (s *Server) enrichSmartMoneyHeatmapMarketData(ctx context.Context, rows []sm.PoolFeeHeatmapRow) {
	if s == nil || s.TokenPrice == nil || len(rows) == 0 {
		return
	}
	addressesByChain := make(map[string][]string)
	seenByChain := make(map[string]map[string]struct{})
	for i := range rows {
		chain := smartMoneyChainSlug(rows[i].ChainID)
		addr, symbol := smartMoneyPickMarketCapToken(
			chain,
			rows[i].Token0Address,
			rows[i].Token1Address,
			rows[i].Token0Symbol,
			rows[i].Token1Symbol,
			rows[i].DisplayTokenAddress,
		)
		rows[i].MarketCapTokenAddress = addr
		rows[i].MarketCapTokenSymbol = symbol
		if addr == "" {
			continue
		}
		seen := seenByChain[chain]
		if seen == nil {
			seen = make(map[string]struct{})
			seenByChain[chain] = seen
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		addressesByChain[chain] = append(addressesByChain[chain], addr)
	}

	marketDataByChain := s.loadTokenMarketDataByChain(ctx, addressesByChain)
	for i := range rows {
		applySmartMoneyMarketDataToFields(
			smartMoneyChainSlug(rows[i].ChainID),
			rows[i].MarketCapTokenAddress,
			&rows[i].MarketCapUSD,
			&rows[i].FDVUSD,
			&rows[i].CurrentTokenFDVUSD,
			&rows[i].MarketCapProvider,
			marketDataByChain,
		)
	}
}

func (s *Server) enrichSmartMoneyPoolStatsMarketData(ctx context.Context, stats *sm.PoolStats) {
	if s == nil || s.TokenPrice == nil || stats == nil {
		return
	}
	chain := smartMoneyChainSlug(stats.ChainID)
	addr, symbol := smartMoneyPickMarketCapToken(
		chain,
		stats.Token0Address,
		stats.Token1Address,
		stats.Token0Symbol,
		stats.Token1Symbol,
		stats.DisplayTokenAddress,
	)
	stats.MarketCapTokenAddress = addr
	stats.MarketCapTokenSymbol = symbol
	if addr == "" {
		return
	}
	dataByChain := s.loadTokenMarketDataByChain(ctx, map[string][]string{chain: []string{addr}})
	applySmartMoneyMarketDataToFields(
		chain,
		addr,
		&stats.MarketCapUSD,
		&stats.FDVUSD,
		&stats.CurrentTokenFDVUSD,
		&stats.MarketCapProvider,
		dataByChain,
	)
}

func (s *Server) loadTokenMarketDataByChain(ctx context.Context, addressesByChain map[string][]string) map[string]map[string]pricing.TokenMarketData {
	out := make(map[string]map[string]pricing.TokenMarketData, len(addressesByChain))
	if s == nil || s.TokenPrice == nil {
		return out
	}
	for chain, addresses := range addressesByChain {
		if len(addresses) == 0 {
			continue
		}
		data, err := s.TokenPrice.GetTokenMarketDataWithContext(ctx, chain, addresses)
		if err != nil {
			log.Printf("[smart_money] load realtime token market data failed chain=%s err=%v", chain, err)
		}
		if len(data) == 0 {
			continue
		}
		out[chain] = data
	}
	return out
}

func applySmartMoneyMarketDataToFields(
	chain string,
	tokenAddress string,
	marketCapUSD *float64,
	fdvUSD *float64,
	currentTokenFDVUSD *float64,
	provider *string,
	dataByChain map[string]map[string]pricing.TokenMarketData,
) {
	tokenAddress = strings.ToLower(strings.TrimSpace(tokenAddress))
	if tokenAddress == "" {
		return
	}
	dataByAddress := dataByChain[chain]
	if len(dataByAddress) == 0 {
		return
	}
	data, ok := dataByAddress[tokenAddress]
	if !ok {
		return
	}
	*marketCapUSD = sanitizeFloat(data.MarketCapUSD)
	*fdvUSD = sanitizeFloat(data.FDVUSD)
	*currentTokenFDVUSD = sanitizeFloat(data.FDVUSD)
	*provider = strings.TrimSpace(data.Provider)
}

func smartMoneyFDVUSD(fdvUSD float64, currentTokenFDVUSD float64) float64 {
	if value := sanitizeFloat(fdvUSD); value > 0 {
		return value
	}
	return sanitizeFloat(currentTokenFDVUSD)
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

func attachSmartMoneyRangeGroupsToPoolList(ctx context.Context, repo *sm.Repository, pools []sm.PoolAggRow, source string) error {
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

	rangeRows, err := repo.ListRecentOpenPositionRanges(ctx, poolAddresses, source)
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

	rangeRows, err := repo.ListRecentOpenPositionRanges(ctx, []string{addr}, "")
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
