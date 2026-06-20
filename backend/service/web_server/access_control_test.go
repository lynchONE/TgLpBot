package web_server

import (
	"net/http"
	"testing"

	"TgLpBot/base/models"
	userSvc "TgLpBot/service/user"
)

func TestRequireAnyModulePermission_AllowsMatchingModule(t *testing.T) {
	enabledModules, err := models.AccessModuleKeysToJSON([]string{models.AccessModuleSwap})
	if err != nil {
		t.Fatalf("build enabled modules: %v", err)
	}

	check := userSvc.AccessCheck{
		Allowed: true,
		Access: &models.UserAccess{
			MiniAppEnabled: true,
			EnabledModules: enabledModules,
		},
	}

	status, msg := requireAnyModulePermission(
		check,
		models.AccessModuleAssets,
		models.AccessModuleSwap,
	)
	if status != 0 || msg != "" {
		t.Fatalf("expected permission to pass, got status=%d msg=%q", status, msg)
	}
}

func TestRequireAnyModulePermission_DeniesWhenNoModuleMatches(t *testing.T) {
	enabledModules, err := models.AccessModuleKeysToJSON([]string{models.AccessModuleHotPools})
	if err != nil {
		t.Fatalf("build enabled modules: %v", err)
	}

	check := userSvc.AccessCheck{
		Allowed: true,
		Access: &models.UserAccess{
			MiniAppEnabled: true,
			EnabledModules: enabledModules,
		},
	}

	status, _ := requireAnyModulePermission(
		check,
		models.AccessModuleAssets,
		models.AccessModuleSwap,
	)
	if status != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, status)
	}
}

func TestRequireAnyModulePermission_AllowsAdmin(t *testing.T) {
	check := userSvc.AccessCheck{Allowed: true, IsAdmin: true}

	status, msg := requireAnyModulePermission(check, models.AccessModuleSwap)
	if status != 0 || msg != "" {
		t.Fatalf("expected admin permission to pass, got status=%d msg=%q", status, msg)
	}
}
