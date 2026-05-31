package web_server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	botSvc "TgLpBot/service/bot"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"TgLpBot/service/strategy"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"

	"github.com/ethereum/go-ethereum/common"
)

type openPositionRequest struct {
	InitData          string   `json:"initData"`
	WalletID          uint     `json:"wallet_id,omitempty"`
	Chain             string   `json:"chain"`
	PoolAddress       string   `json:"pool_address"`
	PoolVersion       string   `json:"pool_version"`
	Amount            float64  `json:"amount"`
	RangeInputMode    string   `json:"range_input_mode,omitempty"`
	RangeLowerPct     float64  `json:"range_lower_pct"`
	RangeUpperPct     float64  `json:"range_upper_pct"`
	TickLower         *int     `json:"tick_lower,omitempty"`
	TickUpper         *int     `json:"tick_upper,omitempty"`
	Slippage          *float64 `json:"slippage_tolerance,omitempty"`
	EntrySwapSlippage *float64 `json:"entry_swap_slippage_tolerance,omitempty"`
	AllowEntrySwap    bool     `json:"allow_entry_swap"`
	ConfirmEntrySwap  bool     `json:"confirm_entry_swap,omitempty"`
	AckLiquidityRisk  bool     `json:"ack_liquidity_risk,omitempty"`

	// DCA overrides (single-open overrides over GlobalConfig defaults).
	// When DCAEnabled is nil, global default is used; when set, any non-nil sibling fields override.
	DCAEnabled         *bool     `json:"dca_enabled,omitempty"`
	DCAPercentages     []float64 `json:"dca_percentages,omitempty"`
	DCAIntervalSeconds *float64  `json:"dca_interval_seconds,omitempty"`
	RebalanceEnabled   *bool     `json:"rebalance_enabled,omitempty"`
	TaskMode           string    `json:"task_mode,omitempty"`
}

type openPositionResponse struct {
	TaskID uint   `json:"task_id"`
	TxHash string `json:"tx_hash,omitempty"`
	Status string `json:"status"`
}

type openPositionPreviewResponse struct {
	Status      string                       `json:"status"`
	Checks      []openPositionCheckItem      `json:"checks,omitempty"`
	EntrySwap   *openPositionEntrySwapInfo   `json:"entry_swap,omitempty"`
	PrivateZap  openPositionPrivateZapInfo   `json:"private_zap"`
	RangeEditor *openPositionRangeEditorInfo `json:"range_editor,omitempty"`
	TokenRisk   *TokenRiskInfo               `json:"token_risk,omitempty"`
}

type openPositionEntrySwapInfo struct {
	Required                     bool    `json:"required"`
	NeedsConfirmation            bool    `json:"needs_confirmation,omitempty"`
	FromTokenAddress             string  `json:"from_token_address,omitempty"`
	FromTokenSymbol              string  `json:"from_token_symbol,omitempty"`
	ToTokenAddress               string  `json:"to_token_address,omitempty"`
	ToTokenSymbol                string  `json:"to_token_symbol,omitempty"`
	AmountIn                     string  `json:"amount_in,omitempty"`
	AmountInRaw                  string  `json:"amount_in_raw,omitempty"`
	ExpectedAmountOut            string  `json:"expected_amount_out,omitempty"`
	ExpectedAmountOutRaw         string  `json:"expected_amount_out_raw,omitempty"`
	RecommendedSlippageTolerance float64 `json:"recommended_slippage_tolerance,omitempty"`
	CurrentSlippageTolerance     float64 `json:"current_slippage_tolerance,omitempty"`
}

type openPositionPrivateZapInfo struct {
	ShowProtectionHint bool `json:"show_protection_hint"`
}

type openPositionCheckItem struct {
	Key    string                 `json:"key"`
	Status string                 `json:"status"`
	Label  string                 `json:"label"`
	Detail string                 `json:"detail,omitempty"`
	Value  *float64               `json:"value,omitempty"`
	Extra  map[string]interface{} `json:"extra,omitempty"`
}

type openPositionError struct {
	Code                     string                     `json:"code"`
	Message                  string                     `json:"message"`
	LiquidityUSD             *float64                   `json:"liquidity_usd,omitempty"`
	MinLiquidityUSD          *float64                   `json:"min_liquidity_usd,omitempty"`
	MaxOpenAmount            *float64                   `json:"max_open_amount,omitempty"`
	RiskAckRequired          bool                       `json:"risk_ack_required,omitempty"`
	PriceDeviationPercent    *float64                   `json:"price_deviation_percent,omitempty"`
	PriceDeviationMaxPercent *float64                   `json:"price_deviation_max_percent,omitempty"`
	EntrySwap                *openPositionEntrySwapInfo `json:"entry_swap,omitempty"`
	TokenRisk                *TokenRiskInfo             `json:"token_risk,omitempty"`
}

