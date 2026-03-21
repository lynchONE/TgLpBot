package web_server

import (
	userSvc "TgLpBot/service/user"
	"encoding/json"
	"net/http"
	"strings"
)

type assetRequestBase struct {
	InitData string `json:"initData"`
}

type userAssetHistoryRequest struct {
	InitData string `json:"initData"`
	Days     int    `json:"days"`
}

type adminSmartMoneyWalletRequest struct {
	InitData string `json:"initData"`
	Address  string `json:"address"`
	ChainID  int    `json:"chain_id"`
	Days     int    `json:"days"`
}

type adminSmartMoneyLeaderboardRequest struct {
	InitData string `json:"initData"`
	Days     int    `json:"days"`
	Metric   string `json:"metric"`
	Limit    int    `json:"limit"`
}

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
	resp, err := s.Assets.GetUserOverview(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, resp)
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
	resp, err := s.Assets.GetUserHistory(r.Context(), userID, req.Days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, resp)
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
	resp, err := s.Assets.GetUserLPStats(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, resp)
}

func (s *Server) handleAdminSmartMoneyOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req userAssetHistoryRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if _, status, msg := authenticateAssetAdmin(req.InitData); status != 0 {
		http.Error(w, msg, status)
		return
	}
	resp, err := s.Assets.GetSmartMoneyOverview(r.Context(), req.Days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, resp)
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
	if _, status, msg := authenticateAssetAdmin(req.InitData); status != 0 {
		http.Error(w, msg, status)
		return
	}
	resp, err := s.Assets.GetSmartMoneyWallet(r.Context(), req.Address, req.ChainID, req.Days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, resp)
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
	if _, status, msg := authenticateAssetAdmin(req.InitData); status != 0 {
		http.Error(w, msg, status)
		return
	}
	resp, err := s.Assets.GetSmartMoneyLeaderboard(r.Context(), req.Metric, req.Days, req.Limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, resp)
}
