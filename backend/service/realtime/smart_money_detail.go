package realtime

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/models"
	"TgLpBot/service/pool"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type SmartMoneyPositionDetail struct {
	RealtimePosition
	PositionRef     string    `json:"position_ref"`
	PollIntervalSec int       `json:"poll_interval_sec"`
	UpdatedAt       time.Time `json:"updated_at"`
	Warnings        []string  `json:"warnings,omitempty"`
}

func (s *RealtimePositionsService) GetSmartMoneyPositionDetail(active *models.SmartMoneyActivePosition) (*SmartMoneyPositionDetail, error) {
	if s == nil {
		return nil, fmt.Errorf("realtime service not initialized")
	}
	if active == nil {
		return nil, fmt.Errorf("smart money active position missing")
	}

	var (
		position *RealtimePosition
		warnings []string
		err      error
	)

	switch strings.ToLower(strings.TrimSpace(active.Protocol)) {
	case "pancake_v3", "uniswap_v3":
		position, warnings, err = s.buildSmartMoneyV3Position(active)
	case "uniswap_v4":
		position, warnings, err = s.buildSmartMoneyV4Position(active)
	default:
		return nil, fmt.Errorf("unsupported smart money protocol: %s", active.Protocol)
	}
	if err != nil {
		return nil, err
	}
	if position == nil {
		return nil, fmt.Errorf("smart money position detail unavailable")
	}

	return &SmartMoneyPositionDetail{
		RealtimePosition: *position,
		PositionRef:      strings.TrimSpace(active.PositionRef),
		PollIntervalSec:  1,
		UpdatedAt:        time.Now(),
		Warnings:         warnings,
	}, nil
}

func (s *RealtimePositionsService) buildSmartMoneyV3Position(active *models.SmartMoneyActivePosition) (*RealtimePosition, []string, error) {
	chain := smartMoneyDetailChain(active.ChainID)
	poolAddr := common.Address{}
	if common.IsHexAddress(active.PoolAddress) {
		poolAddr = common.HexToAddress(active.PoolAddress)
	}

	positionManager := strings.TrimSpace(active.PositionManagerAddress)
	if !common.IsHexAddress(positionManager) {
		warnings := appendWarning(nil, "missing v3 position manager")
		return s.buildStaticSmartMoneyPosition(active, chain, "v3", smartMoneyExchange(active.Protocol), common.Address{}, common.Address{}, nil, 0, warnings), warnings, nil
	}

	token0 := smartMoneyTokenAddress(active.Token0Address)
	token1 := smartMoneyTokenAddress(active.Token1Address)
	tickLower := intValue(active.TickLower)
	tickUpper := intValue(active.TickUpper)
	liq := parseSmartMoneyBigInt(active.CurrentLiquidity)
	owed0 := big.NewInt(0)
	owed1 := big.NewInt(0)
	var warnings []string

	client, _, clientErr := blockchain.GetEVMClient(chain)
	if clientErr != nil {
		warnings = appendWarning(warnings, fmt.Sprintf("v3 rpc unavailable: %v", clientErr))
	} else if active.NftTokenID > 0 {
		pm, err := blockchain.NewV3PositionManager(common.HexToAddress(positionManager), client)
		if err != nil {
			warnings = appendWarning(warnings, fmt.Sprintf("init v3 position manager failed: %v", err))
		} else {
			info, err := pm.Positions(nil, new(big.Int).SetUint64(active.NftTokenID))
			if err != nil {
				if active.IsActive {
					warnings = appendWarning(warnings, fmt.Sprintf("read v3 position failed: %v", err))
				}
			} else if info != nil {
				if info.Token0 != (common.Address{}) {
					token0 = info.Token0
				}
				if info.Token1 != (common.Address{}) {
					token1 = info.Token1
				}
				if info.TickLower < info.TickUpper {
					tickLower = info.TickLower
					tickUpper = info.TickUpper
				}
				if info.Liquidity != nil {
					liq = info.Liquidity
				}
				owed0 = cloneBig(info.TokensOwed0)
				owed1 = cloneBig(info.TokensOwed1)

				if poolAddr != (common.Address{}) {
					sqrtP, currentTick, usedStale, age, err := s.getV3Slot0(chain, poolAddr)
					if err != nil && sqrtP == nil {
						warnings = appendWarning(warnings, fmt.Sprintf("read v3 slot0 failed: %v", err))
						return s.buildStaticSmartMoneyPosition(active, chain, "v3", smartMoneyExchange(active.Protocol), token0, token1, liq, 0, warnings), warnings, nil
					}

					if usedStale && err != nil {
						warnings = appendWarning(warnings, fmt.Sprintf("v3 slot0 cache fallback (%ds)", int(age.Seconds())))
					}
					if fee0, fee1, feeStale, feeAge, feeErr := s.calcV3UnclaimedFeesCached(chain, poolAddr, currentTick, info); fee0 != nil && fee1 != nil {
						owed0 = fee0
						owed1 = fee1
						if feeStale && feeErr != nil {
							warnings = appendWarning(warnings, fmt.Sprintf("v3 fee cache fallback (%ds)", int(feeAge.Seconds())))
						} else if feeErr != nil {
							warnings = appendWarning(warnings, fmt.Sprintf("v3 fee fallback: %v", feeErr))
						}
					} else if feeErr != nil {
						warnings = appendWarning(warnings, fmt.Sprintf("v3 fee read failed: %v", feeErr))
					}
					return s.buildDynamicSmartMoneyPosition(active, chain, "v3", smartMoneyExchange(active.Protocol), token0, token1, liq, owed0, owed1, sqrtP, currentTick, tickLower, tickUpper, warnings), warnings, nil
				}
			}
		}
	}

	return s.buildStaticSmartMoneyPosition(active, chain, "v3", smartMoneyExchange(active.Protocol), token0, token1, liq, 0, warnings), warnings, nil
}

