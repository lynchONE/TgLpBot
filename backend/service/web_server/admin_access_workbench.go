package web_server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	userSvc "TgLpBot/service/user"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
)

type adminAccessRequest struct {
	InitData string `json:"initData"`
	Action   string `json:"action"`

	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Query    string `json:"query"`

	UserID     uint  `json:"userId"`
	UserIDAlt  uint  `json:"user_id"`
	TelegramID int64 `json:"telegram_id"`

	ActiveFrom      *string `json:"active_from"`
	ActiveTo        *string `json:"active_to"`
	ClearActiveFrom bool    `json:"clear_active_from"`
	ClearActiveTo   bool    `json:"clear_active_to"`

	MaxWallets     *int    `json:"max_wallets"`
	MaxActiveTasks *int    `json:"max_active_tasks"`
	MiniAppEnabled *bool   `json:"mini_app_enabled"`
	MiniAppCamel   *bool   `json:"miniAppEnabled"`
	Note           *string `json:"note"`
}

type adminAuthCodesRequest struct {
	InitData string `json:"initData"`
	Action   string `json:"action"`

	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Query    string `json:"query"`
	CodeID   uint   `json:"code_id"`

	ActiveFrom     *string `json:"active_from"`
	ActiveTo       *string `json:"active_to"`
	ClearActiveTo  bool    `json:"clear_active_to"`
	MaxWallets     *int    `json:"max_wallets"`
	MaxActiveTasks *int    `json:"max_active_tasks"`
	MaxRedemptions *int    `json:"max_redemptions"`
	MiniAppEnabled *bool   `json:"mini_app_enabled"`
	Note           *string `json:"note"`
}

