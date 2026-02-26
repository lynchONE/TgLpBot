package bot

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// resolveNewPositionWallet returns the wallet selected in the /newposition flow.
// Fallback: user's default wallet (for legacy flows / missing session).
func (b *Bot) resolveNewPositionWallet(userID uint, telegramID int64) (*models.Wallet, error) {
	if b == nil || b.walletService == nil {
		return nil, fmt.Errorf("bot not initialized")
	}
	if userID == 0 || telegramID == 0 {
		return nil, fmt.Errorf("invalid user_id or telegram_id")
	}

	raw, _ := database.GetUserSession(telegramID, sessionNewPositionWalletID)
	raw = strings.TrimSpace(raw)
	if raw != "" {
		if id64, err := strconv.ParseUint(raw, 10, 64); err == nil && id64 > 0 {
			if w, err := b.walletService.GetWalletByID(userID, uint(id64)); err == nil && w != nil {
				return w, nil
			}
		}
	}
	return b.walletService.GetDefaultWallet(userID)
}

// ensureNewPositionWalletSession makes sure sessionNewPositionWalletID is set and returns the resolved wallet.
func (b *Bot) ensureNewPositionWalletSession(userID uint, telegramID int64) (*models.Wallet, error) {
	w, err := b.resolveNewPositionWallet(userID, telegramID)
	if err != nil {
		return nil, err
	}
	if w == nil || w.ID == 0 {
		return nil, fmt.Errorf("wallet not found")
	}
	_ = database.SetUserSession(telegramID, sessionNewPositionWalletID, fmt.Sprintf("%d", w.ID), 30*time.Minute)
	return w, nil
}
