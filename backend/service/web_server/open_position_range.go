package web_server

import (
	"fmt"
	"strings"

	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"

	"github.com/ethereum/go-ethereum/common"
)

const (
	openPositionRangeInputPercentage = "percentage"
	openPositionRangeInputTick       = "tick"
	openPositionRangeInputGrid       = "grid"
)

type resolvedOpenPositionRange struct {
	InputMode      string
	TickLower      int
	TickUpper      int
	TickLowerPct   float64
	TickUpperPct   float64
	StableLowerPct float64
	StableUpperPct float64
	RangePct       float64
}

type openPositionRangeEditorInfo struct {
	InputMode           string   `json:"input_mode,omitempty"`
	CurrentTick         int      `json:"current_tick"`
	TickSpacing         int      `json:"tick_spacing"`
	MinTick             int      `json:"min_tick"`
	MaxTick             int      `json:"max_tick"`
	AnchorTickLower     int      `json:"anchor_tick_lower"`
	AnchorTickUpper     int      `json:"anchor_tick_upper"`
	CurrentPrice        *float64 `json:"current_price,omitempty"`
	RangeLowerPrice     *float64 `json:"range_lower_price,omitempty"`
	RangeUpperPrice     *float64 `json:"range_upper_price,omitempty"`
	BaseSymbol          string   `json:"base_symbol,omitempty"`
	QuoteSymbol         string   `json:"quote_symbol,omitempty"`
	TickLower           *int     `json:"tick_lower,omitempty"`
	TickUpper           *int     `json:"tick_upper,omitempty"`
	RangeLowerPct       *float64 `json:"range_lower_pct,omitempty"`
	RangeUpperPct       *float64 `json:"range_upper_pct,omitempty"`
	PositionShape       string   `json:"position_shape,omitempty"`
	DominantTokenSymbol string   `json:"dominant_token_symbol,omitempty"`
}

func openPositionIntPtr(v int) *int {
	return &v
}

func normalizeOpenPositionRangeInputMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", openPositionRangeInputPercentage:
		return openPositionRangeInputPercentage
	case openPositionRangeInputTick:
		return openPositionRangeInputTick
	case openPositionRangeInputGrid:
		return openPositionRangeInputGrid
	default:
		return ""
	}
}

func loadOpenPositionCurrentTick(chain string, poolVersion string, poolAddress string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(poolVersion)) {
	case "v4":
		if !common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
			return 0, fmt.Errorf("missing UNISWAP_V4_POOL_MANAGER_ADDRESS")
		}
		if !common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) {
			return 0, fmt.Errorf("missing UNISWAP_V4_STATE_VIEW_ADDRESS")
		}
		poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
		stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
		return blockchain.GetUniswapV4PoolCurrentTickViaStateView(stateView, poolManager, poolAddress)
	default:
		if !common.IsHexAddress(poolAddress) {
			return 0, fmt.Errorf("invalid pool address")
		}
		client, _, err := blockchain.GetEVMClient(chain)
		if err != nil {
			return 0, err
		}
		return blockchain.GetV3PoolCurrentTickWithClient(client, common.HexToAddress(poolAddress))
	}
}

