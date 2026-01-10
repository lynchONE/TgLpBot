package blacklist

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"TgLpBot/base/database"

	"github.com/go-redis/redis/v8"
)

const (
	// 黑名单 Key 前缀
	blacklistKeyPrefix = "blacklist:"
)

// BlacklistService 黑名单服务
type BlacklistService struct{}

// NewBlacklistService 创建黑名单服务实例
func NewBlacklistService() *BlacklistService {
	return &BlacklistService{}
}

// blacklistKey 生成用户黑名单的 Redis Key
func blacklistKey(userID uint) string {
	return fmt.Sprintf("%s%d", blacklistKeyPrefix, userID)
}

// normalizePoolAddress 规范化池子地址
func normalizePoolAddress(addr string) string {
	raw := strings.TrimSpace(addr)
	if raw == "" {
		return ""
	}

	lower := strings.ToLower(raw)
	hex := lower
	if strings.HasPrefix(hex, "0x") {
		hex = hex[2:]
	}
	if isHexStringN(hex, 40) || isHexStringN(hex, 64) {
		return "0x" + hex
	}
	return lower
}

func isHexStringN(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func legacyPoolAddress(canonical string) string {
	if !strings.HasPrefix(canonical, "0x") {
		return ""
	}
	hex := canonical[2:]
	if !isHexStringN(hex, 40) && !isHexStringN(hex, 64) {
		return ""
	}
	return hex
}

// isRedisAvailable 检查 Redis 是否可用
func isRedisAvailable() bool {
	return database.RedisClient != nil
}

// Add 添加池子到黑名单
func (s *BlacklistService) Add(userID uint, poolAddress string) error {
	if !isRedisAvailable() {
		return fmt.Errorf("Redis 不可用")
	}
	if userID == 0 {
		return fmt.Errorf("userID 无效")
	}
	addr := normalizePoolAddress(poolAddress)
	if addr == "" {
		return fmt.Errorf("poolAddress 无效")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	key := blacklistKey(userID)
	legacy := legacyPoolAddress(addr)

	pipe := database.RedisClient.Pipeline()
	pipe.SAdd(ctx, key, addr)
	if legacy != "" {
		pipe.SRem(ctx, key, legacy)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("[Blacklist] 添加黑名单失败: user_id=%d pool=%s err=%v", userID, addr, err)
		return err
	}

	log.Printf("[Blacklist] 添加黑名单: user_id=%d pool=%s", userID, addr)
	return nil
}

// Remove 从黑名单移除池子
func (s *BlacklistService) Remove(userID uint, poolAddress string) error {
	if !isRedisAvailable() {
		return fmt.Errorf("Redis 不可用")
	}
	if userID == 0 {
		return fmt.Errorf("userID 无效")
	}
	addr := normalizePoolAddress(poolAddress)
	if addr == "" {
		return fmt.Errorf("poolAddress 无效")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	key := blacklistKey(userID)
	legacy := legacyPoolAddress(addr)

	pipe := database.RedisClient.Pipeline()
	pipe.SRem(ctx, key, addr)
	if legacy != "" {
		pipe.SRem(ctx, key, legacy)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("[Blacklist] 移除黑名单失败: user_id=%d pool=%s err=%v", userID, addr, err)
		return err
	}

	log.Printf("[Blacklist] 移除黑名单: user_id=%d pool=%s", userID, addr)
	return nil
}

// IsBlacklisted 检查池子是否在黑名单中
func (s *BlacklistService) IsBlacklisted(userID uint, poolAddress string) bool {
	if !isRedisAvailable() {
		return false
	}
	if userID == 0 {
		return false
	}
	addr := normalizePoolAddress(poolAddress)
	if addr == "" {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	key := blacklistKey(userID)
	result, err := database.RedisClient.SIsMember(ctx, key, addr).Result()
	if err != nil && err != redis.Nil {
		log.Printf("[Blacklist] 检查黑名单失败: user_id=%d pool=%s err=%v", userID, addr, err)
		return false
	}

	if result {
		return true
	}

	legacy := legacyPoolAddress(addr)
	if legacy == "" {
		return false
	}
	result, err = database.RedisClient.SIsMember(ctx, key, legacy).Result()
	if err != nil && err != redis.Nil {
		log.Printf("[Blacklist] 检查黑名单失败: user_id=%d pool=%s err=%v", userID, legacy, err)
		return false
	}
	return result
}

// GetAll 获取用户的所有黑名单池子
func (s *BlacklistService) GetAll(userID uint) ([]string, error) {
	if !isRedisAvailable() {
		return nil, fmt.Errorf("Redis 不可用")
	}
	if userID == 0 {
		return nil, fmt.Errorf("userID 无效")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	key := blacklistKey(userID)
	members, err := database.RedisClient.SMembers(ctx, key).Result()
	if err != nil && err != redis.Nil {
		log.Printf("[Blacklist] 获取黑名单列表失败: user_id=%d err=%v", userID, err)
		return nil, err
	}

	dedup := make(map[string]struct{}, len(members))
	var toAdd []string
	var toRem []string

	for _, m := range members {
		stored := strings.ToLower(strings.TrimSpace(m))
		if stored == "" {
			continue
		}
		canonical := normalizePoolAddress(stored)
		if canonical == "" {
			continue
		}
		dedup[canonical] = struct{}{}
		if stored != canonical {
			toAdd = append(toAdd, canonical)
			toRem = append(toRem, stored)
		}
	}

	if len(toAdd) > 0 || len(toRem) > 0 {
		pipe := database.RedisClient.Pipeline()
		for _, v := range toAdd {
			pipe.SAdd(ctx, key, v)
		}
		for _, v := range toRem {
			pipe.SRem(ctx, key, v)
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			log.Printf("[Blacklist] 清理旧黑名单格式失败: user_id=%d err=%v", userID, err)
		}
	}

	out := make([]string, 0, len(dedup))
	for v := range dedup {
		out = append(out, v)
	}
	sort.Strings(out)
	return out, nil
}

// Count 获取用户黑名单池子数量
func (s *BlacklistService) Count(userID uint) int64 {
	if !isRedisAvailable() {
		return 0
	}
	if userID == 0 {
		return 0
	}
	list, err := s.GetAll(userID)
	if err != nil {
		return 0
	}
	return int64(len(list))
}
