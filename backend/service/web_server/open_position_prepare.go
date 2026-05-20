package web_server

import (
	"encoding/json"
	"net/http"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/pool"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"

	"github.com/ethereum/go-ethereum/common"
)

type openPositionPrepareRequest struct {
	InitData    string `json:"initData"`
	WalletID    uint   `json:"wallet_id,omitempty"`
	Chain       string `json:"chain"`
	PoolAddress string `json:"pool_address"`
	PoolVersion string `json:"pool_version"`
}

type openPositionPrepareResponse struct {
	Status                  string                       `json:"status"`
	Checks                  []openPositionCheckItem      `json:"checks,omitempty"`
	PrivateZap              openPositionPrivateZapInfo   `json:"private_zap"`
	WalletSelectionRequired bool                         `json:"wallet_selection_required,omitempty"`
	ResolvedWalletID        uint                         `json:"resolved_wallet_id,omitempty"`
	RangeEditor             *openPositionRangeEditorInfo `json:"range_editor,omitempty"`
}

type openPositionPrepareContext struct {
	user                    *models.User
	chain                   string
	selectedWallet          *models.Wallet
	poolVersion             string
	liquidityService        *liquidity.LiquidityService
	task                    *models.StrategyTask
	currentTick             int
	walletSelectionRequired bool
}

func decodeOpenPositionPrepareRequest(w http.ResponseWriter, r *http.Request) (*openPositionPrepareRequest, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "请求方法不支持", http.StatusMethodNotAllowed)
		return nil, false
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req openPositionPrepareRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid request parameters", http.StatusBadRequest)
		return nil, false
	}

	req.InitData = strings.TrimSpace(req.InitData)
	req.Chain = strings.TrimSpace(req.Chain)
	req.PoolAddress = strings.TrimSpace(req.PoolAddress)
	req.PoolVersion = strings.ToLower(strings.TrimSpace(req.PoolVersion))
	if req.PoolAddress == "" {
		http.Error(w, "缺少池子地址", http.StatusBadRequest)
		return nil, false
	}
	if config.AppConfig == nil {
		http.Error(w, "system config not initialized", http.StatusInternalServerError)
		return nil, false
	}
	return &req, true
}

func buildOpenPositionChecks(
	liquidityService *liquidity.LiquidityService,
	task *models.StrategyTask,
	options liquidity.OpenPositionRiskOptions,
) ([]openPositionCheckItem, error) {
	checkResults, err := liquidityService.CollectOpenPositionChecks(task, options)
	if err != nil {
		return nil, err
	}
	checks := make([]openPositionCheckItem, 0, len(checkResults))
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
	return checks, nil
}

