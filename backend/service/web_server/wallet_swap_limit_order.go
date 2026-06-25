package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/txexec"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	walletSwapLimitOrderDefaultListLimit = 20
	walletSwapLimitOrderMaxListLimit     = 100
	walletSwapLimitOrderMaxErrorLen      = 2000
)

type walletSwapLimitOrderRequest struct {
	InitData string `json:"initData"`
	Action   string `json:"action"`

	Chain    string `json:"chain,omitempty"`
	WalletID uint   `json:"wallet_id,omitempty"`
	OrderID  uint   `json:"order_id,omitempty"`

	FromToken       string  `json:"from_token,omitempty"`
	ToToken         string  `json:"to_token,omitempty"`
	Amount          string  `json:"amount,omitempty"`
	TargetPrice     string  `json:"target_price,omitempty"`
	TargetToAmount  string  `json:"target_to_amount,omitempty"`
	SlippagePercent float64 `json:"slippage_percent,omitempty"`
	Provider        string  `json:"provider,omitempty"`
	Limit           int     `json:"limit,omitempty"`
	Offset          int     `json:"offset,omitempty"`
}

type walletSwapLimitOrderTokenDTO struct {
	Address  string `json:"address"`
	Symbol   string `json:"symbol,omitempty"`
	Name     string `json:"name,omitempty"`
	LogoURL  string `json:"logo_url,omitempty"`
	IsNative bool   `json:"is_native,omitempty"`
}

type walletSwapLimitOrderDTO struct {
	ID                        uint                         `json:"id"`
	Chain                     string                       `json:"chain"`
	WalletID                  uint                         `json:"wallet_id"`
	WalletAddress             string                       `json:"wallet_address"`
	Status                    string                       `json:"status"`
	ProviderPreference        string                       `json:"provider_preference"`
	ProviderLabel             string                       `json:"provider_label"`
	FromToken                 walletSwapLimitOrderTokenDTO `json:"from_token"`
	ToToken                   walletSwapLimitOrderTokenDTO `json:"to_token"`
	FromAmount                string                       `json:"from_amount"`
	FromAmountFloat           string                       `json:"from_amount_float,omitempty"`
	TargetToAmount            string                       `json:"target_to_amount"`
	TargetToAmountFloat       string                       `json:"target_to_amount_float,omitempty"`
	TargetPrice               string                       `json:"target_price,omitempty"`
	SlippagePercent           float64                      `json:"slippage_percent"`
	LastCheckedAt             string                       `json:"last_checked_at,omitempty"`
	NextCheckAt               string                       `json:"next_check_at,omitempty"`
	LastQuoteProvider         string                       `json:"last_quote_provider,omitempty"`
	LastQuoteProviderLabel    string                       `json:"last_quote_provider_label,omitempty"`
	LastQuoteToAmount         string                       `json:"last_quote_to_amount,omitempty"`
	LastQuoteToAmountFloat    string                       `json:"last_quote_to_amount_float,omitempty"`
	LastQuoteGasUSD           float64                      `json:"last_quote_gas_usd,omitempty"`
	TriggerProvider           string                       `json:"trigger_provider,omitempty"`
	TriggerProviderLabel      string                       `json:"trigger_provider_label,omitempty"`
	TriggerQuoteToAmount      string                       `json:"trigger_quote_to_amount,omitempty"`
	TriggerQuoteToAmountFloat string                       `json:"trigger_quote_to_amount_float,omitempty"`
	CheckCount                int                          `json:"check_count"`
	TxHash                    string                       `json:"tx_hash,omitempty"`
	TxURL                     string                       `json:"tx_url,omitempty"`
	ActualToAmount            string                       `json:"actual_to_amount,omitempty"`
	ActualToAmountFloat       string                       `json:"actual_to_amount_float,omitempty"`
	CreatedAt                 string                       `json:"created_at"`
	UpdatedAt                 string                       `json:"updated_at"`
	TriggeredAt               string                       `json:"triggered_at,omitempty"`
	FilledAt                  string                       `json:"filled_at,omitempty"`
	CancelledAt               string                       `json:"cancelled_at,omitempty"`
	FailedAt                  string                       `json:"failed_at,omitempty"`
	LastError                 string                       `json:"last_error,omitempty"`
}

type walletSwapLimitOrderResponse struct {
	OK      bool                      `json:"ok"`
	Chain   string                    `json:"chain,omitempty"`
	Order   *walletSwapLimitOrderDTO  `json:"order,omitempty"`
	Orders  []walletSwapLimitOrderDTO `json:"orders,omitempty"`
	Total   int64                     `json:"total,omitempty"`
	Message string                    `json:"message,omitempty"`
}

