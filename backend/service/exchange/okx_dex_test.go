package exchange

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/okxpool"
	"context"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sync/singleflight"
)

type stubTransport struct {
	fn func(req *http.Request) (*http.Response, error)
}

func (s stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return s.fn(req)
}

type okxDexMemStore struct {
	mu   sync.Mutex
	rows []models.OKXAPIConfig
}

func (s *okxDexMemStore) ListAll(ctx context.Context) ([]models.OKXAPIConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.OKXAPIConfig, len(s.rows))
	copy(out, s.rows)
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsCurrent != out[j].IsCurrent {
			return out[i].IsCurrent
		}
		if out[i].IsEnabled != out[j].IsEnabled {
			return out[i].IsEnabled
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *okxDexMemStore) ListEnabled(ctx context.Context) ([]models.OKXAPIConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []models.OKXAPIConfig
	for _, row := range s.rows {
		if row.IsEnabled {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsCurrent != out[j].IsCurrent {
			return out[i].IsCurrent
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *okxDexMemStore) GetByID(ctx context.Context, id uint) (*models.OKXAPIConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.rows {
		if row.ID == id {
			cp := row
			return &cp, nil
		}
	}
	return nil, nil
}

func (s *okxDexMemStore) Create(ctx context.Context, row *models.OKXAPIConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var maxID uint
	for _, existing := range s.rows {
		if existing.ID > maxID {
			maxID = existing.ID
		}
	}
	if row.ID == 0 {
		row.ID = maxID + 1
	}
	s.rows = append(s.rows, *row)
	return nil
}

func (s *okxDexMemStore) UpdateByID(ctx context.Context, id uint, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.rows {
		if s.rows[i].ID != id {
			continue
		}
		v := reflect.ValueOf(&s.rows[i]).Elem()
		for k, raw := range updates {
			f := v.FieldByName(k)
			if !f.IsValid() || !f.CanSet() {
				continue
			}
			if raw == nil {
				f.Set(reflect.Zero(f.Type()))
				continue
			}
			rv := reflect.ValueOf(raw)
			if rv.Type().AssignableTo(f.Type()) {
				f.Set(rv)
				continue
			}
			if rv.Type().ConvertibleTo(f.Type()) {
				f.Set(rv.Convert(f.Type()))
			}
		}
		return nil
	}
	return nil
}

func (s *okxDexMemStore) DeleteByID(ctx context.Context, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.rows {
		if s.rows[i].ID == id {
			s.rows = append(s.rows[:i], s.rows[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *okxDexMemStore) SetCurrent(ctx context.Context, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.rows {
		s.rows[i].IsCurrent = s.rows[i].ID == id
	}
	return nil
}

func (s *okxDexMemStore) UnsetCurrent(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.rows {
		s.rows[i].IsCurrent = false
	}
	return nil
}

func resetOKXReadCachesForTest() {
	okxAdvancedInfoCache = sync.Map{}
	okxApproveSpenderCache = sync.Map{}
	okxReadGroup = singleflight.Group{}
}

func TestMarketAPIURL_RewritesAggregatorBase(t *testing.T) {
	svc := &OKXDexService{apiURL: "https://www.okx.com/api/v6/dex/aggregator"}
	got := svc.marketAPIURL()
	want := "https://web3.okx.com/api/v6/dex/market"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeOKXSwapFeePercent_TruncatesToNineDecimals(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		chainID string
		want    string
	}{
		{name: "minimum representable", raw: "0.0000000001", chainID: "56", want: "0.000000001"},
		{name: "truncate extra decimals", raw: "1.3269018736", chainID: "56", want: "1.326901873"},
		{name: "evm cap", raw: "4", chainID: "56", want: "3"},
		{name: "solana cap", raw: "12", chainID: "501", want: "10"},
		{name: "zero disables", raw: "0", chainID: "56", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeOKXSwapFeePercent(tc.raw, tc.chainID); got != tc.want {
				t.Fatalf("normalizeOKXSwapFeePercent(%q, %q) = %q, want %q", tc.raw, tc.chainID, got, tc.want)
			}
		})
	}
}

func TestGetSwapData_AppliesDefaultReferrerFee(t *testing.T) {
	oldConfig := config.AppConfig
	config.AppConfig = &config.Config{
		OKXSwapFeeRecipient: "0x7FC630A70948A8d21cD7C7cFA8f203D7b7e120F2",
		OKXSwapFeePercent:   "0.000000001",
		OKXSwapFeeToken:     "to",
	}
	t.Cleanup(func() {
		config.AppConfig = oldConfig
	})

	svc := &OKXDexService{
		apiURL:     "https://www.okx.com/api/v6/dex/aggregator",
		apiKey:     "test-key",
		secretKey:  "test-secret",
		passphrase: "test-pass",
		client: &http.Client{Transport: stubTransport{fn: func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("expected GET request, got %s", req.Method)
			}
			if req.URL.Path != "/api/v6/dex/aggregator/swap" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			query := req.URL.Query()
			if got := query.Get("chainIndex"); got != "56" {
				t.Fatalf("expected chainIndex=56, got %q", got)
			}
			if got := query.Get("slippagePercent"); got != "0.5" {
				t.Fatalf("expected slippagePercent=0.5, got %q", got)
			}
			if got := query.Get("feePercent"); got != "0.000000001" {
				t.Fatalf("expected feePercent=0.000000001, got %q", got)
			}
			if got := query.Get("toTokenReferrerWalletAddress"); got != "0x7FC630A70948A8d21cD7C7cFA8f203D7b7e120F2" {
				t.Fatalf("unexpected toTokenReferrerWalletAddress: %q", got)
			}
			if got := query.Get("fromTokenReferrerWalletAddress"); got != "" {
				t.Fatalf("expected no fromTokenReferrerWalletAddress, got %q", got)
			}
			if req.Header.Get("OK-ACCESS-SIGN") == "" {
				t.Fatalf("expected OK-ACCESS-SIGN header")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[{"tx":{"from":"0x1111111111111111111111111111111111111111","to":"0x2222222222222222222222222222222222222222","data":"0x1234","value":"0","gas":"21000","gasPrice":"1"},"routerResult":{"fromTokenAmount":"100","toTokenAmount":"99","dexRouterList":[]}}]}`)),
				Header:     make(http.Header),
			}, nil
		}}},
	}

	resp, err := svc.GetSwapData(SwapRequest{
		ChainID:           "56",
		FromTokenAddress:  "0x1111111111111111111111111111111111111111",
		ToTokenAddress:    "0x2222222222222222222222222222222222222222",
		Amount:            "100",
		Slippage:          "0.005",
		UserWalletAddress: "0x3333333333333333333333333333333333333333",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp == nil || len(resp.Data) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGetSwapData_UsesOKXPoolAndRetriesNextConfigOnRateLimit(t *testing.T) {
	oldConfig := config.AppConfig
	config.AppConfig = &config.Config{
		EncryptionKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	t.Cleanup(func() {
		config.AppConfig = oldConfig
	})

	ctx := context.Background()
	store := &okxDexMemStore{}
	mgr := okxpool.NewManager(store, nil)
	first, err := mgr.AddConfig(ctx, okxpool.Input{
		Name:       "rate-limited",
		BaseURL:    "https://okx-a.example/api/v6/dex/aggregator",
		APIKey:     "key-a",
		SecretKey:  "secret-a",
		Passphrase: "pass-a",
		SetCurrent: true,
	})
	if err != nil {
		t.Fatalf("add first config failed: %v", err)
	}
	second, err := mgr.AddConfig(ctx, okxpool.Input{
		Name:       "healthy",
		BaseURL:    "https://okx-b.example/api/v6/dex/aggregator",
		APIKey:     "key-b",
		SecretKey:  "secret-b",
		Passphrase: "pass-b",
	})
	if err != nil {
		t.Fatalf("add second config failed: %v", err)
	}

	var hosts []string
	svc := &OKXDexService{
		usePool: true,
		pool:    mgr,
		client: &http.Client{Transport: stubTransport{fn: func(req *http.Request) (*http.Response, error) {
			hosts = append(hosts, req.URL.Host)
			switch req.URL.Host {
			case "okx-a.example":
				if got := req.Header.Get("OK-ACCESS-KEY"); got != "key-a" {
					t.Fatalf("expected first request key-a, got %q", got)
				}
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Body:       io.NopCloser(strings.NewReader(`{"code":"50011","msg":"rate limit"}`)),
					Header:     make(http.Header),
				}, nil
			case "okx-b.example":
				if got := req.Header.Get("OK-ACCESS-KEY"); got != "key-b" {
					t.Fatalf("expected second request key-b, got %q", got)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[{"tx":{"from":"0x1111111111111111111111111111111111111111","to":"0x2222222222222222222222222222222222222222","data":"0x1234","value":"0","gas":"21000","gasPrice":"1"},"routerResult":{"fromTokenAmount":"100","toTokenAmount":"99","dexRouterList":[]}}]}`)),
					Header:     make(http.Header),
				}, nil
			default:
				t.Fatalf("unexpected host: %s", req.URL.Host)
				return nil, nil
			}
		}}},
	}

	resp, err := svc.GetSwapData(SwapRequest{
		ChainID:           "56",
		FromTokenAddress:  "0x1111111111111111111111111111111111111111",
		ToTokenAddress:    "0x2222222222222222222222222222222222222222",
		Amount:            "100",
		Slippage:          "0.005",
		UserWalletAddress: "0x3333333333333333333333333333333333333333",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp == nil || len(resp.Data) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if strings.Join(hosts, ",") != "okx-a.example,okx-b.example" {
		t.Fatalf("unexpected request hosts: %v", hosts)
	}

	firstRow, _ := store.GetByID(ctx, first.ID)
	secondRow, _ := store.GetByID(ctx, second.ID)
	if firstRow == nil || secondRow == nil {
		t.Fatalf("expected both configs to exist")
	}
	if firstRow.IsCurrent {
		t.Fatalf("expected rate-limited config current flag cleared")
	}
	if firstRow.DisabledReason != okxpool.ReasonRateLimited {
		t.Fatalf("expected disabled_reason=%q, got %q", okxpool.ReasonRateLimited, firstRow.DisabledReason)
	}
	if !secondRow.IsCurrent {
		t.Fatalf("expected healthy config promoted to current")
	}
}

func TestGetMarketTokenAdvancedInfo_UsesOfficialEndpoint(t *testing.T) {
	resetOKXReadCachesForTest()
	svc := &OKXDexService{
		apiURL:     "https://www.okx.com/api/v6/dex/aggregator",
		apiKey:     "test-key",
		secretKey:  "test-secret",
		passphrase: "test-pass",
		client: &http.Client{Transport: stubTransport{fn: func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("expected GET request, got %s", req.Method)
			}
			if req.URL.Scheme != "https" || req.URL.Host != "web3.okx.com" {
				t.Fatalf("unexpected request host: %s", req.URL.String())
			}
			if req.URL.Path != "/api/v6/dex/market/token/advanced-info" {
				t.Fatalf("unexpected request path: %s", req.URL.Path)
			}
			if got := req.URL.Query().Get("chainIndex"); got != "56" {
				t.Fatalf("expected chainIndex=56, got %q", got)
			}
			if got := req.URL.Query().Get("tokenContractAddress"); got != "0x1111111111111111111111111111111111111111" {
				t.Fatalf("unexpected tokenContractAddress: %q", got)
			}
			if req.Header.Get("OK-ACCESS-SIGN") == "" {
				t.Fatalf("expected OK-ACCESS-SIGN header")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[{"chainIndex":"56","tokenContractAddress":"0x1111111111111111111111111111111111111111","riskControlLevel":"4","tokenTags":["honeypot","lowLiquidity"],"top10HoldPercent":"0.82"}]}`)),
				Header:     make(http.Header),
			}, nil
		}}},
	}

	resp, err := svc.GetMarketTokenAdvancedInfo(context.Background(), MarketTokenAdvancedInfoRequest{
		ChainIndex:           "56",
		TokenContractAddress: "0x1111111111111111111111111111111111111111",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected one data row, got %+v", resp.Data)
	}
	row := resp.Data[0]
	if row.RiskControlLevel != "4" || len(row.TokenTags) != 2 || row.Top10HoldPercent != "0.82" {
		t.Fatalf("unexpected response row: %+v", row)
	}
}
