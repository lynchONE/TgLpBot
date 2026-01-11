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

// handleCooldowns 处理冷却列表 API
func handleCooldowns(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(CooldownsResponse{
			Success: false,
			Message: "不支持的请求方法",
		})
		return
	}

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

	// 获取冷却列表
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

	// 转换为响应格式
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