type openPositionContext struct {
	user             *models.User
	req              openPositionRequest
	chain            string
	cc               config.ChainConfig
	selectedWallet   *models.Wallet
	poolVersion      string
	liquidityService *liquidity.LiquidityService
	task             *models.StrategyTask
	currentTick      int
	resolvedRange    resolvedOpenPositionRange
}

func float64Ptr(v float64) *float64 {
	return &v
}

func buildOpenPositionErrorFromSafety(err *liquidity.ZapSafetyError) openPositionError {
	if err == nil {
		return openPositionError{
			Code:    "zap_safety_check_failed",
			Message: "开仓风控校验失败",
		}
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		switch {
		case err.PriceDeviationPercent > 0:
			message = "池子价格偏离过大，已拦截开仓"
		case err.MaxOpenAmount > 0 || err.MinLiquidityUSD > 0:
			message = "池子流动性不满足开仓要求"
		default:
			message = "开仓风控校验失败"
		}
	}
	resp := openPositionError{
		Code:    "zap_safety_check_failed",
		Message: message,
	}
	if strings.TrimSpace(err.Code) != "" {
		resp.Code = strings.TrimSpace(err.Code)
	}
	if err.LiquidityUSD >= 0 {
		resp.LiquidityUSD = float64Ptr(err.LiquidityUSD)
	}
	if err.MinLiquidityUSD > 0 {
		resp.MinLiquidityUSD = float64Ptr(err.MinLiquidityUSD)
	}
	if err.MaxOpenAmount > 0 {
		resp.MaxOpenAmount = float64Ptr(err.MaxOpenAmount)
	}
	if err.PriceDeviationPercent > 0 {
		resp.PriceDeviationPercent = float64Ptr(err.PriceDeviationPercent)
	}
	if err.PriceDeviationMaxPercent > 0 {
		resp.PriceDeviationMaxPercent = float64Ptr(err.PriceDeviationMaxPercent)
	}
	resp.RiskAckRequired = err.RiskAckRequired
	return resp
}

// resolveDCAPlan merges the per-open DCA overrides onto the user's GlobalConfig defaults.
// Returns (enabled, percentages, intervalSec, error). If both sides disable DCA, enabled is false
// and the other return values are zero — callers should skip DCA wiring in that case.
func resolveDCAPlan(cfg *models.GlobalConfig, req openPositionRequest) (bool, []float64, float64, error) {
	enabled := false
	if cfg != nil {
		enabled = cfg.DCAEnabled
	}
	if req.DCAEnabled != nil {
		enabled = *req.DCAEnabled
	}
	if !enabled {
		return false, nil, 0, nil
	}

	pcts := req.DCAPercentages
	if len(pcts) == 0 && cfg != nil {
		if parsed, ok := strategy.ParseDCAPercentages(cfg.DCAPercentagesJSON); ok {
			pcts = parsed
		}
	}
	normalized, err := strategy.NormalizeDCAPercentages(pcts)
	if err != nil {
		return false, nil, 0, err
	}

	interval := 0.0
	if cfg != nil {
		interval = cfg.DCAIntervalSeconds
	}
	if req.DCAIntervalSeconds != nil {
		interval = *req.DCAIntervalSeconds
	}
	intervalNorm, err := strategy.NormalizeDCAInterval(interval)
	if err != nil {
		return false, nil, 0, err
	}

	minSplitAmount := 0.0
	if cfg != nil {
		minSplitAmount = cfg.DCAMinSplitAmountUSDT
	}
	minSplitAmountNorm, err := strategy.NormalizeDCAMinSplitAmountUSDT(minSplitAmount)
	if err != nil {
		return false, nil, 0, err
	}
	if minSplitAmountNorm > 0 && req.Amount < minSplitAmountNorm {
		return false, nil, 0, nil
	}

	return true, normalized, intervalNorm, nil
}

