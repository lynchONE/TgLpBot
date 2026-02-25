package bot

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

func floatFromUnits(amount *big.Int, decimals int) float64 {
	if amount == nil || amount.Sign() == 0 {
		return 0
	}
	if decimals <= 0 {
		decimals = 18
	}
	f := new(big.Float).SetInt(amount)
	div := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	f.Quo(f, div)
	v, _ := f.Float64()
	return v
}

func nativeSymbolForChain(chain string) string {
	chain = config.NormalizeChain(chain)
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
			w := strings.ToUpper(strings.TrimSpace(cc.WrappedNativeSymbol))
			if w != "" {
				if strings.HasPrefix(w, "W") && len(w) > 1 {
					return w[1:]
				}
				return w
			}
		}
	}
	switch chain {
	case "base":
		return "ETH"
	default:
		return "BNB"
	}
}

func stableSymbolForChain(chain string) (string, int, string) {
	chain = config.NormalizeChain(chain)
	sym := "USDT"
	decimals := 18
	addr := ""
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
			if v := strings.TrimSpace(cc.StableSymbol); v != "" {
				sym = strings.ToUpper(v)
			}
			if cc.StableDecimals > 0 {
				decimals = cc.StableDecimals
			}
			addr = strings.TrimSpace(cc.StableAddress)
		}
	}
	return sym, decimals, addr
}

func tokenBalanceDisplay(client *ethclient.Client, wallet common.Address, tokenAddrStr string, fallbackDecimals int) string {
	if client == nil || !common.IsHexAddress(tokenAddrStr) {
		return "N/A"
	}
	tokenAddr := common.HexToAddress(tokenAddrStr)
	bal, err := blockchain.GetTokenBalanceWithClient(client, tokenAddr, wallet)
	if err != nil {
		return "N/A"
	}
	decimals := fallbackDecimals
	if d, err := blockchain.GetTokenDecimalsWithClient(client, tokenAddr); err == nil && d > 0 {
		decimals = int(d)
	}
	return fmt.Sprintf("%.2f", floatFromUnits(bal, decimals))
}

// getPoolInfoWalletBalanceText returns balance lines for pool-info display.
// Shows native + configured stable for the selected chain.
// If chain config has a secondary USDC address, include USDC as well.
func (b *Bot) getPoolInfoWalletBalanceText(chain string, address string) string {
	_ = b
	chain = config.NormalizeChain(chain)
	shortAddr := address
	if len(address) > 18 {
		shortAddr = address[:10] + "..." + address[len(address)-8:]
	}

	nativeSym := nativeSymbolForChain(chain)
	stableSym, stableDecimals, stableAddrStr := stableSymbolForChain(chain)

	type displayToken struct {
		Symbol   string
		Address  string
		Decimals int
	}
	tokens := []displayToken{{
		Symbol:   stableSym,
		Address:  stableAddrStr,
		Decimals: stableDecimals,
	}}
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok {
			usdcAddr := strings.TrimSpace(cc.USDCAddress)
			if !strings.EqualFold(stableSym, "USDC") && common.IsHexAddress(usdcAddr) {
				tokens = append(tokens, displayToken{
					Symbol:   "USDC",
					Address:  usdcAddr,
					Decimals: stableDecimals,
				})
			}
		}
	}

	nativeBal := "N/A"
	buildText := func(tokenLines []string) string {
		lines := []string{
			fmt.Sprintf("💵 *当前钱包：* `%s`", shortAddr),
			fmt.Sprintf("💰 %s: %s", nativeSym, nativeBal),
		}
		lines = append(lines, tokenLines...)
		return "\n" + strings.Join(lines, "\n") + "\n"
	}

	if !common.IsHexAddress(address) {
		tokenLines := make([]string, 0, len(tokens))
		for _, token := range tokens {
			tokenLines = append(tokenLines, fmt.Sprintf("💼 %s: %s", token.Symbol, "N/A"))
		}
		return buildText(tokenLines)
	}

	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil || client == nil {
		tokenLines := make([]string, 0, len(tokens))
		for _, token := range tokens {
			tokenLines = append(tokenLines, fmt.Sprintf("💼 %s: %s", token.Symbol, "N/A"))
		}
		return buildText(tokenLines)
	}

	walletAddr := common.HexToAddress(address)
	if bal, err := blockchain.GetBalanceWithClient(client, walletAddr); err == nil {
		nativeBal = fmt.Sprintf("%.6f", floatFromUnits(bal, 18))
	}

	tokenLines := make([]string, 0, len(tokens))
	for _, token := range tokens {
		tokenBal := tokenBalanceDisplay(client, walletAddr, token.Address, token.Decimals)
		tokenLines = append(tokenLines, fmt.Sprintf("💼 %s: %s", token.Symbol, tokenBal))
	}
	return buildText(tokenLines)
}

// getWalletBalancesForChain returns (nativeSym, nativeBal, stableSym, stableBal).
// It is best-effort: when RPC/client/config is missing it returns "N/A" for balances.
func (b *Bot) getWalletBalancesForChain(chain string, address string) (string, string, string, string) {
	_ = b
	chain = config.NormalizeChain(chain)

	nativeSym := nativeSymbolForChain(chain)
	stableSym, stableDecimals, stableAddrStr := stableSymbolForChain(chain)

	if !common.IsHexAddress(address) {
		return nativeSym, "N/A", stableSym, "N/A"
	}

	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil || client == nil {
		return nativeSym, "N/A", stableSym, "N/A"
	}

	walletAddr := common.HexToAddress(address)

	nativeBal := "N/A"
	if bal, err := blockchain.GetBalanceWithClient(client, walletAddr); err == nil {
		nativeBal = fmt.Sprintf("%.6f", floatFromUnits(bal, 18))
	}

	stableBal := "N/A"
	if common.IsHexAddress(stableAddrStr) {
		stableAddr := common.HexToAddress(stableAddrStr)
		if bal, err := blockchain.GetTokenBalanceWithClient(client, stableAddr, walletAddr); err == nil {
			stableBal = fmt.Sprintf("%.2f", floatFromUnits(bal, stableDecimals))
		}
	}

	return nativeSym, nativeBal, stableSym, stableBal
}