func (s *RealtimePositionsService) buildSmartMoneyV4Position(active *models.SmartMoneyActivePosition) (*RealtimePosition, []string, error) {
	chain := smartMoneyDetailChain(active.ChainID)
	poolID := strings.TrimSpace(active.PoolAddress)
	poolManager := smartMoneyTokenAddress(active.PoolManagerAddress)
	stateView := smartMoneyTokenAddress(active.StateViewAddress)
	positionManager := smartMoneyTokenAddress(active.PositionManagerAddress)
	token0 := smartMoneyTokenAddress(active.Token0Address)
	token1 := smartMoneyTokenAddress(active.Token1Address)
	tickLower := intValue(active.TickLower)
	tickUpper := intValue(active.TickUpper)
	liq := parseSmartMoneyBigInt(active.CurrentLiquidity)
	owed0 := big.NewInt(0)
	owed1 := big.NewInt(0)
	var warnings []string

	var v4pos *blockchain.V4PositionInfo
	if active.NftTokenID > 0 && positionManager != (common.Address{}) && poolManager != (common.Address{}) && poolID != "" {
		pos, err := blockchain.GetV4PositionInfo(positionManager, poolManager, poolID, new(big.Int).SetUint64(active.NftTokenID))
		if err != nil {
			if active.IsActive {
				warnings = appendWarning(warnings, fmt.Sprintf("read v4 position failed: %v", err))
			}
		} else if pos != nil {
			v4pos = pos
			if pos.Token0 != (common.Address{}) {
				token0 = pos.Token0
			}
			if pos.Token1 != (common.Address{}) {
				token1 = pos.Token1
			}
			if pos.TickLower < pos.TickUpper {
				tickLower = pos.TickLower
				tickUpper = pos.TickUpper
			}
			if pos.Liquidity != nil {
				liq = pos.Liquidity
			}
			owed0 = cloneBig(pos.TokensOwed0)
			owed1 = cloneBig(pos.TokensOwed1)
		}
	}

	if stateView == (common.Address{}) || poolManager == (common.Address{}) || poolID == "" {
		warnings = appendWarning(warnings, "missing v4 runtime metadata")
		return s.buildStaticSmartMoneyPosition(active, chain, "v4", smartMoneyExchange(active.Protocol), token0, token1, liq, 0, warnings), warnings, nil
	}

	sqrtP, currentTick, usedStale, age, err := s.getV4Slot0(stateView, poolManager, poolID)
	if err != nil && sqrtP == nil {
		warnings = appendWarning(warnings, fmt.Sprintf("read v4 slot0 failed: %v", err))
		return s.buildStaticSmartMoneyPosition(active, chain, "v4", smartMoneyExchange(active.Protocol), token0, token1, liq, 0, warnings), warnings, nil
	}
	if usedStale && err != nil {
		warnings = appendWarning(warnings, fmt.Sprintf("v4 slot0 cache fallback (%ds)", int(age.Seconds())))
	}

	if v4pos != nil {
		if fee0, fee1, feeStale, feeAge, feeErr := s.calcV4UnclaimedFeesCachedUnified(stateView, poolManager, poolID, currentTick, v4pos); fee0 != nil && fee1 != nil {
			owed0 = fee0
			owed1 = fee1
			if feeStale && feeErr != nil {
				warnings = appendWarning(warnings, fmt.Sprintf("v4 fee cache fallback (%ds)", int(feeAge.Seconds())))
			} else if feeErr != nil {
				warnings = appendWarning(warnings, fmt.Sprintf("v4 fee fallback: %v", feeErr))
			}
		} else if feeErr != nil {
			warnings = appendWarning(warnings, fmt.Sprintf("v4 fee read failed: %v", feeErr))
		}
	}

	return s.buildDynamicSmartMoneyPosition(active, chain, "v4", smartMoneyExchange(active.Protocol), token0, token1, liq, owed0, owed1, sqrtP, currentTick, tickLower, tickUpper, warnings), warnings, nil
}

