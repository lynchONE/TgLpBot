package web_server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"TgLpBot/service/blacklist"
)

// CooldownItem 冷却项
type CooldownItem struct {
	TradingPair      string `json:"trading_pair"`
	Reason           string `json:"reason"`
	RemainingSeconds int64  `json:"remaining_seconds"`
	RemainingMinutes int    `json:"remaining_minutes"`
	ExpiresAt        string `json:"expires_at"`
}

// CooldownsResponse 冷却列表响应
type CooldownsResponse struct {
	Success   bool           `json:"success"`
	Message   string         `json:"message,omitempty"`
	Cooldowns []CooldownItem `json:"cooldowns,omitempty"`
	Count     int            `json:"count,omitempty"`
}

// RemoveCooldownRequest 移除冷却请求
type RemoveCooldownRequest struct {
	TradingPair string `json:"trading_pair"`
}

// handleCooldowns 处理冷却列表 API (GET 获取列表, DELETE 移除冷却)
func handleCooldowns(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		handleCooldownsGet(w, r)
	case http.MethodDelete:
		handleCooldownsDelete(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(CooldownsResponse{
			Success: false,
			Message: "不支持的请求方法",
		})
	}
}

// handleCooldownsGet 获取冷却列表
func handleCooldownsGet(w http.ResponseWriter, r *http.Request) {
	initData := r.URL.Query().Get("initData")
	if strings.TrimSpace(initData) == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(CooldownsResponse{
			Success: false,
			Message: "缺少 initData",
		})
		return
	}

	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(CooldownsResponse{Success: false, Message: msg})
		return
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(CooldownsResponse{Success: false, Message: msg})
		return
	}
	if status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(CooldownsResponse{Success: false, Message: msg})
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(CooldownsResponse{Success: false, Message: msg})
		return
	}
	if status, msg := requireAutoModePermission(check); status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(CooldownsResponse{Success: false, Message: msg})
		return
	}

	svc := blacklist.NewCooldownService()
	cooldowns, err := svc.GetAll(user.ID)
	if err != nil {
		log.Printf("[Cooldowns API] 获取冷却列表失败: user_id=%d err=%v", user.ID, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(CooldownsResponse{
			Success: false,
			Message: "获取冷却列表失败: " + err.Error(),
		})
		return
	}

	items := make([]CooldownItem, 0, len(cooldowns))
	for _, c := range cooldowns {
		remainingSec := int64(c.RemainingTime.Seconds())
		remainingMin := int(c.RemainingTime.Minutes())
		if remainingMin < 1 && remainingSec > 0 {
			remainingMin = 1
		}
		items = append(items, CooldownItem{
			TradingPair:      c.TradingPair,
			Reason:           c.Reason,
			RemainingSeconds: remainingSec,
			RemainingMinutes: remainingMin,
			ExpiresAt:        c.ExpiresAt.Format("15:04:05"),
		})
	}

	json.NewEncoder(w).Encode(CooldownsResponse{
		Success:   true,
		Cooldowns: items,
		Count:     len(items),
	})
}

// handleCooldownsDelete 移除冷却
func handleCooldownsDelete(w http.ResponseWriter, r *http.Request) {
	initData := r.URL.Query().Get("initData")
	if strings.TrimSpace(initData) == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(CooldownsResponse{
			Success: false,
			Message: "缺少 initData",
		})
		return
	}

	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(CooldownsResponse{Success: false, Message: msg})
		return
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(CooldownsResponse{Success: false, Message: msg})
		return
	}
	if status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(CooldownsResponse{Success: false, Message: msg})
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(CooldownsResponse{Success: false, Message: msg})
		return
	}
	if status, msg := requireAutoModePermission(check); status != 0 {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(CooldownsResponse{Success: false, Message: msg})
		return
	}

	var req RemoveCooldownRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(CooldownsResponse{
			Success: false,
			Message: "请求格式错误: " + err.Error(),
		})
		return
	}

	tradingPair := strings.TrimSpace(req.TradingPair)
	if tradingPair == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(CooldownsResponse{
			Success: false,
			Message: "缺少 trading_pair 参数",
		})
		return
	}

	svc := blacklist.NewCooldownService()
	if err := svc.Remove(user.ID, tradingPair); err != nil {
		log.Printf("[Cooldowns API] 移除冷却失败: user_id=%d trading_pair=%s err=%v", user.ID, tradingPair, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(CooldownsResponse{
			Success: false,
			Message: "移除冷却失败: " + err.Error(),
		})
		return
	}

	log.Printf("[Cooldowns API] 移除冷却成功: user_id=%d trading_pair=%s", user.ID, tradingPair)
	json.NewEncoder(w).Encode(CooldownsResponse{
		Success: true,
		Message: "已移除冷却: " + tradingPair,
	})
}
