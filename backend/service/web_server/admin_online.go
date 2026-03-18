package web_server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/service/realtime"
	userSvc "TgLpBot/service/user"
)

type adminOnlineUsersResponse struct {
	Users []realtime.AdminOnlineUser `json:"users"`
	Total int                        `json:"total"`
}

type adminActiveTasksResponse struct {
	Tasks []realtime.AdminActiveTask `json:"tasks"`
	Total int                        `json:"total"`
}

// handleAdminOnlineUsers 获取所有有活跃任务的用户
func (s *Server) handleAdminOnlineUsers(w http.ResponseWriter, r *http.Request) {
	initData := ""
	limit := 0

	switch r.Method {
	case http.MethodGet:
		initData = strings.TrimSpace(r.URL.Query().Get("initData"))
		if initData == "" {
			initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
		}
		limitRaw := strings.TrimSpace(r.URL.Query().Get("limit"))
		if limitRaw != "" {
			if n, err := strconv.Atoi(limitRaw); err == nil {
				limit = n
			}
		}
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var req struct {
			InitData string `json:"initData"`
			Limit    int    `json:"limit"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "无效的 JSON 请求体", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)
		limit = req.Limit
	default:
		http.Error(w, "不支持的请求方法", http.StatusMethodNotAllowed)
		return
	}

	if config.AppConfig == nil {
		http.Error(w, "配置未加载", http.StatusInternalServerError)
		return
	}

	parsed, err := ParseTelegramWebAppInitData(initData, config.AppConfig.TelegramBotToken)
	if err != nil {
		if errors.Is(err, ErrMissingInitData) {
			http.Error(w, "缺少 initData", http.StatusBadRequest)
		} else {
			http.Error(w, "无效的 initData", http.StatusUnauthorized)
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
		http.Error(w, "加载用户失败", http.StatusInternalServerError)
		return
	}

	accessService := userSvc.NewAccessService()
	if !accessService.IsAdminUser(user.ID) {
		http.Error(w, "无管理员权限", http.StatusForbidden)
		return
	}

	adminService := realtime.NewAdminRealtimeService()
	users, err := adminService.ListAllOnlineUsers(limit)
	if err != nil {
		http.Error(w, "加载在线用户失败", http.StatusInternalServerError)
		return
	}

	resp := adminOnlineUsersResponse{
		Users: users,
		Total: len(users),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleAdminActiveTasks 获取所有活跃任务
func (s *Server) handleAdminActiveTasks(w http.ResponseWriter, r *http.Request) {
	initData := ""
	limit := 0

	switch r.Method {
	case http.MethodGet:
		initData = strings.TrimSpace(r.URL.Query().Get("initData"))
		if initData == "" {
			initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
		}
		limitRaw := strings.TrimSpace(r.URL.Query().Get("limit"))
		if limitRaw != "" {
			if n, err := strconv.Atoi(limitRaw); err == nil {
				limit = n
			}
		}
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var req struct {
			InitData string `json:"initData"`
			Limit    int    `json:"limit"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "无效的 JSON 请求体", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)
		limit = req.Limit
	default:
		http.Error(w, "不支持的请求方法", http.StatusMethodNotAllowed)
		return
	}

	if config.AppConfig == nil {
		http.Error(w, "配置未加载", http.StatusInternalServerError)
		return
	}

	parsed, err := ParseTelegramWebAppInitData(initData, config.AppConfig.TelegramBotToken)
	if err != nil {
		if errors.Is(err, ErrMissingInitData) {
			http.Error(w, "缺少 initData", http.StatusBadRequest)
		} else {
			http.Error(w, "无效的 initData", http.StatusUnauthorized)
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
		http.Error(w, "加载用户失败", http.StatusInternalServerError)
		return
	}

	accessService := userSvc.NewAccessService()
	if !accessService.IsAdminUser(user.ID) {
		http.Error(w, "无管理员权限", http.StatusForbidden)
		return
	}

	adminService := realtime.NewAdminRealtimeService()
	tasks, err := adminService.ListAllActiveTasks(limit)
	if err != nil {
		http.Error(w, "加载活跃任务失败", http.StatusInternalServerError)
		return
	}

	resp := adminActiveTasksResponse{
		Tasks: tasks,
		Total: len(tasks),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
