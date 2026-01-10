package blacklist

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"TgLpBot/base/database"

	"github.com/go-redis/redis/v8"
)

const (
	// 冷却 Key 前缀
	cooldownKeyPrefix = "cooldown:"
	// 默认冷却时间 30 分钟
	DefaultCooldownDuration = 30 * time.Minute
)

// CooldownService 冷却服务
type CooldownService struct{}

// NewCooldownService 创建冷却服务实例
func NewCooldownService() *CooldownService {
	return &CooldownService{}
}

// cooldownKey 生成冷却的 Redis Key
func cooldownKey(userID uint, tradingPair string) string {
	pair := normalizeTradingPair(tradingPair)
	return fmt.Sprintf("%s%d:%s", cooldownKeyPrefix, userID, pair)
}

// cooldownScanPattern 生成用户冷却扫描模式
func cooldownScanPattern(userID uint) string {
	return fmt.Sprintf("%s%d:*", cooldownKeyPrefix, userID)
}

// normalizeTradingPair 规范化交易对名称
func normalizeTradingPair(pair string) string {
	return strings.ToUpper(strings.TrimSpace(pair))
}

// CooldownInfo 冷却信息
type CooldownInfo struct {
	TradingPair   string        `json:"trading_pair"`
	Reason        string        `json:"reason"`
	RemainingTime time.Duration `json:"remaining_time"`
	ExpiresAt     time.Time     `json:"expires_at"`
}

// Add 添加交易对冷却
func (s *CooldownService) Add(userID uint, tradingPair string, reason string, duration time.Duration) error {
	if !isRedisAvailable() {
		return fmt.Errorf("Redis 不可用")
	}
	if userID == 0 {
		return fmt.Errorf("userID 无效")
	}
	pair := normalizeTradingPair(tradingPair)
	if pair == "" {
		return fmt.Errorf("tradingPair 无效")
	}
	if duration <= 0 {
		duration = DefaultCooldownDuration
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	key := cooldownKey(userID, pair)
	value := strings.TrimSpace(reason)
	if value == "" {
		value = "连续跌破触发冷却"
	}

	if err := database.RedisClient.Set(ctx, key, value, duration).Err(); err != nil {
		log.Printf("[Cooldown] 添加冷却失败: user_id=%d pair=%s err=%v", userID, pair, err)
		return err
	}

	log.Printf("[Cooldown] 添加冷却: user_id=%d pair=%s duration=%v reason=%s", userID, pair, duration, value)
	return nil
}

// IsCoolingDown 检查交易对是否在冷却中
func (s *CooldownService) IsCoolingDown(userID uint, tradingPair string) bool {
	if !isRedisAvailable() {
		return false
	}
	if userID == 0 {
		return false
	}
	pair := normalizeTradingPair(tradingPair)
	if pair == "" {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	key := cooldownKey(userID, pair)
	exists, err := database.RedisClient.Exists(ctx, key).Result()
	if err != nil && err != redis.Nil {
		log.Printf("[Cooldown] 检查冷却失败: user_id=%d pair=%s err=%v", userID, pair, err)
		return false
	}

	return exists > 0
}

// GetInfo 获取单个交易对的冷却信息
func (s *CooldownService) GetInfo(userID uint, tradingPair string) *CooldownInfo {
	if !isRedisAvailable() {
		return nil
	}
	if userID == 0 {
		return nil
	}
	pair := normalizeTradingPair(tradingPair)
	if pair == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	key := cooldownKey(userID, pair)

	// 获取值和 TTL
	reason, err := database.RedisClient.Get(ctx, key).Result()
	if err != nil {
		return nil
	}

	ttl, err := database.RedisClient.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		return nil
	}

	return &CooldownInfo{
		TradingPair:   pair,
		Reason:        reason,
		RemainingTime: ttl,
		ExpiresAt:     time.Now().Add(ttl),
	}
}

// GetAll 获取用户所有冷却中的交易对
func (s *CooldownService) GetAll(userID uint) ([]CooldownInfo, error) {
	if !isRedisAvailable() {
		return nil, fmt.Errorf("Redis 不可用")
	}
	if userID == 0 {
		return nil, fmt.Errorf("userID 无效")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pattern := cooldownScanPattern(userID)
	var cooldowns []CooldownInfo

	// 使用 SCAN 遍历匹配的 Key
	iter := database.RedisClient.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()

		// 从 key 中提取交易对名称
		// key 格式: cooldown:{userID}:{tradingPair}
		prefix := fmt.Sprintf("%s%d:", cooldownKeyPrefix, userID)
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		tradingPair := key[len(prefix):]

		// 获取值和 TTL
		reason, err := database.RedisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		ttl, err := database.RedisClient.TTL(ctx, key).Result()
		if err != nil || ttl <= 0 {
			continue
		}

		cooldowns = append(cooldowns, CooldownInfo{
			TradingPair:   tradingPair,
			Reason:        reason,
			RemainingTime: ttl,
			ExpiresAt:     time.Now().Add(ttl),
		})
	}

	if err := iter.Err(); err != nil {
		log.Printf("[Cooldown] 扫描冷却列表失败: user_id=%d err=%v", userID, err)
		return nil, err
	}

	return cooldowns, nil
}

// Remove 移除交易对冷却（提前解除冷却）
func (s *CooldownService) Remove(userID uint, tradingPair string) error {
	if !isRedisAvailable() {
		return fmt.Errorf("Redis 不可用")
	}
	if userID == 0 {
		return fmt.Errorf("userID 无效")
	}
	pair := normalizeTradingPair(tradingPair)
	if pair == "" {
		return fmt.Errorf("tradingPair 无效")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	key := cooldownKey(userID, pair)
	if err := database.RedisClient.Del(ctx, key).Err(); err != nil {
		log.Printf("[Cooldown] 移除冷却失败: user_id=%d pair=%s err=%v", userID, pair, err)
		return err
	}

	log.Printf("[Cooldown] 移除冷却: user_id=%d pair=%s", userID, pair)
	return nil
}
