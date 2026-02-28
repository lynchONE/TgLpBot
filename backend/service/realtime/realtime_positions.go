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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

type RealtimePositionsService struct {
	walletService *wallet.WalletService
	taskService   *strategy.StrategyTaskService
	priceService  *pricing.TokenPriceService

	computeGroup singleflight.Group

	cacheMu sync.RWMutex
	cache   map[uint]cachedRealtimePositions

	tokenMetaMu sync.RWMutex
	tokenMeta   map[string]cachedTokenMeta

	balanceMu sync.RWMutex
	balance   map[string]cachedTokenBalance

	v3Slot0Mu sync.RWMutex
	v3Slot0   map[string]cachedV3Slot0

	v3PoolMu sync.RWMutex
	v3Pool   map[string]cachedV3PoolAddress

	v4Slot0Mu sync.RWMutex
	v4Slot0   map[string]cachedV4Slot0

	v4ScanMu    sync.RWMutex
	v4ScanCache map[string]cachedV4TokenIDs

	v4FeeMu    sync.RWMutex
	v4FeeCache map[string]cachedV4FeeGrowthGlobals

	v4TickFeeMu    sync.RWMutex
	v4TickFeeCache map[string]cachedV4TickFeeGrowthOutside

	v3FeeMu    sync.RWMutex
	v3FeeCache map[string]cachedV3FeeGrowthGlobals

	v3TickFeeMu    sync.RWMutex
	v3TickFeeCache map[string]cachedV3TickFeeGrowthOutside
}

type cachedRealtimePositions struct {
	resp    *RealtimePositionsResponse
	expires time.Time
}

type cachedTokenMeta struct {
	symbol   string
	decimals int
	expires  time.Time
}

type cachedTokenBalance struct {
	value     *big.Int
	updatedAt time.Time
	expires   time.Time
}

type cachedV4TokenIDs struct {
	ids     []string
	expires time.Time
}

type cachedV3Slot0 struct {
	sqrtPriceX96 *big.Int
	tick         int
	updatedAt    time.Time
	expires      time.Time
}

type cachedV3PoolAddress struct {
	addr    common.Address
	expires time.Time
}

type cachedV4Slot0 struct {
	sqrtPriceX96 *big.Int
	tick         int
	updatedAt    time.Time
	expires      time.Time
}

type cachedV3FeeGrowthGlobals struct {
	global0   *big.Int
	global1   *big.Int
	updatedAt time.Time
	expires   time.Time
}

type cachedV3TickFeeGrowthOutside struct {
	fee0        *big.Int
	fee1        *big.Int
	initialized bool
	updatedAt   time.Time
	expires     time.Time
}

type cachedV4FeeGrowthGlobals struct {
	global0   *big.Int
	global1   *big.Int
	updatedAt time.Time
	expires   time.Time
}

type cachedV4TickFeeGrowthOutside struct {
	fee0      *big.Int
	fee1      *big.Int
	updatedAt time.Time
	expires   time.Time
}

func NewRealtimePositionsService() *RealtimePositionsService {
	return &RealtimePositionsService{
		walletService:  wallet.NewWalletService(),
		taskService:    strategy.NewStrategyTaskService(),
		priceService:   pricing.NewTokenPriceService(),
		cache:          make(map[uint]cachedRealtimePositions),
		tokenMeta:      make(map[string]cachedTokenMeta),
		balance:        make(map[string]cachedTokenBalance),
		v3Slot0:        make(map[string]cachedV3Slot0),
		v3Pool:         make(map[string]cachedV3PoolAddress),
		v4Slot0:        make(map[string]cachedV4Slot0),
		v4ScanCache:    make(map[string]cachedV4TokenIDs),
		v4FeeCache:     make(map[string]cachedV4FeeGrowthGlobals),
		v4TickFeeCache: make(map[string]cachedV4TickFeeGrowthOutside),
		v3FeeCache:     make(map[string]cachedV3FeeGrowthGlobals),
		v3TickFeeCache: make(map[string]cachedV3TickFeeGrowthOutside),
	}
}

func (s *RealtimePositionsService) InvalidateUser(userID uint) {
	if s == nil || userID == 0 {
		return
	}
	s.cacheMu.Lock()
	delete(s.cache, userID)
	s.cacheMu.Unlock()
}

type RealtimePositionsResponse struct {
	Wallet            RealtimeWallet     `json:"wallet"`
	Summary           RealtimeSummary    `json:"summary"`
	Positions         []RealtimePosition `json:"positions"`
	PollIntervalSec   int                `json:"poll_interval_sec"`
	IsAdmin           bool               `json:"is_admin"`
	SmartMoneyEnabled bool               `json:"smart_money_enabled"`
	UpdatedAt         time.Time          `json:"updated_at"`
	Warnings          []string           `json:"warnings,omitempty"`
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
	Chain             string     `json:"chain"`
	Version           string     `json:"version"`
	Exchange          string     `json:"exchange"`
	Title             string     `json:"title"`
	PoolID            string     `json:"pool_id"`
	PositionID        string     `json:"position_id"`
	TaskID            uint       `json:"task_id,omitempty"`
	TaskPaused        bool       `json:"task_paused"`
	TaskIsAuto        bool       `json:"task_is_auto"`
	TaskAmountUSDT    float64    `json:"task_amount_usdt,omitempty"`
	StatusLabel       string     `json:"status_label"`
	InRange           bool       `json:"in_range"`
	CurrentTick       int        `json:"current_tick"`
	TickLower         int        `json:"tick_lower"`
	TickUpper         int        `json:"tick_upper"`
	TickSpacing       int        `json:"tick_spacing,omitempty"` // 费率对应的 tick 间距，用于前端计算格数
	RangePercent      float64    `json:"range_percent"`
	OpenPrice         float64    `json:"open_price,omitempty"`
	TaskRangeLowerPct float64    `json:"task_range_lower_pct,omitempty"`
	TaskRangeUpperPct float64    `json:"task_range_upper_pct,omitempty"`
	OutOfRange        string     `json:"out_of_range"`
	RunningSince      *time.Time `json:"running_since,omitempty"`
	HasLiquidity      bool       `json:"has_liquidity"`
	InitialCostUSD    float64    `json:"initial_cost_usd,omitempty"`
	NetInvestedUSD    float64    `json:"net_invested_usd,omitempty"`

	TokenRows []RealtimeTokenRow `json:"token_rows"`
	Totals    RealtimeTotals     `json:"totals"`
}

