package token_metadata

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/exchange"
	"context"
	"errors"
	"testing"
	"time"
)

type fakeOKXClient struct {
	resp *exchange.MarketTokenBasicInfoResponse
	err  error
}

func (f fakeOKXClient) GetMarketTokenBasicInfos(reqs []exchange.MarketTokenBasicInfoRequest) (*exchange.MarketTokenBasicInfoResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
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

func TestGetBatch_FetchesFromOKXWithoutStorage(t *testing.T) {
	prevCfg := config.AppConfig
	config.AppConfig = &config.Config{
		Chains: map[string]config.ChainConfig{
			"bsc": {Chain: "bsc", ChainID: 56},
		},
	}
	defer func() {
		config.AppConfig = prevCfg
	}()

	svc := NewServiceWithClient(fakeOKXClient{
		resp: &exchange.MarketTokenBasicInfoResponse{
			Code: "0",
			Data: []exchange.MarketTokenBasicInfo{
				{
					ChainIndex:           "56",
					TokenContractAddress: "0x1111111111111111111111111111111111111111",
					TokenSymbol:          "TEST",
					TokenName:            "Test Token",
					TokenLogoURL:         "https://img.example/test.png",
				},
			},
		},
	})

	got, err := svc.GetBatch(context.Background(), "bsc", []string{"0x1111111111111111111111111111111111111111"})
	if err != nil {
		t.Fatalf("GetBatch error: %v", err)
	}
	meta, ok := got["0x1111111111111111111111111111111111111111"]
	if !ok {
		t.Fatalf("expected metadata result, got %#v", got)
	}
	if meta.Symbol != "TEST" || meta.Name != "Test Token" || meta.LogoURL != "https://img.example/test.png" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	if meta.Status != statusOK {
		t.Fatalf("expected status %s, got %s", statusOK, meta.Status)
	}
}

func TestCacheFromModel_RoundTrip(t *testing.T) {
	meta := models.TokenMetadata{
		Chain:        "bsc",
		TokenAddress: "0x1111111111111111111111111111111111111111",
		Symbol:       "ABC",
		Name:         "ABC Token",
		LogoURL:      "https://img.example/abc.png",
		Source:       sourceOKX,
		Status:       statusOK,
	}
	entry := cacheFromModel(meta)
	back := modelFromCache(entry)
	if back.TokenAddress != meta.TokenAddress || back.Symbol != meta.Symbol || back.LogoURL != meta.LogoURL {
		t.Fatalf("round trip mismatch: %#v != %#v", back, meta)
	}
}

func TestShouldRefreshMetadata_RequiresLogoBackfill(t *testing.T) {
	if !shouldRefreshMetadata(models.TokenMetadata{
		Status: statusOK,
	}) {
		t.Fatalf("expected empty-logo ok metadata to require refresh")
	}

	if shouldRefreshMetadata(models.TokenMetadata{
		Status:  statusOK,
		LogoURL: "https://img.example/a.png",
	}) {
		t.Fatalf("expected metadata with logo to skip refresh")
	}

	if shouldRefreshMetadata(models.TokenMetadata{
		Status:    statusNotFound,
		FetchedAt: time.Now(),
	}) {
		t.Fatalf("expected fresh negative cache metadata to skip refresh")
	}

	if !shouldRefreshMetadata(models.TokenMetadata{
		Status:    statusNotFound,
		FetchedAt: time.Now().Add(-negativeRetry).Add(-time.Minute),
	}) {
		t.Fatalf("expected stale negative cache metadata to refresh")
	}

	if !shouldRefreshMetadata(models.TokenMetadata{}) {
		t.Fatalf("expected unknown-status metadata to refresh")
	}
}

func TestGetBatch_ReturnsPartialDataWhenOKXRefreshFails(t *testing.T) {
	prevCfg := config.AppConfig
	config.AppConfig = &config.Config{
		Chains: map[string]config.ChainConfig{
			"bsc": {Chain: "bsc", ChainID: 56},
		},
	}
	defer func() {
		config.AppConfig = prevCfg
	}()

	svc := NewServiceWithClient(fakeOKXClient{
		err: errors.New("okx unavailable"),
	})

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
