package web_server

import (
	"TgLpBot/base/clickhouse"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gorm.io/gorm"
)

const maxWatchedWalletsPerUser = 50

func (s *Server) handleSmartMoneyWatchedWallets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleWatchedWalletsGet(w, r)
	case http.MethodPost:
		s.handleWatchedWalletsAdd(w, r)
	case http.MethodDelete:
		s.handleWatchedWalletsRemove(w, r)
	case http.MethodPut:
		s.handleWatchedWalletsUpdateLabel(w, r)
	case http.MethodOptions:
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func authenticateSmartMoneyRequest(r *http.Request) (*models.User, int, string) {
	initData := initDataFromQuery(r)
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		return nil, status, msg
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		return nil, status, msg
	}
	if status != 0 {
		return nil, status, msg
	}
	if status, msg := requireSmartMoneyPermission(check); status != 0 {
		return nil, status, msg
	}
	return user, 0, ""
}

type watchedWalletJSON struct {
	ID            uint   `json:"id"`
	Chain         string `json:"chain"`
	WalletAddress string `json:"wallet_address"`
	Label         string `json:"label"`
	CreatedAt     string `json:"created_at"`
}

func walletToJSON(w *models.SmartMoneyWatchedWallet) watchedWalletJSON {
	return watchedWalletJSON{
		ID:            w.ID,
		Chain:         w.Chain,
		WalletAddress: w.WalletAddress,
		Label:         w.Label,
		CreatedAt:     w.CreatedAt.Format(time.RFC3339),
	}
}

func (s *Server) handleWatchedWalletsGet(w http.ResponseWriter, r *http.Request) {
	user, status, msg := authenticateSmartMoneyRequest(r)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	chain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("chain")))
	if chain == "" {
		chain = "bsc"
	}

	if database.DB == nil {
		http.Error(w, "database not initialized", http.StatusInternalServerError)
		return
	}

	var wallets []models.SmartMoneyWatchedWallet
	if err := database.DB.Where("user_id = ? AND chain = ?", user.ID, chain).
		Order("created_at DESC").
		Find(&wallets).Error; err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	items := make([]watchedWalletJSON, 0, len(wallets))
	for i := range wallets {
		items = append(items, walletToJSON(&wallets[i]))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"wallets": items,
		"total":   len(items),
	})
}

type addWatchedWalletInput struct {
	Address string `json:"address"`
	Label   string `json:"label"`
}

type addWatchedWalletsRequest struct {
	InitData string                  `json:"initData"`
	Chain    string                  `json:"chain"`
	Wallets  []addWatchedWalletInput `json:"wallets"`
}

func (s *Server) handleWatchedWalletsAdd(w http.ResponseWriter, r *http.Request) {
	var req addWatchedWalletsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	if initData == "" {
		initData = initDataFromQuery(r)
	}
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	check, status2, msg2, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg2, status2)
		return
	}
	if status2 != 0 {
		http.Error(w, msg2, status2)
		return
	}
	if status, msg := requireSmartMoneyPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	chain := strings.ToLower(strings.TrimSpace(req.Chain))
	if chain == "" {
		chain = "bsc"
	}

	if len(req.Wallets) == 0 {
		http.Error(w, "no wallets provided", http.StatusBadRequest)
		return
	}

	if database.DB == nil {
		http.Error(w, "database not initialized", http.StatusInternalServerError)
		return
	}

	// Check current count
	var currentCount int64
	database.DB.Model(&models.SmartMoneyWatchedWallet{}).
		Where("user_id = ? AND chain = ?", user.ID, chain).
		Count(&currentCount)

	remaining := maxWatchedWalletsPerUser - int(currentCount)
	if remaining <= 0 {
		http.Error(w, fmt.Sprintf("已达到最大监控数量限制 (%d)", maxWatchedWalletsPerUser), http.StatusBadRequest)
		return
	}

	added := 0
	duplicates := 0
	var addedWallets []watchedWalletJSON
	var chEntries []clickhouse.WatchedWalletEntry

	for _, input := range req.Wallets {
		if added >= remaining {
			break
		}
		addr := strings.TrimSpace(input.Address)
		if !common.IsHexAddress(addr) {
			continue
		}
		addr = strings.ToLower(common.HexToAddress(addr).Hex())
		label := strings.TrimSpace(input.Label)
		if len(label) > 100 {
			label = label[:100]
		}

		record := models.SmartMoneyWatchedWallet{
			UserID:        user.ID,
			Chain:         chain,
			WalletAddress: addr,
			Label:         label,
		}

		result := database.DB.Where("user_id = ? AND chain = ? AND wallet_address = ?",
			user.ID, chain, addr).FirstOrCreate(&record)
		if result.Error != nil {
			continue
		}
		if result.RowsAffected == 0 {
			// Already existed - update label if provided
			if label != "" && record.Label != label {
				database.DB.Model(&record).Update("label", label)
				record.Label = label
			}
			duplicates++
		} else {
			added++
			chEntries = append(chEntries, clickhouse.WatchedWalletEntry{
				Chain:         chain,
				WalletAddress: addr,
			})
		}
		addedWallets = append(addedWallets, walletToJSON(&record))
	}

	// Sync to ClickHouse
	if s.ClickHouse != nil && len(chEntries) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.ClickHouse.UpsertWatchedWallets(ctx, chEntries)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"added":      added,
		"duplicates": duplicates,
		"wallets":    addedWallets,
	})
}