// tickSpacingFromFee 根据标准 V3 费率（单位 bps×100，例如 3000=0.30%）推导 tick spacing。
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

func (s *RealtimePositionsService) GetForUser(userID uint) (*RealtimePositionsResponse, error) {
	now := time.Now()

	s.cacheMu.RLock()
	if c, ok := s.cache[userID]; ok && c.resp != nil && c.expires.After(now) {
		resp := *c.resp
		s.cacheMu.RUnlock()
		return &resp, nil
	}
	s.cacheMu.RUnlock()

	// De-duplicate concurrent refreshes for the same user (miniapp polls every 1s).
	v, err, _ := s.computeGroup.Do(strconv.FormatUint(uint64(userID), 10), func() (interface{}, error) {
		nowInner := time.Now()

		s.cacheMu.RLock()
		if c, ok := s.cache[userID]; ok && c.resp != nil && c.expires.After(nowInner) {
			resp := *c.resp
			s.cacheMu.RUnlock()
			return &resp, nil
		}
		s.cacheMu.RUnlock()

		resp, err := s.compute(userID)
		if err != nil {
			return nil, err
		}

		s.cacheMu.Lock()
		s.cache[userID] = cachedRealtimePositions{resp: resp, expires: nowInner.Add(3 * time.Second)}
		s.cacheMu.Unlock()
		return resp, nil
	})
	if err != nil {
		return nil, err
	}
	if resp, ok := v.(*RealtimePositionsResponse); ok && resp != nil {
		out := *resp
		return &out, nil
	}
	return nil, fmt.Errorf("unexpected realtime response type: %T", v)
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
		resp.Warnings = append(resp.Warnings, "查询任务信息失败（将仅展示链上数据）")
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
				resp.Warnings = append(resp.Warnings, fmt.Sprintf("初始化 V3 PositionManager 失败: %s", npmAddr.Hex()))
				continue
			}

			bal, err := pm.BalanceOf(nil, walletAddr)
			if err != nil {
				resp.Warnings = append(resp.Warnings, fmt.Sprintf("读取 V3 仓位失败（%s）：%v", npmAddr.Hex(), err))
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
						resp.Warnings = append(resp.Warnings, fmt.Sprintf("初始化 V3 PositionManager 失败: %s", npmAddr.Hex()))
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
		// Avoid blocking the initial load: use cache if present, otherwise warm it in the background.
		if ids, ok := s.getV4OwnedTokenIDsCached(walletAddr); ok && len(ids) > 0 {
			for _, id := range ids {
				ownedV4[id] = struct{}{}
			}
		} else {
			go func(addr common.Address) {
				key := "v4scan:" + strings.ToLower(addr.Hex())
				_, _, _ = s.computeGroup.Do(key, func() (interface{}, error) {
					_, err := s.getV4OwnedTokenIDs(addr)
					if err != nil {
						log.Printf("[Realtime] V4 NFT scan failed: wallet=%s err=%v", addr.Hex(), err)
					}
					return nil, nil
				})
			}(walletAddr)
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

	sort.Slice(positions, func(i, j int) bool {
		pi := positions[i]
		pj := positions[j]

		if pi.Title != pj.Title {
			return pi.Title < pj.Title
		}

		// Keep UI order stable across refreshes: titles can be identical for multiple positions.
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
	})
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

func (s *RealtimePositionsService) getV4OwnedTokenIDsCached(wallet common.Address) ([]string, bool) {
	if config.AppConfig == nil || config.AppConfig.V4NFTScanFromBlock == 0 {
		return nil, false
	}
	if !common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		return nil, false
	}

	key := strings.ToLower(wallet.Hex())
	now := time.Now()

	s.v4ScanMu.RLock()
	if c, ok := s.v4ScanCache[key]; ok && c.expires.After(now) {
		ids := make([]string, len(c.ids))
		copy(ids, c.ids)
		s.v4ScanMu.RUnlock()
		return ids, true
	}
	s.v4ScanMu.RUnlock()
	return nil, false
}

func (s *RealtimePositionsService) getV4OwnedTokenIDs(wallet common.Address) ([]string, error) {
	if config.AppConfig == nil || config.AppConfig.V4NFTScanFromBlock == 0 {
		return nil, nil
	}
	if !common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		return nil, nil
	}

	key := strings.ToLower(wallet.Hex())
	now := time.Now()

	s.v4ScanMu.RLock()
	if c, ok := s.v4ScanCache[key]; ok && c.expires.After(now) {
		ids := make([]string, len(c.ids))
		copy(ids, c.ids)
		s.v4ScanMu.RUnlock()
		return ids, nil
	}
	s.v4ScanMu.RUnlock()

	contract := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
	ids, err := scanERC721OwnedTokenIDs(contract, wallet, config.AppConfig.V4NFTScanFromBlock)
	if err != nil {
		return nil, err
	}

	s.v4ScanMu.Lock()
	s.v4ScanCache[key] = cachedV4TokenIDs{ids: ids, expires: now.Add(90 * time.Second)}
	s.v4ScanMu.Unlock()
	return ids, nil
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
		return nil, fmt.Sprintf("读取 V3 positions() 失败: chain=%s npm=%s tokenId=%s err=%v", chain, npmAddr.Hex(), tokenId.String(), err)
	}

	token0 := info.Token0
	token1 := info.Token1
	tickLower := info.TickLower
	tickUpper := info.TickUpper
	liq := info.Liquidity
	owed0 := info.TokensOwed0
	owed1 := info.TokensOwed1
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
	openPrice := 0.0

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
		if task.GuardOpenPrice > 0 {
			openPrice = task.GuardOpenPrice
		}
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

	// Get tick and sqrtP (poolID required). If missing, we still return a card but tick/amounts may be 0.
	currentTick := 0
	var sqrtP *big.Int
	hasSlot0 := false
	if poolAddr != (common.Address{}) {
		sp, t, usedStale, age, err := s.getV3Slot0(chain, poolAddr)
		if err != nil && sp == nil {
			warn = fmt.Sprintf("读取 V3 Pool slot0 失败: pool=%s tokenId=%s err=%v", poolAddr.Hex(), tokenId.String(), err)
		} else {
			sqrtP = sp
			currentTick = t
			hasSlot0 = true
			if usedStale && err != nil {
				warn = fmt.Sprintf("V3 slot0 RPC 限流/失败，已使用缓存（%ds 前）。建议调大自动刷新或更换 BSC RPC：tokenId=%s", int(age.Seconds()), tokenId.String())
			}
		}
	} else {
		warn = fmt.Sprintf("V3 tokenId=%s 未找到 pool 地址（将无法计算 tick/仓位数量）", tokenId.String())
	}

	inRange := currentTick >= tickLower && currentTick <= tickUpper
	if sqrtP == nil {
		sqrtP = big.NewInt(0)
	}
	if task != nil {
		outOfRangeText = formatOutOfRange(task, tickLower, tickUpper, currentTick)
	}

	if rangePct <= 0 && currentTick != 0 {
		rangePct = estimateRangePercent(currentTick, tickLower, tickUpper)
	}

	if hasSlot0 && poolAddr != (common.Address{}) {
		fee0, fee1, usedStale, age, feeErr := s.calcV3UnclaimedFeesCached(chain, poolAddr, currentTick, info)
		if feeErr != nil && (fee0 == nil || fee1 == nil) {
			msg := fmt.Sprintf("V3 手续费计算失败: tokenId=%s err=%v", tokenId.String(), feeErr)
			if warn == "" {
				warn = msg
			} else {
				warn = warn + "; " + msg
			}
		} else if fee0 != nil && fee1 != nil {
			owed0 = fee0
			owed1 = fee1
			if usedStale && feeErr != nil {
				msg := fmt.Sprintf("V3 手续费 RPC 限流/失败，已使用缓存（%ds 前）。建议调大自动刷新或更换 BSC RPC：tokenId=%s", int(age.Seconds()), tokenId.String())
				if warn == "" {
					warn = msg
				} else {
					warn = warn + "; " + msg
				}
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

	row0 := buildTokenRow(token0, meta0, price0, w0, amt0Raw, owed0)
	row1 := buildTokenRow(token1, meta1, price1, w1, amt1Raw, owed1)

	totals := RealtimeTotals{
		WalletUSD:   row0.WalletUSD + row1.WalletUSD,
		PositionUSD: row0.PositionUSD + row1.PositionUSD,
		FeeUSD:      row0.FeeUSD + row1.FeeUSD,
	}
	totals.TotalUSD = totals.WalletUSD + totals.PositionUSD + totals.FeeUSD

	title := fmt.Sprintf("%s-%s-%s-%.2f%%", exchangeShort(exchange, "UniV3"), row0.Symbol, row1.Symbol, fee)
	if strings.TrimSpace(exchange) == "" {
		exchange = "V3"
	}

	hasLiquidity := liq != nil && liq.Sign() > 0
	taskID := uint(0)
	taskPaused := false
	taskIsAuto := false
	taskRangeLowerPct := 0.0
	taskRangeUpperPct := 0.0
	if task != nil {
		taskID = task.ID
		taskPaused = task.Paused
		taskIsAuto = task.IsAuto

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
		PoolID:     poolID,
		PositionID: tokenId.String(),
		TaskID:     taskID,
		TaskPaused: taskPaused,
		TaskIsAuto: taskIsAuto,
		TaskAmountUSDT: func() float64 {
			if task == nil || task.AmountUSDT <= 0 {
				return 0
			}
			return task.AmountUSDT
		}(),
		StatusLabel: statusLabel,
		InRange:     inRange,
		CurrentTick: currentTick,
		TickLower:   tickLower,
		TickUpper:   tickUpper,
		TickSpacing: func() int {
			if task != nil && task.TickSpacing > 0 {
				return task.TickSpacing
			}
			return tickSpacingFromFee(info.Fee)
		}(),
		RangePercent:      rangePct,
		OpenPrice:         openPrice,
		TaskRangeLowerPct: taskRangeLowerPct,
		TaskRangeUpperPct: taskRangeUpperPct,
		OutOfRange:        outOfRangeText,
		RunningSince:      runningSince,
		HasLiquidity:      hasLiquidity,
		InitialCostUSD: func() float64 {
			if task == nil || task.AmountUSDT <= 0 {
				return 0
			}
			if actual, ok := getTaskActualInvested(task); ok {
				return actual
			}
			return task.AmountUSDT
		}(),
		NetInvestedUSD: func() float64 {
			if task == nil || task.AmountUSDT <= 0 {
				return 0
			}
			if actual, ok := getTaskActualInvested(task); ok {
				return actual
			}
			return task.AmountUSDT
		}(),
		TokenRows: []RealtimeTokenRow{row0, row1},
		Totals:    totals,
	}, warn
}

// getTaskActualInvested 从交易记录获取任务的实际投入金额（与 bot 的 pnlService.getInitialCost 逻辑一致）。
// 返回 (actualInvested, ok)。ok=false 时应 fallback 到 task.AmountUSDT。
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

func (s *RealtimePositionsService) buildV4Position(walletAddr common.Address, tokenId string, task *models.StrategyTask) (*RealtimePosition, string) {
	if config.AppConfig == nil {
		return nil, "config not loaded"
	}
	if task == nil {
		return nil, ""
	}
	if !common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) || !common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) {
		return nil, "V4 配置不完整（UNISWAP_V4_POOL_MANAGER_ADDRESS/UNISWAP_V4_STATE_VIEW_ADDRESS）"
	}
	poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
	stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)

	c0 := common.HexToAddress(task.Token0Address)
	c1 := common.HexToAddress(task.Token1Address)
	if (c0 == common.Address{}) || (c1 == common.Address{}) {
		return nil, fmt.Sprintf("V4 tokenId=%s 缺少 token0/token1 信息", tokenId)
	}

	var warn string
	liq := big.NewInt(0)
	if v, ok := new(big.Int).SetString(strings.TrimSpace(task.CurrentLiquidity), 10); ok && v != nil {
		liq = v
	}
	tickLower := task.TickLower
	tickUpper := task.TickUpper
	var v4pos *blockchain.V4PositionInfo

	if common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		tokenID, parseErr := convert.ParseBigInt(tokenId)
		if parseErr == nil && tokenID.Sign() > 0 {
			v4pmAddr := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
			pos, posErr := blockchain.GetV4PositionInfo(v4pmAddr, poolManager, task.PoolId, tokenID)
			if posErr != nil {
				log.Printf("[Realtime] V4 position info read failed: tokenId=%s err=%v", tokenId, posErr)
			}
			if pos != nil {
				v4pos = pos
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

	// Ignore empty positions (NFT not burned but liquidity already removed).
	if liq == nil || liq.Sign() == 0 {
		return nil, ""
	}

	sqrtP, currentTick, usedStale, age, err := s.getV4Slot0(stateView, poolManager, task.PoolId)
	if err != nil && sqrtP == nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "429") || strings.Contains(errMsg, "too many requests") || strings.Contains(errMsg, "rate limit") {
			return nil, fmt.Sprintf("读取 V4 slot0 失败（RPC 限流 429），请稍后重试或在设置里调大自动刷新/更换 BSC RPC：tokenId=%s", tokenId)
		}
		return nil, fmt.Sprintf("读取 V4 slot0 失败: tokenId=%s err=%v", tokenId, err)
	}
	if usedStale && err != nil {
		warn = fmt.Sprintf("V4 slot0 RPC 限流/失败，已使用缓存（%ds 前）。建议调大自动刷新或更换 BSC RPC：tokenId=%s", int(age.Seconds()), tokenId)
	}

	owed0 := big.NewInt(0)
	owed1 := big.NewInt(0)
	if v4pos != nil {
		owed0 = cloneBig(v4pos.TokensOwed0)
		owed1 = cloneBig(v4pos.TokensOwed1)
		if fee0, fee1, usedStaleFees, feeAge, feeErr := s.calcV4UnclaimedFeesCached(stateView, poolManager, task.PoolId, currentTick, v4pos); fee0 != nil && fee1 != nil {
			owed0 = fee0
			owed1 = fee1
			if feeErr != nil {
				if usedStaleFees {
					msg := fmt.Sprintf("V4 手续费 RPC 限流/失败，已使用缓存（%ds 前）。建议调大自动刷新或更换 BSC RPC：tokenId=%s", int(feeAge.Seconds()), tokenId)
					if warn == "" {
						warn = msg
					} else {
						warn = warn + "; " + msg
					}
				} else {
					msg := fmt.Sprintf("V4 手续费计算失败（显示为 TokensOwed，可能为 0）：tokenId=%s", tokenId)
					if warn == "" {
						warn = msg
					} else {
						warn = warn + "; " + msg
					}
				}
			}
		} else if feeErr != nil {
			msg := fmt.Sprintf("V4 手续费计算失败（显示为 TokensOwed，可能为 0）：tokenId=%s", tokenId)
			if warn == "" {
				warn = msg
			} else {
				warn = warn + "; " + msg
			}
		}
	}

	sqrtA, _ := pool.SqrtRatioAtTick(int32(tickLower))
	sqrtB, _ := pool.SqrtRatioAtTick(int32(tickUpper))
	amt0Raw, amt1Raw := pool.AmountsForLiquidity(sqrtP, sqrtA, sqrtB, liq)

	chain := config.NormalizeChain(task.Chain)
	if chain == "" {
		chain = "bsc"
	}

	w0 := s.getWalletTokenBalance(chain, c0, walletAddr)
	w1 := s.getWalletTokenBalance(chain, c1, walletAddr)

	meta0 := s.getTokenMeta(chain, c0)
	meta1 := s.getTokenMeta(chain, c1)

	prices, _ := s.priceService.GetUSDPrices(chain, []string{c0.Hex(), c1.Hex()})
	price0 := prices[strings.ToLower(c0.Hex())]
	price1 := prices[strings.ToLower(c1.Hex())]

	row0 := buildTokenRow(c0, meta0, price0, w0, amt0Raw, owed0)
	row1 := buildTokenRow(c1, meta1, price1, w1, amt1Raw, owed1)

	totals := RealtimeTotals{
		WalletUSD:   row0.WalletUSD + row1.WalletUSD,
		PositionUSD: row0.PositionUSD + row1.PositionUSD,
		FeeUSD:      row0.FeeUSD + row1.FeeUSD,
	}
	totals.TotalUSD = totals.WalletUSD + totals.PositionUSD + totals.FeeUSD

	inRange := currentTick >= tickLower && currentTick <= tickUpper
	rangePct := task.RangePercentage
	if task.RangeLowerPercentage > 0 && task.RangeUpperPercentage > 0 {
		rangePct = (task.RangeLowerPercentage + task.RangeUpperPercentage) / 2.0
	} else if rangePct <= 0 {
		rangePct = estimateRangePercent(currentTick, tickLower, tickUpper)
	}

	exchange := strings.TrimSpace(task.Exchange)
	if exchange == "" {
		exchange = "Uniswap V4"
	}
	title := fmt.Sprintf("%s-%s-%s-%.2f%%", exchangeShort(exchange, "UniV4"), row0.Symbol, row1.Symbol, float64(task.Fee)/10000.0)

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
	return &RealtimePosition{
		Chain:             chain,
		Version:           "v4",
		Exchange:          exchange,
		Title:             title,
		PoolID:            strings.TrimSpace(task.PoolId),
		PositionID:        tokenId,
		TaskID:            task.ID,
		TaskPaused:        task.Paused,
		TaskIsAuto:        task.IsAuto,
		TaskAmountUSDT:    task.AmountUSDT,
		StatusLabel:       statusLabelFromTask(task),
		InRange:           inRange,
		CurrentTick:       currentTick,
		TickLower:         tickLower,
		TickUpper:         tickUpper,
		TickSpacing:       task.TickSpacing,
		RangePercent:      rangePct,
		OpenPrice:         task.GuardOpenPrice,
		TaskRangeLowerPct: taskRangeLowerPct,
		TaskRangeUpperPct: taskRangeUpperPct,
		OutOfRange:        formatOutOfRange(task, tickLower, tickUpper, currentTick),
		RunningSince:      &task.CreatedAt,
		HasLiquidity:      hasLiquidity,
		InitialCostUSD: func() float64 {
			if actual, ok := getTaskActualInvested(task); ok {
				return actual
			}
			return task.AmountUSDT
		}(),
		NetInvestedUSD: func() float64 {
			if actual, ok := getTaskActualInvested(task); ok {
				return actual
			}
			return task.AmountUSDT
		}(),
		TokenRows: []RealtimeTokenRow{row0, row1},
		Totals:    totals,
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

	rangePct := task.RangePercentage
	if task.RangeLowerPercentage > 0 && task.RangeUpperPercentage > 0 {
		rangePct = (task.RangeLowerPercentage + task.RangeUpperPercentage) / 2.0
	} else if rangePct <= 0 && gotTick && tickLower < tickUpper {
		rangePct = estimateRangePercent(currentTick, tickLower, tickUpper)
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

	meta0 := cachedTokenMeta{symbol: strings.TrimSpace(task.Token0Symbol), decimals: pricing.DefaultTokenDecimals}
	meta1 := cachedTokenMeta{symbol: strings.TrimSpace(task.Token1Symbol), decimals: pricing.DefaultTokenDecimals}
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
	title := fmt.Sprintf("%s-%s-%s-%.2f%%", exchangeShort(exchange, short), row0.Symbol, row1.Symbol, feePct)

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

	return &RealtimePosition{
		Chain:             chain,
		Version:           version,
		Exchange:          exchange,
		Title:             title,
		PoolID:            poolID,
		PositionID:        fmt.Sprintf("task-%d", task.ID),
		TaskID:            task.ID,
		TaskPaused:        task.Paused,
		TaskIsAuto:        task.IsAuto,
		TaskAmountUSDT:    task.AmountUSDT,
		StatusLabel:       statusLabelFromTask(task),
		InRange:           inRange,
		CurrentTick:       currentTick,
		TickLower:         tickLower,
		TickUpper:         tickUpper,
		TickSpacing:       task.TickSpacing,
		RangePercent:      rangePct,
		TaskRangeLowerPct: taskRangeLowerPct,
		TaskRangeUpperPct: taskRangeUpperPct,
		OutOfRange:        formatOutOfRange(task, tickLower, tickUpper, currentTick),
		RunningSince:      &task.CreatedAt,
		HasLiquidity:      false,
		InitialCostUSD: func() float64 {
			if actual, ok := getTaskActualInvested(task); ok {
				return actual
			}
			return task.AmountUSDT
		}(),
		NetInvestedUSD: func() float64 {
			if actual, ok := getTaskActualInvested(task); ok {
				return actual
			}
			return task.AmountUSDT
		}(),
		TokenRows: []RealtimeTokenRow{row0, row1},
		Totals:    totals,
	}, ""
}

func (s *RealtimePositionsService) getTokenMeta(chain string, addr common.Address) cachedTokenMeta {
	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}

	key := chain + "|" + strings.ToLower(addr.Hex())
	now := time.Now()
	s.tokenMetaMu.RLock()
	if m, ok := s.tokenMeta[key]; ok && m.expires.After(now) {
		s.tokenMetaMu.RUnlock()
		return m
	}
	s.tokenMetaMu.RUnlock()

	symbol := ""
	decimals := pricing.DefaultTokenDecimals
	ttl := 24 * time.Hour

	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil || client == nil {
		symbol = addr.Hex()
		ttl = 5 * time.Minute
	} else {
		if sym, err := blockchain.GetTokenSymbolWithClient(client, addr); err == nil && strings.TrimSpace(sym) != "" {
			symbol = strings.TrimSpace(sym)
		} else {
			symbol = addr.Hex()
			ttl = 5 * time.Minute
		}
		if dec, err := blockchain.GetTokenDecimalsWithClient(client, addr); err == nil && dec > 0 {
			decimals = int(dec)
		} else {
			decimals = pricing.DefaultTokenDecimals
			ttl = 5 * time.Minute
		}
	}

	m := cachedTokenMeta{symbol: symbol, decimals: decimals, expires: now.Add(ttl)}
	s.tokenMetaMu.Lock()
	s.tokenMeta[key] = m
	s.tokenMetaMu.Unlock()
	return m
}

