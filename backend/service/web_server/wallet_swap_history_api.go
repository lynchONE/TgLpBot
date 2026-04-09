package web_server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/token_metadata"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"

	"github.com/ethereum/go-ethereum/common"
)

type walletSwapHistoryRequest struct {
	InitData string `json:"initData"`
	WalletID uint   `json:"wallet_id,omitempty"`
	Chain    string `json:"chain,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

type walletSwapHistoryToken struct {
	Address  string `json:"address"`
	Symbol   string `json:"symbol,omitempty"`
	Name     string `json:"name,omitempty"`
	LogoURL  string `json:"logo_url,omitempty"`
	IsNative bool   `json:"is_native,omitempty"`
}

type walletSwapHistoryRow struct {
	ID             uint                   `json:"id"`
	Chain          string                 `json:"chain"`
	Status         string                 `json:"status"`
	TxHash         string                 `json:"tx_hash"`
	TxURL          string                 `json:"tx_url,omitempty"`
	CreatedAt      string                 `json:"created_at"`
	FromToken      walletSwapHistoryToken `json:"from_token"`
	ToToken        walletSwapHistoryToken `json:"to_token"`
	AmountIn       string                 `json:"amount_in"`
	AmountOut      string                 `json:"amount_out"`
	AmountInFloat  string                 `json:"amount_in_float,omitempty"`
	AmountOutFloat string                 `json:"amount_out_float,omitempty"`
	GasUsed        uint64                 `json:"gas_used,omitempty"`
	BlockNumber    uint64                 `json:"block_number,omitempty"`
}

type walletSwapHistoryResponse struct {
	OK      bool                   `json:"ok"`
	Chain   string                 `json:"chain"`
	Total   int64                  `json:"total"`
	Records []walletSwapHistoryRow `json:"records"`
}

func walletSwapShortAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if len(addr) <= 10 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}

func walletSwapHumanAmount(raw string, decimals int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return fmt.Sprintf("%.6f", amountToFloat(raw, decimals))
}

func walletSwapTokenMeta(chain string, addr string, metaByAddr map[string]models.TokenMetadata, cc config.ChainConfig) walletSwapHistoryToken {
	addr = strings.TrimSpace(addr)
	if strings.EqualFold(addr, nativePseudoTokenAddress) {
		symbol := nativeSymbolForChainConfig(chain, cc)
		return walletSwapHistoryToken{
			Address:  nativePseudoTokenAddress,
			Symbol:   symbol,
			Name:     symbol,
			IsNative: true,
		}
	}

	normalized := token_metadata.NormalizeTokenAddress(addr)
	meta := metaByAddr[normalized]
	symbol := strings.TrimSpace(meta.Symbol)
	name := strings.TrimSpace(meta.Name)
	if symbol == "" {
		if strings.EqualFold(normalized, cc.StableAddress) {
			symbol = stableSymbolForChainConfig(cc)
		} else {
			symbol = walletSwapShortAddress(normalized)
		}
	}
	if name == "" {
		name = symbol
	}
	return walletSwapHistoryToken{
		Address: normalized,
		Symbol:  symbol,
		Name:    name,
		LogoURL: strings.TrimSpace(meta.LogoURL),
	}
}

func walletSwapTokenDecimals(clientChain string, addr string, decimalsCache map[string]int) int {
	addr = strings.TrimSpace(addr)
	if strings.EqualFold(addr, nativePseudoTokenAddress) {
		return 18
	}
	if cached, ok := decimalsCache[strings.ToLower(addr)]; ok && cached > 0 {
		return cached
	}

	exec, err := chainexec.GetEVM(clientChain)
	if err != nil || exec == nil || exec.Client() == nil || !common.IsHexAddress(addr) {
		return 18
	}
	decimals := int(tokenDecimals(exec.Client(), common.HexToAddress(addr)))
	if decimals <= 0 {
		decimals = 18
	}
	decimalsCache[strings.ToLower(addr)] = decimals
	return decimals
}

func (s *Server) handleWalletSwapHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req walletSwapHistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	cfgService := userSvc.NewGlobalConfigService()
	cfg, err := cfgService.GetOrCreate(user.ID)
	if err != nil {
		http.Error(w, "failed to load config", http.StatusInternalServerError)
		return
	}

	chain := strings.TrimSpace(req.Chain)
	if chain == "" {
		if cfg != nil && !cfg.MultiChainEnabled {
			chain = config.PickEnabledChain(cfg.DefaultChain)
		} else {
			chain = config.PickEnabledChain("bsc")
		}
	} else {
		chain = config.NormalizeChain(chain)
	}

	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	walletService := wallet.NewWalletService()
	wlt, err := walletService.ResolveTaskWallet(user.ID, req.WalletID, "")
	if err != nil || wlt == nil {
		http.Error(w, "wallet not found", http.StatusBadRequest)
		return
	}

	db := database.DB.Model(&models.Transaction{}).
		Where("user_id = ? AND chain = ? AND type = ? AND from_address = ?", user.ID, chain, models.TxTypeSwap, strings.TrimSpace(wlt.Address))

	var total int64
	db.Count(&total)

	var records []models.Transaction
	if err := db.Order("created_at DESC").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		http.Error(w, "failed to query wallet swap history", http.StatusInternalServerError)
		return
	}

	cc, _ := config.AppConfig.GetChainConfig(chain)
	addressSet := make(map[string]struct{})
	for _, rec := range records {
		for _, addr := range []string{rec.TokenInAddress, rec.TokenOutAddress} {
			normalized := token_metadata.NormalizeTokenAddress(addr)
			if normalized == "" || strings.EqualFold(normalized, nativePseudoTokenAddress) {
				continue
			}
			addressSet[normalized] = struct{}{}
		}
	}

	addresses := make([]string, 0, len(addressSet))
	for addr := range addressSet {
		addresses = append(addresses, addr)
	}
	sort.Strings(addresses)

	metaByAddr := map[string]models.TokenMetadata{}
	if len(addresses) > 0 && s != nil && s.TokenMeta != nil {
		metaByAddr, _ = s.TokenMeta.GetBatch(r.Context(), chain, addresses)
	}

	decimalsCache := make(map[string]int, len(addresses))
	rows := make([]walletSwapHistoryRow, 0, len(records))
	for _, rec := range records {
		fromDecimals := walletSwapTokenDecimals(chain, rec.TokenInAddress, decimalsCache)
		toDecimals := walletSwapTokenDecimals(chain, rec.TokenOutAddress, decimalsCache)

		rows = append(rows, walletSwapHistoryRow{
			ID:             rec.ID,
			Chain:          rec.Chain,
			Status:         string(rec.Status),
			TxHash:         rec.TxHash,
			TxURL:          explorerTxURLHelper(rec.Chain, rec.TxHash),
			CreatedAt:      rec.CreatedAt.Format("2006-01-02 15:04:05"),
			FromToken:      walletSwapTokenMeta(chain, rec.TokenInAddress, metaByAddr, cc),
			ToToken:        walletSwapTokenMeta(chain, rec.TokenOutAddress, metaByAddr, cc),
			AmountIn:       rec.AmountIn,
			AmountOut:      rec.AmountOut,
			AmountInFloat:  walletSwapHumanAmount(rec.AmountIn, fromDecimals),
			AmountOutFloat: walletSwapHumanAmount(rec.AmountOut, toDecimals),
			GasUsed:        rec.GasUsed,
			BlockNumber:    rec.BlockNumber,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(walletSwapHistoryResponse{
		OK:      true,
		Chain:   chain,
		Total:   total,
		Records: rows,
	})
}