func (s *RealtimePositionsService) buildDynamicSmartMoneyPosition(
	active *models.SmartMoneyActivePosition,
	chain string,
	version string,
	exchange string,
	token0 common.Address,
	token1 common.Address,
	liq *big.Int,
	owed0 *big.Int,
	owed1 *big.Int,
	sqrtP *big.Int,
	currentTick int,
	tickLower int,
	tickUpper int,
	warnings []string,
) *RealtimePosition {
	meta0 := s.smartMoneyTokenMeta(chain, token0, active.Token0Symbol, active.Token0Decimals)
	meta1 := s.smartMoneyTokenMeta(chain, token1, active.Token1Symbol, active.Token1Decimals)

	if sqrtP == nil {
		sqrtP = big.NewInt(0)
	}
	if liq == nil {
		liq = big.NewInt(0)
	}
	if owed0 == nil {
		owed0 = big.NewInt(0)
	}
	if owed1 == nil {
		owed1 = big.NewInt(0)
	}

	sqrtA, _ := pool.SqrtRatioAtTick(int32(tickLower))
	sqrtB, _ := pool.SqrtRatioAtTick(int32(tickUpper))
	amt0Raw, amt1Raw := pool.AmountsForLiquidity(sqrtP, sqrtA, sqrtB, liq)

	walletAddr := smartMoneyTokenAddress(active.WalletAddress)
	w0 := s.getWalletTokenBalance(chain, token0, walletAddr)
	w1 := s.getWalletTokenBalance(chain, token1, walletAddr)

	prices, _ := s.priceService.GetUSDPrices(chain, []string{token0.Hex(), token1.Hex()})
	price0 := prices[strings.ToLower(token0.Hex())]
	price1 := prices[strings.ToLower(token1.Hex())]

	row0 := buildTokenRow(token0, meta0, price0, w0, amt0Raw, owed0)
	row1 := buildTokenRow(token1, meta1, price1, w1, amt1Raw, owed1)

	return buildSmartMoneyRealtimePosition(active, chain, version, exchange, row0, row1, currentTick, tickLower, tickUpper, warnings)
}