func (s *RealtimePositionsService) getWalletTokenBalance(chain string, tokenAddress, walletAddress common.Address) *big.Int {
	if (tokenAddress == common.Address{}) || (walletAddress == common.Address{}) {
		return big.NewInt(0)
	}

	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}

	now := time.Now()
	key := chain + "|" + strings.ToLower(walletAddress.Hex()) + "|" + strings.ToLower(tokenAddress.Hex())

	s.balanceMu.RLock()
	if c, ok := s.balance[key]; ok && c.value != nil && c.expires.After(now) {
		v := new(big.Int).Set(c.value)
		s.balanceMu.RUnlock()
		return v
	}
	var staleVal *big.Int
	var staleAt time.Time
	if c, ok := s.balance[key]; ok && c.value != nil {
		staleVal = new(big.Int).Set(c.value)
		staleAt = c.updatedAt
	}
	s.balanceMu.RUnlock()

	client, _, errClient := blockchain.GetEVMClient(chain)
	var (
		bal *big.Int
		err error
	)
	if errClient == nil && client != nil {
		bal, err = blockchain.GetTokenBalanceWithClient(client, tokenAddress, walletAddress)
	} else {
		err = errClient
	}
	if err == nil && bal != nil {
		s.balanceMu.Lock()
		s.balance[key] = cachedTokenBalance{
			value:     new(big.Int).Set(bal),
			updatedAt: now,
			expires:   now.Add(8 * time.Second),
		}
		s.balanceMu.Unlock()
		return bal
	}

	// Best-effort fallback (keeps UI usable when RPC is rate-limited).
	if staleVal != nil && !staleAt.IsZero() && now.Sub(staleAt) <= 60*time.Second {
		return staleVal
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

	now := time.Now()
	key := chain + "|" + strings.ToLower(npmAddr.Hex()) + "|" + strings.ToLower(token0.Hex()) + "|" + strings.ToLower(token1.Hex()) + "|" + strconv.FormatUint(fee, 10)

	s.v3PoolMu.RLock()
	if c, ok := s.v3Pool[key]; ok && c.expires.After(now) {
		addr := c.addr
		s.v3PoolMu.RUnlock()
		if addr == (common.Address{}) {
			return common.Address{}, true, fmt.Errorf("v3 pool not found")
		}
		return addr, true, nil
	}
	s.v3PoolMu.RUnlock()

	addr, err := resolveV3PoolAddress(chain, nil, 10*time.Second, npmAddr, token0, token1, fee)
	ttl := 24 * time.Hour
	if err != nil || addr == (common.Address{}) {
		ttl = 30 * time.Second
	}

	s.v3PoolMu.Lock()
	s.v3Pool[key] = cachedV3PoolAddress{
		addr:    addr,
		expires: now.Add(ttl),
	}
	s.v3PoolMu.Unlock()

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

	now := time.Now()
	key := chain + "|" + strings.ToLower(poolAddress.Hex())

	s.v3Slot0Mu.RLock()
	if c, ok := s.v3Slot0[key]; ok && c.sqrtPriceX96 != nil && c.expires.After(now) {
		sqrt := new(big.Int).Set(c.sqrtPriceX96)
		tick := c.tick
		s.v3Slot0Mu.RUnlock()
		return sqrt, tick, false, 0, nil
	}
	var staleSqrt *big.Int
	var staleTick int
	var staleAt time.Time
	if c, ok := s.v3Slot0[key]; ok && c.sqrtPriceX96 != nil {
		staleSqrt = new(big.Int).Set(c.sqrtPriceX96)
		staleTick = c.tick
		staleAt = c.updatedAt
	}
	s.v3Slot0Mu.RUnlock()

	client, _, errClient := blockchain.GetEVMClient(chain)
	var (
		sqrt *big.Int
		tick int
		err  error
	)
	if errClient == nil && client != nil {
		sqrt, tick, err = blockchain.GetV3PoolSlot0WithClient(client, poolAddress)
	} else {
		err = errClient
	}
	if err == nil && sqrt != nil {
		s.v3Slot0Mu.Lock()
		s.v3Slot0[key] = cachedV3Slot0{
			sqrtPriceX96: new(big.Int).Set(sqrt),
			tick:         tick,
			updatedAt:    now,
			expires:      now.Add(5 * time.Second),
		}
		s.v3Slot0Mu.Unlock()
		return sqrt, tick, false, 0, nil
	}

	// Best-effort fallback (keeps UI usable when RPC is rate-limited).
	if staleSqrt != nil && !staleAt.IsZero() && now.Sub(staleAt) <= 30*time.Second {
		return staleSqrt, staleTick, true, now.Sub(staleAt), err
	}
	return nil, 0, false, 0, err
}

