package web_server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/service/blacklist"
	userSvc "TgLpBot/service/user"
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

	// 验证用户
	if config.AppConfig == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(CooldownsResponse{
			Success: false,
			Message: "配置未加载",
		})
		return
	}

	parsed, err := ParseTelegramWebAppInitData(initData, config.AppConfig.TelegramBotToken)
	if err != nil {
		if errors.Is(err, ErrMissingInitData) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(CooldownsResponse{
				Success: false,
				Message: "缺少 initData",
			})
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(CooldownsResponse{
				Success: false,
				Message: "验证失败: " + err.Error(),
			})
		}
		return
	}

	userService := userSvc.NewUserService()
	user, err := userService.GetOrCreateUser(
		parsed.User.ID,
		parsed.User.Username,
		parsed.User.FirstName,
		parsed.User.LastName,
		parsed.User.LanguageCode,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(CooldownsResponse{
			Success: false,
			Message: "加载用户失败",
		})
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
