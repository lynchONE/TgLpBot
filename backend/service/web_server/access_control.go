package web_server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/models"
	userSvc "TgLpBot/service/user"
)

func initDataFromQuery(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	initData := strings.TrimSpace(r.URL.Query().Get("initData"))
	if initData == "" {
		initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
	}
	return initData
}

func authenticateTelegramWebAppUser(initData string) (*models.User, int, string) {
	if config.AppConfig == nil {
		return nil, http.StatusInternalServerError, "config not loaded"
	}

	parsed, err := ParseTelegramWebAppInitData(initData, config.AppConfig.TelegramBotToken)
	if err != nil {
		if errors.Is(err, ErrMissingInitData) {
			return nil, http.StatusBadRequest, "missing initData"
		}
		return nil, http.StatusUnauthorized, "invalid initData"
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
		return nil, http.StatusInternalServerError, "failed to load user"
	}

	return user, 0, ""
}

func requireUserAccess(userID uint) (userSvc.AccessCheck, int, string, error) {
	accessService := userSvc.NewAccessService()
	check, err := accessService.CheckUserAccess(userID, time.Now())
	if err != nil {
		return check, http.StatusInternalServerError, "failed to check access", err
	}
	if !check.Allowed {
		reason := strings.TrimSpace(check.Reason)
		if reason == "" {
			reason = "forbidden"
		}
		return check, http.StatusForbidden, reason, nil
	}
	return check, 0, "", nil
}

func requireMiniAppPermission(check userSvc.AccessCheck) (int, string) {
	if check.IsAdmin {
		return 0, ""
	}
	if check.Access == nil {
		return http.StatusForbidden, "鏈巿鏉?"
	}
	if !check.Access.MiniAppEnabled {
		return http.StatusForbidden, "鏈紑閫?MiniApp 鏉冮檺"
	}
	return 0, ""
}
