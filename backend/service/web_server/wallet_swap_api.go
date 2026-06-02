package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/pricing"
	"TgLpBot/service/token_metadata"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"

	"github.com/ethereum/go-ethereum/common"
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
	BalanceRaw     string  `json:"balance_raw,omitempty"`
	Decimals       int     `json:"decimals,omitempty"`
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

	rows, err := s.getTokenBalancesFromRPC(r.Context(), user.ID, req.WalletID, chain, minVal)
	if err != nil {
		http.Error(w, "rpc balance query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(walletSwapPreviewResponse{
		OK:     true,
		Chain:  chain,
		Tokens: rows,
	})
}

func (s *Server) getTokenBalancesFromRPC(ctx context.Context, userID uint, walletID uint, chain string, minValueUSD float64) ([]walletSwapTokenRow, error) {
	walletService := wallet.NewWalletService()
	wlt, err := walletService.ResolveTaskWallet(userID, walletID, "")
	if err != nil || wlt == nil {
		return nil, fmt.Errorf("wallet not found")
	}

	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok || strings.TrimSpace(cc.Chain) == "" {
		return nil, fmt.Errorf("invalid chain: %s", chain)
	}
	exec, err := chainexec.GetEVM(chain)
	if err != nil {
		return nil, fmt.Errorf("chain init failed: %w", err)
	}
	client := exec.Client()
	if client == nil {
		return nil, fmt.Errorf("rpc client unavailable")
	}
	walletAddr := common.HexToAddress(wlt.Address)

	tokenAddresses, err := s.walletSwapKnownTokenAddresses(ctx, userID, chain, cc)
	if err != nil {
		return nil, err
	}
	priceAddresses := append([]string(nil), tokenAddresses...)
	if wrapped := token_metadata.NormalizeTokenAddress(cc.WrappedNativeAddress); wrapped != "" {
		priceAddresses = append(priceAddresses, wrapped)
	}
	prices, err := pricing.DefaultTokenPriceService().GetUSDPrices(chain, priceAddresses)
	if err != nil {
		return nil, fmt.Errorf("price query failed: %w", err)
	}

	if s.TokenMeta == nil {
		s.TokenMeta = token_metadata.NewService()
	}
	metaByAddr, err := s.TokenMeta.GetBatch(ctx, chain, tokenAddresses)
	if err != nil {
		return nil, fmt.Errorf("token metadata query failed: %w", err)
	}

	rows := make([]walletSwapTokenRow, 0, len(tokenAddresses)+1)
	nativeSymbol := nativeSymbolForChainConfig(chain, cc)
	wrappedNativeSymbol := strings.ToUpper(strings.TrimSpace(cc.WrappedNativeSymbol))

	nativeBalance, err := walletSwapAssetBalance(client, common.HexToAddress(nativePseudoTokenAddress), walletAddr)
	if err != nil {
		return nil, fmt.Errorf("native balance query failed: %w", err)
	}
	if nativeBalance != nil && nativeBalance.Sign() > 0 {
		displayBalance := formatWalletSwapRawAmount(nativeBalance, 18)
		balanceFloat, _ := strconv.ParseFloat(displayBalance, 64)
		valueUSD := balanceFloat * pricing.GetNativePriceUSD(chain)
		if valueUSD >= minValueUSD {
			disabledReason := fmt.Sprintf("原生 %s 暂不支持直接兑换", nativeSymbol)
			if wrappedNativeSymbol != "" {
				disabledReason = fmt.Sprintf("原生 %s 暂不支持直接兑换，请先换成 %s", nativeSymbol, wrappedNativeSymbol)
			}
			rows = append(rows, walletSwapTokenRow{
				Address:        nativePseudoTokenAddress,
				Symbol:         nativeSymbol,
				Name:           fmt.Sprintf("%s (原生)", nativeSymbol),
				Balance:        displayBalance,
				BalanceRaw:     nativeBalance.String(),
				Decimals:       18,
				ValueUSDT:      valueUSD,
				CanSwap:        true,
				IsNative:       true,
				DisabledReason: disabledReason,
			})
		}
	}

	for _, tokenAddr := range tokenAddresses {
		tokenAddress := common.HexToAddress(tokenAddr)
		rawBalance, err := blockchain.GetTokenBalanceWithClient(client, tokenAddress, walletAddr)
		if err != nil || rawBalance == nil || rawBalance.Sign() <= 0 {
			continue
		}
		decimals := int(tokenDecimals(client, tokenAddress))
		displayBalance := formatWalletSwapRawAmount(rawBalance, decimals)
		balanceFloat, _ := strconv.ParseFloat(displayBalance, 64)
		valueUSD := balanceFloat * prices[tokenAddr]
		if valueUSD < minValueUSD {
			continue
		}
		meta := metaByAddr[tokenAddr]
		symbol := strings.TrimSpace(meta.Symbol)
		if symbol == "" && strings.EqualFold(tokenAddr, cc.StableAddress) {
			symbol = stableSymbolForChainConfig(cc)
		}
		if symbol == "" {
			symbol = shortTokenSymbol(tokenAddr)
		}
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = symbol
		}
		rows = append(rows, walletSwapTokenRow{
			Address:    tokenAddr,
			Symbol:     symbol,
			Name:       name,
			Balance:    displayBalance,
			BalanceRaw: rawBalance.String(),
			Decimals:   decimals,
			ValueUSDT:  valueUSD,
			LogoURL:    strings.TrimSpace(meta.LogoURL),
			CanSwap:    true,
		})
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ValueUSDT == rows[j].ValueUSDT {
			return rows[i].Symbol < rows[j].Symbol
		}
		return rows[i].ValueUSDT > rows[j].ValueUSDT
	})

	return rows, nil
}

