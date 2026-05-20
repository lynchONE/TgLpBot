package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/token_metadata"
	userSvc "TgLpBot/service/user"
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

type walletSwapTokenMetadataRequest struct {
	InitData  string   `json:"initData"`
	Chain     string   `json:"chain,omitempty"`
	Addresses []string `json:"addresses"`
}

type walletSwapTokenMetadataRow struct {
	Address string `json:"address"`
	Symbol  string `json:"symbol,omitempty"`
	Name    string `json:"name,omitempty"`
	LogoURL string `json:"logo_url,omitempty"`
}

type walletSwapTokenMetadataResponse struct {
	OK     bool                         `json:"ok"`
	Chain  string                       `json:"chain"`
	Tokens []walletSwapTokenMetadataRow `json:"tokens"`
}

func (s *Server) handleWalletSwapTokenMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var req walletSwapTokenMetadataRequest
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
	if status, msg := requireModulePermission(check, models.AccessModuleSwap); status != 0 {
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

	normalized := make([]string, 0, len(req.Addresses))
	seen := make(map[string]struct{}, len(req.Addresses))
	for _, raw := range req.Addresses {
		addr := token_metadata.NormalizeTokenAddress(raw)
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		normalized = append(normalized, addr)
	}

	metaByAddr := map[string]models.TokenMetadata{}
	if len(normalized) > 0 && s != nil && s.TokenMeta != nil {
		metaByAddr, err = s.TokenMeta.GetBatch(r.Context(), chain, normalized)
		if err != nil {
			log.Printf("[WalletSwapTokenMetadata] load failed chain=%s err=%v", chain, err)
		}
	}

	rows := make([]walletSwapTokenMetadataRow, 0, len(normalized))
	for _, addr := range normalized {
		meta := metaByAddr[addr]
		rows = append(rows, walletSwapTokenMetadataRow{
			Address: addr,
			Symbol:  strings.TrimSpace(meta.Symbol),
			Name:    strings.TrimSpace(meta.Name),
			LogoURL: strings.TrimSpace(meta.LogoURL),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(walletSwapTokenMetadataResponse{
		OK:     true,
		Chain:  chain,
		Tokens: rows,
	})
}
