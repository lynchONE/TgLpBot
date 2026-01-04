package convert

import (
	"fmt"
	"math/big"
	"strings"
)

// FloatUSDTToWei converts a USDT float amount to wei (1e18) with basic validation.
func FloatUSDTToWei(amount float64) (*big.Int, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be > 0")
	}
	f := new(big.Float).SetFloat64(amount)
	f.Mul(f, big.NewFloat(1e18))
	i, _ := f.Int(nil)
	if i == nil || i.Sign() <= 0 {
		return nil, fmt.Errorf("amount too small")
	}
	return i, nil
}

// ParseBigInt parses a decimal or hex integer string and defaults empty input to 0.
func ParseBigInt(s string) (*big.Int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return big.NewInt(0), nil
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, ok := new(big.Int).SetString(strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X"), 16)
		if !ok {
			return nil, fmt.Errorf("invalid hex integer")
		}
		return v, nil
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("invalid decimal integer")
	}
	return v, nil
}

// ParseBigIntFlexible parses hex/decimal integers but rejects empty input.
func ParseBigIntFlexible(s string) (*big.Int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty number")
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, ok := new(big.Int).SetString(s[2:], 16)
		if !ok {
			return nil, fmt.Errorf("invalid hex number")
		}
		return v, nil
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("invalid decimal number")
	}
	return v, nil
}