func resolveOpenPositionTaskMode(req openPositionRequest) (models.StrategyOutOfRangeMode, bool) {
	switch models.NormalizeStrategyTaskMode(req.TaskMode) {
	case models.StrategyTaskModePause:
		return models.StrategyOutOfRangeModeExitAll, true
	case string(models.StrategyOutOfRangeModeRebalanceAll):
		return models.StrategyOutOfRangeModeRebalanceAll, false
	case string(models.StrategyOutOfRangeModeExitAll):
		return models.StrategyOutOfRangeModeExitAll, false
	case string(models.StrategyOutOfRangeModeRebalanceUpExitDown):
		return models.StrategyOutOfRangeModeRebalanceUpExitDown, false
	}

	if req.RebalanceEnabled != nil {
		if *req.RebalanceEnabled {
			return models.StrategyOutOfRangeModeRebalanceAll, false
		}
		return models.StrategyOutOfRangeModeExitAll, false
	}

	return models.StrategyOutOfRangeModeExitAll, true
}

func isV4PoolId(text string) bool {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		text = text[2:]
	}
	if len(text) != 64 {
		return false
	}
	for _, c := range text {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func normalizeHexPrefixed(v string) string {
	raw := strings.TrimSpace(v)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X") {
		return "0x" + raw[2:]
	}
	return "0x" + raw
}

func applyEnterResult(task *models.StrategyTask, enterRes *liquidity.EnterResult) error {
	if task == nil || enterRes == nil {
		return errors.New("task or enter result is nil")
	}

	updates := map[string]interface{}{
		"current_liquidity":      enterRes.CurrentLiquidity,
		"exit_liquidity_removed": false,
		"error_message":          "",
		"status":                 models.StrategyStatusRunning,
	}

	v3TokenId := strings.TrimSpace(enterRes.V3TokenID)
	if v3TokenId != "" && v3TokenId != "0" {
		updates["v3_position_manager_address"] = enterRes.V3PositionManagerAddress
		updates["v3_token_id"] = enterRes.V3TokenID
	}

	v4TokenId := strings.TrimSpace(enterRes.V4TokenID)
	if v4TokenId != "" && v4TokenId != "0" {
		updates["v4_token_id"] = enterRes.V4TokenID
	}

	if err := database.DB.Model(task).Updates(updates).Error; err != nil {
		return err
	}

	task.CurrentLiquidity = enterRes.CurrentLiquidity
	task.ExitLiquidityRemoved = false
	task.Status = models.StrategyStatusRunning
	task.ErrorMessage = ""

	if v3TokenId != "" && v3TokenId != "0" {
		task.V3PositionManagerAddress = enterRes.V3PositionManagerAddress
		task.V3TokenID = enterRes.V3TokenID
	}
	if v4TokenId != "" && v4TokenId != "0" {
		task.V4TokenID = enterRes.V4TokenID
	}

	return nil
}

func writeOpenPositionError(w http.ResponseWriter, status int, resp openPositionError) {
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func decodeOpenPositionRequest(w http.ResponseWriter, r *http.Request) (*openPositionRequest, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "请求方法不支持", http.StatusMethodNotAllowed)
		return nil, false
	}

	r.Body = http.MaxBytesReader(w, r.Body, 12*1024)
	var req openPositionRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "请求参数格式错误", http.StatusBadRequest)
		return nil, false
	}

	req.InitData = strings.TrimSpace(req.InitData)
	req.Chain = strings.TrimSpace(req.Chain)
	req.PoolAddress = strings.TrimSpace(req.PoolAddress)
	req.PoolVersion = strings.ToLower(strings.TrimSpace(req.PoolVersion))
	req.TaskMode = strings.TrimSpace(req.TaskMode)
	req.RangeInputMode = normalizeOpenPositionRangeInputMode(req.RangeInputMode)

	if req.PoolAddress == "" {
		http.Error(w, "缺少池子地址", http.StatusBadRequest)
		return nil, false
	}
	if req.Amount <= 0 {
		http.Error(w, "开仓金额无效", http.StatusBadRequest)
		return nil, false
	}
	if req.RangeInputMode == "" {
		http.Error(w, "区间参数无效", http.StatusBadRequest)
		return nil, false
	}
	switch req.RangeInputMode {
	case openPositionRangeInputPercentage:
		if req.RangeLowerPct <= 0 || req.RangeUpperPct <= 0 || req.RangeLowerPct >= 100 || req.RangeUpperPct >= 100 {
			http.Error(w, "invalid range parameters", http.StatusBadRequest)
			return nil, false
		}
	case openPositionRangeInputTick, openPositionRangeInputGrid:
		if req.TickLower == nil || req.TickUpper == nil {
			http.Error(w, "invalid range parameters", http.StatusBadRequest)
			return nil, false
		}
	}
	if req.Slippage != nil && (*req.Slippage < 0 || *req.Slippage > 100) {
		http.Error(w, "任务滑点参数无效", http.StatusBadRequest)
		return nil, false
	}
	if req.EntrySwapSlippage != nil && (*req.EntrySwapSlippage < 0 || *req.EntrySwapSlippage > 100) {
		http.Error(w, "前置兑换滑点参数无效", http.StatusBadRequest)
		return nil, false
	}
	if config.AppConfig == nil {
		http.Error(w, "系统配置未加载", http.StatusInternalServerError)
		return nil, false
	}

	return &req, true
}