func (s *RealtimePositionsService) buildStaticSmartMoneyPosition(
	active *models.SmartMoneyActivePosition,
	chain string,
	version string,
	exchange string,
	token0 common.Address,
	token1 common.Address,
	liq *big.Int,
	currentTick int,
	warnings []string,
) *RealtimePosition {
	meta0 := s.smartMoneyTokenMeta(chain, token0, active.Token0Symbol, active.Token0Decimals)
	meta1 := s.smartMoneyTokenMeta(chain, token1, active.Token1Symbol, active.Token1Decimals)
	walletAddr := smartMoneyTokenAddress(active.WalletAddress)

	w0 := s.getWalletTokenBalance(chain, token0, walletAddr)
	w1 := s.getWalletTokenBalance(chain, token1, walletAddr)
	prices, _ := s.priceService.GetUSDPrices(chain, []string{token0.Hex(), token1.Hex()})
	price0 := prices[strings.ToLower(token0.Hex())]
	price1 := prices[strings.ToLower(token1.Hex())]

	row0 := buildTokenRow(token0, meta0, price0, w0, big.NewInt(0), big.NewInt(0))
	row1 := buildTokenRow(token1, meta1, price1, w1, big.NewInt(0), big.NewInt(0))
	if liq == nil {
		liq = big.NewInt(0)
	}

	return buildSmartMoneyRealtimePosition(active, chain, version, exchange, row0, row1, currentTick, intValue(active.TickLower), intValue(active.TickUpper), warnings)
}

func buildSmartMoneyRealtimePosition(
	active *models.SmartMoneyActivePosition,
	chain string,
	version string,
	exchange string,
	row0 RealtimeTokenRow,
	row1 RealtimeTokenRow,
	currentTick int,
	tickLower int,
	tickUpper int,
	_ []string,
) *RealtimePosition {
	totals := RealtimeTotals{
		WalletUSD:   row0.WalletUSD + row1.WalletUSD,
		PositionUSD: row0.PositionUSD + row1.PositionUSD,
		FeeUSD:      row0.FeeUSD + row1.FeeUSD,
	}
	totals.TotalUSD = totals.WalletUSD + totals.PositionUSD + totals.FeeUSD

	netInvestedUSD := parseSmartMoneyUSD(active.NetTotalUSD)
	initialCostUSD := parseSmartMoneyUSD(active.EntryTotalUSD)
	currentValueUSD := totals.PositionUSD + totals.FeeUSD
	hasPnL := netInvestedUSD > 0
	absolutePnLUSD := 0.0
	if hasPnL {
		absolutePnLUSD = currentValueUSD - netInvestedUSD
	}

	feeTierLabel := smartMoneyFeeLabel(active.FeeTier)
	title := fmt.Sprintf("%s-%s-%s", exchangeShort(exchange, exchange), row0.Symbol, row1.Symbol)
	if feeTierLabel != "" {
		title = title + "-" + feeTierLabel
	}

	inRange := tickUpper > tickLower && currentTick >= tickLower && currentTick <= tickUpper
	rangePct := estimateRangePercent(currentTick, tickLower, tickUpper)
	poolID := strings.TrimSpace(active.PoolAddress)
	positionID := strings.TrimSpace(active.PositionRef)
	if active.NftTokenID > 0 {
		positionID = fmt.Sprintf("%d", active.NftTokenID)
	}

	var runningSince *time.Time
	if !active.OpenedAt.IsZero() {
		ts := active.OpenedAt
		runningSince = &ts
	}

	return &RealtimePosition{
		Chain:           chain,
		Version:         version,
		Exchange:        exchange,
		Title:           title,
		PoolID:          poolID,
		PositionID:      positionID,
		WalletAddress:   strings.TrimSpace(active.WalletAddress),
		StatusLabel:     smartMoneyStatusLabel(active.IsActive),
		InRange:         inRange,
		CurrentTick:     currentTick,
		TickLower:       tickLower,
		TickUpper:       tickUpper,
		TickSpacing:     smartMoneyTickSpacing(active),
		RangePercent:    rangePct,
		OutOfRange:      "0/0",
		RunningSince:    runningSince,
		HasLiquidity:    active.IsActive || row0.PositionUSD > 0 || row1.PositionUSD > 0,
		InitialCostUSD:  initialCostUSD,
		NetInvestedUSD:  netInvestedUSD,
		CurrentValueUSD: currentValueUSD,
		AbsolutePnLUSD:  absolutePnLUSD,
		HasPnL:          hasPnL,
		TokenRows:       []RealtimeTokenRow{row0, row1},
		Totals:          totals,
	}
}

