package web_server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/service/exchange"
	"TgLpBot/service/liquidity"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"
)

// --- Wallet Swap Preview (scan tokens) ---

type walletSwapPreviewRequest struct {
	InitData    string  `json:"initData"`
	WalletID    uint    `json:"wallet_id,omitempty"`
	Chain       string  `json:"chain,omitempty"`
	MinValueUSD float64 `json:"min_value_usd,omitempty"`
}

type walletSwapTokenRow struct {
	Address   string  `json:"address"`
	Symbol    string  `json:"symbol"`
	Balance   string  `json:"balance"`
	ValueUSDT float64 `json:"value_usdt"`
	LogoURL   string  `json:"logo_url,omitempty"`
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
		minVal = 0.001
	}

	// 优先使用 OKX API 获取余额（快速）
	rows, err := s.getTokenBalancesFromOKX(user.ID, req.WalletID, chain, minVal)
	if err != nil {
		// 如果 OKX API 失败，回退到链上扫描
		lpService := liquidity.NewLiquidityService()
		tokens, scanErr := lpService.ScanWalletTokensForSwapForChainWithWallet(user.ID, req.WalletID, chain, minVal)
		if scanErr != nil {
			http.Error(w, "scan failed: "+scanErr.Error(), http.StatusInternalServerError)
			return
		}
		rows = make([]walletSwapTokenRow, 0, len(tokens))
		for _, t := range tokens {
			rows = append(rows, walletSwapTokenRow{
				Address:   t.Address.Hex(),
				Symbol:    t.Symbol,
				Balance:   t.Balance,
				ValueUSDT: t.ValueUSDT,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(walletSwapPreviewResponse{
		OK:     true,
		Chain:  chain,
		Tokens: rows,
	})
}

func (s *Server) getTokenBalancesFromOKX(userID uint, walletID uint, chain string, minValueUSD float64) ([]walletSwapTokenRow, error) {
	// 获取用户默认钱包地址
	walletService := wallet.NewWalletService()
	wlt, err := walletService.ResolveTaskWallet(userID, walletID, "")
	if err != nil || wlt == nil {
		return nil, fmt.Errorf("wallet not found")
	}

	// 转换链 ID
	chainIndex := config.ChainToOKXChainIndex(chain)
	if chainIndex == "" {
		return nil, fmt.Errorf("unsupported chain: %s", chain)
	}

	// 调用 OKX API
	okxService := exchange.NewOKXDexService()
	resp, err := okxService.GetAllTokenBalances(chainIndex, wlt.Address)
	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 {
		return []walletSwapTokenRow{}, nil
	}

	// 转换结果并收集需要获取头像的代币
	rows := make([]walletSwapTokenRow, 0)
	tokenRequests := make([]exchange.MarketTokenBasicInfoRequest, 0)

	for _, token := range resp.Data[0].TokenAssets {
		// 解析余额和价格
		balance := strings.TrimSpace(token.Balance)
		priceStr := strings.TrimSpace(token.TokenPrice)

		if balance == "" || balance == "0" {
			continue
		}

		// 计算 USD 价值
		balanceFloat, _ := strconv.ParseFloat(balance, 64)
		priceFloat, _ := strconv.ParseFloat(priceStr, 64)
		valueUSD := balanceFloat * priceFloat

		// 过滤低价值代币
		if valueUSD < minValueUSD {
			continue
		}

		tokenAddr := strings.ToLower(strings.TrimSpace(token.TokenContractAddress))
		rows = append(rows, walletSwapTokenRow{
			Address:   tokenAddr,
			Symbol:    strings.TrimSpace(token.Symbol),
			Balance:   balance,
			ValueUSDT: valueUSD,
		})

		// 收集代币地址用于批量获取头像
		tokenRequests = append(tokenRequests, exchange.MarketTokenBasicInfoRequest{
			ChainIndex:           chainIndex,
			TokenContractAddress: tokenAddr,
		})
	}

	// 批量获取代币头像
	if len(tokenRequests) > 0 {
		logoResp, err := okxService.GetMarketTokenBasicInfos(tokenRequests)
		if err == nil && len(logoResp.Data) > 0 {
			// 创建地址到头像的映射
			logoMap := make(map[string]string)
			for _, info := range logoResp.Data {
				addr := strings.ToLower(strings.TrimSpace(info.TokenContractAddress))
				logoMap[addr] = strings.TrimSpace(info.TokenLogoURL)
			}

			// 填充头像 URL
			for i := range rows {
				if logoURL, ok := logoMap[rows[i].Address]; ok {
					rows[i].LogoURL = logoURL
				}
			}
		}
	}

	return rows, nil
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
