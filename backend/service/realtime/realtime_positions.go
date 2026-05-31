package realtime

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"TgLpBot/service/strategy"
	"TgLpBot/service/wallet"
	"context"
	"fmt"
	"log"
	"math"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/sync/errgroup"
)

type RealtimePositionsService struct {
	walletService *wallet.WalletService
	taskService   *strategy.StrategyTaskService
	priceService  *pricing.TokenPriceService
	pnlService    *strategy.PnLService
}

type realtimeTokenMeta struct {
	symbol   string
	decimals int
}

func NewRealtimePositionsService() *RealtimePositionsService {
	return &RealtimePositionsService{
		walletService: wallet.NewWalletService(),
		taskService:   strategy.NewStrategyTaskService(),
		priceService:  pricing.DefaultTokenPriceService(),
		pnlService:    strategy.NewPnLService(),
	}
}

func (s *RealtimePositionsService) InvalidateUser(userID uint) {
}

type RealtimePositionsResponse struct {
	Wallet          RealtimeWallet     `json:"wallet"`
	Summary         RealtimeSummary    `json:"summary"`
	Positions       []RealtimePosition `json:"positions"`
	PollIntervalSec int                `json:"poll_interval_sec"`
	IsAdmin         bool               `json:"is_admin"`
	UpdatedAt       time.Time          `json:"updated_at"`
	Warnings        []string           `json:"warnings,omitempty"`
}

type RealtimeWallet struct {
	Address       string  `json:"address"`
	BNBBalance    string  `json:"bnb_balance"`
	BNBBalanceWEI string  `json:"bnb_balance_wei,omitempty"`
	BNBPriceUSD   float64 `json:"bnb_price_usd"`
	BNBUSD        float64 `json:"bnb_usd"`
}

type RealtimeSummary struct {
	WalletUSD   float64 `json:"wallet_usd"`
	PositionUSD float64 `json:"position_usd"`
	FeeUSD      float64 `json:"fee_usd"`
	TotalUSD    float64 `json:"total_usd"`
}

type RealtimePosition struct {
	Chain                 string             `json:"chain"`
	Version               string             `json:"version"`
	Exchange              string             `json:"exchange"`
	Title                 string             `json:"title"`
	FeeTier               uint64             `json:"fee_tier,omitempty"`
	PoolID                string             `json:"pool_id"`
	PositionID            string             `json:"position_id"`
	WalletID              uint               `json:"wallet_id,omitempty"`
	WalletAddress         string             `json:"wallet_address,omitempty"`
	TaskID                uint               `json:"task_id,omitempty"`
	TaskPaused            bool               `json:"task_paused"`
	TaskRebalanceEnabled  bool               `json:"task_rebalance_enabled"`
	TaskMode              string             `json:"task_mode,omitempty"`
	TaskAmountUSDT        float64            `json:"task_amount_usdt,omitempty"`
	TaskSlippageTolerance float64            `json:"task_slippage_tolerance,omitempty"`
	StatusLabel           string             `json:"status_label"`
	InRange               bool               `json:"in_range"`
	CurrentTick           int                `json:"current_tick"`
	TickLower             int                `json:"tick_lower"`
	TickUpper             int                `json:"tick_upper"`
	TickSpacing           int                `json:"tick_spacing,omitempty"`
	RangePercent          float64            `json:"range_percent"`
	TaskRangeLowerPct     float64            `json:"task_range_lower_pct,omitempty"`
	TaskRangeUpperPct     float64            `json:"task_range_upper_pct,omitempty"`
	OutOfRange            string             `json:"out_of_range"`
	RunningSince          *time.Time         `json:"running_since,omitempty"`
	HasLiquidity          bool               `json:"has_liquidity"`
	InitialCostUSD        float64            `json:"initial_cost_usd,omitempty"`
	NetInvestedUSD        float64            `json:"net_invested_usd,omitempty"`
	CurrentValueUSD       float64            `json:"current_value_usd,omitempty"`
	AbsolutePnLUSD        float64            `json:"absolute_pnl_usd,omitempty"`
	HasPnL                bool               `json:"has_pnl,omitempty"`
	DCA                   *RealtimeDCAStatus `json:"dca,omitempty"`

	TokenRows []RealtimeTokenRow `json:"token_rows"`
	Totals    RealtimeTotals     `json:"totals"`
}

// tickSpacingFromFee maps common V3 fee tiers to tick spacing.
func tickSpacingFromFee(fee uint64) int {
	switch fee {
	case 100:
		return 1
	case 500:
		return 10
	case 2500:
		return 50
	case 3000:
		return 60
	case 10000:
		return 200
	case 20000:
		return 2000
	default:
		return 0
	}
}

type RealtimeTokenRow struct {
	Address  string  `json:"address"`
	Symbol   string  `json:"symbol"`
	Decimals int     `json:"decimals"`
	PriceUSD float64 `json:"price_usd"`

	WalletAmount   string  `json:"wallet_amount"`
	WalletUSD      float64 `json:"wallet_usd"`
	PositionAmount string  `json:"position_amount"`
	PositionUSD    float64 `json:"position_usd"`
	FeeAmount      string  `json:"fee_amount"`
	FeeUSD         float64 `json:"fee_usd"`

	PriceUSDText string `json:"price_usd_text"`
}

type RealtimeTotals struct {
	WalletUSD   float64 `json:"wallet_usd"`
	PositionUSD float64 `json:"position_usd"`
	FeeUSD      float64 `json:"fee_usd"`
	TotalUSD    float64 `json:"total_usd"`
}

type RealtimeDCAStatus struct {
	Enabled       bool       `json:"enabled"`
	PlanValid     bool       `json:"plan_valid"`
	ExecutedCount int        `json:"executed_count"`
	TotalCount    int        `json:"total_count"`
	RetryCount    int        `json:"retry_count,omitempty"`
	NextBatchAt   *time.Time `json:"next_batch_at,omitempty"`
	Pending       bool       `json:"pending"`
	Finished      bool       `json:"finished"`
	Completed     bool       `json:"completed"`
	Canceled      bool       `json:"canceled"`
}

func (s *RealtimePositionsService) GetForUser(userID uint) (*RealtimePositionsResponse, error) {
	return s.compute(userID)
}