func (s *RealtimePositionsService) smartMoneyTokenMeta(chain string, token common.Address, symbol string, decimals int) cachedTokenMeta {
	meta := cachedTokenMeta{
		symbol:   strings.TrimSpace(symbol),
		decimals: decimals,
	}
	if meta.symbol != "" && meta.decimals > 0 {
		return meta
	}
	if token != (common.Address{}) {
		fallback := s.getTokenMeta(chain, token)
		if meta.symbol == "" {
			meta.symbol = fallback.symbol
		}
		if meta.decimals <= 0 {
			meta.decimals = fallback.decimals
		}
	}
	if meta.symbol == "" {
		meta.symbol = shortTokenSymbol(token.Hex())
	}
	if meta.decimals <= 0 {
		meta.decimals = 18
	}
	return meta
}

func shortTokenSymbol(addr string) string {
	addr = strings.TrimSpace(addr)
	if len(addr) < 8 {
		return "TOKEN"
	}
	return strings.ToUpper(addr[2:6])
}

func smartMoneyTokenAddress(raw string) common.Address {
	raw = strings.TrimSpace(raw)
	if !common.IsHexAddress(raw) {
		return common.Address{}
	}
	return common.HexToAddress(raw)
}

func smartMoneyDetailChain(chainID int) string {
	switch chainID {
	case 8453:
		return "base"
	default:
		return "bsc"
	}
}

func smartMoneyExchange(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "pancake_v3":
		return "PancakeSwap V3"
	case "uniswap_v3":
		return "Uniswap V3"
	case "uniswap_v4":
		return "Uniswap V4"
	default:
		return protocol
	}
}

func smartMoneyStatusLabel(isActive bool) string {
	if isActive {
		return "Open"
	}
	return "Closed"
}

func smartMoneyTickSpacing(active *models.SmartMoneyActivePosition) int {
	if active == nil {
		return 0
	}
	if active.TickSpacing > 0 {
		return active.TickSpacing
	}
	if active.FeeTier != nil {
		return tickSpacingFromFee(uint64(*active.FeeTier))
	}
	return 0
}

func smartMoneyFeeLabel(feeTier *int) string {
	if feeTier == nil {
		return ""
	}
	switch *feeTier {
	case 100:
		return "0.01%"
	case 500:
		return "0.05%"
	case 2500:
		return "0.25%"
	case 3000:
		return "0.30%"
	case 10000:
		return "1%"
	case 20000:
		return "2%"
	default:
		return fmt.Sprintf("%.2f%%", float64(*feeTier)/10000.0)
	}
}

func parseSmartMoneyBigInt(raw string) *big.Int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return big.NewInt(0)
	}
	if value, ok := new(big.Int).SetString(raw, 10); ok && value != nil {
		return value
	}
	return big.NewInt(0)
}

func parseSmartMoneyUSD(value *string) float64 {
	if value == nil {
		return 0
	}
	raw := strings.TrimSpace(*value)
	if raw == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func appendWarning(warnings []string, message string) []string {
	message = strings.TrimSpace(message)
	if message == "" {
		return warnings
	}
	return append(warnings, message)
}