type adminAnnouncementsRequest struct {
	InitData string `json:"initData"`
	Action   string `json:"action"`

	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

type adminAccessDTO struct {
	UserID          uint       `json:"user_id"`
	TelegramID      int64      `json:"telegram_id"`
	Username        string     `json:"username"`
	FirstName       string     `json:"first_name"`
	LastName        string     `json:"last_name"`
	HasAccess       bool       `json:"has_access"`
	Status          string     `json:"status"`
	ActiveFrom      *time.Time `json:"active_from,omitempty"`
	ActiveTo        *time.Time `json:"active_to,omitempty"`
	MaxWallets      int        `json:"max_wallets"`
	MaxActiveTasks  int        `json:"max_active_tasks"`
	MiniAppEnabled  bool       `json:"mini_app_enabled"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	RevokedByUserID *uint      `json:"revoked_by_user_id,omitempty"`
	Note            string     `json:"note"`
	WalletCount     int64      `json:"wallet_count"`
	ActiveTaskCount int64      `json:"active_task_count"`
	CreatedAt       *time.Time `json:"created_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

type adminAuthCodeDTO struct {
	ID              uint       `json:"id"`
	Code            string     `json:"code"`
	Status          string     `json:"status"`
	CreatedByUserID uint       `json:"created_by_user_id"`
	Note            string     `json:"note"`
	ActiveFrom      *time.Time `json:"active_from,omitempty"`
	ActiveTo        *time.Time `json:"active_to,omitempty"`
	MaxRedemptions  int        `json:"max_redemptions"`
	RedeemedCount   int        `json:"redeemed_count"`
	MaxWallets      int        `json:"max_wallets"`
	MaxActiveTasks  int        `json:"max_active_tasks"`
	MiniAppEnabled  bool       `json:"mini_app_enabled"`
	DisabledAt      *time.Time `json:"disabled_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (s *Server) handleAdminUserAccess(w http.ResponseWriter, r *http.Request) {
	s.handleAdminAccess(w, r)
}

func (s *Server) handleAdminAccess(w http.ResponseWriter, r *http.Request) {
	initData, req, ok := decodeAdminAccessRequest(w, r)
	if !ok {
		return
	}
	adminUser, status, msg := authenticateAdminWebAppUserForAdminHandlers(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		if r.Method == http.MethodGet && (req.UserID != 0 || req.UserIDAlt != 0 || req.TelegramID != 0) {
			action = "get"
		} else if r.Method == http.MethodPost {
			action = "update"
		} else {
			action = "list"
		}
	}

	accessService := userSvc.NewAccessService()
	switch action {
	case "list", "search":
		rows, total, err := accessService.ListUserAccessAdminRows(req.Page, req.PageSize, req.Query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := make([]adminAccessDTO, 0, len(rows))
		for _, row := range rows {
			items = append(items, buildAdminAccessDTO(row.User, row.Access, row.WalletCount, row.ActiveTaskCount))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         true,
			"items":      items,
			"users":      items,
			"total":      total,
			"page":       normalizedPage(req.Page),
			"page_size":  normalizedPageSize(req.PageSize, 20, 100),
			"updated_at": time.Now(),
		})
	case "get":
		user, access, err := resolveAdminTargetUser(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		dto, err := buildAdminAccessDetail(accessService, user, access)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, dto)
	case "update", "save", "grant":
		user, _, err := resolveAdminTargetUser(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		activeFrom, err := parseOptionalAdminTime(req.ActiveFrom, "active_from")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		activeTo, err := parseOptionalAdminTime(req.ActiveTo, "active_to")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		miniAppEnabled := req.MiniAppEnabled
		if miniAppEnabled == nil {
			miniAppEnabled = req.MiniAppCamel
		}
		access, err := accessService.UpdateUserAccess(adminUser.ID, user.ID, userSvc.UpdateUserAccessInput{
			ActiveFrom:      activeFrom,
			ActiveTo:        activeTo,
			ClearActiveFrom: req.ClearActiveFrom,
			ClearActiveTo:   req.ClearActiveTo,
			MaxWallets:      req.MaxWallets,
			MaxActiveTasks:  req.MaxActiveTasks,
			MiniAppEnabled:  miniAppEnabled,
			Note:            req.Note,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		dto, err := buildAdminAccessDetail(accessService, access.User, access)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, dto)
	case "revoke", "disable":
		user, _, err := resolveAdminTargetUser(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := accessService.RevokeUserAccess(adminUser.ID, user.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		access, err := accessService.GetUserAccessWithUser(user.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dto, err := buildAdminAccessDetail(accessService, user, access)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, dto)
	case "restore", "enable":
		user, _, err := resolveAdminTargetUser(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := accessService.RestoreUserAccess(adminUser.ID, user.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		access, err := accessService.GetUserAccessWithUser(user.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dto, err := buildAdminAccessDetail(accessService, user, access)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, dto)
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
	}
}

func (s *Server) handleAdminAuthCodes(w http.ResponseWriter, r *http.Request) {
	initData, req, ok := decodeAdminAuthCodesRequest(w, r)
	if !ok {
		return
	}
	adminUser, status, msg := authenticateAdminWebAppUserForAdminHandlers(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		action = "list"
	}
	accessService := userSvc.NewAccessService()
	switch action {
	case "list", "search":
		codes, total, err := accessService.ListAuthCodesPaged(req.Page, req.PageSize, req.Query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := make([]adminAuthCodeDTO, 0, len(codes))
		for _, code := range codes {
			items = append(items, buildAdminAuthCodeDTO(code))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"items":     items,
			"codes":     items,
			"total":     total,
			"page":      normalizedPage(req.Page),
			"page_size": normalizedPageSize(req.PageSize, 20, 100),
		})
	case "create":
		activeFrom, err := parseOptionalAdminTime(req.ActiveFrom, "active_from")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		activeTo, err := parseOptionalAdminTime(req.ActiveTo, "active_to")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		in := userSvc.CreateAuthCodeInput{
			ActiveFrom:     activeFrom,
			ActiveTo:       activeTo,
			MaxWallets:     derefInt(req.MaxWallets, 1),
			MaxActiveTasks: derefInt(req.MaxActiveTasks, 1),
			MaxRedemptions: derefInt(req.MaxRedemptions, 1),
			MiniAppEnabled: req.MiniAppEnabled != nil && *req.MiniAppEnabled,
		}
		if req.Note != nil {
			in.Note = *req.Note
		}
		code, err := accessService.CreateAuthCode(adminUser.ID, in)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "code": buildAdminAuthCodeDTO(*code)})
	case "update":
		if req.CodeID == 0 {
			http.Error(w, "code_id required", http.StatusBadRequest)
			return
		}
		activeTo, err := parseOptionalAdminTime(req.ActiveTo, "active_to")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		code, err := accessService.UpdateAuthCode(req.CodeID, userSvc.UpdateAuthCodeInput{
			ActiveTo:       activeTo,
			ClearActiveTo:  req.ClearActiveTo,
			MaxWallets:     req.MaxWallets,
			MaxActiveTasks: req.MaxActiveTasks,
			MaxRedemptions: req.MaxRedemptions,
			MiniAppEnabled: req.MiniAppEnabled,
			Note:           req.Note,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "code": buildAdminAuthCodeDTO(*code)})
	case "disable":
		if req.CodeID == 0 {
			http.Error(w, "code_id required", http.StatusBadRequest)
			return
		}
		if err := accessService.DisableAuthCode(req.CodeID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		code, err := accessService.GetAuthCode(req.CodeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "code": buildAdminAuthCodeDTO(*code)})
	case "enable":
		if req.CodeID == 0 {
			http.Error(w, "code_id required", http.StatusBadRequest)
			return
		}
		if err := accessService.EnableAuthCode(req.CodeID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		code, err := accessService.GetAuthCode(req.CodeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "code": buildAdminAuthCodeDTO(*code)})
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
	}
}

func (s *Server) handleAdminAnnouncements(w http.ResponseWriter, r *http.Request) {
	initData, req, ok := decodeAdminAnnouncementsRequest(w, r)
	if !ok {
		return
	}
	adminUser, status, msg := authenticateAdminWebAppUserForAdminHandlers(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		action = "list"
	}
	switch action {
	case "list":
		items, total, err := listAnnouncements(req.Page, req.PageSize)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"items":     items,
			"total":     total,
			"page":      normalizedPage(req.Page),
			"page_size": normalizedPageSize(req.PageSize, 20, 100),
		})
	case "publish", "create":
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = "系统公告"
		}
		content := strings.TrimSpace(req.Content)
		if content == "" {
			http.Error(w, "content required", http.StatusBadRequest)
			return
		}
		announcement := models.Announcement{
			CreatedByUserID: adminUser.ID,
			Title:           title,
			Content:         content,
		}
		if database.DB == nil {
			http.Error(w, "database not initialized", http.StatusInternalServerError)
			return
		}
		if err := database.DB.Create(&announcement).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sent, failed, err := broadcastAnnouncement(title, content)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		announcement.SentCount = sent
		announcement.FailedCount = failed
		if err := database.DB.Model(&announcement).Updates(map[string]any{
			"sent_count":   sent,
			"failed_count": failed,
		}).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":           true,
			"announcement": announcement,
			"sent_count":   sent,
			"failed_count": failed,
		})
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
	}
}

func decodeAdminAccessRequest(w http.ResponseWriter, r *http.Request) (string, adminAccessRequest, bool) {
	var req adminAccessRequest
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		req.InitData = initDataFromQuery(r)
		req.Action = strings.TrimSpace(q.Get("action"))
		req.Query = strings.TrimSpace(q.Get("query"))
		req.Page = atoiDefault(q.Get("page"), 1)
		req.PageSize = atoiDefault(q.Get("page_size"), 20)
		if req.PageSize == 20 {
			req.PageSize = atoiDefault(q.Get("pageSize"), 20)
		}
		req.UserID = parseUintQuery(q.Get("userId"))
		if req.UserID == 0 {
			req.UserID = parseUintQuery(q.Get("user_id"))
		}
		req.TelegramID = int64(parseUintQuery(q.Get("telegram_id")))
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return "", req, false
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return "", req, false
	}
	return strings.TrimSpace(req.InitData), req, true
}

func decodeAdminAuthCodesRequest(w http.ResponseWriter, r *http.Request) (string, adminAuthCodesRequest, bool) {
	var req adminAuthCodesRequest
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		req.InitData = initDataFromQuery(r)
		req.Action = strings.TrimSpace(q.Get("action"))
		req.Query = strings.TrimSpace(q.Get("query"))
		req.Page = atoiDefault(q.Get("page"), 1)
		req.PageSize = atoiDefault(q.Get("page_size"), 20)
		if req.PageSize == 20 {
			req.PageSize = atoiDefault(q.Get("pageSize"), 20)
		}
		req.CodeID = parseUintQuery(q.Get("code_id"))
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return "", req, false
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return "", req, false
	}
	return strings.TrimSpace(req.InitData), req, true
}

func decodeAdminAnnouncementsRequest(w http.ResponseWriter, r *http.Request) (string, adminAnnouncementsRequest, bool) {
	var req adminAnnouncementsRequest
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		req.InitData = initDataFromQuery(r)
		req.Action = strings.TrimSpace(q.Get("action"))
		req.Page = atoiDefault(q.Get("page"), 1)
		req.PageSize = atoiDefault(q.Get("page_size"), 20)
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return "", req, false
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return "", req, false
	}
	return strings.TrimSpace(req.InitData), req, true
}

func resolveAdminTargetUser(req adminAccessRequest) (models.User, *models.UserAccess, error) {
	if database.DB == nil {
		return models.User{}, nil, errors.New("database not initialized")
	}
	userID := req.UserID
	if userID == 0 {
		userID = req.UserIDAlt
	}
	var user models.User
	db := database.DB
	if userID > 0 {
		if err := db.First(&user, userID).Error; err != nil {
			return models.User{}, nil, err
		}
	} else if req.TelegramID > 0 {
		if err := db.Where("telegram_id = ?", req.TelegramID).First(&user).Error; err != nil {
			return models.User{}, nil, err
		}
	} else {
		return models.User{}, nil, errors.New("user_id required")
	}
	var access models.UserAccess
	if err := db.Preload("User").Where("user_id = ?", user.ID).First(&access).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return user, nil, nil
		}
		return models.User{}, nil, err
	}
	return user, &access, nil
}

func buildAdminAccessDetail(accessService *userSvc.AccessService, user models.User, access *models.UserAccess) (adminAccessDTO, error) {
	walletCount, err := accessService.CountUserWallets(user.ID)
	if err != nil {
		return adminAccessDTO{}, err
	}
	taskCount, err := accessService.CountUserActiveTasks(user.ID)
	if err != nil {
		return adminAccessDTO{}, err
	}
	return buildAdminAccessDTO(user, access, walletCount, taskCount), nil
}

func buildAdminAccessDTO(user models.User, access *models.UserAccess, walletCount, activeTaskCount int64) adminAccessDTO {
	dto := adminAccessDTO{
		UserID:          user.ID,
		TelegramID:      user.TelegramID,
		Username:        user.Username,
		FirstName:       user.FirstName,
		LastName:        user.LastName,
		Status:          "none",
		WalletCount:     walletCount,
		ActiveTaskCount: activeTaskCount,
	}
	if access == nil {
		return dto
	}
	status := "active"
	now := time.Now()
	if access.RevokedAt != nil {
		status = "revoked"
	} else if access.ActiveFrom != nil && now.Before(*access.ActiveFrom) {
		status = "pending"
	} else if access.ActiveTo != nil && now.After(*access.ActiveTo) {
		status = "expired"
	}
	createdAt := access.CreatedAt
	updatedAt := access.UpdatedAt
	dto.HasAccess = true
	dto.Status = status
	dto.ActiveFrom = access.ActiveFrom
	dto.ActiveTo = access.ActiveTo
	dto.MaxWallets = access.MaxWallets
	dto.MaxActiveTasks = access.MaxActiveTasks
	dto.MiniAppEnabled = access.MiniAppEnabled
	dto.RevokedAt = access.RevokedAt
	dto.RevokedByUserID = access.RevokedByUserID
	dto.Note = access.Note
	dto.CreatedAt = &createdAt
	dto.UpdatedAt = &updatedAt
	return dto
}

func buildAdminAuthCodeDTO(code models.AuthCode) adminAuthCodeDTO {
	status := "active"
	now := time.Now()
	if code.DisabledAt != nil {
		status = "disabled"
	} else if code.ActiveFrom != nil && now.Before(*code.ActiveFrom) {
		status = "pending"
	} else if code.ActiveTo != nil && now.After(*code.ActiveTo) {
		status = "expired"
	} else if code.MaxRedemptions > 0 && code.RedeemedCount >= code.MaxRedemptions {
		status = "exhausted"
	}
	return adminAuthCodeDTO{
		ID:              code.ID,
		Code:            code.Code,
		Status:          status,
		CreatedByUserID: code.CreatedByUserID,
		Note:            code.Note,
		ActiveFrom:      code.ActiveFrom,
		ActiveTo:        code.ActiveTo,
		MaxRedemptions:  code.MaxRedemptions,
		RedeemedCount:   code.RedeemedCount,
		MaxWallets:      code.MaxWallets,
		MaxActiveTasks:  code.MaxActiveTasks,
		MiniAppEnabled:  code.MiniAppEnabled,
		DisabledAt:      code.DisabledAt,
		CreatedAt:       code.CreatedAt,
		UpdatedAt:       code.UpdatedAt,
	}
}

func listAnnouncements(page, pageSize int) ([]models.Announcement, int64, error) {
	if database.DB == nil {
		return nil, 0, errors.New("database not initialized")
	}
	page = normalizedPage(page)
	pageSize = normalizedPageSize(pageSize, 20, 100)
	var total int64
	if err := database.DB.Model(&models.Announcement{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.Announcement
	if err := database.DB.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func broadcastAnnouncement(title, content string) (int, int, error) {
	if database.DB == nil {
		return 0, 0, errors.New("database not initialized")
	}
	if config.AppConfig == nil {
		return 0, 0, errors.New("config not loaded")
	}
	botToken := strings.TrimSpace(config.AppConfig.TelegramBotToken)
	if botToken == "" {
		return 0, 0, errors.New("telegram bot token not configured")
	}
	botAPI, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return 0, 0, err
	}
	var users []models.User
	if err := database.DB.Find(&users).Error; err != nil {
		return 0, 0, err
	}
	message := strings.TrimSpace(fmt.Sprintf("%s\n\n%s", title, content))
	sent := 0
	failed := 0
	for _, u := range users {
		if u.TelegramID == 0 {
			continue
		}
		msg := tgbotapi.NewMessage(u.TelegramID, message)
		if _, err := botAPI.Send(msg); err != nil {
			failed++
			continue
		}
		sent++
	}
	return sent, failed, nil
}

func parseOptionalAdminTime(raw *string, field string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("invalid %s", field)
}

func normalizedPage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func normalizedPageSize(pageSize, defaultValue, maxValue int) int {
	if defaultValue <= 0 {
		defaultValue = 20
	}
	if maxValue <= 0 {
		maxValue = 100
	}
	if pageSize <= 0 {
		return defaultValue
	}
	if pageSize > maxValue {
		return maxValue
	}
	return pageSize
}

func atoiDefault(raw string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return n
}

func parseUintQuery(raw string) uint {
	n, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return uint(n)
}

func derefInt(v *int, fallback int) int {
	if v == nil {
		return fallback
	}
	return *v
}
