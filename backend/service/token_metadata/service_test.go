package token_metadata

import (
	"TgLpBot/base/models"
	"context"
	"errors"
	"testing"
	"time"
)

type fakeMetadataProvider struct {
	data map[string]models.TokenMetadata
	err  error
}

func (f fakeMetadataProvider) GetBatch(ctx context.Context, chain string, addresses []string) (map[string]models.TokenMetadata, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]models.TokenMetadata)
	for _, addr := range normalizeAddresses(addresses) {
		if meta, ok := f.data[addr]; ok {
			meta.Chain = chain
			meta.TokenAddress = addr
			out[addr] = meta
		}
	}
	return out, nil
}

func TestNormalizeTokenAddress(t *testing.T) {
	got := NormalizeTokenAddress("0x55d398326f99059ff775485246999027b3197955")
	if got != "0x55d398326f99059ff775485246999027b3197955" {
		t.Fatalf("unexpected normalized address: %s", got)
	}
}

func TestNormalizeAddresses_DedupesAndSorts(t *testing.T) {
	got := normalizeAddresses([]string{
		"0x55d398326f99059ff775485246999027b3197955",
		"0x8ac76a51cc950d9822d68b83fe1ad97b32cd580d",
		"0x55d398326f99059ff775485246999027b3197955",
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 unique addresses, got %d", len(got))
	}
	if got[0] != "0x55d398326f99059ff775485246999027b3197955" {
		t.Fatalf("unexpected sort order: %#v", got)
	}
}

func TestGetBatch_FetchesFromProvidersWithoutStorage(t *testing.T) {
	token := "0x1111111111111111111111111111111111111111"
	svc := NewServiceWithProviders(
		fakeMetadataProvider{data: map[string]models.TokenMetadata{
			token: {Symbol: "TEST", Name: "Test Token", Source: sourceRPC, Status: statusOK},
		}},
		fakeMetadataProvider{data: map[string]models.TokenMetadata{
			token: {LogoURL: "https://img.example/test.png", Source: sourceGecko, Status: statusOK},
		}},
	)

	got, err := svc.GetBatch(context.Background(), "bsc", []string{token})
	if err != nil {
		t.Fatalf("GetBatch error: %v", err)
	}
	meta, ok := got[token]
	if !ok {
		t.Fatalf("expected metadata result, got %#v", got)
	}
	if meta.Symbol != "TEST" || meta.Name != "Test Token" || meta.LogoURL != "https://img.example/test.png" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	if meta.Source != sourceRPC+"+"+sourceGecko {
		t.Fatalf("expected merged source, got %s", meta.Source)
	}
	if meta.Status != statusOK {
		t.Fatalf("expected status %s, got %s", statusOK, meta.Status)
	}
}

func TestGetBatch_UsesRPCWhenDisplayProviderFails(t *testing.T) {
	token := "0x1111111111111111111111111111111111111111"
	svc := NewServiceWithProviders(
		fakeMetadataProvider{data: map[string]models.TokenMetadata{
			token: {Symbol: "TEST", Name: "Test Token", Source: sourceRPC, Status: statusOK},
		}},
		fakeMetadataProvider{err: errors.New("gecko unavailable")},
	)

	got, err := svc.GetBatch(context.Background(), "bsc", []string{token})
	if err != nil {
		t.Fatalf("GetBatch error: %v", err)
	}
	meta, ok := got[token]
	if !ok {
		t.Fatalf("expected metadata result, got %#v", got)
	}
	if meta.Symbol != "TEST" || meta.Name != "Test Token" || meta.LogoURL != "" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
}

func TestCacheFromModel_RoundTrip(t *testing.T) {
	meta := models.TokenMetadata{
		Chain:        "bsc",
		TokenAddress: "0x1111111111111111111111111111111111111111",
		Symbol:       "ABC",
		Name:         "ABC Token",
		LogoURL:      "https://img.example/abc.png",
		Source:       sourceRPC,
		Status:       statusOK,
	}
	entry := cacheFromModel(meta)
	back := modelFromCache(entry)
	if back.TokenAddress != meta.TokenAddress || back.Symbol != meta.Symbol || back.LogoURL != meta.LogoURL {
		t.Fatalf("round trip mismatch: %#v != %#v", back, meta)
	}
}

func TestShouldRefreshMetadata_RequiresLogoBackfill(t *testing.T) {
	if !shouldRefreshMetadata(models.TokenMetadata{Status: statusOK}) {
		t.Fatalf("expected empty-logo ok metadata to require refresh")
	}

	if shouldRefreshMetadata(models.TokenMetadata{Status: statusOK, LogoURL: "https://img.example/a.png"}) {
		t.Fatalf("expected metadata with logo to skip refresh")
	}

	if shouldRefreshMetadata(models.TokenMetadata{Status: statusNotFound, FetchedAt: time.Now()}) {
		t.Fatalf("expected fresh negative cache metadata to skip refresh")
	}

	if !shouldRefreshMetadata(models.TokenMetadata{Status: statusNotFound, FetchedAt: time.Now().Add(-negativeRetry).Add(-time.Minute)}) {
		t.Fatalf("expected stale negative cache metadata to refresh")
	}

	if !shouldRefreshMetadata(models.TokenMetadata{}) {
		t.Fatalf("expected unknown-status metadata to refresh")
	}
}

func TestGetBatch_ReturnsPartialDataWhenRefreshFails(t *testing.T) {
	svc := NewServiceWithProviders(fakeMetadataProvider{err: errors.New("rpc unavailable")}, nil)

	now := time.Now()
	originalReadCacheBatch := readCacheBatchFn
	readCacheBatchFn = func(chain string, addresses []string) (map[string]models.TokenMetadata, error) {
		return map[string]models.TokenMetadata{
			"0x1111111111111111111111111111111111111111": {
				Chain:        chain,
				TokenAddress: "0x1111111111111111111111111111111111111111",
				Symbol:       "TEST",
				Name:         "Test Token",
				LogoURL:      "https://img.example/test.png",
				Status:       statusOK,
				FetchedAt:    now,
				ExpiresAt:    now.Add(time.Hour),
			},
		}, nil
	}
	defer func() {
		readCacheBatchFn = originalReadCacheBatch
	}()

	got, err := svc.GetBatch(context.Background(), "bsc", []string{
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
	})
	if err != nil {
		t.Fatalf("GetBatch error: %v", err)
	}
	meta, ok := got["0x1111111111111111111111111111111111111111"]
	if !ok {
		t.Fatalf("expected cached metadata to be preserved, got %#v", got)
	}
	if meta.LogoURL != "https://img.example/test.png" {
		t.Fatalf("unexpected cached logo url: %#v", meta)
	}
}