func (s *RealtimePositionsService) getV3FeeGrowthGlobals(chain string, poolAddress common.Address) (*big.Int, *big.Int, bool, time.Duration, error) {
	if (poolAddress == common.Address{}) {
		return nil, nil, false, 0, fmt.Errorf("empty pool address")
	}

	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}

	now := time.Now()
	key := chain + "|" + strings.ToLower(poolAddress.Hex())

	s.v3FeeMu.RLock()
	if c, ok := s.v3FeeCache[key]; ok && c.global0 != nil && c.global1 != nil && c.expires.After(now) {
		g0 := new(big.Int).Set(c.global0)
		g1 := new(big.Int).Set(c.global1)
		s.v3FeeMu.RUnlock()
		return g0, g1, false, 0, nil
	}

	var stale0 *big.Int
	var stale1 *big.Int
	var staleAt time.Time
	if c, ok := s.v3FeeCache[key]; ok && c.global0 != nil && c.global1 != nil {
		stale0 = new(big.Int).Set(c.global0)
		stale1 = new(big.Int).Set(c.global1)
		staleAt = c.updatedAt
	}
	s.v3FeeMu.RUnlock()

	client, _, errClient := blockchain.GetEVMClient(chain)
	var (
		g0  *big.Int
		g1  *big.Int
		err error
	)
	if errClient == nil && client != nil {
		g0, g1, err = blockchain.GetV3PoolFeeGrowthGlobalsWithClient(client, poolAddress)
	} else {
		err = errClient
	}
	if err == nil && g0 != nil && g1 != nil {
		s.v3FeeMu.Lock()
		s.v3FeeCache[key] = cachedV3FeeGrowthGlobals{
			global0:   new(big.Int).Set(g0),
			global1:   new(big.Int).Set(g1),
			updatedAt: now,
			expires:   now.Add(2 * time.Second),
		}
		s.v3FeeMu.Unlock()
		return g0, g1, false, 0, nil
	}

	// Best-effort fallback (keeps UI usable when RPC is rate-limited).
	if stale0 != nil && stale1 != nil && !staleAt.IsZero() && now.Sub(staleAt) <= 30*time.Second {
		return stale0, stale1, true, now.Sub(staleAt), err
	}

	return nil, nil, false, 0, err
}

