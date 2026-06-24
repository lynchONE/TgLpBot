package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	sm "TgLpBot/service/smart_money"
	"context"
	"math"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type smartMoneyPoolMarkerEvent struct {
	EventID                 string   `json:"event_id"`
	T                       int64    `json:"t"`
	BucketT                 int64    `json:"bucket_t"`
	WalletAddress           string   `json:"wallet_address"`
	WalletLabel             string   `json:"wallet_label,omitempty"`
	WalletAvatarURL         string   `json:"wallet_avatar_url,omitempty"`
	WalletSource            string   `json:"wallet_source,omitempty"`
	WalletSourceContract    string   `json:"wallet_source_contract,omitempty"`
	WalletColor             string   `json:"wallet_color,omitempty"`
	Action                  string   `json:"action"`
	TxHash                  string   `json:"tx_hash,omitempty"`
	TxURL                   string   `json:"tx_url,omitempty"`
	TickLower               *int     `json:"tick_lower,omitempty"`
	TickUpper               *int     `json:"tick_upper,omitempty"`
	PriceLower              float64  `json:"price_lower,omitempty"`
	PriceUpper              float64  `json:"price_upper,omitempty"`
	RangePercent            float64  `json:"range_percent,omitempty"`
	MidPrice                float64  `json:"mid_price,omitempty"`
	AnchorPrice             float64  `json:"anchor_price,omitempty"`
	EstimatedUSD            float64  `json:"estimated_usd"`
	MatchedOpenTxHash       string   `json:"matched_open_tx_hash,omitempty"`
	MatchedOpenT            *int64   `json:"matched_open_t,omitempty"`
	EstimatedCostUSD        *float64 `json:"estimated_cost_usd,omitempty"`
	EstimatedRealizedPnlUSD *float64 `json:"estimated_realized_pnl_usd,omitempty"`
	EstimatedRealizedPnlPct *float64 `json:"estimated_realized_pnl_pct,omitempty"`
}

type smartMoneyPoolMarkersEnvelope struct {
	Chain       string                      `json:"chain"`
	PoolVersion string                      `json:"pool_version,omitempty"`
	PoolID      string                      `json:"pool_id"`
	BucketSec   int                         `json:"bucket_sec"`
	WindowSec   int                         `json:"window_sec"`
	UpdatedAt   time.Time                   `json:"updated_at"`
	Events      []smartMoneyPoolMarkerEvent `json:"events"`
	Warnings    []string                    `json:"warnings,omitempty"`
}

type smartMoneyMarkerEstimate struct {
	MatchedOpenTxHash       string
	MatchedOpenT            int64
	EstimatedCostUSD        float64
	EstimatedRealizedPnlUSD float64
	EstimatedRealizedPnlPct *float64
}

type smartMoneyMarkerReplayState struct {
	Liquidity         *big.Int
	CostUSD           float64
	CostReliable      bool
	FirstOpenTxHash   string
	FirstOpenT        int64
	FallbackOpenEvent *models.SmartMoneyLPEvent
	FallbackAmbiguous bool
}

const smartMoneyMarkerEstimateLookback = 14 * 24 * time.Hour
const smartMoneyMarkerEstimateHistoryLimit = 5000

func normalizeSmartMoneyPoolID(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		value = value[2:]
	}
	if len(value) != 40 && len(value) != 64 {
		return ""
	}
	for _, ch := range value {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return ""
		}
	}
	return "0x" + strings.ToLower(value)
}

