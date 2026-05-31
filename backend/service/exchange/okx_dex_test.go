package exchange

import (
	"TgLpBot/base/config"
	"context"
	"encoding/json"
	"io"
	"net/http"
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

func resetOKXReadCachesForTest() {
	okxBasicInfoCache = sync.Map{}
	okxAdvancedInfoCache = sync.Map{}
	okxCandlesCache = sync.Map{}
	okxDeFiCache = sync.Map{}
	okxBalanceCache = sync.Map{}
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

func TestDeFiUserAssetAPIURL_RewritesAggregatorBase(t *testing.T) {
	svc := &OKXDexService{apiURL: "https://www.okx.com/api/v6/dex/aggregator"}
	got := svc.defiUserAssetAPIURL()
	want := "https://web3.okx.com/api/v6/defi/user/asset"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeMarketCandlesRows_ParsesOfficialOrder(t *testing.T) {
	rows := normalizeMarketCandlesRows([][]string{
		{"1710000000000", "1.0", "1.2", "0.9", "1.1", "123.45", "234.56", "1"},
	})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	row := rows[0]
	if row.TimestampMS != 1710000000000 {
		t.Fatalf("unexpected timestamp: %+v", row)
	}
	if row.Open != 1.0 || row.High != 1.2 || row.Low != 0.9 || row.Close != 1.1 {
		t.Fatalf("unexpected OHLC values: %+v", row)
	}
	if row.Volume != 123.45 || row.VolumeUSD != 234.56 {
		t.Fatalf("unexpected volume values: %+v", row)
	}
	if !row.Confirm {
		t.Fatalf("expected confirm=true, got %+v", row)
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

func TestGetMarketTokenBasicInfos_UsesOfficialEndpoint(t *testing.T) {
	resetOKXReadCachesForTest()
	svc := &OKXDexService{
		apiURL:     "https://www.okx.com/api/v6/dex/aggregator",
		apiKey:     "test-key",
		secretKey:  "test-secret",
		passphrase: "test-pass",
		client: &http.Client{Transport: stubTransport{fn: func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST request, got %s", req.Method)
			}
			if req.URL.String() != "https://web3.okx.com/api/v6/dex/market/token/basic-info" {
				t.Fatalf("unexpected request url: %s", req.URL.String())
			}
			if req.Header.Get("OK-ACCESS-SIGN") == "" {
				t.Fatalf("expected OK-ACCESS-SIGN header")
			}
			rawBody, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
			var payload []MarketTokenBasicInfoRequest
			if err := json.Unmarshal(rawBody, &payload); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if len(payload) != 1 {
				t.Fatalf("expected one request item, got %+v", payload)
			}
			if payload[0].ChainIndex != "56" || payload[0].TokenContractAddress != "0x1111111111111111111111111111111111111111" {
				t.Fatalf("unexpected payload: %+v", payload[0])
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[{"chainIndex":"56","tokenContractAddress":"0x1111111111111111111111111111111111111111","tokenSymbol":"TEST","tokenName":"Test Token","tokenLogoUrl":"https://img.example/test.png"}]}`)),
				Header:     make(http.Header),
			}, nil
		}}},
	}

	resp, err := svc.GetMarketTokenBasicInfos([]MarketTokenBasicInfoRequest{{
		ChainIndex:           "56",
		TokenContractAddress: "0x1111111111111111111111111111111111111111",
	}})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected one data row, got %+v", resp.Data)
	}
	if resp.Data[0].TokenName != "Test Token" || resp.Data[0].TokenLogoURL != "https://img.example/test.png" {
		t.Fatalf("unexpected response row: %+v", resp.Data[0])
	}
}

func TestGetMarketTokenBasicInfos_UsesSharedCacheAcrossInstances(t *testing.T) {
	resetOKXReadCachesForTest()
	calls := 0
	transport := stubTransport{fn: func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[{"chainIndex":"56","tokenContractAddress":"0x1111111111111111111111111111111111111111","tokenSymbol":"TEST","tokenName":"Test Token","tokenLogoUrl":"https://img.example/test.png"}]}`)),
			Header:     make(http.Header),
		}, nil
	}}
	first := &OKXDexService{
		apiURL:     "https://www.okx.com/api/v6/dex/aggregator",
		apiKey:     "test-key",
		secretKey:  "test-secret",
		passphrase: "test-pass",
		client:     &http.Client{Transport: transport},
	}
	second := &OKXDexService{
		apiURL:     "https://www.okx.com/api/v6/dex/aggregator",
		apiKey:     "test-key",
		secretKey:  "test-secret",
		passphrase: "test-pass",
		client:     &http.Client{Transport: transport},
	}

	reqs := []MarketTokenBasicInfoRequest{{
		ChainIndex:           "56",
		TokenContractAddress: "0x1111111111111111111111111111111111111111",
	}}
	if _, err := first.GetMarketTokenBasicInfos(reqs); err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	resp, err := second.GetMarketTokenBasicInfos(reqs)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one upstream call, got %d", calls)
	}
	if len(resp.Data) != 1 || resp.Data[0].TokenSymbol != "TEST" {
		t.Fatalf("unexpected cached response: %+v", resp)
	}
}

func TestGetMarketTokenBasicInfos_CachesMissingToken(t *testing.T) {
	resetOKXReadCachesForTest()
	calls := 0
	svc := &OKXDexService{
		apiURL:     "https://www.okx.com/api/v6/dex/aggregator",
		apiKey:     "test-key",
		secretKey:  "test-secret",
		passphrase: "test-pass",
		client: &http.Client{Transport: stubTransport{fn: func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[]}`)),
				Header:     make(http.Header),
			}, nil
		}}},
	}

	reqs := []MarketTokenBasicInfoRequest{{
		ChainIndex:           "56",
		TokenContractAddress: "0x1111111111111111111111111111111111111111",
	}}
	for i := 0; i < 2; i++ {
		resp, err := svc.GetMarketTokenBasicInfos(reqs)
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		if len(resp.Data) != 0 {
			t.Fatalf("expected empty data, got %+v", resp.Data)
		}
	}
	if calls != 1 {
		t.Fatalf("expected missing basic-info cache to avoid second network call, calls=%d", calls)
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

func TestGetAllTokenBalances_UsesChainsParameter(t *testing.T) {
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
			if got := req.URL.Query().Get("chains"); got != "56" {
				t.Fatalf("expected chains=56, got %q", got)
			}
			if got := req.URL.Query().Get("chainIndex"); got != "" {
				t.Fatalf("expected chainIndex to be absent, got %q", got)
			}
			if got := req.URL.Query().Get("address"); got != "0x1111111111111111111111111111111111111111" {
				t.Fatalf("unexpected address: %q", got)
			}
			if req.Header.Get("OK-ACCESS-SIGN") == "" {
				t.Fatalf("expected OK-ACCESS-SIGN header")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[{"tokenAssets":[]}]}`)),
				Header:     make(http.Header),
			}, nil
		}}},
	}

	resp, err := svc.GetAllTokenBalances("56", "0x1111111111111111111111111111111111111111")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected one data row, got %+v", resp.Data)
	}
}

