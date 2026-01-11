package web_server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"TgLpBot/service/blacklist"
)

// BlacklistRequest 黑名单操作请求
type BlacklistRequest struct {
	InitData    string `json:"initData"`
	PoolAddress string `json:"pool_address"`
	Action      string `json:"action"` // "add" or "remove"
}

// BlacklistResponse 黑名单响应
type BlacklistResponse struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message,omitempty"`
	Blacklist []string `json:"blacklist,omitempty"`
	Count     int64    `json:"count,omitempty"`
}

// handleBlacklist 处理黑名单 API
func handleBlacklist(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		handleGetBlacklist(w, r)
	case http.MethodPost:
		handleModifyBlacklist(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "不支持的请求方法",
		})
	}
}

// handleGetBlacklist 获取黑名单列表
func handleGetBlacklist(w http.ResponseWriter, r *http.Request) {
	initData := r.URL.Query().Get("initData")
	if strings.TrimSpace(initData) == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "缺少 initData",
		})
		return
	}

	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}
	if status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}
	if status, msg := requireAutoModePermission(check); status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}

	// 获取黑名单
	svc := blacklist.NewBlacklistService()
	list, err := svc.GetAll(user.ID)
	if err != nil {
		log.Printf("[Blacklist API] 获取黑名单失败: user_id=%d err=%v", user.ID, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "获取黑名单失败: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(BlacklistResponse{
		Success:   true,
		Blacklist: list,
		Count:     int64(len(list)),
	})
}

// handleModifyBlacklist 添加/移除黑名单
func handleModifyBlacklist(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req BlacklistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "请求格式错误",
		})
		return
	}

	if strings.TrimSpace(req.InitData) == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "缺少 initData",
		})
		return
	}

	if strings.TrimSpace(req.PoolAddress) == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "缺少 pool_address",
		})
		return
	}

	user, status, msg := authenticateTelegramWebAppUser(req.InitData)
	if status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}
	if status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}
	if status, msg := requireAutoModePermission(check); status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(BlacklistResponse{Success: false, Message: msg})
		return
	}

	svc := blacklist.NewBlacklistService()
	action := strings.ToLower(strings.TrimSpace(req.Action))

	switch action {
	case "add":
		if err := svc.Add(user.ID, req.PoolAddress); err != nil {
			log.Printf("[Blacklist API] 添加黑名单失败: user_id=%d pool=%s err=%v", user.ID, req.PoolAddress, err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(BlacklistResponse{
				Success: false,
				Message: "添加黑名单失败: " + err.Error(),
			})
			return
		}
		json.NewEncoder(w).Encode(BlacklistResponse{
			Success: true,
			Message: "已添加到黑名单",
			Count:   svc.Count(user.ID),
		})

	case "remove":
		if err := svc.Remove(user.ID, req.PoolAddress); err != nil {
			log.Printf("[Blacklist API] 移除黑名单失败: user_id=%d pool=%s err=%v", user.ID, req.PoolAddress, err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(BlacklistResponse{
				Success: false,
				Message: "移除黑名单失败: " + err.Error(),
			})
			return
		}
		json.NewEncoder(w).Encode(BlacklistResponse{
			Success: true,
			Message: "已从黑名单移除",
			Count:   svc.Count(user.ID),
		})

	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "无效的 action，应为 'add' 或 'remove'",
		})
	}
}