func parseMarkerQueryInt(raw string, fallback int) int {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func decimalStringToFloat(value *string) float64 {
	if value == nil {
		return 0
	}
	raw := strings.TrimSpace(*value)
	if raw == "" {
		return 0
	}
	num, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return sanitizeFloat(num)
}

func poolVersionProtocols(version string) []string {
	switch strings.ToLower(strings.TrimSpace(version)) {
	case "v3":
		return []string{"pancake_v3", "uniswap_v3"}
	case "v4":
		return []string{"uniswap_v4"}
	default:
		return nil
	}
}

func scanExplorerBase(chain string) string {
	if config.NormalizeChain(chain) == "base" {
		return "https://basescan.org"
	}
	return "https://bscscan.com"
}

func smartMoneyMarkerEventID(event *models.SmartMoneyLPEvent) string {
	if event == nil {
		return ""
	}
	hash := strings.ToLower(strings.TrimSpace(event.TxHash))
	if hash == "" {
		return ""
	}
	return hash + ":" + strconv.Itoa(event.LogIndex)
}

func smartMoneyMarkerPositionKey(event *models.SmartMoneyLPEvent) string {
	if event == nil {
		return ""
	}
	wallet := strings.ToLower(strings.TrimSpace(event.WalletAddress))
	pool := strings.ToLower(strings.TrimSpace(event.PoolAddress))
	if wallet == "" || pool == "" {
		return ""
	}
	if event.NftTokenID != nil && *event.NftTokenID > 0 {
		return wallet + "|" + pool + "|nft|" + strconv.FormatUint(*event.NftTokenID, 10)
	}
	if event.TickLower != nil && event.TickUpper != nil {
		return wallet + "|" + pool + "|range|" + strconv.Itoa(*event.TickLower) + "|" + strconv.Itoa(*event.TickUpper)
	}
	return ""
}

func smartMoneyMarkerEventUSD(event *models.SmartMoneyLPEvent) float64 {
	total := decimalStringToFloat(event.TotalUSD)
	if total > 0 {
		return total
	}
	total = decimalStringToFloat(event.Token0AmountUSD) + decimalStringToFloat(event.Token1AmountUSD)
	return sanitizeFloat(total)
}

func smartMoneyMarkerRoundUSD(value float64) float64 {
	return math.Round(sanitizeFloat(value)*100) / 100
}

func smartMoneyMarkerRoundPercent(value float64) float64 {
	return math.Round(sanitizeFloat(value)*100) / 100
}

func smartMoneyMarkerLiquidityDelta(event *models.SmartMoneyLPEvent) (*big.Int, bool) {
	if event == nil {
		return nil, false
	}
	raw := strings.TrimSpace(event.LiquidityDelta)
	if raw == "" {
		return nil, false
	}
	value, ok := new(big.Int).SetString(raw, 10)
	if !ok || value.Sign() == 0 {
		return nil, false
	}
	return value, true
}

func smartMoneyMarkerRatio(numerator *big.Int, denominator *big.Int) float64 {
	if numerator == nil || denominator == nil || numerator.Sign() <= 0 || denominator.Sign() <= 0 {
		return 0
	}
	if numerator.Cmp(denominator) >= 0 {
		return 1
	}
	value, _ := new(big.Rat).SetFrac(numerator, denominator).Float64()
	return sanitizeFloat(value)
}

func smartMoneyMarkerResetInventory(state *smartMoneyMarkerReplayState) {
	if state == nil {
		return
	}
	state.Liquidity = big.NewInt(0)
	state.CostUSD = 0
	state.CostReliable = true
	state.FirstOpenTxHash = ""
	state.FirstOpenT = 0
}

func replaySmartMoneyMarkerEstimates(
	historyEvents []models.SmartMoneyLPEvent,
	targetKeys map[string]struct{},
) (map[string]smartMoneyMarkerEstimate, []string) {
	estimates := make(map[string]smartMoneyMarkerEstimate)
	if len(historyEvents) == 0 || len(targetKeys) == 0 {
		return estimates, nil
	}

	states := make(map[string]*smartMoneyMarkerReplayState)
	warningSet := make(map[string]struct{})
	appendWarning := func(message string) {
		message = strings.TrimSpace(message)
		if message == "" {
			return
		}
		warningSet[message] = struct{}{}
	}

	for i := range historyEvents {
		event := &historyEvents[i]
		key := smartMoneyMarkerPositionKey(event)
		if key == "" {
			continue
		}
		if _, ok := targetKeys[key]; !ok {
			continue
		}

		state := states[key]
		if state == nil {
			state = &smartMoneyMarkerReplayState{Liquidity: big.NewInt(0), CostReliable: true}
			states[key] = state
		}

		action := strings.ToLower(strings.TrimSpace(event.EventType))
		eventUSD := smartMoneyMarkerEventUSD(event)
		liquidityDelta, hasLiquidityDelta := smartMoneyMarkerLiquidityDelta(event)

		if action == "add" {
			if hasLiquidityDelta && liquidityDelta.Sign() > 0 {
				if state.Liquidity == nil {
					state.Liquidity = big.NewInt(0)
				}
				state.Liquidity.Add(state.Liquidity, liquidityDelta)
				if state.FirstOpenTxHash == "" {
					state.FirstOpenTxHash = strings.TrimSpace(event.TxHash)
					state.FirstOpenT = event.TxTimestamp.Unix()
				}
				if eventUSD <= 0 {
					state.CostReliable = false
					appendWarning("smart money remove pnl unavailable for some positions because add-event usd snapshots are missing")
				} else {
					state.CostUSD += eventUSD
				}
				continue
			}

			if state.FallbackOpenEvent != nil {
				state.FallbackAmbiguous = true
				appendWarning("smart money remove pnl unavailable for some positions because the same position has multiple add events before closing and liquidity deltas are missing")
				continue
			}
			openEvent := *event
			state.FallbackOpenEvent = &openEvent
			if eventUSD <= 0 {
				state.FallbackAmbiguous = true
				appendWarning("smart money remove pnl unavailable for some positions because add-event usd snapshots are missing")
			}
			continue
		}

		if action != "remove" {
			continue
		}

		if hasLiquidityDelta && liquidityDelta.Sign() < 0 {
			removeLiquidity := new(big.Int).Abs(liquidityDelta)
			if state.Liquidity == nil || state.Liquidity.Sign() <= 0 {
				appendWarning("smart money remove pnl unavailable for some positions because matched add liquidity is outside the loaded history window")
				continue
			}
			beforeLiquidity := new(big.Int).Set(state.Liquidity)
			ratio := smartMoneyMarkerRatio(removeLiquidity, beforeLiquidity)
			removedCostUSD := state.CostUSD * ratio
			if !state.CostReliable {
				appendWarning("smart money remove pnl unavailable for some positions because earlier add-event usd snapshots are missing")
			} else if eventUSD <= 0 {
				appendWarning("smart money remove pnl unavailable for some positions because remove-event usd snapshots are missing")
			} else if state.CostUSD <= 0 {
				appendWarning("smart money remove pnl unavailable for some positions because matched add-event cost is missing")
			} else {
				costUSD := smartMoneyMarkerRoundUSD(removedCostUSD)
				pnlUSD := smartMoneyMarkerRoundUSD(eventUSD - costUSD)
				estimate := smartMoneyMarkerEstimate{
					MatchedOpenTxHash:       state.FirstOpenTxHash,
					MatchedOpenT:            state.FirstOpenT,
					EstimatedCostUSD:        costUSD,
					EstimatedRealizedPnlUSD: pnlUSD,
				}
				if costUSD > 0 {
					pct := smartMoneyMarkerRoundPercent((pnlUSD / costUSD) * 100)
					estimate.EstimatedRealizedPnlPct = &pct
				}
				estimates[smartMoneyMarkerEventID(event)] = estimate
			}

			if removeLiquidity.Cmp(beforeLiquidity) >= 0 {
				smartMoneyMarkerResetInventory(state)
			} else {
				state.Liquidity.Sub(state.Liquidity, removeLiquidity)
				state.CostUSD -= removedCostUSD
				if state.CostUSD < 0 {
					state.CostUSD = 0
				}
			}
			continue
		}

		if state.FallbackOpenEvent == nil {
			state.FallbackAmbiguous = false
			continue
		}

		openUSD := smartMoneyMarkerEventUSD(state.FallbackOpenEvent)
		if state.FallbackAmbiguous || openUSD <= 0 || eventUSD <= 0 {
			if eventUSD <= 0 {
				appendWarning("smart money remove pnl unavailable for some positions because remove-event usd snapshots are missing")
			}
			if openUSD <= 0 {
				appendWarning("smart money remove pnl unavailable for some positions because add-event usd snapshots are missing")
			}
		} else {
			costUSD := smartMoneyMarkerRoundUSD(openUSD)
			pnlUSD := smartMoneyMarkerRoundUSD(eventUSD - openUSD)
			estimate := smartMoneyMarkerEstimate{
				MatchedOpenTxHash:       strings.TrimSpace(state.FallbackOpenEvent.TxHash),
				MatchedOpenT:            state.FallbackOpenEvent.TxTimestamp.Unix(),
				EstimatedCostUSD:        costUSD,
				EstimatedRealizedPnlUSD: pnlUSD,
			}
			if costUSD > 0 {
				pct := smartMoneyMarkerRoundPercent((pnlUSD / costUSD) * 100)
				estimate.EstimatedRealizedPnlPct = &pct
			}
			estimates[smartMoneyMarkerEventID(event)] = estimate
		}

		state.FallbackOpenEvent = nil
		state.FallbackAmbiguous = false
	}

	warnings := make([]string, 0, len(warningSet))
	for message := range warningSet {
		warnings = append(warnings, message)
	}
	sort.Strings(warnings)
	return estimates, warnings
}

func buildSmartMoneyMarkerEstimates(
	ctx context.Context,
	chainID int,
	poolID string,
	poolVersion string,
	queryStart time.Time,
	queryEnd time.Time,
	visibleEvents []models.SmartMoneyLPEvent,
) (map[string]smartMoneyMarkerEstimate, []string) {
	targetKeys := make(map[string]struct{})
	walletSeen := make(map[string]struct{})
	wallets := make([]string, 0)

	for i := range visibleEvents {
		event := &visibleEvents[i]
		if !strings.EqualFold(event.EventType, "remove") {
			continue
		}
		key := smartMoneyMarkerPositionKey(event)
		if key != "" {
			targetKeys[key] = struct{}{}
		}
		wallet := strings.ToLower(strings.TrimSpace(event.WalletAddress))
		if wallet == "" {
			continue
		}
		if _, ok := walletSeen[wallet]; ok {
			continue
		}
		walletSeen[wallet] = struct{}{}
		wallets = append(wallets, wallet)
	}

	if len(targetKeys) == 0 || len(wallets) == 0 {
		return nil, nil
	}

	historyStart := queryStart.Add(-smartMoneyMarkerEstimateLookback)
	var historyEvents []models.SmartMoneyLPEvent
	db := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyLPEvent{}).
		Where("chain_id = ? AND pool_address = ?", chainID, poolID).
		Where("tx_timestamp BETWEEN ? AND ?", historyStart, queryEnd).
		Where("wallet_address IN ?", wallets).
		Where("event_type IN ?", []string{"add", "remove"})
	if protocols := poolVersionProtocols(poolVersion); len(protocols) > 0 {
		db = db.Where("protocol IN ?", protocols)
	}
	if err := db.
		Order("tx_timestamp ASC").
		Order("block_number ASC").
		Order("log_index ASC").
		Limit(smartMoneyMarkerEstimateHistoryLimit + 1).
		Find(&historyEvents).Error; err != nil {
		return nil, []string{"smart money remove pnl unavailable: failed to load historical position events"}
	}
	if len(historyEvents) == 0 {
		return nil, nil
	}
	if len(historyEvents) > smartMoneyMarkerEstimateHistoryLimit {
		return nil, []string{"smart money remove pnl unavailable: historical position events exceeded safety limit"}
	}

	estimates, warnings := replaySmartMoneyMarkerEstimates(historyEvents, targetKeys)
	return estimates, warnings
}

