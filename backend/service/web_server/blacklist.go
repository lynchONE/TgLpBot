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

// parseAndValidateUser 解析 initData 并获取用户
func parseAndValidateUser(initData string) (uint, error) {
	if config.AppConfig == nil {
		return 0, errors.New("配置未加载")
	}

	parsed, err := ParseTelegramWebAppInitData(initData, config.AppConfig.TelegramBotToken)
	if err != nil {
		return 0, err
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
		return 0, err
	}

	return user.ID, nil
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

	// 验证用户
	userID, err := parseAndValidateUser(initData)
	if err != nil {
		if errors.Is(err, ErrMissingInitData) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(BlacklistResponse{
				Success: false,
				Message: "缺少 initData",
			})
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(BlacklistResponse{
				Success: false,
				Message: "验证失败: " + err.Error(),
			})
		}
		return
	}

	// 获取黑名单
	svc := blacklist.NewBlacklistService()
	list, err := svc.GetAll(userID)
	if err != nil {
		log.Printf("[Blacklist API] 获取黑名单失败: user_id=%d err=%v", userID, err)
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

	// 验证用户
	userID, err := parseAndValidateUser(req.InitData)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "验证失败: " + err.Error(),
		})
		return
	}

	svc := blacklist.NewBlacklistService()
	action := strings.ToLower(strings.TrimSpace(req.Action))

	switch action {
	case "add":
		if err := svc.Add(userID, req.PoolAddress); err != nil {
			log.Printf("[Blacklist API] 添加黑名单失败: user_id=%d pool=%s err=%v", userID, req.PoolAddress, err)
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
			Count:   svc.Count(userID),
		})

	case "remove":
		if err := svc.Remove(userID, req.PoolAddress); err != nil {
			log.Printf("[Blacklist API] 移除黑名单失败: user_id=%d pool=%s err=%v", userID, req.PoolAddress, err)
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
			Count:   svc.Count(userID),
		})

	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BlacklistResponse{
			Success: false,
			Message: "无效的 action，应为 'add' 或 'remove'",
		})
	}
}