func (s *RealtimePositionsService) getV3TickFeeGrowthOutside(chain string, poolAddress common.Address, tick int) (*big.Int, *big.Int, bool, bool, time.Duration, error) {
	if (poolAddress == common.Address{}) {
		return nil, nil, false, false, 0, fmt.Errorf("empty pool address")
	}

	chain = config.NormalizeChain(chain)
	if chain == "" {
		chain = "bsc"
	}

	now := time.Now()
	key := chain + "|" + strings.ToLower(poolAddress.Hex()) + "|" + strconv.Itoa(tick)

	s.v3TickFeeMu.RLock()
	if c, ok := s.v3TickFeeCache[key]; ok && c.fee0 != nil && c.fee1 != nil && c.expires.After(now) {
		f0 := new(big.Int).Set(c.fee0)
		f1 := new(big.Int).Set(c.fee1)
		initialized := c.initialized
		s.v3TickFeeMu.RUnlock()
		return f0, f1, initialized, false, 0, nil
	}

	var stale0 *big.Int
	var stale1 *big.Int
	var staleInit bool
	var staleAt time.Time
	if c, ok := s.v3TickFeeCache[key]; ok && c.fee0 != nil && c.fee1 != nil {
		stale0 = new(big.Int).Set(c.fee0)
		stale1 = new(big.Int).Set(c.fee1)
		staleInit = c.initialized
		staleAt = c.updatedAt
	}
	s.v3TickFeeMu.RUnlock()

	client, _, errClient := blockchain.GetEVMClient(chain)
	var (
		f0          *big.Int
		f1          *big.Int
		initialized bool
		err         error
	)
	if errClient == nil && client != nil {
		f0, f1, initialized, err = blockchain.GetV3PoolTickFeeGrowthOutsideWithClient(client, poolAddress, tick)
	} else {
		err = errClient
	}
	if err == nil && f0 != nil && f1 != nil {
		s.v3TickFeeMu.Lock()
		s.v3TickFeeCache[key] = cachedV3TickFeeGrowthOutside{
			fee0:        new(big.Int).Set(f0),
			fee1:        new(big.Int).Set(f1),
			initialized: initialized,
			updatedAt:   now,
			expires:     now.Add(20 * time.Second),
		}
		s.v3TickFeeMu.Unlock()
		return f0, f1, initialized, false, 0, nil
	}

	// Best-effort fallback (keeps UI usable when RPC is rate-limited).
	if stale0 != nil && stale1 != nil && !staleAt.IsZero() && now.Sub(staleAt) <= 2*time.Minute {
		return stale0, stale1, staleInit, true, now.Sub(staleAt), err
	}

	return nil, nil, false, false, 0, err
}