func (s *RealtimePositionsService) compute(userID uint) (*RealtimePositionsResponse, error) {
	start := time.Now()
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	if blockchain.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if database.DB == nil {
		return nil, fmt.Errorf("db not initialized")
	}

	wallet, err := s.walletService.GetDefaultWallet(userID)
	if err != nil {
		return nil, err
	}
	walletAddrStr := strings.TrimSpace(wallet.Address)
	if !common.IsHexAddress(walletAddrStr) {
		return nil, fmt.Errorf("invalid wallet address: %s", walletAddrStr)
	}
	walletAddr := common.HexToAddress(walletAddrStr)

	bnbBalWei, _ := blockchain.GetBalance(walletAddr)
	if bnbBalWei == nil {
		bnbBalWei = big.NewInt(0)
	}

	const wbnb = "0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c"
	bnbPriceUSD := 0.0
	if prices, err := s.priceService.GetUSDPrices("bsc", []string{wbnb}); err == nil {
		bnbPriceUSD = prices[wbnb]
	}
	bnbUSD := toFloat(bnbBalWei, 18) * bnbPriceUSD

	// Track standalone wallet tokens (e.g., USDT) even when there are no positions.
	extraWalletTokenUSD := make(map[string]float64)
	usdtAddrStr := "0x55d398326f99059fF775485246999027B3197955"
	if config.AppConfig != nil && common.IsHexAddress(config.AppConfig.USDTAddress) {
		usdtAddrStr = config.AppConfig.USDTAddress
	}
	if common.IsHexAddress(usdtAddrStr) {
		usdtAddr := common.HexToAddress(usdtAddrStr)
		if usdtBal := s.getWalletTokenBalance("bsc", usdtAddr, walletAddr); usdtBal != nil && usdtBal.Sign() > 0 {
			meta := s.getTokenMeta("bsc", usdtAddr)
			prices, _ := s.priceService.GetUSDPrices("bsc", []string{usdtAddr.Hex()})
			price := prices[strings.ToLower(usdtAddr.Hex())]
			usd := toFloat(usdtBal, meta.decimals) * price
			if usd > 0 {
				extraWalletTokenUSD["bsc|"+strings.ToLower(usdtAddr.Hex())] = usd
			}
		}
	}

	resp := &RealtimePositionsResponse{
		Wallet: RealtimeWallet{
			Address:       walletAddr.Hex(),
			BNBBalance:    formatUnits(bnbBalWei, 18, 6),
			BNBBalanceWEI: bnbBalWei.String(),
			BNBPriceUSD:   bnbPriceUSD,
			BNBUSD:        bnbUSD,
		},
		PollIntervalSec: 1,
		UpdatedAt:       time.Now(),
	}

	taskByV3Token := make(map[string]models.StrategyTask)
	taskByV3TokenID := make(map[string]models.StrategyTask)
	taskByV4Token := make(map[string]models.StrategyTask)
	pendingV3Tasks := make([]models.StrategyTask, 0, 4)
	pendingV4Tasks := make([]models.StrategyTask, 0, 2)
	v3NPMSet := make(map[common.Address]struct{})

	addV3NPM := func(addrStr string) {
		addrStr = strings.TrimSpace(addrStr)
		if !common.IsHexAddress(addrStr) {
			return
		}
		addr := common.HexToAddress(addrStr)
		if addr == (common.Address{}) {
			return
		}
		v3NPMSet[addr] = struct{}{}
	}

	resolveTaskNPM := func(t models.StrategyTask) string {
		if common.IsHexAddress(t.V3PositionManagerAddress) {
			return strings.TrimSpace(t.V3PositionManagerAddress)
		}
		chain := config.NormalizeChain(t.Chain)
		if chain == "" {
			chain = "bsc"
		}
		ex := strings.ToLower(strings.TrimSpace(t.Exchange))

		if config.AppConfig != nil {
			// Prefer chain-scoped deployments when available.
			if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
				if ex != "" {
					for _, dep := range cc.V3Deployments {
						if !common.IsHexAddress(dep.PositionManagerAddress) {
							continue
						}
						name := strings.ToLower(strings.TrimSpace(dep.Name))
						if name == "" {
							continue
						}
						// Heuristic: match common DEX names.
						if strings.Contains(ex, "pancake") && strings.Contains(name, "pancake") {
							return strings.TrimSpace(dep.PositionManagerAddress)
						}
						if strings.Contains(ex, "uniswap") && strings.Contains(name, "uniswap") {
							return strings.TrimSpace(dep.PositionManagerAddress)
						}
						if strings.Contains(ex, "aero") && (strings.Contains(name, "aero") || strings.Contains(name, "slipstream")) {
							return strings.TrimSpace(dep.PositionManagerAddress)
						}
					}
				}
				if common.IsHexAddress(cc.DefaultV3PositionManagerAddress) {
					return strings.TrimSpace(cc.DefaultV3PositionManagerAddress)
				}
				for _, dep := range cc.V3Deployments {
					if common.IsHexAddress(dep.PositionManagerAddress) {
						return strings.TrimSpace(dep.PositionManagerAddress)
					}
				}
			}

			// Legacy single-chain fallback (BSC).
			if chain == "bsc" {
				if strings.Contains(ex, "pancake") && common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
					return strings.TrimSpace(config.AppConfig.PancakeV3PositionManagerAddress)
				}
				if strings.Contains(ex, "uniswap") && common.IsHexAddress(config.AppConfig.UniswapV3PositionManagerAddress) {
					return strings.TrimSpace(config.AppConfig.UniswapV3PositionManagerAddress)
				}
			}
		}
		return ""
	}

	// Preload tasks for display enhancements (range %, out-of-range timer, etc.)
	var tasks []models.StrategyTask
	if err := database.DB.Where(
		"user_id = ? AND status = ? AND ((v3_token_id <> '' OR v3_position_manager_address <> '') OR rebalance_pending = ? OR exit_pending_action <> '')",
		userID,
		models.StrategyStatusRunning,
		true,
	).Find(&tasks).Error; err == nil {
		for _, t := range tasks {
			if strings.EqualFold(strings.TrimSpace(t.PoolVersion), "v4") {
				continue
			}
			chain := config.NormalizeChain(t.Chain)
			if chain == "" {
				chain = "bsc"
			}
			// V3 on-chain NFT scan is currently BSC-scoped; avoid mixing NPMs from other chains into the scan set.
			if chain == "bsc" {
				addV3NPM(resolveTaskNPM(t))
			}

			tokenKey := strings.TrimSpace(t.V3TokenID)
			if tokenKey == "" {
				if t.RebalancePending || strings.TrimSpace(t.ExitPendingAction) != "" {
					pendingV3Tasks = append(pendingV3Tasks, t)
				}
				continue
			}
			key := tokenKey + "|" + chain + "|" + strings.ToLower(strings.TrimSpace(resolveTaskNPM(t)))
			taskByV3Token[key] = t
			byIDKey := chain + "|" + tokenKey
			if prev, ok := taskByV3TokenID[byIDKey]; !ok || t.UpdatedAt.After(prev.UpdatedAt) {
				taskByV3TokenID[byIDKey] = t
			}
		}
	} else {
		resp.Warnings = append(resp.Warnings, "加载 V3 任务信息失败，部分仓位增强信息可能不完整")
	}

	var v4Tasks []models.StrategyTask
	if err := database.DB.Where(
		"user_id = ? AND status = ? AND (v4_token_id <> '' OR rebalance_pending = ? OR exit_pending_action <> '')",
		userID,
		models.StrategyStatusRunning,
		true,
	).Find(&v4Tasks).Error; err == nil {
		for _, t := range v4Tasks {
			if !strings.EqualFold(strings.TrimSpace(t.PoolVersion), "v4") {
				continue
			}
			key := strings.TrimSpace(t.V4TokenID)
			if key == "" {
				if t.RebalancePending || strings.TrimSpace(t.ExitPendingAction) != "" {
					pendingV4Tasks = append(pendingV4Tasks, t)
				}
				continue
			}
			taskByV4Token[key] = t
		}
	}

	positions := make([]RealtimePosition, 0, 8)
	seenV3 := make(map[string]struct{})

	// 1) V3 positions via NPM balanceOf/tokenOfOwnerByIndex (optional; can be heavy).
	if config.AppConfig.RealtimeV3NFTScan && config.AppConfig.RealtimeV3NFTScanMax > 0 {
		addV3NPM(config.AppConfig.PancakeV3PositionManagerAddress)
		addV3NPM(config.AppConfig.UniswapV3PositionManagerAddress)

		npmAddrs := make([]common.Address, 0, len(v3NPMSet))
		for addr := range v3NPMSet {
			npmAddrs = append(npmAddrs, addr)
		}
		sort.Slice(npmAddrs, func(i, j int) bool {
			return strings.ToLower(npmAddrs[i].Hex()) < strings.ToLower(npmAddrs[j].Hex())
		})

		for _, npmAddr := range npmAddrs {
			pm, err := blockchain.NewV3PositionManager(npmAddr, blockchain.Client)
			if err != nil {
				resp.Warnings = append(resp.Warnings, fmt.Sprintf("创建 V3 PositionManager 失败: %s", npmAddr.Hex()))
				continue
			}

			bal, err := pm.BalanceOf(nil, walletAddr)
			if err != nil {
				resp.Warnings = append(resp.Warnings, fmt.Sprintf("read V3 balance failed: pm=%s err=%v", npmAddr.Hex(), err))
				continue
			}
			if bal == nil || bal.Sign() == 0 {
				continue
			}
			max := bal.Int64()
			if max > int64(config.AppConfig.RealtimeV3NFTScanMax) {
				max = int64(config.AppConfig.RealtimeV3NFTScanMax)
			}
			tokenIDs := make([]*big.Int, 0, int(max))
			var tokenIDsRawMu sync.Mutex
			tokenIDsRaw := make([]*big.Int, 0, int(max))

			// Parallelize tokenOfOwnerByIndex() calls to reduce cold-load latency.
			idGroup, _ := errgroup.WithContext(context.Background())
			idGroup.SetLimit(8)
			for i := int64(0); i < max; i++ {
				idx := i
				idGroup.Go(func() error {
					tokenId, err := pm.TokenOfOwnerByIndex(nil, walletAddr, big.NewInt(idx))
					if err != nil || tokenId == nil || tokenId.Sign() == 0 {
						return nil
					}
					tokenIDsRawMu.Lock()
					tokenIDsRaw = append(tokenIDsRaw, new(big.Int).Set(tokenId))
					tokenIDsRawMu.Unlock()
					return nil
				})
			}
			_ = idGroup.Wait()

			for _, tokenId := range tokenIDsRaw {
				seenKey := strings.TrimSpace(tokenId.String()) + "|bsc|" + strings.ToLower(npmAddr.Hex())
				if _, ok := seenV3[seenKey]; ok {
					continue
				}
				seenV3[seenKey] = struct{}{}
				tokenIDs = append(tokenIDs, tokenId)
			}

			var positionsMu sync.Mutex
			g, _ := errgroup.WithContext(context.Background())
			g.SetLimit(6)
			for _, tokenId := range tokenIDs {
				tid := new(big.Int).Set(tokenId)
				g.Go(func() error {
					p, warn := s.buildV3Position("bsc", pm, npmAddr, walletAddr, tid, taskByV3Token, taskByV3TokenID)
					positionsMu.Lock()
					defer positionsMu.Unlock()
					if warn != "" {
						resp.Warnings = append(resp.Warnings, warn)
					}
					if p != nil {
						positions = append(positions, *p)
					}
					return nil
				})
			}
			_ = g.Wait()
		}
	}

	// 1b) V3 positions referenced by DB tasks (covers staked NFTs / non-enumerable NPMs).
	if len(taskByV3Token) > 0 {
		pmCache := make(map[string]*blockchain.V3PositionManager)
		var mu sync.Mutex

		taskKeys := make([]string, 0, len(taskByV3Token))
		for k := range taskByV3Token {
			taskKeys = append(taskKeys, k)
		}
		sort.Strings(taskKeys)

		g, _ := errgroup.WithContext(context.Background())
		g.SetLimit(6)
		for _, k := range taskKeys {
			task := taskByV3Token[k]
			chain := config.NormalizeChain(task.Chain)
			if chain == "" {
				chain = "bsc"
			}
			tokenStr := strings.TrimSpace(task.V3TokenID)
			npmStr := resolveTaskNPM(task)
			if tokenStr == "" || tokenStr == "0" || !common.IsHexAddress(npmStr) {
				continue
			}
			npmAddr := common.HexToAddress(npmStr)
			seenKey := tokenStr + "|" + chain + "|" + strings.ToLower(npmAddr.Hex())
			if _, ok := seenV3[seenKey]; ok {
				continue
			}
			seenV3[seenKey] = struct{}{}

			tid, parseErr := convert.ParseBigInt(tokenStr)
			if parseErr != nil || tid == nil || tid.Sign() == 0 {
				continue
			}
			tid = new(big.Int).Set(tid)

			g.Go(func() error {
				key := chain + "|" + strings.ToLower(npmAddr.Hex())

				mu.Lock()
				pm := pmCache[key]
				mu.Unlock()

				if pm == nil {
					client, _, err := blockchain.GetEVMClient(chain)
					if err != nil || client == nil {
						mu.Lock()
						resp.Warnings = append(resp.Warnings, fmt.Sprintf("V3 RPC not configured for chain=%s (npm=%s)", chain, npmAddr.Hex()))
						mu.Unlock()
						return nil
					}
					created, err := blockchain.NewV3PositionManager(npmAddr, client)
					if err != nil {
						mu.Lock()
						resp.Warnings = append(resp.Warnings, fmt.Sprintf("创建 V3 PositionManager 失败: %s", npmAddr.Hex()))
						mu.Unlock()
						return nil
					}
					mu.Lock()
					pmCache[key] = created
					pm = created
					mu.Unlock()
				}

				p, warn := s.buildV3Position(chain, pm, npmAddr, walletAddr, tid, taskByV3Token, taskByV3TokenID)

				mu.Lock()
				defer mu.Unlock()
				if warn != "" {
					resp.Warnings = append(resp.Warnings, warn)
				}
				if p != nil {
					positions = append(positions, *p)
				}
				return nil
			})
		}
		_ = g.Wait()
	}

	// 2) V4 positions (best-effort): prefer DB tasks because on-chain tokenId->range mapping is not always enumerable.
	ownedV4 := make(map[string]struct{})
	if len(taskByV4Token) > 0 && config.AppConfig.V4NFTScanFromBlock > 0 && common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		ids, err := s.getV4OwnedTokenIDs(walletAddr)
		if err != nil {
			log.Printf("[Realtime] V4 NFT scan failed: wallet=%s err=%v", walletAddr.Hex(), err)
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("V4 NFT scan failed: %v", err))
		} else {
			for _, id := range ids {
				ownedV4[id] = struct{}{}
			}
		}
	}
	for tokenId, task := range taskByV4Token {
		if len(ownedV4) > 0 {
			if _, ok := ownedV4[tokenId]; !ok {
				continue
			}
		}
		p, warn := s.buildV4Position(walletAddr, tokenId, &task)
		if warn != "" {
			resp.Warnings = append(resp.Warnings, warn)
		}
		if p != nil {
			positions = append(positions, *p)
		}
	}

	// 3) Pending tasks without tokenId (rebalance/exit in progress): show placeholder cards in miniapp.
	for i := range pendingV3Tasks {
		task := pendingV3Tasks[i]
		p, warn := s.buildPendingTaskPosition(walletAddr, &task)
		if warn != "" {
			resp.Warnings = append(resp.Warnings, warn)
		}
		if p != nil {
			positions = append(positions, *p)
		}
	}
	for i := range pendingV4Tasks {
		task := pendingV4Tasks[i]
		p, warn := s.buildPendingTaskPosition(walletAddr, &task)
		if warn != "" {
			resp.Warnings = append(resp.Warnings, warn)
		}
		if p != nil {
			positions = append(positions, *p)
		}
	}

	sortRealtimePositions(positions)
	resp.Positions = positions

	// Build a summary across all positions (dedupe wallet balances by token address).
	summary := RealtimeSummary{}
	walletTokenUSD := make(map[string]float64)
	for _, p := range positions {
		summary.PositionUSD += p.Totals.PositionUSD
		summary.FeeUSD += p.Totals.FeeUSD
		for _, row := range p.TokenRows {
			addr := strings.ToLower(strings.TrimSpace(row.Address))
			if addr == "" {
				continue
			}
			if common.IsHexAddress(addr) && common.HexToAddress(addr) == (common.Address{}) {
				continue
			}
			chainKey := strings.ToLower(strings.TrimSpace(p.Chain))
			if chainKey == "" {
				chainKey = "bsc"
			}
			key := chainKey + "|" + addr
			if prev, ok := walletTokenUSD[key]; !ok || row.WalletUSD > prev {
				walletTokenUSD[key] = row.WalletUSD
			}
		}
	}
	for key, usd := range extraWalletTokenUSD {
		if usd <= 0 {
			continue
		}
		if prev, ok := walletTokenUSD[key]; !ok || usd > prev {
			walletTokenUSD[key] = usd
		}
	}
	for _, v := range walletTokenUSD {
		summary.WalletUSD += v
	}
	summary.WalletUSD += bnbUSD
	summary.TotalUSD = summary.WalletUSD + summary.PositionUSD + summary.FeeUSD
	resp.Summary = summary

	if took := time.Since(start); took > 1200*time.Millisecond {
		log.Printf("[Realtime] user=%d positions=%d took=%s warnings=%d", userID, len(resp.Positions), took.Truncate(10*time.Millisecond), len(resp.Warnings))
	}
	return resp, nil
}

