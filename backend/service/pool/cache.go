package pool

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

const poolInfoCachePrefix = "pool:info"
const poolInfoCacheTTL = 24 * time.Hour

func poolInfoCacheKey(chain string, poolVersion string, poolID string) string {
	chain = config.NormalizeChain(chain)
	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	poolID = strings.ToLower(strings.TrimSpace(poolID))
	return fmt.Sprintf("%s:%s:%s:%s", poolInfoCachePrefix, chain, poolVersion, poolID)
}

func readPoolInfoCache(chain string, poolVersion string, poolID string) (*PoolInfo, bool) {
	if database.RedisClient == nil {
		return nil, false
	}
	key := poolInfoCacheKey(chain, poolVersion, poolID)
	raw, err := database.GetCache(key)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.Printf("[PoolService] warning: redis get failed key=%s err=%v", key, err)
		}
		return nil, false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}

	var info PoolInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		log.Printf("[PoolService] warning: redis unmarshal failed key=%s err=%v", key, err)
		_ = database.DeleteCache(key)
		return nil, false
	}
	if strings.TrimSpace(info.Address) == "" {
		_ = database.DeleteCache(key)
		return nil, false
	}
	return &info, true
}

func writePoolInfoCache(chain string, poolVersion string, poolID string, info *PoolInfo) {
	if database.RedisClient == nil || info == nil {
		return
	}
	key := poolInfoCacheKey(chain, poolVersion, poolID)
	b, err := json.Marshal(info)
	if err != nil {
		log.Printf("[PoolService] warning: redis marshal failed key=%s err=%v", key, err)
		return
	}
	if err := database.SetCache(key, string(b), poolInfoCacheTTL); err != nil {
		log.Printf("[PoolService] warning: redis set failed key=%s err=%v", key, err)
	}
}

func (s *PoolService) GetPoolInfoForVersionCached(chain string, poolVersion string, poolID string) (*PoolInfo, error) {
	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	if poolVersion == "" {
		poolVersion = "v3"
	}
	if cached, ok := readPoolInfoCache(chain, poolVersion, poolID); ok {
		return cached, nil
	}

	var (
		info *PoolInfo
		err  error
	)
	switch poolVersion {
	case "v4":
		info, err = s.GetV4PoolInfo(poolID)
	default:
		info, err = s.GetPoolInfoForChain(chain, poolID)
	}
	if err != nil {
		return nil, err
	}
	writePoolInfoCache(chain, poolVersion, poolID, info)
	return info, nil
}
