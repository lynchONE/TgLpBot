package web_server

import (
	"strings"

	"TgLpBot/base/database"
	"TgLpBot/base/models"

	"github.com/ethereum/go-ethereum/common"
)

func normalizeSmartMoneyWalletAddress(value string) string {
	addr := strings.TrimSpace(value)
	if !common.IsHexAddress(addr) {
		return ""
	}
	return strings.ToLower(common.HexToAddress(addr).Hex())
}

func loadSmartMoneyWalletLabels(userID uint, chain string) map[string]string {
	out := make(map[string]string)
	if userID == 0 || database.DB == nil {
		return out
	}
	chain = strings.ToLower(strings.TrimSpace(chain))
	if chain == "" {
		chain = "bsc"
	}

	var watchedRows []models.SmartMoneyWatchedWallet
	if err := database.DB.
		Select("wallet_address", "label").
		Where("user_id = ? AND chain = ? AND label <> ''", userID, chain).
		Find(&watchedRows).Error; err == nil {
		for _, row := range watchedRows {
			addr := normalizeSmartMoneyWalletAddress(row.WalletAddress)
			label := strings.TrimSpace(row.Label)
			if addr == "" || label == "" {
				continue
			}
			out[addr] = label
		}
	}

	var labelRows []models.SmartMoneyWalletLabel
	if err := database.DB.
		Where("user_id = ? AND chain = ? AND label <> ''", userID, chain).
		Find(&labelRows).Error; err == nil {
		for _, row := range labelRows {
			addr := normalizeSmartMoneyWalletAddress(row.WalletAddress)
			label := strings.TrimSpace(row.Label)
			if addr == "" || label == "" {
				continue
			}
			out[addr] = label
		}
	}

	return out
}

func saveSmartMoneyWalletLabel(userID uint, chain string, walletAddress string, label string) error {
	if userID == 0 || database.DB == nil {
		return nil
	}
	addr := normalizeSmartMoneyWalletAddress(walletAddress)
	if addr == "" {
		return nil
	}
	chain = strings.ToLower(strings.TrimSpace(chain))
	if chain == "" {
		chain = "bsc"
	}
	label = strings.TrimSpace(label)
	if len(label) > 100 {
		label = label[:100]
	}

	if label == "" {
		return database.DB.
			Where("user_id = ? AND chain = ? AND wallet_address = ?", userID, chain, addr).
			Delete(&models.SmartMoneyWalletLabel{}).Error
	}

	record := models.SmartMoneyWalletLabel{
		UserID:        userID,
		Chain:         chain,
		WalletAddress: addr,
	}
	return database.DB.
		Where("user_id = ? AND chain = ? AND wallet_address = ?", userID, chain, addr).
		Assign(models.SmartMoneyWalletLabel{Label: label}).
		FirstOrCreate(&record).Error
}