func sortRealtimePositions(positions []RealtimePosition) {
	sort.SliceStable(positions, func(i, j int) bool {
		return realtimePositionLess(positions[i], positions[j])
	})
}

func realtimePositionLess(pi, pj RealtimePosition) bool {
	if pi.RunningSince != nil && pj.RunningSince != nil && !pi.RunningSince.Equal(*pj.RunningSince) {
		return pi.RunningSince.Before(*pj.RunningSince)
	}
	if pi.RunningSince != nil && pj.RunningSince == nil {
		return true
	}
	if pi.RunningSince == nil && pj.RunningSince != nil {
		return false
	}

	if pi.Title != pj.Title {
		return pi.Title < pj.Title
	}

	// Keep UI order stable across refreshes when creation time is identical or unavailable.
	poolI := strings.ToLower(strings.TrimSpace(pi.PoolID))
	poolJ := strings.ToLower(strings.TrimSpace(pj.PoolID))
	if poolI != poolJ {
		return poolI < poolJ
	}

	if c := compareDecimalStrings(pi.PositionID, pj.PositionID); c != 0 {
		return c < 0
	}
	if pi.TaskID != pj.TaskID {
		return pi.TaskID < pj.TaskID
	}
	if pi.Version != pj.Version {
		return pi.Version < pj.Version
	}
	return strings.ToLower(strings.TrimSpace(pi.Exchange)) < strings.ToLower(strings.TrimSpace(pj.Exchange))
}

func compareDecimalStrings(a, b string) int {
	aRaw := strings.TrimSpace(a)
	bRaw := strings.TrimSpace(b)
	if aRaw == bRaw {
		return 0
	}

	aNorm, okA := normalizeDecimalString(aRaw)
	bNorm, okB := normalizeDecimalString(bRaw)
	if !okA || !okB {
		return strings.Compare(aRaw, bRaw)
	}

	if len(aNorm) != len(bNorm) {
		if len(aNorm) < len(bNorm) {
			return -1
		}
		return 1
	}
	return strings.Compare(aNorm, bNorm)
}

func normalizeDecimalString(s string) (string, bool) {
	if s == "" {
		return "0", true
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return "", false
		}
	}
	s = strings.TrimLeft(s, "0")
	if s == "" {
		s = "0"
	}
	return s, true
}

func (s *RealtimePositionsService) getV4OwnedTokenIDs(wallet common.Address) ([]string, error) {
	if config.AppConfig == nil || config.AppConfig.V4NFTScanFromBlock == 0 {
		return nil, nil
	}
	if !common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		return nil, nil
	}

	contract := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
	return scanERC721OwnedTokenIDs(contract, wallet, config.AppConfig.V4NFTScanFromBlock)
}

func scanERC721OwnedTokenIDs(contract common.Address, wallet common.Address, fromBlock uint64) ([]string, error) {
	if blockchain.Client == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	latest, err := blockchain.Client.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	if fromBlock == 0 || fromBlock > latest {
		return []string{}, nil
	}

	transferSig := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	walletTopic := common.BytesToHash(wallet.Bytes())

	const chunkSize = uint64(100_000)

	var allLogs []types.Log
	for start := fromBlock; start <= latest; start += chunkSize {
		end := start + chunkSize - 1
		if end > latest {
			end = latest
		}

		fromBI := new(big.Int).SetUint64(start)
		toBI := new(big.Int).SetUint64(end)

		// Incoming: to == wallet
		inQuery := ethereum.FilterQuery{
			FromBlock: fromBI,
			ToBlock:   toBI,
			Addresses: []common.Address{contract},
			Topics:    [][]common.Hash{{transferSig}, nil, {walletTopic}},
		}
		inLogs, err := blockchain.Client.FilterLogs(ctx, inQuery)
		if err != nil {
			return nil, err
		}
		allLogs = append(allLogs, inLogs...)

		// Outgoing: from == wallet
		outQuery := ethereum.FilterQuery{
			FromBlock: fromBI,
			ToBlock:   toBI,
			Addresses: []common.Address{contract},
			Topics:    [][]common.Hash{{transferSig}, {walletTopic}},
		}
		outLogs, err := blockchain.Client.FilterLogs(ctx, outQuery)
		if err != nil {
			return nil, err
		}
		allLogs = append(allLogs, outLogs...)
	}

	sort.Slice(allLogs, func(i, j int) bool {
		if allLogs[i].BlockNumber != allLogs[j].BlockNumber {
			return allLogs[i].BlockNumber < allLogs[j].BlockNumber
		}
		return allLogs[i].Index < allLogs[j].Index
	})

	owned := make(map[string]struct{})
	for _, lg := range allLogs {
		if len(lg.Topics) < 4 {
			continue
		}
		from := common.BytesToAddress(lg.Topics[1].Bytes()[12:])
		to := common.BytesToAddress(lg.Topics[2].Bytes()[12:])
		tokenId := new(big.Int).SetBytes(lg.Topics[3].Bytes()).String()

		if to == wallet {
			owned[tokenId] = struct{}{}
		}
		if from == wallet {
			delete(owned, tokenId)
		}
	}

	ids := make([]string, 0, len(owned))
	for id := range owned {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func (s *RealtimePositionsService) buildV3Position(
	chain string,
	pm *blockchain.V3PositionManager,
	npmAddr common.Address,
	walletAddr common.Address,
	tokenId *big.Int,
	taskByV3Token map[string]models.StrategyTask,
	taskByV3TokenID map[string]models.StrategyTask,
) (*RealtimePosition, string) {
	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}
	var warn string
	info, err := pm.Positions(nil, tokenId)
	if err != nil || info == nil {
		if err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "invalid token id") {
				return nil, ""
			}
		}
		return nil, fmt.Sprintf("read V3 positions() failed: chain=%s npm=%s tokenId=%s err=%v", chain, npmAddr.Hex(), tokenId.String(), err)
	}

	token0 := info.Token0
	token1 := info.Token1
	tickLower := info.TickLower
	tickUpper := info.TickUpper
	liq := info.Liquidity
	owed0 := big.NewInt(0)
	owed1 := big.NewInt(0)
	fee := float64(info.Fee) / 10000.0

	// Ignore empty positions (NFT not burned but liquidity already removed).
	// This reduces RPC calls and avoids noisy warnings for "no position" wallets.
	if liq == nil || liq.Sign() == 0 {
		return nil, ""
	}

	meta0 := s.getTokenMeta(chain, token0)
	meta1 := s.getTokenMeta(chain, token1)

	// Try find matching task for richer fields.
	taskKey := strings.TrimSpace(tokenId.String()) + "|" + chain + "|" + strings.ToLower(strings.TrimSpace(npmAddr.Hex()))
	var task *models.StrategyTask
	if t, ok := taskByV3Token[taskKey]; ok {
		task = &t
	} else if t, ok := taskByV3TokenID[chain+"|"+strings.TrimSpace(tokenId.String())]; ok {
		task = &t
	}

	poolID := ""
	exchange := "V3"
	rangePct := 0.0
	outOfRangeText := "0/0"
	var runningSince *time.Time
	statusLabel := "运行中"
	if task != nil {
		poolID = strings.TrimSpace(task.PoolId)
		exchange = strings.TrimSpace(task.Exchange)
		if task.RangeLowerPercentage > 0 && task.RangeUpperPercentage > 0 {
			rangePct = (task.RangeLowerPercentage + task.RangeUpperPercentage) / 2.0
		} else if task.RangePercentage > 0 {
			rangePct = task.RangePercentage
		}
		runningSince = &task.CreatedAt
		statusLabel = statusLabelFromTask(task)
	}

	// Resolve the V3 pool address from factory to avoid mismatches / stale DB pool IDs.
	poolAddr := common.Address{}
	if resolved, _, rErr := s.getV3PoolAddress(chain, npmAddr, token0, token1, info.Fee); rErr == nil && resolved != (common.Address{}) {
		poolAddr = resolved
		poolID = resolved.Hex()
	} else if poolID != "" && common.IsHexAddress(poolID) {
		poolAddr = common.HexToAddress(poolID)
	}

	// Best-effort exchange label when tasks don't carry it.
	if exchange == "V3" || strings.TrimSpace(exchange) == "" {
		if config.AppConfig != nil && common.IsHexAddress(config.AppConfig.PancakeV3PositionManagerAddress) && npmAddr == common.HexToAddress(config.AppConfig.PancakeV3PositionManagerAddress) {
			exchange = "PancakeSwap V3"
		} else if config.AppConfig != nil && common.IsHexAddress(config.AppConfig.UniswapV3PositionManagerAddress) && npmAddr == common.HexToAddress(config.AppConfig.UniswapV3PositionManagerAddress) {
			exchange = "Uniswap V3"
		}
	}

	snapshotBlock := uint64(0)
	var snapshotClient *ethclient.Client
	if poolAddr != (common.Address{}) {
		client, _, clientErr := blockchain.GetEVMClient(chain)
		if clientErr == nil {
			snapshotClient = client
			if blockNumber, blockErr := snapshotBlockNumber(client); blockErr == nil {
				snapshotBlock = blockNumber
				callOpts := &bind.CallOpts{BlockNumber: new(big.Int).SetUint64(snapshotBlock)}
				if snapshotInfo, posErr := pm.Positions(callOpts, tokenId); posErr == nil && snapshotInfo != nil {
					info = snapshotInfo
					token0 = info.Token0
					token1 = info.Token1
					tickLower = info.TickLower
					tickUpper = info.TickUpper
					liq = info.Liquidity
					fee = float64(info.Fee) / 10000.0
				} else if posErr != nil {
					log.Printf("[Realtime] V3 snapshot position read failed: chain=%s tokenId=%s err=%v", chain, tokenId.String(), posErr)
				}
			}
		}
	}

	// Get tick and sqrtP (poolID required). If missing, we still return a card but tick/amounts may be 0.
	currentTick := 0
	var sqrtP *big.Int
	hasSlot0 := false
	if poolAddr != (common.Address{}) {
		if snapshotBlock > 0 && snapshotClient != nil {
			sp, t, slotErr := blockchain.GetV3PoolSlot0AtBlockWithClient(snapshotClient, poolAddr, snapshotBlock)
			if slotErr == nil && sp != nil {
				sqrtP = sp
				currentTick = t
				hasSlot0 = true
			}
		}
		if !hasSlot0 {
			sp, t, _, _, err := s.getV3Slot0(chain, poolAddr)
			if err != nil && sp == nil {
				warn = fmt.Sprintf("read V3 pool slot0 failed: pool=%s tokenId=%s err=%v", poolAddr.Hex(), tokenId.String(), err)
			} else {
				sqrtP = sp
				currentTick = t
				hasSlot0 = true
			}
		}
	} else {
		warn = fmt.Sprintf("missing V3 pool address for tokenId=%s, tick/position amounts unavailable", tokenId.String())
	}

	inRange := currentTick >= tickLower && currentTick <= tickUpper
	if sqrtP == nil {
		sqrtP = big.NewInt(0)
	}
	if task != nil {
		outOfRangeText = formatOutOfRange(task, tickLower, tickUpper, currentTick)
	}

	// Prefer actual tick-based range so that rangePct reflects the CURRENT
	// position, not the (possibly just-updated) rebalance target stored on the task.
	if currentTick != 0 {
		if estimated := estimateRangePercent(currentTick, tickLower, tickUpper); estimated > 0 {
			rangePct = estimated
		}
	}

	if hasSlot0 && poolAddr != (common.Address{}) {
		if snapshotBlock > 0 {
			fee0, fee1, feeErr := pool.CalcV3UnclaimedFeesAtBlock(poolAddr, currentTick, info, snapshotBlock)
			if feeErr == nil && fee0 != nil && fee1 != nil {
				owed0 = fee0
				owed1 = fee1
			} else if feeErr != nil {
				logRealtimeFeeIssue("V3", "snapshot", tokenId.String(), feeErr)
				warn = appendRealtimeWarning(warn, fmt.Sprintf("V3 snapshot fee calculation failed: tokenId=%s err=%v", tokenId.String(), feeErr))
			}
		} else {
			fee0, fee1, _, _, feeErr := s.calcV3UnclaimedFeesLive(chain, poolAddr, currentTick, info)
			if feeErr != nil && (fee0 == nil || fee1 == nil) {
				logRealtimeFeeIssue("V3", "live", tokenId.String(), feeErr)
				warn = appendRealtimeWarning(warn, fmt.Sprintf("V3 fee calculation failed: tokenId=%s err=%v", tokenId.String(), feeErr))
			}
			if fee0 != nil && fee1 != nil {
				owed0 = fee0
				owed1 = fee1
			}
		}
	}

	// Compute amounts in position
	sqrtA, _ := pool.SqrtRatioAtTick(int32(tickLower))
	sqrtB, _ := pool.SqrtRatioAtTick(int32(tickUpper))
	amt0Raw, amt1Raw := pool.AmountsForLiquidity(sqrtP, sqrtA, sqrtB, liq)

	// Wallet balances
	w0 := s.getWalletTokenBalance(chain, token0, walletAddr)
	w1 := s.getWalletTokenBalance(chain, token1, walletAddr)

	// Prices
	prices, _ := s.priceService.GetUSDPrices(chain, []string{token0.Hex(), token1.Hex()})
	price0 := prices[strings.ToLower(token0.Hex())]
	price1 := prices[strings.ToLower(token1.Hex())]
	meta0 = s.getTokenMeta(chain, token0)
	meta1 = s.getTokenMeta(chain, token1)

	row0 := buildTokenRow(token0, meta0, price0, w0, amt0Raw, owed0)
	row1 := buildTokenRow(token1, meta1, price1, w1, amt1Raw, owed1)

	totals := RealtimeTotals{
		WalletUSD:   row0.WalletUSD + row1.WalletUSD,
		PositionUSD: row0.PositionUSD + row1.PositionUSD,
		FeeUSD:      row0.FeeUSD + row1.FeeUSD,
	}
	totals.TotalUSD = totals.WalletUSD + totals.PositionUSD + totals.FeeUSD

	title := fmt.Sprintf("%s-%s-%s-%.4f%%", exchangeShort(exchange, "UniV3"), row0.Symbol, row1.Symbol, fee)
	if strings.TrimSpace(exchange) == "" {
		exchange = "V3"
	}

	hasLiquidity := liq != nil && liq.Sign() > 0
	taskID := uint(0)
	taskPaused := false
	taskRebalanceEnabled := true
	taskMode := ""
	initialCostUSD := 0.0
	netInvestedUSD := 0.0
	currentValueUSD := 0.0
	absolutePnLUSD := 0.0
	hasPnL := false
	pnlMetrics := taskPnLViewMetrics{}
	taskRangeLowerPct := 0.0
	taskRangeUpperPct := 0.0
	taskSlippageTolerance := 0.0
	if task != nil {
		taskID = task.ID
		taskPaused = task.Paused
		taskRebalanceEnabled = models.RebalanceEnabledForOutOfRangeMode(models.ResolveStrategyOutOfRangeMode(task))
		taskMode = models.EffectiveStrategyTaskMode(task)
		pnlMetrics = s.getTaskPnLViewMetrics(task)
		taskSlippageTolerance = task.SlippageTolerance
		initialCostUSD = pnlMetrics.initialCost
		netInvestedUSD = pnlMetrics.netInvested
		currentValueUSD = pnlMetrics.currentValue
		absolutePnLUSD = pnlMetrics.absolutePnL
		hasPnL = pnlMetrics.hasPnL

		lowerPct := task.RangeLowerPercentage
		upperPct := task.RangeUpperPercentage
		if lowerPct <= 0 || upperPct <= 0 {
			if task.RangePercentage > 0 {
				lowerPct = task.RangePercentage
				upperPct = task.RangePercentage
			}
		}
		if lowerPct > 0 && upperPct > 0 {
			stableLower, stableUpper := pricing.StablePercentagesFromTickPercentages(task, lowerPct, upperPct)
			if stableLower > 0 && stableUpper > 0 {
				taskRangeLowerPct = stableLower
				taskRangeUpperPct = stableUpper
			} else {
				taskRangeLowerPct = lowerPct
				taskRangeUpperPct = upperPct
			}
		}
	}
	return &RealtimePosition{
		Chain:      chain,
		Version:    "v3",
		Exchange:   exchange,
		Title:      title,
		FeeTier:    uint64(info.Fee),
		PoolID:     poolID,
		PositionID: tokenId.String(),
		WalletID: func() uint {
			if task != nil {
				return task.WalletID
			}
			return 0
		}(),
		WalletAddress: func() string {
			if task != nil && strings.TrimSpace(task.WalletAddress) != "" {
				return strings.TrimSpace(task.WalletAddress)
			}
			return walletAddr.Hex()
		}(),
		TaskID:                taskID,
		TaskPaused:            taskPaused,
		TaskRebalanceEnabled:  taskRebalanceEnabled,
		TaskMode:              taskMode,
		TaskAmountUSDT:        displayTaskAmountUSDTWithMetrics(task, pnlMetrics),
		TaskSlippageTolerance: taskSlippageTolerance,
		StatusLabel:           statusLabel,
		InRange:               inRange,
		CurrentTick:           currentTick,
		TickLower:             tickLower,
		TickUpper:             tickUpper,
		TickSpacing: func() int {
			if task != nil && task.TickSpacing > 0 {
				return task.TickSpacing
			}
			return tickSpacingFromFee(info.Fee)
		}(),
		RangePercent:      rangePct,
		TaskRangeLowerPct: taskRangeLowerPct,
		TaskRangeUpperPct: taskRangeUpperPct,
		OutOfRange:        outOfRangeText,
		RunningSince:      runningSince,
		HasLiquidity:      hasLiquidity,
		InitialCostUSD:    initialCostUSD,
		NetInvestedUSD:    netInvestedUSD,
		CurrentValueUSD:   currentValueUSD,
		AbsolutePnLUSD:    absolutePnLUSD,
		HasPnL:            hasPnL,
		DCA:               buildRealtimeDCAStatus(task),
		TokenRows:         []RealtimeTokenRow{row0, row1},
		Totals:            totals,
	}, warn
}

