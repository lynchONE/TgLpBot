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
		return http.StatusForbidden, "未授权"
	}
	if !check.Access.MiniAppEnabled {
		return http.StatusForbidden, "未开通 MiniApp 权限"
	}
	return 0, ""
}

func requireModulePermission(check userSvc.AccessCheck, moduleKey string) (int, string) {
	if status, msg := requireMiniAppPermission(check); status != 0 {
		return status, msg
	}
	if check.IsAdmin {
		return 0, ""
	}
	moduleKey = strings.TrimSpace(moduleKey)
	if !models.IsAccessModuleKey(moduleKey) {
		return http.StatusForbidden, "unknown module"
	}
	if check.Access == nil {
		return http.StatusForbidden, "未授权"
	}
	enabled, err := models.AccessModuleKeysFromJSON(check.Access.EnabledModules)
	if err != nil {
		if errors.Is(err, models.ErrAccessModulesNotConfigured) {
			return http.StatusForbidden, "未配置功能模块权限"
		}
		return http.StatusForbidden, "功能模块权限配置错误"
	}
	if !models.AccessModuleKeysContain(enabled, moduleKey) {
		return http.StatusForbidden, "未授权功能模块"
	}
	return 0, ""
}

func enabledModulesForAccessCheck(check userSvc.AccessCheck) ([]string, error) {
	if check.IsAdmin {
		return models.DefaultAccessModuleKeys(), nil
	}
	if check.Access == nil {
		return []string{}, nil
	}
	return models.AccessModuleKeysFromJSON(check.Access.EnabledModules)
}

func authenticateAdminWebAppUser(initData string) (*models.User, int, string) {
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		return nil, status, msg
	}
	accessService := userSvc.NewAccessService()
	if !accessService.IsAdminUser(user.ID) {
		return nil, http.StatusForbidden, "forbidden"
	}
	return user, 0, ""
}

var authenticateAdminWebAppUserForAdminHandlers = authenticateAdminWebAppUser
