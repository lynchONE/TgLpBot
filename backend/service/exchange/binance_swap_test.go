package exchange

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBinanceGetAggregatedQuote_UsesOfficialGETEndpointAndParsesRoutes(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/build/api/v1/dex/aggregator/quote" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("binanceChainId") != "56" {
			t.Fatalf("binanceChainId = %q", r.URL.Query().Get("binanceChainId"))
		}
		if r.URL.Query().Get("amount") != "1000000" {
			t.Fatalf("amount = %q", r.URL.Query().Get("amount"))
		}
		if r.Header.Get("X-OC-APIKEY") != "api-key" {
			t.Fatalf("missing api key header")
		}
		if strings.TrimSpace(r.Header.Get("X-OC-TIMESTAMP")) == "" {
			t.Fatalf("missing timestamp header")
		}
		if strings.TrimSpace(r.Header.Get("X-OC-SIGN")) == "" {
			t.Fatalf("missing sign header")
		}
		if r.Header.Get("X-OC-RECV-WINDOW") != "5000" {
			t.Fatalf("recv window = %q", r.Header.Get("X-OC-RECV-WINDOW"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code":0,
			"msg":"success",
			"success":true,
			"timestamp":1710000000000,
			"data":[{
				"quoteId":"quote-1",
				"vendorName":"Lifi",
				"binanceChainId":"56",
				"fromTokenAmount":"1000000",
				"toTokenAmount":"998500000000000000",
				"tradeFee":"0.12",
				"estimateGasFee":"150000",
				"priceImpactPercent":"-0.01",
				"router":"0x55--0xbb",
				"fromToken":{"tokenContractAddress":"0x55d398326f99059ff775485246999027b3197955","tokenSymbol":"USDT","tokenUnitPrice":"1.0002","decimal":"18","isHoneyPot":false,"taxRate":"0"},
				"toToken":{"tokenContractAddress":"0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c","tokenSymbol":"WBNB","tokenUnitPrice":"600","decimal":"18","isHoneyPot":false,"taxRate":"0"},
				"dexRouterList":[{
					"dexProtocol":{"dexName":"Uniswap V3","percent":"85.99"},
					"fromToken":{"tokenContractAddress":"0x55d398326f99059ff775485246999027b3197955","tokenSymbol":"USDT"},
					"fromTokenIndex":"0",
					"toToken":{"tokenContractAddress":"0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c","tokenSymbol":"WBNB"},
					"toTokenIndex":"1"
				}],
				"executionMode":"SWAP",
				"approveTarget":"0xc67879F4065d3B9fe1C09EE990B891Aa8E3a4c2f",
				"isBest":true
			}]
		}`))
	}))
	defer server.Close()

	svc := NewStaticBinanceSwapService(server.URL, "api-key", "secret", "/build/api/v1/dex/aggregator/quote", "/build/api/v1/dex/aggregator/swap", 5000, server.Client())
	resp, err := svc.GetAggregatedQuote(BinanceQuoteRequest{
		BinanceChainID:    "56",
		Amount:            "1000000",
		FromTokenAddress:  "0x55d398326f99059ff775485246999027b3197955",
		ToTokenAddress:    "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c",
		UserWalletAddress: "0xd8da6bf26964af9d7eed9e03e53415d37aa96045",
	})
	if err != nil {
		t.Fatalf("GetAggregatedQuote error: %v", err)
	}
	if gotPath == "" {
		t.Fatalf("server was not called")
	}
	if len(resp.Data) != 1 {
		t.Fatalf("routes = %d, want 1", len(resp.Data))
	}
	route := resp.Data[0]
	if route.QuoteID != "quote-1" || route.VendorName != "Lifi" || route.ToTokenAmount != "998500000000000000" {
		t.Fatalf("unexpected route: %+v", route)
	}
	if len(route.DexRouterList) != 1 || route.DexRouterList[0].DexProtocol.DexName != "Uniswap V3" {
		t.Fatalf("dex routes not parsed: %+v", route.DexRouterList)
	}
}

func TestBinanceBuildSwapTransaction_UsesOfficialSwapEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/build/api/v1/dex/aggregator/swap" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("quoteId") != "quote-1" {
			t.Fatalf("quoteId = %q", r.URL.Query().Get("quoteId"))
		}
		if r.URL.Query().Get("slippagePercent") != "0.5" {
			t.Fatalf("slippagePercent = %q", r.URL.Query().Get("slippagePercent"))
		}
		if r.URL.Query().Get("approveTransaction") != "true" {
			t.Fatalf("approveTransaction = %q", r.URL.Query().Get("approveTransaction"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code":0,
			"msg":"success",
			"success":true,
			"timestamp":1710000000000,
			"data":{
				"routerResult":{
					"binanceChainId":"56",
					"vendorName":"Lifi",
					"fromTokenAmount":"1000000",
					"toTokenAmount":"998500000000000000",
					"dexRouterList":[]
				},
				"tx":{
					"from":"0xd8da6bf26964af9d7eed9e03e53415d37aa96045",
					"to":"0x1111111254EEB25477B68fb85Ed929f73A960582",
					"data":"0x12aa3caf",
					"value":"0",
					"gas":"200000",
					"gasPrice":"5000000000",
					"minReceiveAmount":"993507500000000000",
					"slippagePercent":"0.5",
					"signatureData":["0xc67879F4065d3B9fe1C09EE990B891Aa8E3a4c2f"]
				},
				"executionMode":"SWAP"
			}
		}`))
	}))
	defer server.Close()

	svc := NewStaticBinanceSwapService(server.URL, "api-key", "secret", "/build/api/v1/dex/aggregator/quote", "/build/api/v1/dex/aggregator/swap", 5000, server.Client())
	resp, err := svc.BuildSwapTransaction(BinanceBuildSwapRequest{
		BinanceChainID:     "56",
		Amount:             "1000000",
		FromTokenAddress:   "0x55d398326f99059ff775485246999027b3197955",
		ToTokenAddress:     "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c",
		UserWalletAddress:  "0xd8da6bf26964af9d7eed9e03e53415d37aa96045",
		QuoteID:            "quote-1",
		SlippagePercent:    "0.5",
		ApproveTransaction: "true",
	})
	if err != nil {
		t.Fatalf("BuildSwapTransaction error: %v", err)
	}
	if resp.Data.ExecutionMode != "SWAP" {
		t.Fatalf("executionMode = %q", resp.Data.ExecutionMode)
	}
	if resp.Data.Tx.To != "0x1111111254EEB25477B68fb85Ed929f73A960582" {
		t.Fatalf("tx.to = %q", resp.Data.Tx.To)
	}
	if len(resp.Data.Tx.SignatureData) != 1 {
		t.Fatalf("signatureData = %+v", resp.Data.Tx.SignatureData)
	}
}