func normalizeLimitOrderProvider(provider string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "", "best":
		return models.WalletSwapLimitOrderProviderBest, nil
	case "okx":
		return "okx", nil
	case "binance":
		return "binance", nil
	case "0x", "li.fi", "lifi":
		return "", fmt.Errorf("provider %s is no longer supported", provider)
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}
}

func limitOrderProviderLabel(provider string) string {
	if strings.EqualFold(strings.TrimSpace(provider), models.WalletSwapLimitOrderProviderBest) {
		return "Best"
	}
	return swapProviderLabel(provider)
}

func parseDecimalAmountToBigInt(amountStr string, decimals uint8) (*big.Int, error) {
	return parseWalletSwapDecimalAmount(amountStr, decimals)
}

func targetToAmountFromPrice(fromAmount *big.Int, targetPrice string, toDecimals uint8, fromDecimals uint8) (*big.Int, error) {
	targetPrice = strings.TrimSpace(targetPrice)
	if targetPrice == "" {
		return nil, fmt.Errorf("missing target price")
	}
	price, ok := new(big.Float).SetString(targetPrice)
	if !ok {
		return nil, fmt.Errorf("invalid target price")
	}
	if price.Sign() <= 0 {
		return nil, fmt.Errorf("target price must be greater than 0")
	}
	base := new(big.Float).SetInt(fromAmount)
	base.Quo(base, new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromDecimals)), nil)))
	base.Mul(base, price)
	base.Mul(base, new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toDecimals)), nil)))
	out := new(big.Int)
	base.Int(out)
	if out.Sign() <= 0 {
		return nil, fmt.Errorf("target amount must be greater than 0")
	}
	return out, nil
}

func safeLimitOrderError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if len(msg) <= walletSwapLimitOrderMaxErrorLen {
		return msg
	}
	return msg[:walletSwapLimitOrderMaxErrorLen]
}

func formatLimitOrderTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func (s *Server) buildLimitOrderDTO(order models.WalletSwapLimitOrder, decimalsCache map[string]int) walletSwapLimitOrderDTO {
	chain := config.NormalizeChain(order.Chain)
	cc, _ := config.AppConfig.GetChainConfig(chain)
	fromDecimals := walletSwapTokenDecimals(chain, order.FromTokenAddress, decimalsCache)
	toDecimals := walletSwapTokenDecimals(chain, order.ToTokenAddress, decimalsCache)
	dto := walletSwapLimitOrderDTO{
		ID:                        order.ID,
		Chain:                     chain,
		WalletID:                  order.WalletID,
		WalletAddress:             strings.TrimSpace(order.WalletAddress),
		Status:                    strings.TrimSpace(order.Status),
		ProviderPreference:        strings.TrimSpace(order.ProviderPreference),
		ProviderLabel:             limitOrderProviderLabel(order.ProviderPreference),
		FromToken:                 walletSwapLimitOrderTokenDTO(walletSwapTokenMeta(chain, order.FromTokenAddress, nil, cc)),
		ToToken:                   walletSwapLimitOrderTokenDTO(walletSwapTokenMeta(chain, order.ToTokenAddress, nil, cc)),
		FromAmount:                strings.TrimSpace(order.FromAmount),
		FromAmountFloat:           walletSwapHumanAmount(order.FromAmount, fromDecimals),
		TargetToAmount:            strings.TrimSpace(order.TargetToAmount),
		TargetToAmountFloat:       walletSwapHumanAmount(order.TargetToAmount, toDecimals),
		TargetPrice:               strings.TrimSpace(order.TargetPrice),
		SlippagePercent:           order.SlippagePercent,
		LastCheckedAt:             formatLimitOrderTime(order.LastCheckedAt),
		NextCheckAt:               formatLimitOrderTime(order.NextCheckAt),
		LastQuoteProvider:         strings.TrimSpace(order.LastQuoteProvider),
		LastQuoteProviderLabel:    swapProviderLabel(order.LastQuoteProvider),
		LastQuoteToAmount:         strings.TrimSpace(order.LastQuoteToAmount),
		LastQuoteToAmountFloat:    walletSwapHumanAmount(order.LastQuoteToAmount, toDecimals),
		LastQuoteGasUSD:           order.LastQuoteGasUSD,
		TriggerProvider:           strings.TrimSpace(order.TriggerProvider),
		TriggerProviderLabel:      swapProviderLabel(order.TriggerProvider),
		TriggerQuoteToAmount:      strings.TrimSpace(order.TriggerQuoteToAmount),
		TriggerQuoteToAmountFloat: walletSwapHumanAmount(order.TriggerQuoteToAmount, toDecimals),
		CheckCount:                order.CheckCount,
		TxHash:                    strings.TrimSpace(order.TxHash),
		TxURL:                     explorerTxURLHelper(chain, order.TxHash),
		ActualToAmount:            strings.TrimSpace(order.ActualToAmount),
		ActualToAmountFloat:       walletSwapHumanAmount(order.ActualToAmount, toDecimals),
		CreatedAt:                 order.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:                 order.UpdatedAt.Format("2006-01-02 15:04:05"),
		TriggeredAt:               formatLimitOrderTime(order.TriggeredAt),
		FilledAt:                  formatLimitOrderTime(order.FilledAt),
		CancelledAt:               formatLimitOrderTime(order.CancelledAt),
		FailedAt:                  formatLimitOrderTime(order.FailedAt),
		LastError:                 strings.TrimSpace(order.LastError),
	}
	return dto
}

