package services

import (
	"TgLpBot/blockchain"
	"TgLpBot/config"
	"TgLpBot/database"
	"TgLpBot/models"
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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type RealtimePositionsService struct {
	walletService *WalletService
	taskService   *StrategyTaskService
	priceService  *TokenPriceService

	cacheMu sync.RWMutex
	cache   map[uint]cachedRealtimePositions

	tokenMetaMu sync.RWMutex
	tokenMeta   map[string]cachedTokenMeta

	balanceMu sync.RWMutex
	balance   map[string]cachedTokenBalance

	v3Slot0Mu sync.RWMutex
	v3Slot0   map[string]cachedV3Slot0

	v4Slot0Mu sync.RWMutex
	v4Slot0   map[string]cachedV4Slot0

	v4ScanMu    sync.RWMutex
	v4ScanCache map[string]cachedV4TokenIDs
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

type cachedV4Slot0 struct {
	sqrtPriceX96 *big.Int
	tick         int
	updatedAt    time.Time
	expires      time.Time
}

func NewRealtimePositionsService() *RealtimePositionsService {
	return &RealtimePositionsService{
		walletService: NewWalletService(),
		taskService:   NewStrategyTaskService(),
		priceService:  NewTokenPriceService(),
		cache:         make(map[uint]cachedRealtimePositions),
		tokenMeta:     make(map[string]cachedTokenMeta),
		balance:       make(map[string]cachedTokenBalance),
		v3Slot0:       make(map[string]cachedV3Slot0),
		v4Slot0:       make(map[string]cachedV4Slot0),
		v4ScanCache:   make(map[string]cachedV4TokenIDs),
	}
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
	Version      string     `json:"version"`
	Exchange     string     `json:"exchange"`
	Title        string     `json:"title"`
	PoolID       string     `json:"pool_id"`
	PositionID   string     `json:"position_id"`
	StatusLabel  string     `json:"status_label"`
	InRange      bool       `json:"in_range"`
	CurrentTick  int        `json:"current_tick"`
	TickLower    int        `json:"tick_lower"`
	TickUpper    int        `json:"tick_upper"`
	RangePercent float64    `json:"range_percent"`
	OutOfRange   string     `json:"out_of_range"`
	RunningSince *time.Time `json:"running_since,omitempty"`
	HasLiquidity bool       `json:"has_liquidity"`

	TokenRows []RealtimeTokenRow `json:"token_rows"`
	Totals    RealtimeTotals     `json:"totals"`
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

	resp, err := s.compute(userID)
	if err != nil {
		return nil, err
	}

	s.cacheMu.Lock()
	s.cache[userID] = cachedRealtimePositions{resp: resp, expires: now.Add(3 * time.Second)}
	s.cacheMu.Unlock()
	return resp, nil
}

func (s *RealtimePositionsService) compute(userID uint) (*RealtimePositionsResponse, error) {
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
		if usdtBal := s.getWalletTokenBalance(usdtAddr, walletAddr); usdtBal != nil && usdtBal.Sign() > 0 {
			meta := s.getTokenMeta(usdtAddr)
			prices, _ := s.priceService.GetUSDPrices("bsc", []string{usdtAddr.Hex()})
			price := prices[strings.ToLower(usdtAddr.Hex())]
			usd := toFloat(usdtBal, meta.decimals) * price
			if usd > 0 {
				extraWalletTokenUSD[strings.ToLower(usdtAddr.Hex())] = usd
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

	// Preload tasks for display enhancements (range %, out-of-range timer, etc.)
	var tasks []models.StrategyTask
	if err := database.DB.Where("user_id = ? AND v3_token_id <> ''", userID).Find(&tasks).Error; err == nil {
		for _, t := range tasks {
			tokenKey := strings.TrimSpace(t.V3TokenID)
			if tokenKey == "" {
				continue
			}
			key := tokenKey + "|" + strings.ToLower(strings.TrimSpace(t.V3PositionManagerAddress))
			taskByV3Token[key] = t
			if prev, ok := taskByV3TokenID[tokenKey]; !ok || t.UpdatedAt.After(prev.UpdatedAt) {
				taskByV3TokenID[tokenKey] = t
			}
		}
	} else {
		resp.Warnings = append(resp.Warnings, "查询任务信息失败（将仅展示链上数据）")
	}

	var v4Tasks []models.StrategyTask
	if err := database.DB.Where("user_id = ? AND v4_token_id <> ''", userID).Find(&v4Tasks).Error; err == nil {
		for _, t := range v4Tasks {
			key := strings.TrimSpace(t.V4TokenID)
			taskByV4Token[key] = t
		}
	}

	positions := make([]RealtimePosition, 0, 8)

	// 1) V3 positions via NPM balanceOf/tokenOfOwnerByIndex
	npmAddrs := []string{
		strings.TrimSpace(config.AppConfig.PancakeV3PositionManagerAddress),
		strings.TrimSpace(config.AppConfig.UniswapV3PositionManagerAddress),
	}
	for _, npmStr := range npmAddrs {
		if !common.IsHexAddress(npmStr) {
			continue
		}
		npmAddr := common.HexToAddress(npmStr)
		pm, err := blockchain.NewV3PositionManager(npmAddr, blockchain.Client)
		if err != nil {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("初始化 V3 PositionManager 失败: %s", npmAddr.Hex()))
			continue
		}

		bal, err := pm.BalanceOf(nil, walletAddr)
		if err != nil || bal == nil || bal.Sign() == 0 {
			continue
		}
		max := bal.Int64()
		if max > 50 {
			max = 50
		}
		for i := int64(0); i < max; i++ {
			tokenId, err := pm.TokenOfOwnerByIndex(nil, walletAddr, big.NewInt(i))
			if err != nil || tokenId == nil || tokenId.Sign() == 0 {
				continue
			}
			p, warn := s.buildV3Position(pm, npmAddr, walletAddr, tokenId, taskByV3Token, taskByV3TokenID)
			if warn != "" {
				resp.Warnings = append(resp.Warnings, warn)
			}
			if p != nil {
				positions = append(positions, *p)
			}
		}
	}

	// 2) V4 positions (best-effort): prefer DB tasks because on-chain tokenId->range mapping is not always enumerable.
	ownedV4 := make(map[string]struct{})
	if config.AppConfig.V4NFTScanFromBlock > 0 && common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		ids, err := s.getV4OwnedTokenIDs(walletAddr)
		if err != nil {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("V4 NFT 扫描失败（将直接展示任务列表）: %v", err))
		} else if len(ids) > 0 {
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

	sort.Slice(positions, func(i, j int) bool {
		return positions[i].Title < positions[j].Title
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
			if prev, ok := walletTokenUSD[addr]; !ok || row.WalletUSD > prev {
				walletTokenUSD[addr] = row.WalletUSD
			}
		}
	}
	for addr, usd := range extraWalletTokenUSD {
		if usd <= 0 {
			continue
		}
		if prev, ok := walletTokenUSD[addr]; !ok || usd > prev {
			walletTokenUSD[addr] = usd
		}
	}
	for _, v := range walletTokenUSD {
		summary.WalletUSD += v
	}
	summary.WalletUSD += bnbUSD
	summary.TotalUSD = summary.WalletUSD + summary.PositionUSD + summary.FeeUSD
	resp.Summary = summary

	return resp, nil
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
	pm *blockchain.V3PositionManager,
	npmAddr common.Address,
	walletAddr common.Address,
	tokenId *big.Int,
	taskByV3Token map[string]models.StrategyTask,
	taskByV3TokenID map[string]models.StrategyTask,
) (*RealtimePosition, string) {
	var warn string

	info, err := pm.Positions(nil, tokenId)
	if err != nil || info == nil {
		return nil, fmt.Sprintf("读取 V3 positions() 失败: tokenId=%s err=%v", tokenId.String(), err)
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

	meta0 := s.getTokenMeta(token0)
	meta1 := s.getTokenMeta(token1)

	// Try find matching task for richer fields.
	taskKey := strings.TrimSpace(tokenId.String()) + "|" + strings.ToLower(strings.TrimSpace(npmAddr.Hex()))
	var task *models.StrategyTask
	if t, ok := taskByV3Token[taskKey]; ok {
		task = &t
	} else if t, ok := taskByV3TokenID[strings.TrimSpace(tokenId.String())]; ok {
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
		if task.RangePercentage > 0 {
			rangePct = task.RangePercentage
		}
		runningSince = &task.CreatedAt
		statusLabel = statusLabelFromTask(task)
	}

	// Get tick and sqrtP (poolID required). If missing, we still return a card but tick/amounts may be 0.
	currentTick := 0
	var sqrtP *big.Int
	hasSlot0 := false
	if poolID != "" && common.IsHexAddress(poolID) {
		poolAddr := common.HexToAddress(poolID)
		sp, t, usedStale, age, err := s.getV3Slot0(poolAddr)
		if err != nil && sp == nil {
			warn = fmt.Sprintf("读取 V3 Pool slot0 失败: pool=%s tokenId=%s err=%v", poolID, tokenId.String(), err)
		} else {
			sqrtP = sp
			currentTick = t
			hasSlot0 = true
			if usedStale && err != nil {
				warn = fmt.Sprintf("V3 slot0 RPC 限流/失败，已使用缓存（%ds 前）。建议调大自动刷新或更换 BSC RPC：tokenId=%s", int(age.Seconds()), tokenId.String())
			}
		}
	} else {
		// Best-effort pool resolve from Pancake V3 factory (covers most BSC V3 LPs).
		pancakeFactory := common.HexToAddress("0x0BFbcf9fa4f9C56B0F40a671Ad40E0805A091865")
		if pool, err := blockchain.GetV3PoolFromFactory(pancakeFactory, token0, token1, info.Fee); err == nil && pool != (common.Address{}) {
			poolID = pool.Hex()
			sp, t, usedStale, age, err := s.getV3Slot0(pool)
			if err == nil || sp != nil {
				sqrtP = sp
				currentTick = t
				hasSlot0 = true
				if exchange == "V3" || strings.TrimSpace(exchange) == "" {
					exchange = "PancakeSwap V3"
				}
				if usedStale && err != nil {
					warn = fmt.Sprintf("V3 slot0 RPC 限流/失败，已使用缓存（%ds 前）。建议调大自动刷新或更换 BSC RPC：tokenId=%s", int(age.Seconds()), tokenId.String())
				}
			}
		}
		if sqrtP == nil {
			warn = fmt.Sprintf("V3 tokenId=%s 未找到 pool 地址（将无法计算 tick/仓位数量）", tokenId.String())
		}
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

	if hasSlot0 && poolID != "" && common.IsHexAddress(poolID) {
		poolAddr := common.HexToAddress(poolID)
		if fee0, fee1, feeErr := calcV3UnclaimedFees(poolAddr, currentTick, info); feeErr == nil {
			owed0 = fee0
			owed1 = fee1
		} else {
			msg := fmt.Sprintf("V3 手续费计算失败: tokenId=%s err=%v", tokenId.String(), feeErr)
			if warn == "" {
				warn = msg
			} else {
				warn = warn + "; " + msg
			}
		}
	}

	// Compute amounts in position
	sqrtA, _ := SqrtRatioAtTick(int32(tickLower))
	sqrtB, _ := SqrtRatioAtTick(int32(tickUpper))
	amt0Raw, amt1Raw := AmountsForLiquidity(sqrtP, sqrtA, sqrtB, liq)

	// Wallet balances
	w0 := s.getWalletTokenBalance(token0, walletAddr)
	w1 := s.getWalletTokenBalance(token1, walletAddr)

	// Prices
	prices, _ := s.priceService.GetUSDPrices("bsc", []string{token0.Hex(), token1.Hex()})
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
	return &RealtimePosition{
		Version:      "v3",
		Exchange:     exchange,
		Title:        title,
		PoolID:       poolID,
		PositionID:   tokenId.String(),
		StatusLabel:  statusLabel,
		InRange:      inRange,
		CurrentTick:  currentTick,
		TickLower:    tickLower,
		TickUpper:    tickUpper,
		RangePercent: rangePct,
		OutOfRange:   outOfRangeText,
		RunningSince: runningSince,
		HasLiquidity: hasLiquidity,
		TokenRows:    []RealtimeTokenRow{row0, row1},
		Totals:       totals,
	}, warn
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

	liq, ok := new(big.Int).SetString(strings.TrimSpace(task.CurrentLiquidity), 10)
	if !ok || liq == nil || liq.Sign() == 0 {
		// Ignore empty positions (stale tasks / already exited).
		return nil, ""
	}

	var warn string
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
	if common.IsHexAddress(config.AppConfig.UniswapV4PositionManagerAddress) {
		tokenID, parseErr := parseBigInt(tokenId)
		if parseErr == nil && tokenID.Sign() > 0 {
			v4pmAddr := common.HexToAddress(config.AppConfig.UniswapV4PositionManagerAddress)
			pos, posErr := blockchain.GetV4PositionInfo(v4pmAddr, poolManager, task.PoolId, tokenID)
			if posErr != nil {
				log.Printf("[Realtime] V4 position info read failed: tokenId=%s err=%v", tokenId, posErr)
			}
			if pos != nil {
				if pos.Liquidity != nil && pos.Liquidity.Sign() > 0 {
					liq = pos.Liquidity
				}
				if pos.TickLower == 0 && pos.TickUpper == 0 {
					pos.TickLower = task.TickLower
					pos.TickUpper = task.TickUpper
				}
				if fee0, fee1, feeErr := calcV4UnclaimedFees(task.PoolId, currentTick, pos); feeErr == nil {
					owed0 = fee0
					owed1 = fee1
				}
			}
		}
	}

	sqrtA, _ := SqrtRatioAtTick(int32(task.TickLower))
	sqrtB, _ := SqrtRatioAtTick(int32(task.TickUpper))
	amt0Raw, amt1Raw := AmountsForLiquidity(sqrtP, sqrtA, sqrtB, liq)

	w0 := s.getWalletTokenBalance(c0, walletAddr)
	w1 := s.getWalletTokenBalance(c1, walletAddr)

	meta0 := s.getTokenMeta(c0)
	meta1 := s.getTokenMeta(c1)

	prices, _ := s.priceService.GetUSDPrices("bsc", []string{c0.Hex(), c1.Hex()})
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

	inRange := currentTick >= task.TickLower && currentTick <= task.TickUpper
	rangePct := task.RangePercentage
	if rangePct <= 0 {
		rangePct = estimateRangePercent(currentTick, task.TickLower, task.TickUpper)
	}

	exchange := strings.TrimSpace(task.Exchange)
	if exchange == "" {
		exchange = "Uniswap V4"
	}
	title := fmt.Sprintf("%s-%s-%s-%.2f%%", exchangeShort(exchange, "UniV4"), row0.Symbol, row1.Symbol, float64(task.Fee)/10000.0)

	hasLiquidity := liq != nil && liq.Sign() > 0
	return &RealtimePosition{
		Version:      "v4",
		Exchange:     exchange,
		Title:        title,
		PoolID:       strings.TrimSpace(task.PoolId),
		PositionID:   tokenId,
		StatusLabel:  "运行中",
		InRange:      inRange,
		CurrentTick:  currentTick,
		TickLower:    task.TickLower,
		TickUpper:    task.TickUpper,
		RangePercent: rangePct,
		OutOfRange:   formatOutOfRange(task, task.TickLower, task.TickUpper, currentTick),
		RunningSince: &task.CreatedAt,
		HasLiquidity: hasLiquidity,
		TokenRows:    []RealtimeTokenRow{row0, row1},
		Totals:       totals,
	}, warn
}

func (s *RealtimePositionsService) getTokenMeta(addr common.Address) cachedTokenMeta {
	key := strings.ToLower(addr.Hex())
	now := time.Now()
	s.tokenMetaMu.RLock()
	if m, ok := s.tokenMeta[key]; ok && m.expires.After(now) {
		s.tokenMetaMu.RUnlock()
		return m
	}
	s.tokenMetaMu.RUnlock()

	symbol := ""
	decimals := defaultTokenDecimals
	ttl := 24 * time.Hour

	if blockchain.Client == nil {
		symbol = addr.Hex()
		ttl = 5 * time.Minute
	} else {
		if sym, err := blockchain.GetTokenSymbol(addr); err == nil && strings.TrimSpace(sym) != "" {
			symbol = strings.TrimSpace(sym)
		} else {
			symbol = addr.Hex()
			ttl = 5 * time.Minute
		}
		if dec, err := blockchain.GetTokenDecimals(addr); err == nil && dec > 0 {
			decimals = int(dec)
		} else {
			decimals = defaultTokenDecimals
			ttl = 5 * time.Minute
		}
	}

	m := cachedTokenMeta{symbol: symbol, decimals: decimals, expires: now.Add(ttl)}
	s.tokenMetaMu.Lock()
	s.tokenMeta[key] = m
	s.tokenMetaMu.Unlock()
	return m
}

func (s *RealtimePositionsService) getWalletTokenBalance(tokenAddress, walletAddress common.Address) *big.Int {
	if (tokenAddress == common.Address{}) || (walletAddress == common.Address{}) {
		return big.NewInt(0)
	}

	now := time.Now()
	key := strings.ToLower(walletAddress.Hex()) + "|" + strings.ToLower(tokenAddress.Hex())

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

	bal, err := blockchain.GetTokenBalance(tokenAddress, walletAddress)
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

func (s *RealtimePositionsService) getV3Slot0(poolAddress common.Address) (*big.Int, int, bool, time.Duration, error) {
	if (poolAddress == common.Address{}) {
		return nil, 0, false, 0, fmt.Errorf("empty pool address")
	}

	now := time.Now()
	key := strings.ToLower(poolAddress.Hex())

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

	sqrt, tick, err := blockchain.GetV3PoolSlot0(poolAddress)
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

func (s *RealtimePositionsService) getV4Slot0(stateView common.Address, poolManager common.Address, poolID string) (*big.Int, int, bool, time.Duration, error) {
	now := time.Now()
	poolIDKey := strings.ToLower(strings.TrimSpace(poolID))
	if poolIDKey != "" && !strings.HasPrefix(poolIDKey, "0x") {
		poolIDKey = "0x" + poolIDKey
	}
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
		FeeAmount:      fmt.Sprintf("%.2f", f),
		FeeUSD:         f * priceUSD,
	}
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
	threshold := task.ReopenDelaySeconds
	if currentTick != 0 && task.StopLossEnabled && task.StopLossDelaySeconds > 0 {
		_, _, _, priceDown := priceDirectionFromTicks(task, tickLower, tickUpper, currentTick)
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