type removeWatchedWalletsRequest struct {
	InitData        string   `json:"initData"`
	Chain           string   `json:"chain"`
	WalletAddresses []string `json:"wallet_addresses"`
}

func (s *Server) handleWatchedWalletsRemove(w http.ResponseWriter, r *http.Request) {
	var req removeWatchedWalletsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	if initData == "" {
		initData = initDataFromQuery(r)
	}
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	check, status2, msg2, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg2, status2)
		return
	}
	if status2 != 0 {
		http.Error(w, msg2, status2)
		return
	}
	if status, msg := requireSmartMoneyPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	chain := strings.ToLower(strings.TrimSpace(req.Chain))
	if chain == "" {
		chain = "bsc"
	}

	if len(req.WalletAddresses) == 0 {
		http.Error(w, "no wallet addresses provided", http.StatusBadRequest)
		return
	}

	if database.DB == nil {
		http.Error(w, "database not initialized", http.StatusInternalServerError)
		return
	}

	normalized := make([]string, 0, len(req.WalletAddresses))
	for _, addr := range req.WalletAddresses {
		addr = strings.TrimSpace(addr)
		if common.IsHexAddress(addr) {
			normalized = append(normalized, strings.ToLower(common.HexToAddress(addr).Hex()))
		}
	}

	if len(normalized) == 0 {
		http.Error(w, "no valid wallet addresses", http.StatusBadRequest)
		return
	}

	result := database.DB.Where("user_id = ? AND chain = ? AND wallet_address IN ?",
		user.ID, chain, normalized).Delete(&models.SmartMoneyWatchedWallet{})

	deleted := int64(0)
	if result.Error == nil {
		deleted = result.RowsAffected
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deleted": deleted,
	})
}

type updateWatchedWalletLabelRequest struct {
	InitData      string `json:"initData"`
	Chain         string `json:"chain"`
	WalletAddress string `json:"wallet_address"`
	Label         string `json:"label"`
}

func (s *Server) handleWatchedWalletsUpdateLabel(w http.ResponseWriter, r *http.Request) {
	var req updateWatchedWalletLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	if initData == "" {
		initData = initDataFromQuery(r)
	}
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	check, status2, msg2, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg2, status2)
		return
	}
	if status2 != 0 {
		http.Error(w, msg2, status2)
		return
	}
	if status, msg := requireSmartMoneyPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	chain := strings.ToLower(strings.TrimSpace(req.Chain))
	if chain == "" {
		chain = "bsc"
	}

	addr := strings.TrimSpace(req.WalletAddress)
	if !common.IsHexAddress(addr) {
		http.Error(w, "invalid wallet address", http.StatusBadRequest)
		return
	}
	addr = strings.ToLower(common.HexToAddress(addr).Hex())

	label := strings.TrimSpace(req.Label)
	if len(label) > 100 {
		label = label[:100]
	}

	if database.DB == nil {
		http.Error(w, "database not initialized", http.StatusInternalServerError)
		return
	}

	var record models.SmartMoneyWatchedWallet
	if err := database.DB.Where("user_id = ? AND chain = ? AND wallet_address = ?",
		user.ID, chain, addr).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "wallet not found", http.StatusNotFound)
			return
		}
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	database.DB.Model(&record).Update("label", label)
	record.Label = label

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"wallet": walletToJSON(&record),
	})
}
