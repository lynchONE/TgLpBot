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

func TestOKXDeFiNormalizeWalletAddress_AcceptsSolana(t *testing.T) {
	addr := "9xQeWvG816bUx9EPjHmaT23yvVM2ZW57QbZz3Mz1Yw7"
	if got := okxDeFiNormalizeWalletAddress(addr); got != addr {
		t.Fatalf("expected Solana address to be preserved, got %q", got)
	}

	if got := okxDeFiNormalizeWalletAddress("0xABCDEFabcdefABCDEFabcdefABCDEFabcdefabcd"); got != "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd" {
		t.Fatalf("expected EVM address to be lower-cased, got %q", got)
	}

	if got := okxDeFiNormalizeWalletAddress("0OIl"); got != "" {
		t.Fatalf("expected invalid address to be rejected, got %q", got)
	}
}

func TestOKXDeFiRequestedChainIndexes_IncludesSolana(t *testing.T) {
	defaults := okxDeFiRequestedChainIndexes(nil)
	hasSolana := false
	for _, chainIndex := range defaults {
		if chainIndex == "501" {
			hasSolana = true
			break
		}
	}
	if !hasSolana {
		t.Fatalf("expected default chain indexes to include Solana 501, got %+v", defaults)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sm/defi_overview?chain=solana", nil)
	indexes := okxDeFiRequestedChainIndexes(req)
	if len(indexes) != 1 || indexes[0] != "501" {
		t.Fatalf("expected chain=solana to normalize to 501, got %+v", indexes)
	}
	if name := okxDeFiChainName("501"); name != "Solana" {
		t.Fatalf("expected Solana chain name, got %q", name)
	}
}

func TestOKXDeFiCompatibleChainIndexes_MatchesWalletType(t *testing.T) {
	evmWallet := "0x1111111111111111111111111111111111111111"
	solanaWallet := "9xQeWvG816bUx9EPjHmaT23yvVM2ZW57QbZz3Mz1Yw7"
	defaults := okxDeFiDefaultChainIndexes()

	evmChains := okxDeFiCompatibleChainIndexes(evmWallet, defaults)
	for _, chainIndex := range evmChains {
		if chainIndex == "501" {
			t.Fatalf("expected EVM wallet to exclude Solana, got %+v", evmChains)
		}
	}
	if len(evmChains) == 0 {
		t.Fatalf("expected EVM wallet to keep EVM chains")
	}

	solanaChains := okxDeFiCompatibleChainIndexes(solanaWallet, defaults)
	if len(solanaChains) != 1 || solanaChains[0] != "501" {
		t.Fatalf("expected Solana wallet to keep only 501, got %+v", solanaChains)
	}

	if got := okxDeFiCompatibleChainIndexes(evmWallet, []string{"501"}); len(got) != 0 {
		t.Fatalf("expected incompatible requested chain to be rejected, got %+v", got)
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

func TestOKXDeFiNormalizeOverview_AssetUpdating(t *testing.T) {
	raw := json.RawMessage(`[
		{
			"walletIdPlatformList": [
				{
					"walletAddress": "0x1111111111111111111111111111111111111111",
					"assetStatus": "2",
					"totalAssets": "0",
					"platformList": []
				}
			]
		}
	]`)

	out, err := okxDeFiNormalizeOverview("0x1111111111111111111111111111111111111111", []string{"56"}, raw, time.Unix(1700000000, 0).UTC())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if out.Status != "updating" || out.AssetStatus != "2" {
		t.Fatalf("expected updating status, got %+v", out)
	}
	if len(out.Warnings) == 0 || out.Warnings[0] != "OKX DeFi asset data is still updating" {
		t.Fatalf("expected updating warning, got %+v", out.Warnings)
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
