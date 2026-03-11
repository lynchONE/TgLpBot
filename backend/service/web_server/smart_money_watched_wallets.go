package web_server

import (
	"TgLpBot/base/clickhouse"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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
	Source        string `json:"source,omitempty"`
	Removable     bool   `json:"removable"`
	EditableLabel bool   `json:"editable_label"`
}

func walletToJSON(w *models.SmartMoneyWatchedWallet) watchedWalletJSON {
	return watchedWalletJSON{
		ID:            w.ID,
		Chain:         w.Chain,
		WalletAddress: w.WalletAddress,
		Label:         w.Label,
		CreatedAt:     w.CreatedAt.Format(time.RFC3339),
		Source:        "user_managed",
		Removable:     true,
		EditableLabel: true,
	}
}

type watchedWalletCHRow struct {
	WalletAddress string
	Source        string
	UpdatedAt     time.Time
}

func mergeWatchedWalletRows(groups ...[]watchedWalletCHRow) []watchedWalletCHRow {
	byAddress := make(map[string]watchedWalletCHRow)
	for _, rows := range groups {
		for _, row := range rows {
			addr := strings.ToLower(strings.TrimSpace(row.WalletAddress))
			if !common.IsHexAddress(addr) {
				continue
			}
			row.WalletAddress = addr
			if row.Source == "" {
				row.Source = "scan_add"
			}
			prev, ok := byAddress[addr]
			if !ok || row.UpdatedAt.After(prev.UpdatedAt) {
				byAddress[addr] = row
			}
		}
	}
	out := make([]watchedWalletCHRow, 0, len(byAddress))
	for _, row := range byAddress {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].WalletAddress < out[j].WalletAddress
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *Server) loadCHWatchedWallets(chain string) ([]watchedWalletCHRow, error) {
	out := make([]watchedWalletCHRow, 0, 128)
	if s == nil || s.ClickHouse == nil || s.ClickHouse.Conn == nil {
		return out, nil
	}
	chain = strings.ToLower(strings.TrimSpace(chain))
	if chain == "" {
		chain = "bsc"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := s.ClickHouse.Conn.Query(ctx, `
		SELECT
			wallet_address,
			argMax(source, updated_at) AS latest_source,
			max(updated_at) AS latest_updated_at
		FROM smart_lp_watched_wallets
		WHERE lowerUTF8(chain) = ?
		GROUP BY wallet_address
		ORDER BY latest_updated_at DESC
		LIMIT 5000
	`, chain)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		var row watchedWalletCHRow
		if err := rows.Scan(&row.WalletAddress, &row.Source, &row.UpdatedAt); err != nil {
			return out, err
		}
		row.WalletAddress = strings.ToLower(strings.TrimSpace(row.WalletAddress))
		row.Source = strings.ToLower(strings.TrimSpace(row.Source))
		if !common.IsHexAddress(row.WalletAddress) {
			continue
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Server) loadCHDiscoveredWalletsFromEvents(chain string, limit int) ([]watchedWalletCHRow, error) {
	out := make([]watchedWalletCHRow, 0, 128)
	if s == nil || s.ClickHouse == nil || s.ClickHouse.Conn == nil {
		return out, nil
	}
	chain = strings.ToLower(strings.TrimSpace(chain))
	if chain == "" {
		chain = "bsc"
	}
	if limit <= 0 || limit > 10000 {
		limit = 5000
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	q := fmt.Sprintf(`
		SELECT
			wallet_address,
			argMax(source, ts) AS source,
			max(ts) AS updated_at
		FROM smart_lp_events
		WHERE lowerUTF8(chain) = ?
			AND action = 'add'
			AND wallet_address != ''
		GROUP BY wallet_address
		ORDER BY updated_at DESC
		LIMIT %d
	`, limit)

	rows, err := s.ClickHouse.Conn.Query(ctx, q, chain)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		var row watchedWalletCHRow
		if err := rows.Scan(&row.WalletAddress, &row.Source, &row.UpdatedAt); err != nil {
			return out, err
		}
		row.WalletAddress = strings.ToLower(strings.TrimSpace(row.WalletAddress))
		row.Source = strings.ToLower(strings.TrimSpace(row.Source))
		if !common.IsHexAddress(row.WalletAddress) {
			continue
		}
		if row.Source == "" {
			row.Source = "scan_add"
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, nil
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
	labelMap := loadSmartMoneyWalletLabels(user.ID, chain)
	for i := range wallets {
		addr := normalizeSmartMoneyWalletAddress(wallets[i].WalletAddress)
		if addr == "" {
			continue
		}
		if label, ok := labelMap[addr]; ok {
			wallets[i].Label = label
		}
	}

	userByAddress := make(map[string]*models.SmartMoneyWatchedWallet, len(wallets))
	for i := range wallets {
		addr := strings.ToLower(strings.TrimSpace(wallets[i].WalletAddress))
		if !common.IsHexAddress(addr) {
			continue
		}
		userByAddress[addr] = &wallets[i]
	}

	items := make([]watchedWalletJSON, 0, len(wallets)+64)
	seen := make(map[string]struct{}, len(wallets)+64)
	systemTotal := 0
	warnings := make([]string, 0, 1)

	chWallets, chErr := s.loadCHWatchedWallets(chain)
	if chErr != nil {
		warnings = append(warnings, "ClickHouse 监控钱包查询失败，已尝试使用合约事件发现数据")
	}
	discoveredWallets, discoveredErr := s.loadCHDiscoveredWalletsFromEvents(chain, 5000)
	if discoveredErr != nil {
		warnings = append(warnings, "ClickHouse 合约发现钱包查询失败")
	}
	chWallets = mergeWatchedWalletRows(chWallets, discoveredWallets)

	for _, row := range chWallets {
		if row.Source == "user_removed" {
			continue
		}
		addr := row.WalletAddress
		if !common.IsHexAddress(addr) {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		if userRow, ok := userByAddress[addr]; ok {
			items = append(items, walletToJSON(userRow))
			seen[addr] = struct{}{}
			continue
		}

		createdAt := row.UpdatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		items = append(items, watchedWalletJSON{
			ID:            0,
			Chain:         chain,
			WalletAddress: addr,
			Label:         strings.TrimSpace(labelMap[addr]),
			CreatedAt:     createdAt.Format(time.RFC3339),
			Source:        row.Source,
			Removable:     true,
			EditableLabel: true,
		})
		seen[addr] = struct{}{}
		systemTotal++
	}

	for i := range wallets {
		addr := strings.ToLower(strings.TrimSpace(wallets[i].WalletAddress))
		if !common.IsHexAddress(addr) {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		items = append(items, walletToJSON(&wallets[i]))
		seen[addr] = struct{}{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"wallets":      items,
		"total":        len(items),
		"manual_total": len(wallets),
		"system_total": systemTotal,
		"max_manual":   maxWatchedWalletsPerUser,
		"warnings":     warnings,
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

	deletedDB := int64(0)
	if result.Error == nil {
		deletedDB = result.RowsAffected
	}
	_ = database.DB.Where("user_id = ? AND chain = ? AND wallet_address IN ?",
		user.ID, chain, normalized).Delete(&models.SmartMoneyWalletLabel{}).Error

	removedFromWatchlist := 0
	warnings := make([]string, 0, 1)
	if s.ClickHouse != nil && len(normalized) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.ClickHouse.MarkWatchedWalletsRemoved(ctx, chain, normalized); err != nil {
			warnings = append(warnings, fmt.Sprintf("clickhouse watchlist remove failed: %v", err))
		} else {
			removedFromWatchlist = len(normalized)
		}
	}

	deleted := int(deletedDB)
	if removedFromWatchlist > deleted {
		deleted = removedFromWatchlist
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deleted":                deleted,
		"db_deleted":             deletedDB,
		"watchlist_removed":      removedFromWatchlist,
		"warnings":               warnings,
		"watchlist_remove_chain": chain,
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
	err = database.DB.Where("user_id = ? AND chain = ? AND wallet_address = ?",
		user.ID, chain, addr).First(&record).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	if err == nil {
		database.DB.Model(&record).Update("label", label)
		record.Label = label
	}
	if err := saveSmartMoneyWalletLabel(user.ID, chain, addr, label); err != nil {
		http.Error(w, "update label failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"wallet": map[string]interface{}{
			"chain":          chain,
			"wallet_address": addr,
			"label":          label,
		},
	})
}
