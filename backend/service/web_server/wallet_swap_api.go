package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/exchange"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/token_metadata"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"

	"github.com/ethereum/go-ethereum/common"
)

const nativePseudoTokenAddress = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
const walletSwapOKXQuoteTimeout = 6 * time.Second

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

	rows, err := s.getTokenBalancesFromOKX(r.Context(), user.ID, req.WalletID, chain, minVal)
	if err != nil {
		http.Error(w, "OKX balance query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(walletSwapPreviewResponse{
		OK:     true,
		Chain:  chain,
		Tokens: rows,
	})
}

func (s *Server) getTokenBalancesFromOKX(ctx context.Context, userID uint, walletID uint, chain string, minValueUSD float64) ([]walletSwapTokenRow, error) {
	walletService := wallet.NewWalletService()
	wlt, err := walletService.ResolveTaskWallet(userID, walletID, "")
	if err != nil || wlt == nil {
		return nil, fmt.Errorf("wallet not found")
	}

	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok || strings.TrimSpace(cc.Chain) == "" {
		return nil, fmt.Errorf("invalid chain: %s", chain)
	}
	if cc.ChainID <= 0 {
		return nil, fmt.Errorf("invalid chain id")
	}

	walletAddr := common.HexToAddress(wlt.Address)
	okxService := exchange.NewOKXDexService()
	balanceResp, err := okxService.GetAllTokenBalancesByAddress(ctx, exchange.BalanceAllTokenBalancesRequest{
		Address: walletAddr.Hex(),
		Chains:  []string{strconv.FormatInt(cc.ChainID, 10)},
	})
	if err != nil {
		return nil, err
	}
	if balanceResp == nil || len(balanceResp.Data) == 0 {
		return nil, fmt.Errorf("empty OKX balance response")
	}

	tokenAssets := make([]exchange.BalanceTokenAsset, 0)
	for _, data := range balanceResp.Data {
		tokenAssets = append(tokenAssets, data.TokenAssets...)
	}
	tokenAddresses := okxBalanceTokenAddresses(tokenAssets)

	if s.TokenMeta == nil {
		s.TokenMeta = token_metadata.NewService()
	}
	metaByAddr, err := s.TokenMeta.GetBatch(ctx, chain, tokenAddresses)
	if err != nil {
		return nil, fmt.Errorf("token metadata query failed: %w", err)
	}

	rows := make([]walletSwapTokenRow, 0, len(tokenAssets))
	nativeSymbol := nativeSymbolForChainConfig(chain, cc)
	for _, asset := range tokenAssets {
		tokenAddr := normalizeOKXBalanceTokenAssetAddress(asset)
		if tokenAddr == "" {
			continue
		}
		rawBalance, ok := parseOKXRawBalance(asset.RawBalance)
		if !ok || rawBalance.Sign() <= 0 {
			continue
		}

		decimals := okxBalanceTokenDecimals(asset, rawBalance)
		valueUSD := okxBalanceValueUSDT(rawBalance, decimals, asset.TokenPrice, tokenAddr, cc)
		if valueUSD < minValueUSD {
			continue
		}

		displayBalance := strings.TrimSpace(asset.Balance)
		if displayBalance == "" {
			displayBalance = formatWalletSwapRawAmount(rawBalance, decimals)
		}

		meta := metaByAddr[tokenAddr]
		symbol := firstNonEmptyString(asset.Symbol, asset.TokenSymbol, meta.Symbol)
		if symbol == "" && strings.EqualFold(tokenAddr, cc.StableAddress) {
			symbol = stableSymbolForChainConfig(cc)
		}
		isNative := strings.EqualFold(tokenAddr, nativePseudoTokenAddress)
		if symbol == "" && isNative {
			symbol = nativeSymbol
		}
		if symbol == "" {
			symbol = shortTokenSymbol(tokenAddr)
		}
		name := firstNonEmptyString(asset.TokenName, meta.Name)
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
			LogoURL:    firstNonEmptyString(asset.TokenLogoURL, meta.LogoURL),
			CanSwap:    true,
			IsNative:   isNative,
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

func okxBalanceTokenAddresses(assets []exchange.BalanceTokenAsset) []string {
	seen := make(map[string]struct{}, len(assets))
	out := make([]string, 0, len(assets))
	for _, asset := range assets {
		tokenAddr := normalizeOKXBalanceTokenAssetAddress(asset)
		if tokenAddr == "" || strings.EqualFold(tokenAddr, nativePseudoTokenAddress) {
			continue
		}
		if _, ok := seen[tokenAddr]; ok {
			continue
		}
		seen[tokenAddr] = struct{}{}
		out = append(out, tokenAddr)
	}
	sort.Strings(out)
	return out
}

func normalizeOKXBalanceTokenAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, nativePseudoTokenAddress) {
		return nativePseudoTokenAddress
	}
	return token_metadata.NormalizeTokenAddress(raw)
}

func normalizeOKXBalanceTokenAssetAddress(asset exchange.BalanceTokenAsset) string {
	return normalizeOKXBalanceTokenAddress(firstNonEmptyString(asset.TokenContractAddress, asset.TokenAddress))
}

