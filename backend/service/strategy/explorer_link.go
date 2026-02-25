package strategy

import (
	"TgLpBot/base/config"
	"fmt"
	"strings"
)

func explorerTxURL(chain string, txHash string) string {
	txHash = strings.TrimSpace(txHash)
	if txHash == "" {
		return ""
	}
	if url := config.ExplorerTxURL(chain, txHash); url != "" {
		return url
	}
	// Best-effort legacy fallback (keeps behavior when CHAINS/config is incomplete).
	switch config.NormalizeChain(chain) {
	case "base":
		return fmt.Sprintf("https://basescan.org/tx/%s", txHash)
	default:
		return fmt.Sprintf("https://bscscan.com/tx/%s", txHash)
	}
}
