package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"TgLpBot/base/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var (
	minioClientOnce sync.Once
	minioClient     *minio.Client
	minioClientErr  error
)

var smartMoneyAvatarExtensions = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
}

func UploadSmartMoneyAvatar(ctx context.Context, walletAddress string, contentType string, data []byte) (string, error) {
	cfg := config.AppConfig
	if cfg == nil {
		return "", fmt.Errorf("config not loaded")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("avatar file is empty")
	}

	bucket := strings.TrimSpace(cfg.MinIOAvatarBucket)
	if bucket == "" {
		bucket = "avatar"
	}

	client, err := getMinIOClient(cfg)
	if err != nil {
		return "", err
	}

	objectKey := buildSmartMoneyAvatarObjectKey(walletAddress, contentType)
	_, err = client.PutObject(
		ctx,
		bucket,
		objectKey,
		bytes.NewReader(data),
		int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return "", fmt.Errorf("upload avatar to minio: %w", err)
	}

	return buildMinIOPublicObjectURL(cfg, bucket, objectKey), nil
}

func buildSmartMoneyAvatarObjectKey(walletAddress string, contentType string) string {
	walletAddress = strings.ToLower(strings.TrimSpace(walletAddress))
	if walletAddress == "" {
		walletAddress = "unknown"
	}

	ext := smartMoneyAvatarExtensions[strings.ToLower(strings.TrimSpace(contentType))]
	if ext == "" {
		ext = ".bin"
	}

	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		copy(suffix[:], []byte("rand"))
	}

	filename := fmt.Sprintf("%d-%s%s", time.Now().UTC().Unix(), hex.EncodeToString(suffix[:]), ext)
	return path.Join("smart-money", walletAddress, filename)
}

func buildMinIOPublicObjectURL(cfg *config.Config, bucket string, objectKey string) string {
	base := normalizeMinIOPublicBaseURL(cfg)
	if base == "" {
		return ""
	}

	parsed, err := url.Parse(base)
	if err != nil {
		base = strings.TrimRight(base, "/")
		return base + "/" + strings.TrimLeft(bucket, "/") + "/" + strings.TrimLeft(objectKey, "/")
	}

	parsed.Path = path.Join(parsed.Path, bucket, strings.TrimLeft(objectKey, "/"))
	return parsed.String()
}

func normalizeMinIOPublicBaseURL(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}

	base := strings.TrimSpace(cfg.MinIOPublicBaseURL)
	if base == "" {
		endpoint, secure := normalizeMinIOEndpoint(cfg.MinIOEndpoint, cfg.MinIOUseSSL)
		if endpoint == "" {
			return ""
		}
		scheme := "http"
		if secure {
			scheme = "https"
		}
		base = scheme + "://" + endpoint
	}

	if !strings.HasPrefix(strings.ToLower(base), "http://") && !strings.HasPrefix(strings.ToLower(base), "https://") {
		scheme := "http"
		if cfg.MinIOUseSSL {
			scheme = "https"
		}
		base = scheme + "://" + base
	}

	return strings.TrimRight(base, "/")
}

func getMinIOClient(cfg *config.Config) (*minio.Client, error) {
	minioClientOnce.Do(func() {
		if cfg == nil {
			minioClientErr = fmt.Errorf("config not loaded")
			return
		}

		endpoint, secure := normalizeMinIOEndpoint(cfg.MinIOEndpoint, cfg.MinIOUseSSL)
		if endpoint == "" {
			minioClientErr = fmt.Errorf("MINIO_ENDPOINT not configured")
			return
		}
		if strings.TrimSpace(cfg.MinIOAccessKey) == "" || strings.TrimSpace(cfg.MinIOSecretKey) == "" {
			minioClientErr = fmt.Errorf("MINIO_ACCESS_KEY or MINIO_SECRET_KEY not configured")
			return
		}

		minioClient, minioClientErr = minio.New(endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
			Secure: secure,
		})
	})

	return minioClient, minioClientErr
}

func normalizeMinIOEndpoint(raw string, fallbackSecure bool) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fallbackSecure
	}

	if strings.HasPrefix(strings.ToLower(value), "http://") || strings.HasPrefix(strings.ToLower(value), "https://") {
		parsed, err := url.Parse(value)
		if err == nil {
			secure := fallbackSecure
			if parsed.Scheme == "https" {
				secure = true
			} else if parsed.Scheme == "http" {
				secure = false
			}
			return parsed.Host, secure
		}
	}

	return strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(value, "https://"), "http://"), "/"), fallbackSecure
}
