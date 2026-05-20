package web_server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/exchange"
	"TgLpBot/service/liquidity"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"
)

const nativePseudoTokenAddress = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

type walletSwapPreviewRequest struct {
	InitData    string  `json:"initData"`
	WalletID    uint    `json:"wallet_id,omitempty"`
	Chain       string  `json:"chain,omitempty"`
	MinValueUSD float64 `json:"min_value_usd,omitempty"`
}

type walletSwapTokenRow struct {
	Address        string  `json:"address"`
	Symbol         string  `json:"symbol"`
	Name           string  `json:"name,omitempty"`
	Balance        string  `json:"balance"`
	ValueUSDT      float64 `json:"value_usdt"`
	LogoURL        string  `json:"logo_url,omitempty"`
	CanSwap        bool    `json:"can_swap"`
	IsNative       bool    `json:"is_native,omitempty"`
	DisabledReason string  `json:"disabled_reason,omitempty"`
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

	minVal := req.MinValueUSD
	if minVal <= 0 {
		minVal = 0.1
	}

	rows, err := s.getTokenBalancesFromOKX(user.ID, req.WalletID, chain, minVal)
	if err != nil {
		http.Error(w, "okx balance query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(walletSwapPreviewResponse{
		OK:     true,
		Chain:  chain,
		Tokens: rows,
	})
}

func (s *Server) getTokenBalancesFromOKX(userID uint, walletID uint, chain string, minValueUSD float64) ([]walletSwapTokenRow, error) {
	walletService := wallet.NewWalletService()
	wlt, err := walletService.ResolveTaskWallet(userID, walletID, "")
	if err != nil || wlt == nil {
		return nil, fmt.Errorf("wallet not found")
	}

	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok || strings.TrimSpace(cc.Chain) == "" {
		return nil, fmt.Errorf("invalid chain: %s", chain)
	}

	chainIndex := config.ChainToOKXChainIndex(chain)
	if chainIndex == "" {
		return nil, fmt.Errorf("unsupported chain: %s", chain)
	}

	okxService := exchange.NewOKXDexService()
	resp, err := okxService.GetAllTokenBalances(chainIndex, wlt.Address)
	if err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return []walletSwapTokenRow{}, nil
	}

	rows := make([]walletSwapTokenRow, 0, len(resp.Data[0].TokenAssets))
	tokenRequests := make([]exchange.MarketTokenBasicInfoRequest, 0, len(resp.Data[0].TokenAssets))
	nativeSymbol := nativeSymbolForChainConfig(chain, cc)
	wrappedNativeSymbol := strings.ToUpper(strings.TrimSpace(cc.WrappedNativeSymbol))

	for _, token := range resp.Data[0].TokenAssets {
		balance := strings.TrimSpace(token.Balance)
		priceStr := strings.TrimSpace(token.TokenPrice)
		if balance == "" || balance == "0" {
			continue
		}

		balanceFloat, _ := strconv.ParseFloat(balance, 64)
		priceFloat, _ := strconv.ParseFloat(priceStr, 64)
		valueUSD := balanceFloat * priceFloat
		if valueUSD < minValueUSD {
			continue
		}

		symbol := strings.TrimSpace(token.Symbol)
		name := symbol
		tokenAddr := strings.ToLower(strings.TrimSpace(token.TokenContractAddress))
		isNative := tokenAddr == ""
		canSwap := true
		disabledReason := ""

		if isNative {
			tokenAddr = nativePseudoTokenAddress
			canSwap = true
			if symbol == "" {
				symbol = nativeSymbol
			}
			if name == "" {
				name = fmt.Sprintf("%s (原生)", nativeSymbol)
			}
			if wrappedNativeSymbol != "" {
				disabledReason = fmt.Sprintf("原生 %s 暂不支持直接兑换，请先换成 %s。", nativeSymbol, wrappedNativeSymbol)
			} else {
				disabledReason = fmt.Sprintf("原生 %s 暂不支持直接兑换。", nativeSymbol)
			}
		} else {
			if symbol == "" && strings.EqualFold(tokenAddr, cc.StableAddress) {
				symbol = stableSymbolForChainConfig(cc)
			}
			if name == "" {
				name = symbol
			}
			if tokenAddr == "" {
				continue
			}
			tokenRequests = append(tokenRequests, exchange.MarketTokenBasicInfoRequest{
				ChainIndex:           chainIndex,
				TokenContractAddress: tokenAddr,
			})
		}

		rows = append(rows, walletSwapTokenRow{
			Address:        tokenAddr,
			Symbol:         symbol,
			Name:           name,
			Balance:        balance,
			ValueUSDT:      valueUSD,
			CanSwap:        canSwap,
			IsNative:       isNative,
			DisabledReason: disabledReason,
		})
	}

	if len(tokenRequests) > 0 {
		logoResp, err := okxService.GetMarketTokenBasicInfos(tokenRequests)
		if err == nil && len(logoResp.Data) > 0 {
			logoMap := make(map[string]string, len(logoResp.Data))
			for _, info := range logoResp.Data {
				addr := strings.ToLower(strings.TrimSpace(info.TokenContractAddress))
				if addr == "" {
					continue
				}
				logoMap[addr] = strings.TrimSpace(info.TokenLogoURL)
			}
			for i := range rows {
				if logoURL, ok := logoMap[rows[i].Address]; ok {
					rows[i].LogoURL = logoURL
				}
			}
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ValueUSDT == rows[j].ValueUSDT {
			return rows[i].Symbol < rows[j].Symbol
		}
		return rows[i].ValueUSDT > rows[j].ValueUSDT
	})

	return rows, nil
}

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
