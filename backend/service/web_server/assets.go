package web_server

import (
	"TgLpBot/base/database"
	userSvc "TgLpBot/service/user"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type assetRequestBase struct {
	InitData     string `json:"initData"`
	ForceRefresh bool   `json:"force_refresh,omitempty"`
}

type userAssetHistoryRequest struct {
	InitData     string `json:"initData"`
	Days         int    `json:"days"`
	ForceRefresh bool   `json:"force_refresh,omitempty"`
}

type adminSmartMoneyOverviewRequest struct {
	InitData     string `json:"initData"`
	Days         int    `json:"days"`
	Page         int    `json:"page"`
	PageSize     int    `json:"page_size"`
	Keyword      string `json:"keyword"`
	ForceRefresh bool   `json:"force_refresh,omitempty"`
}

type adminSmartMoneyWalletRequest struct {
	InitData     string `json:"initData"`
	Address      string `json:"address"`
	ChainID      int    `json:"chain_id"`
	Days         int    `json:"days"`
	ForceRefresh bool   `json:"force_refresh,omitempty"`
}

type adminSmartMoneyLeaderboardRequest struct {
	InitData     string `json:"initData"`
	Days         int    `json:"days"`
	Metric       string `json:"metric"`
	Limit        int    `json:"limit"`
	ForceRefresh bool   `json:"force_refresh,omitempty"`
}

const assetResponseCacheTTL = time.Minute

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dest interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return false
	}
	return true
}

func authenticateAssetUser(initData string) (uint, int, string) {
	user, status, msg := authenticateTelegramWebAppUser(strings.TrimSpace(initData))
	if status != 0 {
		return 0, status, msg
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		return 0, status, msg
	}
	if status != 0 {
		return 0, status, msg
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		return 0, status, msg
	}
	return user.ID, 0, ""
}

func authenticateAssetAdmin(initData string) (uint, int, string) {
	user, status, msg := authenticateTelegramWebAppUser(strings.TrimSpace(initData))
	if status != 0 {
		return 0, status, msg
	}
	if _, status, msg, err := requireUserAccess(user.ID); err != nil || status != 0 {
		return 0, status, msg
	}
	accessService := userSvc.NewAccessService()
	if !accessService.IsAdminUser(user.ID) {
		return 0, http.StatusForbidden, "forbidden"
	}
	return user.ID, 0, ""
}

func assetResponseCacheKey(parts ...string) string {
	out := make([]string, 0, len(parts)+1)
	out = append(out, "assets:resp")
	for _, part := range parts {
		value := strings.TrimSpace(strings.ToLower(part))
		if value == "" {
			value = "-"
		}
		value = strings.ReplaceAll(value, ":", "_")
		value = strings.ReplaceAll(value, " ", "_")
		out = append(out, value)
	}
	return strings.Join(out, ":")
}

func readCachedAssetResponse(key string) ([]byte, bool) {
	if database.RedisClient == nil || strings.TrimSpace(key) == "" {
		return nil, false
	}
	value, err := database.GetCache(key)
	if err != nil || strings.TrimSpace(value) == "" {
		return nil, false
	}
	return []byte(value), true
}

func writeCachedAssetResponse(key string, payload []byte) {
	if database.RedisClient == nil || strings.TrimSpace(key) == "" || len(payload) == 0 {
		return
	}
	_ = database.SetCache(key, string(payload), assetResponseCacheTTL)
}

func respondWithAssetCache(w http.ResponseWriter, key string, forceRefresh bool, load func() (interface{}, error)) error {
	if !forceRefresh {
		if cached, ok := readCachedAssetResponse(key); ok {
			writeJSONBytes(w, http.StatusOK, cached)
			return nil
		}
	}

	payload, err := load()
	if err != nil {
		return err
	}
	body, err := marshalJSONPayload(payload)
	if err != nil {
		return err
	}
	writeCachedAssetResponse(key, body)
	writeJSONBytes(w, http.StatusOK, body)
	return nil
}

func (s *Server) handleAssetOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req assetRequestBase
	if !decodeJSONBody(w, r, &req) {
		return
	}
	userID, status, msg := authenticateAssetUser(req.InitData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	key := assetResponseCacheKey("user", fmt.Sprintf("%d", userID), "overview")
	if err := respondWithAssetCache(w, key, req.ForceRefresh, func() (interface{}, error) {
		return s.Assets.GetUserOverview(r.Context(), userID)
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAssetHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req userAssetHistoryRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	userID, status, msg := authenticateAssetUser(req.InitData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	key := assetResponseCacheKey("user", fmt.Sprintf("%d", userID), "history", fmt.Sprintf("%d", req.Days))
	if err := respondWithAssetCache(w, key, req.ForceRefresh, func() (interface{}, error) {
		return s.Assets.GetUserHistory(r.Context(), userID, req.Days)
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAssetLPStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req assetRequestBase
	if !decodeJSONBody(w, r, &req) {
		return
	}
	userID, status, msg := authenticateAssetUser(req.InitData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	key := assetResponseCacheKey("user", fmt.Sprintf("%d", userID), "lp")
	if err := respondWithAssetCache(w, key, req.ForceRefresh, func() (interface{}, error) {
		return s.Assets.GetUserLPStats(r.Context(), userID)
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminSmartMoneyOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req adminSmartMoneyOverviewRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	adminUserID, status, msg := authenticateAssetAdmin(req.InitData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	key := assetResponseCacheKey(
		"admin",
		fmt.Sprintf("%d", adminUserID),
		"smart-money-overview",
		fmt.Sprintf("%d", req.Days),
		fmt.Sprintf("%d", req.Page),
		fmt.Sprintf("%d", req.PageSize),
		req.Keyword,
	)
	if err := respondWithAssetCache(w, key, req.ForceRefresh, func() (interface{}, error) {
		return s.Assets.GetSmartMoneyOverview(r.Context(), req.Days, req.Page, req.PageSize, req.Keyword, req.ForceRefresh)
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminSmartMoneyWallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req adminSmartMoneyWalletRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	adminUserID, status, msg := authenticateAssetAdmin(req.InitData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	key := assetResponseCacheKey("admin", fmt.Sprintf("%d", adminUserID), "smart-money-wallet", req.Address, fmt.Sprintf("%d", req.ChainID), fmt.Sprintf("%d", req.Days))
	if err := respondWithAssetCache(w, key, req.ForceRefresh, func() (interface{}, error) {
		return s.Assets.GetSmartMoneyWallet(r.Context(), req.Address, req.ChainID, req.Days, req.ForceRefresh)
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAdminSmartMoneyLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req adminSmartMoneyLeaderboardRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	adminUserID, status, msg := authenticateAssetAdmin(req.InitData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	key := assetResponseCacheKey("admin", fmt.Sprintf("%d", adminUserID), "smart-money-leaderboard", req.Metric, fmt.Sprintf("%d", req.Days), fmt.Sprintf("%d", req.Limit))
	if err := respondWithAssetCache(w, key, req.ForceRefresh, func() (interface{}, error) {
		return s.Assets.GetSmartMoneyLeaderboard(r.Context(), req.Metric, req.Days, req.Limit, req.ForceRefresh)
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
