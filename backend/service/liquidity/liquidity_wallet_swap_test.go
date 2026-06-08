package liquidity

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"TgLpBot/base/config"
	"TgLpBot/service/exchange"

	"github.com/ethereum/go-ethereum/common"
)

type liquidityWalletSwapTestTransport struct {
	fn func(req *http.Request) (*http.Response, error)
}

func (t liquidityWalletSwapTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.fn(req)
}

func TestCollectOKXWalletSwapTokensUsesOKXBalances(t *testing.T) {
	wallet := common.HexToAddress("0x0000000000000000000000000000000000000001")
	swapToken := common.HexToAddress("0x1111111111111111111111111111111111111111")
	stable := common.HexToAddress("0x55d398326f99059ff775485246999027b3197955")
	wrapped := common.HexToAddress("0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c")

	okxSvc := exchange.NewStaticOKXDexService("https://web3.okx.com/api/v6/dex/aggregator", "key", "secret", "pass", &http.Client{
		Transport: liquidityWalletSwapTestTransport{fn: func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/v6/dex/balance/all-token-balances-by-address" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			if got := req.URL.Query().Get("address"); !strings.EqualFold(got, wallet.Hex()) {
				t.Fatalf("address = %q, want %s", got, wallet.Hex())
			}
			if got := req.URL.Query().Get("chains"); got != "56" {
				t.Fatalf("chains = %q, want 56", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{"code":"0","data":[{"tokenAssets":[
					{"tokenContractAddress":"0x1111111111111111111111111111111111111111","symbol":"AAA","tokenDecimal":"18","balance":"1.5","rawBalance":"1500000000000000000","tokenPrice":"2"},
					{"tokenContractAddress":"0x55d398326f99059ff775485246999027b3197955","symbol":"USDT","tokenDecimal":"18","balance":"10","rawBalance":"10000000000000000000","tokenPrice":"1"},
					{"tokenContractAddress":"0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c","symbol":"WBNB","tokenDecimal":"18","balance":"1","rawBalance":"1000000000000000000","tokenPrice":"600"},
					{"tokenContractAddress":"0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","symbol":"BNB","tokenDecimal":"18","balance":"1","rawBalance":"1000000000000000000","tokenPrice":"600"}
				]}]}`)),
			}, nil
		}},
	})

	svc := &LiquidityService{okxService: okxSvc}
	tokens, err := svc.collectOKXWalletSwapTokens("bsc", config.ChainConfig{
		ChainID:              56,
		StableAddress:        stable.Hex(),
		USDTAddress:          stable.Hex(),
		WrappedNativeAddress: wrapped.Hex(),
	}, wallet, 0.1)
	if err != nil {
		t.Fatalf("collectOKXWalletSwapTokens failed: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("len(tokens) = %d, want 1: %+v", len(tokens), tokens)
	}
	got := tokens[0]
	if got.Address != swapToken || got.Symbol != "AAA" || got.Balance != "1.5" || got.ValueUSDT != 3 {
		t.Fatalf("unexpected token: %+v", got)
	}
	if got.RawBalance.String() != "1500000000000000000" || got.Decimals != 18 {
		t.Fatalf("unexpected raw balance/decimals: balance=%s decimals=%d", got.RawBalance.String(), got.Decimals)
	}
}