// getTaskActualInvested returns the actual invested amount from the latest open trade record.
func getTaskActualInvested(task *models.StrategyTask) (float64, bool) {
	if task == nil || task.ID == 0 || database.DB == nil {
		return 0, false
	}
	var rec models.TradeRecord
	err := database.DB.
		Where("user_id = ? AND task_id = ? AND status = ?", task.UserID, task.ID, models.TradeStatusOpen).
		Order("opened_at DESC").
		First(&rec).Error
	if err != nil {
		return 0, false
	}
	amountWei, err := convert.ParseBigInt(rec.OpenUSDTSpent)
	if err != nil || amountWei == nil || amountWei.Sign() <= 0 {
		return 0, false
	}
	f := new(big.Float).SetInt(amountWei)
	div := new(big.Float).SetFloat64(math.Pow(10, 18))
	f.Quo(f, div)
	val, _ := f.Float64()
	if val <= 0 {
		return 0, false
	}
	return val, true
}

func displayTaskAmountUSDT(task *models.StrategyTask) float64 {
	return displayTaskAmountUSDTWithMetrics(task, taskPnLViewMetrics{})
}

func displayTaskAmountUSDTWithMetrics(task *models.StrategyTask, metrics taskPnLViewMetrics) float64 {
	if task == nil {
		return 0
	}
	if metrics.netInvested > 0 || metrics.recovered > 0 {
		return metrics.netInvested
	}
	if task.DCAEnabled && task.DCATotalAmountUSDT > task.AmountUSDT {
		return task.DCATotalAmountUSDT
	}
	if task.AmountUSDT > 0 {
		return task.AmountUSDT
	}
	return 0
}

func buildRealtimeDCAStatus(task *models.StrategyTask) *RealtimeDCAStatus {
	if task == nil || !task.DCAEnabled {
		return nil
	}

	status := &RealtimeDCAStatus{
		Enabled:       true,
		ExecutedCount: task.DCAExecutedCount,
		RetryCount:    task.DCARetryCount,
		NextBatchAt:   task.DCANextBatchAt,
	}

	pcts, ok := strategy.ParseDCAPercentages(task.DCAPercentagesJSON)
	if !ok {
		return status
	}

	status.PlanValid = true
	status.TotalCount = len(pcts)
	status.Pending = task.DCAExecutedCount < status.TotalCount && task.DCANextBatchAt != nil
	status.Completed = task.DCAExecutedCount >= status.TotalCount && task.DCANextBatchAt == nil
	status.Canceled = task.DCAExecutedCount > 0 && task.DCAExecutedCount < status.TotalCount && task.DCANextBatchAt == nil
	status.Finished = status.Completed || status.Canceled
	return status
}

type taskPnLViewMetrics struct {
	initialCost  float64
	netInvested  float64
	recovered    float64
	currentValue float64
	absolutePnL  float64
	hasPnL       bool
	dustTracked  bool
}

func finalizeTaskPnLViewMetrics(metrics taskPnLViewMetrics, fallback float64) taskPnLViewMetrics {
	if metrics.initialCost <= 0 {
		metrics.initialCost = fallback
	}
	if metrics.netInvested < 0 {
		metrics.netInvested = 0
	}
	if metrics.netInvested <= 0 {
		switch {
		case metrics.initialCost > 0 && metrics.currentValue > 0 && metrics.recovered <= 0:
			// Display guard: if dust bookkeeping temporarily collapses netInvested to zero,
			// avoid showing the full position value as pure profit right after opening.
			metrics.netInvested = metrics.initialCost
		case !metrics.dustTracked && metrics.recovered <= 0:
			metrics.netInvested = metrics.initialCost
		}
	}
	if metrics.netInvested > 0 {
		metrics.absolutePnL = metrics.currentValue - metrics.netInvested
		metrics.hasPnL = true
	}
	return metrics
}

