package pool

import (
	"fmt"
	"math"
	"math/big"
	"strings"
)

const (
	fullRangeMinTick = -887272
	fullRangeMaxTick = 887272
)

// StandardTickSpacingFromFee maps the common V3/V4 fee presets to tick spacing.
func StandardTickSpacingFromFee(fee uint64) (int, error) {
	switch fee {
	case 100:
		return 1, nil
	case 500:
		return 10, nil
	case 2500:
		return 50, nil
	case 3000:
		return 60, nil
	case 10000:
		return 200, nil
	default:
		return 0, fmt.Errorf("unsupported fee tier: %d", fee)
	}
}

func ceilDiv(a, b int) int {
	if b == 0 {
		return 0
	}
	if a >= 0 {
		return (a + b - 1) / b
	}
	return a / b
}

func floorDiv(a, b int) int {
	if b == 0 {
		return 0
	}
	if a >= 0 {
		return a / b
	}
	if a%b == 0 {
		return a / b
	}
	return (a / b) - 1
}

// AlignTickDown rounds a tick down to the nearest valid tickSpacing boundary.
func AlignTickDown(tick, tickSpacing int) (int, error) {
	if tickSpacing <= 0 {
		return 0, fmt.Errorf("invalid tick spacing: %d", tickSpacing)
	}
	return floorDiv(tick, tickSpacing) * tickSpacing, nil
}

// AlignTickUp rounds a tick up to the nearest valid tickSpacing boundary.
func AlignTickUp(tick, tickSpacing int) (int, error) {
	if tickSpacing <= 0 {
		return 0, fmt.Errorf("invalid tick spacing: %d", tickSpacing)
	}
	return ceilDiv(tick, tickSpacing) * tickSpacing, nil
}

// FullRangeTicks returns the valid full-range ticks for a given tick spacing.
func FullRangeTicks(tickSpacing int) (int, int, error) {
	if tickSpacing <= 0 {
		return 0, 0, fmt.Errorf("invalid tick spacing: %d", tickSpacing)
	}
	lower := ceilDiv(fullRangeMinTick, tickSpacing) * tickSpacing
	upper := floorDiv(fullRangeMaxTick, tickSpacing) * tickSpacing
	if lower >= upper {
		return 0, 0, fmt.Errorf("invalid full range for tick spacing: %d", tickSpacing)
	}
	return lower, upper, nil
}

// ParseDecimalToFloat parses a decimal string into a big.Float with a stable precision.
func ParseDecimalToFloat(raw string) (*big.Float, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, fmt.Errorf("decimal string is empty")
	}
	f, ok := new(big.Float).SetPrec(256).SetMode(big.ToNearestAway).SetString(value)
	if !ok {
		return nil, fmt.Errorf("invalid decimal string: %s", raw)
	}
	if f.Sign() <= 0 {
		return nil, fmt.Errorf("decimal value must be positive")
	}
	return f, nil
}

func pow10Float(exp int) *big.Float {
	if exp == 0 {
		return new(big.Float).SetPrec(256).SetInt64(1)
	}
	if exp > 0 {
		base := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil)
		return new(big.Float).SetPrec(256).SetInt(base)
	}
	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exp)), nil)
	return new(big.Float).SetPrec(256).Quo(
		new(big.Float).SetPrec(256).SetInt64(1),
		new(big.Float).SetPrec(256).SetInt(base),
	)
}

func invertFloat(v *big.Float) (*big.Float, error) {
	if v == nil || v.Sign() <= 0 {
		return nil, fmt.Errorf("value must be positive")
	}
	return new(big.Float).SetPrec(256).Quo(
		new(big.Float).SetPrec(256).SetInt64(1),
		new(big.Float).SetPrec(256).Set(v),
	), nil
}

// DecimalToUnits converts a human-readable decimal string into integer base units.
func DecimalToUnits(raw string, decimals int) (*big.Int, error) {
	if decimals < 0 || decimals > 36 {
		return nil, fmt.Errorf("invalid decimals: %d", decimals)
	}
	value, err := ParseDecimalToFloat(raw)
	if err != nil {
		return nil, err
	}
	scaled := new(big.Float).SetPrec(256).Mul(value, pow10Float(decimals))
	out, _ := scaled.Int(nil)
	if out == nil || out.Sign() <= 0 {
		return nil, fmt.Errorf("amount is too small")
	}
	return out, nil
}

// SqrtPriceX96FromHumanPrice converts a human price quoted as token1 per token0 into sqrtPriceX96.
func SqrtPriceX96FromHumanPrice(humanPrice *big.Float, decimals0, decimals1 int) (*big.Int, error) {
	if humanPrice == nil || humanPrice.Sign() <= 0 {
		return nil, fmt.Errorf("human price must be positive")
	}

	ratio := new(big.Float).SetPrec(256).Set(humanPrice)
	ratio.Mul(ratio, pow10Float(decimals1-decimals0))
	if ratio.Sign() <= 0 {
		return nil, fmt.Errorf("raw price ratio must be positive")
	}

	sqrtRatio := new(big.Float).SetPrec(256).Sqrt(ratio)
	scale := new(big.Float).SetPrec(256).SetInt(q96)
	sqrtRatio.Mul(sqrtRatio, scale)

	out, _ := sqrtRatio.Int(nil)
	if out == nil || out.Sign() <= 0 {
		return nil, fmt.Errorf("sqrtPriceX96 conversion failed")
	}
	return out, nil
}

// TickFromHumanPrice estimates the nearest tick from a human price quoted as token1 per token0.
func TickFromHumanPrice(humanPrice float64, decimals0, decimals1 int) (int, error) {
	if math.IsNaN(humanPrice) || math.IsInf(humanPrice, 0) || humanPrice <= 0 {
		return 0, fmt.Errorf("human price must be positive")
	}
	rawPrice := humanPrice * math.Pow10(decimals1-decimals0)
	if math.IsNaN(rawPrice) || math.IsInf(rawPrice, 0) || rawPrice <= 0 {
		return 0, fmt.Errorf("raw price ratio must be positive")
	}
	tick := int(math.Round(math.Log(rawPrice) / math.Log(1.0001)))
	if tick < fullRangeMinTick {
		tick = fullRangeMinTick
	}
	if tick > fullRangeMaxTick {
		tick = fullRangeMaxTick
	}
	return tick, nil
}

// HumanPriceFromTick converts a tick back to a human price quoted as token1 per token0.
func HumanPriceFromTick(tick int, decimals0, decimals1 int) float64 {
	return math.Pow(1.0001, float64(tick)) * math.Pow10(decimals0-decimals1)
}

// HumanPriceFromBaseQuote converts a price quoted as tokenB per tokenA into a canonical token1 per token0 price.
func HumanPriceFromBaseQuote(priceAB *big.Float, tokenAIsToken0 bool) (*big.Float, error) {
	if priceAB == nil || priceAB.Sign() <= 0 {
		return nil, fmt.Errorf("price must be positive")
	}
	if tokenAIsToken0 {
		return new(big.Float).SetPrec(256).Set(priceAB), nil
	}
	return invertFloat(priceAB)
}
