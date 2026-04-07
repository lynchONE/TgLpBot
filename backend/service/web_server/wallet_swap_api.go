package web_server

import (
	"encoding/json"
	"net/http"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/service/liquidity"
	userSvc "TgLpBot/service/user"
)

// --- Wallet Swap Preview (scan tokens) ---

type walletSwapPreviewRequest struct {
	InitData    string  `json:"initData"`
	Chain       string  `json:"chain,omitempty"`
	MinValueUSD float64 `json:"min_value_usd,omitempty"`
}

type walletSwapTokenRow struct {
	Address   string  `json:"address"`
	Symbol    string  `json:"symbol"`
	Balance   string  `json:"balance"`
	ValueUSDT float64 `json:"value_usdt"`
}

type walletSwapPreviewResponse struct {
	OK     bool                 `json:"ok"`
	Chain  string               `json:"chain"`
	Tokens []walletSwapTokenRow `json:"tokens"`
}

func (s *Server) handleWalletSwapPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req walletSwapPreviewRequest
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

	minVal := req.MinValueUSD
	if minVal <= 0 {
		minVal = 0.1
	}

	lpService := liquidity.NewLiquidityService()
	tokens, err := lpService.ScanWalletTokensForSwapForChain(user.ID, chain, minVal)
	if err != nil {
		http.Error(w, "scan failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rows := make([]walletSwapTokenRow, 0, len(tokens))
	for _, t := range tokens {
		rows = append(rows, walletSwapTokenRow{
			Address:   t.Address.Hex(),
			Symbol:    t.Symbol,
			Balance:   t.Balance,
			ValueUSDT: t.ValueUSDT,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(walletSwapPreviewResponse{
		OK:     true,
		Chain:  chain,
		Tokens: rows,
	})
}

// --- Wallet Swap Execute ---

type walletSwapExecuteRequest struct {
	InitData        string  `json:"initData"`
	Chain           string  `json:"chain,omitempty"`
	SlippagePercent float64 `json:"slippage_percent,omitempty"`
}

type walletSwapExecuteResponse struct {
	OK      bool     `json:"ok"`
	Chain   string   `json:"chain"`
	Swapped []string `json:"swapped,omitempty"`
	Failed  []string `json:"failed,omitempty"`
}

func (s *Server) handleWalletSwapExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req walletSwapExecuteRequest
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

	slippage := req.SlippagePercent
	if slippage <= 0 {
		if cfg != nil && cfg.SlippageTolerance > 0 {
			slippage = cfg.SlippageTolerance
		} else {
			slippage = 1.0
		}
	}

	lpService := liquidity.NewLiquidityService()
	report, err := lpService.SwapWalletOtherTokensToUSDTForChain(user.ID, chain, slippage)
	if err != nil {
		http.Error(w, "swap failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(walletSwapExecuteResponse{
		OK:      true,
		Chain:   chain,
		Swapped: report.Swapped,
		Failed:  report.Failed,
	})
}