// getTaskPnLViewMetrics uses the exact same PnL service as bot task cards.
// It provides invested/current/PnL values for miniapp with unified semantics.
func (s *RealtimePositionsService) getTaskPnLViewMetrics(task *models.StrategyTask) taskPnLViewMetrics {
	metrics := taskPnLViewMetrics{}
	if task == nil || task.AmountUSDT <= 0 {
		return metrics
	}

	fallback := task.AmountUSDT
	if actual, ok := getTaskActualInvested(task); ok && actual > 0 {
		fallback = actual
	}

	metrics.initialCost = fallback
	metrics.netInvested = fallback

	if s != nil && s.pnlService != nil {
		if pnl, err := s.pnlService.GetTaskPnL(task); err == nil && pnl != nil {
			if pnl.InitialCostUSDT > 0 {
				metrics.initialCost = pnl.InitialCostUSDT
			}
			if pnl.NetInvestedUSDT > 0 || pnl.DustValueUSDT > 0 || pnl.RecoveredUSDT > 0 {
				metrics.netInvested = pnl.NetInvestedUSDT
				metrics.dustTracked = pnl.DustValueUSDT > 0
				metrics.recovered = pnl.RecoveredUSDT
			}
			if !math.IsNaN(pnl.CurrentValueUSDT) && !math.IsInf(pnl.CurrentValueUSDT, 0) {
				metrics.currentValue = pnl.CurrentValueUSDT
			}
			if !math.IsNaN(pnl.AbsolutePnLUSDT) && !math.IsInf(pnl.AbsolutePnLUSDT, 0) {
				metrics.absolutePnL = pnl.AbsolutePnLUSDT
				metrics.hasPnL = true
			}
		}
	}

	return finalizeTaskPnLViewMetrics(metrics, fallback)
}

func (s *RealtimePositionsService) buildV4Position(walletAddr common.Address, tokenId string, task *models.StrategyTask) (*RealtimePosition, string) {
	if config.AppConfig == nil {
		return nil, "config not loaded"
	}
	if task == nil {
		return nil, ""
	}
	if !common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) || !common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) {
		return nil, "V4 config incomplete (UNISWAP_V4_POOL_MANAGER_ADDRESS/UNISWAP_V4_STATE_VIEW_ADDRESS)"
	}
	poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
	stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)

	c0, c1, currenciesReady := v4TaskCurrencies(task)
	var warn string
	liq := big.NewInt(0)
	if v, ok := new(big.Int).SetString(strings.TrimSpace(task.CurrentLiquidity), 10); ok && v != nil {
		liq = v
	}
	tickLower := task.TickLower
	tickUpper := task.TickUpper
	var v4pos *blockchain.V4PositionInfo
	snapshotBlock := uint64(0)

	if common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		tokenID, parseErr := convert.ParseBigInt(tokenId)
		if parseErr == nil && tokenID.Sign() > 0 {
			blockCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			latestBlock, blockErr := blockchain.Client.BlockNumber(blockCtx)
			cancel()
			if blockErr != nil {
				log.Printf("[Realtime] V4 snapshot block read failed: tokenId=%s err=%v", tokenId, blockErr)
			} else {
				snapshotBlock = latestBlock
			}
			v4pmAddr := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
			var pos *blockchain.V4PositionInfo
			var posErr error
			if snapshotBlock > 0 {
				pos, posErr = blockchain.GetV4PositionInfoAtBlock(v4pmAddr, poolManager, task.PoolId, tokenID, snapshotBlock)
			} else {
				pos, posErr = blockchain.GetV4PositionInfo(v4pmAddr, poolManager, task.PoolId, tokenID)
			}
			if posErr != nil {
				log.Printf("[Realtime] V4 position info read failed: tokenId=%s err=%v", tokenId, posErr)
			}
			if pos != nil {
				v4pos = pos
				if v4PositionHasCurrencies(pos) {
					c0 = pos.Token0
					c1 = pos.Token1
					currenciesReady = true
				}
				if pos.Liquidity != nil {
					liq = pos.Liquidity
				}
				if (pos.TickLower != 0 || pos.TickUpper != 0) && pos.TickLower < pos.TickUpper {
					tickLower = pos.TickLower
					tickUpper = pos.TickUpper
				}
			}
		}
	}

	if !currenciesReady {
		return nil, fmt.Sprintf("V4 tokenId=%s missing token0/token1 metadata", tokenId)
	}

	// Ignore empty positions (NFT not burned but liquidity already removed).
	if liq == nil || liq.Sign() == 0 {
		return nil, ""
	}

	var (
		sqrtP       *big.Int
		currentTick int
		err         error
	)
	if snapshotBlock > 0 {
		sqrtP, currentTick, err = blockchain.GetUniswapV4PoolSlot0ViaStateViewAtBlock(stateView, poolManager, task.PoolId, snapshotBlock)
	} else {
		sqrtP, currentTick, _, _, err = s.getV4Slot0(stateView, poolManager, task.PoolId)
	}
	if err != nil && sqrtP == nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "429") || strings.Contains(errMsg, "too many requests") || strings.Contains(errMsg, "rate limit") {
			return nil, fmt.Sprintf("读取 V4 slot0 失败：RPC 返回 429 或触发限流，请稍后重试并检查当前链 RPC 配置。tokenId=%s", tokenId)
		}
		return nil, fmt.Sprintf("读取 V4 slot0 失败: tokenId=%s err=%v", tokenId, err)
	}
	owed0 := big.NewInt(0)
	owed1 := big.NewInt(0)
	if v4pos != nil {
		if snapshotBlock > 0 {
			fee0, fee1, feeErr := pool.CalcV4UnclaimedFeesAtBlock(stateView, poolManager, task.PoolId, currentTick, v4pos, snapshotBlock)
			if feeErr == nil && fee0 != nil && fee1 != nil {
				owed0 = fee0
				owed1 = fee1
			} else if feeErr != nil {
				logRealtimeFeeIssue("V4", "snapshot", tokenId, feeErr)
				warn = appendRealtimeWarning(warn, fmt.Sprintf("V4 snapshot fee calculation failed: tokenId=%s err=%v", tokenId, feeErr))
			}
		} else if fee0, fee1, _, _, feeErr := s.calcV4UnclaimedFeesLiveUnified(stateView, poolManager, task.PoolId, currentTick, v4pos); fee0 != nil && fee1 != nil {
			owed0 = fee0
			owed1 = fee1
			if feeErr != nil {
				logRealtimeFeeIssue("V4", "live", tokenId, feeErr)
				warn = appendRealtimeWarning(warn, fmt.Sprintf("V4 fee calculation failed: tokenId=%s err=%v", tokenId, feeErr))
			}
		} else if feeErr != nil {
			logRealtimeFeeIssue("V4", "live", tokenId, feeErr)
			warn = appendRealtimeWarning(warn, fmt.Sprintf("V4 fee calculation failed: tokenId=%s err=%v", tokenId, feeErr))
		}
	}

	sqrtA, _ := pool.SqrtRatioAtTick(int32(tickLower))
	sqrtB, _ := pool.SqrtRatioAtTick(int32(tickUpper))
	amt0Raw, amt1Raw := pool.AmountsForLiquidity(sqrtP, sqrtA, sqrtB, liq)

	chain := config.NormalizeChain(task.Chain)
	if chain == "" {
		chain = "bsc"
	}

	w0 := s.getWalletV4CurrencyBalance(chain, c0, walletAddr)
	w1 := s.getWalletV4CurrencyBalance(chain, c1, walletAddr)

	meta0 := s.getV4CurrencyMeta(chain, c0)
	meta1 := s.getV4CurrencyMeta(chain, c1)

	prices := s.getV4CurrencyUSDPrices(chain, c0, c1)
	price0 := prices[c0]
	price1 := prices[c1]

	row0 := buildTokenRow(c0, meta0, price0, w0, amt0Raw, owed0)
	row1 := buildTokenRow(c1, meta1, price1, w1, amt1Raw, owed1)

	totals := RealtimeTotals{
		WalletUSD:   row0.WalletUSD + row1.WalletUSD,
		PositionUSD: row0.PositionUSD + row1.PositionUSD,
		FeeUSD:      row0.FeeUSD + row1.FeeUSD,
	}
	totals.TotalUSD = totals.WalletUSD + totals.PositionUSD + totals.FeeUSD

	inRange := currentTick >= tickLower && currentTick <= tickUpper
	// Prefer actual tick-based range so that rangePct reflects the CURRENT
	// position, not the (possibly just-updated) rebalance target stored on the task.
	rangePct := estimateRangePercent(currentTick, tickLower, tickUpper)
	if rangePct <= 0 {
		if task.RangeLowerPercentage > 0 && task.RangeUpperPercentage > 0 {
			rangePct = (task.RangeLowerPercentage + task.RangeUpperPercentage) / 2.0
		} else if task.RangePercentage > 0 {
			rangePct = task.RangePercentage
		}
	}

	exchange := strings.TrimSpace(task.Exchange)
	if exchange == "" {
		exchange = "Uniswap V4"
	}
	title := fmt.Sprintf("%s-%s-%s-%.4f%%", exchangeShort(exchange, "UniV4"), row0.Symbol, row1.Symbol, float64(task.Fee)/10000.0)

	hasLiquidity := liq != nil && liq.Sign() > 0
	taskRangeLowerPct := 0.0
	taskRangeUpperPct := 0.0

	lowerPct := task.RangeLowerPercentage
	upperPct := task.RangeUpperPercentage
	if lowerPct <= 0 || upperPct <= 0 {
		if task.RangePercentage > 0 {
			lowerPct = task.RangePercentage
			upperPct = task.RangePercentage
		}
	}
	if lowerPct > 0 && upperPct > 0 {
		stableLower, stableUpper := pricing.StablePercentagesFromTickPercentages(task, lowerPct, upperPct)
		if stableLower > 0 && stableUpper > 0 {
			taskRangeLowerPct = stableLower
			taskRangeUpperPct = stableUpper
		} else {
			taskRangeLowerPct = lowerPct
			taskRangeUpperPct = upperPct
		}
	}
	pnlMetrics := s.getTaskPnLViewMetrics(task)
	initialCostUSD := pnlMetrics.initialCost
	netInvestedUSD := pnlMetrics.netInvested
	currentValueUSD := pnlMetrics.currentValue
	absolutePnLUSD := pnlMetrics.absolutePnL
	hasPnL := pnlMetrics.hasPnL
	return &RealtimePosition{
		Chain:      chain,
		Version:    "v4",
		Exchange:   exchange,
		Title:      title,
		FeeTier:    uint64(task.Fee),
		PoolID:     strings.TrimSpace(task.PoolId),
		PositionID: tokenId,
		WalletID:   task.WalletID,
		WalletAddress: func() string {
			if strings.TrimSpace(task.WalletAddress) != "" {
				return strings.TrimSpace(task.WalletAddress)
			}
			return walletAddr.Hex()
		}(),
		TaskID:                task.ID,
		TaskPaused:            task.Paused,
		TaskRebalanceEnabled:  models.RebalanceEnabledForOutOfRangeMode(models.ResolveStrategyOutOfRangeMode(task)),
		TaskMode:              models.EffectiveStrategyTaskMode(task),
		TaskAmountUSDT:        displayTaskAmountUSDTWithMetrics(task, pnlMetrics),
		TaskSlippageTolerance: task.SlippageTolerance,
		StatusLabel:           statusLabelFromTask(task),
		InRange:               inRange,
		CurrentTick:           currentTick,
		TickLower:             tickLower,
		TickUpper:             tickUpper,
		TickSpacing:           task.TickSpacing,
		RangePercent:          rangePct,
		TaskRangeLowerPct:     taskRangeLowerPct,
		TaskRangeUpperPct:     taskRangeUpperPct,
		OutOfRange:            formatOutOfRange(task, tickLower, tickUpper, currentTick),
		RunningSince:          &task.CreatedAt,
		HasLiquidity:          hasLiquidity,
		InitialCostUSD:        initialCostUSD,
		NetInvestedUSD:        netInvestedUSD,
		CurrentValueUSD:       currentValueUSD,
		AbsolutePnLUSD:        absolutePnLUSD,
		HasPnL:                hasPnL,
		DCA:                   buildRealtimeDCAStatus(task),
		TokenRows:             []RealtimeTokenRow{row0, row1},
		Totals:                totals,
	}, warn
}

