package web_server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/blacklist"
	botSvc "TgLpBot/service/bot"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"
	"TgLpBot/service/ws"

	"github.com/ethereum/go-ethereum/common"
)

type openPositionRequest struct {
	InitData       string   `json:"initData"`
	WalletID       uint     `json:"wallet_id,omitempty"`
	Chain          string   `json:"chain"`
	PoolAddress    string   `json:"pool_address"`
	PoolVersion    string   `json:"pool_version"`
	Amount         float64  `json:"amount"`
	RangeLowerPct  float64  `json:"range_lower_pct"`
	RangeUpperPct  float64  `json:"range_upper_pct"`
	Slippage       *float64 `json:"slippage_tolerance,omitempty"`
	AllowEntrySwap bool     `json:"allow_entry_swap"`
}

type openPositionResponse struct {
	TaskID uint   `json:"task_id"`
	TxHash string `json:"tx_hash,omitempty"`
	Status string `json:"status"`
}

type openPositionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
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

func (s *Server) handleOpenPosition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 12*1024)
	var req openPositionRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	req.InitData = strings.TrimSpace(req.InitData)
	req.Chain = strings.TrimSpace(req.Chain)
	req.PoolAddress = strings.TrimSpace(req.PoolAddress)
	req.PoolVersion = strings.ToLower(strings.TrimSpace(req.PoolVersion))

	if req.PoolAddress == "" {
		http.Error(w, "missing pool_address", http.StatusBadRequest)
		return
	}
	if req.Amount <= 0 {
		http.Error(w, "invalid amount", http.StatusBadRequest)
		return
	}
	if req.RangeLowerPct <= 0 || req.RangeUpperPct <= 0 || req.RangeLowerPct >= 100 || req.RangeUpperPct >= 100 {
		http.Error(w, "invalid range", http.StatusBadRequest)
		return
	}
	if req.Slippage != nil {
		if *req.Slippage < 0 || *req.Slippage > 100 {
			http.Error(w, "invalid slippage_tolerance", http.StatusBadRequest)
			return
		}
	}
	if config.AppConfig == nil {
		http.Error(w, "config not loaded", http.StatusInternalServerError)
		return
	}

	var (
		chain string
		cc    config.ChainConfig
	)

	user, status, msg := authenticateTelegramWebAppUser(req.InitData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	// Step 0: validating permissions & config
	ws.SendProgress(user.ID, "open_position", 0, 0, 5, "active", "")

	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	if status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(openPositionError{
			Code:    "forbidden",
			Message: msg,
		})
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(openPositionError{
			Code:    "miniapp_forbidden",
			Message: msg,
		})
		return
	}

	// 检查任务额度
	cfgService := userSvc.NewGlobalConfigService()
	cfg, cfgErr := cfgService.GetOrCreate(user.ID)
	if cfgErr != nil {
		http.Error(w, "failed to load config", http.StatusInternalServerError)
		return
	}

	// Resolve effective chain based on user's chain mode.
	requestedChain := strings.TrimSpace(req.Chain)
	if cfg != nil && !cfg.MultiChainEnabled {
		chain = config.PickEnabledChain(cfg.DefaultChain)
	} else if requestedChain != "" {
		chain = config.NormalizeChain(requestedChain)
	} else {
		// Backwards-compatible default for older clients.
		chain = config.PickEnabledChain("bsc")
	}

	var ok bool
	cc, ok = config.AppConfig.GetChainConfig(chain)
	if !ok || strings.TrimSpace(cc.Chain) == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openPositionError{
			Code:    "invalid_chain",
			Message: "unsupported chain (enable it via CHAINS env)",
		})
		return
	}

	if !check.IsAdmin && check.Access != nil {
		taskCount, countErr := userSvc.NewAccessService().CountUserActiveTasks(user.ID)
		if countErr != nil {
			http.Error(w, "failed to check task quota", http.StatusInternalServerError)
			return
		}
		if taskCount >= int64(check.Access.MaxActiveTasks) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(openPositionError{
				Code:    "task_quota_exceeded",
				Message: "已达到活跃任务数量上限，请先停止其他任务或联系管理员提升额度",
			})
			return
		}
	}

	walletService := wallet.NewWalletService()
	wallets, err := walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		http.Error(w, "no wallet found", http.StatusBadRequest)
		return
	}
	defaultWallet := &wallets[0]
	for i := range wallets {
		if wallets[i].IsDefault {
			defaultWallet = &wallets[i]
			break
		}
	}

	requireSelection := cfg != nil && cfg.MultiWalletEnabled && len(wallets) > 1
	if requireSelection && req.WalletID == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(openPositionError{
			Code:    "wallet_required",
			Message: "请选择钱包",
		})
		return
	}

	selectedWallet := defaultWallet
	if requireSelection {
		walletRec, werr := walletService.GetWalletByID(user.ID, req.WalletID)
		if werr != nil || walletRec == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(openPositionError{
				Code:    "invalid_wallet",
				Message: "无效的钱包",
			})
			return
		}
		selectedWallet = walletRec
	}

	blacklistSvc := blacklist.NewBlacklistService()
	if blacklistSvc.IsBlacklisted(user.ID, req.PoolAddress) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(openPositionError{
			Code:    "blacklisted",
			Message: "该池子已加入黑名单，禁止开仓",
		})
		return
	}

	poolAddress := normalizeHexPrefixed(req.PoolAddress)
	poolVersion := req.PoolVersion

	// Step 1: querying pool info
	ws.SendProgress(user.ID, "open_position", 0, 1, 5, "active", "")
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
			http.Error(w, "V4 not supported on this chain yet", http.StatusBadRequest)
			return
		}
		poolInfo, err = poolService.GetV4PoolInfo(poolAddress)
	default:
		poolInfo, err = poolService.GetPoolInfoForChain(chain, poolAddress)
	}
	if err != nil || poolInfo == nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		message := "failed to load pool info"
		if err != nil {
			message = strings.TrimSpace(err.Error())
		}
		_ = json.NewEncoder(w).Encode(openPositionError{
			Code:    "pool_info_error",
			Message: message,
		})
		return
	}
	if poolInfo.TickSpacing <= 0 {
		http.Error(w, "invalid tick spacing", http.StatusInternalServerError)
		return
	}
	if !req.AllowEntrySwap {
		stableAddrStr := strings.TrimSpace(cc.StableAddress)
		if common.IsHexAddress(stableAddrStr) {
			stableAddr := strings.ToLower(stableAddrStr)
			token0 := strings.ToLower(strings.TrimSpace(poolInfo.Token0))
			token1 := strings.ToLower(strings.TrimSpace(poolInfo.Token1))
			if token0 != stableAddr && token1 != stableAddr {
				w.WriteHeader(http.StatusConflict)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(openPositionError{
					Code:    "entry_swap_required",
					Message: fmt.Sprintf("pool does not contain %s", strings.TrimSpace(cc.StableSymbol)),
				})
				return
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
	}

	// Step 2: calculating range
	ws.SendProgress(user.ID, "open_position", 0, 2, 5, "active", "")
	tickLowerPctReq, tickUpperPctReq := pricing.TickPercentagesFromStablePercentages(tmpTask, req.RangeLowerPct, req.RangeUpperPct)
	if tickLowerPctReq <= 0 || tickUpperPctReq <= 0 {
		http.Error(w, "invalid range", http.StatusBadRequest)
		return
	}

	var currentTick int
	switch poolVersion {
	case "v4":
		if !common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
			http.Error(w, "UNISWAP_V4_POOL_MANAGER_ADDRESS not configured", http.StatusInternalServerError)
			return
		}
		if !common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) {
			http.Error(w, "UNISWAP_V4_STATE_VIEW_ADDRESS not configured", http.StatusInternalServerError)
			return
		}
		poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
		stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
		currentTick, err = blockchain.GetUniswapV4PoolCurrentTickViaStateView(stateView, poolManager, poolAddress)
	default:
		if !common.IsHexAddress(poolAddress) {
			http.Error(w, "invalid pool address", http.StatusBadRequest)
			return
		}
		client, _, cerr := blockchain.GetEVMClient(chain)
		if cerr != nil {
			http.Error(w, cerr.Error(), http.StatusInternalServerError)
			return
		}
		currentTick, err = blockchain.GetV3PoolCurrentTickWithClient(client, common.HexToAddress(poolAddress))
	}
	if err != nil {
		http.Error(w, "failed to read current tick", http.StatusInternalServerError)
		return
	}

	tc := pool.NewTickCalculator()
	tickLower, tickUpper := tc.CalculateTickFromPercentagesBestFit(currentTick, tickLowerPctReq, tickUpperPctReq, poolInfo.TickSpacing)
	if err := tc.ValidateTickRange(tickLower, tickUpper, poolInfo.TickSpacing); err != nil {
		http.Error(w, "invalid tick range", http.StatusBadRequest)
		return
	}

	tickLowerPctEff, tickUpperPctEff := tc.CalculatePercentagesFromTicks(currentTick, tickLower, tickUpper)
	rangePctEff := (tickLowerPctEff + tickUpperPctEff) / 2.0

	slippage := cfg.SlippageTolerance
	if req.Slippage != nil {
		slippage = *req.Slippage
	}

	hooksAddr := normalizeHexPrefixed(poolInfo.HooksAddress)
	if !common.IsHexAddress(hooksAddr) {
		hooksAddr = "0x0000000000000000000000000000000000000000"
	}

	// Step 3: creating task
	ws.SendProgress(user.ID, "open_position", 0, 3, 5, "active", "")

	task := &models.StrategyTask{
		UserID:               user.ID,
		Chain:                chain,
		PoolId:               poolAddress,
		PoolVersion:          poolVersion,
		Exchange:             poolInfo.Exchange,
		WalletID:             selectedWallet.ID,
		WalletAddress:        selectedWallet.Address,
		Token0Symbol:         poolInfo.Token0Symbol,
		Token1Symbol:         poolInfo.Token1Symbol,
		Token0Address:        poolInfo.Token0,
		Token1Address:        poolInfo.Token1,
		HooksAddress:         hooksAddr,
		Fee:                  poolInfo.Fee,
		TickSpacing:          poolInfo.TickSpacing,
		TickLower:            tickLower,
		TickUpper:            tickUpper,
		RangePercentage:      rangePctEff,
		RangeLowerPercentage: tickLowerPctEff,
		RangeUpperPercentage: tickUpperPctEff,
		AmountUSDT:           req.Amount,
		CurrentLiquidity:     "0",
		ReopenDelaySeconds:   cfg.RebalanceTimeout,
		SlippageTolerance:    slippage,
		AutoReinvest:         cfg.AutoReinvest,
		ResidualTolerance:    cfg.ResidualTolerance,
		AllowEntrySwap:       req.AllowEntrySwap,
		StopLossEnabled:      cfg.StopLossEnabled,
		StopLossDelaySeconds: cfg.StopLossDelaySeconds,
		Status:               models.StrategyStatusRunning,
		LastCheckTime:        time.Now(),
	}

	if err := database.DB.Create(task).Error; err != nil {
		ws.SendProgress(user.ID, "open_position", 0, 3, 5, "error", "failed to create task")
		http.Error(w, "failed to create task", http.StatusInternalServerError)
		return
	}

	// Step 4: executing on-chain transaction
	ws.SendProgress(user.ID, "open_position", task.ID, 4, 5, "active", "")

	liquidityService := liquidity.NewLiquidityService()
	enterRes, err := liquidityService.EnterTaskFromUSDT(user.ID, task)
	if err != nil {
		var swapErr *liquidity.EntrySwapRequiredError
		if errors.As(err, &swapErr) {
			_ = database.DB.Model(task).Updates(map[string]interface{}{
				"status":        models.StrategyStatusWaiting,
				"error_message": "entry swap required",
			}).Error
			ws.SendProgress(user.ID, "open_position", task.ID, 4, 5, "error", swapErr.Error())
			w.WriteHeader(http.StatusConflict)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(openPositionError{
				Code:    "entry_swap_required",
				Message: swapErr.Error(),
			})
			return
		}
		_ = database.DB.Model(task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": err.Error(),
		}).Error
		ws.SendProgress(user.ID, "open_position", task.ID, 4, 5, "error", "open position failed")
		http.Error(w, "open position failed", http.StatusInternalServerError)
		return
	}

	if err := applyEnterResult(task, enterRes); err != nil {
		ws.SendProgress(user.ID, "open_position", task.ID, 4, 5, "error", "failed to update task")
		http.Error(w, "failed to update task", http.StatusInternalServerError)
		return
	}

	// All steps done
	ws.SendProgress(user.ID, "open_position", task.ID, 4, 5, "done", "")

	go func() {
		_ = botSvc.SendTaskCardForUser(user.ID, task.ID)
	}()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openPositionResponse{
		TaskID: task.ID,
		TxHash: enterRes.TxHash,
		Status: "ok",
	})
}
