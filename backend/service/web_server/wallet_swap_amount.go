package web_server

import (
	"fmt"
	"math/big"
	"strings"

	"TgLpBot/base/blockchain"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

func parseWalletSwapDecimalAmount(amountStr string, decimals uint8) (*big.Int, error) {
	raw := strings.TrimSpace(amountStr)
	if raw == "" {
		return nil, fmt.Errorf("missing amount")
	}
	if strings.HasPrefix(raw, "+") {
		raw = strings.TrimPrefix(raw, "+")
	}
	if strings.HasPrefix(raw, "-") {
		return nil, fmt.Errorf("amount must be greater than 0")
	}
	if strings.ContainsAny(raw, "eE") {
		return nil, fmt.Errorf("amount must be a decimal string")
	}

	parts := strings.Split(raw, ".")
	if len(parts) > 2 {
		return nil, fmt.Errorf("invalid amount")
	}
	intPart := parts[0]
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}
	if intPart == "" {
		intPart = "0"
	}
	if !isDecimalDigits(intPart) || (fracPart != "" && !isDecimalDigits(fracPart)) {
		return nil, fmt.Errorf("invalid amount")
	}
	if len(fracPart) > int(decimals) {
		extra := fracPart[int(decimals):]
		if strings.Trim(extra, "0") != "" {
			return nil, fmt.Errorf("amount has more decimals than token supports")
		}
		fracPart = fracPart[:decimals]
	}
	for len(fracPart) < int(decimals) {
		fracPart += "0"
	}

	combined := strings.TrimLeft(intPart+fracPart, "0")
	if combined == "" {
		return nil, fmt.Errorf("amount must be greater than 0")
	}
	amount, ok := new(big.Int).SetString(combined, 10)
	if !ok || amount == nil || amount.Sign() <= 0 {
		return nil, fmt.Errorf("invalid amount")
	}
	return amount, nil
}

func isDecimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func formatWalletSwapRawAmount(amount *big.Int, decimals int) string {
	if amount == nil {
		return "0"
	}
	if amount.Sign() == 0 {
		return "0"
	}
	if decimals <= 0 {
		return amount.String()
	}

	raw := amount.String()
	if len(raw) <= decimals {
		frac := strings.Repeat("0", decimals-len(raw)) + raw
		frac = strings.TrimRight(frac, "0")
		if frac == "" {
			return "0"
		}
		return "0." + frac
	}

	intPart := raw[:len(raw)-decimals]
	fracPart := strings.TrimRight(raw[len(raw)-decimals:], "0")
	if fracPart == "" {
		return intPart
	}
	return intPart + "." + fracPart
}

func walletSwapAssetBalance(client *ethclient.Client, token common.Address, wallet common.Address) (*big.Int, error) {
	if strings.EqualFold(token.Hex(), nativePseudoTokenAddress) {
		return blockchain.GetBalanceWithClient(client, wallet)
	}
	return blockchain.GetTokenBalanceWithClient(client, token, wallet)
}

func balanceString(balance *big.Int) string {
	if balance == nil {
		return "0"
	}
	return balance.String()
}