func (s *RealtimePositionsService) buildPendingTaskPosition(walletAddr common.Address, task *models.StrategyTask) (*RealtimePosition, string) {
	if task == nil {
		return nil, ""
	}

	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	if version == "" {
		version = "v3"
	}
	if version != "v4" {
		version = "v3"
	}

	chain := config.NormalizeChain(task.Chain)
	if chain == "" {
		chain = "bsc"
	}

	// Only show placeholders for tasks that are in some pending state and have no tokenId.
	if !task.RebalancePending && strings.TrimSpace(task.ExitPendingAction) == "" {
		return nil, ""
	}
	if version == "v4" {
		if tid := strings.TrimSpace(task.V4TokenID); tid != "" && tid != "0" {
			return nil, ""
		}
	} else {
		if tid := strings.TrimSpace(task.V3TokenID); tid != "" && tid != "0" {
			return nil, ""
		}
	}

	poolID := strings.TrimSpace(task.PoolId)
	tickLower := task.TickLower
	tickUpper := task.TickUpper

	currentTick := 0
	gotTick := false

	switch version {
	case "v4":
		if config.AppConfig != nil &&
			common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) &&
			common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) &&
			strings.TrimSpace(poolID) != "" {
			stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
			poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
			if t, err := blockchain.GetUniswapV4PoolCurrentTickViaStateView(stateView, poolManager, poolID); err == nil {
				currentTick = t
				gotTick = true
			}
		}
	default:
		if common.IsHexAddress(poolID) {
			poolAddr := common.HexToAddress(poolID)
			if client, _, err := blockchain.GetEVMClient(chain); err == nil && client != nil {
				if t, err := blockchain.GetV3PoolCurrentTickWithClient(client, poolAddr); err == nil {
					currentTick = t
					gotTick = true
				}
			}
		}
	}

	inRange := false
	if gotTick && tickLower < tickUpper {
		inRange = currentTick >= tickLower && currentTick <= tickUpper
	}

	// Prefer actual tick-based range so that rangePct reflects the CURRENT
	// position, not the (possibly just-updated) rebalance target stored on the task.
	rangePct := 0.0
	if gotTick && tickLower < tickUpper {
		rangePct = estimateRangePercent(currentTick, tickLower, tickUpper)
	}
	if rangePct <= 0 {
		if task.RangeLowerPercentage > 0 && task.RangeUpperPercentage > 0 {
			rangePct = (task.RangeLowerPercentage + task.RangeUpperPercentage) / 2.0
		} else if task.RangePercentage > 0 {
			rangePct = task.RangePercentage
		}
	}

	token0 := common.Address{}
	token1 := common.Address{}

	if common.IsHexAddress(task.Token0Address) {
		token0 = common.HexToAddress(task.Token0Address)
	}
	if common.IsHexAddress(task.Token1Address) {
		token1 = common.HexToAddress(task.Token1Address)
	}

	// Best-effort: for V3 tasks, derive token0/token1 from pool if missing.
	if version == "v3" && token0 == (common.Address{}) && token1 == (common.Address{}) && common.IsHexAddress(poolID) {
		poolAddr := common.HexToAddress(poolID)
		if client, _, err := blockchain.GetEVMClient(chain); err == nil && client != nil {
			if t0, t1, err := blockchain.GetV3PoolTokensWithClient(client, poolAddr); err == nil {
				token0 = t0
				token1 = t1
			}
		}
	}

	meta0 := realtimeTokenMeta{symbol: strings.TrimSpace(task.Token0Symbol), decimals: pricing.DefaultTokenDecimals}
	meta1 := realtimeTokenMeta{symbol: strings.TrimSpace(task.Token1Symbol), decimals: pricing.DefaultTokenDecimals}
	if token0 != (common.Address{}) {
		meta0 = s.getTokenMeta(chain, token0)
	}
	if token1 != (common.Address{}) {
		meta1 = s.getTokenMeta(chain, token1)
	}
	if strings.TrimSpace(meta0.symbol) == "" {
		if token0 != (common.Address{}) {
			meta0.symbol = token0.Hex()
		} else {
			meta0.symbol = "-"
		}
	}
	if strings.TrimSpace(meta1.symbol) == "" {
		if token1 != (common.Address{}) {
			meta1.symbol = token1.Hex()
		} else {
			meta1.symbol = "-"
		}
	}

	w0 := s.getWalletTokenBalance(chain, token0, walletAddr)
	w1 := s.getWalletTokenBalance(chain, token1, walletAddr)

	price0 := 0.0
	price1 := 0.0
	addrList := make([]string, 0, 2)
	if token0 != (common.Address{}) {
		addrList = append(addrList, token0.Hex())
	}
	if token1 != (common.Address{}) {
		addrList = append(addrList, token1.Hex())
	}
	if len(addrList) > 0 {
		prices, _ := s.priceService.GetUSDPrices(chain, addrList)
		if token0 != (common.Address{}) {
			price0 = prices[strings.ToLower(token0.Hex())]
		}
		if token1 != (common.Address{}) {
			price1 = prices[strings.ToLower(token1.Hex())]
		}
	}

	row0 := buildTokenRow(token0, meta0, price0, w0, big.NewInt(0), big.NewInt(0))
	row1 := buildTokenRow(token1, meta1, price1, w1, big.NewInt(0), big.NewInt(0))

	totals := RealtimeTotals{
		WalletUSD: row0.WalletUSD + row1.WalletUSD,
	}
	totals.TotalUSD = totals.WalletUSD

	exchange := strings.TrimSpace(task.Exchange)
	if exchange == "" {
		if version == "v4" {
			exchange = "Uniswap V4"
		} else {
			exchange = "V3"
		}
	}
	feePct := 0.0
	if task.Fee > 0 {
		feePct = float64(task.Fee) / 10000.0
	}
	short := "UniV3"
	if version == "v4" {
		short = "UniV4"
	}
	title := fmt.Sprintf("%s-%s-%s-%.4f%%", exchangeShort(exchange, short), row0.Symbol, row1.Symbol, feePct)

	taskRangeLowerPct := 0.0
	taskRangeUpperPct := 0.0
	lowerPct := task.RangeLowerPercentage
	upperPct := task.RangeUpperPercentage
	if lowerPct <= 0 || upperPct <= 0 {
		if task.RangePercentage > 0 {
			lowerPct = task.RangePercentage
			upperPct = task.RangePercentage
		}
	}
	if lowerPct > 0 && upperPct > 0 {
		stableLower, stableUpper := pricing.StablePercentagesFromTickPercentages(task, lowerPct, upperPct)
		if stableLower > 0 && stableUpper > 0 {
			taskRangeLowerPct = stableLower
			taskRangeUpperPct = stableUpper
		} else {
			taskRangeLowerPct = lowerPct
			taskRangeUpperPct = upperPct
		}
	}
	pnlMetrics := s.getTaskPnLViewMetrics(task)
	initialCostUSD := pnlMetrics.initialCost
	netInvestedUSD := pnlMetrics.netInvested
	currentValueUSD := pnlMetrics.currentValue
	absolutePnLUSD := pnlMetrics.absolutePnL
	hasPnL := pnlMetrics.hasPnL

	return &RealtimePosition{
		Chain:      chain,
		Version:    version,
		Exchange:   exchange,
		Title:      title,
		FeeTier:    uint64(task.Fee),
		PoolID:     poolID,
		PositionID: fmt.Sprintf("task-%d", task.ID),
		WalletID:   task.WalletID,
		WalletAddress: func() string {
			if strings.TrimSpace(task.WalletAddress) != "" {
				return strings.TrimSpace(task.WalletAddress)
			}
			return walletAddr.Hex()
		}(),
		TaskID:                task.ID,
		TaskPaused:            task.Paused,
		TaskRebalanceEnabled:  models.RebalanceEnabledForOutOfRangeMode(models.ResolveStrategyOutOfRangeMode(task)),
		TaskMode:              models.EffectiveStrategyTaskMode(task),
		TaskAmountUSDT:        displayTaskAmountUSDTWithMetrics(task, pnlMetrics),
		TaskSlippageTolerance: task.SlippageTolerance,
		StatusLabel:           statusLabelFromTask(task),
		InRange:               inRange,
		CurrentTick:           currentTick,
		TickLower:             tickLower,
		TickUpper:             tickUpper,
		TickSpacing:           task.TickSpacing,
		RangePercent:          rangePct,
		TaskRangeLowerPct:     taskRangeLowerPct,
		TaskRangeUpperPct:     taskRangeUpperPct,
		OutOfRange:            formatOutOfRange(task, tickLower, tickUpper, currentTick),
		RunningSince:          &task.CreatedAt,
		HasLiquidity:          false,
		InitialCostUSD:        initialCostUSD,
		NetInvestedUSD:        netInvestedUSD,
		CurrentValueUSD:       currentValueUSD,
		AbsolutePnLUSD:        absolutePnLUSD,
		HasPnL:                hasPnL,
		DCA:                   buildRealtimeDCAStatus(task),
		TokenRows:             []RealtimeTokenRow{row0, row1},
		Totals:                totals,
	}, ""
}

func (s *RealtimePositionsService) getTokenMeta(chain string, addr common.Address) realtimeTokenMeta {
	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}

	symbol := ""
	decimals := pricing.DefaultTokenDecimals

	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil || client == nil {
		symbol = addr.Hex()
	} else {
		if sym, err := blockchain.GetTokenSymbolWithClient(client, addr); err == nil && strings.TrimSpace(sym) != "" {
			symbol = strings.TrimSpace(sym)
		} else {
			symbol = addr.Hex()
		}
		if dec, err := blockchain.GetTokenDecimalsWithClient(client, addr); err == nil && dec > 0 {
			decimals = int(dec)
		} else {
			decimals = pricing.DefaultTokenDecimals
		}
	}

	return realtimeTokenMeta{symbol: symbol, decimals: decimals}
}

func v4TaskCurrencies(task *models.StrategyTask) (common.Address, common.Address, bool) {
	if task == nil {
		return common.Address{}, common.Address{}, false
	}
	token0 := strings.TrimSpace(task.Token0Address)
	token1 := strings.TrimSpace(task.Token1Address)
	if !common.IsHexAddress(token0) || !common.IsHexAddress(token1) {
		return common.Address{}, common.Address{}, false
	}
	c0 := common.HexToAddress(token0)
	c1 := common.HexToAddress(token1)
	return c0, c1, v4CurrenciesReady(c0, c1)
}

func v4PositionHasCurrencies(pos *blockchain.V4PositionInfo) bool {
	if pos == nil {
		return false
	}
	return v4CurrenciesReady(pos.Token0, pos.Token1)
}

func v4CurrenciesReady(c0 common.Address, c1 common.Address) bool {
	if c0 == c1 {
		return false
	}
	return c0 != (common.Address{}) || c1 != (common.Address{})
}