func buildOpenPositionEntrySwapInfo(preview *liquidity.EntrySwapPreview) *openPositionEntrySwapInfo {
	if preview == nil {
		return nil
	}
	return &openPositionEntrySwapInfo{
		Required:                     preview.Required,
		NeedsConfirmation:            preview.Required,
		FromTokenAddress:             strings.TrimSpace(preview.FromTokenAddress),
		FromTokenSymbol:              strings.TrimSpace(preview.FromTokenSymbol),
		ToTokenAddress:               strings.TrimSpace(preview.ToTokenAddress),
		ToTokenSymbol:                strings.TrimSpace(preview.ToTokenSymbol),
		AmountIn:                     strings.TrimSpace(preview.AmountIn),
		AmountInRaw:                  strings.TrimSpace(preview.AmountInRaw),
		ExpectedAmountOut:            strings.TrimSpace(preview.ExpectedAmountOut),
		ExpectedAmountOutRaw:         strings.TrimSpace(preview.ExpectedAmountOutRaw),
		RecommendedSlippageTolerance: preview.RecommendedSlippageTolerance,
		CurrentSlippageTolerance:     preview.CurrentSlippageTolerance,
	}
}

func buildOpenPositionPrivateZapInfo(liquidityService *liquidity.LiquidityService, chain string, walletID uint) openPositionPrivateZapInfo {
	info := openPositionPrivateZapInfo{}
	if liquidityService == nil || walletID == 0 {
		return info
	}
	showHint, err := liquidityService.ShouldShowWalletPrivateZapProtectionHint(chain, walletID)
	if err != nil {
		log.Printf("[OpenPosition] private zap hint check failed: chain=%s wallet_id=%d err=%v", chain, walletID, err)
		return info
	}
	info.ShowProtectionHint = showHint
	return info
}