func (s *RealtimePositionsService) calcV3UnclaimedFeesCached(chain string, poolAddr common.Address, currentTick int, pos *blockchain.V3PositionInfo) (*big.Int, *big.Int, bool, time.Duration, error) {
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

	owed0 := cloneBig(pos.TokensOwed0)
	owed1 := cloneBig(pos.TokensOwed1)

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

	inside0 := feeGrowthInside(currentTick, pos.TickLower, pos.TickUpper, global0, lower0, upper0)
	inside1 := feeGrowthInside(currentTick, pos.TickLower, pos.TickUpper, global1, lower1, upper1)
	// 注意：由于 uint256 模运算特性和 RPC 调用时序差异，inside 可能暂时"看起来"大于 global。
	// 这里不再报错退出，而是继续计算。delta 计算已有负值保护，不会产生负手续费。

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

	// When using cache fallback, bubble up the last RPC error for optional UI warnings.
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

	now := time.Now()
	poolIDKey := normalizeV4PoolIDKey(poolID)
	key := strings.ToLower(stateView.Hex()) + "|" + strings.ToLower(poolManager.Hex()) + "|" + poolIDKey

	s.v4FeeMu.RLock()
	if c, ok := s.v4FeeCache[key]; ok && c.global0 != nil && c.global1 != nil && c.expires.After(now) {
		g0 := new(big.Int).Set(c.global0)
		g1 := new(big.Int).Set(c.global1)
		s.v4FeeMu.RUnlock()
		return g0, g1, false, 0, nil
	}

	var stale0 *big.Int
	var stale1 *big.Int
	var staleAt time.Time
	if c, ok := s.v4FeeCache[key]; ok && c.global0 != nil && c.global1 != nil {
		stale0 = new(big.Int).Set(c.global0)
		stale1 = new(big.Int).Set(c.global1)
		staleAt = c.updatedAt
	}
	s.v4FeeMu.RUnlock()

	g0, g1, err := blockchain.GetV4PoolFeeGrowthGlobals(stateView, poolManager, poolID)
	if err == nil && g0 != nil && g1 != nil {
		s.v4FeeMu.Lock()
		s.v4FeeCache[key] = cachedV4FeeGrowthGlobals{
			global0:   new(big.Int).Set(g0),
			global1:   new(big.Int).Set(g1),
			updatedAt: now,
			expires:   now.Add(2 * time.Second),
		}
		s.v4FeeMu.Unlock()
		return g0, g1, false, 0, nil
	}

	// Best-effort fallback (keeps UI usable when RPC is rate-limited).
	if stale0 != nil && stale1 != nil && !staleAt.IsZero() && now.Sub(staleAt) <= 30*time.Second {
		return stale0, stale1, true, now.Sub(staleAt), err
	}
	return nil, nil, false, 0, err
}