func realtimeChainConfig(chain string) (config.ChainConfig, bool) {
	if config.AppConfig == nil {
		return config.ChainConfig{}, false
	}
	return config.AppConfig.GetChainConfig(chain)
}

func realtimeWrappedNative(chain string) (common.Address, bool) {
	cc, ok := realtimeChainConfig(chain)
	if !ok || !common.IsHexAddress(cc.WrappedNativeAddress) {
		return common.Address{}, false
	}
	wrapped := common.HexToAddress(cc.WrappedNativeAddress)
	return wrapped, wrapped != (common.Address{})
}

func realtimeNativeSymbol(chain string) string {
	cc, ok := realtimeChainConfig(chain)
	if ok {
		wrapped := strings.ToUpper(strings.TrimSpace(cc.WrappedNativeSymbol))
		if strings.HasPrefix(wrapped, "W") && len(wrapped) > 1 {
			return wrapped[1:]
		}
		if wrapped != "" {
			return wrapped
		}
	}
	switch config.NormalizeChain(chain) {
	case "bsc":
		return "BNB"
	case "base":
		return "ETH"
	default:
		return "NATIVE"
	}
}

func (s *RealtimePositionsService) getV4CurrencyMeta(chain string, currency common.Address) realtimeTokenMeta {
	if currency != (common.Address{}) {
		return s.getTokenMeta(chain, currency)
	}
	return realtimeTokenMeta{symbol: realtimeNativeSymbol(chain), decimals: 18}
}

func (s *RealtimePositionsService) getWalletV4CurrencyBalance(chain string, currency common.Address, walletAddress common.Address) *big.Int {
	if currency != (common.Address{}) {
		return s.getWalletTokenBalance(chain, currency, walletAddress)
	}
	if walletAddress == (common.Address{}) {
		return big.NewInt(0)
	}
	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}
	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil || client == nil {
		return big.NewInt(0)
	}
	bal, err := blockchain.GetBalanceWithClient(client, walletAddress)
	if err != nil || bal == nil {
		return big.NewInt(0)
	}
	return bal
}

func (s *RealtimePositionsService) getV4CurrencyUSDPrices(chain string, currencies ...common.Address) map[common.Address]float64 {
	out := make(map[common.Address]float64, len(currencies))
	priceKeys := make(map[common.Address]string, len(currencies))
	seen := make(map[string]struct{}, len(currencies))
	query := make([]string, 0, len(currencies))

	for _, currency := range currencies {
		priceAddress := currency
		if currency == (common.Address{}) {
			wrapped, ok := realtimeWrappedNative(chain)
			if !ok {
				continue
			}
			priceAddress = wrapped
		}
		key := strings.ToLower(priceAddress.Hex())
		priceKeys[currency] = key
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		query = append(query, key)
	}

	if len(query) > 0 && s != nil && s.priceService != nil {
		if prices, err := s.priceService.GetUSDPrices(chain, query); err == nil {
			for currency, key := range priceKeys {
				out[currency] = prices[key]
			}
		}
	}
	for _, currency := range currencies {
		if currency == (common.Address{}) && out[currency] <= 0 {
			out[currency] = pricing.GetNativePriceUSD(chain)
		}
	}
	return out
}

func (s *RealtimePositionsService) getWalletTokenBalance(chain string, tokenAddress, walletAddress common.Address) *big.Int {
	if (tokenAddress == common.Address{}) || (walletAddress == common.Address{}) {
		return big.NewInt(0)
	}

	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}

	client, _, errClient := blockchain.GetEVMClient(chain)
	if errClient != nil || client == nil {
		return big.NewInt(0)
	}
	bal, err := blockchain.GetTokenBalanceWithClient(client, tokenAddress, walletAddress)
	if err == nil && bal != nil {
		return bal
	}
	return big.NewInt(0)
}

func (s *RealtimePositionsService) getV3PoolAddress(chain string, npmAddr common.Address, token0 common.Address, token1 common.Address, fee uint64) (common.Address, bool, error) {
	if (npmAddr == common.Address{}) || (token0 == common.Address{}) || (token1 == common.Address{}) {
		return common.Address{}, false, fmt.Errorf("v3 pool key missing")
	}

	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}

	addr, err := resolveV3PoolAddress(chain, nil, 10*time.Second, npmAddr, token0, token1, fee)
	if err != nil {
		return common.Address{}, false, err
	}
	if addr == (common.Address{}) {
		return common.Address{}, false, fmt.Errorf("v3 pool not found")
	}
	return addr, false, nil
}

func (s *RealtimePositionsService) getV3Slot0(chain string, poolAddress common.Address) (*big.Int, int, bool, time.Duration, error) {
	if (poolAddress == common.Address{}) {
		return nil, 0, false, 0, fmt.Errorf("empty pool address")
	}

	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}

	client, _, errClient := blockchain.GetEVMClient(chain)
	if errClient != nil {
		return nil, 0, false, 0, errClient
	}
	if client == nil {
		return nil, 0, false, 0, fmt.Errorf("evm client not initialized")
	}
	sqrt, tick, err := blockchain.GetV3PoolSlot0WithClient(client, poolAddress)
	return sqrt, tick, false, 0, err
}

func (s *RealtimePositionsService) getV3FeeGrowthGlobals(chain string, poolAddress common.Address) (*big.Int, *big.Int, bool, time.Duration, error) {
	if (poolAddress == common.Address{}) {
		return nil, nil, false, 0, fmt.Errorf("empty pool address")
	}

	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}

	client, _, errClient := blockchain.GetEVMClient(chain)
	if errClient != nil {
		return nil, nil, false, 0, errClient
	}
	if client == nil {
		return nil, nil, false, 0, fmt.Errorf("evm client not initialized")
	}
	g0, g1, err := blockchain.GetV3PoolFeeGrowthGlobalsWithClient(client, poolAddress)
	return g0, g1, false, 0, err
}

func (s *RealtimePositionsService) getV3TickFeeGrowthOutside(chain string, poolAddress common.Address, tick int) (*big.Int, *big.Int, bool, bool, time.Duration, error) {
	if (poolAddress == common.Address{}) {
		return nil, nil, false, false, 0, fmt.Errorf("empty pool address")
	}

	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}

	client, _, errClient := blockchain.GetEVMClient(chain)
	if errClient != nil {
		return nil, nil, false, false, 0, errClient
	}
	if client == nil {
		return nil, nil, false, false, 0, fmt.Errorf("evm client not initialized")
	}
	f0, f1, initialized, err := blockchain.GetV3PoolTickFeeGrowthOutsideWithClient(client, poolAddress, tick)
	return f0, f1, initialized, false, 0, err
}

func (s *RealtimePositionsService) calcV3UnclaimedFeesLive(chain string, poolAddr common.Address, currentTick int, pos *blockchain.V3PositionInfo) (*big.Int, *big.Int, bool, time.Duration, error) {
	if pos == nil {
		return nil, nil, false, 0, fmt.Errorf("position info missing")
	}
	if pos.Liquidity == nil || pos.Liquidity.Sign() == 0 {
		return cloneBig(pos.TokensOwed0), cloneBig(pos.TokensOwed1), false, 0, nil
	}
	if poolAddr == (common.Address{}) {
		return nil, nil, false, 0, fmt.Errorf("pool address missing")
	}

	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}

	global0, global1, staleG, ageG, errG := s.getV3FeeGrowthGlobals(chain, poolAddr)
	if errG != nil && (global0 == nil || global1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read feeGrowthGlobal failed: %w", errG)
	}
	lower0, lower1, _, staleL, ageL, errL := s.getV3TickFeeGrowthOutside(chain, poolAddr, pos.TickLower)
	if errL != nil && (lower0 == nil || lower1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read tickLower feeGrowthOutside failed: %w", errL)
	}
	upper0, upper1, _, staleU, ageU, errU := s.getV3TickFeeGrowthOutside(chain, poolAddr, pos.TickUpper)
	if errU != nil && (upper0 == nil || upper1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read tickUpper feeGrowthOutside failed: %w", errU)
	}

	usedStale := staleG || staleL || staleU
	age := time.Duration(0)
	if staleG && ageG > age {
		age = ageG
	}
	if staleL && ageL > age {
		age = ageL
	}
	if staleU && ageU > age {
		age = ageU
	}

	fees0, fees1, calcErr := pool.CalcV3UnclaimedFeesFromGrowths(currentTick, pos, global0, global1, lower0, lower1, upper0, upper1)
	if calcErr != nil {
		return nil, nil, usedStale, age, calcErr
	}

	var errOut error
	if usedStale {
		if errG != nil {
			errOut = errG
		} else if errL != nil {
			errOut = errL
		} else if errU != nil {
			errOut = errU
		}
	}
	return fees0, fees1, usedStale, age, errOut

}

func normalizeV4PoolIDKey(poolID string) string {
	poolIDKey := strings.ToLower(strings.TrimSpace(poolID))
	if poolIDKey != "" && !strings.HasPrefix(poolIDKey, "0x") {
		poolIDKey = "0x" + poolIDKey
	}
	return poolIDKey
}

func (s *RealtimePositionsService) getV4FeeGrowthGlobals(stateView common.Address, poolManager common.Address, poolID string) (*big.Int, *big.Int, bool, time.Duration, error) {
	if (stateView == common.Address{}) || (poolManager == common.Address{}) {
		return nil, nil, false, 0, fmt.Errorf("V4 stateView/poolManager missing")
	}
	g0, g1, err := blockchain.GetV4PoolFeeGrowthGlobals(stateView, poolManager, poolID)
	return g0, g1, false, 0, err
}

func (s *RealtimePositionsService) getV4TickFeeGrowthOutside(stateView common.Address, poolManager common.Address, poolID string, tick int) (*big.Int, *big.Int, bool, time.Duration, error) {
	if (stateView == common.Address{}) || (poolManager == common.Address{}) {
		return nil, nil, false, 0, fmt.Errorf("V4 stateView/poolManager missing")
	}
	f0, f1, err := blockchain.GetV4TickFeeGrowthOutside(stateView, poolManager, poolID, tick)
	return f0, f1, false, 0, err
}