func parseOKXRawBalance(raw string) (*big.Int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	amount, ok := new(big.Int).SetString(raw, 10)
	if !ok || amount == nil {
		return nil, false
	}
	return amount, true
}

func inferOKXBalanceDecimals(rawBalance *big.Int, balanceText string) int {
	if rawBalance == nil || rawBalance.Sign() <= 0 {
		return 18
	}
	balanceText = strings.TrimSpace(balanceText)
	if balanceText == "" || strings.ContainsAny(balanceText, "eE") {
		return 18
	}
	human, ok := new(big.Rat).SetString(balanceText)
	if !ok || human == nil || human.Sign() <= 0 {
		return 18
	}
	rawRat := new(big.Rat).SetInt(rawBalance)
	scale := new(big.Rat).Quo(rawRat, human)
	for decimals := 0; decimals <= 36; decimals++ {
		pow := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
		if scale.Cmp(new(big.Rat).SetInt(pow)) == 0 {
			return decimals
		}
	}
	return 18
}

func okxBalanceTokenDecimals(asset exchange.BalanceTokenAsset, rawBalance *big.Int) int {
	for _, raw := range []string{asset.TokenDecimal, asset.Decimals} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		decimals, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		if decimals >= 0 && decimals <= 36 {
			return decimals
		}
	}
	return inferOKXBalanceDecimals(rawBalance, asset.Balance)
}

func okxBalanceValueUSDT(rawBalance *big.Int, decimals int, tokenPrice string, tokenAddr string, cc config.ChainConfig) float64 {
	if rawBalance == nil || rawBalance.Sign() <= 0 {
		return 0
	}
	if walletSwapIsStableToken(tokenAddr, cc) {
		return amountToFloat(rawBalance.String(), decimals)
	}
	tokenPrice = strings.TrimSpace(tokenPrice)
	if tokenPrice == "" {
		return 0
	}
	price, err := strconv.ParseFloat(tokenPrice, 64)
	if err != nil || price <= 0 {
		return 0
	}
	return amountToFloat(rawBalance.String(), decimals) * price
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func walletSwapTokenValueUSDT(ctx context.Context, okxService *exchange.OKXDexService, cc config.ChainConfig, tokenAddr common.Address, amount *big.Int, decimals int, walletAddr common.Address) (float64, error) {
	if amount == nil || amount.Sign() <= 0 {
		return 0, nil
	}
	if walletSwapIsStableToken(tokenAddr.Hex(), cc) {
		return amountToFloat(amount.String(), decimals), nil
	}
	return walletSwapOKXValueUSDT(ctx, okxService, cc, tokenAddr, amount, walletAddr)
}

func walletSwapIsStableToken(addr string, cc config.ChainConfig) bool {
	normalized := token_metadata.NormalizeTokenAddress(addr)
	if normalized == "" {
		return false
	}
	for _, candidate := range []string{cc.StableAddress, cc.USDTAddress, cc.USDCAddress, cc.BUSDAddress} {
		candidate = token_metadata.NormalizeTokenAddress(candidate)
		if candidate != "" && normalized == candidate {
			return true
		}
	}
	return false
}

func walletSwapOKXValueUSDT(ctx context.Context, okxService *exchange.OKXDexService, cc config.ChainConfig, tokenAddr common.Address, amount *big.Int, walletAddr common.Address) (float64, error) {
	if amount == nil || amount.Sign() <= 0 {
		return 0, nil
	}
	if okxService == nil {
		return 0, fmt.Errorf("OKX service unavailable")
	}
	if cc.ChainID <= 0 {
		return 0, fmt.Errorf("invalid chain id")
	}
	if !common.IsHexAddress(cc.StableAddress) {
		return 0, fmt.Errorf("stable address not configured")
	}
	if ctx == nil {
		return 0, fmt.Errorf("context is nil")
	}

	quoteCtx, cancel := context.WithTimeout(ctx, walletSwapOKXQuoteTimeout)
	defer cancel()

	resp, err := okxService.GetSwapDataWithContext(quoteCtx, exchange.SwapRequest{
		ChainID:           strconv.FormatInt(cc.ChainID, 10),
		FromTokenAddress:  okxWalletSwapTokenAddress(tokenAddr.Hex(), tokenAddr),
		ToTokenAddress:    common.HexToAddress(cc.StableAddress).Hex(),
		Amount:            amount.String(),
		Slippage:          "0.01",
		UserWalletAddress: walletAddr.Hex(),
	})
	if err != nil {
		return 0, err
	}
	if resp == nil || len(resp.Data) == 0 {
		return 0, fmt.Errorf("empty OKX quote response")
	}

	toAmountText := strings.TrimSpace(resp.Data[0].RouterResult.ToTokenAmount)
	if toAmountText == "" {
		return 0, fmt.Errorf("empty OKX quote output")
	}
	toAmount, ok := new(big.Int).SetString(toAmountText, 10)
	if !ok || toAmount == nil || toAmount.Sign() <= 0 {
		return 0, fmt.Errorf("invalid OKX quote output")
	}
	return amountToFloat(toAmount.String(), cc.StableDecimals), nil
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
