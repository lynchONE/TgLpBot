package web_server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleSMDeFiOverview_RejectsInvalidWallet(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/sm/defi_overview?address=bad", nil)
	rec := httptest.NewRecorder()

	(&Server{}).handleSMDeFiOverview(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleSMDeFiDetail_RequiresPlatformID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/sm/defi_detail?address=0x1111111111111111111111111111111111111111", nil)
	rec := httptest.NewRecorder()

	(&Server{}).handleSMDeFiDetail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOKXDeFiNormalizeOverview_OfficialPlatformListShape(t *testing.T) {
	raw := json.RawMessage(`[
		{
			"walletIdPlatformList": [
				{
					"walletAddress": "0x1111111111111111111111111111111111111111",
					"totalAssets": "123.45",
					"platformList": [
						{
							"analysisPlatformId": "12",
							"platformName": "PancakeSwap",
							"platformLogo": "https://img.example/pancake.png",
							"currencyAmount": "100.5",
							"networkBalanceVoList": [
								{"networkChainId": "56", "currencyAmount": "100.5"}
							]
						}
					]
				}
			]
		}
	]`)

	out, err := okxDeFiNormalizeOverview("0x1111111111111111111111111111111111111111", []string{"56"}, raw, time.Unix(1700000000, 0).UTC())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if out.TotalValue != "123.45" || out.TotalValueUSD == nil || *out.TotalValueUSD != 123.45 {
		t.Fatalf("unexpected total value: %+v", out)
	}
	if len(out.Platforms) != 1 {
		t.Fatalf("expected one platform, got %+v", out.Platforms)
	}
	platform := out.Platforms[0]
	if platform.AnalysisPlatformID != "12" || platform.PlatformName != "PancakeSwap" {
		t.Fatalf("unexpected platform: %+v", platform)
	}
	if platform.TotalValue != "100.5" || platform.TotalValueUSD == nil || *platform.TotalValueUSD != 100.5 {
		t.Fatalf("unexpected platform value: %+v", platform)
	}
	if platform.ChainIndex != "56" || platform.ChainName != "BSC" {
		t.Fatalf("unexpected platform chain: %+v", platform)
	}
	if len(platform.NetworkBalances) != 1 || platform.NetworkBalances[0].TotalValue != "100.5" {
		t.Fatalf("unexpected network balances: %+v", platform.NetworkBalances)
	}
}

func TestOKXDeFiNormalizeDetail_OfficialDetailShape(t *testing.T) {
	raw := json.RawMessage(`[
		{
			"walletIdPlatformDetailList": [
				{
					"analysisPlatformId": "12",
					"platformName": "PancakeSwap",
					"networkHoldVoList": [
						{
							"networkChainId": "56",
							"investTokenBalanceVoList": [
								{
									"investmentName": "CAKE/USDT",
									"currencyAmount": "88.8",
									"positionList": [
										{
											"positionId": "77",
											"liquidityPoolToken": "CAKE/USDT",
											"currencyAmount": "88.8",
											"lowerPrice": "1.20",
											"upperPrice": "2.40",
											"tickLower": "-120",
											"tickUpper": "120",
											"unclaimFeesDefiTokenInfo": [
												{"tokenSymbol": "CAKE", "currencyAmount": "1.25"},
												{"tokenSymbol": "USDT", "currencyAmount": "2.75"}
											]
										}
									]
								}
							]
						}
					]
				}
			]
		}
	]`)

	out, err := okxDeFiNormalizeDetail("0x1111111111111111111111111111111111111111", "12", "56", []string{"56"}, raw, time.Unix(1700000000, 0).UTC())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if out.Platform.AnalysisPlatformID != "12" || out.Platform.PlatformName != "PancakeSwap" {
		t.Fatalf("unexpected platform: %+v", out.Platform)
	}
	if len(out.Positions) != 1 {
		t.Fatalf("expected one position, got %+v", out.Positions)
	}
	pos := out.Positions[0]
	if pos.PositionID != "77" || pos.Name != "CAKE/USDT" {
		t.Fatalf("unexpected position identity: %+v", pos)
	}
	if pos.ChainIndex != "56" || pos.ChainName != "BSC" {
		t.Fatalf("expected inherited chain context, got %+v", pos)
	}
	if pos.PositionAmount != "88.8" || pos.PositionAmountUSD == nil || *pos.PositionAmountUSD != 88.8 {
		t.Fatalf("unexpected position amount: %+v", pos)
	}
	if pos.Fee != "4" || pos.FeeUSD == nil || *pos.FeeUSD != 4 {
		t.Fatalf("unexpected fee aggregation: %+v", pos)
	}
	if pos.RangeText != "1.20 - 2.40" || pos.TickLower != "-120" || pos.TickUpper != "120" {
		t.Fatalf("unexpected range: %+v", pos)
	}
}