func (s *Server) writeLimitOrder(w http.ResponseWriter, chain string, order models.WalletSwapLimitOrder, message string) {
	decimalsCache := make(map[string]int)
	dto := s.buildLimitOrderDTO(order, decimalsCache)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(walletSwapLimitOrderResponse{
		OK:      true,
		Chain:   chain,
		Order:   &dto,
		Message: message,
	})
}

func (s *Server) handleWalletSwapLimitOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var req walletSwapLimitOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	user, chain, status, msg, err := s.authenticateLimitOrderRequest(req.InitData, req.Chain)
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "create":
		s.handleCreateWalletSwapLimitOrder(w, r, user.ID, chain, req)
	case "list", "":
		s.handleListWalletSwapLimitOrders(w, r, user.ID, chain, req)
	case "cancel":
		s.handleCancelWalletSwapLimitOrder(w, r, user.ID, chain, req)
	case "get":
		s.handleGetWalletSwapLimitOrder(w, r, user.ID, chain, req)
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
	}
}

func (s *Server) authenticateLimitOrderRequest(initData string, requestedChain string) (*models.User, string, int, string, error) {
	user, status, msg := authenticateTelegramWebAppUser(strings.TrimSpace(initData))
	if status != 0 {
		return nil, "", status, msg, nil
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		return nil, "", status, msg, err
	}
	if status != 0 {
		return nil, "", status, msg, nil
	}
	if status, msg := requireModulePermission(check, models.AccessModuleSwap); status != 0 {
		return nil, "", status, msg, nil
	}

	cfgService := userSvc.NewGlobalConfigService()
	cfg, err := cfgService.GetOrCreate(user.ID)
	if err != nil {
		return nil, "", http.StatusInternalServerError, "failed to load config", err
	}

	chain := strings.TrimSpace(requestedChain)
	if chain == "" {
		if cfg != nil && !cfg.MultiChainEnabled {
			chain = config.PickEnabledChain(cfg.DefaultChain)
		} else {
			chain = config.PickEnabledChain("bsc")
		}
	} else {
		chain = config.NormalizeChain(chain)
	}
	return user, chain, 0, "", nil
}

