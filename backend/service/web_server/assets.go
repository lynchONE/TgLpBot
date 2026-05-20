package web_server

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	userSvc "TgLpBot/service/user"
	"encoding/json"
	"fmt"
	"log"
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

type userLPAdjustmentRequest struct {
	InitData            string  `json:"initData"`
	Day                 string  `json:"day"`
	ManualAdjustmentUSD float64 `json:"manual_adjustment_usd"`
	Note                string  `json:"note,omitempty"`
	Action              string  `json:"action,omitempty"`
	Clear               bool    `json:"clear,omitempty"`
}

type userLPProfitBaselineRequest struct {
	InitData   string  `json:"initData"`
	Day        string  `json:"day"`
	BasePnLUSD float64 `json:"base_pnl_usd"`
	Note       string  `json:"note,omitempty"`
	Action     string  `json:"action,omitempty"`
	Clear      bool    `json:"clear,omitempty"`
}

type adminSmartMoneyOverviewRequest struct {
	InitData     string   `json:"initData"`
	Days         int      `json:"days"`
	Page         int      `json:"page"`
	PageSize     int      `json:"page_size"`
	Keyword      string   `json:"keyword"`
	Section      string   `json:"section,omitempty"`
	Sections     []string `json:"sections,omitempty"`
	ForceRefresh bool     `json:"force_refresh,omitempty"`
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
	Page         int    `json:"page"`
	PageSize     int    `json:"page_size"`
	Keyword      string `json:"keyword"`
	Limit        int    `json:"limit,omitempty"`
	ForceRefresh bool   `json:"force_refresh,omitempty"`
}

type adminSmartMoneyLeaderboardRequestCompat struct {
	InitData          string `json:"initData"`
	Days              int    `json:"days"`
	Metric            string `json:"metric"`
	Page              int    `json:"page"`
	PageSize          int    `json:"page_size"`
	PageSizeCamel     int    `json:"pageSize"`
	Keyword           string `json:"keyword"`
	Limit             int    `json:"limit,omitempty"`
	ForceRefresh      bool   `json:"force_refresh,omitempty"`
	ForceRefreshCamel *bool  `json:"forceRefresh,omitempty"`
}

const assetResponseCacheTTL = time.Minute

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dest interface{}) bool {
	return decodeJSONBodyWithMode(w, r, dest, true)
}

func decodeJSONBodyLoose(w http.ResponseWriter, r *http.Request, dest interface{}) bool {
	return decodeJSONBodyWithMode(w, r, dest, false)
}