func (s *RealtimePositionsService) calcV4UnclaimedFeesLive(stateView common.Address, poolManager common.Address, poolID string, currentTick int, pos *blockchain.V4PositionInfo) (*big.Int, *big.Int, bool, time.Duration, error) {
	if pos == nil {
		return nil, nil, false, 0, fmt.Errorf("position info missing")
	}
	owed0 := cloneBig(pos.TokensOwed0)
	owed1 := cloneBig(pos.TokensOwed1)

	if pos.Liquidity == nil || pos.Liquidity.Sign() == 0 {
		return owed0, owed1, false, 0, nil
	}
	if pos.FeeGrowthInside0LastX128 == nil || pos.FeeGrowthInside1LastX128 == nil {
		return nil, nil, false, 0, fmt.Errorf("position feeGrowthInside last missing")
	}

	global0, global1, staleG, ageG, errG := s.getV4FeeGrowthGlobals(stateView, poolManager, poolID)
	if errG != nil && (global0 == nil || global1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read V4 feeGrowthGlobal failed: %w", errG)
	}
	lower0, lower1, staleL, ageL, errL := s.getV4TickFeeGrowthOutside(stateView, poolManager, poolID, pos.TickLower)
	if errL != nil && (lower0 == nil || lower1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read V4 tickLower feeGrowthOutside failed: %w", errL)
	}
	upper0, upper1, staleU, ageU, errU := s.getV4TickFeeGrowthOutside(stateView, poolManager, poolID, pos.TickUpper)
	if errU != nil && (upper0 == nil || upper1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read V4 tickUpper feeGrowthOutside failed: %w", errU)
	}

	usedStale := staleG || staleL || staleU
	age := time.Duration(0)
	if staleG && ageG > age {
		age = ageG
	}
	if staleL && ageL > age {
		age = ageL
	}
	if staleU && ageU > age {
		age = ageU
	}

	inside0 := feeGrowthInside(currentTick, pos.TickLower, pos.TickUpper, global0, lower0, upper0)
	inside1 := feeGrowthInside(currentTick, pos.TickLower, pos.TickUpper, global1, lower1, upper1)
	// Guard against inconsistent fee snapshots returned by RPC.
	// If fee growth appears to go backwards, clamp the delta to zero.
	last0 := cloneBig(pos.FeeGrowthInside0LastX128)
	last1 := cloneBig(pos.FeeGrowthInside1LastX128)

	delta0 := new(big.Int).Sub(inside0, last0)
	if delta0.Sign() < 0 {
		delta0 = big.NewInt(0)
	}
	delta1 := new(big.Int).Sub(inside1, last1)
	if delta1.Sign() < 0 {
		delta1 = big.NewInt(0)
	}

	extra0 := mulDivFloor(delta0, pos.Liquidity, q128)
	extra1 := mulDivFloor(delta1, pos.Liquidity, q128)
	owed0.Add(owed0, extra0)
	owed1.Add(owed1, extra1)

	var err error
	if usedStale {
		if errG != nil {
			err = errG
		} else if errL != nil {
			err = errL
		} else if errU != nil {
			err = errU
		}
	}
	return owed0, owed1, usedStale, age, err
}

func (s *RealtimePositionsService) calcV4UnclaimedFeesLiveUnified(stateView common.Address, poolManager common.Address, poolID string, currentTick int, pos *blockchain.V4PositionInfo) (*big.Int, *big.Int, bool, time.Duration, error) {
	if pos == nil {
		return nil, nil, false, 0, fmt.Errorf("position info missing")
	}

	owed0 := cloneBig(pos.TokensOwed0)
	owed1 := cloneBig(pos.TokensOwed1)

	if pos.Liquidity == nil || pos.Liquidity.Sign() == 0 {
		return owed0, owed1, false, 0, nil
	}
	if pos.FeeGrowthInside0LastX128 == nil || pos.FeeGrowthInside1LastX128 == nil {
		return nil, nil, false, 0, fmt.Errorf("position feeGrowthInside last missing")
	}

	global0, global1, staleG, ageG, errG := s.getV4FeeGrowthGlobals(stateView, poolManager, poolID)
	if errG != nil && (global0 == nil || global1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read V4 feeGrowthGlobal failed: %w", errG)
	}
	lower0, lower1, staleL, ageL, errL := s.getV4TickFeeGrowthOutside(stateView, poolManager, poolID, pos.TickLower)
	if errL != nil && (lower0 == nil || lower1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read V4 tickLower feeGrowthOutside failed: %w", errL)
	}
	upper0, upper1, staleU, ageU, errU := s.getV4TickFeeGrowthOutside(stateView, poolManager, poolID, pos.TickUpper)
	if errU != nil && (upper0 == nil || upper1 == nil) {
		return nil, nil, false, 0, fmt.Errorf("read V4 tickUpper feeGrowthOutside failed: %w", errU)
	}

	usedStale := staleG || staleL || staleU
	age := time.Duration(0)
	if staleG && ageG > age {
		age = ageG
	}
	if staleL && ageL > age {
		age = ageL
	}
	if staleU && ageU > age {
		age = ageU
	}

	fees0, fees1, calcErr := pool.CalcV4UnclaimedFeesFromGrowths(currentTick, pos, global0, global1, lower0, lower1, upper0, upper1)
	if calcErr != nil {
		return nil, nil, usedStale, age, calcErr
	}

	var err error
	if usedStale {
		if errG != nil {
			err = errG
		} else if errL != nil {
			err = errL
		} else if errU != nil {
			err = errU
		}
	}
	return fees0, fees1, usedStale, age, err
}

func (s *RealtimePositionsService) getV4Slot0(stateView common.Address, poolManager common.Address, poolID string) (*big.Int, int, bool, time.Duration, error) {
	sqrt, tick, err := blockchain.GetUniswapV4PoolSlot0ViaStateView(stateView, poolManager, poolID)
	return sqrt, tick, false, 0, err
}

func snapshotBlockNumber(client *ethclient.Client) (uint64, error) {
	if client == nil {
		return 0, fmt.Errorf("evm client not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return client.BlockNumber(ctx)
}

func isTransientFeeCalcError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "inconsistent v3 fee snapshot") ||
		strings.Contains(msg, "inconsistent v4 fee snapshot") ||
		strings.Contains(msg, "invalid feegrowthinside")
}

func appendRealtimeWarning(existing, msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return existing
	}
	if existing == "" {
		return msg
	}
	return existing + "; " + msg
}

func logRealtimeFeeIssue(version, stage, tokenID string, err error) {
	if err == nil {
		return
	}
	log.Printf("[Realtime] %s fee issue: stage=%s tokenId=%s transient=%t err=%v", version, stage, tokenID, isTransientFeeCalcError(err), err)
}

func buildTokenRow(token common.Address, meta realtimeTokenMeta, priceUSD float64, walletAmt, posAmt, feeAmt *big.Int) RealtimeTokenRow {
	w := toFloat(walletAmt, meta.decimals)
	p := toFloat(posAmt, meta.decimals)
	f := toFloat(feeAmt, meta.decimals)

	return RealtimeTokenRow{
		Address:        token.Hex(),
		Symbol:         meta.symbol,
		Decimals:       meta.decimals,
		PriceUSD:       priceUSD,
		PriceUSDText:   fmt.Sprintf("$%.4f", priceUSD),
		WalletAmount:   fmt.Sprintf("%.2f", w),
		WalletUSD:      w * priceUSD,
		PositionAmount: fmt.Sprintf("%.2f", p),
		PositionUSD:    p * priceUSD,
		FeeAmount:      formatFeeAmount(f),
		FeeUSD:         f * priceUSD,
	}
}

func formatFeeAmount(v float64) string {
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return "0.00"
	}
	if math.Abs(v) < 0.01 {
		s := fmt.Sprintf("%.6f", v)
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
		if s == "" || s == "-0" {
			return "0.00"
		}
		return s
	}
	return fmt.Sprintf("%.2f", v)
}

func toFloat(v *big.Int, decimals int) float64 {
	if v == nil || v.Sign() == 0 {
		return 0
	}
	if decimals <= 0 {
		decimals = 18
	}
	r := new(big.Rat).SetInt(v)
	den := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	r.Quo(r, den)
	f, _ := new(big.Float).SetRat(r).Float64()
	return f
}

func formatUnits(v *big.Int, decimals int, displayDecimals int) string {
	f := toFloat(v, decimals)
	if displayDecimals < 0 {
		displayDecimals = 0
	}
	return fmt.Sprintf("%.*f", displayDecimals, f)
}

func estimateRangePercent(currentTick, tickLower, tickUpper int) float64 {
	if currentTick == 0 || tickLower == 0 || tickUpper == 0 {
		return 0
	}
	price := math.Pow(1.0001, float64(currentTick))
	low := math.Pow(1.0001, float64(tickLower))
	high := math.Pow(1.0001, float64(tickUpper))
	if price <= 0 || low <= 0 || high <= 0 {
		return 0
	}
	up := (high/price - 1.0) * 100
	down := (1.0 - low/price) * 100
	pct := (up + down) / 2
	if pct < 0 {
		pct = 0
	}
	if pct > 999 {
		pct = 999
	}
	return math.Round(pct*10) / 10
}

func exchangeShort(exchange, fallback string) string {
	ex := strings.ToLower(strings.TrimSpace(exchange))
	switch {
	case strings.Contains(ex, "uniswap") && strings.Contains(ex, "v4"):
		return "UniV4"
	case strings.Contains(ex, "uniswap") && strings.Contains(ex, "v3"):
		return "UniV3"
	case strings.Contains(ex, "pancake") && strings.Contains(ex, "v3"):
		return "PanV3"
	case strings.Contains(ex, "v4"):
		return "V4"
	case strings.Contains(ex, "v3"):
		return "V3"
	default:
		if fallback != "" {
			return fallback
		}
		return exchange
	}
}

func formatOutOfRange(task *models.StrategyTask, tickLower, tickUpper int, currentTick int) string {
	if task != nil && task.Paused && (task.Status == models.StrategyStatusRunning || task.Status == models.StrategyStatusWaiting) {
		return "暂停"
	}
	if task == nil {
		return "0/0"
	}
	if task.Paused && (task.Status == models.StrategyStatusRunning || task.Status == models.StrategyStatusWaiting) {
		return "⏸"
	}
	if strategy.ShouldDelayOutOfRangeHandling(task) {
		return "0/0"
	}
	threshold := task.ReopenDelaySeconds
	if threshold <= 0 {
		return "0/0"
	}
	den := int(math.Max(1, math.Round(float64(threshold)/60.0)))
	num := 0
	if task.OutOfRangeSince != nil {
		elapsedMin := int(time.Since(*task.OutOfRangeSince).Minutes())
		if elapsedMin > 0 {
			num = elapsedMin
		}
	}
	if num < 0 {
		num = 0
	}
	if num > den {
		num = den
	}
	return fmt.Sprintf("%d/%d", num, den)
}

func statusLabelFromTask(task *models.StrategyTask) string {
	if task == nil {
		return "运行中"
	}
	if action := strings.TrimSpace(task.ExitPendingAction); action != "" {
		switch action {
		case strategy.ExitActionManualStop:
			return "停止中"
		case strategy.ExitActionStopLoss:
			return "止损中"
		case strategy.ExitActionOutOfRangeStop:
			return "撤仓结束中"
		case strategy.ExitActionRebalance:
			return "再平衡中"
		default:
			return "撤仓中"
		}
	}
	if task.RebalancePending {
		return "再平衡中"
	}
	if task.Paused && (task.Status == models.StrategyStatusRunning || task.Status == models.StrategyStatusWaiting) {
		return "已暂停"
	}
	switch task.Status {
	case models.StrategyStatusRunning:
		return "运行中"
	case models.StrategyStatusWaiting:
		return "等待中"
	case models.StrategyStatusStopping:
		return "停止中"
	case models.StrategyStatusStopped:
		return "已停止"
	case models.StrategyStatusError:
		return "错误"
	default:
		return "运行中"
	}

	if task == nil {
		return "运行中"
	}
	if strings.TrimSpace(task.ExitPendingAction) != "" {
		switch strings.TrimSpace(task.ExitPendingAction) {
		case strategy.ExitActionManualStop:
			return "停止中"
		case strategy.ExitActionStopLoss:
			return "止损中"
		case strategy.ExitActionOutOfRangeStop:
			return "撤仓终止中"
		case strategy.ExitActionRebalance:
			return "再平衡中"
		default:
			return "撤出中"
		}
	}
	if task.RebalancePending {
		return "再平衡中"
	}
	if task.Paused && (task.Status == models.StrategyStatusRunning || task.Status == models.StrategyStatusWaiting) {
		return "已暂停"
	}
	switch task.Status {
	case models.StrategyStatusRunning:
		return "运行中"
	case models.StrategyStatusWaiting:
		return "等待中"
	case models.StrategyStatusStopping:
		return "停止中"
	case models.StrategyStatusStopped:
		return "已停止"
	case models.StrategyStatusError:
		return "错误"
	default:
		return "运行中"
	}
}