func (s *Server) handleCreateWalletSwapLimitOrder(w http.ResponseWriter, r *http.Request, userID uint, chain string, req walletSwapLimitOrderRequest) {
	if database.DB == nil {
		http.Error(w, "database not initialized", http.StatusInternalServerError)
		return
	}
	exec, err := chainexec.GetEVM(chain)
	if err != nil {
		http.Error(w, "chain init failed: "+safeLimitOrderError(err), http.StatusInternalServerError)
		return
	}
	if exec == nil || exec.Client() == nil {
		http.Error(w, "chain client unavailable", http.StatusInternalServerError)
		return
	}
	fromTokenStr := strings.TrimSpace(req.FromToken)
	toTokenStr := strings.TrimSpace(req.ToToken)
	if !common.IsHexAddress(fromTokenStr) || !common.IsHexAddress(toTokenStr) {
		http.Error(w, "invalid token address", http.StatusBadRequest)
		return
	}
	fromToken := common.HexToAddress(fromTokenStr)
	toToken := common.HexToAddress(toTokenStr)
	if fromToken == toToken {
		http.Error(w, "from_token and to_token cannot be the same", http.StatusBadRequest)
		return
	}

	fromDecimals := tokenDecimals(exec.Client(), fromToken)
	toDecimals := tokenDecimals(exec.Client(), toToken)
	amount, err := parseDecimalAmountToBigInt(req.Amount, fromDecimals)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var targetToAmount *big.Int
	targetToAmountRaw := strings.TrimSpace(req.TargetToAmount)
	if targetToAmountRaw != "" {
		targetToAmount, err = parseDecimalAmountToBigInt(targetToAmountRaw, toDecimals)
	} else {
		targetToAmount, err = targetToAmountFromPrice(amount, req.TargetPrice, toDecimals, fromDecimals)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	provider, err := normalizeLimitOrderProvider(req.Provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	slippage := req.SlippagePercent
	if slippage <= 0 {
		cfg, cfgErr := userSvc.NewGlobalConfigService().GetOrCreate(userID)
		if cfgErr != nil {
			http.Error(w, "failed to load config", http.StatusInternalServerError)
			return
		}
		if cfg != nil && cfg.SlippageTolerance > 0 {
			slippage = cfg.SlippageTolerance
		} else {
			slippage = 1.0
		}
	}

	walletService := wallet.NewWalletService()
	wlt, err := walletService.ResolveTaskWallet(userID, req.WalletID, "")
	if err != nil || wlt == nil {
		http.Error(w, "wallet not found", http.StatusBadRequest)
		return
	}
	walletAddr := common.HexToAddress(wlt.Address)
	now := time.Now()
	order := models.WalletSwapLimitOrder{
		UserID:             userID,
		Chain:              chain,
		WalletID:           wlt.ID,
		WalletAddress:      walletAddr.Hex(),
		FromTokenAddress:   fromToken.Hex(),
		ToTokenAddress:     toToken.Hex(),
		FromAmount:         amount.String(),
		TargetToAmount:     targetToAmount.String(),
		TargetPrice:        strings.TrimSpace(req.TargetPrice),
		SlippagePercent:    slippage,
		ProviderPreference: provider,
		Status:             models.WalletSwapLimitOrderStatusOpen,
		NextCheckAt:        &now,
	}
	if err := database.DB.WithContext(r.Context()).Create(&order).Error; err != nil {
		http.Error(w, "failed to create limit order", http.StatusInternalServerError)
		return
	}
	s.writeLimitOrder(w, chain, order, "限价单已创建")
}

func (s *Server) handleListWalletSwapLimitOrders(w http.ResponseWriter, r *http.Request, userID uint, chain string, req walletSwapLimitOrderRequest) {
	limit := req.Limit
	if limit <= 0 {
		limit = walletSwapLimitOrderDefaultListLimit
	}
	if limit > walletSwapLimitOrderMaxListLimit {
		limit = walletSwapLimitOrderMaxListLimit
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	db := database.DB.WithContext(r.Context()).Model(&models.WalletSwapLimitOrder{}).
		Where("user_id = ? AND chain = ?", userID, chain)
	if req.WalletID > 0 {
		db = db.Where("wallet_id = ?", req.WalletID)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		http.Error(w, "failed to count limit orders", http.StatusInternalServerError)
		return
	}
	var orders []models.WalletSwapLimitOrder
	if err := db.Order("created_at DESC").Limit(limit).Offset(offset).Find(&orders).Error; err != nil {
		http.Error(w, "failed to query limit orders", http.StatusInternalServerError)
		return
	}
	decimalsCache := make(map[string]int)
	rows := make([]walletSwapLimitOrderDTO, 0, len(orders))
	for _, order := range orders {
		rows = append(rows, s.buildLimitOrderDTO(order, decimalsCache))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(walletSwapLimitOrderResponse{
		OK:     true,
		Chain:  chain,
		Orders: rows,
		Total:  total,
	})
}

func (s *Server) handleGetWalletSwapLimitOrder(w http.ResponseWriter, r *http.Request, userID uint, chain string, req walletSwapLimitOrderRequest) {
	if req.OrderID == 0 {
		http.Error(w, "missing order_id", http.StatusBadRequest)
		return
	}
	var order models.WalletSwapLimitOrder
	if err := database.DB.WithContext(r.Context()).
		Where("id = ? AND user_id = ? AND chain = ?", req.OrderID, userID, chain).
		First(&order).Error; err != nil {
		http.Error(w, "limit order not found", http.StatusNotFound)
		return
	}
	s.writeLimitOrder(w, chain, order, "")
}

func (s *Server) handleCancelWalletSwapLimitOrder(w http.ResponseWriter, r *http.Request, userID uint, chain string, req walletSwapLimitOrderRequest) {
	if req.OrderID == 0 {
		http.Error(w, "missing order_id", http.StatusBadRequest)
		return
	}
	now := time.Now()
	result := database.DB.WithContext(r.Context()).Model(&models.WalletSwapLimitOrder{}).
		Where("id = ? AND user_id = ? AND chain = ? AND status = ?", req.OrderID, userID, chain, models.WalletSwapLimitOrderStatusOpen).
		Updates(map[string]interface{}{
			"status":       models.WalletSwapLimitOrderStatusCancelled,
			"cancelled_at": &now,
			"last_error":   "",
		})
	if result.Error != nil {
		http.Error(w, "failed to cancel limit order", http.StatusInternalServerError)
		return
	}
	if result.RowsAffected == 0 {
		http.Error(w, "limit order cannot be cancelled", http.StatusBadRequest)
		return
	}
	var order models.WalletSwapLimitOrder
	if err := database.DB.WithContext(r.Context()).First(&order, req.OrderID).Error; err != nil {
		http.Error(w, "limit order not found after cancel", http.StatusInternalServerError)
		return
	}
	s.writeLimitOrder(w, chain, order, "限价单已取消")
}

type WalletSwapLimitOrderWorker struct {
	stopCh   chan struct{}
	stopOnce sync.Once
	ticker   *time.Ticker
}

func NewWalletSwapLimitOrderWorker() *WalletSwapLimitOrderWorker {
	return &WalletSwapLimitOrderWorker{
		stopCh: make(chan struct{}),
	}
}

func (w *WalletSwapLimitOrderWorker) Start() {
	if w == nil {
		return
	}
	if config.AppConfig != nil && !config.AppConfig.WalletSwapLimitOrdersEnabled {
		log.Println("[WalletSwapLimitOrder] disabled")
		return
	}
	interval := 15 * time.Second
	if config.AppConfig != nil && config.AppConfig.WalletSwapLimitOrderIntervalSeconds > 0 {
		interval = time.Duration(config.AppConfig.WalletSwapLimitOrderIntervalSeconds) * time.Second
	}
	w.ticker = time.NewTicker(interval)
	go func() {
		w.runOnce()
		for {
			select {
			case <-w.stopCh:
				return
			case <-w.ticker.C:
				w.runOnce()
			}
		}
	}()
}

func (w *WalletSwapLimitOrderWorker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		close(w.stopCh)
		if w.ticker != nil {
			w.ticker.Stop()
		}
	})
}

func (w *WalletSwapLimitOrderWorker) runOnce() {
	if database.DB == nil {
		log.Println("[WalletSwapLimitOrder] skipped: mysql not initialized")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	batchSize := 20
	maxParallel := 2
	if config.AppConfig != nil {
		if config.AppConfig.WalletSwapLimitOrderBatchSize > 0 {
			batchSize = config.AppConfig.WalletSwapLimitOrderBatchSize
		}
		if config.AppConfig.WalletSwapLimitOrderMaxParallel > 0 {
			maxParallel = config.AppConfig.WalletSwapLimitOrderMaxParallel
		}
	}
	orders, err := w.loadDueOrders(ctx, batchSize)
	if err != nil {
		log.Printf("[WalletSwapLimitOrder] load due orders failed: %v", err)
		return
	}
	if len(orders) == 0 {
		return
	}
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for _, order := range orders {
		order := order
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := w.processOrder(ctx, order.ID); err != nil {
				log.Printf("[WalletSwapLimitOrder] process order #%d failed: %v", order.ID, err)
			}
		}()
	}
	wg.Wait()
}

func (w *WalletSwapLimitOrderWorker) loadDueOrders(ctx context.Context, limit int) ([]models.WalletSwapLimitOrder, error) {
	if limit <= 0 {
		return nil, nil
	}
	now := time.Now()
	var unscheduled []models.WalletSwapLimitOrder
	if err := database.DB.WithContext(ctx).
		Where("status = ? AND next_check_at IS NULL", models.WalletSwapLimitOrderStatusOpen).
		Order("created_at ASC").
		Limit(limit).
		Find(&unscheduled).Error; err != nil {
		return nil, err
	}
	var scheduled []models.WalletSwapLimitOrder
	err := database.DB.WithContext(ctx).
		Where("status = ? AND next_check_at <= ?", models.WalletSwapLimitOrderStatusOpen, now).
		Order("next_check_at ASC").
		Order("created_at ASC").
		Limit(limit).
		Find(&scheduled).Error
	if err != nil {
		return nil, err
	}
	orders := append(unscheduled, scheduled...)
	sort.SliceStable(orders, func(i, j int) bool {
		left := limitOrderDueTime(orders[i])
		right := limitOrderDueTime(orders[j])
		if left.Equal(right) {
			return orders[i].ID < orders[j].ID
		}
		return left.Before(right)
	})
	if len(orders) > limit {
		orders = orders[:limit]
	}
	return orders, nil
}

func limitOrderDueTime(order models.WalletSwapLimitOrder) time.Time {
	if order.NextCheckAt != nil {
		return *order.NextCheckAt
	}
	return order.CreatedAt
}

func (w *WalletSwapLimitOrderWorker) processOrder(ctx context.Context, orderID uint) error {
	var order models.WalletSwapLimitOrder
	if err := database.DB.WithContext(ctx).First(&order, orderID).Error; err != nil {
		return err
	}
	if order.Status != models.WalletSwapLimitOrderStatusOpen {
		return nil
	}
	quote, err := w.quoteOrder(ctx, &order)
	now := time.Now()
	if err != nil {
		next := w.nextCheckAt(now)
		return database.DB.WithContext(ctx).Model(&models.WalletSwapLimitOrder{}).
			Where("id = ? AND status = ?", order.ID, models.WalletSwapLimitOrderStatusOpen).
			Updates(map[string]interface{}{
				"last_checked_at": &now,
				"next_check_at":   &next,
				"last_error":      safeLimitOrderError(err),
				"check_count":     gorm.Expr("check_count + 1"),
			}).Error
	}
	if quote == nil || quote.Status != "available" {
		next := w.nextCheckAt(now)
		errMsg := "no available quote"
		if quote != nil && strings.TrimSpace(quote.Error) != "" {
			errMsg = strings.TrimSpace(quote.Error)
		}
		return database.DB.WithContext(ctx).Model(&models.WalletSwapLimitOrder{}).
			Where("id = ? AND status = ?", order.ID, models.WalletSwapLimitOrderStatusOpen).
			Updates(map[string]interface{}{
				"last_checked_at":      &now,
				"next_check_at":        &next,
				"last_quote_provider":  quoteProvider(quote),
				"last_quote_to_amount": quoteAmount(quote),
				"last_error":           errMsg,
				"check_count":          gorm.Expr("check_count + 1"),
			}).Error
	}

	target, ok := parseSwapBigInt(order.TargetToAmount)
	if !ok || target == nil || target.Sign() <= 0 {
		return fmt.Errorf("invalid target amount for order #%d", order.ID)
	}
	got, ok := parseSwapBigInt(quote.NetToAmount)
	if !ok || got == nil || got.Sign() <= 0 {
		return fmt.Errorf("invalid quote amount for order #%d", order.ID)
	}
	if got.Cmp(target) < 0 {
		next := w.nextCheckAt(now)
		return database.DB.WithContext(ctx).Model(&models.WalletSwapLimitOrder{}).
			Where("id = ? AND status = ?", order.ID, models.WalletSwapLimitOrderStatusOpen).
			Updates(map[string]interface{}{
				"last_checked_at":      &now,
				"next_check_at":        &next,
				"last_quote_provider":  quote.Provider,
				"last_quote_to_amount": quote.NetToAmount,
				"last_quote_gas_usd":   quote.EstimatedGasUSD,
				"last_error":           "",
				"check_count":          gorm.Expr("check_count + 1"),
			}).Error
	}

	locked, err := w.lockOrderForTrigger(ctx, order.ID, quote)
	if err != nil {
		return err
	}
	if locked == nil {
		return nil
	}
	executor := txexec.Default()
	if executor == nil {
		return w.releaseTrigger(ctx, locked.ID, "transaction executor unavailable")
	}
	if !executor.TryRunWallet(locked.WalletAddress, func() {
		execCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := w.executeOrder(execCtx, locked); err != nil {
			log.Printf("[WalletSwapLimitOrder] execute order #%d failed: %v", locked.ID, err)
		}
	}) {
		return w.releaseTrigger(ctx, locked.ID, "wallet is processing another transaction")
	}
	return nil
}

func quoteProvider(quote *swapProviderQuote) string {
	if quote == nil {
		return ""
	}
	return strings.TrimSpace(quote.Provider)
}

func quoteAmount(quote *swapProviderQuote) string {
	if quote == nil {
		return ""
	}
	return strings.TrimSpace(quote.NetToAmount)
}

func (w *WalletSwapLimitOrderWorker) nextCheckAt(now time.Time) time.Time {
	interval := 15 * time.Second
	if config.AppConfig != nil && config.AppConfig.WalletSwapLimitOrderIntervalSeconds > 0 {
		interval = time.Duration(config.AppConfig.WalletSwapLimitOrderIntervalSeconds) * time.Second
	}
	return now.Add(interval)
}

func (w *WalletSwapLimitOrderWorker) quoteOrder(ctx context.Context, order *models.WalletSwapLimitOrder) (*swapProviderQuote, error) {
	if order == nil {
		return nil, fmt.Errorf("order is nil")
	}
	exec, err := chainexec.GetEVM(order.Chain)
	if err != nil {
		return nil, err
	}
	if exec == nil || exec.Client() == nil {
		return nil, fmt.Errorf("chain client unavailable")
	}
	cc := exec.Config()
	fromToken := common.HexToAddress(order.FromTokenAddress)
	toToken := common.HexToAddress(order.ToTokenAddress)
	amount, ok := parseSwapBigInt(order.FromAmount)
	if !ok || amount == nil || amount.Sign() <= 0 {
		return nil, fmt.Errorf("invalid order amount")
	}
	toDecimals := int(tokenDecimals(exec.Client(), toToken))
	slippageDecimal := fmt.Sprintf("%.4f", order.SlippagePercent/100)
	quotes := aggregateSwapProviderQuotes(
		order.Chain,
		cc,
		exec.Client(),
		common.HexToAddress(order.WalletAddress),
		order.FromTokenAddress,
		order.ToTokenAddress,
		fromToken,
		toToken,
		amount,
		slippageDecimal,
		order.SlippagePercent,
		toDecimals,
	)
	quotes, best := normalizeProviderQuotes(quotes)
	preference, err := normalizeLimitOrderProvider(order.ProviderPreference)
	if err != nil {
		return nil, err
	}
	if preference == models.WalletSwapLimitOrderProviderBest {
		if best == nil {
			return nil, nil
		}
		return best, nil
	}
	for i := range quotes {
		if strings.EqualFold(quotes[i].Provider, preference) {
			return &quotes[i], nil
		}
	}
	return nil, fmt.Errorf("selected provider quote missing: %s", preference)
}

func (w *WalletSwapLimitOrderWorker) lockOrderForTrigger(ctx context.Context, orderID uint, quote *swapProviderQuote) (*models.WalletSwapLimitOrder, error) {
	now := time.Now()
	result := database.DB.WithContext(ctx).Model(&models.WalletSwapLimitOrder{}).
		Where("id = ? AND status = ?", orderID, models.WalletSwapLimitOrderStatusOpen).
		Updates(map[string]interface{}{
			"status":                  models.WalletSwapLimitOrderStatusTriggering,
			"last_checked_at":         &now,
			"next_check_at":           nil,
			"last_quote_provider":     quote.Provider,
			"last_quote_to_amount":    quote.NetToAmount,
			"last_quote_gas_usd":      quote.EstimatedGasUSD,
			"trigger_provider":        quote.Provider,
			"trigger_quote_to_amount": quote.NetToAmount,
			"triggered_at":            &now,
			"last_error":              "",
			"check_count":             gorm.Expr("check_count + 1"),
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	var locked models.WalletSwapLimitOrder
	if err := database.DB.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&locked, orderID).Error; err != nil {
		return nil, err
	}
	return &locked, nil
}

func (w *WalletSwapLimitOrderWorker) releaseTrigger(ctx context.Context, orderID uint, errMsg string) error {
	now := time.Now()
	next := w.nextCheckAt(now)
	return database.DB.WithContext(ctx).Model(&models.WalletSwapLimitOrder{}).
		Where("id = ? AND status = ?", orderID, models.WalletSwapLimitOrderStatusTriggering).
		Updates(map[string]interface{}{
			"status":        models.WalletSwapLimitOrderStatusOpen,
			"next_check_at": &next,
			"last_error":    strings.TrimSpace(errMsg),
		}).Error
}

func (w *WalletSwapLimitOrderWorker) executeOrder(ctx context.Context, order *models.WalletSwapLimitOrder) error {
	if order == nil {
		return fmt.Errorf("order is nil")
	}
	walletService := wallet.NewWalletService()
	wlt, err := walletService.GetWalletByID(order.UserID, order.WalletID)
	if err != nil {
		w.markFailed(ctx, order.ID, err)
		return err
	}
	pkHex, err := walletService.GetPrivateKey(wlt)
	if err != nil {
		w.markFailed(ctx, order.ID, err)
		return err
	}
	privateKey, err := crypto.HexToECDSA(pkHex)
	if err != nil {
		w.markFailed(ctx, order.ID, fmt.Errorf("invalid private key"))
		return err
	}
	exec, err := chainexec.GetEVM(order.Chain)
	if err != nil {
		w.markFailed(ctx, order.ID, err)
		return err
	}
	amount, ok := parseSwapBigInt(order.FromAmount)
	if !ok || amount == nil || amount.Sign() <= 0 {
		err = fmt.Errorf("invalid order amount")
		w.markFailed(ctx, order.ID, err)
		return err
	}
	provider := strings.TrimSpace(order.TriggerProvider)
	if provider == "" {
		provider = strings.TrimSpace(order.ProviderPreference)
	}
	if provider == models.WalletSwapLimitOrderProviderBest {
		quote, qerr := w.quoteOrder(ctx, order)
		if qerr != nil {
			w.markFailed(ctx, order.ID, qerr)
			return qerr
		}
		if quote == nil || quote.Status != "available" || strings.TrimSpace(quote.Provider) == "" {
			err = fmt.Errorf("no executable provider available")
			w.markFailed(ctx, order.ID, err)
			return err
		}
		provider = strings.TrimSpace(quote.Provider)
	}

	lpService := liquidity.NewLiquidityService()
	walletAddr := common.HexToAddress(order.WalletAddress)
	fromToken := common.HexToAddress(order.FromTokenAddress)
	toToken := common.HexToAddress(order.ToTokenAddress)
	var quoteID string
	if strings.EqualFold(provider, "binance") {
		quote, qerr := w.quoteOrder(ctx, order)
		if qerr != nil {
			w.markFailed(ctx, order.ID, qerr)
			return qerr
		}
		if quote == nil || quote.Status != "available" || !strings.EqualFold(quote.Provider, "binance") || strings.TrimSpace(quote.QuoteID) == "" {
			err = fmt.Errorf("no executable Binance route available")
			w.markFailed(ctx, order.ID, err)
			return err
		}
		quoteID = strings.TrimSpace(quote.QuoteID)
	}
	balance, err := walletSwapAssetBalance(exec.Client(), fromToken, walletAddr)
	if err != nil {
		w.markFailed(ctx, order.ID, fmt.Errorf("check balance failed: %w", err))
		return err
	}
	if balance == nil || balance.Cmp(amount) < 0 {
		err = fmt.Errorf("insufficient balance: have %s, need %s", balanceString(balance), amount.String())
		w.markFailed(ctx, order.ID, err)
		return err
	}
	swapResult, err := lpService.SwapSingleTokenDetailedByProviderQuote(provider, quoteID, exec, privateKey, walletAddr, fromToken, toToken, amount, order.SlippagePercent)
	if err != nil {
		w.markFailed(ctx, order.ID, err)
		return err
	}
	recordWalletSwapTransaction(order.UserID, order.Chain, provider, walletAddr, order.FromTokenAddress, order.ToTokenAddress, amount, swapResult)
	now := time.Now()
	amountOut := ""
	txHash := ""
	if swapResult != nil {
		txHash = strings.TrimSpace(swapResult.TxHash)
		if swapResult.AmountOut != nil && swapResult.AmountOut.Sign() > 0 {
			amountOut = swapResult.AmountOut.String()
		}
		if strings.TrimSpace(swapResult.Provider) != "" {
			provider = strings.TrimSpace(swapResult.Provider)
		}
	}
	return database.DB.WithContext(ctx).Model(&models.WalletSwapLimitOrder{}).
		Where("id = ? AND status = ?", order.ID, models.WalletSwapLimitOrderStatusTriggering).
		Updates(map[string]interface{}{
			"status":           models.WalletSwapLimitOrderStatusFilled,
			"trigger_provider": provider,
			"tx_hash":          txHash,
			"actual_to_amount": amountOut,
			"filled_at":        &now,
			"last_error":       "",
		}).Error
}

func (w *WalletSwapLimitOrderWorker) markFailed(ctx context.Context, orderID uint, err error) {
	now := time.Now()
	if uerr := database.DB.WithContext(ctx).Model(&models.WalletSwapLimitOrder{}).
		Where("id = ? AND status = ?", orderID, models.WalletSwapLimitOrderStatusTriggering).
		Updates(map[string]interface{}{
			"status":     models.WalletSwapLimitOrderStatusFailed,
			"failed_at":  &now,
			"last_error": safeLimitOrderError(err),
		}).Error; uerr != nil {
		log.Printf("[WalletSwapLimitOrder] mark failed order #%d failed: %v", orderID, uerr)
	}
}