func (s *Server) walletSwapKnownTokenAddresses(ctx context.Context, userID uint, chain string, cc config.ChainConfig) ([]string, error) {
	seen := make(map[string]struct{})
	add := func(raw string) {
		addr := token_metadata.NormalizeTokenAddress(raw)
		if addr == "" || strings.EqualFold(addr, nativePseudoTokenAddress) {
			return
		}
		seen[addr] = struct{}{}
	}

	add(cc.StableAddress)
	add(cc.USDCAddress)
	add(cc.BUSDAddress)
	add(cc.WrappedNativeAddress)

	if database.DB != nil {
		var taskRows []struct {
			Token0Address string
			Token1Address string
		}
		if err := database.DB.WithContext(ctx).Model(&models.StrategyTask{}).
			Select("token0_address, token1_address").
			Where("user_id = ? AND chain = ?", userID, chain).
			Find(&taskRows).Error; err != nil {
			return nil, fmt.Errorf("load strategy token candidates failed: %w", err)
		}
		for _, row := range taskRows {
			add(row.Token0Address)
			add(row.Token1Address)
		}

		var txRows []struct {
			TokenInAddress  string
			TokenOutAddress string
		}
		if err := database.DB.WithContext(ctx).Model(&models.Transaction{}).
			Select("token_in_address, token_out_address").
			Where("user_id = ? AND chain = ?", userID, chain).
			Order("id DESC").
			Limit(200).
			Find(&txRows).Error; err != nil {
			return nil, fmt.Errorf("load swap history token candidates failed: %w", err)
		}
		for _, row := range txRows {
			add(row.TokenInAddress)
			add(row.TokenOutAddress)
		}

		var orderRows []struct {
			FromTokenAddress string
			ToTokenAddress   string
		}
		if err := database.DB.WithContext(ctx).Model(&models.WalletSwapLimitOrder{}).
			Select("from_token_address, to_token_address").
			Where("user_id = ? AND chain = ?", userID, chain).
			Order("id DESC").
			Limit(200).
			Find(&orderRows).Error; err != nil {
			return nil, fmt.Errorf("load limit order token candidates failed: %w", err)
		}
		for _, row := range orderRows {
			add(row.FromTokenAddress)
			add(row.ToTokenAddress)
		}

		var poolRows []struct {
			BaseTokenID        string
			QuoteTokenID       string
			PricedTokenAddress string
			StableCoinPosition string
		}
		if err := database.DB.WithContext(ctx).Model(&models.Pool{}).
			Select("base_token_id, quote_token_id, priced_token_address, stable_coin_position").
			Where("(chain = ? OR source_requested_chain = ?)", chain, chain).
			Order("current_pool_value DESC, total_volume DESC, volume_h24 DESC").
			Limit(150).
			Find(&poolRows).Error; err != nil {
			return nil, fmt.Errorf("load pool token candidates failed: %w", err)
		}
		for _, row := range poolRows {
			add(row.PricedTokenAddress)
			add(geckoTokenIDAddress(row.BaseTokenID))
			add(geckoTokenIDAddress(row.QuoteTokenID))
		}
	}

	out := make([]string, 0, len(seen))
	for addr := range seen {
		out = append(out, addr)
	}
	sort.Strings(out)
	const maxRPCBalanceTokens = 300
	if len(out) > maxRPCBalanceTokens {
		out = out[:maxRPCBalanceTokens]
	}
	return out, nil
}

func shortTokenSymbol(addr string) string {
	addr = token_metadata.NormalizeTokenAddress(addr)
	if len(addr) < 10 {
		return "TOKEN"
	}
	return strings.ToUpper(addr[:6] + "..." + addr[len(addr)-4:])
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
