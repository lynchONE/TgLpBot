package web_server

import (
	"TgLpBot/base/database"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

const poolsCacheTTL = 10 * time.Second

func readRedisRawCache(key string) ([]byte, bool) {
	if database.RedisClient == nil {
		return nil, false
	}
	raw, err := database.GetCache(key)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.Printf("[WebCache] redis get failed key=%s err=%v", key, err)
		}
		return nil, false
	}
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}
	return []byte(raw), true
}

func writeRedisRawCache(key string, payload []byte, expiration time.Duration) {
	if database.RedisClient == nil || len(payload) == 0 {
		return
	}
	if err := database.SetCache(key, string(payload), expiration); err != nil {
		log.Printf("[WebCache] redis set failed key=%s err=%v", key, err)
	}
}
