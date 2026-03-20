package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	sm "TgLpBot/service/smart_money"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type smartMoneyPoolMarkerEvent struct {
	EventID       string  `json:"event_id"`
	T             int64   `json:"t"`
	BucketT       int64   `json:"bucket_t"`
	WalletAddress string  `json:"wallet_address"`
	WalletLabel   string  `json:"wallet_label,omitempty"`
	WalletColor   string  `json:"wallet_color,omitempty"`
	Action        string  `json:"action"`
	TxHash        string  `json:"tx_hash,omitempty"`
	TxURL         string  `json:"tx_url,omitempty"`
	TickLower     *int    `json:"tick_lower,omitempty"`
	TickUpper     *int    `json:"tick_upper,omitempty"`
	PriceLower    float64 `json:"price_lower,omitempty"`
	PriceUpper    float64 `json:"price_upper,omitempty"`
	RangePercent  float64 `json:"range_percent,omitempty"`
	MidPrice      float64 `json:"mid_price,omitempty"`
	AnchorPrice   float64 `json:"anchor_price,omitempty"`
	EstimatedUSD  float64 `json:"estimated_usd"`
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

func poolVersionProtocolFilter(version string) string {
	switch strings.ToLower(strings.TrimSpace(version)) {
	case "v3":
		return "%v3%"
	case "v4":
		return "%v4%"
	default:
		return ""
	}
}

func scanExplorerBase(chain string) string {
	if config.NormalizeChain(chain) == "base" {
		return "https://basescan.org"
	}
	return "https://bscscan.com"
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
	if status, msg := requireMiniAppPermission(check); status != 0 {
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

	ctx := r.Context()
	var events []models.SmartMoneyLPEvent
	db := database.DB.WithContext(ctx).
		Model(&models.SmartMoneyLPEvent{}).
		Where("chain_id = ? AND LOWER(pool_address) = ?", cc.ChainID, poolID).
		Where("tx_timestamp BETWEEN ? AND ?", queryStart, queryEnd)
	if protocolFilter := poolVersionProtocolFilter(poolVersion); protocolFilter != "" {
		db = db.Where("LOWER(protocol) LIKE ?", protocolFilter)
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
		Where("LOWER(address) = ?", poolID).
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

	repo := smService.Repo()
	walletCache := make(map[string]*models.MonitoredWallet)
	explorerBase := scanExplorerBase(chain)
	out := make([]smartMoneyPoolMarkerEvent, 0, len(events))

	for _, event := range events {
		action := "add"
		if strings.EqualFold(event.EventType, "remove") {
			action = "remove"
		}

		walletAddress := strings.ToLower(strings.TrimSpace(event.WalletAddress))
		walletLabel := ""
		if cached, ok := walletCache[walletAddress]; ok {
			if cached != nil && cached.Label != nil {
				walletLabel = strings.TrimSpace(*cached.Label)
			}
		} else {
			wallet, err := repo.GetMonitoredWalletByAddress(ctx, walletAddress, event.ChainID)
			if err == nil {
				walletCache[walletAddress] = wallet
				if wallet != nil && wallet.Label != nil {
					walletLabel = strings.TrimSpace(*wallet.Label)
				}
			} else {
				walletCache[walletAddress] = nil
			}
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

		out = append(out, smartMoneyPoolMarkerEvent{
			EventID:       strings.ToLower(strings.TrimSpace(event.TxHash)) + ":" + strconv.Itoa(event.LogIndex),
			T:             event.TxTimestamp.Unix(),
			BucketT:       bucketUnix(event.TxTimestamp.Unix(), bucketSec),
			WalletAddress: walletAddress,
			WalletLabel:   walletLabel,
			WalletColor:   sm.WalletColor(walletAddress),
			Action:        action,
			TxHash:        strings.TrimSpace(event.TxHash),
			TxURL:         txURL,
			TickLower:     event.TickLower,
			TickUpper:     event.TickUpper,
			PriceLower:    priceLower,
			PriceUpper:    priceUpper,
			RangePercent:  smartMoneyRangePercentFromTicks(event.TickLower, event.TickUpper),
			MidPrice:      midPrice,
			AnchorPrice:   midPrice,
			EstimatedUSD:  estimatedUSD,
		})
	}

	writeJSON(w, http.StatusOK, smartMoneyPoolMarkersEnvelope{
		Chain:       chain,
		PoolVersion: poolVersion,
		PoolID:      poolID,
		BucketSec:   bucketSec,
		WindowSec:   durationSeconds(rangeStart, rangeEnd),
		UpdatedAt:   time.Now().UTC(),
		Events:      out,
	})
}
