package web_server

import (
	"context"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"

	"TgLpBot/base/config"
	"TgLpBot/service/exchange"

	"github.com/ethereum/go-ethereum/common"
)

type walletSwapAPITestTransport struct {
	fn func(req *http.Request) (*http.Response, error)
}

func (t walletSwapAPITestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.fn(req)
}

func TestWalletSwapTokenValueUSDT_StableDoesNotCallOKX(t *testing.T) {
	called := false
	okxSvc := exchange.NewStaticOKXDexService("https://web3.okx.com/api/v6/dex/aggregator", "key", "secret", "pass", &http.Client{
		Transport: walletSwapAPITestTransport{fn: func(req *http.Request) (*http.Response, error) {
			called = true
			return nil, nil
		}},
	})

	cc := config.ChainConfig{
		ChainID:        56,
		StableAddress:  "0x55d398326f99059ff775485246999027b3197955",
		StableDecimals: 18,
	}
	amount, ok := new(big.Int).SetString("123450000000000000000", 10)
	if !ok {
		t.Fatalf("invalid test amount")
	}

	value, err := walletSwapTokenValueUSDT(context.Background(), okxSvc, cc, common.HexToAddress(cc.StableAddress), amount, 18, common.HexToAddress("0x0000000000000000000000000000000000000001"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != 123.45 {
		t.Fatalf("value = %f, want 123.45", value)
	}
	if called {
		t.Fatalf("stable token value should not call OKX")
	}
}

func TestWalletSwapOKXValueUSDT_UsesOKXQuoteOutput(t *testing.T) {
	cc := config.ChainConfig{
		ChainID:        56,
		StableAddress:  "0x55d398326f99059ff775485246999027b3197955",
		StableDecimals: 18,
	}
	token := common.HexToAddress("0x1111111111111111111111111111111111111111")
	wallet := common.HexToAddress("0x0000000000000000000000000000000000000001")
	amount := big.NewInt(1000)

	okxSvc := exchange.NewStaticOKXDexService("https://web3.okx.com/api/v6/dex/aggregator", "key", "secret", "pass", &http.Client{
		Transport: walletSwapAPITestTransport{fn: func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("method = %s, want GET", req.Method)
			}
			if req.URL.Path != "/api/v6/dex/aggregator/swap" {
				t.Fatalf("path = %s", req.URL.Path)
			}
			query := req.URL.Query()
			if query.Get("chainIndex") != "56" {
				t.Fatalf("chainIndex = %q", query.Get("chainIndex"))
			}
			if !strings.EqualFold(query.Get("fromTokenAddress"), token.Hex()) {
				t.Fatalf("fromTokenAddress = %q", query.Get("fromTokenAddress"))
			}
			if !strings.EqualFold(query.Get("toTokenAddress"), cc.StableAddress) {
				t.Fatalf("toTokenAddress = %q", query.Get("toTokenAddress"))
			}
			if query.Get("amount") != amount.String() {
				t.Fatalf("amount = %q", query.Get("amount"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"code":"0","data":[{"routerResult":{"fromTokenAmount":"1000","toTokenAmount":"2500000000000000000"}}]}`)),
				Header:     make(http.Header),
			}, nil
		}},
	})

	value, err := walletSwapOKXValueUSDT(context.Background(), okxSvc, cc, token, amount, wallet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != 2.5 {
		t.Fatalf("value = %f, want 2.5", value)
	}
}

func TestOKXBalanceHelpers_ParseDecimalsAndValue(t *testing.T) {
	cc := config.ChainConfig{
		StableAddress: "0x55d398326f99059ff775485246999027b3197955",
		USDTAddress:   "0x55d398326f99059ff775485246999027b3197955",
	}
	token := exchange.BalanceTokenAsset{
		TokenContractAddress: "0x1111111111111111111111111111111111111111",
		TokenDecimal:         "18",
		Balance:              "1.5",
		RawBalance:           "1500000000000000000",
		TokenPrice:           "2.5",
	}
	raw, ok := parseOKXRawBalance(token.RawBalance)
	if !ok {
		t.Fatalf("parseOKXRawBalance failed")
	}
	if decimals := okxBalanceTokenDecimals(token, raw); decimals != 18 {
		t.Fatalf("decimals = %d, want 18", decimals)
	}
	value := okxBalanceValueUSDT(raw, 18, token.TokenPrice, token.TokenContractAddress, cc)
	if value != 3.75 {
		t.Fatalf("value = %f, want 3.75", value)
	}
}

func TestOKXBalanceHelpers_StableValueUsesRawBalance(t *testing.T) {
	cc := config.ChainConfig{
		StableAddress: "0x55d398326f99059ff775485246999027b3197955",
		USDTAddress:   "0x55d398326f99059ff775485246999027b3197955",
	}
	raw, ok := parseOKXRawBalance("123450000")
	if !ok {
		t.Fatalf("parseOKXRawBalance failed")
	}
	asset := exchange.BalanceTokenAsset{
		TokenContractAddress: cc.StableAddress,
		TokenDecimal:         "6",
		Balance:              "123.45",
		RawBalance:           "123450000",
	}
	decimals := okxBalanceTokenDecimals(asset, raw)
	if decimals != 6 {
		t.Fatalf("decimals = %d, want 6", decimals)
	}
	value := okxBalanceValueUSDT(raw, decimals, "", cc.StableAddress, cc)
	if value != 123.45 {
		t.Fatalf("value = %f, want 123.45", value)
	}
}
