package storage

import (
	"strings"
	"testing"

	"TgLpBot/base/config"
)

func TestNormalizeMinIOEndpoint(t *testing.T) {
	endpoint, secure := normalizeMinIOEndpoint("https://minio.example.com:9000", false)
	if endpoint != "minio.example.com:9000" {
		t.Fatalf("unexpected endpoint: %s", endpoint)
	}
	if !secure {
		t.Fatalf("expected secure endpoint")
	}
}

func TestBuildMinIOPublicObjectURL(t *testing.T) {
	cfg := &config.Config{
		MinIOEndpoint:      "minio.internal:9000",
		MinIOUseSSL:        true,
		MinIOPublicBaseURL: "https://cdn.example.com/storage",
	}

	got := buildMinIOPublicObjectURL(cfg, "avatar", "smart-money/0xabc/avatar.png")
	want := "https://cdn.example.com/storage/avatar/smart-money/0xabc/avatar.png"
	if got != want {
		t.Fatalf("unexpected url: got %s want %s", got, want)
	}
}

func TestBuildSmartMoneyAvatarObjectKey(t *testing.T) {
	key := buildSmartMoneyAvatarObjectKey("0xAbC", "image/png")
	if !strings.HasPrefix(key, "smart-money/0xabc/") {
		t.Fatalf("unexpected key prefix: %s", key)
	}
	if !strings.HasSuffix(key, ".png") {
		t.Fatalf("unexpected key suffix: %s", key)
	}
}