func (s *Server) prepareOpenPositionContext(req openPositionRequest) (*openPositionContext, *openPositionError, int) {
	user, status, msg := authenticateTelegramWebAppUser(req.InitData)
	if status != 0 {
		return nil, &openPositionError{Code: "unauthorized", Message: msg}, status
	}

	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		if status == 0 {
			status = http.StatusInternalServerError
		}
		return nil, &openPositionError{Code: "access_check_failed", Message: msg}, status
	}
	if status != 0 {
		return nil, &openPositionError{Code: "forbidden", Message: msg}, http.StatusForbidden
	}
	if status, msg := requireModulePermission(check, models.AccessModulePositions); status != 0 {
		return nil, &openPositionError{Code: "miniapp_forbidden", Message: msg}, status
	}

	cfgService := userSvc.NewGlobalConfigService()
	cfg, err := cfgService.GetOrCreate(user.ID)
	if err != nil {
		return nil, &openPositionError{Code: "open_position_failed", Message: "加载全局配置失败"}, http.StatusInternalServerError
	}

	var chain string
	if cfg != nil && !cfg.MultiChainEnabled {
		chain = config.PickEnabledChain(cfg.DefaultChain)
	} else if strings.TrimSpace(req.Chain) != "" {
		chain = config.NormalizeChain(req.Chain)
	} else {
		chain = config.PickEnabledChain("bsc")
	}

	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok || strings.TrimSpace(cc.Chain) == "" {
		return nil, &openPositionError{
			Code:    "invalid_chain",
			Message: "当前链不支持开仓",
		}, http.StatusBadRequest
	}

	if !check.IsAdmin && check.Access != nil {
		taskCount, countErr := userSvc.NewAccessService().CountUserActiveTasks(user.ID)
		if countErr != nil {
			return nil, &openPositionError{
				Code:    "open_position_failed",
				Message: "检查任务额度失败",
			}, http.StatusInternalServerError
		}
		if taskCount >= int64(check.Access.MaxActiveTasks) {
			return nil, &openPositionError{
				Code:    "task_quota_exceeded",
				Message: "已达到活跃任务数量上限",
			}, http.StatusForbidden
		}
	}

	walletService := wallet.NewWalletService()
	wallets, err := walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		return nil, &openPositionError{
			Code:    "wallet_required",
			Message: "未找到可用钱包",
		}, http.StatusBadRequest
	}
	selectedWallet := &wallets[0]
	for i := range wallets {
		if wallets[i].IsDefault {
			selectedWallet = &wallets[i]
			break
		}
	}

	requireSelection := cfg != nil && cfg.MultiWalletEnabled && len(wallets) > 1
	if requireSelection && req.WalletID == 0 {
		return nil, &openPositionError{
			Code:    "wallet_required",
			Message: "请选择钱包",
		}, http.StatusBadRequest
	}
	if requireSelection {
		walletRec, werr := walletService.GetWalletByID(user.ID, req.WalletID)
		if werr != nil || walletRec == nil {
			return nil, &openPositionError{
				Code:    "invalid_wallet",
				Message: "钱包无效",
			}, http.StatusBadRequest
		}
		selectedWallet = walletRec
	}

	poolAddress := normalizeHexPrefixed(req.PoolAddress)
	poolVersion := req.PoolVersion
	if poolVersion == "" {
		if isV4PoolId(poolAddress) {
			poolVersion = "v4"
		} else {
			poolVersion = "v3"
		}
	}

	poolService := pool.NewPoolService()
	var poolInfo *pool.PoolInfo
	switch poolVersion {
	case "v4":
		if chain != "bsc" {
			return nil, &openPositionError{
				Code:    "invalid_chain",
				Message: "当前链暂不支持 V4 池子",
			}, http.StatusBadRequest
		}
		poolInfo, err = poolService.GetPoolInfoForVersionCached(chain, "v4", poolAddress)
	default:
		poolInfo, err = poolService.GetPoolInfoForVersionCached(chain, "v3", poolAddress)
	}
	if err != nil || poolInfo == nil {
		message := "加载池子信息失败"
		if err != nil {
			message = strings.TrimSpace(err.Error())
		}
		return nil, &openPositionError{
			Code:    "pool_info_error",
			Message: message,
		}, http.StatusInternalServerError
	}
	if poolInfo.TickSpacing <= 0 {
		return nil, &openPositionError{
			Code:    "pool_info_error",
			Message: "池子 TickSpacing 无效",
		}, http.StatusInternalServerError
	}

	if !req.AllowEntrySwap {
		stableAddrStr := strings.TrimSpace(cc.StableAddress)
		if common.IsHexAddress(stableAddrStr) {
			stableAddr := strings.ToLower(stableAddrStr)
			token0 := strings.ToLower(strings.TrimSpace(poolInfo.Token0))
			token1 := strings.ToLower(strings.TrimSpace(poolInfo.Token1))
			if token0 != stableAddr && token1 != stableAddr {
				return nil, &openPositionError{
					Code:    "entry_swap_required",
					Message: fmt.Sprintf("当前池子不包含 %s，需要先兑换", strings.TrimSpace(cc.StableSymbol)),
				}, http.StatusConflict
			}
		}
	}

	tmpTask := &models.StrategyTask{
		Chain:         chain,
		PoolId:        poolAddress,
		PoolVersion:   poolVersion,
		Token0Symbol:  poolInfo.Token0Symbol,
		Token1Symbol:  poolInfo.Token1Symbol,
		Token0Address: poolInfo.Token0,
		Token1Address: poolInfo.Token1,
		AmountUSDT:    req.Amount,
	}

	tickLowerPctReq, tickUpperPctReq := 1.0, 1.0
	if req.RangeInputMode == openPositionRangeInputPercentage {
		tickLowerPctReq, tickUpperPctReq = pricing.TickPercentagesFromStablePercentages(tmpTask, req.RangeLowerPct, req.RangeUpperPct)
		if tickLowerPctReq <= 0 || tickUpperPctReq <= 0 {
			return nil, &openPositionError{
				Code:    "invalid_range",
				Message: "区间参数无效",
			}, http.StatusBadRequest
		}
	}

	currentTick, err := loadOpenPositionCurrentTick(chain, poolVersion, poolAddress)
	if err != nil {
		return nil, &openPositionError{
			Code:    "open_position_failed",
			Message: "读取当前 Tick 失败",
		}, http.StatusInternalServerError
	}

	resolvedRange, errResp, status := resolveOpenPositionRange(tmpTask, req, currentTick, poolInfo.TickSpacing)
	if errResp != nil {
		return nil, errResp, status
	}

	taskSlippage := cfg.SlippageTolerance
	if req.Slippage != nil {
		taskSlippage = *req.Slippage
	}

	liquidityService := liquidity.NewLiquidityService()
	hooksAddr := normalizeHexPrefixed(poolInfo.HooksAddress)
	if !common.IsHexAddress(hooksAddr) {
		hooksAddr = "0x0000000000000000000000000000000000000000"
	}
	outOfRangeMode, paused := resolveOpenPositionTaskMode(req)
	rangeActivationPending := currentTick < resolvedRange.TickLower || currentTick > resolvedRange.TickUpper
	var pausedAt *time.Time
	if paused {
		now := time.Now()
		pausedAt = &now
	}

	task := &models.StrategyTask{
		UserID:                 user.ID,
		Chain:                  chain,
		PoolId:                 poolAddress,
		PoolVersion:            poolVersion,
		Exchange:               poolInfo.Exchange,
		WalletID:               selectedWallet.ID,
		WalletAddress:          selectedWallet.Address,
		Token0Symbol:           poolInfo.Token0Symbol,
		Token1Symbol:           poolInfo.Token1Symbol,
		Token0Address:          poolInfo.Token0,
		Token1Address:          poolInfo.Token1,
		HooksAddress:           hooksAddr,
		Fee:                    poolInfo.Fee,
		TickSpacing:            poolInfo.TickSpacing,
		TickLower:              resolvedRange.TickLower,
		TickUpper:              resolvedRange.TickUpper,
		RangePercentage:        resolvedRange.RangePct,
		RangeLowerPercentage:   resolvedRange.TickLowerPct,
		RangeUpperPercentage:   resolvedRange.TickUpperPct,
		AmountUSDT:             req.Amount,
		CurrentLiquidity:       "0",
		ReopenDelaySeconds:     strategy.NormalizeRebalanceTimeout(cfg.RebalanceTimeout),
		SlippageTolerance:      taskSlippage,
		AutoReinvest:           cfg.AutoReinvest,
		AllowEntrySwap:         req.AllowEntrySwap,
		RebalanceEnabled:       models.RebalanceEnabledForOutOfRangeMode(outOfRangeMode),
		OutOfRangeMode:         string(outOfRangeMode),
		Paused:                 paused,
		PausedAt:               pausedAt,
		RangeActivationPending: rangeActivationPending,
		Status:                 models.StrategyStatusRunning,
		LastCheckTime:          time.Now(),
	}
	positionShape, _ := classifyOpenPositionShape(task, currentTick, resolvedRange.TickLower, resolvedRange.TickUpper)
	singleSidedSelection := strings.HasPrefix(positionShape, "single_")

	if dcaEnabled, dcaPcts, dcaInterval, err := resolveDCAPlan(cfg, req); err != nil {
		return nil, &openPositionError{
			Code:    "invalid_dca_plan",
			Message: err.Error(),
		}, http.StatusBadRequest
	} else if dcaEnabled && !singleSidedSelection {
		pctsJSON, _ := strategy.MarshalDCAPercentages(dcaPcts)
		task.DCAEnabled = true
		task.DCATotalAmountUSDT = req.Amount
		task.DCAPercentagesJSON = pctsJSON
		task.DCAIntervalSeconds = dcaInterval
		task.AmountUSDT = req.Amount * dcaPcts[0] / 100.0
	}

	return &openPositionContext{
		user:             user,
		req:              req,
		chain:            chain,
		cc:               cc,
		selectedWallet:   selectedWallet,
		poolVersion:      poolVersion,
		liquidityService: liquidityService,
		task:             task,
		currentTick:      currentTick,
		resolvedRange:    resolvedRange,
	}, nil, 0
}