func (s *Server) prepareOpenPositionCheckContext(req openPositionPrepareRequest) (*openPositionPrepareContext, *openPositionError, int) {
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
		return nil, &openPositionError{Code: "open_position_failed", Message: "load global config failed"}, http.StatusInternalServerError
	}

	var chain string
	if cfg != nil && !cfg.MultiChainEnabled {
		chain = config.PickEnabledChain(cfg.DefaultChain)
	} else if strings.TrimSpace(req.Chain) != "" {
		chain = config.NormalizeChain(req.Chain)
	} else {
		chain = config.PickEnabledChain("bsc")
	}

	chainCfg, ok := config.AppConfig.GetChainConfig(chain)
	if !ok || strings.TrimSpace(chainCfg.Chain) == "" {
		return nil, &openPositionError{Code: "invalid_chain", Message: "chain is not supported for opening positions"}, http.StatusBadRequest
	}

	walletService := wallet.NewWalletService()
	wallets, err := walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		return nil, &openPositionError{Code: "wallet_required", Message: "no available execution wallet found"}, http.StatusBadRequest
	}

	selectedWallet := &wallets[0]
	for i := range wallets {
		if wallets[i].IsDefault {
			selectedWallet = &wallets[i]
			break
		}
	}

	walletSelectionRequired := cfg != nil && cfg.MultiWalletEnabled && len(wallets) > 1 && req.WalletID == 0
	if req.WalletID != 0 {
		walletRec, werr := walletService.GetWalletByID(user.ID, req.WalletID)
		if werr != nil || walletRec == nil {
			return nil, &openPositionError{Code: "invalid_wallet", Message: "钱包无效"}, http.StatusBadRequest
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
			return nil, &openPositionError{Code: "invalid_chain", Message: "当前链暂不支持 V4 池子"}, http.StatusBadRequest
		}
		poolInfo, err = poolService.GetPoolInfoForVersionCached(chain, "v4", poolAddress)
	default:
		poolInfo, err = poolService.GetPoolInfoForVersionCached(chain, "v3", poolAddress)
	}
	if err != nil || poolInfo == nil {
		message := "load pool info failed"
		if err != nil {
			message = strings.TrimSpace(err.Error())
		}
		return nil, &openPositionError{Code: "pool_info_error", Message: message}, http.StatusInternalServerError
	}
	if poolInfo.TickSpacing <= 0 {
		return nil, &openPositionError{Code: "pool_info_error", Message: "池子 TickSpacing 无效"}, http.StatusInternalServerError
	}

	currentTick, err := loadOpenPositionCurrentTick(chain, poolVersion, poolAddress)
	if err != nil {
		return nil, &openPositionError{Code: "open_position_failed", Message: "read current tick failed"}, http.StatusInternalServerError
	}
	hooksAddr := normalizeHexPrefixed(poolInfo.HooksAddress)
	if !common.IsHexAddress(hooksAddr) {
		hooksAddr = "0x0000000000000000000000000000000000000000"
	}

	task := &models.StrategyTask{
		UserID:        user.ID,
		Chain:         chain,
		PoolId:        poolAddress,
		PoolVersion:   poolVersion,
		Exchange:      poolInfo.Exchange,
		WalletID:      selectedWallet.ID,
		WalletAddress: selectedWallet.Address,
		Token0Symbol:  poolInfo.Token0Symbol,
		Token1Symbol:  poolInfo.Token1Symbol,
		Token0Address: poolInfo.Token0,
		Token1Address: poolInfo.Token1,
		HooksAddress:  hooksAddr,
		Fee:           poolInfo.Fee,
		TickSpacing:   poolInfo.TickSpacing,
		AmountUSDT:    0,
	}

	return &openPositionPrepareContext{
		user:                    user,
		chain:                   chain,
		selectedWallet:          selectedWallet,
		poolVersion:             poolVersion,
		liquidityService:        liquidity.NewLiquidityService(),
		task:                    task,
		currentTick:             currentTick,
		walletSelectionRequired: walletSelectionRequired,
	}, nil, 0
}

func (s *Server) handleOpenPositionPrepare(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeOpenPositionPrepareRequest(w, r)
	if !ok {
		return
	}

	ctx, errResp, status := s.prepareOpenPositionCheckContext(*req)
	if errResp != nil {
		writeOpenPositionError(w, status, *errResp)
		return
	}

	checks, err := buildOpenPositionChecks(ctx.liquidityService, ctx.task, liquidity.OpenPositionRiskOptions{
		AckLiquidityRisk:    false,
		RequireLiquidityAck: false,
	})
	if err != nil {
		writeOpenPositionError(w, http.StatusInternalServerError, openPositionError{
			Code:    "open_position_failed",
			Message: err.Error(),
		})
		return
	}

	statusText := "ok"
	for _, item := range checks {
		if item.Status == "fail" {
			statusText = "fail"
			break
		}
	}
	rangeEditor := buildOpenPositionRangeEditorInfo(ctx.task, ctx.currentTick, ctx.task.TickSpacing, nil)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openPositionPrepareResponse{
		Status:                  statusText,
		Checks:                  checks,
		PrivateZap:              buildOpenPositionPrivateZapInfo(ctx.liquidityService, ctx.chain, ctx.selectedWallet.ID),
		WalletSelectionRequired: ctx.walletSelectionRequired,
		ResolvedWalletID:        ctx.selectedWallet.ID,
		RangeEditor:             rangeEditor,
	})
}