func (s *RealtimePositionsService) getV4TickFeeGrowthOutside(stateView common.Address, poolManager common.Address, poolID string, tick int) (*big.Int, *big.Int, bool, time.Duration, error) {
	if (stateView == common.Address{}) || (poolManager == common.Address{}) {
		return nil, nil, false, 0, fmt.Errorf("V4 stateView/poolManager missing")
	}

	now := time.Now()
	poolIDKey := normalizeV4PoolIDKey(poolID)
	key := strings.ToLower(stateView.Hex()) + "|" + strings.ToLower(poolManager.Hex()) + "|" + poolIDKey + "|" + strconv.Itoa(tick)

	s.v4TickFeeMu.RLock()
	if c, ok := s.v4TickFeeCache[key]; ok && c.fee0 != nil && c.fee1 != nil && c.expires.After(now) {
		f0 := new(big.Int).Set(c.fee0)
		f1 := new(big.Int).Set(c.fee1)
		s.v4TickFeeMu.RUnlock()
		return f0, f1, false, 0, nil
	}

	var stale0 *big.Int
	var stale1 *big.Int
	var staleAt time.Time
	if c, ok := s.v4TickFeeCache[key]; ok && c.fee0 != nil && c.fee1 != nil {
		stale0 = new(big.Int).Set(c.fee0)
		stale1 = new(big.Int).Set(c.fee1)
		staleAt = c.updatedAt
	}
	s.v4TickFeeMu.RUnlock()

	f0, f1, err := blockchain.GetV4TickFeeGrowthOutside(stateView, poolManager, poolID, tick)
	if err == nil && f0 != nil && f1 != nil {
		s.v4TickFeeMu.Lock()
		s.v4TickFeeCache[key] = cachedV4TickFeeGrowthOutside{
			fee0:      new(big.Int).Set(f0),
			fee1:      new(big.Int).Set(f1),
			updatedAt: now,
			expires:   now.Add(20 * time.Second),
		}
		s.v4TickFeeMu.Unlock()
		return f0, f1, false, 0, nil
	}

	// Best-effort fallback (keeps UI usable when RPC is rate-limited).
	if stale0 != nil && stale1 != nil && !staleAt.IsZero() && now.Sub(staleAt) <= 2*time.Minute {
		return stale0, stale1, true, now.Sub(staleAt), err
	}
	return nil, nil, false, 0, err
}