func decodeJSONBodyWithMode(w http.ResponseWriter, r *http.Request, dest interface{}, disallowUnknownFields bool) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	dec := json.NewDecoder(r.Body)
	if disallowUnknownFields {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(dest); err != nil {
		log.Printf("web_server: decode JSON body failed for %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return false
	}
	return true
}

func decodeAdminSmartMoneyLeaderboardRequest(w http.ResponseWriter, r *http.Request) (adminSmartMoneyLeaderboardRequest, bool) {
	var raw adminSmartMoneyLeaderboardRequestCompat
	if !decodeJSONBodyLoose(w, r, &raw) {
		return adminSmartMoneyLeaderboardRequest{}, false
	}

	req := adminSmartMoneyLeaderboardRequest{
		InitData:     raw.InitData,
		Days:         raw.Days,
		Metric:       raw.Metric,
		Page:         raw.Page,
		PageSize:     raw.PageSize,
		Keyword:      raw.Keyword,
		Limit:        raw.Limit,
		ForceRefresh: raw.ForceRefresh,
	}
	if req.PageSize <= 0 && raw.PageSizeCamel > 0 {
		req.PageSize = raw.PageSizeCamel
	}
	if raw.ForceRefreshCamel != nil {
		req.ForceRefresh = *raw.ForceRefreshCamel
	}
	return req, true
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
	if status, msg := requireModulePermission(check, models.AccessModuleAssets); status != 0 {
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

func assetOverviewSectionsCachePart(section string, sections []string) string {
	values := make([]string, 0, len(sections)+1)
	if strings.TrimSpace(section) != "" {
		values = append(values, section)
	}
	values = append(values, sections...)
	if len(values) == 0 {
		return "all"
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, item := range values {
		for _, part := range strings.Split(item, ",") {
			value := strings.TrimSpace(strings.ToLower(part))
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return "all"
	}
	return strings.Join(out, "_")
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

func invalidateCachedAssetResponse(key string) {
	if database.RedisClient == nil || strings.TrimSpace(key) == "" {
		return
	}
	_ = database.DeleteCache(key)
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

func (s *Server) handleAssetLPAdjustment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req userLPAdjustmentRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	userID, status, msg := authenticateAssetUser(req.InitData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	if req.Clear || action == "clear" || action == "delete" {
		if err := s.Assets.ClearUserLPDailyPnLAdjustment(r.Context(), userID, req.Day); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		invalidateCachedAssetResponse(assetResponseCacheKey("user", fmt.Sprintf("%d", userID), "lp"))
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":  true,
			"day": strings.TrimSpace(req.Day),
		})
		return
	}

	adjustment, err := s.Assets.SaveUserLPDailyPnLAdjustment(r.Context(), userID, req.Day, req.ManualAdjustmentUSD, req.Note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	invalidateCachedAssetResponse(assetResponseCacheKey("user", fmt.Sprintf("%d", userID), "lp"))
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"adjustment": adjustment,
	})
}

func (s *Server) handleAssetLPProfitBaseline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req userLPProfitBaselineRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	userID, status, msg := authenticateAssetUser(req.InitData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	if req.Clear || action == "clear" || action == "delete" {
		if err := s.Assets.ClearUserLPProfitBaseline(r.Context(), userID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		invalidateCachedAssetResponse(assetResponseCacheKey("user", fmt.Sprintf("%d", userID), "lp"))
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok": true,
		})
		return
	}

	baseline, err := s.Assets.SaveUserLPProfitBaseline(r.Context(), userID, req.Day, req.BasePnLUSD, req.Note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	invalidateCachedAssetResponse(assetResponseCacheKey("user", fmt.Sprintf("%d", userID), "lp"))
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"baseline": baseline,
	})
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
		assetOverviewSectionsCachePart(req.Section, req.Sections),
		fmt.Sprintf("%d", req.Days),
		fmt.Sprintf("%d", req.Page),
		fmt.Sprintf("%d", req.PageSize),
		req.Keyword,
	)
	if err := respondWithAssetCache(w, key, req.ForceRefresh, func() (interface{}, error) {
		sections := req.Sections
		if strings.TrimSpace(req.Section) != "" {
			sections = append([]string{req.Section}, sections...)
		}
		return s.Assets.GetSmartMoneyOverviewSections(r.Context(), req.Days, req.Page, req.PageSize, req.Keyword, req.ForceRefresh, sections)
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
	req, ok := decodeAdminSmartMoneyLeaderboardRequest(w, r)
	if !ok {
		return
	}
	adminUserID, status, msg := authenticateAssetAdmin(req.InitData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	pageSize := req.PageSize
	if pageSize <= 0 && req.Limit > 0 {
		pageSize = req.Limit
	}
	key := assetResponseCacheKey(
		"admin",
		fmt.Sprintf("%d", adminUserID),
		"smart-money-leaderboard",
		req.Metric,
		fmt.Sprintf("%d", req.Days),
		fmt.Sprintf("%d", req.Page),
		fmt.Sprintf("%d", pageSize),
		req.Keyword,
	)
	if err := respondWithAssetCache(w, key, req.ForceRefresh, func() (interface{}, error) {
		return s.Assets.GetSmartMoneyLeaderboard(r.Context(), req.Metric, req.Days, req.Page, pageSize, req.Keyword, req.ForceRefresh)
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
