package database

import (
	"TgLpBot/base/config"
	"TgLpBot/base/security"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

var RedisClient *redis.Client
var ctx = context.Background()

// InitRedis initializes Redis connection
func InitRedis() error {
	RedisClient = redis.NewClient(&redis.Options{
		Addr:     config.AppConfig.GetRedisAddr(),
		Password: config.AppConfig.RedisPassword,
		DB:       config.AppConfig.RedisDB,
	})

	// Test connection
	_, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Println("Redis connected successfully")
	return nil
}

// CloseRedis closes the Redis connection
func CloseRedis() error {
	if RedisClient != nil {
		return RedisClient.Close()
	}
	return nil
}

// Session management functions

const sessionEncryptedPrefix = "enc:"

// SetUserSession stores user session data
func SetUserSession(telegramID int64, key string, value interface{}, expiration time.Duration) error {
	sessionKey := fmt.Sprintf("session:%d:%s", telegramID, key)
	return RedisClient.Set(ctx, sessionKey, value, expiration).Err()
}

// SetUserSessionEncrypted stores user session data encrypted with the app ENCRYPTION_KEY.
func SetUserSessionEncrypted(telegramID int64, key string, plaintext string, expiration time.Duration) error {
	if config.AppConfig == nil {
		return fmt.Errorf("config not loaded")
	}
	encKey, err := security.DecodeHexKey32(config.AppConfig.EncryptionKey)
	if err != nil {
		return err
	}
	cipherHex, err := security.EncryptAESGCMToHex(encKey, []byte(plaintext))
	if err != nil {
		return err
	}
	return SetUserSession(telegramID, key, sessionEncryptedPrefix+cipherHex, expiration)
}

// GetUserSession retrieves user session data
func GetUserSession(telegramID int64, key string) (string, error) {
	sessionKey := fmt.Sprintf("session:%d:%s", telegramID, key)
	return RedisClient.Get(ctx, sessionKey).Result()
}

// GetUserSessionDecrypted retrieves encrypted session data stored by SetUserSessionEncrypted.
func GetUserSessionDecrypted(telegramID int64, key string) (string, error) {
	raw, err := GetUserSession(telegramID, key)
	if err != nil {
		return "", err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !strings.HasPrefix(raw, sessionEncryptedPrefix) {
		// Backward compat: older sessions stored plaintext.
		return raw, nil
	}
	raw = strings.TrimPrefix(raw, sessionEncryptedPrefix)

	if config.AppConfig == nil {
		return "", fmt.Errorf("config not loaded")
	}
	encKey, err := security.DecodeHexKey32(config.AppConfig.EncryptionKey)
	if err != nil {
		return "", err
	}
	plaintext, err := security.DecryptAESGCMHex(encKey, raw)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// DeleteUserSession deletes user session data
func DeleteUserSession(telegramID int64, key string) error {
	sessionKey := fmt.Sprintf("session:%d:%s", telegramID, key)
	return RedisClient.Del(ctx, sessionKey).Err()
}

// ClearUserSession clears all session data for a user
func ClearUserSession(telegramID int64) error {
	pattern := fmt.Sprintf("session:%d:*", telegramID)
	iter := RedisClient.Scan(ctx, 0, pattern, 0).Iterator()

	for iter.Next(ctx) {
		if err := RedisClient.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}

	return iter.Err()
}

// Cache functions

// SetCache stores data in cache
func SetCache(key string, value interface{}, expiration time.Duration) error {
	return RedisClient.Set(ctx, key, value, expiration).Err()
}

// GetCache retrieves data from cache
func GetCache(key string) (string, error) {
	return RedisClient.Get(ctx, key).Result()
}

// DeleteCache deletes data from cache
func DeleteCache(key string) error {
	return RedisClient.Del(ctx, key).Err()
}

// ExistsCache checks if key exists in cache
func ExistsCache(key string) (bool, error) {
	result, err := RedisClient.Exists(ctx, key).Result()
	return result > 0, err
}