func (s *RealtimePositionsService) calcV4UnclaimedFeesCached(stateView common.Address, poolManager common.Address, poolID string, currentTick int, pos *blockchain.V4PositionInfo) (*big.Int, *big.Int, bool, time.Duration, error) {
	if pos == nil {
		return nil, nil, false, 0, fmt.Errorf("position info missing")
	}

	owed0 := cloneBig(pos.TokensOwed0)
	owed1 := cloneBig(pos.TokensOwed1)

	if pos.Liquidity == nil || pos.Liquidity.Sign() == 0 {
		return owed0, owed1, false, 0, nil
	}
	if pos.FeeGrowthInside0LastX128 == nil || pos.FeeGrowthInside1LastX128 == nil {
		return owed0, owed1, false, 0, fmt.Errorf("position feeGrowthInside last missing")
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
	// 注意：由于 uint256 模运算特性和 RPC 调用时序差异，inside 可能暂时"看起来"大于 global。
	// 这里不再报错退出，而是继续计算。delta 计算已有负值保护，不会产生负手续费。

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

func (s *RealtimePositionsService) getV4Slot0(stateView common.Address, poolManager common.Address, poolID string) (*big.Int, int, bool, time.Duration, error) {
	now := time.Now()
	poolIDKey := normalizeV4PoolIDKey(poolID)
	key := strings.ToLower(stateView.Hex()) + "|" + strings.ToLower(poolManager.Hex()) + "|" + poolIDKey

	s.v4Slot0Mu.RLock()
	if c, ok := s.v4Slot0[key]; ok && c.sqrtPriceX96 != nil && c.expires.After(now) {
		sqrt := new(big.Int).Set(c.sqrtPriceX96)
		tick := c.tick
		s.v4Slot0Mu.RUnlock()
		return sqrt, tick, false, 0, nil
	}
	var staleSqrt *big.Int
	var staleTick int
	var staleAt time.Time
	if c, ok := s.v4Slot0[key]; ok && c.sqrtPriceX96 != nil {
		staleSqrt = new(big.Int).Set(c.sqrtPriceX96)
		staleTick = c.tick
		staleAt = c.updatedAt
	}
	s.v4Slot0Mu.RUnlock()

	sqrt, tick, err := blockchain.GetUniswapV4PoolSlot0ViaStateView(stateView, poolManager, poolID)
	if err == nil && sqrt != nil {
		s.v4Slot0Mu.Lock()
		s.v4Slot0[key] = cachedV4Slot0{
			sqrtPriceX96: new(big.Int).Set(sqrt),
			tick:         tick,
			updatedAt:    now,
			expires:      now.Add(5 * time.Second),
		}
		s.v4Slot0Mu.Unlock()
		return sqrt, tick, false, 0, nil
	}

	// Best-effort fallback (keeps UI usable when RPC is rate-limited).
	if staleSqrt != nil && !staleAt.IsZero() && now.Sub(staleAt) <= 30*time.Second {
		return staleSqrt, staleTick, true, now.Sub(staleAt), err
	}
	return nil, 0, false, 0, err
}

func buildTokenRow(token common.Address, meta cachedTokenMeta, priceUSD float64, walletAmt, posAmt, feeAmt *big.Int) RealtimeTokenRow {
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
	if task == nil {
		return "0/0"
	}
	if task.Paused && (task.Status == models.StrategyStatusRunning || task.Status == models.StrategyStatusWaiting) {
		return "⏸"
	}
	threshold := task.ReopenDelaySeconds
	if currentTick != 0 && task.StopLossEnabled && task.StopLossDelaySeconds > 0 {
		_, _, _, priceDown := pricing.PriceDirectionFromTicks(task, tickLower, tickUpper, currentTick)
		if priceDown {
			threshold = task.StopLossDelaySeconds
		}
	}
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
	if strings.TrimSpace(task.ExitPendingAction) != "" {
		switch strings.TrimSpace(task.ExitPendingAction) {
		case strategy.ExitActionManualStop:
			return "停止中"
		case strategy.ExitActionStopLoss:
			return "止损中"
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
