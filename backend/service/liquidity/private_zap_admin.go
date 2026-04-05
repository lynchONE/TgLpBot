package liquidity

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"fmt"
	"strings"
)

type PrivateZapInvalidationResult struct {
	Chain            string `json:"chain"`
	Kind             string `json:"kind"`
	ClearedBindings  int64  `json:"cleared_bindings"`
	ClearedCacheKeys int64  `json:"cleared_cache_keys"`
}

func SupportedPrivateZapKinds() []string {
	return []string{
		walletChainContractKindZapSimple,
		walletChainContractKindAtomicIncreaseZap,
	}
}

func normalizePrivateZapKind(kind string) (string, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return walletChainContractKindZapSimple, nil
	}
	switch kind {
	case walletChainContractKindZapSimple, walletChainContractKindAtomicIncreaseZap, "all":
		return kind, nil
	default:
		return "", fmt.Errorf("unsupported private zap kind: %s", kind)
	}
}

func EnabledPrivateZapChains() []string {
	if config.AppConfig == nil {
		return nil
	}
	enabled := config.EnabledChainsNormalized()
	if len(enabled) == 0 {
		return nil
	}
	out := make([]string, 0, len(enabled))
	for _, chain := range enabled {
		cc, ok := config.AppConfig.GetChainConfig(chain)
		if !ok || cc.Kind != config.ChainKindEVM {
			continue
		}
		out = append(out, chain)
	}
	return out
}

func (s *LiquidityService) InvalidatePrivateZapBindingsByChain(ctx context.Context, chain string, kind string) (*PrivateZapInvalidationResult, error) {
	chain = config.NormalizeChain(chain)
	if chain == "" {
		return nil, fmt.Errorf("chain required")
	}
	normalizedKind, err := normalizePrivateZapKind(kind)
	if err != nil {
		return nil, err
	}
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	if _, ok := config.AppConfig.GetChainConfig(chain); !ok {
		return nil, fmt.Errorf("unsupported chain: %s", chain)
	}
	if database.DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := &PrivateZapInvalidationResult{Chain: chain, Kind: normalizedKind}

	kinds := []string{normalizedKind}
	if normalizedKind == "all" {
		kinds = SupportedPrivateZapKinds()
	}

	for _, currentKind := range kinds {
		if cleared, err := clearPrivateContractCacheByChain(ctx, chain, currentKind); err != nil {
			return nil, err
		} else {
			result.ClearedCacheKeys += cleared
		}

		tx := database.DB.WithContext(ctx).Model(&models.WalletChainContract{}).
			Where("chain = ? AND kind = ?", chain, currentKind).
			Updates(map[string]interface{}{
				"status":           "",
				"contract_address": "",
				"deploy_tx_hash":   "",
				"config_tx_hash":   "",
			})
		if tx.Error != nil {
			return nil, tx.Error
		}
		result.ClearedBindings += tx.RowsAffected

		if cleared, err := clearPrivateContractCacheByChain(ctx, chain, currentKind); err != nil {
			return nil, err
		} else {
			result.ClearedCacheKeys += cleared
		}
	}

	return result, nil
}

func clearPrivateContractCacheByChain(ctx context.Context, chain string, kind string) (int64, error) {
	if database.RedisClient == nil {
		return 0, nil
	}
	pattern := fmt.Sprintf("%s:%s:*:%s", privateContractCachePrefix(kind), config.NormalizeChain(chain), kind)
	iter := database.RedisClient.Scan(ctx, 0, pattern, 0).Iterator()
	var deleted int64
	for iter.Next(ctx) {
		n, err := database.RedisClient.Del(ctx, iter.Val()).Result()
		if err != nil {
			return deleted, err
		}
		deleted += n
	}
	if err := iter.Err(); err != nil {
		return deleted, err
	}
	return deleted, nil
}
