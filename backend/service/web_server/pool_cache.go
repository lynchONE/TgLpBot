package web_server

import (
	"TgLpBot/base/database"
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

const poolsCacheTTL = 10 * time.Second
const poolsCacheAccessTimeout = 150 * time.Millisecond

func readRedisRawCache(key string) ([]byte, bool) {
	if database.RedisClient == nil || strings.TrimSpace(key) == "" {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), poolsCacheAccessTimeout)
	defer cancel()

	raw, err := database.RedisClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, false
		}
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
	if database.RedisClient == nil || len(payload) == 0 || strings.TrimSpace(key) == "" || expiration <= 0 {
		return
	}
	data := append([]byte(nil), payload...)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), poolsCacheAccessTimeout)
		defer cancel()
		if err := database.RedisClient.Set(ctx, key, string(data), expiration).Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("[WebCache] redis set failed key=%s err=%v", key, err)
		}
	}()
}