func loadSmartMoneyMarkerWallets(ctx context.Context, chainID int, events []models.SmartMoneyLPEvent) map[string]*models.MonitoredWallet {
	out := make(map[string]*models.MonitoredWallet)
	if len(events) == 0 {
		return out
	}
	seen := make(map[string]struct{})
	addresses := make([]string, 0, len(events))
	for i := range events {
		wallet := strings.ToLower(strings.TrimSpace(events[i].WalletAddress))
		if wallet == "" {
			continue
		}
		if _, ok := seen[wallet]; ok {
			continue
		}
		seen[wallet] = struct{}{}
		addresses = append(addresses, wallet)
		out[wallet] = nil
	}
	if len(addresses) == 0 {
		return out
	}

	var wallets []models.MonitoredWallet
	if err := database.DB.WithContext(ctx).
		Model(&models.MonitoredWallet{}).
		Where("chain_id = ? AND address IN ?", chainID, addresses).
		Find(&wallets).Error; err != nil {
		return out
	}
	for i := range wallets {
		wallet := wallets[i]
		addr := strings.ToLower(strings.TrimSpace(wallet.Address))
		if addr == "" {
			continue
		}
		out[addr] = &wallet
	}
	return out
}

func (s *Server) handleSmartMoneyPoolMarkers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	initData := initDataFromQuery(r)
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
	if status, msg := requireModulePermission(check, models.AccessModuleSmartMoney); status != 0 {
		http.Error(w, msg, status)
		return
	}

	query := r.URL.Query()
	chain := config.NormalizeChain(query.Get("chain"))
	if chain == "" {
		chain = config.PickEnabledChain("bsc")
	}
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok || cc.ChainID <= 0 {
		http.Error(w, "invalid chain", http.StatusBadRequest)
		return
	}

	poolID := normalizeSmartMoneyPoolID(query.Get("pool_id"))
	if poolID == "" {
		poolID = normalizeSmartMoneyPoolID(query.Get("pool"))
	}
	if poolID == "" {
		http.Error(w, "pool_id required", http.StatusBadRequest)
		return
	}

	poolVersion := strings.ToLower(strings.TrimSpace(query.Get("pool_version")))

	bucketSec := parseMarkerQueryInt(query.Get("bucket_sec"), 300)
	if bucketSec < 60 {
		bucketSec = 60
	}
	if bucketSec > 86400 {
		bucketSec = 86400
	}

	windowHours := parseMarkerQueryInt(query.Get("window_hours"), 12)
	if windowHours <= 0 {
		windowHours = 12
	}
	if windowHours > 48 {
		windowHours = 48
	}

	limit := parseMarkerQueryInt(query.Get("limit"), 300)
	if limit <= 0 {
		limit = 300
	}
	if limit > 500 {
		limit = 500
	}

	startTS := parseUnixSecondsQuery(query, "start_ts")
	endTS := parseUnixSecondsQuery(query, "end_ts")
	rangeStart, rangeEnd := resolveUnixTimeRange(startTS, endTS, time.Duration(windowHours)*time.Hour)
	queryStart := rangeStart.Add(-time.Duration(bucketSec) * time.Second)
	queryEnd := rangeEnd.Add(time.Duration(bucketSec) * time.Second)

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	var events []models.SmartMoneyLPEvent
	db := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyLPEvent{}).
		Where("chain_id = ? AND pool_address = ?", cc.ChainID, poolID).
		Where("tx_timestamp BETWEEN ? AND ?", queryStart, queryEnd)
	if protocols := poolVersionProtocols(poolVersion); len(protocols) > 0 {
		db = db.Where("protocol IN ?", protocols)
	}
	if err := db.
		Order("tx_timestamp DESC").
		Order("id DESC").
		Limit(limit).
		Find(&events).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].TxTimestamp.Equal(events[j].TxTimestamp) {
			if events[i].BlockNumber == events[j].BlockNumber {
				return events[i].LogIndex < events[j].LogIndex
			}
			return events[i].BlockNumber < events[j].BlockNumber
		}
		return events[i].TxTimestamp.Before(events[j].TxTimestamp)
	})

	var poolMeta models.Pool
	if err := database.DB.WithContext(ctx).
		Model(&models.Pool{}).
		Where("address = ?", poolID).
		First(&poolMeta).Error; err != nil {
		poolMeta = models.Pool{}
	}

	token0Address := smartMoneyNormalizeTokenAddress(poolMeta.BaseTokenID)
	token1Address := smartMoneyNormalizeTokenAddress(poolMeta.QuoteTokenID)
	token0Symbol := strings.TrimSpace(poolMeta.Token0Symbol)
	token1Symbol := strings.TrimSpace(poolMeta.Token1Symbol)
	if len(events) > 0 {
		if token0Address == "" {
			token0Address = smartMoneyNormalizeTokenAddress(events[0].Token0Address)
		}
		if token1Address == "" {
			token1Address = smartMoneyNormalizeTokenAddress(events[0].Token1Address)
		}
		if token0Symbol == "" {
			token0Symbol = strings.TrimSpace(events[0].Token0Symbol)
		}
		if token1Symbol == "" {
			token1Symbol = strings.TrimSpace(events[0].Token1Symbol)
		}
	}

	displayTokenAddress, displayTokenSymbol := smartMoneyPickDisplayToken(
		token0Address,
		token1Address,
		token0Symbol,
		token1Symbol,
	)
	useToken1AsDisplay := smartMoneyDisplayTokenUsesToken1(
		displayTokenAddress,
		displayTokenSymbol,
		token0Address,
		token1Address,
		token0Symbol,
		token1Symbol,
	)

	explorerBase := scanExplorerBase(chain)
	walletsByAddress := loadSmartMoneyMarkerWallets(ctx, int(cc.ChainID), events)
	estimates, warnings := buildSmartMoneyMarkerEstimates(ctx, int(cc.ChainID), poolID, poolVersion, queryStart, queryEnd, events)
	out := make([]smartMoneyPoolMarkerEvent, 0, len(events))

	for _, event := range events {
		action := "add"
		if strings.EqualFold(event.EventType, "remove") {
			action = "remove"
		}

		walletAddress := strings.ToLower(strings.TrimSpace(event.WalletAddress))
		walletLabel := ""
		walletAvatarURL := ""
		walletSource := ""
		walletSourceContract := ""
		wallet := walletsByAddress[walletAddress]
		if wallet != nil && wallet.Label != nil {
			walletLabel = strings.TrimSpace(*wallet.Label)
		}
		if wallet != nil && wallet.AvatarURL != nil {
			walletAvatarURL = strings.TrimSpace(*wallet.AvatarURL)
		}
		if wallet != nil {
			walletSource = smartMoneyWalletSourceValue(wallet)
			walletSourceContract = smartMoneyWalletSourceContractValue(wallet)
		}

		priceLowerText, priceUpperText := smartMoneyFormatPositionPriceBounds(
			event.TickLower,
			event.TickUpper,
			smartMoneyTokenDecimalsOrDefault(poolMeta.Token0Decimals),
			smartMoneyTokenDecimalsOrDefault(poolMeta.Token1Decimals),
			useToken1AsDisplay,
		)
		if priceLowerText == "" || priceUpperText == "" {
			priceLowerText, priceUpperText = smartMoneyFormatPositionPriceBounds(
				event.TickLower,
				event.TickUpper,
				smartMoneyTokenDecimalsOrDefault(poolMeta.Token0Decimals),
				smartMoneyTokenDecimalsOrDefault(poolMeta.Token1Decimals),
				smartMoneyDisplayTokenUsesToken1(
					event.Token1Address,
					event.Token1Symbol,
					event.Token0Address,
					event.Token1Address,
					event.Token0Symbol,
					event.Token1Symbol,
				),
			)
		}

		priceLower, _ := strconv.ParseFloat(priceLowerText, 64)
		priceUpper, _ := strconv.ParseFloat(priceUpperText, 64)
		priceLower = sanitizeFloat(priceLower)
		priceUpper = sanitizeFloat(priceUpper)
		if priceLower > 0 && priceUpper > 0 && priceUpper < priceLower {
			priceLower, priceUpper = priceUpper, priceLower
		}

		midPrice := 0.0
		if priceLower > 0 && priceUpper > 0 {
			midPrice = (priceLower + priceUpper) / 2
		}

		estimatedUSD := decimalStringToFloat(event.TotalUSD)
		if estimatedUSD <= 0 {
			estimatedUSD = decimalStringToFloat(event.Token0AmountUSD) + decimalStringToFloat(event.Token1AmountUSD)
		}

		txURL := ""
		if hash := strings.TrimSpace(event.TxHash); hash != "" {
			txURL = explorerBase + "/tx/" + hash
		}

		marker := smartMoneyPoolMarkerEvent{
			EventID:              smartMoneyMarkerEventID(&event),
			T:                    event.TxTimestamp.Unix(),
			BucketT:              bucketUnix(event.TxTimestamp.Unix(), bucketSec),
			WalletAddress:        walletAddress,
			WalletLabel:          walletLabel,
			WalletAvatarURL:      walletAvatarURL,
			WalletSource:         walletSource,
			WalletSourceContract: walletSourceContract,
			WalletColor:          sm.WalletColor(walletAddress),
			Action:               action,
			TxHash:               strings.TrimSpace(event.TxHash),
			TxURL:                txURL,
			TickLower:            event.TickLower,
			TickUpper:            event.TickUpper,
			PriceLower:           priceLower,
			PriceUpper:           priceUpper,
			RangePercent:         smartMoneyRangePercentFromTicks(event.TickLower, event.TickUpper),
			MidPrice:             midPrice,
			AnchorPrice:          midPrice,
			EstimatedUSD:         estimatedUSD,
		}
		if estimate, ok := estimates[marker.EventID]; ok {
			marker.MatchedOpenTxHash = estimate.MatchedOpenTxHash
			if estimate.MatchedOpenT > 0 {
				matchedOpenT := estimate.MatchedOpenT
				marker.MatchedOpenT = &matchedOpenT
			}
			costUSD := estimate.EstimatedCostUSD
			marker.EstimatedCostUSD = &costUSD
			pnlUSD := estimate.EstimatedRealizedPnlUSD
			marker.EstimatedRealizedPnlUSD = &pnlUSD
			if estimate.EstimatedRealizedPnlPct != nil {
				pnlPct := *estimate.EstimatedRealizedPnlPct
				marker.EstimatedRealizedPnlPct = &pnlPct
			}
		}

		out = append(out, marker)
	}

	writeJSON(w, http.StatusOK, smartMoneyPoolMarkersEnvelope{
		Chain:       chain,
		PoolVersion: poolVersion,
		PoolID:      poolID,
		BucketSec:   bucketSec,
		WindowSec:   durationSeconds(rangeStart, rangeEnd),
		UpdatedAt:   time.Now().UTC(),
		Events:      out,
		Warnings:    warnings,
	})
}