func resolveOpenPositionRange(task *models.StrategyTask, req openPositionRequest, currentTick int, tickSpacing int) (resolvedOpenPositionRange, *openPositionError, int) {
	tc := pool.NewTickCalculator()
	mode := normalizeOpenPositionRangeInputMode(req.RangeInputMode)
	if mode == "" {
		return resolvedOpenPositionRange{}, &openPositionError{
			Code:    "invalid_range_mode",
			Message: "invalid range mode",
		}, 400
	}

	resolved := resolvedOpenPositionRange{InputMode: mode}
	switch mode {
	case openPositionRangeInputPercentage:
		if req.RangeLowerPct <= 0 || req.RangeUpperPct <= 0 || req.RangeLowerPct >= 100 || req.RangeUpperPct >= 100 {
			return resolvedOpenPositionRange{}, &openPositionError{
				Code:    "invalid_range",
				Message: "invalid percentage range",
			}, 400
		}
		tickLowerPctReq, tickUpperPctReq := pricing.TickPercentagesFromStablePercentages(task, req.RangeLowerPct, req.RangeUpperPct)
		if tickLowerPctReq <= 0 || tickUpperPctReq <= 0 {
			return resolvedOpenPositionRange{}, &openPositionError{
				Code:    "invalid_range",
				Message: "invalid percentage range",
			}, 400
		}
		resolved.TickLower, resolved.TickUpper = tc.CalculateTickFromPercentagesBestFit(currentTick, tickLowerPctReq, tickUpperPctReq, tickSpacing)
	case openPositionRangeInputTick, openPositionRangeInputGrid:
		if req.TickLower == nil || req.TickUpper == nil {
			return resolvedOpenPositionRange{}, &openPositionError{
				Code:    "invalid_range",
				Message: "tick range is required",
			}, 400
		}
		tickLower, tickUpper, err := tc.NormalizeTickRange(*req.TickLower, *req.TickUpper, tickSpacing)
		if err != nil {
			return resolvedOpenPositionRange{}, &openPositionError{
				Code:    "invalid_range",
				Message: err.Error(),
			}, 400
		}
		resolved.TickLower = tickLower
		resolved.TickUpper = tickUpper
	default:
		return resolvedOpenPositionRange{}, &openPositionError{
			Code:    "invalid_range_mode",
			Message: "invalid range mode",
		}, 400
	}

	resolved.TickLowerPct, resolved.TickUpperPct = tc.CalculatePercentagesFromTicks(currentTick, resolved.TickLower, resolved.TickUpper)
	if resolved.TickLowerPct <= 0 && resolved.TickUpperPct <= 0 {
		return resolvedOpenPositionRange{}, &openPositionError{
			Code:    "invalid_range",
			Message: "unable to resolve range",
		}, 400
	}
	resolved.StableLowerPct, resolved.StableUpperPct = pricing.StablePercentagesFromTickPercentages(task, resolved.TickLowerPct, resolved.TickUpperPct)
	resolved.RangePct = (resolved.TickLowerPct + resolved.TickUpperPct) / 2.0
	return resolved, nil, 0
}

func buildOpenPositionRangeEditorInfo(task *models.StrategyTask, currentTick int, tickSpacing int, resolved *resolvedOpenPositionRange) *openPositionRangeEditorInfo {
	if task == nil || tickSpacing <= 0 {
		return nil
	}
	minTick, maxTick, err := pool.FullRangeTicks(tickSpacing)
	if err != nil {
		minTick = 0
		maxTick = 0
	}
	tc := pool.NewTickCalculator()
	anchorLower := tc.RoundDownToTickSpacing(currentTick, tickSpacing)
	anchorUpper := anchorLower + tickSpacing
	info := &openPositionRangeEditorInfo{
		CurrentTick:     currentTick,
		TickSpacing:     tickSpacing,
		MinTick:         minTick,
		MaxTick:         maxTick,
		AnchorTickLower: anchorLower,
		AnchorTickUpper: anchorUpper,
	}
	if price, base, quote, ok := pricing.BuildPriceDisplay(task, currentTick); ok {
		info.CurrentPrice = float64Ptr(price)
		info.BaseSymbol = base
		info.QuoteSymbol = quote
	}
	if resolved == nil {
		return info
	}

	info.InputMode = resolved.InputMode
	info.TickLower = openPositionIntPtr(resolved.TickLower)
	info.TickUpper = openPositionIntPtr(resolved.TickUpper)
	info.RangeLowerPct = float64Ptr(resolved.StableLowerPct)
	info.RangeUpperPct = float64Ptr(resolved.StableUpperPct)
	if display := pricing.BuildPriceRangeDisplay(task, resolved.TickLower, resolved.TickUpper, currentTick); display.HasRange {
		info.RangeLowerPrice = float64Ptr(display.Lower)
		info.RangeUpperPrice = float64Ptr(display.Upper)
		if info.BaseSymbol == "" {
			info.BaseSymbol = display.BaseSymbol
		}
		if info.QuoteSymbol == "" {
			info.QuoteSymbol = display.QuoteSymbol
		}
	}
	info.PositionShape, info.DominantTokenSymbol = classifyOpenPositionShape(task, currentTick, resolved.TickLower, resolved.TickUpper)
	return info
}

func classifyOpenPositionShape(task *models.StrategyTask, currentTick int, tickLower int, tickUpper int) (string, string) {
	if task == nil {
		return "", ""
	}
	switch {
	case currentTick < tickLower:
		return "single_token0", strings.TrimSpace(task.Token0Symbol)
	case currentTick >= tickUpper:
		return "single_token1", strings.TrimSpace(task.Token1Symbol)
	default:
		return "dual_sided", ""
	}
}