func TestGetAllTokenBalances_UsesShortSharedCache(t *testing.T) {
	resetOKXReadCachesForTest()
	calls := 0
	svc := &OKXDexService{
		apiURL:     "https://www.okx.com/api/v6/dex/aggregator",
		apiKey:     "test-key",
		secretKey:  "test-secret",
		passphrase: "test-pass",
		client: &http.Client{Transport: stubTransport{fn: func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[{"tokenAssets":[{"tokenContractAddress":"0x1111111111111111111111111111111111111111","symbol":"TEST","balance":"1","tokenPrice":"2"}]}]}`)),
				Header:     make(http.Header),
			}, nil
		}}},
	}

	for i := 0; i < 2; i++ {
		resp, err := svc.GetAllTokenBalances("56", "0x1111111111111111111111111111111111111111")
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		if len(resp.Data) != 1 || len(resp.Data[0].TokenAssets) != 1 {
			t.Fatalf("unexpected response: %+v", resp)
		}
	}
	if calls != 1 {
		t.Fatalf("expected one upstream call, got %d", calls)
	}
}

func TestGetDeFiUserAssetPlatformList_PostsWalletAddressList(t *testing.T) {
	resetOKXReadCachesForTest()
	svc := &OKXDexService{
		apiURL:     "https://www.okx.com/api/v6/dex/aggregator",
		apiKey:     "test-key",
		secretKey:  "test-secret",
		passphrase: "test-pass",
		client: &http.Client{Transport: stubTransport{fn: func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST request, got %s", req.Method)
			}
			if req.URL.String() != "https://web3.okx.com/api/v6/defi/user/asset/platform/list" {
				t.Fatalf("unexpected request url: %s", req.URL.String())
			}
			if req.Header.Get("OK-ACCESS-SIGN") == "" {
				t.Fatalf("expected OK-ACCESS-SIGN header")
			}
			rawBody, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
			var payload DeFiUserAssetPlatformListRequest
			if err := json.Unmarshal(rawBody, &payload); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if len(payload.WalletAddressList) != 2 {
				t.Fatalf("expected two wallet requests, got %+v", payload.WalletAddressList)
			}
			if payload.WalletAddressList[0].ChainIndex != "56" || payload.WalletAddressList[0].WalletAddress != "0x1111111111111111111111111111111111111111" {
				t.Fatalf("unexpected first wallet request: %+v", payload.WalletAddressList[0])
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[{"totalValue":"123.45","platformList":[]}]}`)),
				Header:     make(http.Header),
			}, nil
		}}},
	}

	resp, err := svc.GetDeFiUserAssetPlatformList(context.Background(), DeFiUserAssetPlatformListRequest{
		WalletAddressList: []DeFiWalletAddressRequest{
			{ChainIndex: "56", WalletAddress: "0x1111111111111111111111111111111111111111"},
			{ChainIndex: "8453", WalletAddress: "0x1111111111111111111111111111111111111111"},
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp == nil || resp.Code != "0" || !strings.Contains(string(resp.Data), "platformList") {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGetDeFiUserAssetPlatformList_PreservesSolanaWalletAddress(t *testing.T) {
	resetOKXReadCachesForTest()
	solanaWallet := "9xQeWvG816bUx9EPjHmaT23yvVM2ZW57QbZz3Mz1Yw7"
	svc := &OKXDexService{
		apiURL:     "https://www.okx.com/api/v6/dex/aggregator",
		apiKey:     "test-key",
		secretKey:  "test-secret",
		passphrase: "test-pass",
		client: &http.Client{Transport: stubTransport{fn: func(req *http.Request) (*http.Response, error) {
			rawBody, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
			var payload DeFiUserAssetPlatformListRequest
			if err := json.Unmarshal(rawBody, &payload); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if len(payload.WalletAddressList) != 2 {
				t.Fatalf("expected two wallet requests, got %+v", payload.WalletAddressList)
			}
			if got := payload.WalletAddressList[0].WalletAddress; got != solanaWallet {
				t.Fatalf("expected Solana wallet address to be preserved, got %q", got)
			}
			if got := payload.WalletAddressList[1].WalletAddress; got != "0x1111111111111111111111111111111111111111" {
				t.Fatalf("expected EVM wallet address to be lower-cased, got %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[]}`)),
				Header:     make(http.Header),
			}, nil
		}}},
	}

	_, err := svc.GetDeFiUserAssetPlatformList(context.Background(), DeFiUserAssetPlatformListRequest{
		WalletAddressList: []DeFiWalletAddressRequest{
			{ChainIndex: "501", WalletAddress: solanaWallet},
			{ChainIndex: "56", WalletAddress: "0x1111111111111111111111111111111111111111"},
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestGetDeFiUserAssetPlatformDetail_PostsPlatformList(t *testing.T) {
	resetOKXReadCachesForTest()
	svc := &OKXDexService{
		apiURL:     "https://www.okx.com/api/v6/dex/aggregator",
		apiKey:     "test-key",
		secretKey:  "test-secret",
		passphrase: "test-pass",
		client: &http.Client{Transport: stubTransport{fn: func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST request, got %s", req.Method)
			}
			if req.URL.String() != "https://web3.okx.com/api/v6/defi/user/asset/platform/detail" {
				t.Fatalf("unexpected request url: %s", req.URL.String())
			}
			rawBody, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
			var payload DeFiUserAssetPlatformDetailRequest
			if err := json.Unmarshal(rawBody, &payload); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if len(payload.PlatformList) != 1 {
				t.Fatalf("expected one platform request, got %+v", payload.PlatformList)
			}
			if payload.PlatformList[0].AnalysisPlatformID != "123" || payload.PlatformList[0].ChainIndex != "56" {
				t.Fatalf("unexpected platform request: %+v", payload.PlatformList[0])
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[{"analysisPlatformId":"123","platformName":"PancakeSwap","investmentList":[]}]}`)),
				Header:     make(http.Header),
			}, nil
		}}},
	}

	resp, err := svc.GetDeFiUserAssetPlatformDetail(context.Background(), DeFiUserAssetPlatformDetailRequest{
		WalletAddressList: []DeFiWalletAddressRequest{
			{ChainIndex: "56", WalletAddress: "0x1111111111111111111111111111111111111111"},
		},
		PlatformList: []DeFiPlatformRequest{
			{AnalysisPlatformID: "123", ChainIndex: "56"},
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp == nil || resp.Code != "0" || !strings.Contains(string(resp.Data), "PancakeSwap") {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGetDeFiUserAssetPlatformList_ReturnsAPIError(t *testing.T) {
	resetOKXReadCachesForTest()
	svc := &OKXDexService{
		apiURL:     "https://www.okx.com/api/v6/dex/aggregator",
		apiKey:     "test-key",
		secretKey:  "test-secret",
		passphrase: "test-pass",
		client: &http.Client{Transport: stubTransport{fn: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":"51000","msg":"invalid request","data":[]}`)),
				Header:     make(http.Header),
			}, nil
		}}},
	}

	_, err := svc.GetDeFiUserAssetPlatformList(context.Background(), DeFiUserAssetPlatformListRequest{
		WalletAddressList: []DeFiWalletAddressRequest{
			{ChainIndex: "56", WalletAddress: "0x1111111111111111111111111111111111111111"},
		},
	})
	if err == nil {
		t.Fatalf("expected API error")
	}
	if !strings.Contains(err.Error(), "51000") {
		t.Fatalf("expected code in error, got %v", err)
	}
}

func TestGetDeFiUserAssetPlatformList_DoesNotCacheUpdatingStatus(t *testing.T) {
	resetOKXReadCachesForTest()
	calls := 0
	svc := &OKXDexService{
		apiURL:     "https://www.okx.com/api/v6/dex/aggregator",
		apiKey:     "test-key",
		secretKey:  "test-secret",
		passphrase: "test-pass",
		client: &http.Client{Transport: stubTransport{fn: func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[{"assetStatus":"2","platformList":[]}]}`)),
				Header:     make(http.Header),
			}, nil
		}}},
	}

	req := DeFiUserAssetPlatformListRequest{
		WalletAddressList: []DeFiWalletAddressRequest{
			{ChainIndex: "56", WalletAddress: "0x1111111111111111111111111111111111111111"},
		},
	}
	for i := 0; i < 2; i++ {
		resp, err := svc.GetDeFiUserAssetPlatformList(context.Background(), req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		if resp == nil || !strings.Contains(string(resp.Data), `"assetStatus":"2"`) {
			t.Fatalf("unexpected response: %+v", resp)
		}
	}
	if calls != 2 {
		t.Fatalf("expected updating DeFi response to bypass cache, got calls=%d", calls)
	}
}