func (s *Server) handleOpenPositionPreview(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeOpenPositionRequest(w, r)
	if !ok {
		return
	}

	ctx, errResp, status := s.prepareOpenPositionContext(*req)
	if errResp != nil {
		writeOpenPositionError(w, status, *errResp)
		return
	}

	// Collect safety checks
	checkResults, err := ctx.liquidityService.CollectOpenPositionChecks(ctx.task, liquidity.OpenPositionRiskOptions{
		AckLiquidityRisk:    ctx.req.AckLiquidityRisk,
		RequireLiquidityAck: false,
	})
	if err != nil {
		writeOpenPositionError(w, http.StatusInternalServerError, openPositionError{
			Code:    "open_position_failed",
			Message: err.Error(),
		})
		return
	}

	var checks []openPositionCheckItem
	for _, cr := range checkResults {
		checks = append(checks, openPositionCheckItem{
			Key:    cr.Key,
			Status: cr.Status,
			Label:  cr.Label,
			Detail: cr.Detail,
			Value:  cr.Value,
			Extra:  cr.Extra,
		})
	}
	tokenRisk := resolveOpenPositionTokenRisk(r.Context(), s, ctx.task)
	logTokenRiskWarning("preview", tokenRisk)
	checks = appendTokenRiskCheck(checks, tokenRisk)

	rangeEditor := buildOpenPositionRangeEditorInfo(ctx.task, ctx.currentTick, ctx.task.TickSpacing, &ctx.resolvedRange)

	// Check for hard failures
	hasFail := false
	for _, c := range checks {
		if c.Status == "fail" {
			hasFail = true
			break
		}
	}

	if hasFail {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openPositionPreviewResponse{
			Status:      "fail",
			Checks:      checks,
			PrivateZap:  buildOpenPositionPrivateZapInfo(ctx.liquidityService, ctx.chain, ctx.selectedWallet.ID),
			RangeEditor: rangeEditor,
			TokenRisk:   tokenRisk,
		})
		return
	}

	// Get entry swap preview
	preview, err := ctx.liquidityService.PreviewEntrySwap(ctx.task, ctx.selectedWallet, ctx.task.SlippageTolerance, ctx.req.EntrySwapSlippage)
	if err != nil {
		writeOpenPositionError(w, http.StatusInternalServerError, openPositionError{
			Code:    "open_position_failed",
			Message: err.Error(),
		})
		return
	}

	entrySwapInfo := buildOpenPositionEntrySwapInfo(preview)
	if entrySwapInfo != nil && entrySwapInfo.Required {
		checks = append(checks, openPositionCheckItem{
			Key:    "entry_swap",
			Status: "warn",
			Label:  "前置兑换",
			Detail: fmt.Sprintf("%s %s → %s", entrySwapInfo.AmountIn, entrySwapInfo.FromTokenSymbol, entrySwapInfo.ToTokenSymbol),
			Extra: map[string]interface{}{
				"needs_confirmation": true,
			},
		})
	} else {
		checks = append(checks, openPositionCheckItem{
			Key:    "entry_swap",
			Status: "pass",
			Label:  "前置兑换",
			Detail: "无需兑换",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openPositionPreviewResponse{
		Status:      "ok",
		Checks:      checks,
		EntrySwap:   entrySwapInfo,
		PrivateZap:  buildOpenPositionPrivateZapInfo(ctx.liquidityService, ctx.chain, ctx.selectedWallet.ID),
		RangeEditor: rangeEditor,
		TokenRisk:   tokenRisk,
	})
}

func (s *Server) handleOpenPosition(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeOpenPositionRequest(w, r)
	if !ok {
		return
	}

	ctx, errResp, status := s.prepareOpenPositionContext(*req)
	if errResp != nil {
		writeOpenPositionError(w, status, *errResp)
		return
	}

	if err := ctx.liquidityService.CheckOpenPositionSafety(ctx.task, liquidity.OpenPositionRiskOptions{
		AckLiquidityRisk:    ctx.req.AckLiquidityRisk,
		RequireLiquidityAck: false,
	}); err != nil {
		var zapSafetyErr *liquidity.ZapSafetyError
		if errors.As(err, &zapSafetyErr) {
			resp := buildOpenPositionErrorFromSafety(zapSafetyErr)
			writeOpenPositionError(w, http.StatusConflict, resp)
			return
		}
		writeOpenPositionError(w, http.StatusInternalServerError, openPositionError{
			Code:    "open_position_failed",
			Message: err.Error(),
		})
		return
	}

	tokenRisk := resolveOpenPositionTokenRisk(r.Context(), s, ctx.task)
	logTokenRiskWarning("execute", tokenRisk)
	if tokenRiskBlocksOpen(tokenRisk) {
		writeOpenPositionError(w, http.StatusConflict, openPositionError{
			Code:      "token_honeypot",
			Message:   tokenRiskBlockMessage(tokenRisk),
			TokenRisk: tokenRisk,
		})
		return
	}

	entrySwapPreview, err := ctx.liquidityService.PreviewEntrySwap(ctx.task, ctx.selectedWallet, ctx.task.SlippageTolerance, ctx.req.EntrySwapSlippage)
	if err != nil {
		writeOpenPositionError(w, http.StatusInternalServerError, openPositionError{
			Code:    "open_position_failed",
			Message: err.Error(),
		})
		return
	}

	if entrySwapPreview != nil && entrySwapPreview.Required {
		if !ctx.req.AllowEntrySwap {
			writeOpenPositionError(w, http.StatusConflict, openPositionError{
				Code:      "entry_swap_required",
				Message:   fmt.Sprintf("当前池子不包含 %s，需要先兑换", strings.TrimSpace(ctx.cc.StableSymbol)),
				EntrySwap: buildOpenPositionEntrySwapInfo(entrySwapPreview),
			})
			return
		}
		if !ctx.req.ConfirmEntrySwap {
			writeOpenPositionError(w, http.StatusConflict, openPositionError{
				Code:      "entry_swap_confirmation_required",
				Message:   "请先确认前置兑换",
				EntrySwap: buildOpenPositionEntrySwapInfo(entrySwapPreview),
			})
			return
		}
	}

	if err := ctx.liquidityService.EnsureWalletPrivateZapReady(ctx.user.ID, ctx.chain, ctx.selectedWallet.ID, ctx.selectedWallet.Address, ctx.poolVersion); err != nil {
		var zapSafetyErr *liquidity.ZapSafetyError
		if errors.As(err, &zapSafetyErr) {
			resp := buildOpenPositionErrorFromSafety(zapSafetyErr)
			writeOpenPositionError(w, http.StatusConflict, resp)
			return
		}
		writeOpenPositionError(w, http.StatusInternalServerError, openPositionError{
			Code:    "open_position_failed",
			Message: err.Error(),
		})
		return
	}

	if err := strategy.CreateTaskRecord(ctx.task); err != nil {
		http.Error(w, "创建任务失败", http.StatusInternalServerError)
		return
	}
	enterRes, err := ctx.liquidityService.EnterTaskFromUSDTWithOptions(ctx.user.ID, ctx.task, liquidity.TxOptions{
		EntrySwapSlippageOverride: ctx.req.EntrySwapSlippage,
	})
	if err != nil {
		var swapErr *liquidity.EntrySwapRequiredError
		if errors.As(err, &swapErr) {
			_ = database.DB.Model(ctx.task).Updates(map[string]interface{}{
				"status":        models.StrategyStatusWaiting,
				"error_message": "entry swap required",
			}).Error
			writeOpenPositionError(w, http.StatusConflict, openPositionError{
				Code:    "entry_swap_required",
				Message: swapErr.Error(),
			})
			return
		}
		_ = database.DB.Model(ctx.task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": err.Error(),
		}).Error

		var zapSafetyErr *liquidity.ZapSafetyError
		if errors.As(err, &zapSafetyErr) {
			resp := buildOpenPositionErrorFromSafety(zapSafetyErr)
			writeOpenPositionError(w, http.StatusConflict, resp)
			return
		}
		writeOpenPositionError(w, http.StatusInternalServerError, openPositionError{
			Code:    "open_position_failed",
			Message: err.Error(),
		})
		return
	}

	if err := applyEnterResult(ctx.task, enterRes); err != nil {
		http.Error(w, "更新任务状态失败", http.StatusInternalServerError)
		return
	}

	if ctx.task.DCAEnabled {
		now := time.Now()
		next := now.Add(time.Duration(ctx.task.DCAIntervalSeconds * float64(time.Second)))
		_ = database.DB.Model(ctx.task).Updates(map[string]interface{}{
			"dca_executed_count": 1,
			"dca_next_batch_at":  &next,
		}).Error
		ctx.task.DCAExecutedCount = 1
		ctx.task.DCANextBatchAt = &next
	}

	go func() {
		_ = botSvc.SendTaskCardForUser(ctx.user.ID, ctx.task.ID)
	}()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openPositionResponse{
		TaskID: ctx.task.ID,
		TxHash: enterRes.TxHash,
		Status: "ok",
	})
}
