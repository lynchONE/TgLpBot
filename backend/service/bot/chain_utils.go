package bot

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"fmt"
	"strings"
	"time"

	userSvc "TgLpBot/service/user"
)

const (
	sessionNewPositionChain = "new_position_chain"
	sessionNewPositionState = "awaiting_new_position_chain"
	sessionPendingPoolInput = "pending_pool_input"
)

func enabledChains() []string {
	if config.AppConfig != nil && len(config.AppConfig.EnabledChains) > 0 {
		out := make([]string, 0, len(config.AppConfig.EnabledChains))
		for _, c := range config.AppConfig.EnabledChains {
			ch := config.NormalizeChain(c)
			if ch == "" {
				continue
			}
			out = append(out, ch)
		}
		if len(out) > 0 {
			return out
		}
	}
	return []string{"bsc"}
}

func chainLabel(chain string) string {
	switch config.NormalizeChain(chain) {
	case "bsc":
		return "BSC"
	case "base":
		return "BASE"
	default:
		return strings.ToUpper(config.NormalizeChain(chain))
	}
}

// resolveNewPositionChain reads chain from session.
// It only falls back when exactly one chain is enabled; otherwise it returns error.
func resolveNewPositionChain(userID uint, telegramID int64) (string, error) {
	// Single-chain mode: always use user's default chain.
	if userID != 0 {
		cfg, err := userSvc.NewGlobalConfigService().GetOrCreate(userID)
		if err == nil && cfg != nil && !cfg.MultiChainEnabled {
			chain := config.PickEnabledChain(cfg.DefaultChain)
			_ = database.SetUserSession(telegramID, sessionNewPositionChain, chain, 30*time.Minute)
			return chain, nil
		}
	}

	raw, _ := database.GetUserSession(telegramID, sessionNewPositionChain)
	raw = strings.TrimSpace(raw)
	if raw != "" {
		return config.NormalizeChain(raw), nil
	}

	chains := enabledChains()
	if len(chains) == 1 {
		chain := config.NormalizeChain(chains[0])
		_ = database.SetUserSession(telegramID, sessionNewPositionChain, chain, 30*time.Minute)
		return chain, nil
	}
	return "", fmt.Errorf("chain not selected")
}
