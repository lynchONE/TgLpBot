package liquidity

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"fmt"
)

type PrivateZapInvalidationResult struct {
	Chain            string `json:"chain"`
	ClearedBindings  int64  `json:"cleared_bindings"`
	ClearedCacheKeys int64  `json:"cleared_cache_keys"`
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

func (s *LiquidityService) InvalidatePrivateZapBindingsByChain(ctx context.Context, chain string) (*PrivateZapInvalidationResult, error) {
	chain = config.NormalizeChain(chain)
	if chain == "" {
		return nil, fmt.Errorf("chain required")
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

	result := &PrivateZapInvalidationResult{Chain: chain}

	if cleared, err := clearPrivateZapCacheByChain(ctx, chain); err != nil {
		return nil, err
	} else {
		result.ClearedCacheKeys += cleared
	}

	tx := database.DB.WithContext(ctx).Model(&models.WalletChainContract{}).
		Where("chain = ? AND kind = ?", chain, walletChainContractKindZapSimple).
		Updates(map[string]interface{}{
			"contract_address": "",
			"deploy_tx_hash":   "",
			"config_tx_hash":   "",
		})
	if tx.Error != nil {
		return nil, tx.Error
	}
	result.ClearedBindings = tx.RowsAffected

	if cleared, err := clearPrivateZapCacheByChain(ctx, chain); err != nil {
		return nil, err
	} else {
		result.ClearedCacheKeys += cleared
	}

	return result, nil
}

func clearPrivateZapCacheByChain(ctx context.Context, chain string) (int64, error) {
	if database.RedisClient == nil {
		return 0, nil
	}
	pattern := privateZapCacheScanPattern(chain)
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
